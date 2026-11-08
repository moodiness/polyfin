package playback

import (
	"cmp"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/media"
)

// specLevels are ITU-T's limits of the levels a conversion may declare,
// written apart from the code's table: the largest frame and the most
// samples a second, in macroblocks for H.264 (H.264, table A-1) and luma
// samples for HEVC (H.265, tables A.8 and A.9), then the most bits a
// second and bits buffered of the Main tier, in thousands. H.264's High
// profile takes 1.25 times its bitrates (H.264, table A-2).
var specLevels = map[string]map[int][4]int64{
	"h264": {
		30: {1_620, 40_500, 10_000, 10_000}, 31: {3_600, 108_000, 14_000, 14_000}, 32: {5_120, 216_000, 20_000, 20_000},
		40: {8_192, 245_760, 20_000, 25_000}, 41: {8_192, 245_760, 50_000, 62_500}, 42: {8_704, 522_240, 50_000, 62_500},
		50: {22_080, 589_824, 135_000, 135_000}, 51: {36_864, 983_040, 240_000, 240_000}, 52: {36_864, 2_073_600, 240_000, 240_000},
		61: {139_264, 8_355_840, 480_000, 480_000},
	},
	"hevc": {
		30: {552_960, 16_588_800, 6_000, 6_000}, 31: {983_040, 33_177_600, 10_000, 10_000}, 40: {2_228_224, 66_846_720, 12_000, 12_000},
		41: {2_228_224, 133_693_440, 20_000, 20_000}, 50: {8_912_896, 267_386_880, 25_000, 25_000},
		51: {8_912_896, 534_773_760, 40_000, 40_000}, 52: {8_912_896, 1_069_547_520, 60_000, 60_000},
		61: {35_651_584, 2_139_095_040, 120_000, 120_000},
	},
}

// tenths is a level written as "4.1" or "5", in tenths.
func tenths(level string) int {
	whole, fraction, _ := strings.Cut(level, ".")
	a, _ := strconv.Atoi(whole)
	b, _ := strconv.Atoi(cmp.Or(fraction, "0"))
	return a*10 + b
}

// holds reports whether level, at its Main tier, holds video of codec in
// an H.264 profile, empty for High, of width by height at rate frames a
// second, its encoder told maxRate and bufSize in bits.
func holds(codec, profile, level string, width, height int, rate float64, maxRate, bufSize int64) bool {
	l, ok := specLevels[codec][tenths(level)]
	if !ok {
		return false
	}
	frame, factor := int64(width)*int64(height), int64(1000)
	if codec == "h264" {
		frame = int64((width+15)/16) * int64((height+15)/16)
		if profile == "" || profile == "high" {
			factor = 1250
		}
	}
	return frame <= l[0] && float64(frame)*rate <= float64(l[1]) && maxRate <= l[2]*factor && bufSize <= l[3]*factor
}

// ffmpegRate is the frame rate FFmpeg tells the encoder when no -r pins
// it: r_frame_rate, the average only when r_frame_rate is above 210 and
// the average below 70 (av_guess_frame_rate), else 1/time base, 1000 in
// Matroska; twice that when deinterlacing makes a frame of each field.
func ffmpegRate(average, real float64, double bool) float64 {
	rate := real
	if average > 0 && average < 70 && real > 210 {
		rate = average
	}
	if rate <= 0 {
		rate = 1000
	}
	if double {
		rate *= 2
	}
	return rate
}

// told is what an encoding tells its encoder: the frame rate -r pins, 0
// for none, and the maximum rate and the buffer, in bits.
func told(e *hls.VideoEncoding) (pinned float64, maxRate, bufSize int64) {
	if e.PinRate {
		pinned = e.FrameRate
	}
	return pinned, cmp.Or(e.MaxRate, e.Bitrate*3/2), cmp.Or(e.BufSize, e.Bitrate*2)
}

// codecString is the CODECS a playlist gives video of codec at level,
// HEVC at its Main tier.
func codecString(codec, level string) string {
	if codec == "hevc" {
		return "hvc1.1.6.L" + strconv.Itoa(tenths(level)*3) + ".B0"
	}
	return fmt.Sprintf("avc1.6400%02X", tenths(level))
}

