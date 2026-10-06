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

// artServer is an artwork server answering slowly, counting its requests
// and the most it answers at once; /missing.jpg is not found.
type artServer struct {
	*httptest.Server
	requests atomic.Int32
	mu       sync.Mutex
	current  int
	most     int
}

func newArtServer(t *testing.T, picture []byte) *artServer {
	s := &artServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		s.mu.Lock()
		s.current++
		s.most = max(s.most, s.current)
		s.mu.Unlock()
		defer func() {
			s.mu.Lock()
			s.current--
			s.mu.Unlock()
		}()
		time.Sleep(100 * time.Millisecond)
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

// mostAtOnce returns the most requests answered at once since the last
// call.
func (s *artServer) mostAtOnce() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	most := s.most
	s.most = 0
	return most
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

	// One host gets at most imageHostFetches downloads at once.
	art.mostAtOnce()
	burst(t, h, art, "first", 24, false)
	if most := art.mostAtOnce(); most <= imageSlowHostFetches || most > imageHostFetches {
		t.Errorf("%d downloads at once from one host", most)
	}
	// A host that answers 503 is spared for a while.
	if _, err := h.fetchArtwork(t.Context(), art.URL+"/busy.jpg", false, false); err == nil {
		t.Error("a busy host answered")
	}
	art.mostAtOnce()
	burst(t, h, art, "spared", 12, false)
	if most := art.mostAtOnce(); most > imageSlowHostFetches {
		t.Errorf("%d downloads at once from a busy host", most)
	}
	// So is a host that serves an IPTV source.
	logos := newArtServer(t, jpegOf(t, 40, 40))
	burst(t, h, logos, "logo", 12, true)
	if most := logos.mostAtOnce(); most > imageSlowHostFetches {
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
