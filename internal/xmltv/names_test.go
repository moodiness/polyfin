package xmltv

import "testing"

// IPTV lists dress channel names up; guides name channels plainly. Both
// fold to the same loose name.
func TestChannelNamesMatchAcrossTheirDressing(t *testing.T) {
	for _, tc := range []struct{ listed, guide string }{
		{"ZEB1 HD", "ZEB1"},
		{"ZEB1ᴴᴰ", "ZEB1"},
		{"FR: ZEB1 FHD", "ZEB1"},
		{"|FR| ZEB 1 UHD", "ZEB1"},
		{"[UK] Quill One 4K", "QUILL ONE"},
		{"FR | Orbe 2 SD", "Orbe 2"},
		{"Orbe 3 ⁴ᴷ", "Orbe 3"},
		{"Lumo ★ HEVC", "LUMO"},
		{"Télé-Matou", "tele matou"},
		{"Kanal+ Jeux 1080p", "Kanal+ Jeux"},
		{"Équipe 21 ²", "Equipe 21"},
		{"Zorba Découverte | HD", "Zorba Decouverte"},
		{"Ciné+ Frémir (HD)", "Cine+ Fremir"},
		// Some guides escape their names twice: the entities are left in.
		{"Orbe Séries", "Orbe S&eacute;ries"},
		{"Pif &amp; Hercule", "PIF & HERCULE"},
	} {
		if a, b := ParseName(tc.listed).Loose, ParseName(tc.guide).Loose; a != b || a == "" {
			t.Errorf("%q → %q, %q → %q", tc.listed, a, tc.guide, b)
		}
	}
	for _, tc := range []struct{ a, b string }{
		{"Orbe 2", "Orbe 3"},
		{"ZEB1", "ZEB1 Séries Films"},
		{"M7", "W8"},
	} {
		if ParseName(tc.a).Loose == ParseName(tc.b).Loose {
			t.Errorf("%q and %q match", tc.a, tc.b)
		}
	}
	for _, name := range []string{"HD", "FR: 4K", "★"} {
		if got := ParseName(name).Loose; got != "" {
			t.Errorf("%q → %q, want nothing to match", name, got)
		}
	}
}

// The exact form keeps quality tags, which tell a placeholder "ZEB1 4K"
// from "ZEB1", and drops only the dressing around the name.
func TestExactNamesKeepQualityTags(t *testing.T) {
	if a, b := ParseName("ZEB1 4K").Exact, ParseName("ZEB1").Exact; a == b {
		t.Errorf("ZEB1 4K and ZEB1 are exactly %q", a)
	}
	if a, b := ParseName("FR| Zeb 1 ᴴᴰ").Exact, ParseName("ZEB1").Exact; a != b {
		t.Errorf("exact forms %q and %q", a, b)
	}
}

func TestCountries(t *testing.T) {
	for name, want := range map[string]string{"FR| ZEB1": "fr", "[UK] Quill": "gb", "de: Lumo": "de", "SP| Zeb": "", "ENF| Zeb": "", "Zeb": ""} {
		if got := ParseName(name).Country; got != want {
			t.Errorf("%q: country %q, want %q", name, got, want)
		}
	}
	for id, want := range map[string]string{"Zeb1.fr": "fr", "Quill.uk": "gb", "zeb.DE": "de", "Zeb1": "", "loc.Zeb (WZEB) Town, MA": "", "Zeb.frx": "", "Zeb.zz": ""} {
		if got := IDCountry(id); got != want {
			t.Errorf("%q: country %q, want %q", id, got, want)
		}
	}
	for language, want := range map[string]string{"fr": "fr", "de": "de", "pt": "pt", "en": "", "fr-CA": "ca", "": ""} {
		if got := LanguageCountry(language); got != want {
			t.Errorf("%q: country %q, want %q", language, got, want)
		}
	}
}
