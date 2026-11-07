package trackers

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/importlinks"
	"github.com/moodiness/polyfin/internal/library"
)

// withLinks has the imports of h keep the titles they do not find, and
// apply the links made of them.
func (h harness) withLinks(t *testing.T) {
	t.Helper()
	titles, ok := h.titles.(importlinks.Titles)
	if !ok {
		t.Fatalf("%T cannot describe titles", h.titles)
	}
	h.links = importlinks.New(importlinks.Options{DB: h.db, Settings: h.store.Settings, Titles: titles, UserData: h.userData,
		Watchlist: h.watchlist})
}

// kept lists what the imports of source kept for user, by key, and season
// and episode for an episode.
func (h harness) kept(t *testing.T, user accounts.ID, source string) map[string]importlinks.Row {
	t.Helper()
	rows, err := h.links.List(t.Context(), user, source)
	if err != nil {
		t.Fatal(err)
	}
	kept := make(map[string]importlinks.Row, len(rows))
	for _, row := range rows {
		place := row.Key
		if row.Kind == importlinks.KindEpisode {
			place = fmt.Sprintf("%s %dx%d", row.Key, row.Season, row.Episode)
		}
		kept[place] = row
	}
	return kept
}

// importAgain imports user's history from service at once.
func (h harness) importAgain(t *testing.T, user accounts.ID, service string) ImportResult {
	t.Helper()
	return h.imported(t, user, service, func() {
		if _, err := h.ImportNow(t.Context(), user, service); err != nil {
			t.Fatal(err)
		}
	})
}

