package xmltv

import (
	"html"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// listPrefix is a code that IPTV lists put before a channel's name: a
// country or language ("FR: ", "|FR| ", "[UK] ", "FR | "), or a group
// ("ENF| ", "VO| ", "PLAY+| ").
var listPrefix = regexp.MustCompile(`^\s*[\[(|]?\s*([A-Za-z][A-Za-z0-9+]{1,5})\s*[\])|:]\s*`)

// qualityTags are the words that tell a stream's quality or encoding, not
// the channel.
var qualityTags = map[string]bool{
	"sd": true, "hd": true, "fhd": true, "uhd": true, "qhd": true, "hq": true, "lq": true,
	"4k": true, "8k": true, "hdr": true, "hevc": true, "h264": true, "h265": true, "x264": true, "x265": true,
	"480p": true, "576p": true, "720p": true, "1080p": true, "1080i": true, "2160p": true,
	"50fps": true, "60fps": true,
}

// Name is a channel name in the forms guides are matched by. Exact is the
// name folded: without its list prefix, HTML entities decoded, without
// case, accents, punctuation, separators, superscript and marker
// characters (such as ᴴᴰ, ⁴ᴷ or ★), its words joined, so "ZEB 1" is "ZEB1",
// and "+" read as "plus", as guide identifiers write it ("Zeb+ 1" is
// "zebplus1", as "ZebPlus1.fr" is). Loose is Exact without the quality
// tags too, such as HD, FHD, UHD, 4K or SD, and without the "+", which
// lists leave out at times ("Zeb Sport" for "Zeb+ Sport"). A name left
// with nothing is empty, and matches nothing. Country is the country its
// prefix names, if any (see Country).
type Name struct {
	Exact, Loose string
	Country      string
}

// ParseName reads a channel name in the forms it is matched by. A group
// prefix is the name when nothing would be left of it, as in "Zeb | HD";
// a country prefix never is.
func ParseName(name string) Name {
	name = html.UnescapeString(name)
	if prefix := listPrefix.FindStringSubmatch(name); prefix != nil {
		parsed := foldName(name[len(prefix[0]):])
		if parsed.Country = Country(prefix[1]); parsed.Country != "" || parsed.Loose != "" {
			return parsed
		}
	}
	return foldName(name)
}

// foldName folds a name without its prefix (see Name).
func foldName(name string) Name {
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
	var exact, loose, word strings.Builder
	flush := func() {
		if word.Len() > 0 {
			exact.WriteString(word.String())
			if !qualityTags[word.String()] {
				loose.WriteString(word.String())
			}
			word.Reset()
		}
	}
	for _, r := range norm.NFKD.String(folded.String()) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case r == '+':
			flush()
			exact.WriteString("plus")
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			word.WriteRune(unicode.ToLower(r))
		default:
			flush()
		}
	}
	flush()
	return Name{Exact: exact.String(), Loose: loose.String()}
}

// frenchShortName is how French lists write the public networks numbered
// 2 to 5: "F3" (or "F3:") before a region, or alone.
var frenchShortName = regexp.MustCompile(`^\s*F([2-5])(?:\s*:)?(\s|$)`)

// FrenchName is a French channel name with its public network written in
// full ("F3 Zebria" is "France 3 Zebria", "FR| F3 Zebria" is "FR| France 3
// Zebria"), or "" when it has none. "F1" is not one of these networks.
func FrenchName(name string) string {
	name = html.UnescapeString(name)
	prefix := ""
	// "F3:" reads as a list prefix too: the network comes first.
	if found := listPrefix.FindString(name); found != "" && !frenchShortName.MatchString(name) {
		prefix, name = found, name[len(found):]
	}
	match := frenchShortName.FindStringSubmatchIndex(name)
	if match == nil {
		return ""
	}
	return prefix + "France " + name[match[2]:match[3]] + name[match[4]:]
}

// IDName is the name a guide channel's identifier gives, as in
// "ZebPlus1.fr" for "Zeb+ 1": the identifier without its country suffix,
// folded (see Name), or "" for an identifier without one, which is not
// written as a name.
func IDName(id string) string {
	at := strings.LastIndexByte(id, '.')
	if at <= 0 || IDCountry(id) == "" || strings.ContainsAny(id[:at], " .") {
		return ""
	}
	return foldName(id[:at]).Exact
}

// countries are the ISO 3166-1 alpha-2 country codes.
var countries = func() map[string]bool {
	codes := map[string]bool{}
	for _, code := range strings.Fields(`ad ae af ag ai al am ao aq ar as at au aw ax az ba bb bd be bf bg bh bi bj bl bm bn bo bq br bs bt
		bv bw by bz ca cc cd cf cg ch ci ck cl cm cn co cr cu cv cw cx cy cz de dj dk dm do dz ec ee eg eh er es et fi fj fk fm fo fr ga gb
		gd ge gf gg gh gi gl gm gn gp gq gr gs gt gu gw gy hk hm hn hr ht hu id ie il im in io iq ir is it je jm jo jp ke kg kh ki km kn kp
		kr kw ky kz la lb lc li lk lr ls lt lu lv ly ma mc md me mf mg mh mk ml mm mn mo mp mq mr ms mt mu mv mw mx my mz na nc ne nf ng ni
		nl no np nr nu nz om pa pe pf pg ph pk pl pm pn pr ps pt pw py qa re ro rs ru rw sa sb sc sd se sg sh si sj sk sl sm sn so sr ss st
		sv sx sy sz tc td tf tg th tj tk tl tm tn to tr tt tv tw tz ua ug um us uy uz va vc ve vg vi vn vu wf ws ye yt za zm zw`) {
		codes[code] = true
	}
	return codes
}()

// Country returns the country a code names, as a lower-case ISO 3166-1
// alpha-2 code: "uk" is "gb", and a code that is not a country's, such as
// "SP" or "ENF", names none.
func Country(code string) string {
	code = strings.ToLower(code)
	if code == "uk" {
		return "gb"
	}
	if countries[code] {
		return code
	}
	return ""
}

// IDCountry returns the country an XMLTV channel identifier ends with, as
// in "Name.fr", if any.
func IDCountry(id string) string {
	if i := strings.LastIndexByte(id, '.'); i >= 0 && len(id)-i == 3 {
		return Country(id[i+1:])
	}
	return ""
}

// languageCountries maps languages, ISO 639-1, to the country whose guides
// they suggest. Languages spoken in many countries alike, such as English,
// suggest none.
var languageCountries = map[string]string{
	"fr": "fr", "de": "de", "es": "es", "it": "it", "nl": "nl", "pl": "pl", "pt": "pt",
	"da": "dk", "sv": "se", "nb": "no", "nn": "no", "no": "no", "fi": "fi", "is": "is",
	"cs": "cz", "sk": "sk", "hu": "hu", "ro": "ro", "bg": "bg", "hr": "hr", "sr": "rs", "sl": "si",
	"el": "gr", "tr": "tr", "ru": "ru", "uk": "ua", "ja": "jp", "ko": "kr", "he": "il",
}

// LanguageCountry returns the country whose guides a language suggests:
// the region of a tag such as "fr-CA", else the country where the
// language is mainly spoken, if one is.
func LanguageCountry(language string) string {
	language, region, _ := strings.Cut(strings.ReplaceAll(language, "_", "-"), "-")
	if country := Country(region); country != "" {
		return country
	}
	return languageCountries[strings.ToLower(language)]
}
