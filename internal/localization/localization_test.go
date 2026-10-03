package localization

import "testing"

func TestLanguagesCompareWhicheverCodeNamesThem(t *testing.T) {
	for _, test := range []struct {
		a, b string
		same bool
	}{
		{"fre", "fra", true},
		{"ger", "deu", true},
		{"chi", "zh", true},
		{"FR", "fre", true},
		{"fr-CA", "fra", true},
		{"pt_BR", "por", true},
		{"eng", "eng", true},
		{"fil", "FIL", true},
		{"eng", "fre", false},
		{"", "", false},
		{"", "und", false},
		{"klingon", "Klingon", true},
	} {
		if got := SameLanguage(test.a, test.b); got != test.same {
			t.Errorf("SameLanguage(%q, %q) = %v", test.a, test.b, got)
		}
	}
}

func TestEveryLanguageHasACode(t *testing.T) {
	for _, language := range Languages() {
		if len(language.Bibliographic) != 3 || language.Name == "" || LanguageCode(language.Bibliographic) != language.Bibliographic {
			t.Errorf("language %+v", language)
		}
	}
	for _, country := range Countries() {
		if len(country.Alpha2) != 2 || len(country.Alpha3) != 3 || country.Name == "" {
			t.Errorf("country %+v", country)
		}
	}
}

func TestCountriesAreTheAssignedCodes(t *testing.T) {
	codes := map[string]Country{}
	for _, country := range Countries() {
		codes[country.Alpha2] = country
	}
	// Reserved (UK, EU), withdrawn (YU, ZR) and user-assigned (XK, ZZ)
	// codes are left out.
	for _, code := range []string{"UK", "EU", "YU", "ZR", "XK", "ZZ"} {
		if country, ok := codes[code]; ok {
			t.Errorf("%s is listed: %+v", code, country)
		}
	}
	if gb := codes["GB"]; gb.Alpha3 != "GBR" || gb.Name != "United Kingdom" {
		t.Errorf("GB: %+v", gb)
	}
	if len(codes) != 249 {
		t.Errorf("%d countries, ISO 3166-1 assigns 249 codes", len(codes))
	}
	if list := Countries(); list[1].Alpha2 != "AX" {
		t.Errorf("Åland Islands sorted at %+v", list[1])
	}
}
