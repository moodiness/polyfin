package playback

import (
	"encoding/json"
	"os"
	"path"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/hls"
)

const playbackFixtures = "../jellyfin/testdata/jellyfin-12.1/playback/"

// recordedDecision is an entry of decisions.json: Jellyfin 12.1's answer
// for one clip, one DeviceProfile and one request.
type recordedDecision struct {
	Media   string
	Profile string
	Options struct {
		MaxStreamingBitrate int64
		AudioStreamIndex    *int
		SubtitleStreamIndex *int
		EnableDirectPlay    *bool
		EnableDirectStream  *bool
	}
	Result struct {
		SupportsDirectPlay         bool
		SupportsDirectStream       bool
		SupportsTranscoding        bool
		TranscodeReasons           []string
		TranscodingSubProtocol     string
		TranscodingContainer       *string
		Container                  string
		Bitrate                    int64
		DefaultAudioStreamIndex    *int
		DefaultSubtitleStreamIndex *int
	}
	MediaStreams []MediaStream
}

func readDeviceProfile(t *testing.T, name string) *DeviceProfile {
	t.Helper()
	data, err := os.ReadFile(playbackFixtures + "profiles/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var profile DeviceProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return &profile
}

func TestDeviceProfilesDecode(t *testing.T) {
	profile := readDeviceProfile(t, "jellyfin-web-chrome")
	video := profile.TranscodingProfiles[9]
	if video.Container != "mp4" || video.Protocol != "hls" || video.MinSegments != "2" || !video.BreakOnNonKeyFrames {
		t.Errorf("jellyfin-web's video transcoding profile = %+v", video)
	}
	if got := readDeviceProfile(t, "swiftfin").TranscodingProfiles[0].MinSegments; got != "2" {
		t.Errorf("Swiftfin's numeric MinSegments = %q", got)
	}
}

func TestDecisionsMatchJellyfin(t *testing.T) {
	data, err := os.ReadFile(playbackFixtures + "decisions.json")
	if err != nil {
		t.Fatal(err)
	}
	var entries []recordedDecision
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	profiles := map[string]*DeviceProfile{}
	for _, entry := range entries {
		if profiles[entry.Profile] == nil {
			profiles[entry.Profile] = readDeviceProfile(t, entry.Profile)
		}
	}
	// The source container is recorded where a profile without direct-play
	// profiles leaves it untouched (Findroid's).
	containers := map[string]string{}
	for _, entry := range entries {
		if len(profiles[entry.Profile].DirectPlayProfiles) == 0 {
			containers[entry.Media] = entry.Result.Container
		}
	}

	for i, entry := range entries {
		t.Run(entry.Media+"/"+entry.Profile, func(t *testing.T) {
			container, ok := containers[entry.Media]
			if !ok {
				t.Fatalf("entry %d: no recorded container for %s", i, entry.Media)
			}
			options := Options{
				MaxStreamingBitrate: entry.Options.MaxStreamingBitrate,
				AudioStreamIndex:    entry.Options.AudioStreamIndex,
				SubtitleStreamIndex: entry.Options.SubtitleStreamIndex,
				EnableDirectPlay:    entry.Options.EnableDirectPlay == nil || *entry.Options.EnableDirectPlay,
				EnableDirectStream:  entry.Options.EnableDirectStream == nil || *entry.Options.EnableDirectStream,
			}
			// Without a requested subtitle, Jellyfin applied its own default
			// (SubtitleMode Default); Polyfin picks its default outside the
			// decision, so the recorded choice is passed in.
			if options.SubtitleStreamIndex == nil {
				options.SubtitleStreamIndex = entry.Result.DefaultSubtitleStreamIndex
			}
			got := Decide(profiles[entry.Profile], MediaSource{Container: container, Bitrate: entry.Result.Bitrate, Streams: entry.MediaStreams}, options)

			want := entry.Result
			if want.SupportsDirectPlay != want.SupportsDirectStream {
				t.Fatalf("entry %d: SupportsDirectPlay %v differs from SupportsDirectStream %v", i, want.SupportsDirectPlay, want.SupportsDirectStream)
			}
			if got.DirectPlay != want.SupportsDirectPlay {
				t.Errorf("entry %d: DirectPlay = %v, want %v", i, got.DirectPlay, want.SupportsDirectPlay)
			}
			if !slices.Equal(got.Reasons, want.TranscodeReasons) {
				t.Errorf("entry %d: Reasons = %v, want %v", i, got.Reasons, want.TranscodeReasons)
			}
			if got.Container != want.Container {
				t.Errorf("entry %d: Container = %q, want %q", i, got.Container, want.Container)
			}
			if index := orNone(want.DefaultAudioStreamIndex); got.AudioStreamIndex != index {
				t.Errorf("entry %d: AudioStreamIndex = %d, want %d", i, got.AudioStreamIndex, index)
			}
			if index := orNone(want.DefaultSubtitleStreamIndex); got.SubtitleStreamIndex != index {
				t.Errorf("entry %d: SubtitleStreamIndex = %d, want %d", i, got.SubtitleStreamIndex, index)
			}
			if (got.Transcoding != nil) != want.SupportsTranscoding {
				t.Errorf("entry %d: Transcoding = %+v, want SupportsTranscoding %v", i, got.Transcoding, want.SupportsTranscoding)
			}
			protocol, transcodingContainer := "http", ""
			if !got.DirectPlay && got.Transcoding != nil {
				protocol, transcodingContainer = got.Transcoding.Protocol, got.Transcoding.Container
			}
			if protocol != want.TranscodingSubProtocol || transcodingContainer != orEmpty(want.TranscodingContainer) {
				t.Errorf("entry %d: transcoding %s/%q, want %s/%q", i, protocol, transcodingContainer, want.TranscodingSubProtocol, orEmpty(want.TranscodingContainer))
			}
			for _, stream := range entry.MediaStreams {
				if stream.Type != "Subtitle" {
					continue
				}
				delivery := got.Subtitles[stream.Index]
				if delivery.Method != stream.DeliveryMethod {
					t.Errorf("entry %d: subtitle %d delivered by %q, want %q", i, stream.Index, delivery.Method, stream.DeliveryMethod)
				}
				if stream.DeliveryMethod != "External" {
					continue
				}
				file, _, _ := strings.Cut(path.Base(stream.DeliveryUrl), "?")
				if want := strings.TrimPrefix(path.Ext(file), "."); delivery.Format != want {
					t.Errorf("entry %d: subtitle %d format %q, want %q (%s)", i, stream.Index, delivery.Format, want, stream.DeliveryUrl)
				}
			}
		})
	}
}

func orNone(index *int) int {
	if index == nil {
		return -1
	}
	return *index
}

func orEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func TestDecideConditions(t *testing.T) {
	source := MediaSource{
		Container: "mkv",
		Bitrate:   1_000_000,
		Streams: []MediaStream{
			{Type: "Video", Index: 0, Codec: "hevc", Profile: "Main 10", Level: new(150.0), VideoRangeType: "HDR10", Width: new(3840), Height: new(2160)},
			{Type: "Audio", Index: 1, Codec: "aac", IsDefault: true, Channels: new(6)},
		},
	}
	video := func(conditions, apply []ProfileCondition) []CodecProfile {
		return []CodecProfile{{Type: "Video", Codec: "hevc", Conditions: conditions, ApplyConditions: apply}}
	}
	condition := func(kind, property, value string, required bool) ProfileCondition {
		return ProfileCondition{Condition: kind, Property: property, Value: value, IsRequired: required}
	}
	tests := []struct {
		name    string
		profile []CodecProfile
		reasons []string
	}{
		{"required unknown property fails", video([]ProfileCondition{condition("EqualsAny", "VideoCodecTag", "hvc1|dvh1", true)}, nil), []string{"VideoCodecTagNotSupported"}},
		{"optional unknown property passes", video([]ProfileCondition{condition("EqualsAny", "VideoCodecTag", "hvc1|dvh1", false)}, nil), nil},
		{"required unknown bit depth fails", video([]ProfileCondition{condition("LessThanEqual", "VideoBitDepth", "10", true)}, nil), []string{"VideoBitDepthNotSupported"}},
		{"EqualsAny ignores case", video([]ProfileCondition{condition("EqualsAny", "VideoProfile", "main|main 10", false)}, nil), nil},
		{"EqualsAny without the value fails", video([]ProfileCondition{condition("EqualsAny", "VideoProfile", "main|rext", false)}, nil), []string{"VideoProfileNotSupported"}},
		{"EqualsAny on numbers", video([]ProfileCondition{condition("EqualsAny", "Width", "1920|3840", false)}, nil), nil},
		{"NotEquals takes a list", video([]ProfileCondition{condition("NotEquals", "VideoRangeType", "DOVI|HDR10", false)}, nil), []string{"VideoRangeTypeNotSupported"}},
		{
			"unmet ApplyConditions skip the profile",
			video([]ProfileCondition{condition("LessThanEqual", "VideoLevel", "120", false)}, []ProfileCondition{condition("Equals", "VideoProfile", "main", false)}),
			nil,
		},
		{
			"met ApplyConditions apply the profile",
			video([]ProfileCondition{condition("LessThanEqual", "VideoLevel", "120", false), condition("LessThanEqual", "Height", "1080", false)}, []ProfileCondition{condition("Equals", "VideoProfile", "main 10", false)}),
			[]string{"VideoLevelNotSupported", "VideoResolutionNotSupported"},
		},
		{
			"VideoAudio conditions see the audio",
			[]CodecProfile{{Type: "VideoAudio", Codec: "aac", Conditions: []ProfileCondition{condition("LessThanEqual", "AudioChannels", "2", false)}}},
			[]string{"AudioChannelsNotSupported"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile := &DeviceProfile{
				DirectPlayProfiles:  []DirectPlayProfile{{Type: "Video", Container: "mkv", VideoCodec: "hevc", AudioCodec: "aac"}},
				TranscodingProfiles: []TranscodingProfile{{Type: "Video", Container: "ts", Protocol: "hls", VideoCodec: "hevc", AudioCodec: "aac"}},
				CodecProfiles:       test.profile,
			}
			got := Decide(profile, source, Options{EnableDirectPlay: true, EnableDirectStream: true})
			if got.DirectPlay != (test.reasons == nil) || !slices.Equal(got.Reasons, test.reasons) {
				t.Errorf("DirectPlay %v, Reasons %v; want reasons %v", got.DirectPlay, got.Reasons, test.reasons)
			}
		})
	}
}

func TestRemuxesCopyTheVideoAndConvertWhatAudioTheAppCannotTake(t *testing.T) {
	// A Matroska remux: HEVC with Dolby Vision over HDR10, a French
	// E-AC-3 track flagged default, then English AAC in 5.1.
	source := MediaSource{
		Container: "mkv",
		Bitrate:   60_000_000,
		Streams: []MediaStream{
			{Type: "Video", Index: 0, Codec: "hevc", Profile: "Main 10", Level: new(153.0), VideoRangeType: "DOVIWithHDR10",
				Width: new(3840), Height: new(2160), AverageFrameRate: new(23.976)},
			{Type: "Audio", Index: 1, Codec: "eac3", IsDefault: true, Channels: new(6)},
			{Type: "Audio", Index: 2, Codec: "aac", Channels: new(6)},
		},
	}
	sdr := source
	sdr.Streams = slices.Clone(source.Streams)
	sdr.Streams[0].VideoRangeType = "SDR"
	stereo := &AudioConversion{Codec: "aac", Channels: 2, Bitrate: 192_000}
	tests := []struct {
		name    string
		profile string
		source  MediaSource
		options Options
		remux   bool
		audio   *AudioConversion
		reasons []string
	}{
		// Its native player takes HEVC in MP4 tagged hvc1 only, which a
		// remux writes, and E-AC-3.
		{"Swiftfin's native player", "swiftfin-native", source, Options{}, true, nil, []string{"ContainerNotSupported", "VideoCodecTagNotSupported"}},
		// An app may ask for the audio to be converted all the same.
		{"audio copy refused", "swiftfin-native", source, Options{ConvertAudio: true}, true, &AudioConversion{Codec: "aac", Channels: 6, Bitrate: 384_000},
			[]string{"ContainerNotSupported", "VideoCodecTagNotSupported"}},
		// Chrome refuses Dolby Vision, which a remux keeps.
		{"Dolby Vision in Chrome", "jellyfin-web-chrome", source, Options{AudioStreamIndex: new(2)}, false, nil,
			[]string{"SecondaryAudioNotSupported", "VideoRangeTypeNotSupported"}},
		// Chrome takes AAC in stereo, and no E-AC-3: both are converted
		// to stereo AAC.
		{"5.1 AAC in Chrome", "jellyfin-web-chrome", sdr, Options{AudioStreamIndex: new(2)}, true, stereo, []string{"SecondaryAudioNotSupported"}},
		{"E-AC-3 in Chrome", "jellyfin-web-chrome", sdr, Options{}, true, stereo, []string{"AudioCodecNotSupported"}},
		// A remux cannot lower the bitrate.
		{"over the bitrate limit", "swiftfin-native", source, Options{MaxStreamingBitrate: 20_000_000}, false, nil,
			[]string{"ContainerBitrateExceedsLimit"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := test.options
			options.EnableDirectPlay, options.EnableDirectStream = true, true
			got := Decide(readDeviceProfile(t, test.profile), test.source, options)
			if got.DirectPlay || got.HLS != test.remux || !reflect.DeepEqual(got.Audio, test.audio) || !slices.Equal(got.Reasons, test.reasons) {
				t.Errorf("DirectPlay %v, Remux %v, Audio %+v, Reasons %v; want remux %v, audio %+v, reasons %v",
					got.DirectPlay, got.HLS, got.Audio, got.Reasons, test.remux, test.audio, test.reasons)
			}
		})
	}
	// With more channels allowed, Chrome takes the AAC track as it is.
	chrome := readDeviceProfile(t, "jellyfin-web-chrome")
	for i := range chrome.TranscodingProfiles {
		chrome.TranscodingProfiles[i].MaxAudioChannels = "6"
	}
	if english := Decide(chrome, sdr, Options{AudioStreamIndex: new(2), EnableDirectPlay: true, EnableDirectStream: true}); !english.HLS || english.Audio != nil {
		t.Errorf("5.1 AAC with 6 channels allowed: remux %v, audio %+v", english.HLS, english.Audio)
	}
}

func TestAudioIsConvertedToTheFirstCodecFFmpegEncodes(t *testing.T) {
	for _, test := range []struct {
		codecs, maxChannels string
		channels            int
		want                *AudioConversion
	}{
		{"aac,mp2,opus,flac", "2", 8, &AudioConversion{Codec: "aac", Channels: 2, Bitrate: 192_000}},
		{"mp3,eac3,aac", "", 8, &AudioConversion{Codec: "eac3", Channels: 6, Bitrate: 384_000}},
		{"FLAC", "8", 8, &AudioConversion{Codec: "flac", Channels: 8}},
		{"aac", "", 1, &AudioConversion{Codec: "aac", Channels: 1, Bitrate: 128_000}},
		{"aac", "", 0, &AudioConversion{Codec: "aac", Channels: 2, Bitrate: 192_000}},
		{"mp2,opus", "2", 6, nil},
	} {
		if got := ConvertAudio(test.codecs, test.maxChannels, test.channels, "", Tuning{}); !reflect.DeepEqual(got, test.want) {
			t.Errorf("ConvertAudio(%q, %q, %d) = %+v, want %+v", test.codecs, test.maxChannels, test.channels, got, test.want)
		}
	}
}

func TestVideoTheAppCannotTakeIsConverted(t *testing.T) {
	// A 4K Dolby Vision remux over HDR10, with E-AC-3 flagged default.
	source := MediaSource{
		Container: "mkv",
		Bitrate:   60_000_000,
		Streams: []MediaStream{
			{Type: "Video", Index: 0, Codec: "hevc", Profile: "Main 10", Level: new(153.0), VideoRange: "HDR", VideoRangeType: "DOVIWithHDR10",
				Width: new(3840), Height: new(2160), AverageFrameRate: new(23.976), BitRate: new(int64(55_000_000))},
			{Type: "Audio", Index: 1, Codec: "eac3", IsDefault: true, Channels: new(6), BitRate: new(int64(640_000))},
		},
	}
	software := Capabilities{Encoders: []string{"libx264", "libx265"}, ToneMapping: true}
	sdr720 := &VideoConversion{Codec: "h264", Encoder: "libx264", Width: 1280, Height: 720, Bitrate: 5_000_000, ToneMap: true}
	stereo := &AudioConversion{Codec: "aac", Channels: 2, Bitrate: 192_000}
	tests := []struct {
		name    string
		profile string
		source  MediaSource
		options Options
		hls     bool
		video   *VideoConversion
		audio   *AudioConversion
	}{
		// Chrome refuses Dolby Vision and E-AC-3: both are converted, the
		// video to SDR H.264, at 720p as it is tone mapped in software.
		{"Dolby Vision in Chrome", "jellyfin-web-chrome", source, Options{Can: software}, true, sdr720, stereo},
		// Without the filters converting HDR to SDR, nothing is offered.
		{"no tone mapping", "jellyfin-web-chrome", source, Options{Can: Capabilities{Encoders: []string{"libx264"}}}, false, nil, nil},
		{"no encoder", "jellyfin-web-chrome", source, Options{}, false, nil, nil},
		// Over a limit, the video takes what the audio leaves: 720p at
		// 3.36 Mb/s within 4 Mb/s.
		{"over the bitrate limit", "swiftfin-native", source, Options{MaxStreamingBitrate: 4_000_000, Can: software}, true,
			&VideoConversion{Codec: "h264", Encoder: "libx264", Width: 1280, Height: 720, Bitrate: 3_360_000, ToneMap: true}, nil},
		// An app may refuse the copy of video it could take.
		{"video copy refused", "swiftfin-native", source, Options{ConvertVideo: true, Can: software}, true, sdr720, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := test.options
			options.EnableDirectPlay, options.EnableDirectStream = true, true
			got := Decide(readDeviceProfile(t, test.profile), test.source, options)
			if got.DirectPlay || got.HLS != test.hls || !reflect.DeepEqual(got.Video, test.video) || !reflect.DeepEqual(got.Audio, test.audio) {
				t.Errorf("HLS %v, Video %+v, Audio %+v; want %v, %+v, %+v", got.HLS, got.Video, got.Audio, test.hls, test.video, test.audio)
			}
		})
	}
	// Dolby Vision with no base layer other players read cannot be
	// converted without the colors going wrong.
	profile5 := source
	profile5.Streams = slices.Clone(source.Streams)
	profile5.Streams[0].VideoRangeType = "DOVI"
	if got := Decide(readDeviceProfile(t, "jellyfin-web-chrome"), profile5, Options{EnableDirectPlay: true, Can: software}); got.HLS {
		t.Errorf("Dolby Vision profile 5 converted: %+v", got.Video)
	}
}

