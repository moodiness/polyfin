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
		{12_000_000, 640_000},
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

// With no limit, converted video keeps the source's bitrate, as Jellyfin
// does: more for an 8-bit conversion of 10-bit video, and more for H.264
// than for the HEVC or AV1 it replaces, more again below 3 Mb/s, and for
// a smaller picture, as the processor, the user's quality group or the
// settings' maximum make it, the share of the source's it keeps. A source
// of unknown bitrate gets its height's usual bitrate.
func TestConvertedVideoKeepsTheSourcesBitrate(t *testing.T) {
	aac := MediaStream{Codec: "aac", Channels: new(2), BitRate: new(int64(256_000))}
	hevc10 := MediaStream{Codec: "hevc", BitDepth: new(10), BitRate: new(int64(15_000_000))}
	for _, test := range []struct {
		name          string
		video         MediaStream
		convert       bool
		group, server int
		height        int
		bitrate       int64
	}{
		{"HEVC 10-bit", hevc10, false, 0, 0, 1080, 26_250_000},
		{"HEVC 8-bit", MediaStream{Codec: "hevc", BitDepth: new(8), BitRate: new(int64(15_000_000))}, false, 0, 0, 1080, 25_000_000},
		{"AV1", MediaStream{Codec: "av1", BitDepth: new(8), BitRate: new(int64(10_000_000))}, false, 0, 0, 1080, 20_000_000},
		{"H.264", MediaStream{Codec: "h264", BitDepth: new(8), BitRate: new(int64(12_000_000))}, true, 0, 0, 1080, 12_000_000},
		{"H.264 at 2 Mb/s", MediaStream{Codec: "h264", BitDepth: new(8), BitRate: new(int64(2_000_000))}, true, 0, 0, 1080, 5_000_000},
		{"4K HEVC on the processor", MediaStream{Codec: "hevc", BitDepth: new(10), BitRate: new(int64(40_000_000)),
			Width: new(3840), Height: new(2160)}, false, 0, 0, 1080, 17_500_000},
		{"a 720p quality group", hevc10, false, 720, 0, 720, 11_666_667},
		{"a 720p maximum of converted video", hevc10, false, 0, 720, 720, 11_666_667},
		{"unknown bitrate", MediaStream{Codec: "hevc", BitDepth: new(10)}, false, 0, 0, 1080, 10_000_000},
	} {
		options := Options{EnableDirectPlay: true, EnableDirectStream: true, ConvertVideo: test.convert,
			MaxHeight: test.group, ConversionHeight: test.server, Can: Capabilities{Encoders: []string{"libx264"}}}
		got := Decide(readDeviceProfile(t, "minimal"), bitrateSource(test.video, aac), options)
		if got.Video == nil || got.Video.Codec != "h264" || got.Video.Height != test.height || got.Video.Bitrate != test.bitrate {
			t.Errorf("%s: video %+v, want H.264 %dp at %d", test.name, got.Video, test.height, test.bitrate)
		}
	}
}

// A limit caps converted video at what it leaves once the audio sent is
// taken off, and the qualities offered to step down to halve from it.
func TestConvertedVideoFollowsTheLimit(t *testing.T) {
	aac := MediaStream{Codec: "aac", Channels: new(2), BitRate: new(int64(256_000))}
	hevc := MediaStream{Codec: "hevc", BitDepth: new(10), BitRate: new(int64(15_000_000))}
	options := Options{MaxStreamingBitrate: 12_000_000, EnableDirectPlay: true, EnableDirectStream: true,
		Can: Capabilities{Encoders: []string{"libx264"}}}
	got := Decide(readDeviceProfile(t, "minimal"), bitrateSource(hevc, aac), options)
	if got.Video == nil || got.Video.Height != 1080 || got.Video.Bitrate != 11_744_000 {
		t.Fatalf("video %+v, want 1080p at 11744000", got.Video)
	}
	var steps []int64
	for _, step := range got.Video.Steps(1920, 1080) {
		steps = append(steps, step.Bitrate)
	}
	if len(steps) != 2 || steps[0] > got.Video.Bitrate/2 || steps[1] > steps[0]/2 {
		t.Errorf("steps %v below %d", steps, got.Video.Bitrate)
	}
}
