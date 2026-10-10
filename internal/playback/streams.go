package playback

import (
	"cmp"
	"path"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/media"
)

// Rules below were observed on Jellyfin 12.2, with the oracle's media and
// with clips made for the purpose, under UICulture en-US and fr. Those
// marked [INFERENCE] could not be observed: ffmpeg cannot produce Dolby
// Vision, HDR10+, Atmos or DTS:X tracks, nor DVD or DVB subtitles.

// ExternalSubtitle is a subtitle file offered beside the video (by an
// addon, or added by a user), listed before the container's tracks.
type ExternalSubtitle struct {
	// Language is as the addon gives it: ISO 639-2/B or /T, ISO 639-1, or
	// an English or French name.
	Language string
	// Title is optional.
	Title string
	// Codec is "subrip", "webvtt", "ass" or "ssa".
	Codec string
	// Forced and HearingImpaired are the flags a user gave a file they
	// added.
	Forced, HearingImpaired bool
}

// MediaStreams describes an analyzed version's tracks as Jellyfin does: the
// external subtitles first (indexes 0…k-1), then the container's tracks
// shifted by k. language is the server language ("en" or "fr") for
// DisplayTitle and Localized* strings.
func MediaStreams(analysis media.Analysis, externals []ExternalSubtitle, language string) []MediaStream {
	w := wordsFor(language)
	streams := make([]MediaStream, 0, len(externals)+len(analysis.Streams))
	streams = appendExternals(streams, externals, w)
	shift := len(externals)
	video := true // the first video track still to see
	for _, s := range analysis.Streams {
		index := s.Index + shift
		switch {
		case s.Type == "video" && s.AttachedPicture:
			streams = append(streams, imageStream(s, index))
		case s.Type == "video":
			stream := videoStream(s, index, analysis.Remote)
			if video && stream.BitRate == nil {
				stream.BitRate = derivedVideoBitrate(analysis)
			}
			video = false
			streams = append(streams, stream)
		case s.Type == "audio":
			streams = append(streams, audioStream(s, index, w))
		case s.Type == "subtitle":
			streams = append(streams, subtitleStream(s, index, w))
		case s.Type == "data":
			streams = append(streams, dataStream(s, index))
		}
		// Attachments, fonts mostly, are not tracks for Jellyfin.
	}
	return streams
}

// ExternalStreams describes only the external subtitles, for versions not
// analyzed yet.
func ExternalStreams(externals []ExternalSubtitle, language string) []MediaStream {
	return appendExternals(make([]MediaStream, 0, len(externals)), externals, wordsFor(language))
}

// Container is Jellyfin's container value for a source: the probe's format
// names normalized as Jellyfin does ("mkv", "mkv,webm",
// "mov,mp4,m4a,3gp,3g2,mj2"). Jellyfin keeps "webm" only when every video,
// audio and subtitle track can be in WebM, whatever the file's extension:
// a .mkv file of VP9 and Opus is "mkv,webm", and a .webm file of H.264 is
// "mkv".
func Container(analysis media.Analysis) string {
	var b strings.Builder
	for name := range strings.SplitSeq(analysis.Format, ",") {
		switch strings.ToLower(name) {
		case "matroska":
			name = "mkv"
		case "mpegts": // [INFERENCE]
			name = "ts"
		case "mpegvideo": // [INFERENCE]
			name = "mpeg"
		case "webm":
			if !webmCodecs(analysis.Streams) {
				continue
			}
		}
		if name == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(name)
	}
	return b.String()
}

// DisplayContainer is the single name item detail reports ("mkv", "mp4",
// "webm"): the file name's extension when Container lists it, else the
// first name Container lists. A .webm file of H.264 is "mkv", and a .mov
// file is "mov".
func DisplayContainer(analysis media.Analysis, filename string) string {
	container := Container(analysis)
	extension := strings.ToLower(strings.TrimPrefix(path.Ext(filename), "."))
	first := ""
	for name := range strings.SplitSeq(container, ",") {
		if first == "" {
			first = name
		}
		if extension != "" && name == extension {
			return name
		}
	}
	return first
}

// webmCodecs reports whether every track could be in a WebM file.
func webmCodecs(streams []media.Stream) bool {
	for _, s := range streams {
		var ok bool
		switch s.Type {
		case "video":
			ok = s.Codec == "vp8" || s.Codec == "vp9" || s.Codec == "av1"
		case "audio":
			ok = s.Codec == "opus" || s.Codec == "vorbis"
		case "subtitle":
			ok = s.Codec == "webvtt" // [INFERENCE]: subrip is refused
		default:
			ok = true
		}
		if !ok {
			return false
		}
	}
	return true
}

