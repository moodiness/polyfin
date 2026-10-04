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
)

// css asks path as jellyfin-web does, without signing in, and returns the
// answer's status, content type and body.
func (s testServer) css(path string) (int, string, string) {
	s.t.Helper()
	response, err := http.Get(s.url + path)
	if err != nil {
		s.t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	return response.StatusCode, response.Header.Get("Content-Type"), string(body)
}

func TestBrandingComesFromTheSettings(t *testing.T) {
	s, _, member := administrated(t)
	// Nothing set: what a new Jellyfin answers.
	for _, path := range []string{"/Branding/Css", "/Branding/Css.css"} {
		if status, contentType, body := s.css(path); status != http.StatusOK || contentType != "text/css; charset=utf-8" || body != "" {
			t.Errorf("%s with no CSS: %d %q %q", path, status, contentType, body)
		}
	}
	settings := s.store.Settings()
	settings.CustomCss = ".skinHeader { background: teal; }"
	settings.LoginDisclaimer = "Private server: **members only**. <a href=\"https://example.com\">Rules</a>"
	settings.CustomJs = "console.log('not branding')"
	if _, err := s.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/Branding/Css", "/Branding/Css.css"} {
		if status, contentType, body := s.css(path); status != http.StatusOK || contentType != "text/css; charset=utf-8" || body != settings.CustomCss {
			t.Errorf("%s: %d %q %q", path, status, contentType, body)
		}
	}
	raw, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.1", "branding-configuration-set.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want any
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	for _, request := range []struct{ path, token string }{
		{"/Branding/Configuration", ""},
		{"/System/Configuration/branding", member},
	} {
		status, body := s.call(http.MethodGet, request.path, request.token, nil)
		var got BrandingOptions
		if err := json.Unmarshal(body, &got); status != http.StatusOK || err != nil ||
			got.CustomCss != settings.CustomCss || got.LoginDisclaimer != settings.LoginDisclaimer || got.SplashscreenEnabled {
			t.Errorf("%s: %d %s", request.path, status, body)
		}
		// The custom script is Polyfin's, not part of Jellyfin's branding.
		if strings.Contains(string(body), "not branding") {
			t.Errorf("%s shows the custom script: %s", request.path, body)
		}
		var shape any
		_ = json.Unmarshal(body, &shape)
		for _, difference := range compareShapes("branding-configuration-set", want, shape, shapeRules{}) {
			t.Errorf("%s: %s", request.path, difference)
		}
	}
}

func TestBrandingIsSavedByAdministratorsOnly(t *testing.T) {
	s, admin, member := administrated(t)
	settings := s.store.Settings()
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
	if got := s.store.Settings(); got.CustomCss != "" || got.LoginDisclaimer != "" {
		t.Fatalf("refused writes changed the branding: %+v", got)
	}
	if status, body := s.call(http.MethodPost, "/System/Configuration/Branding", admin, branding); status != http.StatusNoContent {
		t.Fatalf("administrator: %d %s", status, body)
	}
	if _, body := s.call(http.MethodGet, "/Branding/Configuration", "", nil); string(body) !=
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
	// Other parts of the configuration stay read only.
	if status, _ := s.call(http.MethodPost, "/System/Configuration/encoding", admin, map[string]any{}); status != http.StatusMethodNotAllowed {
		t.Errorf("encoding: %d", status)
	}
}
