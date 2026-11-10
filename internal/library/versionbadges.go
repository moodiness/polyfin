package library

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

	"github.com/moodiness/polyfin/internal/i18n"
	"github.com/moodiness/polyfin/internal/localization"
	"github.com/moodiness/polyfin/internal/media"
)

// Badge is one thing a version's badges tell of it, the words Polyfin's
// web player shows in its badge row and its short version names write
// (see VersionBadges): Kind, one of the Badge kinds; Text, what the badge
// shows, short and plain ("4K", "DV", "Atmos", "FR"); and Label, the same
// in words, for screen readers and tooltips ("4K Ultra HD", "Dolby
// Vision", "Audio: French").
type Badge struct {
	Kind  string
	Text  string
	Label string
}

// The kinds of badges, in the order a version's come: its resolution, its
// kind of HDR, its video codec, its audio format, its audio channels, its
// edition, and its audio languages.
const (
	BadgeResolution = "resolution"
	BadgeRange      = "range"
	BadgeCodec      = "codec"
	BadgeAudio      = "audio"
	BadgeChannels   = "channels"
	BadgeEdition    = "edition"
	BadgeLanguage   = "language"
)

// mostLanguages is how many audio languages badges name: a version with
// more names its first mostLanguages.
const mostLanguages = 4

// versionTraits are what a version's badges tell (see VersionBadges):
// the height of its video, as a 16:9 video's of its width would be, so
// that a wide 4K movie is 4K; its kind of HDR, of HDRKinds, "hdr" for
// HDR of an unknown kind, empty for SDR or unknown; its video codec; its
// best audio format; its audio languages, by ISO 639-2/B code, and multi
// when its labels say it has more; and its edition.
type versionTraits struct {
	height    int
	hdr       string
	codec     string
	audio     AudioFormat
	languages []string
	multi     bool
	edition   string
}

// VersionBadges are the badges of a version, in the order of their kinds,
// those it tells nothing of left out, their words in language, one of
// i18n.Languages (English for another). known are what describes its
// file, best first: its analysis, then RemuxDB's description. Each thing
// comes from the first of them that tells it, else from the addon's labels
// (see Labels): the analysis wins over the labels, which may be wrong. The
// edition comes from the labels only.
func VersionBadges(version Version, language string, known ...media.Analysis) []Badge {
	t := traitsOf(version, known)
	var badges []Badge
	add := func(kind, text, label string) {
		badges = append(badges, Badge{Kind: kind, Text: text, Label: label})
	}
	if text, label := resolutionWords(t.height, language); text != "" {
		add(BadgeResolution, text, label)
	}
	if t.hdr != "" {
		text, label := rangeWords(t.hdr)
		add(BadgeRange, text, label)
	}
	if t.codec != "" {
		text := codecWords(t.codec)
		add(BadgeCodec, text, fmt.Sprintf(i18n.Text{"%s video", "Vidéo %s", "%s-Video", "Vídeo %s", "Video %s", "Vídeo %s"}.In(language), text))
	}
	if t.audio.Format != "" {
		text, label := audioFormatWords(t.audio.Format)
		add(BadgeAudio, text, label)
	}
	if t.audio.Channels > 0 {
		text, label := channelWords(t.audio.Channels, language)
		add(BadgeChannels, text, label)
	}
	if t.edition != "" {
		text, label := editionBadgeWords(t.edition, language)
		add(BadgeEdition, text, label)
	}
	languages := t.languages
	if len(languages) > mostLanguages {
		languages = languages[:mostLanguages]
	}
	for _, code := range languages {
		add(BadgeLanguage, strings.ToUpper(localization.LanguageTag(code)),
			fmt.Sprintf(i18n.Text{"Audio: %s", "Audio\u202f: %s", "Audio: %s", "Audio: %s", "Audio: %s", "Áudio: %s"}.In(language), languageName(code, language)))
	}
	// MULTI says there are several only when no language is named.
	if t.multi && len(languages) == 0 {
		add(BadgeLanguage, "MULTI", i18n.Text{"Several audio languages", "Plusieurs langues audio", "Mehrere Audiosprachen",
			"Varios idiomas de audio", "Più lingue audio", "Vários idiomas de áudio"}.In(language))
	}
	return badges
}

