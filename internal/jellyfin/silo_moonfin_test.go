package jellyfin

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// A Moonfin app signed in through Silo's API finds the companion among the
// profile's plugins, and calls its routes under the plugin's content path,
// with the answers of the Jellyfin API's for the same user. A user the
// companion is not served to finds no plugin, and its routes answer 404.
func TestSiloAppsFindMoonfinAmongThePlugins(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("alice", nil)
	bob := s.user("bob", nil)
	allowSilo(t, s)
	a := servingSilo(t, s)
	token, profile := a.signIn(t, "alice")

	var plugins struct {
		Items []struct {
			ID       string `json:"id"`
			PluginID string `json:"plugin_id"`
			Version  string `json:"version"`
			Routes   []any  `json:"routes"`
		} `json:"items"`
	}
	a.get(t, "/api/v2/settings/plugins", token, profile, &plugins)
	if len(plugins.Items) != 1 || plugins.Items[0].PluginID != "moonfin.moonbase" || plugins.Items[0].ID == "" || plugins.Items[0].Routes == nil {
		t.Fatalf("plugins: %+v", plugins)
	}
	base := "/api/v2/plugin-content/plugins/" + plugins.Items[0].ID
	status, data := a.do(t, http.MethodGet, base+"/Moonfin/Ping?profile=tv", token, profile, nil)
	var ping map[string]any
	if status != http.StatusOK || json.Unmarshal(data, &ping) != nil || ping["installed"] != true {
		t.Fatalf("ping: %d %s", status, data)
	}
	if status, data = a.do(t, http.MethodPost, base+"/Moonfin/Settings/Profile/global", token, profile,
		map[string]any{"profile": map[string]any{"theme": "dark"}, "clientId": "moonfin-flutter"}); status != http.StatusOK {
		t.Fatalf("save: %d %s", status, data)
	}
	var resolved map[string]any
	if status, data = a.do(t, http.MethodGet, base+"/Moonfin/Settings/Resolved/mobile", token, profile, nil); status != http.StatusOK ||
		json.Unmarshal(data, &resolved) != nil || resolved["theme"] != "dark" {
		t.Errorf("resolved: %d %s", status, data)
	}
	// What the Jellyfin API keeps for alice is what her Silo app saved.
	jellyfin := app("phone", s.signIn("alice", "phone"))
	if status, data = s.call(http.MethodGet, "/Moonfin/Settings/Resolved/mobile", jellyfin, nil); status != http.StatusOK ||
		json.Unmarshal(data, &resolved) != nil || resolved["theme"] != "dark" {
		t.Errorf("resolved through the Jellyfin API: %d %s", status, data)
	}
	for _, path := range []string{"/api/v2/plugin-content/plugins/other/Moonfin/Ping", base + "/Items"} {
		if status, data = a.do(t, http.MethodGet, path, token, profile, nil); status != http.StatusNotFound {
			t.Errorf("%s: %d %s", path, status, data)
		}
	}

	off := accounts.MoonfinOff
	if _, err := s.store.UpdateUser(t.Context(), bob.ID, accounts.UserChanges{Moonfin: &off}, nil); err != nil {
		t.Fatal(err)
	}
	bobToken, bobProfile := a.signIn(t, "bob")
	a.get(t, "/api/v2/settings/plugins", bobToken, bobProfile, &plugins)
	if len(plugins.Items) != 0 {
		t.Errorf("bob's plugins: %+v", plugins)
	}
	if status, _ = a.do(t, http.MethodGet, base+"/Moonfin/Ping", bobToken, bobProfile, nil); status != http.StatusNotFound {
		t.Errorf("bob's ping: %d", status)
	}
}
