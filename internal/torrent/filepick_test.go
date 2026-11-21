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

// Without a file index or a file's name, the file of a pack of several
// movies played is the largest video whose name holds the movie's title
// right before its year, a year off either way included, else the largest
// whose name holds its title, not followed by a sequel's number, else the
// largest file. The index and the file's name still come first, and a
// torrent of one file plays it whatever its name.
func TestTheFileOfAMovieInAPack(t *testing.T) {
	t.Parallel()
	files := func(sized ...any) Info {
		var info Info
		for i := 0; i < len(sized); i += 2 {
			info.Files = append(info.Files, File{Path: sized[i].(string), Length: int64(sized[i+1].(int))})
		}
		return info
	}
	pack := files("Pack/Harbor (1990)/Harbor (1990).mkv", 700, "Pack/Harbor (1990)/Harbor.1990.Sample.mkv", 10,
		"Pack/Harbor 2 (1994).mkv", 800, "Pack/The Lighthouse Keeper (1996).mkv", 900, "Pack/Harbor (1990) Poster.jpg", 50)
	remade := files("Tide.1975.720p.mkv", 900, "Tide.2019.1080p.mkv", 500, "Tide.2.1979.mkv", 1000)
	for _, c := range []struct {
		name  string
		info  Info
		spec  Spec
		wants string
	}{
		{"by its title and year", pack, Spec{File: -1, Title: "Harbor", Year: 1990}, "Pack/Harbor (1990)/Harbor (1990).mkv"},
		{"a year off", pack, Spec{File: -1, Title: "harbor", Year: 1991}, "Pack/Harbor (1990)/Harbor (1990).mkv"},
		{"by its title alone", pack, Spec{File: -1, Title: "Harbor"}, "Pack/Harbor (1990)/Harbor (1990).mkv"},
		{"the sequel", pack, Spec{File: -1, Title: "Harbor 2", Year: 1994}, "Pack/Harbor 2 (1994).mkv"},
		{"another movie", pack, Spec{File: -1, Title: "The Lighthouse Keeper", Year: 1996}, "Pack/The Lighthouse Keeper (1996).mkv"},
		{"not in the pack", pack, Spec{File: -1, Title: "Open Water", Year: 1990}, "Pack/The Lighthouse Keeper (1996).mkv"},
		{"only its sequel in the pack", files("Harbor.2.1994.mkv", 900, "Quay.1990.mkv", 100), Spec{File: -1, Title: "Harbor", Year: 1990},
			"Harbor.2.1994.mkv"},
		{"the remake", remade, Spec{File: -1, Title: "Tide", Year: 2019}, "Tide.2019.1080p.mkv"},
		{"the first of the name", remade, Spec{File: -1, Title: "Tide", Year: 1975}, "Tide.1975.720p.mkv"},
		{"another year of the name", remade, Spec{File: -1, Title: "Tide", Year: 2005}, "Tide.1975.720p.mkv"},
		{"an apostrophe", files("Oceans.Run.2003.mkv", 100, "Other.2003.mkv", 900), Spec{File: -1, Title: "Ocean's Run", Year: 2003},
			"Oceans.Run.2003.mkv"},
		{"by its name", pack, Spec{File: -1, Name: "Harbor 2 (1994).mkv", Title: "Harbor", Year: 1990}, "Pack/Harbor 2 (1994).mkv"},
		{"by its index", pack, Spec{File: 3, Title: "Harbor", Year: 1990}, "Pack/The Lighthouse Keeper (1996).mkv"},
		{"one file", files("Movie/release.mkv", 100), Spec{File: -1, Title: "Harbor", Year: 1990}, "Movie/release.mkv"},
	} {
		file, err := c.info.File(c.spec)
		if err != nil || file.Path != c.wants {
			t.Errorf("%s: got %q %v, want %q", c.name, file.Path, err, c.wants)
		}
	}
}
