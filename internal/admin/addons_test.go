package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
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

// appNames lists the names apps show for a scope's libraries, "" for the
// libraries they do not show.
func (b browser) appNames(scope string) []string {
	b.api.t.Helper()
	response, err := b.client.Get(b.api.url + "/admin/api/scopes/" + scope + "/libraries")
	if err != nil {
		b.api.t.Fatal(err)
	}
	defer response.Body.Close()
	var libraries []struct {
		AppName *string `json:"appName"`
	}
	if err := json.NewDecoder(response.Body).Decode(&libraries); err != nil {
		b.api.t.Fatal(err)
	}
	names := make([]string, len(libraries))
	for i, library := range libraries {
		if library.AppName != nil {
			names[i] = *library.AppName
		}
	}
	return names
}

func TestLibrariesShowTheNameAppsShow(t *testing.T) {
	api := newTestAPI(t, 10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id": "local", "name": "Local", "version": "1.0.0", "resources": ["catalog"],
			"catalogs": [{"type": "movie", "id": "top", "name": "Top"}, {"type": "series", "id": "top", "name": "Top"},
				{"type": "movie", "id": "search", "name": "Search", "extra": [{"name": "search", "isRequired": true}]}]}`))
	}))
	defer server.Close()
	administrator := api.signedIn("administrator", true)
	for _, scope := range []string{"shared", "me"} {
		if status, body, _ := administrator.call(http.MethodPost, "/scopes/"+scope+"/addons", map[string]string{"manifestUrl": server.URL + "/manifest.json"}); status != http.StatusCreated {
			t.Fatalf("installing in %s: %d %v", scope, status, body)
		}
	}
	if got := administrator.appNames("shared"); !slices.Equal(got, []string{"Top (Movies)", "Top (Shows)", ""}) {
		t.Errorf("shared libraries: %q", got)
	}
	// A user's own libraries are named after the server's they follow.
	want := []string{"Top (Movies, Local) (2)", "Top (Shows, Local) (2)", ""}
	if got := administrator.appNames("me"); !slices.Equal(got, want) {
		t.Errorf("own libraries: %q, want %q", got, want)
	}
	settings := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "fr"}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK {
		t.Fatalf("saving French: %d %v", status, body)
	}
	if got := administrator.appNames("shared"); !slices.Equal(got, []string{"Top (Films)", "Top (Séries)", ""}) {
		t.Errorf("shared libraries in French: %q", got)
	}
}
