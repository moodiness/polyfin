package jellyfin

import (
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/userdata"
)

// contentServer is a test server with the ratings addon's titles, an
// administrator and a member, signed in, and the identifiers of the
// libraries and titles by name.
type contentServer struct {
	testServer
	admin, member           accounts.User
	adminToken, memberToken string
	ids                     map[string]string
}

func newContentServer(t *testing.T) contentServer {
	t.Helper()
	s := contentServer{testServer: newTestServer(t, 10)}
	s.admin = s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	s.member = s.user("member", nil)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), ratingsAddon(t), false); err != nil {
		t.Fatal(err)
	}
	s.adminToken, s.memberToken = s.signIn("admin", "tv"), s.signIn("member", "tablet")
	s.ids = map[string]string{}
	var views QueryResult
	s.get(t, "/UserViews", s.adminToken, &views)
	for _, view := range views.Items {
		s.ids[view.Name] = view.Id
		var page QueryResult
		s.get(t, "/Items?ParentId="+view.Id, s.adminToken, &page)
		for _, item := range page.Items {
			s.ids[item.Name] = item.Id
		}
	}
	if s.ids["Top"] == "" || s.ids["Shows"] == "" || s.ids["Restricted"] == "" {
		t.Fatalf("titles: %v", s.ids)
	}
	return s
}

// policy reads a user's policy as an administrator's app does.
func (s contentServer) policy(t *testing.T, user accounts.User) map[string]any {
	t.Helper()
	var dto map[string]any
	if status := s.get(t, "/Users/"+user.ID.String(), s.adminToken, &dto); status != http.StatusOK {
		t.Fatalf("user: %d", status)
	}
	return dto["Policy"].(map[string]any)
}

// setPolicy posts a user's policy, changed, as an administrator's app does.
func (s contentServer) setPolicy(t *testing.T, user accounts.User, change func(map[string]any)) (int, []byte) {
	t.Helper()
	policy := s.policy(t, user)
	change(policy)
	encoded, _ := json.Marshal(policy)
	return s.postRaw("/Users/"+user.ID.String()+"/Policy", app("tv", s.adminToken), string(encoded))
}

func (s contentServer) names(t *testing.T, token, path string) []string {
	t.Helper()
	var page QueryResult
	if status := s.get(t, path, token, &page); status != http.StatusOK {
		t.Fatalf("%s: %d", path, status)
	}
	return itemNames(page.Items)
}

