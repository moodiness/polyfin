// Package hls remuxes versions into HLS on demand: it cuts them into
// segments on their own keyframes, writes the playlists, and runs FFmpeg
// from the segment a player asks for, a bounded distance ahead of it.
package hls

import (
	"math"
	"slices"
	"time"
)

const (
	// firstDuration is the length the first segment aims for, and
	// targetDuration that of the others: each ends on the first keyframe
	// this long after its start. A short first segment is the one a player
	// waits for before it shows anything, after a start or a seek; 4 s
	// segments then cost few more requests than longer ones while each
	// seek downloads less picture before it plays.
	firstDuration  = 2 * time.Second
	targetDuration = 4 * time.Second
	// seekMargin keeps a keyframe followed this closely by another from
	// starting a segment: FFmpeg, asked to start on it, could land on the
	// next one (see seekTime).
	seekMargin = 250 * time.Millisecond
	// shortestTail is the shortest a last segment may be: a keyframe closer
	// to the end does not start one.
	shortestTail = time.Second
	// fastLength is how long a fast segment aims to be, and fastSpan, two
	// of them, the length a segment must pass to be cut into fast ones
	// (see FastFrom).
	fastLength = 2 * time.Second
	fastSpan   = 2 * fastLength
)

// segmentLength is how long segment n aims to be.
func segmentLength(n int) time.Duration {
	if n == 0 {
		return firstDuration
	}
	return targetDuration
}

// Plan is how a version is cut: where each segment starts, in the source's
// presentation time.
type Plan struct {
	starts   []time.Duration
	duration time.Duration
	// grid marks a plan cut on a fixed grid, not on keyframes (see
	// NewGridPlan).
	grid bool
	// searched marks a plan whose version FFmpeg seeks in by searching its
	// timestamps (see Searched).
	searched bool
	// fastFrom and fastTo are the plan's fast segments, from fastFrom up
	// to fastTo excluded, both zero for none (see FastFrom).
	fastFrom, fastTo int
}

// NewGridPlan cuts a version lasting duration, whose keyframes are not
// known, such as an MPEG-TS file, as NewPlan would cut one with a
// keyframe everywhere: a first segment of firstDuration, then one every
// targetDuration. Only converted video, whose keyframes the encoder
// places at each segment's start, can be cut so: FFmpeg starts on an
// earlier keyframe, and what comes before the segment is left out (see
// encoding.take).
func NewGridPlan(duration time.Duration) Plan {
	starts := []time.Duration{0}
	for at := firstDuration; at <= duration-shortestTail; at += segmentLength(len(starts)) {
		starts = append(starts, at)
	}
	return Plan{starts: starts, duration: duration, grid: true}
}

// Grid reports whether the plan is cut on a fixed grid rather than on
// the version's keyframes: its segments need the video converted.
func (p Plan) Grid() bool { return p.grid }

// Searched returns the plan of a version FFmpeg seeks in by searching its
// timestamps, as its MPEG-TS and MPEG program stream demuxers do: it then
// lands on the first keyframe after the time asked, not on the one before
// (see seekTime). Nor does FFmpeg probe less of it before a seek: an
// MPEG-TS stream needs a full probe to find every track.
func (p Plan) Searched() Plan {
	p.searched = true
	return p
}

// IsSearched reports whether FFmpeg seeks in the plan's version by
// searching its timestamps (see Searched): only then does a lookback
// apply (see Remux.Lookback).
func (p Plan) IsSearched() bool { return p.searched }

// NewPlan cuts a version lasting duration into segments starting on its
// keyframes: the first ends on the first keyframe at least firstDuration
// in, each next one on the first keyframe at least targetDuration after
// its start. The first segment starts at zero, whatever the first
// keyframe.
func NewPlan(keyframes []time.Duration, duration time.Duration) Plan {
	starts := []time.Duration{0}
	for i, keyframe := range keyframes {
		if keyframe <= 0 || keyframe > duration-shortestTail {
			continue
		}
		if i+1 < len(keyframes) && keyframes[i+1]-keyframe < seekMargin {
			continue
		}
		if keyframe-starts[len(starts)-1] >= segmentLength(len(starts)-1) {
			starts = append(starts, keyframe)
		}
	}
	return Plan{starts: starts, duration: duration}
}

