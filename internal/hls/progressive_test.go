package hls

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"
)

// A conversion streamed as one file counts toward the conversions at once
// while it runs, from before its input opens: one more is refused, the
// dashboard lists it, and stopping its play session ends it.
func TestProgressiveConversionsCount(t *testing.T) {
	t.Parallel()
	m, err := NewManager("ffmpeg-not-installed", t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	m.LimitConversions(func() int { return 1 })
	// The input opens only once the test lets it: until then, the
	// conversion is under way.
	opening := make(chan struct{})
	open := func(ctx context.Context) (Remux, func(), error) {
		close(opening)
		<-ctx.Done()
		return Remux{}, nil, ctx.Err()
	}
	download := Key{Session: "phone-1", Audio: -1, Format: FMP4, User: "alice", Version: "v1", Converts: true}
	ended := make(chan error, 1)
	go func() { ended <- m.Progressive(t.Context(), download, open, 0, &bytes.Buffer{}) }()
	<-opening

	if running, limit := m.Conversions(); running != 1 || limit != 1 {
		t.Errorf("conversions: %d of %d", running, limit)
	}
	if m.MayConvert("bob", "v2") {
		t.Error("a conversion past the limit may start")
	}
	other := Key{Session: "bob-1", Audio: -1, Format: TS, User: "bob", Version: "v2", Converts: true}
	if err := m.Progressive(t.Context(), other, open, 0, &bytes.Buffer{}); !errors.Is(err, ErrBusy) {
		t.Errorf("a second download past the limit: %v", err)
	}
	if _, err := m.Segment(t.Context(), other, func(context.Context) (Remux, func(), error) { return Remux{}, nil, errors.New("opened") }, 0); !errors.Is(err, ErrBusy) {
		t.Errorf("an HLS conversion past the limit: %v", err)
	}
	running := m.Running()
	if len(running) != 1 || !running[0].Progressive || running[0].Key != download {
		t.Errorf("running: %+v", running)
	}
	m.Stop("phone-1")
	if err := <-ended; !errors.Is(err, ErrStopped) {
		t.Errorf("stopped: %v", err)
	}
	if running, _ := m.Conversions(); running != 0 || !m.MayConvert("bob", "v2") {
		t.Errorf("after the download stopped: %d running", running)
	}
}

// Two conversions streamed as one file that start at once, of the same
// playback even, run two FFmpeg: at a limit of one, one runs and the other
// is refused, and a play of the version is refused while it runs.
func TestProgressiveDownloadsAtOnce(t *testing.T) {
	t.Parallel()
	m, err := NewManager("ffmpeg-not-installed", t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	m.LimitConversions(func() int { return 1 })
	opened := make(chan string, 2)
	ended := make(chan error, 2)
	for _, session := range []string{"phone-1", "tablet-1"} {
		key := Key{Session: session, Audio: -1, Format: FMP4, User: "alice", Version: "v1", Converts: true}
		// The input opens only once the download is stopped.
		open := func(ctx context.Context) (Remux, func(), error) {
			opened <- session
			<-ctx.Done()
			return Remux{}, nil, ctx.Err()
		}
		go func() { ended <- m.Progressive(t.Context(), key, open, 0, &bytes.Buffer{}) }()
	}
	var running []string
	for refused := false; !refused || len(running) == 0; {
		select {
		case err := <-ended:
			if !errors.Is(err, ErrBusy) {
				t.Fatalf("a download ended: %v", err)
			}
			refused = true
		case session := <-opened:
			if running = append(running, session); len(running) == 2 {
				t.Fatal("both downloads started past the limit")
			}
		}
	}
	if n, _ := m.Conversions(); n != 1 || m.MayConvert("alice", "v1") || m.MayConvertAlone() {
		t.Errorf("while one download runs: %d conversions, may convert", n)
	}
	m.Stop(running[0])
	if err := <-ended; !errors.Is(err, ErrStopped) {
		t.Errorf("stopped: %v", err)
	}
	if !m.MayConvertAlone() {
		t.Error("a download may not start once the other stopped")
	}
}

// A version streamed as one file, copied or converted, in fragmented MP4
// or MPEG-TS, plays from the start asked, starting at zero.
func TestProgressiveStreamsPlay(t *testing.T) {
	t.Parallel()
	ffmpeg, ffprobe := tools(t)
	input := filepath.Join(t.TempDir(), "source.mkv")
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "6", "-c:v", "libx264", "-preset", "ultrafast", "-g", "48",
		"-c:a", "aac", "-b:a", "96k", input).CombinedOutput(); err != nil {
		t.Fatalf("make the source: %v: %s", err, out)
	}
	m, err := NewManager(ffmpeg, t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	if !slices.Contains(m.Encoders(), "libx264") {
		t.Skip("FFmpeg has no libx264")
	}
	for _, c := range []struct {
		name   string
		format Format
		encode *VideoEncoding
		start  time.Duration
		// width is the video's, duration about how long it plays.
		width    int
		duration float64
	}{
		{"copied to MP4", FMP4, nil, 0, 320, 6},
		{"converted to MP4", FMP4, &VideoEncoding{Encoder: "libx264", Level: "3.0", Width: 160, Height: 90, Bitrate: 200_000, FrameRate: 24}, 0, 160, 6},
		{"converted to MPEG-TS from 2 s", TS, &VideoEncoding{Encoder: "libx264", Level: "3.0", Width: 160, Height: 90, Bitrate: 200_000, FrameRate: 24},
			2 * time.Second, 160, 4},
	} {
		t.Run(c.name, func(t *testing.T) {
			open := func(context.Context) (Remux, func(), error) {
				return Remux{Input: input, Video: 0, Audio: 1, Format: c.format, Encode: c.encode}, func() {}, nil
			}
			key := Key{Session: c.name, Audio: 1, Format: c.format, User: "alice", Version: "v1", Converts: c.encode != nil}
			var out bytes.Buffer
			if err := m.Progressive(t.Context(), key, open, c.start, &out); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(t.TempDir(), "out."+c.format.Extension())
			if err := writeFile(file, out.Bytes()); err != nil {
				t.Fatal(err)
			}
			probe, err := exec.Command(ffprobe, "-v", "error", "-show_entries", "stream=codec_type,width,start_time:format=duration",
				"-of", "json", file).Output()
			if err != nil {
				t.Fatalf("probe: %v", err)
			}
			var result struct {
				Streams []struct {
					Type  string `json:"codec_type"`
					Width int
					Start string `json:"start_time"`
				}
				Format struct{ Duration string }
			}
			if err := json.Unmarshal(probe, &result); err != nil || len(result.Streams) != 2 {
				t.Fatalf("probe: %v %s", err, probe)
			}
			duration, _ := strconv.ParseFloat(result.Format.Duration, 64)
			start, _ := strconv.ParseFloat(result.Streams[0].Start, 64)
			if result.Streams[0].Type != "video" || result.Streams[0].Width != c.width || duration < c.duration-0.5 || duration > c.duration+0.5 ||
				start > 1.5 {
				t.Errorf("streamed: %s", probe)
			}
			// It decodes from start to end.
			if out, err := exec.Command(ffmpeg, "-v", "error", "-xerror", "-i", file, "-f", "null", "-").CombinedOutput(); err != nil {
				t.Errorf("decode: %v: %s", err, out)
			}
		})
	}
}

// The command lines stream one file: the source's timestamps shifted to
// start at zero, keyframes every 3 seconds when converting, a subtitle
// burned in, AAC from MPEG-TS reframed for MP4.
func TestProgressiveArgs(t *testing.T) {
	t.Parallel()
	burn := 2
	copied := Remux{Input: "http://127.0.0.1:1/a.ts", InputOptions: []string{"-start_at_zero"}, Video: 0, Audio: 1, Format: FMP4, ADTS: true,
		VideoTag: "avc1"}.progressiveArgs(90 * time.Second)
	for _, want := range [][]string{{"-ss", "90.000000"}, {"-start_at_zero", "-copyts", "-i", "http://127.0.0.1:1/a.ts"},
		{"-bsf:a", "aac_adtstoasc"}, {"-avoid_negative_ts", "make_zero"}, {"-tag:v", "avc1"}, {"-f", "mp4"}} {
		if !containsRun(copied, want) {
			t.Errorf("copied: %v lacks %v", copied, want)
		}
	}
	converted := Remux{Input: "http://127.0.0.1:1/a.mkv", Video: 0, Audio: 1, Format: TS, AudioCodec: "aac", AudioChannels: 2, AudioBitrate: 192_000,
		Encode: &VideoEncoding{Encoder: "libx264", Level: "4.0", Width: 1920, Height: 1080, Bitrate: 8_000_000, FrameRate: 24, Burn: &burn}}.progressiveArgs(0)
	for _, want := range [][]string{{"-force_key_frames", progressiveKeyframes}, {"-map", "[video]"}, {"-c:a", "aac"}, {"-f", "mpegts", "pipe:1"}} {
		if !containsRun(converted, want) {
			t.Errorf("converted: %v lacks %v", converted, want)
		}
	}
	if slices.Contains(converted, "-ss") || slices.Contains(converted, "-bsf:a") {
		t.Errorf("converted from the start: %v", converted)
	}
}

// containsRun reports whether args holds want, in a row.
func containsRun(args, want []string) bool {
	for i := range args {
		if i+len(want) <= len(args) && slices.Equal(args[i:i+len(want)], want) {
			return true
		}
	}
	return false
}
