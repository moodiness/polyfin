package admin

import (
	"maps"
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

func TestSettingsPlaybackChoices(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	choices := func(body map[string]any) [5]any {
		return [5]any{body["analysisTimeout"], body["versionAttempts"], body["preferDirectPlay"], body["maxConversions"], body["maxConversionHeight"]}
	}
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); choices(body) != [5]any{float64(20), float64(3), false, float64(0), float64(0)} {
		t.Errorf("default settings: %v", body)
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "legacyAuthorization": false, "language": "en"}
	settings := maps.Clone(base)
	maps.Copy(settings, map[string]any{"analysisTimeout": 30, "versionAttempts": 6, "preferDirectPlay": true, "maxConversions": 2, "maxConversionHeight": 720})
	saved := [5]any{float64(30), float64(6), true, float64(2), float64(720)}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK || choices(body) != saved {
		t.Fatalf("saving the playback choices: %d %v", status, body)
	}
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); choices(body) != saved {
		t.Errorf("settings after saving: %v", body)
	}
	// A page or script older than them leaves them as they are.
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || choices(body) != saved {
		t.Errorf("saving without the playback choices: %d %v", status, body)
	}
	if got := api.store.Settings(); got.AnalysisTimeout != 30 || got.VersionAttempts != 6 || !got.PreferDirectPlay || got.MaxConversions != 2 || got.MaxConversionHeight != 720 {
		t.Errorf("stored after a save without them: %+v", got)
	}
	for _, tc := range []struct {
		key   string
		value any
		code  string
	}{
		{"analysisTimeout", accounts.MinAnalysisTimeout - 1, "invalid_analysis_timeout"},
		{"analysisTimeout", accounts.MaxAnalysisTimeout + 1, "invalid_analysis_timeout"},
		{"versionAttempts", accounts.MinVersionAttempts - 1, "invalid_version_attempts"},
		{"versionAttempts", accounts.MaxVersionAttempts + 1, "invalid_version_attempts"},
		{"maxConversions", accounts.MinMaxConversions - 1, "invalid_max_conversions"},
		{"maxConversions", accounts.MaxMaxConversions + 1, "invalid_max_conversions"},
		{"maxConversionHeight", 600, "invalid_max_conversion_height"},
		{"maxConversionHeight", 4320, "invalid_max_conversion_height"},
	} {
		refused := maps.Clone(base)
		refused[tc.key] = tc.value
		if status, body, _ := administrator.call(http.MethodPut, "/settings", refused); status != http.StatusBadRequest || body["error"] != tc.code {
			t.Errorf("%s %v: %d %v", tc.key, tc.value, status, body)
		}
	}
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); choices(body) != saved {
		t.Errorf("a refused value changed the settings: %v", body)
	}
}
