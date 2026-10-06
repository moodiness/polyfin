package library

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// lifetimeEnv serves one movie with a stream, on a clock the test moves.
type lifetimeEnv struct {
	env
	addon   *fakeAddon
	elapsed *atomic.Int64
}

func newLifetimeEnv(t *testing.T) lifetimeEnv {
	t.Helper()
	e := newEnv(t)
	elapsed := new(atomic.Int64)
	e.service.now = func() time.Time { return time.Now().Add(time.Duration(elapsed.Load())) }
	movies := titles("movie", 1)
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "a", Name: "A", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{"movie/top": movies},
		metas:    map[string]stremio.Meta{"movie/" + movies[0].ID: movies[0]},
		streams:  map[string][]stremio.Stream{"movie/" + movies[0].ID: {{Name: "1080p", URL: "https://cdn.example/movie"}}},
	}
	e.install(addons.Shared(), addon)
	return lifetimeEnv{env: e, addon: addon, elapsed: elapsed}
}

// requests counts the requests whose path starts with prefix.
func (e lifetimeEnv) requests(prefix string) int {
	e.addon.mu.Lock()
	defer e.addon.mu.Unlock()
	count := 0
	for _, request := range e.addon.requests {
		if strings.HasPrefix(request, prefix) {
			count++
		}
	}
	return count
}

func (e lifetimeEnv) wait(d time.Duration) { e.elapsed.Add(int64(d)) }

// list reads the Top library and returns its single movie.
func (e lifetimeEnv) list() Item {
	e.t.Helper()
	page, err := e.service.Children(e.t.Context(), e.member, e.library(e.member, "Top").ID, 0, 10, "")
	if err != nil || len(page.Items) != 1 {
		e.t.Fatalf("listing: %+v %v", page, err)
	}
	return page.Items[0]
}

func (e lifetimeEnv) versions(movie accounts.ID) {
	e.t.Helper()
	if versions, err := e.service.Versions(e.t.Context(), e.member, movie); err != nil || len(versions) != 1 {
		e.t.Fatalf("versions: %+v %v", versions, err)
	}
}

func TestVersionListsAreKeptForTheirSetLife(t *testing.T) {
	e := newLifetimeEnv(t)
	if got := e.users.Settings().VersionListMinutes; got != 10 {
		t.Fatalf("default list life: %d minutes", got)
	}
	movie := e.list().ID
	e.versions(movie)
	if got := e.requests("stream/"); got != 1 {
		t.Fatalf("first listing: %d stream requests", got)
	}
	// By default, lists are kept ten minutes.
	e.wait(9 * time.Minute)
	e.versions(movie)
	if got := e.requests("stream/"); got != 1 {
		t.Errorf("9 minutes later, by default: %d stream requests", got)
	}
	e.wait(2 * time.Minute)
	e.versions(movie)
	if got := e.requests("stream/"); got != 2 {
		t.Errorf("11 minutes later, by default: %d stream requests", got)
	}

	// A longer life keeps the list past ten minutes, the one just fetched
	// included.
	e.setting(func(s *accounts.Settings) { s.VersionListMinutes = 60 })
	e.wait(50 * time.Minute)
	e.versions(movie)
	if got := e.requests("stream/"); got != 2 {
		t.Errorf("50 minutes later, kept an hour: %d stream requests", got)
	}
	e.wait(11 * time.Minute)
	e.versions(movie)
	if got := e.requests("stream/"); got != 3 {
		t.Errorf("61 minutes later, kept an hour: %d stream requests", got)
	}

	// A shorter one fetches it again sooner, and applies at once.
	e.wait(3 * time.Minute)
	e.setting(func(s *accounts.Settings) { s.VersionListMinutes = 2 })
	e.versions(movie)
	if got := e.requests("stream/"); got != 4 {
		t.Errorf("3 minutes later, kept 2 minutes: %d stream requests", got)
	}
	e.wait(time.Minute)
	e.versions(movie)
	if got := e.requests("stream/"); got != 4 {
		t.Errorf("1 minute later, kept 2 minutes: %d stream requests", got)
	}
}
