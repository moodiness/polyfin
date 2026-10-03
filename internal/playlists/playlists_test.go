package playlists

import (
	"errors"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/testdb"
)

func setup(t *testing.T) (*Store, *accounts.Store) {
	t.Helper()
	pool := testdb.New(t)
	if err := database.Migrate(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	users, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	return New(pool), users
}

func createUser(t *testing.T, users *accounts.Store, name string) accounts.User {
	t.Helper()
	user, err := users.CreateUser(t.Context(), accounts.NewUser{Name: name, Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	return user
}

// item returns a distinct title identifier.
func item(n byte) accounts.ID { return accounts.ID{15: n} }

func items(t *testing.T, store *Store, id accounts.ID) []accounts.ID {
	t.Helper()
	p, err := store.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	result := make([]accounts.ID, len(p.Entries))
	for i, entry := range p.Entries {
		result[i] = entry.Item
	}
	return result
}

func TestEntriesKeepTheirIdentityThroughChanges(t *testing.T) {
	store, users := setup(t)
	owner := createUser(t, users, "owner")
	ctx := t.Context()
	id, err := store.Create(ctx, Draft{Owner: owner.ID, Name: "Mix", Items: []accounts.ID{item(1), item(2), item(1)}})
	if err != nil {
		t.Fatal(err)
	}
	created, _ := store.Get(ctx, id)
	if created.Entries[0].ID == created.Entries[2].ID || created.LastAdded == nil {
		t.Fatalf("a title added twice shares one entry, or the addition is not dated: %+v", created)
	}

	if err := store.Add(ctx, id, []accounts.ID{item(3), item(4)}, 1); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(ctx, id, []accounts.ID{item(5)}, 99); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(ctx, id, []accounts.ID{item(6)}, -3); err != nil {
		t.Fatal(err)
	}
	want := []accounts.ID{item(6), item(1), item(3), item(4), item(2), item(1), item(5)}
	if got := items(t, store, id); !slices.Equal(got, want) {
		t.Fatalf("after insertions: %v, want %v", got, want)
	}

	// Moving the second copy of item 1 first, then removing the first copy,
	// keeps the moved one.
	p, _ := store.Get(ctx, id)
	first, second := p.Entries[1].ID, p.Entries[5].ID
	if err := store.Move(ctx, id, second, p.Entries[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Remove(ctx, id, []accounts.ID{first, item(9)}); err != nil {
		t.Fatal(err)
	}
	want = []accounts.ID{item(1), item(6), item(3), item(4), item(2), item(5)}
	if got := items(t, store, id); !slices.Equal(got, want) {
		t.Fatalf("after a move and a removal: %v, want %v", got, want)
	}
	if p, _ := store.Get(ctx, id); p.Entries[0].ID != second {
		t.Errorf("the moved entry lost its identifier")
	}

	// Moving before nothing puts the entry last; positions stay dense, so
	// a later insertion lands where asked.
	if err := store.Move(ctx, id, second, accounts.ID{}); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(ctx, id, []accounts.ID{item(7)}, 5); err != nil {
		t.Fatal(err)
	}
	want = []accounts.ID{item(6), item(3), item(4), item(2), item(5), item(7), item(1)}
	if got := items(t, store, id); !slices.Equal(got, want) {
		t.Fatalf("after moving last: %v, want %v", got, want)
	}

	if err := store.Update(ctx, id, Changes{Items: &[]accounts.ID{item(8)}}); err != nil {
		t.Fatal(err)
	}
	if got := items(t, store, id); !slices.Equal(got, []accounts.ID{item(8)}) {
		t.Errorf("replaced titles: %v", got)
	}
}

func TestSharesAndVisibility(t *testing.T) {
	store, users := setup(t)
	owner, guest, other := createUser(t, users, "owner"), createUser(t, users, "guest"), createUser(t, users, "other")
	ctx := t.Context()
	// The owner needs no share, an unknown user gets none, and a user named
	// twice keeps the last.
	id, err := store.Create(ctx, Draft{Owner: owner.ID, Name: "Mix", Shares: []Share{
		{User: owner.ID, CanEdit: true}, {User: item(1)}, {User: guest.ID, CanEdit: true}, {User: guest.ID},
	}})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := store.Get(ctx, id)
	if !slices.Equal(p.Shares, []Share{{User: guest.ID}}) {
		t.Fatalf("shares: %+v", p.Shares)
	}
	if !p.Visible(guest.ID) || p.Editable(guest.ID) || p.Visible(other.ID) || !p.Editable(owner.ID) {
		t.Errorf("a read-only share: %+v", p)
	}

	if err := store.SetShare(ctx, id, Share{User: other.ID, CanEdit: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetShare(ctx, id, Share{User: guest.ID, CanEdit: true}); err != nil {
		t.Fatal(err)
	}
	p, _ = store.Get(ctx, id)
	if !slices.Equal(p.Shares, []Share{{User: other.ID, CanEdit: true}, {User: guest.ID, CanEdit: true}}) {
		t.Errorf("a changed share does not come last: %+v", p.Shares)
	}

	stranger := createUser(t, users, "stranger")
	if lists, _ := store.Visible(ctx, stranger.ID); len(lists) != 0 {
		t.Errorf("a private playlist is visible to a stranger")
	}
	if err := store.Update(ctx, id, Changes{OpenAccess: new(true)}); err != nil {
		t.Fatal(err)
	}
	if visible, _ := store.AnyVisible(ctx, stranger.ID); !visible {
		t.Errorf("an open playlist is not visible to everyone")
	}

	if err := store.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted playlist: %v", err)
	}
	if err := store.Add(ctx, id, []accounts.ID{item(1)}, 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("adding to a deleted playlist: %v", err)
	}
}
