package trackers

import (
	"net/http"
	"strings"
	"testing"
)

// sentTo counts the requests service received.
func (f *fakes) sentTo(service string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.requests {
		if r.service == service {
			n++
		}
	}
	return n
}

// Deleting a user revokes, in the background, their Trakt and Simkl
// tokens, as disconnecting does; the keys they typed and Last.fm's session
// key are only forgotten, and other users' connections stay.
func TestDeletingAUserRevokesTheirTokens(t *testing.T) {
	h := newHarness(t)
	h.store.OnDelete(h.Leaving)
	gina, hugo := h.user(t, "gina"), h.user(t, "hugo")
	h.connected(t, gina, Trakt, "trakt-gina")
	h.connected(t, gina, Simkl, "simkl-gina")
	h.connected(t, gina, MDBList, "mdblist-gina")
	h.connected(t, gina, LastFM, "lastfm-gina")
	h.connected(t, hugo, Trakt, "trakt-hugo")

	if err := h.store.DeleteUser(t.Context(), gina); err != nil {
		t.Fatal(err)
	}
	revoke := h.f.wait(t, 1, "trakt", "/oauth/revoke")[0]
	sameJSON(t, "Trakt revocation", revoke.body, `{"token":"trakt-gina","client_id":"trakt-client","client_secret":"trakt-secret"}`)
	if revoke := h.f.wait(t, 1, "simkl", "/oauth2/revoke")[0]; revoke.form.Get("token") != "simkl-gina-refresh" {
		t.Errorf("Simkl revocation: %v", revoke.form)
	}
	h.Close()
	if n := len(h.f.sent("trakt", "/oauth/revoke")); n != 1 {
		t.Errorf("%d Trakt revocations", n)
	}
	for _, service := range []string{"mdblist", "lastfm"} {
		if n := h.f.sentTo(service); n != 0 {
			t.Errorf("%s was sent %d requests", service, n)
		}
	}
	if !h.status(t, hugo, Trakt).Connected {
		t.Error("Hugo's connection went")
	}
}

// A revocation that never answers or fails leaves the deletion done at
// once; the failure is logged without the token.
func TestDeletingAUserNeverWaitsForTheService(t *testing.T) {
	h := newHarness(t)
	h.store.OnDelete(h.Leaving)
	ivan, jade := h.user(t, "ivan"), h.user(t, "jade")
	h.connected(t, ivan, Trakt, "trakt-ivan")
	h.connected(t, jade, Simkl, "simkl-jade")
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	h.f.on("POST", "/trakt/oauth/revoke", func(request) answer {
		<-release
		return answer{status: http.StatusOK, body: "{}"}
	})
	h.f.reply("POST", "/simkl/oauth2/revoke", http.StatusInternalServerError, `{}`)

	if err := h.store.DeleteUser(t.Context(), ivan); err != nil {
		t.Fatal(err)
	}
	if err := h.store.DeleteUser(t.Context(), jade); err != nil {
		t.Fatal(err)
	}
	h.f.wait(t, 1, "trakt", "/oauth/revoke")
	h.f.wait(t, 1, "simkl", "/oauth2/revoke")
	eventually(t, "the failure logged", func() bool { return strings.Contains(h.log.String(), "could not be revoked") })
	if log := h.log.String(); strings.Contains(log, "simkl-jade") || strings.Contains(log, "trakt-ivan") {
		t.Errorf("the log holds a token: %s", log)
	}
}
