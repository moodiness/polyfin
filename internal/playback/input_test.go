package playback

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// fakeInputs finds the inputs of discs by key (see Inputs): "dvd" a DVD's
// main title, any other key a Blu-ray disc's playlist, whose keyframes
// are those of keyframes, or none that can be read. reads, when set,
// counts the reads of keyframes.
type fakeInputs struct {
	keyframes map[string][]time.Duration
	reads     *atomic.Int32
}

func (f fakeInputs) Input(_ context.Context, key string) (media.Input, error) {
	if key == "dvd" {
		return media.Input{Format: "dvdvideo", Options: []string{"-title", "2"}, Path: "/discs/Movie (2008)/VIDEO_TS"}, nil
	}
	return media.Input{Options: []string{"-playlist", key}, Path: "bluray:/discs/Movie (2008)"}, nil
}

func (f fakeInputs) Keyframes(_ context.Context, key string) ([]time.Duration, error) {
	if f.reads != nil {
		f.reads.Add(1)
	}
	if times, ok := f.keyframes[key]; ok {
		return times, nil
	}
	return nil, errors.New("no keyframes")
}

// A Blu-ray disc's title is cut on the keyframes the disc lists, read once
// and saved as a file's index is, and its video copied when it is not
// converted. A disc whose keyframes cannot be read, as a DVD's, is cut
// every few seconds, its video only converted.
func TestDiscsAreCutOnTheirKeyframes(t *testing.T) {
	s := newService(t, &fakeSource{}, "ffprobe-not-installed", nil)
	keyframes := make([]time.Duration, 190)
	for i := range keyframes {
		keyframes[i] = time.Duration(i) * time.Second
	}
	var reads atomic.Int32
	base := s.LocalInputs(fakeInputs{keyframes: map[string][]time.Duration{"0": keyframes}, reads: &reads})
	analysis := media.Analysis{Format: "mpegts", Duration: 190 * time.Second,
		Streams: []media.Stream{{Index: 0, Type: "video", Codec: "h264"}, {Index: 1, Type: "audio", Codec: "ac3"}}}
	bluray := library.Version{ID: accounts.ID{41}, URL: base + "0"}
	s.analyses.Put(bluray.ID, analysis)
	want := hls.NewPlan(keyframes, 190*time.Second)
	for range 2 {
		plan, err := s.Plan(t.Context(), bluray)
		if err != nil || plan.Grid() || plan.Len() != want.Len() || plan.Start(3) != want.Start(3) || plan.End(plan.Len()-1) != 190*time.Second {
			t.Fatalf("the title's plan: %d segments, %v", plan.Len(), err)
		}
	}
	var saved int
	if err := s.db.QueryRow(t.Context(), "SELECT count(*) FROM media_keyframes WHERE version_id = $1", bluray.ID).Scan(&saved); err != nil || saved != 1 || reads.Load() != 1 {
		t.Errorf("the keyframes were read %d times, saved %d: %v", reads.Load(), saved, err)
	}
	r, release, err := s.remuxOpener(Remux{Session: "session", Version: bluray, Audio: 1, Format: hls.FMP4})(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	release()
	if r.Encode != nil || r.Input != "bluray:/discs/Movie (2008)" || !slices.Equal(r.InputOptions, []string{"-playlist", "0", "-start_at_zero"}) || r.Plan.Grid() {
		t.Errorf("the title's remux: %+v", r)
	}

	for key, format := range map[string]string{"1": "mpegts", "dvd": "dvdvideo"} {
		version := library.Version{ID: accounts.ID{42, key[0]}, URL: base + key}
		other := analysis
		other.Format = format
		s.analyses.Put(version.ID, other)
		if plan, err := s.Plan(t.Context(), version); err != nil || !plan.Grid() {
			t.Errorf("%s: a grid %v, %v", key, plan.Grid(), err)
		}
		if _, _, err := s.remuxOpener(Remux{Session: "session", Version: version, Audio: 1, Format: hls.FMP4})(t.Context()); !errors.Is(err, ErrNotRemuxable) {
			t.Errorf("%s: copied: %v", key, err)
		}
	}
}

// FFmpeg's bluray protocol reads a Blu-ray image, and no DVD image: a scan
// tells a Blu-ray image whose UDF file system it does not read so (see
// localfiles.Service.UseProbe).
func TestProbesInput(t *testing.T) {
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	if output, err := exec.Command(ffmpeg, "-hide_banner", "-protocols").Output(); err != nil || !slices.Contains(strings.Fields(string(output)), "bluray") {
		t.Skip("the FFmpeg of POLYFIN_TEST_FFMPEG has no bluray protocol")
	}
	s := newService(t, &fakeSource{}, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), nil)
	dir := t.TempDir()
	for _, name := range []string{"bluray", "dvd"} {
		archive, err := os.Open(filepath.Join("..", "localfiles", "testdata", name+".iso.gz"))
		if err != nil {
			t.Fatal(err)
		}
		unzipped, err := gzip.NewReader(archive)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(unzipped)
		archive.Close()
		if err != nil {
			t.Fatal(err)
		}
		image := filepath.Join(dir, name+".iso")
		if err := os.WriteFile(image, data, 0o644); err != nil {
			t.Fatal(err)
		}
		in := media.Input{Options: []string{"-playlist", "-1"}, Path: "bluray:" + image}
		if got := s.ProbesInput(t.Context(), in); got != (name == "bluray") {
			t.Errorf("%s: %v", name, got)
		}
	}
}
