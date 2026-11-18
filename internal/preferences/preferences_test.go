package preferences

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

func setup(t *testing.T) (*Store, *accounts.Store) {
	t.Helper()
	pool := migrated.New(t)
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
	t.Parallel()
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
	t.Parallel()
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

func kept(t *testing.T, store *Store, user accounts.ID) int {
	t.Helper()
	var count int
	if err := store.db.QueryRow(t.Context(), "SELECT count(*) FROM display_preferences WHERE user_id = $1", user).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// A value or a client's name over its bound is refused, and what was kept
// stays. One of the bound's size is kept whatever its shape: the database
// bounds the text it would print of a value, a space after each comma, and
// that is longest for a list of digits.
func TestPreferencesOverABoundAreRefused(t *testing.T) {
	t.Parallel()
	store, users := setup(t)
	alice := createUser(t, users, "alice")
	ctx := t.Context()

	digits := strings.Repeat("0,", (MaxValueBytes-10)/2)
	full, longest := `{"ab":[`+digits+`0]}`, strings.Repeat("é", MaxClientBytes/2)
	if len(full) != MaxValueBytes || len(longest) != MaxClientBytes {
		t.Fatalf("a value of %d bytes and a client of %d", len(full), len(longest))
	}
	if err := store.Put(ctx, alice.ID, "home", "web", json.RawMessage(full)); err != nil {
		t.Fatalf("a value of the bound's size: %v", err)
	}
	if err := store.Put(ctx, alice.ID, "home", longest, json.RawMessage(`{"SortBy":"Name"}`)); err != nil {
		t.Fatalf("a client of the bound's size: %v", err)
	}
	for _, refused := range []struct{ what, id, client, value string }{
		{"a new value over the bound", "other", "web", `{"ab":[` + digits + `10]}`},
		{"a value over the bound in place of one kept", "home", longest, `{"ab":[` + digits + `10]}`},
		{"a client over the bound", "home", longest + "x", `{}`},
	} {
		if err := store.Put(ctx, alice.ID, refused.id, refused.client, json.RawMessage(refused.value)); !errors.Is(err, ErrTooLarge) {
			t.Errorf("%s: %v, want ErrTooLarge", refused.what, err)
		}
	}
	if n := kept(t, store, alice.ID); n != 2 {
		t.Errorf("alice keeps %d display preferences, want 2", n)
	}
	if value, ok := get(t, store, alice.ID, "home", longest); !ok || value["SortBy"] != "Name" {
		t.Errorf("after a refused value: SortBy %v (stored %t), want Name", value["SortBy"], ok)
	}
}

// A user keeps MaxPerUser display preferences: a new one past the bound
// takes the place of the one written longest ago, whether that was its
// first writing or a later one. Another user's are not counted, nor
// touched.
func TestANewKeyPastTheBoundReplacesTheOneWrittenLongestAgo(t *testing.T) {
	t.Parallel()
	store, users := setup(t)
	alice, bob := createUser(t, users, "alice"), createUser(t, users, "bob")
	ctx := t.Context()
	put := func(user accounts.ID, id, client string) {
		t.Helper()
		if err := store.Put(ctx, user, id, client, json.RawMessage(`{"SortBy":"`+id+`"}`)); err != nil {
			t.Fatal(err)
		}
	}

	put(bob.ID, "home", "web")
	for i := range MaxPerUser + 1 {
		put(alice.ID, fmt.Sprintf("view-%03d", i), "web")
	}
	if n := kept(t, store, alice.ID); n != MaxPerUser {
		t.Fatalf("alice keeps %d display preferences, want %d", n, MaxPerUser)
	}
	if _, ok := get(t, store, alice.ID, fmt.Sprintf("view-%03d", MaxPerUser), "web"); !ok {
		t.Error("the one written last was not kept")
	}

	// The times of writing are the database's. They are set apart here, in
	// the order of the names, so that which one was written longest ago
	// does not depend on its clock.
	rows, err := store.db.Query(ctx, `UPDATE display_preferences
		SET updated_at = timestamptz '2000-01-01' + make_interval(mins => substr(preference_id, 6)::int)
		WHERE user_id = $1 RETURNING preference_id`, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	slices.Sort(ids)

	// The oldest is written again, then two new ones: its id for another
	// client, which is a key of its own, and a new id.
	put(alice.ID, ids[0], "web")
	put(alice.ID, ids[0], "tv")
	put(alice.ID, "one more", "web")
	if n := kept(t, store, alice.ID); n != MaxPerUser {
		t.Errorf("alice keeps %d display preferences, want %d", n, MaxPerUser)
	}
	for _, check := range []struct {
		id, client string
		stored     bool
	}{
		{ids[0], "web", true},
		{ids[0], "tv", true},
		{"one more", "web", true},
		{ids[1], "web", false},
		{ids[2], "web", false},
		{ids[3], "web", true},
	} {
		if _, ok := get(t, store, alice.ID, check.id, check.client); ok != check.stored {
			t.Errorf("%s/%s: stored %t, want %t", check.id, check.client, ok, check.stored)
		}
	}
	if value, ok := get(t, store, bob.ID, "home", "web"); !ok || value["SortBy"] != "home" || kept(t, store, bob.ID) != 1 {
		t.Errorf("bob's display preferences: %v (stored %t)", value, ok)
	}
}

// Rows over the bound that a user kept from before the bound go with
// their next new one, which is kept. All were written at the same time,
// as the rows of an upgraded server are.
func TestRowsOverTheBoundFromBeforeGoWithTheNextNewKey(t *testing.T) {
	t.Parallel()
	store, users := setup(t)
	alice, bob := createUser(t, users, "alice"), createUser(t, users, "bob")
	ctx := t.Context()
	if _, err := store.db.Exec(ctx, `INSERT INTO display_preferences (user_id, preference_id, client, value)
		SELECT u, 'view-' || n, 'web', '{}' FROM unnest($1::uuid[]) u, generate_series(1, $2) n`,
		[]accounts.ID{alice.ID, bob.ID}, MaxPerUser+20); err != nil {
		t.Fatal(err)
	}
	// One of them written again leaves the others.
	if err := store.Put(ctx, alice.ID, "view-1", "web", json.RawMessage(`{"SortBy":"Name"}`)); err != nil {
		t.Fatal(err)
	}
	if n := kept(t, store, alice.ID); n != MaxPerUser+20 {
		t.Errorf("alice keeps %d display preferences after writing one again, want %d", n, MaxPerUser+20)
	}
	if err := store.Put(ctx, alice.ID, "home", "web", json.RawMessage(`{"SortBy":"Random"}`)); err != nil {
		t.Fatal(err)
	}
	if n := kept(t, store, alice.ID); n != MaxPerUser {
		t.Errorf("alice keeps %d display preferences after a new one, want %d", n, MaxPerUser)
	}
	for id, sortBy := range map[string]string{"home": "Random", "view-1": "Name"} {
		if value, ok := get(t, store, alice.ID, id, "web"); !ok || value["SortBy"] != sortBy {
			t.Errorf("%s: %v (stored %t), want SortBy %q", id, value, ok, sortBy)
		}
	}
	if n := kept(t, store, bob.ID); n != MaxPerUser+20 {
		t.Errorf("bob keeps %d display preferences, want %d", n, MaxPerUser+20)
	}
}

// New keys a user saves at once count each other: the user holds the
// bound and never more, and each of them is kept, in place of rows written
// before.
func TestNewKeysSavedAtOnceStayWithinTheBound(t *testing.T) {
	t.Parallel()
	store, users := setup(t)
	alice := createUser(t, users, "alice")
	ctx := t.Context()
	if _, err := store.db.Exec(ctx, `INSERT INTO display_preferences (user_id, preference_id, client, value, updated_at)
		SELECT $1, 'view-' || n, 'web', '{}', timestamptz '2000-01-01' FROM generate_series(1, $2) n`, alice.ID, MaxPerUser); err != nil {
		t.Fatal(err)
	}
	const atOnce = 8
	saved := make(chan error, atOnce)
	for i := range atOnce {
		go func() {
			saved <- store.Put(ctx, alice.ID, fmt.Sprintf("new-%d", i), "web", json.RawMessage(`{"SortBy":"Name"}`))
		}()
	}
	for range atOnce {
		if err := <-saved; err != nil {
			t.Error(err)
		}
	}
	if n := kept(t, store, alice.ID); n != MaxPerUser {
		t.Errorf("alice keeps %d display preferences, want %d", n, MaxPerUser)
	}
	for i := range atOnce {
		if _, ok := get(t, store, alice.ID, fmt.Sprintf("new-%d", i), "web"); !ok {
			t.Errorf("new-%d was not kept", i)
		}
	}
}
