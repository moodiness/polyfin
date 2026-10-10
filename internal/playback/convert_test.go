package playback

import (
	"testing"

	"github.com/moodiness/polyfin/internal/hls"
)

// sizeless is a conversion without what set its size, for the tests about
// the rest; nil stays nil.
func sizeless(v *VideoConversion) *VideoConversion {
	if v == nil {
		return nil
	}
	c := *v
	c.Size = ConversionSize{}
	return &c
}

// A conversion records what set its size: the lowest cap below the
// source's, the settings before the server's own caps on a tie, the user's
// group before the settings' maximum, and the bitrate when it allows less
// than all of them.
func TestAConversionTellsWhatSetItsSize(t *testing.T) {
	software := Capabilities{Encoders: []string{"libx264"}, ToneMapping: true}
	nvidia := software
	nvidia.Hardware = &hls.Hardware{Method: "cuda", Encoders: []string{"h264_nvenc"}, ToneMapping: true}
	sdr := func(width, height int) MediaStream {
		return MediaStream{Codec: "mpeg2video", VideoRange: "SDR", Width: new(width), Height: new(height)}
	}
	hdr := MediaStream{Codec: "hevc", VideoRange: "HDR", VideoRangeType: "HDR10", Width: new(3840), Height: new(1600)}
	for _, test := range []struct {
		name   string
		can    Capabilities
		limits Limits
		video  MediaStream
		want   ConversionSize
		height int
	}{
		{"a source smaller than every cap", software, Limits{Group: 1080, Server: 1080}, sdr(1280, 720), ConversionSize{Reason: SizeSource}, 720},
		{"a source the bitrate's rung holds", software, Limits{Bitrate: 4_000_000, Video: 4_000_000}, sdr(1280, 720), ConversionSize{Reason: SizeSource}, 720},
		{"the GPU's 4K", nvidia, Limits{}, sdr(7680, 4320), ConversionSize{Reason: SizeGPU, MaxHeight: 2160}, 2160},
		{"the processor's 1080p", software, Limits{}, sdr(3840, 2160), ConversionSize{Reason: SizeProcessor, MaxHeight: 1080}, 1080},
		{"a size unknown", software, Limits{}, MediaStream{Codec: "mpeg2video", VideoRange: "SDR"}, ConversionSize{Reason: SizeProcessor, MaxHeight: 1080}, 1080},
		{"HDR on the processor", software, Limits{}, hdr, ConversionSize{Reason: SizeToneMapping, MaxHeight: 720}, 532},
		{"HDR on a GPU that tone maps", nvidia, Limits{}, hdr, ConversionSize{Reason: SizeSource}, 1600},
		{"a quality group", software, Limits{Group: 720}, sdr(1920, 1080), ConversionSize{Reason: SizeQualityGroup, MaxHeight: 720}, 720},
		{"the server's maximum", software, Limits{Server: 480}, sdr(1920, 1080), ConversionSize{Reason: SizeServerMax, MaxHeight: 480}, 480},
		{"the app's bitrate", software, Limits{Bitrate: 6_000_000, Video: 5_600_000}, sdr(1920, 1080),
			ConversionSize{Reason: SizeBitrate, MaxHeight: 720, Bitrate: 6_000_000}, 720},
		{"the user's bitrate", software, Limits{Bitrate: 2_000_000, Video: 1_800_000, ByUser: true}, sdr(1920, 1080),
			ConversionSize{Reason: SizeBitrate, MaxHeight: 540, Bitrate: 2_000_000, ByUser: true}, 540},
		// Ties.
		{"a group at the server's maximum", software, Limits{Group: 720, Server: 720}, sdr(1920, 1080), ConversionSize{Reason: SizeQualityGroup, MaxHeight: 720}, 720},
		{"a group at the tone mapping's cap", software, Limits{Group: 720}, hdr, ConversionSize{Reason: SizeQualityGroup, MaxHeight: 720}, 532},
		{"the server's maximum at the processor's", software, Limits{Server: 1080}, sdr(3840, 2160), ConversionSize{Reason: SizeServerMax, MaxHeight: 1080}, 1080},
		{"a bitrate allowing the group's height", software, Limits{Bitrate: 4_000_000, Video: 4_000_000, Group: 720}, sdr(1920, 1080),
			ConversionSize{Reason: SizeQualityGroup, MaxHeight: 720}, 720},
		// A group above the processor's cap does not set the size.
		{"a group above the processor's cap", software, Limits{Group: 1440}, sdr(3840, 2160), ConversionSize{Reason: SizeProcessor, MaxHeight: 1080}, 1080},
	} {
		got := ConvertVideo("h264", test.limits, test.video, test.can)
		if got == nil || got.Size != test.want || got.Height != test.height {
			t.Errorf("%s: %+v, want %+v at %dp", test.name, got, test.want, test.height)
		}
	}
}

// A decision tells whose the bitrate limit is: the user's when the app's
// was not lower.
func TestADecisionTellsWhoseBitrateLimitSetTheSize(t *testing.T) {
	source := MediaSource{Container: "mkv", Bitrate: 30_000_000, Streams: []MediaStream{
		{Type: "Video", Index: 0, Codec: "h264", Profile: "High", Level: new(41.0), VideoRange: "SDR", VideoRangeType: "SDR",
			Width: new(1920), Height: new(1080), AverageFrameRate: new(24.0), BitRate: new(int64(29_800_000))},
		{Type: "Audio", Index: 1, Codec: "aac", IsDefault: true, Channels: new(2), BitRate: new(int64(200_000))},
	}}
	profile := readDeviceProfile(t, "swiftfin-native")
	for _, test := range []struct {
		name string
		user int64
		want ConversionSize
	}{
		{"no user limit", 0, ConversionSize{Reason: SizeBitrate, MaxHeight: 540, Bitrate: 2_000_000}},
		{"a user limit above the app's", 3_000_000, ConversionSize{Reason: SizeBitrate, MaxHeight: 540, Bitrate: 2_000_000}},
		{"the user's limit", 2_000_000, ConversionSize{Reason: SizeBitrate, MaxHeight: 540, Bitrate: 2_000_000, ByUser: true}},
	} {
		options := Options{MaxStreamingBitrate: 2_000_000, UserBitrate: test.user, EnableDirectPlay: true, EnableDirectStream: true,
			Can: Capabilities{Encoders: []string{"libx264"}}}
		got := Decide(profile, source, options)
		if !got.HLS || got.Video == nil || got.Video.Size != test.want {
			t.Errorf("%s: HLS %v, video %+v, want %+v", test.name, got.HLS, got.Video, test.want)
		}
	}
}
