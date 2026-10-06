package thumbnails

import (
	"cmp"
	"math/bits"
	"slices"
	"sort"
	"time"
)

// nearest returns the index of the time in times, ascending and not
// empty, nearest t: the earlier of two as near.
func nearest(times []time.Duration, t time.Duration) int {
	i := sort.Search(len(times), func(i int) bool { return times[i] >= t })
	switch {
	case i == len(times):
		return i - 1
	case i > 0 && t-times[i-1] <= times[i]-t:
		return i - 1
	}
	return i
}

// thumbnailCount is how many thumbnails a version lasting duration has,
// one every interval from its start, as Jellyfin makes them.
func thumbnailCount(duration, interval time.Duration) int {
	return int((duration + interval - 1) / interval)
}

// thumbnailTimes are the times of the thumbnails of a version lasting
// duration, one every interval from its start.
func thumbnailTimes(duration, interval time.Duration) []time.Duration {
	times := make([]time.Duration, thumbnailCount(duration, interval))
	for i := range times {
		times[i] = time.Duration(i) * interval
	}
	return times
}

// stepFor is the interval between the thumbnails of a version lasting
// duration when reads keyframes may be read: the interval asked, unless
// that makes more thumbnails than reads, whose images would repeat; then
// the runtime divided among the reads, rounded up to the second, so that
// each thumbnail shows a keyframe of its own.
func stepFor(duration, asked time.Duration, reads int) time.Duration {
	if reads < 1 || thumbnailCount(duration, asked) <= reads {
		return asked
	}
	step := (duration + time.Duration(reads) - 1) / time.Duration(reads)
	return max(asked, (step+time.Second-1)/time.Second*time.Second)
}

// chooseKeyframes returns the keyframes, by index, nearest the targets,
// each once, coarse to fine: the first ones spread over the whole
// runtime, the next ones between them. Read in that order, the keyframes
// read when the budget runs out still cover the version evenly.
func chooseKeyframes(keyframes, targets []time.Duration) []int {
	chosen := make([]int, 0, len(targets))
	for _, t := range targets {
		chosen = append(chosen, nearest(keyframes, t))
	}
	slices.Sort(chosen)
	chosen = slices.Compact(chosen)
	order := coarseToFine(len(chosen))
	result := make([]int, len(chosen))
	for i, position := range order {
		result[i] = chosen[position]
	}
	return result
}

// coarseToFine orders the positions 0 to n-1 by their bits reversed: 0,
// then n/2, then n/4 and 3n/4, and so on.
func coarseToFine(n int) []int {
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	if n < 2 {
		return order
	}
	width := bits.Len(uint(n - 1))
	slices.SortFunc(order, func(a, b int) int {
		ra, rb := bits.Reverse(uint(a))>>(bits.UintSize-width), bits.Reverse(uint(b))>>(bits.UintSize-width)
		return cmp.Compare(ra, rb)
	})
	return order
}

// shown returns, for each time asked, the position among read, the times
// of the keyframes read in any order, of the one nearest it.
func shown(read, asked []time.Duration) []int {
	sorted := make([]int, len(read))
	for i := range sorted {
		sorted[i] = i
	}
	slices.SortStableFunc(sorted, func(a, b int) int { return cmp.Compare(read[a], read[b]) })
	times := make([]time.Duration, len(sorted))
	for i, position := range sorted {
		times[i] = read[position]
	}
	result := make([]int, len(asked))
	for i, t := range asked {
		result[i] = sorted[nearest(times, t)]
	}
	return result
}
