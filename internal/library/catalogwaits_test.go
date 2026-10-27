package library

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// askingAddon serves fakeAddon, and answers its catalog pages as a test
// asks: 429 with Retry-After while refusing, 500 for the pages whose path
// holds failing. While gate is set, catalog requests wait until it is
// closed; while release is set, each takes one value of it first. It
// counts the catalog requests, and the most under way at once.
type askingAddon struct {
	*fakeAddon
	mu         sync.Mutex
	refusing   bool
	retryAfter string
	failing    string
	gate       chan struct{}
	release    chan struct{}
	asked      int
	underWay   int
	most       int
}

func (a *askingAddon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/catalog/") {
		a.fakeAddon.ServeHTTP(w, r)
		return
	}
	a.mu.Lock()
	a.asked++
	a.underWay++
	a.most = max(a.most, a.underWay)
	refusing, after, gate, release := a.refusing, a.retryAfter, a.gate, a.release
	failing := a.failing != "" && strings.Contains(r.URL.Path, a.failing)
	a.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-r.Context().Done():
		}
	}
	if release != nil {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}
	// The request is no longer under way once it is answered: counted
	// out first, as its answer frees a place for the next one.
	a.mu.Lock()
	a.underWay--
	a.mu.Unlock()
	switch {
	case refusing:
		w.Header().Set("Retry-After", after)
		w.WriteHeader(http.StatusTooManyRequests)
	case failing:
		http.Error(w, "failing", http.StatusInternalServerError)
	default:
		a.fakeAddon.ServeHTTP(w, r)
	}
}

// counts returns the catalog requests asked, those under way, and the most
// under way at once.
func (a *askingAddon) counts() (asked, underWay, most int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.asked, a.underWay, a.most
}

// askingEnv is an environment whose shared addon is an askingAddon, on a
// clock the test moves, logging into log.
type askingEnv struct {
	env
	addon    *askingAddon
	manifest string
	elapsed  *atomic.Int64
	log      *lockedBuffer
}

func newAskingEnv(t *testing.T, addon *fakeAddon) askingEnv {
	t.Helper()
	e := newEnv(t)
	elapsed := new(atomic.Int64)
	e.service.now = func() time.Time { return time.Now().Add(time.Duration(elapsed.Load())) }
	log := &lockedBuffer{}
	e.service.logger = slog.New(slog.NewTextHandler(log, &slog.HandlerOptions{Level: slog.LevelDebug}))
	asking := &askingAddon{fakeAddon: addon}
	server := httptest.NewServer(asking)
	t.Cleanup(server.Close)
	manifest := server.URL + "/manifest.json"
	if _, err := e.addons.Install(t.Context(), addons.Shared(), manifest, false); err != nil {
		t.Fatal(err)
	}
	// The libraries find no image of their own: an image looked up as they
	// are listed would read their pages too.
	for _, catalog := range addon.manifest.Catalogs {
		key := addons.LibraryKey{AddonID: e.addonID(addon.manifest.ID), CatalogType: catalog.Type, CatalogID: catalog.ID}
		if err := e.addons.SetLibraryImage(t.Context(), addons.Shared(), key, addons.LibraryImageNone); err != nil {
			t.Fatal(err)
		}
	}
	return askingEnv{env: e, addon: asking, manifest: manifest, elapsed: elapsed, log: log}
}

func (e askingEnv) wait(d time.Duration) { e.elapsed.Add(int64(d)) }

// settled waits for the refreshes under way of the pages of the catalog
// movie/id from 0 to below end, by size, to end.
func (e askingEnv) settled(id string, end, size int) {
	addon := e.addonID(e.addon.manifest.ID)
	for skip := 0; skip < end; skip += size {
		key := pageKey{addon: addon, catalogType: "movie", catalogID: id, skip: skip}
		_, _, _ = e.service.flight.Do(fmt.Sprintf("refresh page %v", key), func() (any, error) { return nil, nil })
	}
}

