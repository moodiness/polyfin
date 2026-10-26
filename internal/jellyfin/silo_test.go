package jellyfin

import (
	"bytes"
	"cmp"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/homerows"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/silo"
	"github.com/moodiness/polyfin/internal/silo/silotest"
	"github.com/moodiness/polyfin/internal/stremio"
)

// The tests of Silo's API (package silo), which answers Silo's apps from
// the Jellyfin API's answers: they live beside the Jellyfin API's tests,
// whose servers and addons they use.

// siloApps serves a test server's Silo API on its own address.
type siloApps struct {
	testServer
	site string
}

func servingSilo(t *testing.T, s testServer) siloApps {
	t.Helper()
	api := silo.New(silo.Options{ServerID: testServerID, Version: "test", Accounts: s.store, Jellyfin: s.handler, DB: s.pool,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	site := httptest.NewServer(api)
	t.Cleanup(site.Close)
	return siloApps{testServer: s, site: site.URL}
}

// allowSilo turns the setting on.
func allowSilo(t *testing.T, s testServer) {
	t.Helper()
	s.setting(t, func(settings *accounts.Settings) { settings.SiloApps = true })
}

// do sends a request as Silo's apps do: the bearer token, the profile,
// and the device's identity.
func (a siloApps) do(t *testing.T, method, path, token, profile string, body any) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	request, _ := http.NewRequestWithContext(t.Context(), method, a.site+path, reader)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if profile != "" {
		request.Header.Set("X-Profile-Id", profile)
	}
	request.Header.Set("X-Silo-Device-Id", "pixel-1")
	request.Header.Set("X-Silo-Device-Name", "Pixel")
	request.Header.Set("X-Silo-Client", "Silo for Android")
	request.Header.Set("X-Silo-Client-Version", "1.2.3")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	return response.StatusCode, data
}

// get decodes a successful answer into into, once it met the rules the
// apps apply to the answers of its operation.
func (a siloApps) get(t *testing.T, path, token, profile string, into any) {
	t.Helper()
	status, data := a.do(t, http.MethodGet, path, token, profile, nil)
	if status != http.StatusOK || json.Unmarshal(data, into) != nil {
		t.Fatalf("%s: %d %s", path, status, data)
	}
	if err := appsAccept(path, data); err != nil {
		t.Errorf("%s: %v: %s", path, err, data)
	}
}

// appsAccept checks an answer against the rules the apps apply to the
// answers of its operation (see package silotest).
func appsAccept(path string, data []byte) error {
	target, _ := url.Parse(path)
	segments := strings.Split(strings.TrimPrefix(target.Path, "/api/v2/"), "/")
	switch {
	case target.Path == "/api/v2/user/libraries":
		return silotest.CheckLibraries(data)
	case target.Path == "/api/v2/home/sections", len(segments) == 3 && segments[0] == "library" && segments[2] == "sections":
		return silotest.CheckSections(data)
	case len(segments) == 4 && segments[0] == "home" && segments[3] == "items":
		return silotest.CheckSection(data, segments[2])
	case len(segments) == 5 && segments[0] == "library" && segments[2] == "sections" && segments[4] == "items":
		return silotest.CheckSection(data, segments[3])
	case target.Path == "/api/v2/catalog":
		return silotest.CheckCatalogPage(data)
	case len(segments) == 3 && segments[0] == "library" && segments[2] == "collections":
		return silotest.CheckCollectionTab(data, segments[1])
	case target.Path == "/api/v2/playback/capabilities":
		return silotest.CheckCapabilities(data)
	case target.Path == "/api/v2/favorites", target.Path == "/api/v2/watchlist":
		return silotest.CheckPersonalList(data)
	case len(segments) == 2 && (segments[0] == "favorites" || segments[0] == "watchlist"):
		return silotest.CheckMembershipEntry(data, segments[1])
	case len(segments) == 3 && segments[0] == "catalog" && segments[1] == "people":
		return silotest.CheckPerson(data, segments[2])
	case len(segments) == 3 && segments[0] == "catalog" && segments[1] == "items":
		return silotest.CheckItemDetail(data, segments[2])
	case len(segments) == 3 && segments[0] == "recommendations" && segments[1] == "similar":
		limit, err := strconv.Atoi(cmp.Or(target.Query().Get("limit"), "50"))
		if err != nil {
			return err
		}
		return silotest.CheckCards(data, limit)
	case target.Path == "/api/v2/recommendations/discover":
		return silotest.CheckDiscover(data)
	case target.Path == "/api/v2/calendar":
		return silotest.CheckCalendar(data)
	case target.Path == "/api/v2/images/capabilities":
		return silotest.CheckImageCapabilities(data)
	case (strings.HasSuffix(target.Path, "/capabilities") || strings.HasSuffix(target.Path, "/status")) &&
		!slices.Contains([]string{"/api/v2/catalog/search/capabilities", "/api/v2/shuffles/capabilities",
			"/api/v2/collections/capabilities"}, target.Path):
		return silotest.CheckUnsupported(data, target.Path)
	}
	return nil
}

// signIn signs a user in, returning the access token and the profile.
func (a siloApps) signIn(t *testing.T, name string) (token, profile string) {
	t.Helper()
	status, data := a.do(t, http.MethodPost, "/api/v2/auth/login", "", "", map[string]string{"username": name, "password": "correct horse"})
	var pair struct {
		AccessToken string `json:"access_token"`
		User        struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if status != http.StatusOK || json.Unmarshal(data, &pair) != nil || pair.AccessToken == "" {
		t.Fatalf("sign-in of %s: %d %s", name, status, data)
	}
	return pair.AccessToken, pair.User.ID
}

// problemCode is the code of an error answer: the last segment of its
// type.
func problemCode(data []byte) string {
	var problem struct{ Type string }
	_ = json.Unmarshal(data, &problem)
	return problem.Type[strings.LastIndex(problem.Type, "/")+1:]
}

func TestSiloAppsAnswerNothingWhileNotAllowed(t *testing.T) {
	t.Parallel()
	a := servingSilo(t, newTestServer(t, 10))
	a.user("member", nil)
	for _, path := range []string{"/api/v2/system/info", "/api/v2/system/identity", "/api/v2/auth/login"} {
		if status, data := a.do(t, http.MethodGet, path, "", "", nil); status != http.StatusNotFound || len(data) != 0 {
			t.Errorf("%s while not allowed: %d %q", path, status, data)
		}
	}
	if status, data := a.do(t, http.MethodPost, "/api/v2/auth/login", "", "", map[string]string{"username": "member", "password": "correct horse"}); status != http.StatusNotFound || len(data) != 0 {
		t.Errorf("sign-in while not allowed: %d %q", status, data)
	}

	allowSilo(t, a.testServer)
	var info struct {
		APIMajor int `json:"api_major"`
	}
	a.get(t, "/api/v2/system/info", "", "", &info)
	if info.APIMajor != 2 {
		t.Errorf("api_major %d", info.APIMajor)
	}
	var identity struct {
		ServerID string `json:"server_id"`
	}
	a.get(t, "/api/v2/system/identity", "", "", &identity)
	if identity.ServerID != testServerID {
		t.Errorf("server_id %q", identity.ServerID)
	}
	var setup struct {
		NeedsSetup bool `json:"needs_setup"`
	}
	a.get(t, "/api/v2/system/setup", "", "", &setup)
	if required, _ := a.store.SetupRequired(t.Context()); setup.NeedsSetup != required {
		t.Errorf("needs_setup %v, setup required %v", setup.NeedsSetup, required)
	}
	if status, data := a.do(t, http.MethodGet, "/api/v2/requests", "", "", nil); status != http.StatusNotFound || problemCode(data) != "not_found" {
		t.Errorf("an operation Polyfin does not serve: %d %s", status, data)
	}
}

// Silo's apps sign in as Jellyfin apps do: the same refusals, the same
// throttle, and a device of their own among the user's.
func TestSiloAppsSignInAsJellyfinAppsDo(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 3)
	allowSilo(t, s)
	a := servingSilo(t, s)
	alice := s.user("alice", nil)
	s.user("carol", func(c *accounts.UserChanges) { c.IsDisabled = new(true) })
	s.user("dora", func(c *accounts.UserChanges) {
		c.AccessSchedules = &[]accounts.AccessSchedule{{Day: "Weekday", StartHour: 9, EndHour: 17}}
	})
	s.user("root", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	s.setting(t, func(settings *accounts.Settings) { settings.TwoStepAdministrators = true })
	// A Sunday night: dora is outside her hours.
	sunday := time.Date(2026, 10, 25, 3, 0, 0, 0, time.Local)
	s.handler.now = func() time.Time { return sunday }

	token, profile := a.signIn(t, "alice")
	device, user, err := s.store.DeviceByToken(t.Context(), token, "127.0.0.1")
	if err != nil || user.ID != alice.ID || device.Client != "Silo for Android" || device.DeviceName != "Pixel" || device.DeviceID != "pixel-1" {
		t.Errorf("device signed in: %+v of %s, %v", device.DeviceInfo, user.Name, err)
	}
	var me struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	a.get(t, "/api/v2/account/me", token, "", &me)
	if me.ID != alice.ID.String() || me.Username != "alice" || me.Role != "user" || profile != me.ID {
		t.Errorf("account: %+v, profile %s", me, profile)
	}
	var profiles struct {
		Items []struct {
			ID, Name  string
			IsPrimary bool `json:"is_primary"`
			HasPin    bool `json:"has_pin"`
		} `json:"items"`
	}
	a.get(t, "/api/v2/profiles", token, "", &profiles)
	if len(profiles.Items) != 1 || profiles.Items[0].ID != alice.ID.String() || profiles.Items[0].Name != "alice" ||
		!profiles.Items[0].IsPrimary || profiles.Items[0].HasPin {
		t.Errorf("profiles: %+v", profiles.Items)
	}
	if status, data := a.do(t, http.MethodGet, "/api/v2/account/me", "", "", nil); status != http.StatusUnauthorized ||
		problemCode(data) != "authentication_required" {
		t.Errorf("without a token: %d %s", status, data)
	}
	if status, data := a.do(t, http.MethodGet, "/api/v2/home/sections", token, strings.Repeat("ab", 16), nil); status != http.StatusForbidden ||
		problemCode(data) != "permission_denied" {
		t.Errorf("another profile: %d %s", status, data)
	}
	if status, data := a.do(t, http.MethodGet, "/api/v2/home/sections", token, "", nil); status != http.StatusUnprocessableEntity {
		t.Errorf("no profile: %d %s", status, data)
	}

	login := func(name, password string) (int, string) {
		t.Helper()
		status, data := a.do(t, http.MethodPost, "/api/v2/auth/login", "", "", map[string]string{"username": name, "password": password})
		return status, problemCode(data)
	}
	for _, tc := range []struct {
		name, user, password string
		status               int
		code                 string
	}{
		{"disabled account", "carol", "correct horse", http.StatusForbidden, "permission_denied"},
		{"outside the hours", "dora", "correct horse", http.StatusForbidden, "permission_denied"},
		{"wrong password", "alice", "wrong horse", http.StatusUnauthorized, "invalid_token"},
		// Silo's apps have no field for a code: two-step sign-in refuses
		// the password alone, as for Jellyfin apps.
		{"two-step sign-in", "root", "correct horse", http.StatusUnauthorized, "invalid_token"},
		{"unknown user", "nobody", "correct horse", http.StatusUnauthorized, "invalid_token"},
		// Three failures: the client waits, whatever it sends.
		{"throttled", "alice", "correct horse", http.StatusTooManyRequests, "rate_limited"},
	} {
		if status, code := login(tc.user, tc.password); status != tc.status || code != tc.code {
			t.Errorf("%s: %d %s", tc.name, status, code)
		}
	}

	// The refresh token is the session's: it stands until the device
	// signs out.
	var refreshed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	status, data := a.do(t, http.MethodPost, "/api/v2/auth/refresh", "", "", map[string]string{"refresh_token": token})
	if status != http.StatusOK || json.Unmarshal(data, &refreshed) != nil || refreshed.AccessToken != token || refreshed.ExpiresIn <= 0 {
		t.Errorf("refresh: %d %s", status, data)
	}
	if status, _ := a.do(t, http.MethodPost, "/api/v2/auth/logout", token, "", nil); status != http.StatusNoContent {
		t.Errorf("logout: %d", status)
	}
	if status, data := a.do(t, http.MethodPost, "/api/v2/auth/refresh", "", "", map[string]string{"refresh_token": token}); status != http.StatusUnauthorized ||
		problemCode(data) != "invalid_grant" {
		t.Errorf("refresh after logout: %d %s", status, data)
	}
	if status, _ := a.do(t, http.MethodGet, "/api/v2/account/me", token, "", nil); status != http.StatusUnauthorized {
		t.Errorf("account after logout: %d", status)
	}
}

// A signed-in session of an account disabled since is refused, as Jellyfin
// apps' are.
func TestSiloAppsSessionsEndWithTheirAccount(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	allowSilo(t, s)
	a := servingSilo(t, s)
	bob := s.user("bob", nil)
	token, profile := a.signIn(t, "bob")
	if status, _ := a.do(t, http.MethodGet, "/api/v2/user/libraries", token, profile, nil); status != http.StatusOK {
		t.Fatalf("libraries: %d", status)
	}
	s.limit(t, bob, func(c *accounts.UserChanges) { c.IsDisabled = new(true) })
	if status, data := a.do(t, http.MethodGet, "/api/v2/user/libraries", token, profile, nil); status != http.StatusUnauthorized ||
		problemCode(data) != "invalid_token" {
		t.Errorf("disabled since: %d %s", status, data)
	}
}

type siloCard struct {
	ContentID       string   `json:"content_id"`
	Title           string   `json:"title"`
	Type            string   `json:"type"`
	Genres          []string `json:"genres"`
	PositionSeconds float64  `json:"position_seconds"`
	ItemSource      string   `json:"item_source"`
	UserState       *struct {
		Played bool `json:"played"`
	} `json:"user_state"`
}

type siloPage struct {
	Items []siloCard `json:"items"`
	Page  struct {
		HasMore    bool   `json:"has_more"`
		NextCursor string `json:"next_cursor"`
	} `json:"page"`
	Total int `json:"total"`
}

func cardTitles(cards []siloCard) []string {
	var titles []string
	for _, card := range cards {
		titles = append(titles, card.Title)
	}
	return titles
}

type siloLibrary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

func (a siloApps) libraries(t *testing.T, token, profile string) []siloLibrary {
	t.Helper()
	var libraries struct {
		Items []siloLibrary `json:"items"`
	}
	a.get(t, "/api/v2/user/libraries", token, profile, &libraries)
	return libraries.Items
}

// Silo's apps browse what Jellyfin apps browse, under the same limits.
func TestSiloAppsBrowseUnderTheUsersLimits(t *testing.T) {
	t.Parallel()
	p := playing(t)
	allowSilo(t, p.testServer)
	a := servingSilo(t, p.testServer)
	token, profile := a.signIn(t, "member")

	libraries := a.libraries(t, token, profile)
	if len(libraries) != 1 || libraries[0].Type != "movies" {
		t.Fatalf("libraries: %+v", libraries)
	}
	number, err := strconv.Atoi(libraries[0].ID)
	if err != nil || number <= 0 || strconv.Itoa(number) != libraries[0].ID {
		t.Errorf("a library's number: %q", libraries[0].ID)
	}
	if again := a.libraries(t, token, profile); again[0].ID != libraries[0].ID {
		t.Errorf("the library's number changed: %s then %s", libraries[0].ID, again[0].ID)
	}
	library := "/api/v2/catalog?library_id=" + libraries[0].ID
	var first, second siloPage
	a.get(t, library+"&limit=1", token, profile, &first)
	if cardTitles(first.Items) == nil || !first.Page.HasMore || first.Page.NextCursor == "" || first.Total != 2 {
		t.Fatalf("first page: %+v", first)
	}
	a.get(t, library+"&limit=1&cursor="+first.Page.NextCursor, token, profile, &second)
	if all := append(cardTitles(first.Items), cardTitles(second.Items)...); !slices.Equal(all, []string{"Movie", "Remote"}) || second.Page.HasMore {
		t.Errorf("pages: %v, more %v", all, second.Page.HasMore)
	}
	// The library's landing has its latest titles, also read alone; the
	// sections of another library are not its own.
	var landing struct {
		Sections []struct {
			ID    string     `json:"id"`
			Items []siloCard `json:"items"`
		} `json:"sections"`
	}
	a.get(t, "/api/v2/library/"+libraries[0].ID+"/sections", token, profile, &landing)
	latest := "latest-" + libraries[0].ID
	if len(landing.Sections) != 1 || landing.Sections[0].ID != latest ||
		!slices.Equal(slices.Sorted(slices.Values(cardTitles(landing.Sections[0].Items))), []string{"Movie", "Remote"}) {
		t.Fatalf("landing: %+v", landing.Sections)
	}
	var alone struct {
		Items []siloCard `json:"items"`
	}
	a.get(t, "/api/v2/library/"+libraries[0].ID+"/sections/"+latest+"/items", token, profile, &alone)
	if !slices.Equal(cardTitles(alone.Items), cardTitles(landing.Sections[0].Items)) {
		t.Errorf("the latest alone: %v", cardTitles(alone.Items))
	}
	if status, _ := a.do(t, http.MethodGet, "/api/v2/library/"+libraries[0].ID+"/sections/latest-999/items", token, profile, nil); status != http.StatusNotFound {
		t.Errorf("another library's section: %d", status)
	}
	var detail struct {
		siloCard
		ContentRating string `json:"content_rating"`
		Versions      []struct {
			FileID string `json:"file_id"`
		} `json:"versions"`
	}
	a.get(t, "/api/v2/catalog/items/"+p.movie, token, profile, &detail)
	if detail.Title != "Movie" || detail.Type != "movie" || !slices.Equal(detail.Genres, []string{"Drama"}) || detail.ContentRating != "PG" ||
		len(detail.Versions) != 2 || detail.Versions[0].FileID == detail.Versions[1].FileID {
		t.Errorf("details: %+v", detail)
	}
	// A search finds what the Jellyfin API's search finds.
	var found siloPage
	a.get(t, "/api/v2/catalog?q=Movie", token, profile, &found)
	var searched QueryResult
	p.get(t, "/Items?SearchTerm=Movie&Recursive=true&IncludeItemTypes=Movie,Series", p.token, &searched)
	var jellyfinTitles []string
	for _, item := range searched.Items {
		jellyfinTitles = append(jellyfinTitles, item.Name)
	}
	if !slices.Equal(cardTitles(found.Items), jellyfinTitles) {
		t.Errorf("search: %v, in the Jellyfin API %v", cardTitles(found.Items), jellyfinTitles)
	}

	// A title the user hid leaves the catalog, as it leaves Jellyfin
	// apps' lists.
	if status, _ := p.call(http.MethodPost, "/Polyfin/UserItems/"+p.remote+"/Hidden", app("tv", p.token), nil); status != http.StatusNoContent && status != http.StatusOK {
		t.Fatalf("hiding: %d", status)
	}
	var page siloPage
	a.get(t, library, token, profile, &page)
	if titles := cardTitles(page.Items); !slices.Equal(titles, []string{"Movie"}) {
		t.Errorf("with a hidden title: %v", titles)
	}

	// A blocked genre, then parental control, leave nothing; a hidden
	// library is not listed at all.
	p.limit(t, p.user, func(c *accounts.UserChanges) { c.BlockedGenres = &[]string{"Drama"} })
	a.get(t, library, token, profile, &page)
	if len(page.Items) != 0 {
		t.Errorf("with Drama blocked: %v", cardTitles(page.Items))
	}
	if status, _ := a.do(t, http.MethodGet, "/api/v2/catalog/items/"+p.movie, token, profile, nil); status != http.StatusNotFound {
		t.Errorf("a blocked title's page: %d", status)
	}
	p.limit(t, p.user, func(c *accounts.UserChanges) {
		c.BlockedGenres = &[]string{}
		c.Parental = &accounts.ParentalControl{MaxRating: new(0)}
	})
	a.get(t, library, token, profile, &page)
	if len(page.Items) != 0 {
		t.Errorf("under parental control: %v", cardTitles(page.Items))
	}
	var views QueryResult
	p.get(t, "/UserViews", p.token, &views)
	viewID, _ := accounts.ParseID(views.Items[0].Id)
	p.limit(t, p.user, func(c *accounts.UserChanges) {
		c.Parental = &accounts.ParentalControl{}
		c.HiddenLibraries = &[]accounts.ID{viewID}
	})
	if listed := a.libraries(t, token, profile); len(listed) != 0 {
		t.Errorf("a hidden library listed: %+v", listed)
	}
	if status, _ := a.do(t, http.MethodGet, library, token, profile, nil); status != http.StatusNotFound {
		t.Errorf("a hidden library's catalog: %d", status)
	}
}

// Series open on their seasons and episodes, as in Jellyfin apps.
func TestSiloAppsBrowseSeries(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	allowSilo(t, s)
	a := servingSilo(t, s)
	s.user("member", nil)
	var metas atomic.Int32
	if _, err := s.addons.Install(t.Context(), addons.Shared(), seriesAddon(t, &metas), false); err != nil {
		t.Fatal(err)
	}
	token, profile := a.signIn(t, "member")
	libraries := a.libraries(t, token, profile)
	if len(libraries) != 1 || libraries[0].Type != "shows" {
		t.Fatalf("libraries: %+v", libraries)
	}
	var page siloPage
	a.get(t, "/api/v2/catalog?library_id="+libraries[0].ID+"&limit=2", token, profile, &page)
	if len(page.Items) != 2 || page.Items[0].Type != "series" || page.Total != manySeries {
		t.Fatalf("series: %+v", page)
	}
	series := page.Items[0].ContentID
	var seasons struct {
		Items []struct {
			ContentID    string `json:"content_id"`
			SeasonNumber int    `json:"season_number"`
		} `json:"items"`
	}
	a.get(t, "/api/v2/catalog/series/"+series+"/seasons", token, profile, &seasons)
	if len(seasons.Items) != 1 || seasons.Items[0].SeasonNumber != 1 {
		t.Fatalf("seasons: %+v", seasons.Items)
	}
	type episodes struct {
		Items []struct {
			ContentID     string `json:"content_id"`
			SeasonNumber  int    `json:"season_number"`
			EpisodeNumber int    `json:"episode_number"`
		} `json:"items"`
	}
	var byNumber, bySeason episodes
	a.get(t, "/api/v2/catalog/series/"+series+"/seasons/1/episodes", token, profile, &byNumber)
	a.get(t, "/api/v2/catalog/items/"+seasons.Items[0].ContentID+"/episodes", token, profile, &bySeason)
	if len(byNumber.Items) != 2 || byNumber.Items[1].EpisodeNumber != 2 || byNumber.Items[1].SeasonNumber != 1 ||
		len(bySeason.Items) != 2 || bySeason.Items[0].ContentID != byNumber.Items[0].ContentID {
		t.Errorf("episodes: %+v, of the season %+v", byNumber.Items, bySeason.Items)
	}
	var none episodes
	a.get(t, "/api/v2/catalog/series/"+series+"/seasons/2/episodes", token, profile, &none)
	if len(none.Items) != 0 {
		t.Errorf("episodes of a season the series lacks: %+v", none.Items)
	}
}

// startBody is a start of playback as Silo's apps send it.
func siloStart(file, profile, installation string, containers, video, audio []string, more map[string]any) map[string]any {
	body := map[string]any{"protocol_version": 3, "client_features": []string{"playback_plan_v3"}, "file_id": file,
		"profile_id": profile, "installation_id": installation, "playback_attempt_id": "attempt-1", "quality_preference": "auto",
		"subtitle_fidelity_preference": "compatible",
		"client_capabilities": map[string]any{"audio_evidence": "declared", "video_evidence": "declared", "hdr": false,
			"containers": containers, "codecs_video": video, "codecs_video_hardware": []string{}, "codecs_audio": audio},
		"client_playback_context": map[string]any{"protocol_version": 3, "app_version": "1.2.3", "form_factor": "phone",
			"device": map[string]any{}, "output": map[string]any{}, "deliveries": map[string]any{
				"hls": map[string]any{"enabled": true, "supported_on_device": true, "containers": []string{"fmp4"}, "video_codecs": video,
					"audio_decode_codecs": audio, "audio_passthrough_codecs": []string{}, "auth_header_refresh": true, "features": []string{},
					"transformations": []string{}, "validated_claims": []string{}, "subtitles": map[string]any{"sidecar_text": true}}}}}
	for key, value := range more {
		body[key] = value
	}
	return body
}

type siloDecision struct {
	Outcome        string   `json:"outcome"`
	SessionID      string   `json:"session_id"`
	ServerFeatures []string `json:"server_features"`
	Terminal       *struct {
		Reason string `json:"reason"`
	} `json:"terminal"`
	Plan *struct {
		Delivery string `json:"delivery"`
		Stream   struct {
			URL      string `json:"url"`
			Protocol string `json:"protocol"`
		} `json:"stream"`
		SelectedTracks struct {
			Audio, Subtitle *struct {
				ID    string `json:"id"`
				Index int    `json:"index"`
			}
		} `json:"selected_tracks"`
		Subtitle struct {
			Mode     string `json:"mode"`
			Artifact *struct {
				URL    string `json:"url"`
				Format string `json:"format"`
			} `json:"artifact"`
			Inventory []struct {
				CombinedIndex int    `json:"combined_index"`
				Delivery      string `json:"delivery"`
				URL           string `json:"url"`
				Language      string `json:"language"`
			} `json:"inventory"`
		} `json:"subtitle"`
		RequestedFile string `json:"requested_media_file_id"`
		EffectiveFile string `json:"effective_media_file_id"`
	} `json:"playback_plan"`
}

// fileOf is the number Silo's apps know a title's version by.
func (a siloApps) fileOf(t *testing.T, item, token, profile string, version int) string {
	t.Helper()
	var watch struct {
		Versions []struct {
			FileID string `json:"file_id"`
		} `json:"versions"`
	}
	a.get(t, "/api/v2/watch/"+item, token, profile, &watch)
	if len(watch.Versions) <= version {
		t.Fatalf("versions: %+v", watch.Versions)
	}
	return watch.Versions[version].FileID
}

// installation is the installation the capabilities document tells,
// which meets the rules the apps check before they start a playback.
func (a siloApps) installation(t *testing.T, token, profile string) string {
	t.Helper()
	status, data := a.do(t, http.MethodGet, "/api/v2/playback/capabilities", token, profile, nil)
	if status != http.StatusOK {
		t.Fatalf("capabilities: %d %s", status, data)
	}
	if err := silotest.CheckCapabilities(data); err != nil {
		t.Fatalf("capabilities: %v: %s", err, data)
	}
	var capabilities struct {
		InstallationID string `json:"installation_id"`
	}
	_ = json.Unmarshal(data, &capabilities)
	return capabilities.InstallationID
}

// start starts a playback, whose decision meets the rules the apps check
// before they play one.
func (a siloApps) start(t *testing.T, token, profile string, body map[string]any) siloDecision {
	t.Helper()
	status, data := a.do(t, http.MethodPost, "/api/v2/playback/start", token, profile, body)
	var d siloDecision
	if status != http.StatusCreated || json.Unmarshal(data, &d) != nil {
		t.Fatalf("start: %d %s", status, data)
	}
	file, _ := body["file_id"].(string)
	if err := silotest.CheckDecision(data, []string{file}); err != nil {
		t.Errorf("decision: %v: %s", err, data)
	}
	return d
}

// fetch reads a media address of a plan with the app's bearer token.
func (a siloApps) fetch(t *testing.T, path, token string) (int, string) {
	t.Helper()
	status, data := a.do(t, http.MethodGet, path, token, "", nil)
	return status, string(data)
}

// A movie plays as it is for a client that takes its file, its text
// subtitle beside it as WebVTT; progress and stop are numbered, and the
// resume point is the one Jellyfin apps show.
func TestSiloAppsPlayAMovieAsItIs(t *testing.T) {
	t.Parallel()
	p := playing(t)
	// The version lasts as long as a movie, so that a stop keeps a resume
	// point.
	probe, err := os.ReadFile(filepath.Join(playbackFixtures, "probes", "h264-aac-mp4.json"))
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := media.Parse(probe)
	if err != nil {
		t.Fatal(err)
	}
	analysis.Remote, analysis.Duration = true, 2*time.Hour
	p.storeAnalysis(t, p.versions[1], analysis)
	allowSilo(t, p.testServer)
	a := servingSilo(t, p.testServer)
	token, profile := a.signIn(t, "member")
	installation := a.installation(t, token, profile)
	file := a.fileOf(t, p.movie, token, profile, 1)
	d := a.start(t, token, profile, siloStart(file, profile, installation, []string{"mp4"}, []string{"h264"}, []string{"aac"},
		map[string]any{"subtitle_track_index": 0}))
	if d.Outcome != "playable" || d.Plan == nil || d.SessionID == "" || !slices.Contains(d.ServerFeatures, "neutral_playback_v3_contract_v1") {
		t.Fatalf("decision: %+v", d)
	}
	plan := d.Plan
	if plan.Delivery != "original_http" || plan.Stream.Protocol != "http_progressive" || plan.Stream.URL != "/api/v2/stream/"+d.SessionID ||
		plan.RequestedFile != file || plan.EffectiveFile != file {
		t.Errorf("plan: %+v", plan)
	}
	if len(plan.Subtitle.Inventory) != 1 || plan.Subtitle.Inventory[0].Delivery != "sidecar" || plan.Subtitle.Mode != "render" ||
		plan.Subtitle.Artifact == nil || plan.Subtitle.Artifact.Format != "vtt" || plan.Subtitle.Artifact.URL != plan.Subtitle.Inventory[0].URL ||
		plan.SelectedTracks.Subtitle == nil || plan.SelectedTracks.Subtitle.Index != 0 {
		t.Errorf("subtitles: %+v, selected %+v", plan.Subtitle, plan.SelectedTracks.Subtitle)
	}
	if status, body := a.fetch(t, plan.Stream.URL, token); status != http.StatusOK || !strings.Contains(body, "media bytes") {
		t.Errorf("the file: %d %q", status, body)
	}
	if status, _ := a.fetch(t, plan.Stream.URL, ""); status != http.StatusUnauthorized {
		t.Errorf("the file without the token: %d", status)
	}
	if status, body := a.fetch(t, plan.Subtitle.Artifact.URL, token); status != http.StatusOK || !strings.HasPrefix(body, "WEBVTT") ||
		!strings.Contains(body, "Bonjour") {
		t.Errorf("the subtitle: %d %q", status, body)
	}

	progress := func(sequence int, position float64, paused bool) string {
		t.Helper()
		status, data := a.do(t, http.MethodPost, "/api/v2/playback/"+d.SessionID+"/progress", token, profile,
			map[string]any{"installation_id": installation, "sequence": sequence, "position": position, "is_paused": paused})
		var m struct{ Outcome string }
		if status != http.StatusOK || json.Unmarshal(data, &m) != nil {
			t.Fatalf("progress %d: %d %s", sequence, status, data)
		}
		return m.Outcome
	}
	for _, tc := range []struct {
		sequence int
		position float64
		paused   bool
		want     string
	}{{1, 30, false, "applied"}, {1, 30, false, "replayed"}, {1, 45, false, "stale_sample"}, {3, 300, true, "applied"}, {2, 200, false, "stale_sample"}} {
		if got := progress(tc.sequence, tc.position, tc.paused); got != tc.want {
			t.Errorf("sample %d at %v: %s, want %s", tc.sequence, tc.position, got, tc.want)
		}
	}
	stop := func() (int, string) {
		status, data := a.do(t, http.MethodDelete, "/api/v2/playback/"+d.SessionID, token, profile,
			map[string]any{"installation_id": installation, "stop_id": "stop-1", "sequence": 4, "position": 600.0, "is_paused": false})
		var m struct {
			Outcome string `json:"outcome"`
			StopID  string `json:"stop_id"`
		}
		_ = json.Unmarshal(data, &m)
		return status, m.Outcome + " " + m.StopID
	}
	if status, got := stop(); status != http.StatusOK || got != "stopped stop-1" {
		t.Errorf("stop: %d %s", status, got)
	}
	if status, got := stop(); status != http.StatusOK || got != "replayed stop-1" {
		t.Errorf("the same stop again: %d %s", status, got)
	}
	if status, _ := a.fetch(t, plan.Stream.URL, token); status != http.StatusNotFound {
		t.Errorf("the file once stopped: %d", status)
	}
	// Jellyfin apps resume where Silo's app stopped, and Silo's apps list
	// the movie in Continue Watching.
	var movie BaseItemDto
	p.get(t, "/Items/"+p.movie, p.token, &movie)
	if movie.UserData.PlaybackPositionTicks != 600*10_000_000 {
		t.Errorf("resume point in the Jellyfin API: %d", movie.UserData.PlaybackPositionTicks)
	}
	var sections struct {
		Sections []struct {
			ID    string     `json:"id"`
			Items []siloCard `json:"items"`
		} `json:"sections"`
	}
	a.get(t, "/api/v2/home/sections", token, profile, &sections)
	if len(sections.Sections) == 0 || sections.Sections[0].ID != "continue_watching" || len(sections.Sections[0].Items) != 1 ||
		sections.Sections[0].Items[0].ContentID != p.movie || sections.Sections[0].Items[0].PositionSeconds != 600 {
		t.Errorf("home: %+v", sections.Sections)
	}
	// The library's landing has it in its own Continue Watching.
	number := a.libraries(t, token, profile)[0].ID
	a.get(t, "/api/v2/library/"+number+"/sections", token, profile, &sections)
	if len(sections.Sections) == 0 || sections.Sections[0].ID != "resume-"+number || len(sections.Sections[0].Items) != 1 ||
		sections.Sections[0].Items[0].ContentID != p.movie {
		t.Errorf("the library's landing: %+v", sections.Sections)
	}

	// Marked played from Silo's app, played in the Jellyfin API.
	if status, _ := a.do(t, http.MethodPost, "/api/v2/watched/"+p.movie, token, profile, nil); status != http.StatusNoContent {
		t.Errorf("mark played: %d", status)
	}
	p.get(t, "/Items/"+p.movie, p.token, &movie)
	if !movie.UserData.Played {
		t.Errorf("not played in the Jellyfin API: %+v", movie.UserData)
	}
}

// A client that cannot take the file's container gets it repackaged to
// HLS; one that cannot take its video gets it converted, unless the user
// may not have video converted.
func TestSiloAppsPlayRepackagedOrConverted(t *testing.T) {
	t.Parallel()
	p := playing(t)
	p.remuxable(t)
	allowSilo(t, p.testServer)
	a := servingSilo(t, p.testServer)
	token, profile := a.signIn(t, "member")
	installation := a.installation(t, token, profile)
	file := a.fileOf(t, p.movie, token, profile, 0)

	d := a.start(t, token, profile, siloStart(file, profile, installation, []string{"mp4"}, []string{"h264"}, []string{"aac", "opus"}, nil))
	if d.Plan == nil || d.Plan.Delivery != "server_remux_hls" || d.Plan.Stream.Protocol != "hls" ||
		d.Plan.Stream.URL != "/api/v2/playback/transcode/"+d.SessionID+"/master.m3u8" {
		t.Fatalf("repackaged: %+v %+v", d, d.Plan)
	}
	status, master := a.fetch(t, d.Plan.Stream.URL, token)
	if status != http.StatusOK || !strings.HasPrefix(master, "#EXTM3U") {
		t.Fatalf("master playlist: %d %s", status, master)
	}
	variant := ""
	for line := range strings.SplitSeq(master, "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			variant = line
		}
	}
	if status, media := a.fetch(t, "/api/v2/playback/transcode/"+d.SessionID+"/"+variant, token); status != http.StatusOK ||
		!strings.Contains(media, "#EXT-X-ENDLIST") {
		t.Errorf("media playlist %s: %d %s", variant, status, media)
	}

	converted := a.start(t, token, profile, siloStart(file, profile, installation, []string{"mp4"}, []string{"hevc"}, []string{"aac"},
		map[string]any{"playback_attempt_id": "attempt-2"}))
	if converted.Plan == nil || converted.Plan.Delivery != "server_transcode_hls" {
		t.Errorf("converted: %+v", converted)
	}
	p.permit(t, p.user, false, false, false)
	refused := a.start(t, token, profile, siloStart(file, profile, installation, []string{"mp4"}, []string{"hevc"}, []string{"aac"},
		map[string]any{"playback_attempt_id": "attempt-3"}))
	if refused.Outcome != "adaptation_unavailable" || refused.Plan != nil || refused.SessionID != "" || refused.Terminal == nil {
		t.Errorf("without conversions allowed: %+v", refused)
	}
}

// Silo's apps list the collections of a library on its Collections tab,
// page through a collection's titles, and show a home row of one on Home
// with its page: each with the titles the user may see, as Jellyfin apps
// list them; a hidden or blocked title stays out. The apps' own
// collections cannot be made.
func TestSiloAppsListCollections(t *testing.T) {
	t.Parallel()
	p := playing(t)
	if _, err := p.store.CreateUser(t.Context(), accounts.NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true}); err != nil {
		t.Fatal(err)
	}
	picks := p.createCollection(t, p.signIn("admin", "tv"), "name=Picks&ids="+p.movie+","+p.remote)
	id, _ := accounts.ParseID(picks)
	p.addRow(t, homerows.Row{Name: "Our picks", Kind: homerows.KindCollection, Place: homerows.PlaceBottom, Collection: id, Everyone: true})
	allowSilo(t, p.testServer)
	a := servingSilo(t, p.testServer)
	token, profile := a.signIn(t, "member")
	libraries := a.libraries(t, token, profile)
	library := libraries[0].ID
	// Polyfin's Collections view is a library too, of the titles of its
	// collections.
	collections := ""
	for _, l := range libraries {
		if l.Name == "Collections" && l.Type == "mixed" {
			collections = l.ID
		}
	}
	if collections == "" {
		t.Fatalf("libraries: %+v", libraries)
	}

	type collectionCard struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		ItemCount int    `json:"item_count"`
	}
	tab := func() []collectionCard {
		t.Helper()
		var answer struct {
			LibraryID   string           `json:"library_id"`
			Collections []collectionCard `json:"collections"`
			Groups      []any            `json:"groups"`
		}
		a.get(t, "/api/v2/library/"+library+"/collections", token, profile, &answer)
		if answer.LibraryID != library || answer.Groups == nil {
			t.Errorf("tab: %+v", answer)
		}
		return answer.Collections
	}
	titles := func(path string) []string {
		t.Helper()
		var page siloPage
		a.get(t, path, token, profile, &page)
		return cardTitles(page.Items)
	}
	row := func() (string, []string) {
		t.Helper()
		var home struct {
			Sections []struct {
				ID    string     `json:"id"`
				Type  string     `json:"section_type"`
				Title string     `json:"title"`
				Items []siloCard `json:"items"`
			} `json:"sections"`
		}
		a.get(t, "/api/v2/home/sections", token, profile, &home)
		for _, section := range home.Sections {
			if section.Title == "Our picks" && section.Type == "collection" {
				return section.ID, cardTitles(section.Items)
			}
		}
		return "", nil
	}
	check := func(label string, want []string) {
		t.Helper()
		if cards := tab(); len(want) > 0 && (len(cards) != 1 || cards[0].ID != picks || cards[0].Title != "Picks" || cards[0].ItemCount != len(want)) {
			t.Errorf("%s: the tab %+v", label, cards)
		}
		var listed QueryResult
		p.get(t, "/Items?ParentId="+picks, p.token, &listed)
		var jellyfin []string
		for _, item := range listed.Items {
			jellyfin = append(jellyfin, item.Name)
		}
		if !slices.Equal(jellyfin, want) {
			t.Errorf("%s: Jellyfin apps list %v", label, jellyfin)
		}
		for _, path := range []string{"/api/v2/catalog?source=library_collection&collection_id=" + picks + "&library_id=" + library,
			"/api/v2/library/" + library + "/collections/" + picks + "/items"} {
			if got := titles(path); !slices.Equal(got, want) {
				t.Errorf("%s: %s lists %v", label, path, got)
			}
		}
		if got := titles("/api/v2/catalog?library_id=" + collections); !slices.Equal(got, want) {
			t.Errorf("%s: the Collections library lists %v", label, got)
		}
		var held struct {
			Collections []collectionCard `json:"collections"`
		}
		a.get(t, "/api/v2/library/"+collections+"/collections", token, profile, &held)
		if len(held.Collections) != 1 || held.Collections[0].ID != picks {
			t.Errorf("%s: the Collections library's tab %+v", label, held.Collections)
		}
		var landing struct {
			Sections []struct {
				Title string     `json:"title"`
				Items []siloCard `json:"items"`
			} `json:"sections"`
		}
		a.get(t, "/api/v2/library/"+collections+"/sections", token, profile, &landing)
		if len(want) > 0 && (len(landing.Sections) != 1 || landing.Sections[0].Title != "Picks" ||
			!slices.Equal(cardTitles(landing.Sections[0].Items), want)) || len(want) == 0 && len(landing.Sections) != 0 {
			t.Errorf("%s: the Collections library's landing %+v", label, landing.Sections)
		}
		section, shown := row()
		if len(want) == 0 {
			if section != "" {
				t.Errorf("%s: the home row shows %v", label, shown)
			}
			return
		}
		if !slices.Equal(shown, want) {
			t.Errorf("%s: the home row %q shows %v", label, section, shown)
		}
		if got := titles("/api/v2/catalog?source=section&section_id=" + section); !slices.Equal(got, want) {
			t.Errorf("%s: the home row's page lists %v", label, got)
		}
	}
	check("as made", []string{"Movie", "Remote"})

	if status, _ := p.call(http.MethodPost, "/Polyfin/UserItems/"+p.remote+"/Hidden", app("tv", p.token), nil); status != http.StatusNoContent &&
		status != http.StatusOK {
		t.Fatalf("hiding: %d", status)
	}
	check("with a hidden title", []string{"Movie"})
	p.limit(t, p.user, func(c *accounts.UserChanges) { c.BlockedGenres = &[]string{"Drama"} })
	check("with Drama blocked", nil)

	if status, data := a.do(t, http.MethodPost, "/api/v2/collections", token, profile, map[string]any{"name": "Mine"}); status != http.StatusNotImplemented ||
		problemCode(data) != "capability_unsupported" {
		t.Errorf("making a collection: %d %s", status, data)
	}
}

