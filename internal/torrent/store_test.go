package torrent

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/workfolder"
)

// storePiece is the size of the pieces of the store's tests.
const storePiece = 4 << 10

func newTestStore(t *testing.T, budget int64) *store {
	t.Helper()
	s, err := newStore(filepath.Join(t.TempDir(), "torrents"), func() int64 { return budget }, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// pieceBytes are the bytes of a piece, its own.
func pieceBytes(key pieceKey, size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = key.hash[0] ^ byte(key.index*7) ^ byte(i)
	}
	return data
}

// isPiece tells whether a file is one of the piece of key.
func isPiece(name string, key pieceKey) bool {
	return strings.HasPrefix(filepath.Base(name), fmt.Sprintf("%s-%d-", key.hash, key.index))
}

func mustPut(t *testing.T, s *store, key pieceKey, size int) {
	t.Helper()
	if err := s.put(key, pieceBytes(key, size)); err != nil {
		t.Fatalf("piece %v: %v", key, err)
	}
}

// kept tells whether the store reads the whole piece of key.
func kept(t *testing.T, s *store, key pieceKey, size int) bool {
	t.Helper()
	p := make([]byte, size)
	found, err := s.read(key, p, 0)
	if err != nil {
		t.Fatalf("piece %v: %v", key, err)
	}
	if found && !bytes.Equal(p, pieceBytes(key, size)) {
		t.Fatalf("piece %v: wrong bytes", key)
	}
	return found
}

// onDisk is the size of each file of the store's folder. It may run in
// any goroutine.
func onDisk(t *testing.T, dir string) map[string]int64 {
	t.Helper()
	files := map[string]int64{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Error(err)
		return files
	}
	for _, entry := range entries {
		if entry.Name() == workfolder.Marker {
			continue
		}
		if info, err := entry.Info(); err == nil {
			files[entry.Name()] = info.Size()
		}
	}
	return files
}

func total(files map[string]int64) int64 {
	var n int64
	for _, size := range files {
		n += size
	}
	return n
}

// until waits for done, as long as the test may run.
func until(done func() bool) {
	for !done() {
		time.Sleep(time.Millisecond)
	}
}

// indexed tells whether the store's index holds the piece of key.
func (s *store) indexed(key pieceKey) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pieces[key] != nil
}

// heldFile is a piece's file whose reads wait for release, once they told
// reading they started.
type heldFile struct {
	pieceFile
	reading chan<- struct{}
	release <-chan struct{}
}

func (f heldFile) ReadAt(p []byte, off int64) (int, error) {
	f.reading <- struct{}{}
	<-f.release
	return f.pieceFile.ReadAt(p, off)
}

// A torrent's reads and writes go on while another torrent's piece is
// being written: the store's lock is not held through the write.
func TestAStoreReadsAndWritesWhileAnotherTorrentWrites(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, 8*storePiece)
	defer s.close()
	slow, other := pieceKey{Hash{1}, 0}, pieceKey{Hash{2}, 0}
	writing, release := make(chan struct{}), make(chan struct{})
	s.write = func(name string, data []byte) error {
		if isPiece(name, slow) {
			close(writing)
			<-release
		}
		return writeFile(name, data)
	}
	mustPut(t, s, other, storePiece)
	written := make(chan error)
	go func() { written <- s.put(slow, pieceBytes(slow, storePiece)) }()
	<-writing
	if !kept(t, s, other, storePiece) {
		t.Fatal("the other torrent's piece is not read while a piece is written")
	}
	next := pieceKey{other.hash, 1}
	mustPut(t, s, next, storePiece)
	if !kept(t, s, next, storePiece) {
		t.Fatal("the other torrent's next piece is not kept while a piece is written")
	}
	if kept(t, s, slow, storePiece) {
		t.Fatal("a piece is read before it is written")
	}
	if used := s.usage(); used != 3*storePiece {
		t.Fatalf("the store counts %d bytes, want the 3 pieces kept or being written", used)
	}
	close(release)
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	if !kept(t, s, slow, storePiece) {
		t.Fatal("the piece written is not kept")
	}
}

