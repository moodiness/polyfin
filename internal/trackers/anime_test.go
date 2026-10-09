package trackers

import (
	"context"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/anime"
	"github.com/moodiness/polyfin/internal/library"
)

// The anime mapping's lists: a series AniDB splits in two entries, its
// first and second seasons on TVDB; a long series numbered as TVDB's
// absolute numbering; a movie.
const (
	animeIDs = `[
		{"type":"TV","anidb_id":1,"kitsu_id":11,"mal_id":21,"imdb_id":["tt0000100"],"tvdb_id":500},
		{"type":"TV","anidb_id":2,"kitsu_id":12,"mal_id":22,"imdb_id":["tt0000100"],"tvdb_id":500},
		{"type":"TV","anidb_id":3,"mal_id":23,"imdb_id":["tt0000300"],"tvdb_id":600},
		{"type":"MOVIE","anidb_id":4,"kitsu_id":14,"imdb_id":["tt0000400"],"themoviedb_id":{"movie":[4000]}}
	]`
	animeEpisodes = `<anime-list>
		<anime anidbid="1" tvdbid="500" defaulttvdbseason="1"><name>First</name></anime>
		<anime anidbid="2" tvdbid="500" defaulttvdbseason="2"><name>Second</name></anime>
		<anime anidbid="3" tvdbid="600" defaulttvdbseason="a"><name>Long</name></anime>
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
	eventually(t, "the anime mapping being read", m.Ready)
	return m
}

// listLongSeries has Polyfin list the first season of the absolutely
// numbered series, two episodes, and the first episode of its second.
func (h harness) listLongSeries(t *testing.T) {
	t.Helper()
	if _, err := h.titles.Resolve(t.Context(), []library.TitleRef{
		{Episode: true, IMDb: "tt0000300", Season: 1, Number: 1}, {Episode: true, IMDb: "tt0000300", Season: 1, Number: 2},
		{Episode: true, IMDb: "tt0000300", Season: 2, Number: 1},
	}); err != nil {
		t.Fatal(err)
	}
}

// itemOf is the item Polyfin keeps under key, as the library derives it.
func itemOf(key string) accounts.ID {
	sum := sha256.Sum256([]byte("polyfin:item:" + key))
	var id accounts.ID
	copy(id[:], sum[:16])
	return id
}

// listAnime records what an anime catalog listed: the second entry of the
// split series under its Kitsu identifier, with three episodes numbered
// as the entry numbers them, and the movie.
func (h harness) listAnime(t *testing.T) {
	t.Helper()
	records := map[string]string{
		"series|kitsu:12": `{"kind":"series","meta":{"id":"kitsu:12","type":"series","name":"Second"}}`,
		"movie|kitsu:14":  `{"kind":"movie","meta":{"id":"kitsu:14","type":"movie","name":"The movie"}}`,
	}
	for n := 1; n <= 3; n++ {
		video := "kitsu:12:" + strconv.Itoa(n)
		records["episode|"+video] = `{"kind":"episode","seriesId":"kitsu:12","season":1,"video":{"id":"` + video + `","season":1,"episode":` + strconv.Itoa(n) + `}}`
	}
	for key, data := range records {
		kind, _, _ := strings.Cut(key, "|")
		if _, err := h.db.Exec(t.Context(), "INSERT INTO items (id, key, kind, data) VALUES ($1, $2, $3, $4)", itemOf(key), key, kind, data); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSimklAnimeHistoryIsImportedInTVDBNumbering(t *testing.T) {
	h := newHarness(t)
	user := h.user(t, "ivy")
	h.connected(t, user, Simkl, "simkl-ivy")
	h.listLongSeries(t)
	h.listAnime(t)
	h.f.reply("GET", "/simkl/sync/activities", http.StatusOK, `{"all":"2026-10-01T00:00:00Z"}`)
	h.f.reply("GET", "/simkl/sync/all-items/anime/watching", http.StatusOK, `{"anime":[
		{"show":{"title":"Second","ids":{"simkl":900,"anidb":"2","mal":"22"}},"anime_type":"tv",
			"seasons":[{"number":1,"episodes":[{"number":1,"watched_at":"2026-09-01T10:00:00Z"},{"number":2,"watched_at":"2026-09-02T10:00:00Z"}]}]},
		{"show":{"title":"Long","ids":{"simkl":901,"mal":"23"}},"anime_type":"tv",
			"seasons":[{"number":1,"episodes":[{"number":3,"watched_at":"2026-09-03T10:00:00Z"}]}]},
		{"show":{"title":"Unknown","ids":{"simkl":902,"mal":"99"}},"anime_type":"tv",
			"seasons":[{"number":1,"episodes":[{"number":1},{"number":2}]}]}]}`)
	h.f.reply("GET", "/simkl/sync/all-items/anime/completed", http.StatusOK,
		`{"anime":[{"last_watched_at":"2026-08-01T20:00:00Z","anime_type":"movie","show":{"title":"The movie","ids":{"simkl":903,"kitsu":"14"}}}]}`)
	h.f.reply("GET", "/simkl/sync/playback", http.StatusOK, `[
		{"progress":95,"paused_at":"2026-10-02T08:00:00Z","type":"episode","anime":{"title":"First","ids":{"kitsu":"11"}},"episode":{"episode":4}},
		{"progress":50,"paused_at":"2026-10-02T09:00:00Z","type":"episode","anime":{"title":"Unknown","ids":{"kitsu":"98"}},"episode":{"episode":1}}]`)

	// Until the mapping is read, anime is left for a later import.
	if result := h.turnOn(t, user, Simkl); result.Played != 0 || result.Unmapped != 0 {
		t.Errorf("without the mapping: %+v", result)
	}
	if n := len(h.f.sent("simkl", "/sync/all-items/anime/watching")); n != 0 {
		t.Errorf("anime read without the mapping: %d", n)
	}
	h.anime = animeMapping(t)
	again := func() ImportResult {
		return h.imported(t, user, Simkl, func() {
			if _, err := h.ImportNow(t.Context(), user, Simkl); err != nil {
				t.Fatal(err)
			}
		})
	}
	// Then it is read whole, though the shows did not change: two
	// episodes of the second season, the long series' third episode, the
	// movie, and the paused episode played past the played mark; the
	// unknown anime's two episodes and its paused one are not found. The
	// titles an anime catalog listed count with those found by IMDb and
	// TVDB, once.
	if result := again(); result.Played != 5 || result.Unmapped != 3 || result.Problem != "" {
		t.Errorf("result: %+v", result)
	}
	if watching := h.f.sent("simkl", "/sync/all-items/anime/watching"); len(watching) != 1 || watching[0].query.Get("date_from") != "" ||
		watching[0].query.Get("episode_watched_at") != "yes" {
		t.Errorf("anime read: %+v", watching)
	}
	if n := len(h.f.sent("simkl", "/sync/all-items/shows/watching")); n != 1 {
		t.Errorf("shows read again unchanged: %d", n)
	}
	second := library.TitleRef{Episode: true, IMDb: "tt0000100", Season: 2, Number: 1}
	got := h.data(t, user, second, library.TitleRef{Episode: true, IMDb: "tt0000100", Season: 2, Number: 2},
		library.TitleRef{Episode: true, IMDb: "tt0000300", Season: 2, Number: 1}, library.TitleRef{IMDb: "tt0000400"},
		library.TitleRef{Episode: true, IMDb: "tt0000100", Season: 1, Number: 4}, library.TitleRef{Episode: true, IMDb: "tt0000100", Season: 1, Number: 1},
		library.TitleRef{Episode: true, IMDb: "tt0000300", Season: 1, Number: 3})
	if !got[0].Played || !got[0].LastPlayed.Equal(*at("2026-09-01T10:00:00Z")) || !got[1].Played {
		t.Errorf("second season: %+v %+v", got[0], got[1])
	}
	if !got[2].Played || !got[3].Played || !got[3].LastPlayed.Equal(*at("2026-08-01T20:00:00Z")) || !got[4].Played {
		t.Errorf("long series, movie, paused episode: %+v %+v %+v", got[2], got[3], got[4])
	}
	// AniDB's numbers are not taken for TVDB's.
	if got[5].Played || got[6].Played {
		t.Errorf("numbered as AniDB: %+v %+v", got[5], got[6])
	}
	// As the anime catalog listed them, the entry's first two episodes and
	// the movie are played too, the entry's third is not, and the first
	// entry, which no anime catalog listed, gets no record.
	listed, err := h.userData.Get(t.Context(), user, []accounts.ID{itemOf("episode|kitsu:12:1"), itemOf("episode|kitsu:12:2"),
		itemOf("episode|kitsu:12:3"), itemOf("movie|kitsu:14")})
	if err != nil {
		t.Fatal(err)
	}
	if first := listed[itemOf("episode|kitsu:12:1")]; !first.Played || !first.LastPlayed.Equal(*at("2026-09-01T10:00:00Z")) ||
		!listed[itemOf("episode|kitsu:12:2")].Played || listed[itemOf("episode|kitsu:12:3")].Played || !listed[itemOf("movie|kitsu:14")].Played {
		t.Errorf("listed by an anime catalog: %+v", listed)
	}
	var unlisted int
	if err := h.db.QueryRow(t.Context(), "SELECT count(*) FROM items WHERE key LIKE '%kitsu:11%'").Scan(&unlisted); err != nil || unlisted != 0 {
		t.Errorf("%d records of the unlisted entry: %v", unlisted, err)
	}
	// Nothing changed: the anime lists are not read again.
	again()
	if n := len(h.f.sent("simkl", "/sync/all-items/anime/watching")); n != 1 {
		t.Errorf("anime read again unchanged: %d", n)
	}
	h.noWrites(t)
}

func TestTitlesAnimeAddonsNameAreSentInTVDBNumbering(t *testing.T) {
	h := newHarness(t)
	h.anime = animeMapping(t)
	h.listLongSeries(t)
	user := h.connectAll(t, "jane")
	start := func(stremioID string, movie bool, item accounts.ID) {
		h.Playback(user, Playback{Event: Started, Device: device, Item: item, PositionKnown: true, Runtime: 100 * time.Minute,
			Title: func(ctx context.Context) (Title, bool) { return h.Anime(ctx, item, stremioID, movie) }})
	}
	// The third episode of AniDB's second entry, the movie, and the long
	// series' third episode, numbered absolutely.
	start("kitsu:12:3", false, itemID(50))
	start("kitsu:14", true, itemID(51))
	start("mal:23:3", false, itemID(52))
	starts := h.f.wait(t, 3, "trakt", "/scrobble/start")
	sameJSON(t, "episode", starts[0].body, `{"show":{"ids":{"imdb":"tt0000100","tvdb":500}},"episode":{"season":2,"number":3},"progress":0}`)
	sameJSON(t, "movie", starts[1].body, `{"movie":{"ids":{"imdb":"tt0000400","tmdb":4000}},"progress":0}`)
	sameJSON(t, "absolute", starts[2].body, `{"show":{"ids":{"imdb":"tt0000300","tvdb":600}},"episode":{"season":2,"number":1},"progress":0}`)

	// What the mapping does not know, or names otherwise, is left to the
	// item's own identifiers.
	for _, id := range []string{"kitsu:98:1", "tt0000100:1:1", "kitsu:12"} {
		if title, ok := h.Anime(t.Context(), itemID(53), id, false); ok {
			t.Errorf("%s: %+v", id, title)
		}
	}
}
