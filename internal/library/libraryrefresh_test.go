package library

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// refreshAddon lists 30 movies, 10 a page, and describes them with
// overview, which a test changes; limited, when set, answers a title's
// description "too many requests" once.
type refreshAddon struct {
	*fakeAddon
	limited string
}

func newRefreshAddon(overview string) *refreshAddon {
	movies := titles("movie", 30)
	a := &refreshAddon{fakeAddon: &fakeAddon{
		manifest: stremio.Manifest{ID: "a", Name: "A", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta", IDPrefixes: []string{"tt"}}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top", Extra: []stremio.Extra{{Name: "skip"}}}}},
		catalogs: map[string][]stremio.Meta{"movie/top": movies},
		metas:    map[string]stremio.Meta{},
		pageSize: 10,
	}}
	a.describe(overview)
	return a
}

func (a *refreshAddon) describe(overview string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, movie := range a.catalogs["movie/top"] {
		movie.Description = overview
		a.metas["movie/"+movie.ID] = movie
	}
}

func (a *refreshAddon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	limited := a.limited != "" && strings.Contains(r.URL.Path, "/meta/movie/"+a.limited+".json")
	if limited {
		a.limited = ""
		a.requests = append(a.requests, strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), ".json"))
	}
	a.mu.Unlock()
	if limited {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	a.fakeAddon.ServeHTTP(w, r)
}

// metaRequests lists the descriptions asked for since the request at
// index from.
func (a *refreshAddon) metaRequests(from int) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var asked []string
	for _, request := range a.requests[from:] {
		if strings.HasPrefix(request, "meta/") {
			asked = append(asked, request)
		}
	}
	return asked
}

func (a *refreshAddon) asked() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.requests)
}

// pacer stands for the waits of a refresh: it records each, and holds it
// until the test releases it when hold is set.
type pacer struct {
	mu      sync.Mutex
	waits   []time.Duration
	hold    bool
	waiting chan struct{}
	release chan struct{}
}

func newPacer(hold bool) *pacer {
	return &pacer{hold: hold, waiting: make(chan struct{}, 100), release: make(chan struct{})}
}

func (p *pacer) wait(ctx context.Context, d time.Duration) error {
	p.mu.Lock()
	p.waits = append(p.waits, d)
	p.mu.Unlock()
	if !p.hold {
		return ctx.Err()
	}
	p.waiting <- struct{}{}
	select {
	case <-p.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *pacer) recorded() []time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.waits)
}

func newRefreshEnv(t *testing.T, addon *refreshAddon, p *pacer) (env, accounts.ID) {
	e := newEnv(t)
	e.service.refreshes.wait = p.wait
	server := httptest.NewServer(addon)
	t.Cleanup(server.Close)
	if _, err := e.addons.Install(t.Context(), addons.Shared(), server.URL+"/manifest.json", false); err != nil {
		t.Fatal(err)
	}
	return e, e.library(e.admin, "Top").ID
}

// refreshEnded waits for a library's refresh to end, and returns it.
func refreshEnded(t *testing.T, e env, id accounts.ID) LibraryRefresh {
	t.Helper()
	var state LibraryRefresh
	eventually(t, "the refresh's end", func() bool {
		state = e.service.LibraryRefreshes()[id]
		return state.State != RefreshQueued && state.State != RefreshRunning
	})
	return state
}

