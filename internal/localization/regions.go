package localization

import (
	"slices"
	"strings"
	"unicode"
)

// Polyfin tells apart the regional variants of the audio languages that
// are dubbed apart for a region: French for France and for Quebec, Spanish
// for Spain and for Latin America, Portuguese for Brazil and for Portugal.
// It reads a track's variant from its language tag, else from the words of
// its title, and a release's from the words releases name their dub with
// (see ReleaseVariants). Jellyfin compares languages without their region.

// AudioVariants are the regional variants, as BCP 47 tags: the language's
// ISO 639-1 code and the region's, an ISO 3166-1 country or 419 for Latin
// America.
var AudioVariants = []string{"fr-FR", "fr-CA", "es-ES", "es-419", "pt-BR", "pt-PT"}

// variantRegions are, by the ISO 639-2/B code of each language with
// variants, the regions a tag names its variants by, in lower case: the
// variant's own, and the other countries that hear the same dub (Belgium,
// Switzerland, Luxembourg and Monaco France's; every Latin American
// country, and the United States, Latin America's; the other Portuguese
// speaking countries Portugal's).
var variantRegions = map[string]map[string]string{
	"fre": {"fr": "fr-FR", "be": "fr-FR", "ch": "fr-FR", "lu": "fr-FR", "mc": "fr-FR", "ca": "fr-CA"},
	"spa": {"es": "es-ES", "419": "es-419", "mx": "es-419", "ar": "es-419", "co": "es-419", "cl": "es-419", "pe": "es-419",
		"ve": "es-419", "ec": "es-419", "gt": "es-419", "cu": "es-419", "bo": "es-419", "do": "es-419", "hn": "es-419",
		"py": "es-419", "sv": "es-419", "ni": "es-419", "cr": "es-419", "pa": "es-419", "uy": "es-419", "pr": "es-419",
		"us": "es-419"},
	"por": {"br": "pt-BR", "pt": "pt-PT", "ao": "pt-PT", "mz": "pt-PT", "cv": "pt-PT", "gw": "pt-PT", "st": "pt-PT", "tl": "pt-PT"},
}

// variantWords are, by language as in variantRegions, the words a track's
// title names its variant by, in lower case, besides the regions' codes:
// the region's name, and how the dub is called, in English and in the
// language itself ("French (Canada)", "Québécois", "Latino",
// "Castellano", "Brazilian").
var variantWords = map[string]map[string]string{
	"fre": {"canada": "fr-CA", "canadian": "fr-CA", "canadien": "fr-CA", "canadienne": "fr-CA", "quebec": "fr-CA",
		"québec": "fr-CA", "quebecois": "fr-CA", "québécois": "fr-CA", "quebecoise": "fr-CA", "québécoise": "fr-CA",
		"france": "fr-FR", "parisian": "fr-FR"},
	"spa": {"latino": "es-419", "latina": "es-419", "latin": "es-419", "latam": "es-419", "latinoamerica": "es-419",
		"latinoamérica": "es-419", "latinoamericano": "es-419", "hispanoamérica": "es-419", "hispanoamerica": "es-419",
		"mexico": "es-419", "méxico": "es-419", "mexican": "es-419", "mexicano": "es-419",
		"spain": "es-ES", "españa": "es-ES", "espana": "es-ES", "castellano": "es-ES", "castilian": "es-ES",
		"european": "es-ES", "europeo": "es-ES"},
	"por": {"brazil": "pt-BR", "brasil": "pt-BR", "brazilian": "pt-BR", "brasileiro": "pt-BR", "brasileira": "pt-BR",
		"dublado": "pt-BR", "portugal": "pt-PT", "european": "pt-PT", "europeu": "pt-PT"},
}

// releaseVariants are the words, in lower case, that name a variant in a
// release's name without the language: the French scene's VFF and
// TRUEFRENCH (France), VFQ (Quebec) and VF2 (both), and the Spanish and
// Portuguese ones' LATINO, CASTELLANO, DUBLADO and PTBR.
var releaseVariants = map[string][]string{
	"vff": {"fr-FR"}, "truefrench": {"fr-FR"}, "vfq": {"fr-CA"}, "vf2": {"fr-FR", "fr-CA"},
	"latino": {"es-419"}, "latam": {"es-419"}, "castellano": {"es-ES"}, "castilian": {"es-ES"},
	"dublado": {"pt-BR"}, "ptbr": {"pt-BR"},
}

// TagVariant returns the variant of AudioVariants a BCP 47 language tag
// names, by any of its language's ISO 639 codes and in any letter case,
// with or without a script ("fr-CA", "fre-CA", "es-MX", "pt_BR"); "pob",
// the code some subtitle sites and Jellyfin's list give Brazilian
// Portuguese, names pt-BR. "" when the tag names none: no region, or one
// of a language without variants.
func TagVariant(tag string) string {
	parts := strings.Split(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(tag)), "_", "-"), "-")
	if parts[0] == "pob" {
		return "pt-BR"
	}
	regions, ok := variantRegions[LanguageCode(parts[0])]
	if !ok || len(parts) < 2 {
		return ""
	}
	region := parts[1]
	if len(region) == 4 && len(parts) > 2 {
		region = parts[2]
	}
	return regions[region]
}

// AudioVariant returns the variant of AudioVariants of an audio track
// tagged language, with title: the variant its tag names (see TagVariant),
// else the first its title names by a tag ("Portuguese (PT-BR)"), a word
// of variantWords, a region's code ("French (CA)"), or a word of
// releaseVariants naming one of its language's ("French VFQ"). "" when
// neither names one.
func AudioVariant(language, title string) string {
	if variant := TagVariant(language); variant != "" {
		return variant
	}
	code := LanguageCode(language)
	regions, ok := variantRegions[code]
	if !ok {
		return ""
	}
	words := strings.FieldsFunc(strings.ToLower(title), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	for i, word := range words {
		if i+1 < len(words) {
			if variant := TagVariant(word + "-" + words[i+1]); variant != "" && VariantLanguage(variant) == code {
				return variant
			}
		}
		if variant, ok := variantWords[code][word]; ok {
			return variant
		}
		if variant, ok := regions[word]; ok {
			return variant
		}
		if variants := releaseVariants[word]; len(variants) == 1 && VariantLanguage(variants[0]) == code {
			return variants[0]
		}
	}
	return ""
}

// ReleaseVariants returns the variants a word of a release's name names
// alone, in lower case (see releaseVariants); nil for most.
func ReleaseVariants(word string) []string {
	return releaseVariants[word]
}

// VariantLanguage returns the ISO 639-2/B code of a variant's language:
// "fr-CA" gives "fre".
func VariantLanguage(variant string) string {
	base, _, _ := strings.Cut(variant, "-")
	return LanguageCode(base)
}

// AudioVariantOffered reports whether variant is one of AudioVariants.
func AudioVariantOffered(variant string) bool {
	return slices.Contains(AudioVariants, variant)
}
