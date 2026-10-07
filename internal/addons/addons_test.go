package addons

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/testdb"
)

// fakeAddon serves a manifest whose catalogs the test can change.
type fakeAddon struct {
	server   *httptest.Server
	catalogs atomic.Value
}

func newFakeAddon(t *testing.T, catalogs []stremio.Catalog) *fakeAddon {
	addon := &fakeAddon{}
	addon.catalogs.Store(catalogs)
	addon.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "test", Name: "Test", Version: "1.0.0",
			Resources: []stremio.Resource{{Name: "catalog"}}, Catalogs: addon.catalogs.Load().([]stremio.Catalog)})
	}))
	t.Cleanup(addon.server.Close)
	return addon
}

func (a *fakeAddon) url(path string) string { return a.server.URL + "/" + path + "/manifest.json" }

func catalogs(kind string, count int) []stremio.Catalog {
	result := make([]stremio.Catalog, count)
	for i := range result {
		result[i] = stremio.Catalog{Type: kind, ID: fmt.Sprintf("%s-%d", kind, i), Name: fmt.Sprintf("%s %d", kind, i)}
	}
	return result
}

func newStore(t *testing.T) (*Store, *accounts.Store) {
	t.Helper()
	pool := testdb.New(t)
	if err := database.Migrate(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	users, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	return New(pool, stremio.NewClient("test")), users
}

func enabled(t *testing.T, store *Store, scope Scope) []string {
	t.Helper()
	libraries, err := store.Libraries(t.Context(), scope)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, library := range libraries {
		if library.Enabled {
			ids = append(ids, library.Catalog.ID)
		}
	}
	return ids
}

func TestInstallEnablesBrowsableCatalogsUpToTheDefault(t *testing.T) {
	store, _ := newStore(t)
	search := stremio.Catalog{Type: "movie", ID: "search", Extra: []stremio.Extra{{Name: "search", IsRequired: true}}}
	other := stremio.Catalog{Type: "other", ID: "other"}
	addon := newFakeAddon(t, append([]stremio.Catalog{search, other}, append(catalogs("movie", 15), catalogs("series", 10)...)...))

	if _, err := store.Install(t.Context(), Shared(), addon.url("a"), false); err != nil {
		t.Fatal(err)
	}
	ids := enabled(t, store, Shared())
	if len(ids) != DefaultLibraries || ids[0] != "movie-0" || ids[15] != "series-0" {
		t.Fatalf("default libraries: %v", ids)
	}
	if _, err := store.Install(t.Context(), Shared(), addon.url("a"), false); !errors.Is(err, ErrExists) {
		t.Errorf("second install of the same URL: got %v", err)
	}
	// A second addon adds nothing once the scope has its default libraries.
	if _, err := store.Install(t.Context(), Shared(), addon.url("b"), false); err != nil {
		t.Fatal(err)
	}
	if got := enabled(t, store, Shared()); len(got) != DefaultLibraries {
		t.Errorf("libraries after a second addon: %d", len(got))
	}
}

func TestInstallPrefersCollectionCatalogs(t *testing.T) {
	store, _ := newStore(t)
	streaming := stremio.Catalog{Type: "collection", ID: "streaming", Extra: []stremio.Extra{{Name: "genre", IsRequired: true, Options: []string{"None"}}}}
	unbrowsable := stremio.Catalog{Type: "collection", ID: "search", Extra: []stremio.Extra{{Name: "search", IsRequired: true}}}
	addon := newFakeAddon(t, append([]stremio.Catalog{unbrowsable}, append(catalogs("movie", 3), streaming)...))
	if _, err := store.Install(t.Context(), Shared(), addon.url("a"), false); err != nil {
		t.Fatal(err)
	}
	if ids := enabled(t, store, Shared()); !slices.Equal(ids, []string{"streaming"}) {
		t.Errorf("default libraries of an addon with collections: %v", ids)
	}
	// Only a browsable collection catalog counts.
	other := newFakeAddon(t, append([]stremio.Catalog{unbrowsable}, catalogs("series", 2)...))
	if _, err := store.Install(t.Context(), Shared(), other.url("b"), false); err != nil {
		t.Fatal(err)
	}
	if ids := enabled(t, store, Shared()); !slices.Equal(ids, []string{"streaming", "series-0", "series-1"}) {
		t.Errorf("default libraries after an addon without browsable collections: %v", ids)
	}
}

func TestRefreshForgetsOnlyVanishedCatalogs(t *testing.T) {
	store, _ := newStore(t)
	addon := newFakeAddon(t, catalogs("movie", 3))
	installed, err := store.Install(t.Context(), Shared(), addon.url("a"), false)
	if err != nil {
		t.Fatal(err)
	}
	name := "Renamed"
	if _, err := store.SetLibraries(t.Context(), Shared(), []LibraryChoice{
		{AddonID: installed.ID, CatalogType: "movie", CatalogID: "movie-2", Name: &name},
		{AddonID: installed.ID, CatalogType: "movie", CatalogID: "movie-0"},
	}); err != nil {
		t.Fatal(err)
	}
	addon.catalogs.Store(append(catalogs("movie", 2)[1:], stremio.Catalog{Type: "movie", ID: "movie-2", Name: "two"}, stremio.Catalog{Type: "movie", ID: "new"}))
	if _, err := store.Refresh(t.Context(), Shared(), installed.ID, false); err != nil {
		t.Fatal(err)
	}
	libraries, _ := store.Libraries(t.Context(), Shared())
	if !libraries[0].Enabled || libraries[0].Catalog.ID != "movie-2" || *libraries[0].Name != "Renamed" || libraries[1].Enabled {
		t.Errorf("after refresh: %+v", libraries)
	}
	for _, library := range libraries {
		if library.Catalog.ID == "new" && library.Enabled {
			t.Error("a catalog added by a refresh was enabled")
		}
	}
}

func TestLibrariesRejectInvalidChoices(t *testing.T) {
	store, users := newStore(t)
	search := stremio.Catalog{Type: "movie", ID: "search", Extra: []stremio.Extra{{Name: "search", IsRequired: true}}}
	addon := newFakeAddon(t, append(catalogs("movie", 1), search))
	installed, err := store.Install(t.Context(), Shared(), addon.url("a"), false)
	if err != nil {
		t.Fatal(err)
	}
	alice, _ := users.CreateUser(t.Context(), accounts.NewUser{Name: "alice", Password: "correct horse"})
	long := string(make([]rune, 65))
	for name, choices := range map[string][]LibraryChoice{
		"unbrowsable": {{AddonID: installed.ID, CatalogType: "movie", CatalogID: "search"}},
		"unknown":     {{AddonID: installed.ID, CatalogType: "movie", CatalogID: "missing"}},
		"duplicate":   {{AddonID: installed.ID, CatalogType: "movie", CatalogID: "movie-0"}, {AddonID: installed.ID, CatalogType: "movie", CatalogID: "movie-0"}},
	} {
		if _, err := store.SetLibraries(t.Context(), Shared(), choices); !errors.Is(err, ErrInvalidLibrary) {
			t.Errorf("%s: got %v", name, err)
		}
	}
	if _, err := store.SetLibraries(t.Context(), Shared(), []LibraryChoice{{AddonID: installed.ID, CatalogType: "movie", CatalogID: "movie-0", Name: &long}}); !errors.Is(err, ErrInvalidLibraryName) {
		t.Errorf("long name: got %v", err)
	}
	// Another scope cannot use the server's addons as its own.
	if _, err := store.SetLibraries(t.Context(), Personal(alice.ID), []LibraryChoice{{AddonID: installed.ID, CatalogType: "movie", CatalogID: "movie-0"}}); !errors.Is(err, ErrInvalidLibrary) {
		t.Errorf("foreign addon: got %v", err)
	}
	if err := store.Remove(t.Context(), Personal(alice.ID), installed.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("removing another scope's addon: got %v", err)
	}
}

// A library keeps the genre and the maximum it is given, of its catalog's
// genres and within bounds, on a catalog of titles or collections; saving
// it without clears them. A genre its addon stops offering lists the whole
// catalog. A catalog that requires its only genre offers none to choose.
func TestLibrariesKeepAGenreAndAMaximum(t *testing.T) {
	store, _ := newStore(t)
	top := stremio.Catalog{Type: "movie", ID: "top", Name: "Top", Extra: []stremio.Extra{{Name: "genre", Options: []string{"Comedy", "Drama"}}}}
	plain := stremio.Catalog{Type: "movie", ID: "plain", Name: "Plain"}
	channels := stremio.Catalog{Type: "tv", ID: "channels", Name: "Channels"}
	sets := stremio.Catalog{Type: "collection", ID: "sets", Name: "Sets", Extra: []stremio.Extra{{Name: "genre", IsRequired: true, Options: []string{"None"}}}}
	addon := newFakeAddon(t, []stremio.Catalog{top, plain, channels, sets})
	installed, err := store.Install(t.Context(), Shared(), addon.url("a"), false)
	if err != nil {
		t.Fatal(err)
	}
	choice := func(catalog stremio.Catalog, genre string, most int) LibraryChoice {
		c := LibraryChoice{AddonID: installed.ID, CatalogType: catalog.Type, CatalogID: catalog.ID}
		if genre != "" {
			c.Genre = &genre
		}
		if most != 0 {
			c.MaxItems = &most
		}
		return c
	}
	byID := func(libraries []Library) map[string]Library {
		result := map[string]Library{}
		for _, l := range libraries {
			result[l.Catalog.ID] = l
		}
		return result
	}
	saved, err := store.SetLibraries(t.Context(), Shared(), []LibraryChoice{choice(top, "Drama", 50), choice(plain, "", 7), choice(channels, "", 0), choice(sets, "", 3)})
	if err != nil {
		t.Fatal(err)
	}
	libraries := byID(saved)
	if l := libraries["top"]; l.Genre != "Drama" || l.MaxItems != 50 || !l.Filterable {
		t.Errorf("top: %+v", l)
	}
	if l := libraries["plain"]; l.Genre != "" || l.MaxItems != 7 || !l.Filterable {
		t.Errorf("plain: %+v", l)
	}
	if l := libraries["channels"]; l.Filterable {
		t.Errorf("a live TV catalog takes a genre and a maximum: %+v", l)
	}
	if l := libraries["sets"]; l.MaxItems != 3 || !l.Filterable || GenreChoices(l.Catalog) != nil {
		t.Errorf("a collection catalog that requires its only genre: %+v, choices %v", l, GenreChoices(l.Catalog))
	}

	for name, tc := range map[string]struct {
		choice LibraryChoice
		want   error
	}{
		"a genre the catalog does not offer":    {choice(top, "Horror", 0), ErrInvalidLibraryGenre},
		"a genre for a catalog without genres":  {choice(plain, "Drama", 0), ErrInvalidLibraryGenre},
		"a genre for a live TV catalog":         {choice(channels, "Drama", 0), ErrInvalidLibraryGenre},
		"the genre a catalog requires":          {choice(sets, "None", 0), ErrInvalidLibraryGenre},
		"no title at most":                      {choice(top, "", -1), ErrInvalidLibraryMaxItems},
		"more than the most a library may list": {choice(top, "", MaxLibraryItems+1), ErrInvalidLibraryMaxItems},
		"a maximum for a live TV catalog":       {choice(channels, "", 10), ErrInvalidLibraryMaxItems},
	} {
		if _, err := store.SetLibraries(t.Context(), Shared(), []LibraryChoice{tc.choice}); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", name, err, tc.want)
		}
	}
	// The refused saves changed nothing.
	current, _ := store.Libraries(t.Context(), Shared())
	if l := byID(current)["top"]; l.Genre != "Drama" || l.MaxItems != 50 {
		t.Errorf("after refused saves: %+v", l)
	}

	// A genre the addon no longer offers lists the whole catalog.
	addon.catalogs.Store([]stremio.Catalog{{Type: "movie", ID: "top", Name: "Top", Extra: []stremio.Extra{{Name: "genre", Options: []string{"Comedy"}}}}, plain, channels, sets})
	if _, err := store.Refresh(t.Context(), Shared(), installed.ID, false); err != nil {
		t.Fatal(err)
	}
	current, _ = store.Libraries(t.Context(), Shared())
	if l := byID(current)["top"]; l.Genre != "" || l.MaxItems != 50 {
		t.Errorf("after the addon dropped its genre: %+v", l)
	}
	// Saved without them, a library lists its whole catalog again.
	empty := ""
	cleared := choice(top, "", 0)
	cleared.Genre = &empty
	saved, err = store.SetLibraries(t.Context(), Shared(), []LibraryChoice{cleared})
	if err != nil {
		t.Fatal(err)
	}
	if l := byID(saved)["top"]; l.Genre != "" || l.MaxItems != 0 {
		t.Errorf("cleared: %+v", l)
	}
}

func TestReorderNeedsEveryAddonOnce(t *testing.T) {
	store, _ := newStore(t)
	addon := newFakeAddon(t, nil)
	first, _ := store.Install(t.Context(), Shared(), addon.url("a"), false)
	second, _ := store.Install(t.Context(), Shared(), addon.url("b"), false)
	for _, ids := range [][]accounts.ID{{first.ID}, {first.ID, first.ID}} {
		if err := store.Reorder(t.Context(), Shared(), ids); !errors.Is(err, ErrInvalidOrder) {
			t.Errorf("%v: got %v", ids, err)
		}
	}
	if err := store.Reorder(t.Context(), Shared(), []accounts.ID{second.ID, first.ID}); err != nil {
		t.Fatal(err)
	}
	installed, _ := store.Addons(t.Context(), Shared())
	if installed[0].ID != second.ID {
		t.Error("order not applied")
	}
}
