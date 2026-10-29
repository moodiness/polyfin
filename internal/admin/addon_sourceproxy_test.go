package admin

import (
	"net/http"
	"testing"
)

// The source proxy is off by default, saved per server addon, and reported
// as available only once its URL is set.
func TestAddonSourceProxy(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": localAddon(t)})
	if status != http.StatusCreated {
		t.Fatalf("installing: %d %v", status, body)
	}
	id, _ := body["id"].(string)
	if body["useSourceProxy"] != false || body["sourceProxyAvailable"] != false {
		t.Fatalf("default: %v", body)
	}
	status, body, _ = administrator.call(http.MethodPatch, "/scopes/shared/addons/"+id, map[string]any{"useSourceProxy": true})
	if status != http.StatusOK || body["useSourceProxy"] != true {
		t.Fatalf("turning on: %d %v", status, body)
	}
	settings := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en",
		"sourceProxy": "socks5h://warp:1080"}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK {
		t.Fatalf("saving the proxy: %d %v", status, body)
	}
	var list []addonJSON
	if status := administrator.raw(http.MethodGet, "/scopes/shared/addons", "", &list); status != http.StatusOK {
		t.Fatalf("listing: %d", status)
	}
	if len(list) != 1 || !list[0].UseSourceProxy || !list[0].SourceProxyAvailable {
		t.Fatalf("listed: %+v", list)
	}
}