func TestHiddenLibrariesLeaveTheUsersViews(t *testing.T) {
	s := newContentServer(t)
	policy := s.policy(t, s.member)
	if policy["EnableAllFolders"] != true || !reflect.DeepEqual(policy["EnabledFolders"], []any{}) {
		t.Fatalf("default folders: %v %v", policy["EnableAllFolders"], policy["EnabledFolders"])
	}
	// jellyfin-web's library access page unticks Top.
	status, body := s.setPolicy(t, s.member, func(p map[string]any) {
		p["EnableAllFolders"], p["EnabledFolders"] = false, []string{s.ids["Shows"]}
	})
	if status != http.StatusNoContent {
		t.Fatalf("hiding Top: %d %s", status, body)
	}
	policy = s.policy(t, s.member)
	if policy["EnableAllFolders"] != false || !reflect.DeepEqual(policy["EnabledFolders"], []any{s.ids["Shows"]}) {
		t.Errorf("folders read back: %v %v", policy["EnableAllFolders"], policy["EnabledFolders"])
	}
	if got := s.names(t, s.memberToken, "/UserViews"); slices.Contains(got, "Top") || !slices.Contains(got, "Shows") {
		t.Errorf("member's views: %v", got)
	}
	// Listing it is refused as Jellyfin refuses a folder the user may not
	// access.
	status, body = s.call(http.MethodGet, "/Items?parentId="+s.ids["Top"], app("tablet", s.memberToken), nil)
	var message string
	if status != http.StatusUnauthorized || json.Unmarshal(body, &message) != nil || message != "member is not permitted to access Library Top." {
		t.Errorf("listing a hidden library: %d %s", status, body)
	}
	// Its titles stay reachable.
	if status := s.get(t, "/Items/"+s.ids["Restricted"], s.memberToken, nil); status != http.StatusOK {
		t.Errorf("a title of the hidden library: %d", status)
	}
	if got := s.names(t, s.memberToken, "/Items?recursive=true&searchTerm=restricted&includeItemTypes=Movie"); !slices.Equal(got, []string{"Restricted"}) {
		t.Errorf("search with Top hidden: %v", got)
	}
	// The administrator's views are their own.
	if got := s.names(t, s.adminToken, "/UserViews"); !slices.Contains(got, "Top") {
		t.Errorf("administrator's views: %v", got)
	}

	// An identifier that is none is refused as Jellyfin refuses it.
	status, body = s.setPolicy(t, s.member, func(p map[string]any) { p["EnabledFolders"] = []string{"nope"} })
	var problem problemDetails
	if status != http.StatusBadRequest || json.Unmarshal(body, &problem) != nil || problem.Errors["$.EnabledFolders[0]"] == nil {
		t.Errorf("malformed folder: %d %s", status, body)
	}
	// Every folder again.
	if status, body := s.setPolicy(t, s.member, func(p map[string]any) { p["EnableAllFolders"] = true }); status != http.StatusNoContent {
		t.Fatalf("showing every library: %d %s", status, body)
	}
	if got := s.names(t, s.memberToken, "/UserViews"); !slices.Contains(got, "Top") {
		t.Errorf("member's views with every library: %v", got)
	}
	if stored, _ := s.store.User(t.Context(), s.member.ID); len(stored.HiddenLibraries) != 0 {
		t.Errorf("hidden libraries left: %v", stored.HiddenLibraries)
	}
}

