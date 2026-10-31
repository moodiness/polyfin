package source

import (
	"bytes"
	"context"
	"errors"
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
	if used := c.Usage().Used; used != 11*blockSize {
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

// blockHeldFile is a file whose second block is held until the read's
// request ends; its other blocks read at once. atHeld is told as the held
// block is asked for.
type blockHeldFile struct {
	data   []byte
	ctx    context.Context
	atHeld chan<- struct{}
}

func (f blockHeldFile) ReadAt(p []byte, off int64) (int, error) {
	if off/blockSize == 1 {
		select {
		case f.atHeld <- struct{}{}:
		default:
		}
		<-f.ctx.Done()
		return 0, f.ctx.Err()
	}
	return copy(p, f.data[off:]), nil
}

func (blockHeldFile) Close() error { return nil }

// A reader waiting for a block of a source only playbacks read, as a
// torrent's file, gets it once the last playback stops urging it, though
// the connection that read towards it, a block before, ends then: another
// fetches it, rather than the reader waiting for good.
func TestAReaderWaitingWhenThePlaybackEndsGetsItsBlock(t *testing.T) {
	c := newCache(t, 64*blockSize)
	// One connection per file: the one reading towards the block is the
	// only one.
	c.connections = 1
	data := torrentBytes(4*blockSize + 123)
	atHeld := make(chan struct{}, 1)
	opener := FileOpener(func(ctx context.Context, _ string) (RandomFile, int64, error) {
		return blockHeldFile{data: data, ctx: ctx, atHeld: atHeld}, int64(len(data)), nil
	})
	s := c.Open(accounts.ID{5}, Location{URL: "torrent:5", Opener: opener, StallTimeout: 10 * time.Minute,
		Playing: func() func() { return func() {} }}, nil)
	defer s.Release()
	done := s.Urge()
	r := s.NewReader()
	if _, err := r.ReadAt(t.Context(), make([]byte, blockSize), 0); err != nil {
		t.Fatal(err)
	}
	select {
	case <-atHeld:
	case <-time.After(time.Minute):
		t.Fatal("the playback was not read ahead of")
	}
	// A read in the background, as of the file's index, waits for the
	// block after the one the connection reads.
	read := make(chan error, 1)
	go func() {
		got := make([]byte, blockSize)
		_, err := s.ReadAt(t.Context(), got, 2*blockSize)
		if err == nil && !bytes.Equal(got, data[2*blockSize:3*blockSize]) {
			err = errors.New("not the file's bytes")
		}
		read <- err
	}()
	for deadline := time.Now().Add(time.Minute); ; time.Sleep(10 * time.Millisecond) {
		s.mu.Lock()
		waiting := s.wanted[2] != nil
		s.mu.Unlock()
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the background read does not wait for its block")
		}
	}
	r.Close()
	done()
	select {
	case err := <-read:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the background read still waits for its block once the playback ended")
	}
}

// slowProgressFile is a file whose reads take whole seconds, while they
// tell, every interval, that bytes are coming; a read ends early once its
// request does.
type slowProgressFile struct {
	ctx      context.Context
	data     []byte
	took     time.Duration
	interval time.Duration
}

func (f slowProgressFile) ReadAt(p []byte, off int64) (int, error) {
	return f.ReadAtProgress(p, off, nil)
}

func (f slowProgressFile) ReadAtProgress(p []byte, off int64, progress func()) (int, error) {
	ticker := time.NewTicker(f.interval)
	defer ticker.Stop()
	for end := time.Now().Add(f.took); time.Now().Before(end); {
		select {
		case <-f.ctx.Done():
			return 0, f.ctx.Err()
		case <-ticker.C:
		}
		if progress != nil {
			progress()
		}
	}
	return copy(p, f.data[off:]), nil
}

func (slowProgressFile) Close() error { return nil }

// A file whose blocks take longer than the stall timeout to read whole,
// while their bytes keep coming, as a slow torrent's pieces, is not taken
// for one that stalled: it is read.
func TestASlowFileWhoseBytesKeepComingDoesNotStall(t *testing.T) {
	c := newCache(t, 64*blockSize)
	data := torrentBytes(2*blockSize + 77)
	opener := FileOpener(func(ctx context.Context, _ string) (RandomFile, int64, error) {
		return slowProgressFile{ctx: ctx, data: data, took: 10 * time.Second, interval: 200 * time.Millisecond}, int64(len(data)), nil
	})
	s := c.Open(accounts.ID{6}, Location{URL: "torrent:6", Opener: opener, StallTimeout: 3 * time.Second}, nil)
	defer s.Release()
	got := make([]byte, blockSize)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	if _, err := s.ReadAt(ctx, got, 0); err != nil || !bytes.Equal(got, data[:blockSize]) {
		t.Fatalf("the slow file was not read: %v", err)
	}
}

// torrentBytes are n bytes that differ from block to block.
func torrentBytes(n int) []byte {
	data := make([]byte, n)
	for i := range data {
		data[i] = byte(i/977 + i%251)
	}
	return data
}
