package appimport

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/anime"
	"github.com/moodiness/polyfin/internal/library"
)

// The anime mapping's lists: a series the lists split in two entries, its
// first and second seasons on TVDB, and a movie.
const (
	animeIDs = `[
		{"type":"TV","anidb_id":1,"kitsu_id":11,"mal_id":21,"anilist_id":31,"imdb_id":["tt0000100"],"tvdb_id":500},
		{"type":"TV","anidb_id":2,"kitsu_id":12,"mal_id":22,"anilist_id":32,"imdb_id":["tt0000100"],"tvdb_id":500},
		{"type":"MOVIE","anidb_id":4,"kitsu_id":14,"mal_id":24,"anilist_id":34,"imdb_id":["tt0000400"],"themoviedb_id":{"movie":[4000]}}
	]`
	animeEpisodes = `<anime-list>
		<anime anidbid="1" tvdbid="500" defaulttvdbseason="1"><name>First</name></anime>
		<anime anidbid="2" tvdbid="500" defaulttvdbseason="2"><name>Second</name></anime>
		<anime anidbid="4" tvdbid="movie" defaulttvdbseason="1"><name>The movie</name></anime>
	</anime-list>`
)

// animeMapping is a mapping that read the lists above.
func animeMapping(t *testing.T) *anime.Service {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ids.json" {
			_, _ = w.Write([]byte(animeIDs))
			return
		}
		_, _ = w.Write([]byte(animeEpisodes))
	}))
	t.Cleanup(server.Close)
	m := anime.New(anime.Options{Dir: t.TempDir(), Version: "1.2.3", IDsURL: server.URL + "/ids.json", EpisodesURL: server.URL + "/episodes.xml"})
	t.Cleanup(m.Close)
	deadline := time.Now().Add(10 * time.Second)
	for !m.Ready() {
		if time.Now().After(deadline) {
			t.Fatal("the anime mapping was never read")
		}
		time.Sleep(5 * time.Millisecond)
	}
	return m
}

// MyAnimeList identifiers, and the lists' other anime identifiers, go
// through the anime mapping as Kitsu ones do: to the title and TVDB
// numbering it gives, and the entry as anime catalogs list it, its
// episodes numbered in the entry.
func TestAnimeIdentifiersGoThroughTheMapping(t *testing.T) {
	t.Parallel()
	mapping := animeMapping(t)
	second := library.AnimeRef{Kitsu: 12, MyAnimeList: 22, AniDB: 2, Episode: 2}
	episode := library.TitleRef{Episode: true, IMDb: "tt0000100", TVDB: 500, Season: 2, Number: 2, Anime: second}
	movie := library.TitleRef{IMDb: "tt0000400", TMDB: 4000, Anime: library.AnimeRef{Kitsu: 14, MyAnimeList: 24, AniDB: 4}}
	for _, c := range []struct {
		id              string
		movie           bool
		season, episode int
		want            library.TitleRef
	}{
		{"kitsu:12", false, 1, 2, episode},
		{"mal:22", false, 1, 2, episode},
		{"anidb:2", false, 1, 2, episode},
		{"mal:22", false, 0, 0, library.TitleRef{Series: true, IMDb: "tt0000100", TVDB: 500, Anime: library.AnimeRef{Kitsu: 12, MyAnimeList: 22, AniDB: 2}}},
		{"mal:24", true, 0, 0, movie},
		{"anidb:4", true, 0, 0, movie},
		// An entry the lists do not know, as anime catalogs list it.
		{"mal:99", false, 1, 3, library.TitleRef{Episode: true, Anime: library.AnimeRef{MyAnimeList: 99, Episode: 3}}},
	} {
		got, ok := TitleRef(mapping, c.id, c.movie, c.season, c.episode)
		if !ok || got != c.want {
			t.Errorf("%s %d %d: %+v %v, want %+v", c.id, c.season, c.episode, got, ok, c.want)
		}
	}
}

// An app's MyAnimeList entry is imported onto the title the mapping gives,
// and the entry the lists do not know onto the one an anime catalog
// listed under it.
func TestMyAnimeListEntriesAreImported(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.user(t, "alice", false)
	mapping := animeMapping(t)
	h.listed(t, "series", "series|mal:99", `{"meta":{"id":"mal:99","type":"series","name":"Listed by MyAnimeList"}}`)
	h.listed(t, "episode", "episode|mal:99:3", `{"seriesId":"mal:99","season":1,"video":{"id":"mal:99:3","season":1,"episode":3}}`)
	mapped, ok := TitleRef(mapping, "mal:22", false, 1, 2)
	if !ok {
		t.Fatal("mal:22 is not matched")
	}
	listed, ok := TitleRef(mapping, "mal:99", false, 1, 3)
	if !ok {
		t.Fatal("mal:99 is not matched")
	}
	entries := []Entry{
		{Title: mapped, Label: Label{ID: "mal:22", Name: "Second", Kind: "episode", Season: 1, Episode: 2}, Played: true},
		{Title: listed, Label: Label{ID: "mal:99", Name: "Listed by MyAnimeList", Kind: "episode", Season: 1, Episode: 3}, Played: true},
	}
	status := h.imported(t, alice, alice, Account{App: "test", Profiles: []Profile{{ID: "1", Entries: entries}}}, Choice{Profile: "1"})
	if status.UnmatchedCount != 0 {
		t.Errorf("unmatched: %d", status.UnmatchedCount)
	}
	for name, ref := range map[string]library.TitleRef{
		"the mapped episode": {Episode: true, IMDb: "tt0000100", Season: 2, Number: 2},
		"the listed episode": listed,
	} {
		if !h.dataOf(t, alice.ID, ref).Played {
			t.Errorf("%s is not played", name)
		}
	}
}