// traitsOf finds what a version's badges tell (see VersionBadges).
func traitsOf(version Version, known []media.Analysis) versionTraits {
	var t versionTraits
	ranged, heard, spoken := false, false, false
	for _, analysis := range known {
		for _, stream := range analysis.Streams {
			if stream.Type != "video" || stream.AttachedPicture {
				continue
			}
			t.height = cmp.Or(t.height, max(stream.Height, stream.Width*9/16))
			t.codec = cmp.Or(t.codec, stream.Codec)
			if !ranged {
				t.hdr, ranged = streamHDR(stream), true
			}
			break
		}
		if audio, ok := AnalysisAudio(analysis); ok && !heard {
			t.audio, heard = audio, true
		}
		if languages := audioLanguages(analysis); len(languages) > 0 && !spoken {
			t.languages, spoken = languages, true
		}
	}
	labels := version.Labels
	t.height = cmp.Or(t.height, version.Height)
	t.codec = cmp.Or(t.codec, labels.Codec)
	if !ranged && labels.Range == HighRange {
		t.hdr = cmp.Or(labels.HDR, "hdr")
	}
	if !heard {
		t.audio = labels.Audio
	}
	if !spoken {
		t.languages, t.multi = labels.Languages, labels.Multi
	}
	t.edition = labels.Edition
	return t
}

// AnalysisAudio is the best audio format of an analysis's audio tracks
// (see AudioFormat.Better), false when it has none: lossless for Dolby
// TrueHD, FLAC, ALAC, PCM and DTS-HD Master Audio, object based for Dolby
// Atmos and DTS:X, which ffprobe names in the track's profile ("Dolby
// TrueHD + Dolby Atmos", "DTS-HD MA + DTS:X"). Of tracks alike, the one of
// the better Format counts.
func AnalysisAudio(analysis media.Analysis) (AudioFormat, bool) {
	var best AudioFormat
	found := false
	for _, stream := range analysis.Streams {
		if stream.Type != "audio" {
			continue
		}
		profile := strings.ToLower(stream.Profile)
		format := AudioFormat{
			Lossless: slices.Contains([]string{"truehd", "mlp", "flac", "alac"}, stream.Codec) || strings.HasPrefix(stream.Codec, "pcm_") ||
				stream.Codec == "dts" && strings.Contains(profile, "dts-hd ma"),
			Object:   strings.Contains(profile, "atmos") || strings.Contains(profile, "dts:x"),
			Channels: stream.Channels,
			Format:   trackFormat(stream.Codec, profile),
		}
		if !found || format.Better(best) || !best.Better(format) && betterFormat(best.Format, format.Format) != best.Format {
			best, found = format, true
		}
	}
	return best, found
}

// trackFormat is the format of AudioFormats of an audio track, by its
// codec and its profile in lower case, as ffprobe names them; empty for
// another.
func trackFormat(codec, profile string) string {
	switch {
	case strings.Contains(profile, "atmos"):
		return "atmos"
	case strings.Contains(profile, "dts:x"):
		return "dtsx"
	case codec == "truehd":
		return "truehd"
	case codec == "dts" && strings.Contains(profile, "dts-hd ma"):
		return "dtshdma"
	case codec == "dts" && strings.Contains(profile, "dts-hd"):
		return "dtshd"
	case codec == "eac3":
		return "ddp"
	}
	return ""
}

// streamHDR is the kind of HDRKinds of a video track, empty for SDR: Dolby
// Vision when its configuration record describes a base layer with
// metadata, or when the track is tagged as Dolby Vision, as
// playback.HighRange reads it; else by its transfer function, PQ being
// HDR10, or HDR10+ with dynamic metadata, and HLG HLG.
func streamHDR(s media.Stream) string {
	tagged := false
	switch strings.ToLower(s.CodecTag) {
	case "dovi", "dvh1", "dvhe", "dav1":
		tagged = true
	}
	if dv := s.DolbyVision; dv != nil && (dv.RPU && dv.BL || tagged) {
		return "dv"
	}
	switch s.ColorTransfer {
	case "smpte2084":
		if s.HDR10Plus {
			return "hdr10plus"
		}
		return "hdr10"
	case "arib-std-b67":
		return "hlg"
	}
	return ""
}

// audioLanguages are the languages of an analysis's audio tracks, by ISO
// 639-2/B code, in their order, those that state none left out.
func audioLanguages(analysis media.Analysis) []string {
	var languages []string
	for _, stream := range analysis.Streams {
		if stream.Type != "audio" || localization.Undetermined(stream.Language) {
			continue
		}
		if code := localization.LanguageCode(stream.Language); !slices.Contains(languages, code) {
			languages = append(languages, code)
		}
	}
	return languages
}

// resolutionWords are the words of a video's height, empty for none:
// 8K, 4K, 1440p, 1080p, 720p, and SD below.
func resolutionWords(height int, language string) (text, label string) {
	switch {
	case height <= 0:
		return "", ""
	case height >= 3600:
		return "8K", "8K Ultra HD"
	case height >= 1800:
		return "4K", "4K Ultra HD"
	case height >= 1300:
		return "1440p", "1440p Quad HD"
	case height >= 900:
		return "1080p", "1080p Full HD"
	case height >= 650:
		return "720p", "720p HD"
	}
	return "SD", i18n.Text{"Standard definition", "Définition standard", "Standardauflösung", "Definición estándar",
		"Definizione standard", "Definição padrão"}.In(language)
}

