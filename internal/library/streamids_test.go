package library

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/anime"
	"github.com/moodiness/polyfin/internal/stremio"
)

// The anime mapping's lists: a series AniDB splits in two entries, the
// first and second seasons of tt0000100 on TVDB; a movie; and a movie
// two entries claim, which no addon is asked for by an anime identifier.
const (
	streamAnimeIDs = `[
		{"type":"TV","anidb_id":1,"kitsu_id":11,"mal_id":21,"imdb_id":["tt0000100"],"tvdb_id":500},
		{"type":"TV","anidb_id":2,"kitsu_id":12,"mal_id":22,"imdb_id":["tt0000100"],"tvdb_id":500},
		{"type":"MOVIE","anidb_id":4,"kitsu_id":14,"mal_id":24,"imdb_id":["tt0000400"],"themoviedb_id":{"movie":[4000]}},
		{"type":"MOVIE","anidb_id":6,"kitsu_id":16,"imdb_id":["tt0000600"]},
		{"type":"MOVIE","anidb_id":7,"kitsu_id":17,"imdb_id":["tt0000600"]}
	]`
	streamAnimeEpisodes = `<anime-list>
		<anime anidbid="1" tvdbid="500" defaulttvdbseason="1"><name>First</name></anime>
		<anime anidbid="2" tvdbid="500" defaulttvdbseason="2"><name>Second</name></anime>
		<anime anidbid="4" tvdbid="movie" defaulttvdbseason="1"><name>The movie</name></anime>
	</anime-list>`
)

// useAnimeMapping gives e's service a mapping that read the lists above.
func (e env) useAnimeMapping() {
	e.t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ids.json" {
			_, _ = w.Write([]byte(streamAnimeIDs))
			return
		}
		_, _ = w.Write([]byte(streamAnimeEpisodes))
	}))
	e.t.Cleanup(server.Close)
	mapping := anime.New(anime.Options{Dir: e.t.TempDir(), Version: "1.2.3", IDsURL: server.URL + "/ids.json", EpisodesURL: server.URL + "/episodes.xml"})
	e.t.Cleanup(mapping.Close)
	eventually(e.t, "the anime mapping being read", mapping.Ready)
	e.service.UseAnime(mapping)
}

// streamsOnly is an addon that lists streams only for the identifiers that
// begin with prefix, one stream named name for each key of streams.
func streamsOnly(id, prefix, name string, keys ...string) *fakeAddon {
	streams := map[string][]stremio.Stream{}
	for _, key := range keys {
		streams[key] = []stremio.Stream{{Name: name, URL: "https://cdn.example/" + id + "/" + strings.ReplaceAll(key, ":", "-"),
			BehaviorHints: stremio.StreamBehavior{Filename: name + ".mkv", VideoSize: 1_000_000}}}
	}
	return &fakeAddon{
		manifest: stremio.Manifest{ID: id, Name: id, Version: "1", Types: []string{"movie", "series"},
			Resources: []stremio.Resource{{Name: "stream", IDPrefixes: []string{prefix}}}},
		streams: streams,
	}
}

// streamRequests lists the stream requests addon received.
func (a *fakeAddon) streamRequests() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var result []string
	for _, request := range a.requests {
		if strings.HasPrefix(request, "stream/") {
			unescaped, _ := url.PathUnescape(request)
			result = append(result, unescaped)
		}
	}
	return result
}

