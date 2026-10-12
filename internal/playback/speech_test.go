package playback

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/subtitles"
	"github.com/moodiness/polyfin/internal/testspeech"
)

// An addon's subtitle shifted by 4 seconds, and one timed for 25 fps on a
// 23.976 fps version, come back in time, against a text track inside the
// file and against the audio; one already in time and one of another
// title are left as they are.
func TestAddonSubtitlesAreSyncedToTheVersionPlayed(t *testing.T) {
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	t.Parallel()
	const length = 12 * time.Minute
	spoken := testspeech.Bursts(1, length)
	same := func(d time.Duration) time.Duration { return d }
	for _, reference := range []string{speechTrack, speechAudio} {
		t.Run(reference, func(t *testing.T) {
			t.Parallel()
			origin := &fileSource{data: testspeech.Version(t, ffmpeg, spoken, length, reference == speechTrack)}
			s := newService(t, origin, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), nil)
			segments, err := hls.NewManager(ffmpeg, t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(segments.Close)
			s.segments = segments
			version := library.Version{ID: accounts.ID{1}, URL: "http://files.example/version.mkv", Addon: "Files"}
			s.findSpeech(Background(t.Context()), version)
			if stored, ok := s.storedSpeech(t.Context(), version.ID); !ok || stored.kind != reference {
				t.Fatalf("the reference is %q (%v), want %q", stored.kind, ok, reference)
			}
			for i, tc := range []struct {
				name   string
				cues   []subtitles.Cue
				synced bool
			}{
				{"shifted by 4 seconds", testspeech.Cues(spoken, func(d time.Duration) time.Duration { return d + 4*time.Second }), true},
				{"timed for 25 fps", testspeech.Cues(spoken, func(d time.Duration) time.Duration { return time.Duration(float64(d) * 24000 / 1001 / 25) }), true},
				{"in time", testspeech.Cues(spoken, same), false},
				{"of another title", testspeech.Cues(testspeech.Bursts(2, length), same), false},
			} {
				subtitle := accounts.ID{byte(10 + i)}
				timing, ok := s.SyncSubtitle(t.Context(), version.ID, subtitle, tc.cues)
				if !ok || timing.Changes() != tc.synced {
					t.Errorf("%s: timed %+v (%v)", tc.name, timing, ok)
					continue
				}
				for j, cue := range timing.Apply(tc.cues) {
					if gap := cue.Start - spoken[j].Start; tc.synced && gap.Abs() > 200*time.Millisecond {
						t.Errorf("%s: cue %d starts at %s, the speech at %s", tc.name, j, cue.Start, spoken[j].Start)
						break
					}
				}
				// The timing is kept: the next request gets the same.
				if again, _ := s.SyncSubtitle(t.Context(), version.ID, subtitle, nil); again != timing {
					t.Errorf("%s: timed %+v, then %+v", tc.name, timing, again)
				}
			}
		})
	}
}

// Within the read budget, an addon's file is looked at in stretches as
// long as the budget reads at its bitrate: shorter than 3 minutes, its
// late subtitle is still synced; under 30 seconds each, the version has
// no speech reference, and its subtitles are served as they are.
func TestStretchesShortenToTheReadBudget(t *testing.T) {
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	t.Parallel()
	const length = 12 * time.Minute
	spoken := testspeech.Bursts(1, length)
	data := testspeech.Version(t, ffmpeg, spoken, length, false)
	late := testspeech.Cues(spoken, func(d time.Duration) time.Duration { return d + 4*time.Second })
	for _, tc := range []struct {
		name    string
		stretch time.Duration
	}{{"shorter stretches", time.Minute}, {"under the floor", 25 * time.Second}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newService(t, &fileSource{data: data}, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), nil)
			segments, err := hls.NewManager(ffmpeg, t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(segments.Close)
			s.segments = segments
			// The budget reads three stretches of tc.stretch, and half a
			// second more, at the file's bitrate (see stretchWithin).
			perSecond := float64(len(data)) / length.Seconds()
			s.speech.budget = int64(3 * readOverhead * (readAhead + (tc.stretch+keyframeLead+time.Second/2).Seconds()*perSecond))
			version := library.Version{ID: accounts.ID{1}, URL: "http://files.example/version.mkv", Addon: "Files"}
			s.findSpeech(Background(t.Context()), version)
			stored, ok := s.storedSpeech(t.Context(), version.ID)
			timing, synced := s.SyncSubtitle(t.Context(), version.ID, accounts.ID{2}, late)
			if tc.stretch < minStretch {
				if !ok || stored.kind != speechNone || synced || timing.Changes() {
					t.Errorf("the reference is %q (%v), the late subtitle timed %+v (%v)", stored.kind, ok, timing, synced)
				}
				return
			}
			if !ok || stored.kind != speechAudio || len(stored.reference.Windows) != 3 {
				t.Fatalf("the reference is %q (%v), in %d windows", stored.kind, ok, len(stored.reference.Windows))
			}
			for _, window := range stored.reference.Windows {
				if got := window.End - window.Start; got != tc.stretch {
					t.Errorf("a window lasts %s, want %s", got, tc.stretch)
				}
			}
			for j, cue := range timing.Apply(late) {
				if gap := cue.Start - spoken[j].Start; !synced || gap.Abs() > 200*time.Millisecond {
					t.Fatalf("cue %d starts at %s, the speech at %s (timed %+v, %v)", j, cue.Start, spoken[j].Start, timing, synced)
				}
			}
		})
	}
}
