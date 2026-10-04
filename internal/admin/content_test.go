package admin

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// genresAddon serves a manifest whose movie catalog offers genres, and a
// series catalog that offers some of the same.
func genresAddon(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id": "genres", "name": "Genres", "version": "1.0.0", "resources": ["catalog"],
			"catalogs": [{"type": "movie", "id": "top", "name": "Top", "extra": [{"name": "genre", "options": ["Horror", "Comedy"]}]},
				{"type": "series", "id": "shows", "name": "Shows", "extra": [{"name": "genre", "options": ["comedy", "Animation"]}]}]}`))
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

func TestAdministratorsSetUserContent(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	if status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": genresAddon(t)}); status != http.StatusCreated {
		t.Fatalf("installing the addon: %d %v", status, body)
	}
	_, created, _ := administrator.call(http.MethodPost, "/users", map[string]any{"name": "child", "password": "correct horse"})
	for field, want := range map[string]any{"hiddenLibraries": []any{}, "blockedGenres": []any{}, "accessSchedules": []any{}} {
		if !reflect.DeepEqual(created[field], want) {
			t.Errorf("new user's %s: %v", field, created[field])
		}
	}
	path := "/users/" + created["id"].(string)

	// The choices: the server's libraries, and their genres sorted, each
	// once.
	status, choices, _ := administrator.call(http.MethodGet, "/user-content-choices", nil)
	libraries, _ := choices["libraries"].([]any)
	if status != http.StatusOK || len(libraries) != 2 || !reflect.DeepEqual(choices["genres"], []any{"Animation", "Comedy", "Horror"}) {
		t.Fatalf("choices: %d %v", status, choices)
	}
	top := libraries[0].(map[string]any)
	if top["name"] != "Top" {
		t.Fatalf("first library: %v", top)
	}
	if status, _, _ := member.call(http.MethodGet, "/user-content-choices", nil); status != http.StatusForbidden {
		t.Errorf("member reading the choices: %d", status)
	}

	status, updated, _ := administrator.call(http.MethodPatch, path, map[string]any{
		"hiddenLibraries": []string{top["id"].(string)},
		"blockedGenres":   []string{"Horror", " horror ", "Thriller"},
		"accessSchedules": []map[string]any{{"day": "weekday", "startHour": 7.5, "endHour": 20}},
	})
	want := map[string]any{
		"hiddenLibraries": []any{top["id"]},
		"blockedGenres":   []any{"Horror", "Thriller"},
		"accessSchedules": []any{map[string]any{"day": "Weekday", "startHour": 7.5, "endHour": float64(20)}},
	}
	if status != http.StatusOK {
		t.Fatalf("setting the content: %d %v", status, updated)
	}
	for field, value := range want {
		if !reflect.DeepEqual(updated[field], value) {
			t.Errorf("%s: %v, want %v", field, updated[field], value)
		}
	}
	// Other changes keep them.
	_, updated, _ = administrator.call(http.MethodPatch, path, map[string]any{"isHidden": true})
	for field, value := range want {
		if !reflect.DeepEqual(updated[field], value) {
			t.Errorf("%s after another change: %v", field, updated[field])
		}
	}

	for code, patch := range map[string]map[string]any{
		"invalid_hidden_libraries": {"hiddenLibraries": []string{"nope"}},
		"invalid_blocked_genres":   {"blockedGenres": []string{" "}},
		"invalid_access_schedules": {"accessSchedules": []map[string]any{{"day": "Weekday", "startHour": 20, "endHour": 25}}},
	} {
		if status, body, _ := administrator.call(http.MethodPatch, path, patch); status != http.StatusBadRequest || body["error"] != code {
			t.Errorf("%v: %d %v", patch, status, body)
		}
	}
	if status, _, _ := member.call(http.MethodPatch, path, map[string]any{"blockedGenres": []string{}}); status != http.StatusForbidden {
		t.Errorf("member changing the genres: %d", status)
	}
	// Clearing them.
	_, updated, _ = administrator.call(http.MethodPatch, path, map[string]any{"hiddenLibraries": []string{}, "blockedGenres": []string{}, "accessSchedules": []any{}})
	for field := range want {
		if !reflect.DeepEqual(updated[field], []any{}) {
			t.Errorf("cleared %s: %v", field, updated[field])
		}
	}
}

func TestAllowedHoursInTheAdminApp(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	weekdays := &[]accounts.AccessSchedule{{Day: "Weekday", StartHour: 9, EndHour: 17}}
	for _, name := range []string{"administrator", "member"} {
		user, _ := api.store.Authenticate(t.Context(), name, "correct horse")
		if _, err := api.store.UpdateUser(t.Context(), user.ID, accounts.UserChanges{AccessSchedules: weekdays}, nil); err != nil {
			t.Fatal(err)
		}
	}
	saturday := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.Local)
	api.clock.Store(&saturday)
	signIn := func(name string) (int, any) {
		status, body, _ := api.browser().call(http.MethodPost, "/session", map[string]string{"name": name, "password": "correct horse"})
		return status, body["error"]
	}

	// Outside the hours, nobody signs in, as in Jellyfin apps.
	for _, name := range []string{"member", "administrator"} {
		if status, code := signIn(name); status != http.StatusForbidden || code != "outside_allowed_hours" {
			t.Errorf("%s signing in on Saturday: %d %v", name, status, code)
		}
	}
	// A member's session shows who they are, and nothing else.
	if status, _, _ := member.call(http.MethodGet, "/session", nil); status != http.StatusOK {
		t.Errorf("member's session on Saturday: %d", status)
	}
	if status, body, _ := member.call(http.MethodGet, "/account/devices", nil); status != http.StatusForbidden || body["error"] != "outside_allowed_hours" {
		t.Errorf("member's devices on Saturday: %d %v", status, body)
	}
	// An administrator's session goes on.
	if status, _, _ := administrator.call(http.MethodGet, "/users", nil); status != http.StatusOK {
		t.Errorf("administrator on Saturday: %d", status)
	}

	monday := saturday.AddDate(0, 0, 2).Add(-2 * time.Hour)
	api.clock.Store(&monday)
	if status, code := signIn("member"); status != http.StatusOK {
		t.Errorf("member signing in on Monday at 10: %d %v", status, code)
	}
	if status, _, _ := member.call(http.MethodGet, "/account/devices", nil); status != http.StatusOK {
		t.Errorf("member's devices on Monday at 10: %d", status)
	}
}