// A library's refresh describes again the titles of its catalog pages
// Polyfin keeps, one at a time, LibraryRefreshPace apart, and asks for no
// page, nor for the titles of pages no one read: the apps then show the
// new descriptions.
func TestLibraryRefreshesDescribeTheirKeptTitlesAtTheirPace(t *testing.T) {
	t.Parallel()
	addon := newRefreshAddon("Before.")
	p := newPacer(false)
	e, library := newRefreshEnv(t, addon, p)
	// Apps read the first two pages, and opened a title.
	e.children(e.member, "Top", 0, 20)
	first := e.children(e.member, "Top", 0, 1).Items[0]
	if item, err := e.service.Item(t.Context(), e.member, first.ID); err != nil || item.Overview != "Before." {
		t.Fatalf("before: %q %v", item.Overview, err)
	}
	addon.describe("After.")
	from := addon.asked()
	if _, err := e.service.RefreshLibrary(t.Context(), library); err != nil {
		t.Fatal(err)
	}
	state := refreshEnded(t, e, library)
	if state.State != RefreshDone || state.Titles != 20 || state.Done != 20 || state.Failed != 0 || state.Started.IsZero() || state.Ended.IsZero() {
		t.Errorf("the refresh: %+v", state)
	}
	asked := addon.metaRequests(from)
	want := make([]string, 20)
	for i := range want {
		want[i] = fmt.Sprintf("meta/movie/ttm%03d", i)
	}
	if !slices.Equal(asked, want) || addon.asked()-from != 20 {
		t.Errorf("asked %v, %d requests in all", asked, addon.asked()-from)
	}
	if waits := p.recorded(); len(waits) != 19 || slices.ContainsFunc(waits, func(d time.Duration) bool { return d != LibraryRefreshPace }) {
		t.Errorf("waits between titles: %v", waits)
	}
	if item, err := e.service.Item(t.Context(), e.member, first.ID); err != nil || item.Overview != "After." {
		t.Errorf("after: %q %v", item.Overview, err)
	}
}

// An addon that answers "too many requests" is waited for as long as it
// asks, then asked again; a refresh stopped asks nothing more, and the
// titles described stay so.
func TestLibraryRefreshesWaitOutRateLimitsAndStop(t *testing.T) {
	t.Parallel()
	addon := newRefreshAddon("Before.")
	addon.limited = "ttm001"
	p := newPacer(true)
	e, library := newRefreshEnv(t, addon, p)
	e.children(e.member, "Top", 0, 10)
	from := addon.asked()
	if _, err := e.service.RefreshLibrary(t.Context(), library); err != nil {
		t.Fatal(err)
	}
	// After the first title, the pace; after the second's 429, its wait.
	<-p.waiting
	p.release <- struct{}{}
	<-p.waiting
	if waits := p.recorded(); !slices.Equal(waits, []time.Duration{LibraryRefreshPace, 7 * time.Second}) {
		t.Errorf("waits: %v", waits)
	}
	p.release <- struct{}{}
	// The second title, asked again, answers; the refresh waits before the
	// third.
	<-p.waiting
	if !e.service.StopLibraryRefresh(library) {
		t.Fatal("no refresh to stop")
	}
	state := refreshEnded(t, e, library)
	if state.State != RefreshStopped || state.Titles != 10 || state.Done != 2 || state.Failed != 0 {
		t.Errorf("the refresh stopped: %+v", state)
	}
	if asked := addon.metaRequests(from); !slices.Equal(asked, []string{"meta/movie/ttm000", "meta/movie/ttm001", "meta/movie/ttm001"}) {
		t.Errorf("asked %v", asked)
	}
}

