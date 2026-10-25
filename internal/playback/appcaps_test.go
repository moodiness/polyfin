package playback

import "testing"

// Video converted for an app keeps within what its profile allows besides
// the frame: the codec level it decodes, which a smaller frame then fits;
// the frame rate, the bitrate, and the conditions of its transcoding
// profile, as Jellyfin's StreamBuilder applies them; and a VideoProfile
// condition allowing none of High, Main and Baseline leaves High.
func TestConversionKeepsWithinTheAppsProfile(t *testing.T) {
	t.Parallel()
	source := func(rate float64) MediaSource {
		return MediaSource{Container: "mkv", Bitrate: 8_200_000, Streams: []MediaStream{
			{Type: "Video", Index: 0, Codec: "h264", Profile: "High", Level: new(40.0), VideoRange: "SDR", VideoRangeType: "SDR",
				Width: new(1920), Height: new(1080), AverageFrameRate: new(rate), BitDepth: new(8), BitRate: new(int64(8_000_000))},
			{Type: "Audio", Index: 1, Codec: "aac", IsDefault: true, Channels: new(2), BitRate: new(int64(128_000))},
		}}
	}
	lessThan := func(property, value string) ProfileCondition {
		return ProfileCondition{Condition: "LessThanEqual", Property: property, Value: value, IsRequired: true}
	}
	profile := func(transcoding []ProfileCondition, conditions ...ProfileCondition) *DeviceProfile {
		return &DeviceProfile{
			DirectPlayProfiles: []DirectPlayProfile{{Container: "mkv,mp4", Type: "Video", VideoCodec: "h264", AudioCodec: "aac"}},
			TranscodingProfiles: []TranscodingProfile{{Container: "ts", Type: "Video", VideoCodec: "h264", AudioCodec: "aac", Protocol: "hls",
				Context: "Streaming", Conditions: transcoding}},
			CodecProfiles: []CodecProfile{{Type: "Video", Codec: "h264", Conditions: conditions}},
		}
	}
	software := Capabilities{Encoders: []string{"libx264"}}
	for _, test := range []struct {
		name          string
		profile       *DeviceProfile
		rate          float64
		width, height int
		level         string
		frameRate     float64
		bitrate       int64
		profileName   string
	}{
		// Level 3.1 holds 720p, not 1080p; the bitrate follows the picture.
		{"a codec level", profile(nil, lessThan("VideoLevel", "31")), 24, 1280, 720, "3.1", 0, 3_555_556, ""},
		{"a frame rate", profile(nil, lessThan("VideoFramerate", "30")), 60, 1920, 1080, "4.1", 30, 8_000_000, ""},
		// 2 Mb/s is a 540p rung's bitrate.
		{"a bitrate", profile(nil, lessThan("VideoBitrate", "2000000")), 24, 960, 540, "4.1", 0, 2_000_000, ""},
		{"the transcoding profile's conditions", profile([]ProfileCondition{lessThan("Height", "480")}, lessThan("Height", "720")), 24,
			852, 480, "4.1", 0, 3_944_444, ""},
		{"no profile Polyfin writes", profile(nil, ProfileCondition{Condition: "EqualsAny", Property: "VideoProfile", Value: "high 10"}), 24,
			1920, 1080, "4.1", 0, 8_000_000, ""},
		{"Baseline only", profile(nil, ProfileCondition{Condition: "Equals", Property: "VideoProfile", Value: "constrained baseline"}), 24,
			1920, 1080, "4.1", 0, 8_000_000, "baseline"},
	} {
		t.Run(test.name, func(t *testing.T) {
			decision := Decide(test.profile, source(test.rate), Options{EnableDirectPlay: true, Can: software})
			v := decision.Video
			if !decision.HLS || v == nil {
				t.Fatalf("not converted: %+v", decision)
			}
			rate := test.rate
			if v.FrameRate > 0 {
				rate = v.FrameRate
			}
			if v.Width != test.width || v.Height != test.height || v.Level(rate) != test.level || v.FrameRate != test.frameRate ||
				v.Bitrate != test.bitrate || v.Profile != test.profileName {
				t.Errorf("%dx%d level %s, %v frames a second, %d b/s, profile %q", v.Width, v.Height, v.Level(rate), v.FrameRate, v.Bitrate, v.Profile)
			}
		})
	}
}
