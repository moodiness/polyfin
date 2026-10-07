package library

import (
	"fmt"
	"slices"
	"strings"
	"sync"
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
// titles read so far, even fewer than a listing of count, rather than wait
// for the addon, whose request would time out only after 15 seconds.
func TestAWholeListingWaitsForAddonsAWhile(t *testing.T) {
	e := newEnv(t)
	addon := e.settingsLibraries(libraryOf("movie", "top", "", 0))
	top := e.library(e.member, "Top")
	// The first page is read; then the addon answers nothing for a while.
	if _, err := e.service.Children(t.Context(), e.member, top.ID, 0, 4, ""); err != nil {
		t.Fatal(err)
	}
	answer := blockCatalogs(t, addon)
	time.AfterFunc(7*time.Second, answer)
	e.service.wholeWait = 50 * time.Millisecond
	started := time.Now()
	page, err := e.service.Whole(t.Context(), e.member, top.ID, 0, 6, "")
	if waited := time.Since(started); waited > 5*time.Second {
		t.Errorf("waited %v for the addon", waited)
	}
	if want := []string{"movie 0", "movie 1", "movie 2", "movie 3"}; err != nil || !slices.Equal(names(page.Items), want) || !page.More {
		t.Errorf("listed %v, more %v, %v; want %v and more", names(page.Items), page.More, err, want)
	}
}

// blockCatalogs has addon answer no catalog request until the function it
// returns is called, or the test ends.
func blockCatalogs(t *testing.T, addon *fakeAddon) func() {
	gate := make(chan struct{})
	answer := sync.OnceFunc(func() { close(gate) })
	addon.mu.Lock()
	addon.catalogGate = gate
	addon.mu.Unlock()
	t.Cleanup(answer)
	return answer
}

// pairAddon serves a collection catalog, Sets, whose one collection, Pair,
// groups two movie catalogs of size titles each, in pages of 2: movie 0,
// movie 1… and other 0, other 1….
func pairAddon(size int) *fakeAddon {
	second := titles("movie", size)
	for i := range second {
		second[i].ID, second[i].Name = fmt.Sprintf("ttb%03d", i), fmt.Sprintf("other %d", i)
	}
	return &fakeAddon{
		manifest: stremio.Manifest{ID: "pair", Name: "Pair", Version: "1", Types: []string{"movie", "collection"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs: []stremio.Catalog{
				{Type: "movie", ID: "first", Name: "First", Extra: []stremio.Extra{{Name: "skip"}}},
				{Type: "movie", ID: "second", Name: "Second", Extra: []stremio.Extra{{Name: "skip"}}},
				{Type: "collection", ID: "sets", Name: "Sets"},
			}},
		catalogs: map[string][]stremio.Meta{
			"movie/first":     titles("movie", size),
			"movie/second":    second,
			"collection/sets": {{ID: "col:pair", Type: "collection", Name: "Pair"}},
		},
		metas: map[string]stremio.Meta{
			"collection/col:pair": {ID: "col:pair", Type: "collection", Name: "Pair", Collection: &stremio.Collection{
				Sources: []stremio.CollectionSource{{Type: "movie", CatalogID: "first"}, {Type: "movie", CatalogID: "second"}}}},
		},
		pageSize: 2,
	}
}

// pairOf installs addon and returns its collection Pair, whose first two
// pages of each catalog an earlier listing read: 4 titles of each.
func (e env) pairOf(addon *fakeAddon) accounts.ID {
	e.t.Helper()
	e.install(addons.Shared(), addon)
	e.chooseLibraries(libraryOf("collection", "sets", "", 0))
	collections := e.children(e.member, "Sets", 0, 10)
	if len(collections.Items) != 1 {
		e.t.Fatalf("the collections: %v", names(collections.Items))
	}
	pair := collections.Items[0].ID
	if page, err := e.service.Children(e.t.Context(), e.member, pair, 0, 4, ""); err != nil || len(page.Items) != 4 {
		e.t.Fatalf("the first titles: %v %v", names(page.Items), err)
	}
	return pair
}

// A whole listing of a collection that runs out of time lists what was
// read of each of its catalogs, though the late listing, which merges
// what its catalogs answered in time, may have left some out.
func TestALateWholeListingOfACollectionKeepsEachCatalog(t *testing.T) {
	e := newEnv(t)
	addon := pairAddon(6)
	pair := e.pairOf(addon)
	answer := blockCatalogs(t, addon)
	time.AfterFunc(7*time.Second, answer)
	e.service.wholeWait = 50 * time.Millisecond
	page, err := e.service.Whole(t.Context(), e.member, pair, 0, 10, "")
	want := []string{"movie 0", "other 0", "movie 1", "other 1", "movie 2", "other 2", "movie 3", "other 3"}
	if err != nil || !slices.Equal(names(page.Items), want) || !page.More {
		t.Errorf("listed %v, more %v, %v; want %v and more", names(page.Items), page.More, err, want)
	}
}

// A whole listing opened again, whose last listing kept a listing of
// count, waits for the addon only keptWait before it lists what was kept;
// the rest is read in the background, to the end, so that the next one
// lists all of it at once.
func TestAWholeListingOpenedAgainWaitsOnlyAMoment(t *testing.T) {
	e := newEnv(t)
	addon := pairAddon(30)
	pair := e.pairOf(addon)
	e.service.readAhead = true
	answer := blockCatalogs(t, addon)
	time.AfterFunc(7*time.Second, answer)
	e.service.wholeWait, e.service.keptWait = 5*time.Second, 50*time.Millisecond
	started := time.Now()
	page, err := e.service.Whole(t.Context(), e.member, pair, 0, 4, "")
	if waited := time.Since(started); waited > time.Second {
		t.Errorf("waited %v for the addon", waited)
	}
	want := []string{"movie 0", "other 0", "movie 1", "other 1", "movie 2", "other 2", "movie 3", "other 3"}
	if err != nil || !slices.Equal(names(page.Items), want) || !page.More {
		t.Errorf("listed %v, more %v, %v; want %v and more", names(page.Items), page.More, err, want)
	}
	answer()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		kept, err := e.service.Children(withKeptOnly(t.Context()), e.member, pair, 0, WholeListing, "")
		if err == nil && len(kept.Items) == 60 && !kept.More {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("read in the background: %d titles, more %v, %v", len(kept.Items), kept.More, err)
		}
	}
	// The addon answers nothing again: all of it was kept.
	blockCatalogs(t, addon)
	page, err = e.service.Whole(t.Context(), e.member, pair, 0, 4, "")
	if err != nil || len(page.Items) != 60 || page.More {
		t.Errorf("listed again: %d titles, more %v, %v; want all 60", len(page.Items), page.More, err)
	}
}

// A collection first opened while its addon is slow, past wholeWait before
// anything of it was read, lists the first page of each of its catalogs,
// waited for as long as the addon takes, rather than nothing.
func TestASlowFirstOpeningListsTheCollectionsTitles(t *testing.T) {
	for name, tc := range map[string]struct {
		addon   func() *fakeAddon
		library string
		want    []string
	}{
		"a collection of catalogs":    {func() *fakeAddon { return pairAddon(6) }, "Sets", []string{"movie 0", "other 0", "movie 1", "other 1"}},
		"a collection of collections": {nestedAddon, "Genres", []string{"movie 0", "series 0", "movie 1", "series 1"}},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			addon := tc.addon()
			e.install(addons.Shared(), addon)
			collections := e.children(e.member, tc.library, 0, 10)
			if len(collections.Items) != 1 {
				t.Fatalf("the collections: %v", names(collections.Items))
			}
			answer := blockCatalogs(t, addon)
			time.AfterFunc(300*time.Millisecond, answer)
			e.service.wholeWait = 50 * time.Millisecond
			page, err := e.service.Whole(t.Context(), e.member, collections.Items[0].ID, 0, 4, "")
			if err != nil || !slices.Equal(names(page.Items), tc.want) || !page.More {
				t.Errorf("listed %v, more %v, %v; want %v and more", names(page.Items), page.More, err, tc.want)
			}
		})
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
