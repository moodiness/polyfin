package library

import (
	"slices"
	"testing"
)

// Labels tell a stream's codec, range and audio languages as releases and
// addons write them; words of the title name no language, and labels
// describing the video without HDR tell SDR.
func TestLabelsTellCodecRangeAndLanguages(t *testing.T) {
	for _, tc := range []struct {
		title  string
		labels []string
		want   Labels
	}{
		{"Movie", []string{"Movie.2008.2160p.UHD.BluRay.x265.HDR10.DTS-HD.MA.5.1-GROUP.mkv"},
			Labels{Codec: "hevc", Range: HighRange}},
		{"Movie", []string{"Server\n1080p", "Movie 2008 H.264 AAC"}, Labels{Codec: "h264", Range: StandardRange}},
		{"Movie", []string{"4K DV", "Dolby Vision HEVC"}, Labels{Codec: "hevc", Range: HighRange}},
		{"Movie", []string{"Movie (2008) MULTi VFF 1080p AV1"}, Labels{Codec: "av1", Range: StandardRange, Languages: []string{"fre"}, Multi: true}},
		{"Movie", []string{"🇫🇷 🇬🇧 Dual Audio"}, Labels{Languages: []string{"fre", "eng"}, Multi: true}},
		{"Movie", []string{"ITA ENG", "Italian"}, Labels{Languages: []string{"ita", "eng"}}},
		// A code counts in capitals only: "may" is a word.
		{"Movie", []string{"may be slow", "cat"}, Labels{}},
		// The title's words are the title's.
		{"The French Dispatch", []string{"The.French.Dispatch.2021.1080p.x264.mkv"}, Labels{Codec: "h264", Range: StandardRange}},
		{"The French Dispatch", []string{"The.French.Dispatch.2021.GERMAN.1080p"}, Labels{Range: StandardRange, Languages: []string{"ger"}}},
		// Codecs that disagree name none.
		{"Movie", []string{"x264", "HEVC"}, Labels{}},
		{"Movie", []string{"Movie SDR VC-1"}, Labels{Codec: "vc1", Range: StandardRange}},
		{"Movie", []string{"Cached", "Movie.2008.mkv"}, Labels{}},
	} {
		got := ReadLabels(tc.title, tc.labels...)
		if got.Codec != tc.want.Codec || got.Range != tc.want.Range || !slices.Equal(got.Languages, tc.want.Languages) || got.Multi != tc.want.Multi {
			t.Errorf("%q in %q: %+v, want %+v", tc.labels, tc.title, got, tc.want)
		}
	}
}

// Labels tag low-quality releases by whole words after the title's own,
// in any letter case: a title named after a tag, a tag inside a word and
// a .ts file are none; each kind's tags are.
func TestReleaseKindsReadTagsAfterTheTitle(t *testing.T) {
	for _, tc := range []struct {
		titles []string
		labels []string
		want   []string
	}{
		// The movie Cam, its name and others'.
		{[]string{"Cam"}, []string{"Cam.2018.1080p.WEB-DL.mkv"}, nil},
		{[]string{"The Cam"}, []string{"The Cam (2019) 1080p", "The.Cam.2019.mkv"}, nil},
		{[]string{"Cam"}, []string{"Cam.2018.HDCAM.mkv"}, []string{"cam"}},
		// The title's name spelled otherwise: the year ends its words.
		{[]string{"The Cam"}, []string{"Cam.2019.WEB.mkv"}, nil},
		{[]string{"Some Movie"}, []string{"Some.Movie.2026.HDCAM.x264.mkv"}, []string{"cam"}},
		{[]string{"Some Movie"}, []string{"Some.Movie.2026.1080p.WEB.ts"}, nil},
		{[]string{"Some Movie"}, []string{"Some.Movie.2026.TS.x264.ts"}, []string{"telesync"}},
		{[]string{"It's Always Something"}, []string{"Its.Always.Something.S01E01.1080p.mkv"}, nil},
		{[]string{"Some Show", "Pilot"}, []string{"Some.Show.S01E01.TS.mkv"}, []string{"telesync"}},
		// An episode's own name is the title's.
		{[]string{"Some Show", "The Cam"}, []string{"Some.Show.S01E02.The.Cam.1080p.mkv"}, nil},
		{[]string{"Some Movie"}, []string{"Some Movie (2026) [HDTS] [x264]"}, []string{"telesync"}},
		{[]string{"Some Movie"}, []string{"some.movie.2026.dvdscr.mkv"}, []string{"screener"}},
		{[]string{"Some Movie"}, []string{"Server\nCAM 720p", "Some_Movie_2026_TELECINE"}, []string{"cam", "telecine"}},
		{[]string{"Some Movie"}, []string{"Some.Movie.2026.TC.mkv", "Some-Movie-2026-SCR"}, []string{"telecine", "screener"}},
		{[]string{"Some Movie"}, []string{"Some.Movie.2026.R5.LiNE", "Some Movie 2026 WORKPRINT"}, []string{"early"}},
		{[]string{"Some Movie"}, []string{"HDCAM\n1080p", "TELESYNC 720p"}, []string{"cam", "telesync"}},
		// Tags inside words, and words like tags, are none.
		{[]string{"Some Movie"}, []string{"Some.Movie.2026.Cameo.Nights.TSX.DTS.mkv"}, nil},
		{[]string{"Some Movie"}, []string{"Some.Movie.2026.2160p.WEB-DL.DDP5.1.mkv"}, nil},
	} {
		if got := ReleaseKinds(tc.titles, tc.labels...); !slices.Equal(got, tc.want) {
			t.Errorf("%q of %q: %q, want %q", tc.labels, tc.titles, got, tc.want)
		}
	}
}

