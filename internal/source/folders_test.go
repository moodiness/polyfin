package source

import (
	"bytes"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// warnings is a log the test counts the warnings of.
type warnings struct {
	mu   sync.Mutex
	text strings.Builder
}

func (w *warnings) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.text.Write(p)
}

func (w *warnings) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.Count(w.text.String(), "level=WARN")
}

// loggedCache is keptCache within 1 GiB, its warnings logged to the log
// returned.
func loggedCache(t *testing.T, dir string) (*Cache, *warnings) {
	t.Helper()
	c := keptCache(t, dir, 1<<30)
	logged := &warnings{}
	c.logger = slog.New(slog.NewTextHandler(logged, &slog.HandlerOptions{Level: slog.LevelWarn}))
	return c, logged
}

// saveDue reports whether a save of the source is due.
func (s *Source) saveDue() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveTimer != nil
}

// A source read past the block cache alone, as the subtitle blocks of an
// index are (see Fetch), learns its size without a block written, nor a
// folder: released, it has nothing to save, and saves nothing, rather than
// failing to write its state into a folder that never was, every
// saveDelay for as long as the server runs.
func TestASourceFetchedAloneSavesNothing(t *testing.T) {
	t.Parallel()
	_, server := newOrigin(t, 3*blockSize)
	c, logged := loggedCache(t, t.TempDir())
	s := c.Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	if _, err := s.Fetch(t.Context(), blockSize, 100); err != nil {
		t.Fatal(err)
	}
	if _, known := s.knownSize(); !known {
		t.Fatal("the fetch did not tell the size")
	}
	s.Release()
	c.save(s)
	if n := logged.count(); n != 0 || s.saveDue() {
		t.Errorf("%d warnings, a save due: %t", n, s.saveDue())
	}
	if _, err := os.Stat(s.dir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the source's folder: %v", err)
	}
}

// A source's folder removed from outside while nobody reads the source, as
// by someone freeing disk space, is told once, by the next save: the cache
// stops counting the chunks gone with it, and saves nothing more of them.
// The next reader fetches the blocks again, which the cache keeps then, a
// restart included.
func TestARemovedFolderIsToldOnce(t *testing.T) {
	t.Parallel()
	o, server := newOrigin(t, 3*blockSize+123)
	size := int64(len(o.data))
	dir := t.TempDir()
	c, logged := loggedCache(t, dir)
	location := Location{URL: server.URL + "/file"}
	s := c.Open(accounts.ID{1}, location, nil)
	readAll(t, s)
	s.Release()
	c.save(s)
	if err := os.RemoveAll(s.dir); err != nil {
		t.Fatal(err)
	}
	c.save(s)
	// Nothing is left to save: no failure, nor a warning again.
	c.save(s)
	if n := logged.count(); n != 1 || s.saveDue() {
		t.Errorf("%d warnings, a save due: %t", n, s.saveDue())
	}
	if usage := c.Usage(); usage.Used != 0 || usage.Kept != 0 {
		t.Errorf("after the removal: %+v", usage)
	}

	requests := o.requests.Load()
	s = c.Open(accounts.ID{1}, location, nil)
	got := readAll(t, s)
	s.Release()
	if !bytes.Equal(got, o.data) || o.requests.Load() == requests {
		t.Fatalf("read again: content equal %t, %d requests", bytes.Equal(got, o.data), o.requests.Load()-requests)
	}
	c.save(s)
	if usage := c.Usage(); usage.Used != size || chunkBytes(t, dir) != size {
		t.Errorf("read again: %+v, %d bytes on disk", usage, chunkBytes(t, dir))
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if usage := keptCache(t, dir, 1<<30).Usage(); usage.Used != size {
		t.Errorf("after a restart: %+v", usage)
	}
}

// A source's folder removed from outside, then written into again before
// a save finds it gone, is told once, and never has a chunk made again
// read as holding the blocks it held: they are fetched again, not read as
// zeros, and the cache counts the blocks fetched again once.
func TestAChunkMadeAgainHoldsNothingRemoved(t *testing.T) {
	t.Parallel()
	o, server := newOrigin(t, 4*blockSize)
	dir := t.TempDir()
	c, logged := loggedCache(t, dir)
	c.chunkBlocks = 2
	// The block after the one read before the removal is written into the
	// chunk of the block read, gone with the folder, or, after block 1,
	// into a chunk of its own, made with the folder again.
	for _, first := range []int64{0, 1} {
		s := c.Open(accounts.ID{byte(first + 1)}, Location{URL: server.URL + "/file"}, nil)
		read := func(block int64) {
			t.Helper()
			got := make([]byte, 1000)
			off := block * blockSize
			if _, err := s.ReadAt(t.Context(), got, off); err != nil || !bytes.Equal(got, o.data[off:off+1000]) {
				t.Fatalf("block %d after block %d: content equal %t, %v", block, first, bytes.Equal(got, o.data[off:off+1000]), err)
			}
		}
		read(first)
		if err := os.RemoveAll(s.dir); err != nil {
			t.Fatal(err)
		}
		read(first + 1)
		read(first)
		s.Release()
		if n := logged.count(); n != int(first)+1 {
			t.Errorf("block %d read first: %d warnings in all", first, n)
		}
	}
	// Blocks 0 and 1 of the first source, 1 and 2 of the second.
	if used := c.Usage().Used; used != 4*blockSize {
		t.Errorf("%d bytes counted", used)
	}
}

// A source whose state keeps failing to save is tried again a few times,
// told once, then saved again only once a block is written or a reader
// releases it, and as the cache closes: it does not try every saveDelay
// for as long as the server runs.
func TestFailingSavesStop(t *testing.T) {
	t.Parallel()
	_, server := newOrigin(t, 2*blockSize)
	c, logged := loggedCache(t, t.TempDir())
	s := c.Open(accounts.ID{1}, Location{URL: server.URL + "/file"}, nil)
	readAll(t, s)
	s.Release()
	// A file where the folder was: nothing can be written into it.
	if err := os.RemoveAll(s.dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.dir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for try := 1; try <= saveTries+1; try++ {
		c.save(s)
		if due := s.saveDue(); due != (try < saveTries) {
			t.Errorf("after %d failed saves, a save due: %t", try, due)
		}
	}
	if n := logged.count(); n != 1 {
		t.Errorf("%d warnings", n)
	}
}
