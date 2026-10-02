package playback

import (
	"encoding/json"
	"os"
	"path"
	"slices"
	"strings"
	"testing"
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
