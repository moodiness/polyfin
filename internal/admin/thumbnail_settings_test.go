package admin

import (
	"maps"
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

func TestSettingsThumbnails(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	thumbnails := func(body map[string]any) [5]any {
		return [5]any{body["trickplay"], body["trickplayInterval"], body["trickplayWidth"], body["chapterImages"], body["thumbnailStorageGB"]}
	}
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); thumbnails(body) != [5]any{false, float64(10), float64(320), false, float64(2)} {
		t.Errorf("default settings: %v", body)
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "legacyAuthorization": false, "language": "en"}
	settings := maps.Clone(base)
	maps.Copy(settings, map[string]any{"trickplay": true, "trickplayInterval": 30, "trickplayWidth": 480, "chapterImages": true, "thumbnailStorageGB": 12})
	saved := [5]any{true, float64(30), float64(480), true, float64(12)}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK || thumbnails(body) != saved {
		t.Fatalf("saving the thumbnail settings: %d %v", status, body)
	}
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); thumbnails(body) != saved {
		t.Errorf("settings after saving: %v", body)
	}
	// A page or script older than them leaves them as they are.
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || thumbnails(body) != saved {
		t.Errorf("saving without the thumbnail settings: %d %v", status, body)
	}
	if got := api.store.Settings(); !got.Trickplay || got.TrickplayInterval != 30 || got.TrickplayWidth != 480 || !got.ChapterImages || got.ThumbnailStorageGB != 12 {
		t.Errorf("stored after a save without them: %+v", got)
	}
	for _, tc := range []struct {
		key   string
		value any
		code  string
	}{
		{"trickplayInterval", accounts.MinTrickplayInterval - 1, "invalid_trickplay_interval"},
		{"trickplayInterval", accounts.MaxTrickplayInterval + 1, "invalid_trickplay_interval"},
		{"trickplayWidth", 360, "invalid_trickplay_width"},
		{"thumbnailStorageGB", accounts.MinThumbnailStorageGB - 1, "invalid_thumbnail_storage_gb"},
		{"thumbnailStorageGB", accounts.MaxThumbnailStorageGB + 1, "invalid_thumbnail_storage_gb"},
	} {
		refused := maps.Clone(base)
		refused[tc.key] = tc.value
		if status, body, _ := administrator.call(http.MethodPut, "/settings", refused); status != http.StatusBadRequest || body["error"] != tc.code {
			t.Errorf("%s %v: %d %v", tc.key, tc.value, status, body)
		}
	}
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); thumbnails(body) != saved {
		t.Errorf("a refused value changed the settings: %v", body)
	}
}
