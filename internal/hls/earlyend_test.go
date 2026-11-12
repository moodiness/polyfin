package hls

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/process"
)

// earlyEnd is a version and what FFmpeg makes of it from its start: 30
// seconds of video with a keyframe every second, cut into 8 segments, the
// fifth from 14 to 18 seconds; and its fragmented MP4 output, with the
// offsets of its fragments by their second.
type earlyEnd struct {
	ffmpeg, source, output string
	plan                   Plan
	// fragments are the offsets of the fragments' moof boxes, and middles
	// those of the middle of their media data, by the fragments' second.
	fragments, middles map[int]int
}

// earlyEndVersion makes the version, with the FFmpeg of
// POLYFIN_TEST_FFMPEG, and FFmpeg's output of it as a job from its start
// gets it.
func earlyEndVersion(t *testing.T) earlyEnd {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("FFmpeg's stand-in is a shell script")
	}
	ffmpeg, _ := tools(t)
	dir := t.TempDir()
	v := earlyEnd{ffmpeg: ffmpeg, source: filepath.Join(dir, "source.mkv"), output: filepath.Join(dir, "output.mp4"),
		fragments: map[int]int{}, middles: map[int]int{}}
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=24", "-t", "30",
		"-c:v", "libx264", "-preset", "veryfast", "-bf", "0", "-g", "24", "-keyint_min", "24", "-sc_threshold", "0", v.source).CombinedOutput(); err != nil {
		t.Fatalf("make the source: %v: %s", err, out)
	}
	var keyframes []time.Duration
	for at := range 30 {
		keyframes = append(keyframes, time.Duration(at)*time.Second)
	}
	v.plan = NewPlan(keyframes, 30*time.Second)
	if v.plan.Len() != 8 || v.plan.Start(4) != 14*time.Second || v.plan.Start(5) != 18*time.Second {
		t.Fatalf("the plan: %v", v.plan.starts)
	}
	// The command line of a job from the start, writing to a file rather
	// than to the pipes.
	args := Remux{Input: v.source, Video: 0, Audio: -1, Format: FMP4, Plan: v.plan}.args(0, 0)
	if i := slices.Index(args, "-progress"); i >= 0 {
		args = slices.Delete(args, i, i+2)
	}
	args[len(args)-1] = v.output
	if out, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
		t.Fatalf("make the output: %v: %s", err, out)
	}
	data, err := os.ReadFile(v.output)
	if err != nil {
		t.Fatal(err)
	}
	var video uint32
	var timescale uint64
	second := -1
	for at := 0; at+8 <= len(data); {
		size := int(binary.BigEndian.Uint32(data[at:]))
		if size < 8 || at+size > len(data) {
			t.Fatalf("a box of %d bytes at %d", size, at)
		}
		switch string(data[at+4 : at+8]) {
		case "moov":
			if video, timescale, err = videoTrack(data[at+8 : at+size]); err != nil {
				t.Fatal(err)
			}
		case "moof":
			presentation, _ := fragmentStart(data[at+8:at+size], video)
			second = int(((ticks(presentation, timescale) - timestampOffset) + 10*time.Millisecond) / time.Second)
			v.fragments[second] = at
		case "mdat":
			v.middles[second] = at + size/2
		}
		at += size
	}
	if _, ok := v.middles[15]; !ok {
		t.Fatalf("no fragment at 15 s: %v", v.fragments)
	}
	return v
}

// standIn is an FFmpeg that runs the real one for what is not a job, and
// does as body says for a job, writing down each run.
func (v earlyEnd) standIn(t *testing.T, body string) (*Manager, *syncBuffer, func() int) {
	t.Helper()
	dir := t.TempDir()
	runs, script := filepath.Join(dir, "runs"), filepath.Join(dir, "ffmpeg")
	text := "#!/bin/sh\ncase \"$*\" in *pipe:1*) ;; *) exec '" + v.ffmpeg + "' \"$@\" ;; esac\necho run >> '" + runs + "'\n" + body
	if err := process.WriteExecutable(script, strings.NewReader(text), 0o700); err != nil {
		t.Fatal(err)
	}
	log := &syncBuffer{}
	m, err := NewManager(script, t.TempDir(), slog.New(slog.NewTextHandler(log, &slog.HandlerOptions{Level: slog.LevelDebug})))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m, log, func() int {
		data, _ := os.ReadFile(runs)
		return bytes.Count(data, []byte("\n"))
	}
}

// earlySize is the size the version's file is said to have.
const earlySize = 123456

// segment opens segment n of the version in a play session, as a player
// asks for it.
func (v earlyEnd) segment(ctx context.Context, m *Manager, session string, n int) error {
	open := func(context.Context) (Remux, func(), error) {
		return Remux{Input: v.source, Video: 0, Audio: -1, Format: FMP4, Plan: v.plan, Size: earlySize}, func() {}, nil
	}
	f, err := m.Segment(ctx, Key{Session: session, Audio: -1, Format: FMP4, Version: "v1"}, open, n)
	if err == nil {
		_ = f.Close()
	}
	return err
}

