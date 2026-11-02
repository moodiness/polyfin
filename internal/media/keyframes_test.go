package media

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// ffprobe's lines give the keyframe packets' times, in decoding order,
// and the file's start: N/A times and other packets are left out; times
// that go back past the reordering of frames make no keyframes.
func TestParseKeyframesReadsKeyframePackets(t *testing.T) {
	times, start, err := parseKeyframes(strings.NewReader(
		"packet,1.483333,K__,\npacket,1.525000,___,\npacket,N/A,K__,\npacket,3.983333,K_D,\npacket,3.900000,K__\nformat,1.462000\n"))
	if err != nil || start != 1462*time.Millisecond ||
		!slices.Equal(times, []time.Duration{1483333 * time.Microsecond, 3983333 * time.Microsecond, 3900 * time.Millisecond}) {
		t.Errorf("%v from %v: %v", times, start, err)
	}
	if _, _, err := parseKeyframes(strings.NewReader("packet,7200.0,K__,\npacket,2.0,K__,\n")); !errors.Is(err, ErrNoKeyframes) {
		t.Errorf("timestamps going back: %v", err)
	}
}

// Keyframes reads an MPEG-TS file whole: the times of its video's
// keyframes, from the file's start, which its audio sets 21 ms before its
// video; a file of no media is unreadable.
func TestKeyframesOfAnMPEGTSFile(t *testing.T) {
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	prober := Prober{Path: filepath.Join(filepath.Dir(ffmpeg), "ffprobe")}
	dir := t.TempDir()
	file := filepath.Join(dir, "movie.ts")
	if out, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=128x72:rate=24", "-f", "lavfi", "-i", "sine=sample_rate=48000",
		"-t", "30", "-c:v", "libx264", "-preset", "veryfast", "-bf", "3", "-g", "1000", "-sc_threshold", "0",
		"-force_key_frames", "2.5,7,9,13,17.5,22,26", "-c:a", "aac", "-f", "mpegts", file).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	times, err := prober.Keyframes(t.Context(), file)
	if err != nil {
		t.Fatal(err)
	}
	var want []time.Duration
	for _, at := range []time.Duration{0, 2500, 7000, 9000, 13000, 17500, 22000, 26000} {
		want = append(want, at*time.Millisecond+21333*time.Microsecond)
	}
	if !slices.Equal(times, want) {
		t.Errorf("keyframes %v, not %v", times, want)
	}
	garbage := filepath.Join(dir, "broken.ts")
	if err := os.WriteFile(garbage, []byte(strings.Repeat("not a transport stream ", 1000)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := prober.Keyframes(t.Context(), garbage); !errors.Is(err, ErrUnreadable) && !errors.Is(err, ErrNoKeyframes) {
		t.Errorf("a file of no media: %v", err)
	}
}
