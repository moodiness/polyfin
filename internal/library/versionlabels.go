package library

import (
	"slices"
	"strings"
	"unicode"

	"github.com/moodiness/polyfin/internal/localization"
)

// DynamicRange is what is known of a version's video range.
type DynamicRange int

const (
	// UnknownRange is a version nothing tells the range of.
	UnknownRange DynamicRange = iota
	// StandardRange is SDR video.
	StandardRange
	// HighRange is HDR video: HDR10, HDR10+, HLG or Dolby Vision.
	HighRange
)

// Labels are what an addon's labels for a stream tell of its file, read
// as the version rules read them until the file is analyzed or described
// by RemuxDB: its video codec, of accounts.VersionCodecs, empty when they
// name none, or several; its dynamic range; and its audio languages, by
// their ISO 639-2/B codes, with Multi for labels saying it has several
// (MULTI, DUAL) without naming them all. The languages the labels name as
// its subtitles' are not its audio's (see subtitleWords).
//
// Regions are the regional variants of its audio languages the labels
// name, of localization.AudioVariants, in the order they name them ("VFQ"
// gives fr-CA); their languages are among Languages.
//
// Releases are the kinds of low-quality releases, of
// accounts.ReleaseKinds, the labels tag the stream as, in the order they
// tag them (see ReleaseKinds); none for most.
//
// Elsewhere is set when the labels name another title than the stream's:
// the marker naming it, such as "S01E03", "S02" or "1998" (see
// OtherTitle); empty for most.
//
// Tier, Audio and Edition are its source tier, of Tiers, its best audio
// format and its edition, of accounts.VersionEditions, empty or zero when
// the labels name none (see ReadRelease).
type Labels struct {
	Codec     string
	Range     DynamicRange
	Languages []string
	Multi     bool
	Regions   []string
	Releases  []string
	Elsewhere string
	Tier      string
	Audio     AudioFormat
	Edition   string
}

// labelCodecs are the video codecs labels name, by the words naming them;
// labelCodecPairs, by two words in a row ("H.265", "VC-1").
var (
	labelCodecs = map[string]string{
		"av1": "av1", "hevc": "hevc", "x265": "hevc", "h265": "hevc", "vp9": "vp9",
		"h264": "h264", "x264": "h264", "avc": "h264", "vc1": "vc1", "xvid": "mpeg4", "divx": "mpeg4", "mpeg2": "mpeg2video",
	}
	labelCodecPairs = map[[2]string]string{
		{"h", "265"}: "hevc", {"h", "264"}: "h264", {"vc", "1"}: "vc1", {"mpeg", "2"}: "mpeg2video",
	}
)

// highRangeWords name HDR video in labels; "sdr" names SDR video.
var highRangeWords = map[string]bool{"hdr": true, "hdr10": true, "hdr10plus": true, "hdr10p": true, "hlg": true,
	"dv": true, "dovi": true, "dolbyvision": true}

// multiWords say a stream has several audio languages.
var multiWords = map[string]bool{"multi": true, "dual": true, "multilang": true, "multiaudio": true}

// labelLanguages are the languages labels name by a word in any letter
// case: the English names of the usual audio languages, and the words
// releases use for some of them. labelLanguageCodes are those named by an
// ISO 639-2 code, only when written in capitals ("ENG", "ITA"), as words
// such as "may" or "cat" would otherwise be taken for one; the codes that
// are common words even in capitals are left out.
var labelLanguages, labelLanguageCodes = func() (map[string]string, map[string]string) {
	usual := []string{"ar", "bg", "bn", "ca", "cs", "da", "de", "el", "en", "es", "et", "eu", "fa", "fi", "fr", "gl", "he", "hi",
		"hr", "hu", "id", "is", "it", "ja", "kn", "ko", "lt", "lv", "ml", "mr", "ms", "nl", "no", "pa", "pl", "pt", "ro", "ru",
		"sk", "sl", "sr", "sv", "ta", "te", "th", "tr", "uk", "ur", "vi", "zh"}
	commonWords := []string{"cat", "may", "mal", "mar", "pan", "kan", "per", "lit"}
	names, codes := map[string]string{}, map[string]string{}
	for _, language := range localization.Languages() {
		if !slices.Contains(usual, language.Alpha2) {
			continue
		}
		for name := range strings.FieldsFuncSeq(language.Name, func(r rune) bool { return r == ';' || r == ',' }) {
			if name = strings.ToLower(strings.TrimSpace(name)); !strings.ContainsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) }) {
				names[name] = language.Bibliographic
			}
		}
		for _, code := range []string{language.Bibliographic, language.Terminology} {
			if code != "" && !slices.Contains(commonWords, code) {
				codes[code] = language.Bibliographic
			}
		}
	}
	for word, code := range map[string]string{
		"greek": "gre", "farsi": "per", "mandarin": "chi", "cantonese": "chi",
		"francais": "fre", "français": "fre", "truefrench": "fre", "vf": "fre", "vff": "fre", "vfq": "fre", "vfi": "fre", "vf2": "fre",
		"latino": "spa", "castellano": "spa", "espanol": "spa", "español": "spa", "deutsch": "ger", "italiano": "ita",
		"portugues": "por", "português": "por", "dublado": "por", "nederlands": "dut", "polski": "pol",
	} {
		names[word] = code
	}
	return names, codes
}()

