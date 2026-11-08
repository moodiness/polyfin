package userdata

import (
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// inTransaction returns a store whose changes run in a transaction of
// their own, rolled back when the test ends, with the count of the row
// versions of table updated so far in it. The count starts from what the
// connection's earlier transactions left unflushed.
func inTransaction(t *testing.T, store *Store, table string) (*Store, func() int64) {
	t.Helper()
	tx, err := store.db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(t.Context()) })
	count := func() int64 {
		t.Helper()
		var updated int64
		if err := tx.QueryRow(t.Context(), "SELECT n_tup_upd FROM pg_stat_xact_user_tables WHERE relname = $1", table).
			Scan(&updated); err != nil {
			t.Fatal(err)
		}
		return updated
	}
	before := count()
	return &Store{db: tx}, func() int64 { return count() - before }
}

// marks gives n items, movies and episodes, every value set or not.
func marks(n int) ([]Item, []Data) {
	items, data := make([]Item, n), make([]Data, n)
	at := time.Date(2026, 9, 1, 20, 0, 0, 0, time.UTC)
	for i := range n {
		items[i] = Item{ID: accounts.ID{byte(i >> 8), byte(i), 1}}
		if i%3 > 0 {
			items[i].Series, items[i].Season = accounts.ID{byte(i % 7), 2}, accounts.ID{byte(i % 11), 3}
		}
		d := Data{Played: i%2 == 0, PlayCount: i % 4, Favorite: i%5 == 0}
		if i%3 == 0 {
			d.Position, d.Runtime = time.Duration(i)*time.Second, time.Hour
		}
		if i%4 > 0 {
			d.Rating = new(float64(i % 11))
		}
		if i%6 > 0 {
			d.LastPlayed = new(at.Add(time.Duration(i) * time.Minute))
		}
		if i%9 == 0 {
			d.ResumeRemoved = new(at.Add(-time.Duration(i) * time.Minute))
		}
		data[i] = d
	}
	return items, data
}

// Imports and app connections re-apply whole histories: marks that did
// not change write no row, as the database audit measured with 1,598.
func TestReapplyingUnchangedMarksWritesNoRow(t *testing.T) {
	t.Parallel()
	store, alice, _ := setup(t)
	items, data := marks(1598)
	set := func(store *Store, items []Item) []Data {
		t.Helper()
		changed, err := store.ChangeEach(t.Context(), alice, items, func(item Item, d *Data) {
			*d = data[int(item.ID[0])<<8|int(item.ID[1])]
		})
		if err != nil {
			t.Fatal(err)
		}
		return changed
	}
	set(store, items)
	tx, updated := inTransaction(t, store, "user_data")
	if again := set(tx, items); len(again) != len(data) || again[7].PlayCount != data[7].PlayCount {
		t.Fatalf("re-applied: %d", len(again))
	}
	// A playback report names no series for a movie, nor the episodes'
	// series when it does not know them.
	bare := make([]Item, len(items))
	for i, item := range items {
		bare[i] = Item{ID: item.ID}
	}
	set(tx, bare)
	if n := updated(); n != 0 {
		t.Errorf("re-applying %d unchanged marks updated %d rows", len(items), n)
	}
}

