package playback

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Tuning is what the server's settings change in conversions (see
// accounts.Settings). Its zero value is what Polyfin did before they were
// settings.
type Tuning struct {
	// Preset is x264's name of the encoder preset, empty for Polyfin's
	// choice for each encoder.
	Preset string
	// H264Quality and HEVCQuality are the encoders' quality factors, 0
	// aiming for the bitrate alone.
	H264Quality, HEVCQuality int
	// PreferHEVC converts video to the first of H.264 and HEVC the app
	// lists, rather than to H.264 whenever the app takes it.
	PreferHEVC bool
	// CPUDecoded are the codecs, of accounts.HardwareDecodingCodecs, a GPU
	// leaves to FFmpeg's own decoders.
	CPUDecoded []string
	// NoToneMapping converts HDR video to SDR without tone mapping it.
	NoToneMapping bool
	// CPUToneMapping tone maps HDR on the processor even on a GPU that
	// could.
	CPUToneMapping bool
	// ToneMapCurve is the tone mapping curve, empty for Polyfin's;
	// ToneMapPeak, in nits, overrides the video's peak, 0 keeping it;
	// ToneMapDesat desaturates highlights.
	ToneMapCurve string
	ToneMapPeak  int
	ToneMapDesat float64
	// Deinterlacer is bwdif, or empty for yadif; DoubleRate makes a frame
	// of each field of video up to 30 frames a second.
	Deinterlacer string
	DoubleRate   bool
	// Downmix is Jellyfin's name of the algorithm audio is mixed down to
	// stereo with, empty for FFmpeg's own; DownmixBoost multiplies the
	// volume of a downmix, 0 or 1 changing nothing, above 1 through the
	// limiter.
	Downmix      string
	DownmixBoost float64
	// MaxAudioChannels caps converted audio, 0 for no cap but the app's.
	MaxAudioChannels int
	// AudioBitrate is converted audio's bitrate for each channel, in bits
	// per second, 0 for Polyfin's.
	AudioBitrate int64
	// Normalize evens the loudness of the audio of converted videos (see
	// normalizer).
	Normalize bool
	// Threads is how many threads FFmpeg converts with, 0 letting it
	// choose.
	Threads int
}

// TuningOf is the tuning settings ask for.
func TuningOf(settings accounts.Settings) Tuning {
	t := Tuning{
		Preset:           settings.EncoderPreset,
		H264Quality:      settings.H264Quality,
		HEVCQuality:      settings.HevcQuality,
		PreferHEVC:       settings.AllowHevcEncoding,
		NoToneMapping:    !settings.ToneMapping,
		CPUToneMapping:   !settings.GPUToneMapping,
		ToneMapCurve:     settings.ToneMappingAlgorithm,
		ToneMapPeak:      settings.ToneMappingPeak,
		ToneMapDesat:     settings.ToneMappingDesat,
		Deinterlacer:     settings.DeinterlaceMethod,
		DoubleRate:       settings.DeinterlaceDoubleRate,
		Downmix:          settings.DownmixAlgorithm,
		DownmixBoost:     settings.DownmixBoost,
		MaxAudioChannels: settings.MaxAudioChannels,
		AudioBitrate:     int64(settings.AudioBitratePerChannel) * 1000,
		Threads:          settings.EncodingThreads,
		Normalize:        settings.NormalizeConvertedAudio,
	}
	// The values that change nothing are the zero values.
	if t.Preset == "auto" {
		t.Preset = ""
	}
	if t.ToneMapCurve == "auto" {
		t.ToneMapCurve = ""
	}
	if t.Deinterlacer == "yadif" {
		t.Deinterlacer = ""
	}
	if t.Downmix == "None" {
		t.Downmix = ""
	}
	if t.DownmixBoost == 1 {
		t.DownmixBoost = 0
	}
	for _, codec := range accounts.HardwareDecodingCodecs {
		if !slices.Contains(settings.HardwareDecodingCodecs, codec) {
			t.CPUDecoded = append(t.CPUDecoded, codec)
		}
	}
	return t
}

