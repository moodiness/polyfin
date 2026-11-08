package jellyfinimport

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/importlinks"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
)

// An import keeps the titles it did not find in the user's list of the
// server's kind, with what it would have brought of them: a title without
// identifiers by its identifier on the server, an episode by its series',
// one entry for each episode of a file holding several. Linked by hand, a
// title gets at once what was kept of it, and every later import finds it
// as its link names it, identifiers or not.
func TestTitlesNotFoundAreKeptAndLinkedByHand(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	alice := h.user(t, "alice")
	played := func(date string, count int) map[string]any {
		return map[string]any{"Played": true, "PlayCount": count, "LastPlayedDate": date}
	}
	// The series is known by no identifier.
	h.f.series = map[string]map[string]string{"s-none": {}}
	holiday := movie("m3", "Holiday", map[string]string{}, played("2026-09-03T20:00:00.0000000Z", 2))
	holiday["UserData"].(map[string]any)["Rating"] = 8.0
	pair := episode("e9", "s-none", 1, new(2), played("2026-09-04T20:00:00.0000000Z", 1))
	pair["IndexNumberEnd"] = 3
	h.f.items["jf-alice"] = map[string][]map[string]any{
		"IsPlayed": {holiday, movie("m4", "Unknown", map[string]string{"Tmdb": "999999"}, played("2026-09-05T20:00:00.0000000Z", 1)), pair},
		"IsResumable": {movie("r9", "Paused", map[string]string{},
			map[string]any{"PlaybackPositionTicks": int64(30 * time.Minute / tick), "LastPlayedDate": "2026-09-06T20:00:00.0000000Z"})},
	}
	start := func() UserStatus {
		t.Helper()
		if _, err := h.Start(t.Context(), Connection{Kind: Jellyfin, Address: h.f.url, Credentials: Credentials{Key: goodKey},
			Extras: Extras{Ratings: true}}, accounts.User{}, func(Server, []User, SignIn) ([]Target, error) {
			return []Target{{JellyfinID: "jf-alice", User: alice.ID, UserName: "alice"}}, nil
		}); err != nil {
			t.Fatal(err)
		}
		status := h.ended(t)
		if status.State != StateDone {
			t.Fatalf("import: %+v", status)
		}
		return status.Users[0]
	}

	if got := start(); got.UnmatchedCount != 4 {
		t.Errorf("counts: %+v", got)
	}
	kept := map[string]importlinks.Row{}
	for _, row := range h.kept(t, alice.ID, Jellyfin) {
		name := row.Name
		if row.Kind == importlinks.KindEpisode {
			name = fmt.Sprintf("%s %d", name, row.Episode)
		}
		kept[name] = row
	}
	if len(kept) != 5 {
		t.Fatalf("kept: %+v", kept)
	}
	h1, unknown, paused := kept["Holiday"], kept["Unknown"], kept["Paused"]
	second, third := kept["Series s-none 2"], kept["Series s-none 3"]
	if !strings.HasPrefix(h1.Key, "movie:item:jellyfin:") || !strings.HasSuffix(h1.Key, ":m3") || h1.Kind != importlinks.KindMovie ||
		h1.Year != 2019 || h1.IDs != "" || h1.Reason != ReasonNoIdentifier || !h1.Played || h1.Plays != 2 ||
		!h1.PlayedAt.Equal(at("2026-09-03T20:00:00Z")) || h1.Rating == nil || *h1.Rating != 8 {
		t.Errorf("Holiday: %+v", h1)
	}
	if unknown.Key != "movie:tmdb:999999" || unknown.IDs != "tmdb:999999" || unknown.Reason != ReasonNotFound || !unknown.Played {
		t.Errorf("Unknown: %+v", unknown)
	}
	if paused.Played || paused.Resume == nil || paused.Resume.Position != 30*time.Minute || !paused.Resume.At.Equal(at("2026-09-06T20:00:00Z")) {
		t.Errorf("Paused: %+v", paused)
	}
	if !strings.HasPrefix(second.Key, "series:item:jellyfin:") || !strings.HasSuffix(second.Key, ":s-none") || second.Key != third.Key ||
		second.Season != 1 || second.Reason != ReasonNoIdentifier || !second.Played || !third.Played ||
		!third.PlayedAt.Equal(at("2026-09-04T20:00:00Z")) {
		t.Errorf("episodes: %+v %+v", second, third)
	}

	// Linked, the movie and the series' episodes get what was kept of
	// them at once.
	movieRef := library.TitleRef{IMDb: "tt0000042"}
	episodeRef := func(number int) library.TitleRef {
		return library.TitleRef{Episode: true, IMDb: "tt0100", Season: 1, Number: number}
	}
	if _, err := h.links.Link(t.Context(), alice.ID, alice.ID, string(Jellyfin), h1.Key, "tt0000042"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.links.Link(t.Context(), alice.ID, alice.ID, string(Jellyfin), second.Key, "https://www.imdb.com/title/tt0100/"); err != nil {
		t.Fatal(err)
	}
	check := func(name string, ref library.TitleRef, want func(userdata.Data) bool) {
		t.Helper()
		if d := h.dataOf(t, alice.ID, ref); !want(d) {
			t.Errorf("%s: %+v", name, d)
		}
	}
	check("linked movie", movieRef, func(d userdata.Data) bool {
		return d.Played && d.PlayCount == 2 && d.LastPlayed.Equal(at("2026-09-03T20:00:00Z")) && d.Rating != nil && *d.Rating == 8
	})
	check("linked episode", episodeRef(2), func(d userdata.Data) bool { return d.Played })
	check("linked episode of the same file", episodeRef(3), func(d userdata.Data) bool { return d.Played })

	// Played again since, and a new episode of the series: the next
	// import finds them through their links, though they have no
	// identifier, and no longer counts them as not found.
	holiday["UserData"] = played("2026-10-01T20:00:00.0000000Z", 3)
	h.f.items["jf-alice"]["IsPlayed"] = append(h.f.items["jf-alice"]["IsPlayed"],
		episode("e10", "s-none", 1, new(4), played("2026-10-02T20:00:00.0000000Z", 1)))
	// The movie was played already: only the new episode counts.
	if got := start(); got.UnmatchedCount != 2 || got.Played != 1 {
		t.Errorf("again: %+v", got)
	}
	check("linked movie, again", movieRef, func(d userdata.Data) bool {
		return d.PlayCount == 3 && d.LastPlayed.Equal(at("2026-10-01T20:00:00Z"))
	})
	check("new episode", episodeRef(4), func(d userdata.Data) bool { return d.Played })
	// The linked entries stay listed with their links.
	linked := 0
	for _, row := range h.kept(t, alice.ID, Jellyfin) {
		if row.Link != nil {
			linked++
		}
	}
	if linked != 3 {
		t.Errorf("linked entries: %d", linked)
	}
}
