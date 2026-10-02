package source

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// origin serves a file the way debrid servers do, counting the requests
// (each one a connection) and misbehaving as asked.
type origin struct {
	data     []byte
	requests atomic.Int32
	// expired paths answer 403; busy makes the next requests answer 429;
	// rangeless ignores ranges and hides the size.
	mu        sync.Mutex
	expired   map[string]bool
	busy      int
	rangeless bool
}

func newOrigin(t *testing.T, size int) (*origin, *httptest.Server) {
	t.Helper()
	o := &origin{data: make([]byte, size), expired: map[string]bool{}}
	_, _ = rand.Read(o.data)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		o.requests.Add(1)
		o.mu.Lock()
		expired, busy, rangeless := o.expired[r.URL.Path], o.busy > 0, o.rangeless
		if busy {
			o.busy--
		}
		o.mu.Unlock()
		switch {
		case expired:
			http.Error(w, "link expired", http.StatusForbidden)
		case busy:
			w.Header().Set("Retry-After", "0")
			http.Error(w, "slow down", http.StatusTooManyRequests)
		case rangeless:
			// Chunked, without a length: the size is learned at the end.
			for chunk := range slices.Chunk(o.data, 64<<10) {
				_, _ = w.Write(chunk)
				w.(http.Flusher).Flush()
			}
		default:
			http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(o.data))
		}
	}))
	t.Cleanup(server.Close)
	return o, server
}

type client struct{}

func (client) Open(ctx context.Context, method, target string, header http.Header, _ bool) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return nil, err
	}
	request.Header = header
	return http.DefaultClient.Do(request)
}

