package torrent

import (
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/moodiness/polyfin/internal/workfolder"
)

// pieceKey names a piece of a torrent.
type pieceKey struct {
	hash  Hash
	index int
}

// storeFiles is the most files of pieces the store keeps open between
// reads: those of the pieces read last.
const storeFiles = 64

// store keeps the pieces of the torrents on disk, one file each, within
// the bytes budget gives, read at each write: the least recently used
// pieces are removed first, and their torrent told, which no longer has
// them.
//
// Its lock guards the index of the pieces alone, a few steps at a time:
// files are written, opened, read, closed and deleted outside it, so that
// a torrent's read never waits for another's write or removal. Each file
// written has a name of its own, which no deletion of an earlier file of
// the same piece can reach.
type store struct {
	dir    string
	budget func() int64
	logger *slog.Logger
	// files bounds the files kept open between reads (see storeFiles).
	files int
	// write makes a piece's file, open opens one for reads, and unlink
	// deletes one: on disk, but in tests.
	write  func(name string, data []byte) error
	open   func(name string) (pieceFile, error)
	unlink func(name string) error

	mu sync.Mutex
	// changed is signalled as bytes leave the disk, a write ends or the
	// store closes, which a write waiting for room waits for.
	changed sync.Cond
	closed  bool
	// used is the bytes on disk or about to be: those of the pieces kept,
	// of the files being written, and of the pieces removed whose files
	// are not deleted yet, which leaving counts. While a piece fits the
	// budget, used never exceeds it.
	used, leaving int64
	pieces        map[pieceKey]*storedPiece
	// recent lists the pieces kept, the most recently used first; opened
	// those whose file is open, the most recently read first.
	recent, opened pieceList
	// writing counts the files of each torrent being written (see drop).
	writing map[Hash]int
	// named is the number in the name of the last file written.
	named uint64
	// evicted tells a torrent that it lost a piece.
	evicted func(pieceKey)
	// runs is the number of the latest run of each torrent whose pieces
	// may be kept, and run the last number given (see started).
	runs map[Hash]uint64
	run  uint64
}

// pieceFile is the file of a piece open for reads.
type pieceFile interface {
	io.ReaderAt
	io.Closer
}

type storedPiece struct {
	key  pieceKey
	size int64
	name string
	// links are the piece's places in the store's lists.
	links [2]pieceLink
	// file is the piece's file while it is open, which reads share, and
	// readers the reads under way, which keep the file on disk.
	file    pieceFile
	readers int
	// gone tells a piece removed from the index: the last of its reads
	// deletes its file. deleted tells the file is deleted.
	gone, deleted bool
}

// newStore returns a store in dir, removing what a previous run left
// there: it is not trusted. A folder Polyfin did not make is not emptied,
// and no store is returned for it (see workfolder.ErrOccupied).
func newStore(dir string, budget func() int64, logger *slog.Logger) (*store, error) {
	if err := workfolder.Torrents.Empty(dir); err != nil {
		return nil, err
	}
	s := &store{dir: dir, budget: budget, logger: logger, files: storeFiles, write: writeFile, open: openFile, unlink: os.Remove,
		pieces: map[pieceKey]*storedPiece{}, recent: pieceList{which: recentList}, opened: pieceList{which: openedList},
		writing: map[Hash]int{}, runs: map[Hash]uint64{}}
	s.changed.L = &s.mu
	return s, nil
}

// writeFile writes a piece's file, in its torrent's folder.
func writeFile(name string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		return err
	}
	return os.WriteFile(name, data, 0o600)
}

func openFile(name string) (pieceFile, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	return file, nil
}

// put keeps a piece checked against its hash, removing first the least
// recently used ones the budget has no room left for, so that the pieces
// on disk never exceed it while a piece fits it. A piece being read keeps
// its room until its reads end, and the write waits for them. A write
// that fails, as on a full disk, keeps nothing, and deletes what it
// wrote.
func (s *store) put(key pieceKey, data []byte) error {
	size := int64(len(data))
	var told []pieceKey
	s.mu.Lock()
	for {
		if s.closed {
			s.mu.Unlock()
			s.tell(told)
			return errClosed
		}
		var doomed []*storedPiece
		if old := s.pieces[key]; old != nil {
			doomed = s.unindexLocked(old, doomed)
		}
		budget := s.budget()
		for s.used-s.leaving+size > budget && s.recent.oldest != nil {
			victim := s.recent.oldest
			told = append(told, victim.key)
			doomed = s.unindexLocked(victim, doomed)
		}
		if len(doomed) > 0 {
			s.mu.Unlock()
			s.remove(doomed)
			s.mu.Lock()
			continue
		}
		if s.used+size <= budget || s.used == 0 {
			break
		}
		// The room left is that of files being written, or of pieces
		// whose reads end before their files are deleted.
		s.changed.Wait()
	}
	s.used += size
	s.writing[key.hash]++
	s.named++
	piece := &storedPiece{key: key, size: size,
		name: filepath.Join(s.dir, key.hash.String(), strconv.Itoa(key.index)+"-"+strconv.FormatUint(s.named, 10))}
	s.mu.Unlock()
	s.tell(told)
	err := s.write(piece.name, data)
	s.mu.Lock()
	if err == nil && !s.closed {
		var doomed []*storedPiece
		if old := s.pieces[key]; old != nil {
			doomed = s.unindexLocked(old, doomed)
		}
		s.pieces[key] = piece
		s.recent.push(piece)
		s.wroteLocked(key.hash)
		s.mu.Unlock()
		s.remove(doomed)
		return nil
	}
	closed := s.closed
	s.mu.Unlock()
	// Nothing is kept: a full disk leaves part of the file.
	s.deleteFile(piece.name)
	s.mu.Lock()
	s.used -= size
	s.wroteLocked(key.hash)
	s.mu.Unlock()
	if closed {
		return errClosed
	}
	return err
}

