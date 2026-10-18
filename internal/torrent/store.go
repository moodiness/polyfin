package torrent

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// pieceKey names a piece of a torrent.
type pieceKey struct {
	hash  Hash
	index int
}

// store keeps the pieces of the torrents on disk, one file each, within
// the bytes budget gives, read at each write: the least recently used
// pieces are removed first, and their torrent told, which no longer has
// them.
type store struct {
	dir    string
	budget func() int64
	logger *slog.Logger

	mu     sync.Mutex
	closed bool
	used   int64
	pieces map[pieceKey]*storedPiece
	// evicted tells a torrent that it lost a piece.
	evicted func(pieceKey)
}

type storedPiece struct {
	size int64
	used time.Time
}

// newStore returns a store in dir, removing what a previous run left
// there: it is not trusted.
func newStore(dir string, budget func() int64, logger *slog.Logger) (*store, error) {
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &store{dir: dir, budget: budget, logger: logger, pieces: map[pieceKey]*storedPiece{}}, nil
}

func (s *store) path(key pieceKey) string {
	return filepath.Join(s.dir, key.hash.String(), strconv.Itoa(key.index))
}

// put keeps a piece checked against its hash, removing first the least
// recently used ones the budget has no room left for, so that the pieces
// on disk never exceed it while a piece fits it.
func (s *store) put(key pieceKey, data []byte) error {
	var gone []pieceKey
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errClosed
	}
	s.removeLocked(key)
	for budget := s.budget(); s.used+int64(len(data)) > budget; {
		var oldest pieceKey
		var found *storedPiece
		for k, p := range s.pieces {
			if found == nil || p.used.Before(found.used) {
				oldest, found = k, p
			}
		}
		if found == nil {
			break
		}
		s.removeLocked(oldest)
		gone = append(gone, oldest)
	}
	err := os.MkdirAll(filepath.Dir(s.path(key)), 0o700)
	if err == nil {
		err = os.WriteFile(s.path(key), data, 0o600)
	}
	if err == nil {
		s.pieces[key] = &storedPiece{size: int64(len(data)), used: time.Now()}
		s.used += int64(len(data))
	}
	evicted := s.evicted
	s.mu.Unlock()
	for _, k := range gone {
		if evicted != nil {
			evicted(k)
		}
	}
	return err
}

func (s *store) removeLocked(key pieceKey) {
	p := s.pieces[key]
	if p == nil {
		return
	}
	delete(s.pieces, key)
	s.used -= p.size
	if err := os.Remove(s.path(key)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		s.logger.Warn("A torrent's piece could not be removed", "error", err)
	}
}

// read reads a piece's bytes at off into p, false when the store no
// longer has the piece.
func (s *store) read(key pieceKey, p []byte, off int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	piece := s.pieces[key]
	if piece == nil {
		return false, nil
	}
	piece.used = time.Now()
	file, err := os.Open(s.path(key))
	if err != nil {
		return false, err
	}
	defer file.Close()
	if _, err := file.ReadAt(p, off); err != nil {
		return false, err
	}
	return true, nil
}

// drop removes the pieces of a torrent.
func (s *store) drop(hash Hash) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key := range s.pieces {
		if key.hash == hash {
			s.removeLocked(key)
		}
	}
	_ = os.Remove(filepath.Join(s.dir, hash.String()))
}

// errClosed reports a piece kept once the engine closed.
var errClosed = errors.New("the torrent engine closed")

// close removes every piece, and keeps none from now on.
func (s *store) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for key := range s.pieces {
		s.removeLocked(key)
	}
	_ = os.RemoveAll(s.dir)
}

// usage is the bytes the store keeps.
func (s *store) usage() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.used
}
