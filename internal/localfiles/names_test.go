package localfiles

import "testing"

// Movie files are read the usual ways, a folder per movie completing a
// file's name, and the identifiers written in names are kept.
func TestMovieNamesAreRead(t *testing.T) {
	for _, tc := range []struct {
		path         string
		title        string
		year, height int
		imdb, tmdb   string
	}{
		{"Night of the Living Dead (1968).mkv", "Night of the Living Dead", 1968, 0, "", ""},
		{"Night.of.the.Living.Dead.1968.1080p.BluRay.x264-GROUP.mkv", "Night of the Living Dead", 1968, 1080, "", ""},
		{"Nosferatu (1922)/Nosferatu (1922) - 720p.mp4", "Nosferatu", 1922, 720, "", ""},
		// The folder gives the year a file's name lacks.
		{"The General (1926)/the.general.2160p.remux.mkv", "The General", 1926, 2160, "", ""},
		// A title that is a year keeps it; the last year is the release's.
		{"1917.1917.mkv", "1917", 1917, 0, "", ""},
		{"Metropolis.mkv", "Metropolis", 0, 0, "", ""},
		{"Metropolis.1080p.WEB-DL.mkv", "Metropolis", 0, 1080, "", ""},
		{"Mr. Smith Goes to Washington (1939).mkv", "Mr. Smith Goes to Washington", 1939, 0, "", ""},
		{"The Kid (1921) {imdb-tt0012349}.mkv", "The Kid", 1921, 0, "tt0012349", ""},
		{"The Kid (1921) [imdbid-tt0012349]/The Kid.mkv", "The Kid", 1921, 0, "tt0012349", ""},
		{"Sherlock Jr. (1924) {tmdb-992}.mkv", "Sherlock Jr.", 1924, 0, "", "992"},
		{"Sherlock Jr (1924) [tmdbid-992] [1080p].mkv", "Sherlock Jr", 1924, 1080, "", "992"},
	} {
		got := movieName(tc.path)
		if got.title != tc.title || got.year != tc.year || got.height != tc.height || got.imdb != tc.imdb || got.tmdb != tc.tmdb {
			t.Errorf("%s: %+v", tc.path, got)
		}
	}
}

// A disc is read by its folder's name, the folder above's when it only
// numbers the disc, the folder's own at the root; its version is named
// alike.
func TestDiscNamesAreRead(t *testing.T) {
	for _, tc := range []struct {
		path, root   string
		unit, title  string
		year, height int
		label        string
	}{
		{"Movie (2001)/BDMV/STREAM/00001.m2ts", "movies", "Movie (2001)", "Movie", 2001, 0, "Movie (2001)"},
		{"Movie.2001.1080p.BluRay/BDMV/STREAM/00001.m2ts", "movies", "Movie.2001.1080p.BluRay", "Movie", 2001, 1080, "Movie.2001.1080p.BluRay"},
		{"Other (1999)/Disc 1/BDMV/STREAM/00001.m2ts", "movies", "Other (1999)/Disc 1", "Other", 1999, 0, "Other (1999) - Disc 1"},
		{"Other (1999)/DISC01/BDMV/STREAM/00001.m2ts", "movies", "Other (1999)/DISC01", "Other", 1999, 0, "Other (1999) - DISC01"},
		{"Other/disk_2/bdmv/stream/00001.M2TS", "movies", "Other/disk_2", "Other", 0, 0, "Other - disk_2"},
		{"Other/CD1/BDMV/STREAM/00001.m2ts", "movies", "Other/CD1", "Other", 0, 0, "Other - CD1"},
		{"Other (1999)/BD 1/BDMV/STREAM/00001.m2ts", "movies", "Other (1999)/BD 1", "Other", 1999, 0, "Other (1999) - BD 1"},
		{"Other (1999)/Part.2/BDMV/STREAM/00001.m2ts", "movies", "Other (1999)/Part.2", "Other", 1999, 0, "Other (1999) - Part.2"},
		// The folder above gives the year the disc's name lacks.
		{"Nosferatu (1922)/Nosferatu/BDMV/STREAM/00001.m2ts", "movies", "Nosferatu (1922)/Nosferatu", "Nosferatu", 1922, 0, "Nosferatu"},
		{"Box/DISC01_xyz/BDMV/STREAM/00010.m2ts", "movies", "Box/DISC01_xyz", "DISC01 xyz", 0, 0, "DISC01_xyz"},
		{"Disc 2/BDMV/STREAM/00001.m2ts", "Other (1999)", "Disc 2", "Other", 1999, 0, "Other (1999) - Disc 2"},
		{"BDMV/STREAM/00001.m2ts", "The General (1926)", ".", "The General", 1926, 0, "The General (1926)"},
	} {
		disc, ok := discStream(tc.path)
		e := readEntry(KindMovies, tc.root, tc.path, found{})
		if !ok || e.unit != tc.unit || e.title.title != tc.title || e.title.year != tc.year || e.title.height != tc.height ||
			discLabel(disc, tc.root) != tc.label {
			t.Errorf("%s: %v %s %+v %s", tc.path, ok, e.unit, e.title, discLabel(disc, tc.root))
		}
	}
	for _, rel := range []string{"Movie/BDMV/00001.m2ts", "Movie/BDMV/STREAM/sub/00001.m2ts", "Movie/BDMV/STREAM/00001.mkv", "Movie/STREAM/00001.m2ts"} {
		if _, ok := discStream(rel); ok {
			t.Errorf("%s is read as a disc's stream", rel)
		}
	}
	for location, name := range map[string]string{
		"/media/movies":                          "movies",
		"/media/The General (1926)":              "The General (1926)",
		"smb://nas/share":                        "share",
		"https://nas/dav/The%20General%20(1926)": "The General (1926)",
		"https://nas":                            "",
	} {
		if got := locationName(location); got != name {
			t.Errorf("%s: %q", location, got)
		}
	}
}