// wroteLocked tells that a write of a piece of the torrent of hash ended.
func (s *store) wroteLocked(hash Hash) {
	if s.writing[hash]--; s.writing[hash] == 0 {
		delete(s.writing, hash)
	}
	s.changed.Broadcast()
}

// tell tells the torrents the pieces they lost.
func (s *store) tell(keys []pieceKey) {
	if s.evicted == nil {
		return
	}
	for _, key := range keys {
		s.evicted(key)
	}
}

// unindexLocked removes a piece from the index: no read finds it from
// now on. Its file is deleted by the last of the reads under way, or by
// the caller when there is none: the piece is then added to doomed (see
// remove).
func (s *store) unindexLocked(piece *storedPiece, doomed []*storedPiece) []*storedPiece {
	delete(s.pieces, piece.key)
	s.recent.remove(piece)
	piece.gone = true
	s.leaving += piece.size
	if piece.readers > 0 {
		return doomed
	}
	if piece.file != nil {
		s.opened.remove(piece)
	}
	return append(doomed, piece)
}

// remove closes and deletes the files of pieces removed from the index
// that no read holds, then gives their room back.
func (s *store) remove(doomed []*storedPiece) {
	if len(doomed) == 0 {
		return
	}
	var freed int64
	for _, piece := range doomed {
		if piece.file != nil {
			_ = piece.file.Close()
		}
		s.deleteFile(piece.name)
		freed += piece.size
	}
	s.mu.Lock()
	for _, piece := range doomed {
		piece.file, piece.deleted = nil, true
	}
	s.used -= freed
	s.leaving -= freed
	s.changed.Broadcast()
	s.mu.Unlock()
}

func (s *store) deleteFile(name string) {
	if err := s.unlink(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		s.logger.Warn("A torrent's piece could not be removed", "error", err)
	}
}

// read reads a piece's bytes at off into p, false when the store no
// longer has the piece. The piece's file stays open for the next reads,
// within the bound of open files (see release).
func (s *store) read(key pieceKey, p []byte, off int64) (bool, error) {
	s.mu.Lock()
	piece := s.pieces[key]
	if piece == nil {
		s.mu.Unlock()
		return false, nil
	}
	s.recent.touch(piece)
	piece.readers++
	file := piece.file
	if file != nil {
		s.opened.touch(piece)
	}
	s.mu.Unlock()
	var opened pieceFile
	var err error
	if file == nil {
		opened, err = s.open(piece.name)
		file = opened
	}
	if err == nil {
		_, err = file.ReadAt(p, off)
	}
	s.release(piece, opened)
	return err == nil, err
}

// release ends a read of a piece. The file the read opened, when the
// piece had none open, stays open for the next reads, as long as the
// files open stay within the bound: the least recently read that no read
// uses is closed for it, if need be. The last read of a piece removed
// from the index deletes its file. A file closes before its piece's file
// may be deleted, as Windows deletes no file still open: its piece stays
// held as by a read until then.
func (s *store) release(piece *storedPiece, opened pieceFile) {
	if opened == nil {
		s.unpin(piece)
		return
	}
	var unused, idleFile pieceFile
	var idle *storedPiece
	s.mu.Lock()
	kept := false
	if piece.file == nil && !piece.gone {
		kept, idle = s.roomToOpenLocked()
	}
	if kept {
		piece.file = opened
		s.opened.push(piece)
	} else {
		unused = opened
	}
	if idle != nil {
		idleFile, idle.file = idle.file, nil
		idle.readers++
	}
	s.mu.Unlock()
	if unused != nil {
		_ = unused.Close()
	}
	if idle != nil {
		_ = idleFile.Close()
		s.unpin(idle)
	}
	s.unpin(piece)
}

