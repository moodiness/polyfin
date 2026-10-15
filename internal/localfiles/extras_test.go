package localfiles

import (
	"maps"
	"slices"
	"testing"
	"time"
)

// A scan reading extras reads them by Jellyfin's names: the videos of
// extras folders, in any case, those named with a suffix or "trailer",
// theme songs and theme videos; the others are titles, and the files of
// extras folders that are no extras are left out.
func TestReadExtra(t *testing.T) {
	t.Parallel()
	for rel, want := range map[string]struct {
		how             int
		extraType, base string
		dir             string
	}{
		"Movie (2008)/Extras/Making of.mkv":              {readAsExtra, "Unknown", "", "Movie (2008)"},
		"Movie (2008)/BEHIND THE SCENES/Crew.mkv":        {readAsExtra, "BehindTheScenes", "", "Movie (2008)"},
		"Movie (2008)/Clips/One.mp4":                     {readAsExtra, "Clip", "", "Movie (2008)"},
		"Movie (2008)/Movie (2008)-featurette.mkv":       {readAsExtra, "Featurette", "Movie (2008)", "Movie (2008)"},
		"Movie (2008)/Movie (2008) trailer.mkv":          {readAsExtra, "Trailer", "Movie (2008)", "Movie (2008)"},
		"Movie (2008)/Movie (2008).sample.mkv":           {readAsExtra, "Sample", "Movie (2008)", "Movie (2008)"},
		"Movie (2008)/Movie (2008)-deletedscene.mkv":     {readAsExtra, "DeletedScene", "Movie (2008)", "Movie (2008)"},
		"Movie (2008)/trailer.mp4":                       {readAsExtra, "Trailer", "", "Movie (2008)"},
		"Movie (2008)/backdrops/intro.mkv":               {readAsExtra, "ThemeVideo", "", "Movie (2008)"},
		"Show/theme.mp3":                                 {readAsExtra, "ThemeSong", "", "Show"},
		"Show/theme-music/Opening.flac":                  {readAsExtra, "ThemeSong", "", "Show"},
		"Show/Season 01/Extras/Bloopers.mkv":             {readAsExtra, "Unknown", "", "Show/Season 01"},
		"Movie (2008)/Movie (2008).mkv":                  {readAsTitle, "", "", ""},
		"Trailer Park (2001).mkv":                        {readAsTitle, "", "", ""},
		"Movie (2008)/BDMV/STREAM/00001.m2ts":            {readAsTitle, "", "", ""},
		"Movie (2008)/Extras/Nested/Deep.mkv":            {leftOut, "", "", ""},
		"Movie (2008)/Extras/BDMV/STREAM/00001.m2ts":     {leftOut, "", "", ""},
		"Movie (2008)/theme-music/Opening.mkv":           {leftOut, "", "", ""},
		"Movie (2008)/Trailers/Score.mp3":                {leftOut, "", "", ""},
		"Movie (2008)/Soundtrack.mp3":                    {leftOut, "", "", ""},
		"Show/Season 01/Show S01E01 - The Interview.mkv": {readAsTitle, "", "", ""},
	} {
		e, how := readExtra(rel)
		if how != want.how || e.extraType != want.extraType || e.base != want.base || how == readAsExtra && e.dir != want.dir {
			t.Errorf("%s: %d %+v, want %+v", rel, how, e, want)
		}
	}
}

// Without extras, a walk's files are those it listed before scans read
// extras: nothing of the extras folders, no sample, no audio file.
func TestSplitExtrasWithoutExtras(t *testing.T) {
	t.Parallel()
	at := time.Unix(0, 0)
	list := listing{files: map[string]found{}, sides: map[string]found{}}
	for _, rel := range []string{"Movie (2008)/Movie (2008).mkv", "Movie (2008)/Extras/Making of.mkv", "Movie (2008)/Movie (2008)-featurette.mkv",
		"Movie (2008)/trailer.mp4", "Movie (2008)/sample.mkv", "Movie (2008)/Movie-sample.mkv", "Movie (2008)/backdrops/intro.mkv",
		"Movie (2008)/Clips/One.mkv", "Movie (2008)/theme.mp3"} {
		list.files[rel] = found{size: 1, modified: at}
	}
	list.sides["Movie (2008)/Extras/Making of.srt"] = found{size: 1, modified: at}
	list.sides["Movie (2008)/Movie (2008).srt"] = found{size: 1, modified: at}
	if extras := splitExtras(KindMovies, false, &list); extras != nil {
		t.Errorf("extras without reading them: %v", extras)
	}
	want := []string{"Movie (2008)/Clips/One.mkv", "Movie (2008)/Movie (2008)-featurette.mkv", "Movie (2008)/Movie (2008).mkv",
		"Movie (2008)/backdrops/intro.mkv", "Movie (2008)/trailer.mp4"}
	if got := slices.Sorted(maps.Keys(list.files)); !slices.Equal(got, want) {
		t.Errorf("files: %v", got)
	}
	if got := slices.Sorted(maps.Keys(list.sides)); !slices.Equal(got, []string{"Movie (2008)/Movie (2008).srt"}) {
		t.Errorf("side files: %v", got)
	}
}