// words are Jellyfin's localized stream strings.
type words struct {
	undefined, byDefault, forced, external, hearingImpaired, original string
}

var (
	englishWords = words{"Undefined", "Default", "Forced", "External", "Hearing Impaired", "Original"}
	// frenchWords were observed with UICulture=fr. Language and channel
	// layout names stay in English there.
	frenchWords = words{"Non défini", "Par défaut", "Forcé", "Externe", "Malentendants", "Original"}
)

func wordsFor(language string) *words {
	if language == "fr" || strings.HasPrefix(language, "fr-") {
		return &frenchWords
	}
	return &englishWords
}

func appendExternals(streams []MediaStream, externals []ExternalSubtitle, w *words) []MediaStream {
	for i, external := range externals {
		stream := MediaStream{
			Codec:                  external.Codec,
			Language:               addonLanguage(external.Language),
			TimeBase:               "1/1000",
			Title:                  external.Title,
			Type:                   "Subtitle",
			Index:                  i,
			IsExternal:             true,
			IsForced:               external.Forced,
			IsHearingImpaired:      external.HearingImpaired,
			IsTextSubtitleStream:   true,
			SupportsExternalStream: true,
		}
		finishSubtitle(&stream, w)
		streams = append(streams, stream)
	}
	return streams
}

func videoStream(s media.Stream, index int, remote bool) MediaStream {
	stream := MediaStream{
		Codec:             s.Codec,
		CodecTag:          codecTag(s.CodecTag),
		Language:          streamLanguage(s.Language),
		ColorSpace:        s.ColorSpace,
		ColorTransfer:     s.ColorTransfer,
		ColorPrimaries:    s.ColorPrimaries,
		TimeBase:          s.TimeBase,
		Title:             s.Title,
		NalLengthSize:     s.NalLengthSize,
		IsInterlaced:      s.FieldOrder != "" && s.FieldOrder != "progressive" && s.FieldOrder != "unknown",
		IsDefault:         s.Default,
		IsForced:          s.Forced,
		IsHearingImpaired: s.HearingImpaired,
		IsOriginal:        s.Original,
		Profile:           profile(s.Profile),
		Type:              "Video",
		AspectRatio:       aspectRatio(s.AspectRatio, s.Width, s.Height),
		Index:             index,
		PixelFormat:       s.PixelFormat,
		Level:             new(float64(s.Level)),
		// Jellyfin reports anamorphic any sample aspect ratio but square
		// pixels; an unknown one ("0:1" or none) is not.
		IsAnamorphic:       new(s.SampleAspect != "" && s.SampleAspect != "1:1" && s.SampleAspect != "0:1"),
		AudioSpatialFormat: "None",
	}
	if dv := s.DolbyVision; dv != nil {
		stream.DvVersionMajor = new(dv.VersionMajor)
		stream.DvVersionMinor = new(dv.VersionMinor)
		stream.DvProfile = new(dv.Profile)
		stream.DvLevel = new(dv.Level)
		stream.RpuPresentFlag = new(bit(dv.RPU))
		stream.ElPresentFlag = new(bit(dv.EL))
		stream.BlPresentFlag = new(bit(dv.BL))
		stream.DvBlSignalCompatibilityId = new(dv.Compatibility)
		stream.VideoDoViTitle = doviTitle(dv)
	}
	if s.HDR10Plus {
		stream.Hdr10PlusPresentFlag = new(true)
	}
	stream.VideoRange, stream.VideoRangeType = videoRange(s)
	stream.MasteringDisplay = s.MasteringDisplay
	if s.Codec == "h264" {
		stream.IsAVC = new(s.IsAVC)
	}
	if s.Bitrate > 0 {
		stream.BitRate = new(s.Bitrate)
	}
	if s.BitDepth > 0 {
		stream.BitDepth = new(s.BitDepth)
	}
	// Jellyfin reports one reference frame for every video it reads from a
	// file, and none for a remote source (observed on the oracle's .strm).
	if !remote {
		stream.RefFrames = new(max(s.RefFrames, 1))
	}
	if s.Width > 0 && s.Height > 0 {
		stream.Width, stream.Height = new(s.Width), new(s.Height)
	}
	stream.AverageFrameRate = frameRate(s.AverageRate)
	stream.RealFrameRate = frameRate(s.FrameRate)
	// The reference rate is the average one, unless it is implausible.
	if stream.AverageFrameRate != nil && *stream.AverageFrameRate < 1000 {
		stream.ReferenceFrameRate = stream.AverageFrameRate
	} else {
		stream.ReferenceFrameRate = stream.RealFrameRate
	}

	var attributes [3]string
	attributes[0] = resolutionLabel(s.Width, s.Height, stream.IsInterlaced)
	attributes[1] = strings.ToUpper(s.Codec)
	if stream.VideoDoViTitle != "" {
		attributes[2] = stream.VideoDoViTitle
	} else {
		attributes[2] = stream.VideoRange
	}
	stream.DisplayTitle = displayTitle(s.Title, " ", attributes[:])
	return stream
}

