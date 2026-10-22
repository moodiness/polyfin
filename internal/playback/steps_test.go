package playback

import (
	"fmt"
	"testing"

	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/media"
)

// A conversion offers up to two lower rungs of the ladder, each the
// tallest whose bitrate is at most half of the quality above, down to
// 360p, keeping its codec, encoder, tone mapping and shape.
func TestConversionsStepDownTheLadder(t *testing.T) {
	software := Capabilities{Encoders: []string{"libx264"}, ToneMapping: true}
	nvidia := Capabilities{Encoders: []string{"libx264"}, Hardware: &hls.Hardware{Method: "cuda", Encoders: []string{"hevc_nvenc"}, ToneMapping: true}}
	sdr := func(width, height int) MediaStream {
		return MediaStream{Codec: "mpeg2video", VideoRange: "SDR", Width: new(width), Height: new(height)}
	}
	for _, test := range []struct {
		name   string
		codecs string
		can    Capabilities
		limits Limits
		video  MediaStream
		want   string
	}{
		{"1080p", "h264", software, Limits{}, sdr(1920, 1080), "1920x1080@10000000 1280x720@5000000 852x480@2000000"},
		{"720p", "h264", software, Limits{Group: 720}, sdr(1920, 1080), "1280x720@5000000 852x480@2000000 640x360@1000000"},
		// A 720p source converts at 1080p's bitrate: half of it is 540p's.
		{"a 720p source", "h264", software, Limits{}, sdr(1280, 720), "1280x720@10000000 960x540@3000000 640x360@1000000"},
		{"a wide 1080p", "h264", software, Limits{}, sdr(1920, 800), "1920x800@10000000 1280x532@5000000 854x354@2000000"},
		{"a 4:3 picture", "h264", software, Limits{}, sdr(1440, 1080), "1440x1080@10000000 960x720@5000000 640x480@2000000"},
		// The app's bitrate sets the halves from its own.
		{"1080p at 7 Mb/s", "h264", software, Limits{Bitrate: 7_000_000, Video: 7_000_000}, sdr(1920, 1080),
			"1920x1080@7000000 960x540@3000000 640x360@1000000"},
		{"4K on a GPU", "hevc", nvidia, Limits{}, sdr(3840, 2160), "3840x2160@40000000 2560x1440@20000000 1920x1080@10000000"},
		// The first keeps about the source's bitrate; the others halve
		// from it.
		{"a 15 Mb/s HEVC source", "h264", software, Limits{}, MediaStream{Codec: "hevc", VideoRange: "SDR", Width: new(1920), Height: new(1080),
			BitDepth: new(10), BitRate: new(int64(15_000_000))}, "1920x1080@26250000 1280x720@5000000 852x480@2000000"},
		// Nothing below 360p, nor half a bitrate too low for it.
		{"480p", "h264", software, Limits{Server: 480}, sdr(1920, 1080), "852x480@2000000 640x360@1000000"},
		{"360p", "h264", software, Limits{Server: 360}, sdr(1920, 1080), "640x360@1000000"},
		{"1.5 Mb/s", "h264", software, Limits{Bitrate: 1_500_000, Video: 1_500_000}, sdr(1920, 1080), "960x540@1500000"},
	} {
		first := ConvertVideo(test.codecs, test.limits, test.video, test.can)
		if first == nil {
			t.Fatalf("%s: not converted", test.name)
		}
		got := fmt.Sprintf("%dx%d@%d", first.Width, first.Height, first.Bitrate)
		for _, step := range first.Steps(*test.video.Width, *test.video.Height) {
			got += fmt.Sprintf(" %dx%d@%d", step.Width, step.Height, step.Bitrate)
			if step.Codec != first.Codec || step.Encoder != first.Encoder || step.Hardware != first.Hardware || step.ToneMap != first.ToneMap {
				t.Errorf("%s: a step %+v of %+v", test.name, step, first)
			}
		}
		if got != test.want {
			t.Errorf("%s: %s, want %s", test.name, got, test.want)
		}
	}
}

// A remux's variant converts to its step; one past the steps has none,
// and a copy has only the first.
func TestRemuxVariantsAreTheSteps(t *testing.T) {
	first := ConvertVideo("h264", Limits{}, MediaStream{Codec: "mpeg2video", VideoRange: "SDR", Width: new(1920), Height: new(1080)},
		Capabilities{Encoders: []string{"libx264"}})
	remux := Remux{ConvertVideo: first}
	for variant, height := range []int{1080, 720, 480} {
		remux.Variant = variant
		if c := remux.conversion(media.Stream{Width: 1920, Height: 1080}); !remux.HasVariant() || c == nil || c.Height != height {
			t.Errorf("variant %d: %+v", variant, c)
		}
	}
	remux.Variant = 3
	if remux.HasVariant() {
		t.Error("a variant past the steps")
	}
	if copied := (Remux{Variant: 1}); copied.HasVariant() {
		t.Error("a lower variant of a copy")
	}
}