func TestVideoIsConvertedToWhatTheLimitAllows(t *testing.T) {
	can := Capabilities{Encoders: []string{"libx264", "libx265"}, ToneMapping: true}
	video := func(width, height int, bitrate int64, interlaced bool) MediaStream {
		stream := MediaStream{Codec: "mpeg2video", VideoRange: "SDR", Width: new(width), Height: new(height), IsInterlaced: interlaced}
		if bitrate > 0 {
			stream.BitRate = new(bitrate)
		}
		return stream
	}
	for _, test := range []struct {
		name   string
		codecs string
		limit  int64
		video  MediaStream
		want   *VideoConversion
	}{
		{"no limit", "h264", 0, video(3840, 2160, 0, false), &VideoConversion{Codec: "h264", Encoder: "libx264", Width: 1920, Height: 1080, Bitrate: 10_000_000}},
		{"never larger than the source", "h264", 0, video(1280, 720, 0, false), &VideoConversion{Codec: "h264", Encoder: "libx264", Width: 1280, Height: 720, Bitrate: 10_000_000}},
		{"never more than the source's bitrate", "h264", 0, video(1920, 1080, 4_000_000, false), &VideoConversion{Codec: "h264", Encoder: "libx264", Width: 1920, Height: 1080, Bitrate: 4_000_000}},
		{"a low limit", "h264", 1_000_000, video(1920, 1080, 0, false), &VideoConversion{Codec: "h264", Encoder: "libx264", Width: 852, Height: 480, Bitrate: 1_000_000}},
		{"a very low limit", "h264", 100_000, video(1920, 800, 0, true), &VideoConversion{Codec: "h264", Encoder: "libx264", Width: 864, Height: 360, Bitrate: 300_000, Deinterlace: true}},
		{"HDR tone mapped at 720p at most", "h264", 0, MediaStream{Codec: "hevc", VideoRange: "HDR", VideoRangeType: "HDR10", Width: new(3840), Height: new(2160)},
			&VideoConversion{Codec: "h264", Encoder: "libx264", Width: 1280, Height: 720, Bitrate: 5_000_000, ToneMap: true}},
		{"HEVC only", "hevc,mpeg4", 0, video(1920, 1080, 0, false), &VideoConversion{Codec: "hevc", Encoder: "libx265", Width: 1920, Height: 1080, Bitrate: 10_000_000}},
		{"H.264 first, whatever the order", "av1,hevc,h264,vp9", 0, video(1920, 1080, 0, false), &VideoConversion{Codec: "h264", Encoder: "libx264", Width: 1920, Height: 1080, Bitrate: 10_000_000}},
		{"no codec Polyfin encodes", "av1,vp9", 0, video(1920, 1080, 0, false), nil},
	} {
		if got := ConvertVideo(test.codecs, test.limit, 0, test.video, can); !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s: %+v, want %+v", test.name, got, test.want)
		}
	}
	h264 := VideoConversion{Codec: "h264"}
	hevc := VideoConversion{Codec: "hevc"}
	for _, c := range []struct{ got, want string }{
		{h264.CodecString(23.976), "avc1.640029"}, {h264.CodecString(59.94), "avc1.64002A"}, {hevc.CodecString(59.94), "hvc1.1.6.L123.B0"},
	} {
		if c.got != c.want {
			t.Errorf("codec string %s, want %s", c.got, c.want)
		}
	}
}

