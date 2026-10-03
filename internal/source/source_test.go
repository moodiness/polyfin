package source

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/container"
)

// A Source reads subtitle blocks for the container package.
var _ container.RangeFetcher = (*Source)(nil)

// origin serves a file the way debrid servers do, counting the requests
// (each one a connection) and misbehaving as asked.
type origin struct {
	data     []byte
	requests atomic.Int32
	// expired paths answer 403; busy makes the next requests answer 429,
	// asking to retry after retryAfter seconds, and failing the next ones
	// 502; rangeless ignores ranges and hides the size. ranges holds the
	// ranges asked, and times when.
	mu         sync.Mutex
	expired    map[string]bool
	busy       int
	failing    int
	retryAfter string
	rangeless  bool
	ranges     []string
	times      []time.Time
}

func newOrigin(t *testing.T, size int) (*origin, *httptest.Server) {
	t.Helper()
	o := &origin{data: make([]byte, size), expired: map[string]bool{}, retryAfter: "0"}
	_, _ = rand.Read(o.data)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		o.requests.Add(1)
		o.mu.Lock()
		expired, busy, failing, rangeless, retryAfter := o.expired[r.URL.Path], o.busy > 0, o.failing > 0, o.rangeless, o.retryAfter
		o.ranges = append(o.ranges, r.Header.Get("Range"))
		o.times = append(o.times, time.Now())
		if busy {
			o.busy--
		} else if failing {
			o.failing--
		}
		o.mu.Unlock()
		switch {
		case expired:
			http.Error(w, "link expired", http.StatusForbidden)
		case busy:
			w.Header().Set("Retry-After", retryAfter)
			http.Error(w, "slow down", http.StatusTooManyRequests)
		case failing:
			http.Error(w, "bad gateway", http.StatusBadGateway)
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

func TestFetchReadsTheRangeAsked(t *testing.T) {
	o, server := newOrigin(t, 3*blockSize)
	s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	got, err := s.Fetch(t.Context(), 1000, 5000)
	if err != nil || !bytes.Equal(got, o.data[1000:6000]) {
		t.Fatalf("%d bytes: %v", len(got), err)
	}
	// At the end of the file, what it holds.
	end := int64(len(o.data))
	got, err = s.Fetch(t.Context(), end-10, 100)
	if err != nil || !bytes.Equal(got, o.data[end-10:]) {
		t.Fatalf("at the end: %d bytes: %v", len(got), err)
	}
	if !slices.Equal(o.ranges, []string{"bytes=1000-5999", "bytes=3145718-3145817"}) {
		t.Errorf("ranges asked: %q", o.ranges)
	}
	// The size was learned: past the end needs no request.
	if _, err := s.Fetch(t.Context(), end, 10); err != io.EOF || o.requests.Load() != 2 {
		t.Errorf("past the end: %v after %d requests", err, o.requests.Load())
	}
	// Nothing was cached: the reads still fetch.
	if _, err := s.ReadAt(t.Context(), make([]byte, 10), 1000); err != nil || o.requests.Load() != 3 {
		t.Errorf("read: %v after %d requests", err, o.requests.Load())
	}
}

func TestFetchReportsIgnoredRanges(t *testing.T) {
	o, server := newOrigin(t, blockSize)
	o.rangeless = true
	s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	if _, err := s.Fetch(t.Context(), 1000, 10); !errors.Is(err, ErrRangesIgnored) {
		t.Errorf("got %v, want ErrRangesIgnored", err)
	}
}

func TestFetchWaitsForBusySources(t *testing.T) {
	o, server := newOrigin(t, blockSize)
	o.busy, o.retryAfter = 1, "1"
	s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	started := time.Now()
	got, err := s.Fetch(t.Context(), 10, 20)
	if err != nil || !bytes.Equal(got, o.data[10:30]) {
		t.Fatalf("%d bytes: %v", len(got), err)
	}
	if waited := time.Since(started); waited < time.Second || o.requests.Load() != 2 {
		t.Errorf("%d requests in %v, want 2 a second apart", o.requests.Load(), waited)
	}
	// A source still refusing is given up on, after a bounded number of
	// tries.
	o.busy, o.retryAfter = 100, "0"
	before := o.requests.Load()
	if _, err := s.Fetch(t.Context(), 10, 20); !errors.Is(err, ErrUnavailable) || o.requests.Load()-before != attempts {
		t.Errorf("got %v after %d requests", err, o.requests.Load()-before)
	}
}

func TestFetchRenewsExpiredLinks(t *testing.T) {
	o, server := newOrigin(t, blockSize)
	o.expired["/old"] = true
	var renewals atomic.Int32
	renew := func(context.Context) (Location, error) {
		renewals.Add(1)
		return Location{URL: server.URL + "/new"}, nil
	}
	s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: server.URL + "/old"}, renew)
	defer s.Release()
	got, err := s.Fetch(t.Context(), 100, 200)
	if err != nil || !bytes.Equal(got, o.data[100:300]) {
		t.Fatalf("%d bytes: %v", len(got), err)
	}
	// The fresh link is kept for the next requests.
	if _, err := s.Fetch(t.Context(), 0, 10); err != nil || renewals.Load() != 1 || o.requests.Load() != 3 {
		t.Errorf("%v: %d renewals, %d requests", err, renewals.Load(), o.requests.Load())
	}
}

