package library

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// pagedAddon serves a movie catalog, Top, of count titles in pages of size
// (one page when 0), and their descriptions, which tell more than the
// catalog does.
func pagedAddon(count, size int) *fakeAddon {
	movies := titles("movie", count)
	metas := map[string]stremio.Meta{}
	for _, movie := range movies {
		full := movie
		full.Description = "Full description of " + movie.Name
		full.Cast = []string{"Jane Doe"}
		metas["movie/"+movie.ID] = full
	}
	return &fakeAddon{
		manifest: stremio.Manifest{ID: "paged", Name: "Paged", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top", Extra: []stremio.Extra{{Name: "skip"}}}}},
		catalogs: map[string][]stremio.Meta{"movie/top": movies},
		metas:    metas,
		pageSize: size,
	}
}

// restarted is the environment after a restart of the server: a new
// service on the same database.
func (e env) restarted() env {
	e.t.Helper()
	service := New(e.service.db, e.addons, stremio.NewClient("test"), slog.New(slog.NewTextHandler(io.Discard, nil)), e.users.Settings)
	service.followUpDelays = nil
	service.readAhead = false
	service.now = e.service.now
	e.service = service
	return e
}

// requestsTo counts the requests addon received whose path starts with
// prefix.
func requestsTo(addon *fakeAddon, prefix string) int {
	addon.mu.Lock()
	defer addon.mu.Unlock()
	count := 0
	for _, request := range addon.requests {
		if strings.HasPrefix(request, prefix) {
			count++
		}
	}
	return count
}

// holdLimit bounds how long a held request waits for the others (see
// holdCatalogs): long, so that a slow machine never answers before the
// requests asked together arrived.
const holdLimit = 10 * time.Second

// catalogHold holds catalog requests until count of them arrived.
type catalogHold struct {
	count   int
	arrived int
	reached chan struct{}
}

// holdCatalogs holds the catalog requests addon receives from now on
// until count of them wait together, or holdLimit passed; requests that
// arrive afterwards answer at once. together reports whether count
// requests did wait together: they were asked at once, whatever the
// machine's speed.
func holdCatalogs(addon *fakeAddon, count int) (together func() bool) {
	hold := &catalogHold{count: count, reached: make(chan struct{})}
	addon.mu.Lock()
	addon.hold = hold
	addon.mu.Unlock()
	return func() bool {
		select {
		case <-hold.reached:
			return true
		default:
			return false
		}
	}
}

func eventually(t *testing.T, what string, done func() bool) {
	t.Helper()
	for deadline := time.Now().Add(holdLimit); !done(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%s never happened", what)
		}
	}
}

func TestNextPagesAreReadAhead(t *testing.T) {
	e := newEnv(t)
	e.service.readAhead = true
	addon := pagedAddon(100, 20)
	e.install(addons.Shared(), addon)
	if page := e.children(e.member, "Top", 0, 20); len(page.Items) != 20 || !page.More {
		t.Fatalf("first page: %v", names(page.Items))
	}
	eventually(t, "reading the next page ahead", func() bool { return requestsTo(addon, "catalog/movie/top/skip=20") == 1 })
	page := e.children(e.member, "Top", 20, 20)
	if len(page.Items) != 20 || page.Items[0].Name != "movie 20" {
		t.Fatalf("next page: %v", names(page.Items))
	}
	if got := requestsTo(addon, "catalog/movie/top/skip=20"); got != 1 {
		t.Errorf("the page read ahead was asked again: %d requests", got)
	}
	// The last page reads nothing ahead.
	eventually(t, "reading the page after ahead", func() bool { return requestsTo(addon, "catalog/movie/top/skip=40") == 1 })
	e.children(e.member, "Top", 80, 20)
	time.Sleep(100 * time.Millisecond)
	if got := requestsTo(addon, "catalog/movie/top/skip=100"); got > 1 {
		t.Errorf("past the end: %d requests", got)
	}
}

