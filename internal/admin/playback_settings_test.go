package admin

import (
	"fmt"
	"maps"
	"net/http"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

func TestSettingsPlaybackChoices(t *testing.T) {
	t.Parallel()
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

// RemuxDB is off by default, with its public server's address; a PUT
// saves both, one leaving them out keeps them, and an invalid address is
// refused without saving anything.
func TestSettingsRemuxDB(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	remuxDB := func(body map[string]any) [2]any { return [2]any{body["remuxDb"], body["remuxDbUrl"]} }
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); remuxDB(body) != [2]any{false, accounts.DefaultRemuxDBURL} {
		t.Errorf("default settings: %v", remuxDB(body))
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "legacyAuthorization": false, "language": "en"}
	settings := maps.Clone(base)
	maps.Copy(settings, map[string]any{"remuxDb": true, "remuxDbUrl": "https://remuxdb.example/base/"})
	saved := [2]any{true, "https://remuxdb.example/base"}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK || remuxDB(body) != saved {
		t.Fatalf("saving RemuxDB: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || remuxDB(body) != saved {
		t.Errorf("saving without RemuxDB: %d %v", status, body)
	}
	refused := maps.Clone(base)
	maps.Copy(refused, map[string]any{"remuxDb": false, "remuxDbUrl": "ftp://remuxdb.example"})
	if status, body, _ := administrator.call(http.MethodPut, "/settings", refused); status != http.StatusBadRequest || body["error"] != "invalid_remuxdb_url" {
		t.Errorf("an invalid address: %d %v", status, body)
	}
	if got := api.store.Settings(); !got.RemuxDB || got.RemuxDBURL != "https://remuxdb.example/base" {
		t.Errorf("stored after a refused address: %v %q", got.RemuxDB, got.RemuxDBURL)
	}
}

// The version rules are saved as sent, the words trimmed and each once; a
// PUT leaving them out keeps them, and one breaking them is refused with
// its code.
func TestSettingsVersionRules(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "legacyAuthorization": false, "language": "en"}
	settings := maps.Clone(base)
	maps.Copy(settings, map[string]any{"versionOrder": []string{"language", "resolution-high"}, "hideVersionsLargerGb": 40,
		"hiddenVersionCodecs": []string{"av1"}, "hiddenVersionWords": []string{" Downloading", "downloading", ""}})
	rules := func(body map[string]any) string {
		return fmt.Sprint(body["versionOrder"], body["versionCodecOrder"], body["hideVersionsLargerGb"], body["hiddenVersionCodecs"],
			body["hiddenVersionWords"])
	}
	want := "[language resolution-high] [av1 hevc vp9 h264 vc1 mpeg4 mpeg2video] 40 [av1] [Downloading]"
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK || rules(body) != want {
		t.Fatalf("saving the version rules: %d %s", status, rules(body))
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || rules(body) != want {
		t.Errorf("saving without them: %d %s", status, rules(body))
	}
	for _, tc := range []struct {
		key   string
		value any
		code  string
	}{
		{"versionOrder", []string{"size-large", "size-small"}, "invalid_version_order"},
		{"versionCodecOrder", []string{"av1"}, "invalid_version_codec_order"},
		{"hideVersionsLargerGb", -1, "invalid_hide_versions_larger"},
		{"hiddenVersionCodecs", []string{"theora"}, "invalid_hidden_version_codecs"},
		{"hiddenVersionWords", []string{strings.Repeat("a", accounts.MaxHiddenVersionWordLength+1)}, "invalid_hidden_version_words"},
	} {
		refused := maps.Clone(base)
		refused[tc.key] = tc.value
		if status, body, _ := administrator.call(http.MethodPut, "/settings", refused); status != http.StatusBadRequest || body["error"] != tc.code {
			t.Errorf("%s %v: %d %v", tc.key, tc.value, status, body)
		}
	}
}

// The default audio and subtitle languages are none by default; a PUT
// saves a language users' own preferences offer, or none again, one
// leaving them out keeps them, and another code is refused.
func TestSettingsDefaultLanguages(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	languages := func(body map[string]any) [2]any {
		return [2]any{body["defaultAudioLanguage"], body["defaultSubtitleLanguage"]}
	}
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); languages(body) != [2]any{"", ""} {
		t.Errorf("default settings: %v", languages(body))
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "legacyAuthorization": false, "language": "en"}
	settings := maps.Clone(base)
	maps.Copy(settings, map[string]any{"defaultAudioLanguage": "fre", "defaultSubtitleLanguage": "ger"})
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK || languages(body) != [2]any{"fre", "ger"} {
		t.Fatalf("saving the languages: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || languages(body) != [2]any{"fre", "ger"} {
		t.Errorf("saving without them: %d %v", status, languages(body))
	}
	for _, code := range []string{"fra", "fr", "xx", "FRE"} {
		refused := maps.Clone(base)
		refused["defaultAudioLanguage"] = code
		if status, body, _ := administrator.call(http.MethodPut, "/settings", refused); status != http.StatusBadRequest || body["error"] != "invalid_default_language" {
			t.Errorf("%q: %d %v", code, status, body)
		}
	}
	maps.Copy(settings, map[string]any{"defaultAudioLanguage": "", "defaultSubtitleLanguage": ""})
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK || languages(body) != [2]any{"", ""} {
		t.Errorf("saving none again: %d %v", status, languages(body))
	}
}
