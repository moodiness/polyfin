package localfiles

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/stremio"
)

// failingSearch is a metadata addon whose search catalog answers status
// to its first failures searches, then finds The Lone Ranger; searches
// counts them.
func failingSearch(t *testing.T, status, failures int) (manifest string, searches *atomic.Int32) {
	t.Helper()
	searches = &atomic.Int32{}
	show := stremio.Meta{ID: "tt0041038", Type: "series", Name: "The Lone Ranger", Year: "1949–1957"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "flaky", Name: "Flaky", Version: "1", Types: []string{"series"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "series", ID: "top", Name: "Top", Extra: []stremio.Extra{{Name: "search"}}}}})
		case strings.HasPrefix(path, "/catalog/series/top/search="):
			if int(searches.Add(1)) <= failures {
				http.Error(w, "failing", status)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{show}})
		case path == "/meta/series/tt0041038.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": show})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json", searches
}

// A search the metadata addon fails (HTTP 5xx) is tried again within the
// scan, and the file is matched when the addon answers then.
func TestSearchesAnAddonFailedAreTriedAgain(t *testing.T) {
	t.Parallel()
	manifest, searches := failingSearch(t, http.StatusGatewayTimeout, 1)
	e := newEnvWith(t, manifest)
	rel := "The Lone Ranger (1949)/Season 01/The Lone Ranger S01E01.mkv"
	addon, _ := e.folder(KindShows, map[string]int{rel: 10})
	if got := e.files(addon.ID)[rel]; got != "tt0041038" {
		t.Errorf("the file once the addon answers: %s", got)
	}
	if got := searches.Load(); got != 2 {
		t.Errorf("searches: %d", got)
	}
	folder, err := e.service.Folder(t.Context(), addon.ID)
	if err != nil || folder.SearchFailed != 0 || folder.SearchRetryAt != nil {
		t.Errorf("the folder: %d failed, retry at %v, %v", folder.SearchFailed, folder.SearchRetryAt, err)
	}
}

// A search the addon keeps failing is tried twice more, then left to the
// next scan; one it refuses (HTTP 4xx) is not tried again within the scan.
func TestSearchesTriedAgainAtMostTwice(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		status int
		want   int32
	}{{http.StatusBadGateway, 3}, {http.StatusNotFound, 1}} {
		manifest, searches := failingSearch(t, tc.status, 100)
		e := newEnvWith(t, manifest)
		rel := "The Lone Ranger (1949)/Season 01/The Lone Ranger S01E01.mkv"
		addon, _ := e.folder(KindShows, map[string]int{rel: 10})
		if got := e.files(addon.ID)[rel]; got != "unmatched: "+reasonSearchFailed {
			t.Errorf("HTTP %d: the file: %s", tc.status, got)
		}
		if got := searches.Load(); got != tc.want {
			t.Errorf("HTTP %d: searches %d, want %d", tc.status, got, tc.want)
		}
	}
}

// recordedWaits records the waits of a scan's searches before a failed
// search is tried again (see Service.serverWait), without waiting.
type recordedWaits struct {
	mu    sync.Mutex
	waits []time.Duration
}

func (r *recordedWaits) wait(ctx context.Context, d time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.waits = append(r.waits, d)
	return ctx.Err()
}

func (r *recordedWaits) all() []time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.waits)
}

// oneAtATime is the environment of addons, whose service records its
// waits and matches one unit at a time, so that the searches come in
// order.
func oneAtATime(t *testing.T, addons ...*searchAddon) (env, *recordedWaits) {
	t.Helper()
	manifests := make([]string, len(addons))
	for i, addon := range addons {
		manifests[i] = addon.start(t)
	}
	e := newEnvWith(t, manifests...)
	waits := &recordedWaits{}
	e.service.serverWait = waits.wait
	e.service.workers = 1
	return e, waits
}

