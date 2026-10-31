package source

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// keptCache is a cache in dir keeping its blocks for a week across
// restarts, within limit, closed at the end of the test.
func keptCache(t *testing.T, dir string, limit int64) *Cache {
	t.Helper()
	cache, err := New(dir, func() Policy { return Policy{Limit: limit, Keep: 7 * 24 * time.Hour, Credit: 24 * time.Hour} },
		client{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	return cache
}

// A file read before a restart is read from the disk after it, without a
// request to its source, and the cache counts the blocks it kept.
func TestARestartKeepsTheBlocksRead(t *testing.T) {
	o, server := newOrigin(t, 5*blockSize+12345)
	dir := t.TempDir()
	location := Location{URL: server.URL + "/file"}
	before := keptCache(t, dir, 1<<30)
	s := before.Open(accounts.ID{1}, location, nil)
	if got := readAll(t, s); !bytes.Equal(got, o.data) {
		t.Fatal("content differs")
	}
	s.Release()
	kept := before.Usage()
	if err := before.Close(); err != nil {
		t.Fatal(err)
	}
	requests := o.requests.Load()

	after := keptCache(t, dir, 1<<30)
	if usage := after.Usage(); usage.Used != kept.Used || usage.Used != int64(len(o.data)) || usage.Kept != 1 {
		t.Fatalf("after the restart: %+v, before it %+v", usage, kept)
	}
	s = after.Open(accounts.ID{1}, location, nil)
	defer s.Release()
	if got := readAll(t, s); !bytes.Equal(got, o.data) {
		t.Fatal("content read back differs")
	}
	if n := o.requests.Load(); n != requests {
		t.Errorf("the source was asked %d more times", n-requests)
	}
	if usage := after.Usage(); usage.Hits != int64(len(o.data)) || usage.Misses != 0 {
		t.Errorf("hits %d, misses %d", usage.Hits, usage.Misses)
	}
}

// A link that changed across the restart, as addons hand out new ones, has
// the blocks kept read again only once the source told the same size; a
// link now naming a file of another size has them removed.
func TestAChangedLinkKeepsTheBlocksOfTheSameSize(t *testing.T) {
	o, server := newOrigin(t, 4*blockSize+100)
	dir := t.TempDir()
	before := keptCache(t, dir, 1<<30)
	s := before.Open(accounts.ID{1}, Location{URL: server.URL + "/file?token=1"}, nil)
	readAll(t, s)
	s.Release()
	if err := before.Close(); err != nil {
		t.Fatal(err)
	}
	requests := o.requests.Load()

	after := keptCache(t, dir, 1<<30)
	s = after.Open(accounts.ID{1}, Location{URL: server.URL + "/file?token=2"}, nil)
	if got := readAll(t, s); !bytes.Equal(got, o.data) {
		t.Fatal("content differs")
	}
	s.Release()
	// One request tells the size; the blocks kept are read for the rest,
	// none waited for.
	if n := o.requests.Load() - requests; n != 1 {
		t.Errorf("%d requests for a file kept", n)
	}
	if usage := after.Usage(); usage.Misses != 0 || usage.Used != int64(len(o.data)) {
		t.Errorf("after the restart: %+v", usage)
	}
	if err := after.Close(); err != nil {
		t.Fatal(err)
	}

	other, otherServer := newOrigin(t, 2*blockSize+7)
	again := keptCache(t, dir, 1<<30)
	s = again.Open(accounts.ID{1}, Location{URL: otherServer.URL + "/file"}, nil)
	defer s.Release()
	if got := readAll(t, s); !bytes.Equal(got, other.data) {
		t.Fatal("the blocks kept of the other file were read")
	}
	if used := again.Usage().Used; used != int64(len(other.data)) {
		t.Errorf("the cache counts %d bytes, the new file has %d", used, len(other.data))
	}
}

// A transient source, a torrent's file, keeps nothing across a restart,
// and neither does a cache whose policy keeps nothing.
func TestTransientSourcesAreNotKept(t *testing.T) {
	o, server := newOrigin(t, 2*blockSize)
	dir := t.TempDir()
	before := keptCache(t, dir, 1<<30)
	s := before.Open(accounts.ID{1}, Location{URL: server.URL + "/file", Transient: true}, nil)
	readAll(t, s)
	s.Release()
	if err := before.Close(); err != nil {
		t.Fatal(err)
	}
	if usage := keptCache(t, dir, 1<<30).Usage(); usage.Used != 0 || usage.Kept != 0 {
		t.Errorf("a transient source was kept: %+v", usage)
	}
	if o.requests.Load() == 0 {
		t.Fatal("nothing was read")
	}
}

// A title played again is evicted after one played once lately, and kept
// past the retention, by the credit of its plays after the first.
func TestPlaysAgainCreditTheirChunks(t *testing.T) {
	c := keptCache(t, t.TempDir(), 3*blockSize)
	c.chunkBlocks = 1
	var mu sync.Mutex
	clock := time.Now().Add(-30 * 24 * time.Hour)
	c.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return clock
	}
	advance := func(d time.Duration) {
		mu.Lock()
		clock = clock.Add(d)
		mu.Unlock()
	}
	block := bytes.Repeat([]byte{7}, blockSize)
	store := func(s *Source, index int64) {
		t.Helper()
		if err := s.store(index, block, false); err != nil {
			t.Fatal(err)
		}
	}
	again := c.Open(accounts.ID{1}, Location{URL: "again"}, nil)
	defer again.Release()
	once := c.Open(accounts.ID{2}, Location{URL: "once"}, nil)
	defer once.Release()
	newer := c.Open(accounts.ID{3}, Location{URL: "newer"}, nil)
	defer newer.Release()
	// Played twice, a day apart, the second time 3 hours before the
	// other title's single play.
	again.Urge()()
	advance(24 * time.Hour)
	again.Urge()()
	store(again, 0)
	advance(3 * time.Hour)
	once.Urge()()
	store(once, 0)
	advance(time.Hour)
	store(newer, 0)
	// Past the protection of chunks read lately, a fourth chunk evicts
	// one: the title played once, though read after the other.
	advance(time.Hour)
	store(newer, 1)
	has := func(s *Source, block int64) bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.has(block)
	}
	if !has(again, 0) || has(once, 0) {
		t.Fatalf("kept: played again %v, played once %v", has(again, 0), has(once, 0))
	}
	// A week and an hour after their last read, the newer title's chunks
	// expire; the title played again has a day more.
	advance(7*24*time.Hour + time.Hour)
	c.mu.Lock()
	c.expire()
	c.mu.Unlock()
	if !has(again, 0) || has(newer, 0) || has(newer, 1) {
		t.Fatalf("after a week: played again %v, newer %v %v", has(again, 0), has(newer, 0), has(newer, 1))
	}
	advance(24 * time.Hour)
	c.mu.Lock()
	c.expire()
	c.mu.Unlock()
	if has(again, 0) {
		t.Fatal("the credit outlived its day")
	}
}

