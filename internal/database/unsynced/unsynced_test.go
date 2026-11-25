package unsynced

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/testdb"
)

// A statement Exec runs commits with synchronous_commit off, its
// transaction only: the connection goes back to the pool with the
// setting as it was, and a statement run as usual still waits for the
// disk. Its commit leaves the write-ahead log to the WAL writer: the
// connection that writes 20 rows as usual writes the log itself, at each
// commit; through Exec, it does not.
func TestExecCommitsWithoutWaitingForTheDisk(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	config, err := pgxpool.ParseConfig(testdb.CopyURL(t, testdb.Empty(t)))
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, "CREATE TABLE written (n integer)"); err != nil {
		t.Fatal(err)
	}
	modes := testdb.WatchCommitModes(t, pool, "written")
	logWrites := func() int64 {
		t.Helper()
		// The connection's statistics, sent at the end of this
		// transaction, are read in the next one.
		if _, err := pool.Exec(ctx, "SELECT pg_stat_force_next_flush()"); err != nil {
			t.Fatal(err)
		}
		var writes int64
		if err := pool.QueryRow(ctx, `SELECT writes FROM pg_stat_get_backend_io(pg_backend_pid())
			WHERE object = 'wal' AND context = 'normal'`).Scan(&writes); err != nil {
			t.Fatal(err)
		}
		return writes
	}

	before := logWrites()
	for i := range 20 {
		tag, err := Exec(ctx, pool, "INSERT INTO written VALUES ($1)", i)
		if err != nil || tag.RowsAffected() != 1 {
			t.Fatalf("%v, %v", tag, err)
		}
	}
	unsyncedWrites := logWrites() - before
	var after string
	if err := pool.QueryRow(ctx, "SELECT current_setting('synchronous_commit')").Scan(&after); err != nil || after != "on" {
		t.Errorf("the connection after: %q, %v", after, err)
	}
	before = logWrites()
	for i := range 20 {
		if _, err := pool.Exec(ctx, "INSERT INTO written VALUES ($1)", i); err != nil {
			t.Fatal(err)
		}
	}
	syncedWrites := logWrites() - before

	want := slices.Concat(slices.Repeat([]string{"off"}, 20), slices.Repeat([]string{"on"}, 20))
	if got := modes("written"); !slices.Equal(got, want) {
		t.Errorf("commit modes: %v", got)
	}
	if syncedWrites < 20 || unsyncedWrites > 2 {
		t.Errorf("the connection wrote the log %d times for 20 rows written through Exec, %d for 20 written as usual", unsyncedWrites, syncedWrites)
	}
}
