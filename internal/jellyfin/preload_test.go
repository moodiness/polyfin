package jellyfin

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// readEpisodes is an addon whose episodes each have a file of their own,
// named after the episode's stream identifier, and which counts the
// requests for each file, and those for streams. A file is served only by
// the links of the addon's latest answers: renew makes those given before
// expire.
type readEpisodes struct {
	mu    sync.Mutex
	read  map[string]int
	asked int
	links int
}

func newReadEpisodes(t *testing.T) (*readEpisodes, string) {
	t.Helper()
	e := &readEpisodes{read: map[string]int{}}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.Path; {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "episodes", Name: "Episodes", Version: "1", Types: []string{"series"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "stream"}}})
		case strings.HasPrefix(path, "/stream/series/"):
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/stream/series/"), ".json")
			e.mu.Lock()
			e.asked++
			links := e.links
			e.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{
				{Name: "1080p", URL: fmt.Sprintf("%s/files/%d/%s.mkv", server.URL, links, id)}}})
		case strings.HasPrefix(path, "/files/"):
			links, file, _ := strings.Cut(strings.TrimPrefix(path, "/files/"), "/")
			e.mu.Lock()
			current := links == fmt.Sprint(e.links)
			if current {
				e.read[strings.TrimSuffix(file, ".mkv")]++
			}
			e.mu.Unlock()
			if !current {
				http.NotFound(w, r)
				return
			}
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader("\x1a\x45\xdf\xa3 media bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return e, server.URL + "/manifest.json"
}

// streamsAsked is how many requests for streams the addon answered.
func (e *readEpisodes) streamsAsked() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.asked
}

// renew makes the links the addon gave expire: its next answers give new
// ones.
func (e *readEpisodes) renew() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.links++
}

// files lists the files read, by the season and episode they are of.
func (e *readEpisodes) files() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var files []string
	for id := range maps.Keys(e.read) {
		parts := strings.Split(id, ":")
		files = append(files, strings.Join(parts[len(parts)-2:], "x"))
	}
	slices.Sort(files)
	return files
}

// The next episodes of a series a user watches lately are read ahead into
// the source cache while nothing plays, once preloading is on.
func TestTheNextEpisodesArePreloaded(t *testing.T) {
	t.Parallel()
	tr := trackingOn(t, newTestServer(t, 10))
	episodes, manifest := newReadEpisodes(t)
	if _, err := tr.addons.Install(t.Context(), addons.Shared(), manifest, false); err != nil {
		t.Fatal(err)
	}
	numbered := func(id string) string {
		t.Helper()
		var item BaseItemDto
		if status := tr.get(t, "/Users/"+tr.user+"/Items/"+id, tr.token, &item); status != http.StatusOK {
			t.Fatalf("episode %s: %d", id, status)
		}
		return fmt.Sprintf("%dx%d", *item.ParentIndexNumber, *item.IndexNumber)
	}
	tr.mark(t, http.MethodPost, "/UserPlayedItems/"+tr.episodes[0])

	tr.handler.preloadNext(t.Context())
	if files := episodes.files(); len(files) != 0 {
		t.Fatalf("switched off, read %v", files)
	}
	// The disk keeps the least reserve: the default, 20 GB, is more than
	// some CI runners' disks have free, and preloading holds back below it
	// (see source.Preloading), which the source cache's tests check.
	tr.setting(t, func(s *accounts.Settings) {
		s.PreloadNextEpisodes, s.PreloadEpisodes, s.PreloadReserveGB = true, 2, accounts.MinPreloadReserveGB
	})
	// A playback under way holds preloading back.
	tr.report(t, "/Sessions/Playing", map[string]any{"ItemId": tr.movie, "PositionTicks": 0, "PlayMethod": "DirectPlay"})
	tr.handler.preloadNext(t.Context())
	if files := episodes.files(); len(files) != 0 {
		t.Fatalf("while a movie plays, read %v", files)
	}
	tr.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": tr.movie, "PositionTicks": 0})

	tr.handler.preloadNext(t.Context())
	want := []string{numbered(tr.episodes[1]), numbered(tr.episodes[2])}
	slices.Sort(want)
	if files := episodes.files(); !slices.Equal(files, want) {
		t.Errorf("read %v, want the next two episodes %v", files, want)
	}
}

// preloadingOn is a tracking server with preloading on, one episode for
// each series, on a library clock elapsed moves, and an addon of episodes:
// the member played the first episode, so a pass preloads the second.
func preloadingOn(t *testing.T) (tracking, *readEpisodes, *atomic.Int64) {
	t.Helper()
	s := newTestServer(t, 10)
	elapsed := new(atomic.Int64)
	s.library.SetClock(func() time.Time { return time.Now().Add(time.Duration(elapsed.Load())) })
	tr := trackingOn(t, s)
	episodes, manifest := newReadEpisodes(t)
	if _, err := tr.addons.Install(t.Context(), addons.Shared(), manifest, false); err != nil {
		t.Fatal(err)
	}
	tr.mark(t, http.MethodPost, "/UserPlayedItems/"+tr.episodes[0])
	tr.setting(t, func(s *accounts.Settings) {
		s.PreloadNextEpisodes, s.PreloadEpisodes, s.PreloadReserveGB = true, 1, accounts.MinPreloadReserveGB
	})
	return tr, episodes, elapsed
}

// A pass lists, every preloadEvery all night, the next episodes of the
// same series, whose version lists expire between two passes. Once an
// episode's versions are known, a pass preloads from them, as a play
// would play them, and asks the addons nothing.
func TestPreloadingAsksNoAddonAgainForKnownVersions(t *testing.T) {
	t.Parallel()
	tr, episodes, elapsed := preloadingOn(t)
	tr.handler.preloadNext(t.Context())
	if asked, files := episodes.streamsAsked(), episodes.files(); asked != 1 || len(files) != 1 {
		t.Fatalf("first pass: the addon asked %d times, read %v; want once, the next episode read", asked, files)
	}
	for pass := 2; pass <= 5; pass++ {
		elapsed.Add(int64(preloadEvery + time.Minute))
		tr.handler.preloadNext(t.Context())
		if asked := episodes.streamsAsked(); asked != 1 {
			t.Fatalf("pass %d, the version list expired: the addon asked %d times, want once", pass, asked)
		}
	}
}

// A version known whose link expired is not preloaded: the addons are
// asked again, and the version of their answer is preloaded. The passes
// after ask them nothing again.
func TestPreloadingAsksAgainWhenTheKnownVersionCannotBeRead(t *testing.T) {
	t.Parallel()
	tr, episodes, elapsed := preloadingOn(t)
	member, _ := tr.store.Authenticate(t.Context(), "member", "correct horse")
	if _, err := tr.library.Versions(t.Context(), member, mustID(t, tr.episodes[1])); err != nil {
		t.Fatal(err)
	}
	episodes.renew()
	elapsed.Add(int64(preloadEvery + time.Minute))
	tr.handler.preloadNext(t.Context())
	if asked, files := episodes.streamsAsked(), episodes.files(); asked != 2 || len(files) != 1 {
		t.Fatalf("the link known expired: the addon asked %d times, read %v; want twice, the episode read by the new link", asked, files)
	}
	elapsed.Add(int64(preloadEvery + time.Minute))
	tr.handler.preloadNext(t.Context())
	if asked := episodes.streamsAsked(); asked != 2 {
		t.Errorf("the pass after: the addon asked %d times, want twice", asked)
	}
}
