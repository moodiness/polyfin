package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// genreMoviesAddon serves a movie catalog "picks" of the movies given.
func genreMoviesAddon(t *testing.T, id, name string, movies ...stremio.Meta) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: id, Name: name, Version: "1", Types: []string{"movie"},
				Resources: []stremio.Resource{{Name: "catalog"}}, Catalogs: []stremio.Catalog{{Type: "movie", ID: "picks", Name: name}}})
		case strings.HasPrefix(path, "/catalog/movie/picks"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": movies})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// A genre opens by its identifier for a user whose titles have it: a
// title only another user's own addon holds gives no one else the name
// of its genres, nor how one is spelled, while its owner was shown them,
// nor once they are read again from the titles kept, after a restart.
func TestAnotherUsersOwnTitlesNameNoGenre(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	movie := func(id, name string, genres ...string) stremio.Meta {
		return stremio.Meta{ID: id, Type: "movie", Name: name, Genres: genres, ReleaseInfo: "2020", Released: "2020-04-10T00:00:00.000Z"}
	}
	if _, err := s.addons.Install(t.Context(), addons.Shared(), genreMoviesAddon(t, "shared.picks", "Server Picks",
		movie("tt3001", "Ash Road", "Drama")), false); err != nil {
		t.Fatal(err)
	}
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	other := s.user("other", nil)
	theirs := s.signIn("other", "tv")
	var listed QueryResult
	if status := s.get(t, "/Items?ParentId="+s.viewIDs(t, theirs)["Server Picks"], theirs, &listed); status != http.StatusOK || len(listed.Items) != 1 {
		t.Fatalf("the server's titles: %d %v", status, itemNames(listed.Items))
	}
	owner := s.user("owner", func(c *accounts.UserChanges) { c.IsAdministrator, c.UseSharedAddons = new(true), new(false) })
	installed, err := s.addons.Install(t.Context(), addons.Personal(owner.ID), genreMoviesAddon(t, "own.picks", "My Picks",
		movie("tt9001", "Kept Apart", "Hidden Genre", "DRAMA"), movie("tt9002", "Kept Too", "DRAMA")), false)
	if err != nil {
		t.Fatal(err)
	}
	choices := []addons.LibraryChoice{{AddonID: installed.ID, CatalogType: "movie", CatalogID: "picks"}}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Personal(owner.ID), choices); err != nil {
		t.Fatal(err)
	}
	// The owner opens their own titles, which name their genres.
	mine := s.signIn("owner", "phone")
	var page QueryResult
	if status := s.get(t, "/Items?ParentId="+s.viewIDs(t, mine)["My Picks"], mine, &page); status != http.StatusOK || len(page.Items) != 2 {
		t.Fatalf("the owner's titles: %d %v", status, itemNames(page.Items))
	}
	for _, item := range page.Items {
		var title BaseItemDto
		if status := s.get(t, "/Users/"+owner.ID.String()+"/Items/"+item.Id, mine, &title); status != http.StatusOK || title.GenreItems == nil {
			t.Fatalf("%s: %d %+v", item.Name, status, title.GenreItems)
		}
	}
	if _, err := s.store.UpdateUser(t.Context(), owner.ID, accounts.UserChanges{IsAdministrator: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	mine = s.signIn("owner", "tv")

	// genre answers the status of the genre named so, as the user of
	// token, and the name it opens under.
	genre := func(user accounts.User, token, name string) (int, string) {
		t.Helper()
		status, body := s.call(http.MethodGet, "/Users/"+user.ID.String()+"/Items/"+nameID("genre", name), app("tv", token), nil)
		var page namedItemDto
		if status == http.StatusOK {
			if err := json.Unmarshal(body, &page); err != nil {
				t.Fatal(err)
			}
		}
		return status, page.Name
	}
	for _, when := range []string{"named to the owner", "after a restart"} {
		if status, name := genre(other, theirs, "Hidden Genre"); status != http.StatusNotFound {
			t.Errorf("%s, another user opens the genre of the owner's title: %d %q", when, status, name)
		}
		if status, name := genre(other, theirs, "drama"); status != http.StatusOK || name != "Drama" {
			t.Errorf("%s, another user's Drama: %d %q", when, status, name)
		}
		if status, name := genre(owner, mine, "hidden genre"); status != http.StatusOK || name != "Hidden Genre" {
			t.Errorf("%s, the owner's genre: %d %q", when, status, name)
		}
		s.handler.names.forget()
	}
}
