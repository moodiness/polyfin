package playback

import (
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/media"
)

// A converted variant's BANDWIDTH in the master playlist is its video's
// peak with the audio sent, and its AVERAGE-BANDWIDTH its video's bitrate
// with that audio: the converted audio's bitrate, a copied track's own, or
// for a copied track of unknown bitrate the converted bitrate of its
// channels. Within 3 Mb/s, the first quality's BANDWIDTH is then exactly
// the limit, and every lower quality's below it.
func TestMasterBandwidthCountsTheAudioSent(t *testing.T) {
	for _, tc := range []struct {
		name     string
		audio    media.Stream
		profile  string
		sent     int64
		convert  bool
		channels int
	}{
		// Chrome takes no AC-3: stereo AAC at 384 kb/s.
		{"AC-3 converted", media.Stream{Codec: "ac3", Channels: 6, ChannelLayout: "5.1(side)", Bitrate: 448_000}, "jellyfin-web-chrome", 384_000, true, 2},
		// Under 3 Mb/s, AAC 5.1 gets 448 kb/s.
		{"AAC 5.1 converted", media.Stream{Codec: "dts", Channels: 6, ChannelLayout: "5.1(side)", Bitrate: 1_509_000}, "minimal", 448_000, true, 6},
		// A copied track counts its own bitrate.
		{"AAC copied", media.Stream{Codec: "aac", Channels: 2, Bitrate: 160_000}, "jellyfin-web-chrome", 160_000, false, 2},
		// AC-3 copied without a known bitrate counts as 5.1 converted.
		{"AC-3 copied, bitrate unknown", media.Stream{Codec: "ac3", Channels: 6, ChannelLayout: "5.1(side)"}, "swiftfin-native", 640_000, false, 6},
	} {
		tc.audio.Index, tc.audio.Type, tc.audio.Default, tc.audio.SampleRate = 1, "audio", true, 48000
		video := media.Stream{Index: 0, Type: "video", Codec: "h264", Profile: "High", Width: 1280, Height: 720, FrameRate: 24, AverageRate: 24,
			PixelFormat: "yuv420p", BitDepth: 8, Bitrate: 4_632_000}
		analysis := media.Analysis{Format: "matroska,webm", Duration: time.Hour, Bitrate: 4_632_000 + tc.audio.Bitrate, Streams: []media.Stream{video, tc.audio}}
		source := MediaSource{Container: "mkv", Bitrate: analysis.Bitrate, Streams: MediaStreams(analysis, nil, "en")}
		for _, limit := range []int64{3_000_000, 0} {
			d := Decide(readDeviceProfile(t, tc.profile), source, Options{MaxStreamingBitrate: limit, EnableDirectStream: true,
				ConvertVideo: true, Can: Capabilities{Encoders: []string{"libx264"}}})
			if d.Video == nil || (d.Audio != nil) != tc.convert || d.Audio != nil && (d.Audio.Channels != tc.channels || d.Audio.Bitrate != tc.sent && limit > 0) {
				t.Fatalf("%s within %d: video %+v, audio %+v", tc.name, limit, d.Video, d.Audio)
			}
			sent := tc.sent
			if d.Audio != nil {
				sent = d.Audio.Bitrate
			}
			remux := Remux{Audio: 1, ConvertVideo: d.Video, ConvertAudio: d.Audio}
			for i := 0; i <= len(d.Video.Steps(video.Width, video.Height)); i++ {
				remux.Variant = i
				c := remux.conversion(video)
				v := variant(analysis, video, analysis.Bitrate, remux)
				if v.Bandwidth != c.peak()+sent || v.AverageBandwidth != c.Bitrate+sent {
					t.Errorf("%s within %d, quality %d: BANDWIDTH %d, AVERAGE-BANDWIDTH %d; want %d and %d", tc.name, limit, i,
						v.Bandwidth, v.AverageBandwidth, c.peak()+sent, c.Bitrate+sent)
				}
				if limit > 0 && v.Bandwidth > limit {
					t.Errorf("%s within %d, quality %d: BANDWIDTH %d", tc.name, limit, i, v.Bandwidth)
				}
			}
		}
	}
}
