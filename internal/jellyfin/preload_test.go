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
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// readEpisodes is an addon whose episodes each have a file of their own,
// named after the episode's stream identifier, and which counts the
// requests for each file.
type readEpisodes struct {
	mu   sync.Mutex
	read map[string]int
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
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{
				{Name: "1080p", URL: server.URL + "/files/" + id + ".mkv"}}})
		case strings.HasPrefix(path, "/files/"):
			e.mu.Lock()
			e.read[strings.TrimSuffix(strings.TrimPrefix(path, "/files/"), ".mkv")]++
			e.mu.Unlock()
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader("\x1a\x45\xdf\xa3 media bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return e, server.URL + "/manifest.json"
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
