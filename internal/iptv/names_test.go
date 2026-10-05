package iptv

import "testing"

// A merged channel shows its entries' name without the list's country
// prefix, quality tags and the separators they leave, but keeps the
// brackets that belong to its name.
func TestMergedNamesKeepTheirOwnBrackets(t *testing.T) {
	for name, want := range map[string]string{
		"FR| ZEB ONE (PRIME) FHD": "ZEB ONE (PRIME)",
		"FR| ORBE+CINEMA(S) HD":   "ORBE+CINEMA(S)",
		"FR| QUILL [VF] HD":       "QUILL [VF]",
		"FR| LUMO (FHD)":          "LUMO",
		"FR| TAC ( HD":            "TAC",
		"FR| PIF (BACKUP) HD":     "PIF",
		"FR| ZEB ONE - HD":        "ZEB ONE",
		"[HD] ORBE":               "ORBE",
		"FR: (":                   "FR: (",
	} {
		if got := cleanName(name); got != want {
			t.Errorf("%q: %q, want %q", name, got, want)
		}
	}
}
