package torrent

import "testing"

// Without a file index, the file of a season's pack played is the one the
// addon names, else the largest video whose name tells the episode, as
// release names write it, else the largest file.
func TestTheFileOfAnEpisodeInAPack(t *testing.T) {
	t.Parallel()
	pack := func(paths ...string) Info {
		var info Info
		for i, p := range paths {
			info.Files = append(info.Files, File{Path: p, Length: int64(100 + i)})
		}
		info.Files = append(info.Files, File{Path: "Show/Show.Extras.mkv", Length: 1000})
		return info
	}
	for _, c := range []struct {
		name  string
		info  Info
		spec  Spec
		wants string
	}{
		{"by its name", pack("Show/Show.S01E01.mkv", "Show/Show.S01E02.mkv"),
			Spec{File: -1, Name: "show.s01e01.mkv", Season: 1, Episode: 2}, "Show/Show.S01E01.mkv"},
		{"S01E02", pack("Show/Show.S01E01.mkv", "Show/Show.S01E02.mkv", "Show/Show.S02E02.mkv"),
			Spec{File: -1, Name: "Another.Name.mkv", Season: 1, Episode: 2}, "Show/Show.S01E02.mkv"},
		{"s1e2 beside its sample", pack("Show/Sample/show.s1e2.sample.mkv", "Show/show.s1e2.mkv", "Show/show.s1e2.nfo"),
			Spec{File: -1, Season: 1, Episode: 2}, "Show/show.s1e2.mkv"},
		{"1x02", pack("Show/Show - 1x01 - Pilot.mkv", "Show/Show - 1x02 - Second.mkv"),
			Spec{File: -1, Season: 1, Episode: 2}, "Show/Show - 1x02 - Second.mkv"},
		{"two episodes in a file", pack("Show/Show.S01E01-E02.mkv", "Show/Show.S01E03.mkv"),
			Spec{File: -1, Season: 1, Episode: 2}, "Show/Show.S01E01-E02.mkv"},
		{"in a season's folder", pack("Show/Season 01/Episode 01.mkv", "Show/Season 01/Episode 02.mkv", "Show/Season 02/Episode 02.mkv"),
			Spec{File: -1, Season: 1, Episode: 2}, "Show/Season 01/Episode 02.mkv"},
		{"in a season's pack", pack("Show.S02.1080p/E01.mkv", "Show.S02.1080p/E02.mkv"),
			Spec{File: -1, Season: 2, Episode: 2}, "Show.S02.1080p/E02.mkv"},
		{"no episode named", pack("Show/Show.S01E01.mkv"), Spec{File: -1, Season: 1, Episode: 2}, "Show/Show.Extras.mkv"},
		{"a movie", pack("Movie/Movie.mkv"), Spec{File: -1}, "Show/Show.Extras.mkv"},
		{"by its index", pack("Show/Show.S01E01.mkv", "Show/Show.S01E02.mkv"),
			Spec{File: 0, Name: "Show.S01E02.mkv", Season: 1, Episode: 2}, "Show/Show.S01E01.mkv"},
	} {
		file, err := c.info.File(c.spec)
		if err != nil || file.Path != c.wants {
			t.Errorf("%s: got %q %v, want %q", c.name, file.Path, err, c.wants)
		}
	}
}