func TestFetchRetriesFailingSources(t *testing.T) {
	o, server := newOrigin(t, blockSize)
	o.failing = 2
	s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	started := time.Now()
	got, err := s.Fetch(t.Context(), 10, 20)
	if err != nil || !bytes.Equal(got, o.data[10:30]) {
		t.Fatalf("%d bytes: %v", len(got), err)
	}
	// Without Retry-After, waits of 0.5 then 1 s.
	if waited := time.Since(started); o.requests.Load() != 3 || waited < 1400*time.Millisecond {
		t.Errorf("%d requests in %v", o.requests.Load(), waited)
	}
}

func TestFetchesArePaced(t *testing.T) {
	o, server := newOrigin(t, blockSize)
	cache := newCache(t, 1<<30)
	cache.fetchInterval = 40 * time.Millisecond
	s := cache.Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Go(func() {
			if i%2 == 0 {
				if _, err := s.Fetch(t.Context(), int64(i)*100, 10); err != nil {
					t.Error(err)
				}
			} else if _, err := s.FetchRanges(t.Context(), []container.Range{{Off: int64(i) * 100, N: 10}, {Off: int64(i)*100 + 50, N: 10}}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	o.mu.Lock()
	times := slices.Clone(o.times)
	o.mu.Unlock()
	slices.SortFunc(times, func(a, b time.Time) int { return a.Compare(b) })
	if len(times) != 10 || times[9].Sub(times[0]) < 9*cache.fetchInterval*8/10 {
		t.Fatalf("%d requests in %v", len(times), times[len(times)-1].Sub(times[0]))
	}
	for i := 1; i < len(times); i++ {
		if gap := times[i].Sub(times[i-1]); gap < cache.fetchInterval/2 {
			t.Errorf("requests %v apart", gap)
		}
	}
}

func TestFetchRangesReadsParts(t *testing.T) {
	o, server := newOrigin(t, 3*blockSize)
	s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	// Out of order, and the last one past the end of the file.
	end := int64(len(o.data))
	ranges := []container.Range{{Off: 100, N: 50}, {Off: 2 * blockSize, N: 4096}, {Off: 10, N: 20}, {Off: end - 10, N: 100}}
	got, err := s.FetchRanges(t.Context(), ranges)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]byte{o.data[100:150], o.data[2*blockSize : 2*blockSize+4096], o.data[10:30], o.data[end-10:]}
	if !slices.EqualFunc(got, want, bytes.Equal) {
		t.Errorf("got %d ranges", len(got))
	}
	if !slices.Equal(o.ranges, []string{"bytes=100-149,2097152-2101247,10-29,3145718-3145817"}) {
		t.Errorf("ranges asked: %q", o.ranges)
	}
	// Ranges past the known end are empty, and not asked.
	got, err = s.FetchRanges(t.Context(), []container.Range{{Off: 0, N: 10}, {Off: end, N: 10}, {Off: 20, N: 10}})
	if err != nil || !slices.EqualFunc(got, [][]byte{o.data[:10], {}, o.data[20:30]}, bytes.Equal) {
		t.Errorf("past the end: %v", err)
	}
	if o.ranges[len(o.ranges)-1] != "bytes=0-9,20-29" {
		t.Errorf("ranges asked: %q", o.ranges)
	}
}

// partsServer answers every request with the parts given, as a multipart
// body of byte ranges of data, or a single part without a multipart body.
func partsServer(t *testing.T, data []byte, parts ...[2]int) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentRange := func(part [2]int) string {
			return "bytes " + strconv.Itoa(part[0]) + "-" + strconv.Itoa(part[1]) + "/" + strconv.Itoa(len(data))
		}
		if len(parts) == 1 {
			w.Header().Set("Content-Range", contentRange(parts[0]))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(data[parts[0][0] : parts[0][1]+1])
			return
		}
		body := multipart.NewWriter(w)
		w.Header().Set("Content-Type", "multipart/byteranges; boundary="+body.Boundary())
		w.WriteHeader(http.StatusPartialContent)
		for _, part := range parts {
			writer, err := body.CreatePart(textproto.MIMEHeader{
				"Content-Type": {"video/x-matroska"}, "Content-Range": {contentRange(part)}})
			if err != nil {
				return
			}
			_, _ = writer.Write(data[part[0] : part[1]+1])
		}
		_ = body.Close()
	}))
	t.Cleanup(server.Close)
	return server
}

