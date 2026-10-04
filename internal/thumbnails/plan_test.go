package thumbnails

import (
	"slices"
	"testing"
	"time"
)

func secondsOf(values ...float64) []time.Duration {
	durations := make([]time.Duration, len(values))
	for i, v := range values {
		durations[i] = time.Duration(v * float64(time.Second))
	}
	return durations
}

// The keyframes read are those nearest times spread over the runtime,
// each once, coarse to fine: however many the budget lets through, they
// cover the whole runtime.
func TestChooseKeyframes(t *testing.T) {
	keyframes := make([]time.Duration, 3600)
	for i := range keyframes {
		keyframes[i] = time.Duration(i) * time.Second
	}
	order := chooseKeyframes(keyframes, spread(time.Hour, 57))
	if len(order) != 57 {
		t.Fatalf("%d keyframes chosen", len(order))
	}
	sorted := slices.Sorted(slices.Values(order))
	if sorted[0] > 40 || sorted[56] < 3560 || len(slices.Compact(slices.Clone(sorted))) != 57 {
		t.Errorf("chosen %v", sorted)
	}
	// Any first few read span the hour: the first 8 leave no gap above a
	// quarter of it.
	first := slices.Sorted(slices.Values(order[:8]))
	if first[0] > 600 || first[7] < 3000 {
		t.Errorf("the first keyframes read %v", first)
	}
	for i := 1; i < len(first); i++ {
		if first[i]-first[i-1] > 900 {
			t.Errorf("the first keyframes read %v leave a gap", first)
		}
	}
	// Sparse keyframes are read once each.
	if got := chooseKeyframes(secondsOf(0, 100), spread(time.Minute, 20)); !slices.Equal(got, []int{0, 1}) {
		t.Errorf("sparse: %v", got)
	}
	if got := coarseToFine(5); !slices.Equal(got, []int{0, 4, 2, 1, 3}) {
		t.Errorf("coarse to fine: %v", got)
	}
}

// Every thumbnail shows the keyframe read nearest its time, the earlier
// of two as near, whatever the order they were read in.
func TestShownCoversEveryThumbnail(t *testing.T) {
	read := secondsOf(30, 0, 14, 6)
	asked := make([]time.Duration, 8)
	for i := range asked {
		asked[i] = time.Duration(i) * 5 * time.Second
	}
	// 0, 5, 10, 15, 20, 25, 30, 35 s: 0, 6, 6 (as near as 14), 14, 14,
	// 30, 30, 30 s, at their place in read.
	if got, want := shown(read, asked), []int{1, 3, 3, 2, 2, 0, 0, 0}; !slices.Equal(got, want) {
		t.Errorf("shown %v, want %v", got, want)
	}
	if got := thumbnailCount(10*time.Second, 10*time.Second); got != 1 {
		t.Errorf("%d thumbnails for 10 s", got)
	}
	if got := thumbnailCount(24*time.Minute, 10*time.Second); got != 144 {
		t.Errorf("%d thumbnails for 24 minutes", got)
	}
}
