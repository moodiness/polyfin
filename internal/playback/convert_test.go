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

// HDR the processor tone maps is capped at the height in force, where it
// is the lowest cap: 720p when unknown, and never above the software
// encoder's 1080p.
func TestTheToneMappedCapFollowsTheCapability(t *testing.T) {
	amd := &hls.Hardware{Method: "vaapi", Device: "/dev/dri/renderD128", Encoders: []string{"h264_vaapi"}}
	hdr := MediaStream{Codec: "hevc", VideoRange: "HDR", VideoRangeType: "HDR10", ColorTransfer: "smpte2084", Width: new(3840), Height: new(1600)}
	for _, test := range []struct {
		name   string
		gpu    *hls.Hardware
		height int
		want   ConversionSize
		frame  int
	}{
		{"unknown, in software", nil, 0, ConversionSize{Reason: SizeToneMapping, MaxHeight: 720}, 532},
		{"720p, in software", nil, 720, ConversionSize{Reason: SizeToneMapping, MaxHeight: 720}, 532},
		{"1080p, in software", nil, 1080, ConversionSize{Reason: SizeProcessor, MaxHeight: 1080}, 800},
		{"4K, in software", nil, 2160, ConversionSize{Reason: SizeProcessor, MaxHeight: 1080}, 800},
		{"1080p, encoded by a GPU", amd, 1080, ConversionSize{Reason: SizeToneMapping, MaxHeight: 1080}, 800},
		{"1440p, encoded by a GPU", amd, 1440, ConversionSize{Reason: SizeToneMapping, MaxHeight: 1440}, 1066},
		{"4K, encoded by a GPU", amd, 2160, ConversionSize{Reason: SizeSource}, 1600},
	} {
		can := Capabilities{Encoders: []string{"libx264"}, ToneMapping: true, Hardware: test.gpu, ToneMappedHeight: test.height}
		got := ConvertVideo("h264", Limits{}, hdr, can)
		if got == nil || got.Size != test.want || got.Height != test.frame || !got.ToneMap || got.ToneMapOnCPU {
			t.Errorf("%s: %+v, want %+v at %dp", test.name, got, test.want, test.frame)
		}
	}
}

// An Intel GPU tone maps only HDR10 whose frames carry their mastering
// display; HLG, Dolby Vision profile 5 and a mastering display unknown or
// missing go to the processor, profile 5 then not at all. With tone
// mapping on the graphics card turned off, NVIDIA's and Intel's GPUs both
// leave HDR to the processor, and profile 5 is refused.
func TestWhereHDRIsToneMapped(t *testing.T) {
	nvidia := &hls.Hardware{Method: "cuda", Encoders: []string{"h264_nvenc"}, ToneMapping: true}
	intel := &hls.Hardware{Method: "vaapi", Device: "/dev/dri/renderD128", Encoders: []string{"h264_vaapi"}, ToneMapping: true}
	hdr10 := func(mastering *bool) MediaStream {
		return MediaStream{Codec: "hevc", VideoRange: "HDR", VideoRangeType: "HDR10", ColorTransfer: "smpte2084", Width: new(3840), Height: new(2160),
			MasteringDisplay: mastering}
	}
	hlg := MediaStream{Codec: "hevc", VideoRange: "HDR", VideoRangeType: "HLG", ColorTransfer: "arib-std-b67", Width: new(3840), Height: new(2160),
		MasteringDisplay: new(true)}
	profile5 := MediaStream{Codec: "hevc", VideoRange: "HDR", VideoRangeType: "DOVI", Width: new(3840), Height: new(2160), MasteringDisplay: new(true)}
	const gpu, cpu, refused = "gpu", "cpu", "refused"
	for _, test := range []struct {
		name  string
		hw    *hls.Hardware
		off   bool
		video MediaStream
		want  string
	}{
		{"Intel, HDR10 mastered", intel, false, hdr10(new(true)), gpu},
		{"Intel, HDR10 without its mastering display", intel, false, hdr10(new(false)), cpu},
		{"Intel, HDR10 not probed yet", intel, false, hdr10(nil), cpu},
		{"Intel, HLG", intel, false, hlg, cpu},
		{"Intel, Dolby Vision profile 5", intel, false, profile5, refused},
		{"NVIDIA, HDR10 not probed", nvidia, false, hdr10(nil), gpu},
		{"NVIDIA, HLG", nvidia, false, hlg, gpu},
		{"NVIDIA, Dolby Vision profile 5", nvidia, false, profile5, gpu},
		{"Intel, turned off", intel, true, hdr10(new(true)), cpu},
		{"NVIDIA, turned off", nvidia, true, hdr10(nil), cpu},
		{"NVIDIA, turned off, Dolby Vision profile 5", nvidia, true, profile5, refused},
	} {
		can := Capabilities{Encoders: []string{"libx264"}, ToneMapping: true, Hardware: test.hw, Tuning: Tuning{CPUToneMapping: test.off}}
		got := ConvertVideo("h264", Limits{}, test.video, can)
		switch {
		case test.want == refused:
			if got != nil {
				t.Errorf("%s: converted %+v", test.name, got)
			}
		case got == nil || !got.ToneMap || got.Hardware != test.hw:
			t.Errorf("%s: %+v", test.name, got)
		case test.want == gpu && (got.ToneMapOnCPU || got.Height != 2160):
			t.Errorf("%s: not on the GPU: %+v", test.name, got)
		case test.want == cpu && (!got.ToneMapOnCPU || got.Size != ConversionSize{Reason: SizeToneMapping, MaxHeight: 720}):
			t.Errorf("%s: not on the processor: %+v", test.name, got)
		}
	}
}
