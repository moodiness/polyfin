package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/userdata"
)

// airingShowsAddon is an addon of count series "Show N", each of an
// episode released in 2020 and one airing in 2099, Show N's N hours after
// Show 0's, so that Upcoming lists them by number.
func airingShowsAddon(t *testing.T, count int) string {
	t.Helper()
	show := func(i int, videos bool) stremio.Meta {
		meta := stremio.Meta{ID: fmt.Sprintf("tt8%03d", i), Type: "series", Name: fmt.Sprintf("Show %d", i)}
		if videos {
			airing := time.Date(2099, 1, 1, i, 0, 0, 0, time.UTC).Format(time.RFC3339)
			meta.Videos = []stremio.Video{{ID: meta.ID + ":1:1", Title: "Episode 1", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00Z"},
				{ID: meta.ID + ":1:2", Title: "Episode 2", Season: 1, Episode: 2, Released: airing}}
		}
		return meta
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "airing", Name: "Airing", Version: "1", Types: []string{"series"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs:  []stremio.Catalog{{Type: "series", ID: "shows", Name: "Shows"}}})
		case strings.HasPrefix(path, "/catalog/series/shows"):
			metas := make([]stremio.Meta, count)
			for i := range metas {
				metas[i] = show(i, false)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		case strings.HasPrefix(path, "/meta/series/tt8"):
			i, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(path, "/meta/series/tt8"), ".json"))
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": show(i, true)})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// newAiringShows serves count series from airingShowsAddon.
func newAiringShows(t *testing.T, count int) shows {
	t.Helper()
	s := newTestServer(t, 10)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), airingShowsAddon(t, count), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "shows"}}); err != nil {
		t.Fatal(err)
	}
	return listedShows(t, s, count, func(token string) string { return s.viewIDs(t, token)["Shows"] })
}

// Upcoming looks at the 50 series of which the user played an episode,
// the latest played first, those played without a date last, ties by
// episode; a series only started is none, nor is a hidden series or one
// dropped on a tracking service, which do not count in the 50. Favorite
// series come on top of them.
func TestUpcomingLooksAtTheFiftySeriesPlayedLast(t *testing.T) {
	t.Parallel()
	const count = 56
	x := newAiringShows(t, count)
	name := func(i int) string { return fmt.Sprintf("Show %d", i) }
	base := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	at := func(hours int) time.Time { return base.Add(time.Duration(hours) * time.Hour) }
	// Shows 0 to 49 and 51 played, Show i at hour i, 51 last; 52 and 53
	// played without a date; 54 only started, last of all; 55 a
	// favorite. Show 49 is hidden, Show 48 dropped.
	for i := range 52 {
		if i != 50 {
			x.played(t, name(i), "S1E1", at(i))
		}
	}
	undated := []int{52, 53}
	for _, i := range undated {
		x.set(t, name(i), "S1E1", func(d *userdata.Data) { d.Played, d.PlayCount = true, 1 })
	}
	x.started(t, name(54), "S1E1", at(100))
	// An episode played earlier of a show played later counts as its
	// latest play.
	x.played(t, name(0), "S1E2", at(60))
	if _, err := x.handler.Options.UserData.Change(t.Context(), x.member.ID, []userdata.Item{{ID: mustID(t, x.series[name(55)])}},
		func(d *userdata.Data) { d.Favorite = true }); err != nil {
		t.Fatal(err)
	}
	if status, body := x.call(http.MethodPost, "/Polyfin/UserItems/"+x.series[name(49)]+"/Hidden", app("tv", x.token), nil); status/100 != 2 {
		t.Fatalf("hide: %d %s", status, body)
	}
	x.drop(t, x.member.ID.String(), "trakt", x.series[name(48)], at(200))

	// Dated: 0 (hour 60), 51, then 47 down to 1: 49 series. The 50th is
	// the undated one whose played episode comes first.
	first := undated[0]
	if a, b := x.episode(t, name(undated[0]), "S1E1").Id, x.episode(t, name(undated[1]), "S1E1").Id; b < a {
		first = undated[1]
	}
	var want []string
	for i := range count {
		if i <= 47 || i == 51 || i == first || i == 55 {
			want = append(want, name(i)+" S1E2")
		}
	}
	if got, _ := x.list(t, "/Shows/Upcoming"); !slices.Equal(got, want) {
		t.Errorf("upcoming: %q\nwant %q", got, want)
	}
	if got, _ := x.list(t, "/Shows/Upcoming?startIndex=48&limit=5"); !slices.Equal(got, want[48:]) {
		t.Errorf("upcoming from 48: %q, want %q", got, want[48:])
	}
}

// My account's Next Up lists the series taken off it until an episode is
// played or started after the removal: a play or a start before it, or a
// play without a date, keeps it off.
func TestNextUpRemovalsLiftByALaterPlayOrStart(t *testing.T) {
	t.Parallel()
	x := newAiringShows(t, 5)
	name := func(i int) string { return fmt.Sprintf("Show %d", i) }
	removed := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	for i := range 5 {
		x.played(t, name(i), "S1E1", removed.Add(-time.Duration(i+1)*time.Hour))
		if err := x.handler.Options.UserData.RemoveFromNextUp(t.Context(), x.member.ID, mustID(t, x.series[name(i)]), removed.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	x.played(t, name(1), "S1E2", removed.Add(time.Hour))
	x.started(t, name(2), "S1E2", removed.Add(time.Hour))
	x.started(t, name(3), "S1E2", removed.Add(-time.Hour))
	x.set(t, name(4), "S1E2", func(d *userdata.Data) { d.Played, d.PlayCount = true, 1 })
	removals, err := x.handler.NextUpRemovals(t.Context(), x.member)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, removal := range removals {
		got = append(got, removal.Item.Name)
	}
	if want := []string{name(4), name(3), name(0)}; !slices.Equal(got, want) {
		t.Errorf("still off Next Up: %q, want %q", got, want)
	}
}
