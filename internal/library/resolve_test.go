package library

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

func TestResolveFindsTitlesByAnyOfTheirIdentifiers(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// metaAsked counts the requests addon received for the description of
// the title of type metaType and Stremio identifier id.
func metaAsked(addon *fakeAddon, metaType, id string) int {
	return requestsTo(addon, "meta/"+metaType+"/"+strings.ReplaceAll(id, ":", "%3A"))
}

func TestResolveAsksTheAddonsForTitlesKnownByTMDBOnly(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	// An addon describes titles by their TMDB identifier, with the other
	// identifiers it knows of them.
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "tmdb", Name: "By TMDB", Version: "1", Types: []string{"movie", "series"},
			Resources: []stremio.Resource{{Name: "meta"}}, IDPrefixes: []string{"tt", "tmdb:"}},
		metas: map[string]stremio.Meta{
			"movie/tmdb:1685070": {ID: "tmdb:1685070", Type: "movie", Name: "Described by TMDB", ImdbID: "tt1727587", Runtime: "1h 30min"},
			"movie/tmdb:1483614": {ID: "tmdb:1483614", Type: "movie", Name: "Described without IMDb"},
			"series/tmdb:2000": {ID: "tmdb:2000", Type: "series", Name: "A TMDB series",
				Videos: []stremio.Video{{ID: "tmdb:2000:1:3", Name: "The third one", Season: 1, Episode: 3, Runtime: "25min"}}},
			"series/tmdb:3000": {ID: "tmdb:3000", Type: "series", Name: "Known by TVDB", TvdbID: "81189"},
			"series/tmdb:4000": {ID: "tmdb:4000", Type: "series", Name: "Numbered absolutely"},
			"movie/tmdb:550":   {ID: "tmdb:550", Type: "movie", Name: "Listed"},
			"movie/tmdb:9":     {ID: "tmdb:9", Type: "movie", Name: "Known by IMDb"},
		},
	}
	e.install(addons.Shared(), addon)
	// A catalog listed a movie by its TMDB identifier, and a series by its
	// TVDB identifier, with its first episode.
	movie := stremio.Meta{ID: "tmdb:550", Type: "movie", Name: "Listed"}
	listed := stremio.Meta{ID: "tvdb:81189", Type: "series", Name: "Listed by TVDB"}
	pilot := stremio.Video{ID: "tvdb:81189:1:1", Season: 1, Episode: 1}
	seriesItem, seasonItem := itemID(titleKey(KindSeries, listed.ID)), itemID(seasonKey(listed.ID, 1))
	if err := e.service.save(t.Context(), []record{
		{ID: itemID(titleKey(KindMovie, movie.ID)), Key: titleKey(KindMovie, movie.ID), Kind: KindMovie, Meta: &movie},
		{ID: seriesItem, Key: titleKey(KindSeries, listed.ID), Kind: KindSeries, Meta: &listed},
		{ID: seasonItem, Key: seasonKey(listed.ID, 1), Kind: KindSeason, Parent: &seriesItem, SeriesID: listed.ID, Season: 1},
		{ID: itemID(episodeKey(pilot.ID)), Key: episodeKey(pilot.ID), Kind: KindEpisode, Parent: &seasonItem, SeriesID: listed.ID, Season: 1, Video: &pilot},
	}); err != nil {
		t.Fatal(err)
	}

	refs := []TitleRef{
		{TMDB: 1685070, Name: "Its name elsewhere"}, // described with an IMDb identifier
		{TMDB: 1483614}, // described without one
		{Episode: true, TMDB: 2000, Season: 1, Number: 3}, // placed with the description's videos
		{Series: true, TMDB: 3000},                        // the series listed by the TVDB identifier described
		{Episode: true, TMDB: 3000, Season: 1, Number: 1}, // and its listed episode
		{TMDB: 550}, // listed: not asked
		{IMDb: "tt0000009", TMDB: 9, Name: "Known by IMDb"}, // has an IMDb identifier: not asked
		{TMDB: 1685070}, // asked once
		{Episode: true, TMDB: 4000, Number: 5, Absolute: true}, // placed among those listed only
	}
	got, err := e.service.Resolve(t.Context(), refs)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]TitleTarget{
		{{ID: itemID("movie|tt1727587"), Runtime: 90 * time.Minute}},
		{{ID: itemID("movie|tmdb:1483614")}},
		{{ID: itemID("episode|tmdb:2000:1:3"), Series: itemID("series|tmdb:2000"), Season: itemID("season|tmdb:2000|1"), SeasonNumber: 1, Number: 3,
			Runtime: 25 * time.Minute}},
		{{ID: seriesItem}},
		{{ID: itemID(episodeKey(pilot.ID)), Series: seriesItem, Season: seasonItem, SeasonNumber: 1, Number: 1}},
		{{ID: itemID("movie|tmdb:550")}},
		{{ID: itemID("movie|tt0000009")}},
		{{ID: itemID("movie|tt1727587"), Runtime: 90 * time.Minute}},
		nil,
	}
	for i := range want {
		if len(got[i]) != len(want[i]) || len(want[i]) > 0 && got[i][0] != want[i][0] {
			t.Errorf("%+v: %+v, want %+v", refs[i], got[i], want[i])
		}
	}
	asked := map[string]int{"movie/tmdb:1685070": 1, "movie/tmdb:1483614": 1, "series/tmdb:2000": 1, "series/tmdb:3000": 1, "series/tmdb:4000": 1}
	for title, want := range asked {
		metaType, id, _ := strings.Cut(title, "/")
		if got := metaAsked(addon, metaType, id); got != want {
			t.Errorf("%s asked %d times, want %d", title, got, want)
		}
	}
	if got := requestsTo(addon, "meta/"); got != len(asked) {
		t.Errorf("%d meta requests, want %d", got, len(asked))
	}

	// The titles are recorded under the identifier found, with the
	// description's name.
	for id, want := range map[accounts.ID]stremio.Meta{
		itemID("movie|tt1727587"):    {ID: "tt1727587", Type: "movie", Name: "Described by TMDB", ImdbID: "tt1727587", TmdbID: "1685070"},
		itemID("movie|tmdb:1483614"): {ID: "tmdb:1483614", Type: "movie", Name: "Described without IMDb", TmdbID: "1483614"},
		itemID("series|tmdb:2000"):   {ID: "tmdb:2000", Type: "series", Name: "A TMDB series", TmdbID: "2000"},
	} {
		r, err := e.service.load(t.Context(), id)
		if err != nil || r.Meta == nil || r.Meta.ID != want.ID || r.Meta.Type != want.Type || r.Meta.Name != want.Name ||
			r.Meta.ImdbID != want.ImdbID || r.Meta.TmdbID != want.TmdbID {
			t.Errorf("record of %s: %+v %v", want.ID, r.Meta, err)
		}
	}
	if r, err := e.service.load(t.Context(), itemID("episode|tmdb:2000:1:3")); err != nil || r.Kind != KindEpisode || r.SeriesID != "tmdb:2000" ||
		r.Video == nil || r.Video.ID != "tmdb:2000:1:3" {
		t.Errorf("episode recorded: %+v %v", r, err)
	}

	// Resolved again, the titles recorded are found, and the description
	// of the one still unfound is kept, after a restart too.
	for _, e := range []env{e, e.restarted()} {
		again, err := e.service.Resolve(t.Context(), refs)
		if err != nil {
			t.Fatal(err)
		}
		for i := range want {
			if len(again[i]) != len(want[i]) || len(want[i]) > 0 && again[i][0].ID != want[i][0].ID {
				t.Errorf("again, %+v: %+v, want %+v", refs[i], again[i], want[i])
			}
		}
		if got := requestsTo(addon, "meta/"); got != len(asked) {
			t.Errorf("again: %d meta requests, want %d", got, len(asked))
		}
	}
}

