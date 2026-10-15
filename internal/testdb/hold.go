package testdb

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Hold keeps rows locked for a test, from a connection of its own outside
// the pool, so that the requests that lock or write them wait in the
// database until the test releases them.
type Hold struct {
	t  testing.TB
	tx pgx.Tx
}

// HoldRows locks the rows that query locks, such as SELECT … FOR UPDATE,
// in the database of a pool of Copy, until Release or the end of the test.
func HoldRows(t testing.TB, pool *pgxpool.Pool, query string, args ...any) *Hold {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), CopyURL(t, pool))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	tx, err := conn.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), query, args...); err != nil {
		t.Fatal(err)
	}
	return &Hold{t: t, tx: tx}
}

// AwaitWaiting returns once done, given how many connections of the
// database wait on a lock, says so.
func (h *Hold) AwaitWaiting(done func(waiting int) bool) {
	h.t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		// Within the transaction, the activity read is kept unless cleared.
		if _, err := h.tx.Exec(h.t.Context(), "SELECT pg_stat_clear_snapshot()"); err != nil {
			h.t.Fatal(err)
		}
		var waiting int
		if err := h.tx.QueryRow(h.t.Context(), `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&waiting); err != nil {
			h.t.Fatal(err)
		}
		if done(waiting) {
			return
		}
		<-ticker.C
	}
}

// Release unlocks the rows.
func (h *Hold) Release() {
	h.t.Helper()
	if err := h.tx.Rollback(h.t.Context()); err != nil {
		h.t.Fatal(err)
	}
}