// An addon that answers 429 with Retry-After is asked for no catalog page
// until the wait ends: the refreshes of a listing's stale pages reach it
// a few at a time, those still waiting for a place then ask nothing, the
// listings meanwhile serve the pages kept, and a page never read fails at
// once. The log tells the wait once, and Health tells it. Once the wait
// ends, the pages whose refresh failed are refreshed again a minute later.
func TestAnAddonAskingToWaitIsAskedForNoCatalogPageUntilTheWaitEnds(t *testing.T) {
	t.Parallel()
	e := newAskingEnv(t, pagedAddon(150, 10))
	e.setting(func(s *accounts.Settings) { s.CatalogRefreshMinutes = 1 })
	top := e.library(e.member, "Top").ID
	list := func(start, count int) (Page, error) {
		return e.service.Children(t.Context(), e.member, top, start, count, "")
	}
	served := func(what string) {
		t.Helper()
		if page, err := list(0, 100); err != nil || len(page.Items) != 100 || page.Items[99].Name != "movie 99" {
			t.Fatalf("%s: %d items, %v", what, len(page.Items), err)
		}
	}
	served("first listing")

	// The 10 pages are past their refresh age, and the addon refuses every
	// page, asking to wait 10 seconds. The listing starts a refresh for
	// each page: they are held at the addon until 8 are under way.
	e.wait(2 * time.Minute)
	gate := make(chan struct{})
	e.addon.mu.Lock()
	e.addon.refusing, e.addon.retryAfter, e.addon.gate = true, "10", gate
	e.addon.mu.Unlock()
	before, _, _ := e.addon.counts()
	served("stale pages")
	eventually(t, "8 refreshes under way", func() bool { _, underWay, _ := e.addon.counts(); return underWay >= 8 })
	close(gate)
	e.settled("top", 100, 10)
	if asked, _, most := e.addon.counts(); asked-before != 8 || most > 8 {
		t.Errorf("refreshes asked: %d, at most %d at once; want 8, 8 at most", asked-before, most)
	}

	// During the wait, listings serve the pages kept and ask nothing; a
	// page never read fails at once.
	for range 3 {
		served("during the wait")
		e.settled("top", 100, 10)
	}
	if _, err := list(100, 50); !errors.Is(err, stremio.ErrAskedToWait) {
		t.Errorf("a page never read during the wait: %v", err)
	}
	if asked, _, _ := e.addon.counts(); asked-before != 8 {
		t.Errorf("asked during the wait: %d in all", asked-before)
	}
	if lines := strings.Count(e.log.String(), "An addon asked to wait"); lines != 1 || !strings.Contains(e.log.String(), "skip=") {
		t.Errorf("the wait told %d times:\n%s", lines, e.log.String())
	}
	if health, _ := e.service.client.Health(e.manifest); health.Failure != stremio.FailureRateLimited || health.CatalogsWaitUntil.IsZero() {
		t.Errorf("health during the wait: %+v", health)
	}

	// Past the wait, the pages whose refresh failed a moment ago are not
	// asked again yet; a minute later, they are.
	e.addon.mu.Lock()
	e.addon.refusing, e.addon.gate = false, nil
	e.addon.mu.Unlock()
	e.wait(11 * time.Second)
	served("past the wait")
	e.settled("top", 100, 10)
	if asked, _, _ := e.addon.counts(); asked-before != 8 {
		t.Errorf("asked again at once: %d in all", asked-before)
	}
	e.wait(time.Minute)
	served("a minute later")
	e.settled("top", 100, 10)
	if asked, _, _ := e.addon.counts(); asked-before != 18 {
		t.Errorf("refreshed a minute later: %d in all, want 18", asked-before)
	}
}

