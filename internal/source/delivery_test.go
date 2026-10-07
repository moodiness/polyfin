package source

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
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

// paced serves a file with ranges the way debrid hosts do: each response
// after delay, at rate bytes a second, counting the responses and the most
// served at once. With refuse, a request made while another is served is
// answered refuse instead.
type paced struct {
	data   []byte
	rate   int
	delay  time.Duration
	refuse int

	requests, active, peak, refused atomic.Int32
}

func (p *paced) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.requests.Add(1)
	n := p.active.Add(1)
	defer p.active.Add(-1)
	for peak := p.peak.Load(); n > peak && !p.peak.CompareAndSwap(peak, n); peak = p.peak.Load() {
	}
	if p.refuse != 0 && n > 1 {
		p.refused.Add(1)
		http.Error(w, "one connection per file", p.refuse)
		return
	}
	time.Sleep(p.delay)
	http.ServeContent(throttled{w, p.rate}, r, "", time.Time{}, bytes.NewReader(p.data))
}

// throttled writes at rate bytes a second.
type throttled struct {
	http.ResponseWriter
	rate int
}

func (t throttled) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		chunk := p[:min(len(p), 16<<10)]
		n, err := t.ResponseWriter.Write(chunk)
		written += n
		if err != nil {
			return written, err
		}
		t.ResponseWriter.(http.Flusher).Flush()
		time.Sleep(time.Duration(n) * time.Second / time.Duration(t.rate))
		p = p[n:]
	}
	return written, nil
}

func randomData(size int) []byte {
	data := make([]byte, size)
	_, _ = rand.Read(data)
	return data
}

// streamAll reads a source from off to its end as FFmpeg does, as one
// streaming reader.
func streamAll(t *testing.T, s *Source, off int64) []byte {
	t.Helper()
	r := s.NewReader()
	defer r.Close()
	size, err := r.Size(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	result := make([]byte, size-off)
	for at := off; at < size; at += 32 << 10 {
		if _, err := r.ReadAt(t.Context(), result[at-off:min(at-off+32<<10, size-off)], at); err != nil {
			t.Fatal(err)
		}
	}
	return result
}

// A host that serves each connection at its own pace serves a file read
// ahead over several connections, faster than over one; the reader gets
// its bytes, in order.
func TestFilesAreReadOverSeveralConnections(t *testing.T) {
	host := &paced{data: randomData(32 * blockSize), rate: 16 << 20, delay: 20 * time.Millisecond}
	server := httptest.NewServer(host)
	t.Cleanup(server.Close)
	cache := newCache(t, 1<<30)
	s := cache.Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	started := time.Now()
	if got := streamAll(t, s, 0); !bytes.Equal(got, host.data) {
		t.Fatal("content differs")
	}
	took := time.Since(started)
	// One connection alone would take 2 s.
	if host.peak.Load() < 2 || took > 1800*time.Millisecond {
		t.Errorf("%d connections at most, in %v", host.peak.Load(), took)
	}
	if n := host.requests.Load(); n > 12 {
		t.Errorf("%d requests", n)
	}
}

// A host refusing a connection more, or asking to slow down, is read over
// one connection per file from then on: the refusal fails nothing.
func TestHostsRefusingConnectionsAreReadOverOne(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusForbidden} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			host := &paced{data: randomData(24 * blockSize), rate: 32 << 20, refuse: status}
			server := httptest.NewServer(host)
			t.Cleanup(server.Close)
			cache := newCache(t, 1<<30)
			s := cache.Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
			defer s.Release()
			if got := streamAll(t, s, 0); !bytes.Equal(got, host.data) {
				t.Fatal("content differs")
			}
			if host.refused.Load() == 0 || cache.connectionsFor(hostOf(server.URL)) != 1 {
				t.Fatalf("%d refused; %d connections allowed", host.refused.Load(), cache.connectionsFor(hostOf(server.URL)))
			}
			// Another file of the host is read over one connection.
			refused := host.refused.Load()
			other := cache.Open(accounts.ID{2}, Location{URL: server.URL + "/other"}, nil)
			defer other.Release()
			if got := streamAll(t, other, 0); !bytes.Equal(got, host.data) || host.refused.Load() != refused {
				t.Errorf("another file: %d more refused", host.refused.Load()-refused)
			}
		})
	}
}