func TestVideoIsConvertedOnTheGPUThatEncodesTheCodec(t *testing.T) {
	// This GPU encodes H.264 only.
	gpu := &hls.Hardware{Method: "cuda", Encoders: []string{"h264_nvenc"}}
	can := Capabilities{Encoders: []string{"libx264", "libx265"}, ToneMapping: true, Hardware: gpu}
	video := MediaStream{Codec: "mpeg2video", VideoRange: "SDR", Width: new(1920), Height: new(1080)}
	for codecs, want := range map[string]*VideoConversion{
		"h264,hevc": {Codec: "h264", Encoder: "h264_nvenc", Width: 1920, Height: 1080, Bitrate: 10_000_000, Hardware: gpu},
		// HEVC the app needs is encoded in software rather than H.264 on
		// the GPU.
		"hevc": {Codec: "hevc", Encoder: "libx265", Width: 1920, Height: 1080, Bitrate: 10_000_000},
	} {
		if got := ConvertVideo(codecs, 0, 0, video, can); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %+v, want %+v", codecs, got, want)
		}
	}
}

func TestHDRIsToneMappedOnTheGPUWithoutTheProcessorsLimits(t *testing.T) {
	nvidia := &hls.Hardware{Method: "cuda", Encoders: []string{"h264_nvenc"}, ToneMapping: true}
	vaapi := &hls.Hardware{Method: "vaapi", Device: "/dev/dri/renderD128", Encoders: []string{"h264_vaapi"}}
	hdr10 := MediaStream{Codec: "hevc", VideoRange: "HDR", VideoRangeType: "HDR10", Width: new(3840), Height: new(2160)}
	profile5 := MediaStream{Codec: "hevc", VideoRange: "HDR", VideoRangeType: "DOVI", Width: new(3840), Height: new(2160)}
	for _, test := range []struct {
		name   string
		codecs string
		gpu    *hls.Hardware
		video  MediaStream
		want   *VideoConversion
	}{
		{"HDR10 on a GPU that tone maps", "h264", nvidia, hdr10,
			&VideoConversion{Codec: "h264", Encoder: "h264_nvenc", Width: 1920, Height: 1080, Bitrate: 10_000_000, ToneMap: true, Hardware: nvidia}},
		{"Dolby Vision profile 5 on a GPU that tone maps", "h264", nvidia, profile5,
			&VideoConversion{Codec: "h264", Encoder: "h264_nvenc", Width: 1920, Height: 1080, Bitrate: 10_000_000, ToneMap: true, Hardware: nvidia}},
		// HEVC only: encoded in software, so tone mapped on the processor.
		{"HDR10 encoded in software", "hevc", nvidia, hdr10,
			&VideoConversion{Codec: "hevc", Encoder: "libx265", Width: 1280, Height: 720, Bitrate: 5_000_000, ToneMap: true}},
		{"Dolby Vision profile 5 encoded in software", "hevc", nvidia, profile5, nil},
		{"HDR10 on a GPU that does not tone map", "h264", vaapi, hdr10,
			&VideoConversion{Codec: "h264", Encoder: "h264_vaapi", Width: 1280, Height: 720, Bitrate: 5_000_000, ToneMap: true, Hardware: vaapi}},
		{"Dolby Vision profile 5 on a GPU that does not tone map", "h264", vaapi, profile5, nil},
	} {
		can := Capabilities{Encoders: []string{"libx264", "libx265"}, ToneMapping: true, Hardware: test.gpu}
		if got := ConvertVideo(test.codecs, 0, 0, test.video, can); !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s: %+v, want %+v", test.name, got, test.want)
		}
	}
}
