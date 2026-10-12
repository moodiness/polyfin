package userdata

import (
	"maps"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

func setup(t *testing.T) (*Store, accounts.ID, accounts.ID) {
	t.Helper()
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
	return New(pool), ids[0], ids[1]
}

func TestDataIsKeptPerUser(t *testing.T) {
	store, alice, bob := setup(t)
	movie := accounts.ID{1}
	now := time.Date(2026, 10, 2, 20, 0, 0, 0, time.UTC)
	changed, err := store.Change(t.Context(), alice, []Item{{ID: movie}}, func(d *Data) {
		d.Start(now, true)
		d.Reach(time.Hour, 2*time.Hour, jellyfin)
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
		d.Reach(time.Minute*10, time.Hour, jellyfin)
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
			d.Start(at, true)
			d.Reach(time.Hour, 2*time.Hour, jellyfin)
		}); err != nil {
			t.Fatal(err)
		}
	}
	resume(accounts.ID{1}, start)
	resume(accounts.ID{2}, start.Add(time.Hour))
	// A position reported without a start has no date: it comes last.
	if _, err := store.Change(t.Context(), alice, []Item{{ID: accounts.ID{3}}}, func(d *Data) { d.Reach(time.Hour, 2*time.Hour, jellyfin) }); err != nil {
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

// What users did with items follows them to others as an import adds it:
// played marks are added, the newer resume point is kept, favorites are
// set, nothing is taken back, each user's apart.
func TestDataFollowsAsAnImport(t *testing.T) {
	store, alice, bob := setup(t)
	older := time.Date(2026, 9, 1, 20, 0, 0, 0, time.UTC)
	newer := older.Add(48 * time.Hour)
	played, resumed, favorite := accounts.ID{1}, accounts.ID{2}, accounts.ID{3}
	wasResumed, resumedLater, untouched, wasPlayed := accounts.ID{11}, accounts.ID{12}, accounts.ID{13}, accounts.ID{14}
	set := func(user, id accounts.ID, change func(*Data)) {
		t.Helper()
		if _, err := store.Change(t.Context(), user, []Item{{ID: id}}, change); err != nil {
			t.Fatal(err)
		}
	}
	set(alice, played, func(d *Data) { d.MarkPlayed(&newer, newer); d.PlayCount = 3 })
	set(alice, resumed, func(d *Data) { d.Position, d.Runtime, d.LastPlayed = 30*time.Minute, 2*time.Hour, &older })
	set(alice, favorite, func(d *Data) { d.Favorite = true })
	set(alice, wasResumed, func(d *Data) { d.Position, d.Runtime, d.LastPlayed = 10*time.Minute, 2*time.Hour, &older })
	set(alice, resumedLater, func(d *Data) { d.Position, d.Runtime, d.LastPlayed = time.Hour, 2*time.Hour, &newer })
	set(alice, wasPlayed, func(d *Data) { d.MarkPlayed(&newer, newer) })
	set(bob, favorite, func(d *Data) { d.Favorite = true })
	if err := store.Follow(t.Context(), []Move{
		{From: []accounts.ID{played}, To: []Item{{ID: wasResumed}}},
		{From: []accounts.ID{resumed}, To: []Item{{ID: resumedLater}, {ID: untouched}}},
		{From: []accounts.ID{favorite}, To: []Item{{ID: wasPlayed}}},
	}, jellyfin); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(t.Context(), alice, []accounts.ID{wasResumed, resumedLater, untouched, wasPlayed})
	if err != nil {
		t.Fatal(err)
	}
	if d := got[wasResumed]; !d.Played || d.PlayCount != 3 || d.Position != 0 {
		t.Errorf("played over a resume point: %+v", d)
	}
	if d := got[resumedLater]; d.Played || d.Position != time.Hour {
		t.Errorf("an older resume point over a newer one: %+v", d)
	}
	if d := got[untouched]; d.Played || d.Position != 30*time.Minute || d.LastPlayed == nil || !d.LastPlayed.Equal(older) {
		t.Errorf("a resume point: %+v", d)
	}
	if d := got[wasPlayed]; !d.Played || !d.Favorite {
		t.Errorf("a favorite over a played mark: %+v", d)
	}
	theirs, err := store.Get(t.Context(), bob, []accounts.ID{wasResumed, resumedLater, untouched, wasPlayed})
	if err != nil || len(theirs) != 1 || !theirs[wasPlayed].Favorite || theirs[wasPlayed].Played {
		t.Errorf("another user's: %+v %v", theirs, err)
	}
}
