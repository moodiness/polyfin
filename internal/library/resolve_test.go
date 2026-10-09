package library

import (
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
	for _, video := range []stremio.Video{{ID: "tvdb:81189:1:1", Season: 1, Episode: 1, Runtime: "58min"}, {ID: "tvdb:81189:1:2", Season: 1, Episode: 2}} {
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
		{{ID: itemID("episode|tvdb:81189:1:1"), Series: itemID("series|tvdb:81189"), Season: itemID("season|tvdb:81189|1"), Runtime: 58 * time.Minute}},
		{{ID: itemID("episode|tvdb:81189:3:4"), Series: itemID("series|tvdb:81189"), Season: itemID("season|tvdb:81189|3"), Runtime: 47 * time.Minute}},
		{{ID: itemID("episode|tt0903747:2:5"), Series: itemID("series|tt0903747"), Season: itemID("season|tt0903747|2")}},
		{{ID: itemID("episode|kitsu:1-ep-7"), Series: itemID("series|kitsu:1"), Season: itemID("season|kitsu:1|1")}},
		nil,
		nil,
		{{ID: itemID("series|tvdb:81189"), Runtime: 47 * time.Minute}},
		{{ID: itemID("series|tt0903747")}},
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
