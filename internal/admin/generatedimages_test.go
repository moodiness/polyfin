package admin

import (
	"maps"
	"net/http"
	"testing"
)

// Generate images for collections without one: on by default, saved, and
// kept when a PUT leaves it out.
func TestSettingsGenerateImages(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); body["generateImages"] != true {
		t.Errorf("default: %v", body["generateImages"])
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	off := maps.Clone(base)
	off["generateImages"] = false
	if status, body, _ := administrator.call(http.MethodPut, "/settings", off); status != http.StatusOK || body["generateImages"] != false {
		t.Fatalf("turning it off: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || body["generateImages"] != false {
		t.Errorf("left out: %d %v", status, body)
	}
	if api.store.Settings().GenerateImages {
		t.Error("stored on after a save without it")
	}
}
