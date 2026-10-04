package library

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/testdb"
)

// fakeAddon is a Stremio addon serving fixed catalogs and metas, recording
// the catalog requests it receives.
type fakeAddon struct {
	manifest stremio.Manifest
	catalogs map[string][]stremio.Meta // "type/id" → every item
	metas    map[string]stremio.Meta   // "type/id" → complete meta
	streams  map[string][]stremio.Stream
	pageSize int
	// short shortens the page at a skip by that many items, as addons that
	// filter their catalogs do.
	short map[int]int
	// metaGate, when set, holds every meta request until it is closed.
	metaGate chan struct{}

	mu       sync.Mutex
	requests []string
}

func (a *fakeAddon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(strings.TrimPrefix(r.URL.EscapedPath(), "/"), ".json")
	parts := strings.Split(path, "/")
	a.mu.Lock()
	a.requests = append(a.requests, path)
	a.mu.Unlock()
	switch {
	case path == "manifest":
		_ = json.NewEncoder(w).Encode(a.manifest)
	case parts[0] == "catalog" && len(parts) >= 3:
		catalogID, _ := url.PathUnescape(parts[2])
		extra := url.Values{}
		if len(parts) == 4 {
			for _, pair := range strings.Split(parts[3], "&") {
				name, value, _ := strings.Cut(pair, "=")
				name, _ = url.PathUnescape(name)
				value, _ = url.PathUnescape(value)
				extra.Set(name, value)
			}
		}
		items := a.catalogs[parts[1]+"/"+catalogID]
		if search := extra.Get("search"); search != "" {
			items = slices.DeleteFunc(slices.Clone(items), func(m stremio.Meta) bool {
				return !strings.Contains(strings.ToLower(m.Name), strings.ToLower(search))
			})
		}
		if genre := extra.Get("genre"); genre != "" && genre != "None" {
			items = slices.DeleteFunc(slices.Clone(items), func(m stremio.Meta) bool { return !slices.Contains(m.Genres, genre) })
		}
		skip, _ := strconv.Atoi(extra.Get("skip"))
		size := a.pageSize
		if size == 0 {
			size = len(items)
		}
		from, to := min(skip, len(items)), min(skip+size, len(items))
		to = max(from, to-a.short[skip])
		_ = json.NewEncoder(w).Encode(map[string]any{"metas": items[from:to]})
	case parts[0] == "meta" && len(parts) == 3:
		if a.metaGate != nil {
			select {
			case <-a.metaGate:
			case <-r.Context().Done():
				return
			}
		}
		id, _ := url.PathUnescape(parts[2])
		meta, ok := a.metas[parts[1]+"/"+id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
	case parts[0] == "stream" && len(parts) == 3:
		id, _ := url.PathUnescape(parts[2])
		a.mu.Lock()
		streams := a.streams[parts[1]+"/"+id]
		a.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"streams": streams})
	default:
		http.NotFound(w, r)
	}
}

func (a *fakeAddon) catalogRequests() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var result []string
	for _, request := range a.requests {
		if strings.HasPrefix(request, "catalog/") {
			result = append(result, request)
		}
	}
	return result
}

func titles(kind string, count int, genres ...string) []stremio.Meta {
	result := make([]stremio.Meta, count)
	for i := range result {
		result[i] = stremio.Meta{ID: fmt.Sprintf("tt%s%03d", kind[:1], i), Type: kind, Name: fmt.Sprintf("%s %d", kind, i),
			Poster: fmt.Sprintf("https://images.example/%s-%d.jpg", kind, i), Genres: genres}
	}
	return result
}

type env struct {
	t        *testing.T
	service  *Service
	addons   *addons.Store
	users    *accounts.Store
	admin    accounts.User
	member   accounts.User
	language *string // the server language the service reads
}

