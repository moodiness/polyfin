package jellyfin

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// heldAddon lists two streams of every movie, named after it, once
// released; broken, it then fails instead.
type heldAddon struct {
	url     string
	asked   atomic.Int32
	once    sync.Once
	release chan struct{}
}

func newHeldAddon(t *testing.T, name string, broken bool) *heldAddon {
	t.Helper()
	a := &heldAddon{release: make(chan struct{})}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: strings.ToLower(name), Name: name, Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "stream"}}})
		case strings.HasPrefix(path, "/stream/movie/"):
			a.asked.Add(1)
			select {
			case <-a.release:
			case <-r.Context().Done():
				return
			}
			if broken {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
				return
			}
			stream := func(height string, size int64) stremio.Stream {
				file := name + "." + height + ".mkv"
				return stremio.Stream{Name: name + " " + height, URL: server.URL + "/files/" + file,
					BehaviorHints: stremio.StreamBehavior{Filename: file, VideoSize: stremio.Number(size)}}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{stream("1080p", 3_000_000_000), stream("720p", 1_000_000_000)}})
		case strings.HasPrefix(path, "/files/"):
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader("\x1a\x45\xdf\xa3 media bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	// Released first, so that the requests held end and the server closes.
	t.Cleanup(func() {
		a.answer()
		server.Close()
	})
	a.url = server.URL + "/manifest.json"
	return a
}

// answer releases the streams.
func (a *heldAddon) answer() { a.once.Do(func() { close(a.release) }) }