// Within a scan, an addon that failed a search through all its tries is
// asked once by each search after, which waits for it no more: a scan of
// many files waits once, not for each file. The files another addon
// finds are matched, and the others left as failed searches, as before.
func TestAnAddonFailingThroughItsTriesIsAskedOnce(t *testing.T) {
	t.Parallel()
	broken := &searchAddon{name: "Broken", extra: []stremio.Extra{{Name: "search", IsRequired: true}}, answer: failing(http.StatusInternalServerError)}
	e, waits := oneAtATime(t, broken, &searchAddon{name: "Meta", extra: []stremio.Extra{{Name: "search", IsRequired: true}}})
	folder, _ := e.folder(KindMovies, map[string]int{"The General (1926).mkv": 1, "Nosferatu (1922).mkv": 1,
		"The Gold Rush (1925).mkv": 1, "Lost Reel (1931).mkv": 1, "Harbor Lights (2011).mkv": 1, "Quay Street (1999).mkv": 1})
	want := map[string]string{"The General (1926).mkv": "tt0017925", "Nosferatu (1922).mkv": "tt0013442",
		"The Gold Rush (1925).mkv": "tt0015864", "Lost Reel (1931).mkv": "unmatched: search_failed",
		"Harbor Lights (2011).mkv": "unmatched: search_failed", "Quay Street (1999).mkv": "unmatched: search_failed"}
	if got := e.files(folder.ID); !maps.Equal(got, want) {
		t.Errorf("files: %v", got)
	}
	// The first search waits 1 then 3 seconds, the five after it none.
	if got := waits.all(); !slices.Equal(got, serverWaits[:]) {
		t.Errorf("waits: %v", got)
	}
	if got, want := len(broken.asked()), 1+len(serverWaits)+5; got != want {
		t.Errorf("the failing addon was asked %d searches, want %d", got, want)
	}
	if f, err := e.service.Folder(t.Context(), folder.ID); err != nil || f.SearchFailed != 3 || f.SearchRetryAt == nil {
		t.Errorf("the folder: %d failed, retry at %v, %v", f.SearchFailed, f.SearchRetryAt, err)
	}
}

// An addon that answers a search of the scan again gets its tries back:
// the next search it fails is tried again, as the first was.
func TestAnAddonAnsweringAgainGetsItsTriesBack(t *testing.T) {
	t.Parallel()
	// The addon fails the first search's tries, answers the second
	// search, and fails every request after.
	broken := &searchAddon{name: "Broken", extra: []stremio.Extra{{Name: "search", IsRequired: true}},
		answer: func(w http.ResponseWriter, n int, _ map[string]string) bool {
			if n == 1+len(serverWaits)+1 {
				return false
			}
			w.WriteHeader(http.StatusBadGateway)
			return true
		}}
	e, waits := oneAtATime(t, broken, &searchAddon{name: "Meta", extra: []stremio.Extra{{Name: "search", IsRequired: true}}})
	folder, _ := e.folder(KindMovies, map[string]int{"Lost Reel (1931).mkv": 1, "Harbor Lights (2011).mkv": 1,
		"Quay Street (1999).mkv": 1, "Tidewater (2015).mkv": 1})
	// The first search tries three times, the second asks once and is
	// answered, the third tries three times again, the fourth asks once.
	if got, want := waits.all(), slices.Concat(serverWaits[:], serverWaits[:]); !slices.Equal(got, want) {
		t.Errorf("waits: %v, want %v", got, want)
	}
	if got, want := len(broken.asked()), 2*(1+len(serverWaits))+2; got != want {
		t.Errorf("the failing addon was asked %d searches, want %d", got, want)
	}
	reasons := map[string]int{}
	for _, got := range e.files(folder.ID) {
		reasons[got]++
	}
	if want := map[string]int{"unmatched: search_failed": 3, "unmatched: not_found": 1}; !maps.Equal(reasons, want) {
		t.Errorf("files: %v", reasons)
	}
}
