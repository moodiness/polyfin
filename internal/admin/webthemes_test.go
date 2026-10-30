package admin

import (
	"net/http"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// themes reads the themes an answer lists.
func themes(body map[string]any) []string {
	list, _ := body["themes"].([]any)
	result := []string{}
	for _, theme := range list {
		text, _ := theme.(string)
		result = append(result, text)
	}
	return result
}

// An administrator gives each user a web player theme, the server's by
// default, and may keep them from changing it; users change their own
// under My account otherwise. The Custom theme is offered while it has a
// text. Reset my preferences puts back the server's, unless the
// administrator chose it.
func TestUsersHaveTheirWebTheme(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	memberPath := "/users/" + api.userID("member").String()
	reader := "/users/" + mustCreateUser(t, api, "reader").String()

	// Every signed-in user reads the themes; without a Custom text, the
	// Custom theme is not offered.
	if status, body, _ := member.call(http.MethodGet, "/web-themes", nil); status != http.StatusOK || body["serverTheme"] != accounts.WebThemeLumaaGlaass ||
		!slices.Equal(themes(body), []string{accounts.WebThemeLumaaGlaass, accounts.WebThemeNone}) {
		t.Errorf("themes: %d %v", status, body)
	}
	for _, refused := range []string{accounts.WebThemeCustom, "another"} {
		if status, body, _ := administrator.call(http.MethodPatch, reader, map[string]any{"webTheme": refused}); status != http.StatusBadRequest || body["error"] != "invalid_web_theme" {
			t.Errorf("giving %q: %d %v", refused, status, body)
		}
	}
	settings := api.store.Settings()
	settings.CustomCss = ".mine {}"
	if _, err := api.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	if _, body, _ := member.call(http.MethodGet, "/web-themes", nil); !slices.Equal(themes(body), accounts.WebThemes) {
		t.Errorf("themes with a Custom CSS: %v", body)
	}
	if status, body, _ := administrator.call(http.MethodPatch, reader, map[string]any{"webTheme": accounts.WebThemeCustom, "webThemeLocked": true}); status != http.StatusOK ||
		body["webTheme"] != accounts.WebThemeCustom || body["webThemeLocked"] != true {
		t.Errorf("giving Custom, locked: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPatch, reader, map[string]any{"webTheme": accounts.WebThemeNone}); status != http.StatusForbidden {
		t.Errorf("a member giving another user a theme: %d %v", status, body)
	}

	// The user's own.
	if status, body, _ := member.call(http.MethodGet, "/account/web-theme", nil); status != http.StatusOK || body["webTheme"] != "" || body["locked"] != false ||
		body["serverTheme"] != accounts.WebThemeLumaaGlaass || !slices.Equal(themes(body), accounts.WebThemes) {
		t.Errorf("own theme: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPut, "/account/web-theme", map[string]any{"webTheme": "another"}); status != http.StatusBadRequest || body["error"] != "invalid_web_theme" {
		t.Errorf("an own unknown theme: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPut, "/account/web-theme", map[string]any{"webTheme": accounts.WebThemeNone}); status != http.StatusOK || body["webTheme"] != accounts.WebThemeNone {
		t.Errorf("choosing jellyfin-web's own look: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPost, "/account/preferences/reset", nil); status != http.StatusNoContent {
		t.Fatalf("resetting: %d %v", status, body)
	}
	if _, body, _ := member.call(http.MethodGet, "/account/web-theme", nil); body["webTheme"] != "" {
		t.Errorf("after the reset: %v", body)
	}

	// Locked by the administrator, it is theirs to change only.
	if status, body, _ := administrator.call(http.MethodPatch, memberPath, map[string]any{"webTheme": accounts.WebThemeNone, "webThemeLocked": true}); status != http.StatusOK {
		t.Fatalf("locking the member's: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPut, "/account/web-theme", map[string]any{"webTheme": accounts.WebThemeLumaaGlaass}); status != http.StatusForbidden || body["error"] != "web_theme_locked" {
		t.Errorf("changing a locked theme: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPost, "/account/preferences/reset", nil); status != http.StatusNoContent {
		t.Fatalf("resetting while locked: %d %v", status, body)
	}
	if _, body, _ := member.call(http.MethodGet, "/account/web-theme", nil); body["webTheme"] != accounts.WebThemeNone || body["locked"] != true {
		t.Errorf("a locked theme after the reset: %v", body)
	}
	if user, err := api.store.User(t.Context(), api.userID("member")); err != nil || user.WebTheme != accounts.WebThemeNone {
		t.Errorf("the member's theme kept: %q, %v", user.WebTheme, err)
	}
}
