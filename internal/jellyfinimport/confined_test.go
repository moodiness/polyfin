package jellyfinimport

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/appimport/nuvio/nuviotest"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// publicFront serves, until the test ends, a server standing for one on a
// public address, in front of the server at target, on the loopback. The
// first segment of a request's path names what it does with the rest:
// "pass" hands every request on to target, as a reverse proxy does;
// "probe-302" and "probe-307" answer every request with that redirect to
// the same path and query at local; "sign-in-302" and "sign-in-307"
// redirect the sign-in alone, and "reads-307" everything a user signed in
// asks, the sign-out included, handing the rest on to target.
func publicFront(t *testing.T, target, local string) string {
	t.Helper()
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	proxy := &httputil.ReverseProxy{Rewrite: func(r *httputil.ProxyRequest) { r.SetURL(parsed) }}
	const signIn, info = "/Users/AuthenticateByName", "/System/Info/Public"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, rest, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
		r.URL.Path = "/" + rest
		redirect := 0
		switch {
		case name == "probe-302", name == "sign-in-302" && r.URL.Path == signIn:
			redirect = http.StatusFound
		case name == "probe-307", name == "sign-in-307" && r.URL.Path == signIn,
			name == "reads-307" && r.URL.Path != signIn && r.URL.Path != info:
			redirect = http.StatusTemporaryRedirect
		}
		if redirect != 0 {
			http.Redirect(w, r, local+r.URL.RequestURI(), redirect)
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// administrator creates an administrator named name.
func (h harness) administrator(t *testing.T, name string) accounts.User {
	t.Helper()
	user, err := h.store.CreateUser(t.Context(), accounts.NewUser{Name: name, Password: "correct horse", IsAdministrator: true})
	if err != nil {
		t.Fatal(err)
	}
	return user
}

// bobs gives each fake the user Bob, his password and the movie he played.
func bobs(fakes ...*fakeJellyfin) {
	for _, f := range fakes {
		f.users = []map[string]any{{"Id": "jf-bob", "Name": "Bob"}}
		f.passwords = map[string]string{"Bob": "bob's password"}
		f.items["jf-bob"] = map[string][]map[string]any{
			"IsPlayed": {movie("m2", "Bob's", map[string]string{"Imdb": "tt0000002"}, map[string]any{"Played": true})}}
	}
}

// asBob is Bob's sign-in to the server of kind at address.
func asBob(kind Kind, address string) Connection {
	return Connection{Kind: kind, Address: address, Credentials: Credentials{Name: "Bob", Password: "bob's password"}}
}

// A user who is no administrator imports their own watch data from a
// server on a public address only: one on the local network, or one that
// redirects there, at its first answer or at the sign-in, is refused
// before anything is sent there, and starts nothing. A server on a public
// address is read as before. An administrator's own import reaches the
// local network, redirects included.
func TestAUsersOwnImportReachesPublicAddressesOnly(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	local := newFakeJellyfin(t)
	bobs(h.f, local)
	front := publicFront(t, h.f.url, local.url)
	h.confined = nuviotest.Confined(t, front)
	bob, root := h.user(t, "bob"), h.administrator(t, "root")

	for _, connection := range []Connection{
		asBob(Jellyfin, local.url), asBob(Emby, local.url),
		asBob(Jellyfin, front+"/probe-302"), asBob(Emby, front+"/probe-302"),
		asBob(Jellyfin, front+"/probe-307"), asBob(Emby, front+"/probe-307"),
		asBob(Jellyfin, front+"/sign-in-302"), asBob(Jellyfin, front+"/sign-in-307"),
	} {
		if _, err := h.ImportOwn(t.Context(), connection, bob); !errors.Is(err, stremio.ErrPrivateNetwork) {
			t.Errorf("Bob's %s server at %s: %v", connection.Kind, connection.Address, err)
		}
		if sent := local.sent(); len(sent) != 0 {
			t.Fatalf("Bob's %s server at %s: the local server was sent %s %s", connection.Kind, connection.Address, sent[0].Method, sent[0].URL)
		}
	}
	if current := h.Current(); current != nil {
		t.Errorf("a refused address started an import: %+v", current)
	}

	if _, err := h.ImportOwn(t.Context(), asBob(Jellyfin, front+"/pass"), bob); err != nil {
		t.Fatalf("Bob's server on a public address: %v", err)
	}
	if status := h.ended(t); status.State != StateDone || len(status.Users) != 1 || status.Users[0].Played != 1 {
		t.Errorf("Bob's import from a public address: %+v", status)
	}
	if !h.dataOf(t, bob.ID, library.TitleRef{IMDb: "tt0000002"}).Played {
		t.Error("Bob's movie is not played")
	}
	if sent := local.sent(); len(sent) != 0 {
		t.Errorf("the local server was sent %s %s", sent[0].Method, sent[0].URL)
	}

	for _, address := range []string{local.url, front + "/probe-307", front + "/sign-in-307"} {
		before := len(local.sent())
		if _, err := h.ImportOwn(t.Context(), asBob(Jellyfin, address), root); err != nil {
			t.Fatalf("the administrator's server at %s: %v", address, err)
		}
		h.ended(t)
		signedIn := false
		for _, r := range local.sent()[before:] {
			signedIn = signedIn || r.Method == http.MethodPost && r.URL.Path == "/Users/AuthenticateByName"
		}
		if !signedIn {
			t.Errorf("the administrator's server at %s: the local server was not signed in to", address)
		}
	}
	if !h.dataOf(t, root.ID, library.TitleRef{IMDb: "tt0000002"}).Played {
		t.Error("the administrator's movie is not played")
	}
}

// A user's own import stays on public addresses for as long as it runs: a
// server that, once the user is signed in, sends what the import reads
// and its sign-out to the local network is not followed there, and the
// import ends telling the server did not answer.
func TestAUsersOwnImportStaysOnPublicAddressesWhileItRuns(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	local := newFakeJellyfin(t)
	bobs(h.f, local)
	front := publicFront(t, h.f.url, local.url)
	h.confined = nuviotest.Confined(t, front)
	bob := h.user(t, "bob")

	if _, err := h.ImportOwn(t.Context(), asBob(Jellyfin, front+"/reads-307"), bob); err != nil {
		t.Fatal(err)
	}
	status := h.ended(t)
	if len(status.Users) != 1 || status.Users[0].Problem != ProblemUnreachable || status.Users[0].Played != 0 {
		t.Errorf("the import: %+v", status)
	}
	// The sign-out is sent once the import ended: Close waits for it.
	h.Close()
	if sent := local.sent(); len(sent) != 0 {
		t.Errorf("the local server was sent %d requests, the first %s %s", len(sent), sent[0].Method, sent[0].URL)
	}
	if h.dataOf(t, bob.ID, library.TitleRef{IMDb: "tt0000002"}).Played {
		t.Error("Bob was given what the local server holds")
	}
}

// sent lists the requests the fake received.
func (f *fakePlex) sent() []*http.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*http.Request(nil), f.requests...)
}