func TestFetchRangesReadsMergedParts(t *testing.T) {
	data := make([]byte, 64<<10)
	_, _ = rand.Read(data)
	ranges := []container.Range{{Off: 0, N: 100}, {Off: 1000, N: 500}, {Off: 1500, N: 500}, {Off: 5000, N: 10}}
	want := [][]byte{data[:100], data[1000:1500], data[1500:2000], data[5000:5010]}
	for name, parts := range map[string][][2]int{
		// Adjacent ranges merged into one part, overlapping parts, out of
		// order.
		"multipart": {{4990, 5020}, {1000, 1999}, {0, 99}, {50, 1200}},
		// Every range in one part.
		"single part": {{0, 5009}},
	} {
		t.Run(name, func(t *testing.T) {
			server := partsServer(t, data, parts...)
			s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
			defer s.Release()
			got, err := s.FetchRanges(t.Context(), ranges)
			if err != nil || !slices.EqualFunc(got, want, bytes.Equal) {
				t.Errorf("got %d ranges: %v", len(got), err)
			}
		})
	}
}

func TestFetchRangesUnsupported(t *testing.T) {
	data := make([]byte, 64<<10)
	_, _ = rand.Read(data)
	ranges := []container.Range{{Off: 0, N: 100}, {Off: 1000, N: 500}, {Off: 5000, N: 10}}
	for name, answer := range map[string]func(w http.ResponseWriter) int64{
		// The whole file, of 1 GiB.
		"whole file": func(w http.ResponseWriter) int64 {
			w.Header().Set("Content-Length", strconv.Itoa(1<<30))
			w.WriteHeader(http.StatusOK)
			return 1 << 30
		},
		// One part, of the first range only, of a body that goes on.
		"first range": func(w http.ResponseWriter) int64 {
			w.Header().Set("Content-Range", "bytes 0-99/65536")
			w.WriteHeader(http.StatusPartialContent)
			return 1 << 30
		},
		// One part covering every range, and the rest of the file.
		"one part of the whole file": func(w http.ResponseWriter) int64 {
			w.Header().Set("Content-Range", "bytes 0-1073741823/1073741824")
			w.WriteHeader(http.StatusPartialContent)
			return 1 << 30
		},
		"another multipart type": func(w http.ResponseWriter) int64 {
			w.Header().Set("Content-Type", "multipart/mixed; boundary=x")
			w.WriteHeader(http.StatusPartialContent)
			return 1 << 30
		},
	} {
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int32
			written := make(chan int64, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if !strings.Contains(r.Header.Get("Range"), ",") {
					http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
					return
				}
				// The body is written until the client goes away: it does
				// when the body is closed unread.
				var n int64
				for length := answer(w); n < length; n += int64(len(data)) {
					if _, err := w.Write(data); err != nil {
						break
					}
				}
				written <- n
			}))
			defer server.Close()
			s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
			defer s.Release()
			if _, err := s.FetchRanges(t.Context(), ranges); !errors.Is(err, container.ErrMultiRangeUnsupported) {
				t.Fatalf("got %v, want ErrMultiRangeUnsupported", err)
			}
			select {
			case n := <-written:
				if n >= 32<<20 {
					t.Errorf("%d bytes of the body sent", n)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("the body is still being read")
			}
			// Remembered: the source is not asked again for several ranges,
			// and serves them one by one.
			if _, err := s.FetchRanges(t.Context(), ranges); !errors.Is(err, container.ErrMultiRangeUnsupported) || requests.Load() != 1 {
				t.Errorf("got %v after %d requests", err, requests.Load())
			}
			for _, r := range ranges {
				if got, err := s.Fetch(t.Context(), r.Off, r.N); err != nil || !bytes.Equal(got, data[r.Off:r.Off+int64(r.N)]) {
					t.Errorf("at %d: %v", r.Off, err)
				}
			}
		})
	}
}