// The newest reader is served first: a request FFmpeg is leaving, still
// waiting for the head of the file, does not pull the connection back
// while the new one reads; it is served once the new one's window is. The
// host is slower than the reads, so that the new reader always waits for
// its next block, as a playback does.
func TestTheNewestReaderIsServedFirst(t *testing.T) {
	o, server := newOrigin(t, 64*blockSize)
	o.mu.Lock()
	o.rate = 16 << 20
	o.mu.Unlock()
	cache := newCache(t, 1<<30)
	cache.connections, cache.readahead, cache.aheadBudget = 1, 2, 2
	s := cache.Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	old := s.NewReader()
	defer old.Close()
	if _, err := old.ReadAt(t.Context(), make([]byte, 10), 0); err != nil {
		t.Fatal(err)
	}
	newer := s.NewReader()
	defer newer.Close()
	if _, err := newer.ReadAt(t.Context(), make([]byte, 10), 40*blockSize); err != nil {
		t.Fatal(err)
	}
	// The old reader now wants a block far behind, as the new one reads on.
	stale := make(chan error, 1)
	go func() {
		_, err := old.ReadAt(t.Context(), make([]byte, 10), 20*blockSize)
		stale <- err
	}()
	for block := int64(40); block < 48; block++ {
		got := make([]byte, blockSize)
		if _, err := newer.ReadAt(t.Context(), got, block*blockSize); err != nil || !bytes.Equal(got, o.data[block*blockSize:(block+1)*blockSize]) {
			t.Fatalf("block %d: %v", block, err)
		}
	}
	if err := <-stale; err != nil {
		t.Fatal(err)
	}
	o.mu.Lock()
	ranges := slices.Clone(o.ranges)
	o.mu.Unlock()
	// The head, the new reader's stretch, then the old reader's block.
	if len(ranges) != 3 || ranges[1] != "bytes=41943040-" || ranges[2] != "bytes=20971520-" {
		t.Errorf("ranges asked: %q", ranges)
	}
}

