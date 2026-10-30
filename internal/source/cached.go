package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/moodiness/polyfin/internal/accounts"
)

// ErrNotCached reports bytes of a source the cache does not hold: Cached
// reads them with no request.
var ErrNotCached = errors.New("not in the source cache")

// Cached reads what the cache holds of a source, and nothing else: never a
// request to its host, never a wait, never a block marked as read lately.
// Thumbnails made during a play read the keyframes the playback brought in
// so.
type Cached struct {
	s *Source
}

// Cached returns what the cache holds of the source of id, false when it
// holds no such source.
func (c *Cache) Cached(id accounts.ID) (Cached, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.sources[id]
	return Cached{s: s}, s != nil
}

// KnownSize returns the size of the source, once it answered.
func (c Cached) KnownSize() (int64, bool) {
	return c.s.knownSize()
}

// ReadAt reads len(p) bytes at off from the blocks cached, failing with
// ErrNotCached at the first that is not.
func (c Cached) ReadAt(_ context.Context, p []byte, off int64) (int, error) {
	s := c.s
	n := 0
	for n < len(p) {
		position := off + int64(n)
		size, known := s.knownSize()
		if known && position >= size {
			return n, io.EOF
		}
		block := position / blockSize
		s.mu.Lock()
		has := s.has(block)
		s.mu.Unlock()
		if !has {
			return n, fmt.Errorf("%w: %d bytes at %d", ErrNotCached, len(p), off)
		}
		end := (block + 1) * blockSize
		if known {
			end = min(end, size)
		}
		part := p[n:min(len(p), n+int(end-position))]
		index := block / s.cache.chunkBlocks
		read, err := readFileAt(s.chunkPath(index), part, position-index*s.cache.chunkBlocks*blockSize)
		// A block evicted meanwhile may have left its chunk's file gone, or
		// holding other blocks alone.
		s.mu.Lock()
		has = s.has(block)
		s.mu.Unlock()
		switch {
		case errors.Is(err, fs.ErrNotExist) || !has:
			return n, fmt.Errorf("%w: %d bytes at %d", ErrNotCached, len(p), off)
		case err != nil && (!errors.Is(err, io.EOF) || read < len(part)):
			return n + read, err
		}
		n += read
	}
	return n, nil
}

// Fetch returns the n bytes at off, or fewer at the end of the source.
func (c Cached) Fetch(ctx context.Context, off int64, n int) ([]byte, error) {
	if size, known := c.KnownSize(); known {
		n = int(max(0, min(int64(n), size-off)))
	}
	p := make([]byte, n)
	read, err := c.ReadAt(ctx, p, off)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return p[:read], err
}

func readFileAt(path string, p []byte, off int64) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	return file.ReadAt(p, off)
}
