package library

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// numberedEnv serves a series whose metadata addon names it
// "tvdb:<id>" and numbers its episodes as a fixture's addon side does,
// with its IMDb and TMDB identifiers. When references is set, a server
// addon describing series by their IMDb identifiers, and one describing
// them by their TMDB identifiers, number them as the fixture's other side
// does. Two stream addons, one accepting IMDb's identifiers only and one
// TMDB's only, list a stream for each episode as the other side numbers
// it, named after the numbers.
type numberedEnv struct {
	env
	series, imdb string
	tmdb         int
	// imdbMeta is the server addon describing series by IMDb's
	// identifiers, nil without references.
	imdbMeta *fakeAddon
	// filled receives the series of each pin's background work ended.
	filled chan string
}

func newNumberedEnv(t *testing.T, fixture []airedFixture, tvdb, tmdbID int, imdb string, references bool) numberedEnv {
	t.Helper()
	e := numberedEnv{env: newEnv(t), series: "tvdb:" + strconv.Itoa(tvdb), imdb: imdb, tmdb: tmdbID, filled: make(chan string, 8)}
	e.service.streamNumberings.filled = func(series string) { e.filled <- series }
	show := stremio.Meta{ID: e.series, Type: "series", Name: "Series " + imdb, ImdbID: imdb, TmdbID: stremio.Text(strconv.Itoa(tmdbID))}
	byIMDb := stremio.Meta{ID: imdb, Type: "series", Name: show.Name, ImdbID: imdb}
	tmdbSeries := "tmdb:" + strconv.Itoa(tmdbID)
	byTMDB := stremio.Meta{ID: tmdbSeries, Type: "series", Name: show.Name}
	imdbStreams, tmdbStreams := map[string][]stremio.Stream{}, map[string][]stremio.Stream{}
	stream := func(name, key string) []stremio.Stream {
		return []stremio.Stream{{Name: name, URL: "https://cdn.example/" + key,
			BehaviorHints: stremio.StreamBehavior{Filename: key + ".mkv", VideoSize: 1_000_000}}}
	}
	for _, f := range fixture {
		if f.addonNumber > 0 {
			show.Videos = append(show.Videos, stremio.Video{ID: fmt.Sprintf("%s:%d:%d", e.series, f.addonSeason, f.addonNumber),
				Season: stremio.Number(f.addonSeason), Episode: stremio.Number(f.addonNumber), Name: cmpOrString(f.addonName, f.name),
				Released: addonDate(f.day)})
		}
		if f.number == 0 {
			continue
		}
		numbers := fmt.Sprintf("S%dE%d", f.season, f.number)
		suffix := episodeSuffix(f.season, f.number)
		byIMDb.Videos = append(byIMDb.Videos, stremio.Video{ID: imdb + suffix, Season: stremio.Number(f.season),
			Episode: stremio.Number(f.number), Name: f.name, Released: f.day + "T00:00:00.000Z"})
		byTMDB.Videos = append(byTMDB.Videos, stremio.Video{ID: tmdbSeries + suffix, Season: stremio.Number(f.season),
			Episode: stremio.Number(f.number), Name: f.name, Released: f.day + "T00:00:00.000Z"})
		imdbStreams["series/"+imdb+suffix] = stream("IMDb "+numbers, "imdb"+suffix)
		tmdbStreams["series/"+tmdbSeries+suffix] = stream("TMDB "+numbers, "tmdb"+suffix)
	}
	// A video without an episode number, such as an extra, pairs with
	// none.
	byIMDb.Videos = append(byIMDb.Videos, stremio.Video{ID: imdb + ":extra", Season: 1, Name: "Behind the scenes"})
	e.install(addons.Shared(), catalogOf("series", "tvdb:", show))
	if references {
		e.imdbMeta = &fakeAddon{
			manifest: stremio.Manifest{ID: "imdb-meta", Name: "By IMDb", Version: "1", Types: []string{"series"},
				Resources: []stremio.Resource{{Name: "meta"}}, IDPrefixes: []string{"tt"}},
			metas: map[string]stremio.Meta{"series/" + imdb: byIMDb},
		}
		e.install(addons.Shared(), e.imdbMeta)
		e.install(addons.Shared(), &fakeAddon{
			manifest: stremio.Manifest{ID: "tmdb-meta", Name: "By TMDB", Version: "1", Types: []string{"series"},
				Resources: []stremio.Resource{{Name: "meta"}}, IDPrefixes: []string{"tmdb:"}},
			metas: map[string]stremio.Meta{"series/" + tmdbSeries: byTMDB},
		})
	}
	for id, streams := range map[string]map[string][]stremio.Stream{"imdb": imdbStreams, "tmdb": tmdbStreams} {
		prefix := map[string]string{"imdb": "tt", "tmdb": "tmdb:"}[id]
		e.install(addons.Shared(), &fakeAddon{
			manifest: stremio.Manifest{ID: id, Name: id, Version: "1", Types: []string{"series"},
				Resources: []stremio.Resource{{Name: "stream", IDPrefixes: []string{prefix}}}},
			streams: streams,
		})
	}
	// The series' episodes are listed, as a season opened lists them.
	ctx := t.Context()
	page, err := e.service.Children(ctx, e.member, e.library(e.member, "Top").ID, 0, 10, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("listing: %+v %v", page, err)
	}
	if _, err := e.service.Episodes(ctx, e.member, page.Items[0].ID, nil); err != nil {
		t.Fatal(err)
	}
	return e
}