// The pieces removed for room are those used least recently, written or
// read, as many as the new piece needs, and their torrents are told.
func TestAStoreRemovesTheLeastRecentlyUsedPiecesFirst(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, 3*storePiece)
	defer s.close()
	var told []pieceKey
	s.evicted = func(key pieceKey) { told = append(told, key) }
	a0, a1, a2, b0, b1, b2 := pieceKey{Hash{1}, 0}, pieceKey{Hash{1}, 1}, pieceKey{Hash{1}, 2}, pieceKey{Hash{2}, 0}, pieceKey{Hash{2}, 1}, pieceKey{Hash{2}, 2}
	mustPut(t, s, a0, storePiece)
	mustPut(t, s, b0, storePiece)
	mustPut(t, s, a1, storePiece)
	kept(t, s, a0, storePiece)
	mustPut(t, s, b1, storePiece)
	kept(t, s, a1, storePiece)
	mustPut(t, s, a2, storePiece)
	mustPut(t, s, b2, 2*storePiece)
	if want := []pieceKey{b0, a0, b1, a1}; !slices.Equal(told, want) {
		t.Fatalf("removed %v, want %v", told, want)
	}
	for _, key := range []pieceKey{a0, a1, b0, b1} {
		if kept(t, s, key, storePiece) {
			t.Errorf("piece %v is still read", key)
		}
	}
	if !kept(t, s, a2, storePiece) || !kept(t, s, b2, 2*storePiece) {
		t.Error("the last pieces are not kept")
	}
	if files := onDisk(t, s.dir); len(files) != 2 || total(files) != 3*storePiece || s.usage() != 3*storePiece {
		t.Errorf("the disk holds %v, the store counts %d bytes", files, s.usage())
	}
}

// A write waits for a read of the piece it removes before writing, so
// that the pieces on disk stay within the budget, and the read ends with
// the piece's bytes. A torrent that stops has its pieces deleted once
// their reads end.
func TestAStoreKeepsItsBoundWhileAPieceIsRead(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, storePiece)
	defer s.close()
	hash := Hash{1}
	first, second := pieceKey{hash, 0}, pieceKey{hash, 1}
	var over atomic.Value
	s.write = func(name string, data []byte) error {
		if files := onDisk(t, s.dir); total(files)+int64(len(data)) > storePiece {
			over.Store(fmt.Sprintf("%s written beside %v", filepath.Base(name), files))
		}
		return writeFile(name, data)
	}
	reading, release := make(chan struct{}), make(chan struct{})
	s.open = func(name string) (pieceFile, error) {
		file, err := openFile(name)
		if err != nil {
			return nil, err
		}
		return heldFile{file, reading, release}, nil
	}
	run := s.started(hash)
	mustPut(t, s, first, storePiece)
	read := make(chan bool)
	go func() {
		p := make([]byte, storePiece)
		found, err := s.read(first, p, 0)
		read <- found && err == nil && bytes.Equal(p, pieceBytes(first, storePiece))
	}()
	<-reading
	written := make(chan error)
	go func() { written <- s.put(second, pieceBytes(second, storePiece)) }()
	until(func() bool { return !s.indexed(first) })
	release <- struct{}{}
	if !<-read {
		t.Fatal("the read under way did not end with the piece's bytes")
	}
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	if v := over.Load(); v != nil {
		t.Fatalf("over the budget: %v", v)
	}

	// A torrent that stops has its piece deleted once its read ends.
	go func() {
		found, err := s.read(second, make([]byte, storePiece), 0)
		read <- found && err == nil
	}()
	<-reading
	var ended atomic.Bool
	dropped := make(chan bool)
	go func() {
		s.drop(hash, run)
		dropped <- ended.Load()
	}()
	until(func() bool { return !s.indexed(second) })
	ended.Store(true)
	release <- struct{}{}
	if !<-read {
		t.Fatal("the read under way as the torrent stopped failed")
	}
	if !<-dropped {
		t.Fatal("the stop returned before the read of its piece ended")
	}
	if files := onDisk(t, s.dir); len(files) != 0 || s.usage() != 0 {
		t.Fatalf("after the stop, the disk holds %v, the store counts %d bytes", files, s.usage())
	}
}

// watchedFile is a piece's file whose closings a test counts, telling
// whether the file was still on disk then: a file open cannot be deleted
// on Windows.
type watchedFile struct {
	pieceFile
	name   string
	closes atomic.Int32
	// deletedFirst tells the file was gone as it closed.
	deletedFirst atomic.Bool
}

func (f *watchedFile) Close() error {
	f.closes.Add(1)
	if _, err := os.Stat(f.name); err != nil {
		f.deletedFirst.Store(true)
	}
	return f.pieceFile.Close()
}

// watchFiles has the store's files opened as watchedFiles, returned by
// the name of their piece in order.
func watchFiles(s *store) func() []*watchedFile {
	var mu sync.Mutex
	var files []*watchedFile
	s.open = func(name string) (pieceFile, error) {
		file, err := openFile(name)
		if err != nil {
			return nil, err
		}
		watched := &watchedFile{pieceFile: file, name: name}
		mu.Lock()
		files = append(files, watched)
		mu.Unlock()
		return watched, nil
	}
	return func() []*watchedFile {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(files)
	}
}

