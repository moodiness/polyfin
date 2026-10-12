package audiosegments

import (
	"math/bits"
	"time"
)

const (
	// maxBitErrors is how many of their 32 bits two frames may differ by
	// and still sound the same: the same sound decoded from two files
	// differs by a few, unrelated sounds by about 16.
	maxBitErrors = 8
	// closeBy is how far apart, in frames, two frames that sound the same
	// must be for either to count: unrelated sounds give a few frames that
	// sound the same, alone, which would otherwise stretch a stretch shared
	// past its end.
	closeBy = 3
	// maxGap is how many frames in a row that do not count a stretch two
	// fingerprints share may hold, about a second: a few frames of the
	// same sound differ more, where a codec dropped more of it.
	maxGap = 15
)

// sameFrame reports whether two frames sound the same: neither is silent
// and they differ by maxBitErrors bits at most.
func sameFrame(a, b uint32) bool {
	return a != 0 && b != 0 && bits.OnesCount32(a^b) <= maxBitErrors
}

// span is a part of a file, from Start to End.
type span struct {
	Start, End time.Duration
}

func (s span) length() time.Duration { return s.End - s.Start }

// match is a stretch of audio two fingerprints share: where it is in the
// file of the first, a, and in that of the second, b.
type match struct {
	a, b span
}

// shared finds the longest stretch of audio two fingerprints share
// lasting from shortest to longest, at any place in each. A stretch
// longer than longest is no such stretch, not even in part: two files of
// the same episode, or two that share a long recap, share minutes. Each
// shift of one fingerprint against the other is tried: a stretch is
// frames that sound the same (see sameFrame) at that shift, closeBy frames
// apart at most, but for gaps of maxGap frames at most.
func shared(a, b Fingerprint, shortest, longest time.Duration) (match, bool) {
	var best match
	found := false
	consider := func(shift, first, last int) {
		found, best = longer(a, b, shift, first, last, shortest, longest, found, best)
	}
	for shift := -(len(a.Frames) - 1); shift < len(b.Frames); shift++ {
		from, to := max(0, -shift), min(len(a.Frames), len(b.Frames)-shift)
		first, last, previous := -1, -1, -1
		for i := from; i < to; i++ {
			if !sameFrame(a.Frames[i], b.Frames[i+shift]) {
				continue
			}
			if previous >= 0 && i-previous <= closeBy {
				if first >= 0 && previous-last > maxGap+1 {
					consider(shift, first, last)
					first = -1
				}
				if first < 0 {
					first = previous
				}
				last = i
			}
			previous = i
		}
		if first >= 0 {
			consider(shift, first, last)
		}
	}
	return best, found
}

// longer keeps the stretch of frames first to last of a, at shift in b,
// over best when it lasts from shortest to longest and longer than best.
func longer(a, b Fingerprint, shift, first, last int, shortest, longest time.Duration, found bool, best match) (bool, match) {
	stretch := match{
		a: span{a.at(first), a.at(last)},
		b: span{b.at(first + shift), b.at(last + shift)},
	}
	length := stretch.a.length()
	if length < shortest || length > longest || found && length <= best.a.length() {
		return found, best
	}
	return true, stretch
}