func TestBlockedGenresHideTitlesFromApps(t *testing.T) {
	s := newContentServer(t)
	kid := s.user("kid", func(c *accounts.UserChanges) { c.BlockedGenres = &[]string{"crime"} })
	kidToken := s.signIn("kid", "phone")
	// Jellyfin's tags are not genres: the policy blocks none.
	if policy := s.policy(t, kid); !reflect.DeepEqual(policy["BlockedTags"], []any{}) {
		t.Errorf("blocked tags: %v", policy["BlockedTags"])
	}

	if got := s.names(t, kidToken, "/Items?parentId="+s.ids["Top"]); !slices.Equal(got, []string{"Allowed", "Unrated"}) {
		t.Errorf("kid's movies: %v", got)
	}
	if got := s.names(t, kidToken, "/Items?parentId="+s.ids["Shows"]); len(got) != 0 {
		t.Errorf("kid's shows: %v", got)
	}
	if got := s.names(t, kidToken, "/Items?recursive=true&searchTerm=restricted&includeItemTypes=Movie"); len(got) != 0 {
		t.Errorf("kid's search: %v", got)
	}
	for _, title := range []string{"Restricted", "Show"} {
		if status := s.get(t, "/Items/"+s.ids[title], kidToken, nil); status != http.StatusNotFound {
			t.Errorf("kid opening %s: %d", title, status)
		}
	}
	if status, _ := s.call(http.MethodPost, "/Items/"+s.ids["Restricted"]+"/PlaybackInfo", app("phone", kidToken), map[string]any{}); status != http.StatusNotFound {
		t.Errorf("kid's playback info of Restricted: %d", status)
	}
	download := func(token, title string) int {
		response, _ := fetchURL(t, s.url+"/Items/"+s.ids[title]+"/Download?ApiKey="+token, nil)
		return response.StatusCode
	}
	if status := download(kidToken, "Restricted"); status != http.StatusNotFound {
		t.Errorf("kid downloading Restricted: %d", status)
	}
	if status := download(kidToken, "Allowed"); status != http.StatusOK {
		t.Errorf("kid downloading Allowed: %d", status)
	}
	if status := s.get(t, "/Items/"+s.ids["Restricted"], s.memberToken, nil); status != http.StatusOK {
		t.Errorf("member opening Restricted: %d", status)
	}

	// Continue Watching leaves out what the kid may not see.
	now := time.Now()
	for _, title := range []string{"Restricted", "Allowed"} {
		id, _ := accounts.ParseID(s.ids[title])
		if _, err := s.handler.UserData.Change(t.Context(), kid.ID, []userdata.Item{{ID: id}}, func(d *userdata.Data) {
			d.Position, d.Runtime, d.LastPlayed = 10*time.Minute, 2*time.Hour, &now
		}); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.names(t, kidToken, "/UserItems/Resume"); !slices.Equal(got, []string{"Allowed"}) {
		t.Errorf("kid's Continue Watching: %v", got)
	}

	// People and similar titles: the administrator opens both dramas,
	// which records their credits.
	people := map[string]string{}
	for _, title := range []string{"Allowed", "Restricted"} {
		var item BaseItemDto
		if status := s.get(t, "/Items/"+s.ids[title], s.adminToken, &item); status != http.StatusOK || item.People == nil {
			t.Fatalf("%s: %d", title, status)
		}
		for _, person := range *item.People {
			people[person.Name] = person.Id
		}
	}
	var ann namedItemDto
	if status := s.get(t, "/Items/"+people["Ann Lee"], kidToken, &ann); status != http.StatusOK || ann.MovieCount != 1 {
		t.Errorf("Ann Lee for the kid: %d, %d movies", status, ann.MovieCount)
	}
	if status := s.get(t, "/Items/"+people["Rex Only"], kidToken, nil); status != http.StatusNotFound {
		t.Errorf("Rex Only for the kid: %d", status)
	}
	if got := s.names(t, kidToken, "/Items/"+s.ids["Allowed"]+"/Similar"); slices.Contains(got, "Restricted") {
		t.Errorf("similar titles for the kid: %v", got)
	}
}

func TestAllowedHours(t *testing.T) {
	s := newContentServer(t)
	var clock atomic.Pointer[time.Time]
	set := func(at time.Time) { clock.Store(&at) }
	set(time.Now())
	s.handler.now = func() time.Time { return *clock.Load() }
	monday := func(hour, minute int) time.Time {
		day := time.Date(2026, time.October, 5, hour, minute, 0, 0, time.Local)
		for day.Weekday() != time.Monday {
			day = day.AddDate(0, 0, 1)
		}
		return day
	}

	// jellyfin-web's schedule editor sends the hours as strings.
	status, body := s.setPolicy(t, s.member, func(p map[string]any) {
		p["AccessSchedules"] = []any{
			map[string]any{"DayOfWeek": "Weekday", "StartHour": "9", "EndHour": "17.5"},
			map[string]any{"DayOfWeek": 9, "StartHour": 10, "EndHour": 12},
		}
	})
	if status != http.StatusNoContent {
		t.Fatalf("setting the schedules: %d %s", status, body)
	}
	member := s.member.ID.String()
	want := []any{
		map[string]any{"Id": float64(1), "UserId": member, "DayOfWeek": "Weekday", "StartHour": float64(9), "EndHour": 17.5},
		map[string]any{"Id": float64(2), "UserId": member, "DayOfWeek": "Weekend", "StartHour": float64(10), "EndHour": float64(12)},
	}
	if got := s.policy(t, s.member)["AccessSchedules"]; !reflect.DeepEqual(got, want) {
		t.Errorf("schedules read back: %v", got)
	}

	memberApp := app("tablet", s.memberToken)
	signIn := func(name string) int {
		status, _ := s.call(http.MethodPost, "/Users/AuthenticateByName", app("laptop", ""), map[string]string{"Username": name, "Pw": "correct horse"})
		return status
	}
	download := func() int {
		response, _ := fetchURL(t, s.url+"/Items/"+s.ids["Allowed"]+"/Download?ApiKey="+s.memberToken, nil)
		return response.StatusCode
	}
	for _, tc := range []struct {
		at      time.Time
		allowed bool
		when    string
	}{
		{monday(9, 0), true, "Monday at the start hour"},
		{monday(17, 30), true, "Monday at the half-hour end"},
		{monday(17, 31), false, "Monday after the end"},
		{monday(8, 59), false, "Monday before the start"},
		{monday(11, 0).AddDate(0, 0, 5), true, "Saturday in the weekend's hours"},
		{monday(13, 0).AddDate(0, 0, 6), false, "Sunday after the weekend's hours"},
	} {
		set(tc.at)
		wantStatus := map[bool]int{true: http.StatusOK, false: http.StatusForbidden}[tc.allowed]
		if got := signIn("member"); got != wantStatus {
			t.Errorf("%s: sign-in %d, want %d", tc.when, got, wantStatus)
		}
		status, body := s.call(http.MethodGet, "/UserViews", memberApp, nil)
		if status != wantStatus || (!tc.allowed && len(body) != 0) {
			t.Errorf("%s: views %d %q, want %d", tc.when, status, body, wantStatus)
		}
		if got := download(); tc.allowed != (got == http.StatusOK) {
			t.Errorf("%s: download %d", tc.when, got)
		}
	}

	// Outside the hours, the app still reads the user and the server, as
	// Jellyfin lets it.
	set(monday(20, 0))
	for _, path := range []string{"/Users/" + member, "/System/Info"} {
		if status, _ := s.call(http.MethodGet, path, memberApp, nil); status != http.StatusOK {
			t.Errorf("%s outside the hours: %d", path, status)
		}
	}
	if status, _ := s.call(http.MethodGet, "/Users/Me", memberApp, nil); status != http.StatusForbidden {
		t.Errorf("/Users/Me outside the hours: %d", status)
	}
	// An administrator with hours is refused sign-in, but their apps are
	// served.
	if _, err := s.store.UpdateUser(t.Context(), s.admin.ID, accounts.UserChanges{
		AccessSchedules: &[]accounts.AccessSchedule{{Day: "Weekday", StartHour: 9, EndHour: 17}},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if got := signIn("admin"); got != http.StatusForbidden {
		t.Errorf("administrator signing in outside the hours: %d", got)
	}
	if status, _ := s.call(http.MethodGet, "/UserViews", app("tv", s.adminToken), nil); status != http.StatusOK {
		t.Errorf("administrator's app outside the hours: %d", status)
	}

	// Days and hours Jellyfin's policy cannot hold are refused.
	for name, schedule := range map[string]map[string]any{
		"$.AccessSchedules[0].DayOfWeek": {"DayOfWeek": "Someday", "StartHour": 1, "EndHour": 2},
		"$.AccessSchedules[0]":           {"DayOfWeek": "Sunday", "StartHour": 20, "EndHour": 25},
	} {
		status, body := s.setPolicy(t, s.member, func(p map[string]any) { p["AccessSchedules"] = []any{schedule} })
		var problem problemDetails
		if status != http.StatusBadRequest || json.Unmarshal(body, &problem) != nil || problem.Errors[name] == nil {
			t.Errorf("%v: %d %s", schedule, status, body)
		}
	}
	// A policy without schedules lifts them, as it replaces the whole
	// policy.
	if status, body := s.setPolicy(t, s.member, func(p map[string]any) { delete(p, "AccessSchedules") }); status != http.StatusNoContent {
		t.Fatalf("lifting the hours: %d %s", status, body)
	}
	if got := signIn("member"); got != http.StatusOK {
		t.Errorf("sign-in without hours: %d", got)
	}
}