func audioStream(s media.Stream, index int, w *words) MediaStream {
	stream := MediaStream{
		Codec:              s.Codec,
		CodecTag:           codecTag(s.CodecTag),
		Language:           streamLanguage(s.Language),
		TimeBase:           s.TimeBase,
		Title:              s.Title,
		VideoRange:         "Unknown",
		VideoRangeType:     "Unknown",
		AudioSpatialFormat: spatialFormat(s.Profile),
		LocalizedDefault:   w.byDefault,
		LocalizedExternal:  w.external,
		LocalizedOriginal:  w.original,
		ChannelLayout:      channelLayout(s.ChannelLayout),
		IsDefault:          s.Default,
		IsForced:           s.Forced,
		IsHearingImpaired:  s.HearingImpaired,
		IsOriginal:         s.Original,
		Profile:            profile(s.Profile),
		Type:               "Audio",
		Index:              index,
	}
	stream.LocalizedLanguage = languageName(stream.Language)
	if bitrate := audioBitrate(s); bitrate > 0 {
		stream.BitRate = new(bitrate)
	}
	if depth := cmp.Or(s.BitDepth, s.SampleBits); depth > 0 {
		stream.BitDepth = new(depth)
	}
	if s.Channels > 0 {
		stream.Channels = new(s.Channels)
	}
	if s.SampleRate > 0 {
		stream.SampleRate = new(s.SampleRate)
	}

	var attributes [5]string
	if stream.Language != "" && !isSpecialLanguage(stream.Language) {
		attributes[0] = languageLabel(stream.Language, stream.LocalizedLanguage)
	}
	if stream.Profile != "" && !strings.EqualFold(stream.Profile, "lc") {
		attributes[1] = stream.Profile
	} else {
		attributes[1] = audioCodecName(s.Codec)
	}
	if stream.ChannelLayout != "" {
		attributes[2] = capitalized(stream.ChannelLayout)
	} else if s.Channels > 0 {
		attributes[2] = strconv.Itoa(s.Channels) + " ch"
	}
	if s.Default {
		attributes[3] = w.byDefault
	}
	if s.Original {
		attributes[4] = w.original
	}
	stream.DisplayTitle = displayTitle(s.Title, " - ", attributes[:])
	return stream
}

func subtitleStream(s media.Stream, index int, w *words) MediaStream {
	codec := subtitleCodec(s.Codec)
	stream := MediaStream{
		Codec:                codec,
		CodecTag:             codecTag(s.CodecTag),
		Language:             streamLanguage(s.Language),
		TimeBase:             s.TimeBase,
		Title:                s.Title,
		IsDefault:            s.Default,
		IsForced:             s.Forced,
		IsHearingImpaired:    s.HearingImpaired,
		IsOriginal:           s.Original,
		Type:                 "Subtitle",
		Index:                index,
		IsTextSubtitleStream: !imageSubtitle(codec),
		// Observed for text subtitles and PGS alike.
		SupportsExternalStream: true,
	}
	// Bitmap subtitles have the size of the video they cover.
	if s.Width > 0 && s.Height > 0 {
		stream.Width, stream.Height = new(s.Width), new(s.Height)
	}
	if s.Bitrate > 0 {
		stream.BitRate = new(s.Bitrate)
	}
	finishSubtitle(&stream, w)
	return stream
}