// A reader that gives up stops wanting its blocks: they are not fetched.
func TestReadersGivingUpDropTheirBlocks(t *testing.T) {
	host := &paced{data: randomData(40 * blockSize), rate: 64 << 20, delay: 300 * time.Millisecond}
	server := httptest.NewServer(host)
	t.Cleanup(server.Close)
	cache := newCache(t, 1<<30)
	cache.connections = 1
	s := cache.Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	gone := s.NewReader()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err := gone.ReadAt(ctx, make([]byte, 10), 30*blockSize); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
	gone.Close()
	// Another reader reads the head meanwhile, then nothing more.
	if _, err := s.ReadAt(t.Context(), make([]byte, 10), 0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.has(30) || len(s.wanted) != 0 {
		t.Errorf("block 30 fetched %v; %d blocks wanted", s.has(30), len(s.wanted))
	}
}

// A streaming reader reads ahead by more as it reads on, up to the budget;
// the reads of an index, scattered, read nothing ahead.
func TestReadAheadGrowsWithTheStream(t *testing.T) {
	_, server := newOrigin(t, 200*blockSize)
	cache := newCache(t, 1<<30)
	cache.readahead, cache.aheadBudget = 4, 64
	s := cache.Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	if _, err := s.ReadAt(t.Context(), make([]byte, 10), 100*blockSize); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	window := s.window
	s.mu.Unlock()
	if window.owner != nil {
		t.Errorf("an index read moved the window: %+v", window)
	}
	r := s.NewReader()
	defer r.Close()
	horizon := func() int64 {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.window.horizon - s.window.cursor
	}
	if _, err := r.ReadAt(t.Context(), make([]byte, 10), 0); err != nil || horizon() != 5 {
		t.Fatalf("at the start, %d ahead: %v", horizon(), err)
	}
	for block := int64(1); block <= 20; block++ {
		if _, err := r.ReadAt(t.Context(), make([]byte, 10), block*blockSize); err != nil {
			t.Fatal(err)
		}
	}
	if got := horizon(); got != 41 {
		t.Errorf("20 blocks read, %d ahead", got)
	}
	for block := int64(21); block <= 60; block++ {
		if _, err := r.ReadAt(t.Context(), make([]byte, 10), block*blockSize); err != nil {
			t.Fatal(err)
		}
	}
	if got := horizon(); got != 65 {
		t.Errorf("60 blocks read, %d ahead", got)
	}
}

// An idle connection stays open longer while a streaming reader is
// attached, as FFmpeg pauses ahead of the player, than when nobody is.
func TestConnectionsLingerWhileAReaderIsAttached(t *testing.T) {
	_, server := newOrigin(t, 4*blockSize)
	cache := newCache(t, 1<<30)
	cache.linger, cache.attachedLinger = 50*time.Millisecond, time.Hour
	connections := func(s *Source) int {
		s.mu.Lock()
		defer s.mu.Unlock()
		return len(s.conns)
	}
	attached := cache.Open(accounts.ID{1}, Location{URL: server.URL + "/attached"}, nil)
	defer attached.Release()
	r := attached.NewReader()
	defer r.Close()
	if _, err := r.ReadAt(t.Context(), make([]byte, 10), 0); err != nil {
		t.Fatal(err)
	}
	alone := cache.Open(accounts.ID{2}, Location{URL: server.URL + "/alone"}, nil)
	defer alone.Release()
	if _, err := alone.ReadAt(t.Context(), make([]byte, 10), 0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	if connections(attached) != 1 || connections(alone) != 0 {
		t.Errorf("connections: %d attached, %d alone", connections(attached), connections(alone))
	}
}

// A connection that stops sending bytes is closed after a while and
// opened again, as one more try: the reader gets the file.
func TestStalledConnectionsAreOpenedAgain(t *testing.T) {
	data := randomData(3 * blockSize)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("Content-Range", "bytes 0-"+strconv.Itoa(len(data)-1)+"/"+strconv.Itoa(len(data)))
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(data[:100<<10])
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(server.Close)
	cache := newCache(t, 1<<30)
	cache.stallTimeout = 200 * time.Millisecond
	s := cache.Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	started := time.Now()
	if got := streamAll(t, s, 0); !bytes.Equal(got, data) {
		t.Fatal("content differs")
	}
	if took := time.Since(started); requests.Load() != 2 || took > 2*time.Second {
		t.Errorf("%d requests in %v", requests.Load(), took)
	}
}

// A host that accepts a connection and never answers fails the source
// within the header timeout, not the reader's deadline; what reads it
// learns at once, and why.
func TestSilentHostsFailTheSource(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	cache := newCache(t, 1<<30)
	cache.headerTimeout = 300 * time.Millisecond
	s := cache.Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	failed := s.Failed()
	started := time.Now()
	_, err := s.ReadAt(t.Context(), make([]byte, 10), 0)
	if !errors.Is(err, ErrUnavailable) || Answer(err) != "no answer in time" || time.Since(started) > 2*time.Second {
		t.Fatalf("%v (%q) after %v", err, Answer(err), time.Since(started))
	}
	select {
	case <-failed:
	default:
		t.Fatal("Failed is not closed")
	}
	if !errors.Is(s.Failure(), ErrUnavailable) || requests.Load() != 1 {
		t.Errorf("failure %v after %d requests", s.Failure(), requests.Load())
	}
	// A request of the loopback is answered 502 at once.
	loopback := httptest.NewServer(s)
	defer loopback.Close()
	response, err := http.Get(loopback.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadGateway || requests.Load() != 1 {
		t.Errorf("loopback: %d, %d requests", response.StatusCode, requests.Load())
	}
}

// A host that stops answering in the middle of a file, its connection
// silent and new ones unanswered, fails the source within seconds: the
// stall, then a short wait for a new connection's headers.
func TestHostsThatStopAnsweringFailWithinSeconds(t *testing.T) {
	data := randomData(8 * blockSize)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("Content-Range", "bytes 0-"+strconv.Itoa(len(data)-1)+"/"+strconv.Itoa(len(data)))
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(data[:2*blockSize])
			w.(http.Flusher).Flush()
		}
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	cache := newCache(t, 1<<30)
	cache.stallTimeout, cache.recoveryTimeout = 300*time.Millisecond, 200*time.Millisecond
	s := cache.Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	r := s.NewReader()
	defer r.Close()
	if _, err := r.ReadAt(t.Context(), make([]byte, 10), 0); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	_, err := r.ReadAt(t.Context(), make([]byte, 10), 5*blockSize)
	if !errors.Is(err, ErrUnavailable) || time.Since(started) > 2*time.Second {
		t.Fatalf("%v after %v", err, time.Since(started))
	}
	if requests.Load() != 2 {
		t.Errorf("%d requests", requests.Load())
	}
}

// switching serves a file, then, once switched, another of another size,
// as links answering another file do; renewed links too.
type switching struct {
	file, other []byte
	switched    atomic.Bool
	requests    atomic.Int32
	// unsatisfiable answers the other file's size with 416.
	unsatisfiable bool
}

func (sw *switching) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sw.requests.Add(1)
	data := sw.file
	if sw.switched.Load() {
		data = sw.other
		if sw.unsatisfiable {
			w.Header().Set("Content-Range", "bytes */"+strconv.Itoa(len(data)))
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
	}
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
}

// A link answering another file, as one now naming another, is asked again
// once, after a renewal, then fails the source at once: its readers learn
// why, and the other file's bytes are never read as the file's, nor its
// size taken, from a 206 or a 416.
func TestAnotherFileFailsAfterOneRetry(t *testing.T) {
	for _, unsatisfiable := range []bool{false, true} {
		t.Run("416 "+strconv.FormatBool(unsatisfiable), func(t *testing.T) {
			host := &switching{file: randomData(32 * blockSize), other: randomData(3 * blockSize), unsatisfiable: unsatisfiable}
			server := httptest.NewServer(host)
			t.Cleanup(server.Close)
			var renewals atomic.Int32
			s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: server.URL + "/old"}, func(context.Context) (Location, error) {
				renewals.Add(1)
				return Location{URL: server.URL + "/new"}, nil
			})
			defer s.Release()
			if _, err := s.ReadAt(t.Context(), make([]byte, 10), 0); err != nil {
				t.Fatal(err)
			}
			host.switched.Store(true)
			before := host.requests.Load()
			started := time.Now()
			_, err := s.ReadAt(t.Context(), make([]byte, 10), 20*blockSize)
			if !errors.Is(err, ErrUnavailable) || Answer(err) != "an answer of another size than the file" || time.Since(started) > 2*time.Second {
				t.Fatalf("%v (%q) after %v", err, Answer(err), time.Since(started))
			}
			if n := host.requests.Load() - before; n != 2 || renewals.Load() != 1 {
				t.Errorf("%d requests, %d renewals", n, renewals.Load())
			}
			if size, _ := s.KnownSize(); size != int64(len(host.file)) {
				t.Errorf("size %d", size)
			}
			select {
			case <-s.Failed():
			default:
				t.Error("Failed is not closed")
			}
		})
	}
}

