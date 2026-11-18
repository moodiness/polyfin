package admin

import (
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/i18n"
)

// A share whose scan met a bound of what one scan reads is told as such,
// in every language, not as a share to check the address or the user of:
// its page tells which bound.
func TestHealthTellsAShareThatHoldsMoreThanAScanReads(t *testing.T) {
	t.Parallel()
	want := map[string]string{
		"en": "Movies: this network share holds more than Polyfin reads in one scan. Its page tells which limit it met.",
		"fr": "Movies\u202f: ce partage réseau contient plus que ce que Polyfin lit en une analyse. Sa page indique la limite atteinte.",
	}
	for _, failure := range []string{"too_deep", "too_many_folders", "too_many_files", "listing_too_large"} {
		p := healthProblemJSON{Code: problemFolder, Name: "Movies", Failure: failure, Share: "webdav"}
		other := healthProblemJSON{Code: problemFolder, Name: "Movies", Failure: "missing", Share: "webdav"}
		for _, language := range i18n.Languages {
			got := p.text(language)
			if got == other.text(language) || !strings.HasPrefix(got, "Movies") {
				t.Errorf("%s in %s: %s", failure, language, got)
			}
			if told, ok := want[language]; ok && got != told {
				t.Errorf("%s in %s:\n%s\nwant\n%s", failure, language, got, told)
			}
		}
	}
}
