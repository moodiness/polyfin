package playback

import "testing"

// bitrateSource is a Matroska source of a video, 1080p unless it says,
// and one audio track, the default, its bitrate their sum.
func bitrateSource(video, audio MediaStream) MediaSource {
	video.Type, video.Index, video.VideoRange, video.VideoRangeType = "Video", 0, "SDR", "SDR"
	if video.Width == nil {
		video.Width, video.Height = new(1920), new(1080)
	}
	video.AverageFrameRate = new(23.976)
	audio.Type, audio.Index, audio.IsDefault = "Audio", 1, true
	var bitrate int64
	for _, s := range []MediaStream{video, audio} {
		if s.BitRate != nil {
			bitrate += *s.BitRate
		}
	}
	return MediaSource{Container: "mkv", Bitrate: bitrate, Streams: []MediaStream{video, audio}}
}

// dts is a DTS track of channels, which no app of these tests takes.
func dts(channels int) MediaStream {
	return MediaStream{Codec: "dts", Channels: new(channels), BitRate: new(int64(1_509_000))}
}

// Converted audio gets Jellyfin's bitrates, by the channels it keeps: 128
// kb/s in mono, 384 kb/s up to five channels, 640 kb/s from six, for a
// 7.1 source too, which AAC keeps as 5.1, and for one mixed down to
// stereo the stereo's.
func TestConvertedAudioGetsJellyfinsBitrates(t *testing.T) {
	video := MediaStream{Codec: "h264", BitDepth: new(8), BitRate: new(int64(4_000_000))}
	options := Options{EnableDirectPlay: true, EnableDirectStream: true, Can: Capabilities{Encoders: []string{"libx264"}}}
	for _, test := range []struct {
		name, profile string
		channels      int
		sent          int
		bitrate       int64
	}{
		{"mono", "minimal", 1, 1, 128_000},
		{"stereo", "minimal", 2, 2, 384_000},
		{"four channels", "minimal", 4, 4, 384_000},
		{"5.1", "minimal", 6, 6, 640_000},
		{"7.1", "minimal", 8, 6, 640_000},
		{"5.1 mixed down to stereo", "jellyfin-web-chrome", 6, 2, 384_000},
	} {
		got := Decide(readDeviceProfile(t, test.profile), bitrateSource(video, dts(test.channels)), options)
		if got.Audio == nil || got.Audio.Codec != "aac" || got.Audio.Channels != test.sent || got.Audio.Bitrate != test.bitrate {
			t.Errorf("%s: audio %+v, want AAC %d channels at %d", test.name, got.Audio, test.sent, test.bitrate)
		}
	}
}

// Under a bitrate limit, converted audio gets at most Jellyfin's share of
// it, and the video what the limit leaves.
func TestUnderALimitConvertedAudioTakesItsShare(t *testing.T) {
	video := MediaStream{Codec: "h264", BitDepth: new(8), BitRate: new(int64(20_000_000))}
	for _, test := range []struct {
		limit, audio int64
	}{
		{600_000, 128_000},
		{1_500_000, 384_000},
		{3_000_000, 448_000},
		{4_000_000, 640_000},
		{10_000_000, 640_000},
	} {
		options := Options{MaxStreamingBitrate: test.limit, EnableDirectPlay: true, EnableDirectStream: true,
			Can: Capabilities{Encoders: []string{"libx264"}}}
		got := Decide(readDeviceProfile(t, "minimal"), bitrateSource(video, dts(6)), options)
		if got.Audio == nil || got.Audio.Channels != 6 || got.Audio.Bitrate != test.audio {
			t.Errorf("within %d: audio %+v, want 5.1 at %d", test.limit, got.Audio, test.audio)
		}
		if got.Video == nil || got.Video.Bitrate != max(test.limit-test.audio, 300_000) {
			t.Errorf("within %d: video %+v, want %d", test.limit, got.Video, test.limit-test.audio)
		}
	}
}

// Audio bitrate per channel, when set, wins over the automatic bitrates and
// over the share of a limit.
func TestTheAudioBitrateSetWins(t *testing.T) {
	video := MediaStream{Codec: "h264", BitDepth: new(8), BitRate: new(int64(20_000_000))}
	can := Capabilities{Encoders: []string{"libx264"}, Tuning: Tuning{AudioBitrate: 256_000}}
	for _, limit := range []int64{0, 3_000_000} {
		options := Options{MaxStreamingBitrate: limit, EnableDirectPlay: true, EnableDirectStream: true, Can: can}
		got := Decide(readDeviceProfile(t, "minimal"), bitrateSource(video, dts(6)), options)
		if got.Audio == nil || got.Audio.Bitrate != 1_536_000 {
			t.Errorf("within %d: audio %+v, want 1536000", limit, got.Audio)
		}
	}
}
