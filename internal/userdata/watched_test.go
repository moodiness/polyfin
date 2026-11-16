package userdata

import (
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// WatchedSeries lists the series as the user's dated episodes, latest
// first, name them first, ties by episode, the latest of each; an
// episode played without a date, or another user's, makes none.
// SeriesEpisodes lists the entries Episodes lists of the series asked.
func TestWatchedSeriesFollowTheEpisodes(t *testing.T) {
	t.Parallel()
	store, alice, bob := setup(t)
	at := func(hours int) *time.Time { return new(time.Date(2026, 1, 10, hours, 0, 0, 0, time.UTC)) }
	a, b, c, d := accounts.ID{0xa}, accounts.ID{0xb}, accounts.ID{0xc}, accounts.ID{0xd}
	set := func(user, episode, series accounts.ID, change func(*Data)) {
		t.Helper()
		if _, err := store.Change(t.Context(), user, []Item{{ID: episode, Series: series, Season: accounts.ID{0xff}}}, change); err != nil {
			t.Fatal(err)
		}
	}
	set(alice, accounts.ID{0x01}, a, func(d *Data) { d.Played, d.LastPlayed = true, at(1) })
	set(alice, accounts.ID{0x20}, a, func(d *Data) { d.Position, d.LastPlayed = time.Minute, at(3) })
	set(alice, accounts.ID{0x10}, b, func(d *Data) { d.Played, d.LastPlayed = true, at(3) })
	set(alice, accounts.ID{0x11}, b, func(d *Data) { d.Played = true })
	set(alice, accounts.ID{0x30}, c, func(d *Data) { d.Played = true })
	set(alice, accounts.ID{0x40}, d, func(d *Data) { d.LastPlayed = at(2) })
	set(bob, accounts.ID{0x50}, c, func(d *Data) { d.Played, d.LastPlayed = true, at(9) })

	episodes, err := store.Episodes(t.Context(), alice)
	if err != nil {
		t.Fatal(err)
	}
	var want []Watched
	for _, e := range episodes {
		if e.LastPlayed != nil && !slices.ContainsFunc(want, func(w Watched) bool { return w.Series == e.Series }) {
			want = append(want, Watched{Series: e.Series, Latest: *e.LastPlayed})
		}
	}
	got, err := store.WatchedSeries(t.Context(), alice)
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("watched series: %v %v, want %v", got, err, want)
	}
	if len(want) != 3 || want[0].Series != b || want[1].Series != a || want[2].Series != d {
		t.Errorf("the episodes' order: %v", want)
	}

	for _, series := range [][]accounts.ID{{a}, {b, c}, {a, b, c, d}} {
		got, err := store.SeriesEpisodes(t.Context(), alice, series)
		wanted := slices.DeleteFunc(slices.Clone(episodes), func(e Entry) bool { return !slices.Contains(series, e.Series) })
		if err != nil || !slices.EqualFunc(got, wanted, func(x, y Entry) bool {
			return x.Item == y.Item && x.Series == y.Series && x.Played == y.Played && x.Position == y.Position
		}) {
			t.Errorf("the episodes of %v: %+v %v, want %+v", series, got, err, wanted)
		}
	}
}
