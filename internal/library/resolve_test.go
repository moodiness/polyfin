package library

import (
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

func TestResolveFindsTitlesByAnyOfTheirIdentifiers(t *testing.T) {
	e := newEnv(t)
	// A catalog listed a movie by its TMDB identifier, giving its IMDb one
	// too; another listed a series by its TVDB identifier, with two of its
	// episodes; a third listed a series by an identifier of its own, with
	// an episode named its own way.
	movie := stremio.Meta{ID: "tmdb:550", Type: "movie", Name: "Listed by TMDB", ImdbID: "tt0137523", Runtime: "2h 19min"}
	tvdbShow := stremio.Meta{ID: "tvdb:81189", Type: "series", Name: "Listed by TVDB", Runtime: "47min"}
	ownShow := stremio.Meta{ID: "kitsu:1", Type: "series", Name: "Own identifiers", TmdbID: "30"}
	records := []record{
		{ID: itemID(titleKey(KindMovie, movie.ID)), Key: titleKey(KindMovie, movie.ID), Kind: KindMovie, Meta: &movie},
		{ID: itemID(titleKey(KindSeries, tvdbShow.ID)), Key: titleKey(KindSeries, tvdbShow.ID), Kind: KindSeries, Meta: &tvdbShow},
		{ID: itemID(titleKey(KindSeries, ownShow.ID)), Key: titleKey(KindSeries, ownShow.ID), Kind: KindSeries, Meta: &ownShow},
	}
	// The TVDB-listed series' first season has two episodes, its second
	// one; a special is listed too.
	for _, video := range []stremio.Video{{ID: "tvdb:81189:1:1", Season: 1, Episode: 1, Runtime: "58min"}, {ID: "tvdb:81189:1:2", Season: 1, Episode: 2},
		{ID: "tvdb:81189:2:1", Season: 2, Episode: 1}, {ID: "tvdb:81189:0:1", Season: 0, Episode: 1}} {
		records = append(records, record{ID: itemID(episodeKey(video.ID)), Key: episodeKey(video.ID), Kind: KindEpisode,
			SeriesID: tvdbShow.ID, Season: int(video.Season), Video: &video})
	}
	odd := stremio.Video{ID: "kitsu:1-ep-7", Season: 1, Episode: 7}
	records = append(records, record{ID: itemID(episodeKey(odd.ID)), Key: episodeKey(odd.ID), Kind: KindEpisode, SeriesID: ownShow.ID, Season: 1, Video: &odd})
	if err := e.service.save(t.Context(), records); err != nil {
		t.Fatal(err)
	}

	refs := []TitleRef{
		{IMDb: "tt0137523"},            // the TMDB-listed movie, by IMDb
		{TMDB: 550},                    // the same, by TMDB
		{IMDb: "tt0111161", TMDB: 278}, // a movie never listed: by IMDb
		{TMDB: 278},                    // never listed, TMDB only: not found
		{Episode: true, TVDB: 81189, Season: 1, Number: 1}, // a listed episode
		{Episode: true, TVDB: 81189, Season: 3, Number: 4}, // named as the listed ones are
		{Episode: true, IMDb: "tt0903747", Season: 2, Number: 5},
		{Episode: true, TMDB: 30, Season: 1, Number: 7}, // listed under its own name
		{Episode: true, TMDB: 30, Season: 1, Number: 8}, // not listed, named an unknown way
		{TVDB: 81189},                     // movies are not looked up by TVDB
		{Series: true, TVDB: 81189},       // a listed series, by TVDB
		{Series: true, IMDb: "tt0903747"}, // a series never listed: by IMDb
		{Series: true, TMDB: 278},         // never listed, TMDB only: not found
		// Numbered absolutely: the third regular episode listed, the
		// second season's first; a fourth is not listed; nor is a series
		// only IMDb names.
		{Episode: true, TVDB: 81189, Number: 3, Absolute: true},
		{Episode: true, TVDB: 81189, Number: 4, Absolute: true},
		{Episode: true, IMDb: "tt0903747", Number: 1, Absolute: true},
	}
	got, err := e.service.Resolve(t.Context(), refs)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]TitleTarget{
		{{ID: itemID("movie|tmdb:550"), Runtime: 139 * time.Minute}},
		{{ID: itemID("movie|tmdb:550"), Runtime: 139 * time.Minute}},
		{{ID: itemID("movie|tt0111161")}},
		nil,
		{{ID: itemID("episode|tvdb:81189:1:1"), Series: itemID("series|tvdb:81189"), Season: itemID("season|tvdb:81189|1"), SeasonNumber: 1, Number: 1,
			Runtime: 58 * time.Minute}},
		{{ID: itemID("episode|tvdb:81189:3:4"), Series: itemID("series|tvdb:81189"), Season: itemID("season|tvdb:81189|3"), SeasonNumber: 3, Number: 4,
			Runtime: 47 * time.Minute}},
		{{ID: itemID("episode|tt0903747:2:5"), Series: itemID("series|tt0903747"), Season: itemID("season|tt0903747|2"), SeasonNumber: 2, Number: 5}},
		{{ID: itemID("episode|kitsu:1-ep-7"), Series: itemID("series|kitsu:1"), Season: itemID("season|kitsu:1|1"), SeasonNumber: 1, Number: 7}},
		nil,
		nil,
		{{ID: itemID("series|tvdb:81189"), Runtime: 47 * time.Minute}},
		{{ID: itemID("series|tt0903747")}},
		nil,
		{{ID: itemID("episode|tvdb:81189:2:1"), Series: itemID("series|tvdb:81189"), Season: itemID("season|tvdb:81189|2"), SeasonNumber: 2, Number: 1,
			Runtime: 47 * time.Minute}},
		nil,
		nil,
	}
	if len(got) != len(want) {
		t.Fatalf("%d results", len(got))
	}
	for i := range want {
		if len(got[i]) != len(want[i]) || len(want[i]) > 0 && got[i][0] != want[i][0] {
			t.Errorf("%+v: %+v, want %+v", refs[i], got[i], want[i])
		}
	}
}

