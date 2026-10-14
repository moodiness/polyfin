package loudness

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// ReplayGain tags are read as written, and Opus's R128 gains moved from
// EBU R128's reference, -23 LUFS, to -18 LUFS.
func TestTagGains(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		tag  string
		gain float64
		ok   bool
	}{
		{"-6.54 dB", -6.54, true},
		{"+2.10 DB", 2.1, true},
		{" 1.5 ", 1.5, true},
		{"loud", 0, false},
		{"-900 dB", 0, false},
		{"", 0, false},
	} {
		if gain, ok := ReplayGain(tc.tag); ok != tc.ok || gain != tc.gain {
			t.Errorf("ReplayGain(%q) = %v %v", tc.tag, gain, ok)
		}
	}
	for _, tc := range []struct {
		tag  string
		gain float64
		ok   bool
	}{
		// -5 dB brings a song at -18 LUFS to -23 LUFS, which it needs no
		// gain to be at -18 LUFS for.
		{"-1280", 0, true},
		{"256", 6, true},
		{"-3840", -10, true},
		{"70000", 0, false},
		{"-1.5", 0, false},
	} {
		if gain, ok := R128Gain(tc.tag); ok != tc.ok || gain != tc.gain {
			t.Errorf("R128Gain(%q) = %v %v", tc.tag, gain, ok)
		}
	}
	if gain, ok := Gain(-23.456); !ok || gain != 5.46 {
		t.Errorf("Gain(-23.456) = %v %v", gain, ok)
	}
	if _, ok := Gain(Silence); ok {
		t.Error("a silent song has a gain")
	}
}

// testFFmpeg returns the FFmpeg of POLYFIN_TEST_FFMPEG, skipping the test
// without it.
func testFFmpeg(t *testing.T) string {
	t.Helper()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	return ffmpeg
}

// tone makes a mono sine of a second at path, at amplitude: EBU R128 reads
// it at 20·log10(amplitude) - 3.01 LUFS, a mono channel counting as one
// of a stereo pair.
func tone(t *testing.T, ffmpeg, path string, amplitude float64) {
	t.Helper()
	if out, err := exec.Command(ffmpeg, "-v", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=1000:duration=3",
		"-af", "volume="+strconv.FormatFloat(amplitude*8, 'f', -1, 64), path).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}

// A meter measures a tone's integrated loudness as EBU R128 has it.
func TestMeterMeasuresTones(t *testing.T) {
	t.Parallel()
	ffmpeg := testFFmpeg(t)
	dir := t.TempDir()
	meter := NewMeter(ffmpeg)
	for _, tc := range []struct {
		name      string
		amplitude float64
	}{
		{"soft.flac", 0.125},
		{"loud.flac", 0.5},
	} {
		path := filepath.Join(dir, tc.name)
		tone(t, ffmpeg, path, tc.amplitude)
		l, err := meter.Measure(t.Context(), path)
		want := 20*math.Log10(tc.amplitude) - 3.01
		if err != nil || math.Abs(l-want) > 0.3 {
			t.Errorf("%s: %v LUFS, want %.1f: %v", tc.name, l, want, err)
		}
	}
	if _, err := meter.Measure(t.Context(), filepath.Join(dir, "missing.flac")); err == nil {
		t.Error("a missing file was measured")
	}
}

// newService returns a service measuring with the FFmpeg at ffmpeg, under
// settings, at the time now gives.
func newService(t *testing.T, ffmpeg string, settings func() accounts.Settings, now func() time.Time) *Service {
	t.Helper()
	s := New(migrated.New(t), NewMeter(ffmpeg), slog.New(slog.NewTextHandler(io.Discard, nil)), settings)
	s.now = now
	t.Cleanup(s.Close)
	return s
}

// countingReader reads the file at path, counting its reads; it fails
// while failing is set.
type countingReader struct {
	path    string
	reads   atomic.Int32
	failing atomic.Bool
}