func TestResolveAsksTheAddonsForSeriesKnownByTVDBOnly(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	// An addon describes titles by their TVDB identifier, with the other
	// identifiers it knows of them; another describes none by it.
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "tvdb", Name: "By TVDB", Version: "1", Types: []string{"movie", "series"},
			Resources: []stremio.Resource{{Name: "meta"}}, IDPrefixes: []string{"tt", "tvdb:"}},
		metas: map[string]stremio.Meta{
			"series/tvdb:5001": {ID: "tvdb:5001", Type: "series", Name: "Described with IMDb", ImdbID: "tt5000001"},
			"series/tvdb:5002": {ID: "tvdb:5002", Type: "series", Name: "Described with TMDB", TmdbID: "6002"},
			"series/tvdb:5003": {ID: "tvdb:5003", Type: "series", Name: "Described by TVDB only",
				Videos: []stremio.Video{{ID: "tvdb:5003:2:4", Name: "The fourth one", Season: 2, Episode: 4}}},
			"movie/tvdb:5004": {ID: "tvdb:5004", Type: "movie", Name: "Not a series"},
		},
	}
	tmdbOnly := &fakeAddon{
		manifest: stremio.Manifest{ID: "tmdb", Name: "By TMDB", Version: "1", Types: []string{"movie", "series"},
			Resources: []stremio.Resource{{Name: "meta"}}, IDPrefixes: []string{"tmdb:"}},
		metas: map[string]stremio.Meta{"series/tvdb:5003": {ID: "tvdb:5003", Type: "series", Name: "Not asked", ImdbID: "tt5000003"}},
	}
	e.install(addons.Shared(), addon)
	e.install(addons.Shared(), tmdbOnly)
	// A catalog listed a series under an identifier of its own, giving its
	// TMDB one.
	listed := stremio.Meta{ID: "catalog:6002", Type: "series", Name: "Listed with TMDB", TmdbID: "6002"}
	if err := e.service.save(t.Context(), []record{
		{ID: itemID(titleKey(KindSeries, listed.ID)), Key: titleKey(KindSeries, listed.ID), Kind: KindSeries, Meta: &listed},
	}); err != nil {
		t.Fatal(err)
	}

	refs := []TitleRef{
		{Series: true, TVDB: 5001},                             // described with an IMDb identifier
		{Episode: true, TVDB: 5001, Season: 1, Number: 2},      // named as the IMDb identifier names it
		{Series: true, TVDB: 5002},                             // the series listed with the TMDB identifier described
		{Series: true, TVDB: 5003, Name: "Its name elsewhere"}, // described without another identifier
		{Episode: true, TVDB: 5003, Season: 2, Number: 4},      // placed with the description's videos
		{TVDB: 5004}, // movies are not looked up by TVDB
	}
	got, err := e.service.Resolve(t.Context(), refs)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]TitleTarget{
		{{ID: itemID("series|tt5000001")}},
		{{ID: itemID("episode|tt5000001:1:2"), Series: itemID("series|tt5000001"), Season: itemID("season|tt5000001|1"), SeasonNumber: 1, Number: 2}},
		{{ID: itemID("series|catalog:6002")}},
		{{ID: itemID("series|tvdb:5003")}},
		{{ID: itemID("episode|tvdb:5003:2:4"), Series: itemID("series|tvdb:5003"), Season: itemID("season|tvdb:5003|2"), SeasonNumber: 2, Number: 4}},
		nil,
	}
	for i := range want {
		if len(got[i]) != len(want[i]) || len(want[i]) > 0 && got[i][0] != want[i][0] {
			t.Errorf("%+v: %+v, want %+v", refs[i], got[i], want[i])
		}
	}
	for _, id := range []string{"tvdb:5001", "tvdb:5002", "tvdb:5003"} {
		if got := metaAsked(addon, "series", id); got != 1 {
			t.Errorf("%s asked %d times, want 1", id, got)
		}
	}
	if got := requestsTo(addon, "meta/"); got != 3 {
		t.Errorf("%d meta requests, want 3", got)
	}
	if got := requestsTo(tmdbOnly, "meta/"); got != 0 {
		t.Errorf("the addon serving TMDB identifiers only was asked %d times", got)
	}

	// The series no other identifier names is recorded under its TVDB one,
	// with the description's name, and its episode with it.
	if r, err := e.service.load(t.Context(), itemID("series|tvdb:5003")); err != nil || r.Meta == nil || r.Meta.ID != "tvdb:5003" ||
		r.Meta.Name != "Described by TVDB only" || r.Meta.TvdbID != "5003" || r.Meta.ImdbID != "" {
		t.Errorf("record of tvdb:5003: %+v %v", r.Meta, err)
	}
	if r, err := e.service.load(t.Context(), itemID("episode|tvdb:5003:2:4")); err != nil || r.SeriesID != "tvdb:5003" || r.Video == nil ||
		r.Video.ID != "tvdb:5003:2:4" {
		t.Errorf("episode recorded: %+v %v", r, err)
	}
	if r, err := e.service.load(t.Context(), itemID("series|tt5000001")); err != nil || r.Meta == nil || r.Meta.Name != "Described with IMDb" ||
		r.Meta.TvdbID != "5001" {
		t.Errorf("record of tt5000001: %+v %v", r.Meta, err)
	}
}

