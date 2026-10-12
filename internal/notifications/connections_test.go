package notifications

import (
	"strings"
	"testing"
)

// A connection that stopped is told to its user's targets and the
// server's, naming the app or service and the user, with the page where
// the user connects it again, and its kind for programs.
func TestStoppedConnectionsReachTheirUserAndTheServer(t *testing.T) {
	h := newHarness(t)
	settings := h.store.Settings()
	settings.PublicAddress = "https://media.example.org"
	if _, err := h.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	h.add(t, &h.member, h.webhook("/member", ConnectionStopped))
	h.add(t, &h.admin, h.webhook("/admin", ConnectionStopped))
	h.add(t, nil, h.webhook("/server", ConnectionStopped))
	// A target that did not choose the event hears nothing of it.
	h.add(t, &h.member, h.webhook("/episodes", NewEpisode))

	h.ConnectionStopped(h.member.ID, ConnectionApp, "stremio")
	h.targets.wait(t, "/member", 1)
	h.ConnectionStopped(h.member.ID, ConnectionTracking, "trakt")
	h.targets.wait(t, "/member", 2)
	h.targets.wait(t, "/server", 2)
	h.idle(t)
	want := []struct{ kind, service, name, url string }{
		{"app", "stremio", "Stremio", "https://media.example.org/admin/me/account#app-import"},
		{"tracking", "trakt", "Trakt", "https://media.example.org/admin/me/account#tracking"},
	}
	for _, path := range []string{"/member", "/server"} {
		for i, got := range h.targets.at(path) {
			connection, _ := got.body["connection"].(map[string]any)
			user, _ := got.body["user"].(map[string]any)
			message, _ := got.body["message"].(string)
			if got.body["type"] != ConnectionStopped || connection["kind"] != want[i].kind || connection["service"] != want[i].service ||
				connection["name"] != want[i].name || got.body["url"] != want[i].url || user["name"] != "member" ||
				user["id"] != h.member.ID.String() || !strings.Contains(message, want[i].name) || !strings.Contains(message, "member") {
				t.Errorf("%s %d: %v", path, i, got.body)
			}
		}
	}
	if got := len(h.targets.at("/admin")) + len(h.targets.at("/episodes")); got != 0 {
		t.Errorf("%d messages reached targets of other users or events", got)
	}
}
