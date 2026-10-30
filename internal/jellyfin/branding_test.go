package jellyfin

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/webtheme"
)

// css asks path as jellyfin-web does, with the authorization and the cookie
// given, each empty for none, and returns the answer's status, content type
// and body.
func (s testServer) css(path, authorization, cookie string) (int, string, string) {
	s.t.Helper()
	request, _ := http.NewRequestWithContext(s.t.Context(), http.MethodGet, s.url+path, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	if cookie != "" {
		request.Header.Set("Cookie", cookie)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		s.t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	return response.StatusCode, response.Header.Get("Content-Type"), string(body)
}

// brandingCss is the CustomCss /Branding/Configuration answers with the
// authorization and the cookie given.
func (s testServer) brandingCss(authorization, cookie string) string {
	s.t.Helper()
	status, contentType, body := s.css("/Branding/Configuration", authorization, cookie)
	var branding BrandingOptions
	if err := json.Unmarshal([]byte(body), &branding); status != http.StatusOK || !strings.HasPrefix(contentType, "application/json") || err != nil {
		s.t.Fatalf("/Branding/Configuration: %d %q %s", status, contentType, body)
	}
	return branding.CustomCss
}

func TestBrandingComesFromTheSettings(t *testing.T) {
	t.Parallel()
	s, _, member := administrated(t)
	settings := s.store.Settings()
	settings.WebTheme = accounts.WebThemeNone
	settings.CustomCss = ".kept { color: red; }"
	if _, err := s.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	// No CSS, jellyfin-web's own look: what a new Jellyfin answers.
	for _, path := range []string{"/Branding/Css", "/Branding/Css.css"} {
		if status, contentType, body := s.css(path, member, ""); status != http.StatusOK || contentType != "text/css; charset=utf-8" || body != "" {
			t.Errorf("%s with no CSS: %d %q %q", path, status, contentType, body)
		}
	}
	settings.WebTheme = accounts.WebThemeCustom
	settings.CustomCss = ".skinHeader { background: teal; }"
	settings.LoginDisclaimer = "Private server: **members only**. <a href=\"https://example.com\">Rules</a>"
	settings.CustomJs = "console.log('not branding')"
	if _, err := s.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/Branding/Css", "/Branding/Css.css"} {
		if status, contentType, body := s.css(path, member, ""); status != http.StatusOK || contentType != "text/css; charset=utf-8" || body != settings.CustomCss {
			t.Errorf("%s: %d %q %q", path, status, contentType, body)
		}
	}
	raw, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.2", "branding-configuration-set.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want any
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/Branding/Configuration", "/System/Configuration/branding"} {
		status, body := s.call(http.MethodGet, path, member, nil)
		var got BrandingOptions
		if err := json.Unmarshal(body, &got); status != http.StatusOK || err != nil ||
			got.CustomCss != settings.CustomCss || got.LoginDisclaimer != settings.LoginDisclaimer || got.SplashscreenEnabled {
			t.Errorf("%s: %d %s", path, status, body)
		}
		// The custom script is Polyfin's, not part of Jellyfin's branding.
		if strings.Contains(string(body), "not branding") {
			t.Errorf("%s shows the custom script: %s", path, body)
		}
		var shape any
		_ = json.Unmarshal(body, &shape)
		for _, difference := range compareShapes("branding-configuration-set", want, shape, shapeRules{}) {
			t.Errorf("%s: %s", path, difference)
		}
	}
}

