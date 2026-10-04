package xmltv

import "testing"

// IPTV lists dress channel names up; guides name channels plainly. Both
// fold to the same name.
func TestChannelNamesMatchAcrossTheirDressing(t *testing.T) {
	for _, tc := range []struct{ listed, guide string }{
		{"TF1 HD", "TF1"},
		{"TF1ᴴᴰ", "TF1"},
		{"FR: TF1 FHD", "TF1"},
		{"|FR| TF 1 UHD", "TF1"},
		{"[UK] BBC One 4K", "BBC ONE"},
		{"FR | France 2 SD", "France 2"},
		{"France 3 ⁴ᴷ", "France 3"},
		{"Arte ★ HEVC", "ARTE"},
		{"Télé-Matin", "tele matin"},
		{"Canal+ Sport 1080p", "Canal+ Sport"},
		{"Équipe 21 ²", "Equipe 21"},
		{"RMC Découverte | HD", "RMC Decouverte"},
		{"Ciné+ Frisson (HD)", "Cine+ Frisson"},
	} {
		if a, b := NormalizeName(tc.listed), NormalizeName(tc.guide); a != b || a == "" {
			t.Errorf("%q → %q, %q → %q", tc.listed, a, tc.guide, b)
		}
	}
	for _, tc := range []struct{ a, b string }{
		{"France 2", "France 3"},
		{"TF1", "TF1 Séries Films"},
		{"M6", "W9"},
	} {
		if NormalizeName(tc.a) == NormalizeName(tc.b) {
			t.Errorf("%q and %q match", tc.a, tc.b)
		}
	}
	for _, name := range []string{"HD", "FR: 4K", "★"} {
		if got := NormalizeName(name); got != "" {
			t.Errorf("%q → %q, want nothing to match", name, got)
		}
	}
}
