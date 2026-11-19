package jellyfin

import (
	"bytes"
	"errors"
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// pngAnswer answers a PNG.
func pngAnswer(png []byte) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}
}

// The artwork relay keeps what it downloads. An image of a local address,
// kept for a title that may reach it, is not answered for a member's
// confined title naming the same address, and the title that may reach
// it is still answered the image kept.
func TestTheArtworkKeptIsNotAnsweredPastTheConfinement(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	local := newCountingArt(t)
	png := pngOf(t, 60, 90)
	local.answer("/nas/cover.png", pngAnswer(png))
	admin := s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })

	// An administrator's own addon may reach the local network.
	s.withLibrary(t, addons.Personal(admin.ID), libraryAddon(t, "trusted", "movie",
		[]stremio.Meta{{ID: "trusted:1", Type: "movie", Name: "Trusted", Poster: local.URL + "/nas/cover.png"}}, nil), "movie")
	trusted := s.libraryItems(t, s.signIn("admin", "tv"), "trusted library")["Trusted"]
	// Apps ask by the tag the titles give, without credentials.
	tagged := "/Images/Primary?tag=" + library.ImageTag(local.URL+"/nas/cover.png")
	if response, body := s.fetch(t, "/Items/"+trusted+tagged); response.StatusCode != http.StatusOK || !bytes.Equal(body, png) ||
		local.requests("/nas/cover.png") != 1 {
		t.Fatalf("the administrator's poster: %d, %d bytes, asked %d times", response.StatusCode, len(body), local.requests("/nas/cover.png"))
	}

	// A member's confined title names the same address.
	owner := s.user("owner", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	s.withLibrary(t, addons.Personal(owner.ID), libraryAddon(t, "own", "movie",
		[]stremio.Meta{{ID: "own:2", Type: "movie", Name: "Mine", Poster: local.URL + "/nas/cover.png"}}, nil), "movie")
	s.libraryItems(t, s.signIn("owner", "tv"), "own library")
	s.becomesMember(t, owner)
	mine := s.libraryItems(t, s.signIn("owner", "tv"), "own library")["Mine"]
	if response, body := s.fetch(t, "/Items/"+mine+tagged); response.StatusCode != http.StatusForbidden {
		t.Errorf("the confined title's poster, kept for another title: %d, %d bytes, want 403", response.StatusCode, len(body))
	}
	if response, body := s.fetch(t, "/Items/"+trusted+tagged); response.StatusCode != http.StatusOK || !bytes.Equal(body, png) {
		t.Errorf("the administrator's poster after the confined request: %d, %d bytes", response.StatusCode, len(body))
	}
	if n := local.requests("/nas/cover.png"); n != 1 {
		t.Errorf("the poster was downloaded %d times, want once", n)
	}
}

// A failed download is remembered with the confinement it was met with: a
// confined request refused a local address does not fail the request
// that may reach it, and what that request met is not told to a confined
// one. Health tells either failure, and forgets both.
func TestArtworkFailuresAreRememberedByConfinement(t *testing.T) {
	t.Parallel()
	h := imageHandler(t.TempDir())
	local := newCountingArt(t)
	local.answer("/missing.jpg", status(http.StatusNotFound, ""))
	cover, missing := local.URL+"/cover.jpg", local.URL+"/missing.jpg"
	statusOf := func(err error) int {
		failed, _ := errors.AsType[downloadError](err)
		return failed.status
	}

	if _, err := h.fetchArtwork(t.Context(), cover, true, false); statusOf(err) != http.StatusForbidden {
		t.Fatalf("a confined download from a local address: %v, want 403", err)
	}
	if !h.ArtworkFailing(cover) {
		t.Error("the refused download is not told as failing")
	}
	if _, err := h.fetchArtwork(t.Context(), cover, false, false); err != nil || local.requests("/cover.jpg") != 1 {
		t.Errorf("the download that may reach the address, after a confined one was refused: %v, %d requests", err, local.requests("/cover.jpg"))
	}
	if _, err := h.fetchArtwork(t.Context(), cover, true, false); statusOf(err) != http.StatusForbidden {
		t.Errorf("a confined download of an image kept: %v, want 403", err)
	}

	if _, err := h.fetchArtwork(t.Context(), missing, false, false); statusOf(err) != http.StatusNotFound {
		t.Fatalf("an image its server does not have: %v, want 404", err)
	}
	if _, err := h.fetchArtwork(t.Context(), missing, true, false); statusOf(err) != http.StatusForbidden {
		t.Errorf("a confined download of an address found missing: %v, want 403", err)
	}
	if cover, missing := local.requests("/cover.jpg"), local.requests("/missing.jpg"); cover != 1 || missing != 1 {
		t.Errorf("the local server was asked the cover %d times and the missing image %d times, want once each", cover, missing)
	}

	h.ForgetArtwork([]string{cover, missing})
	if h.ArtworkFailing(cover) || h.ArtworkFailing(missing) {
		t.Error("failures are still told once forgotten")
	}
}
