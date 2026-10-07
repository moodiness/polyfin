package library

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// settingsAddon serves a movie catalog, Top, of 6 comedies then 4 dramas,
// which it narrows to either genre, in pages of 4, and a collection
// catalog, Sets, whose one collection groups Top.
func settingsAddon() *fakeAddon {
	comedies := titles("movie", 6, "Comedy")
	dramas := titles("movie", 4, "Drama")
	for i := range dramas {
		dramas[i].ID, dramas[i].Name = fmt.Sprintf("ttd%03d", i), fmt.Sprintf("drama %d", i)
		dramas[i].Poster = fmt.Sprintf("https://images.example/drama-%d.jpg", i)
	}
	return &fakeAddon{
		manifest: stremio.Manifest{ID: "settings", Name: "Settings", Version: "1", Types: []string{"movie", "collection"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs: []stremio.Catalog{
				{Type: "movie", ID: "top", Name: "Top", Extra: []stremio.Extra{{Name: "genre", Options: []string{"Comedy", "Drama"}}, {Name: "skip"}}},
				{Type: "collection", ID: "sets", Name: "Sets"},
			}},
		catalogs: map[string][]stremio.Meta{
			"movie/top":       append(comedies, dramas...),
			"collection/sets": {{ID: "col:all", Type: "collection", Name: "All"}},
		},
		metas: map[string]stremio.Meta{
			"collection/col:all": {ID: "col:all", Type: "collection", Name: "All",
				Collection: &stremio.Collection{Sources: []stremio.CollectionSource{{Type: "movie", CatalogID: "top"}}}},
		},
		pageSize: 4,
	}
}

// settingsLibraries installs settingsAddon as the server's, and makes its
// catalogs the server's libraries as choices set them.
func (e env) settingsLibraries(choices ...func(id accounts.ID) addons.LibraryChoice) *fakeAddon {
	e.t.Helper()
	addon := settingsAddon()
	e.install(addons.Shared(), addon)
	e.chooseLibraries(choices...)
	return addon
}

// chooseLibraries makes the catalogs of the server's one addon its
// libraries as choices set them.
func (e env) chooseLibraries(choices ...func(id accounts.ID) addons.LibraryChoice) {
	e.t.Helper()
	list, err := e.addons.Addons(e.t.Context(), addons.Shared())
	if err != nil || len(list) != 1 {
		e.t.Fatalf("addons: %d %v", len(list), err)
	}
	set := make([]addons.LibraryChoice, len(choices))
	for i, choice := range choices {
		set[i] = choice(list[0].ID)
	}
	if _, err := e.addons.SetLibraries(e.t.Context(), addons.Shared(), set); err != nil {
		e.t.Fatal(err)
	}
}

// libraryOf chooses a catalog of settingsAddon, with a genre and a maximum
// when given.
func libraryOf(catalogType, catalogID, genre string, most int) func(id accounts.ID) addons.LibraryChoice {
	return func(id accounts.ID) addons.LibraryChoice {
		choice := addons.LibraryChoice{AddonID: id, CatalogType: catalogType, CatalogID: catalogID}
		if genre != "" {
			choice.Genre = &genre
		}
		if most != 0 {
			choice.MaxItems = &most
		}
		return choice
	}
}

// A library narrowed to a genre lists that genre's titles only, is named
// after it, offers it alone, adds its catalog to that genre's page only,
// and finds its image among that genre's titles.
func TestALibraryNarrowedToAGenreListsItOnly(t *testing.T) {
	e := newEnv(t)
	e.settingsLibraries(libraryOf("movie", "top", "Drama", 0))
	library := e.library(e.member, "Top · Drama")
	children := func(genre string) Page {
		t.Helper()
		page, err := e.service.Children(t.Context(), e.member, library.ID, 0, 100, genre)
		if err != nil {
			t.Fatal(err)
		}
		return page
	}
	dramas := []string{"drama 0", "drama 1", "drama 2", "drama 3"}
	if page := children(""); !slices.Equal(names(page.Items), dramas) || page.Total != 4 || page.More {
		t.Errorf("the library: %v, total %d, more %v", names(page.Items), page.Total, page.More)
	}
	if page := children("Drama"); !slices.Equal(names(page.Items), dramas) {
		t.Errorf("narrowed to its own genre: %v", names(page.Items))
	}
	if page := children("Comedy"); len(page.Items) != 0 {
		t.Errorf("narrowed to another genre: %v", names(page.Items))
	}
	if genres, err := e.service.Genres(t.Context(), e.member, library.ID); err != nil || !slices.Equal(genres, []string{"Drama"}) {
		t.Errorf("its genres: %v %v", genres, err)
	}
	page := func(genre string) []string {
		t.Helper()
		page, err := e.service.Narrowed(t.Context(), e.member, func(option string) bool { return option == genre }, []Kind{KindMovie}, 0, 100)
		if err != nil {
			t.Fatal(err)
		}
		return names(page.Items)
	}
	if got := page("Drama"); !slices.Equal(got, dramas) {
		t.Errorf("the Drama page: %v", got)
	}
	if got := page("Comedy"); len(got) != 0 {
		t.Errorf("the Comedy page lists the Drama library's catalog: %v", got)
	}

	// Its automatic image comes from its genre's first page.
	key := addons.LibraryKey{AddonID: e.libraries()[0].AddonID, CatalogType: "movie", CatalogID: "top"}
	if err := e.addons.SetLibraryImage(t.Context(), addons.Shared(), key, addons.LibraryImageAutomatic); err != nil {
		t.Fatal(err)
	}
	images, err := e.service.LibraryImages(t.Context(), addons.Shared(), false, e.libraries())
	if err != nil || images[0].URL != "https://images.example/drama-0.jpg" {
		t.Errorf("its image: %+v %v", images, err)
	}
}

// libraries lists the server's catalogs as libraries.
func (e env) libraries() []addons.Library {
	e.t.Helper()
	libraries, err := e.addons.Libraries(e.t.Context(), addons.Shared())
	if err != nil {
		e.t.Fatal(err)
	}
	return libraries
}

// A library lists at most its maximum, reading no page past it, and so do
// the collections of a collection library, whole listings included; a
// whole listing of a library without one lists all its titles.
func TestALibraryListsAtMostItsMaximum(t *testing.T) {
	e := newEnv(t)
	addon := e.settingsLibraries(libraryOf("movie", "top", "", 5), libraryOf("collection", "sets", "", 3))
	top := e.library(e.member, "Top")
	for _, tc := range []struct {
		start, count, want int
	}{{0, 100, 5}, {4, 10, 1}, {5, 10, 0}} {
		page, err := e.service.Children(t.Context(), e.member, top.ID, tc.start, tc.count, "")
		if err != nil || len(page.Items) != tc.want || page.Total != 5 || page.More {
			t.Errorf("from %d: %d titles, total %d, more %v, %v; want %d of 5", tc.start, len(page.Items), page.Total, page.More, err, tc.want)
		}
	}
	// Pages of 4: the third, past the fifth title, is never asked for.
	if requests := addon.catalogRequests(); slices.ContainsFunc(requests, func(r string) bool { return strings.Contains(r, "skip=8") }) {
		t.Errorf("a page past the maximum was read: %v", requests)
	}

	sets := e.library(e.member, "Sets")
	collections, err := e.service.Children(t.Context(), e.member, sets.ID, 0, 100, "")
	if err != nil || len(collections.Items) != 1 {
		t.Fatalf("the collections: %v %v", names(collections.Items), err)
	}
	all := collections.Items[0]
	if page, err := e.service.Children(t.Context(), e.member, all.ID, 0, 100, ""); err != nil || len(page.Items) != 3 || page.Total != 3 || page.More {
		t.Errorf("a collection of the library: %v, total %d, more %v, %v", names(page.Items), page.Total, page.More, err)
	}

	whole := func(parent accounts.ID) []string {
		t.Helper()
		page, err := e.service.Whole(t.Context(), e.member, parent, 0, 2, "")
		if err != nil {
			t.Fatal(err)
		}
		return names(page.Items)
	}
	for name, tc := range map[string]struct {
		parent accounts.ID
		want   int
	}{"the library": {top.ID, 5}, "a collection of a library": {all.ID, 3}} {
		if got := whole(tc.parent); len(got) != tc.want {
			t.Errorf("%s asked for no limit: %v, want %d titles", name, got, tc.want)
		}
	}
	e.chooseLibraries(libraryOf("movie", "top", "", 0))
	if got := whole(e.library(e.member, "Top").ID); len(got) != 10 {
		t.Errorf("a library without a maximum asked for no limit: %v", got)
	}
}

// A whole listing waits for addons at most wholeWait: it then lists the
// titles read so far, well before the addon's request would time out (15
// seconds), while the reads go on for the next listing.
func TestAWholeListingWaitsForAddonsAWhile(t *testing.T) {
	e := newEnv(t)
	addon := e.settingsLibraries(libraryOf("movie", "top", "", 0))
	top := e.library(e.member, "Top")
	// The first page is read; then the addon answers nothing until the end.
	if _, err := e.service.Children(t.Context(), e.member, top.ID, 0, 4, ""); err != nil {
		t.Fatal(err)
	}
	gate := make(chan struct{})
	addon.mu.Lock()
	addon.catalogGate = gate
	addon.mu.Unlock()
	t.Cleanup(func() { close(gate) })
	e.service.wholeWait = 50 * time.Millisecond
	started := time.Now()
	page, err := e.service.Whole(t.Context(), e.member, top.ID, 0, 4, "")
	if waited := time.Since(started); waited > 5*time.Second {
		t.Errorf("waited %v for the addon", waited)
	}
	if want := []string{"movie 0", "movie 1", "movie 2", "movie 3"}; err != nil || !slices.Equal(names(page.Items), want) || !page.More {
		t.Errorf("listed %v, more %v, %v; want %v and more", names(page.Items), page.More, err, want)
	}
}

// A whole listing of a collection that runs out of time lists what was
// read of each of its catalogs, though the late listing, which merges
// what its catalogs answered in time, may have left some out.
func TestALateWholeListingOfACollectionKeepsEachCatalog(t *testing.T) {
	e := newEnv(t)
	second := titles("movie", 6)
	for i := range second {
		second[i].ID, second[i].Name = fmt.Sprintf("ttb%03d", i), fmt.Sprintf("other %d", i)
	}
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "pair", Name: "Pair", Version: "1", Types: []string{"movie", "collection"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs: []stremio.Catalog{
				{Type: "movie", ID: "first", Name: "First", Extra: []stremio.Extra{{Name: "skip"}}},
				{Type: "movie", ID: "second", Name: "Second", Extra: []stremio.Extra{{Name: "skip"}}},
				{Type: "collection", ID: "sets", Name: "Sets"},
			}},
		catalogs: map[string][]stremio.Meta{
			"movie/first":     titles("movie", 6),
			"movie/second":    second,
			"collection/sets": {{ID: "col:pair", Type: "collection", Name: "Pair"}},
		},
		metas: map[string]stremio.Meta{
			"collection/col:pair": {ID: "col:pair", Type: "collection", Name: "Pair", Collection: &stremio.Collection{
				Sources: []stremio.CollectionSource{{Type: "movie", CatalogID: "first"}, {Type: "movie", CatalogID: "second"}}}},
		},
		pageSize: 2,
	}
	e.install(addons.Shared(), addon)
	e.chooseLibraries(libraryOf("collection", "sets", "", 0))
	collections := e.children(e.member, "Sets", 0, 10)
	if len(collections.Items) != 1 {
		t.Fatalf("the collections: %v", names(collections.Items))
	}
	pair := collections.Items[0].ID
	// The first two pages of each catalog are read; then the addon answers
	// nothing until the end.
	if page, err := e.service.Children(t.Context(), e.member, pair, 0, 4, ""); err != nil || len(page.Items) != 4 {
		t.Fatalf("the first titles: %v %v", names(page.Items), err)
	}
	gate := make(chan struct{})
	addon.mu.Lock()
	addon.catalogGate = gate
	addon.mu.Unlock()
	t.Cleanup(func() { close(gate) })
	e.service.wholeWait = 50 * time.Millisecond
	page, err := e.service.Whole(t.Context(), e.member, pair, 0, 4, "")
	want := []string{"movie 0", "other 0", "movie 1", "other 1", "movie 2", "other 2", "movie 3", "other 3"}
	if err != nil || !slices.Equal(names(page.Items), want) || !page.More {
		t.Errorf("listed %v, more %v, %v; want %v and more", names(page.Items), page.More, err, want)
	}
}

// A library's maximum replaces the catalog limit, higher as well as lower;
// a library without one lists up to the catalog limit.
func TestALibrarysMaximumReplacesTheCatalogLimit(t *testing.T) {
	e := newEnv(t)
	e.setting(func(s *accounts.Settings) { s.CatalogLimit = accounts.MinCatalogLimit })
	e.install(addons.Shared(), pagedAddon(150, 50))
	listed := func(most int) Page {
		t.Helper()
		e.chooseLibraries(libraryOf("movie", "top", "", most))
		page, err := e.service.Children(t.Context(), e.member, e.library(e.member, "Top").ID, 0, 200, "")
		if err != nil {
			t.Fatal(err)
		}
		return page
	}
	for _, tc := range []struct{ most, want int }{{0, 100}, {120, 120}, {30, 30}} {
		if page := listed(tc.most); len(page.Items) != tc.want || page.Total != tc.want || page.More {
			t.Errorf("a library of %d at most: %d titles, total %d, more %v; want %d", tc.most, len(page.Items), page.Total, page.More, tc.want)
		}
	}
}