// A preload averages the rate it is given, and stops once the disk would
// keep less than the reserve free, or something plays.
func TestPreloadKeepsItsRateAndReserve(t *testing.T) {
	_, server := newOrigin(t, 40*blockSize)
	c := newCache(t, 1<<30)
	var mu sync.Mutex
	start := time.Now()
	clock := start
	c.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return clock
	}
	c.sleep = func(_ context.Context, d time.Duration) error {
		mu.Lock()
		clock = clock.Add(d)
		mu.Unlock()
		return nil
	}
	whole := func(size int64) []Span { return []Span{{0, size}} }
	s := c.Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	defer s.Release()
	const rate = 8 << 20
	fetched, err := s.Preload(t.Context(), Preloading{Rate: rate}, whole)
	if err != nil || fetched != 40*blockSize {
		t.Fatalf("fetched %d: %v", fetched, err)
	}
	if took, want := c.now().Sub(start), 5*time.Second; took != want {
		t.Errorf("40 MiB at 8 MiB/s took %v, want %v", took, want)
	}
	// What is cached costs nothing.
	if fetched, err := s.Preload(t.Context(), Preloading{Rate: rate}, whole); err != nil || fetched != 0 {
		t.Errorf("again: %d %v", fetched, err)
	}

	// A block stored is counted just after Warm returns: the counts are
	// read once they all are.
	waitFor(t, "the blocks preloaded to be counted", func() bool { return c.Usage().Used == 40*blockSize })
	// 20 MiB free, 4 MiB to keep: two pieces of 8 MiB fit, a third does
	// not.
	other := c.Open(accounts.ID{2}, Location{URL: server.URL + "/other"}, nil)
	defer other.Release()
	used := c.Usage().Used
	c.free = func() (int64, bool) { return 20<<20 - (c.Usage().Used - used), true }
	fetched, err = other.Preload(t.Context(), Preloading{Reserve: 4 << 20}, whole)
	if !errors.Is(err, ErrPreloadHeld) || fetched != 16<<20 {
		t.Errorf("with a reserve: fetched %d: %v", fetched, err)
	}
	waitFor(t, "the blocks preloaded to be counted", func() bool { return c.Usage().Used-used >= 16<<20 })
	if cached := c.Usage().Used - used; cached != 16<<20 {
		t.Errorf("with a reserve: %d bytes cached", cached)
	}

	// Something playing stops it before the next piece.
	c.free = func() (int64, bool) { return 0, false }
	third := c.Open(accounts.ID{3}, Location{URL: server.URL + "/third"}, nil)
	defer third.Release()
	pieces := 0
	idle := func() bool { pieces++; return pieces <= 2 }
	fetched, err = third.Preload(t.Context(), Preloading{Idle: idle}, whole)
	if !errors.Is(err, ErrPreloadHeld) || fetched != preloadPiece {
		t.Errorf("stopped by a play: fetched %d: %v", fetched, err)
	}
}