// The files of the pieces read last stay open, within the bound: a read
// past it closes the file read least recently.
func TestAStoreKeepsTheFilesOfThePiecesReadLastOpen(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, 8*storePiece)
	defer s.close()
	s.files = 3
	files := watchFiles(s)
	key := func(i int) pieceKey { return pieceKey{Hash{1}, i} }
	for i := range 6 {
		mustPut(t, s, key(i), storePiece)
	}
	// open lists the pieces whose files are open, in the order they were
	// opened.
	open := func() []int {
		var indexes []int
		for _, f := range files() {
			if f.closes.Load() == 0 {
				for i := range 6 {
					if isPiece(f.name, key(i)) {
						indexes = append(indexes, i)
					}
				}
			}
		}
		return indexes
	}
	for i := range 6 {
		kept(t, s, key(i), storePiece)
	}
	if got := open(); !slices.Equal(got, []int{3, 4, 5}) {
		t.Fatalf("open: %v, want the 3 pieces read last", got)
	}
	for i := 3; i < 6; i++ {
		kept(t, s, key(i), storePiece)
	}
	if opened := len(files()); opened != 6 {
		t.Fatalf("the pieces read again opened their files again: %d opened", opened)
	}
	kept(t, s, key(0), storePiece)
	if got := open(); !slices.Equal(got, []int{4, 5, 0}) {
		t.Fatalf("open: %v, want piece 3's file closed for piece 0's", got)
	}
	for _, f := range files() {
		if f.closes.Load() > 1 {
			t.Errorf("%s closed %d times", filepath.Base(f.name), f.closes.Load())
		}
	}
}

// The file a piece keeps open is closed as the piece is removed, before
// the file is deleted: for room, as its torrent stops, and as the store
// closes.
func TestAStoreClosesTheFilesOfThePiecesItRemoves(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, 2*storePiece)
	files := watchFiles(s)
	a, b := Hash{1}, Hash{2}
	run := s.started(a)
	mustPut(t, s, pieceKey{a, 0}, storePiece)
	mustPut(t, s, pieceKey{a, 1}, storePiece)
	kept(t, s, pieceKey{a, 0}, storePiece)
	kept(t, s, pieceKey{a, 1}, storePiece)
	closed := func() int {
		n := 0
		for _, f := range files() {
			if f.closes.Load() > 0 {
				n++
			}
		}
		return n
	}
	if n := closed(); n != 0 {
		t.Fatalf("%d files closed after their reads", n)
	}
	mustPut(t, s, pieceKey{a, 2}, storePiece)
	if f := files()[0]; f.closes.Load() != 1 || !isPiece(f.name, pieceKey{a, 0}) {
		t.Fatalf("the file of the piece removed for room was closed %d times", f.closes.Load())
	}
	kept(t, s, pieceKey{a, 2}, storePiece)
	s.drop(a, run)
	if n := closed(); n != 3 {
		t.Fatalf("%d of the 3 files closed once the torrent stopped", n)
	}
	mustPut(t, s, pieceKey{b, 0}, storePiece)
	kept(t, s, pieceKey{b, 0}, storePiece)
	s.close()
	for _, f := range files() {
		if f.closes.Load() != 1 || f.deletedFirst.Load() {
			t.Errorf("%s closed %d times, after it was deleted: %v", filepath.Base(f.name), f.closes.Load(), f.deletedFirst.Load())
		}
	}
	if _, err := os.Stat(s.dir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the folder is left after the store closed: %v", err)
	}
}

// A piece whose write fails, as the disk is full, is not kept, and what
// was written of it is deleted. The pieces removed for its room are gone,
// and told. Once the disk has room again, the next piece is kept.
func TestAStoreOnAFullDiskKeepsNothingOfThePiece(t *testing.T) {
	t.Parallel()
	s := newTestStore(t, 2*storePiece)
	defer s.close()
	var told []pieceKey
	s.evicted = func(key pieceKey) { told = append(told, key) }
	full := false
	s.write = func(name string, data []byte) error {
		if !full {
			return writeFile(name, data)
		}
		if err := writeFile(name, data[:len(data)/2]); err != nil {
			return err
		}
		return &fs.PathError{Op: "write", Path: name, Err: syscall.ENOSPC}
	}
	key := func(i int) pieceKey { return pieceKey{Hash{1}, i} }
	mustPut(t, s, key(0), storePiece)
	mustPut(t, s, key(1), storePiece)
	full = true
	if err := s.put(key(2), pieceBytes(key(2), storePiece)); !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("got %v, want the full disk's error", err)
	}
	if !slices.Equal(told, []pieceKey{key(0)}) {
		t.Fatalf("told %v, want the piece removed for room", told)
	}
	if kept(t, s, key(2), storePiece) || kept(t, s, key(0), storePiece) || !kept(t, s, key(1), storePiece) {
		t.Fatal("the pieces kept are not those written whole")
	}
	if files := onDisk(t, s.dir); len(files) != 1 || total(files) != storePiece || s.usage() != storePiece {
		t.Fatalf("the disk holds %v, the store counts %d bytes", files, s.usage())
	}
	full = false
	mustPut(t, s, key(2), storePiece)
	if !kept(t, s, key(2), storePiece) || !kept(t, s, key(1), storePiece) {
		t.Fatal("the pieces written once the disk has room are not kept")
	}
	if files := onDisk(t, s.dir); len(files) != 2 || s.usage() != 2*storePiece {
		t.Fatalf("the disk holds %v, the store counts %d bytes", files, s.usage())
	}
}