// An addon that answers 429 without Retry-After waits catalogWaitFirst, and
// twice as long at its next refusal after a wait.
func TestAWaitWithoutRetryAfterDoubles(t *testing.T) {
	t.Parallel()
	e := newAskingEnv(t, pagedAddon(30, 10))
	top := e.library(e.member, "Top").ID
	e.addon.mu.Lock()
	e.addon.refusing = true
	e.addon.mu.Unlock()
	list := func() error {
		_, err := e.service.Children(t.Context(), e.member, top, 0, 10, "")
		return err
	}
	if err := list(); stremio.StatusOf(err) != http.StatusTooManyRequests {
		t.Fatalf("first refusal: %v", err)
	}
	err := list()
	if wait, ok := stremio.RetryAfter(err); !errors.Is(err, stremio.ErrAskedToWait) || !ok || wait > catalogWaitFirst {
		t.Fatalf("during the first wait: %v", err)
	}
	e.wait(catalogWaitFirst)
	if err := list(); stremio.StatusOf(err) != http.StatusTooManyRequests {
		t.Fatalf("second refusal: %v", err)
	}
	// The second wait lasts twice as long: it runs on past the first's.
	e.wait(catalogWaitFirst)
	if err := list(); !errors.Is(err, stremio.ErrAskedToWait) {
		t.Errorf("%v into the second wait: %v", catalogWaitFirst, err)
	}
	if asked, _, _ := e.addon.counts(); asked != 2 {
		t.Errorf("asked %d times", asked)
	}
}

// The catalog pages asked of one addon at once are bounded, whatever the
// listings asking for them: two listings of 8 pages each, all needed at
// once, reach it 8 at a time, and both list every title.
func TestCatalogPagesAskedOfOneAddonAtOnceAreBounded(t *testing.T) {
	t.Parallel()
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "two", Name: "Two", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top", Extra: []stremio.Extra{{Name: "skip"}}},
				{Type: "movie", ID: "fresh", Name: "Fresh", Extra: []stremio.Extra{{Name: "skip"}}}}},
		catalogs: map[string][]stremio.Meta{"movie/top": titles("movie", 100), "movie/fresh": titles("movie", 100)},
		pageSize: 10,
	}
	e := newAskingEnv(t, addon)
	libraries := []accounts.ID{e.library(e.member, "Top").ID, e.library(e.member, "Fresh").ID}
	// Two pages of each teach the page size: the next listings ask for
	// the 8 pages they need at once.
	for _, library := range libraries {
		if page, err := e.service.Children(t.Context(), e.member, library, 0, 20, ""); err != nil || len(page.Items) != 20 {
			t.Fatalf("first listing: %d items, %v", len(page.Items), err)
		}
	}
	release := make(chan struct{})
	e.addon.mu.Lock()
	e.addon.release = release
	e.addon.mu.Unlock()
	var wg sync.WaitGroup
	listed := make([]int, len(libraries))
	for i, library := range libraries {
		wg.Go(func() {
			page, err := e.service.Children(t.Context(), e.member, library, 0, 100, "")
			if err != nil {
				t.Errorf("listing: %v", err)
			}
			listed[i] = len(page.Items)
		})
	}
	// The pages are held at the addon until 8 are under way, then each is
	// answered once the test lets one through.
	eventually(t, "8 pages under way", func() bool { _, underWay, _ := e.addon.counts(); return underWay >= 8 })
	for range 16 {
		release <- struct{}{}
	}
	wg.Wait()
	if asked, _, most := e.addon.counts(); asked != 4+16 || most > catalogHostFetches || listed[0] != 100 || listed[1] != 100 {
		t.Errorf("%d pages asked, at most %d at once, listed %v", asked, most, listed)
	}
}

// A listing serves the titles of the pages before one the addon cannot
// give, as the whole listing; a listing that starts past them fails.
func TestAListingServesThePagesBeforeOneThatCannotBeRead(t *testing.T) {
	t.Parallel()
	e := newAskingEnv(t, pagedAddon(100, 10))
	e.addon.mu.Lock()
	e.addon.failing = "skip=50"
	e.addon.mu.Unlock()
	top := e.library(e.member, "Top").ID
	page, err := e.service.Children(t.Context(), e.member, top, 0, 100, "")
	if err != nil || len(page.Items) != 50 || page.Items[49].Name != "movie 49" || page.More || page.Total != 50 {
		t.Fatalf("listing: %d items, total %d, more %v, %v", len(page.Items), page.Total, page.More, err)
	}
	if _, err := e.service.Children(t.Context(), e.member, top, 60, 20, ""); stremio.StatusOf(err) != http.StatusInternalServerError {
		t.Errorf("a listing past the page: %v", err)
	}
}