// The size an addon announces catches another file at the first answer,
// once the addon's sizes proved right; an addon whose sizes are wrong
// refuses nothing. A size within 1% of the file's is the file's, as some
// addons' are a little off: the file's own size is taken.
func TestAnnouncedSizesCatchAnotherFile(t *testing.T) {
	_, server := newOrigin(t, 2*blockSize)
	cache := newCache(t, 1<<30)
	open := func(id byte, size int64, announcer string) *Source {
		s := cache.Open(accounts.ID{id}, Location{URL: server.URL + "/" + strconv.Itoa(int(id)), Size: size, Announcer: announcer}, nil)
		t.Cleanup(s.Release)
		return s
	}
	read := func(s *Source) error {
		_, err := s.ReadAt(t.Context(), make([]byte, 10), 0)
		return err
	}
	// Unknown yet: the answer is taken.
	if err := read(open(1, 3*blockSize, "wrong")); err != nil {
		t.Fatalf("an unknown addon's size refused a file: %v", err)
	}
	for id := byte(2); id < 6; id++ {
		if err := read(open(id, 2*blockSize-blockSize/10, "wrong")); err != nil {
			t.Fatalf("a wrong addon's size refused a file: %v", err)
		}
	}
	// Right once: within 1%, either way, a file is read, its own size
	// taken, and its addon's sizes trusted all the same.
	if err := read(open(10, 2*blockSize, "exact")); err != nil {
		t.Fatal(err)
	}
	for id, size := range map[byte]int64{11: 2*blockSize + 2*blockSize/100, 12: 2*blockSize - 1000} {
		near := open(id, size, "exact")
		if err := read(near); err != nil {
			t.Fatalf("announced %d bytes for %d: %v", size, 2*blockSize, err)
		}
		if known, _ := near.KnownSize(); known != 2*blockSize {
			t.Errorf("announced %d bytes: %d taken", size, known)
		}
	}
	// Further, another size is another file.
	for id, size := range map[byte]int64{20: 5 * blockSize, 21: 2*blockSize + 2*blockSize/50, 22: 2*blockSize - 2*blockSize/50} {
		other := open(id, size, "exact")
		err := read(other)
		if !errors.Is(err, ErrUnavailable) || Answer(err) != "an answer of another size than the file" {
			t.Fatalf("announced %d bytes for %d: %v", size, 2*blockSize, err)
		}
		if _, known := other.KnownSize(); known {
			t.Error("the other file's size was taken")
		}
	}
	if cache.sizeTrusted("wrong") {
		t.Error("wrong sizes are trusted")
	}
}