func TestReadsAskForTheMissingPagesAtOnce(t *testing.T) {
	e := newEnv(t)
	addon := pagedAddon(200, 10)
	e.install(addons.Shared(), addon)
	e.children(e.member, "Top", 0, 30)
	before := len(addon.catalogRequests())
	// 11 pages are needed, 3 of them kept: the 8 others are asked at once.
	together := holdCatalogs(addon, 8)
	page := e.children(e.member, "Top", 0, 110)
	if len(page.Items) != 110 || page.Items[109].Name != "movie 109" {
		t.Fatalf("listing: %d items", len(page.Items))
	}
	if requests := len(addon.catalogRequests()) - before; requests != 8 || !together() {
		t.Errorf("pages asked: %d, all at once: %t", requests, together())
	}

	// After a restart, the page size is remembered: a listing whose pages
	// are no longer kept asks for all of them at once, the first included.
	if _, err := e.service.db.Exec(t.Context(), "DELETE FROM catalog_pages"); err != nil {
		t.Fatal(err)
	}
	e = e.restarted()
	before = len(addon.catalogRequests())
	together = holdCatalogs(addon, 5)
	if page := e.children(e.member, "Top", 0, 50); len(page.Items) != 50 {
		t.Fatalf("listing after a restart: %d items", len(page.Items))
	}
	if requests := len(addon.catalogRequests()) - before; requests != 5 || !together() {
		t.Errorf("pages asked after a restart: %d, all at once: %t", requests, together())
	}
}

func TestLatestReadsTheFirstPageAndSmallCatalogsEndEarly(t *testing.T) {
	e := newEnv(t)
	addon := pagedAddon(3, 20)
	e.install(addons.Shared(), addon)
	top := e.library(e.member, "Top")
	latest, err := e.service.Latest(t.Context(), e.member, top.ID, 16)
	if err != nil || len(latest.Items) != 3 {
		t.Fatalf("latest: %v %v", names(latest.Items), err)
	}
	if got := addon.catalogRequests(); !slices.Equal(got, []string{"catalog/movie/top"}) {
		t.Errorf("latest asked %v", got)
	}
	// A first page this short may be the whole catalog: one page more is
	// asked, not as many as a page size would fill.
	if page := e.children(e.member, "Top", 0, 100); len(page.Items) != 3 || page.More {
		t.Fatalf("listing: %v more=%v", names(page.Items), page.More)
	}
	if got := addon.catalogRequests(); !slices.Equal(got, []string{"catalog/movie/top", "catalog/movie/top/skip=3"}) {
		t.Errorf("listing asked %v", got)
	}
	// Its end is known: nothing is asked past it.
	e.children(e.member, "Top", 0, 100)
	if got := len(addon.catalogRequests()); got != 2 {
		t.Errorf("listing again asked %d pages in all", got)
	}
}

func TestStalePagesShowWhileTheyAreRefreshed(t *testing.T) {
	e := newLifetimeEnv(t)
	e.setting(func(s *accounts.Settings) { s.CatalogRefreshMinutes = 60 })
	listing := func() []string {
		t.Helper()
		page, err := e.service.Children(t.Context(), e.member, e.library(e.member, "Top").ID, 0, 10, "")
		if err != nil {
			t.Fatal(err)
		}
		return names(page.Items)
	}
	listing()
	gate := make(chan struct{})
	e.addon.mu.Lock()
	e.addon.catalogGate = gate
	e.addon.catalogs["movie/top"] = titles("movie", 2)
	e.addon.mu.Unlock()

	// Past the refresh age, the page kept shows at once while the addon,
	// which does not answer yet, is asked again.
	e.wait(61 * time.Minute)
	if got := listing(); len(got) != 1 {
		t.Errorf("stale page: %v", got)
	}
	eventually(t, "asking the addon again", func() bool { return e.requests("catalog/") == 2 })
	if got := listing(); len(got) != 1 {
		t.Errorf("stale page while refreshed: %v", got)
	}
	close(gate)
	eventually(t, "showing the refreshed page", func() bool { return len(listing()) == 2 })
	if got := e.requests("catalog/"); got != 2 {
		t.Errorf("refreshing asked %d times in all", got)
	}

	// A page fails to refresh: the stale one stays.
	e.addon.mu.Lock()
	e.addon.catalogs["movie/top"] = nil
	e.addon.mu.Unlock()
	e.wait(23 * time.Hour)
	if got := listing(); len(got) != 2 {
		t.Errorf("23 hours later: %v", got)
	}
	// A day past the refresh age, the page is no longer kept: the listing
	// waits for the addon.
	e.wait(2 * time.Hour)
	if got := listing(); len(got) != 0 {
		t.Errorf("a day past the refresh age: %v", got)
	}
}