// unpin ends a read of a piece, or the closing of one of its files. The
// last of a piece removed from the index deletes its file.
func (s *store) unpin(piece *storedPiece) {
	s.mu.Lock()
	piece.readers--
	last := piece.gone && piece.readers == 0
	if last && piece.file != nil {
		s.opened.remove(piece)
	}
	s.mu.Unlock()
	if last {
		s.remove([]*storedPiece{piece})
	}
}

// roomToOpenLocked tells whether one more file may stay open: the files
// open are under the bound, or the least recently read that no read uses
// is given up, returned for the caller to close, its file still set.
func (s *store) roomToOpenLocked() (bool, *storedPiece) {
	if s.opened.length < s.files {
		return true, nil
	}
	for piece := s.opened.oldest; piece != nil; piece = piece.links[openedList].newer {
		if piece.readers == 0 {
			s.opened.remove(piece)
			return true, piece
		}
	}
	return false, nil
}

// started numbers a run of the torrent of hash that starts: its pieces are
// the torrent's from now on, which an earlier run that stops no longer
// drops (see drop).
func (s *store) started(hash Hash) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.run++
	s.runs[hash] = s.run
	return s.run
}

// drop removes the pieces of a torrent whose run stopped, unless a later
// run of the torrent started meanwhile: they are that run's. The pieces
// being written are kept first, then removed with the others, and drop
// returns once their files are deleted, the reads under way ended, and
// the torrent's folder with them when no write of it started since.
func (s *store) drop(hash Hash, run uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runs[hash] != run {
		return
	}
	delete(s.runs, hash)
	for s.writing[hash] > 0 {
		s.changed.Wait()
	}
	if _, later := s.runs[hash]; later {
		return
	}
	var doomed, held []*storedPiece
	for key, piece := range s.pieces {
		if key.hash != hash {
			continue
		}
		if piece.readers > 0 {
			held = append(held, piece)
		}
		doomed = s.unindexLocked(piece, doomed)
	}
	s.mu.Unlock()
	s.remove(doomed)
	s.mu.Lock()
	for _, piece := range held {
		for !piece.deleted {
			s.changed.Wait()
		}
	}
	// A write that starts later makes the folder again.
	if s.writing[hash] == 0 {
		_ = os.Remove(filepath.Join(s.dir, hash.String()))
	}
}

// mark sets have[i] for each piece of the torrent of hash the store keeps,
// as a run stopped while played leaves them for the next (see
// Engine.remove): they were checked against their hashes as they were
// kept.
func (s *store) mark(hash Hash, have []bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key := range s.pieces {
		if key.hash == hash && key.index >= 0 && key.index < len(have) {
			have[key.index] = true
		}
	}
}

// errClosed reports a piece kept once the engine closed.
var errClosed = errors.New("the torrent engine closed")

// close removes every piece, and keeps none from now on. It returns once
// the reads and writes under way ended, and every file is deleted.
func (s *store) close() {
	s.mu.Lock()
	s.closed = true
	var doomed []*storedPiece
	for _, piece := range s.pieces {
		doomed = s.unindexLocked(piece, doomed)
	}
	s.changed.Broadcast()
	s.mu.Unlock()
	s.remove(doomed)
	s.mu.Lock()
	for s.used > 0 {
		s.changed.Wait()
	}
	s.mu.Unlock()
	_ = os.RemoveAll(s.dir)
}

// usage is the bytes of the pieces the store keeps, or is writing. Those
// of pieces removed are not, though their files may still be deleted, or
// read: the store no longer has them. A torrent's pieces count no more
// once drop removed them from the index, as Windows deletes files slowly.
func (s *store) usage() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.used - s.leaving
}

// The store's lists, each a place in the links of the pieces.
const (
	recentList = iota
	openedList
)

// pieceList lists pieces, the newest first, through their links.
type pieceList struct {
	which          int
	newest, oldest *storedPiece
	length         int
}

// pieceLink is a piece's place in a list.
type pieceLink struct {
	newer, older *storedPiece
}

// push puts a piece at the head of the list.
func (l *pieceList) push(piece *storedPiece) {
	piece.links[l.which] = pieceLink{older: l.newest}
	if l.newest != nil {
		l.newest.links[l.which].newer = piece
	} else {
		l.oldest = piece
	}
	l.newest = piece
	l.length++
}

// remove takes a piece out of the list.
func (l *pieceList) remove(piece *storedPiece) {
	link := &piece.links[l.which]
	if link.newer != nil {
		link.newer.links[l.which].older = link.older
	} else {
		l.newest = link.older
	}
	if link.older != nil {
		link.older.links[l.which].newer = link.newer
	} else {
		l.oldest = link.newer
	}
	*link = pieceLink{}
	l.length--
}

// touch moves a piece of the list to its head.
func (l *pieceList) touch(piece *storedPiece) {
	if l.newest != piece {
		l.remove(piece)
		l.push(piece)
	}
}
