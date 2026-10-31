package admin

import (
	"maps"
	"net/http"
	"testing"
)

// The admin API saves the source cache by its preset, which sets its
// limits, and tells each preset's; a limit set alone, as an older page
// sets the cache's size, makes the preset custom.
func TestSourceCachePresetsSetTheCachesLimits(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	put := func(changes map[string]any) map[string]any {
		t.Helper()
		body := maps.Clone(base)
		maps.Copy(body, changes)
		status, saved, _ := administrator.call(http.MethodPut, "/settings", body)
		if status != http.StatusOK {
			t.Fatalf("saving %v: %d %v", changes, status, saved)
		}
		return saved
	}
	check := func(when string, body map[string]any, want map[string]any) {
		t.Helper()
		for name, value := range want {
			if body[name] != value {
				t.Errorf("%s: %s %#v, want %#v", when, name, body[name], value)
			}
		}
	}

	saved := put(map[string]any{"sourceCachePreset": "large", "cacheSizeGb": 5})
	check("large", saved, map[string]any{"sourceCachePreset": "large", "cacheSizeGb": 250.0, "sourceCacheDays": 60.0,
		"sourceCacheCreditDays": 7.0, "preloadWholeFiles": true})
	limits, _ := saved["sourceCachePresetLimits"].(map[string]any)
	if everyday, _ := limits["everyday"].(map[string]any); everyday["cacheSizeGb"] != 50.0 {
		t.Errorf("the presets' limits: %v", limits)
	}
	if got := api.store.Settings(); got.CacheSizeGB != 250 || got.SourceCacheDays != 60 {
		t.Errorf("stored: %d GB, %d days", got.CacheSizeGB, got.SourceCacheDays)
	}

	saved = put(map[string]any{"cacheSizeGb": 300})
	check("a size alone", saved, map[string]any{"sourceCachePreset": "custom", "cacheSizeGb": 300.0, "sourceCacheDays": 60.0})

	// The preset's own values leave it as it is.
	put(map[string]any{"sourceCachePreset": "everyday"})
	saved = put(map[string]any{"cacheSizeGb": 50, "sourceCacheDays": 14})
	check("the preset's values", saved, map[string]any{"sourceCachePreset": "everyday"})
}