// finishSubtitle fills what embedded and external subtitles share.
func finishSubtitle(stream *MediaStream, w *words) {
	stream.VideoRange, stream.VideoRangeType = "Unknown", "Unknown"
	stream.AudioSpatialFormat = "None"
	stream.LocalizedUndefined = w.undefined
	stream.LocalizedDefault = w.byDefault
	stream.LocalizedForced = w.forced
	stream.LocalizedExternal = w.external
	stream.LocalizedHearingImpaired = w.hearingImpaired
	stream.LocalizedLanguage = languageName(stream.Language)

	// Unlike audio, subtitles show und and zxx.
	var attributes [6]string
	if stream.Language != "" {
		attributes[0] = languageLabel(stream.Language, stream.LocalizedLanguage)
	} else {
		attributes[0] = w.undefined
	}
	if stream.IsHearingImpaired {
		attributes[1] = w.hearingImpaired
	}
	if stream.IsDefault {
		attributes[2] = w.byDefault
	}
	if stream.IsForced {
		attributes[3] = w.forced
	}
	attributes[4] = strings.ToUpper(stream.Codec)
	if stream.IsExternal {
		attributes[5] = w.external
	}
	stream.DisplayTitle = displayTitle(stream.Title, " - ", attributes[:])
}

// imageStream describes cover art. [INFERENCE]: Jellyfin's fields for it.
func imageStream(s media.Stream, index int) MediaStream {
	stream := MediaStream{
		Codec:              s.Codec,
		CodecTag:           codecTag(s.CodecTag),
		Language:           streamLanguage(s.Language),
		TimeBase:           s.TimeBase,
		Title:              s.Title,
		VideoRange:         "Unknown",
		VideoRangeType:     "Unknown",
		AudioSpatialFormat: "None",
		IsDefault:          s.Default,
		Profile:            profile(s.Profile),
		Type:               "EmbeddedImage",
		Index:              index,
		PixelFormat:        s.PixelFormat,
	}
	if s.Width > 0 && s.Height > 0 {
		stream.Width, stream.Height = new(s.Width), new(s.Height)
	}
	return stream
}

// dataStream describes a data track, such as an MP4 timecode. [INFERENCE]
func dataStream(s media.Stream, index int) MediaStream {
	return MediaStream{
		Codec:              s.Codec,
		CodecTag:           codecTag(s.CodecTag),
		Language:           streamLanguage(s.Language),
		TimeBase:           s.TimeBase,
		Title:              s.Title,
		VideoRange:         "Unknown",
		VideoRangeType:     "Unknown",
		AudioSpatialFormat: "None",
		Type:               "Data",
		Index:              index,
	}
}

// displayTitle joins a track's non-empty attributes with sep. A track with
// a title shows it first, followed by the attributes it does not already
// contain in any letter case ("Surround 5.1 - English - Dolby Digital"),
// always separated by " - ".
func displayTitle(title, sep string, attributes []string) string {
	var b strings.Builder
	if title != "" {
		b.WriteString(title)
		lower := strings.ToLower(title)
		for _, attribute := range attributes {
			if attribute != "" && !strings.Contains(lower, strings.ToLower(attribute)) {
				b.WriteString(" - ")
				b.WriteString(attribute)
			}
		}
		return b.String()
	}
	for _, attribute := range attributes {
		if attribute == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString(sep)
		}
		b.WriteString(attribute)
	}
	return b.String()
}

// resolutions are the labels Jellyfin gives a video, each for the sizes
// that fit within its width and height. 2560x1440 is "1080p": there is no
// 1440p label.
var resolutions = [...]struct {
	width, height int
	label         string
}{
	{256, 144, "144"},
	{426, 240, "240"},
	{640, 360, "360"},
	{682, 384, "384"},
	{720, 404, "404"},
	{854, 480, "480"},
	{960, 544, "540"},
	{1024, 576, "576"},
	{1280, 962, "720"},
	{2560, 1440, "1080"},
	{4096, 3072, "4K"},
	{8192, 6144, "8K"}, // [INFERENCE] beyond 4096x3072 to 4500x2160
}

// resolutionLabel is a video's resolution as display titles show it:
// "1080p", "576i", "4K"; "" when the size is unknown or beyond 8K.
func resolutionLabel(width, height int, interlaced bool) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	for _, r := range resolutions {
		if width > r.width || height > r.height {
			continue
		}
		switch {
		case strings.HasSuffix(r.label, "K"):
			return r.label
		case interlaced:
			return r.label + "i"
		default:
			return r.label + "p"
		}
	}
	return ""
}

