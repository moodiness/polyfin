package playback

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/subtitles"
)

func TestExtractedSubtitlesAccumulateAcrossRemuxes(t *testing.T) {
	s := func(n float64) time.Duration { return time.Duration(n * float64(time.Second)) }
	cue := func(start, end float64, lines ...string) subtitles.Cue {
		return subtitles.Cue{Start: s(start), End: s(end), Lines: lines}
	}
	x := newExtracted()
	// A remux resumed at 60 s, then one from the start that overlaps it.
	x.Cover(s(60), s(120))
	x.Add(3, cue(70, 72, "Later"))
	x.Add(3, cue(65, 66, "Sooner"))
	x.Cover(0, s(30))
	if x.Covers(0, s(120)) || !x.Covers(s(10), s(20)) || x.Covers(s(20), s(70)) {
		t.Fatalf("covered %v", x.covered)
	}
	x.Cover(s(30), s(90))
	x.Add(3, cue(65, 66, "Sooner"))
	x.Add(3, cue(5, 7, "First", "on two lines"))
	if !x.Covers(0, s(120)) || len(x.covered) != 1 {
		t.Fatalf("spans not merged: %v", x.covered)
	}
	texts := func(cues []subtitles.Cue) []string {
		var result []string
		for _, c := range cues {
			result = append(result, c.Lines[0])
		}
		return result
	}
	if got := texts(x.cues(3, 0, 0)); !slices.Equal(got, []string{"First", "Sooner", "Later"}) {
		t.Errorf("cues: %v", got)
	}
	// A segment gets the cues shown during it, including one that started
	// before.
	if got := texts(x.cues(3, s(6), s(66))); !slices.Equal(got, []string{"First", "Sooner"}) {
		t.Errorf("cues shown from 6 s to 66 s: %v", got)
	}
	// Saved, and read again by a later playback.
	data, changed := x.marshal()
	if !changed {
		t.Fatal("nothing to save")
	}
	if _, again := x.marshal(); again {
		t.Error("saved twice without a change")
	}
	read, err := unmarshalExtracted(data)
	if err != nil {
		t.Fatal(err)
	}
	if !read.Covers(0, s(120)) || !slices.EqualFunc(read.cues(3, 0, 0), x.cues(3, 0, 0), func(a, b subtitles.Cue) bool {
		return a.Start == b.Start && a.End == b.End && slices.Equal(a.Lines, b.Lines)
	}) {
		t.Errorf("read back: %v %+v", read.covered, read.cues(3, 0, 0))
	}
}

func TestStoredSubtitlesAreNotHiddenByARequestTheAppGaveUp(t *testing.T) {
	path, _ := fakeProbe(t, "", false)
	s := newService(t, &fakeSource{}, path, nil)
	version := accounts.ID{9}
	stored := newExtracted()
	stored.Cover(0, time.Hour)
	stored.Add(3, subtitles.Cue{Start: time.Minute, End: time.Minute + 2*time.Second, Lines: []string{"Hello"}})
	s.saveExtracted(t.Context(), version, stored)
	// After a restart, an app asks, then stops waiting; a later request
	// still finds the subtitles extracted before.
	restarted, err := New(s.db, &fakeSource{}, path, s.signer, s.sources, s.segments, nil, s.logger, s.settings)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	restarted.SubtitlesExtracted(ctx, version, time.Hour)
	if !restarted.SubtitlesExtracted(t.Context(), version, time.Hour) {
		t.Error("the stored subtitles were hidden")
	}
}
