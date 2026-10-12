package hls

import (
	"context"
	"log/slog"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/metrics"
)

// A conversion on the processor counts while it runs, and the segments it
// makes count; one FFmpeg fails counts as failed.
func TestConversionsCountInTheMetrics(t *testing.T) {
	ffmpeg, ffprobe := tools(t)
	input, video, audio := unindexedSource(t, ffmpeg, ffprobe, "second.ts", "-c:v", "libx264", "-preset", "veryfast", "-g", "24", "-c:a", "aac")
	m, err := NewManager(ffmpeg, t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if !slices.Contains(m.Encoders(), "libx264") {
		t.Skip("FFmpeg has no libx264")
	}
	converting := func() float64 {
		for _, sample := range m.ConversionsByMethod() {
			if sample.Labels[0] == MethodProcessor {
				return sample.Value
			}
		}
		return -1
	}
	processor := map[string]string{"method": MethodProcessor}
	segments, failures := metrics.Value("polyfin_segments_total", processor), metrics.Value("polyfin_encoding_failures_total", processor)
	opener := func(input string) Opener {
		return func(context.Context) (Remux, func(), error) {
			return Remux{Input: input, InputOptions: []string{"-start_at_zero"}, Video: video, Audio: audio, Format: TS,
				Plan:   NewGridPlan(30 * time.Second).Searched(),
				Encode: &VideoEncoding{Encoder: "libx264", Level: "4.1", Width: 128, Height: 72, Bitrate: 300_000, FrameRate: 24}}, func() {}, nil
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	f, err := m.Segment(ctx, Key{Session: "converting", Audio: audio, Format: TS}, opener(input), 0)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if got := converting(); got != 1 {
		t.Errorf("conversions on the processor while one runs: %v", got)
	}
	if got := metrics.Value("polyfin_segments_total", processor); got <= segments {
		t.Errorf("segments made: %v, before %v", got, segments)
	}
	m.Stop("converting")
	if got := converting(); got != 0 {
		t.Errorf("conversions on the processor once stopped: %v", got)
	}

	if _, err := m.Segment(ctx, Key{Session: "failing", Audio: audio, Format: TS}, opener(filepath.Join(t.TempDir(), "missing.ts")), 0); err == nil {
		t.Fatal("a missing file made a segment")
	}
	m.Stop("failing")
	if got := metrics.Value("polyfin_encoding_failures_total", processor); got != failures+1 {
		t.Errorf("failures: %v, before %v", got, failures)
	}
}
