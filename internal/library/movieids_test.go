package library

import (
	"fmt"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// shelvesAddon is shaped as the addons whose movie catalogs all ask for a
// genre, among options, and can be paged, and whose collections group
// those catalogs: titles with IMDb identifiers, without ratings.
func shelvesAddon() *fakeAddon {
	movies := func(prefix string, count int, genre string) []stremio.Meta {
		result := make([]stremio.Meta, count)
		for i := range result {
			result[i] = stremio.Meta{ID: fmt.Sprintf("tt%s%04d", prefix, i), Type: "movie", Name: fmt.Sprintf("%s %d", prefix, i), Genres: []string{genre}}
		}
		return result
	}
	byGenre := func(options ...string) []stremio.Extra {
		return []stremio.Extra{{Name: "genre", IsRequired: true, Options: options}, {Name: "skip"}}
	}
	return &fakeAddon{
		manifest: stremio.Manifest{ID: "shelves", Name: "Shelves", Version: "1", Types: []string{"movie", "series", "collection"}, IDPrefixes: []string{"tt"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs: []stremio.Catalog{
				{Type: "movie", ID: "popular", Name: "Popular", Extra: byGenre("Action", "Drama")},
				{Type: "movie", ID: "saga", Name: "Saga", Extra: byGenre("Saga")},
				{Type: "series", ID: "shows", Name: "Shows", Extra: byGenre("Drama")},
				{Type: "collection", ID: "lists", Name: "Lists", Extra: byGenre("Lists")},
				{Type: "movie", ID: "search", Name: "Search", Extra: []stremio.Extra{{Name: "search", IsRequired: true}}},
			}},
		catalogs: map[string][]stremio.Meta{
			"movie/popular":    append(movies("1", 30, "Action"), movies("2", 10, "Drama")...),
			"movie/saga":       movies("3", 5, "Saga"),
			"series/shows":     {{ID: "tt9000001", Type: "series", Name: "Show", Genres: []string{"Drama"}}},
			"collection/lists": {{ID: "list:action", Type: "collection", Name: "Action", Genres: []string{"Lists"}}},
		},
		metas: map[string]stremio.Meta{
			"collection/list:action": {ID: "list:action", Type: "collection", Name: "Action", Collection: &stremio.Collection{
				Sources: []stremio.CollectionSource{{Type: "movie", CatalogID: "popular", Genre: "Action"}, {Type: "movie", CatalogID: "saga"},
					{Type: "series", CatalogID: "shows"}}}},
		},
		pageSize: 10,
	}
}

// Box sets learn the movies of the libraries of movies only: a server
// whose libraries are all of collections has none, and its addons are
// not asked for any, though its collections hold movies.
func TestMovieIDsReadLibrariesOfMoviesOnly(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	addon := shelvesAddon()
	e.install(addons.Shared(), addon)
	e.chooseLibraries(libraryOf("collection", "lists", "", 0))
	ids, libraries, read, err := e.service.MovieIDs(t.Context(), 2000)
	if err != nil || len(ids) != 0 || libraries != 0 || read != 0 {
		t.Fatalf("collections only: %d movies of %d libraries, %d read, %v", len(ids), libraries, read, err)
	}
	if requests := addon.catalogRequests(); len(requests) != 0 {
		t.Errorf("the addon was asked %v", requests)
	}
}

// The movies of a library of movies whose catalog asks for a genre are
// those of its first genre, read page after page, up to the bound, each
// with its IMDb identifier.
func TestMovieIDsReadCatalogsAskingForAGenre(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.install(addons.Shared(), shelvesAddon())
	e.chooseLibraries(libraryOf("collection", "lists", "", 0), libraryOf("movie", "popular", "", 0), libraryOf("movie", "saga", "", 0))
	ids, libraries, read, err := e.service.MovieIDs(t.Context(), 25)
	if err != nil || libraries != 2 || read != 2 || len(ids) != 30 {
		t.Fatalf("%d movies of %d libraries, %d read, %v", len(ids), libraries, read, err)
	}
	for i, id := range ids {
		want := fmt.Sprintf("tt1%04d", i)
		if i >= 25 {
			want = fmt.Sprintf("tt3%04d", i-25)
		}
		if id["Imdb"] != want {
			t.Errorf("movie %d: %v, want %s", i, id, want)
		}
	}
}
