package iptv

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/moodiness/polyfin/internal/xmltv"
)

// countryPrefixes are the ways lists write a country before a name, such
// as "||FR||", "|FR|", "[FR]", "(FR)", "FR:", "FR/", "FR - " or "FR|"; the
// code counts only when it is a country's (see xmltv.Country).
var countryPrefixes = []*regexp.Regexp{
	regexp.MustCompile(`^\s*\|\|\s*([A-Za-z]{2})\s*\|\|\s*`),
	regexp.MustCompile(`^\s*\|\s*([A-Za-z]{2})\s*\|\s*`),
	regexp.MustCompile(`^\s*\[\s*([A-Za-z]{2})\s*\]\s*`),
	regexp.MustCompile(`^\s*\(\s*([A-Za-z]{2})\s*\)\s*`),
	regexp.MustCompile(`^\s*([A-Za-z]{2})\s*[:/]\s*`),
	regexp.MustCompile(`^\s*([A-Za-z]{2})\s*[-–—|]\s*`),
}

// flagCountry returns the country of the first flag emoji in name, a pair
// of regional indicator letters, if any.
func flagCountry(name string) (string, int, int) {
	runes := []rune(name)
	for i := 0; i+1 < len(runes); i++ {
		a, b := runes[i], runes[i+1]
		if a >= 0x1F1E6 && a <= 0x1F1FF && b >= 0x1F1E6 && b <= 0x1F1FF {
			code := string([]rune{'a' + (a - 0x1F1E6), 'a' + (b - 0x1F1E6)})
			if country := xmltv.Country(code); country != "" {
				start := len(string(runes[:i]))
				return strings.ToUpper(country), start, start + len(string(runes[i:i+2]))
			}
		}
	}
	return "", 0, 0
}

// detectCountry returns the country, upper-case ISO 3166-1 alpha-2, that a
// group or channel name gives by a flag emoji or a prefix, "" for none.
func detectCountry(name string) string {
	if country, _, _ := flagCountry(name); country != "" {
		return country
	}
	for _, pattern := range countryPrefixes {
		if match := pattern.FindStringSubmatch(name); match != nil {
			if country := xmltv.Country(match[1]); country != "" {
				return strings.ToUpper(country)
			}
		}
	}
	return ""
}

// otherCountry is the country key of entries no country was found for.
const otherCountry = "OTHER"

// quality is a stream quality a name tells: its label, and its rank, lower
// being better.
type quality struct {
	pattern *regexp.Regexp
	label   string
	rank    int
}

// qualities are the quality markers of stream names, the first found
// giving a stream its label; all of them are left out of merged names.
var qualities = func() []quality {
	q := func(pattern, label string, rank int) quality {
		return quality{regexp.MustCompile(pattern), label, rank}
	}
	return []quality{
		q(`(?i)\b(?:2160|3840)[pi]\b`, "4K", 1),
		q(`(?i)\b1080[pi]\b`, "FHD", 3),
		q(`(?i)\b720[pi]\b`, "HD", 4),
		q(`(?i)\b(?:576|480)[pi]\b`, "SD", 5),
		q(`(?i)\b360[pi]\b`, "LQ", 6),
		q(`(?i)[\[(]\s*4K\s*[\])]`, "4K", 1),
		q(`(?i)[\[(]\s*UHD\s*[\])]`, "UHD", 2),
		q(`(?i)[\[(]\s*FHD\s*[\])]`, "FHD", 3),
		q(`(?i)[\[(]\s*HD\s*[\])]`, "HD", 4),
		q(`(?i)[\[(]\s*SD\s*[\])]`, "SD", 5),
		q(`(?i)\bUltra\s*HD\b`, "UHD", 2),
		q(`(?i)\bFull\s*HD\b`, "FHD", 3),
		q(`(?i)\b4K\b`, "4K", 1),
		q(`(?i)\bUHD\b`, "UHD", 2),
		q(`(?i)\bFHD\b`, "FHD", 3),
		q(`(?i)\bHD\b`, "HD", 4),
		q(`(?i)\bSD\b`, "SD", 5),
		q(`(?i)\bLQ\b`, "LQ", 6),
		q(`(?i)\bHQ\b`, "HQ", 7),
		q(`⁴ᴷ`, "4K", 1),
		q(`ᵁᴴᴰ`, "UHD", 2),
		q(`ᶠᴴᴰ`, "FHD", 3),
		q(`ᴴᴰ`, "HD", 4),
		q(`ˢᴰ`, "SD", 5),
		q(`ᴸᵠ`, "LQ", 6),
		q(`(?i)\bHDR(?:10\+?)?\b|\bHLG\b|\bDolby\s*Vision\b`, "HDR", 10),
		q(`(?i)\bH\.?26[45]\b|\bHEVC\b|\bAV1\b|\bVP9\b|ᴴᴱᵛᶜ`, "", 99),
		q(`(?i)\b[56]0\s*fps\b`, "", 99),
	}
}()