// aspectRatios are the ratios Jellyfin names, in the order it tries them,
// with how far a ratio may be from each. Tolerances other than those of
// 16:9, 4:3, 1.85:1, 2.35:1 and 2.40:1 are [INFERENCE].
var aspectRatios = [...]struct {
	ratio, tolerance float64
	name             string
}{
	{16.0 / 9, 0.03, "16:9"},
	{4.0 / 3, 0.05, "4:3"},
	{1.41, 0.005, "1.41:1"},
	{1.5, 0.005, "1.5:1"},
	{1.6, 0.005, "1.6:1"},
	{5.0 / 3, 0.005, "5:3"},
	{1.85, 0.02, "1.85:1"},
	{2.35, 0.025, "2.35:1"},
	{2.4, 0.025, "2.40:1"},
}

// aspectRatio is a video's AspectRatio: the name of a common ratio close
// to its display aspect ratio, or to its size when ffprobe gave none, else
// the display aspect ratio as ffprobe wrote it ("19:5").
func aspectRatio(display string, width, height int) string {
	w, h := width, height
	if a, b, ok := strings.Cut(display, ":"); ok {
		x, errX := strconv.Atoi(a)
		y, errY := strconv.Atoi(b)
		if errX == nil && errY == nil && x > 0 && y > 0 {
			w, h = x, y
		}
	}
	if w > 0 && h > 0 {
		ratio := float64(w) / float64(h)
		for _, r := range aspectRatios {
			if ratio >= r.ratio-r.tolerance && ratio <= r.ratio+r.tolerance {
				return r.name
			}
		}
	}
	if display == "0:1" {
		return ""
	}
	return display
}

// frameRate is a frame rate as Jellyfin stores it, in single precision:
// 30000/1001 is 29.97003.
func frameRate(rate float64) *float64 {
	if rate <= 0 {
		return nil
	}
	rounded, _ := strconv.ParseFloat(strconv.FormatFloat(rate, 'g', -1, 32), 64)
	return &rounded
}

// videoRange is a video's dynamic range and its kind. Dolby Vision comes
// from the configuration record when it describes a base layer with
// metadata, or when the track is tagged as Dolby Vision (dvh1, dvhe, dav1
// or dovi), which Jellyfin 12.2 reads: profile 5 is DOVI, 7 DOVIWithEL, 8
// and 10 depend on their base layer's compatibility. Without it, the
// transfer function decides: PQ is HDR10 (HDR10Plus with dynamic
// metadata), HLG is HLG, the rest SDR. [INFERENCE]: every Dolby Vision and
// HDR10+ case, as no such stream could be observed; a record Jellyfin
// would not classify falls back to the transfer function.
func videoRange(s media.Stream) (videoRange, rangeType string) {
	if dv := s.DolbyVision; dv != nil && (dv.RPU && dv.BL || doviTag(s.CodecTag)) {
		switch dv.Profile {
		case 5:
			return "HDR", "DOVI"
		case 7:
			if s.HDR10Plus {
				return "HDR", "DOVIWithELHDR10Plus"
			}
			return "HDR", "DOVIWithEL"
		case 8, 10:
			switch dv.Compatibility {
			case 0:
				if dv.Profile == 10 {
					return "HDR", "DOVI"
				}
			case 1, 6:
				if s.HDR10Plus {
					return "HDR", "DOVIWithHDR10Plus"
				}
				return "HDR", "DOVIWithHDR10"
			case 2:
				return "SDR", "DOVIWithSDR"
			case 4:
				return "HDR", "DOVIWithHLG"
			}
		}
	}
	switch s.ColorTransfer {
	case "smpte2084":
		if s.HDR10Plus {
			return "HDR", "HDR10Plus"
		}
		return "HDR", "HDR10"
	case "arib-std-b67":
		return "HDR", "HLG"
	}
	return "SDR", "SDR"
}

// doviTitle names a Dolby Vision configuration in display titles, such as
// "Dolby Vision Profile 8.1 (HDR10)". [INFERENCE]
func doviTitle(dv *media.DolbyVision) string {
	switch dv.Profile {
	case 4, 5, 7, 8, 9, 10:
	default:
		return ""
	}
	if !dv.RPU || !dv.BL {
		return ""
	}
	title := "Dolby Vision Profile " + strconv.Itoa(dv.Profile)
	if dv.Compatibility > 0 {
		title += "." + strconv.Itoa(dv.Compatibility)
	}
	switch dv.Compatibility {
	case 1, 6:
		return title + " (HDR10)"
	case 2:
		return title + " (SDR)"
	case 4:
		return title + " (HLG)"
	}
	return title
}