// resolver answers each request for /resolve with a redirect to /file
// after a while, as stream addons do in front of debrid hosts.
type resolver struct {
	hits, files atomic.Int32
	expired     atomic.Int32
	data        []byte
	headers     sync.Map
}

func (rs *resolver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/resolve" {
		rs.hits.Add(1)
		http.Redirect(w, r, "/file?token=t", http.StatusFound)
		return
	}
	rs.files.Add(1)
	rs.headers.Store(r.Header.Get("X-Key"), true)
	if rs.expired.Load() > 0 {
		rs.expired.Add(-1)
		http.Error(w, "expired", http.StatusForbidden)
		return
	}
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(rs.data))
}

// Where the origin redirected is kept for the server's own reads: the
// resolver is asked once, not per connection nor per Fetch; the headers
// go along. An address that stops working sends the reads back to the
// origin, before any renewal.
func TestRedirectTargetsArePinned(t *testing.T) {
	rs := &resolver{data: randomData(64 * blockSize)}
	server := httptest.NewServer(rs)
	t.Cleanup(server.Close)
	renewals := 0
	cache := newCache(t, 1<<30)
	cache.connections = 1
	s := cache.Open(accounts.ID{1}, Location{URL: server.URL + "/resolve", Headers: map[string]string{"X-Key": "k"}}, func(context.Context) (Location, error) {
		renewals++
		return Location{URL: server.URL + "/resolve"}, nil
	})
	defer s.Release()
	for _, off := range []int64{0, 40 * blockSize, 20 * blockSize} {
		got := make([]byte, 100)
		if _, err := s.ReadAt(t.Context(), got, off); err != nil || !bytes.Equal(got, rs.data[off:off+100]) {
			t.Fatalf("at %d: %v", off, err)
		}
	}
	if _, err := s.Fetch(t.Context(), 60*blockSize, 100); err != nil {
		t.Fatal(err)
	}
	if rs.hits.Load() != 1 || rs.files.Load() != 4 {
		t.Errorf("%d resolutions for %d requests", rs.hits.Load(), rs.files.Load())
	}
	if _, ok := rs.headers.Load("k"); !ok {
		t.Error("the headers were not sent")
	}
	if _, ok := rs.headers.Load(""); ok {
		t.Error("a request went without the headers")
	}
	// The address expires: the origin is asked again, nothing renewed.
	rs.expired.Store(1)
	if _, err := s.ReadAt(t.Context(), make([]byte, 10), 10*blockSize); err != nil {
		t.Fatal(err)
	}
	if rs.hits.Load() != 2 || renewals != 0 {
		t.Errorf("after expiring: %d resolutions, %d renewals", rs.hits.Load(), renewals)
	}
	// A new link replaces the address kept.
	again := cache.Open(accounts.ID{1}, Location{URL: server.URL + "/resolve?fresh"}, nil)
	defer again.Release()
	s.mu.Lock()
	pinned := s.pinned
	s.mu.Unlock()
	if pinned != nil {
		t.Error("a new link kept the old address")
	}
}

