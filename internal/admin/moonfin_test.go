package admin

import (
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/moonfin"
)

// moonfinAPI serves the admin API with Moonfin's settings, and returns
// their store.
func moonfinAPI(t *testing.T) (testAPI, *moonfin.Store) {
	t.Helper()
	var store *moonfin.Store
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		store = moonfin.New(deps.pool)
		o.Moonfin = store
	})
	return api, store
}

// An administrator serves Moonfin's companion to everyone by default, and
// sets each user apart, on or off, keeping them from changing it or not;
// users change their own under My account otherwise.
func TestUsersHaveTheirMoonfin(t *testing.T) {
	t.Parallel()
	api, _ := moonfinAPI(t)
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	memberPath := "/users/" + api.userID("member").String()

	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); body["moonfin"] != true {
		t.Errorf("the server's default: %v", body["moonfin"])
	}
	if status, body, _ := member.call(http.MethodGet, "/account/moonfin", nil); status != http.StatusOK || body["moonfin"] != "" ||
		body["locked"] != false || body["serverDefault"] != true {
		t.Errorf("own Moonfin: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPut, "/account/moonfin", map[string]any{"moonfin": "sometimes"}); status != http.StatusBadRequest || body["error"] != "invalid_moonfin" {
		t.Errorf("an unknown choice: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPut, "/account/moonfin", map[string]any{"moonfin": accounts.MoonfinOff}); status != http.StatusOK || body["moonfin"] != accounts.MoonfinOff {
		t.Errorf("turning it off: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPatch, memberPath, map[string]any{"moonfin": "sometimes"}); status != http.StatusBadRequest || body["error"] != "invalid_moonfin" {
		t.Errorf("giving an unknown choice: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPatch, memberPath, map[string]any{"moonfin": accounts.MoonfinOn}); status != http.StatusForbidden {
		t.Errorf("a member setting themselves apart through the users: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPatch, memberPath, map[string]any{"moonfin": accounts.MoonfinOn, "moonfinLocked": true}); status != http.StatusOK ||
		body["moonfin"] != accounts.MoonfinOn || body["moonfinLocked"] != true {
		t.Fatalf("locking it on: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPut, "/account/moonfin", map[string]any{"moonfin": accounts.MoonfinServer}); status != http.StatusForbidden || body["error"] != "moonfin_locked" {
		t.Errorf("changing a locked choice: %d %v", status, body)
	}
	if user, err := api.store.User(t.Context(), api.userID("member")); err != nil || user.Moonfin != accounts.MoonfinOn || !user.MoonfinLocked {
		t.Errorf("the member's choice kept: %+v, %v", user.Moonfin, err)
	}
	_, settings, _ := administrator.call(http.MethodGet, "/settings", nil)
	settings["moonfin"] = false
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK || body["moonfin"] != false {
		t.Errorf("turning the server's off: %d %v", status, body)
	}
	if _, body, _ := member.call(http.MethodGet, "/account/moonfin", nil); body["serverDefault"] != false || body["moonfin"] != accounts.MoonfinOn {
		t.Errorf("own Moonfin with the server's off: %v", body)
	}
}

// What a user's apps synced shows on their page and on My account, where
// it can be forgotten; only administrators see another user's.
func TestSyncedMoonfinSettingsShowAndAreForgotten(t *testing.T) {
	t.Parallel()
	api, store := moonfinAPI(t)
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	memberID := api.userID("member")
	path := "/users/" + memberID.String() + "/moonfin"

	if status, body, _ := administrator.call(http.MethodGet, path, nil); status != http.StatusOK || body["synced"] != false || body["updatedAt"] != nil {
		t.Errorf("before any sync: %d %v", status, body)
	}
	if _, err := store.SaveProfile(t.Context(), memberID, moonfin.Global, moonfin.Settings{"theme": "dark", "clock": "always"}, "moonfin-flutter"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveProfile(t.Context(), memberID, moonfin.TV, moonfin.Settings{"theme": "oled"}, "moonfin-tv"); err != nil {
		t.Fatal(err)
	}
	status, body, _ := administrator.call(http.MethodGet, path, nil)
	if profiles, _ := body["profiles"].([]any); status != http.StatusOK || body["synced"] != true || body["settings"] != float64(3) ||
		len(profiles) != 2 || body["updatedBy"] != "moonfin-tv" || body["updatedAt"] == nil {
		t.Errorf("synced: %d %v", status, body)
	}
	if status, _, _ := member.call(http.MethodGet, path, nil); status != http.StatusForbidden {
		t.Errorf("a member reading another's: %d", status)
	}
	if _, body, _ = member.call(http.MethodGet, "/account/moonfin", nil); body["sync"].(map[string]any)["settings"] != float64(3) {
		t.Errorf("own sync: %v", body)
	}
	if status, _, _ := member.call(http.MethodDelete, "/account/moonfin/settings", nil); status != http.StatusNoContent {
		t.Errorf("forgetting own: %d", status)
	}
	if _, body, _ = administrator.call(http.MethodGet, path, nil); body["synced"] != false {
		t.Errorf("after forgetting: %v", body)
	}
	if _, err := store.SaveProfile(t.Context(), memberID, moonfin.Global, moonfin.Settings{"theme": "dark"}, "web"); err != nil {
		t.Fatal(err)
	}
	if status, _, _ := administrator.call(http.MethodDelete, path, nil); status != http.StatusNoContent {
		t.Errorf("an administrator forgetting the member's: %d", status)
	}
	if _, err := store.Envelope(t.Context(), memberID); err == nil {
		t.Error("the member's settings are kept")
	}
}

// The defaults are taken from a user's synced profile, edited as JSON,
// and applied to every user who synced, by administrators alone.
func TestMoonfinDefaultsAreTakenEditedAndApplied(t *testing.T) {
	t.Parallel()
	api, store := moonfinAPI(t)
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	memberID := api.userID("member")
	if _, err := store.SaveProfile(t.Context(), memberID, moonfin.Global, moonfin.Settings{"theme": "dark", "tmdbApiKey": "mine"}, "web"); err != nil {
		t.Fatal(err)
	}

	status, body, _ := administrator.call(http.MethodGet, "/moonfin/defaults", nil)
	profiles, _ := body["profiles"].(map[string]any)
	users, _ := body["users"].([]any)
	if status != http.StatusOK || len(profiles) != 4 || len(users) != 1 || users[0].(map[string]any)["name"] != "member" {
		t.Fatalf("defaults: %d %v", status, body)
	}
	if status, _, _ := member.call(http.MethodGet, "/moonfin/defaults", nil); status != http.StatusForbidden {
		t.Errorf("a member reading the defaults: %d", status)
	}
	if status, body, _ = administrator.call(http.MethodPost, "/moonfin/defaults/tv/copy", map[string]any{"userId": memberID.String(), "profile": "desktop"}); status != http.StatusNotFound ||
		body["error"] != "moonfin_settings_not_found" {
		t.Errorf("copying a profile the member has not: %d %v", status, body)
	}
	status, body, _ = administrator.call(http.MethodPost, "/moonfin/defaults/tv/copy", map[string]any{"userId": memberID.String(), "profile": "global"})
	tv, _ := body["profiles"].(map[string]any)["tv"].(map[string]any)
	if status != http.StatusOK || tv["theme"] != "dark" || tv["tmdbApiKey"] != nil {
		t.Errorf("copied: %d %v", status, body)
	}
	if status, body, _ = administrator.call(http.MethodPut, "/moonfin/defaults/global", []string{"not", "an", "object"}); status != http.StatusBadRequest ||
		body["error"] != "invalid_moonfin_defaults" {
		t.Errorf("defaults that are no object: %d %v", status, body)
	}
	if status, _, _ = administrator.call(http.MethodPut, "/moonfin/defaults/watch", map[string]any{}); status != http.StatusNotFound {
		t.Errorf("an unknown profile: %d", status)
	}
	if status, body, _ = administrator.call(http.MethodPut, "/moonfin/defaults/global", map[string]any{"clock": "always"}); status != http.StatusOK ||
		body["profiles"].(map[string]any)["global"].(map[string]any)["clock"] != "always" {
		t.Errorf("edited: %d %v", status, body)
	}
	if status, _, _ = member.call(http.MethodPost, "/moonfin/apply", map[string]any{}); status != http.StatusForbidden {
		t.Errorf("a member applying the defaults: %d", status)
	}
	if status, body, _ = administrator.call(http.MethodPost, "/moonfin/apply", map[string]any{"reset": false}); status != http.StatusOK || body["users"] != float64(1) {
		t.Errorf("applied: %d %v", status, body)
	}
	resolved, err := store.Resolved(t.Context(), memberID, moonfin.Global)
	if err != nil || resolved["clock"] != "always" || resolved["theme"] != "dark" || resolved["tmdbApiKey"] != "mine" {
		t.Errorf("the member's settings: %v %v", resolved, err)
	}
}
