package library

import (
	"fmt"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// fixtureEpisodeID is the TMDB identifier of the fixture's episode TMDB
// numbers season and number, as the addon's moviedb_id and a server's
// episode Tmdb identifier give it.
func fixtureEpisodeID(season, number int) int {
	return 100000 + 100*season + number
}

// jellyfinAired is the time a Jellyfin server east of Greenwich keeps for
// an episode TMDB tells aired day: midnight in UTC+2, the evening before
// in UTC.
func jellyfinAired(day string) time.Time {
	t, err := time.Parse(time.DateOnly, day)
	if err != nil {
		panic(err)
	}
	return t.Add(-2 * time.Hour)
}

// serverSource is the fixture's episodes as a server numbering them as
// TMDB does holds them, with their TMDB identifiers when ids, else none.
func serverSource(fixture []airedFixture, ids bool) *SourceEpisodes {
	source := &SourceEpisodes{}
	for _, e := range fixture {
		if e.number == 0 {
			continue
		}
		episode := SourceEpisode{Season: e.season, Number: e.number, Aired: jellyfinAired(e.day), Name: e.name}
		if ids {
			episode.TMDB = fixtureEpisodeID(e.season, e.number)
		}
		source.Episodes = append(source.Episodes, episode)
	}
	return source
}

// heldOnly is source with only its episodes of places, as a library
// holding some files of a series.
func heldOnly(source *SourceEpisodes, places ...[2]int) *SourceEpisodes {
	kept := &SourceEpisodes{OtherOrder: source.OtherOrder}
	for _, e := range source.Episodes {
		if slices.Contains(places, [2]int{e.Season, e.Number}) {
			kept.Episodes = append(kept.Episodes, e)
		}
	}
	return kept
}

// describedVideos are the fixture's episodes as the addon describes them,
// "<imdb>:<season>:<episode>", with the TMDB identifiers of TMDB's
// episodes when ids.
func describedVideos(imdb string, fixture []airedFixture, ids bool) []stremio.Video {
	var videos []stremio.Video
	for _, f := range fixture {
		if f.addonNumber == 0 {
			continue
		}
		video := stremio.Video{ID: fmt.Sprintf("%s:%d:%d", imdb, f.addonSeason, f.addonNumber), Season: stremio.Number(f.addonSeason),
			Episode: stremio.Number(f.addonNumber), Name: cmpOrString(f.addonName, f.name), Released: addonDate(f.day)}
		if ids && f.number > 0 {
			video.MoviedbID = stremio.Text(strconv.Itoa(fixtureEpisodeID(f.season, f.number)))
		}
		videos = append(videos, video)
	}
	return videos
}

// countEpisodes counts the episodes Polyfin keeps of the series imdb.
func countEpisodes(t *testing.T, e env, imdb string) int {
	t.Helper()
	var count int
	if err := e.service.db.QueryRow(t.Context(), `SELECT count(*) FROM items WHERE kind = 'episode' AND data->>'seriesId' = $1`,
		imdb).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestNearestDayRoundsAServersTimes(t *testing.T) {
	t.Parallel()
	for text, want := range map[string]string{
		// Midnight in UTC+2, UTC-5 and UTC, as Jellyfin keeps them.
		"2021-06-10T22:00:00Z": "2021-06-11", "2021-06-11T05:00:00Z": "2021-06-11", "2021-06-11T00:00:00Z": "2021-06-11",
		"2021-06-11T11:59:00Z": "2021-06-11", "2021-06-11T12:00:00Z": "2021-06-12",
	} {
		at, err := time.Parse(time.RFC3339, text)
		if err != nil {
			t.Fatal(err)
		}
		if got := nearestDay(at).Format(time.DateOnly); got != want {
			t.Errorf("%s: %s, want %s", text, got, want)
		}
	}
	if !nearestDay(time.Time{}).IsZero() {
		t.Error("no time gave a day")
	}
	// Truncated rather than rounded, the evening before is two days from
	// the addon's date, too far to pair.
	addon := airedEpisode{day: episodeDay(addonDate("2021-06-11"))}
	if aired := jellyfinAired("2021-06-11"); !nearDays(addon, airedEpisode{day: nearestDay(aired)}) ||
		nearDays(addon, airedEpisode{day: episodeDay(aired.Format(time.RFC3339))}) {
		t.Error("the server's day is not rounded to the nearest")
	}
}

// What a server holds of a series is placed among the addon's episodes by
// the episodes' own identifiers, else by their days and names, else kept
// at its numbers where the series is shown numbered alike, and nowhere
// otherwise: no episode the addon does not list is made up.
func TestServerEpisodesArePlacedByIDsThenDaysAndNames(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	described := &fakeAddon{
		manifest: stremio.Manifest{ID: "described", Name: "Described", Version: "1", Types: []string{"series"},
			Resources: []stremio.Resource{{Name: "meta"}}, IDPrefixes: []string{"tt"}},
		metas: map[string]stremio.Meta{},
	}
	describe := func(imdb string, videos []stremio.Video) {
		described.metas["series/"+imdb] = stremio.Meta{ID: imdb, Type: "series", Name: "Series " + imdb, Videos: videos}
	}
	// DVD order: the server numbers the third aired second, and the second
	// third, and holds the fifth undated and unnamed.
	dvd := weekly(1, 6, "2019-01-06")
	dvdSource := serverSource(dvd, false)
	dvdSource.OtherOrder = true
	dvdSource.Episodes[1].Number, dvdSource.Episodes[2].Number = 3, 2
	dvdSource.Episodes[4].Aired, dvdSource.Episodes[4].Name = time.Time{}, ""
	// The episodes' own identifiers win over their days: the server's
	// fourth names the addon's fifth.
	byID := weekly(1, 6, "2018-01-07")
	byIDSource := serverSource(byID, false)
	byIDSource.Episodes[3].TMDB = fixtureEpisodeID(1, 5)
	// Identifiers that name two episodes place none.
	conflict := weekly(1, 4, "2017-01-08")
	conflictSource := serverSource(conflict, false)
	conflictSource.Episodes[1].TMDB = fixtureEpisodeID(1, 2)
	conflictSource.Episodes[1].TVDB = 77
	conflictVideos := describedVideos("tt0000730", conflict, true)
	conflictVideos[2].TvdbID = "77"
	// Without identifiers, days or names, the numbers stay.
	bare := &SourceEpisodes{Episodes: []SourceEpisode{{Season: 1, Number: 3}}}
	together := releasedTogether(1, 1, 8, "2020-03-20")
	for imdb, videos := range map[string][]stremio.Video{
		"tt2531336": describedVideos("tt2531336", lupin(), true),
		"tt2531337": describedVideos("tt2531337", lupin(), false),
		"tt2531338": describedVideos("tt2531338", lupin(), false),
		"tt0239195": describedVideos("tt0239195", survivorSecondSeason(), true),
		"tt0000600": describedVideos("tt0000600", dvd, false),
		"tt0000400": describedVideos("tt0000400", weekly(1, 8, "2021-02-07"), false),
		"tt0000700": describedVideos("tt0000700", byID, true),
		"tt0000730": conflictVideos,
		"tt0000800": describedVideos("tt0000800", together, false),
		"tt0000900": describedVideos("tt0000900", weekly(1, 5, "2016-02-07"), false),
	} {
		describe(imdb, videos)
	}
	// The addon does not describe the split season: Polyfin's listing of
	// it stands for its list.
	listSeries(t, e, "tt0001200", splitSeason())
	e.install(addons.Shared(), described)

	lupinIDs, lupinDays := serverSource(lupin(), true), serverSource(lupin(), false)
	split := serverSource(splitSeason(), false)
	survivor := serverSource(survivorSecondSeason(), false)
	alike := serverSource(weekly(1, 8, "2021-02-07"), false)
	ref := func(imdb string, source *SourceEpisodes, season, number int) TitleRef {
		return TitleRef{Episode: true, IMDb: imdb, Season: season, Number: number, Source: source}
	}
	cases := []struct {
		name string
		ref  TitleRef
		// want is the addon's season and number, none when refused.
		want []int
	}{
		{"Lupin by ids, S1E8", ref("tt2531336", lupinIDs, 1, 8), []int{2, 3}},
		{"Lupin by ids, S2E1", ref("tt2531336", lupinIDs, 2, 1), []int{3, 1}},
		{"Lupin by days, S1E8", ref("tt2531337", lupinDays, 1, 8), []int{2, 3}},
		{"Lupin by days, S2E1", ref("tt2531337", lupinDays, 2, 1), []int{3, 1}},
		{"Lupin, the file S1E8 alone", ref("tt2531338", heldOnly(lupinDays, [2]int{1, 8}), 1, 8), nil},
		{"split season, S2E3", ref("tt0001200", split, 2, 3), []int{1, 13}},
		{"split season, S3E1", ref("tt0001200", split, 3, 1), []int{2, 1}},
		{"Survivor S2E10", ref("tt0239195", survivor, 2, 10), []int{2, 9}},
		{"Survivor's S2E8, which the addon leaves out", ref("tt0239195", survivor, 2, 8), nil},
		{"DVD order S1E2", ref("tt0000600", dvdSource, 1, 2), []int{1, 3}},
		{"DVD order S1E3", ref("tt0000600", dvdSource, 1, 3), []int{1, 2}},
		{"DVD order, an episode undated", ref("tt0000600", dvdSource, 1, 5), nil},
		{"numbered alike S1E4", ref("tt0000400", alike, 1, 4), []int{1, 4}},
		{"numbered alike, the file S1E4 alone", ref("tt0000400", heldOnly(alike, [2]int{1, 4}), 1, 4), []int{1, 4}},
		{"an identifier against the day", ref("tt0000700", byIDSource, 1, 4), []int{1, 5}},
		{"identifiers naming two episodes", ref("tt0000730", conflictSource, 1, 2), nil},
		{"some files of a season released in a day", ref("tt0000800", heldOnly(serverSource(together, false), [2]int{1, 2}), 1, 2),
			[]int{1, 2}},
		{"no identifier, day or name", ref("tt0000900", bare, 1, 3), []int{1, 3}},
		{"no identifier, day or name, an episode the addon lacks", ref("tt0000900", bare, 1, 9), nil},
		{"a series the addon lists nothing of", ref("tt0000990", bare, 1, 3), []int{1, 3}},
	}
	refs := make([]TitleRef, len(cases))
	for i, c := range cases {
		refs[i] = c.ref
	}
	got, unplaced, err := e.service.ResolveFromSource(t.Context(), refs)
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range cases {
		var want []TitleTarget
		if c.want != nil {
			videoID := fmt.Sprintf("%s:%d:%d", c.ref.IMDb, c.want[0], c.want[1])
			want = []TitleTarget{{ID: itemID(episodeKey(videoID)), Series: itemID(titleKey(KindSeries, c.ref.IMDb)),
				Season: itemID(seasonKey(c.ref.IMDb, c.want[0])), SeasonNumber: c.want[0], Number: c.want[1]}}
		}
		if !slices.Equal(got[i], want) || unplaced[i] != (c.want == nil) {
			t.Errorf("%s: %+v unplaced %v, want %+v", c.name, got[i], unplaced[i], want)
		}
	}
	// No episode the addon does not list was recorded: the split season
	// keeps the 28 Polyfin listed, and Lupin's S1E8 none.
	if n := countEpisodes(t, e, "tt0001200"); n != 28 {
		t.Errorf("the split season has %d episodes, want 28", n)
	}
	if n := countEpisodes(t, e, "tt2531338"); n != 0 {
		t.Errorf("Lupin's lone S1E8 recorded %d episodes", n)
	}
	// Each series' description was asked once.
	if asked := metaAsked(described, "series", "tt2531336"); asked != 1 {
		t.Errorf("Lupin's description asked %d times", asked)
	}
}

// Episodes Polyfin listed keep the identifiers the addon gave them, which
// place a server's episodes when the addon describes the series no more.
func TestListedEpisodesKeepTheirOwnIdentifiers(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	videos := describedVideos("tt2531336", lupin(), true)
	records := []record{unlistedTitle(KindSeries, "tt2531336", TitleRef{Series: true, IMDb: "tt2531336"})}
	for _, video := range videos {
		records = append(records, record{ID: itemID(episodeKey(video.ID)), Key: episodeKey(video.ID), Kind: KindEpisode,
			SeriesID: "tt2531336", Season: int(video.Season), Video: &video})
	}
	if err := e.service.save(t.Context(), records); err != nil {
		t.Fatal(err)
	}
	// Undated, only the identifiers place it.
	source := serverSource(lupin(), true)
	for i := range source.Episodes {
		source.Episodes[i].Aired = time.Time{}
	}
	got, unplaced, err := e.service.ResolveFromSource(t.Context(),
		[]TitleRef{{Episode: true, IMDb: "tt2531336", Season: 1, Number: 8, Source: source}})
	if err != nil || unplaced[0] || len(got[0]) != 1 || got[0][0].SeasonNumber != 2 || got[0][0].Number != 3 {
		t.Errorf("Lupin's S1E8 by its listed identifier: %+v %v %v", got, unplaced, err)
	}
}
