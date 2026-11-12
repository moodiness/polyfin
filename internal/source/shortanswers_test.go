package source

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// shortHost serves the file it holds with ranges, each answer telling the
// size it tells, but cut where cutAt says for the nth answer, counted from
// 1, or not when cutAt answers -1: a host whose file is shorter than it
// tells, or one closing connections early. It records the ranges asked.
type shortHost struct {
	mu       sync.Mutex
	data     []byte
	cutAt    func(n int) int
	answers  int
	ranges   []string
	requests atomic.Int32
}

func (h *shortHost) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.requests.Add(1)
	h.mu.Lock()
	h.answers++
	h.ranges = append(h.ranges, r.Header.Get("Range"))
	data, cut := h.data, h.cutAt(h.answers)
	h.mu.Unlock()
	start := 0
	if value, ok := strings.CutPrefix(r.Header.Get("Range"), "bytes="); ok {
		start, _ = strconv.Atoi(strings.Split(value, "-")[0])
	}
	if cut < 0 {
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
		return
	}
	http.ServeContent(&cutWriter{ResponseWriter: w, limit: max(cut-start, 0)}, r, "", time.Time{}, bytes.NewReader(data))
}

func (h *shortHost) asked() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.ranges)
}

// readToEnd reads a source as FFmpeg does over the loopback: one request
// from its start, read to the end of its body, which is all the reader
// gets of the file.
func readToEnd(t *testing.T, s *Source) (body []byte, length int64) {
	t.Helper()
	loopback := httptest.NewServer(s)
	defer loopback.Close()
	response, err := clientHTTP.Get(loopback.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ = io.ReadAll(response.Body)
	return body, response.ContentLength
}

// A host that tells a file's size, then closes every connection at the
// same byte, ends the file there: it is asked again from that byte, after
// a growing wait, and sends nothing each time. The reader gets the bytes
// before it, and reads past it end at once, without a request. The size
// told stays the file's: an answer of another size is still another
// file.
func TestAHostSendingLessThanItTellsEndsTheFileThere(t *testing.T) {
	t.Parallel()
	data := randomData(6*blockSize + 777)
	cut := 3*blockSize + 12345
	host := &shortHost{data: data, cutAt: func(int) int { return cut }}
	server := httptest.NewServer(host)
	t.Cleanup(server.Close)
	s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()

	body, _ := readToEnd(t, s)
	if !bytes.Equal(body, data[:cut]) {
		t.Fatalf("read %d bytes, want the %d before the cut", len(body), cut)
	}
	from := "bytes=" + strconv.Itoa(cut) + "-"
	asked := host.asked()
	if len(asked) != attempts || asked[0] != "bytes=0-" || slices.ContainsFunc(asked[1:], func(r string) bool { return r != from }) {
		t.Errorf("ranges asked: %v", asked)
	}
	if size, err := s.Size(t.Context()); size != int64(cut) || err != nil {
		t.Errorf("size %d, %v; want %d", size, err, cut)
	}
	if told, _ := s.KnownSize(); told != int64(len(data)) {
		t.Errorf("the size told is %d, want %d", told, len(data))
	}
	if s.Failure() != nil {
		t.Errorf("the source failed: %v", s.Failure())
	}

	// Read again, from the cache, past the cut too: nothing is asked.
	if n, err := s.ReadAt(t.Context(), make([]byte, 100), int64(cut)-50); n != 50 || !errors.Is(err, io.EOF) {
		t.Errorf("a read across the cut: %d bytes, %v", n, err)
	}
	if n, err := s.ReadAt(t.Context(), make([]byte, 100), int64(cut)+blockSize); n != 0 || !errors.Is(err, io.EOF) {
		t.Errorf("a read past the cut: %d bytes, %v", n, err)
	}
	body, length := readToEnd(t, s)
	if length != int64(cut) || !bytes.Equal(body, data[:cut]) {
		t.Errorf("read again: %d bytes of %d told, want %d", len(body), length, cut)
	}
	if got := host.requests.Load(); got != attempts {
		t.Errorf("the host was asked %d times, want %d", got, attempts)
	}
}

// A host closing connections early, but not at the same byte each time,
// or not at each try, is no file ending there: a host that then serves
// the rest has the whole file read, and one cutting at another byte each
// time fails the source as before.
func TestAPassingCutIsNoEnd(t *testing.T) {
	t.Parallel()
	data := randomData(6*blockSize + 777)
	cut := 3*blockSize + 12345
	for _, c := range []struct {
		name  string
		cutAt func(n int) int
	}{
		{"once", func(n int) int { return map[bool]int{true: cut, false: -1}[n == 1] }},
		{"at each try but the last", func(n int) int { return map[bool]int{true: cut, false: -1}[n < attempts] }},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			host := &shortHost{data: data, cutAt: c.cutAt}
			server := httptest.NewServer(host)
			t.Cleanup(server.Close)
			s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
			defer s.Release()
			body, length := readToEnd(t, s)
			if length != int64(len(data)) || !bytes.Equal(body, data) {
				t.Fatalf("read %d bytes of %d told, want %d", len(body), length, len(data))
			}
			if size, err := s.Size(t.Context()); size != int64(len(data)) || err != nil {
				t.Errorf("size %d, %v", size, err)
			}
		})
	}
	t.Run("at another byte each time", func(t *testing.T) {
		t.Parallel()
		host := &shortHost{data: data, cutAt: func(n int) int { return cut + n*1000 }}
		server := httptest.NewServer(host)
		t.Cleanup(server.Close)
		s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
		defer s.Release()
		r := s.NewReader()
		defer r.Close()
		var err error
		for at := int64(0); err == nil; at += 32 << 10 {
			_, err = r.ReadAt(t.Context(), make([]byte, 32<<10), at)
		}
		if !errors.Is(err, ErrUnavailable) || !errors.Is(s.Failure(), io.ErrUnexpectedEOF) || host.requests.Load() != attempts {
			t.Errorf("%v after %d requests; failure %v", err, host.requests.Load(), s.Failure())
		}
	})
}

