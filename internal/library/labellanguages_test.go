package library

import (
	"slices"
	"testing"
)

// Labels name the regional variant of a dub by the release words for it,
// by a language tag split in two words, or by the flag of a country other
// than the one the language is named after; each also names the language.
func TestLabelsTellRegionalVariants(t *testing.T) {
	for _, tc := range []struct {
		title     string
		label     string
		languages []string
		regions   []string
	}{
		{"Movie", "Movie.2008.FRENCH.VFQ.1080p.WEB.mkv", []string{"fre"}, []string{"fr-CA"}},
		{"Movie", "Movie 2008 MULTi VFF 1080p", []string{"fre"}, []string{"fr-FR"}},
		{"Movie", "Movie.2008.TRUEFRENCH.720p", []string{"fre"}, []string{"fr-FR"}},
		{"Movie", "Movie 2008 MULTi VF2 2160p", []string{"fre"}, []string{"fr-FR", "fr-CA"}},
		{"Movie", "Movie (2008) FRENCH 1080p", []string{"fre"}, nil},
		{"Movie", "Movie.2008.VFI.1080p", []string{"fre"}, nil},
		{"Movie", "Movie 2008 1080p LATINO", []string{"spa"}, []string{"es-419"}},
		{"Movie", "Movie 2008 1080p CASTELLANO", []string{"spa"}, []string{"es-ES"}},
		{"Movie", "Movie [ES-MX] 1080p", []string{"spa"}, []string{"es-419"}},
		{"Movie", "Movie es-419 1080p", []string{"spa"}, []string{"es-419"}},
		{"Movie", "Filme 2008 1080p PT-BR Dublado", []string{"por"}, []string{"pt-BR"}},
		{"Movie", "Filme 2008 PTBR 720p", []string{"por"}, []string{"pt-BR"}},
		{"Movie", "Filme 2008 PT-PT 720p", []string{"por"}, []string{"pt-PT"}},
		{"Movie", "🇲🇽 🇬🇧", []string{"spa", "eng"}, []string{"es-419"}},
		{"Movie", "🇧🇷", []string{"por"}, []string{"pt-BR"}},
		// France's, Spain's and Portugal's flags stand for the language.
		{"Movie", "🇫🇷 🇪🇸 🇵🇹", []string{"fre", "spa", "por"}, nil},
		// A word of the title is the title's.
		{"Latino Review", "Latino.Review.2008.1080p", nil, nil},
	} {
		got := ReadLabels(tc.title, tc.label)
		if !slices.Equal(got.Languages, tc.languages) || !slices.Equal(got.Regions, tc.regions) {
			t.Errorf("%q: languages %q regions %q, want %q %q", tc.label, got.Languages, got.Regions, tc.languages, tc.regions)
		}
	}
}

// The languages labels name as the subtitles' are not the audio's: after a
// word saying subtitles, or before one that follows them, or in a tag for
// a subtitled release. MULTi-SUBS names no several audio languages.
func TestLabelsTellSubtitlesApartFromAudio(t *testing.T) {
	for _, tc := range []struct {
		title     string
		label     string
		languages []string
		regions   []string
		multi     bool
	}{
		{"Movie", "Movie (2008) 1080p | Audio: English | Subtitles: French, Spanish", []string{"eng"}, nil, false},
		{"Movie", "Movie.2008.1080p.ENG.Sub.ITA", []string{"eng"}, nil, false},
		{"Movie", "Movie 2008 1080p English French Subs", []string{"eng"}, nil, false},
		{"Movie", "Movie.2008.VOSTFR.1080p", nil, nil, false},
		{"Movie", "Movie 2008 VOST FRENCH 720p", nil, nil, false},
		{"Movie", "Movie.2008.SUBFRENCH.720p", nil, nil, false},
		{"Movie", "Movie.2008.1080p.VOSE", nil, nil, false},
		{"Movie", "Filme 2008 1080p PT-BR Legendado", nil, nil, false},
		{"Movie", "Filme 2008 1080p PT-BR Dublado", []string{"por"}, []string{"pt-BR"}, false},
		{"Movie", "Movie.2008.1080p.MULTi-SUBS.x264", nil, nil, false},
		{"Movie", "Movie 2008 MULTi VFQ Subs English", []string{"fre"}, []string{"fr-CA"}, true},
		// A title named after the word is the title's.
		{"Sub", "Sub French 2008 1080p", []string{"fre"}, nil, false},
	} {
		got := ReadLabels(tc.title, tc.label)
		if !slices.Equal(got.Languages, tc.languages) || !slices.Equal(got.Regions, tc.regions) || got.Multi != tc.multi {
			t.Errorf("%q: %+v, want languages %q regions %q multi %v", tc.label, got, tc.languages, tc.regions, tc.multi)
		}
	}
}
