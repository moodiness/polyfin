package appimport

import (
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/importlinks"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// rowOf is the row of rows of key, at season and episode for an episode,
// failing the test when there is none.
func rowOf(t *testing.T, rows []importlinks.Row, key string, season, episode int) importlinks.Row {
	t.Helper()
	for _, r := range rows {
		if r.Key == key && r.Season == season && r.Episode == episode {
			return r
		}
	}
	t.Fatalf("no row %s S%dE%d in %+v", key, season, episode, rows)
	return importlinks.Row{}
}

// The titles an import does not find are kept with what it would have
// imported of them; a link applies that at once, and again at the next
// import, which then finds them through it, an episode at its own season
// and number.
func TestTitlesNotFoundAreKeptAndLinked(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.user(t, "alice", false)
	firstPlay, secondPlay := at("2026-09-01T20:00:00Z"), at("2026-10-01T20:00:00Z")
	movie := Entry{Title: ref(t, "tmdb:999999", true, 0, 0), Label: Label{ID: "tmdb:999999", Name: "Unknown", Kind: "movie"},
		Played: true, PlayedAt: &firstPlay, Plays: 2, Watchlist: true}
	episode := Entry{Title: ref(t, "tmdb:888888", false, 2, 3), Label: Label{ID: "tmdb:888888", Name: "Show", Kind: "episode", Season: 2,
		Episode: 3}, Played: true, PlayedAt: &firstPlay}
	home := Entry{Label: Label{ID: "local:home", Name: "Holiday", Kind: "movie"},
		Resume: &Resume{Position: 30 * time.Minute, Runtime: 100 * time.Minute, At: firstPlay}}
	account := Account{App: "nuvio", Profiles: []Profile{{ID: "1", Name: "Main", Entries: []Entry{movie, episode, home}}}}

	if status := h.imported(t, alice, alice, account, Choice{Profile: "1"}); status.State != StateDone || status.UnmatchedCount != 3 {
		t.Fatalf("the import: %+v", status)
	}
	kept := h.unfound(t, alice.ID, "nuvio")
	if len(kept) != 3 {
		t.Fatalf("kept: %+v", kept)
	}
	if r := rowOf(t, kept, "tmdb:999999", 0, 0); r.Kind != importlinks.KindMovie || r.Name != "Unknown" || r.IDs != "tmdb:999999" ||
		r.Reason != ReasonNotFound || !r.Played || !r.PlayedAt.Equal(firstPlay) || r.Plays != 2 || r.Watchlist != watchlist.Import ||
		r.Link != nil {
		t.Errorf("the movie: %+v", r)
	}
	if r := rowOf(t, kept, "tmdb:888888", 2, 3); r.Kind != importlinks.KindEpisode || r.Name != "Show" || r.Reason != ReasonNotFound ||
		!r.Played || r.Watchlist != "" {
		t.Errorf("the episode: %+v", r)
	}
	if r := rowOf(t, kept, "local:home", 0, 0); r.Reason != ReasonOtherIdentifier || r.Resume == nil ||
		r.Resume.Position != 30*time.Minute || r.Resume.Runtime != 100*time.Minute || !r.Resume.At.Equal(firstPlay) {
		t.Errorf("the home video: %+v", r)
	}

	// Linked, each title gets at once what the import would have brought.
	for key, target := range map[string]string{"tmdb:999999": "tt0000077", "tmdb:888888": "tt0000088", "local:home": "tt0000099"} {
		if _, err := h.links.Link(t.Context(), alice.ID, alice.ID, "nuvio", key, target); err != nil {
			t.Fatalf("linking %s: %v", key, err)
		}
	}
	linkedMovie := library.TitleRef{IMDb: "tt0000077"}
	episodeAt := func(number int) library.TitleRef {
		return library.TitleRef{IMDb: "tt0000088", Episode: true, Season: 2, Number: number}
	}
	check := func(name string, ref library.TitleRef, want func(userdata.Data) bool) {
		t.Helper()
		if d := h.dataOf(t, alice.ID, ref); !want(d) {
			t.Errorf("%s: %+v", name, d)
		}
	}
	check("movie, linked", linkedMovie, func(d userdata.Data) bool {
		return d.Played && d.PlayCount == 2 && d.LastPlayed.Equal(firstPlay)
	})
	check("episode, linked", episodeAt(3), func(d userdata.Data) bool { return d.Played && d.LastPlayed.Equal(firstPlay) })
	check("home video, linked", library.TitleRef{IMDb: "tt0000099"}, func(d userdata.Data) bool {
		return !d.Played && d.Position == 30*time.Minute
	})
	// The linked movie, on the app's library, is played: with nothing
	// left to watch, it adds nothing to the watchlist playlist.
	if got := h.watchlistOf(t, alice.ID); len(got) != 0 {
		t.Errorf("watchlist: %v, want none", got)
	}

	// The next import finds them through their links: the movie played
	// again, and the next episode of the series, at its own place.
	movie.PlayedAt = &secondPlay
	next := episode
	next.Title, next.Label.Episode, next.PlayedAt = ref(t, "tmdb:888888", false, 2, 4), 4, &secondPlay
	account.Profiles[0].Entries = []Entry{movie, episode, next, home}
	if status := h.imported(t, alice, alice, account, Choice{Profile: "1"}); status.State != StateDone || status.UnmatchedCount != 0 {
		t.Fatalf("the next import: %+v", status)
	}
	check("movie, imported again", linkedMovie, func(d userdata.Data) bool { return d.Played && d.LastPlayed.Equal(secondPlay) })
	check("next episode", episodeAt(4), func(d userdata.Data) bool { return d.Played && d.LastPlayed.Equal(secondPlay) })
	check("another episode", episodeAt(5), func(d userdata.Data) bool { return !d.Played })
	check("another season", library.TitleRef{IMDb: "tt0000088", Episode: true, Season: 1, Number: 4},
		func(d userdata.Data) bool { return !d.Played })
	// Listed still with their links, none not found.
	kept = h.unfound(t, alice.ID, "nuvio")
	for _, r := range kept {
		if r.Link == nil || r.Episode == 4 {
			t.Errorf("after the next import: %+v", r)
		}
	}
	counts, err := h.links.Counts(t.Context(), alice.ID)
	if err != nil || len(counts) != 1 || counts[0] != (importlinks.Count{Source: "nuvio", NotFound: 0, Linked: 3}) {
		t.Errorf("counts: %+v %v", counts, err)
	}
}

// A connection's scheduled imports keep the titles they do not find in
// place of those the last import kept, and apply the links made of them.
func TestAConnectionKeepsTheTitlesNotFound(t *testing.T) {
	t.Parallel()
	h := newConnectionHarness(t)
	alice := h.user(t, "alice", false)
	unknown := func(id string) Entry {
		return Entry{Title: ref(t, id, true, 0, 0), Label: Label{ID: id, Name: "Unknown " + id, Kind: "movie"}, Watchlist: true}
	}
	account := Account{Profiles: []Profile{{ID: "1", Name: "Main", Entries: []Entry{unknown("tmdb:999999")}}}}
	h.source.set(func(f *fakeSource) { f.account, f.accepted["alice's"] = account, true })
	c := h.connected(t, alice, alice, account, SignIn{Secret: "alice's", Owned: true}, Choice{Profile: "1"})
	if c.LastImport.Unmatched != 1 {
		t.Fatalf("the first import: %+v", c.LastImport)
	}
	if kept := h.unfound(t, alice.ID, "stremio"); len(kept) != 1 || kept[0].Key != "tmdb:999999" ||
		kept[0].Watchlist != watchlist.Connection("stremio") {
		t.Fatalf("kept by the first import: %+v", kept)
	}

	// The app's library changed: the scheduled import keeps what it holds
	// now.
	played := at("2026-10-01T20:00:00Z")
	other := unknown("tmdb:999998")
	other.Played, other.PlayedAt = true, &played
	h.source.set(func(f *fakeSource) { f.account.Profiles[0].Entries = []Entry{other} })
	c = h.importedNow(t, alice)
	if c.LastImport.Unmatched != 1 || c.LastImport.Problem != "" {
		t.Fatalf("the scheduled import: %+v", c.LastImport)
	}
	kept := h.unfound(t, alice.ID, "stremio")
	if len(kept) != 1 || kept[0].Key != "tmdb:999998" || !kept[0].Played || kept[0].Watchlist != watchlist.Connection("stremio") {
		t.Fatalf("kept by the scheduled import: %+v", kept)
	}

	// Linked, it is played at once; played again in the app since, the
	// next scheduled import finds it through its link.
	if _, err := h.links.Link(t.Context(), alice.ID, alice.ID, "stremio", "tmdb:999998", "tt0000066"); err != nil {
		t.Fatal(err)
	}
	linked := library.TitleRef{IMDb: "tt0000066"}
	if d := h.dataOf(t, alice.ID, linked); !d.Played || !d.LastPlayed.Equal(played) {
		t.Errorf("the linked movie: %+v", d)
	}
	again := at("2026-10-05T20:00:00Z")
	h.source.set(func(f *fakeSource) { f.account.Profiles[0].Entries[0].PlayedAt = &again })
	c = h.importedNow(t, alice)
	if c.LastImport.Unmatched != 0 || c.LastImport.Problem != "" {
		t.Errorf("the import after the link: %+v", c.LastImport)
	}
	if d := h.dataOf(t, alice.ID, linked); !d.Played || !d.LastPlayed.Equal(again) {
		t.Errorf("the linked movie, imported again: %+v", d)
	}
	if kept := h.unfound(t, alice.ID, "stremio"); len(kept) != 1 || kept[0].Link == nil {
		t.Errorf("kept after the link: %+v", kept)
	}
}