func TestCatalogPagesAreRefreshedAfterTheirSetAge(t *testing.T) {
	e := newLifetimeEnv(t)
	e.setting(func(s *accounts.Settings) { s.CatalogRefreshMinutes = 10 })
	e.list()
	if got := e.requests("catalog/"); got != 1 {
		t.Fatalf("first listing: %d catalog requests", got)
	}
	e.wait(9 * time.Minute)
	e.list()
	if got := e.requests("catalog/"); got != 1 {
		t.Errorf("9 minutes later, refreshed after 10: %d catalog requests", got)
	}
	e.wait(2 * time.Minute)
	refreshed(t, e, "refreshing after 11 minutes", 2)

	e.setting(func(s *accounts.Settings) { s.CatalogRefreshMinutes = 24 * 60 })
	e.wait(23 * time.Hour)
	e.list()
	time.Sleep(50 * time.Millisecond)
	if got := e.requests("catalog/"); got != 2 {
		t.Errorf("23 hours later, refreshed daily: %d catalog requests", got)
	}
	e.wait(2 * time.Hour)
	// The addon answers the refresh 3 minutes after it was asked: the page
	// is as old as the request, and a shorter age applies at once, to the
	// page kept too.
	gate := make(chan struct{})
	e.addon.mu.Lock()
	e.addon.catalogGate = gate
	e.addon.mu.Unlock()
	refreshed(t, e, "refreshing after 25 hours", 3)
	e.wait(3 * time.Minute)
	close(gate)
	e.setting(func(s *accounts.Settings) { s.CatalogRefreshMinutes = 2 })
	refreshed(t, e, "refreshing after 3 minutes, every 2", 4)
	e.wait(time.Minute)
	e.list()
	time.Sleep(50 * time.Millisecond)
	if got := e.requests("catalog/"); got != 4 {
		t.Errorf("1 minute later, refreshed every 2 minutes: %d catalog requests", got)
	}
}

// refreshed lists the Top library until the addon was asked requests
// times in all. A read past the age starts a refresh unless one is under
// way, and the previous refresh may still be storing what it fetched: the
// read joins it then, so the library is read again until its own starts.
// When it does not come, it tells every request the addon had and what
// the service logged.
func refreshed(t *testing.T, e lifetimeEnv, what string, requests int) {
	t.Helper()
	for deadline := time.Now().Add(holdLimit); ; time.Sleep(10 * time.Millisecond) {
		e.list()
		got := e.requests("catalog/")
		if got == requests {
			return
		}
		if got > requests || time.Now().After(deadline) {
			e.addon.mu.Lock()
			asked := slices.Clone(e.addon.requests)
			e.addon.mu.Unlock()
			t.Fatalf("%s: %d catalog requests, want %d; now %v, catalog life %v; requests %q; log:\n%s",
				what, got, requests, e.service.now().Sub(time.Now()).Round(time.Second), e.service.catalogLife(), asked, e.log.String())
		}
	}
}

