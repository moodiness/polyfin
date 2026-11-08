package testdb

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// restoresLock is the key of the advisory lock LockRestores takes, in the
// shared test database: "pfrestor". The production migrations' lock is
// another key, and templates' locks, from hashtext, are integers.
const restoresLock int64 = 0x7066726573746f72

// restoresApp starts the application_name of LockRestores' connections,
// which the test's name ends: pg_stat_activity tells which test holds the
// lock, or waits for it.
const restoresApp = "testdb restores: "

// LockRestores waits until no other test, in this test binary or another,
// holds the restores lock, then holds it until the test ends, its later
// cleanups included. A test restoring a whole database takes it once, at
// its start. A restore drops and makes every table in one transaction,
// holding a lock on each until it ends: thousands of entries of the
// server's shared lock table, which holds 64 per connection, 6,400 on CI's
// server. Two restores at once can exhaust it ("out of shared memory"),
// and fail them and any test then asking for a lock.
//
// The lock is a session advisory lock, on a connection of its own to the
// shared test database, which every test binary reaches.
func LockRestores(t testing.TB) {
	t.Helper()
	config, err := pgx.ParseConfig(URL(t))
	if err != nil {
		t.Fatal(err)
	}
	config.RuntimeParams["application_name"] = restoresApp + t.Name()
	conn, err := pgx.ConnectConfig(t.Context(), config)
	if err != nil {
		t.Fatalf("connect for the restores lock: %v", err)
	}
	// Closing the connection releases the lock too, should unlocking
	// fail.
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	if _, err := conn.Exec(t.Context(), "SELECT pg_advisory_lock($1)", restoresLock); err != nil {
		t.Fatalf("take the restores lock: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		var held bool
		if err := conn.QueryRow(ctx, "SELECT pg_advisory_unlock($1)", restoresLock).Scan(&held); err != nil {
			t.Errorf("release the restores lock: %v", err)
		} else if !held {
			t.Error("release the restores lock: it was not held")
		}
	})
}
