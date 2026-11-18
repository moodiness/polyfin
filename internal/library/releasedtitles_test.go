package library

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A member's own addon reaches public addresses only, and no test server
// is on one: as in TestMergedTitlesKeepTheCatalogThatListedThem, what such
// an addon answers is put in the caches the service reads, as an addon on
// a public host would have answered.

// localImages is an image server on the loopback, standing for a host of
// the server's local network, counting the requests it gets.
func localImages(t *testing.T) (string, *atomic.Int32) {
	t.Helper()
	hits := new(atomic.Int32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("\x89PNG\r\n\x1a\n"))
	}))
	t.Cleanup(server.Close)
	return server.URL, hits
}

// ownAddon installs, for the member, their own addon of manifest.
func (e env) ownAddon(manifest stremio.Manifest) accounts.ID {
	e.t.Helper()
	e.install(addons.Personal(e.member.ID), &fakeAddon{manifest: manifest})
	return e.installedID(addons.Personal(e.member.ID), manifest.ID)
}

// refused reports whether art is confined to public addresses and its
// download, made as the artwork relay makes it, is refused.
func (e env) refused(art Artwork) bool {
	e.t.Helper()
	_, _, err := e.service.client.Image(e.t.Context(), art.URL, art.URLConfined())
	return art.URLConfined() && errors.Is(err, stremio.ErrPrivateNetwork)
}

// A title a member's own addon listed is released when the addon goes,
// with the description that addon gave, artwork addresses included: it
// stays confined to public addresses, however it was released, so that
// its artwork on a local address is still never downloaded. The next
// catalog that lists the title replaces its description and its
// confinement together.
func TestReleasedTitlesStayConfined(t *testing.T) {
	t.Parallel()
	manifest := stremio.Manifest{ID: "own", Name: "Own", Version: "1", Resources: []stremio.Resource{{Name: "catalog"}},
		Catalogs: []stremio.Catalog{{Type: "movie", ID: "mine", Name: "Mine"}}}
	for name, release := range map[string]func(e env, own accounts.ID){
		"its member removes the addon": func(e env, own accounts.ID) {
			if _, err := e.addons.Remove(e.t.Context(), addons.Personal(e.member.ID), own); err != nil {
				e.t.Fatal(err)
			}
		},
		"its member is deleted": func(e env, _ accounts.ID) {
			if err := e.users.DeleteUser(e.t.Context(), e.member.ID); err != nil {
				e.t.Fatal(err)
			}
			if cleaned, err := e.service.CleanUpRemoved(e.t.Context()); err != nil || cleaned.Released != 1 {
				e.t.Fatalf("the cleanup: %+v %v, want 1 title released", cleaned, err)
			}
		},
		"the addon drops its catalog": func(e env, own accounts.ID) {
			dropped := manifest
			dropped.Catalogs = []stremio.Catalog{{Type: "movie", ID: "other", Name: "Other"}}
			server := httptest.NewServer(&fakeAddon{manifest: dropped})
			e.t.Cleanup(server.Close)
			if _, _, err := e.addons.Replace(e.t.Context(), addons.Personal(e.member.ID), own, server.URL+"/manifest.json", false); err != nil {
				e.t.Fatal(err)
			}
			if cleaned, err := e.service.CleanUpRemoved(e.t.Context()); err != nil || cleaned.Released != 1 {
				e.t.Fatalf("the cleanup: %+v %v, want 1 title released", cleaned, err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t)
			ctx := t.Context()
			// As the server wires it.
			e.addons.SetCleanup(e.service.CleanRemoved)
			local, hits := localImages(t)
			own := e.ownAddon(manifest)
			mine := stremio.Meta{ID: "own:1", Type: "movie", Name: "Mine 1", Poster: local + "/poster.png", Background: local + "/backdrop.png"}
			e.service.pages.Put(pageKey{addon: own, catalogType: "movie", catalogID: "mine"}, fetched[[]stremio.Meta]{[]stremio.Meta{mine}, time.Now()})
			listed := e.listed(e.member, "Mine")
			if len(listed) != 1 {
				t.Fatalf("the member's library lists %v", listed)
			}
			id := listed[0]
			if r, err := e.service.load(ctx, id); err != nil || !r.Confined || r.Addon == nil || *r.Addon != own {
				t.Fatalf("the member's title before its release: %+v %v", r, err)
			}

			release(e, own)

			r, err := e.service.load(ctx, id)
			if err != nil || r.Addon != nil || r.Meta == nil || r.Meta.Poster != mine.Poster {
				t.Fatalf("the title was not released with its description: %+v %v", r, err)
			}
			if !r.Confined {
				t.Error("the released title is no longer confined")
			}
			for imageType, want := range map[string]string{"Primary": mine.Poster, "Backdrop": mine.Background} {
				art, err := e.service.Artwork(ctx, id, imageType)
				if err != nil || art.URL != want {
					t.Fatalf("%s of the released title: %+v %v", imageType, art, err)
				}
				if !e.refused(art) {
					t.Errorf("%s of the released title is downloaded from a local address", imageType)
				}
			}
			if tagged, ok := e.service.TaggedArtwork(ctx, id, ImageTag(mine.Poster)); !ok || tagged.URL != mine.Poster || !e.refused(tagged) {
				t.Errorf("the released title's poster asked by its tag: %+v %t, want it confined", tagged, ok)
			}
			if n := hits.Load(); n != 0 {
				t.Errorf("the local server got %d requests", n)
			}

			// One of the server's catalogs takes the title: its description
			// and its reach are that addon's.
			taken := mine
			taken.Poster, taken.Background = "https://images.example/taken.jpg", ""
			e.install(addons.Shared(), &fakeAddon{
				manifest: stremio.Manifest{ID: "shared", Name: "Shared", Version: "1", Resources: []stremio.Resource{{Name: "catalog"}},
					Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}},
				catalogs: map[string][]stremio.Meta{"movie/top": {taken}},
			})
			if listed := e.listed(e.admin, "Top"); len(listed) != 1 || listed[0] != id {
				t.Fatalf("the server's library lists %v, want %v", listed, id)
			}
			art, err := e.service.Artwork(ctx, id, "Primary")
			if err != nil || art.URL != taken.Poster || art.URLConfined() {
				t.Errorf("the poster once a server catalog took the title: %+v %v", art, err)
			}
		})
	}
}
