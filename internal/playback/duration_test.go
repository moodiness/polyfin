package playback

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// A version lasting media.MaxDuration is cut whole. One whose analysis
// tells more, as an analysis kept by an earlier release may, is not cut,
// as one without a duration is not: no plan is made from its length.
func TestAVersionPastTheLongestDurationIsNotCut(t *testing.T) {
	t.Parallel()
	s := newService(t, &fileSource{}, "ffprobe", nil)
	analyzed := func(n byte, duration time.Duration) library.Version {
		version := library.Version{ID: accounts.ID{0xD1, n}, URL: "https://example.invalid/long.m3u8"}
		s.analyses.Put(version.ID, media.Analysis{Format: "hls", Duration: duration,
			Streams: []media.Stream{{Index: 0, Type: "video", Codec: "h264", Width: 160, Height: 90}}})
		return version
	}
	plan, err := s.plan(t.Context(), analyzed(0, media.MaxDuration))
	if err != nil {
		t.Fatal(err)
	}
	// A segment of 2 seconds, then one every 4 seconds, the last one 2
	// seconds long.
	if want := 2 + int((media.MaxDuration-3*time.Second)/(4*time.Second)); plan.Len() != want || !plan.Grid() ||
		plan.End(plan.Len()-1) != media.MaxDuration {
		t.Errorf("the longest version: %d segments, up to %v, want %d", plan.Len(), plan.End(plan.Len()-1), want)
	}
	_, none := s.plan(t.Context(), analyzed(1, 0))
	if none == nil {
		t.Fatal("a version without a duration is cut")
	}
	// The shorter length comes first: a plan made from it stops the test
	// before one is made from 285 years.
	for n, duration := range []time.Duration{media.MaxDuration + time.Second, 2500 * time.Hour, 9_000_000_000 * time.Second} {
		plan, err := s.plan(t.Context(), analyzed(byte(n+2), duration))
		if !errors.Is(err, none) || plan.Len() != 0 {
			t.Fatalf("%v: a plan of %d segments, %v", duration, plan.Len(), err)
		}
	}
}

// An analysis kept by an earlier release may tell more than
// media.MaxDuration. Its version is then not told as cut into segments,
// and no plan is made for it, as for one kept without a duration; one
// kept with a movie's length is.
func TestAKeptAnalysisPastTheLongestDurationIsNotSegmented(t *testing.T) {
	t.Parallel()
	s := newService(t, &fileSource{}, "ffprobe", nil)
	kept := func(n byte, duration time.Duration) library.Version {
		version := library.Version{ID: accounts.ID{0xD2, n}, URL: "https://example.invalid/kept.m3u8"}
		data, err := json.Marshal(media.Analysis{Format: "hls", Duration: duration,
			Streams: []media.Stream{{Index: 0, Type: "video", Codec: "h264", Width: 160, Height: 90}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", version.ID, data); err != nil {
			t.Fatal(err)
		}
		return version
	}
	movie := kept(0, 2*time.Hour)
	if plan, err := s.plan(t.Context(), movie); err != nil || plan.Len() == 0 || !s.Segmented(t.Context(), movie) {
		t.Fatalf("a movie: a plan of %d segments, %v", plan.Len(), err)
	}
	without := kept(1, 0)
	_, none := s.plan(t.Context(), without)
	if none == nil || s.Segmented(t.Context(), without) {
		t.Fatal("a version kept without a duration is cut")
	}
	// The shorter length comes first: a plan made from it stops the test
	// before one is made from 285 years.
	for n, duration := range []time.Duration{media.MaxDuration + time.Second, 9_000_000_000 * time.Second} {
		version := kept(byte(n+2), duration)
		if s.Segmented(t.Context(), version) {
			t.Fatalf("%v: told as cut into segments", duration)
		}
		if plan, err := s.plan(t.Context(), version); !errors.Is(err, none) || plan.Len() != 0 {
			t.Fatalf("%v: a plan of %d segments, %v", duration, plan.Len(), err)
		}
	}
}
