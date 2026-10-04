package xmltv

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// countryPrefix is a country or language code that IPTV lists put before
// a channel's name: "FR: ", "|FR| ", "[UK] ", "FR | ".
var countryPrefix = regexp.MustCompile(`^\s*[\[(|]?\s*[A-Za-z]{2,3}\s*[\])|:]\s*`)

// qualityTags are the words that tell a stream's quality or encoding, not
// the channel.
var qualityTags = map[string]bool{
	"sd": true, "hd": true, "fhd": true, "uhd": true, "qhd": true, "hq": true, "lq": true,
	"4k": true, "8k": true, "hdr": true, "hevc": true, "h264": true, "h265": true, "x264": true, "x265": true,
	"480p": true, "576p": true, "720p": true, "1080p": true, "1080i": true, "2160p": true,
	"50fps": true, "60fps": true,
}

// NormalizeName folds a channel name into the form names are matched in:
// without a country prefix, case, accents, punctuation, separators,
// superscript and marker characters (such as ᴴᴰ, ⁴ᴷ or ★), nor quality
// tags such as HD, FHD, UHD, 4K or SD. Words are joined, so "TF 1" is
// "TF1". A name left with nothing is empty, and matches nothing.
func NormalizeName(name string) string {
	name = countryPrefix.ReplaceAllString(name, "")
	var folded strings.Builder
	for _, r := range name {
		switch {
		// Modifier letters (ᴴᴰ), superscripts and subscripts (⁴, ²),
		// symbols (★, ●) and invisible characters mark a stream, not the
		// channel; drop them before compatibility folding turns them into
		// letters.
		case unicode.In(r, unicode.Lm, unicode.Sk, unicode.So, unicode.Co, unicode.Cf),
			r >= 0x2070 && r <= 0x209f, r == 0xb2 || r == 0xb3 || r == 0xb9:
			folded.WriteRune(' ')
		default:
			folded.WriteRune(r)
		}
	}
	var words []string
	var word strings.Builder
	flush := func() {
		if word.Len() > 0 {
			if w := word.String(); !qualityTags[w] {
				words = append(words, w)
			}
			word.Reset()
		}
	}
	for _, r := range norm.NFKD.String(folded.String()) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			word.WriteRune(unicode.ToLower(r))
		default:
			flush()
		}
	}
	flush()
	return strings.Join(words, "")
}
