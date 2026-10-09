package admin

import (
	"encoding/json"
	"maps"
	"net/http"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

func TestTrackingSettingsKeepTheSecret(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("admin", true)
	member := api.signedIn("member", false)
	status, body, _ := administrator.call(http.MethodGet, "/settings", nil)
	if status != http.StatusOK || body["traktClientId"] != "" || body["traktClientSecretSet"] != false || body["simklClientId"] != "" ||
		body["lastFmApiKey"] != "" || body["lastFmSecretSet"] != false {
		t.Fatalf("defaults: %d %v", status, body)
	}
	for _, secret := range []string{"traktClientSecret", "lastFmSecret"} {
		if _, ok := body[secret]; ok {
			t.Errorf("the field %s is sent", secret)
		}
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	set := maps.Clone(base)
	set["traktClientId"], set["traktClientSecret"], set["simklClientId"] = "trakt-id", "trakt-secret-value", "simkl-id"
	set["lastFmApiKey"], set["lastFmSecret"] = "lastfm-key", "lastfm-secret-value"
	status, body, _ = administrator.call(http.MethodPut, "/settings", set)
	encoded, _ := json.Marshal(body)
	if status != http.StatusOK || body["traktClientId"] != "trakt-id" || body["traktClientSecretSet"] != true || body["simklClientId"] != "simkl-id" ||
		body["lastFmApiKey"] != "lastfm-key" || body["lastFmSecretSet"] != true ||
		strings.Contains(string(encoded), "trakt-secret-value") || strings.Contains(string(encoded), "lastfm-secret-value") {
		t.Fatalf("saved: %d %s", status, encoded)
	}
	// Left out, they stay; the Set flag is not a field to write.
	kept := maps.Clone(base)
	kept["traktClientSecretSet"], kept["lastFmSecretSet"] = false, false
	if status, body, _ := administrator.call(http.MethodPut, "/settings", kept); status != http.StatusOK || body["traktClientSecretSet"] != true ||
		body["traktClientId"] != "trakt-id" || body["lastFmSecretSet"] != true || body["lastFmApiKey"] != "lastfm-key" {
		t.Errorf("left out: %d %v", status, body)
	}
	if got := api.store.Settings(); got.TraktClientSecret != "trakt-secret-value" || got.LastFMSecret != "lastfm-secret-value" || !got.LastFMAvailable() {
		t.Errorf("secrets: %q %q", got.TraktClientSecret, got.LastFMSecret)
	}
	for field, code := range map[string]string{"traktClientId": "invalid_trakt_app", "traktClientSecret": "invalid_trakt_app", "simklClientId": "invalid_simkl_app",
		"lastFmApiKey": "invalid_lastfm_app", "lastFmSecret": "invalid_lastfm_app"} {
		for _, value := range []string{"with space", "é", strings.Repeat("a", accounts.MaxTrackingAppBytes+1)} {
			refused := maps.Clone(base)
			refused[field] = value
			if status, body, _ := administrator.call(http.MethodPut, "/settings", refused); status != http.StatusBadRequest || body["error"] != code {
				t.Errorf("%s %q: %d %v", field, value, status, body)
			}
		}
	}
	if status, _, _ := member.call(http.MethodPut, "/settings", set); status != http.StatusForbidden {
		t.Errorf("member: %d", status)
	}
	cleared := maps.Clone(base)
	cleared["traktClientSecret"], cleared["simklClientId"], cleared["lastFmSecret"] = "", "", ""
	if status, body, _ := administrator.call(http.MethodPut, "/settings", cleared); status != http.StatusOK || body["traktClientSecretSet"] != false ||
		body["simklClientId"] != "" || body["traktClientId"] != "trakt-id" || body["lastFmSecretSet"] != false || body["lastFmApiKey"] != "lastfm-key" {
		t.Errorf("cleared: %d %v", status, body)
	}
}
