package admin

import (
	"maps"
	"net/http"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The web player's CSS, script and sign-in disclaimer: kept when a PUT
// leaves them out, bounded, and set by administrators only. The disclaimer
// is empty by default.
func TestSettingsWebPlayerCode(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); body["loginDisclaimer"] != "" {
		t.Errorf("default disclaimer: %v", body["loginDisclaimer"])
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	set := maps.Clone(base)
	// The largest allowed, with characters JSON escapes, to fit the body.
	set["customCss"] = strings.Repeat("<", accounts.MaxCustomCodeBytes)
	set["customJs"] = strings.Repeat("&", accounts.MaxCustomCodeBytes)
	set["loginDisclaimer"] = strings.Repeat("é", accounts.MaxLoginDisclaimerBytes/2)
	if status, body, _ := administrator.call(http.MethodPut, "/settings", set); status != http.StatusOK || body["customCss"] != set["customCss"] ||
		body["customJs"] != set["customJs"] || body["loginDisclaimer"] != set["loginDisclaimer"] {
		t.Fatalf("saving the largest: %d %v", status, body["error"])
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || body["customCss"] != set["customCss"] ||
		body["customJs"] != set["customJs"] || body["loginDisclaimer"] != set["loginDisclaimer"] {
		t.Errorf("left out: %d %v", status, body["error"])
	}
	for field, code := range map[string]string{"customCss": "invalid_custom_css", "customJs": "invalid_custom_js", "loginDisclaimer": "invalid_login_disclaimer"} {
		limit := accounts.MaxCustomCodeBytes
		if field == "loginDisclaimer" {
			limit = accounts.MaxLoginDisclaimerBytes
		}
		for _, value := range []string{strings.Repeat("a", limit+1), "a\x00b"} {
			refused := maps.Clone(base)
			refused[field] = value
			if status, body, _ := administrator.call(http.MethodPut, "/settings", refused); status != http.StatusBadRequest || body["error"] != code {
				t.Errorf("%s of %d bytes: %d %v", field, len(value), status, body)
			}
		}
	}
	cleared := maps.Clone(base)
	cleared["customCss"], cleared["customJs"], cleared["loginDisclaimer"] = "", "", ""
	if status, body, _ := member.call(http.MethodPut, "/settings", cleared); status != http.StatusForbidden {
		t.Errorf("member: %d %v", status, body)
	}
	if got := api.store.Settings(); got.CustomJs != set["customJs"] {
		t.Error("a member's write changed the script")
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", cleared); status != http.StatusOK || body["customCss"] != "" || body["customJs"] != "" || body["loginDisclaimer"] != "" {
		t.Errorf("clearing: %d %v", status, body["error"])
	}
}