// An FFmpeg dying twice in the middle of the same segment, its source
// sound, found where the version ends: the whole fragments it wrote of
// that segment are the version's last segment, and the segments past it
// are answered that they do not exist, without FFmpeg, in any play of the
// version, until the version comes with another size. The first death
// is told once, with FFmpeg's exit status and what it wrote.
func TestFFmpegDyingTwiceAtTheSamePointEndsTheVersionThere(t *testing.T) {
	t.Parallel()
	v := earlyEndVersion(t)
	m, log, runs := v.standIn(t, "head -c "+strconv.Itoa(v.middles[15])+" '"+v.output+"'\necho 'Invalid NAL unit size' >&2\nexit 134\n")
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	for n := range 5 {
		if err := v.segment(ctx, m, "player", n); err != nil {
			t.Fatalf("segment %d: %v", n, err)
		}
	}
	if got := runs(); got != 2 {
		t.Errorf("%d runs of FFmpeg to segment 4, want 2", got)
	}
	for n := 5; n < v.plan.Len(); n++ {
		if err := v.segment(ctx, m, "player", n); !errors.Is(err, ErrNotFound) {
			t.Errorf("segment %d past the end: %v", n, err)
		}
	}
	if err := v.segment(ctx, m, "another player", 6); !errors.Is(err, ErrNotFound) {
		t.Errorf("segment 6 in another play: %v", err)
	}
	if got := runs(); got != 2 {
		t.Errorf("%d runs of FFmpeg once the end was found, want 2", got)
	}
	end, ok := m.End("v1", earlySize)
	if !ok || end != v.plan.Start(5) {
		t.Errorf("the version ends at %v (%v), want %v", end, ok, v.plan.Start(5))
	}
	if cut := v.plan.Until(end); cut.Len() != 5 || cut.End(4) != end {
		t.Errorf("the plan cut at the end: %d segments, the last ending at %v", cut.Len(), cut.End(4))
	}
	logged := log.String()
	if strings.Count(logged, `level=WARN msg="A remux failed"`) != 1 || !strings.Contains(logged, "exit status 134") ||
		!strings.Contains(logged, "Invalid NAL unit size") {
		t.Errorf("the death is not told once with FFmpeg's status and words:\n%s", logged)
	}
	if strings.Count(logged, `msg="A version ends early`) != 1 {
		t.Errorf("the end is not told once:\n%s", logged)
	}
	if _, ok := m.End("v1", earlySize+1); ok {
		t.Error("the end is kept for another size of the version")
	}
	if _, ok := m.End("v1", earlySize); ok {
		t.Error("the end is kept once the version came with another size")
	}
}

// An FFmpeg failing every run from the same segment is run 3 times from
// there: the segment is then answered that it does not exist, with the
// failure, without FFmpeg. The failure is a warning once, then told at
// debug level, and giving up is a warning.
func TestFFmpegFailingFromTheSameSegmentIsGivenUpOn(t *testing.T) {
	t.Parallel()
	v := earlyEndVersion(t)
	m, log, runs := v.standIn(t, "head -c "+strconv.Itoa(v.fragments[14])+" '"+v.output+"'\necho 'a fault' >&2\nexit 1\n")
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	for attempt := 1; attempt <= 2; attempt++ {
		if err := v.segment(ctx, m, "player", 4); !errors.Is(err, ErrFailed) || errors.Is(err, ErrNotFound) {
			t.Errorf("attempt %d: %v", attempt, err)
		}
	}
	for attempt := 3; attempt <= 5; attempt++ {
		if err := v.segment(ctx, m, "player", 4); !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "a fault") {
			t.Errorf("attempt %d, once given up: %v", attempt, err)
		}
	}
	if got := runs(); got != 3 {
		t.Errorf("%d runs of FFmpeg, want 3", got)
	}
	if _, ok := m.End("v1", earlySize); ok {
		t.Error("failures were taken for the version's end")
	}
	logged := log.String()
	if strings.Count(logged, `level=WARN msg="A remux failed"`) != 1 || strings.Count(logged, `level=DEBUG msg="A remux failed"`) != 2 ||
		strings.Count(logged, `level=WARN msg="An encoding gave up`) != 1 {
		t.Errorf("the failures are not told once, then at debug level, then given up on:\n%s", logged)
	}
}

// An FFmpeg dying once in the middle of a segment of a sound version is
// no end: the next request starts FFmpeg again, which plays the version
// to its end, nothing is told of an end, and the death is told with
// FFmpeg's exit status and what it wrote.
func TestFFmpegDyingOnceIsNotTheVersionsEnd(t *testing.T) {
	t.Parallel()
	v := earlyEndVersion(t)
	marker := filepath.Join(t.TempDir(), "died")
	m, log, runs := v.standIn(t, "if [ ! -f '"+marker+"' ]; then\n\t: > '"+marker+"'\n\thead -c "+strconv.Itoa(v.middles[15])+" '"+v.output+
		"'\n\techo 'Invalid NAL unit size' >&2\n\texit 134\nfi\nexec '"+v.ffmpeg+"' \"$@\"\n")
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	for n := range v.plan.Len() {
		if err := v.segment(ctx, m, "player", n); err != nil {
			t.Fatalf("segment %d: %v", n, err)
		}
	}
	if got := runs(); got != 2 {
		t.Errorf("%d runs of FFmpeg, want 2", got)
	}
	if _, ok := m.End("v1", earlySize); ok {
		t.Error("a single death was taken for the version's end")
	}
	logged := log.String()
	if strings.Contains(logged, "A version ends early") {
		t.Errorf("an end is told:\n%s", logged)
	}
	if strings.Count(logged, `level=WARN msg="A remux failed"`) != 1 || !strings.Contains(logged, "exit status 134") ||
		!strings.Contains(logged, "Invalid NAL unit size") {
		t.Errorf("the death is not told with FFmpeg's status and words:\n%s", logged)
	}
}