// extraTags are what stream names add besides the channel, left out of
// merged names and never a label.
var extraTags = regexp.MustCompile(`(?i)^\s*(?:VIP|Premium)\s+|\(\s*Backup(?:\s*\d+)?\s*\)|\bBackup\s*\d*|[-–—]\s*Link\s*\d+|\bLink\s*\d+|` +
	`\bMulti\s*-?\s*Audio\b|\bDual\s*Audio\b|\bMULTI\b|\[\s*(?:not\s+24/7|Geo[- ]?blocked|Occasional|Part[- ]?Time)\s*\]`)

// streamQuality returns the label and rank of a stream's quality from its
// name: "" and 99 when it tells none.
func streamQuality(name string) (string, int) {
	for _, q := range qualities {
		if q.label != "" && q.pattern.MatchString(name) {
			return q.label, q.rank
		}
	}
	return "", 99
}

// cleanName is a stream name shown as its merged channel's: without its
// flag, country prefix, quality markers, extra tags, superscript markers
// and the separators left around them. A name left empty stays as it was.
func cleanName(name string) string {
	cleaned := name
	if country, start, end := flagCountry(cleaned); country != "" {
		cleaned = cleaned[:start] + cleaned[end:]
	}
	for _, pattern := range countryPrefixes {
		if match := pattern.FindStringSubmatchIndex(cleaned); match != nil && xmltv.Country(cleaned[match[2]:match[3]]) != "" {
			cleaned = cleaned[match[1]:]
			break
		}
	}
	for _, q := range qualities {
		cleaned = q.pattern.ReplaceAllString(cleaned, " ")
	}
	cleaned = extraTags.ReplaceAllString(cleaned, " ")
	cleaned = strings.Map(func(r rune) rune {
		if unicode.In(r, unicode.Lm, unicode.Sk) || r >= 0x2070 && r <= 0x209f {
			return -1
		}
		return r
	}, cleaned)
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	cleaned = strings.Trim(cleaned, " -_|:/—–()[]")
	if cleaned == "" {
		return strings.TrimSpace(name)
	}
	return cleaned
}

// mergeKey is the name entries of the same channel share, for merging: the
// loose form of their names (see xmltv.ParseName) once extra tags and
// every quality marker are left out; "" when nothing is left.
func mergeKey(name string) string {
	stripped := extraTags.ReplaceAllString(name, " ")
	for _, q := range qualities {
		stripped = q.pattern.ReplaceAllString(stripped, " ")
	}
	if country, start, end := flagCountry(stripped); country != "" {
		stripped = stripped[:start] + stripped[end:]
	}
	return xmltv.ParseName(stripped).Loose
}

// Fold is text as searches compare it: lower case, without accents.
func Fold(text string) string {
	var folded strings.Builder
	for _, r := range norm.NFKD.String(text) {
		if !unicode.Is(unicode.Mn, r) {
			folded.WriteRune(unicode.ToLower(r))
		}
	}
	return folded.String()
}