// spatialFormat is an audio track's object-based format, which ffprobe
// names in its profile ("Dolby TrueHD + Dolby Atmos", "DTS-HD MA + DTS:X").
// [INFERENCE]: no such track could be observed.
func spatialFormat(profile string) string {
	lower := strings.ToLower(profile)
	switch {
	case strings.Contains(lower, "atmos"):
		return "DolbyAtmos"
	case strings.Contains(lower, "dts:x"):
		return "DTSX"
	}
	return "None"
}

// profile is a track's profile as Jellyfin reports it: ffprobe's name, but
// not "unknown".
func profile(name string) string {
	if strings.EqualFold(name, "unknown") {
		return ""
	}
	return name
}

// codecTag is the codec tag Jellyfin reports of a track: ffprobe's, but
// none for the tags made of zero bytes ("[0][0][0][0]") Matroska and WebM
// tracks have.
func codecTag(tag string) string {
	if strings.TrimSpace(tag) == "" || strings.Contains(tag, "[0]") {
		return ""
	}
	return tag
}

// doviTag reports a codec tag naming Dolby Vision.
func doviTag(tag string) bool {
	switch strings.ToLower(tag) {
	case "dovi", "dvh1", "dvhe", "dav1":
		return true
	}
	return false
}

// channelLayout drops the speaker variant: "5.1(side)" is "5.1".
func channelLayout(layout string) string {
	if i := strings.IndexByte(layout, '('); i > 0 {
		return layout[:i]
	}
	return layout
}

// audioCodecName is how display titles name an audio codec whose profile
// says nothing more ("TRUEHD", "OPUS", "PCM_S24LE").
func audioCodecName(codec string) string {
	switch codec {
	case "ac3":
		return "Dolby Digital"
	case "eac3":
		return "Dolby Digital+"
	case "dca", "dts":
		return "DTS"
	}
	return strings.ToUpper(codec)
}

// audioBitrate is an audio track's bitrate, or the one Jellyfin assumes
// for a track that states none, as Matroska tracks muxed by ffmpeg do.
// The assumed values were observed: AAC 192 kb/s up to stereo and 320 kb/s
// above, Opus 128 and 256 kb/s, FLAC and ALAC 480 kb/s per channel, TrueHD
// 700 kb/s per channel. Opus with three to five channels is [INFERENCE].
func audioBitrate(s media.Stream) int64 {
	if s.Bitrate > 0 || s.Channels <= 0 {
		return s.Bitrate
	}
	channels := int64(s.Channels)
	switch s.Codec {
	case "aac":
		if channels <= 2 {
			return 192000
		}
		return 320000
	case "opus":
		if channels <= 2 {
			return 128000
		}
		return 256000
	case "flac", "alac":
		return 480000 * channels
	case "truehd":
		return 700000 * channels
	}
	return 0
}

// derivedVideoBitrate is the bitrate Jellyfin gives a video track that
// states none: the source's bitrate minus its audio tracks', or none when
// that leaves nothing.
func derivedVideoBitrate(analysis media.Analysis) *int64 {
	if analysis.Bitrate <= 0 {
		return nil
	}
	bitrate := analysis.Bitrate
	for _, s := range analysis.Streams {
		if s.Type == "audio" {
			bitrate -= audioBitrate(s)
		}
	}
	if bitrate <= 0 {
		return nil
	}
	return &bitrate
}

// subtitleCodec names bitmap subtitle codecs as Jellyfin does: PGS is
// "PGSSUB" (observed), DVD, DVB and teletext subtitles "DVDSUB", "DVBSUB"
// and "DVBTXT" ([INFERENCE]).
func subtitleCodec(codec string) string {
	switch codec {
	case "hdmv_pgs_subtitle":
		return "PGSSUB"
	case "dvd_subtitle":
		return "DVDSUB"
	case "dvb_subtitle":
		return "DVBSUB"
	case "dvb_teletext":
		return "DVBTXT"
	}
	return codec
}

// imageSubtitle reports whether a subtitle codec draws bitmaps.
func imageSubtitle(codec string) bool {
	switch strings.ToLower(codec) {
	case "pgssub", "dvdsub", "dvbsub", "dvbtxt", "xsub", "vobsub":
		return true
	}
	return false
}

// bit is 1 for true, as Jellyfin writes Dolby Vision flags.
func bit(b bool) int {
	if b {
		return 1
	}
	return 0
}