// A user who is no administrator and signed in to Plex has their server
// asked at its public addresses only: those plex.tv lists on the local
// network, or that redirect there, are not asked, and the import reads at
// the public one that answers. A server with no other address is refused,
// keeping the sign-in. An administrator's own import asks the local
// addresses first, as before.
func TestAUsersPlexImportAsksPublicAddressesOnly(t *testing.T) {
	t.Parallel()
	tv := newFakePlexTV(t)
	h, clock, _ := plexSigningIn(t, tv)
	p, local := smallPlex(t), smallPlex(t)
	played := map[string]map[string]any{"11": {"viewCount": 1, "lastViewedAt": at("2026-09-01T20:00:00Z").Unix()}}
	p.states["friend-server-token"], local.states["friend-server-token"] = played, played
	front := publicFront(t, p.url, local.url)
	h.confined = nuviotest.Confined(t, front)
	friend, root := h.user(t, "friend"), h.administrator(t, "root")
	tv.users["account-token"] = map[string]any{"uuid": "u-friend", "title": "Friend"}
	atHome := plexServer("friend-server-token", map[string]any{"uri": local.url, "local": true},
		map[string]any{"uri": front + "/probe-302", "local": false})
	atHome["name"], atHome["clientIdentifier"] = "At home", "ffffff"
	tv.resources["account-token"] = []map[string]any{atHome, plexServer("friend-server-token",
		map[string]any{"uri": local.url, "local": true}, map[string]any{"uri": front + "/probe-307", "local": false},
		map[string]any{"uri": front + "/pass", "relay": true})}

	h.signedIn(t, tv, clock, friend)
	if _, err := h.ImportOwnPlex(t.Context(), friend, "ffffff", Extras{}); !errors.Is(err, stremio.ErrPrivateNetwork) {
		t.Errorf("a server on the local network only: %v", err)
	}
	if account, err := h.PlexAccount(t.Context(), friend.ID); err != nil || len(account.Servers) != 2 {
		t.Errorf("the sign-in is kept: %+v %v", account, err)
	}
	snapshot, err := h.ImportOwnPlex(t.Context(), friend, "0a1b2c", Extras{})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Address != front+"/pass" {
		t.Errorf("the friend's import reads at %s", snapshot.Address)
	}
	if status := h.ended(t); status.State != StateDone || len(status.Users) != 1 || status.Users[0].Played != 1 {
		t.Errorf("the friend's import: %+v", status)
	}
	if sent := local.sent(); len(sent) != 0 {
		t.Fatalf("the local server was sent %d requests, the first %s %s", len(sent), sent[0].Method, sent[0].URL)
	}

	// Plex signs the administrator in with a PIN of their own.
	tv.claim("")
	h.signedIn(t, tv, clock, root)
	snapshot, err = h.ImportOwnPlex(t.Context(), root, "0a1b2c", Extras{})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Address != local.url {
		t.Errorf("the administrator's import reads at %s", snapshot.Address)
	}
	if status := h.ended(t); status.State != StateDone || len(status.Users) != 1 || status.Users[0].Played != 1 {
		t.Errorf("the administrator's import: %+v", status)
	}
	if sent := local.sent(); len(sent) == 0 {
		t.Error("the administrator's import did not read the local server")
	}
}