// prime keeps what tracking and descriptions keep: the series' alignment
// on TMDB's numbering, and the description of its IMDb identifier.
func (e numberedEnv) prime() {
	e.t.Helper()
	if _, err := e.service.TMDBNumbering(e.t.Context(), itemID(titleKey(KindSeries, e.series))); err != nil {
		e.t.Fatal(err)
	}
	if _, ok := e.service.TitleMeta(e.t.Context(), "series", e.imdb); !ok {
		e.t.Fatal("the IMDb identifier's description could not be kept")
	}
}

// askedAs returns the identifiers the stream addons are asked for the
// episode the metadata addon numbers season and number by, by addon.
func (e numberedEnv) askedAs(season, number int) map[string]string {
	e.t.Helper()
	id := itemID(episodeKey(e.series + episodeSuffix(season, number)))
	t, v, err := e.service.target(e.t.Context(), e.member, id)
	if err != nil {
		e.t.Fatal(err)
	}
	asked := map[string]string{}
	for _, source := range e.service.streamServing(e.t.Context(), v, t) {
		asked[source.addon.Manifest.ID] = source.id
	}
	return asked
}

// wantAsked is what the stream addons are asked for the episode the
// other side numbers season and number.
func (e numberedEnv) wantAsked(season, number int) map[string]string {
	return map[string]string{"imdb": e.imdb + episodeSuffix(season, number), "tmdb": "tmdb:" + strconv.Itoa(e.tmdb) + episodeSuffix(season, number)}
}

// absoluteRun is a series the addon numbers as one season of 28
// episodes, which the other side numbers in three seasons of 10, 10 and
// 8.
func absoluteRun() []airedFixture {
	return renumbered(slices.Concat(weekly(1, 10, "2015-01-04"), weekly(2, 10, "2015-09-06"), weekly(3, 8, "2016-09-04")), 1, 1)
}

func TestStreamAddonsAreAskedForEpisodesAsTheirIdentifiersNumberThem(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name    string
		fixture func() []airedFixture
		// The addon's episode, and the other side's numbers of it.
		season, number, wantSeason, wantNumber int
	}{
		{"a split season", splitSeason, 1, 15, 2, 5},
		{"a special", familyGuyEighthSeason, 8, 21, 0, 28},
		{"an absolute numbering", absoluteRun, 1, 25, 3, 5},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			e := newNumberedEnv(t, c.fixture(), 81189, 1396, "tt0903747", true)
			e.prime()
			if got, want := e.askedAs(c.season, c.number), e.wantAsked(c.wantSeason, c.wantNumber); !maps.Equal(got, want) {
				t.Errorf("asked %v, want %v", got, want)
			}
			numbers := fmt.Sprintf("S%dE%d", c.wantSeason, c.wantNumber)
			versions, err := e.service.Versions(t.Context(), e.member, itemID(episodeKey(e.series+episodeSuffix(c.season, c.number))))
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, version := range versions {
				names = append(names, version.Name)
			}
			slices.Sort(names)
			if want := []string{"IMDb " + numbers, "TMDB " + numbers}; !slices.Equal(names, want) {
				t.Errorf("versions %v, want %v", names, want)
			}
		})
	}
}