// An extra belongs to the movies directly in its folder, those its name
// begins with when the folder holds several, and at the folder's top only
// to the one its name begins with; a show's to its show, a season
// folder's to the season.
func TestExtraOwners(t *testing.T) {
	t.Parallel()
	movies := map[string]entry{}
	for _, rel := range []string{"Movie (2008)/Movie (2008) 1080p.mkv", "Movie (2008)/Movie (2008) 2160p.mkv", "Alone (2001).mkv", "Other (2002).mkv",
		"Disc (2003)/BDMV/STREAM/00001.m2ts"} {
		movies[rel] = readEntry(KindMovies, "Movies", rel, found{})
	}
	extras := map[string]extraFile{}
	for _, rel := range []string{"Movie (2008)/Extras/Making of.mkv", "Movie (2008)/Movie (2008) 1080p-trailer.mkv", "Alone (2001)-trailer.mkv",
		"trailer.mkv", "Extras/Lost.mkv", "Disc (2003)/Featurettes/Crew.mkv", "Nowhere/Extras/Lost.mkv"} {
		e, _ := readExtra(rel)
		extras[rel] = e
	}
	rows := extraOwners(KindMovies, extras, movies)
	for rel, want := range map[string][]string{
		"Movie (2008)/Extras/Making of.mkv":           {"Movie (2008)/Movie (2008) 1080p.mkv", "Movie (2008)/Movie (2008) 2160p.mkv"},
		"Movie (2008)/Movie (2008) 1080p-trailer.mkv": {"Movie (2008)/Movie (2008) 1080p.mkv"},
		"Alone (2001)-trailer.mkv":                    {"Alone (2001).mkv"},
		"Disc (2003)/Featurettes/Crew.mkv":            {"Disc (2003)"},
	} {
		if got := rows[rel].units; !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", rel, got, want)
		}
	}
	for _, rel := range []string{"trailer.mkv", "Extras/Lost.mkv", "Nowhere/Extras/Lost.mkv"} {
		if row, ok := rows[rel]; ok {
			t.Errorf("%s belongs to %v", rel, row.units)
		}
	}

	shows := map[string]entry{}
	for _, rel := range []string{"Show/Season 01/Show S01E01.mkv", "Show/Season 02/Show S02E01.mkv"} {
		shows[rel] = readEntry(KindShows, "Shows", rel, found{})
	}
	extras = map[string]extraFile{}
	for _, rel := range []string{"Show/theme.mp3", "Show/Season 01/theme.mp3", "Show/Specials/Extras/Pilot.mkv", "Show/Misc/Extras/Lost.mkv",
		"Gone/theme.mp3", "theme.mp3"} {
		e, _ := readExtra(rel)
		extras[rel] = e
	}
	rows = extraOwners(KindShows, extras, shows)
	for rel, want := range map[string]int{"Show/theme.mp3": -1, "Show/Season 01/theme.mp3": 1, "Show/Specials/Extras/Pilot.mkv": 0} {
		row, ok := rows[rel]
		season := -1
		if row.season != nil {
			season = *row.season
		}
		if !ok || !slices.Equal(row.units, []string{"Show"}) || season != want {
			t.Errorf("%s: %+v", rel, row)
		}
	}
	for _, rel := range []string{"Show/Misc/Extras/Lost.mkv", "Gone/theme.mp3", "theme.mp3"} {
		if row, ok := rows[rel]; ok {
			t.Errorf("%s belongs to %v", rel, row.units)
		}
	}
}

// An extra named by its suffix alone, or named "trailer", takes its NFO
// file's title, else its file's name.
func TestExtraName(t *testing.T) {
	t.Parallel()
	sides := map[string]found{"Movie/Trailer.NFO": {}}
	for rel, want := range map[string]string{
		"Movie/Extras/Making of.mkv": "Making of", "Movie/Movie-featurette.mkv": "Movie", "Movie/sample.mp4": "sample",
		"Movie/Trailer.mkv": "The first trailer",
	} {
		e, _ := readExtra(rel)
		nfo := nfoInfo{}
		if extraNFO(rel, e, sides) == "Movie/Trailer.NFO" {
			nfo.title = "The first trailer"
		}
		if got := extraName(rel, e, nfo); got != want {
			t.Errorf("%s: %q, want %q", rel, got, want)
		}
	}
}