// Each user has their web player theme, which the web client's page loads
// itself: Jellyfin's branding gives the CSS of the theme the page names in
// its cookie, as the page loads it, else that of the user signed in, else
// none, where Jellyfin gives the server's to everyone. The server's
// configuration stays the server's, for the dashboard posts it back.
func TestBrandingGivesTheCssOfTheThemeShown(t *testing.T) {
	t.Parallel()
	s, _, member := administrated(t)
	settings := s.store.Settings()
	settings.WebTheme = accounts.WebThemeLumaaGlaass
	settings.CustomCss, settings.CustomJs = ".mine {}", "console.log('mine')"
	if _, err := s.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	s.user("bob", func(c *accounts.UserChanges) { c.WebTheme = new(accounts.WebThemeNone) })
	s.user("carol", func(c *accounts.UserChanges) { c.WebTheme = new(accounts.WebThemeCustom) })
	bob, carol := app("bob", s.signIn("bob", "bob")), app("carol", s.signIn("carol", "carol"))
	_, key, err := s.store.CreateAPIKey(t.Context(), "Branding")
	if err != nil {
		t.Fatal(err)
	}
	cookie := webtheme.Cookie(testServerID) + "="
	customImport := "@import url('custom.css?v=" + webtheme.Hash(".mine {}") + "');"
	for _, tc := range []struct {
		who, authorization, cookie string
		configuration, css         string
	}{
		// None to anonymous requests: the page loads the theme itself.
		{"anonymous", "", "", "", ""},
		{"an unknown token", app("tv", "0123456789abcdef0123456789abcdef"), "", "", ""},
		// A user's own theme, else the server's.
		{"alice, the server's", member, "", accounts.LumaaGlaassCss, accounts.LumaaGlaassCss},
		{"bob, jellyfin-web's own look", bob, "", "", ""},
		{"carol, Custom", carol, "", ".mine {}", ".mine {}"},
		{"an API key, the server's", `MediaBrowser Token="` + key + `"`, "", accounts.LumaaGlaassCss, accounts.LumaaGlaassCss},
		// The theme the page names wins, the page's Custom CSS imported
		// from where the page loads it.
		{"bob's page showing LumaaGlaass", bob, cookie + "lumaaglaass", accounts.LumaaGlaassCss, accounts.LumaaGlaassCss},
		{"alice's page falling back on an old browser", member, cookie + "none", "", ""},
		{"a page signed out showing Custom", "", cookie + "custom", customImport, ".mine {}"},
		{"a page naming no theme Polyfin has", member, cookie + "another", "", ""},
		// Another server's page on the same host is not this one's.
		{"another server's page", bob, webtheme.Cookie("fedcba9876543210fedcba9876543210") + "=lumaaglaass", "", ""},
	} {
		if got := s.brandingCss(tc.authorization, tc.cookie); got != tc.configuration {
			t.Errorf("%s: /Branding/Configuration gives %q, want %q", tc.who, got, tc.configuration)
		}
		if _, _, got := s.css("/Branding/Css", tc.authorization, tc.cookie); got != tc.css {
			t.Errorf("%s: /Branding/Css gives %q, want %q", tc.who, got, tc.css)
		}
	}
	for _, authorization := range []string{member, bob} {
		status, body := s.call(http.MethodGet, "/System/Configuration/branding", authorization, nil)
		var got BrandingOptions
		if err := json.Unmarshal(body, &got); status != http.StatusOK || err != nil || got.CustomCss != accounts.LumaaGlaassCss {
			t.Errorf("the server's branding: %d %s", status, body)
		}
	}
}

// GET /Polyfin/WebTheme answers the theme the web player shows the user
// signed in, and their own, empty while they keep the server's.
func TestWebThemeAnswersTheUsersTheme(t *testing.T) {
	t.Parallel()
	s, _, member := administrated(t)
	s.user("bob", func(c *accounts.UserChanges) { c.WebTheme = new(accounts.WebThemeNone) })
	bob := app("bob", s.signIn("bob", "bob"))
	_, key, err := s.store.CreateAPIKey(t.Context(), "Theme")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		who, authorization, want string
	}{
		{"alice", member, `{"Theme":"lumaaglaass","UserTheme":""}`},
		{"bob", bob, `{"Theme":"none","UserTheme":"none"}`},
		{"an API key", `MediaBrowser Token="` + key + `"`, `{"Theme":"lumaaglaass","UserTheme":""}`},
	} {
		if status, body := s.call(http.MethodGet, "/Polyfin/WebTheme", tc.authorization, nil); status != http.StatusOK || strings.TrimSpace(string(body)) != tc.want {
			t.Errorf("%s: %d %s, want %s", tc.who, status, body, tc.want)
		}
	}
	settings := s.store.Settings()
	settings.WebTheme = accounts.WebThemeCustom
	if _, err := s.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	if _, body := s.call(http.MethodGet, "/Polyfin/WebTheme", member, nil); strings.TrimSpace(string(body)) != `{"Theme":"custom","UserTheme":""}` {
		t.Errorf("alice after the server's theme changed: %s", body)
	}
	if status, _ := s.call(http.MethodGet, "/Polyfin/WebTheme", "", nil); status != http.StatusUnauthorized {
		t.Errorf("signed out: %d", status)
	}
}