func newEnv(t *testing.T) env {
	t.Helper()
	pool := testdb.New(t)
	if err := database.Migrate(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	users, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	admin, _ := users.CreateUser(t.Context(), accounts.NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true})
	member, _ := users.CreateUser(t.Context(), accounts.NewUser{Name: "member", Password: "correct horse"})
	client := stremio.NewClient("test")
	store := addons.New(pool, client)
	language := new("en")
	service := New(pool, store, client, slog.New(slog.NewTextHandler(io.Discard, nil)), func() string { return *language })
	return env{t: t, service: service, addons: store, users: users, admin: admin, member: member, language: language}
}

// install serves addon and installs it in scope as a trusted (local) addon.
func (e env) install(scope addons.Scope, addon *fakeAddon) {
	e.t.Helper()
	server := httptest.NewServer(addon)
	e.t.Cleanup(server.Close)
	if _, err := e.addons.Install(e.t.Context(), scope, server.URL+"/manifest.json", false); err != nil {
		e.t.Fatal(err)
	}
}

func (e env) library(user accounts.User, name string) Item {
	e.t.Helper()
	libraries, err := e.service.Libraries(e.t.Context(), user)
	if err != nil {
		e.t.Fatal(err)
	}
	for _, library := range libraries {
		if library.Name == name {
			return library
		}
	}
	e.t.Fatalf("no library %q in %v", name, libraries)
	return Item{}
}

func names(items []Item) []string {
	result := make([]string, len(items))
	for i, item := range items {
		result[i] = item.Name
	}
	return result
}

func TestCatalogsArePagedWithSkip(t *testing.T) {
	e := newEnv(t)
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "a", Name: "A", Version: "1", Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{
				{Type: "movie", ID: "top", Name: "Top", Extra: []stremio.Extra{{Name: "skip"}}},
				{Type: "movie", ID: "fixed", Name: "Fixed"},
			}},
		catalogs: map[string][]stremio.Meta{"movie/top": titles("movie", 45), "movie/fixed": titles("movie", 45)},
		pageSize: 20,
	}
	e.install(addons.Shared(), addon)
	top := e.library(e.member, "Top")

	page, err := e.service.Children(t.Context(), e.member, top.ID, 0, 30, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 30 || page.Items[29].Name != "movie 29" || !page.More {
		t.Fatalf("first page: %d items, more=%v", len(page.Items), page.More)
	}
	page, _ = e.service.Children(t.Context(), e.member, top.ID, 40, 30, "")
	if len(page.Items) != 5 || page.Items[0].Name != "movie 40" || page.More || page.Total != 45 {
		t.Fatalf("last page: %v more=%v total=%d", names(page.Items), page.More, page.Total)
	}
	if page, _ := e.service.Children(t.Context(), e.member, top.ID, 100, 30, ""); len(page.Items) != 0 || page.Total != 45 || page.More {
		t.Fatalf("beyond the end: %v more=%v total=%d", names(page.Items), page.More, page.Total)
	}
	// A catalog without the skip property has a single page.
	fixed := e.library(e.member, "Fixed")
	page, _ = e.service.Children(t.Context(), e.member, fixed.ID, 0, 100, "")
	if len(page.Items) != 20 || page.More {
		t.Fatalf("catalog without skip: %d items, more=%v", len(page.Items), page.More)
	}
	if page, _ := e.service.Children(t.Context(), e.member, fixed.ID, 20, 20, ""); len(page.Items) != 0 {
		t.Fatalf("beyond the only page: %v", names(page.Items))
	}
}

func TestShortPagesDoNotEndACatalog(t *testing.T) {
	e := newEnv(t)
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "a", Name: "A", Version: "1", Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top", Extra: []stremio.Extra{{Name: "skip"}}}}},
		catalogs: map[string][]stremio.Meta{"movie/top": titles("movie", 70)},
		pageSize: 20,
		short:    map[int]int{20: 6},
	}
	e.install(addons.Shared(), addon)
	top := e.library(e.member, "Top")
	page, err := e.service.Children(t.Context(), e.member, top.ID, 0, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 70 || page.More || page.Items[34].Name != "movie 34" || page.Items[69].Name != "movie 69" {
		t.Fatalf("catalog with a short page: %d items, more=%v", len(page.Items), page.More)
	}
	// The page after the short one starts where it stopped.
	if !slices.Contains(addon.catalogRequests(), "catalog/movie/top/skip=34") {
		t.Errorf("requests: %v", addon.catalogRequests())
	}
}

