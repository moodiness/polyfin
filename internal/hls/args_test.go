package hls

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"
)

// defaultCases are conversions of every kind, by name, with the
// settings left as they are by default.
func defaultCases() []struct {
	name string
	args func() []string
} {
	plan := NewPlan([]time.Duration{0, 6 * time.Second, 12 * time.Second}, 18*time.Second)
	nvidia := &Hardware{Method: "cuda", Encoders: []string{"h264_nvenc", "hevc_nvenc"}}
	nvidiaToneMapping := &Hardware{Method: "cuda", Encoders: []string{"h264_nvenc", "hevc_nvenc"}, ToneMapping: true}
	vaapi := &Hardware{Method: "vaapi", Device: "/dev/dri/renderD128", Encoders: []string{"h264_vaapi", "hevc_vaapi"}}
	burn := 3
	return []struct {
		name string
		args func() []string
	}{
		{"copy", func() []string {
			return Remux{Input: "http://127.0.0.1:1/a.mkv", Video: 0, Audio: 1, Format: FMP4, Plan: plan, VideoTag: "hvc1"}.args(1)
		}},
		{"cpu h264", func() []string {
			return Remux{Input: "http://127.0.0.1:1/a.mkv", Video: 0, Audio: 1, Format: FMP4, Plan: plan, VideoTag: "avc1",
				AudioCodec: "aac", AudioChannels: 2, AudioBitrate: 192_000,
				Encode: &VideoEncoding{Encoder: "libx264", Level: "4.1", Width: 1280, Height: 720, Bitrate: 5_000_000, FrameRate: 23.976, Deinterlace: true}}.args(1)
		}},
		{"cpu hevc tone mapped", func() []string {
			return Remux{Input: "http://127.0.0.1:1/a.mkv", Video: 0, Audio: 1, Format: TS, Plan: plan, Subtitles: []int{2},
				AudioCodec: "ac3", AudioChannels: 6, AudioBitrate: 384_000,
				Encode: &VideoEncoding{Encoder: "libx265", Level: "4.1", Width: 1280, Height: 720, Bitrate: 5_000_000, FrameRate: 24, ToneMap: true}}.args(0)
		}},
		{"cpu burn", func() []string {
			return Remux{Input: "http://127.0.0.1:1/a.mkv", Video: 0, Audio: -1, Format: TS, Plan: plan,
				Encode: &VideoEncoding{Encoder: "libx264", Level: "4.2", Width: 1920, Height: 1080, Bitrate: 10_000_000, FrameRate: 50, Burn: &burn}}.args(2)
		}},
		{"nvenc", func() []string {
			return Remux{Input: "http://127.0.0.1:1/a.mkv", Video: 0, Audio: 1, Format: FMP4, Plan: plan, VideoTag: "avc1",
				AudioCodec: "aac", AudioChannels: 2, AudioBitrate: 192_000,
				Encode: &VideoEncoding{Encoder: "h264_nvenc", Level: "4.1", Width: 1920, Height: 1080, Bitrate: 8_000_000, FrameRate: 25, Deinterlace: true, Hardware: nvidia}}.args(0)
		}},
		{"nvenc tone mapped", func() []string {
			return Remux{Input: "http://127.0.0.1:1/a.mkv", Video: 0, Audio: 1, Format: FMP4, Plan: plan, VideoTag: "hvc1",
				Encode: &VideoEncoding{Encoder: "hevc_nvenc", Level: "4.1", Width: 3840, Height: 2160, Bitrate: 20_000_000, FrameRate: 23.976, ToneMap: true, Hardware: nvidiaToneMapping}}.args(1)
		}},
		{"nvenc burn", func() []string {
			return Remux{Input: "http://127.0.0.1:1/a.mkv", Video: 0, Audio: 1, Format: TS, Plan: plan,
				Encode: &VideoEncoding{Encoder: "h264_nvenc", Level: "4.1", Width: 1920, Height: 1080, Bitrate: 8_000_000, FrameRate: 24, ToneMap: true, Burn: &burn, Hardware: nvidiaToneMapping}}.args(0)
		}},
		{"vaapi", func() []string {
			return Remux{Input: "http://127.0.0.1:1/a.mkv", Video: 0, Audio: 1, Format: FMP4, Plan: plan, VideoTag: "avc1",
				AudioCodec: "aac", AudioChannels: 6, AudioBitrate: 384_000,
				Encode: &VideoEncoding{Encoder: "h264_vaapi", Level: "4.1", Width: 1920, Height: 1080, Bitrate: 8_000_000, FrameRate: 29.97, Deinterlace: true, Hardware: vaapi}}.args(1)
		}},
		{"vaapi tone mapped", func() []string {
			return Remux{Input: "http://127.0.0.1:1/a.mkv", Video: 0, Audio: -1, Format: TS, Plan: plan,
				Encode: &VideoEncoding{Encoder: "hevc_vaapi", Level: "4.1", Width: 1280, Height: 720, Bitrate: 5_000_000, FrameRate: 24, ToneMap: true, Burn: &burn, Hardware: vaapi}}.args(0)
		}},
		{"live cpu", func() []string {
			return Remux{Input: "http://127.0.0.1:1/live.m3u8", InputOptions: []string{"-re"}, Video: 0, Audio: 1, Format: TS,
				AudioCodec: "aac", AudioChannels: 2, AudioBitrate: 192_000,
				Encode: &VideoEncoding{Encoder: "libx264", Level: "4.1", Width: 1280, Height: 720, Bitrate: 5_000_000, FrameRate: 25, Deinterlace: true}}.liveArgs("/tmp/live", 4)
		}},
		{"live nvenc", func() []string {
			return Remux{Input: "http://127.0.0.1:1/live.m3u8", Video: 0, Audio: 1, Format: FMP4, ADTS: true, VideoTag: "avc1",
				Encode: &VideoEncoding{Encoder: "h264_nvenc", Level: "4.2", Width: 1920, Height: 1080, Bitrate: 8_000_000, FrameRate: 50, Hardware: nvidia}}.liveArgs("/tmp/live", 0)
		}},
		{"live vaapi", func() []string {
			return Remux{Input: "http://127.0.0.1:1/live.m3u8", Video: 0, Audio: 1, Format: TS,
				AudioCodec: "aac", AudioChannels: 2, AudioBitrate: 192_000,
				Encode: &VideoEncoding{Encoder: "h264_vaapi", Level: "4.1", Width: 1280, Height: 720, Bitrate: 4_000_000, FrameRate: 25, Deinterlace: true, Hardware: vaapi}}.liveArgs("/tmp/live", 0)
		}},
	}
}

// The conversion settings' defaults keep the FFmpeg command lines
// Polyfin ran before they were settings: testdata/default-args.json was
// recorded from version 0.11.0.
func TestDefaultSettingsKeepTheArguments(t *testing.T) {
	data, err := os.ReadFile("testdata/default-args.json")
	if err != nil {
		t.Fatal(err)
	}
	var want map[string][]string
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatal(err)
	}
	cases := defaultCases()
	if len(cases) != len(want) {
		t.Errorf("%d cases, %d recorded", len(cases), len(want))
	}
	for _, c := range cases {
		if got := c.args(); !reflect.DeepEqual(got, want[c.name]) {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, want[c.name])
		}
	}
}
