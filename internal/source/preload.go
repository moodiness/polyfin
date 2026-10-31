package source

import (
	"context"
	"errors"
	"time"
)

// preloadPiece is how much Preload reads at a time, between its checks.
const preloadPiece = 8 << 20

// Preloading bounds what Preload reads.
type Preloading struct {
	// Rate is the bytes a second Preload fetches at most, on average.
	Rate int64
	// Reserve is the space Preload leaves free on the disk of the cache's
	// folder.
	Reserve int64
	// Budget is the bytes Preload fetches at most, 0 for no bound.
	Budget int64
	// Idle reports whether Preload may read on: nothing plays. Nil for
	// always.
	Idle func() bool
}

// ErrPreloadHeld reports a preload stopped before its end: something
// played, the disk reached its reserve, or the budget was spent.
var ErrPreloadHeld = errors.New("the preload was held back")

// Preload reads into the cache the spans of the source that spans gives
// for its size, for a play that may come: in the background, as Warm
// does, in pieces of preloadPiece. Before each piece, nothing may play,
// the disk of the cache's folder must keep the reserve free past it, and
// the budget must hold it; the pieces are spaced so that what is fetched
// averages at most the rate. Pieces already cached cost nothing. It
// returns the bytes fetched, with ErrPreloadHeld when a check stopped it.
func (s *Source) Preload(ctx context.Context, p Preloading, spans func(size int64) []Span) (int64, error) {
	c := s.cache
	s.mu.Lock()
	sized, head := s.size >= 0, !s.has(0)
	s.mu.Unlock()
	if !sized && !c.mayPreload(p, blockSize) {
		return 0, ErrPreloadHeld
	}
	started := c.now()
	size, err := s.Size(ctx)
	if err != nil {
		return 0, err
	}
	var fetched int64
	if !sized && head {
		// Reaching the source read its first block.
		fetched = min(blockSize, size)
	}
	for _, span := range spans(size) {
		span.Off, span.End = max(span.Off, 0), min(span.End, size)
		for off := span.Off; off < span.End; off += preloadPiece {
			piece := Span{Off: off, End: min(off+preloadPiece, span.End)}
			missing := s.missingBytes(piece)
			if missing == 0 {
				continue
			}
			if p.Budget > 0 && fetched+missing > p.Budget || !c.mayPreload(p, missing) {
				return fetched, ErrPreloadHeld
			}
			if err := s.Warm(ctx, piece); err != nil {
				return fetched, err
			}
			fetched += missing
			if p.Rate > 0 {
				due := started.Add(time.Duration(float64(fetched) / float64(p.Rate) * float64(time.Second)))
				if wait := due.Sub(c.now()); wait > 0 {
					if err := c.sleep(ctx, wait); err != nil {
						return fetched, err
					}
				}
			}
		}
	}
	return fetched, nil
}

// mayPreload reports whether Preload may fetch n bytes more: nothing
// plays, and the disk keeps its reserve past them.
func (c *Cache) mayPreload(p Preloading, n int64) bool {
	if p.Idle != nil && !p.Idle() {
		return false
	}
	free, ok := c.free()
	return !ok || free-n >= p.Reserve
}

// missingBytes is how many bytes the blocks of span not cached yet hold,
// those Warm fetches for it. The source's size is known.
func (s *Source) missingBytes(span Span) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var missing int64
	for block := span.Off / blockSize; block*blockSize < span.End; block++ {
		if s.missing(block) {
			missing += min(blockSize, s.size-block*blockSize)
		}
	}
	return missing
}