func TestFetchStalledAttemptsAreRetried(t *testing.T) {
	data := make([]byte, blockSize)
	_, _ = rand.Read(data)
	for name, stall := range map[string]func(w http.ResponseWriter){
		"before the headers": func(http.ResponseWriter) {},
		"within the body": func(w http.ResponseWriter) {
			w.Header().Set("Content-Range", "bytes 10-29/"+strconv.Itoa(len(data)))
			w.Header().Set("Content-Length", "20")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(data[10:20])
			w.(http.Flusher).Flush()
		},
	} {
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if requests.Add(1) == 1 {
					stall(w)
					// Until the client gives up.
					select {
					case <-r.Context().Done():
					case <-time.After(10 * time.Second):
					}
					return
				}
				http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
			}))
			defer server.Close()
			cache := newCache(t, 1<<30)
			cache.attemptTime = 100 * time.Millisecond
			s := cache.Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
			defer s.Release()
			started := time.Now()
			got, err := s.Fetch(t.Context(), 10, 20)
			if err != nil || !bytes.Equal(got, data[10:30]) {
				t.Fatalf("%d bytes: %v", len(got), err)
			}
			// The attempt, then a wait of 0.5 s.
			if waited := time.Since(started); requests.Load() != 2 || waited > 3*time.Second {
				t.Errorf("%d requests in %v", requests.Load(), waited)
			}
		})
	}
}

// memoryReader reads a file in memory for the container package.
type memoryReader []byte

func (m memoryReader) ReadAt(_ context.Context, p []byte, off int64) (int, error) {
	return bytes.NewReader(m).ReadAt(p, off)
}

