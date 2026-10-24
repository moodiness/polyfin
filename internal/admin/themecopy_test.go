package admin

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/themecopy"
)

// The theme choice: LumaaGlaass by default, with the texts Polyfin serves
// for it shown, saved, kept when a PUT leaves it out, and refused when it
// is none of the three; choosing one keeps the Custom texts.
func TestSettingsWebTheme(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	_, body, _ := administrator.call(http.MethodGet, "/settings", nil)
	shown, _ := body["lumaaGlaass"].(map[string]any)
	if body["webTheme"] != accounts.WebThemeLumaaGlaass || shown["css"] != accounts.LumaaGlaassCss || shown["js"] != accounts.LumaaGlaassJs {
		t.Fatalf("a new server's theme: %v, %v", body["webTheme"], shown)
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	custom := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en",
		"webTheme": accounts.WebThemeCustom, "customCss": ".mine {}", "customJs": "mine()"}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", custom); status != http.StatusOK || body["webTheme"] != accounts.WebThemeCustom {
		t.Fatalf("choosing Custom: %d %v", status, body)
	}
	none := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en", "webTheme": accounts.WebThemeNone}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", none); status != http.StatusOK || body["webTheme"] != accounts.WebThemeNone ||
		body["customCss"] != ".mine {}" || body["customJs"] != "mine()" {
		t.Errorf("choosing None: %d %v", status, body)
	}
	if got := api.store.Settings(); got.ServedCss() != "" || got.ServedJs() != "" {
		t.Errorf("None serves %q and %q", got.ServedCss(), got.ServedJs())
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || body["webTheme"] != accounts.WebThemeNone {
		t.Errorf("left out: %d %v", status, body["webTheme"])
	}
	refused := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en", "webTheme": "another"}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", refused); status != http.StatusBadRequest || body["error"] != "invalid_web_theme" {
		t.Errorf("another theme: %d %v", status, body)
	}
}

// Keeping a copy of the theme's files is for administrators only, and
// asks addresses on the internet only: a theme on the local network is
// refused, with the address and why, and the texts stay as they were.
func TestThemeCopyIsForAdministratorsAndThePublicInternet(t *testing.T) {
	t.Parallel()
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		_, _ = w.Write([]byte("body {}"))
	}))
	t.Cleanup(local.Close)
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		copier, err := themecopy.New(t.Context(), themecopy.Options{DB: deps.pool, Accounts: o.Accounts, Dir: filepath.Join(o.DataDir, "themes")})
		if err != nil {
			t.Fatal(err)
		}
		o.ThemeCopier = copier
	})
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	css := `@import url("` + local.URL + `/theme.css");`
	settings := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en", "webTheme": accounts.WebThemeCustom, "customCss": css}
	if status, _, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK {
		t.Fatalf("settings: %d", status)
	}
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		if status, _, _ := member.call(method, "/settings/theme-copy", nil); status != http.StatusForbidden {
			t.Errorf("member %s: %d", method, status)
		}
	}
	status, body, _ := administrator.call(http.MethodGet, "/settings/theme-copy", nil)
	outside, _ := body["outside"].([]any)
	if status != http.StatusOK || body["copy"] != nil || len(outside) != 1 || outside[0] != local.URL+"/theme.css" {
		t.Errorf("status: %d %v", status, body)
	}
	status, body, _ = administrator.call(http.MethodPost, "/settings/theme-copy", nil)
	unsaved, _ := body["unsaved"].([]any)
	first, _ := func() (map[string]any, bool) {
		if len(unsaved) != 1 {
			return nil, false
		}
		entry, ok := unsaved[0].(map[string]any)
		return entry, ok
	}()
	if status != http.StatusUnprocessableEntity || body["error"] != "theme_copy_failed" || first["reason"] != themecopy.ReasonPrivate ||
		first["address"] != local.URL+"/theme.css" {
		t.Errorf("a theme on the local network: %d %v", status, body)
	}
	if got := api.store.Settings().CustomCss; got != css {
		t.Errorf("the texts changed: %q", got)
	}
	if status, body, _ := administrator.call(http.MethodDelete, "/settings/theme-copy", nil); status != http.StatusNotFound {
		t.Errorf("removing no copy: %d %v", status, body)
	}
}
