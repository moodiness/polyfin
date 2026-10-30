package localization

import "testing"

// A track's language tag names its regional variant by any of the
// language's codes, the countries hearing the same dub folding into one;
// else its title does, by a tag, a region's name or code, or the release
// word of a dub of its language.
func TestAudioVariantReadsTagsAndTitles(t *testing.T) {
	for _, tc := range []struct {
		language, title, want string
	}{
		{"fr-CA", "", "fr-CA"},
		{"FRE-ca", "", "fr-CA"},
		{"fr_FR", "", "fr-FR"},
		{"fr-BE", "", "fr-FR"},
		{"es-419", "", "es-419"},
		{"es-MX", "", "es-419"},
		{"spa-AR", "", "es-419"},
		{"es-ES", "", "es-ES"},
		{"es-Latn-MX", "", "es-419"},
		{"pt-BR", "", "pt-BR"},
		{"pob", "", "pt-BR"},
		{"pt-PT", "", "pt-PT"},
		{"pt-AO", "", "pt-PT"},
		// A tag's region wins over the title.
		{"fr-FR", "VFQ", "fr-FR"},
		// Without a region or a known variant, none.
		{"fre", "", ""},
		{"fr", "Stereo", ""},
		{"en-US", "", ""},
		{"en-GB", "British", ""},
		{"zh-TW", "", ""},
		{"und", "VFQ", ""},
		// Titles.
		{"fre", "VFQ", "fr-CA"},
		{"fre", "VFF 5.1", "fr-FR"},
		{"fra", "Français (Québec)", "fr-CA"},
		{"fre", "French (Canada)", "fr-CA"},
		{"fre", "French (CA)", "fr-CA"},
		{"fre", "TrueFrench", "fr-FR"},
		{"fre", "VF2", ""},
		{"spa", "Español Latino", "es-419"},
		{"spa", "Spanish (Latin America)", "es-419"},
		{"spa", "Castellano", "es-ES"},
		{"spa", "Español (ES-MX)", "es-419"},
		{"es", "European Spanish", "es-ES"},
		{"por", "Portuguese (PT-BR)", "pt-BR"},
		{"por", "Brazilian Portuguese", "pt-BR"},
		{"por", "Português (Portugal)", "pt-PT"},
		{"por", "Dublado", "pt-BR"},
		// A release word of another language's dub names none.
		{"por", "Latino", ""},
		{"eng", "VFQ", ""},
	} {
		if got := AudioVariant(tc.language, tc.title); got != tc.want {
			t.Errorf("AudioVariant(%q, %q) = %q, want %q", tc.language, tc.title, got, tc.want)
		}
	}
}
