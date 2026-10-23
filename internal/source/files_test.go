package source

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// memoryFile is a file read at offsets, counting its reads.
type memoryFile struct {
	*bytes.Reader
	reads *atomic.Int32
}

func (f memoryFile) ReadAt(p []byte, off int64) (int, error) {
	f.reads.Add(1)
	return f.Reader.ReadAt(p, off)
}

func (memoryFile) Close() error { return nil }

// A file read at offsets, as over SSH, answers ranges as a server does,
// and is read through the cache in blocks: a seek reads only the blocks
// missing.
func TestFileOpenerAnswersRanges(t *testing.T) {
	data := make([]byte, 5*blockSize+123)
	for i := range data {
		data[i] = byte(i % 251)
	}
	var opens, reads atomic.Int32
	opener := FileOpener(func(_ context.Context, target string) (RandomFile, int64, error) {
		if !strings.HasSuffix(target, "/movie.mkv") {
			return nil, 0, fs.ErrNotExist
		}
		opens.Add(1)
		return memoryFile{bytes.NewReader(data), &reads}, int64(len(data)), nil
	})
	for _, test := range []struct {
		ranges string
		status int
		body   []byte
		header string
	}{
		{"bytes=10-19", http.StatusPartialContent, data[10:20], "bytes 10-19/" + strconv.Itoa(len(data))},
		{"bytes=" + strconv.Itoa(len(data)-5) + "-", http.StatusPartialContent, data[len(data)-5:], ""},
		{"bytes=-3", http.StatusPartialContent, data[len(data)-3:], ""},
		{"bytes=0-1,5-6", http.StatusOK, data, ""},
		{"", http.StatusOK, data, ""},
		{"bytes=" + strconv.Itoa(len(data)) + "-", http.StatusRequestedRangeNotSatisfiable, nil, "bytes */" + strconv.Itoa(len(data))},
	} {
		response, err := opener.Open(t.Context(), http.MethodGet, "sftp://host/movie.mkv", http.Header{"Range": {test.ranges}}, false)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != test.status || test.body != nil && !bytes.Equal(body, test.body) ||
			test.header != "" && response.Header.Get("Content-Range") != test.header {
			t.Errorf("%q: %d %q, %d bytes, %v", test.ranges, response.StatusCode, response.Header.Get("Content-Range"), len(body), err)
		}
	}
	if response, err := opener.Open(t.Context(), http.MethodGet, "sftp://host/other.mkv", http.Header{}, false); err != nil ||
		response.StatusCode != http.StatusNotFound {
		t.Errorf("a missing file: %v", err)
	}

	s := newCache(t, 1<<30).Open(accounts.ID{1}, Location{URL: "sftp://host/movie.mkv", Opener: opener}, nil)
	defer s.Release()
	read := func(off int64) {
		t.Helper()
		got := make([]byte, 1000)
		if _, err := s.ReadAt(t.Context(), got, off); err != nil || !bytes.Equal(got, data[off:off+1000]) {
			t.Fatalf("at %d: %v", off, err)
		}
	}
	opens.Store(0)
	read(4 * blockSize)
	read(5)
	before := opens.Load()
	read(4*blockSize + 10)
	read(100)
	if after := opens.Load(); before != 2 || after != before {
		t.Errorf("opens: %d then %d", before, after)
	}
}