func TestCollectionsCanGroupCollections(t *testing.T) {
	e := newEnv(t)
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "a", Name: "A", Version: "1", Types: []string{"movie", "collection"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs: []stremio.Catalog{
				{Type: "collection", ID: "genres", Name: "Genres"},
				{Type: "movie", ID: "popular", Name: "Popular", Extra: []stremio.Extra{{Name: "genre", Options: []string{"Comedy"}}}},
			}},
		catalogs: map[string][]stremio.Meta{
			"collection/genres": {{ID: "col:comedy", Type: "collection", Name: "Comedy"}},
			"movie/popular":     titles("movie", 2, "Comedy"),
		},
		metas: map[string]stremio.Meta{
			"collection/col:comedy": {ID: "col:comedy", Type: "collection", Name: "Comedy",
				Collection: &stremio.Collection{Items: []stremio.Meta{{ID: "col:comedy:films", Type: "collection", Name: "Films"}}}},
			"collection/col:comedy:films": {ID: "col:comedy:films", Type: "collection", Name: "Films",
				Collection: &stremio.Collection{Sources: []stremio.CollectionSource{{Type: "movie", CatalogID: "popular", Genre: "Comedy"}}}},
		},
	}
	e.install(addons.Shared(), addon)
	genres := e.library(e.member, "Genres")
	page, _ := e.service.Children(t.Context(), e.member, genres.ID, 0, 10, "")
	comedy := page.Items[0]
	page, err := e.service.Children(t.Context(), e.member, comedy.ID, 0, 10, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].Kind != KindCollection || page.Items[0].Name != "Films" || page.Total != 1 {
		t.Fatalf("collection of collections: %+v %v", page, err)
	}
	page, _ = e.service.Children(t.Context(), e.member, page.Items[0].ID, 0, 10, "")
	if got := names(page.Items); !slices.Equal(got, []string{"movie 0", "movie 1"}) {
		t.Fatalf("nested collection: %v", got)
	}
	ancestors, _ := e.service.Ancestors(t.Context(), e.member, page.Items[0].ID)
	if got := names(ancestors); !slices.Equal(got, []string{"Films", "Comedy", "Genres"}) {
		t.Errorf("ancestors: %v", got)
	}
}

