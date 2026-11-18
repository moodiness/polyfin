package thumbnails

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

func secondsOf(values ...float64) []time.Duration {
	durations := make([]time.Duration, len(values))
	for i, v := range values {
		durations[i] = time.Duration(v * float64(time.Second))
	}
	return durations
}

// The keyframes read are those nearest the thumbnails' times, each once,
// coarse to fine: however many the budget lets through, they cover the
// whole runtime.
func TestChooseKeyframes(t *testing.T) {
	t.Parallel()
	keyframes := make([]time.Duration, 3600)
	for i := range keyframes {
		keyframes[i] = time.Duration(i) * time.Second
	}
	order := chooseKeyframes(keyframes, thumbnailTimes(time.Hour, stepFor(time.Hour, 10*time.Second, 57)))
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
	if got := chooseKeyframes(secondsOf(0, 100), thumbnailTimes(time.Minute, 3*time.Second)); !slices.Equal(got, []int{0, 1}) {
		t.Errorf("sparse: %v", got)
	}
	if got := coarseToFine(5); !slices.Equal(got, []int{0, 4, 2, 1, 3}) {
		t.Errorf("coarse to fine: %v", got)
	}
}

// Every thumbnail shows the keyframe read nearest its time, the earlier
// of two as near, whatever the order they were read in.
func TestShownCoversEveryThumbnail(t *testing.T) {
	t.Parallel()
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

// Thumbnails come every interval asked while the reads give each its own
// keyframe; a longer version spreads them over its runtime, one per read,
// in whole seconds, rather than repeating images.
func TestTheStepGrowsWithTheRuntime(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		duration time.Duration
		reads    int
		want     time.Duration
	}{
		{10 * time.Minute, 60, 10 * time.Second},
		{9 * time.Minute, 54, 10 * time.Second},
		{45 * time.Minute, 58, 47 * time.Second},
		{2 * time.Hour, 58, 125 * time.Second},
		{25 * time.Second, 2, 13 * time.Second},
		// No read left keeps the interval asked.
		{2 * time.Hour, 0, 10 * time.Second},
	} {
		step := stepFor(tc.duration, 10*time.Second, tc.reads)
		if step != tc.want {
			t.Errorf("%v with %d reads: a thumbnail every %v, want %v", tc.duration, tc.reads, step, tc.want)
		}
		if count := len(thumbnailTimes(tc.duration, step)); tc.reads > 0 && count > tc.reads {
			t.Errorf("%v with %d reads: %d thumbnails", tc.duration, tc.reads, count)
		}
	}
	if got := thumbnailTimes(25*time.Second, 10*time.Second); !slices.Equal(got, secondsOf(0, 10, 20)) {
		t.Errorf("times %v", got)
	}
}

// A version has 172,800 thumbnails at most, those of ten days at the
// shortest interval of the settings: one of ten days has all of them at
// every interval, and one whose duration gives more has none, whatever
// the duration, and nothing is sized by it.
func TestThumbnailsAreCountedWithinABound(t *testing.T) {
	t.Parallel()
	const shortest, longest = accounts.MinTrickplayInterval * time.Second, accounts.MaxTrickplayInterval * time.Second
	if longestVersion != 240*time.Hour || maxThumbnails != 172_800 || time.Duration(maxThumbnails)*shortest != longestVersion {
		t.Fatalf("%d thumbnails at most, for %v at one every %v", maxThumbnails, longestVersion, shortest)
	}
	// Ten days, and what is shorter, at every interval of the settings.
	for seconds := accounts.MinTrickplayInterval; seconds <= accounts.MaxTrickplayInterval; seconds++ {
		interval := time.Duration(seconds) * time.Second
		for _, duration := range []time.Duration{time.Nanosecond, 2 * time.Hour, longestVersion - time.Nanosecond, longestVersion} {
			if count := thumbnailCount(duration, interval); !spreads(duration, interval) || count < 1 || count > maxThumbnails {
				t.Fatalf("%v at one every %v: %d thumbnails, none made", duration, interval, count)
			}
		}
	}
	// At the bound, every thumbnail: ten days at both ends of the
	// settings, and the longest version the longest interval allows.
	for _, tc := range []struct {
		duration, interval time.Duration
		want               int
	}{
		{longestVersion, shortest, maxThumbnails},
		{longestVersion, longest, 14_400},
		{time.Duration(maxThumbnails) * longest, longest, maxThumbnails},
	} {
		times := thumbnailTimes(tc.duration, tc.interval)
		if len(times) != tc.want || times[len(times)-1] != tc.duration-tc.interval {
			t.Errorf("%v at one every %v: %d thumbnail times, want %d up to %v", tc.duration, tc.interval, len(times), tc.want, tc.duration-tc.interval)
		}
	}
	// Past it, none: by one thumbnail, by 104 days, by 285 years and by
	// the longest time there is, which is counted without wrapping.
	const declared = 9_000_000_000 * time.Second
	for _, tc := range []struct{ duration, interval time.Duration }{
		{longestVersion + time.Nanosecond, shortest},
		{time.Duration(maxThumbnails)*longest + time.Nanosecond, longest},
		{declared / 1000, 10 * time.Second},
		{declared, shortest},
		{declared, 10 * time.Second},
		{declared, longest},
		{math.MaxInt64, shortest},
		{math.MaxInt64, longest},
	} {
		if count := thumbnailCount(tc.duration, tc.interval); count <= maxThumbnails {
			t.Errorf("%v at one every %v counts %d thumbnails", tc.duration, tc.interval, count)
		}
		// Checked first: the times are asked for only once known to be
		// none.
		if spreads(tc.duration, tc.interval) {
			t.Fatalf("%v at one every %v has thumbnails", tc.duration, tc.interval)
		}
		if times := thumbnailTimes(tc.duration, tc.interval); times != nil {
			t.Errorf("%v at one every %v: %d thumbnail times", tc.duration, tc.interval, len(times))
		}
	}
	if thumbnailCount(0, shortest) != 0 || thumbnailCount(-time.Hour, shortest) != 0 || spreads(0, shortest) || spreads(time.Hour, 0) {
		t.Error("a version without a duration, or thumbnails without an interval, are counted")
	}
}
