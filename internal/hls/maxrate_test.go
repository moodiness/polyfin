package hls

import (
	"strconv"
	"testing"
	"time"
)

// Every encoder holds the video to its MaxRate, with a buffer of twice its
// bitrate, and to one and a half times its bitrate without one, with a
// quality factor too.
func TestEveryEncoderPeaksAtItsMaxRate(t *testing.T) {
	plan := NewPlan(seconds(0, 6, 12), 18*time.Second)
	gpu := map[string]*Hardware{
		"nvenc": {Method: "cuda", Encoders: []string{"h264_nvenc", "hevc_nvenc"}},
		"vaapi": {Method: "vaapi", Device: "/dev/dri/renderD128", Encoders: []string{"h264_vaapi", "hevc_vaapi"}, QVBR: true},
		"qsv":   {Method: "qsv", Encoders: []string{"h264_qsv", "hevc_qsv"}},
		"vt":    {Method: "videotoolbox", Encoders: []string{"h264_videotoolbox", "hevc_videotoolbox"}},
		"amf":   {Method: "amf", Encoders: []string{"h264_amf", "hevc_amf"}},
	}
	for _, tc := range []struct {
		encoder string
		gpu     *Hardware
	}{
		{"libx264", nil}, {"libx265", nil}, {"h264_nvenc", gpu["nvenc"]}, {"hevc_nvenc", gpu["nvenc"]}, {"h264_vaapi", gpu["vaapi"]},
		{"hevc_vaapi", gpu["vaapi"]}, {"h264_qsv", gpu["qsv"]}, {"hevc_qsv", gpu["qsv"]}, {"h264_videotoolbox", gpu["vt"]},
		{"hevc_videotoolbox", gpu["vt"]}, {"h264_amf", gpu["amf"]}, {"hevc_amf", gpu["amf"]},
	} {
		for _, maxRate := range []int64{0, 2_616_000} {
			for _, quality := range []int{0, 23} {
				e := VideoEncoding{Encoder: tc.encoder, Level: "4.1", Width: 1280, Height: 720, FrameRate: 24, Bitrate: 2_616_000, MaxRate: maxRate,
					Quality: quality, Hardware: tc.gpu}
				want := strconv.FormatInt(maxRate, 10)
				if maxRate == 0 {
					want = "3924000"
				}
				if args := e.args(plan, 0); !contains(args, "-maxrate", want, "-bufsize", "5232000") {
					t.Errorf("%s, max rate %d, quality %d: %q", tc.encoder, maxRate, quality, args)
				}
			}
		}
	}
}
