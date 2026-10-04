package playback

import (
	"slices"
	"testing"
)

// Video taller than the user's quality group never plays as it is nor
// copied: it is converted down, reported as Jellyfin reports a profile's
// height limit; video that fits plays as before.
func TestVideoTallerThanTheGroupIsConvertedDown(t *testing.T) {
	// H.264 and AAC in MP4 at 1080p, which Chrome plays as it is.
	source := MediaSource{
		Container: "mov,mp4,m4a,3gp,3g2,mj2",
		Bitrate:   8_000_000,
		Streams: []MediaStream{
			{Type: "Video", Index: 0, Codec: "h264", Profile: "High", Level: new(40.0), VideoRange: "SDR", VideoRangeType: "SDR",
				Width: new(1920), Height: new(1080), AverageFrameRate: new(23.976), BitDepth: new(8)},
			{Type: "Audio", Index: 1, Codec: "aac", IsDefault: true, Channels: new(2), BitRate: new(int64(192_000))},
		},
	}
	chrome := readDeviceProfile(t, "jellyfin-web-chrome")
	software := Capabilities{Encoders: []string{"libx264", "libx265"}}
	decide := func(options Options) Decision {
		options.EnableDirectPlay, options.EnableDirectStream = true, true
		return Decide(chrome, source, options)
	}
	for _, group := range []int{0, 1080, 2160} {
		if got := decide(Options{MaxHeight: group, Can: software}); !got.DirectPlay || got.HLS {
			t.Errorf("group %d: direct play %v, HLS %v", group, got.DirectPlay, got.HLS)
		}
	}
	for _, test := range []struct {
		name               string
		group, cap, height int
	}{
		{"under the group", 720, 0, 720},
		{"under a lower cap", 720, 480, 480},
		{"under the group, lower than the cap", 480, 720, 480},
	} {
		got := decide(Options{MaxHeight: test.group, ConversionHeight: test.cap, Can: software})
		if got.DirectPlay || !got.HLS || got.Video == nil || got.Video.Height != test.height ||
			!slices.Equal(got.Reasons, []string{"VideoResolutionNotSupported"}) {
			t.Errorf("%s: direct play %v, HLS %v, video %+v, reasons %v", test.name, got.DirectPlay, got.HLS, got.Video, got.Reasons)
		}
	}
	// Without an encoder, it plays neither as it is nor copied.
	if got := decide(Options{MaxHeight: 720}); got.DirectPlay || got.HLS {
		t.Errorf("without an encoder: direct play %v, HLS %v", got.DirectPlay, got.HLS)
	}
}
