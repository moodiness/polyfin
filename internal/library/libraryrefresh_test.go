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
