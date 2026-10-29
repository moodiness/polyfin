package admin

import (
	"context"
	"net/http"
	"testing"
)

// The source proxy is empty by default, saved trimmed, kept when a PUT
// leaves it out, and refused when it is not a proxy URL.
func TestSettingsSourceProxy(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); body["sourceProxy"] != "" {
		t.Errorf("default: %#v", body["sourceProxy"])
	}
	body := func(proxy string) map[string]any {
		return map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en", "sourceProxy": proxy}
	}
	if status, answer, _ := administrator.call(http.MethodPut, "/settings", body(" socks5h://warp:1080 ")); status != http.StatusOK ||
		answer["sourceProxy"] != "socks5h://warp:1080" {
		t.Fatalf("saving: %d %v", status, answer)
	}
	if status, answer, _ := administrator.call(http.MethodPut, "/settings", map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}); status != http.StatusOK ||
		answer["sourceProxy"] != "socks5h://warp:1080" {
		t.Errorf("left out: %d %v", status, answer["sourceProxy"])
	}
	if status, answer, _ := administrator.call(http.MethodPut, "/settings", body("warp:1080")); status != http.StatusBadRequest || answer["error"] != "invalid_source_proxy" {
		t.Errorf("invalid: %d %v", status, answer)
	}
	if status, answer, _ := administrator.call(http.MethodPut, "/settings", body("")); status != http.StatusOK || answer["sourceProxy"] != "" {
		t.Errorf("emptied: %d %v", status, answer["sourceProxy"])
	}
}

// Detection answers the proxy installed beside Polyfin and its URL.
func TestSourceProxyDetection(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.SourceProxyDetect = func(context.Context) (string, bool) { return "socks5h://warp:1080", true }
	})
	administrator := api.signedIn("administrator", true)
	status, body, _ := administrator.call(http.MethodGet, "/settings/source-proxy", nil)
	if status != http.StatusOK || body["available"] != true || body["url"] != "socks5h://warp:1080" {
		t.Errorf("detection: %d %v", status, body)
	}
}