func TestSearchesAreKeptBrieflyWhateverTheCatalogLife(t *testing.T) {
	e := newEnv(t)
	elapsed := new(atomic.Int64)
	e.service.now = func() time.Time { return time.Now().Add(time.Duration(elapsed.Load())) }
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "a", Name: "A", Version: "1", Types: []string{"movie"}, Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top", Extra: []stremio.Extra{{Name: "search"}}}}},
		catalogs: map[string][]stremio.Meta{"movie/top": titles("movie", 12)},
	}
	e.install(addons.Shared(), addon)
	e.setting(func(s *accounts.Settings) { s.CatalogRefreshMinutes = 60 })
	search := func() {
		t.Helper()
		if found, err := e.service.Search(t.Context(), e.member, "movie 1", []Kind{KindMovie}, 10); err != nil || len(found) != 3 {
			t.Fatalf("search: %v %v", names(found), err)
		}
	}
	searches := func() int { return requestsTo(addon, "catalog/movie/top/search=") }
	search()
	elapsed.Add(int64(9 * time.Minute))
	search()
	if got := searches(); got != 1 {
		t.Errorf("9 minutes later: %d search requests", got)
	}
	// Searches are kept ten minutes, though catalog pages are kept longer,
	// and they are never shown stale.
	elapsed.Add(int64(2 * time.Minute))
	search()
	if got := searches(); got != 2 {
		t.Errorf("11 minutes later: %d search requests", got)
	}
	var kept int
	if err := e.service.db.QueryRow(t.Context(), "SELECT count(*) FROM catalog_pages").Scan(&kept); err != nil || kept != 0 {
		t.Errorf("searches kept in the database: %d %v", kept, err)
	}
}

func TestKeptPagesAndDescriptionsSurviveARestart(t *testing.T) {
	e := newEnv(t)
	addon := pagedAddon(30, 10)
	e.install(addons.Shared(), addon)
	page := e.children(e.member, "Top", 0, 30)
	item, err := e.service.Item(t.Context(), e.member, page.Items[0].ID)
	if err != nil || item.Overview != "Full description of movie 0" {
		t.Fatalf("item: %+v %v", item, err)
	}
	before := len(addon.requests)

	e = e.restarted()
	after := e.children(e.member, "Top", 0, 30)
	if !slices.Equal(names(after.Items), names(page.Items)) {
		t.Errorf("listing after a restart: %v", names(after.Items))
	}
	item, err = e.service.Item(t.Context(), e.member, page.Items[0].ID)
	if err != nil || item.Overview != "Full description of movie 0" {
		t.Errorf("item after a restart: %+v %v", item, err)
	}
	if got := len(addon.requests) - before; got != 0 {
		t.Errorf("after a restart, the addon was asked %d times: %v", got, addon.requests[before:])
	}

	// An addon given another address, another configuration, answers anew:
	// what was kept for the old one is not served.
	moved := httptest.NewServer(addon)
	t.Cleanup(moved.Close)
	if _, err := e.service.db.Exec(t.Context(), "UPDATE addons SET manifest_url = $1", moved.URL+"/manifest.json"); err != nil {
		t.Fatal(err)
	}
	e = e.restarted()
	catalogs, metas := len(addon.catalogRequests()), addon.metaRequests()
	e.children(e.member, "Top", 0, 10)
	if _, err := e.service.Item(t.Context(), e.member, page.Items[0].ID); err != nil {
		t.Fatal(err)
	}
	if got := len(addon.catalogRequests()) - catalogs; got != 1 {
		t.Errorf("another address read %d catalog pages", got)
	}
	if got := addon.metaRequests() - metas; got != 1 {
		t.Errorf("another address read %d descriptions", got)
	}
}

func TestDescriptionsShowWhileTheyAreRefreshed(t *testing.T) {
	e := newLifetimeEnv(t)
	movie := e.list().ID
	describe := func() string {
		t.Helper()
		item, err := e.service.Item(t.Context(), e.member, movie)
		if err != nil {
			t.Fatal(err)
		}
		return item.Overview
	}
	describe()
	if got := e.requests("meta/"); got != 1 {
		t.Fatalf("first description: %d requests", got)
	}
	e.wait(5 * time.Hour)
	describe()
	if got := e.requests("meta/"); got != 1 {
		t.Errorf("5 hours later: %d requests", got)
	}
	gate := make(chan struct{})
	e.addon.mu.Lock()
	e.addon.metaGate = gate
	meta := e.addon.metas["movie/"+titles("movie", 1)[0].ID]
	meta.Description = "Rewritten"
	e.addon.metas["movie/"+meta.ID] = meta
	e.addon.mu.Unlock()
	// Past 6 hours, the description kept shows at once while the addon is
	// asked again.
	e.wait(2 * time.Hour)
	if got := describe(); got == "Rewritten" {
		t.Errorf("7 hours later: %q", got)
	}
	eventually(t, "asking the addon again", func() bool { return e.requests("meta/") == 2 })
	close(gate)
	eventually(t, "showing the new description", func() bool { return describe() == "Rewritten" })
}

