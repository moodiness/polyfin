package playback

import (
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/media"
)

// FFmpeg's bluray protocol reads a Blu-ray image, and no DVD image: a scan
// tells a Blu-ray image with a UDF file system only so (see
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