// Len is the number of segments.
func (p Plan) Len() int {
	return len(p.starts)
}

// Start is when segment n starts.
func (p Plan) Start(n int) time.Duration {
	return p.starts[n]
}

// End is when segment n ends: when the next starts, or the version ends.
func (p Plan) End(n int) time.Duration {
	if n+1 < len(p.starts) {
		return p.starts[n+1]
	}
	return p.duration
}

// Segment is the segment playing at t.
func (p Plan) Segment(t time.Duration) int {
	n, found := slices.BinarySearch(p.starts, t)
	if !found {
		n--
	}
	return max(n, 0)
}

// TargetDuration is the playlist's EXT-X-TARGETDURATION: the longest
// segment in whole seconds, rounded up.
func (p Plan) TargetDuration() int {
	longest := time.Duration(0)
	for n := range p.starts {
		longest = max(longest, p.End(n)-p.Start(n))
	}
	return int(math.Ceil(longest.Seconds()))
}

// FastFrom returns the plan of a play starting at t that starts with fast
// segments, which an encoding makes quickly at a lower quality (see
// Remux.Fast): the segment playing at t is cut anew every fastLength, its
// last part up to twice as long. A player waits for its first segment
// before it shows anything: the first fast segment comes once fastLength
// of the version is read and converted, rather than the whole segment.
// The segments before and after keep their times, and the fast segments
// end on the keyframe the next one starts on. It reports false, and
// returns p, when the segment playing at t lasts fastSpan or less: its
// picture comes about as soon without fast segments.
func (p Plan) FastFrom(t time.Duration) (Plan, bool) {
	if len(p.starts) == 0 {
		return p, false
	}
	from := p.Segment(t)
	start, end := p.Start(from), p.End(from)
	if end-start <= fastSpan {
		return p, false
	}
	starts := slices.Clone(p.starts[:from])
	for at := start; ; at += fastLength {
		starts = append(starts, at)
		if end-at-fastLength < fastLength {
			break
		}
	}
	p.fastFrom, p.fastTo = from, len(starts)
	p.starts = append(starts, p.starts[from+1:]...)
	return p, true
}

// HasFast reports whether the plan has fast segments (see FastFrom).
func (p Plan) HasFast() bool {
	return p.fastTo > 0
}

// Fast reports whether segment n is one of the plan's fast segments (see
// FastFrom).
func (p Plan) Fast(n int) bool {
	return n >= p.fastFrom && n < p.fastTo
}

// switches reports whether segment n is made otherwise than the one
// before it: the first fast segment, or the first after them. A playlist
// marks it with a discontinuity.
func (p Plan) switches(n int) bool {
	return n > 0 && p.fastTo > 0 && (n == p.fastFrom || n == p.fastTo)
}

// stretchEnd is the first segment after n made otherwise than n: the
// first fast segment for one before them, the first after them for a fast
// one, else the plan's length.
func (p Plan) stretchEnd(n int) int {
	switch {
	case p.fastTo == 0:
	case n < p.fastFrom:
		return p.fastFrom
	case n < p.fastTo:
		return p.fastTo
	}
	return len(p.starts)
}

// seekPast is how far past the time FFmpeg should start at it is asked
// for (see seekTime).
const seekPast = 132 * time.Millisecond

// seekTime is where FFmpeg is asked to start for segment n. Its command
// line seeks 3/23 s before the time it is given when the video has
// B-frames, and its demuxers then go back to the closest keyframe: asking
// a little past the keyframe a segment starts on lands on it in both
// cases, as no other keyframe follows it within seekMargin.
//
// A searched version is asked for the previous segment's start, or for
// lookback before segment n's start when that is earlier: its demuxer
// goes on to the next keyframe, and what it reads before segment n is
// left out (see encoding.take). That keyframe comes at the latest on
// segment n's start when the keyframes are no further apart than the
// distance asked before it, as they are in a plan cut on them; a job that
// lands past it starts again with a longer lookback (see
// encoding.searchFurther). Zero, when lookback reaches the version's
// start, has FFmpeg read it from there, without seeking.
func (p Plan) seekTime(n int, lookback time.Duration) time.Duration {
	if !p.searched {
		return p.starts[n] + seekPast
	}
	return max(min(p.starts[n-1]+seekPast, p.starts[n]-lookback), 0)
}
