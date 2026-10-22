package playback

import "testing"

// The web player is offered no HEVC, Dolby Vision or AV1 video, as it is
// or converted; a profile left with none is dropped, "any codec" narrowed.
func TestWithoutRiskyVideo(t *testing.T) {
	t.Parallel()
	profile := &DeviceProfile{
		DirectPlayProfiles: []DirectPlayProfile{
			{Container: "mp4,m4v", Type: "Video", VideoCodec: "h264,hevc,av1", AudioCodec: "aac"},
			{Container: "mkv", Type: "Video", VideoCodec: "hevc"},
			{Container: "webm", Type: "Video"},
			{Container: "mp3", Type: "Audio"},
		},
		TranscodingProfiles: []TranscodingProfile{
			{Container: "mp4", Type: "Video", Protocol: "hls", VideoCodec: "hevc,h264"},
			{Container: "ts", Type: "Video", Protocol: "hls", VideoCodec: "av1"},
			{Container: "aac", Type: "Audio"},
		},
	}
	safe := WithoutRiskyVideo(profile)
	if len(safe.DirectPlayProfiles) != 3 || safe.DirectPlayProfiles[0].VideoCodec != "h264" ||
		safe.DirectPlayProfiles[1].VideoCodec != "h264,vp8,vp9" || safe.DirectPlayProfiles[2].Type != "Audio" {
		t.Errorf("direct play: %+v", safe.DirectPlayProfiles)
	}
	if safe.TranscodingProfiles[0].VideoCodec != "h264" || safe.TranscodingProfiles[1].VideoCodec != "h264" || safe.TranscodingProfiles[2].VideoCodec != "" {
		t.Errorf("transcoding: %+v", safe.TranscodingProfiles)
	}
	if profile.DirectPlayProfiles[0].VideoCodec != "h264,hevc,av1" || profile.TranscodingProfiles[0].VideoCodec != "hevc,h264" {
		t.Error("the app's own profile changed")
	}
}

func TestBurningSubtitlesEncodesEveryFormat(t *testing.T) {
	profile := &DeviceProfile{SubtitleProfiles: []SubtitleProfile{{Format: "vtt", Method: "External"}}}
	burned := BurningSubtitles(profile)
	if len(profile.SubtitleProfiles) != 1 || profile.SubtitleProfiles[0].Method != "External" {
		t.Fatal("the posted profile changed")
	}
	for _, p := range burned.SubtitleProfiles {
		if p.Method != "Encode" {
			t.Fatalf("%+v", p)
		}
	}
}