func (c *countingReader) read(ctx context.Context, read func(context.Context, string, []string) error) error {
	c.reads.Add(1)
	if c.failing.Load() {
		return errors.New("HTTP 502")
	}
	return read(ctx, c.path, nil)
}

// An addon's song is measured once after its first play, and its gain
// given after; a song whose stream fails is tried again after a day, three
// times at most. Songs longer than 15 minutes, or of unknown length, are
// never measured, nor any song with measuring off or for local songs alone.
func TestAddonSongsAreMeasuredOnce(t *testing.T) {
	t.Parallel()
	ffmpeg := testFFmpeg(t)
	path := filepath.Join(t.TempDir(), "song.flac")
	tone(t, ffmpeg, path, 0.125)
	var clock atomic.Int64
	clock.Store(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	now := func() time.Time { return time.Unix(0, clock.Load()) }
	settings := accounts.Settings{EvenVolume: true, MeasureLoudness: "all"}
	s := newService(t, ffmpeg, func() accounts.Settings { return settings }, now)
	album := accounts.ID{15: 1}
	first, second := Track{ID: accounts.ID{15: 2}, Album: album, Length: 3 * time.Second}, Track{ID: accounts.ID{15: 3}, Album: album, Length: 3 * time.Second}
	reader := &countingReader{path: path}
	s.Played(first, reader.read)
	s.Wait()
	s.Played(first, reader.read)
	s.Wait()
	if n := reader.reads.Load(); n != 1 {
		t.Errorf("the song was read %d times", n)
	}
	gains := s.Gains(t.Context(), []accounts.ID{first.ID, second.ID}, []Album{{ID: album, Tracks: 2}})
	if gain, ok := gains[first.ID]; !ok || math.Abs(gain-(Reference+21.07)) > 0.3 {
		t.Errorf("the song's gain: %v", gains)
	}
	if _, ok := gains[album]; ok || len(gains) != 1 {
		t.Errorf("an album with a song unknown has a gain: %v", gains)
	}
	s.Played(second, reader.read)
	s.Wait()
	gains = s.Gains(t.Context(), nil, []Album{{ID: album, Tracks: 2}})
	if gain, ok := gains[album]; !ok || math.Abs(gain-(Reference+21.07)) > 0.3 {
		t.Errorf("the album's gain: %v", gains)
	}

	// A failure is tried again after a day, three times at most.
	failing := Track{ID: accounts.ID{15: 4}, Length: time.Minute}
	reader = &countingReader{path: path}
	reader.failing.Store(true)
	for day := range 5 {
		s.Played(failing, reader.read)
		s.Wait()
		s.Played(failing, reader.read)
		s.Wait()
		if want := int32(min(day+1, MaxAttempts)); reader.reads.Load() != want {
			t.Fatalf("day %d: %d attempts, want %d", day, reader.reads.Load(), want)
		}
		clock.Add(int64(RetryAfter + time.Minute))
	}

	// Too long, of unknown length, or with measuring off for addons.
	reader = &countingReader{path: path}
	s.Played(Track{ID: accounts.ID{15: 5}, Length: MaxLength + time.Second}, reader.read)
	s.Played(Track{ID: accounts.ID{15: 6}}, reader.read)
	s.Wait()
	for _, measure := range []string{"off", "local"} {
		settings.MeasureLoudness = measure
		s.Played(Track{ID: accounts.ID{15: 7}, Length: time.Minute}, reader.read)
		s.Wait()
	}
	settings.MeasureLoudness, settings.EvenVolume = "all", false
	s.Played(Track{ID: accounts.ID{15: 7}, Length: time.Minute}, reader.read)
	s.Wait()
	if n := reader.reads.Load(); n != 0 {
		t.Errorf("%d songs read that are not to be measured", n)
	}
	if gains := s.Gains(t.Context(), []accounts.ID{first.ID}, nil); len(gains) != 0 {
		t.Errorf("gains with the switch off: %v", gains)
	}
}