// DecodesOnGPU reports whether a GPU decodes video of codec, FFmpeg's
// name, in bitDepth bits: HEVC in more than 8 bits needs hevc_10bit too.
// Codecs the settings do not list are left to the GPU, which hands those
// it cannot decode to FFmpeg's own decoder.
func (t Tuning) DecodesOnGPU(codec string, bitDepth int) bool {
	codec = strings.ToLower(codec)
	if slices.Contains(t.CPUDecoded, codec) {
		return false
	}
	return codec != "hevc" || bitDepth <= 8 || !slices.Contains(t.CPUDecoded, "hevc_10bit")
}

// quality is the quality factor of a codec, h264 or hevc.
func (t Tuning) quality(codec string) int {
	if codec == "hevc" {
		return t.HEVCQuality
	}
	return t.H264Quality
}

// downmixFilters are Jellyfin's downmixes to stereo, by algorithm and
// source channel layout; 7.1 is mixed down to 5.1 first.
var downmixFilters = map[[2]string]string{
	{"Dave750", "5.1"}:           "pan=stereo|c0=0.5*c2+0.707*c0+0.707*c4+0.5*c3|c1=0.5*c2+0.707*c1+0.707*c5+0.5*c3",
	{"Dave750", "7.1"}:           "pan=5.1(side)|c0=c0|c1=c1|c2=c2|c3=c3|c4=0.707*c4+0.707*c6|c5=0.707*c5+0.707*c7,pan=stereo|c0=0.5*c2+0.707*c0+0.707*c4+0.5*c3|c1=0.5*c2+0.707*c1+0.707*c5+0.5*c3",
	{"NightmodeDialogue", "5.1"}: "pan=stereo|c0=c2+0.30*c0+0.30*c4|c1=c2+0.30*c1+0.30*c5",
	{"NightmodeDialogue", "7.1"}: "pan=5.1(side)|c0=c0|c1=c1|c2=c2|c3=c3|c4=0.707*c4+0.707*c6|c5=0.707*c5+0.707*c7,pan=stereo|c0=c2+0.30*c0+0.30*c4|c1=c2+0.30*c1+0.30*c5",
	{"Rfc7845", "3.0"}:           "pan=stereo|c0=0.414214*c2+0.585786*c0|c1=0.414214*c2+0.585786*c1",
	{"Rfc7845", "quad"}:          "pan=stereo|c0=0.422650*c0+0.366025*c2+0.211325*c3|c1=0.422650*c1+0.366025*c3+0.211325*c2",
	{"Rfc7845", "5.0"}:           "pan=stereo|c0=0.460186*c2+0.650802*c0+0.563611*c3+0.325401*c4|c1=0.460186*c2+0.650802*c1+0.563611*c4+0.325401*c3",
	{"Rfc7845", "5.1"}:           "pan=stereo|c0=0.374107*c2+0.529067*c0+0.458186*c4+0.264534*c5+0.374107*c3|c1=0.374107*c2+0.529067*c1+0.458186*c5+0.264534*c4+0.374107*c3",
	{"Rfc7845", "6.1"}:           "pan=stereo|c0=0.321953*c2+0.455310*c0+0.394310*c5+0.227655*c6+0.278819*c4+0.321953*c3|c1=0.321953*c2+0.455310*c1+0.394310*c6+0.227655*c5+0.278819*c4+0.321953*c3",
	{"Rfc7845", "7.1"}:           "pan=stereo|c0=0.274804*c2+0.388631*c0+0.336565*c6+0.194316*c7+0.336565*c4+0.194316*c5+0.274804*c3|c1=0.274804*c2+0.388631*c1+0.336565*c7+0.194316*c6+0.336565*c5+0.194316*c4+0.274804*c3",
	{"Ac4", "3.0"}:               "pan=stereo|c0=c0+0.707*c2|c1=c1+0.707*c2",
	{"Ac4", "5.0"}:               "pan=stereo|c0=c0+0.707*c2+0.707*c3|c1=c1+0.707*c2+0.707*c4",
	{"Ac4", "5.1"}:               "pan=stereo|c0=c0+0.707*c2+0.707*c4|c1=c1+0.707*c2+0.707*c5",
	{"Ac4", "7.0"}:               "pan=5.0(side)|c0=c0|c1=c1|c2=c2|c3=0.707*c3+0.707*c5|c4=0.707*c4+0.707*c6,pan=stereo|c0=c0+0.707*c2+0.707*c3|c1=c1+0.707*c2+0.707*c4",
	{"Ac4", "7.1"}:               "pan=5.1(side)|c0=c0|c1=c1|c2=c2|c3=c3|c4=0.707*c4+0.707*c6|c5=0.707*c5+0.707*c7,pan=stereo|c0=c0+0.707*c2+0.707*c4|c1=c1+0.707*c2+0.707*c5",
}

