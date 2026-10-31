package library

import (
	"strings"

	"github.com/moodiness/polyfin/internal/localization"
)

// flagVariant is the regional variant of localization.AudioVariants a
// flag names by its country, "" for none: the flags of Mexico, Argentina
// and Colombia name Latin American Spanish, Brazil's Brazilian Portuguese.
// The flag of the country a language is named after stands for the
// language at large: releases show France's for any French dub.
func flagVariant(country string) string {
	code, ok := flagLanguages[country]
	if !ok || strings.EqualFold(localization.LanguageTag(code), country) {
		return ""
	}
	return localization.TagVariant(code + "-" + country)
}

// tagVariant is the regional variant a language tag split into two words
// names, the words at i and after it in words ("PT-BR", "es-419", "FR CA"),
// "" for none (see localization.TagVariant): the language by its ISO 639-1
// code, or its ISO 639-2 code in capitals, as labelLanguageCodes reads
// them, then the region, neither a word of the title.
func tagVariant(word string, words []string, i int, inTitle map[string]bool) string {
	if i+1 >= len(words) || len(word) != 2 && (len(word) != 3 || word != strings.ToUpper(word)) {
		return ""
	}
	next := strings.ToLower(words[i+1])
	if inTitle[next] {
		return ""
	}
	return localization.TagVariant(strings.ToLower(word) + "-" + next)
}

// subtitleMarkers are the words, in lower case, saying that the languages
// named right after them are the subtitles' ("Sub ITA", "Subtitles:
// French, Spanish", "VOST FR"); subtitleSuffixes those that say so of the
// language named right before them ("French Subs", "PT-BR Legendado")
// when none follows. Releases with subtitles in the audience's language
// name them so, the audio being the original's.
var (
	subtitleMarkers = map[string]bool{"sub": true, "subs": true, "subbed": true, "subtitle": true, "subtitles": true,
		"subtitled": true, "vost": true, "vos": true, "vose": true, "legenda": true, "legendas": true, "legendado": true,
		"legendada": true, "subtitulado": true, "subtitulada": true, "subtitulos": true, "subtítulos": true,
		"sottotitoli": true, "sottotitolato": true, "untertitel": true, "untertitelt": true}
	subtitleSuffixes = map[string]bool{"sub": true, "subs": true, "subbed": true, "subtitles": true, "subtitled": true,
		"legendado": true, "legendada": true, "subtitulado": true, "subtitulada": true, "sottotitolato": true, "untertitelt": true}
)

// subtitleWords marks, by their index in words, the words of a label that
// name its subtitles, which ReadLabels reads no audio language or region
// in: after a word of subtitleMarkers, the run of words naming languages
// that follows it; before a word of subtitleSuffixes followed by none, the
// language right before it, a word or a tag in two ("PT-BR"). A run also
// holds the words that join languages in a list ("and"), the two-letter
// codes and regions of tags, and MULTI ("MULTi-SUBS" names no audio).
// Words of the title are no markers.
func subtitleWords(words []string, inTitle map[string]bool) []bool {
	marked := make([]bool, len(words))
	lower := make([]string, len(words))
	for i, word := range words {
		lower[i] = strings.ToLower(word)
	}
	for i, word := range lower {
		if inTitle[word] || !subtitleMarkers[word] {
			continue
		}
		after := i + 1
		for after < len(lower) && languageWord(lower[after]) {
			marked[after] = true
			after++
		}
		if after > i+1 || !subtitleSuffixes[word] || i == 0 || !languageWord(lower[i-1]) {
			continue
		}
		marked[i-1] = true
		if i >= 2 && len(lower[i-2]) == 2 && localization.KnownLanguage(lower[i-2]) {
			marked[i-2] = true
		}
	}
	return marked
}

// languageWord reports whether a word of a label, in lower case, may be
// part of a list of languages (see subtitleWords).
func languageWord(word string) bool {
	if _, ok := labelLanguages[word]; ok {
		return true
	}
	if _, ok := labelLanguageCodes[word]; ok {
		return true
	}
	return multiWords[word] || word == "and" || word == "419" || len(word) == 2 && !strings.ContainsAny(word, "0123456789")
}