// flagLanguages are the languages flags name, by the country of the flag:
// those of countries with one main language.
var flagLanguages = map[string]string{
	"FR": "fre", "GB": "eng", "US": "eng", "AU": "eng", "IE": "eng", "NZ": "eng", "ES": "spa", "MX": "spa", "AR": "spa", "CO": "spa",
	"DE": "ger", "AT": "ger", "IT": "ita", "PT": "por", "BR": "por", "RU": "rus", "JP": "jpn", "KR": "kor", "CN": "chi", "TW": "chi",
	"HK": "chi", "IN": "hin", "NL": "dut", "SE": "swe", "NO": "nor", "DK": "dan", "FI": "fin", "PL": "pol", "TR": "tur", "CZ": "cze",
	"HU": "hun", "GR": "gre", "IL": "heb", "TH": "tha", "VN": "vie", "UA": "ukr", "RO": "rum", "ID": "ind", "IR": "per", "BG": "bul",
	"HR": "hrv", "RS": "srp", "SI": "slv", "SK": "slo", "SA": "ara", "AE": "ara", "EG": "ara",
}

// ReadLabels reads what the labels of a stream tell of its file: an
// addon's name, title, description or file name. title is the name of the
// movie or the series, whose words name no language: a language in it,
// as in a file name, is the title's, not the audio's. Codecs that
// disagree name none. Labels that name the video's height or codec but no
// HDR tell SDR video, as addons that describe the video name HDR when it
// is.
func ReadLabels(title string, labels ...string) Labels {
	var read Labels
	inTitle := map[string]bool{}
	for _, word := range labelWords(title) {
		inTitle[strings.ToLower(word)] = true
	}
	codecClash, standard := false, false
	setCodec := func(codec string) {
		if read.Codec != "" && read.Codec != codec {
			codecClash = true
		}
		read.Codec = codec
	}
	addLanguage := func(code string) {
		if !slices.Contains(read.Languages, code) {
			read.Languages = append(read.Languages, code)
		}
	}
	addRegion := func(variant string) {
		addLanguage(localization.VariantLanguage(variant))
		if !slices.Contains(read.Regions, variant) {
			read.Regions = append(read.Regions, variant)
		}
	}
	for _, label := range labels {
		for _, country := range labelFlags(label) {
			if code, ok := flagLanguages[country]; ok {
				addLanguage(code)
			}
			if variant := flagVariant(country); variant != "" {
				addRegion(variant)
			}
		}
		words := labelWords(label)
		subtitles := subtitleWords(words, inTitle)
		for i, word := range words {
			lower := strings.ToLower(word)
			next := ""
			if i+1 < len(words) {
				next = strings.ToLower(words[i+1])
			}
			if codec, ok := labelCodecs[lower]; ok {
				setCodec(codec)
			} else if codec, ok := labelCodecPairs[[2]string{lower, next}]; ok {
				setCodec(codec)
			}
			switch {
			case highRangeWords[lower] || lower == "dolby" && next == "vision":
				read.Range = HighRange
			case lower == "sdr":
				standard = true
			case multiWords[lower] && !subtitles[i]:
				read.Multi = true
			}
			if inTitle[lower] || subtitles[i] {
				continue
			}
			for _, variant := range localization.ReleaseVariants(lower) {
				addRegion(variant)
			}
			if variant := tagVariant(word, words, i, inTitle); variant != "" {
				addRegion(variant)
			}
			if code, ok := labelLanguages[lower]; ok {
				addLanguage(code)
			} else if code, ok := labelLanguageCodes[lower]; ok && len(word) == 3 && word == strings.ToUpper(word) {
				addLanguage(code)
			}
		}
	}
	if codecClash {
		read.Codec = ""
	}
	if read.Range == UnknownRange && (standard || read.Codec != "" || LabelHeight(labels...) > 0) {
		read.Range = StandardRange
	}
	return read
}

