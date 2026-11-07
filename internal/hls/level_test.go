package hls

import (
	"slices"
	"testing"
	"time"
)

// Every encoder is told the frame rate, maximum rate and buffer its level
// holds, and no tier: HEVC stays at its Main tier.
func TestEncoderIsToldWhatItsLevelHolds(t *testing.T) {
	t.Parallel()
	plan := NewPlan([]time.Duration{0, 6 * time.Second}, 12*time.Second)
	gpu := func(method string, encoders ...string) *Hardware {
		return &Hardware{Method: method, Device: "/dev/dri/renderD128", Encoders: encoders}
	}
	for _, tc := range []struct {
		encoder  string
		hardware *Hardware
	}{
		{"hevc_nvenc", gpu("cuda", "hevc_nvenc")},
		{"hevc_vaapi", gpu("vaapi", "hevc_vaapi")},
		{"hevc_amf", gpu("amf", "hevc_amf")},
		{"libx265", nil},
		{"hevc_qsv", gpu("qsv", "hevc_qsv")},
		{"hevc_videotoolbox", gpu("videotoolbox", "hevc_videotoolbox")},
		{"h264_nvenc", gpu("cuda", "h264_nvenc")},
	} {
		e := &VideoEncoding{Encoder: tc.encoder, Hardware: tc.hardware, Level: "5.1", Width: 3840, Height: 2160,
			Bitrate: 26_000_000, MaxRate: 39_000_000, BufSize: 40_000_000, FrameRate: 24000.0 / 1001, PinRate: true}
		args := Remux{Input: "http://127.0.0.1:1/a.mkv", Video: 0, Audio: 1, Format: FMP4, Plan: plan, Encode: e}.args(0, 0)
		if !contains(args, "-maxrate", "39000000", "-bufsize", "40000000") || !contains(args, "-r", "23.976023976023978") ||
			slices.Contains(args, "-tier") || slices.Contains(args, "-profile_tier") {
			t.Errorf("%s: %q", tc.encoder, args)
		}
	}
}
