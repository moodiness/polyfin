package admin

import (
	"maps"
	"net/http"
	"reflect"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The settings answer what each of them accepts, and its default, from the
// Go constants; a PUT cannot change them.
func TestSettingsAnswerTheirBounds(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	_, body, _ := administrator.call(http.MethodGet, "/settings", nil)
	bounds, _ := body["bounds"].(map[string]any)
	entry := func(name string) map[string]any {
		t.Helper()
		found, ok := bounds[name].(map[string]any)
		if !ok {
			t.Fatalf("no bounds for %s in %v", name, bounds)
		}
		return found
	}
	if got := entry("analysisTimeout"); !reflect.DeepEqual(got, map[string]any{
		"min": float64(accounts.MinAnalysisTimeout), "max": float64(accounts.MaxAnalysisTimeout), "default": float64(accounts.DefaultAnalysisTimeout),
	}) {
		t.Errorf("analysisTimeout: %v", got)
	}
	if got := entry("catalogRefreshMinutes"); got["default"] != float64(60) || got["max"] != float64(1440) {
		t.Errorf("catalogRefreshMinutes: %v", got)
	}
	if got := entry("loginAttempts"); got["zero"] != true || got["min"] != float64(accounts.MinLoginAttempts) || got["default"] != float64(0) {
		t.Errorf("loginAttempts: %v", got)
	}
	if got := entry("hardwareAcceleration"); !reflect.DeepEqual(got["choices"], []any{"auto", "nvenc", "vaapi", "none"}) || got["default"] != "auto" {
		t.Errorf("hardwareAcceleration: %v", got)
	}
	if got := entry("prepareAhead"); !reflect.DeepEqual(got, map[string]any{"default": true}) {
		t.Errorf("prepareAhead: %v", got)
	}
	if got := entry("segmentOrder"); !reflect.DeepEqual(got["default"], []any{"theintrodb", "introdb", "publicmetadb"}) {
		t.Errorf("segmentOrder: %v", got)
	}
	if got := entry("customCss"); got["max"] != float64(accounts.MaxCustomCodeBytes) {
		t.Errorf("customCss: %v", got)
	}
	if got := entry("addVersionsToOpenPage"); !reflect.DeepEqual(got, map[string]any{"default": true}) {
		t.Errorf("addVersionsToOpenPage: %v", got)
	}
	// Every setting a PUT saves has bounds, and every bounds a setting;
	// the secrets are never answered, but have bounds.
	readOnly := map[string]bool{"bounds": true, "conversionHardware": true, "recordingsFolderDefault": true, "backupFolderDefault": true,
		"renderNodes": true, "publicMetaDbKeySet": true, "theIntroDbKeySet": true, "traktClientSecretSet": true, "lastFmSecretSet": true,
		"smtpPasswordSet": true}
	secrets := map[string]bool{"publicMetaDbKey": true, "theIntroDbKey": true, "traktClientSecret": true, "lastFmSecret": true, "smtpPassword": true}
	for name := range body {
		if _, found := bounds[name]; !found && !readOnly[name] {
			t.Errorf("no bounds for %s", name)
		}
	}
	for name := range bounds {
		if _, found := body[name]; !found && !secrets[name] {
			t.Errorf("bounds for %s, which is not a setting", name)
		}
	}
	for _, removed := range []string{"chapters", "downloads", "segmentOrderDefault"} {
		if _, found := bounds[removed]; found {
			t.Errorf("bounds for %s", removed)
		}
	}

	// A PUT sending other bounds changes neither them nor what is checked.
	put := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en",
		"bounds": map[string]any{"analysisTimeout": map[string]any{"min": 1, "max": 999, "default": 1}}}
	status, saved, _ := administrator.call(http.MethodPut, "/settings", put)
	if status != http.StatusOK || !reflect.DeepEqual(saved["bounds"], body["bounds"]) {
		t.Errorf("saving with other bounds: %d %v", status, saved["bounds"])
	}
	tooLong := maps.Clone(put)
	tooLong["analysisTimeout"] = accounts.MaxAnalysisTimeout + 1
	if status, answer, _ := administrator.call(http.MethodPut, "/settings", tooLong); status != http.StatusBadRequest || answer["error"] != "invalid_analysis_timeout" {
		t.Errorf("past the bounds: %d %v", status, answer)
	}
}