func TestResolveFindsWhatAnimeCatalogsListed(t *testing.T) {
	e := newEnv(t)
	// An anime catalog listed an entry of a series under its Kitsu
	// identifier, giving the series' IMDb one, with two of its episodes,
	// and a movie under its MyAnimeList identifier.
	entry := stremio.Meta{ID: "kitsu:12", Type: "series", Name: "Second", ImdbID: "tt0000100"}
	movie := stremio.Meta{ID: "mal:5", Type: "movie", Name: "The movie"}
	records := []record{
		{ID: itemID(titleKey(KindSeries, entry.ID)), Key: titleKey(KindSeries, entry.ID), Kind: KindSeries, Meta: &entry},
		{ID: itemID(titleKey(KindMovie, movie.ID)), Key: titleKey(KindMovie, movie.ID), Kind: KindMovie, Meta: &movie},
	}
	for _, video := range []stremio.Video{{ID: "kitsu:12:1", Season: 1, Episode: 1}, {ID: "kitsu:12:3", Season: 1, Episode: 3, Runtime: "24min"}} {
		records = append(records, record{ID: itemID(episodeKey(video.ID)), Key: episodeKey(video.ID), Kind: KindEpisode,
			SeriesID: entry.ID, Season: 1, Video: &video})
	}
	if err := e.service.save(t.Context(), records); err != nil {
		t.Fatal(err)
	}

	refs := []TitleRef{
		// The entry's third episode, the series' S2E3: both.
		{Episode: true, IMDb: "tt0000100", Season: 2, Number: 3, Anime: AnimeRef{Kitsu: 12, Episode: 3}},
		// An episode the anime catalog did not list: nothing recorded.
		{Episode: true, Anime: AnimeRef{Kitsu: 12, Episode: 2}},
		// Another entry's first episode, the series' S1E1: not the listed
		// entry's first.
		{Episode: true, IMDb: "tt0000100", Season: 1, Number: 1, Anime: AnimeRef{Kitsu: 11, Episode: 1}},
		{Anime: AnimeRef{MyAnimeList: 5}},
	}
	got, err := e.service.Resolve(t.Context(), refs)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]TitleTarget{
		{{ID: itemID("episode|tt0000100:2:3"), Series: itemID("series|tt0000100"), Season: itemID("season|tt0000100|2"), SeasonNumber: 2, Number: 3},
			{ID: itemID("episode|kitsu:12:3"), Series: itemID("series|kitsu:12"), Season: itemID("season|kitsu:12|1"), SeasonNumber: 1, Number: 3,
				Runtime: 24 * time.Minute, Anime: true}},
		nil,
		{{ID: itemID("episode|tt0000100:1:1"), Series: itemID("series|tt0000100"), Season: itemID("season|tt0000100|1"), SeasonNumber: 1, Number: 1}},
		{{ID: itemID("movie|mal:5"), Anime: true}},
	}
	for i := range want {
		if !slices.Equal(got[i], want[i]) {
			t.Errorf("%+v: %+v, want %+v", refs[i], got[i], want[i])
		}
	}
	if _, err := e.service.load(t.Context(), itemID("episode|kitsu:12:2")); err == nil {
		t.Error("an episode no anime catalog listed was recorded")
	}
}

