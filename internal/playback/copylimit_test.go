package playback

import (
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/media"
)

// Under a bitrate limit, with the video converted, a track the app takes
// is copied only when its bitrate is within the audio's share of the
// limit, as Jellyfin copies it; a heavier one, such as FLAC 5.1 counted
// at 2.88 Mb/s, is converted to a lossy codec the app takes, at the
// share, and leaves the video the rest. With no limit, and where the track
// fits its share, it is copied as before.
func TestCopiedAudioStaysWithinTheLimit(t *testing.T) {
	// Swiftfin's native player takes AAC, AC-3, ALAC, E-AC-3 and FLAC
	// over HLS, up to 8 channels, and no Matroska file as it is.
	profile := readDeviceProfile(t, "swiftfin-native")
	for _, tc := range []struct {
		name        string
		audio       media.Stream
		limit       int64
		convert     *AudioConversion
		videoBudget int64
	}{
		// The FLAC track states no bitrate: 480 kb/s a channel.
		{"FLAC 5.1 within 3 Mb/s", media.Stream{Codec: "flac", Channels: 6, ChannelLayout: "5.1(side)", SampleRate: 48000}, 3_000_000,
			&AudioConversion{Codec: "aac", Channels: 6, Bitrate: 448_000}, 2_552_000},
		{"AC-3 5.1 at 448 kb/s within 3 Mb/s", media.Stream{Codec: "ac3", Channels: 6, ChannelLayout: "5.1(side)", SampleRate: 48000, Bitrate: 448_000},
			3_000_000, nil, 2_552_000},
		// FLAC in stereo, 960 kb/s, fits 10 Mb/s's share of 1.536 Mb/s.
		{"FLAC stereo within 10 Mb/s", media.Stream{Codec: "flac", Channels: 2, ChannelLayout: "stereo", SampleRate: 48000}, 10_000_000,
			nil, 9_040_000},
		{"FLAC 5.1 with no limit", media.Stream{Codec: "flac", Channels: 6, ChannelLayout: "5.1(side)", SampleRate: 48000}, 0, nil, 0},
	} {
		tc.audio.Index, tc.audio.Type, tc.audio.Default = 1, "audio", true
		video := media.Stream{Index: 0, Type: "video", Codec: "h264", Profile: "High", Width: 1920, Height: 1080, FrameRate: 24, AverageRate: 24,
			PixelFormat: "yuv420p", BitDepth: 8, Bitrate: 8_000_000}
		analysis := media.Analysis{Format: "matroska,webm", Duration: time.Hour, Bitrate: 8_800_000, Streams: []media.Stream{video, tc.audio}}
		source := MediaSource{Container: "mkv", Bitrate: analysis.Bitrate, Streams: MediaStreams(analysis, nil, "en")}
		d := Decide(profile, source, Options{MaxStreamingBitrate: tc.limit, EnableDirectPlay: true, EnableDirectStream: true, ConvertVideo: true,
			Can: Capabilities{Encoders: []string{"libx264"}}})
		if !d.HLS || d.Video == nil {
			t.Fatalf("%s: HLS %v, video %+v", tc.name, d.HLS, d.Video)
		}
		if (d.Audio == nil) != (tc.convert == nil) || d.Audio != nil && *d.Audio != *tc.convert {
			t.Errorf("%s: audio %+v, want %+v", tc.name, d.Audio, tc.convert)
		}
		if budget := VideoLimit(tc.limit, SentAudioBitrate(&source.Streams[1], d.Audio)); budget != tc.videoBudget || tc.limit > 0 && d.Video.Bitrate > budget {
			t.Errorf("%s: video budget %d, bitrate %d; want %d", tc.name, budget, d.Video.Bitrate, tc.videoBudget)
		}
	}
}

// Under a limit, audio converted for an app that also takes FLAC goes to
// a lossy codec it takes, whose bitrate the limit bounds; with no limit it
// keeps the app's order.
func TestAudioConvertedUnderALimitIsLossy(t *testing.T) {
	if got := ConvertAudio("flac,aac", "6", 6, "5.1(side)", 3_000_000, Tuning{}); got == nil || got.Codec != "aac" || got.Bitrate != 448_000 {
		t.Errorf("within 3 Mb/s: %+v", got)
	}
	if got := ConvertAudio("flac,aac", "6", 6, "5.1(side)", 0, Tuning{}); got == nil || got.Codec != "flac" {
		t.Errorf("with no limit: %+v", got)
	}
	if got := ConvertAudio("flac", "6", 6, "5.1(side)", 3_000_000, Tuning{}); got == nil || got.Codec != "flac" {
		t.Errorf("FLAC only, within 3 Mb/s: %+v", got)
	}
}