// A release names another title when its markers contradict the episode
// or the movie: another episode, a season without it, a movie's year more
// than one off. Packs and ranges holding the episode, and names without
// markers, name none.
func TestOtherTitleReadsSeasonsEpisodesAndYears(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		season, episode, year int
		labels                []string
		want                  string
	}{
		// The episode itself, as releases write it.
		{"Some Show", 1, 2, 0, []string{"Some.Show.S01E02.1080p.WEB.h264.mkv"}, ""},
		{"Some Show", 1, 2, 0, []string{"Some Show 1x02 720p"}, ""},
		{"Some Show", 1, 2, 0, []string{"some_show_s01_e02_hdtv"}, ""},
		{"Some Show", 1, 2, 0, []string{"Some Show - Season 1 Episode 2 [1080p]"}, ""},
		{"Some Show", 10, 2, 0, []string{"Some.Show.S10E02.mkv"}, ""},
		// Another episode, or another season's.
		{"Some Show", 1, 2, 0, []string{"Some.Show.S01E03.1080p.WEB.h264.mkv"}, "S01E03"},
		{"Some Show", 1, 2, 0, []string{"Server\n4K", "Some Show 1x12 HDTV"}, "S01E12"},
		{"Some Show", 1, 2, 0, []string{"Some.Show.S02E02.720p"}, "S02E02"},
		{"Some Show", 1, 2, 0, []string{"Some Show S01 E05 [1080p]"}, "S01E05"},
		// A season pack, or a pack of seasons, without it.
		{"Some Show", 1, 2, 0, []string{"Some.Show.S02.COMPLETE.1080p"}, "S02"},
		{"Some Show", 1, 2, 0, []string{"Some Show Season 3 1080p"}, "S03"},
		{"Some Show", 1, 2, 0, []string{"Some.Show.S02-S04.1080p"}, "S02-S04"},
		{"Some Show", 1, 2, 0, []string{"Some.Show.S01E05-E08.1080p"}, "S01E05-E08"},
		// Packs and ranges holding it are kept: the file is picked in the
		// torrent.
		{"Some Show", 1, 2, 0, []string{"Some.Show.S01.COMPLETE.1080p.WEB"}, ""},
		{"Some Show", 2, 7, 0, []string{"Some.Show.S01-S03.1080p"}, ""},
		{"Some Show", 2, 7, 0, []string{"Some Show Seasons 1-3 Complete"}, ""},
		{"Some Show", 2, 7, 0, []string{"Some Show S01-03 1080p"}, ""},
		{"Some Show", 1, 2, 0, []string{"Some.Show.S01E01-E04.1080p"}, ""},
		{"Some Show", 1, 2, 0, []string{"Some.Show.S01E01-03.1080p"}, ""},
		{"Some Show", 1, 2, 0, []string{"Some.Show.S01E01E02.1080p"}, ""},
		{"Some Show", 1, 2, 0, []string{"Some Show 1x01-1x03 720p"}, ""},
		{"Some Show", 1, 2, 0, []string{"Some Show S01 [E01-E10] 1080p"}, ""},
		// A pack naming the file it plays, of another episode.
		{"Some Show", 1, 2, 0, []string{"Some.Show.S01.1080p.WEB\nSome.Show.S01E06.mkv"}, "S01E06"},
		// Names without markers, and complete series.
		{"Some Show", 1, 2, 0, []string{"Some Show Complete Series 1080p"}, ""},
		{"Some Show", 1, 2, 0, []string{"Server\n1080p", "👤 12 💾 1.2 GB"}, ""},
		{"Some Show", 1, 2, 0, []string{"Some Show - 02 [1080p]"}, ""},
		// The series' own words are no marker; neither are a resolution and
		// codec.
		{"S2 Station", 1, 2, 0, []string{"S2.Station.S01E02.1920x1080.x265"}, ""},
		{"1x1 Club", 1, 2, 0, []string{"1x1 Club S01E02"}, ""},
		// A movie's year, one off at most.
		{"Some Movie", 0, 0, 2008, []string{"Some.Movie.2008.1080p.BluRay.x264.mkv"}, ""},
		{"Some Movie", 0, 0, 2008, []string{"Some Movie (2009) 2160p"}, ""},
		{"Some Movie", 0, 0, 2008, []string{"Some.Movie.2007.720p"}, ""},
		{"Some Movie", 0, 0, 2008, []string{"Some.Movie.1985.1080p.BluRay.mkv"}, "1985"},
		{"Some Movie", 0, 0, 2008, []string{"Server", "Some Movie (2019) [1080p]"}, "2019"},
		// A year that is the title's, or after other words, or in a line not
		// naming the movie, tells nothing.
		{"1917", 0, 0, 2019, []string{"1917.2019.1080p.mkv"}, ""},
		{"Space Odyssey 2001", 0, 0, 1968, []string{"Space.Odyssey.2001.1968.mkv"}, ""},
		{"Some Movie", 0, 0, 2008, []string{"Some.Movie.Remastered.2020.mkv"}, ""},
		{"Some Movie", 0, 0, 2008, []string{"Un Film Traduit 1990 1080p"}, ""},
		{"Some Movie", 0, 0, 0, []string{"Some.Movie.1985.mkv"}, ""},
		// A movie's markers are not read as an episode's.
		{"Some Movie", 0, 0, 2008, []string{"Some Movie 2008 S01E01"}, ""},
	} {
		if got := OtherTitle(tc.name, tc.season, tc.episode, tc.year, tc.labels...); got != tc.want {
			t.Errorf("%q for %q S%dE%d %d: %q, want %q", tc.labels, tc.name, tc.season, tc.episode, tc.year, got, tc.want)
		}
	}
}
