package library

import (
	"path"
	"slices"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/media"
)

// While a version's tracks are unknown, neither analyzed nor described,
// its labels often tell enough of them to estimate how an app plays it
// (see LabelAnalyses): its video codec, height and kind of HDR, and its
// audio codecs.

// Audio codecs the labels of a stream may name (see ReadAudioCodecs): as
// ffprobe names them, Dolby TrueHD and DTS-HD Master Audio apart from DTS
// and DTS-HD High Resolution Audio, whose tracks ffprobe names "dts" with
// a profile, and PCM.
const (
	LabelTrueHD = "truehd"
	LabelDTSMA  = "dtshdma"
	LabelDTSHD  = "dtshd"
	LabelDTS    = "dts"
	LabelEAC3   = "eac3"
	LabelAC3    = "ac3"
	LabelAAC    = "aac"
	LabelFLAC   = "flac"
	LabelOpus   = "opus"
	LabelMP3    = "mp3"
	LabelPCM    = "pcm"
)

// audioCodecWords are the words naming an audio codec of the labels, in
// lower case. Words of letters then digits ("AAC2", "DDP5", "DD5", from
// "AAC2.0", "DDP5.1", "DD5.1") and runs of words ("DTS-HD MA", "E-AC-3",
// "Dolby Digital Plus") are read by audioCodecAt. Atmos alone names no
// codec: Dolby TrueHD and Dolby Digital Plus both carry it.
var audioCodecWords = map[string]string{
	"truehd": LabelTrueHD, "dtshdma": LabelDTSMA, "dtsma": LabelDTSMA, "dtsx": LabelDTSMA, "dtshd": LabelDTSHD,
	"eac3": LabelEAC3, "ac3": LabelAC3, "aac": LabelAAC, "flac": LabelFLAC, "opus": LabelOpus, "mp3": LabelMP3,
	"lpcm": LabelPCM, "pcm": LabelPCM,
}

// ReadAudioCodecs reads the audio codecs the labels of a stream name, of
// the Label constants, each once, in the order they are named: an
// addon's name, title, description or file name, each line apart, after
// the title's own words, as ReadRelease reads them (titles are the same).
// A release in two languages may name two.
func ReadAudioCodecs(titles []string, labels ...string) []string {
	var named [][]string
	for _, title := range titles {
		if words := lowerWords(title); len(words) > 0 {
			named = append(named, words)
		}
	}
	var codecs []string
	add := func(codec string) {
		if codec != "" && !slices.Contains(codecs, codec) {
			codecs = append(codecs, codec)
		}
	}
	for _, label := range labels {
		for line := range strings.Lines(label) {
			words := lowerWords(line)
			words = words[titleEnd(words, named):]
			plus := plusAfter(line, "dd")
			for i := range words {
				add(audioCodecAt(words, i, plus))
			}
		}
	}
	return codecs
}

// audioCodecAt is the audio codec the words starting at words[i] name,
// empty for none. plus tells that their line writes "DD+", Dolby
// Digital Plus, whose sign is no word's: "DD" there is no Dolby Digital.
func audioCodecAt(words []string, i int, plus bool) string {
	word := words[i]
	next := ""
	if i+1 < len(words) {
		next = words[i+1]
	}
	prefixed := func(prefix string) bool {
		return strings.HasPrefix(word, prefix) && len(word) > len(prefix) && strings.Trim(word[len(prefix):], "0123456789") == ""
	}
	switch {
	case word == "dts":
		switch {
		case runAt(words, i, [][]string{{"dts", "hd", "ma"}}), next == "x", next == "ma":
			return LabelDTSMA
		case next == "hd":
			return LabelDTSHD
		}
		return LabelDTS
	case word == "dtshd" && next == "ma":
		return LabelDTSMA
	case word == "true" && next == "hd":
		return LabelTrueHD
	case word == "e" && runAt(words, i, [][]string{{"e", "ac", "3"}}):
		return LabelEAC3
	case word == "dolby" && runAt(words, i, [][]string{{"dolby", "digital", "plus"}}):
		return LabelEAC3
	case word == "dolby" && next == "digital":
		return LabelAC3
	case word == "ddp" || prefixed("ddp"):
		return LabelEAC3
	case word == "dd" || prefixed("dd"):
		if plus {
			return LabelEAC3
		}
		// "DD" alone may be anything; with its channels it is Dolby
		// Digital: "DD5.1", "DD 5.1", "DD2.0".
		if prefixed("dd") || labelChannels("x"+next, nextOf(words, i+1)) > 0 {
			return LabelAC3
		}
		return ""
	case prefixed("aac"):
		return LabelAAC
	}
	return audioCodecWords[word]
}

// nextOf is words[i], empty past their end.
func nextOf(words []string, i int) string {
	if i+1 < len(words) {
		return words[i+1]
	}
	return ""
}

// maxLabelAnalyses bounds the analyses LabelAnalyses returns: labels
// leaving that much open tell too little.
const maxLabelAnalyses = 48