// The bytes kept beside the cache, as the pieces of torrents, count in
// its usage and within its limit: its own chunks make room for them. The
// blocks are stored as a connection stores them, on the test's goroutine:
// no connection runs, so none stores or touches a chunk after the clock
// moves, and the clock is read by this goroutine alone.
func TestSharedBytesCountWithinTheLimit(t *testing.T) {
	c := newCache(t, 8*blockSize)
	c.chunkBlocks = 1
	clock := time.Now()
	c.now = func() time.Time { return clock }
	var shared atomic.Int64
	c.Share(shared.Load)
	s := c.Open(accounts.ID{2}, Location{URL: "torrent:1"}, nil)
	defer s.Release()
	block := bytes.Repeat([]byte{7}, blockSize)
	for index := range int64(6) {
		// A second apart, so that the oldest is told apart.
		clock = clock.Add(time.Second)
		if err := s.store(index, block, false); err != nil {
			t.Fatal(err)
		}
	}
	shared.Store(5 * blockSize)
	if used, _, _ := c.Usage(); used != 11*blockSize {
		t.Fatalf("usage %d, want the cache's 6 blocks and the 5 shared", used)
	}
	// Past the protection of chunks read lately, the next block stored
	// evicts the cache's own chunks down to what the shared bytes leave,
	// the oldest first.
	clock = clock.Add(time.Hour)
	if err := s.store(6, block, false); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	own := c.used
	c.mu.Unlock()
	if own+shared.Load() > 8*blockSize {
		t.Fatalf("the cache keeps %d bytes beside %d shared, over its limit of %d", own, shared.Load(), 8*blockSize)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.has(6) || s.has(0) {
		t.Fatalf("kept the blocks %b: the newest must stay, the oldest go", s.present)
	}
}

// A playback's urge runs its source location's Playing until it stops.
func TestUrgeRunsPlaying(t *testing.T) {
	c := newCache(t, 8*blockSize)
	var playing atomic.Int32
	s := c.Open(accounts.ID{3}, Location{URL: "torrent:2", Playing: func() func() {
		playing.Add(1)
		return func() { playing.Add(-1) }
	}}, nil)
	defer s.Release()
	done := s.Urge()
	if playing.Load() != 1 {
		t.Fatal("Playing did not run while the source is urged")
	}
	done()
	done()
	if playing.Load() != 0 {
		t.Fatal("Playing did not stop once with the urge")
	}
}

// heldFile is a file whose first block reads at once, and whose other
// blocks are held until the read's request ends, as a torrent's pieces no
// peer sends: ahead is told of each read held. open counts the files open.
type heldFile struct {
	data  []byte
	ctx   context.Context
	ahead chan<- struct{}
	open  *atomic.Int32
}

func (f heldFile) ReadAt(p []byte, off int64) (int, error) {
	if off < blockSize {
		return copy(p, f.data[off:]), nil
	}
	select {
	case f.ahead <- struct{}{}:
	default:
	}
	<-f.ctx.Done()
	return 0, f.ctx.Err()
}

func (f heldFile) Close() error {
	f.open.Add(-1)
	return nil
}

// A source only playbacks read, as a torrent's file, is no longer read
// once none urges it: what was read ahead of the playback stops, and the
// file is closed at once, rather than read on and kept open for a next
// read, which goes on downloading a torrent.
func TestASourceOnlyPlaybacksReadStopsWithTheLastOne(t *testing.T) {
	c := newCache(t, 64*blockSize)
	data := make([]byte, 40*blockSize)
	ahead := make(chan struct{}, 1)
	var open atomic.Int32
	opener := FileOpener(func(ctx context.Context, _ string) (RandomFile, int64, error) {
		open.Add(1)
		return heldFile{data: data, ctx: ctx, ahead: ahead, open: &open}, int64(len(data)), nil
	})
	// The file sends nothing for minutes before it counts as stalled.
	s := c.Open(accounts.ID{4}, Location{URL: "torrent:4", Opener: opener, StallTimeout: 10 * time.Minute,
		Playing: func() func() { return func() {} }}, nil)
	defer s.Release()
	done := s.Urge()
	r := s.NewReader()
	first := make([]byte, blockSize)
	if _, err := r.ReadAt(t.Context(), first, 0); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ahead:
	case <-time.After(time.Minute):
		t.Fatal("the playback was not read ahead of")
	}
	r.Close()
	done()
	for deadline := time.Now().Add(30 * time.Second); open.Load() != 0; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%d reads of the file still run once no playback urges it", open.Load())
		}
	}
}