func TestStreamAddonsAreAskedForEpisodesByTheirOwnNumbersWithoutPairs(t *testing.T) {
	t.Parallel()
	t.Run("nothing numbers the series otherwise", func(t *testing.T) {
		t.Parallel()
		// No addon describes the series by IMDb's or TMDB's identifier,
		// and the server has no TMDB key.
		e := newNumberedEnv(t, splitSeason(), 81189, 1396, "tt0903747", false)
		if got, want := e.askedAs(1, 15), e.wantAsked(1, 15); !maps.Equal(got, want) {
			t.Errorf("asked %v, want %v", got, want)
		}
		// TMDB's numbering was looked for in the background, and none was
		// found.
		if series := <-e.filled; series != e.series {
			t.Errorf("filled %q", series)
		}
		if got, want := e.askedAs(1, 15), e.wantAsked(1, 15); !maps.Equal(got, want) {
			t.Errorf("then asked %v, want %v", got, want)
		}
	})
	t.Run("the other side numbers it alike", func(t *testing.T) {
		t.Parallel()
		e := newNumberedEnv(t, weekly(1, 8, "2021-02-07"), 81189, 1396, "tt0903747", true)
		e.prime()
		if got, want := e.askedAs(1, 5), e.wantAsked(1, 5); !maps.Equal(got, want) {
			t.Errorf("asked %v, want %v", got, want)
		}
	})
}

// The pairs of a series are pinned: once made, its requests use them
// while what made them is gone, until they are made again, from what the
// background asked for.
func TestStreamNumberingIsPinnedPerSeries(t *testing.T) {
	t.Parallel()
	e := newNumberedEnv(t, splitSeason(), 81189, 1396, "tt0903747", true)
	e.prime()
	want := e.wantAsked(2, 5)
	if got := e.askedAs(1, 15); !maps.Equal(got, want) {
		t.Fatalf("asked %v, want %v", got, want)
	}
	// What was kept is gone: the pairs pinned still answer, for every
	// episode of the series.
	ctx := t.Context()
	if _, err := e.service.db.Exec(ctx, "DELETE FROM episode_alignments WHERE series = $1", e.series); err != nil {
		t.Fatal(err)
	}
	if err := e.service.forgetMeta(ctx, metaKey{e.installedID(addons.Shared(), "imdb-meta"), "series", e.imdb}); err != nil {
		t.Fatal(err)
	}
	if got := e.askedAs(1, 15); !maps.Equal(got, want) {
		t.Errorf("pinned, asked %v, want %v", got, want)
	}
	if got, want := e.askedAs(1, 12), e.wantAsked(2, 2); !maps.Equal(got, want) {
		t.Errorf("pinned, the addon's S1E12 asked %v, want %v", got, want)
	}
	asked := metaAsked(e.imdbMeta, "series", e.imdb)
	// An hour later, they are made again from what is kept: nothing, so
	// the episode keeps its numbers while both are asked again in the
	// background.
	e.service.SetClock(func() time.Time { return time.Now().Add(streamPinned + time.Minute) })
	if got, want := e.askedAs(1, 15), (map[string]string{"imdb": e.imdb + ":1:15", "tmdb": "tmdb:1396:1:15"}); !maps.Equal(got, want) {
		t.Errorf("nothing kept, asked %v, want %v", got, want)
	}
	for range 2 {
		if series := <-e.filled; series != e.series {
			t.Errorf("filled %q", series)
		}
	}
	if got := metaAsked(e.imdbMeta, "series", e.imdb); got != asked+1 {
		t.Errorf("the IMDb identifier's description asked %d times, want %d", got, asked+1)
	}
	if got := e.askedAs(1, 15); !maps.Equal(got, want) {
		t.Errorf("made again, asked %v, want %v", got, want)
	}
}