func TestResolveRecordsTheTitlesNoCatalogListed(t *testing.T) {
	e := newEnv(t)
	// An addon describes a movie and a series no catalog listed.
	e.install(addons.Shared(), &fakeAddon{
		manifest: stremio.Manifest{ID: "described", Name: "Described", Version: "1", Types: []string{"movie", "series"},
			Resources: []stremio.Resource{{Name: "meta"}}},
		metas: map[string]stremio.Meta{
			"movie/tt1727587": {ID: "tt1727587", Type: "movie", Name: "A short movie", Description: "Described by the addon"},
			"series/tt0944947": {ID: "tt0944947", Type: "series", Name: "A long series",
				Videos: []stremio.Video{{ID: "tt0944947:1:2", Name: "The second one", Season: 1, Episode: 2}}},
		},
	})
	// A catalog listed another series by its TVDB identifier, with its
	// first season, which has artwork of its own, and first episode.
	listed := stremio.Meta{ID: "tvdb:81189", Type: "series", Name: "Listed by TVDB"}
	pilot := stremio.Video{ID: "tvdb:81189:1:1", Season: 1, Episode: 1}
	seriesItem, seasonItem := itemID(titleKey(KindSeries, listed.ID)), itemID(seasonKey(listed.ID, 1))
	if err := e.service.save(t.Context(), []record{
		{ID: seriesItem, Key: titleKey(KindSeries, listed.ID), Kind: KindSeries, Meta: &listed},
		{ID: seasonItem, Key: seasonKey(listed.ID, 1), Kind: KindSeason, Parent: &seriesItem, SeriesID: listed.ID, Season: 1, Poster: "season-one.jpg"},
		{ID: itemID(episodeKey(pilot.ID)), Key: episodeKey(pilot.ID), Kind: KindEpisode, Parent: &seasonItem, SeriesID: listed.ID, Season: 1, Video: &pilot},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := e.service.Resolve(t.Context(), []TitleRef{
		{IMDb: "tt1727587", Name: "Its name elsewhere"},
		{Episode: true, IMDb: "tt0944947", Season: 1, Number: 2, Name: "A long series"},
		{Episode: true, TVDB: 81189, Season: 1, Number: 3, Name: "Listed by TVDB"},
	})
	if err != nil || len(got[0]) != 1 || len(got[1]) != 1 || len(got[2]) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	movie, err := e.service.Item(t.Context(), e.member, got[0][0].ID)
	if err != nil || movie.Name != "A short movie" || movie.Overview != "Described by the addon" {
		t.Errorf("unlisted movie: %+v %v", movie, err)
	}
	episode, err := e.service.Item(t.Context(), e.member, got[1][0].ID)
	if err != nil || episode.Name != "The second one" || episode.SeriesName != "A long series" || episode.IndexNumber != 2 ||
		episode.SeriesID != got[1][0].Series {
		t.Errorf("episode of an unlisted series: %+v %v", episode, err)
	}
	items, err := e.service.Items(t.Context(), e.member, []accounts.ID{got[0][0].ID, got[1][0].ID})
	if err != nil || len(items) != 2 {
		t.Errorf("listed together: %v %v", names(items), err)
	}
	// The listed season keeps its artwork.
	if season, err := e.service.load(t.Context(), seasonItem); err != nil || season.Poster != "season-one.jpg" {
		t.Errorf("listed season: %+v %v", season, err)
	}
}
