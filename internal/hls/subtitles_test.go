package hls

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/subtitles"
)

// extraction keeps extracted cues and spans as they come.
type extraction struct {
	mu    sync.Mutex
	cues  map[int][]subtitles.Cue
	spans [][2]time.Duration
}

func (x *extraction) Add(stream int, cue subtitles.Cue) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.cues[stream] = append(x.cues[stream], cue)
}

func (x *extraction) Cover(from, to time.Duration) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.spans = append(x.spans, [2]time.Duration{from, to})
}

func (x *extraction) Covers(from, to time.Duration) bool {
	x.mu.Lock()
	defer x.mu.Unlock()
	for _, s := range x.spans {
		if s[0] <= from && to <= s[1] {
			return true
		}
	}
	return false
}

// texts lists the cues of a stream starting in [from, to), as
// "start text".
func (x *extraction) texts(stream int, from, to time.Duration) []string {
	x.mu.Lock()
	defer x.mu.Unlock()
	var texts []string
	for _, cue := range x.cues[stream] {
		if cue.Start >= from && cue.Start < to {
			texts = append(texts, cue.Start.String()+" "+strings.Join(cue.Lines, "|"))
		}
	}
	slices.Sort(texts)
	return slices.Compact(texts)
}

func TestSubtitlesAreExtractedWhileRemuxing(t *testing.T) {
	ffmpeg, ffprobe := tools(t)
	video, keyframes := source(t, ffmpeg, ffprobe, "aac")
	dir := t.TempDir()
	srt := filepath.Join(dir, "cues.srt")
	ass := filepath.Join(dir, "cues.ass")
	if err := os.WriteFile(srt, []byte("1\n00:00:01,000 --> 00:00:03,000\nOne\n\n2\n00:00:09,500 --> 00:00:12,000\n<i>Two</i>\n\n"+
		"3\n00:00:18,000 --> 00:00:19,000\nThree\non two lines\n\n4\n00:00:25,000 --> 00:00:26,000\nFour\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ass, []byte("[Script Info]\nScriptType: v4.00+\n\n[V4+ Styles]\n"+
		"Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n"+
		"Style: Default,Arial,20,&H00FFFFFF,&H000000FF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,1,0,2,10,10,10,1\n\n[Events]\n"+
		"Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"+
		"Dialogue: 0,0:00:02.00,0:00:04.00,Default,,0,0,0,,{\\i1}Styled{\\i0} text\n"+
		"Dialogue: 0,0:00:23.00,0:00:24.00,Default,,0,0,0,,{\\pos(10,10)}Placed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(dir, "subtitled.mkv")
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-i", video, "-i", srt, "-i", ass,
		"-map", "0", "-map", "1", "-map", "2", "-c:v", "copy", "-c:a", "copy", "-c:s:0", "srt", "-c:s:1", "ass", input).CombinedOutput(); err != nil {
		t.Fatalf("add the subtitles: %v: %s", err, out)
	}
	m, err := NewManager(ffmpeg, t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	plan := NewPlan(keyframes, 30*time.Second)
	x := &extraction{cues: map[int][]subtitles.Cue{}}
	open := func(context.Context) (Remux, func(), error) {
		return Remux{Input: input, Video: 0, Audio: 1, Format: FMP4, Plan: plan, Subtitles: []int{2, 3}, Extracted: x}, func() {}, nil
	}
	key := Key{Session: "session", Audio: 1, Format: FMP4}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	// A player resuming at the segment playing at 18 s gets its subtitles
	// once FFmpeg went past it, in the version's time.
	resumed := plan.Segment(18 * time.Second)
	if err := m.Subtitles(ctx, key, open, resumed); err != nil {
		t.Fatal(err)
	}
	if !x.Covers(plan.Start(resumed), plan.End(resumed)) || x.Covers(0, plan.End(0)) {
		t.Fatalf("covered %v", x.spans)
	}
	if got := x.texts(2, plan.Start(resumed), plan.End(resumed)); !slices.Equal(got, []string{"18s Three|on two lines"}) {
		t.Errorf("SubRip in segment %d: %q", resumed, got)
	}
	// Starting over extracts the rest; ASS keeps its italics, not its
	// positions.
	for _, n := range []int{0, plan.Len() - 1} {
		if err := m.Subtitles(ctx, key, open, n); err != nil {
			t.Fatal(err)
		}
	}
	if got := x.texts(2, 0, plan.End(plan.Len()-1)); !slices.Equal(got, []string{"18s Three|on two lines", "1s One", "25s Four", "9.5s <i>Two</i>"}) {
		t.Errorf("SubRip: %q", got)
	}
	if got := x.texts(3, 0, plan.End(plan.Len()-1)); !slices.Equal(got, []string{"23s Placed", "2s <i>Styled</i> text"}) {
		t.Errorf("ASS: %q", got)
	}
	// A segment holds the cues shown during it, mapped onto the remux's
	// timestamps.
	shown := plan.Segment(9500 * time.Millisecond)
	x.mu.Lock()
	segment := string(SubtitleSegment(x.cues[2], plan, shown))
	x.mu.Unlock()
	if !strings.HasPrefix(segment, "WEBVTT\nX-TIMESTAMP-MAP=MPEGTS:900000,LOCAL:00:00:00.000\n\n00:00:09.500 --> 00:00:12.000\n<i>Two</i>\n") ||
		strings.Contains(segment, "One") || strings.Contains(segment, "Three") {
		t.Errorf("segment %d:\n%s", shown, segment)
	}
}