func TestBrandingIsSavedByAdministratorsOnly(t *testing.T) {
	t.Parallel()
	s, admin, member := administrated(t)
	settings := s.store.Settings()
	settings.WebTheme = accounts.WebThemeCustom
	settings.CustomJs = "console.log('kept')"
	if _, err := s.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	branding := map[string]any{"LoginDisclaimer": "Hello", "CustomCss": "body { color: red; }", "SplashscreenEnabled": true}
	if status, _ := s.call(http.MethodPost, "/System/Configuration/branding", member, branding); status != http.StatusForbidden {
		t.Errorf("member: %d", status)
	}
	if status, _ := s.call(http.MethodPost, "/System/Configuration/branding", "", branding); status != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", status)
	}
	if got := s.store.Settings(); got.CustomCss != settings.CustomCss || got.LoginDisclaimer != settings.LoginDisclaimer {
		t.Fatalf("refused writes changed the branding: %+v", got)
	}
	if status, body := s.call(http.MethodPost, "/System/Configuration/Branding", admin, branding); status != http.StatusNoContent {
		t.Fatalf("administrator: %d %s", status, body)
	}
	if _, body := s.call(http.MethodGet, "/Branding/Configuration", member, nil); string(body) !=
		`{"LoginDisclaimer":"Hello","CustomCss":"body { color: red; }","SplashscreenEnabled":false}`+"\n" {
		t.Errorf("saved: %s", body)
	}
	// As in Jellyfin, the body replaces the branding whole; the custom
	// script is not part of it.
	if status, _ := s.call(http.MethodPost, "/System/Configuration/branding", admin, map[string]any{"CustomCss": "a {}"}); status != http.StatusNoContent {
		t.Fatalf("partial: %d", status)
	}
	if got := s.store.Settings(); got.CustomCss != "a {}" || got.LoginDisclaimer != "" || got.CustomJs != "console.log('kept')" {
		t.Errorf("after a partial body: %+v", got)
	}
	for _, refused := range []any{
		map[string]any{"CustomCss": strings.Repeat("a", accounts.MaxCustomCodeBytes+1)},
		map[string]any{"LoginDisclaimer": strings.Repeat("a", accounts.MaxLoginDisclaimerBytes+1)},
		map[string]any{"SplashscreenEnabled": "yes"},
	} {
		if status, body := s.call(http.MethodPost, "/System/Configuration/branding", admin, refused); status != http.StatusBadRequest {
			t.Errorf("refused: %d %s", status, body)
		}
	}
	if got := s.store.Settings(); got.CustomCss != "a {}" {
		t.Errorf("a refused body changed the CSS: %q", got.CustomCss)
	}
	// The largest CSS fits the body, even with every character escaped by
	// JSON, so that a valid CSS is never refused for its body's size.
	largest := strings.Repeat("<", accounts.MaxCustomCodeBytes)
	if status, body := s.call(http.MethodPost, "/System/Configuration/branding", admin, map[string]any{"CustomCss": largest}); status != http.StatusNoContent {
		t.Fatalf("largest CSS: %d %s", status, body)
	}
	if got := s.store.Settings(); got.CustomCss != largest {
		t.Errorf("largest CSS: %d bytes saved", len(got.CustomCss))
	}
	// Other parts of the configuration stay read only.
	if status, _ := s.call(http.MethodPost, "/System/Configuration/encoding", admin, map[string]any{}); status != http.StatusMethodNotAllowed {
		t.Errorf("encoding: %d", status)
	}
}

// jellyfin-web's branding page posts back the CSS it read, the theme's:
// the theme stays, and the Custom texts kept while it is shown stay as
// they were. Another CSS becomes the Custom CSS, and the page keeps the
// script it had, which Jellyfin's branding knows nothing of.
func TestBrandingKeepsTheThemeItsCssNames(t *testing.T) {
	t.Parallel()
	s, admin, _ := administrated(t)
	settings := s.store.Settings()
	settings.WebTheme = accounts.WebThemeLumaaGlaass
	settings.CustomCss, settings.CustomJs = ".mine {}", "console.log('mine')"
	if _, err := s.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	_, body := s.call(http.MethodGet, "/System/Configuration/branding", admin, nil)
	var read BrandingOptions
	if err := json.Unmarshal(body, &read); err != nil || read.CustomCss != accounts.LumaaGlaassCss {
		t.Fatalf("the branding read: %s", body)
	}
	read.LoginDisclaimer = "Welcome"
	if status, body := s.call(http.MethodPost, "/System/Configuration/branding", admin, read); status != http.StatusNoContent {
		t.Fatalf("posted back: %d %s", status, body)
	}
	got := s.store.Settings()
	if got.WebTheme != accounts.WebThemeLumaaGlaass || got.CustomCss != ".mine {}" || got.CustomJs != "console.log('mine')" || got.LoginDisclaimer != "Welcome" {
		t.Errorf("after the theme's CSS came back: theme %q, texts %q and %q, disclaimer %q", got.WebTheme, got.CustomCss, got.CustomJs, got.LoginDisclaimer)
	}
	if status, body := s.call(http.MethodPost, "/System/Configuration/branding", admin, map[string]any{"CustomCss": "body { color: red; }"}); status != http.StatusNoContent {
		t.Fatalf("another CSS: %d %s", status, body)
	}
	got = s.store.Settings()
	if got.WebTheme != accounts.WebThemeCustom || got.CustomCss != "body { color: red; }" || got.CustomJs != accounts.LumaaGlaassJs {
		t.Errorf("after another CSS: theme %q, texts %q and %q", got.WebTheme, got.CustomCss, got.CustomJs)
	}
}
