package jellyfin

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/stremio"
)

// holdLimit bounds how long a held request waits for the others of its
// wave (see artServer.holdWaves): long, so that a slow machine never
// answers a wave before its requests all arrived.
const holdLimit = 10 * time.Second

// artServer is an artwork server counting its requests and the most it
// answers at once; /missing.jpg is not found, /busy.jpg answers 503.
type artServer struct {
	*httptest.Server
	requests atomic.Int32
	mu       sync.Mutex
	current  int
	most     int
	// hold, when set, holds requests in waves: until hold of them wait,
	// waiting counting them; released is closed when a wave completes.
	hold     int
	waiting  int
	released chan struct{}
}

func newArtServer(t *testing.T, picture []byte) *artServer {
	s := &artServer{released: make(chan struct{})}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		s.mu.Lock()
		s.current++
		s.most = max(s.most, s.current)
		if s.hold > 0 {
			s.waiting++
			if s.waiting == s.hold {
				s.waiting = 0
				close(s.released)
				s.released = make(chan struct{})
			} else {
				released := s.released
				s.mu.Unlock()
				select {
				case <-released:
				case <-time.After(holdLimit):
				case <-r.Context().Done():
				}
				s.mu.Lock()
				if !isClosed(released) {
					s.waiting--
				}
			}
		}
		s.mu.Unlock()
		// Downloads more than allowed would arrive meanwhile.
		time.Sleep(20 * time.Millisecond)
		// The answer is counted out before it is sent: the next download
		// the server allows can only start once it is received.
		s.mu.Lock()
		s.current--
		s.mu.Unlock()
		switch r.URL.Path {
		case "/missing.jpg":
			http.NotFound(w, r)
			return
		case "/busy.jpg":
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(picture)
	}))
	t.Cleanup(s.Close)
	return s
}

func isClosed(c chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}

// holdWaves holds the requests to come in waves of size, 0 answering
// them at once, and forgets the most answered at once.
func (s *artServer) holdWaves(size int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hold, s.waiting, s.most = size, 0, 0
}

// mostAtOnce returns the most requests answered at once since the last
// holdWaves.
func (s *artServer) mostAtOnce() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.most
}

// burst downloads count different images from art at once.
func burst(t *testing.T, h *Handler, art *artServer, name string, count int, iptv bool) {
	var wg sync.WaitGroup
	for i := range count {
		wg.Go(func() { _, _ = h.fetchArtwork(t.Context(), fmt.Sprintf("%s/%s-%d.jpg", art.URL, name, i), false, iptv) })
	}
	wg.Wait()
}

func jpegOf(t *testing.T, width, height int) []byte {
	var out bytes.Buffer
	if err := jpeg.Encode(&out, image.NewRGBA(image.Rect(0, 0, width, height)), nil); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func imageHandler(dir string) *Handler {
	h := &Handler{Options: Options{Stremio: stremio.NewClient("test"), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}}
	h.images.open(dir, h.Logger)
	return h
}

func TestArtworkDownloadsAreSharedBoundedAndKept(t *testing.T) {
	art := newArtServer(t, jpegOf(t, 40, 60))
	dir := t.TempDir()
	h := imageHandler(dir)

	// Apps asking for the same image at once share one download.
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			if _, err := h.fetchArtwork(t.Context(), art.URL+"/one.jpg", false, false); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if got := art.requests.Load(); got != 1 {
		t.Errorf("5 apps at once: %d downloads", got)
	}

	// One host gets imageHostFetches downloads at once: waves of that many
	// all arrive, whatever the machine's speed, and never more.
	art.holdWaves(imageHostFetches)
	burst(t, h, art, "first", 2*imageHostFetches, false)
	if most := art.mostAtOnce(); most != imageHostFetches {
		t.Errorf("%d downloads at once from one host", most)
	}
	// A host that answers 503 is spared for a while.
	art.holdWaves(0)
	if _, err := h.fetchArtwork(t.Context(), art.URL+"/busy.jpg", false, false); err == nil {
		t.Error("a busy host answered")
	}
	art.holdWaves(imageSlowHostFetches)
	burst(t, h, art, "spared", 3*imageSlowHostFetches, false)
	if most := art.mostAtOnce(); most != imageSlowHostFetches {
		t.Errorf("%d downloads at once from a busy host", most)
	}
	art.holdWaves(0)
	// So is a host that serves an IPTV source.
	logos := newArtServer(t, jpegOf(t, 40, 40))
	logos.holdWaves(imageSlowHostFetches)
	burst(t, h, logos, "logo", 3*imageSlowHostFetches, true)
	if most := logos.mostAtOnce(); most != imageSlowHostFetches {
		t.Errorf("%d downloads at once from an IPTV host", most)
	}

	// A failed download is not asked again for a while.
	before := art.requests.Load()
	for range 2 {
		if _, err := h.fetchArtwork(t.Context(), art.URL+"/missing.jpg", false, false); err == nil {
			t.Error("a missing image was found")
		}
	}
	if got := art.requests.Load() - before; got != 1 {
		t.Errorf("a missing image was asked %d times", got)
	}

	// The disk keeps the images across a restart.
	before = art.requests.Load()
	restarted := imageHandler(dir)
	if image, err := restarted.fetchArtwork(t.Context(), art.URL+"/one.jpg", false, false); err != nil || image.contentType != "image/jpeg" {
		t.Errorf("after a restart: %v %v", image.contentType, err)
	}
	if got := art.requests.Load() - before; got != 0 {
		t.Errorf("after a restart: %d downloads", got)
	}
}

func TestArtworkIsResizedWhenMuchSmallerIsAsked(t *testing.T) {
	original := artwork{body: jpegOf(t, 3840, 2160), contentType: "image/jpeg"}
	query := func(raw string) url.Values {
		values, _ := url.ParseQuery(raw)
		return values
	}
	for raw, want := range map[string]int{"maxWidth=1920": 1920, "MaxHeight=300": 640, "maxWidth=3000": 0, "": 0, "fillWidth=100&fillHeight=4000": 120} {
		if got := resizedWidth(query(raw), 3840, 2160); got != want {
			t.Errorf("%q: width %d, want %d", raw, got, want)
		}
	}
	h := imageHandler(t.TempDir())
	resized := h.resized(t.Context(), "https://art.example/backdrop.jpg", original, 1920)
	config, _, err := resized.decodeConfig()
	if err != nil || config.Width != 1920 || config.Height != 1080 || len(resized.body) >= len(original.body) {
		t.Errorf("resized: %+v %v, %d bytes of %d", config, err, len(resized.body), len(original.body))
	}
}
