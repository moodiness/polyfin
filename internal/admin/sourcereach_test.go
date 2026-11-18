package admin

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio/reachtest"
)

// askedAddon serves an addon's manifest, counting the requests for it.
func askedAddon(t *testing.T) (address string, asked *atomic.Int32) {
	t.Helper()
	asked = new(atomic.Int32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked.Add(1)
		_, _ = w.Write([]byte(`{"id": "asked", "name": "Asked", "version": "1.0.0", "resources": ["catalog"],
			"catalogs": [{"type": "movie", "id": "top", "name": "Top"}]}`))
	}))
	t.Cleanup(server.Close)
	return server.URL, asked
}

// The admin API reads a source where it is, and says where that is: an
// administrator's addon on a public address is told public, one on the
// local network local, and a member's is read from public addresses only.
// Refresh tells it again, and the health check asks an addon as its own
// requests are made: a member's addon on the local network is not asked.
func TestTheAdminAPIReadsASourceWhereItIs(t *testing.T) {
	t.Parallel()
	var deps testDeps
	var network *reachtest.Network
	api := newTestAPI(t, 10, func(o *Options, d testDeps) {
		deps = d
		network = reachtest.New(d.client)
		o.Health = HealthSources{Addons: d.client}
	})
	admin, bob := api.signedIn("root", true), api.signedIn("bob", false)
	local, localAsked := askedAddon(t)
	internet, publicAsked := askedAddon(t)
	public := network.Public(t, internet)
	bobs, bobsAsked := askedAddon(t)
	hidden, hiddenAsked := askedAddon(t)

	install := func(b browser, scope, address string) (int, map[string]any) {
		status, body, _ := b.call(http.MethodPost, "/scopes/"+scope+"/addons", map[string]string{"manifestUrl": address + "/manifest.json"})
		return status, body
	}
	status, onInternet := install(admin, "shared", public)
	if status != http.StatusCreated || onInternet["network"] != "public" {
		t.Fatalf("the server's addon on a public address: %d %v", status, onInternet["network"])
	}
	status, onLAN := install(admin, "shared", local)
	if status != http.StatusCreated || onLAN["network"] != "local" {
		t.Fatalf("the server's addon on the local network: %d %v", status, onLAN["network"])
	}
	if status, own := install(admin, "me", network.Serve(t, "192.168.1.7", local)); status != http.StatusCreated || own["network"] != "local" {
		t.Errorf("an administrator's own addon on the local network: %d %v", status, own["network"])
	}
	status, members := install(bob, "me", network.Public(t, bobs))
	if status != http.StatusCreated || members["network"] != "public" {
		t.Fatalf("a member's addon on a public address: %d %v", status, members["network"])
	}
	if status, body := install(bob, "me", hidden); status != http.StatusForbidden || body["error"] != "private_network" || hiddenAsked.Load() != 0 {
		t.Errorf("a member's addon on the local network: %d %v, asked %d times", status, body, hiddenAsked.Load())
	}
	// A member's addon that was read from the local network, as one whose
	// owner was an administrator then.
	kept, err := deps.addons.Install(t.Context(), addons.Personal(api.userID("bob")), hidden+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}

	// Refresh reads each where it is, again.
	for _, c := range []struct {
		b       browser
		scope   string
		addon   map[string]any
		network string
		asked   *atomic.Int32
	}{{admin, "shared", onInternet, "public", publicAsked}, {admin, "shared", onLAN, "local", localAsked}, {bob, "me", members, "public", bobsAsked}} {
		before := c.asked.Load()
		status, body, _ := c.b.call(http.MethodPost, "/scopes/"+c.scope+"/addons/"+c.addon["id"].(string)+"/refresh", nil)
		if status != http.StatusOK || body["network"] != c.network || c.asked.Load()-before != 1 {
			t.Errorf("refreshing the addon on the %s network: %d %v, asked %d times", c.network, status, body["network"], c.asked.Load()-before)
		}
	}
	before := hiddenAsked.Load()
	if status, body, _ := bob.call(http.MethodPost, "/scopes/me/addons/"+kept.ID.String()+"/refresh", nil); status != http.StatusForbidden ||
		body["error"] != "private_network" || hiddenAsked.Load() != before {
		t.Errorf("a member refreshing their addon on the local network: %d %v, asked %d times", status, body, hiddenAsked.Load()-before)
	}

	// The health check asks an addon as its own requests are made.
	for _, c := range []struct {
		name  string
		id    string
		asked *atomic.Int32
		want  int32
	}{{"the server's public addon", onInternet["id"].(string), publicAsked, 1}, {"the server's local addon", onLAN["id"].(string), localAsked, 1},
		{"a member's public addon", members["id"].(string), bobsAsked, 1}, {"a member's local addon", kept.ID.String(), hiddenAsked, 0}} {
		before := c.asked.Load()
		if status := admin.raw(http.MethodPost, "/health/addons/"+c.id+"/check", "", nil); status != http.StatusOK || c.asked.Load()-before != c.want {
			t.Errorf("checking %s: %d, asked %d times", c.name, status, c.asked.Load()-before)
		}
	}
}