func TestResolveLeavesUnfoundWhatNoAddonDescribes(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	// One addon fails; the other describes no title by its TMDB
	// identifier, whatever it would answer.
	failing := &fakeAddon{
		manifest: stremio.Manifest{ID: "failing", Name: "Failing", Version: "1", Types: []string{"movie", "series"},
			Resources: []stremio.Resource{{Name: "meta"}}, IDPrefixes: []string{"tmdb:"}},
		metaFails: true,
	}
	imdbOnly := &fakeAddon{
		manifest: stremio.Manifest{ID: "imdb", Name: "By IMDb", Version: "1", Types: []string{"movie", "series"},
			Resources: []stremio.Resource{{Name: "meta"}}, IDPrefixes: []string{"tt"}},
		metas: map[string]stremio.Meta{"movie/tmdb:1483614": {ID: "tmdb:1483614", Type: "movie", Name: "Not asked", ImdbID: "tt1727587"}},
	}
	e.install(addons.Shared(), failing)
	e.install(addons.Shared(), imdbOnly)

	got, err := e.service.Resolve(t.Context(), []TitleRef{{TMDB: 1483614}, {Episode: true, TMDB: 2000, Season: 1, Number: 3}})
	if err != nil || len(got) != 2 || got[0] != nil || got[1] != nil {
		t.Fatalf("%+v %v", got, err)
	}
	if metaAsked(failing, "movie", "tmdb:1483614") == 0 || metaAsked(failing, "series", "tmdb:2000") == 0 {
		t.Error("the failing addon was not asked")
	}
	if got := requestsTo(imdbOnly, "meta/"); got != 0 {
		t.Errorf("the addon serving IMDb identifiers only was asked %d times", got)
	}
	if _, err := e.service.load(t.Context(), itemID("movie|tmdb:1483614")); err == nil {
		t.Error("an undescribed title was recorded")
	}
}
