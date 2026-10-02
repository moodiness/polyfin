package userdata

import (
	"maps"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/testdb"
)

func setup(t *testing.T) (*Store, accounts.ID, accounts.ID) {
	t.Helper()
	pool := testdb.New(t)
	if err := database.Migrate(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
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
	return New(pool), ids[0], ids[1]
}

func TestDataIsKeptPerUser(t *testing.T) {
	store, alice, bob := setup(t)
	movie := accounts.ID{1}
	now := time.Date(2026, 10, 2, 20, 0, 0, 0, time.UTC)
	changed, err := store.Change(t.Context(), alice, []Item{{ID: movie}}, func(d *Data) {
		d.Start(now)
		d.Reach(time.Hour, 2*time.Hour)
		d.Favorite = true
		d.Like(true)
	})
	if err != nil || len(changed) != 1 || changed[0].Position != time.Hour {
		t.Fatalf("change: %+v %v", changed, err)
	}
	stored, err := store.Get(t.Context(), alice, []accounts.ID{movie, {2}})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := stored[movie]
	if !ok || got.PlayCount != 1 || got.Position != time.Hour || got.Runtime != 2*time.Hour || !got.Favorite ||
		got.Rating == nil || *got.Rating != 10 || got.LastPlayed == nil || !got.LastPlayed.Equal(now) {
		t.Errorf("stored: %+v", got)
	}
	if _, ok := stored[accounts.ID{2}]; ok {
		t.Error("an untouched item has data")
	}
	if others, _ := store.Get(t.Context(), bob, []accounts.ID{movie}); len(others) != 0 {
		t.Errorf("another user sees %+v", others)
	}
}

func TestSeriesAndSeasonsCountTheirEpisodes(t *testing.T) {
	store, alice, _ := setup(t)
	series, first, second := accounts.ID{10}, accounts.ID{11}, accounts.ID{12}
	episodes := []Item{
		{ID: accounts.ID{1}, Series: series, Season: first},
		{ID: accounts.ID{2}, Series: series, Season: first},
		{ID: accounts.ID{3}, Series: series, Season: second},
	}
	if _, err := store.Change(t.Context(), alice, episodes, func(d *Data) { d.Played = true }); err != nil {
		t.Fatal(err)
	}
	// A started episode is not played, but can be resumed.
	if _, err := store.Change(t.Context(), alice, []Item{{ID: accounts.ID{4}, Series: series, Season: second}}, func(d *Data) {
		d.Reach(time.Minute*10, time.Hour)
	}); err != nil {
		t.Fatal(err)
	}
	counts, err := store.EpisodeCounts(t.Context(), alice, []accounts.ID{series, first, second, {99}})
	if err != nil {
		t.Fatal(err)
	}
	want := map[accounts.ID]Counts{series: {Played: 3, Resumable: 1}, first: {Played: 2}, second: {Played: 1, Resumable: 1}}
	if !maps.Equal(counts, want) {
		t.Errorf("counts: %v", counts)
	}
}

func TestListsFollowWhatUsersDid(t *testing.T) {
	store, alice, _ := setup(t)
	start := time.Date(2026, 10, 2, 20, 0, 0, 0, time.UTC)
	resume := func(id accounts.ID, at time.Time) {
		t.Helper()
		if _, err := store.Change(t.Context(), alice, []Item{{ID: id}}, func(d *Data) {
			d.Start(at)
			d.Reach(time.Hour, 2*time.Hour)
		}); err != nil {
			t.Fatal(err)
		}
	}
	resume(accounts.ID{1}, start)
	resume(accounts.ID{2}, start.Add(time.Hour))
	// A position reported without a start has no date: it comes last.
	if _, err := store.Change(t.Context(), alice, []Item{{ID: accounts.ID{3}}}, func(d *Data) { d.Reach(time.Hour, 2*time.Hour) }); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Change(t.Context(), alice, []Item{{ID: accounts.ID{4}}}, func(d *Data) { d.Favorite = true; d.Played = true }); err != nil {
		t.Fatal(err)
	}
	resumable, err := store.Resumable(t.Context(), alice)
	if err != nil {
		t.Fatal(err)
	}
	if ids := entryIDs(resumable); len(ids) != 3 || ids[0] != (accounts.ID{2}) || ids[1] != (accounts.ID{1}) || ids[2] != (accounts.ID{3}) {
		t.Errorf("resumable: %v", ids)
	}
	if favorites, err := store.Favorites(t.Context(), alice); err != nil || len(favorites) != 1 || favorites[0].Item != (accounts.ID{4}) {
		t.Errorf("favorites: %v %v", entryIDs(favorites), err)
	}
	if played, err := store.Played(t.Context(), alice); err != nil || len(played) != 1 || played[0].Item != (accounts.ID{4}) {
		t.Errorf("played: %v %v", entryIDs(played), err)
	}
}

func TestConcurrentChangesAreNotLost(t *testing.T) {
	store, alice, _ := setup(t)
	item := []Item{{ID: accounts.ID{1}}}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := store.Change(t.Context(), alice, item, func(d *Data) { d.PlayCount++ }); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if stored, _ := store.Get(t.Context(), alice, []accounts.ID{{1}}); stored[accounts.ID{1}].PlayCount != 8 {
		t.Errorf("play count: %d", stored[accounts.ID{1}].PlayCount)
	}
}

func entryIDs(entries []Entry) []accounts.ID {
	ids := make([]accounts.ID, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.Item)
	}
	return ids
}