func newCache(t *testing.T, limit int64) *Cache {
	t.Helper()
	cache, err := New(t.TempDir(), limit, client{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	return cache
}

// readAll reads a whole source in small reads, as FFmpeg does.
func readAll(t *testing.T, s *Source) []byte {
	t.Helper()
	size, err := s.Size(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	result := make([]byte, size)
	for off := int64(0); off < size; off += 32 << 10 {
		if _, err := s.ReadAt(t.Context(), result[off:min(off+32<<10, size)], off); err != nil {
			t.Fatal(err)
		}
	}
	return result
}

func TestSequentialReadsUseOneConnection(t *testing.T) {
	o, server := newOrigin(t, 5*blockSize+12345)
	s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	if got := readAll(t, s); !bytes.Equal(got, o.data) {
		t.Fatal("content differs")
	}
	if n := o.requests.Load(); n != 1 {
		t.Errorf("requests: %d", n)
	}
}

func TestSeeksReadCachedBlocksAgain(t *testing.T) {
	o, server := newOrigin(t, 80*blockSize)
	s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	read := func(off int64) {
		t.Helper()
		got := make([]byte, 1000)
		if _, err := s.ReadAt(t.Context(), got, off); err != nil || !bytes.Equal(got, o.data[off:off+1000]) {
			t.Fatalf("at %d: %v", off, err)
		}
	}
	// Far apart: a new connection each.
	read(70 * blockSize)
	read(5)
	before := o.requests.Load()
	// Both are cached now: no request.
	read(70*blockSize + 10)
	read(100)
	if after := o.requests.Load(); before != 2 || after != before {
		t.Errorf("requests: %d then %d", before, after)
	}
}

func TestExpiredLinksAreRenewed(t *testing.T) {
	o, server := newOrigin(t, 3*blockSize)
	o.expired["/old"] = true
	var renewals atomic.Int32
	renew := func(context.Context) (Location, error) {
		renewals.Add(1)
		return Location{URL: server.URL + "/new"}, nil
	}
	s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: server.URL + "/old"}, renew)
	defer s.Release()
	if got := readAll(t, s); !bytes.Equal(got, o.data) {
		t.Fatal("content differs")
	}
	if renewals.Load() != 1 {
		t.Errorf("renewals: %d", renewals.Load())
	}
}

func TestBusySourcesAreRetried(t *testing.T) {
	o, server := newOrigin(t, blockSize)
	o.busy = 2
	s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	if got := readAll(t, s); !bytes.Equal(got, o.data) {
		t.Fatal("content differs")
	}
	// A source still refusing is reported, and not asked again at once.
	o.busy = 100
	other := s.cache.Open(accounts.ID{2}, Location{URL: server.URL + "/other"}, nil)
	defer other.Release()
	if _, err := other.Size(t.Context()); err == nil {
		t.Fatal("a refusing source was read")
	}
	before := o.requests.Load()
	if _, err := other.ReadAt(t.Context(), make([]byte, 10), 0); err == nil || o.requests.Load() != before {
		t.Errorf("after a failure: %v, %d requests", err, o.requests.Load()-before)
	}
}

func TestSourcesIgnoringRangesAreReadThrough(t *testing.T) {
	o, server := newOrigin(t, 4*blockSize+777)
	o.rangeless = true
	s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	got := make([]byte, 2000)
	off := int64(3*blockSize + 100)
	if _, err := s.ReadAt(t.Context(), got, off); err != nil || !bytes.Equal(got, o.data[off:off+2000]) {
		t.Fatalf("read past the start: %v", err)
	}
	// The end of the file tells its size.
	if size, err := s.Size(t.Context()); err != nil || size != int64(len(o.data)) {
		t.Errorf("size: %d %v", size, err)
	}
	if n, err := s.ReadAt(t.Context(), make([]byte, 10), int64(len(o.data))-5); n != 5 || err != io.EOF {
		t.Errorf("read across the end: %d %v", n, err)
	}
}

// testClock is a clock the test moves forward.
type testClock struct{ at atomic.Int64 }

func (c *testClock) now() time.Time          { return time.Unix(0, c.at.Load()) }
func (c *testClock) advance(d time.Duration) { c.at.Add(int64(d)) }

// smallCache keeps one block per chunk, reads one block ahead, and dates
// chunks by a test clock.
func smallCache(t *testing.T, limit int64) (*Cache, *testClock) {
	t.Helper()
	cache := newCache(t, limit)
	clock := &testClock{}
	clock.at.Store(time.Now().UnixNano())
	cache.chunkBlocks, cache.readahead, cache.now = 1, 1, clock.now
	return cache, clock
}

func (c *Cache) chunkExists(id accounts.ID, index int64) bool {
	_, err := os.Stat(filepath.Join(c.dir, id.String()+".blocks", strconv.FormatInt(index, 10)))
	return err == nil
}

func TestLongReadsStayWithinTheLimit(t *testing.T) {
	o, server := newOrigin(t, 10*blockSize)
	cache, clock := smallCache(t, 3*blockSize)
	s := cache.Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	// A minute between reads: what was read long ago makes room.
	for block := range int64(10) {
		got := make([]byte, 100)
		if _, err := s.ReadAt(t.Context(), got, block*blockSize); err != nil || !bytes.Equal(got, o.data[block*blockSize:block*blockSize+100]) {
			t.Fatalf("block %d: %v", block, err)
		}
		clock.advance(time.Minute)
	}
	cache.mu.Lock()
	used := cache.used
	cache.mu.Unlock()
	if used > 3*blockSize || cache.chunkExists(accounts.ID{1}, 0) || !cache.chunkExists(accounts.ID{1}, 9) {
		t.Errorf("cached %d bytes; first chunk kept %v, last %v", used, cache.chunkExists(accounts.ID{1}, 0), cache.chunkExists(accounts.ID{1}, 9))
	}
	// An evicted block is fetched again.
	got := make([]byte, 100)
	if _, err := s.ReadAt(t.Context(), got, 50); err != nil || !bytes.Equal(got, o.data[50:150]) {
		t.Errorf("evicted block read again: %v", err)
	}
}

func TestBlocksReadLatelyAreKept(t *testing.T) {
	o, server := newOrigin(t, 4*blockSize)
	cache, _ := smallCache(t, blockSize)
	s := cache.Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	if got := readAll(t, s); !bytes.Equal(got, o.data) {
		t.Fatal("content differs")
	}
	// The limit is exceeded rather than evicting what readers are on.
	for index := range int64(4) {
		if !cache.chunkExists(accounts.ID{1}, index) {
			t.Errorf("chunk %d was evicted while read", index)
		}
	}
}

func TestIdleSourcesLeaveWithTheirLastChunk(t *testing.T) {
	_, server := newOrigin(t, 2*blockSize)
	cache, clock := smallCache(t, 2*blockSize)
	first := cache.Open(accounts.ID{1}, Location{URL: server.URL + "/first"}, nil)
	readAll(t, first)
	first.Release()
	clock.advance(time.Hour)
	second := cache.Open(accounts.ID{2}, Location{URL: server.URL + "/second"}, nil)
	defer second.Release()
	readAll(t, second)
	cache.mu.Lock()
	_, kept := cache.sources[accounts.ID{1}]
	cache.mu.Unlock()
	if _, err := os.Stat(filepath.Join(cache.dir, accounts.ID{1}.String()+".blocks")); kept || !os.IsNotExist(err) {
		t.Errorf("the idle source is still cached: %v %v", kept, err)
	}
}

func TestSourcesAreServedWithRanges(t *testing.T) {
	o, server := newOrigin(t, 3*blockSize)
	s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	loopback := httptest.NewServer(s)
	defer loopback.Close()
	request, _ := http.NewRequest(http.MethodGet, loopback.URL, nil)
	request.Header.Set("Range", "bytes=1048570-1048590")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusPartialContent || !bytes.Equal(body, o.data[1048570:1048591]) ||
		response.Header.Get("Content-Range") != "bytes 1048570-1048590/3145728" {
		t.Errorf("%d %s %d bytes", response.StatusCode, response.Header.Get("Content-Range"), len(body))
	}
}

func TestStaleBlocksAreRemovedAtStart(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "0123.blocks")
	kept := filepath.Join(dir, "notes.txt")
	for _, file := range []string{stale, kept} {
		if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := New(dir, 1<<20, client{}, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("stale blocks were kept")
	}
	if _, err := os.Stat(kept); err != nil {
		t.Error("a file Polyfin did not write was removed")
	}
}