// labelWords splits a label into its words of letters and digits, in their
// letter case.
func labelWords(label string) []string {
	return strings.FieldsFunc(label, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// labelFlags lists the countries of the flags a label holds, by their
// ISO 3166-1 codes in capitals, in order: two regional indicator symbols
// make a flag.
func labelFlags(label string) []string {
	var countries []string
	var first rune
	for _, r := range label {
		if r < 0x1F1E6 || r > 0x1F1FF {
			first = 0
			continue
		}
		if first == 0 {
			first = r
			continue
		}
		countries = append(countries, string([]rune{'A' + first - 0x1F1E6, 'A' + r - 0x1F1E6}))
		first = 0
	}
	return countries
}

// releaseTags are the tags naming low-quality releases, in lower case, by
// their kind of accounts.ReleaseKinds: camera copies, telesyncs,
// telecines, screeners and early copies.
var releaseTags = map[string]string{
	"cam": "cam", "hdcam": "cam",
	"ts": "telesync", "hdts": "telesync", "telesync": "telesync",
	"tc": "telecine", "telecine": "telecine",
	"scr": "screener", "dvdscr": "screener",
	"r5": "early", "workprint": "early",
}

// ReleaseKinds reads the kinds of low-quality releases the labels of a
// stream tag it as, each once, in the order they come: an addon's name,
// title, description or file name, each line of them apart. titles are
// the names of what the stream plays, the movie's or the series', and the
// episode's: a tag is a whole word of a line (see labelWords), in any
// letter case, after the title's own words, so that a title named "Cam"
// is not a camera copy. The title's words end where the line names one of
// titles, after the last of them it names; in a line naming none, at its
// first year or episode number (2018, S01E01, 1x01), which follows the
// title's words in a release's name; in a line holding neither, every
// word may be a tag. A line ending with ".ts" names an MPEG-TS file: the
// extension is no telesync.
func ReleaseKinds(titles []string, labels ...string) []string {
	var named [][]string
	for _, title := range titles {
		if words := lowerWords(title); len(words) > 0 {
			named = append(named, words)
		}
	}
	var kinds []string
	for _, label := range labels {
		for line := range strings.Lines(label) {
			line = strings.TrimSpace(line)
			words := lowerWords(line)
			if len(words) > 0 && strings.HasSuffix(strings.ToLower(line), ".ts") {
				words = words[:len(words)-1]
			}
			for _, word := range words[titleEnd(words, named):] {
				if kind, ok := releaseTags[word]; ok && !slices.Contains(kinds, kind) {
					kinds = append(kinds, kind)
				}
			}
		}
	}
	return kinds
}

// lowerWords are the words of a label (see labelWords), in lower case.
func lowerWords(label string) []string {
	words := labelWords(label)
	for i, word := range words {
		words[i] = strings.ToLower(word)
	}
	return words
}

// titleEnd is where the title's words end among the words of a line (see
// ReleaseKinds): after the last of titles the line names, each where it
// first does; else at the line's first year or episode number; else at
// its start.
func titleEnd(words []string, titles [][]string) int {
	end := -1
	for _, title := range titles {
		for i := 0; i+len(title) <= len(words); i++ {
			if slices.Equal(words[i:i+len(title)], title) {
				end = max(end, i+len(title))
				break
			}
		}
	}
	if end >= 0 {
		return end
	}
	if i := slices.IndexFunc(words, releaseMark); i >= 0 {
		return i + 1
	}
	return 0
}

// releaseMark reports whether a word of a release's name, in lower case,
// is a year, from 1900 to 2099, or an episode number: S01, S01E01,
// S01E01E02 or 1x01.
func releaseMark(word string) bool {
	digits := func(s string) bool {
		return s != "" && !strings.ContainsFunc(s, func(r rune) bool { return r < '0' || r > '9' })
	}
	if len(word) == 4 && digits(word) {
		return word[:2] == "19" || word[:2] == "20"
	}
	if season, episode, ok := strings.Cut(word, "x"); ok {
		return len(season) <= 2 && digits(season) && digits(episode)
	}
	if rest, ok := strings.CutPrefix(word, "s"); ok {
		season, episodes, numbered := strings.Cut(rest, "e")
		return digits(season) && (!numbered || digits(strings.ReplaceAll(episodes, "e", "")))
	}
	return false
}
