package testdb

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// The restores lock is held by one test at a time, whatever test binary it
// runs in: a second test taking it waits in the database until the first
// one's cleanups are done.
func TestRestoresTakeTurns(t *testing.T) {
	t.Parallel()
	url := URL(t)
	var mu sync.Mutex
	var events []string
	event := func(what string) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, what)
	}
	// The test's cleanups run once both subtests are done.
	t.Cleanup(func() {
		if want := []string{"first holds it", "first ends", "second holds it"}; !slices.Equal(events, want) {
			t.Errorf("events %q, not %q", events, want)
		}
	})
	held := make(chan struct{})
	t.Run("first", func(t *testing.T) {
		t.Parallel()
		LockRestores(t)
		event("first holds it")
		// Registered after LockRestores' own, it runs before the lock is
		// released.
		t.Cleanup(func() { event("first ends") })
		close(held)
		awaitRestoresWait(t, url, "TestRestoresTakeTurns/second")
	})
	t.Run("second", func(t *testing.T) {
		t.Parallel()
		<-held
		LockRestores(t)
		event("second holds it")
	})
}

// awaitRestoresWait returns once the test named name waits for the
// restores lock, and fails after half a minute.
func awaitRestoresWait(t *testing.T, url, name string) {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	deadline := time.Now().Add(30 * time.Second)
	for {
		var waiting bool
		if err := conn.QueryRow(t.Context(), `SELECT EXISTS (SELECT FROM pg_stat_activity
			WHERE application_name = left($1, 63) AND wait_event_type = 'Lock' AND wait_event = 'advisory')`,
			restoresApp+name).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s did not wait for the restores lock", name)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