// Episodes are read from their file's name, the show from its folder, and
// a file holding several episodes covers them all.
func TestEpisodeNamesAreRead(t *testing.T) {
	for _, tc := range []struct {
		path                         string
		show                         string
		year                         int
		imdb                         string
		unit                         string
		season, episode, lastEpisode int
		episodic                     bool
	}{
		{"The Lone Ranger (1949)/Season 01/The Lone Ranger - S01E02.mkv", "The Lone Ranger", 1949, "", "The Lone Ranger (1949)", 1, 2, 2, true},
		{"The Lone Ranger/The.Lone.Ranger.S02E10.720p.mkv", "The Lone Ranger", 0, "", "The Lone Ranger", 2, 10, 10, true},
		{"The Lone Ranger/Season 1/The Lone Ranger - s01e01-e03.mkv", "The Lone Ranger", 0, "", "The Lone Ranger", 1, 1, 3, true},
		{"The Lone Ranger/Show S01E01E02.mkv", "The Lone Ranger", 0, "", "The Lone Ranger", 1, 1, 2, true},
		{"The Lone Ranger/Show S01E05-06 Title.mkv", "The Lone Ranger", 0, "", "The Lone Ranger", 1, 5, 6, true},
		{"The Lone Ranger/Show 3x04.mkv", "The Lone Ranger", 0, "", "The Lone Ranger", 3, 4, 4, true},
		// A number after the episode is not another episode.
		{"The Lone Ranger/Show - S01E01 - 1949 Pilot 1080p.mkv", "The Lone Ranger", 0, "", "The Lone Ranger", 1, 1, 1, true},
		{"The Lone Ranger {imdb-tt0041038}/Season 02/Episode 7.mkv", "The Lone Ranger", 0, "tt0041038", "The Lone Ranger {imdb-tt0041038}", 2, 7, 7, true},
		{"Specials/S00E01.mkv", "", 0, "", "Specials/S00E01.mkv", 0, 1, 1, true},
		// At the folder's top, the file names its show and is its unit.
		{"The.Cisco.Kid.1950.S01E03.mkv", "The Cisco Kid", 1950, "", "The.Cisco.Kid.1950.S01E03.mkv", 1, 3, 3, true},
		{"The Lone Ranger/Bonus.mkv", "The Lone Ranger", 0, "", "The Lone Ranger", 0, 0, 0, false},
	} {
		show, episode, unit := episodeName(tc.path)
		if show.title != tc.show || show.year != tc.year || show.imdb != tc.imdb || unit != tc.unit || episode.episodic != tc.episodic ||
			episode.season != tc.season || episode.episode != tc.episode || episode.lastEpisode != tc.lastEpisode {
			t.Errorf("%s: show %+v, episode %+v, unit %q", tc.path, show, episode, unit)
		}
	}
}

// Titles compare without case, accents nor punctuation.
func TestTitlesFold(t *testing.T) {
	if fold("Les Misérables") != fold("les miserables") || fold("Sherlock Jr.") != fold("Sherlock Jr") || fold("Mr. & Mrs.") != "mr and mrs" ||
		fold("Night of the Living Dead") == fold("Dawn of the Dead") {
		t.Errorf("folded: %q %q", fold("Les Misérables"), fold("Mr. & Mrs."))
	}
}
