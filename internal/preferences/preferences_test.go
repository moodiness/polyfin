package preferences

import (
	"encoding/json"
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

func get(t *testing.T, store *Store, user accounts.ID, id, client string) (map[string]any, bool) {
	t.Helper()
	raw, ok, err := store.Get(t.Context(), user, id, client)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		return nil, false
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("%v in %s", err, raw)
	}
	return value, true
}

func TestPreferencesAreKeptPerUserIDAndClient(t *testing.T) {
	store, users := setup(t)
	alice, bob := createUser(t, users, "alice"), createUser(t, users, "bob")
	ctx := t.Context()

	if _, ok := get(t, store, alice.ID, "home", "web"); ok {
		t.Fatal("preferences found before any were saved")
	}
	if err := store.Put(ctx, alice.ID, "home", "web", json.RawMessage(`{"SortBy":"Name"}`)); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, alice.ID, "home", "android", json.RawMessage(`{"SortBy":"DateCreated"}`)); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, bob.ID, "home", "web", json.RawMessage(`{"SortBy":"Random"}`)); err != nil {
		t.Fatal(err)
	}
	// Saving again replaces the value.
	if err := store.Put(ctx, alice.ID, "home", "web", json.RawMessage(`{"SortBy":"PremiereDate"}`)); err != nil {
		t.Fatal(err)
	}

	for _, check := range []struct {
		user           accounts.ID
		id, client     string
		sortBy         string
		expectedStored bool
	}{
		{alice.ID, "home", "web", "PremiereDate", true},
		{alice.ID, "home", "android", "DateCreated", true},
		{bob.ID, "home", "web", "Random", true},
		{bob.ID, "home", "android", "", false},
		{alice.ID, "Home", "web", "", false},
		{alice.ID, "home", "Web", "", false},
	} {
		value, ok := get(t, store, check.user, check.id, check.client)
		if ok != check.expectedStored || (ok && value["SortBy"] != check.sortBy) {
			t.Errorf("%s/%s: got %v (stored %t), want SortBy %q (stored %t)",
				check.id, check.client, value, ok, check.sortBy, check.expectedStored)
		}
	}
}

func TestPreferencesGoWithTheirUser(t *testing.T) {
	store, users := setup(t)
	alice := createUser(t, users, "alice")
	if err := store.Put(t.Context(), alice.ID, "home", "web", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := users.DeleteUser(t.Context(), alice.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := get(t, store, alice.ID, "home", "web"); ok {
		t.Error("preferences outlived their user")
	}
}
