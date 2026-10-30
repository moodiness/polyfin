package speech_test

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/speech"
	"github.com/moodiness/polyfin/internal/subtitles"
	"github.com/moodiness/polyfin/internal/testspeech"
)

const (
	length    = 100 * time.Minute
	cutAt     = 40 * time.Minute
	scene     = 90 * time.Second
	frameRate = 24000.0 / 1001
)

// whole is the reference of a version whose text track has lines at the
// times of spans.
func whole(spans []speech.Span, end time.Duration) speech.Reference {
	return speech.Reference{Speech: spans, Windows: []speech.Span{{Start: 0, End: end}}, Whole: true}
}

// An addon subtitle timed on another cut, with a 90-second scene more or
// less than the version at 40 minutes, lands within 0.3 seconds of the
// version's lines on both sides of the scene against a text track of the
// whole version; the lines of a scene the version lacks are left out.
func TestASubtitleOfAnotherCutIsSyncedPieceByPiece(t *testing.T) {
	t.Parallel()
	spoken := testspeech.Bursts(1, length)
	extra := testspeech.Bursts(3, scene)
	for _, tc := range []struct {
		name string
		// version is the time in the version of a line of the
		// subtitle's cut, false when the version lacks it; lines are the
		// spans of the version's own lines.
		version func(time.Duration) (time.Duration, bool)
		lines   []speech.Span
		// subtitle maps the subtitle's cut to the times of its file.
		subtitle func(time.Duration) time.Duration
	}{
		{
			name: "a scene removed",
			version: func(d time.Duration) (time.Duration, bool) {
				switch {
				case d < cutAt:
					return d, true
				case d < cutAt+scene:
					return 0, false
				}
				return d - scene, true
			},
			subtitle: func(d time.Duration) time.Duration { return d },
		},
		{
			name:     "a scene added",
			version:  func(d time.Duration) (time.Duration, bool) { return d + scene*time.Duration(btoi(d >= cutAt)), true },
			lines:    shifted(extra, cutAt),
			subtitle: func(d time.Duration) time.Duration { return d },
		},
		{
			name: "a scene removed, 3 seconds late and timed for 25 fps",
			version: func(d time.Duration) (time.Duration, bool) {
				switch {
				case d < cutAt:
					return d, true
				case d < cutAt+scene:
					return 0, false
				}
				return d - scene, true
			},
			subtitle: func(d time.Duration) time.Duration {
				return time.Duration(float64(d+3*time.Second) * frameRate / 25)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			lines := slices.Clone(tc.lines)
			// want are the version's times of the subtitle's lines, by
			// text; lacking those of a scene the version lacks, which are
			// left out; and those across the cut are not looked at.
			want, lacking := map[string]time.Duration{}, map[string]bool{}
			for i, span := range spoken {
				at, ok := tc.version(span.Start)
				end, endOK := tc.version(span.End)
				switch {
				case !ok && !endOK:
					lacking[line(i)] = true
				case ok && endOK && end-at == span.End-span.Start:
					lines = append(lines, speech.Span{Start: at, End: end})
					want[line(i)] = at
				}
			}
			slices.SortFunc(lines, func(a, b speech.Span) int { return int(a.Start - b.Start) })
			cues := testspeech.Cues(spoken, tc.subtitle)
			timing := speech.Align(cues, whole(lines, length+scene), frameRate)
			if len(timing.Cuts) != 1 {
				t.Errorf("timed %+v, want one cut", timing)
			}
			// in and off count the lines within 0.3 seconds of the version's
			// and those further, before the scene and after it.
			var in, off [2]int
			for _, cue := range timing.Apply(cues) {
				at, ok := want[cue.Lines[0]]
				if lacking[cue.Lines[0]] {
					t.Errorf("%s, of a scene the version lacks, is kept at %s", cue.Lines[0], cue.Start)
				}
				if !ok {
					continue
				}
				side := btoi(at >= cutAt)
				if gap := cue.Start - at; gap.Abs() > 300*time.Millisecond {
					off[side]++
					continue
				}
				in[side]++
			}
			if in[0]+in[1] != len(want) || in[0] == 0 || in[1] == 0 {
				t.Errorf("before the scene, %d lines in time and %d off; after it, %d in time and %d off; want %d in time",
					in[0], off[0], in[1], off[1], len(want))
			}
		})
	}
}

// Against the stretches of a version's audio, where a scene cannot be
// placed, a subtitle of another cut keeps one shift; and a subtitle of the
// same cut gets the timing it gets without the whole reference: one shift
// and one speed, or none.
func TestOnlyACutAgainstTheWholeVersionIsFittedInPieces(t *testing.T) {
	t.Parallel()
	spoken := testspeech.Bursts(1, length)
	var removed []speech.Span
	for _, span := range spoken {
		switch {
		case span.End <= cutAt:
			removed = append(removed, span)
		case span.Start >= cutAt+scene:
			removed = append(removed, speech.Span{Start: span.Start - scene, End: span.End - scene})
		}
	}
	audio := whole(removed, length)
	audio.Whole = false
	if timing := speech.Align(testspeech.Cues(spoken, func(d time.Duration) time.Duration { return d }), audio, frameRate); len(timing.Cuts) != 0 {
		t.Errorf("against the audio, timed %+v", timing)
	}
	for _, tc := range []struct {
		name   string
		cues   []subtitles.Cue
		synced bool
	}{
		{"shifted by 4 seconds", testspeech.Cues(spoken, func(d time.Duration) time.Duration { return d + 4*time.Second }), true},
		{"timed for 25 fps", testspeech.Cues(spoken, func(d time.Duration) time.Duration { return time.Duration(float64(d) * frameRate / 25) }), true},
		{"in time", testspeech.Cues(spoken, func(d time.Duration) time.Duration { return d }), false},
		{"of another title", testspeech.Cues(testspeech.Bursts(2, length), func(d time.Duration) time.Duration { return d }), false},
	} {
		reference := whole(spoken, length)
		timing := speech.Align(tc.cues, reference, frameRate)
		reference.Whole = false
		if global := speech.Align(tc.cues, reference, frameRate); !reflect.DeepEqual(timing, global) || timing.Changes() != tc.synced {
			t.Errorf("%s: timed %+v against the whole version, %+v without", tc.name, timing, global)
		}
	}
}

func line(i int) string {
	return fmt.Sprintf("Line %d", i+1)
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func shifted(spans []speech.Span, by time.Duration) []speech.Span {
	result := make([]speech.Span, len(spans))
	for i, span := range spans {
		result[i] = speech.Span{Start: span.Start + by, End: span.End + by}
	}
	return result
}
