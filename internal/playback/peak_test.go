package playback

import (
	"testing"

	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/media"
)

// peaking is a conversion with its peak under a limit.
func peaking(c *VideoConversion, peak int64) *VideoConversion {
	c.Peak = peak
	return c
}

// Under an app's bitrate limit, converted video peaks at most at what the
// limit leaves once the audio sent is taken off, on every encoder, for the
// first quality and the lower ones: the video's peak and the audio stay
// within the limit. Where the limit leaves more, and with no limit, the
// video peaks at one and a half times its bitrate, as before.
func TestConvertedVideoPeaksWithinTheLimit(t *testing.T) {
	video := MediaStream{Codec: "hevc", BitDepth: new(10), BitRate: new(int64(15_000_000))}
	source := bitrateSource(video, dts(6))
	stream := media.Stream{Codec: "hevc", BitDepth: 10, Width: 1920, Height: 1080, AverageRate: 24}
	gpu := func(method string, encoders ...string) *hls.Hardware {
		return &hls.Hardware{Method: method, Device: "/dev/dri/renderD128", Encoders: encoders}
	}
	for _, tc := range []struct {
		name, codec string
		can         Capabilities
	}{
		{"x264", "h264", Capabilities{Encoders: []string{"libx264"}}},
		{"x264 with a quality factor", "h264", Capabilities{Encoders: []string{"libx264"}, Tuning: Tuning{H264Quality: 23}}},
		{"x265", "hevc", Capabilities{Encoders: []string{"libx265"}}},
		{"NVENC", "h264", Capabilities{Hardware: gpu("cuda", "h264_nvenc", "hevc_nvenc")}},
		{"VAAPI", "hevc", Capabilities{Hardware: gpu("vaapi", "h264_vaapi", "hevc_vaapi")}},
		{"QSV", "h264", Capabilities{Hardware: gpu("qsv", "h264_qsv", "hevc_qsv")}},
		{"VideoToolbox", "h264", Capabilities{Hardware: gpu("videotoolbox", "h264_videotoolbox", "hevc_videotoolbox")}},
		{"AMF", "hevc", Capabilities{Hardware: gpu("amf", "h264_amf", "hevc_amf")}},
	} {
		profile := readDeviceProfile(t, "minimal")
		profile.TranscodingProfiles[0].VideoCodec = tc.codec
		for _, limit := range []int64{3_000_000, 12_000_000, 120_000_000, 0} {
			d := Decide(profile, source, Options{MaxStreamingBitrate: limit, EnableDirectPlay: true, EnableDirectStream: true, ConvertVideo: true,
				Can: tc.can})
			if d.Video == nil || d.Audio == nil {
				t.Fatalf("%s within %d: %+v", tc.name, limit, d)
			}
			remux := Remux{Format: hls.FMP4, ConvertVideo: d.Video, ConvertAudio: d.Audio}
			for variant := 0; variant <= len(d.Video.Steps(1920, 1080)); variant++ {
				remux.Variant = variant
				r := hls.Remux{Video: 0, Audio: 1, Format: hls.FMP4}
				remux.convert(&r, stream, tc.can.Tuning)
				e := r.Encode
				if e == nil {
					t.Fatalf("%s within %d, quality %d: not converted", tc.name, limit, variant)
				}
				peak := e.MaxRate
				if peak == 0 {
					peak = e.Bitrate * 3 / 2
				}
				switch {
				case limit > 0 && peak+r.AudioBitrate > limit:
					t.Errorf("%s within %d, quality %d: %s peaks at %d with %d of audio", tc.name, limit, variant, e.Encoder, peak, r.AudioBitrate)
				case limit-r.AudioBitrate >= e.Bitrate*3/2 && peak != e.Bitrate*3/2 || limit == 0 && peak != e.Bitrate*3/2:
					t.Errorf("%s within %d, quality %d: peaks at %d for %d", tc.name, limit, variant, peak, e.Bitrate)
				}
			}
		}
	}
}
