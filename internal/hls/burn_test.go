package hls

import (
	"context"
	"encoding/binary"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"
)

// pgsRectangle writes a PGS subtitle stream on a canvas of width × height:
// a rectangle in palette color 1, red, at x, y, shown during each span of
// shown.
func pgsRectangle(canvasWidth, canvasHeight, x, y, width, height int, shown [][2]time.Duration) []byte {
	var out []byte
	segment := func(at time.Duration, kind byte, payload []byte) {
		ticks := uint32(at / time.Second * 90000)
		header := []byte{'P', 'G', 0, 0, 0, 0, 0, 0, 0, 0, kind, 0, 0}
		binary.BigEndian.PutUint32(header[2:], ticks)
		binary.BigEndian.PutUint16(header[11:], uint16(len(payload)))
		out = append(append(out, header...), payload...)
	}
	u16 := func(v int) []byte { return []byte{byte(v >> 8), byte(v)} }
	window := slices.Concat([]byte{1, 0}, u16(x), u16(y), u16(width), u16(height))
	// Each line: width pixels of color 1, then the end of the line.
	var lines []byte
	for range height {
		lines = append(lines, 0, 0xC0|byte(width>>8), byte(width), 1, 0, 0)
	}
	object := slices.Concat([]byte{0, 0, 0, 0xC0}, []byte{byte((len(lines) + 4) >> 16), byte((len(lines) + 4) >> 8), byte(len(lines) + 4)},
		u16(width), u16(height), lines)
	for i, span := range shown {
		composition := slices.Concat(u16(canvasWidth), u16(canvasHeight), []byte{0x10}, u16(2*i), []byte{0x80, 0, 0, 1}, []byte{0, 0, 0, 0}, u16(x), u16(y))
		segment(span[0], 0x16, composition)
		segment(span[0], 0x17, window)
		// Y, Cr, Cb and alpha of red.
		segment(span[0], 0x14, []byte{0, 0, 1, 81, 240, 90, 255})
		segment(span[0], 0x15, object)
		segment(span[0], 0x80, nil)
		// A composition without objects clears the screen.
		segment(span[1], 0x16, slices.Concat(u16(canvasWidth), u16(canvasHeight), []byte{0x10}, u16(2*i+1), []byte{0, 0, 0, 0}))
		segment(span[1], 0x17, window)
		segment(span[1], 0x80, nil)
	}
	return out
}

func TestImageSubtitlesAreBurnedIn(t *testing.T) {
	ffmpeg, ffprobe := tools(t)
	video, keyframes := source(t, ffmpeg, ffprobe, "aac")
	dir := t.TempDir()
	sup := filepath.Join(dir, "rectangle.sup")
	// A canvas twice the size of the 160×90 video: the rectangle lands at
	// 30, 60 and is 100 × 20 once scaled.
	if err := os.WriteFile(sup, pgsRectangle(320, 180, 60, 120, 200, 40, [][2]time.Duration{{2 * time.Second, 5 * time.Second}, {16 * time.Second, 19 * time.Second}}), 0o600); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(dir, "subtitled.mkv")
	// The subtitles keep their times, rather than starting at zero.
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-copyts", "-i", video, "-i", sup, "-map", "0", "-map", "1", "-c", "copy", input).CombinedOutput(); err != nil {
		t.Fatalf("add the subtitles: %v: %s", err, out)
	}
	m, err := NewManager(ffmpeg, t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if !slices.Contains(m.Encoders(), "libx264") {
		t.Skip("FFmpeg has no libx264")
	}
	plan := NewPlan(keyframes, 30*time.Second)
	open := func(context.Context) (Remux, func(), error) {
		return Remux{Input: input, Video: 0, Audio: 1, Format: FMP4, Plan: plan,
			Encode: &VideoEncoding{Encoder: "libx264", Level: "4.1", Width: 160, Height: 90, Bitrate: 1_000_000, FrameRate: 24, Burn: new(2)}}, func() {}, nil
	}
	key := Key{Session: "session", Audio: 1, Format: FMP4}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	// A run started at segment 2, then one from the start.
	if f, err := m.Segment(ctx, key, open, 2); err != nil {
		t.Fatal(err)
	} else {
		f.Close()
	}
	init, err := m.Init(ctx, key, open)
	if err != nil {
		t.Fatal(err)
	}
	joined, _ := os.ReadFile(init.Name())
	init.Close()
	for n := range plan.Len() {
		f, err := m.Segment(ctx, key, open, n)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(f.Name())
		f.Close()
		joined = append(joined, data...)
	}
	all := filepath.Join(dir, "joined.mp4")
	if err := os.WriteFile(all, joined, 0o600); err != nil {
		t.Fatal(err)
	}
	// The color of a pixel inside the rectangle, at a time of the source.
	pixel := func(at float64) []byte {
		t.Helper()
		out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-copyts", "-i", all,
			"-vf", "select=gte(t\\,"+strconv.FormatFloat(at+timestampOffset.Seconds(), 'f', 3, 64)+"),format=rgb24,crop=1:1:80:70",
			"-frames:v", "1", "-f", "rawvideo", "-pix_fmt", "rgb24", "-").Output()
		if err != nil || len(out) != 3 {
			t.Fatalf("pixel at %.1f s: %v", at, err)
		}
		return out
	}
	red := func(rgb []byte) bool { return rgb[0] > 160 && rgb[1] < 90 && rgb[2] < 90 }
	for _, c := range []struct {
		at  float64
		red bool
	}{{3, true}, {10, false}, {17, true}, {21, false}} {
		if got := pixel(c.at); red(got) != c.red {
			t.Errorf("at %.0f s: color %v, subtitle shown %v", c.at, got, c.red)
		}
	}
}
