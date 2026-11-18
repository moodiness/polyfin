package admin

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/appimport/nuvio/nuviotest"
	"github.com/moodiness/polyfin/internal/jellyfinimport"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
)

// localServer serves, on the loopback, as a server of Polyfin's local
// network, what the server at target answers, and keeps the requests it
// received.
type localServer struct {
	url string
	mu  sync.Mutex
	got []string
}

func newLocalServer(t *testing.T, target string) *localServer {
	t.Helper()
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	s := &localServer{}
	proxy := &httputil.ReverseProxy{Rewrite: func(r *httputil.ProxyRequest) { r.SetURL(parsed) }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.got = append(s.got, r.Method+" "+r.URL.RequestURI())
		s.mu.Unlock()
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	s.url = server.URL
	return s
}

// requests lists the requests the server received.
func (s *localServer) requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.got)
}

// redirectingJellyfin serves, until the test ends, a server standing for
// one on a public address that leads to the one at local. The first
// segment of a request's path names what it does with the rest:
// "probe-302" and "probe-307" answer every request with that redirect to
// the same path and query at local; "sign-in-302" and "sign-in-307" say
// themselves which server they are, as Jellyfin does, and redirect the
// sign-in and all that follows.
func redirectingJellyfin(t *testing.T, local string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, rest, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
		if strings.HasPrefix(name, "sign-in-") && rest == "System/Info/Public" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"Id":"f1e2d3","ServerName":"Home","Version":"10.10.7","ProductName":"Jellyfin Server"}`)
			return
		}
		status := http.StatusTemporaryRedirect
		if strings.HasSuffix(name, "-302") {
			status = http.StatusFound
		}
		r.URL.Path = "/" + rest
		http.Redirect(w, r, local+r.URL.RequestURI(), status)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// ownImportDone waits for the import b last started for themselves to
// end, and fails unless it is done.
func ownImportDone(t *testing.T, b browser) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		_, body, _ := b.call(http.MethodGet, "/account/server-import", nil)
		if current, _ := body["import"].(map[string]any); current != nil && current["state"] != "running" {
			if current["state"] != "done" {
				t.Fatalf("import: %v", current)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the import never ended: %v", body)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A member's import from another server takes a server on a public
// address, as their addons do: one on the local network, or one that
// redirects there, at its first answer or at the sign-in, answers 403
// private_network, and the local server receives nothing. An
// administrator's imports, for the server and for their own account,
// reach the same addresses.
func TestAMembersOwnImportIsFromAServerOnAPublicAddress(t *testing.T) {
	t.Parallel()
	local := newLocalServer(t, fakeJellyfin(t))
	front := redirectingJellyfin(t, local.url)
	api, _, _ := confinedJellyfinImportAPI(t, nuviotest.Confined(t, front))
	admin, bob := api.signedIn("root", true), api.signedIn("bob", false)

	for _, server := range []struct{ kind, address string }{
		{"jellyfin", local.url}, {"emby", local.url},
		{"jellyfin", front + "/probe-302"}, {"emby", front + "/probe-302"},
		{"jellyfin", front + "/probe-307"}, {"emby", front + "/probe-307"},
		{"jellyfin", front + "/sign-in-302"}, {"jellyfin", front + "/sign-in-307"},
	} {
		asBob := map[string]any{"kind": server.kind, "address": server.address, "name": "Bob", "password": "bob's password"}
		if status, body, _ := bob.call(http.MethodPost, "/account/server-import", asBob); status != http.StatusForbidden ||
			body["error"] != "private_network" {
			t.Errorf("Bob's %s server at %s: %d %v", server.kind, server.address, status, body)
		}
		if got := local.requests(); len(got) != 0 {
			t.Fatalf("Bob's %s server at %s: the local server received %v", server.kind, server.address, got)
		}
	}
	if _, body, _ := bob.call(http.MethodGet, "/account/server-import", nil); body["import"] != nil {
		t.Errorf("a refused address started an import: %v", body)
	}

	for _, address := range []string{local.url, front + "/probe-307", front + "/sign-in-307"} {
		asAlice := map[string]any{"kind": "jellyfin", "address": address, "name": "Alice", "password": "alice's password"}
		if status, body, _ := admin.call(http.MethodPost, "/account/server-import", asAlice); status != http.StatusOK {
			t.Fatalf("the administrator's own import at %s: %d %v", address, status, body)
		}
		ownImportDone(t, admin)
	}
	for _, address := range []string{local.url, front + "/probe-307"} {
		connection := map[string]any{"address": address, "apiKey": jellyfinKey}
		status, body, _ := admin.call(http.MethodPost, "/jellyfin-import/users", connection)
		if users, _ := body["users"].([]any); status != http.StatusOK || len(users) != 3 {
			t.Errorf("the administrator's import at %s: %d %v", address, status, body)
		}
	}
	if got := local.requests(); !slices.Contains(got, "POST /Users/AuthenticateByName") || !slices.Contains(got, "GET /Users") {
		t.Errorf("the administrator's imports sent the local server %v", got)
	}
}

// A member's import from Plex asks none of the addresses on the local
// network that plex.tv lists for their server: with no other address, it
// answers 403 private_network, keeping the sign-in. An administrator's own
// import reads the server there.
func TestAMembersPlexImportAsksNoLocalAddress(t *testing.T) {
	t.Parallel()
	plex := newFakePlexAccount(t)
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		lib := library.New(deps.pool, deps.addons, deps.client, logger, o.Accounts.Settings)
		// No client is given for members' requests: they are confined as
		// a running server's are.
		o.JellyfinImport = jellyfinimport.New(jellyfinimport.Options{Settings: o.Accounts.Settings, Titles: lib,
			UserData: userdata.New(deps.pool), Version: "1.2.3", Logger: logger,
			PlexTV: jellyfinimport.PlexTV{API: plex.tv, SignIn: plex.tv + "/auth", Discover: plex.tv}})
		t.Cleanup(o.JellyfinImport.Close)
	})
	admin, member := api.signedIn("root", true), api.signedIn("member", false)
	close(plex.claimed)
	signIn := func(b browser, who string) {
		t.Helper()
		if status, body, _ := b.call(http.MethodPost, "/account/server-import/plex/sign-in", nil); status != http.StatusOK {
			t.Fatalf("%s's sign-in: %d %v", who, status, body)
		}
		status, body, _ := b.call(http.MethodPost, "/account/server-import/plex/sign-in/poll", nil)
		if servers, _ := body["servers"].([]any); status != http.StatusOK || len(servers) != 1 {
			t.Fatalf("%s signed in: %d %v", who, status, body)
		}
	}
	fromPlex := map[string]any{"kind": "plex", "server": "0a1b2c"}

	signIn(member, "the member")
	if status, body, _ := member.call(http.MethodPost, "/account/server-import", fromPlex); status != http.StatusForbidden ||
		body["error"] != "private_network" {
		t.Errorf("the member's import: %d %v", status, body)
	}
	if reached := plex.reached(); len(reached) != 0 {
		t.Fatalf("the member's import sent the local server %v", reached)
	}
	if status, body, _ := member.call(http.MethodPost, "/account/server-import/plex/sign-in/poll", nil); status != http.StatusOK {
		t.Errorf("the sign-in is kept: %d %v", status, body)
	}

	signIn(admin, "the administrator")
	if status, body, _ := admin.call(http.MethodPost, "/account/server-import", fromPlex); status != http.StatusOK {
		t.Fatalf("the administrator's import: %d %v", status, body)
	}
	ownImportDone(t, admin)
	if reached := plex.reached(); !slices.Contains(reached, "GET /identity") || !slices.Contains(reached, "GET /library/sections") {
		t.Errorf("the administrator's import sent the local server %v", reached)
	}
}