// catalogOf is an addon listing metas in one catalog of type kind, and
// describing them, for the identifiers that begin with prefix.
func catalogOf(kind, prefix string, metas ...stremio.Meta) *fakeAddon {
	described := map[string]stremio.Meta{}
	previews := make([]stremio.Meta, len(metas))
	for i, meta := range metas {
		described[kind+"/"+meta.ID] = meta
		previews[i] = meta
		previews[i].Videos = nil
	}
	return &fakeAddon{
		manifest: stremio.Manifest{ID: "catalog", Name: "Catalog", Version: "1", Types: []string{kind},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta", Types: []string{kind}, IDPrefixes: []string{prefix}}},
			Catalogs:  []stremio.Catalog{{Type: kind, ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{kind + "/top": previews},
		metas:    described,
	}
}

// versionNames lists the names of the versions of the only title of the
// library Top, sorted, or of its episode number of season when it is a
// series.
func (e env) versionNames(season, number int) []string {
	e.t.Helper()
	ctx := e.t.Context()
	page, err := e.service.Children(ctx, e.member, e.library(e.member, "Top").ID, 0, 10, "")
	if err != nil || len(page.Items) != 1 {
		e.t.Fatalf("listing: %+v %v", page, err)
	}
	item := page.Items[0]
	if item.Kind == KindSeries {
		episodes, err := e.service.Episodes(ctx, e.member, item.ID, nil)
		if err != nil {
			e.t.Fatal(err)
		}
		i := slices.IndexFunc(episodes, func(episode Item) bool {
			return episode.ParentIndexNumber == season && episode.IndexNumber == number
		})
		if i < 0 {
			e.t.Fatalf("no episode %d of season %d among %+v", number, season, episodes)
		}
		item = episodes[i]
	}
	versions, err := e.service.Versions(ctx, e.member, item.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	var result []string
	for _, version := range versions {
		result = append(result, version.Name)
	}
	slices.Sort(result)
	return result
}

func TestAddonsAreAskedForAnIMDbEpisodeByIdentifiersTheyAccept(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.useAnimeMapping()
	show := stremio.Meta{ID: "tt0000100", Type: "series", Name: "Show", TmdbID: "900", Videos: []stremio.Video{
		{ID: "tt0000100:1:1", Season: 1, Episode: 1}, {ID: "tt0000100:2:3", Season: 2, Episode: 3},
	}}
	e.install(addons.Shared(), catalogOf("series", "tt", show))
	// The anime addon counts the second season's episodes from 1 again, in
	// its own entry; the first entry's third episode is another episode.
	kitsu := streamsOnly("kitsu", "kitsu:", "Anime", "series/kitsu:12:3", "series/kitsu:11:3")
	tmdb := streamsOnly("tmdb", "tmdb:", "TMDB", "series/tmdb:900:2:3")
	e.install(addons.Shared(), kitsu)
	e.install(addons.Shared(), tmdb)
	if got := e.versionNames(2, 3); !slices.Equal(got, []string{"Anime", "TMDB"}) {
		t.Fatalf("versions: %v; asked %v and %v", got, kitsu.streamRequests(), tmdb.streamRequests())
	}
	if got := kitsu.streamRequests(); !slices.Equal(got, []string{"stream/series/kitsu:12:3"}) {
		t.Errorf("the anime addon was asked %v", got)
	}
}

func TestAddonsAreAskedForAnAnimeEpisodeByIdentifiersTheyAccept(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.useAnimeMapping()
	// The anime catalog gives no IMDb identifier: the mapping does.
	show := stremio.Meta{ID: "kitsu:12", Type: "series", Name: "Second", Videos: []stremio.Video{
		{ID: "kitsu:12:3", Season: 1, Episode: 3},
	}}
	e.install(addons.Shared(), catalogOf("series", "kitsu:", show))
	imdb := streamsOnly("imdb", "tt", "IMDb", "series/tt0000100:2:3")
	mal := streamsOnly("mal", "mal:", "MAL", "series/mal:22:3")
	e.install(addons.Shared(), imdb)
	e.install(addons.Shared(), mal)
	if got := e.versionNames(1, 3); !slices.Equal(got, []string{"IMDb", "MAL"}) {
		t.Fatalf("versions: %v; asked %v and %v", got, imdb.streamRequests(), mal.streamRequests())
	}
}

func TestAddonsAreAskedForAnAnimeMovieByIdentifiersTheyAccept(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.useAnimeMapping()
	movie := stremio.Meta{ID: "kitsu:14", Type: "movie", Name: "The movie"}
	e.install(addons.Shared(), catalogOf("movie", "kitsu:", movie))
	imdb := streamsOnly("imdb", "tt", "IMDb", "movie/tt0000400")
	tmdb := streamsOnly("tmdb", "tmdb:", "TMDB", "movie/tmdb:4000")
	e.install(addons.Shared(), imdb)
	e.install(addons.Shared(), tmdb)
	if got := e.versionNames(0, 0); !slices.Equal(got, []string{"IMDb", "TMDB"}) {
		t.Fatalf("versions: %v; asked %v and %v", got, imdb.streamRequests(), tmdb.streamRequests())
	}
	// A version asked by another identifier is renewed by asking it again.
	versions, err := e.service.Versions(t.Context(), e.member, itemID(titleKey(KindMovie, "kitsu:14")))
	if err != nil || len(versions) != 2 {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	for _, version := range versions {
		if _, err := e.service.Renew(t.Context(), version); err != nil {
			t.Errorf("renewing %s (%s %s): %v", version.Name, version.Origin.Type, version.Origin.ID, err)
		}
	}
	// A refresh forgets the lists kept under the other identifiers too.
	imdb.mu.Lock()
	imdb.streams["movie/tt0000400"][0].Name = "IMDb again"
	imdb.mu.Unlock()
	if err := e.service.Refresh(t.Context(), e.member, versions[0].Item); err != nil {
		t.Fatal(err)
	}
	if got := e.versionNames(0, 0); !slices.Equal(got, []string{"IMDb again", "TMDB"}) {
		t.Errorf("versions after a refresh: %v", got)
	}
}

func TestAnAmbiguousMappingAsksNothing(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.useAnimeMapping()
	// Two anime entries claim the first movie's IMDb identifier, one the
	// second's.
	claimed := stremio.Meta{ID: "tt0000600", Type: "movie", Name: "Claimed"}
	mapped := stremio.Meta{ID: "tt0000400", Type: "movie", Name: "Mapped"}
	e.install(addons.Shared(), catalogOf("movie", "tt", claimed, mapped))
	kitsu := streamsOnly("kitsu", "kitsu:", "Anime", "movie/kitsu:14", "movie/kitsu:16", "movie/kitsu:17")
	e.install(addons.Shared(), kitsu)
	if _, err := e.service.Children(t.Context(), e.member, e.library(e.member, "Top").ID, 0, 10, ""); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]int{"tt0000600": 0, "tt0000400": 1} {
		versions, err := e.service.Versions(t.Context(), e.member, itemID(titleKey(KindMovie, id)))
		if err != nil || len(versions) != want {
			t.Errorf("versions of %s: %+v %v", id, versions, err)
		}
	}
	if got := kitsu.streamRequests(); !slices.Equal(got, []string{"stream/movie/kitsu:14"}) {
		t.Errorf("the anime addon was asked %v", got)
	}
}