// The level a conversion declares, at its Main tier, holds what its
// encoder is told, which an NVIDIA GPU checks before it encodes a frame:
// the frame, the rate -r pins, or else the one FFmpeg works out itself,
// the maximum rate and the buffer. The level rises with the peak rather
// than the peak being lowered, unless the app decodes no higher level,
// which then holds the bitrates. The playlist's CODECS and FRAME-RATE say
// the same.
func TestTheLevelHoldsWhatTheEncoderIsTold(t *testing.T) {
	t.Parallel()
	gpu := &hls.Hardware{Method: "cuda", Encoders: []string{"h264_nvenc", "hevc_nvenc"}}
	can := Capabilities{Encoders: []string{"libx264", "libx265"}, ToneMapping: true, Hardware: gpu}
	for _, c := range []struct {
		name, source, codec string
		width, height       int
		average, real       float64
		bitrate             int64
		field               string
		tuning              Tuning
		app                 AppCaps
		level               string
	}{
		// The production case of issue #1006: level 5 holds 25 Mb/s, the
		// encoder was told 39; 5.1 holds 40 Mb/s and a 40 Mbit buffer.
		{name: "4K HEVC at 23.976 fps and 26 Mb/s", source: "hevc", codec: "hevc", width: 3840, height: 2160,
			average: 24000.0 / 1001, real: 24000.0 / 1001, bitrate: 26_000_000, level: "5.1"},
		{name: "4K HEVC, r_frame_rate twice the average", source: "hevc", codec: "hevc", width: 3840, height: 2160,
			average: 24000.0 / 1001, real: 48000.0 / 1001, bitrate: 15_000_000, level: "5"},
		{name: "1080p H.264 at 59.94 fps", source: "h264", codec: "h264", width: 1920, height: 1080,
			average: 60000.0 / 1001, real: 60000.0 / 1001, bitrate: 10_000_000, level: "4.2"},
		{name: "1080p H.264 at 59.94 fps, averaging 29.97", source: "h264", codec: "h264", width: 1920, height: 1080,
			average: 30000.0 / 1001, real: 60000.0 / 1001, bitrate: 10_000_000, level: "4.1"},
		{name: "1080i H.264, r_frame_rate its field rate, a frame made of each field", source: "h264", codec: "h264", width: 1920, height: 1080,
			average: 25, real: 50, bitrate: 10_000_000, field: "tt", tuning: Tuning{DoubleRate: true}, level: "4.2"},
		{name: "no average, Matroska's time base", source: "hevc", codec: "hevc", width: 3840, height: 2160,
			average: 0, real: 1000, bitrate: 15_000_000, level: "5"},
		{name: "no average, MPEG-TS's time base", source: "h264", codec: "h264", width: 1920, height: 1080,
			average: 0, real: 90000, bitrate: 10_000_000, level: "4.1"},
		{name: "no rate known", source: "hevc", codec: "hevc", width: 3840, height: 2160, bitrate: 15_000_000, level: "5"},
		// Apps capping the level: H.264 4.0 holds 25 Mb/s in High, HEVC
		// 4.1 20 Mb/s.
		{name: "H.264 capped at 4.0, 30 Mb/s", source: "h264", codec: "h264", width: 1920, height: 1080,
			average: 24000.0 / 1001, real: 24000.0 / 1001, bitrate: 30_000_000, app: AppCaps{Levels: map[string]int{"h264": 40}}, level: "4"},
		{name: "HEVC capped at 4.1, 40 Mb/s", source: "h264", codec: "hevc", width: 1920, height: 1080,
			average: 24000.0 / 1001, real: 24000.0 / 1001, bitrate: 40_000_000, app: AppCaps{Levels: map[string]int{"hevc": 123}}, level: "4.1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			video := media.Stream{Index: 0, Type: "video", Codec: c.source, Width: c.width, Height: c.height, AverageRate: c.average,
				FrameRate: c.real, PixelFormat: "yuv420p", BitDepth: 8, Bitrate: c.bitrate, FieldOrder: c.field}
			can := can
			can.Tuning = c.tuning
			conversion := ConvertVideo(c.codec, Limits{App: c.app}, videoStream(video, 0, true), can)
			if conversion == nil || conversion.Codec != c.codec || conversion.Width != c.width || conversion.Height != c.height {
				t.Fatalf("conversion: %+v", conversion)
			}
			remux := Remux{Audio: -1, ConvertVideo: conversion}
			var r hls.Remux
			remux.convert(&r, video, c.tuning)
			e := r.Encode
			pinned, maxRate, bufSize := told(e)
			rate := pinned
			if rate <= 0 {
				rate = ffmpegRate(c.average, c.real, e.DoubleRate)
			}
			if !holds(c.codec, e.Profile, e.Level, e.Width, e.Height, rate, maxRate, bufSize) || e.Level != c.level {
				t.Errorf("level %s at %dx%d, want %s: the encoder told %.3f fps, -maxrate %d, -bufsize %d", e.Level, e.Width, e.Height, c.level,
					rate, maxRate, bufSize)
			}
			if most := c.app.level(c.codec); most > 0 && tenths(e.Level) > most {
				t.Errorf("level %s above the app's %d", e.Level, most)
			}
			// Without a cap, the level rises to the peak.
			if c.app.Levels == nil && (e.Bitrate != conversion.Bitrate || maxRate != conversion.peak()) {
				t.Errorf("bitrate %d peaking at %d, the conversion's %d at %d", e.Bitrate, maxRate, conversion.Bitrate, conversion.peak())
			}
			v := variant(media.Analysis{Streams: []media.Stream{video}}, video, conversion.Bitrate, remux, c.tuning)
			if want := codecString(c.codec, e.Level); v.Codecs != want || v.FrameRate != math.Round(rate*1000)/1000 {
				t.Errorf("playlist: CODECS %s at %v frames a second, want %s at %.3f", v.Codecs, v.FrameRate, want, rate)
			}
		})
	}
}
