package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func localAddon(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id": "local", "name": "Local", "version": "1.0.0", "resources": ["catalog"],
			"catalogs": [{"type": "movie", "id": "top", "name": "Top"}]}`))
	}))
	t.Cleanup(server.Close)
	return server.URL + "/u/secret-token/manifest.json"
}

func TestAddonScopes(t *testing.T) {
	api := newTestAPI(t, 10)
	manifestURL := localAddon(t)
	member := api.signedIn("member", false)
	administrator := api.signedIn("administrator", true)

	if status, body, _ := member.call(http.MethodGet, "/scopes/shared/addons", nil); status != http.StatusForbidden || body["error"] != "forbidden" {
		t.Errorf("member reading shared addons: %d %v", status, body)
	}
	// Only administrators may make the server reach local networks.
	if status, body, _ := member.call(http.MethodPost, "/scopes/me/addons", map[string]string{"manifestUrl": manifestURL}); status != http.StatusForbidden || body["error"] != "private_network" {
		t.Errorf("member installing a local addon: %d %v", status, body)
	}
	status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": manifestURL})
	if status != http.StatusCreated {
		t.Fatalf("administrator installing a local addon: %d %v", status, body)
	}
	if url, _ := body["manifestUrl"].(string); strings.Contains(url, "secret-token") || !strings.HasSuffix(url, "/…/manifest.json") {
		t.Errorf("manifest URL not redacted: %q", url)
	}
	if status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": "https://example.com/catalog.json"}); status != http.StatusBadRequest || body["error"] != "invalid_manifest_url" {
		t.Errorf("invalid URL: %d %v", status, body)
	}
}

func TestAddonPreferences(t *testing.T) {
	api := newTestAPI(t, 10)
	member := api.signedIn("member", false)
	if _, body, _ := member.call(http.MethodGet, "/account/addon-preferences", nil); body["useSharedAddons"] != true {
		t.Errorf("default preference: %v", body)
	}
	member.call(http.MethodPut, "/account/addon-preferences", map[string]bool{"useSharedAddons": false})
	if _, body, _ := member.call(http.MethodGet, "/account/addon-preferences", nil); body["useSharedAddons"] != false {
		t.Errorf("saved preference: %v", body)
	}
}
