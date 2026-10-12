package hls

import (
	"context"
	"io"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// unindexedSource writes a 30-second file of 24 frames a second FFmpeg
// reads without an index, encoded with args, and returns it with the
// FFmpeg indexes of its video and audio.
func unindexedSource(t *testing.T, ffmpeg, ffprobe, name string, args ...string) (string, int, int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	command := slices.Concat([]string{"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=160x90:rate=24", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "30"},
		args, []string{path})
	if out, err := exec.Command(ffmpeg, command...).CombinedOutput(); err != nil {
		t.Fatalf("make the source: %v: %s", err, out)
	}
	out, err := exec.Command(ffprobe, "-v", "error", "-show_entries", "stream=index,codec_type", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	video, audio := -1, -1
	for line := range strings.Lines(string(out)) {
		fields := strings.Split(strings.TrimSpace(line), ",")
		if len(fields) < 2 {
			continue
		}
		n, _ := strconv.Atoi(fields[0])
		switch fields[1] {
		case "video":
			video = n
		case "audio":
			audio = n
		}
	}
	if video < 0 || audio < 0 {
		t.Fatalf("the source's streams: %s", out)
	}
	return path, video, audio
}

// A segment of a version FFmpeg searches by its timestamps, read first, as
// after a seek, holds the frames it holds when the segments are read in
// order, whether the source's keyframes are closer than a segment or
// further apart: FFmpeg, which lands on the first keyframe past the time
// asked, starts far enough before the segment.
func TestSearchedSegmentsHoldTheFramesReadInOrder(t *testing.T) {
	ffmpeg, ffprobe := tools(t)
	for _, c := range []struct {
		name, file string
		args       []string
	}{
		{"MPEG-TS, a keyframe a second", "second.ts", []string{"-c:v", "libx264", "-preset", "veryfast", "-bf", "3", "-g", "24", "-keyint_min", "24",
			"-sc_threshold", "0", "-c:a", "aac"}},
		{"MPEG-TS, a keyframe every 10 seconds", "long.ts", []string{"-c:v", "libx264", "-preset", "veryfast", "-bf", "3", "-g", "240", "-keyint_min", "240",
			"-sc_threshold", "0", "-c:a", "aac"}},
		{"MPEG program stream, a keyframe every 10 seconds", "long.mpg", []string{"-c:v", "mpeg2video", "-g", "240", "-bf", "2", "-c:a", "mp2"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			input, video, audio := unindexedSource(t, ffmpeg, ffprobe, c.file, c.args...)
			m, err := NewManager(ffmpeg, t.TempDir(), slog.New(slog.DiscardHandler))
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			if !slices.Contains(m.Encoders(), "libx264") {
				t.Skip("FFmpeg has no libx264")
			}
			plan := NewGridPlan(30 * time.Second).Searched()
			open := func(context.Context) (Remux, func(), error) {
				return Remux{Input: input, InputOptions: []string{"-start_at_zero"}, Video: video, Audio: audio, Format: TS, Plan: plan,
					Encode: &VideoEncoding{Encoder: "libx264", Level: "4.1", Width: 128, Height: 72, Bitrate: 300_000, FrameRate: 24}}, func() {}, nil
			}
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
			defer cancel()
			dir := t.TempDir()
			// frames are the times of a segment's video frames, read in a play
			// session.
			frames := func(session string, n int) []string {
				t.Helper()
				f, err := m.Segment(ctx, Key{Session: session, Audio: audio, Format: TS}, open, n)
				if err != nil {
					t.Fatalf("segment %d in %s: %v", n, session, err)
				}
				defer f.Close()
				data, err := io.ReadAll(f)
				if err != nil {
					t.Fatal(err)
				}
				one := filepath.Join(dir, session+"-"+strconv.Itoa(n)+".ts")
				if err := os.WriteFile(one, data, 0o600); err != nil {
					t.Fatal(err)
				}
				var times []string
				for _, p := range probePackets(t, ffprobe, one) {
					if p.Type == "video" {
						times = append(times, p.PTS)
					}
				}
				return times
			}
			inOrder := make([][]string, plan.Len())
			for n := range plan.Len() {
				inOrder[n] = frames("in order", n)
				at, _ := strconv.ParseFloat(inOrder[n][0], 64)
				if start := plan.Start(n).Seconds(); n > 0 && math.Abs(at-timestampOffset.Seconds()-start) > 1.0/24 {
					t.Fatalf("segment %d, read in order, starts at %.3f, want %.3f", n, at-timestampOffset.Seconds(), start)
				}
			}
			m.Stop("in order")
			for n := 1; n < plan.Len(); n++ {
				session := "seek " + strconv.Itoa(n)
				if got := frames(session, n); !slices.Equal(got, inOrder[n]) {
					t.Errorf("segment %d, read first, holds the frames at %v, want %v", n, got, inOrder[n])
				}
				m.Stop(session)
			}
		})
	}
}