// oneMovieAddon describes one movie and lists no stream.
func oneMovieAddon(t *testing.T) string {
	t.Helper()
	movie := stremio.Meta{ID: "tt1000", Type: "movie", Name: "Movie", Runtime: "2h", Year: "2008"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "metadata", Name: "Metadata", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case path == "/meta/movie/tt1000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

type heldSetup struct {
	testServer
	token string
	user  accounts.User
	movie string
}

// heldOn installs the metadata addon, then the stream addons in order, and
// signs a member in.
func heldOn(t *testing.T, s testServer, streams ...*heldAddon) heldSetup {
	t.Helper()
	user := s.user("member", nil)
	urls := []string{oneMovieAddon(t)}
	for _, a := range streams {
		urls = append(urls, a.url)
	}
	for _, url := range urls {
		if _, err := s.addons.Install(t.Context(), addons.Shared(), url, false); err != nil {
			t.Fatal(err)
		}
	}
	token := s.signIn("member", "tv")
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	var page QueryResult
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	if len(page.Items) != 1 {
		t.Fatalf("library: %+v", page.Items)
	}
	return heldSetup{testServer: s, token: token, user: user, movie: page.Items[0].Id}
}

// sources are the media sources of the movie's details.
func (h heldSetup) sources(t *testing.T) []MediaSourceInfo {
	t.Helper()
	var movie BaseItemDto
	if status := h.get(t, "/Users/"+h.user.ID.String()+"/Items/"+h.movie, h.token, &movie); status != http.StatusOK || movie.MediaSources == nil {
		t.Fatalf("details: %d %+v", status, movie.MediaSources)
	}
	return *movie.MediaSources
}

func (h heldSetup) progress(t *testing.T) VersionProgress {
	t.Helper()
	var progress VersionProgress
	if status := h.get(t, "/Polyfin/Items/"+h.movie+"/Versions", h.token, &progress); status != http.StatusOK {
		t.Fatalf("progress: %d", status)
	}
	return progress
}

// reaches waits for the progress to be want, then checks that details list
// as many sources as it counts, named after the addons in order, or after
// the title for the placeholder.
func (h heldSetup) reaches(t *testing.T, want VersionProgress, names ...string) {
	t.Helper()
	var got VersionProgress
	eventually(t, "the progress to reach "+strings.Join(names, ", "), func() bool {
		got = h.progress(t)
		return got == want
	})
	sources := h.sources(t)
	if len(sources) != got.Count || len(sources) != len(names) {
		t.Fatalf("progress %+v, details list %d sources, want %d", got, len(sources), len(names))
	}
	for i, source := range sources {
		if source.Name != names[i] && !strings.HasPrefix(source.Name, names[i]+" ") {
			t.Errorf("source %d: %q, want one of %s", i, source.Name, names[i])
		}
	}
}

func TestDetailsAnswerBeforeTheAddons(t *testing.T) {
	first, second := newHeldAddon(t, "First", false), newHeldAddon(t, "Second", false)
	h := heldOn(t, newTestServer(t, 10), first, second)

	// Both addons are held: details answer with the placeholder, the
	// title's own identifier, and ask them in the background.
	sources := h.sources(t)
	if len(sources) != 1 || sources[0].Id != h.movie || !sources[0].SupportsDirectPlay {
		t.Fatalf("details before any addon answered: %+v", sources)
	}
	if got := h.progress(t); got != (VersionProgress{Pending: 2, Count: 1}) {
		t.Errorf("before any addon answered: %+v", got)
	}
	// Opened again meanwhile, the title asks neither addon again.
	h.sources(t)
	eventually(t, "both addons to be asked", func() bool { return first.asked.Load() == 1 && second.asked.Load() == 1 })

	first.answer()
	h.reaches(t, VersionProgress{Pending: 1, Count: 2}, "First", "First")
	second.answer()
	h.reaches(t, VersionProgress{Pending: 0, Count: 4}, "First", "First", "Second", "Second")
	if first.asked.Load() != 1 || second.asked.Load() != 1 {
		t.Errorf("addons asked %d and %d times", first.asked.Load(), second.asked.Load())
	}
	// Known now, the versions come with the details at once.
	h.sources(t)
	if got := h.progress(t); got != (VersionProgress{Pending: 0, Count: 4}) || first.asked.Load() != 1 {
		t.Errorf("reopened: %+v, first addon asked %d times", got, first.asked.Load())
	}

	if status := h.get(t, "/Polyfin/Items/"+h.movie+"/Versions", "", nil); status != http.StatusUnauthorized {
		t.Errorf("without credentials: %d", status)
	}
	if status := h.get(t, "/Polyfin/Items/0123456789abcdef0123456789abcdef/Versions", h.token, nil); status != http.StatusNotFound {
		t.Errorf("unknown item: %d", status)
	}
}

func TestAFailingAddonLeavesNothingPending(t *testing.T) {
	first, broken := newHeldAddon(t, "First", false), newHeldAddon(t, "Broken", true)
	h := heldOn(t, newTestServer(t, 10), first, broken)

	h.sources(t)
	broken.answer()
	h.reaches(t, VersionProgress{Pending: 1, Count: 1}, "Movie")
	first.answer()
	h.reaches(t, VersionProgress{Pending: 0, Count: 2}, "First", "First")
}

func TestPlaybackInfoWaitsForTheAddonsAsked(t *testing.T) {
	probe := newFakeProbe(t, true)
	first, second := newHeldAddon(t, "First", false), newHeldAddon(t, "Second", false)
	h := heldOn(t, newProbingServer(t, 10, probe.path), first, second)
	t.Cleanup(func() { h.settle(t) })

	h.sources(t)
	eventually(t, "both addons to be asked", func() bool { return first.asked.Load() == 1 && second.asked.Load() == 1 })
	profile, err := os.ReadFile(filepath.Join(playbackFixtures, "profiles", "jellyfin-web-chrome.json"))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"UserId": h.user.ID.String(), "DeviceProfile": json.RawMessage(profile)})
	type answer struct {
		status int
		info   playbackInfoResponse
		err    error
	}
	answered := make(chan answer, 1)
	go func() {
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, h.url+"/Items/"+h.movie+"/PlaybackInfo", bytes.NewReader(body))
		request.Header.Set("Authorization", app("tv", h.token))
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			answered <- answer{err: err}
			return
		}
		defer response.Body.Close()
		data, _ := io.ReadAll(response.Body)
		a := answer{status: response.StatusCode}
		a.err = json.Unmarshal(data, &a.info)
		answered <- a
	}()
	for _, a := range []*heldAddon{first, second} {
		select {
		case got := <-answered:
			t.Fatalf("PlaybackInfo answered while an addon was held: %+v", got)
		case <-time.After(200 * time.Millisecond):
		}
		a.answer()
	}
	select {
	case got := <-answered:
		if got.err != nil || got.status != http.StatusOK || len(got.info.MediaSources) != 4 {
			t.Fatalf("PlaybackInfo: %d %v %+v", got.status, got.err, got.info.MediaSources)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("PlaybackInfo did not answer")
	}
	if first.asked.Load() != 1 || second.asked.Load() != 1 {
		t.Errorf("addons asked %d and %d times", first.asked.Load(), second.asked.Load())
	}
	if got := h.progress(t); got != (VersionProgress{Pending: 0, Count: 4}) {
		t.Errorf("once played: %+v", got)
	}
}
