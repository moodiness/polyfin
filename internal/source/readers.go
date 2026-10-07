package source

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"slices"
)

// Reader reads a source on behalf of one request: an FFmpeg or ffprobe
// connection to the loopback address, a player's relayed request, or a
// read of the container's index. The blocks it waits for are tagged with
// it: the fetch serves the newest reader first, and a reader that gave up
// or closed stops wanting them. Only the newest streaming reader moves the
// window read ahead of it, so that a request FFmpeg is leaving, or a job
// replaced, does not pull its connection back to where it was.
type Reader struct {
	s         *Source
	streaming bool
	// guarded by the source's lock: closed, waiting is the block waited
	// for, -1 for none, and from and last the blocks a run of sequential
	// reads started at and last read.
	closed      bool
	waiting     int64
	from, last  int64
	sequentials int64
}

// window is the read-ahead window of a streaming reader: the blocks from
// cursor to horizon are fetched ahead of it.
type window struct {
	owner           *Reader
	cursor, horizon int64
}

// NewReader returns a reader of the source for one request that streams
// it, reading ahead of it as long as it is the newest one: by a few tens
// of MiB as it starts, by up to the cache's budget once it read as much
// in order. The caller closes it once done.
func (s *Source) NewReader() *Reader {
	return s.reader(true)
}

func (s *Source) reader(streaming bool) *Reader {
	r := &Reader{s: s, streaming: streaming, waiting: -1, last: -1}
	s.mu.Lock()
	s.readers = append(s.readers, r)
	if streaming {
		s.attached++
	}
	s.mu.Unlock()
	return r
}

// Close ends the reader: it no longer wants anything, and its window, if
// it had it, closes.
func (r *Reader) Close() {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.closed {
		return
	}
	r.closed = true
	s.readers = slices.DeleteFunc(s.readers, func(other *Reader) bool { return other == r })
	if r.streaming {
		s.attached--
	}
	if s.window.owner == r {
		s.window = window{}
	}
}

// Size returns the size of the source, reaching it if needed. A source that
// does not announce it is read to its end.
func (r *Reader) Size(ctx context.Context) (int64, error) {
	for block := int64(0); ; block++ {
		if size, known := r.s.knownSize(); known {
			return size, nil
		}
		if err := r.s.wait(ctx, r, block); err != nil && !errors.Is(err, io.EOF) {
			return 0, err
		}
	}
}

// ReadAt reads len(p) bytes at off, waiting for the blocks not cached yet
// at most until ctx is done.
func (r *Reader) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	s := r.s
	n := 0
	for refetched := false; n < len(p); {
		position := off + int64(n)
		if size, known := s.knownSize(); known && position >= size {
			return n, io.EOF
		}
		block := position / blockSize
		if err := s.wait(ctx, r, block); err != nil {
			return n, err
		}
		end := (block + 1) * blockSize
		if size, known := s.knownSize(); known {
			end = min(end, size)
		}
		part := p[n:min(len(p), n+int(end-position))]
		index := block / s.cache.chunkBlocks
		read, err := s.readChunk(index, part, position-index*s.cache.chunkBlocks*blockSize)
		if errors.Is(err, fs.ErrNotExist) && !refetched {
			// The chunk was evicted between the wait and the read: fetch it
			// again, once.
			s.forget(index)
			refetched = true
			continue
		}
		n += read
		if err != nil && (!errors.Is(err, io.EOF) || read < len(part)) {
			return n, err
		}
		refetched = false
	}
	return n, nil
}

// moveWindow places the read-ahead window after block, which r reads, when
// r is the newest streaming reader. The window grows with what r read in
// order: twice as much, from the cache's readahead to its budget. The
// source's lock is held.
func (s *Source) moveWindow(r *Reader, block int64) {
	if r.last < 0 || block < r.last || block > r.last+skipLimit {
		r.from = block
	}
	r.last = block
	if s.newestStreaming() != r {
		return
	}
	ahead := min(max(2*(block-r.from), s.cache.readahead), max(s.cache.aheadBudget(), s.cache.readahead))
	s.window = window{owner: r, cursor: block, horizon: block + 1 + ahead}
}

// newestStreaming is the newest streaming reader open, or nil. The
// source's lock is held.
func (s *Source) newestStreaming() *Reader {
	for i := len(s.readers) - 1; i >= 0; i-- {
		if s.readers[i].streaming {
			return s.readers[i]
		}
	}
	return nil
}

// newest is the newest reader open, or nil. The source's lock is held.
func (s *Source) newest() *Reader {
	if len(s.readers) == 0 {
		return nil
	}
	return s.readers[len(s.readers)-1]
}

// missingAhead reports whether a block of the read-ahead window is not
// cached yet. The source's lock is held.
func (s *Source) missingAhead() bool {
	if s.window.owner == nil {
		return false
	}
	for block := s.window.cursor; block < s.window.horizon && !s.pastEnd(block); block++ {
		if !s.has(block) {
			return true
		}
	}
	return false
}

// Span is a range of a source's bytes, from Off to End, excluded.
type Span struct {
	Off, End int64
}

// warmSpans are the blocks a Warm keeps read, first block included, last
// excluded, by span.
type warmSpans struct {
	spans [][2]int64
}

// Warm reads spans of the source into the cache ahead of any reader, after
// what readers wait for and read ahead: the bytes a playback about to start
// reads first. It returns once they are all cached, the source failed, or
// ctx is done.
func (s *Source) Warm(ctx context.Context, spans ...Span) error {
	warm := &warmSpans{}
	for _, span := range spans {
		if span.End > span.Off && span.Off >= 0 {
			warm.spans = append(warm.spans, [2]int64{span.Off / blockSize, (span.End + blockSize - 1) / blockSize})
		}
	}
	if len(warm.spans) == 0 {
		return nil
	}
	s.mu.Lock()
	s.warms = append(s.warms, warm)
	defer func() {
		s.mu.Lock()
		s.warms = slices.DeleteFunc(s.warms, func(other *warmSpans) bool { return other == warm })
		s.mu.Unlock()
	}()
	for {
		if s.coolingLocked() {
			err := s.failure
			s.mu.Unlock()
			return err
		}
		if !s.warmMissing(warm) {
			s.mu.Unlock()
			return nil
		}
		s.kick()
		progress := s.progress
		s.mu.Unlock()
		select {
		case <-progress:
		case <-ctx.Done():
			return ctx.Err()
		}
		s.mu.Lock()
	}
}

// warmMissing reports whether a block a warm keeps read is not cached.
// The source's lock is held.
func (s *Source) warmMissing(warm *warmSpans) bool {
	for _, span := range warm.spans {
		for block := span[0]; block < span[1]; block++ {
			if s.missing(block) {
				return true
			}
		}
	}
	return false
}
