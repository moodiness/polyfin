package thumbnails

import (
	"slices"
	"sort"
	"time"

	"github.com/moodiness/polyfin/internal/media"
)

// shots are the images made of distinct keyframes, and which of them each
// image shows: images[i] is the index in keyframes of the keyframe image i
// shows. keyframes are indexes in the version's keyframes, ascending.
type shots struct {
	keyframes []int
	images    []int
}

// nearest returns the index of the keyframe nearest t, the earlier of two
// as near. keyframes is ascending and not empty.
func nearest(keyframes []time.Duration, t time.Duration) int {
	i := sort.Search(len(keyframes), func(i int) bool { return keyframes[i] >= t })
	switch {
	case i == len(keyframes):
		return i - 1
	case i > 0 && t-keyframes[i-1] <= keyframes[i]-t:
		return i - 1
	}
	return i
}

// thumbnailCount is how many thumbnails a version lasting duration has,
// one every interval from its start, as Jellyfin makes them.
func thumbnailCount(duration, interval time.Duration) int {
	return int((duration + interval - 1) / interval)
}

// planThumbnails chooses the keyframe each thumbnail shows: the one
// nearest its time. A version that would need more than limit keyframes
// read shows the same keyframe on a few thumbnails in a row, as Jellyfin's
// thumbnails of a version with sparse keyframes do.
func planThumbnails(keyframes []time.Duration, duration, interval time.Duration, limit int) shots {
	count := thumbnailCount(duration, interval)
	step := max(1, (count+limit-1)/limit)
	var s shots
	for i := range count {
		k := nearest(keyframes, time.Duration(i/step*step)*interval)
		s.images = append(s.images, s.index(k))
	}
	return s
}

// planChapters chooses the keyframe each chapter's image shows: the one
// nearest its start.
func planChapters(keyframes []time.Duration, chapters []media.Chapter) shots {
	var s shots
	nearestOf := make([]int, len(chapters))
	for c, chapter := range chapters {
		nearestOf[c] = nearest(keyframes, chapter.Start)
	}
	s.keyframes = slices.Compact(slices.Sorted(slices.Values(nearestOf)))
	for _, k := range nearestOf {
		position, _ := slices.BinarySearch(s.keyframes, k)
		s.images = append(s.images, position)
	}
	return s
}

// index returns the position of keyframe k among those read, adding it
// when it follows them.
func (s *shots) index(k int) int {
	if n := len(s.keyframes); n > 0 && s.keyframes[n-1] == k {
		return n - 1
	}
	s.keyframes = append(s.keyframes, k)
	return len(s.keyframes) - 1
}

// union is the keyframes either reads, ascending.
func union(a, b []int) []int {
	all := append(slices.Clone(a), b...)
	slices.Sort(all)
	return slices.Compact(all)
}