// guessedLayouts are the layouts Jellyfin assumes of audio whose layout
// is unknown, by channel count.
var guessedLayouts = map[int]string{1: "mono", 2: "stereo", 3: "2.1", 4: "4.0", 5: "5.0", 6: "5.1", 7: "6.1", 8: "7.1"}

// limiter keeps the peaks of audio made louder 1.5 dB under full scale,
// sample by sample, with FFmpeg's lookahead limiter, its automatic level
// off: FFmpeg's AAC and AC-3 encoders add some 0.2 dB to them, so loud
// passages end at or under -1 dBTP. Without it, a volume above 1 overloads
// the encoder on loud scenes, which distorts them, and the normalizer lets
// some peaks of its first seconds through.
const limiter = "alimiter=limit=0.841395:level=disabled"

// stereoFirst mixes audio down to stereo with FFmpeg's own downmix, as -ac
// 2 would after the filters: before them, the volume, the normalizer and
// the limiter work on the stereo the app gets.
const stereoFirst = "aformat=channel_layouts=stereo"

// normalizer evens loudness at -16 LUFS, peaks at -1.5 dBTP and a range
// of 11 LU at most, with FFmpeg's EBU R128 normalization in one pass: it
// reads 3 seconds ahead, and settles over the first seconds of each
// FFmpeg run. It works at 192 kHz, so a resampling follows it.
const normalizer = "loudnorm=I=-16:TP=-1.5:LRA=11"

// downmix is the filter mixing audio of channels in layout, as ffprobe
// names it, down to stereo, empty when FFmpeg's own downmix does: Jellyfin's
// algorithm for the layout, then the boost, then the limiter when the boost
// makes it louder. Like Jellyfin's probe, it reads ffprobe's 5.1(side) as
// 5.1. Music and audiobooks go through it alone, never normalized: apps
// even their volume themselves, from their NormalizationGain.
func (t Tuning) downmix(channels int, layout string) string {
	return strings.Join(t.audioFilters(channels, layout, true, false, 0), ",")
}

// videoAudio is the filter graph a video's audio track, source, goes
// through when it is converted to channels at rate Hz, 0 keeping the
// source's rate, empty for none: the downmix to stereo when channels
// mixes it down, then, when the tuning normalizes, the normalizer.
func (t Tuning) videoAudio(source AudioSource, channels, rate int) string {
	return strings.Join(t.audioFilters(source.Channels, source.ChannelLayout, channels == 2 && source.Channels > 2, t.Normalize,
		cmp.Or(rate, source.SampleRate, 48_000)), ",")
}

// audioFilters are the filters converted audio of channels in layout goes
// through, in order: when stereo, Jellyfin's algorithm for the layout,
// else FFmpeg's downmix when a filter follows, and the boost; when
// normalize, the normalizer, resampled back to rate; then the limiter
// after a boost above 1 or the normalizer.
func (t Tuning) audioFilters(channels int, layout string, stereo, normalize bool, rate int) []string {
	var filters []string
	limit, mixed := false, !stereo
	if stereo {
		layout, _, _ = strings.Cut(layout, "(")
		if layout == "" {
			layout = guessedLayouts[channels]
		}
		if filter, ok := downmixFilters[[2]string{t.Downmix, layout}]; ok {
			filters, mixed = append(filters, filter), true
		}
		if t.DownmixBoost != 0 && t.DownmixBoost != 1 {
			filters = append(filters, "volume="+strconv.FormatFloat(t.DownmixBoost, 'f', -1, 64))
		}
		limit = t.DownmixBoost > 1
	}
	if normalize {
		filters = append(filters, normalizer, "aresample="+strconv.Itoa(rate))
		limit = true
	}
	if limit {
		filters = append(filters, limiter)
	}
	if !mixed && len(filters) > 0 {
		filters = append([]string{stereoFirst}, filters...)
	}
	return filters
}
