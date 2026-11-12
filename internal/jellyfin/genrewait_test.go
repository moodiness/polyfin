package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A genre's page whose catalog is slow to answer opens as jellyfin-web
// opens it: its item at once, with counts that tell there are titles, then
// its listing within the page's wait, with the titles Polyfin keeps of the
// genre, more to follow; once the catalog answered, the listing has its
// titles first, and the true total.
func TestAGenrePageOpensWhileItsCatalogIsSlow(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	const wait = 300 * time.Millisecond
	s.library.SetNarrowedWait(wait)
	s.user("member", nil)
	gate := make(chan struct{})
	late := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.EscapedPath(), ".json")
		switch {
		case path == "/manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "late", Name: "Late", Version: "1", Types: []string{"movie"},
				Resources: []stremio.Resource{{Name: "catalog"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "late", Name: "Late",
					Extra: []stremio.Extra{{Name: "genre", IsRequired: true, Options: []string{"Mystery"}}}}}})
		case strings.HasPrefix(path, "/catalog/"):
			select {
			case <-gate:
			case <-r.Context().Done():
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{{ID: "tt5001", Type: "movie", Name: "Late Mystery",
				Genres: stremio.Names{"Mystery"}, ReleaseInfo: "2020", Extras: &stremio.Extras{Certification: "PG"}}}})
		default:
			http.NotFound(w, r)
		}
	}))
	opened := false
	open := func() {
		if !opened {
			opened = true
			close(gate)
		}
	}
	t.Cleanup(func() {
		open()
		late.Close()
	})
	var choices []addons.LibraryChoice
	for _, manifest := range []string{shelfAddon(t), late.URL + "/manifest.json"} {
		addon, err := s.addons.Install(t.Context(), addons.Shared(), manifest, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, catalog := range addon.Manifest.Catalogs {
			choices = append(choices, addons.LibraryChoice{AddonID: addon.ID, CatalogType: catalog.Type, CatalogID: catalog.ID})
		}
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), choices); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	// The titles kept, as the libraries listed them.
	var movies QueryResult
	for _, view := range views.Items {
		if view.Name == "Late" {
			continue
		}
		var titles QueryResult
		s.get(t, "/Items?parentId="+view.Id, token, &titles)
		if view.Name == "Shelf" {
			movies = titles
		}
	}
	var movie BaseItemDto
	s.get(t, "/Items/"+movies.Items[0].Id, token, &movie)
	if movie.GenreItems == nil || len(*movie.GenreItems) != 1 {
		t.Fatalf("genres of %s: %+v", movie.Name, movie.GenreItems)
	}
	linked := (*movie.GenreItems)[0].Id

	started := time.Now()
	genre := namedPageOf(t, s, token, "/Items/"+linked)
	if took := time.Since(started); took >= time.Second || genre.Name != "Mystery" || genre.MovieCount == 0 || genre.SeriesCount == 0 {
		t.Errorf("the genre's item, in %v: %d movies, %d series", took, genre.MovieCount, genre.SeriesCount)
	}
	started = time.Now()
	names, total := listed(t, s, token, titlesOf+"genreIds="+linked)
	if took := time.Since(started); took < wait || took > wait+time.Second ||
		!slices.Equal(names, []string{"Ash Road", "Cold Night", "Harbor Tales"}) || total != 4 {
		t.Errorf("its titles, in %v: %v of %d", took, names, total)
	}

	open()
	want := []string{"Ash Road", "Cold Night", "Harbor Tales", "Late Mystery"}
	deadline := time.Now().Add(10 * time.Second)
	for {
		names, total := listed(t, s, token, titlesOf+"genreIds="+linked)
		if slices.Equal(names, want) && total == len(want) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("its titles, once read: %v of %d", names, total)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if genre := namedPageOf(t, s, token, "/Items/"+linked); genre.MovieCount != 3 || genre.SeriesCount != 1 {
		t.Errorf("the genre's item, once read: %d movies, %d series", genre.MovieCount, genre.SeriesCount)
	}
}