func TestCollectionsGroupCatalogs(t *testing.T) {
	e := newEnv(t)
	movies := titles("movie", 3)
	series := titles("series", 2)
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "a", Name: "A", Version: "1", Types: []string{"movie", "series", "collection"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs: []stremio.Catalog{
				{Type: "collection", ID: "themes", Name: "Themes", Extra: []stremio.Extra{{Name: "genre", IsRequired: true, Options: []string{"None"}}}},
				{Type: "movie", ID: "space", Name: "Space movies", Extra: []stremio.Extra{{Name: "genre", Options: []string{"Action"}}}},
				{Type: "series", ID: "space", Name: "Space series"},
				{Type: "movie", ID: "search", Name: "Search", Extra: []stremio.Extra{{Name: "search", IsRequired: true}}},
			}},
		catalogs: map[string][]stremio.Meta{
			"collection/themes": {{ID: "col:space", Type: "collection", Name: "Space", Poster: "https://images.example/space.png"}},
			"movie/space":       append(append([]stremio.Meta{}, movies...), series[0]), // a duplicate across catalogs
			"series/space":      series,
		},
		metas: map[string]stremio.Meta{
			"collection/col:space": {ID: "col:space", Type: "collection", Name: "Space", Collection: &stremio.Collection{Sources: []stremio.CollectionSource{
				{Type: "movie", CatalogID: "space", Genre: "Action"}, {Type: "series", CatalogID: "space"},
			}}},
		},
	}
	for i := range addon.catalogs["movie/space"] {
		addon.catalogs["movie/space"][i].Genres = []string{"Action"}
	}
	e.install(addons.Shared(), addon)

	// An addon with collections gets its collection catalogs as libraries.
	libraries, _ := e.service.Libraries(t.Context(), e.member)
	if got := names(libraries); !slices.Equal(got, []string{"Themes"}) {
		t.Fatalf("default libraries: %v", got)
	}
	page, err := e.service.Children(t.Context(), e.member, libraries[0].ID, 0, 10, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].Kind != KindCollection {
		t.Fatalf("collections: %+v %v", page, err)
	}
	space := page.Items[0]
	page, err = e.service.Children(t.Context(), e.member, space.ID, 0, 10, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"movie 0", "series 0", "movie 1", "series 1", "movie 2"}
	if got := names(page.Items); !slices.Equal(got, want) {
		t.Fatalf("collection content: %v, want %v (interleaved, without the duplicate)", got, want)
	}
	if !slices.ContainsFunc(addon.catalogRequests(), func(r string) bool { return strings.HasPrefix(r, "catalog/movie/space/genre=Action") }) {
		t.Errorf("the source's genre was not requested: %v", addon.catalogRequests())
	}
	ancestors, _ := e.service.Ancestors(t.Context(), e.member, page.Items[0].ID)
	if got := names(ancestors); !slices.Equal(got, []string{"Space", "Themes"}) {
		t.Errorf("ancestors: %v", got)
	}
}

