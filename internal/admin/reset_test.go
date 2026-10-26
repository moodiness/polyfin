package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/homerows"
	"github.com/moodiness/polyfin/internal/preferences"
	"github.com/moodiness/polyfin/internal/userdata"
)

// limitFields are the fields of a user's JSON a group holds, which Reset
// to defaults puts back.
var limitFields = []string{"parentalControl", "transcoding", "downloads", "personalAddons", "maxPlaybacks", "maxBitrate", "liveTv",
	"syncPlay", "remoteControl", "hiddenLibraries", "blockedGenres", "accessSchedules", "collectionManagement", "liveTvManagement",
	"subtitleManagement", "canShare", "torrentStreams", "qualityGroup"}

// storedPreferences resets the preferences store as the Jellyfin handler
// does, which also forgets what it remembers of them.
type storedPreferences struct{ *preferences.Store }

func (s storedPreferences) ResetPreferences(ctx context.Context, user accounts.ID) error {
	return s.Reset(ctx, user)
}

// differentLimits lists the limit fields in which a and b differ.
func differentLimits(a, b map[string]any) []string {
	var differ []string
	for _, field := range limitFields {
		x, _ := json.Marshal(a[field])
		y, _ := json.Marshal(b[field])
		if string(x) != string(y) {
			differ = append(differ, field)
		}
	}
	return differ
}

// Reset to defaults, on a user's page and on a group's, says first what it
// gives, then gives it: a new user's limits, the rest kept. A member who
// follows their group is not reset; a group's reset reaches its followers.
// The activity log names who reset whose settings.
func TestResetToDefaultsOnUsersAndGroups(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	admin := api.signedIn("admin", true)
	member := api.signedIn("member", false)
	fresh := api.user(t, admin, createdID(t, admin, "/users", map[string]any{"name": "fresh", "password": "correct horse"}))
	sam := createdID(t, admin, "/users", map[string]any{"name": "sam", "password": "correct horse"})
	status, changed, _ := admin.call(http.MethodPatch, "/users/"+sam, map[string]any{"isHidden": true, "downloads": false, "maxPlaybacks": 2,
		"qualityGroup": 720, "blockedGenres": []string{"Horror"}, "parentalControl": map[string]any{"maxRating": 13},
		"accessSchedules": []map[string]any{{"day": "Everyday", "startHour": 7, "endHour": 21}}})
	if status != http.StatusOK || len(differentLimits(changed, fresh)) != 6 {
		t.Fatalf("changing sam: %d %v", status, changed)
	}

	status, defaults, _ := admin.call(http.MethodGet, "/users/"+sam+"/defaults", nil)
	if status != http.StatusOK || len(differentLimits(defaults, fresh)) != 0 {
		t.Errorf("sam's defaults: %d %v, differing in %v", status, defaults, differentLimits(defaults, fresh))
	}
	if differ := differentLimits(api.user(t, admin, sam), changed); len(differ) != 0 {
		t.Errorf("reading the defaults changed %v", differ)
	}
	if status, _, _ := member.call(http.MethodPost, "/users/"+sam+"/reset", nil); status != http.StatusForbidden {
		t.Errorf("a member resetting: %d", status)
	}
	status, reset, _ := admin.call(http.MethodPost, "/users/"+sam+"/reset", nil)
	if status != http.StatusOK || len(differentLimits(reset, fresh)) != 0 || reset["name"] != "sam" || reset["isHidden"] != true {
		t.Errorf("resetting sam: %d %v", status, reset)
	}
	for _, path := range []string{"/users/0123456789abcdef0123456789abcdef/reset", "/groups/0123456789abcdef0123456789abcdef/reset"} {
		if status, body, _ := admin.call(http.MethodPost, path, nil); status != http.StatusNotFound || body["error"] != "not_found" {
			t.Errorf("%s: %d %v", path, status, body)
		}
	}

	group := createdID(t, admin, "/groups", map[string]any{"name": "Kids"})
	if status, body, _ := admin.call(http.MethodPatch, "/groups/"+group, map[string]any{"maxPlaybacks": 3}); status != http.StatusOK {
		t.Fatalf("changing the group: %d %v", status, body)
	}
	ann := createdID(t, admin, "/users", map[string]any{"name": "ann", "password": "correct horse", "groupId": group})
	for _, call := range []struct{ method, path string }{{http.MethodGet, "/users/" + ann + "/defaults"}, {http.MethodPost, "/users/" + ann + "/reset"}} {
		if status, body, _ := admin.call(call.method, call.path, nil); status != http.StatusConflict || body["error"] != "follows_group" {
			t.Errorf("%s %s for a follower: %d %v", call.method, call.path, status, body)
		}
	}
	if status, defaults, _ := admin.call(http.MethodGet, "/groups/"+group+"/defaults", nil); status != http.StatusOK || len(differentLimits(defaults, fresh)) != 0 {
		t.Errorf("the group's defaults: %d %v", status, defaults)
	}
	status, kids, _ := admin.call(http.MethodPost, "/groups/"+group+"/reset", nil)
	if status != http.StatusOK || kids["name"] != "Kids" || kids["memberCount"] != 1.0 || len(differentLimits(kids, fresh)) != 0 {
		t.Errorf("resetting the group: %d %v", status, kids)
	}
	if user := api.user(t, admin, ann); user["followGroup"] != true || user["maxPlaybacks"] != 0.0 {
		t.Errorf("ann after the group's reset: %v", user)
	}

	var page struct {
		Items []activityEntryJSON `json:"items"`
	}
	admin.raw(http.MethodGet, "/activity?limit=100&type=UserPolicyUpdated,GroupChanged", "", &page)
	var names []string
	for _, entry := range slices.Backward(page.Items) {
		names = append(names, entry.Name)
	}
	want := []string{
		"User sam was changed",
		"admin reset the settings of sam to their defaults",
		"Group Kids was changed: Playback and access",
		"admin reset the settings of the group Kids to their defaults",
	}
	if !slices.Equal(names, want) {
		t.Errorf("activity:\n%q\nwant\n%q", names, want)
	}
	if len(page.Items) == 4 && (page.Items[2].UserID == nil || *page.Items[2].UserID != sam) {
		t.Errorf("the reset's entry is not about sam: %+v", page.Items[2])
	}
}

