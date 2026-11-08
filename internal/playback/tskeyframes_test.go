package playback

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// A local MPEG-TS file is cut on the keyframes read from the whole file at
// its first plan, searched as FFmpeg seeks in it, and they are kept: the
// version names know them, after a restart too, and the file is not read
// again. One whose keyframes cannot be read is cut on a grid, its video
// converted, as before, and is not read again either; nor is an addon's
// MPEG-TS file read whole.
func TestLocalMPEGTSFilesAreCutOnTheirKeyframes(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	dir := t.TempDir()
	if out, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=128x72:rate=24", "-f", "lavfi", "-i", "sine=sample_rate=48000",
		"-t", "30", "-c:v", "libx264", "-preset", "veryfast", "-bf", "3", "-g", "1000", "-sc_threshold", "0",
		"-force_key_frames", "2.5,7,9,13,17.5,22,26", "-c:a", "aac", "-f", "mpegts", filepath.Join(dir, "movie.ts")).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.ts"), []byte(strings.Repeat("\x47not a transport stream", 2000)), 0o644); err != nil {
		t.Fatal(err)
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	open := func(_ context.Context, key string) (io.ReadSeekCloser, time.Time, error) {
		file, err := os.Open(filepath.Join(dir, key))
		return file, time.Time{}, err
	}
	s := newService(t, &fakeSource{}, ffprobe, nil)
	base := s.LocalFiles(open)
	analysis := media.Analysis{Format: "mpegts", Duration: 30 * time.Second,
		Streams: []media.Stream{{Index: 0, Type: "video", Codec: "h264"}, {Index: 1, Type: "audio", Codec: "aac"}}}
	movie := library.Version{ID: accounts.ID{1}, URL: base + "movie.ts", Filename: "movie.ts"}
	broken := library.Version{ID: accounts.ID{2}, URL: base + "broken.ts", Filename: "broken.ts"}
	addon := library.Version{ID: accounts.ID{3}, URL: "http://addon.example/movie.ts", Filename: "movie.ts"}
	for _, v := range []library.Version{movie, broken, addon} {
		s.analyses.Put(v.ID, analysis)
	}
	if !s.GridPlanned(movie, analysis) {
		t.Error("a local MPEG-TS file is named as copied before its keyframes are read")
	}

	plan, err := s.Plan(t.Context(), movie)
	if err != nil {
		t.Fatal(err)
	}
	// The keyframes, from the file's start, its audio's, 21 ms before its
	// video's.
	var keyframes []time.Duration
	for _, at := range []time.Duration{0, 2500, 7000, 9000, 13000, 17500, 22000, 26000} {
		keyframes = append(keyframes, at*time.Millisecond+21333*time.Microsecond)
	}
	if want := hls.NewPlan(keyframes, 30*time.Second); plan.Grid() || !plan.IsSearched() || !sameStarts(plan, want) {
		t.Errorf("the plan: grid %v, searched %v, %d segments", plan.Grid(), plan.IsSearched(), plan.Len())
	}
	if s.GridPlanned(movie, analysis) {
		t.Error("a local MPEG-TS file whose keyframes were read is named as converted")
	}
	again := newServiceOn(t, s.db, &fakeSource{}, "ffprobe-not-installed", nil)
	againMovie := movie
	againMovie.URL = again.LocalFiles(open) + "movie.ts"
	again.analyses.Put(movie.ID, analysis)
	if again.GridPlanned(againMovie, analysis) {
		t.Error("after a restart, a local MPEG-TS file whose keyframes were read is named as converted")
	}
	if plan, err := again.Plan(t.Context(), againMovie); err != nil || plan.Grid() || !sameStarts(plan, hls.NewPlan(keyframes, 30*time.Second)) {
		t.Errorf("after a restart, without ffprobe: grid %v, %v", plan.Grid(), err)
	}

	plan, err = s.Plan(t.Context(), broken)
	if err != nil || !plan.Grid() || !plan.IsSearched() || !s.GridPlanned(broken, analysis) {
		t.Errorf("a file of no media: grid %v, searched %v: %v", plan.Grid(), plan.IsSearched(), err)
	}
	if _, failed := s.unscanned.Get(broken.ID); !failed {
		t.Error("a file whose keyframes cannot be read would be read again")
	}
	if s.scansKeyframes(addon, analysis) || !s.GridPlanned(addon, analysis) {
		t.Error("an addon's MPEG-TS file would be read whole")
	}
}

// sameStarts reports whether two plans start their segments at the same
// times.
func sameStarts(a, b hls.Plan) bool {
	if a.Len() != b.Len() {
		return false
	}
	for n := range a.Len() {
		if a.Start(n) != b.Start(n) {
			return false
		}
	}
	return true
}
