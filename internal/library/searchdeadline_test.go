package library

import (
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// searchAddon serves a movie search catalog listing the movies named, and
// describes them when describes is set, as metadata addons do.
func searchAddon(id, name string, describes bool, movies ...string) *fakeAddon {
	resources := []stremio.Resource{{Name: "catalog"}}
	if describes {
		resources = append(resources, stremio.Resource{Name: "meta"})
	}
	metas := make([]stremio.Meta, len(movies))
	for i, movie := range movies {
		metas[i] = stremio.Meta{ID: "tt" + id + string(rune('0'+i)), Type: "movie", Name: movie}
	}
	return &fakeAddon{
		manifest: stremio.Manifest{ID: id, Name: name, Version: "1", Types: []string{"movie"}, Resources: resources,
			Catalogs: []stremio.Catalog{{Type: "movie", ID: id + "_search", Name: "Search", Extra: []stremio.Extra{{Name: "search", IsRequired: true}}}}},
		catalogs: map[string][]stremio.Meta{"movie/" + id + "_search": metas},
	}
}

// held holds addon's catalog requests until the test ends or opens the
// gate it returns; the server closes once the gate is open.
func held(e env, addon *fakeAddon) (open func()) {
	gate := make(chan struct{})
	addon.catalogGate = gate
	e.install(addons.Shared(), addon)
	opened := false
	open = func() {
		if !opened {
			opened = true
			close(gate)
		}
	}
	e.t.Cleanup(open)
	return open
}

// timedSearch searches term for movies, failing the test on an error, and
// tells how long the search took.
func timedSearch(e env, term string) ([]string, time.Duration) {
	e.t.Helper()
	started := time.Now()
	found, err := e.service.Search(e.t.Context(), e.member, term, []Kind{KindMovie}, 100)
	if err != nil {
		e.t.Fatalf("search %q: %v", term, err)
	}
	return names(found), time.Since(started)
}

// An addon that never answers holds no search past its deadline: the
// search answers with the other addons' titles, and the next searches
// leave that addon out without waiting for it, until a while has passed.
// System › Health tells it meanwhile.
func TestASearchDoesNotWaitForAnAddonThatNeverAnswers(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	elapsed := new(atomic.Int64)
	e.service.now = func() time.Time { return time.Now().Add(time.Duration(elapsed.Load())) }
	quick := searchAddon("quick", "Quick", false, "Lantern Harbor", "Lantern Bay")
	e.install(addons.Shared(), quick)
	silent := searchAddon("silent", "Silent", false, "Lantern Point")
	held(e, silent)
	silentID := e.addonID("silent")

	// The silent addon is held until the test ends: a search that waited
	// for it would wait for its request's 15-second timeout, far more
	// than the deadline on any machine.
	found, took := timedSearch(e, "lantern")
	if took > 10*time.Second || !slices.Equal(found, []string{"Lantern Harbor", "Lantern Bay"}) {
		t.Fatalf("first search: %v after %v", found, took)
	}
	if !e.service.SearchesLeaveOut(silentID) {
		t.Error("Health does not tell the late addon")
	}
	if e.service.SearchesLeaveOut(e.addonID("quick")) {
		t.Error("Health tells the addon that answered")
	}

	// Searches past the deadline would now wait for the silent addon's
	// request to time out: they do not ask it.
	e.service.searchGrace, e.service.searchBound = time.Hour, time.Hour
	for _, term := range []string{"lantern", "harbor"} {
		found, took := timedSearch(e, term)
		if took > 10*time.Second || len(found) == 0 {
			t.Errorf("search %q again: %v after %v", term, found, took)
		}
	}
	if got := len(silent.catalogRequests()); got != 1 {
		t.Errorf("the silent addon was asked %d times", got)
	}

	// After a while, it is asked again.
	e.service.searchGrace, e.service.searchBound = searchGrace, searchBound
	elapsed.Store(int64(lateSearchLife + time.Second))
	if found, _ := timedSearch(e, "bay"); !slices.Equal(found, []string{"Lantern Bay"}) {
		t.Errorf("later search: %v", found)
	}
	eventually(t, "asking the silent addon again", func() bool { return len(silent.catalogRequests()) == 2 })
}

// A page that comes after the search's deadline is kept: the next search
// of the term lists its titles without asking the addon again.
func TestALateSearchPageIsKeptForTheNextSearch(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.install(addons.Shared(), searchAddon("quick", "Quick", false, "Lantern Harbor"))
	slow := searchAddon("slow", "Slow", false, "Lantern Point")
	open := held(e, slow)

	found, took := timedSearch(e, "lantern")
	if took > 10*time.Second || !slices.Equal(found, []string{"Lantern Harbor"}) {
		t.Fatalf("first search: %v after %v", found, took)
	}
	open()
	eventually(t, "the late page listed", func() bool {
		found, _ := timedSearch(e, "lantern")
		return slices.Equal(found, []string{"Lantern Harbor", "Lantern Point"})
	})
	if got := len(slow.catalogRequests()); got != 1 {
		t.Errorf("the slow addon was asked %d times", got)
	}
	if e.service.SearchesLeaveOut(e.addonID("slow")) {
		t.Error("Health still tells the addon that answered")
	}
}

// Folder matching and the metadata editor still wait for every metadata
// addon, one that missed a search's deadline included.
func TestRemoteSearchWaitsForEveryAddon(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.install(addons.Shared(), searchAddon("quick", "Quick", true, "Lantern Harbor"))
	slow := searchAddon("slow", "Slow", true, "Lantern Point")
	open := held(e, slow)
	if found, _ := timedSearch(e, "lantern"); !slices.Equal(found, []string{"Lantern Harbor"}) {
		t.Fatalf("search: %v", found)
	}
	if !e.service.SearchesLeaveOut(e.addonID("slow")) {
		t.Fatal("the slow addon is not left out of searches")
	}

	type answer struct {
		results []RemoteResult
		err     error
	}
	answered := make(chan answer, 1)
	go func() {
		results, err := e.service.RemoteSearch(t.Context(), KindMovie, "point", 10)
		answered <- answer{results, err}
	}()
	eventually(t, "asking the slow addon", func() bool { return len(slow.catalogRequests()) == 2 })
	open()
	got := <-answered
	if got.err != nil || len(got.results) != 1 || got.results[0].Item.Name != "Lantern Point" || got.results[0].Provider != "Slow" {
		t.Errorf("remote search: %+v %v", got.results, got.err)
	}
}

// An empty answer to a search is kept a minute only: an addon may answer
// nothing while what it reads from fails, then list the title.
func TestAnEmptySearchAnswerIsAskedAgainSoon(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	elapsed := new(atomic.Int64)
	e.service.now = func() time.Time { return time.Now().Add(time.Duration(elapsed.Load())) }
	addon := searchAddon("meta", "Meta", false)
	e.install(addons.Shared(), addon)
	if found, _ := timedSearch(e, "lantern"); len(found) != 0 {
		t.Fatalf("first search: %v", found)
	}
	addon.mu.Lock()
	addon.catalogs["movie/meta_search"] = []stremio.Meta{{ID: "tt100", Type: "movie", Name: "Lantern Harbor"}}
	addon.mu.Unlock()
	elapsed.Store(int64(2 * time.Minute))
	if found, _ := timedSearch(e, "lantern"); !slices.Equal(found, []string{"Lantern Harbor"}) {
		t.Errorf("two minutes later: %v", found)
	}
}