// collectionCatalogs serves an addon of two catalogs of collections, of
// the movies and the series of two other catalogs: Picks holds Action
// picks, its Action movies, and Show picks, its series; Sagas holds
// Everything, all of both.
func collectionCatalogs(t *testing.T) string {
	t.Helper()
	movies := []stremio.Meta{
		{ID: "tt9001", Type: "movie", Name: "Alpha", Genres: []string{"Action"}, ReleaseInfo: "2001", Released: "2001-01-01T00:00:00.000Z"},
		{ID: "tt9002", Type: "movie", Name: "Beta", Genres: []string{"Drama"}, ReleaseInfo: "2002", Released: "2002-01-01T00:00:00.000Z"},
		{ID: "tt9003", Type: "movie", Name: "Gamma", Genres: []string{"Action"}, ReleaseInfo: "2003", Released: "2003-01-01T00:00:00.000Z"},
	}
	show := stremio.Meta{ID: "tt9100", Type: "series", Name: "Show", Genres: []string{"Drama"}, ReleaseInfo: "2010",
		Released: "2010-01-01T00:00:00.000Z", Videos: []stremio.Video{{ID: "tt9100:1:1", Title: "Pilot", Season: 1, Episode: 1,
			Released: "2010-01-01T00:00:00.000Z"}}}
	collection := func(id, name string, sources ...stremio.CollectionSource) stremio.Meta {
		return stremio.Meta{ID: id, Type: "collection", Name: name, Collection: &stremio.Collection{Sources: sources}}
	}
	films, shows := stremio.CollectionSource{Type: "movie", CatalogID: "films"}, stremio.CollectionSource{Type: "series", CatalogID: "shows"}
	catalogs := map[string][]stremio.Meta{
		"picks": {collection("picks:action", "Action picks", stremio.CollectionSource{Type: "movie", CatalogID: "films", Genre: "Action"}),
			collection("picks:shows", "Show picks", shows)},
		"sagas": {collection("sagas:all", "Everything", films, shows)},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		send := func(key string, value any) { _ = json.NewEncoder(w).Encode(map[string]any{key: value}) }
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "collections", Name: "Collections", Version: "1",
				Types: []string{"movie", "series", "collection"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{
					{Type: "movie", ID: "films", Name: "Films", Extra: []stremio.Extra{{Name: "genre", Options: []string{"Action", "Drama"}}}},
					{Type: "series", ID: "shows", Name: "Shows"},
					{Type: "collection", ID: "picks", Name: "Picks"},
					{Type: "collection", ID: "sagas", Name: "Sagas"},
				}})
		case strings.HasPrefix(path, "/catalog/movie/films"):
			listed := movies
			if _, genre, ok := strings.Cut(strings.TrimSuffix(path, ".json"), "genre="); ok {
				listed = slices.DeleteFunc(slices.Clone(movies), func(m stremio.Meta) bool { return m.Genres[0] != genre })
			}
			send("metas", listed)
		case strings.HasPrefix(path, "/catalog/series/shows"):
			send("metas", []stremio.Meta{show})
		case strings.HasPrefix(path, "/catalog/collection/"):
			send("metas", catalogs[strings.TrimSuffix(strings.TrimPrefix(path, "/catalog/collection/"), ".json")])
		case strings.HasPrefix(path, "/meta/collection/"):
			id, _ := url.PathUnescape(strings.TrimSuffix(strings.TrimPrefix(path, "/meta/collection/"), ".json"))
			for _, listed := range catalogs {
				for _, meta := range listed {
					if meta.ID == id {
						send("meta", meta)
						return
					}
				}
			}
			http.NotFound(w, r)
		case path == "/meta/series/tt9100.json":
			send("meta", show)
		case strings.HasPrefix(path, "/meta/movie/"):
			for _, meta := range movies {
				if path == "/meta/movie/"+meta.ID+".json" {
					send("meta", meta)
					return
				}
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// A server whose only libraries are catalogs of collections, hidden from
// the web player's menus, has them listed in Silo's apps, as libraries of
// movies and series, whether or not an administrator has the Jellyfin API
// tell the apps that they hold collections: each opens on the titles of
// its collections, each once, in its order, as Jellyfin apps list them,
// its landing has a row of each collection, its Collections tab lists
// them, each with its titles, and Home a row of each library, in the
// user's order, without those they left out of My Media or of their
// latest titles. A title the user hid, and a genre they may not see, stay
// out of all of them.
func TestSiloAppsBrowseCollectionLibraries(t *testing.T) {
	t.Parallel()
	for _, told := range []bool{false, true} {
		t.Run("told collections "+strconv.FormatBool(told), func(t *testing.T) {
			t.Parallel()
			browseCollectionLibraries(t, told)
		})
	}
}

func browseCollectionLibraries(t *testing.T, told bool) {
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), collectionCatalogs(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "collection", CatalogID: "picks", HideInMenus: true},
		{AddonID: addon.ID, CatalogType: "collection", CatalogID: "sagas", HideInMenus: true},
	}); err != nil {
		t.Fatal(err)
	}
	if told {
		s.setting(t, func(settings *accounts.Settings) { settings.CollectionLibraryApps = []string{"Silo for Android"} })
	}
	allowSilo(t, s)
	a := servingSilo(t, s)
	token, profile := a.signIn(t, "member")
	jellyfin := s.signIn("member", "tv")
	var views QueryResult
	s.get(t, "/UserViews", jellyfin, &views)
	viewIDs := map[string]string{}
	for _, view := range views.Items {
		viewIDs[view.Name] = view.Id
	}
	// The Jellyfin API tells Silo's apps the libraries' type as asked.
	status, body := s.call(http.MethodGet, "/UserViews",
		`MediaBrowser Client="Silo for Android", Device="Pixel", DeviceId="pixel-1", Version="1.2.3", Token="`+token+`"`, nil)
	var silos QueryResult
	if status != http.StatusOK || json.Unmarshal(body, &silos) != nil || len(silos.Items) != 2 {
		t.Fatalf("libraries in the Jellyfin API: %d %s", status, body)
	}
	for _, view := range silos.Items {
		if want := map[bool]string{true: "boxsets"}[told]; view.CollectionType != want {
			t.Fatalf("%s is told %q", view.Name, view.CollectionType)
		}
	}
	// names lists the titles a Jellyfin listing gives Jellyfin apps.
	names := func(path string) []string {
		t.Helper()
		var page QueryResult
		s.get(t, path, jellyfin, &page)
		return itemNames(page.Items)
	}
	titles := func(path string) []string {
		t.Helper()
		var page siloPage
		a.get(t, path, token, profile, &page)
		return cardTitles(page.Items)
	}
	type siloSection struct {
		ID    string     `json:"id"`
		Type  string     `json:"section_type"`
		Title string     `json:"title"`
		Items []siloCard `json:"items"`
	}
	sections := func(path string) []siloSection {
		t.Helper()
		var answer struct {
			Sections []siloSection `json:"sections"`
		}
		a.get(t, path, token, profile, &answer)
		return answer.Sections
	}
	// check checks each library against what Jellyfin apps get, wants
	// those listed, each with the titles of its own, and Home with the rows
	// of home.
	check := func(label string, listed map[string][]string, home []string) {
		t.Helper()
		var libraries []string
		numbers := map[string]string{}
		for _, library := range a.libraries(t, token, profile) {
			libraries = append(libraries, library.Name)
			numbers[library.Name] = library.ID
			if library.Type != "mixed" {
				t.Errorf("%s: %s is of type %s", label, library.Name, library.Type)
			}
		}
		if want := slices.Sorted(maps.Keys(listed)); !slices.Equal(slices.Sorted(slices.Values(libraries)), want) {
			t.Fatalf("%s: libraries %v, want %v", label, libraries, want)
		}
		for name, want := range listed {
			number, view := numbers[name], viewIDs[name]
			grid := titles("/api/v2/catalog?library_id=" + number)
			if !slices.Equal(grid, names("/Items?ParentId="+view+"&Recursive=true&IncludeItemTypes=Movie,Series")) ||
				!slices.Equal(slices.Sorted(slices.Values(grid)), want) {
				t.Errorf("%s: %s lists %v, want %v", label, name, grid, want)
			}
			var tab struct {
				Collections []struct {
					ID        string `json:"id"`
					Title     string `json:"title"`
					ItemCount int    `json:"item_count"`
				} `json:"collections"`
			}
			a.get(t, "/api/v2/library/"+number+"/collections", token, profile, &tab)
			var collections, rows []string
			for _, collection := range tab.Collections {
				collections = append(collections, collection.Title)
				held := names("/Items?ParentId=" + collection.ID + "&IncludeItemTypes=Movie,Series")
				if got := titles("/api/v2/catalog?source=library_collection&collection_id=" + collection.ID); !slices.Equal(got, held) ||
					collection.ItemCount != len(held) {
					t.Errorf("%s: %s lists %v of %d, Jellyfin apps %v", label, collection.Title, got, collection.ItemCount, held)
				}
				if len(held) > 0 {
					rows = append(rows, collection.Title)
				}
			}
			if !slices.Equal(collections, names("/Items?ParentId="+view)) {
				t.Errorf("%s: %s's collections %v", label, name, collections)
			}
			var landing []string
			for _, row := range sections("/api/v2/library/" + number + "/sections") {
				landing = append(landing, row.Title)
				collection := strings.TrimPrefix(row.ID, "collection-")
				if held := names("/Items?ParentId=" + collection + "&IncludeItemTypes=Movie,Series"); !slices.Equal(cardTitles(row.Items), held) {
					t.Errorf("%s: the row %s of %s shows %v, its collection %v", label, row.Title, name, cardTitles(row.Items), held)
				}
				var one siloSection
				a.get(t, "/api/v2/library/"+number+"/sections/"+row.ID+"/items", token, profile, &one)
				if one.Title != row.Title || !slices.Equal(cardTitles(one.Items), cardTitles(row.Items)) {
					t.Errorf("%s: the row %s alone: %+v", label, row.Title, one)
				}
			}
			if !slices.Equal(landing, rows) {
				t.Errorf("%s: %s's landing %v, want %v", label, name, landing, rows)
			}
		}
		var rows []string
		for _, row := range sections("/api/v2/home/sections") {
			rows = append(rows, row.Title)
			number := numbers[row.Title]
			if grid := titles("/api/v2/catalog?library_id=" + number); number == "" || !slices.Equal(cardTitles(row.Items), grid) {
				t.Errorf("%s: the Home row %s shows %v", label, row.Title, cardTitles(row.Items))
			}
		}
		if !slices.Equal(rows, home) {
			t.Errorf("%s: Home %v, want %v", label, rows, home)
		}
	}
	check("as made", map[string][]string{"Picks": {"Alpha", "Gamma", "Show"}, "Sagas": {"Alpha", "Beta", "Gamma", "Show"}},
		[]string{"Picks", "Sagas"})

	if status, _ := s.call(http.MethodPost, "/Polyfin/UserItems/"+itemIDOf(t, s, jellyfin, viewIDs["Sagas"], "Gamma")+"/Hidden",
		app("tv", jellyfin), nil); status != http.StatusNoContent && status != http.StatusOK {
		t.Fatalf("hiding: %d", status)
	}
	check("with a hidden title", map[string][]string{"Picks": {"Alpha", "Show"}, "Sagas": {"Alpha", "Beta", "Show"}},
		[]string{"Picks", "Sagas"})
	s.limit(t, member, func(c *accounts.UserChanges) { c.BlockedGenres = &[]string{"Drama"} })
	check("with Drama blocked", map[string][]string{"Picks": {"Alpha"}, "Sagas": {"Alpha"}}, []string{"Picks", "Sagas"})

	if status := s.configure(t, "/Users/Configuration", jellyfin, map[string]any{"OrderedViews": []string{viewIDs["Sagas"]},
		"MyMediaExcludes": []string{viewIDs["Picks"]}}); status != http.StatusNoContent {
		t.Fatalf("leaving Picks out of My Media: %d", status)
	}
	check("without Picks in My Media", map[string][]string{"Sagas": {"Alpha"}}, []string{"Sagas"})
	if status := s.configure(t, "/Users/Configuration", jellyfin, map[string]any{"OrderedViews": []string{viewIDs["Sagas"]},
		"MyMediaExcludes": []string{}, "LatestItemsExcludes": []string{viewIDs["Sagas"]}}); status != http.StatusNoContent {
		t.Fatalf("leaving Sagas out of the latest: %d", status)
	}
	check("without Sagas in the latest", map[string][]string{"Picks": {"Alpha"}, "Sagas": {"Alpha"}}, []string{"Picks"})
}

// itemIDOf is the identifier of a title of a library, by its name.
func itemIDOf(t *testing.T, s testServer, token, library, name string) string {
	t.Helper()
	var page QueryResult
	s.get(t, "/Items?ParentId="+library+"&Recursive=true&IncludeItemTypes=Movie,Series", token, &page)
	for _, item := range page.Items {
		if item.Name == name {
			return item.Id
		}
	}
	t.Fatalf("no %s in %s", name, library)
	return ""
}