// A collection library's refresh describes again its collections first,
// by their addon, then the titles of the catalogs they group, one of each
// catalog in turn, from what Polyfin keeps alone: neither a collection nor
// a title never described, nor a collection no one opened. A collection
// library asked to refresh while another refreshes leaves out what that
// one read again since: a collection or title both list is asked once.
func TestCollectionLibraryRefreshesDescribeTheirKeptCollectionsAndTitles(t *testing.T) {
	t.Parallel()
	movie := func(id string) stremio.Meta {
		return stremio.Meta{ID: id, Type: "movie", Name: "Movie " + id, Description: "Before."}
	}
	first := []stremio.Meta{movie("tt1"), movie("tt2"), movie("tt3")}
	second := []stremio.Meta{movie("tt2"), movie("tt4"), movie("tt5")}
	set := func(id string, catalog string) stremio.Meta {
		return stremio.Meta{ID: id, Type: "collection", Name: "Set " + id,
			Collection: &stremio.Collection{Sources: []stremio.CollectionSource{{Type: "movie", CatalogID: catalog}}}}
	}
	metas := map[string]stremio.Meta{}
	for _, m := range slices.Concat(first, second, []stremio.Meta{movie("tt6")}) {
		metas["movie/"+m.ID] = m
	}
	for _, m := range []stremio.Meta{set("setone", "first"), set("settwo", "second"), set("setthree", "third")} {
		metas["collection/"+m.ID] = m
	}
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "sets", Name: "Sets", Version: "1", Types: []string{"movie", "collection"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs: []stremio.Catalog{
				{Type: "collection", ID: "sets", Name: "Sets"},
				{Type: "collection", ID: "picks", Name: "Picks"},
				{Type: "movie", ID: "first", Name: "First"},
				{Type: "movie", ID: "second", Name: "Second"},
				{Type: "movie", ID: "third", Name: "Third"},
			}},
		catalogs: map[string][]stremio.Meta{
			"collection/sets": {
				{ID: "setone", Type: "collection", Name: "Set setone"},
				{ID: "settwo", Type: "collection", Name: "Set settwo"},
				{ID: "setthree", Type: "collection", Name: "Set setthree"},
			},
			"collection/picks": {{ID: "settwo", Type: "collection", Name: "Set settwo"}},
			"movie/first":      first,
			"movie/second":     second,
			"movie/third":      {movie("tt6")},
		},
		metas: metas,
	}
	p := newPacer(true)
	e := newEnv(t)
	e.service.refreshes.wait = p.wait
	e.install(addons.Shared(), addon)
	e.chooseLibraries(libraryOf("collection", "sets", "", 0), libraryOf("collection", "picks", "", 0))
	sets, picks := e.library(e.admin, "Sets").ID, e.library(e.admin, "Picks").ID
	// Apps listed both libraries, opened the first two collections of Sets,
	// never the third, and three titles.
	e.children(e.member, "Picks", 0, 10)
	for _, collection := range e.children(e.member, "Sets", 0, 10).Items[:2] {
		if _, err := e.service.Children(t.Context(), e.member, collection.ID, 0, 10, ""); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"tt1", "tt2", "tt5"} {
		if item, err := e.service.Item(t.Context(), e.member, itemID(titleKey(KindMovie, id))); err != nil || item.Overview != "Before." {
			t.Fatalf("%s before: %q %v", id, item.Overview, err)
		}
	}
	addon.mu.Lock()
	for key, m := range addon.metas {
		m.Description = "After."
		addon.metas[key] = m
	}
	from := len(addon.requests)
	addon.mu.Unlock()

	if _, err := e.service.RefreshLibrary(t.Context(), sets); err != nil {
		t.Fatal(err)
	}
	// Sets described its first collection, and waits before the next:
	// Picks is asked for then.
	<-p.waiting
	if _, err := e.service.RefreshLibrary(t.Context(), picks); err != nil {
		t.Fatal(err)
	}
	close(p.release)
	state := refreshEnded(t, e, sets)
	if state.State != RefreshDone || state.Titles != 5 || state.Done != 5 || state.Failed != 0 {
		t.Errorf("the refresh of Sets: %+v", state)
	}
	state = refreshEnded(t, e, picks)
	if state.State != RefreshDone || state.Titles != 3 || state.Done != 3 || state.Failed != 0 {
		t.Errorf("the refresh of Picks: %+v", state)
	}
	addon.mu.Lock()
	asked := slices.Clone(addon.requests[from:])
	addon.mu.Unlock()
	want := []string{"meta/collection/setone", "meta/collection/settwo", "meta/movie/tt1", "meta/movie/tt2", "meta/movie/tt5"}
	if !slices.Equal(asked, want) {
		t.Errorf("asked %v, want %v", asked, want)
	}
	if waits := p.recorded(); len(waits) != 4 {
		t.Errorf("waits between titles: %v", waits)
	}
	if item, err := e.service.Item(t.Context(), e.member, itemID(titleKey(KindMovie, "tt5"))); err != nil || item.Overview != "After." {
		t.Errorf("tt5 after: %q %v", item.Overview, err)
	}
}
