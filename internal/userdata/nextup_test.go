package userdata

import (
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

func TestNextUpRemovalsArePerUserAndGoWithTheUser(t *testing.T) {
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
	first, second := accounts.ID{1}, accounts.ID{2}
	early := time.Date(2026, 10, 1, 20, 0, 0, 0, time.UTC)
	late := early.Add(time.Hour)
	for _, removal := range []struct {
		user, series accounts.ID
		at           time.Time
	}{{alice, first, early}, {alice, second, early}, {bob, first, early}, {alice, first, late.Add(time.Hour)}} {
		if err := store.RemoveFromNextUp(t.Context(), removal.user, removal.series, removal.at); err != nil {
			t.Fatal(err)
		}
	}
	// Taken off again, a series counts from the latest removal.
	got, err := store.NextUpRemovals(t.Context(), alice)
	if want := []NextUpRemoval{{first, late.Add(time.Hour)}, {second, early}}; err != nil || !slices.Equal(got, want) {
		t.Fatalf("alice's removals: %+v %v, want %+v", got, err, want)
	}
	if removed, err := store.PutBackOnNextUp(t.Context(), alice, second); err != nil || !removed {
		t.Errorf("putting back: %v %v", removed, err)
	}
	if removed, err := store.PutBackOnNextUp(t.Context(), alice, second); err != nil || removed {
		t.Errorf("putting back again: %v %v", removed, err)
	}
	if got, err := store.NextUpRemovals(t.Context(), alice); err != nil || len(got) != 1 || got[0].Series != first {
		t.Errorf("after putting back: %+v %v", got, err)
	}

	// A user deleted takes their removals along, and theirs only.
	if err := users.DeleteUser(t.Context(), alice); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM next_up_removed WHERE user_id = $1", alice).Scan(&left); err != nil || left != 0 {
		t.Errorf("alice's removals after deleting her: %d %v", left, err)
	}
	if got, err := store.NextUpRemovals(t.Context(), bob); err != nil || len(got) != 1 || got[0].Series != first {
		t.Errorf("bob's removals: %+v %v", got, err)
	}
}
