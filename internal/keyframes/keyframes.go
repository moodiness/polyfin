// Package keyframes finds where a file's video keyframes are from the index
// its container keeps: the Cues of a Matroska file, the sample tables of an
// MP4 one. Files are remote and large, so the index is reached with a few
// large reads, never by reading the media itself.
package keyframes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"
)

// Reader reads a file at an offset; *source.Source is one. Every call may
// be a network round trip, so the file is read in large spans.
type Reader interface {
	ReadAt(ctx context.Context, p []byte, off int64) (int, error)
}

// ErrNoIndex reports a file without a usable index.
var ErrNoIndex = errors.New("no keyframe index")

// errInvalid reports an index that contradicts itself or its file.
var errInvalid = errors.New("invalid keyframe index")

const (
	// window is how much is read at once when a few bytes are needed:
	// element and box headers come in groups, and a source fetches whole
	// blocks anyway.
	window = 256 << 10
	// maxIndex bounds what is allocated for one index structure, Cues or
	// a sample table, so that a hostile file cannot exhaust memory.
	maxIndex = 64 << 20
	// maxKeyframes bounds the times returned, for files that claim every
	// one of a huge number of samples is a keyframe.
	maxKeyframes = 1 << 23
	// maxTopLevel bounds the top-level elements or boxes walked to find
	// the index: a few come before it, and each may cost a read.
	maxTopLevel = 64
)

// Read returns the presentation times of the first video track's keyframes
// a decoder can start from, ascending and without duplicates, as FFmpeg
// reports them. size is the file size.
func Read(ctx context.Context, r Reader, size int64) ([]time.Duration, error) {
	return read(ctx, r, size, window)
}

// read is Read with the size of the reads made for small spans, which
// tests lower to count the reads a large file would take.
func read(ctx context.Context, r Reader, size, window int64) ([]time.Duration, error) {
	f := &file{ctx: ctx, r: r, size: size, window: window}
	head, err := f.span(0, min(size, 8))
	if err != nil {
		return nil, err
	}
	var times []time.Duration
	switch {
	case bytes.HasPrefix(head, ebmlMagic):
		times, err = readMatroska(f)
	case len(head) == 8 && topLevelBoxes[string(head[4:8])]:
		times, err = readMP4(f)
	default:
		return nil, ErrNoIndex
	}
	if err != nil {
		return nil, err
	}
	slices.Sort(times)
	return slices.Compact(times), nil
}

// file reads a Reader through the last two spans read, so that the many
// small reads of headers make few calls, even when they alternate between
// the start of the file and its end.
type file struct {
	ctx    context.Context
	r      Reader
	size   int64
	window int64
	// spans holds the last two spans read, the latest used first. Their
	// bytes are never written to once read, so the slices handed out stay
	// valid when they are replaced.
	spans [2]held
}

// held is a span read from the file.
type held struct {
	start int64
	data  []byte
}

// span returns the n bytes at off, from a span held when one has them,
// else from a read of at least a window. Near the end of the file the
// read is the file's last window: indexes written after the media often
// come in pieces there, a SeekHead after the Cues it points to.
func (f *file) span(off, n int64) ([]byte, error) {
	if off < 0 || n < 0 || off > f.size || n > f.size-off {
		return nil, fmt.Errorf("reading %d bytes at %d of %d: %w", n, off, f.size, io.ErrUnexpectedEOF)
	}
	for i, h := range f.spans {
		if off >= h.start && off+n <= h.start+int64(len(h.data)) {
			f.spans[0], f.spans[i] = h, f.spans[0]
			return h.data[off-h.start:][:n], nil
		}
	}
	start, length := off, max(n, f.window)
	if length > f.size-off {
		start = max(0, f.size-length)
		length = f.size - start
	}
	p := make([]byte, length)
	read, err := f.r.ReadAt(f.ctx, p, start)
	if read < len(p) {
		if err == nil || errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return nil, fmt.Errorf("reading %d bytes at %d: %w", len(p), start, err)
	}
	f.spans[1], f.spans[0] = f.spans[0], held{start: start, data: p}
	return p[off-start:][:n], nil
}

// duration converts ticks of a timescale to a duration, failing rather
// than overflowing.
func duration(ticks int64, timescale uint32) (time.Duration, error) {
	if timescale == 0 {
		return 0, fmt.Errorf("zero timescale: %w", errInvalid)
	}
	seconds, rest := ticks/int64(timescale), ticks%int64(timescale)
	if seconds > maxSeconds || seconds < -maxSeconds {
		return 0, fmt.Errorf("time %d/%d out of range: %w", ticks, timescale, errInvalid)
	}
	return time.Duration(seconds)*time.Second + time.Duration(rest*int64(time.Second)/int64(timescale)), nil
}

// maxSeconds is the furthest a duration reaches, with room for the
// fraction added to it.
const maxSeconds = int64(1<<63-1)/int64(time.Second) - 1