// The end found where a host stops is kept with the source, which keeps
// one file of the size told: the same file read again ends there, with
// no request; once the source's file changed, the file put in its place,
// of another size, is read whole.
func TestACutIsForgottenWithItsFile(t *testing.T) {
	t.Parallel()
	data := randomData(6*blockSize + 777)
	cut := 3*blockSize + 12345
	host := &shortHost{data: data, cutAt: func(int) int { return cut }}
	server := httptest.NewServer(host)
	t.Cleanup(server.Close)
	cache := newCache(t, 1<<30)
	id := accounts.ID{1}
	s := cache.Open(id, Location{URL: server.URL + "/file", Size: int64(len(data))}, nil)
	if body, _ := readToEnd(t, s); len(body) != cut {
		t.Fatalf("read %d bytes, want %d", len(body), cut)
	}
	s.Release()
	s = cache.Open(id, Location{URL: server.URL + "/file", Size: int64(len(data))}, nil)
	if body, _ := readToEnd(t, s); len(body) != cut || host.requests.Load() != attempts {
		t.Fatalf("read again: %d bytes, %d requests", len(body), host.requests.Load())
	}
	s.Release()

	other := randomData(7*blockSize + 99)
	host.mu.Lock()
	host.data, host.cutAt = other, func(int) int { return -1 }
	host.mu.Unlock()
	if !cache.Discard(id) {
		t.Fatal("the source was kept")
	}
	s = cache.Open(id, Location{URL: server.URL + "/file", Size: int64(len(other))}, nil)
	defer s.Release()
	if body, length := readToEnd(t, s); length != int64(len(other)) || !bytes.Equal(body, other) {
		t.Errorf("the new file: %d bytes of %d told, want %d", len(body), length, len(other))
	}
}
