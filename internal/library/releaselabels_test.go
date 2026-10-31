package library

import "testing"

// Labels tell a release's source tier, the best they name: a full disc
// copy before the encode its words also name; the title's own words name
// none.
func TestReadReleaseTellsTheSourceTier(t *testing.T) {
	for _, tc := range []struct {
		titles []string
		labels []string
		want   string
	}{
		{[]string{"Movie"}, []string{"Movie.2008.2160p.UHD.BluRay.REMUX.HDR.HEVC.mkv"}, "disc"},
		{[]string{"Movie"}, []string{"Movie 2008 COMPLETE BLURAY"}, "disc"},
		{[]string{"Movie"}, []string{"Movie.2008.1080p.Blu-ray.x264.mkv"}, "bluray"},
		{[]string{"Movie"}, []string{"Movie (2008) BDRip 720p"}, "bluray"},
		{[]string{"Movie"}, []string{"Server\n4K", "Movie.2008.2160p.WEB-DL.DDP5.1.mkv"}, "web"},
		{[]string{"Movie"}, []string{"Movie.2008.1080p.WEBRip.x265"}, "web"},
		{[]string{"Movie"}, []string{"Movie.2008.DVDRip.XviD"}, "dvd"},
		{[]string{"Some Show"}, []string{"Some.Show.S01E01.720p.HDTV.x264"}, "tv"},
		// The title's words are the title's.
		{[]string{"Web"}, []string{"Web.2008.1080p.x264.mkv"}, ""},
		{[]string{"Movie"}, []string{"Movie.2008.1080p.mkv"}, ""},
		// A screener is no DVD.
		{[]string{"Movie"}, []string{"Movie.2008.DVDSCR"}, ""},
	} {
		if got, _, _ := ReadRelease(tc.titles, tc.labels...); got != tc.want {
			t.Errorf("%q of %q: %q, want %q", tc.labels, tc.titles, got, tc.want)
		}
	}
}

// Labels tell a release's best audio: lossless, object based and its
// channels, as releases write them glued to the codec or apart; DTS-HD
// without MA is lossy, and a year or a height is no channel layout.
func TestReadReleaseTellsTheAudioFormat(t *testing.T) {
	for _, tc := range []struct {
		labels []string
		want   AudioFormat
	}{
		{[]string{"Movie.2008.2160p.BluRay.TrueHD.Atmos.7.1.mkv"}, AudioFormat{Lossless: true, Object: true, Channels: 8}},
		{[]string{"Movie.2008.1080p.BluRay.DTS-HD.MA.5.1.x264"}, AudioFormat{Lossless: true, Channels: 6}},
		{[]string{"Movie 2008 DTS-HD HRA 5.1"}, AudioFormat{Channels: 6}},
		{[]string{"Movie.2008.WEB-DL.DDP5.1.Atmos.mkv"}, AudioFormat{Object: true, Channels: 6}},
		{[]string{"Movie (2008) DTS:X 7.1"}, AudioFormat{Object: true, Channels: 8}},
		{[]string{"Movie.2008.1080p.WEB.AAC2.0"}, AudioFormat{Channels: 2}},
		{[]string{"Movie.2008.FLAC.2.0", "5.1"}, AudioFormat{Lossless: true, Channels: 6}},
		{[]string{"Movie.2008.1080p.x264.mkv", "Movie.2008.Part.2.10bit.H.264"}, AudioFormat{}},
	} {
		if _, got, _ := ReadRelease([]string{"Movie"}, tc.labels...); got != tc.want {
			t.Errorf("%q: %+v, want %+v", tc.labels, got, tc.want)
		}
	}
}

// Labels tell a release's edition, the first named, whichever way they
// write a director's cut; a title naming an edition names none.
func TestReadReleaseTellsTheEdition(t *testing.T) {
	for _, tc := range []struct {
		titles []string
		labels []string
		want   string
	}{
		{[]string{"Movie"}, []string{"Movie.2008.Theatrical.1080p"}, "theatrical"},
		{[]string{"Movie"}, []string{"Movie 2008 EXTENDED 2160p"}, "extended"},
		{[]string{"Movie"}, []string{"Movie (2008) Director's Cut"}, "directors"},
		{[]string{"Movie"}, []string{"Movie.2008.Directors.Cut.1080p"}, "directors"},
		{[]string{"Movie"}, []string{"Movie.2008.IMAX.Extended.2160p"}, "imax"},
		{[]string{"Movie"}, []string{"Movie.2008.1080p"}, ""},
		{[]string{"The Extended"}, []string{"The.Extended.2008.1080p"}, ""},
	} {
		if _, _, got := ReadRelease(tc.titles, tc.labels...); got != tc.want {
			t.Errorf("%q of %q: %q, want %q", tc.labels, tc.titles, got, tc.want)
		}
	}
}