// rangeWords are the words of a kind of HDRKinds, or "hdr".
func rangeWords(kind string) (text, label string) {
	switch kind {
	case "dv":
		return "DV", "Dolby Vision"
	case "hdr10plus":
		return "HDR10+", "HDR10+"
	case "hdr10":
		return "HDR10", "HDR10"
	case "hlg":
		return "HLG", "HLG"
	}
	return "HDR", "HDR"
}

// codecNames are the names of the usual video codecs, by ffprobe's name,
// as accounts.VersionCodecs names them too.
var codecNames = map[string]string{"hevc": "HEVC", "h264": "H.264", "av1": "AV1", "vp9": "VP9", "vp8": "VP8", "vc1": "VC-1",
	"mpeg4": "MPEG-4", "mpeg2video": "MPEG-2"}

// codecWords is the name of a video codec, by ffprobe's name: its usual
// name, else ffprobe's in capitals.
func codecWords(codec string) string {
	if name, ok := codecNames[codec]; ok {
		return name
	}
	return strings.ToUpper(codec)
}

// audioFormatWords are the words of a format of AudioFormats.
func audioFormatWords(format string) (text, label string) {
	switch format {
	case "atmos":
		return "Atmos", "Dolby Atmos"
	case "dtsx":
		return "DTS:X", "DTS:X"
	case "truehd":
		return "TrueHD", "Dolby TrueHD"
	case "dtshdma":
		return "DTS-HD MA", "DTS-HD Master Audio"
	case "dtshd":
		return "DTS-HD", "DTS-HD"
	}
	return "DD+", "Dolby Digital Plus"
}

// channelWords are the words of a number of audio channels: Mono for 1
// and Stereo for 2, in language, else 7.1 for 8, 5.1 for 6, and so on,
// the low-frequency channel counted from 6 on, and from 3 for 3.
func channelWords(channels int, language string) (text, label string) {
	switch channels {
	case 1:
		return "Mono", "Mono"
	case 2:
		stereo := i18n.Text{"Stereo", "Stéréo", "Stereo", "Estéreo", "Stereo", "Estéreo"}.In(language)
		return stereo, stereo
	case 3:
		text = "2.1"
	case 4, 5:
		text = strconv.Itoa(channels) + ".0"
	default:
		text = strconv.Itoa(channels-1) + ".1"
	}
	return text, fmt.Sprintf(i18n.Text{"%s surround sound", "Son surround %s", "%s-Surround-Sound", "Sonido envolvente %s",
		"Audio surround %s", "Som surround %s"}.In(language), text)
}

// editionBadgeWords are the words of an edition of accounts.VersionEditions.
func editionBadgeWords(edition, language string) (text, label string) {
	switch edition {
	case "imax":
		return "IMAX", i18n.Text{"IMAX edition", "Version IMAX", "IMAX-Fassung", "Edición IMAX", "Edizione IMAX", "Edição IMAX"}.In(language)
	case "extended":
		return i18n.Text{"Extended", "Version longue", "Extended", "Extendida", "Estesa", "Estendida"}.In(language),
			i18n.Text{"Extended edition", "Version longue", "Erweiterte Fassung", "Edición extendida", "Edizione estesa", "Edição estendida"}.In(language)
	case "directors":
		return i18n.Text{"Director’s Cut", "Director’s cut", "Director’s Cut", "Montaje del director", "Director’s cut", "Versão do diretor"}.In(language),
			i18n.Text{"Director’s cut", "Version du réalisateur", "Director’s Cut", "Montaje del director", "Versione del regista", "Versão do diretor"}.In(language)
	}
	return i18n.Text{"Theatrical", "Cinéma", "Kinofassung", "Cines", "Cinema", "Cinema"}.In(language),
		i18n.Text{"Theatrical cut", "Version cinéma", "Kinofassung", "Versión de cines", "Versione cinematografica", "Versão de cinema"}.In(language)
}

// languageName names a language, by its ISO 639-2/B code, in language,
// one of i18n.Languages: as the Unicode CLDR names it, else by its English
// name, else by its code.
func languageName(code, in string) string {
	tag := localization.LanguageTag(code)
	if namer := display.Languages(language.Make(cmp.Or(i18n.Match(in), "en"))); namer != nil {
		if name := namer.Name(language.Make(tag)); name != "" {
			return name
		}
	}
	for _, l := range localization.Languages() {
		if l.Bibliographic == code {
			name, _, _ := strings.Cut(l.Name, ";")
			return name
		}
	}
	return code
}
