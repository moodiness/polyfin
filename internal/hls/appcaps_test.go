package hls

import (
	"slices"
	"testing"
	"time"
)

// FFmpeg writes the H.264 profile an app's profile allows, under the name
// each encoder gives it, and brings the video down to the frame rate it
// allows; HEVC stays Main.
func TestEncoderWritesTheAppsProfileAndFrameRate(t *testing.T) {
	t.Parallel()
	plan := NewPlan([]time.Duration{0, 6 * time.Second}, 12*time.Second)
	vaapi := &Hardware{Method: "vaapi", Device: "/dev/dri/renderD128", Encoders: []string{"h264_vaapi", "hevc_vaapi"}}
	videoToolbox := &Hardware{Method: "videotoolbox", Encoders: []string{"h264_videotoolbox", "hevc_videotoolbox"}}
	for _, test := range []struct {
		encoder  string
		hardware *Hardware
		profile  string
		drop     bool
		want     []string
	}{
		{"libx264", nil, "", false, []string{"-profile:v", "high"}},
		{"libx264", nil, "main", false, []string{"-profile:v", "main"}},
		{"libx264", nil, "baseline", false, []string{"-profile:v", "baseline"}},
		{"h264_vaapi", vaapi, "baseline", false, []string{"-profile:v", "constrained_baseline"}},
		{"h264_videotoolbox", videoToolbox, "baseline", false, []string{"-profile:v", "constrained_baseline"}},
		{"libx265", nil, "baseline", false, []string{"-profile:v", "main"}},
		{"libx264", nil, "", true, []string{"-r", "30"}},
	} {
		encoding := &VideoEncoding{Encoder: test.encoder, Hardware: test.hardware, Level: "4.1", Width: 1920, Height: 1080, Bitrate: 8_000_000,
			FrameRate: 30, DropFrames: test.drop, Profile: test.profile}
		args := Remux{Input: "http://127.0.0.1:1/a.mkv", Video: 0, Audio: 1, Format: TS, Plan: plan, Encode: encoding}.args(0, 0)
		at := slices.Index(args, test.want[0])
		if at < 0 || at+1 >= len(args) || args[at+1] != test.want[1] {
			t.Errorf("%s %q, dropping frames %v: %q", test.encoder, test.profile, test.drop, args)
		}
		if !test.drop && slices.Contains(args, "-r") {
			t.Errorf("%s keeping the frame rate: %q", test.encoder, args)
		}
	}
}
