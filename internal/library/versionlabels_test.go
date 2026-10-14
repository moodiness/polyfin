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