// Every change is still written: a value, a value set, a value taken
// away, a series or season learnt.
func TestChangedMarksAreWritten(t *testing.T) {
	t.Parallel()
	store, alice, _ := setup(t)
	movie, episode := accounts.ID{1}, accounts.ID{2}
	series, season, otherSeason := accounts.ID{10}, accounts.ID{11}, accounts.ID{12}
	if _, err := store.ChangeEach(t.Context(), alice, []Item{{ID: movie}, {ID: episode}}, func(Item, *Data) {}); err != nil {
		t.Fatal(err)
	}
	tx, updated := inTransaction(t, store, "user_data")
	at := time.Date(2026, 9, 1, 20, 0, 0, 0, time.UTC)
	steps := []struct {
		name   string
		item   Item
		change func(*Data)
		check  func(Entry) bool
	}{
		{"played", Item{ID: movie}, func(d *Data) { d.Played, d.PlayCount = true, 1 },
			func(e Entry) bool { return e.Played && e.PlayCount == 1 }},
		{"position", Item{ID: movie}, func(d *Data) { d.Position, d.Runtime = time.Minute, time.Hour },
			func(e Entry) bool { return e.Position == time.Minute && e.Runtime == time.Hour }},
		{"favorite", Item{ID: movie}, func(d *Data) { d.Favorite = true }, func(e Entry) bool { return e.Favorite }},
		{"rating set", Item{ID: movie}, func(d *Data) { d.Rating = new(10.0) },
			func(e Entry) bool { return e.Rating != nil && *e.Rating == 10 }},
		{"rating changed", Item{ID: movie}, func(d *Data) { d.Rating = new(1.0) },
			func(e Entry) bool { return e.Rating != nil && *e.Rating == 1 }},
		{"rating taken away", Item{ID: movie}, func(d *Data) { d.Rating = nil }, func(e Entry) bool { return e.Rating == nil }},
		{"last played set", Item{ID: movie}, func(d *Data) { d.LastPlayed = new(at) },
			func(e Entry) bool { return e.LastPlayed != nil && e.LastPlayed.Equal(at) }},
		{"last played taken away", Item{ID: movie}, func(d *Data) { d.LastPlayed = nil },
			func(e Entry) bool { return e.LastPlayed == nil }},
		{"resume removed set", Item{ID: movie}, func(d *Data) { d.ResumeRemoved = new(at) },
			func(e Entry) bool { return e.ResumeRemoved != nil && e.ResumeRemoved.Equal(at) }},
		{"resume removed taken away", Item{ID: movie}, func(d *Data) { d.ResumeRemoved = nil },
			func(e Entry) bool { return e.ResumeRemoved == nil }},
		{"series learnt", Item{ID: episode, Series: series}, func(*Data) {},
			func(e Entry) bool { return e.Series == series && e.Season == (accounts.ID{}) }},
		{"season learnt", Item{ID: episode, Series: series, Season: season}, func(*Data) {},
			func(e Entry) bool { return e.Series == series && e.Season == season }},
		{"season changed", Item{ID: episode, Season: otherSeason}, func(*Data) {},
			func(e Entry) bool { return e.Series == series && e.Season == otherSeason }},
	}
	for _, step := range steps {
		before := updated()
		if _, err := tx.ChangeEach(t.Context(), alice, []Item{step.item}, func(_ Item, d *Data) { step.change(d) }); err != nil {
			t.Fatal(err)
		}
		if n := updated() - before; n != 1 {
			t.Errorf("%s: %d rows updated", step.name, n)
		}
		entries, err := tx.entries(t.Context(), "SELECT "+columns+" FROM user_data WHERE user_id = $1 AND item_id = $2", alice, step.item.ID)
		if err != nil || len(entries) != 1 || !step.check(entries[0]) {
			t.Errorf("%s: stored %+v %v", step.name, entries, err)
		}
	}
}

// Taking a series off Next Up again at the same time writes nothing.
func TestRemovingFromNextUpAgainWritesNothingNew(t *testing.T) {
	t.Parallel()
	store, alice, _ := setup(t)
	series, at := accounts.ID{10}, time.Date(2026, 9, 1, 20, 0, 0, 0, time.UTC)
	tx, updated := inTransaction(t, store, "next_up_removed")
	for range 2 {
		if err := tx.RemoveFromNextUp(t.Context(), alice, series, at); err != nil {
			t.Fatal(err)
		}
	}
	if n := updated(); n != 0 {
		t.Errorf("the same removal updated %d rows", n)
	}
	if err := tx.RemoveFromNextUp(t.Context(), alice, series, at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n := updated(); n != 1 {
		t.Errorf("a later removal updated %d rows", n)
	}
}
