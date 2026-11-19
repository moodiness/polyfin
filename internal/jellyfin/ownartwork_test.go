package jellyfin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// The artwork of a title only a user's own addon holds, and of its
// episodes, is served with the tag its listings give, and without it to
// the owner's sign-in only: an item's identifier can be computed from the
// identifier its addon gives it, and image requests come without a user.
// The artwork of the server's titles is served to anyone, as before.
func TestTheArtworkOfAUsersOwnTitleNeedsItsTagOrItsOwner(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	png := picture(t, 20, 10, 255)
	art := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/art/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	t.Cleanup(art.Close)
	add := func(scope addons.Scope, manifest string) {
		t.Helper()
		installed, err := s.addons.Install(t.Context(), scope, manifest, false)
		if err != nil {
			t.Fatal(err)
		}
		choices := []addons.LibraryChoice{{AddonID: installed.ID, CatalogType: "series", CatalogID: "shows"}}
		if _, err := s.addons.SetLibraries(t.Context(), scope, choices); err != nil {
			t.Fatal(err)
		}
	}
	add(addons.Shared(), manySeriesAddon(t, "shared.shows", "Server Shows", []string{"tt"},
		stremio.Meta{ID: "tt7000", Type: "series", Name: "The Server's Show", Poster: art.URL + "/art/server.png"}))
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	s.user("other", nil)
	other := s.signIn("other", "tv")
	// first lists the series of a library, as the user of token, and
	// returns the first with its episodes.
	first := func(token, view string) (BaseItemDto, []BaseItemDto) {
		t.Helper()
		var page, episodes QueryResult
		if status := s.get(t, "/Items?ParentId="+s.viewIDs(t, token)[view], token, &page); status != http.StatusOK || len(page.Items) == 0 {
			t.Fatalf("the library %s: %d %v", view, status, itemNames(page.Items))
		}
		if status := s.get(t, "/Shows/"+page.Items[0].Id+"/Episodes", token, &episodes); status != http.StatusOK {
			t.Fatalf("the episodes of %s: %d", page.Items[0].Name, status)
		}
		return page.Items[0], episodes.Items
	}
	shared, _ := first(other, "Server Shows")

	owner := s.user("owner", func(c *accounts.UserChanges) { c.IsAdministrator, c.UseSharedAddons = new(true), new(false) })
	add(addons.Personal(owner.ID), manySeriesAddon(t, "own.shows", "My Shows", []string{"tt"},
		stremio.Meta{ID: "tt9000", Type: "series", Name: "My Show", Poster: art.URL + "/art/own.png", Videos: []stremio.Video{
			{ID: "tt9000:1:1", Title: "Mine", Thumbnail: art.URL + "/art/mine.png", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00.000Z"}}}))
	series, episodes := first(s.signIn("owner", "phone"), "My Shows")
	if series.Id != library.TitleItemID(library.KindSeries, "tt9000").String() || len(episodes) != 1 ||
		episodes[0].Id != library.EpisodeItemID("tt9000:1:1").String() {
		t.Fatalf("the owner's series: %+v %v", series, itemNames(episodes))
	}
	if _, err := s.store.UpdateUser(t.Context(), owner.ID, accounts.UserChanges{IsAdministrator: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	mine := s.signIn("owner", "tv")

	image := func(item BaseItemDto, token string, tagged bool) int {
		t.Helper()
		path := "/Items/" + item.Id + "/Images/Primary"
		if tagged {
			if item.ImageTags["Primary"] == "" {
				t.Fatalf("%s has no tag: %+v", item.Name, item.ImageTags)
			}
			path += "?tag=" + item.ImageTags["Primary"]
		}
		return s.get(t, path, token, nil)
	}
	for _, item := range []BaseItemDto{series, episodes[0]} {
		for _, tc := range []struct {
			who    string
			token  string
			tagged bool
			want   int
		}{
			{"no one", "", false, http.StatusNotFound},
			{"another user", other, false, http.StatusNotFound},
			{"no one, with its tag", "", true, http.StatusOK},
			{"another user, with its tag", other, true, http.StatusOK},
			{"the owner", mine, false, http.StatusOK},
			{"the owner, with its tag", mine, true, http.StatusOK},
		} {
			if got := image(item, tc.token, tc.tagged); got != tc.want {
				t.Errorf("%s's image for %s: %d, want %d", item.Name, tc.who, got, tc.want)
			}
		}
	}
	if got := image(shared, "", false); got != http.StatusOK {
		t.Errorf("the server's series' image for no one: %d", got)
	}
}
