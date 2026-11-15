package userdata

import (
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

func TestDroppedHoldsUntilALaterPlay(t *testing.T) {
	t.Parallel()
	pool := migrated.New(t)
	users, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	var ids [2]accounts.ID
	for i, name := range []string{"alice", "bob"} {
		user, err := users.CreateUser(t.Context(), accounts.NewUser{Name: name, Password: "correct horse"})
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = user.ID
	}
	alice, bob := ids[0], ids[1]
	store := New(pool)
	day := func(d int) time.Time { return time.Date(2026, 10, d, 20, 0, 0, 0, time.UTC) }
	drop := func(user accounts.ID, service string, series accounts.ID, at *time.Time, seen time.Time) {
		t.Helper()
		if _, err := pool.Exec(t.Context(), `INSERT INTO tracker_dropped (user_id, service, series_id, dropped_at, seen_at)
			VALUES ($1, $2, $3, $4, $5)`, user, service, series, at, seen); err != nil {
			t.Fatal(err)
		}
	}
	play := func(user, series, episode accounts.ID, at time.Time, played bool) {
		t.Helper()
		if _, err := pool.Exec(t.Context(), `INSERT INTO user_data (user_id, item_id, series_id, played, position_ticks, last_played_at)
			VALUES ($1, $2, $3, $4, $5, $6)`, user, episode, series, played, map[bool]int64{true: 0, false: 1e9}[played], at); err != nil {
			t.Fatal(err)
		}
	}
	held, laterPlayed, laterStarted, olderPlay, unknownDate, twoServices := accounts.ID{1}, accounts.ID{2}, accounts.ID{3}, accounts.ID{4}, accounts.ID{5}, accounts.ID{6}
	at := func(d int) *time.Time { t := day(d); return &t }

	drop(alice, "simkl", held, at(3), day(20))
	drop(alice, "simkl", laterPlayed, at(5), day(20))
	play(alice, laterPlayed, accounts.ID{21}, day(6), true)
	drop(alice, "trakt", laterStarted, at(5), day(20))
	play(alice, laterStarted, accounts.ID{31}, day(6), false)
	drop(alice, "mdblist", olderPlay, at(7), day(20))
	play(alice, olderPlay, accounts.ID{41}, day(6), true)
	// Without a date, the drop counts from when an import first saw it:
	// a play before that keeps it.
	drop(alice, "anilist", unknownDate, nil, day(9))
	play(alice, unknownDate, accounts.ID{51}, day(8), true)
	// Two services: the later date counts, and a play between keeps it.
	drop(alice, "simkl", twoServices, at(10), day(20))
	drop(alice, "trakt", twoServices, at(12), day(20))
	play(alice, twoServices, accounts.ID{61}, day(11), true)
	// Bob's drops are his own.
	drop(bob, "simkl", laterPlayed, at(1), day(1))

	want := []Dropped{
		{twoServices, "simkl", day(10)}, {twoServices, "trakt", day(12)},
		{unknownDate, "anilist", day(9)},
		{olderPlay, "mdblist", day(7)},
		{held, "simkl", day(3)},
	}
	got, err := store.Dropped(t.Context(), alice)
	if err != nil {
		t.Fatal(err)
	}
	// Each row carries its own service's date; the series come by their
	// latest one.
	if !slices.Equal(got, want) {
		t.Fatalf("alice's dropped series:\n got %+v\nwant %+v", got, want)
	}
	set, err := store.DroppedSeries(t.Context(), alice)
	if err != nil || len(set) != 4 || !set[held] || !set[olderPlay] || !set[unknownDate] || !set[twoServices] {
		t.Errorf("alice's dropped set: %v %v", set, err)
	}

	// A play after the later of the two dates lifts it.
	play(alice, twoServices, accounts.ID{62}, day(13), true)
	if set, err := store.DroppedSeries(t.Context(), alice); err != nil || set[twoServices] || len(set) != 3 {
		t.Errorf("after a play past both dates: %v %v", set, err)
	}
	if set, err := store.DroppedSeries(t.Context(), bob); err != nil || len(set) != 1 || !set[laterPlayed] {
		t.Errorf("bob's dropped set: %v %v", set, err)
	}
}