// spreadMatroska is a Matroska file of a SubRip track whose blocks, one a
// second, each in a Cluster of its own, are farther apart than the
// container package reads with one range.
func spreadMatroska(texts ...string) []byte {
	idBytes := func(id uint32) []byte {
		b := binary.BigEndian.AppendUint32(nil, id)
		return bytes.TrimLeft(b, "\x00")
	}
	// Sizes of 8 bytes keep lengths fixed while positions are worked out.
	element := func(id uint32, children ...[]byte) []byte {
		payload := slices.Concat(children...)
		return slices.Concat(idBytes(id), binary.BigEndian.AppendUint64(nil, 1<<56|uint64(len(payload))), payload)
	}
	integer := func(id uint32, value uint64) []byte {
		return element(id, binary.BigEndian.AppendUint64(nil, value))
	}
	seekHead := func(info, tracks, cues uint64) []byte {
		seek := func(id uint32, position uint64) []byte {
			return element(0x4DBB, element(0x53AB, idBytes(id)), integer(0x53AC, position))
		}
		return element(0x114D9B74, seek(0x1549A966, info), seek(0x1654AE6B, tracks), seek(0x1C53BB6B, cues))
	}
	info := element(0x1549A966, integer(0x2AD7B1, 1_000_000))
	tracks := element(0x1654AE6B, element(0xAE, integer(0xD7, 1), integer(0x83, 0x11), element(0x86, []byte("S_TEXT/UTF8"))))
	position := uint64(len(seekHead(0, 0, 0)) + len(info) + len(tracks))
	var clusters, points []byte
	for i, text := range texts {
		ms := uint64(i) * 1000
		head := slices.Concat(integer(0xE7, ms), element(0xEC, make([]byte, 100<<10)))
		block := element(0xA0, element(0xA1, []byte{0x81, 0, 0, 0}, []byte(text)), integer(0x9B, 500))
		points = append(points, element(0xBB, integer(0xB3, ms),
			element(0xB7, integer(0xF7, 1), integer(0xF1, position), integer(0xF0, uint64(len(head)))))...)
		cluster := element(0x1F43B675, head, block)
		clusters = append(clusters, cluster...)
		position += uint64(len(cluster))
	}
	first := uint64(len(seekHead(0, 0, 0)))
	return slices.Concat(
		element(0x1A45DFA3, element(0x4282, []byte("matroska"))),
		element(0x18538067, seekHead(first, first+uint64(len(info)), position), info, tracks, clusters, element(0x1C53BB6B, points)),
	)
}

// TestSubtitleBlocksThroughSources reads a track's blocks from hosts that
// serve several ranges at once, merge them, or ignore them.
func TestSubtitleBlocksThroughSources(t *testing.T) {
	texts := []string{"One.", "Two.", "Three.", "Four.", "Five."}
	data := spreadMatroska(texts...)
	merged := func(w http.ResponseWriter, ranges string) {
		first, last := int64(len(data)), int64(0)
		for span := range strings.SplitSeq(strings.TrimPrefix(ranges, "bytes="), ",") {
			from, to, _ := strings.Cut(span, "-")
			start, _ := strconv.ParseInt(from, 10, 64)
			end, _ := strconv.ParseInt(to, 10, 64)
			first, last = min(first, start), max(last, min(end, int64(len(data))-1))
		}
		w.Header().Set("Content-Range", "bytes "+strconv.FormatInt(first, 10)+"-"+strconv.FormatInt(last, 10)+"/"+strconv.Itoa(len(data)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(data[first : last+1])
	}
	whole := func(w http.ResponseWriter, _ string) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
	for _, test := range []struct {
		name string
		// several answers requests for several ranges, nil for multipart
		// byte ranges.
		several  func(w http.ResponseWriter, ranges string)
		requests int32
	}{
		// The check reads a Cluster's header and data, then the blocks
		// come with one request; or one each, once the host ignored
		// several ranges.
		{"multipart", nil, 3},
		{"merged", merged, 3},
		{"ignored", whole, 3 + int32(len(texts))},
	} {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if ranges := r.Header.Get("Range"); strings.Contains(ranges, ",") && test.several != nil {
					test.several(w, ranges)
					return
				}
				http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
			}))
			defer server.Close()
			cache := newCache(t, 1<<30)
			cache.fetchInterval = time.Millisecond
			s := cache.Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
			defer s.Release()
			m, err := container.OpenMatroska(t.Context(), memoryReader(data), int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			blocks, err := m.SubtitleBlocks(t.Context(), s, 1)
			if err != nil {
				t.Fatal(err)
			}
			if len(blocks) != len(texts) {
				t.Fatalf("%d blocks", len(blocks))
			}
			for i, block := range blocks {
				if string(block.Data) != texts[i] || block.Start != time.Duration(i)*time.Second || block.Duration != 500*time.Millisecond {
					t.Errorf("block %d: %v %v %q", i, block.Start, block.Duration, block.Data)
				}
			}
			if requests.Load() != test.requests {
				t.Errorf("%d requests, want %d", requests.Load(), test.requests)
			}
		})
	}
}
