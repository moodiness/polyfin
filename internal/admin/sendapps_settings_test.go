package admin

import (
	"maps"
	"net/http"
	"testing"
)

// Send apps to the source is off on a new server and on one upgraded:
// only an administrator who saves it on lets apps be sent to the sources'
// addresses. A save that leaves it out, as a page or script older than
// the setting sends, keeps it as it is.
func TestSendAppsToTheSourceIsOffUntilSavedOn(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); body["sendAppsToSource"] != false {
		t.Errorf("default: %v", body["sendAppsToSource"])
	}
	if api.store.Settings().SendAppsToSource {
		t.Error("stored on by default")
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "legacyAuthorization": false, "language": "en"}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || body["sendAppsToSource"] != false {
		t.Errorf("a save without it: %d %v", status, body["sendAppsToSource"])
	}
	on := maps.Clone(base)
	on["sendAppsToSource"] = true
	if status, body, _ := administrator.call(http.MethodPut, "/settings", on); status != http.StatusOK || body["sendAppsToSource"] != true {
		t.Fatalf("saving it on: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || body["sendAppsToSource"] != true {
		t.Errorf("a save without it, once on: %d %v", status, body["sendAppsToSource"])
	}
	if !api.store.Settings().SendAppsToSource {
		t.Error("not stored once saved on")
	}
	member := api.signedIn("member", false)
	off := maps.Clone(base)
	off["sendAppsToSource"] = false
	if status, _, _ := member.call(http.MethodPut, "/settings", off); status != http.StatusForbidden {
		t.Errorf("a member's save: %d", status)
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", off); status != http.StatusOK || body["sendAppsToSource"] != false ||
		api.store.Settings().SendAppsToSource {
		t.Errorf("saving it off: %d %v", status, body["sendAppsToSource"])
	}
}
