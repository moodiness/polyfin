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
	// targetDuration is the length segments aim for: each ends on the first
	// keyframe this long after its start.
	targetDuration = 6 * time.Second
	// seekMargin keeps a keyframe followed this closely by another from
	// starting a segment: FFmpeg, asked to start on it, could land on the
	// next one (see seekTime).
	seekMargin = 250 * time.Millisecond
	// shortestTail is the shortest a last segment may be: a keyframe closer
	// to the end does not start one.
	shortestTail = time.Second
)

// Plan is how a version is cut: where each segment starts, in the source's
// presentation time.
type Plan struct {
	starts   []time.Duration
	duration time.Duration
	// grid marks a plan cut every targetDuration, not on keyframes (see
	// NewGridPlan).
	grid bool
}

// NewGridPlan cuts a version lasting duration, whose keyframes are not
// known, such as an MPEG-TS file, every targetDuration. Only converted
// video, whose keyframes the encoder places at each segment's start, can
// be cut so: FFmpeg starts on an earlier keyframe, and what comes before
// the segment is left out (see encoding.take).
func NewGridPlan(duration time.Duration) Plan {
	starts := []time.Duration{0}
	for at := targetDuration; at <= duration-shortestTail; at += targetDuration {
		starts = append(starts, at)
	}
	return Plan{starts: starts, duration: duration, grid: true}
}

// Grid reports whether the plan is cut every targetDuration rather than
// on the version's keyframes: its segments need the video converted.
func (p Plan) Grid() bool { return p.grid }

// NewPlan cuts a version lasting duration into segments starting on its
// keyframes, about targetDuration long. The first segment starts at zero,
// whatever the first keyframe.
func NewPlan(keyframes []time.Duration, duration time.Duration) Plan {
	starts := []time.Duration{0}
	for i, keyframe := range keyframes {
		if keyframe <= 0 || keyframe > duration-shortestTail {
			continue
		}
		if i+1 < len(keyframes) && keyframes[i+1]-keyframe < seekMargin {
			continue
		}
		if keyframe-starts[len(starts)-1] >= targetDuration {
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

// seekTime is where FFmpeg is asked to start for segment n. Its command
// line seeks 3/23 s before the time it is given when the video has
// B-frames, and its demuxers then go back to the closest keyframe: asking
// a little past the keyframe a segment starts on lands on it in both
// cases, as no other keyframe follows it within seekMargin.
func (p Plan) seekTime(n int) time.Duration {
	return p.starts[n] + 132*time.Millisecond
}
