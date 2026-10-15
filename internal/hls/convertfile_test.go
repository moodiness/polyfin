package hls

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

// A file converted whole copies every track but the video, converted to
// HEVC at the source's size with the settings' quality and deinterlacing,
// on the GPU chosen or with libx265; the source's bitrate caps the copy's.
func TestAFileConversionKeepsItsTracks(t *testing.T) {
	software := FileConversion{Input: "in.ts", Output: "out.mkv", Video: VideoEncoding{Encoder: "libx265", Width: 720, Height: 576,
		FrameRate: 50, Deinterlace: true, Deinterlacer: "bwdif", DoubleRate: true, Bitrate: 6_000_000}}.args()
	for _, want := range [][]string{
		{"-i", "in.ts", "-map", "0", "-c", "copy", "-c:v", "libx265", "-vf", "bwdif=1,scale=w=720:h=576,format=yuv420p"},
		{"-preset", "veryfast", "-crf", "28"},
		{"-maxrate", "6000000", "-bufsize", "12000000"},
		{"-profile:v", "main", "-f", "matroska", "out.mkv"},
	} {
		if !contains(software, want...) {
			t.Errorf("libx265: %q lacks %q", software, want)
		}
	}
	if slices.Contains(software, "-hwaccel") || slices.Contains(software, "-force_key_frames") {
		t.Errorf("libx265: %q", software)
	}
	nvidia := &Hardware{Method: "cuda", Encoders: []string{"h264_nvenc", "hevc_nvenc"}, Resident: true, Deinterlacers: []string{"yadif_cuda", "bwdif_cuda"}}
	gpu := FileConversion{Input: "in.mkv", Output: "out.mkv", Video: VideoEncoding{Encoder: "hevc_nvenc", Width: 1920, Height: 1080,
		FrameRate: 25, Quality: 24, Preset: "slow", Hardware: nvidia}}.args()
	for _, want := range [][]string{
		{"-hwaccel", "cuda"},
		{"-c:v", "hevc_nvenc"},
		{"-preset", "p5", "-rc", "vbr", "-cq", "24", "-b:v", "0"},
	} {
		if !contains(gpu, want...) {
			t.Errorf("NVENC: %q lacks %q", gpu, want)
		}
	}
	if slices.Contains(gpu, "-maxrate") {
		t.Errorf("NVENC capped without a known bitrate: %q", gpu)
	}
	vaapi := &Hardware{Method: "vaapi", Device: "/dev/dri/renderD128", Encoders: []string{"h264_vaapi", "hevc_vaapi"}}
	intel := FileConversion{Input: "in.mkv", Output: "out.mkv", Video: VideoEncoding{Encoder: "hevc_vaapi", Width: 1280, Height: 720,
		FrameRate: 50, Hardware: vaapi}}.args()
	if !contains(intel, "-rc_mode", "CQP", "-qp", "28") || !strings.HasSuffix(intel[slices.Index(intel, "-vf")+1], "hwupload") {
		t.Errorf("VAAPI: %q", intel)
	}
}

// A file conversion counts as one of the conversions at once, and waits
// while playbacks fill them.
func TestAFileConversionWaitsForAFreeConversion(t *testing.T) {
	m := &Manager{done: make(chan struct{}), encodings: map[Key]*encoding{}, slotWait: 10 * time.Millisecond}
	m.LimitConversions(func() int { return 1 })
	playback := Key{Session: "s", User: "u", Version: "v", Converts: true}
	m.mu.Lock()
	m.encodings[playback] = nil
	m.mu.Unlock()
	took := make(chan error, 1)
	go func() { took <- m.takeFileSlot(t.Context()) }()
	// The playback holds the only conversion: the file waits.
	select {
	case err := <-took:
		t.Fatalf("started beside a playback filling the conversions: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	m.mu.Lock()
	delete(m.encodings, playback)
	m.mu.Unlock()
	if err := <-took; err != nil {
		t.Fatal(err)
	}
	if running, limit := m.Conversions(); running != 1 || limit != 1 {
		t.Errorf("conversions: %d of %d", running, limit)
	}
	m.mu.Lock()
	admits := m.admits(playback, nil)
	m.mu.Unlock()
	if admits {
		t.Error("a playback started beside a file conversion filling the conversions")
	}
	// One waiting stops with its context.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := m.takeFileSlot(ctx); err == nil {
		t.Error("a cancelled wait took a conversion")
	}
}