// Reset my preferences puts back what the user chose themselves: the
// preferences and configuration Jellyfin apps saved, the home rows turned
// off and the metadata language. What they watched, their favorites and
// resume points stay, as do other users' preferences; their apps already
// signed in read the server's descriptions at once.
func TestResetMyPreferences(t *testing.T) {
	t.Parallel()
	var pool *pgxpool.Pool
	api := newTestAPI(t, 10, func(o *Options, d testDeps) {
		pool = d.pool
		o.HomeRows = homerows.New(d.pool)
		o.Preferences = storedPreferences{preferences.New(d.pool)}
	})
	ctx := t.Context()
	prefs, rows, data := preferences.New(pool), homerows.New(pool), userdata.New(pool)
	sam := api.signedIn("sam", false)
	samID := api.userID("sam")
	kim := mustCreateUser(t, api, "kim")
	if _, err := api.store.UpdateUser(ctx, samID, accounts.UserChanges{MetadataLanguage: new("fr")}, nil); err != nil {
		t.Fatal(err)
	}
	for _, user := range []accounts.ID{samID, kim} {
		if err := prefs.PutConfiguration(ctx, user, json.RawMessage(`{"SubtitleMode":"Always","SubtitleLanguagePreference":"fre"}`)); err != nil {
			t.Fatal(err)
		}
		if err := prefs.Put(ctx, user, "usersettings", "emby", json.RawMessage(`{"SortBy":"SortName"}`)); err != nil {
			t.Fatal(err)
		}
	}
	row, err := rows.Create(ctx, homerows.Row{Name: "New here", Kind: homerows.KindNew, Place: homerows.PlaceTop, Count: 1, Everyone: true})
	if err != nil {
		t.Fatal(err)
	}
	if status, _, _ := sam.call(http.MethodPut, "/account/home-rows/"+row.ID.String(), map[string]any{"off": true}); status != http.StatusNoContent {
		t.Fatalf("turning the row off: %d", status)
	}
	watched := []userdata.Item{{ID: accounts.ID{7}}}
	if _, err := data.Change(ctx, samID, watched, func(d *userdata.Data) {
		d.Played, d.Favorite, d.Position, d.Runtime = true, true, 10*time.Minute, time.Hour
	}); err != nil {
		t.Fatal(err)
	}
	token, _, err := api.store.SignInDevice(ctx, samID, accounts.DeviceInfo{DeviceID: "sam-tv", DeviceName: "TV", Client: "Test",
		ClientVersion: "1.0", RemoteAddress: "192.0.2.1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, user, err := api.store.DeviceByToken(ctx, token, "192.0.2.1"); err != nil || user.MetadataLanguage != "fr" {
		t.Fatalf("sam's app before: %+v %v", user, err)
	}

	if status, body, _ := sam.call(http.MethodPost, "/account/preferences/reset", nil); status != http.StatusNoContent {
		t.Fatalf("resetting: %d %v", status, body)
	}

	if _, found, err := prefs.Configuration(ctx, samID); err != nil || found {
		t.Errorf("sam's configuration after the reset: %v %v", found, err)
	}
	if _, found, err := prefs.Get(ctx, samID, "usersettings", "emby"); err != nil || found {
		t.Errorf("sam's display preferences after the reset: %v %v", found, err)
	}
	if _, found, err := prefs.Configuration(ctx, kim); err != nil || !found {
		t.Errorf("kim's configuration after sam's reset: %v %v", found, err)
	}
	if _, found, err := prefs.Get(ctx, kim, "usersettings", "emby"); err != nil || !found {
		t.Errorf("kim's display preferences after sam's reset: %v %v", found, err)
	}
	if off, err := rows.Off(ctx, samID); err != nil || len(off) != 0 {
		t.Errorf("rows off after the reset: %v %v", off, err)
	}
	if _, language, _ := sam.call(http.MethodGet, "/account/metadata-language", nil); language["language"] != "" {
		t.Errorf("metadata language after the reset: %v", language)
	}
	if _, user, err := api.store.DeviceByToken(ctx, token, "192.0.2.1"); err != nil || user.MetadataLanguage != "" {
		t.Errorf("sam's app after the reset: %+v %v", user, err)
	}
	kept, err := data.Get(ctx, samID, []accounts.ID{{7}})
	if d := kept[accounts.ID{7}]; err != nil || !d.Played || !d.Favorite || d.Position != 10*time.Minute {
		t.Errorf("what sam watched after the reset: %+v %v", d, err)
	}
	if status, _, _ := sam.call(http.MethodGet, "/session", nil); status != http.StatusOK {
		t.Errorf("sam's admin session after the reset: %d", status)
	}
}

// mustCreateUser creates a user named name and returns their identifier.
func mustCreateUser(t *testing.T, api testAPI, name string) accounts.ID {
	t.Helper()
	user, err := api.store.CreateUser(t.Context(), accounts.NewUser{Name: name, Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	return user.ID
}
