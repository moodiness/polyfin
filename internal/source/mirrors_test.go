package source

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// cutWriter cuts the answer it writes once it wrote limit bytes, as a host
// going down mid-read does.
type cutWriter struct {
	http.ResponseWriter
	limit int
}

func (w *cutWriter) Write(p []byte) (int, error) {
	if len(p) > w.limit {
		n, _ := w.ResponseWriter.Write(p[:w.limit])
		w.limit -= n
		w.ResponseWriter.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	}
	n, err := w.ResponseWriter.Write(p)
	w.limit -= n
	return n, err
}

// dyingHost serves data until it sent cut bytes, then cuts the answer and
// refuses every request after it, its connections closed at once.
func dyingHost(t *testing.T, data []byte, cut int) *httptest.Server {
	t.Helper()
	var dead atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dead.Load() {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return
		}
		dead.Store(true)
		http.ServeContent(&cutWriter{ResponseWriter: w, limit: cut}, r, "", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(server.Close)
	return server
}

// copyHost serves data and records the ranges asked of it.
func copyHost(t *testing.T, data []byte) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var ranges []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		ranges = append(ranges, r.Header.Get("Range"))
		mu.Unlock()
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(server.Close)
	return server, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(ranges)
	}
}

// A source whose host goes down mid-read reads on from its mirror, from
// where the read stopped: the reader gets the whole file, and the mirror
// is asked only for what was not read yet. Opened again with the location
// that failed, the source stays on the mirror.
func TestAFailedSourceReadsOnFromItsMirror(t *testing.T) {
	data := make([]byte, 6*blockSize+12345)
	_, _ = rand.Read(data)
	first := dyingHost(t, data, 2*blockSize+100)
	mirror, ranges := copyHost(t, data)
	cache := newCache(t, 1<<30)
	location := Location{URL: first.URL + "/file", Mirrors: []Mirror{{Location: Location{URL: mirror.URL + "/copy"}}}}
	s := cache.Open(accounts.ID{1}, location, nil)
	defer s.Release()
	if got := readAll(t, s); !bytes.Equal(got, data) {
		t.Fatal("content differs")
	}
	asked := ranges()
	if len(asked) == 0 || asked[0] != "bytes="+strconv.Itoa(2*blockSize)+"-" {
		t.Errorf("ranges asked of the mirror: %q", asked)
	}
	select {
	case <-s.Failed():
		t.Error("the source failed")
	default:
	}
	again := cache.Open(accounts.ID{1}, location, nil)
	defer again.Release()
	if target := again.target(); target.url != mirror.URL+"/copy" {
		t.Errorf("opened again, the source reads %s", target.url)
	}
}

// Without a mirror, the same host going down fails the read: the mirror
// is what carries it on.
func TestAFailedSourceWithoutMirrorFails(t *testing.T) {
	data := make([]byte, 6*blockSize+12345)
	_, _ = rand.Read(data)
	first := dyingHost(t, data, 2*blockSize+100)
	s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: first.URL + "/file"}, nil)
	defer s.Release()
	if readUntilError(t, s, len(data)) == nil {
		t.Fatal("a read past the cut succeeded")
	}
	select {
	case <-s.Failed():
	default:
		t.Error("the source did not fail")
	}
}

// A mirror that is another file, of another size, is no copy: it fails
// in turn, and the source fails once no mirror is left.
func TestAMirrorOfAnotherSizeFails(t *testing.T) {
	data := make([]byte, 6*blockSize+12345)
	_, _ = rand.Read(data)
	first := dyingHost(t, data, 2*blockSize+100)
	other, _ := copyHost(t, data[:5*blockSize])
	s := newCache(t, 1<<30).Open(accounts.ID{1},
		Location{URL: first.URL + "/file", Mirrors: []Mirror{{Location: Location{URL: other.URL + "/copy"}}}}, nil)
	defer s.Release()
	if readUntilError(t, s, len(data)) == nil {
		t.Fatal("another file was read as the mirror")
	}
	select {
	case <-s.Failed():
	default:
		t.Error("the source did not fail")
	}
}

// readUntilError reads a source of size bytes from its start, as readAll
// does, and returns the first error.
func readUntilError(t *testing.T, s *Source, size int) error {
	t.Helper()
	buffer := make([]byte, 32<<10)
	for off := 0; off < size; off += len(buffer) {
		if _, err := s.ReadAt(t.Context(), buffer[:min(len(buffer), size-off)], int64(off)); err != nil {
			return err
		}
	}
	return nil
}

// Two connections of a source failing at once, as when its host goes
// down: the first moves the source to its mirror and drops the other,
// whose failure, about the copy left, then fails nothing.
func TestAConnectionOfTheCopyLeftFailsNothing(t *testing.T) {
	s := newCache(t, 1<<30).Open(accounts.ID{1},
		Location{URL: "http://192.0.2.1/file", Mirrors: []Mirror{{Location: Location{URL: "http://192.0.2.2/copy"}}}}, nil)
	defer s.Release()
	first, second := newConn(), newConn()
	s.mu.Lock()
	s.conns = []*conn{first, second}
	s.mu.Unlock()
	err := fmt.Errorf("%w: connection refused", ErrUnavailable)
	s.fail(first, err)
	s.fail(second, err)
	select {
	case <-s.Failed():
		t.Error("the source failed")
	default:
	}
	if target := s.target(); target.url != "http://192.0.2.2/copy" {
		t.Errorf("the source reads %s", target.url)
	}
}
