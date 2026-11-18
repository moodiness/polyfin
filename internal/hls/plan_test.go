package hls

import (
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/media"
)

// A version lasting media.MaxDuration, the longest an analysis tells, is
// cut whole: on the grid, on its keyframes, and as a track.
func TestTheLongestVersionIsCutWhole(t *testing.T) {
	t.Parallel()
	grid := NewGridPlan(media.MaxDuration)
	last := grid.Len() - 1
	if want := 2 + int((media.MaxDuration-firstDuration-shortestTail)/targetDuration); grid.Len() != want {
		t.Errorf("a grid of %d segments, want %d", grid.Len(), want)
	}
	if grid.Start(last) != media.MaxDuration-2*time.Second || grid.End(last) != media.MaxDuration {
		t.Errorf("the grid's last segment goes from %v to %v", grid.Start(last), grid.End(last))
	}
	late := media.MaxDuration - 10*time.Second
	cut := NewPlan([]time.Duration{0, late}, media.MaxDuration)
	if !slices.Equal(cut.starts, []time.Duration{0, late}) || cut.End(1) != media.MaxDuration {
		t.Errorf("cut at %v, up to %v", cut.starts, cut.End(1))
	}
	track := AudioPlan(media.MaxDuration)
	if want := int(media.MaxDuration / AudioSegment); track.Len() != want || track.End(want-1) != media.MaxDuration {
		t.Errorf("a track of %d segments, up to %v, want %d", track.Len(), track.End(track.Len()-1), want)
	}
}

// A plan cuts media.MaxDuration of a version said to last longer, as a
// disc's tables or an addon may say: it holds the segments of the longest
// version, whatever the length, and so do the plans made from it.
func TestAPlanCutsNoMoreThanTheLongestVersion(t *testing.T) {
	t.Parallel()
	most := int(media.MaxDuration / fastLength)
	grid, track := NewGridPlan(media.MaxDuration), AudioPlan(media.MaxDuration)
	// The shorter length comes first: a plan that followed it stops the
	// test before one is cut from 285 years.
	for _, duration := range []time.Duration{media.MaxDuration + time.Hour, 9_000_000_000 * time.Second} {
		if got := NewGridPlan(duration); !slices.Equal(got.starts, grid.starts) || got.End(got.Len()-1) != media.MaxDuration || !got.Grid() {
			t.Fatalf("%v: a grid of %d segments, up to %v", duration, got.Len(), got.End(got.Len()-1))
		}
		if got := AudioPlan(duration); !slices.Equal(got.starts, track.starts) || got.End(got.Len()-1) != media.MaxDuration {
			t.Fatalf("%v: a track of %d segments, up to %v", duration, got.Len(), got.End(got.Len()-1))
		}
		// A keyframe past the longest version starts no segment, and the
		// last segment ends with it.
		cut := NewPlan([]time.Duration{0, time.Minute, media.MaxDuration + time.Minute}, duration)
		if !slices.Equal(cut.starts, []time.Duration{0, time.Minute}) || cut.End(1) != media.MaxDuration {
			t.Fatalf("%v: cut at %v, up to %v", duration, cut.starts, cut.End(1))
		}
		// Its last segment, cut into fast ones, gives no more of them than
		// the longest version holds.
		fast, ok := cut.Searched().FastFrom(time.Hour)
		if !ok || fast.Len() > most || fast.End(fast.Len()-1) != media.MaxDuration {
			t.Fatalf("%v: %d segments with the fast ones, %d at most, up to %v", duration, fast.Len(), most, fast.End(fast.Len()-1))
		}
		if whole, ok := NewPlan(nil, duration).FastFrom(0); !ok || whole.Len() != most {
			t.Fatalf("%v: one segment cut into %d fast ones, want %d", duration, whole.Len(), most)
		}
	}
}
