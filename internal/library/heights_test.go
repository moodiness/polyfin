package library

import "testing"

func TestLabelHeightReadsAddonsLabels(t *testing.T) {
	for _, tc := range []struct {
		labels []string
		want   int
	}{
		// Names, descriptions and file names as stream addons write them.
		{[]string{"Addon\n4K", "Movie.Name.2023.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265\n👤 12 💾 15.2 GB", "Movie.Name.2023.2160p.WEB-DL.mkv"}, 2160},
		{[]string{"Addon 1080p", "Movie Name (2023) [1080p] BluRay x264 DTS-HD MA 5.1"}, 1080},
		{[]string{"", "", "", "Show.S01E02.720p.HDTV.x264.mkv"}, 720},
		{[]string{"Addon\nUHD", "Remux · HEVC · 10bit"}, 2160},
		{[]string{"Addon Ultra HD"}, 2160},
		{[]string{"Addon\nFull HD", "WEB"}, 1080},
		{[]string{"FHD | H264"}, 1080},
		{[]string{"Addon\nHD"}, 720},
		{[]string{"SD · XviD"}, 480},
		{[]string{"Movie.1440p.WEB.mkv"}, 1440},
		{[]string{"Addon 2K", "QHD"}, 1440},
		{[]string{"Movie.Name.576p.DVDRip.mkv"}, 576},
		{[]string{"Movie 480P"}, 480},
		{[]string{"Broadcast 1080i MPEG-2"}, 1080},
		{[]string{"Video 1920x1080 AAC"}, 1080},
		// Lines win over words, which releases use loosely.
		{[]string{"Movie.4K.Remastered.1080p.BluRay.x264"}, 1080},
		{[]string{"Addon HD", "Movie.2023.720p.WEB.mkv"}, 720},
		// Labels that agree, in several places.
		{[]string{"4K", "2160p", "UHD"}, 2160},
	} {
		if got := LabelHeight(tc.labels...); got != tc.want {
			t.Errorf("%q: %d, want %d", tc.labels, got, tc.want)
		}
	}
}

func TestLabelHeightLeavesUnknownWhatItCannotTell(t *testing.T) {
	for _, labels := range [][]string{
		// No label at all.
		{"Addon", "Movie Name (2023) WEB-DL", "movie.mkv"},
		{},
		// Near misses: codecs, ranges, bit depths, audio and sizes.
		{"HDR10 · DV · HEVC · 10bit · x265 · H.264"},
		{"HDRip · HDTV · WEBRip"},
		{"DTS-HD MA 7.1", "Dolby True HD Atmos", "HD audio"},
		{"Movie 1080 BluRay", "2160 WEB"},
		{"Movie.1200p.mkv", "10800p", "p", "1080pp"},
		{"Episode S04K", "4KB", "x1080", "12x1080"},
		{"💾 2.1 GB ⚙️ 1080"},
		// Labels that disagree.
		{"Addon 1080p", "Movie.2160p.mkv"},
		{"Addon 4K", "Addon HD"},
	} {
		if got := LabelHeight(labels...); got != 0 {
			t.Errorf("%q: %d, want unknown", labels, got)
		}
	}
}
