package hls

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/testsubtitles"
)

func TestImageSubtitlesAreBurnedIn(t *testing.T) {
	ffmpeg, ffprobe := tools(t)
	video, keyframes := source(t, ffmpeg, ffprobe, "aac")
	dir := t.TempDir()
	sup := filepath.Join(dir, "rectangle.sup")
	// A canvas twice the size of the 160×90 video: the rectangle lands at
	// 30, 60 and is 100 × 20 once scaled.
	rectangle := testsubtitles.Rectangle{CanvasWidth: 320, CanvasHeight: 180, X: 60, Y: 120, Width: 200, Height: 40,
		Shown: [][2]time.Duration{{2 * time.Second, 5 * time.Second}, {16 * time.Second, 19 * time.Second}}}
	if err := os.WriteFile(sup, rectangle.PGS(), 0o600); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(dir, "subtitled.mkv")
	// The subtitles keep their times, rather than starting at zero.
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-copyts", "-i", video, "-i", sup, "-map", "0", "-map", "1", "-c", "copy", input).CombinedOutput(); err != nil {
		t.Fatalf("add the subtitles: %v: %s", err, out)
	}
	// In software, then on each GPU found, which takes larger pictures, and
	// on an NVIDIA GPU that tone maps, the subtitles laid over the frames
	// libplacebo returns.
	for _, variant := range []struct {
		name, gpu string
		toneMap   bool
	}{{"software", "", false}, {"nvenc", "nvenc", false}, {"vaapi", "vaapi", false}, {"nvenc tone mapped", "nvenc", true}} {
		t.Run(variant.name, func(t *testing.T) {
			m, err := NewManager(ffmpeg, t.TempDir(), slog.New(slog.DiscardHandler))
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			encoding := &VideoEncoding{Encoder: "libx264", Level: "4.1", Width: 160, Height: 90, Bitrate: 1_000_000, FrameRate: 24, Burn: new(2)}
			switch {
			case variant.gpu != "":
				hw, ok := m.DetectHardware(variant.gpu, "")
				i := slices.IndexFunc(hw.Encoders, func(e string) bool { return strings.HasPrefix(e, "h264_") })
				if !ok || i < 0 {
					t.Skip("no GPU encodes H.264 with " + variant.gpu)
				}
				if variant.toneMap && !hw.ToneMapping {
					t.Skip("no GPU tone maps with " + variant.gpu)
				}
				encoding = &VideoEncoding{Encoder: hw.Encoders[i], Level: "4.1", Width: 320, Height: 180, Bitrate: 2_000_000, FrameRate: 24, Burn: new(2), ToneMap: variant.toneMap, Hardware: &hw}
			case !slices.Contains(m.Encoders(), "libx264"):
				t.Skip("FFmpeg has no libx264")
			}
			checkBurned(t, ffmpeg, m, Remux{Input: input, Video: 0, Audio: 1, Format: FMP4, Plan: NewPlan(keyframes, 30*time.Second), Encode: encoding}, 2,
				[]shown{{3, true}, {10, false}, {17, true}, {21, false}})
		})
	}
}

// A PGS file and a VobSub pair beside the video, read as a second input,
// are burned in as an image track is; a run started while a subtitle is
// shown shows it.
func TestImageSubtitleFilesAreBurnedIn(t *testing.T) {
	ffmpeg, ffprobe := tools(t)
	video, keyframes := source(t, ffmpeg, ffprobe, "aac")
	dir := t.TempDir()
	rectangle := testsubtitles.Rectangle{CanvasWidth: 320, CanvasHeight: 180, X: 60, Y: 120, Width: 200, Height: 40,
		Shown: [][2]time.Duration{{2 * time.Second, 5 * time.Second}, {15 * time.Second, 19 * time.Second}}}
	idx, sub := rectangle.VobSub()
	for name, data := range map[string][]byte{"rectangle.sup": rectangle.PGS(), "rectangle.idx": []byte(idx), "rectangle.sub": sub} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	plan := NewPlan(keyframes, 30*time.Second)
	// The run first asked starts at the keyframe at 15.3 s, while the
	// second subtitle is shown.
	first := 0
	for first < plan.Len() && plan.Start(first) <= 15*time.Second {
		first++
	}
	if first == plan.Len() || plan.Start(first) > 15500*time.Millisecond {
		t.Fatal("no segment starts between 15 and 15.5 s")
	}
	for _, file := range []string{"rectangle.sup", "rectangle.idx"} {
		t.Run(file, func(t *testing.T) {
			m, err := NewManager(ffmpeg, t.TempDir(), slog.New(slog.DiscardHandler))
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			if !slices.Contains(m.Encoders(), "libx264") {
				t.Skip("FFmpeg has no libx264")
			}
			encoding := &VideoEncoding{Encoder: "libx264", Level: "4.1", Width: 160, Height: 90, Bitrate: 1_000_000, FrameRate: 24, BurnFile: filepath.Join(dir, file)}
			checkBurned(t, ffmpeg, m, Remux{Input: video, Video: 0, Audio: 1, Format: FMP4, Plan: plan, Encode: encoding}, first,
				[]shown{{3, true}, {10, false}, {15.5, true}, {17, true}, {21, false}})
		})
	}
}

// shown is whether the subtitle is shown at a time of the source, in
// seconds.
type shown struct {
	at  float64
	red bool
}

// checkBurned plays remux as an app would, a run started at segment first
// then one from the start, and checks a pixel inside the rectangle, at 80,
// 70 of a 160×90 picture, at each time of want.
func checkBurned(t *testing.T, ffmpeg string, m *Manager, remux Remux, first int, want []shown) {
	t.Helper()
	open := func(context.Context) (Remux, func(), error) { return remux, func() {}, nil }
	key := Key{Session: "session", Audio: remux.Audio, Format: remux.Format}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	if f, err := m.Segment(ctx, key, open, first); err != nil {
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
	for n := range remux.Plan.Len() {
		f, err := m.Segment(ctx, key, open, n)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(f.Name())
		f.Close()
		joined = append(joined, data...)
	}
	all := filepath.Join(t.TempDir(), "joined.mp4")
	if err := os.WriteFile(all, joined, 0o600); err != nil {
		t.Fatal(err)
	}
	encoding := remux.Encode
	crop := "crop=1:1:" + strconv.Itoa(80*encoding.Width/160) + ":" + strconv.Itoa(70*encoding.Height/90)
	for _, c := range want {
		out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-copyts", "-i", all,
			"-vf", "select=gte(t\\,"+strconv.FormatFloat(c.at+timestampOffset.Seconds(), 'f', 3, 64)+"),format=rgb24,"+crop,
			"-frames:v", "1", "-f", "rawvideo", "-pix_fmt", "rgb24", "-").Output()
		if err != nil || len(out) != 3 {
			t.Fatalf("pixel at %.1f s: %v", c.at, err)
		}
		if red := out[0] > 160 && out[1] < 90 && out[2] < 90; red != c.red {
			t.Errorf("at %.1f s: color %v, subtitle shown %v", c.at, out, c.red)
		}
	}
}