func TestTraktTitlesNotFoundAreKeptAndLinked(t *testing.T) {
	h := newHarness(t)
	h.withLinks(t)
	user := h.user(t, "kim")
	h.connected(t, user, Trakt, "trakt-kim")
	h.f.reply("GET", "/trakt/sync/last_activities", http.StatusOK,
		`{"movies":{"watched_at":"2026-10-01T10:00:00.000Z"},"episodes":{"watched_at":"2026-10-01T10:00:00.000Z"}}`)
	// Polyfin never listed the titles known only by TMDB.
	h.f.reply("GET", "/trakt/sync/history/movies", http.StatusOK, `[
		{"watched_at":"2026-09-01T20:00:00.000Z","type":"movie","movie":{"ids":{"trakt":1,"imdb":"tt0000001"}}},
		{"watched_at":"2026-09-02T20:00:00.000Z","type":"movie","movie":{"title":"Lost movie","ids":{"trakt":2,"tmdb":99}}}]`)
	h.f.reply("GET", "/trakt/sync/history/episodes", http.StatusOK,
		`[{"watched_at":"2026-09-03T21:00:00.000Z","type":"episode","episode":{"season":1,"number":2},"show":{"title":"Lost show","ids":{"tmdb":77}}}]`)
	h.f.reply("GET", "/trakt/sync/playback", http.StatusOK,
		`[{"progress":25,"paused_at":"2026-10-03T08:00:00.000Z","type":"movie","movie":{"title":"Paused movie","runtime":120,"ids":{"tmdb":98}}}]`)

	if result := h.turnOn(t, user, Trakt); result.Played != 1 || result.Unmapped != 3 || result.Problem != "" {
		t.Fatalf("result: %+v", result)
	}
	kept := h.kept(t, user, Trakt)
	if len(kept) != 3 {
		t.Fatalf("kept: %+v", kept)
	}
	if e := kept["movie:tmdb:99"]; e.Kind != importlinks.KindMovie || e.Name != "Lost movie" || e.IDs != "tmdb:99" ||
		e.Reason != "not_found" || !e.Played || e.PlayedAt == nil || !e.PlayedAt.Equal(*at("2026-09-02T20:00:00Z")) || e.Resume != nil {
		t.Errorf("movie: %+v", e)
	}
	if e := kept["series:tmdb:77 1x2"]; e.Kind != importlinks.KindEpisode || e.Name != "Lost show" || e.IDs != "tmdb:77" ||
		e.Reason != "not_found" || !e.Played {
		t.Errorf("episode: %+v", e)
	}
	if e := kept["movie:tmdb:98"]; e.Played || e.Resume == nil || e.Resume.Percent != 25 || e.Resume.Runtime != 2*time.Hour ||
		!e.Resume.At.Equal(*at("2026-10-03T08:00:00Z")) {
		t.Errorf("paused movie: %+v %+v", e, e.Resume)
	}

	// A link applies at once what the import would have.
	if _, err := h.links.Link(t.Context(), user, user, Trakt, "movie:tmdb:99", "tt0000050"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.links.Link(t.Context(), user, user, Trakt, "series:tmdb:77", "tt0000060"); err != nil {
		t.Fatal(err)
	}
	linkedMovie := library.TitleRef{IMDb: "tt0000050"}
	episode := func(n int) library.TitleRef {
		return library.TitleRef{Episode: true, IMDb: "tt0000060", Season: 1, Number: n}
	}
	got := h.data(t, user, linkedMovie, episode(2))
	if !got[0].Played || got[0].LastPlayed == nil || !got[0].LastPlayed.Equal(*at("2026-09-02T20:00:00Z")) {
		t.Errorf("linked movie: %+v", got[0])
	}
	if !got[1].Played {
		t.Errorf("linked series' episode: %+v", got[1])
	}

	// The next import meets the linked titles again, and a new episode of
	// the linked series: they are found by their links. It reads what
	// changed since the last one, so what it does not read stays kept.
	h.f.reply("GET", "/trakt/sync/last_activities", http.StatusOK,
		`{"movies":{"watched_at":"2026-10-04T10:00:00.000Z"},"episodes":{"watched_at":"2026-10-04T10:00:00.000Z"}}`)
	h.f.reply("GET", "/trakt/sync/history/movies", http.StatusOK, `[
		{"watched_at":"2026-09-02T20:00:00.000Z","type":"movie","movie":{"title":"Lost movie","ids":{"trakt":2,"tmdb":99}}},
		{"watched_at":"2026-10-04T09:00:00.000Z","type":"movie","movie":{"title":"Newer lost movie","ids":{"trakt":3,"tmdb":97}}}]`)
	h.f.reply("GET", "/trakt/sync/history/episodes", http.StatusOK, `[
		{"watched_at":"2026-09-03T21:00:00.000Z","type":"episode","episode":{"season":1,"number":2},"show":{"title":"Lost show","ids":{"tmdb":77}}},
		{"watched_at":"2026-10-04T08:00:00.000Z","type":"episode","episode":{"season":1,"number":3},"show":{"title":"Lost show","ids":{"tmdb":77}}}]`)
	h.f.reply("GET", "/trakt/sync/playback", http.StatusOK, `[]`)
	if result := h.importAgain(t, user, Trakt); result.Played != 1 || result.Unmapped != 1 || result.Problem != "" {
		t.Errorf("again: %+v", result)
	}
	if got := h.data(t, user, episode(3)); !got[0].Played || !got[0].LastPlayed.Equal(*at("2026-10-04T08:00:00Z")) {
		t.Errorf("new episode of the linked series: %+v", got[0])
	}
	kept = h.kept(t, user, Trakt)
	if len(kept) != 4 || kept["movie:tmdb:97"].Reason != "not_found" || kept["movie:tmdb:98"].Resume == nil ||
		kept["movie:tmdb:99"].Link == nil || kept["movie:tmdb:99"].Link.IMDb != "tt0000050" || kept["series:tmdb:77 1x2"].Link == nil {
		t.Errorf("kept again: %+v", kept)
	}
}

func TestTitlesNoMappingKnowsAreKeptAndLinked(t *testing.T) {
	h := newHarness(t)
	h.withLinks(t)
	h.anime = animeMapping(t)
	user := h.user(t, "lee")
	h.connected(t, user, Simkl, "simkl-lee")
	h.f.reply("GET", "/simkl/sync/activities", http.StatusOK, `{"all":"2026-10-01T00:00:00Z"}`)
	// An entry the mapping does not know: its special is named no way,
	// its other episodes by the entry, which no title is.
	h.f.reply("GET", "/simkl/sync/all-items/anime/watching", http.StatusOK, `{"anime":[
		{"show":{"title":"Unknown","ids":{"simkl":902,"mal":"99"}},"anime_type":"tv",
			"seasons":[{"number":0,"episodes":[{"number":1,"watched_at":"2026-09-01T10:00:00Z"}]},
				{"number":1,"episodes":[{"number":1,"watched_at":"2026-09-02T10:00:00Z"}]}]}]}`)
	h.f.reply("GET", "/simkl/sync/playback", http.StatusOK, `[]`)

	if result := h.turnOn(t, user, Simkl); result.Unmapped != 2 || result.Problem != "" {
		t.Fatalf("result: %+v", result)
	}
	kept := h.kept(t, user, Simkl)
	if e := kept["anime:mal:99 0x1"]; len(kept) != 2 || e.Reason != "not_mapped" || e.Name != "Unknown" || e.IDs != "mal:99" || !e.Played {
		t.Fatalf("kept: %+v", kept)
	}
	if e := kept["anime:mal:99 1x1"]; e.Reason != "not_found" || e.IDs != "mal:99" || !e.Played {
		t.Errorf("episode named by the entry: %+v", e)
	}

	if _, err := h.links.Link(t.Context(), user, user, Simkl, "anime:mal:99", "tt0000070"); err != nil {
		t.Fatal(err)
	}
	episode := func(season, n int) library.TitleRef {
		return library.TitleRef{Episode: true, IMDb: "tt0000070", Season: season, Number: n}
	}
	if got := h.data(t, user, episode(0, 1), episode(1, 1)); !got[0].Played || !got[1].Played {
		t.Errorf("linked: %+v %+v", got[0], got[1])
	}

	// Watched since: the next import finds the entry's episodes by its link.
	h.f.reply("GET", "/simkl/sync/activities", http.StatusOK, `{"all":"2026-10-04T00:00:00Z"}`)
	h.f.reply("GET", "/simkl/sync/all-items/anime/watching", http.StatusOK, `{"anime":[
		{"show":{"title":"Unknown","ids":{"simkl":902,"mal":"99"}},"anime_type":"tv",
			"seasons":[{"number":0,"episodes":[{"number":2,"watched_at":"2026-10-03T10:00:00Z"}]},
				{"number":1,"episodes":[{"number":2,"watched_at":"2026-10-03T11:00:00Z"}]}]}]}`)
	if result := h.importAgain(t, user, Simkl); result.Played != 2 || result.Unmapped != 0 || result.Problem != "" {
		t.Errorf("again: %+v", result)
	}
	if got := h.data(t, user, episode(0, 2), episode(1, 2)); !got[0].Played || !got[1].Played {
		t.Errorf("linked again: %+v %+v", got[0], got[1])
	}
}