// Torrents that write and read at once read the bytes written, while the
// pieces on disk stay within the budget and the files open within their
// bound, a few reads under way aside. Once they end, the store counts
// what the disk holds, and closing it closes every file.
func TestAStoreUnderConcurrentPutsAndReads(t *testing.T) {
	t.Parallel()
	const (
		torrents = 4
		readers  = 2
		pieces   = 60
		budget   = 10 * storePiece
	)
	s := newTestStore(t, budget)
	s.files = 3
	var over atomic.Value
	s.write = func(name string, data []byte) error {
		if files := onDisk(t, s.dir); total(files)+int64(len(data)) > budget {
			over.Store(fmt.Sprintf("%s written beside %v", filepath.Base(name), files))
		}
		return writeFile(name, data)
	}
	var open, most atomic.Int32
	s.open = func(name string) (pieceFile, error) {
		file, err := openFile(name)
		if err != nil {
			return nil, err
		}
		n := open.Add(1)
		for m := most.Load(); n > m && !most.CompareAndSwap(m, n); m = most.Load() {
		}
		return countedFile{file, &open}, nil
	}
	var told atomic.Int32
	s.evicted = func(pieceKey) { told.Add(1) }
	var wg sync.WaitGroup
	var found, wrong atomic.Int32
	for n := range torrents {
		hash := Hash{byte(n + 1)}
		var written atomic.Int32
		wg.Go(func() {
			for i := range pieces {
				if err := s.put(pieceKey{hash, i}, pieceBytes(pieceKey{hash, i}, storePiece)); err != nil {
					t.Error(err)
				}
				written.Store(int32(i + 1))
			}
		})
		for r := range readers {
			wg.Go(func() {
				p := make([]byte, 1<<10)
				for done := false; !done; {
					done = written.Load() == pieces
					last := int(written.Load())
					// One reader reads the last pieces, as a playback;
					// the other older ones, as a peer asks.
					for i := max(0, last-3-r*3); i < last; i++ {
						key := pieceKey{hash, i}
						for off := 0; off < storePiece; off += len(p) {
							ok, err := s.read(key, p, int64(off))
							if err != nil {
								t.Error(err)
								return
							}
							if ok {
								found.Add(1)
								if !bytes.Equal(p, pieceBytes(key, storePiece)[off:off+len(p)]) {
									wrong.Add(1)
								}
							}
						}
					}
				}
			})
		}
	}
	wg.Wait()
	if v := over.Load(); v != nil {
		t.Errorf("over the budget: %v", v)
	}
	if wrong.Load() > 0 {
		t.Errorf("%d reads of %d got wrong bytes", wrong.Load(), found.Load())
	}
	if found.Load() == 0 || told.Load() == 0 {
		t.Errorf("%d reads found their piece, %d pieces removed for room", found.Load(), told.Load())
	}
	if m := most.Load(); m > int32(s.files+torrents*readers) {
		t.Errorf("%d files open at once, over the bound of %d and the %d readers", m, s.files, torrents*readers)
	}
	if n := open.Load(); n > int32(s.files) {
		t.Errorf("%d files open once the reads ended, over the bound of %d", n, s.files)
	}
	files := onDisk(t, s.dir)
	s.mu.Lock()
	var index int64
	for _, piece := range s.pieces {
		index += piece.size
		if files[filepath.Base(piece.name)] != piece.size {
			t.Errorf("piece %v: %d bytes on disk", piece.key, files[filepath.Base(piece.name)])
		}
	}
	used := s.used
	s.mu.Unlock()
	if total(files) != index || used != index || index > budget {
		t.Errorf("the disk holds %d bytes, the index %d, the store counts %d", total(files), index, used)
	}
	s.close()
	if n := open.Load(); n != 0 {
		t.Errorf("%d files open once the store closed", n)
	}
}

// countedFile is a piece's file that counts the files open.
type countedFile struct {
	pieceFile
	open *atomic.Int32
}

func (f countedFile) Close() error {
	f.open.Add(-1)
	return f.pieceFile.Close()
}
