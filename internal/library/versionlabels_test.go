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