// Warm reads the spans asked into the cache: reading them after takes no
// request.
func TestWarmReadsSpans(t *testing.T) {
	o, server := newOrigin(t, 32*blockSize)
	s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	if err := s.Warm(t.Context(), Span{Off: 0, End: 3 * blockSize}, Span{Off: 20*blockSize + 10, End: 22 * blockSize}); err != nil {
		t.Fatal(err)
	}
	before := o.requests.Load()
	for _, off := range []int64{0, 2*blockSize + 100, 20 * blockSize, 22*blockSize - 10} {
		if _, err := s.ReadAt(t.Context(), make([]byte, 10), off); err != nil {
			t.Fatal(err)
		}
	}
	if o.requests.Load() != before {
		t.Errorf("%d requests after warming", o.requests.Load()-before)
	}
	// A failed source ends a warm at once.
	silent := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	t.Cleanup(silent.Close)
	cache := newCache(t, 1<<30)
	cache.headerTimeout = 100 * time.Millisecond
	gone := cache.Open(accounts.ID{2}, Location{URL: silent.URL + "/file"}, nil)
	defer gone.Release()
	if err := gone.Warm(t.Context(), Span{Off: 0, End: blockSize}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("a failed source: %v", err)
	}
}

// The cache tells when a source answered at a link, for the checks that a
// player sent there would make.
func TestAnsweredTellsWhenTheSourceAnswered(t *testing.T) {
	_, server := newOrigin(t, blockSize)
	cache := newCache(t, 1<<30)
	link := server.URL + "/file"
	if !cache.Answered(accounts.ID{1}, link).IsZero() {
		t.Error("a source never opened answered")
	}
	s := cache.Open(accounts.ID{1}, Location{URL: link}, nil)
	defer s.Release()
	if _, err := s.ReadAt(t.Context(), make([]byte, 10), 0); err != nil {
		t.Fatal(err)
	}
	if answered := cache.Answered(accounts.ID{1}, link); time.Since(answered) > time.Second {
		t.Errorf("answered at %v", answered)
	}
	if !cache.Answered(accounts.ID{1}, link+"?other").IsZero() {
		t.Error("another link answered")
	}
	// A streaming reader tells the source is being played.
	if cache.Streamed(accounts.ID{1}) {
		t.Error("an index read streams the source")
	}
	r := s.NewReader()
	if !cache.Streamed(accounts.ID{1}) {
		t.Error("a streaming reader does not stream the source")
	}
	r.Close()
	if cache.Streamed(accounts.ID{1}) || cache.Streamed(accounts.ID{2}) {
		t.Error("a closed reader still streams the source")
	}
}

// A request of the loopback for a source that fails while it is sent is
// held a moment before being cut, for what watches Failed to stop FFmpeg
// first: a body cut short reads as the end of the file. Failed is closed
// before the body is cut.
func TestLoopbackHoldsAFailingBody(t *testing.T) {
	data := randomData(4 * blockSize)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) > 1 {
			http.Error(w, "gone", http.StatusNotFound)
			return
		}
		// The first answer breaks after a block.
		w.Header().Set("Content-Range", "bytes 0-"+strconv.Itoa(len(data)-1)+"/"+strconv.Itoa(len(data)))
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(data[:blockSize])
	}))
	t.Cleanup(server.Close)
	s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	failed := s.Failed()
	loopback := httptest.NewServer(s)
	defer loopback.Close()
	response, err := http.Get(loopback.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	buffer := make([]byte, blockSize)
	if _, err := io.ReadFull(response.Body, buffer); err != nil || !bytes.Equal(buffer, data[:blockSize]) {
		t.Fatalf("the first block: %v", err)
	}
	_, err = io.Copy(io.Discard, response.Body)
	cut := time.Now()
	select {
	case <-failed:
	default:
		t.Fatal("cut before Failed was closed")
	}
	if err == nil {
		t.Error("the body was not cut")
	}
	s.mu.Lock()
	failedAt := s.failedAt
	s.mu.Unlock()
	if cut.Sub(failedAt) < failedHold*9/10 {
		t.Errorf("cut %v after the failure", cut.Sub(failedAt))
	}
}