// LabelAnalyses are the analyses a version's file may have, as far as its
// labels tell: one for each way of filling what they leave open, so that
// an estimate made from them holds only when it is the same for all. The
// labels must name the video codec and at least one audio codec; nil
// otherwise, or when they leave too much open. runtime is the title's,
// for the bitrate its size gives. The rest is typical of such files:
//   - the container is the file name's, else Matroska or MP4;
//   - the height is the labels', else 1080 or 2160 lines, 16:9;
//   - HEVC is Main 10 when HDR, else Main or Main 10; H.264 is High;
//   - the HDR is the labels' kind, "HDR" alone being HDR10, and Dolby
//     Vision any of profiles 5, 7 and 8.1, but 5 when the labels name an
//     HDR10 base layer (see Labels.HDR10Base);
//   - each audio codec named, one track, with the labels' channels, else
//     2 or 6.
//
// No subtitle track is listed: the labels do not tell them.
func LabelAnalyses(version Version, runtime time.Duration) []media.Analysis {
	labels := version.Labels
	if labels.Codec == "" || len(labels.AudioCodecs) == 0 {
		return nil
	}
	formats := []string{"matroska,webm", "mov,mp4,m4a,3gp,3g2,mj2"}
	switch strings.ToLower(strings.TrimPrefix(path.Ext(version.Filename), ".")) {
	case "mkv", "webm":
		formats = formats[:1]
	case "mp4", "m4v", "mov":
		formats = formats[1:]
	case "avi":
		formats = []string{"avi"}
	case "ts", "m2ts", "mts":
		formats = []string{"mpegts"}
	}
	heights := []int{version.Height}
	if version.Height == 0 {
		heights = []int{1080, 2160}
	}
	high := labels.Range == HighRange || labels.HDR != ""
	videos := []media.Stream{}
	for _, height := range heights {
		base := media.Stream{Index: 0, Type: "video", Codec: labels.Codec, Width: height * 16 / 9, Height: height, Default: true,
			PixelFormat: "yuv420p", BitDepth: 8, ColorTransfer: "bt709", ColorPrimaries: "bt709", ColorSpace: "bt709",
			FrameRate: 23.976, AverageRate: 23.976, RefFrames: 1}
		switch labels.Codec {
		case "hevc":
			base.Profile, base.Level = "Main", 120
			if height > 1080 {
				base.Level = 153
			}
		case "h264":
			base.Profile, base.Level, base.IsAVC = "High", 41, true
			if height > 1080 {
				base.Level = 51
			}
		case "av1":
			base.Profile, base.Level = "Main", 8
		case "vp9":
			base.Profile = "Profile 0"
		}
		if !high {
			videos = append(videos, base)
			if labels.Codec == "hevc" {
				videos = append(videos, tenBit(base))
			}
			continue
		}
		hdr := tenBit(base)
		hdr.ColorPrimaries, hdr.ColorSpace, hdr.ColorTransfer = "bt2020", "bt2020nc", "smpte2084"
		switch labels.HDR {
		case "hlg":
			hdr.ColorTransfer = "arib-std-b67"
			videos = append(videos, hdr)
		case "hdr10plus":
			hdr.HDR10Plus = true
			videos = append(videos, hdr)
		case "dv":
			profiles := []media.DolbyVision{{Profile: 5}, {Profile: 7, Compatibility: 6, EL: true}, {Profile: 8, Compatibility: 1}}
			if labels.HDR10Base {
				profiles = profiles[1:]
			}
			for _, dv := range profiles {
				dv.VersionMajor, dv.Level, dv.RPU, dv.BL = 1, 6, true, true
				stream := hdr
				stream.DolbyVision = &dv
				videos = append(videos, stream)
			}
		default:
			videos = append(videos, hdr)
		}
	}
	channels := []int{labels.Audio.Channels}
	if labels.Audio.Channels == 0 {
		channels = []int{2, 6}
	}
	var audios []media.Stream
	for _, codec := range labels.AudioCodecs {
		for _, count := range channels {
			audios = append(audios, labelAudio(codec, count))
		}
	}
	if len(formats)*len(videos)*len(audios) > maxLabelAnalyses {
		return nil
	}
	var bitrate int64
	if version.Size > 0 && runtime > 0 {
		bitrate = int64(float64(version.Size*8) / runtime.Seconds())
	}
	var analyses []media.Analysis
	for _, format := range formats {
		for _, video := range videos {
			for _, audio := range audios {
				analyses = append(analyses, media.Analysis{Format: format, Duration: runtime, Size: version.Size, Bitrate: bitrate,
					Streams: []media.Stream{video, audio}, Remote: true})
			}
		}
	}
	return analyses
}

// tenBit is a video track in 10 bits: HEVC's Main 10.
func tenBit(stream media.Stream) media.Stream {
	stream.BitDepth, stream.PixelFormat = 10, "yuv420p10le"
	if stream.Codec == "hevc" {
		stream.Profile = "Main 10"
	}
	return stream
}

// labelAudio is the audio track of a codec the labels name, with
// channels, as ffprobe describes one.
func labelAudio(codec string, channels int) media.Stream {
	stream := media.Stream{Index: 1, Type: "audio", Codec: codec, Channels: channels, Default: true}
	switch codec {
	case LabelDTSMA:
		stream.Codec, stream.Profile = "dts", "DTS-HD MA"
	case LabelDTSHD:
		stream.Codec, stream.Profile = "dts", "DTS-HD HRA"
	case LabelDTS:
		stream.Profile = "DTS"
	case LabelTrueHD:
		stream.Profile = "Dolby TrueHD"
	case LabelAAC:
		stream.Profile = "LC"
	case LabelPCM:
		stream.Codec = "pcm_s16le"
	}
	return stream
}