func TestSeriesSeasonsAndEpisodes(t *testing.T) {
	e := newEnv(t)
	now := time.Now().UTC()
	future := now.Add(30 * 24 * time.Hour).Format(time.RFC3339)
	show := stremio.Meta{ID: "tt100", Type: "series", Name: "Show", Poster: "https://images.example/show.jpg",
		Extras: &stremio.Extras{SeasonPosterByNumber: map[string]string{"2": "https://images.example/s2.jpg"}},
		Videos: []stremio.Video{
			{ID: "tt100:2:1", Title: "Second one", Season: 2, Episode: 1, Released: future},
			{ID: "tt100:1:2", Title: "Pilot two", Season: 1, Episode: 2, Released: "2020-01-08T00:00:00Z"},
			{ID: "tt100:1:1", Title: "Pilot", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00Z", Runtime: "42min"},
			{ID: "tt100:0:1", Name: "Special", Season: 0, Episode: 1, Released: "2019-12-01T00:00:00Z"},
		}}
	preview := show
	preview.Videos, preview.Extras = nil, nil
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "a", Name: "A", Version: "1", Types: []string{"series"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta", Types: []string{"series"}, IDPrefixes: []string{"tt"}}},
			Catalogs:  []stremio.Catalog{{Type: "series", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{"series/top": {preview}},
		metas:    map[string]stremio.Meta{"series/tt100": show},
	}
	e.install(addons.Shared(), addon)
	top := e.library(e.member, "Top")
	page, _ := e.service.Children(t.Context(), e.member, top.ID, 0, 10, "")
	series := page.Items[0]

	seasons, err := e.service.Seasons(t.Context(), e.member, series.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(seasons); !slices.Equal(got, []string{"Specials", "Season 1", "Season 2"}) {
		t.Fatalf("seasons: %v", got)
	}
	episodes, err := e.service.Episodes(t.Context(), e.member, series.ID, &seasons[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(episodes); !slices.Equal(got, []string{"Pilot", "Pilot two"}) || episodes[0].Runtime != 42*time.Minute {
		t.Fatalf("season 1: %v (runtime %v)", got, episodes[0].Runtime)
	}
	upcoming, _ := e.service.Episodes(t.Context(), e.member, series.ID, &seasons[2].ID)
	if len(upcoming) != 1 || upcoming[0].Available {
		t.Errorf("an episode released next month is available: %+v", upcoming)
	}
	// Seasons and episodes resolve by identifier, and images follow them.
	if item, err := e.service.Item(t.Context(), e.member, episodes[1].ID); err != nil || item.Name != "Pilot two" || item.SeasonID != seasons[1].ID {
		t.Errorf("episode by identifier: %+v %v", item, err)
	}
	if url, _, err := e.service.Artwork(t.Context(), seasons[2].ID, "Primary"); err != nil || url != "https://images.example/s2.jpg" {
		t.Errorf("season 2 poster: %q %v", url, err)
	}
}

func TestSearchQueriesOnlySearchCatalogs(t *testing.T) {
	e := newEnv(t)
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "a", Name: "A", Version: "1", Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{
				{Type: "movie", ID: "top", Name: "Top"},
				{Type: "movie", ID: "search", Name: "Search", Extra: []stremio.Extra{{Name: "search", IsRequired: true}}},
				{Type: "series", ID: "search", Name: "Search", Extra: []stremio.Extra{{Name: "search", IsRequired: true}}},
			}},
		catalogs: map[string][]stremio.Meta{
			"movie/top":     titles("movie", 5),
			"movie/search":  titles("movie", 12),
			"series/search": titles("series", 3),
		},
	}
	e.install(addons.Shared(), addon)
	found, err := e.service.Search(t.Context(), e.member, "movie 1", []Kind{KindMovie}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(found); !slices.Equal(got, []string{"movie 1", "movie 10", "movie 11"}) {
		t.Errorf("movie search: %v", got)
	}
	for _, request := range addon.catalogRequests() {
		if strings.Contains(request, "/top") || strings.Contains(request, "series/") {
			t.Errorf("a catalog that was not asked for was queried: %s", request)
		}
	}
}

func TestSearchKeepsTheFolderATitleWasListedIn(t *testing.T) {
	e := newEnv(t)
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "a", Name: "A", Version: "1", Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{
				{Type: "movie", ID: "top", Name: "Top"},
				{Type: "movie", ID: "search", Name: "Search", Extra: []stremio.Extra{{Name: "search", IsRequired: true}}},
			}},
		catalogs: map[string][]stremio.Meta{"movie/top": titles("movie", 2), "movie/search": titles("movie", 2)},
	}
	e.install(addons.Shared(), addon)
	top := e.library(e.member, "Top")
	page, _ := e.service.Children(t.Context(), e.member, top.ID, 0, 10, "")
	if _, err := e.service.Search(t.Context(), e.member, "movie 1", []Kind{KindMovie}, 10); err != nil {
		t.Fatal(err)
	}
	ancestors, err := e.service.Ancestors(t.Context(), e.member, page.Items[1].ID)
	if got := names(ancestors); err != nil || !slices.Equal(got, []string{"Top"}) {
		t.Errorf("ancestors after a search found the title: %v %v", got, err)
	}
}

// A title is recorded with the catalog that listed it, whose addon decides
// whether its artwork may be fetched from the local network.
func TestMergedTitlesKeepTheCatalogThatListedThem(t *testing.T) {
	e := newEnv(t)
	search := []stremio.Extra{{Name: "search", IsRequired: true}}
	e.install(addons.Shared(), &fakeAddon{
		manifest: stremio.Manifest{ID: "shared", Name: "Shared", Version: "1", Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{{Type: "movie", ID: "search", Name: "Search", Extra: search}}},
		catalogs: map[string][]stremio.Meta{"movie/search": titles("movie", 1)},
	})
	server := httptest.NewServer(&fakeAddon{manifest: stremio.Manifest{ID: "own", Name: "Own", Version: "1",
		Resources: []stremio.Resource{{Name: "catalog"}}, Catalogs: []stremio.Catalog{{Type: "movie", ID: "mine", Name: "Mine", Extra: search}}}})
	t.Cleanup(server.Close)
	own, err := e.addons.Install(t.Context(), addons.Personal(e.member.ID), server.URL+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	// The member's own addon only reaches public addresses; its page comes
	// from cache, as from an addon on a public host.
	mine := stremio.Meta{ID: "tt-own", Type: "movie", Name: "movie own", Poster: "http://192.168.1.2/poster.jpg"}
	e.service.pages.Put(pageKey{own.ID, "movie", "mine", "", "movie", 0}, []stremio.Meta{mine})
	found, err := e.service.Search(t.Context(), e.member, "movie", []Kind{KindMovie}, 10)
	if got := names(found); err != nil || !slices.Equal(got, []string{"movie 0", "movie own"}) {
		t.Fatalf("search: %v %v", got, err)
	}
	r, err := e.service.load(t.Context(), found[1].ID)
	if err != nil || r.Addon == nil || *r.Addon != own.ID || r.CatalogID != "mine" || !r.Confined {
		t.Errorf("record of the member's title: %+v %v", r, err)
	}
	if _, confined, err := e.service.Artwork(t.Context(), found[1].ID, "Primary"); err != nil || !confined {
		t.Errorf("artwork of the member's title: confined %t, %v", confined, err)
	}
}

func TestUsersOnlyReachTheirLibraries(t *testing.T) {
	e := newEnv(t)
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "a", Name: "A", Version: "1", Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{"movie/top": titles("movie", 2)},
	}
	e.install(addons.Shared(), addon)
	top := e.library(e.member, "Top")
	if err := e.addons.SetUsesSharedAddons(t.Context(), e.member.ID, false); err != nil {
		t.Fatal(err)
	}
	if libraries, _ := e.service.Libraries(t.Context(), e.member); len(libraries) != 0 {
		t.Errorf("libraries with the server's addons off: %v", names(libraries))
	}
	if _, err := e.service.Children(t.Context(), e.member, top.ID, 0, 10, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("listing a library without access: got %v", err)
	}
	// A member's own addon on the local network is refused even if it was
	// installed: its requests stay on public addresses.
	server := httptest.NewServer(addon)
	defer server.Close()
	if _, err := e.addons.Install(t.Context(), addons.Personal(e.member.ID), server.URL+"/manifest.json", false); err != nil {
		t.Fatal(err)
	}
	mine := e.library(e.member, "Top")
	if _, err := e.service.Children(t.Context(), e.member, mine.ID, 0, 10, ""); !errors.Is(err, stremio.ErrPrivateNetwork) {
		t.Errorf("a member's local addon was reached: %v", err)
	}
}

func TestLibraryNamesTellCollisionsApart(t *testing.T) {
	catalog := func(addon, kind, name string) addons.Library {
		return addons.Library{AddonName: addon, Catalog: stremio.Catalog{Type: kind, ID: kind + name, Name: name}}
	}
	custom := func(addon, kind, name, custom string) addons.Library {
		l := catalog(addon, kind, name)
		l.Name = &custom
		return l
	}
	for _, tc := range []struct {
		name      string
		language  string
		libraries []addons.Library
		want      []string
	}{
		{"distinct names are kept", "en",
			[]addons.Library{catalog("A", "movie", "Popular"), catalog("A", "series", "Trending"), custom("A", "movie", "Top", "Best")},
			[]string{"Popular", "Trending", "Best"}},
		{"content types in English", "en",
			[]addons.Library{catalog("A", "movie", "Popular"), catalog("A", "series", " popular "), catalog("A", "collection", "POPULAR"),
				catalog("A", "anime.movie", "Classics"), catalog("A", "anime.series", "Classics"), catalog("A", "tv", "Classics")},
			[]string{"Popular (Movies)", "popular (Shows)", "POPULAR (Collections)", "Classics (Movies)", "Classics (Shows)", "Classics (tv)"}},
		{"content types in French", "fr",
			[]addons.Library{catalog("A", "movie", "Populaires"), catalog("A", "series", "Populaires"), catalog("A", "collection", "Populaires")},
			[]string{"Populaires (Films)", "Populaires (Séries)", "Populaires (Collections)"}},
		{"unknown languages fall back to English", "de",
			[]addons.Library{catalog("A", "movie", "Top"), catalog("A", "series", "Top")},
			[]string{"Top (Movies)", "Top (Shows)"}},
		{"the same type from two addons gets the addon name", "fr",
			[]addons.Library{catalog("AIOMetadata", "movie", "Top"), catalog("Cinemeta", "movie", "Top"), catalog("Cinemeta", "series", "Top")},
			[]string{"Top (Films, AIOMetadata)", "Top (Films, Cinemeta)", "Top (Séries)"}},
		{"the same addon twice gets a counter", "en",
			[]addons.Library{catalog("A", "movie", "Top"), catalog("A", "movie", "Top"), catalog("A", "movie", "Top")},
			[]string{"Top (Movies, A)", "Top (Movies, A) (2)", "Top (Movies, A) (3)"}},
		{"counters skip names in use", "en",
			[]addons.Library{catalog("A", "movie", "Top"), catalog("A", "movie", "Top"), custom("B", "series", "X", "Top (Movies, A) (2)")},
			[]string{"Top (Movies, A)", "Top (Movies, A) (3)", "Top (Movies, A) (2)"}},
		{"a custom name is kept and the catalog names are suffixed", "en",
			[]addons.Library{catalog("A", "movie", "Classics"), custom("A", "series", "Shows", "classics"), catalog("A", "series", "Classics")},
			[]string{"Classics (Movies)", "classics", "Classics (Shows)"}},
		{"custom names given twice get their type", "en",
			[]addons.Library{custom("A", "movie", "Popular", "Mine"), custom("A", "series", "Popular", "Mine")},
			[]string{"Mine (Movies)", "Mine (Shows)"}},
		{"custom names of the same type get their addon", "fr",
			[]addons.Library{custom("A", "movie", "Popular", "Mine"), custom("B", "movie", "Popular", "Mine")},
			[]string{"Mine (Films, A)", "Mine (Films, B)"}},
	} {
		if got := LibraryNames(tc.libraries, tc.language); !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestGeneratedNamesFollowTheServerLanguage(t *testing.T) {
	e := newEnv(t)
	show := stremio.Meta{ID: "tt200", Type: "series", Name: "Show", Videos: []stremio.Video{
		{ID: "tt200:0:1", Title: "Behind the scenes", Season: 0, Episode: 1, Released: "2019-12-01T00:00:00Z"},
		{ID: "tt200:1:1", Title: "Pilot", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00Z"},
		{ID: "tt200:1:2", Season: 1, Episode: 2, Released: "2020-01-08T00:00:00Z"},
	}}
	preview := show
	preview.Videos = nil
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "a", Name: "A", Version: "1", Types: []string{"movie", "series"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta", Types: []string{"series"}, IDPrefixes: []string{"tt"}}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}, {Type: "series", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{"movie/top": titles("movie", 1), "series/top": {preview}},
		metas:    map[string]stremio.Meta{"series/tt200": show},
	}
	e.install(addons.Shared(), addon)

	english, err := e.service.Libraries(t.Context(), e.member)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(english); !slices.Equal(got, []string{"Top (Movies)", "Top (Shows)"}) {
		t.Fatalf("libraries in English: %v", got)
	}
	page, _ := e.service.Children(t.Context(), e.member, english[1].ID, 0, 10, "")
	series := page.Items[0]
	seasons, _ := e.service.Seasons(t.Context(), e.member, series.ID)
	episodes, _ := e.service.Episodes(t.Context(), e.member, series.ID, nil)
	if got := names(seasons); !slices.Equal(got, []string{"Specials", "Season 1"}) {
		t.Errorf("seasons in English: %v", got)
	}
	if got := names(episodes); !slices.Equal(got, []string{"Behind the scenes", "Pilot", "Episode 2"}) {
		t.Errorf("episodes in English: %v", got)
	}

	// The language applies at once, and identifiers stay.
	*e.language = "fr"
	french, _ := e.service.Libraries(t.Context(), e.member)
	if got := names(french); !slices.Equal(got, []string{"Top (Films)", "Top (Séries)"}) {
		t.Errorf("libraries in French: %v", got)
	}
	if french[0].ID != english[0].ID || french[1].ID != english[1].ID {
		t.Errorf("library identifiers changed with the language")
	}
	seasons, _ = e.service.Seasons(t.Context(), e.member, series.ID)
	if got := names(seasons); !slices.Equal(got, []string{"Épisodes spéciaux", "Saison 1"}) {
		t.Errorf("seasons in French: %v", got)
	}
	episodes, _ = e.service.Episodes(t.Context(), e.member, series.ID, &seasons[1].ID)
	if got := names(episodes); !slices.Equal(got, []string{"Pilot", "Épisode 2"}) || episodes[1].SeasonName != "Saison 1" {
		t.Errorf("season 1 in French: %v (season %q)", got, episodes[1].SeasonName)
	}
	if item, err := e.service.Item(t.Context(), e.member, seasons[0].ID); err != nil || item.Name != "Épisodes spéciaux" {
		t.Errorf("specials by identifier: %q %v", item.Name, err)
	}
}

func TestExpiredLinksAreRenewedWithTheSameFile(t *testing.T) {
	e := newEnv(t)
	movies := titles("movie", 1)
	stream := func(link string) stremio.Stream {
		return stremio.Stream{Name: "1080p", URL: link, BehaviorHints: stremio.StreamBehavior{Filename: "movie.mkv", VideoSize: 4_000_000_000}}
	}
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "a", Name: "A", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{"movie/top": movies},
		metas:    map[string]stremio.Meta{"movie/" + movies[0].ID: movies[0]},
		streams:  map[string][]stremio.Stream{"movie/" + movies[0].ID: {stream("https://cdn.example/old")}},
	}
	e.install(addons.Shared(), addon)
	libraries, _ := e.service.Libraries(t.Context(), e.member)
	page, err := e.service.Children(t.Context(), e.member, libraries[0].ID, 0, 10, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("listing: %+v %v", page, err)
	}
	versions, err := e.service.Versions(t.Context(), e.member, page.Items[0].ID)
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	old := versions[0]
	// The addon now gives another link to the same file, among others.
	addon.mu.Lock()
	addon.streams["movie/"+movies[0].ID] = []stremio.Stream{{Name: "720p", URL: "https://cdn.example/other",
		BehaviorHints: stremio.StreamBehavior{Filename: "other.mkv", VideoSize: 1_000}}, stream("https://cdn.example/new")}
	addon.mu.Unlock()
	renewed, err := e.service.Renew(t.Context(), old)
	if err != nil || renewed.ID != old.ID || renewed.URL != "https://cdn.example/new" {
		t.Fatalf("renewed: %+v %v", renewed, err)
	}
	if current, _ := e.service.Version(t.Context(), e.member, old.Item, old.ID); current.URL != renewed.URL {
		t.Errorf("the version still has the old link: %s", current.URL)
	}
	// A file the addon no longer lists cannot be renewed.
	addon.mu.Lock()
	addon.streams["movie/"+movies[0].ID] = nil
	addon.mu.Unlock()
	if _, err := e.service.Renew(t.Context(), old); !errors.Is(err, ErrNotFound) {
		t.Errorf("a file gone: %v", err)
	}
}