func TestWarmReadsEachLibrarysFirstPage(t *testing.T) {
	e := newEnv(t)
	addon := pagedAddon(30, 10)
	addon.manifest.Catalogs = append(addon.manifest.Catalogs, stremio.Catalog{Type: "movie", ID: "new", Name: "New"})
	addon.catalogs["movie/new"] = titles("movie", 5)
	e.install(addons.Shared(), addon)
	// Pages no one read for long are forgotten.
	list, _ := e.addons.Addons(t.Context(), addons.Shared())
	if _, err := e.service.db.Exec(t.Context(), `INSERT INTO catalog_pages (addon_id, catalog_type, catalog_id, genre, skip, config, metas, fetched_at)
		VALUES ($1, 'movie', 'old', '', 0, '', '[]', now() - interval '3 days')`, list[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Warm(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := addon.catalogRequests(); len(got) != 2 || !slices.Contains(got, "catalog/movie/top") || !slices.Contains(got, "catalog/movie/new") {
		t.Errorf("warming asked %v", got)
	}
	var old int
	if err := e.service.db.QueryRow(t.Context(), "SELECT count(*) FROM catalog_pages WHERE catalog_id = 'old'").Scan(&old); err != nil || old != 0 {
		t.Errorf("old pages kept: %d %v", old, err)
	}
	// Home rows then answer without asking.
	e = e.restarted()
	for _, name := range []string{"Top", "New"} {
		if _, err := e.service.Latest(t.Context(), e.member, e.library(e.member, name).ID, 16); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(addon.catalogRequests()); got != 2 {
		t.Errorf("home rows after warming asked %d pages in all", got)
	}
}

// searchAddons serves a title search, paged, and a slow people search.
func searchAddons(e env) (titleSearch, peopleSearch *fakeAddon) {
	search := []stremio.Extra{{Name: "search", IsRequired: true}, {Name: "skip"}}
	titleSearch = &fakeAddon{
		manifest: stremio.Manifest{ID: "titles", Name: "Titles", Version: "1", Types: []string{"movie"}, Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{{Type: "movie", ID: "search", Name: "Search", Extra: search}}},
		catalogs: map[string][]stremio.Meta{"movie/search": titles("movie", 30)},
		pageSize: 10,
	}
	people := titles("movie", 40)[35:]
	peopleSearch = &fakeAddon{
		manifest: stremio.Manifest{ID: "people", Name: "People", Version: "1", Types: []string{"movie"}, Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{{Type: "movie", ID: "people_search", Name: "People Search", Extra: search[:1]}}},
		catalogs:    map[string][]stremio.Meta{"movie/people_search": people},
		catalogGate: make(chan struct{}),
	}
	e.install(addons.Shared(), titleSearch)
	e.install(addons.Shared(), peopleSearch)
	return titleSearch, peopleSearch
}

func TestSearchesReadFirstPagesAndWaitBrieflyForPeople(t *testing.T) {
	e := newEnv(t)
	titleSearch, peopleSearch := searchAddons(e)
	started := time.Now()
	found, err := e.service.Search(t.Context(), e.member, "movie", []Kind{KindMovie}, 800)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Errorf("the search waited %v for the people search", elapsed)
	}
	if len(found) != 10 || found[0].Name != "movie 0" {
		t.Errorf("found %v", names(found))
	}
	if got := titleSearch.catalogRequests(); !slices.Equal(got, []string{"catalog/movie/search/search=movie"}) {
		t.Errorf("title search asked %v", got)
	}
	// The people search answers late: its titles come with the next search.
	close(peopleSearch.catalogGate)
	eventually(t, "the people search's titles", func() bool {
		found, _ := e.service.Search(t.Context(), e.member, "movie", []Kind{KindMovie}, 800)
		return slices.Contains(names(found), "movie 35")
	})
	if got := len(peopleSearch.catalogRequests()); got != 1 {
		t.Errorf("people search asked %d times", got)
	}
	// A search of people search catalogs only waits for them.
	if _, err := e.addons.SetEnabled(t.Context(), addons.Shared(), e.addonID("titles"), false); err != nil {
		t.Fatal(err)
	}
	found, _ = e.service.Search(t.Context(), e.member, "movie 3", []Kind{KindMovie}, 10)
	if len(found) == 0 {
		t.Errorf("people search only: %v", names(found))
	}
}

// addonID returns the identifier of the server's addon whose manifest has
// id.
func (e env) addonID(id string) accounts.ID {
	e.t.Helper()
	list, err := e.addons.Addons(e.t.Context(), addons.Shared())
	if err != nil {
		e.t.Fatal(err)
	}
	for _, addon := range list {
		if addon.Manifest.ID == id {
			return addon.ID
		}
	}
	e.t.Fatalf("no addon %q", id)
	return accounts.ID{}
}

func TestAnAppGivingUpDoesNotFailOthers(t *testing.T) {
	e := newEnv(t)
	addon := pagedAddon(5, 0)
	e.install(addons.Shared(), addon)
	top := e.library(e.member, "Top")
	gate := make(chan struct{})
	addon.mu.Lock()
	addon.catalogGate = gate
	addon.mu.Unlock()
	leaving, leave := context.WithCancel(t.Context())
	left := make(chan error, 1)
	go func() {
		_, err := e.service.Children(leaving, e.member, top.ID, 0, 5, "")
		left <- err
	}()
	eventually(t, "asking the addon", func() bool { return len(addon.catalogRequests()) == 1 })
	staying := make(chan Page, 1)
	go func() {
		page, err := e.service.Children(t.Context(), e.member, top.ID, 0, 5, "")
		if err != nil {
			t.Errorf("the app still waiting failed: %v", err)
		}
		staying <- page
	}()
	time.Sleep(50 * time.Millisecond)
	leave()
	if err := <-left; !errors.Is(err, context.Canceled) {
		t.Errorf("the app that left: %v", err)
	}
	close(gate)
	if page := <-staying; len(page.Items) != 5 {
		t.Errorf("the app still waiting: %v", names(page.Items))
	}
	// The page read for both is kept.
	e.children(e.member, "Top", 0, 5)
	if got := len(addon.catalogRequests()); got != 1 {
		t.Errorf("the addon was asked %d times", got)
	}

	// So is a description an app gave up on.
	movie := e.children(e.member, "Top", 0, 1).Items[0]
	addon.mu.Lock()
	addon.metaGate = make(chan struct{})
	addon.mu.Unlock()
	leaving, leave = context.WithCancel(t.Context())
	go func() {
		_, _ = e.service.Item(leaving, e.member, movie.ID)
	}()
	eventually(t, "asking for the description", func() bool { return addon.metaRequests() == 1 })
	leave()
	close(addon.metaGate)
	eventually(t, "keeping the description", func() bool {
		item, err := e.service.Item(t.Context(), e.member, movie.ID)
		return err == nil && item.Overview != ""
	})
	if got := addon.metaRequests(); got != 1 {
		t.Errorf("the description was asked %d times", got)
	}
}

func TestColdCollectionsReadTheirCatalogsInOneWave(t *testing.T) {
	e := newEnv(t)
	skip := []stremio.Extra{{Name: "skip"}}
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "sets", Name: "Sets", Version: "1", Types: []string{"movie", "collection"}, IDPrefixes: []string{"tt", "set"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs: []stremio.Catalog{{Type: "collection", ID: "sets", Name: "Sets", Extra: skip},
				{Type: "movie", ID: "one", Name: "One", Extra: skip}, {Type: "movie", ID: "two", Name: "Two", Extra: skip}}},
		catalogs: map[string][]stremio.Meta{
			"collection/sets": {{ID: "set1", Type: "collection", Name: "Set"}},
			"movie/one":       titles("movie", 50),
			"movie/two":       titles("series", 50),
		},
		metas: map[string]stremio.Meta{"collection/set1": {ID: "set1", Type: "collection", Name: "Set", Collection: &stremio.Collection{
			Sources: []stremio.CollectionSource{{Type: "movie", CatalogID: "one"}, {Type: "movie", CatalogID: "two"}}}}},
		pageSize: 10,
	}
	for i, meta := range addon.catalogs["movie/two"] {
		meta.Type = "movie"
		addon.catalogs["movie/two"][i] = meta
	}
	e.install(addons.Shared(), addon)
	set := e.children(e.member, "Sets", 0, 10).Items[0]
	if page, err := e.service.Children(t.Context(), e.member, set.ID, 0, 40, ""); err != nil || len(page.Items) != 40 {
		t.Fatalf("collection: %d items, %v", len(page.Items), err)
	}

	// After a restart, with the pages forgotten, the collection's
	// description and its catalogs' page sizes are known: every page
	// needed is asked at once.
	if _, err := e.service.db.Exec(t.Context(), "DELETE FROM catalog_pages"); err != nil {
		t.Fatal(err)
	}
	e = e.restarted()
	addon.mu.Lock()
	addon.requests = nil
	addon.mu.Unlock()
	// Each of the two catalogs is read for 26 titles: 3 pages of 10.
	together := holdCatalogs(addon, 6)
	page, err := e.service.Children(t.Context(), e.member, set.ID, 0, 40, "")
	if err != nil || len(page.Items) != 40 {
		t.Fatalf("collection after a restart: %d items, %v", len(page.Items), err)
	}
	if requests := len(addon.catalogRequests()); requests != 6 || !together() || addon.metaRequests() != 0 {
		t.Errorf("after a restart: %d pages, all at once: %t, %d descriptions", requests, together(), addon.metaRequests())
	}
}

func TestARequestBuildsTheViewOnce(t *testing.T) {
	e := newEnv(t)
	e.install(addons.Shared(), pagedAddon(3, 0))
	ctx := PerRequest(t.Context())
	if libraries, err := e.service.Libraries(ctx, e.member); err != nil || len(libraries) != 1 {
		t.Fatalf("libraries: %v %v", names(libraries), err)
	}
	other := pagedAddon(3, 0)
	other.manifest.ID, other.manifest.Catalogs[0].ID, other.manifest.Catalogs[0].Name = "other", "other", "Other"
	e.install(addons.Shared(), other)
	if libraries, _ := e.service.Libraries(ctx, e.member); len(libraries) != 1 {
		t.Errorf("the same request: %v", names(libraries))
	}
	if libraries, _ := e.service.Libraries(t.Context(), e.member); len(libraries) != 2 {
		t.Errorf("another request: %v", names(libraries))
	}
}

func TestItemsOfOtherKindsAreNotDescribed(t *testing.T) {
	e := newEnv(t)
	addon := pagedAddon(5, 0)
	e.install(addons.Shared(), addon)
	page := e.children(e.member, "Top", 0, 5)
	ids := make([]accounts.ID, 0, len(page.Items))
	for _, item := range page.Items {
		ids = append(ids, item.ID)
	}
	if items, err := e.service.ItemsOf(t.Context(), e.member, ids, []Kind{KindEpisode}); err != nil || len(items) != 0 {
		t.Errorf("episodes: %v %v", names(items), err)
	}
	if got := addon.metaRequests(); got != 0 {
		t.Errorf("movies described for episodes: %d", got)
	}
	items, err := e.service.ItemsOf(t.Context(), e.member, slices.Concat(ids[3:], ids[:3]), []Kind{KindMovie})
	if err != nil || !slices.Equal(names(items), []string{"movie 3", "movie 4", "movie 0", "movie 1", "movie 2"}) || items[0].Overview == "" {
		t.Errorf("movies: %v %v", names(items), err)
	}
}

func TestCreditsAreRecordedWhenADescriptionIsFetched(t *testing.T) {
	e := newEnv(t)
	addon := pagedAddon(1, 0)
	e.install(addons.Shared(), addon)
	movie := e.children(e.member, "Top", 0, 1).Items[0]
	person := PersonID("Jane Doe")
	if _, err := e.service.Item(t.Context(), e.member, movie.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.load(t.Context(), person); err != nil {
		t.Fatalf("person after the description: %v", err)
	}
	if _, err := e.service.db.Exec(t.Context(), "DELETE FROM items WHERE id = $1", person); err != nil {
		t.Fatal(err)
	}
	// Reading the kept description writes nothing.
	if _, err := e.service.Item(t.Context(), e.member, movie.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.load(t.Context(), person); !errors.Is(err, ErrNotFound) {
		t.Errorf("person written again on a read: %v", err)
	}
	// Fetching it again records them again.
	if err := e.service.Refresh(t.Context(), e.member, movie.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.load(t.Context(), person); err != nil {
		t.Errorf("person after a refresh: %v", err)
	}
}

func TestAutomaticImagesNeverWait(t *testing.T) {
	e := newEnv(t)
	addon := pagedAddon(3, 0)
	for i := range addon.catalogs["movie/top"] {
		addon.catalogs["movie/top"][i].Background = "https://images.example/backdrop.jpg"
	}
	e.install(addons.Shared(), addon)
	key := addons.LibraryKey{AddonID: e.addonID("paged"), CatalogType: "movie", CatalogID: "top"}
	if err := e.addons.SetLibraryImage(t.Context(), addons.Shared(), key, addons.LibraryImageAutomatic); err != nil {
		t.Fatal(err)
	}
	gate := make(chan struct{})
	addon.mu.Lock()
	addon.catalogGate = gate
	addon.mu.Unlock()
	image := func() string {
		t.Helper()
		started := time.Now()
		libraries, err := e.service.Libraries(t.Context(), e.member)
		if err != nil {
			t.Fatal(err)
		}
		if elapsed := time.Since(started); elapsed > time.Second {
			t.Errorf("the libraries waited %v", elapsed)
		}
		return libraries[0].Images.Primary
	}
	if got := image(); got != "" {
		t.Errorf("an image not found yet: %q", got)
	}
	close(gate)
	eventually(t, "finding the image", func() bool { return image() == "https://images.example/backdrop.jpg" })

	// After a restart, with nothing kept of the catalog, the image found
	// last shows while it is looked up again.
	if _, err := e.service.db.Exec(t.Context(), "DELETE FROM catalog_pages"); err != nil {
		t.Fatal(err)
	}
	e = e.restarted()
	addon.mu.Lock()
	addon.catalogGate = make(chan struct{})
	addon.mu.Unlock()
	if got := image(); got != "https://images.example/backdrop.jpg" {
		t.Errorf("after a restart: %q", got)
	}
	close(addon.catalogGate)
}

func TestKnownPagesAskNothing(t *testing.T) {
	e := newEnv(t)
	addon := pagedAddon(30, 10)
	e.install(addons.Shared(), addon)
	pages, err := e.service.KnownPages(t.Context(), e.member, 100)
	if err != nil || len(pages) != 1 || len(pages[0].Items) != 0 || len(addon.catalogRequests()) != 0 {
		t.Fatalf("nothing kept: %+v %v, %d requests", pages, err, len(addon.catalogRequests()))
	}
	e.children(e.member, "Top", 0, 10)
	before := len(addon.catalogRequests())
	pages, _ = e.service.KnownPages(t.Context(), e.member, 100)
	if len(pages[0].Items) != 10 || !pages[0].More || len(addon.catalogRequests()) != before {
		t.Errorf("first page kept: %d items, more=%v, %d requests", len(pages[0].Items), pages[0].More, len(addon.catalogRequests())-before)
	}
}
