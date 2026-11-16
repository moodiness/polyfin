package userdata

import (
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// PlayedSeries lists the series as the user's played episodes come in
// Episodes, each at its first: the latest played first, those played
// without a date last, ties by episode; an episode only started or
// dated, or another user's, makes none.
func TestPlayedSeriesFollowTheEpisodes(t *testing.T) {
	t.Parallel()
	store, alice, bob := setup(t)
	at := func(hours int) *time.Time { return new(time.Date(2026, 1, 10, hours, 0, 0, 0, time.UTC)) }
	a, b, c, d, e, f := accounts.ID{0xa}, accounts.ID{0xb}, accounts.ID{0xc}, accounts.ID{0xd}, accounts.ID{0xe}, accounts.ID{0xf}
	set := func(user, episode, series accounts.ID, change func(*Data)) {
		t.Helper()
		if _, err := store.Change(t.Context(), user, []Item{{ID: episode, Series: series, Season: accounts.ID{0xff}}}, change); err != nil {
			t.Fatal(err)
		}
	}
	set(alice, accounts.ID{0x01}, a, func(d *Data) { d.Played, d.LastPlayed = true, at(1) })
	set(alice, accounts.ID{0x02}, a, func(d *Data) { d.Position, d.LastPlayed = time.Minute, at(5) })
	set(alice, accounts.ID{0x10}, b, func(d *Data) { d.Played, d.LastPlayed = true, at(3) })
	set(alice, accounts.ID{0x11}, b, func(d *Data) { d.Played = true })
	set(alice, accounts.ID{0x31}, c, func(d *Data) { d.Played = true })
	set(alice, accounts.ID{0x30}, d, func(d *Data) { d.Played = true })
	set(alice, accounts.ID{0x40}, e, func(d *Data) { d.LastPlayed = at(9) })
	set(alice, accounts.ID{0x50}, f, func(d *Data) { d.Played, d.LastPlayed = true, at(3) })
	set(bob, accounts.ID{0x60}, e, func(d *Data) { d.Played, d.LastPlayed = true, at(9) })

	episodes, err := store.Episodes(t.Context(), alice)
	if err != nil {
		t.Fatal(err)
	}
	var want []accounts.ID
	for _, entry := range episodes {
		if entry.Played && !slices.Contains(want, entry.Series) {
			want = append(want, entry.Series)
		}
	}
	got, err := store.PlayedSeries(t.Context(), alice)
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("played series: %v %v, want %v", got, err, want)
	}
	if !slices.Equal(want, []accounts.ID{b, f, a, d, c}) {
		t.Errorf("the episodes' order: %v", want)
	}
}
