package thumbnails

import (
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/media"
)

func secondsOf(values ...float64) []time.Duration {
	durations := make([]time.Duration, len(values))
	for i, v := range values {
		durations[i] = time.Duration(v * float64(time.Second))
	}
	return durations
}

// Each thumbnail, one every interval from the start, shows the keyframe
// nearest its time, the earlier of two as near; a keyframe several show
// is read once.
func TestPlanThumbnails(t *testing.T) {
	keyframes := secondsOf(0, 1.3, 4, 6, 14, 30)
	s := planThumbnails(keyframes, 31*time.Second, 5*time.Second, 1000)
	// 0, 5, 10, 15, 20, 25, 30 s: 0, 4 (as near as 6), 6, 14, 14, 30, 30.
	if want := []int{0, 2, 3, 4, 5}; !slices.Equal(s.keyframes, want) {
		t.Errorf("keyframes read %v, want %v", s.keyframes, want)
	}
	if want := []int{0, 1, 2, 3, 3, 4, 4}; !slices.Equal(s.images, want) {
		t.Errorf("thumbnails show %v, want %v", s.images, want)
	}
	// A version needing more keyframes than allowed shows each on a few
	// thumbnails in a row.
	dense := make([]time.Duration, 3600)
	for i := range dense {
		dense[i] = time.Duration(i) * time.Second
	}
	s = planThumbnails(dense, time.Hour, 5*time.Second, 100)
	if len(s.images) != 720 || len(s.keyframes) > 100 {
		t.Errorf("%d thumbnails from %d keyframes", len(s.images), len(s.keyframes))
	}
	if s.images[0] != 0 || s.images[7] != 0 || s.images[8] != 1 {
		t.Errorf("thumbnails show %v", s.images[:10])
	}
	if got := thumbnailCount(10*time.Second, 10*time.Second); got != 1 {
		t.Errorf("%d thumbnails for 10 s", got)
	}
}

// Each chapter's image shows the keyframe nearest its start.
func TestPlanChapters(t *testing.T) {
	keyframes := secondsOf(0, 2, 9, 20)
	s := planChapters(keyframes, []media.Chapter{{Start: 0}, {Start: 8 * time.Second}, {Start: 10 * time.Second}, {Start: 30 * time.Second}})
	if !slices.Equal(s.keyframes, []int{0, 2, 3}) || !slices.Equal(s.images, []int{0, 1, 1, 2}) {
		t.Errorf("keyframes %v, images %v", s.keyframes, s.images)
	}
	if got := union([]int{0, 3, 5}, []int{1, 3}); !slices.Equal(got, []int{0, 1, 3, 5}) {
		t.Errorf("union %v", got)
	}
}
