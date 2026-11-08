package library

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// playEnv serves one movie from a slow addon, which answers its stream
// requests once let, or after slowFor, and from a fast addon, which
// answers hers at once with fast. Stream requests are given 2 seconds,
// the others 1, and plays a grace of 100 ms.
type playEnv struct {
	env
	slow  *fakeAddon
	let   chan struct{}
	movie accounts.ID
}

func newPlayEnv(t *testing.T, fast []stremio.Stream, slowFor time.Duration) playEnv {
	t.Helper()
	e := newEnv(t)
	e.service.client.SetTimeouts(time.Second, 2*time.Second)
	e.service.SetPlayGrace(100 * time.Millisecond)
	let := make(chan struct{})
	movies := titles("movie", 1)
	slow := &fakeAddon{
		manifest: stremio.Manifest{ID: "slow", Name: "Slow", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{"movie/top": movies},
		metas:    map[string]stremio.Meta{"movie/" + movies[0].ID: movies[0]},
		reply: func(w http.ResponseWriter, r *http.Request, resource string) {
			if resource == "stream" {
				select {
				case <-let:
				case <-time.After(slowFor):
				case <-r.Context().Done():
					return
				}
			}
			answerNumbered(w, resource, []int{1})
		},
	}
	e.install(addons.Shared(), slow)
	e.install(addons.Shared(), &fakeAddon{
		manifest: stremio.Manifest{ID: "fast", Name: "Fast", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "stream"}}, IDPrefixes: []string{"tt"}},
		streams: map[string][]stremio.Stream{"movie/" + movies[0].ID: fast},
	})
	page, err := e.service.Children(t.Context(), e.member, e.library(e.member, "Top").ID, 0, 10, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("listing: %+v %v", page, err)
	}
	return playEnv{env: e, slow: slow, let: let, movie: page.Items[0].ID}
}

// slowAsked counts the slow addon's stream requests.
func (e playEnv) slowAsked() int {
	e.slow.mu.Lock()
	defer e.slow.mu.Unlock()
	n := 0
	for _, request := range e.slow.requests {
		if strings.HasPrefix(request, "stream/") {
			n++
		}
	}
	return n
}

// A play has the version the fast addon lists a grace after its answer,
// without waiting for the slow one, and is not complete: that one is still
// asked, and counted pending for the title's page. Its answer, once it
// comes, is kept, the play's request joined rather than sent again, and
// the next play has both versions at once.
func TestAPlayDoesNotWaitForASlowAddonOnceAnotherAnswered(t *testing.T) {
	t.Parallel()
	e := newPlayEnv(t, []stremio.Stream{{Name: "Fast", URL: "https://fast.example/movie.mkv"}}, time.Hour)
	started := time.Now()
	versions, complete, err := e.service.VersionsToPlay(t.Context(), e.member, e.movie)
	if err != nil || len(versions) != 1 || versions[0].Name != "Fast" || complete {
		t.Fatalf("the play: %+v, complete %t, %v; want the fast addon's version alone", versions, complete, err)
	}
	if took := time.Since(started); took > time.Second {
		t.Errorf("the play waited %s", took)
	}
	if pending, err := e.service.Pending(t.Context(), e.member, e.movie); err != nil || pending != 1 {
		t.Errorf("%d addons pending, %v; want the slow one", pending, err)
	}

	close(e.let)
	eventually(t, "the slow addon's answer to be kept", func() bool {
		versions, ok := e.service.CachedVersions(t.Context(), e.member, e.movie)
		return ok && len(versions) == 2
	})
	if got := e.slowAsked(); got != 1 {
		t.Errorf("the slow addon was asked %d times, want once", got)
	}
	versions, complete, err = e.service.VersionsToPlay(t.Context(), e.member, e.movie)
	if err != nil || len(versions) != 2 || !complete {
		t.Errorf("the next play: %d versions, complete %t, %v; want both", len(versions), complete, err)
	}
}

// With no version to play from the other addons, a play waits for the
// slow one: its answer, slower than the limit of the requests for
// anything but streams, is listed and kept.
func TestAPlayWaitsForASlowAddonWhenTheOthersHaveNothingToPlay(t *testing.T) {
	t.Parallel()
	e := newPlayEnv(t, nil, 1500*time.Millisecond)
	started := time.Now()
	versions, complete, err := e.service.VersionsToPlay(t.Context(), e.member, e.movie)
	if err != nil || len(versions) != 1 || !complete {
		t.Fatalf("the play: %d versions, complete %t, %v; want the slow addon's", len(versions), complete, err)
	}
	if took := time.Since(started); took < 1500*time.Millisecond {
		t.Errorf("the play took %s, less than the slow addon", took)
	}
	if versions, ok := e.service.CachedVersions(t.Context(), e.member, e.movie); !ok || len(versions) != 1 {
		t.Errorf("kept: %d versions (%t)", len(versions), ok)
	}
}
