package admin

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/activity"
	"github.com/moodiness/polyfin/internal/forwarded"
)

// Users › Access and the group page set whether a user may connect from
// outside the local network. Without it, the admin app refuses their
// sign-in from there, administrators included, and every request but
// reading and ending the session, as Known proxies tells the network; the
// refused sign-in is in the activity log.
func TestRemoteAccessInTheAdminApp(t *testing.T) {
	t.Parallel()
	var options *Options
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) { options = o })
	admin := api.signedIn("admin", true)
	adminID := api.users(t, admin)[0]["id"].(string)
	// The same API behind a reverse proxy on 127.0.0.1, a known proxy, as
	// Polyfin serves it.
	proxy := httptest.NewServer(forwarded.New(api.store.KnownProxies, nil).Handler(New(*options)))
	t.Cleanup(proxy.Close)
	settings := api.store.Settings()
	settings.KnownProxies = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}
	if _, err := api.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	// from calls through the proxy for the client at address, in b's
	// session.
	from := func(b browser, address, method, path string, body any) (int, map[string]any) {
		t.Helper()
		b.api.url = proxy.URL
		status, answer, _ := b.call(method, path, body, "X-Forwarded-For", address)
		return status, answer
	}
	const home, away = "192.168.1.20", "203.0.113.9"

	// A new user may; the user page takes it away.
	status, kid, _ := admin.call(http.MethodPost, "/users", map[string]any{"name": "kid", "password": "correct horse"})
	if status != http.StatusCreated || kid["remoteAccess"] != true {
		t.Fatalf("a new user: %d %v", status, kid)
	}
	id := kid["id"].(string)
	if status, kid, _ = admin.call(http.MethodPatch, "/users/"+id, map[string]any{"remoteAccess": false}); status != http.StatusOK || kid["remoteAccess"] != false {
		t.Fatalf("taking it away: %d %v", status, kid)
	}

	signIn := map[string]string{"name": "kid", "password": "correct horse"}
	if status, body := from(api.browser(), away, http.MethodPost, "/session", signIn); status != http.StatusForbidden || body["error"] != "outside_local_network" {
		t.Errorf("signing in from the internet: %d %v", status, body)
	}
	b := api.browser()
	if status, body := from(b, home, http.MethodPost, "/session", signIn); status != http.StatusOK {
		t.Fatalf("signing in from the local network: %d %v", status, body)
	}
	if status, body := from(b, home, http.MethodGet, "/account/two-step", nil); status != http.StatusOK {
		t.Errorf("a request from the local network: %d %v", status, body)
	}
	// The browser taken outside: refused, but it still reads and ends
	// its session.
	if status, body := from(b, away, http.MethodGet, "/account/two-step", nil); status != http.StatusForbidden || body["error"] != "outside_local_network" {
		t.Errorf("a request from the internet: %d %v", status, body)
	}
	if status, body := from(b, away, http.MethodGet, "/session", nil); status != http.StatusOK {
		t.Errorf("reading the session from the internet: %d %v", status, body)
	}
	if status, body := from(b, away, http.MethodDelete, "/session", nil); status != http.StatusNoContent {
		t.Errorf("signing out from the internet: %d %v", status, body)
	}
	page, err := options.Activity.Entries(t.Context(), activity.Query{Types: []string{"AuthenticationFailed"}})
	if err != nil || page.Total != 1 || page.Entries[0].UserID == nil || page.Entries[0].UserID.String() != id ||
		page.Entries[0].ShortOverview == nil || !strings.Contains(*page.Entries[0].ShortOverview, away) {
		t.Errorf("refused sign-ins: %+v %v", page.Entries, err)
	}

	// From outside, an administrator cannot take their own away, which
	// would lock them out; another's they can. From home they can.
	if status, body := from(admin, away, http.MethodPatch, "/users/"+adminID, map[string]any{"remoteAccess": false}); status != http.StatusConflict ||
		body["error"] != "remote_access_self" {
		t.Errorf("the administrator's own from the internet: %d %v", status, body)
	}
	if status, body := from(admin, away, http.MethodPatch, "/users/"+id, map[string]any{"remoteAccess": false, "name": "kid"}); status != http.StatusOK {
		t.Errorf("another's from the internet: %d %v", status, body)
	}
	for _, user := range api.users(t, admin) {
		if user["id"] == adminID && user["remoteAccess"] != true {
			t.Fatalf("the administrator's own after the refusal: %v", user["remoteAccess"])
		}
	}
	if status, body := from(admin, home, http.MethodPatch, "/users/"+adminID, map[string]any{"remoteAccess": false}); status != http.StatusOK {
		t.Fatalf("the administrator's own from the local network: %d %v", status, body)
	}
	// Without it, they are refused from outside as well, and served at home.
	if status, body := from(admin, away, http.MethodGet, "/users", nil); status != http.StatusForbidden || body["error"] != "outside_local_network" {
		t.Errorf("the administrator from the internet: %d %v", status, body)
	}
	if status, body := from(admin, home, http.MethodGet, "/users", nil); status != http.StatusOK {
		t.Errorf("the administrator from the local network: %d %v", status, body)
	}

	// A group holds it for the members who follow it.
	status, group, _ := admin.call(http.MethodPost, "/groups", map[string]any{"name": "Home only"})
	if status != http.StatusCreated || group["remoteAccess"] != true {
		t.Fatalf("a new group: %d %v", status, group)
	}
	if status, group, _ = admin.call(http.MethodPatch, "/groups/"+group["id"].(string), map[string]any{"remoteAccess": false}); status != http.StatusOK || group["remoteAccess"] != false {
		t.Fatalf("the group's: %d %v", status, group)
	}
	status, cat, _ := admin.call(http.MethodPost, "/users", map[string]any{"name": "cat", "password": "correct horse", "groupId": group["id"]})
	if status != http.StatusCreated || cat["remoteAccess"] != false || cat["followGroup"] != true {
		t.Errorf("a member of the group: %d %v", status, cat)
	}
}
