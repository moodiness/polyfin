package thumbnails

import (
	"cmp"
	"math/bits"
	"slices"
	"sort"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
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

// longestVersion is the longest a version with thumbnails lasts at the
// shortest interval of the settings: ten days, well above the longest
// recordings and mixes.
const longestVersion = 240 * time.Hour

// maxThumbnails is the most thumbnails a version has: those of
// longestVersion at the shortest interval, 172,800 in 1,728 tiles. A
// version whose duration gives more at the interval of the settings gets
// none, as one without a duration: what is counted, sized and kept by
// the thumbnails of a version then follows no number its file states
// alone.
const maxThumbnails = int(longestVersion / (accounts.MinTrickplayInterval * time.Second))

// What a set keeps in memory until it is whole is bounded in bytes too,
// as a count bounds neither a thumbnail's weight nor a keyframe's:
//
//   - maxTileBytes is the most the tiles of one set weigh, 256 MiB: a day
//     at the shortest interval and the largest width takes about half of
//     it. A set past it is given up, as one past maxThumbnails, and it
//     is also the most one set writes.
//   - maxFrameBytes is the most the images kept for one set weigh until
//     its tiles are made, 2 GiB: each keyframe's thumbnail, encoded as a
//     JPEG as soon as it is decoded, and each chapter's image. A
//     thumbnail encoded alone weighs about its place in a tile, so a set
//     within maxTileBytes keeps about as much; the bound guards against
//     thumbnails that would weigh far more alone. A set past it is given
//     up. During a play, thumbnails go into their tiles as their
//     keyframes are decoded, and a pass keeps its tiles alone.
const (
	maxTileBytes  int64 = 256 << 20
	maxFrameBytes int64 = 2 << 30
)

// thumbnailCount is how many thumbnails a version lasting duration has,
// one every interval from its start, as Jellyfin makes them: none
// without a duration.
func thumbnailCount(duration, interval time.Duration) int {
	if duration <= 0 || interval <= 0 {
		return 0
	}
	return int((duration-1)/interval) + 1
}

// spreads reports whether a version lasting duration has thumbnails every
// interval: it has a duration, and they are maxThumbnails at most.
func spreads(duration, interval time.Duration) bool {
	return duration > 0 && interval > 0 && (duration-1)/interval < time.Duration(maxThumbnails)
}

// thumbnailTimes are the times of the thumbnails of a version lasting
// duration, one every interval from its start: none for a version
// thumbnails do not spread over (see spreads), whose count sizes nothing.
func thumbnailTimes(duration, interval time.Duration) []time.Duration {
	if !spreads(duration, interval) {
		return nil
	}
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
