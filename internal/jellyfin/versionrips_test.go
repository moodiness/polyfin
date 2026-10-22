package jellyfin

import (
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// Rips of the excluded types are told by their release tags as whole
// words, and file extensions and words that only hold a tag are not.
func TestRipTypesExcludeTaggedVersions(t *testing.T) {
	t.Parallel()
	settings := accounts.DefaultSettings()
	rules := newVersionRules(settings)
	cases := []struct {
		name, filename string
		excluded       bool
	}{
		{"Movie 2026 HDCAM x264", "", true},
		{"Movie.2026.1080p.HD-TS.x264", "", true},
		{"", "Movie.2026.CAM-Rip.mkv", true},
		{"Movie 2026 DVDScr", "", true},
		{"Movie (2026) TELESYNC", "", true},
		{"Movie 2026 1080p WEB-DL", "Movie.2026.1080p.WEB-DL.mkv", false},
		{"Channel recording", "Its.Always.Sunny.S01E01.ts", false},
		{"Bats 2026 1080p BluRay", "", false},
		{"Movie 2026 Scream", "", false},
	}
	for _, c := range cases {
		got := rules.excluded(library.Version{Name: c.name, Filename: c.filename})
		if got != c.excluded {
			t.Errorf("%q / %q excluded %v, want %v", c.name, c.filename, got, c.excluded)
		}
	}
	// Excluding none keeps them all.
	settings.HiddenRipTypes = []string{}
	if newVersionRules(settings).excluded(library.Version{Name: "Movie HDCAM"}) {
		t.Error("a CAM is excluded when no rip type is")
	}
	// Each type is its own.
	settings.HiddenRipTypes = []string{"screener"}
	rules = newVersionRules(settings)
	if rules.excluded(library.Version{Name: "Movie HDCAM"}) || !rules.excluded(library.Version{Name: "Movie DVDSCR"}) {
		t.Error("screeners alone are not excluded alone")
	}
}
