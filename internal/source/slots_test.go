package source

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// crowdedHost serves files by path the way a debrid proxy does, at rate
// bytes a second each, counting the answers served at once and the most.
// With limit, a connection made while limit others are served is answered
// 429, asking to retry after a second.
type crowdedHost struct {
	files map[string][]byte
	rate  int
	limit int32

	requests, active, peak, refused atomic.Int32
}

func (h *crowdedHost) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.requests.Add(1)
	n := h.active.Add(1)
	defer h.active.Add(-1)
	if h.limit > 0 && n > h.limit {
		h.refused.Add(1)
		w.Header().Set("Retry-After", "1")
		http.Error(w, "too many connections", http.StatusTooManyRequests)
		return
	}
	for peak := h.peak.Load(); n > peak && !h.peak.CompareAndSwap(peak, n); peak = h.peak.Load() {
	}
	http.ServeContent(throttled{w, h.rate}, r, "", time.Time{}, bytes.NewReader(h.files[r.URL.Path]))
}

// waitFor waits for cond, a few seconds at most.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !cond(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("waited for %s", what)
		}
	}
}

// warmAll warms the first blocks of each source in the background, the
// errors sent on the channel returned.
func warmAll(t *testing.T, blocks int64, sources ...*Source) <-chan error {
	t.Helper()
	errs := make(chan error, len(sources))
	for _, s := range sources {
		go func() {
			errs <- s.Warm(t.Context(), Span{Off: 0, End: blocks * blockSize})
		}()
	}
	return errs
}

// A host answering 429 to a connection more than two, as a debrid proxy
// does to a burst, fails nothing: the playback and its seek get their
// bytes once the host is asked again, a background read giving way, and
// the warms finish too. The warms last longer than the tries a playback
// makes: without giving way, it would fail.
func TestHostsAskingToSlowDownFailNothing(t *testing.T) {
	host := &crowdedHost{files: map[string][]byte{"/a": randomData(24 * blockSize), "/b": randomData(24 * blockSize),
		"/played": randomData(32 * blockSize)}, rate: 4 << 20, limit: 2}
	server := httptest.NewServer(host)
	t.Cleanup(server.Close)
	cache := newCache(t, 1<<30)
	open := func(id byte, path string) *Source {
		s := cache.Open(accounts.ID{id}, Location{URL: server.URL + path}, nil)
		t.Cleanup(s.Release)
		return s
	}
	a, b, played := open(1, "/a"), open(2, "/b"), open(3, "/played")
	warms := warmAll(t, 24, a, b)
	waitFor(t, "the warms to read", func() bool { return host.active.Load() == 2 })
	// The playback's connection is one too many: the host refuses it.
	done := played.Urge()
	r := played.NewReader()
	head := make([]byte, 2*blockSize)
	if _, err := r.ReadAt(t.Context(), head, 0); err != nil || !bytes.Equal(head, host.files["/played"][:2*blockSize]) {
		t.Fatalf("the head: %v", err)
	}
	// A seek, as a reader of its own.
	seek := played.NewReader()
	far := make([]byte, 2*blockSize)
	if _, err := seek.ReadAt(t.Context(), far, 28*blockSize+123); err != nil || !bytes.Equal(far, host.files["/played"][28*blockSize+123:30*blockSize+123]) {
		t.Fatalf("the seek: %v", err)
	}
	if host.refused.Load() == 0 {
		t.Error("the host refused nothing")
	}
	if played.Failure() != nil || !played.HeldBack(time.Time{}) {
		t.Errorf("failure %v", played.Failure())
	}
	seek.Close()
	r.Close()
	done()
	for range 2 {
		if err := <-warms; err != nil {
			t.Errorf("a warm: %v", err)
		}
	}
	for _, s := range []*Source{a, b} {
		got := make([]byte, 24*blockSize)
		if _, err := s.ReadAt(t.Context(), got, 0); err != nil || !bytes.Equal(got, host.files[hostPath(s)]) {
			t.Errorf("%s: %v", hostPath(s), err)
		}
	}
	// The host is given two connections for a while.
	if limit := cache.slotCap(hostOf(server.URL)); limit != 2 {
		t.Errorf("%d connections allowed", limit)
	}
}

// hostPath is the path a source's origin names.
func hostPath(s *Source) string {
	return s.origin.URL[len("http://")+len(hostOf(s.origin.URL)):]
}

// The connections to a host are bounded: background reads wait for a slot,
// and give theirs up to a playback's seek, served at once; they finish
// once it is done.
func TestBackgroundReadsGiveWayToPlaybacks(t *testing.T) {
	host := &crowdedHost{files: map[string][]byte{"/a": randomData(8 * blockSize), "/b": randomData(8 * blockSize),
		"/c": randomData(8 * blockSize), "/played": randomData(16 * blockSize)}, rate: 4 << 20}
	server := httptest.NewServer(host)
	t.Cleanup(server.Close)
	cache := newCache(t, 1<<30)
	cache.hostConnections = 2
	open := func(id byte, path string) *Source {
		s := cache.Open(accounts.ID{id}, Location{URL: server.URL + path}, nil)
		t.Cleanup(s.Release)
		return s
	}
	a, b, c, played := open(1, "/a"), open(2, "/b"), open(3, "/c"), open(4, "/played")
	warms := warmAll(t, 8, a, b)
	waitFor(t, "the warms to read", func() bool { return host.active.Load() == 2 })
	// A third waits for a slot.
	waiting := time.Now()
	more := warmAll(t, 8, c)
	time.Sleep(300 * time.Millisecond)
	if host.peak.Load() > 2 || !c.HeldBack(waiting) {
		t.Fatalf("%d connections at once; the third held back: %v", host.peak.Load(), c.HeldBack(waiting))
	}
	// A playback's seek is served at once: a block takes 250 ms.
	done := played.Urge()
	seek := played.NewReader()
	started := time.Now()
	got := make([]byte, 100)
	if _, err := seek.ReadAt(t.Context(), got, 9*blockSize+5); err != nil || !bytes.Equal(got, host.files["/played"][9*blockSize+5:9*blockSize+105]) {
		t.Fatalf("the seek: %v", err)
	}
	if took := time.Since(started); took > 1500*time.Millisecond {
		t.Errorf("the seek took %v", took)
	}
	if peak := host.peak.Load(); peak > 3 {
		t.Errorf("%d connections at once", peak)
	}
	seek.Close()
	done()
	for range 2 {
		if err := <-warms; err != nil {
			t.Errorf("a warm: %v", err)
		}
	}
	if err := <-more; err != nil {
		t.Errorf("the third warm: %v", err)
	}
	for _, s := range []*Source{a, b, c} {
		got := make([]byte, 8*blockSize)
		if _, err := s.ReadAt(t.Context(), got, 0); err != nil || !bytes.Equal(got, host.files[hostPath(s)]) {
			t.Errorf("%s: %v", hostPath(s), err)
		}
	}
}
