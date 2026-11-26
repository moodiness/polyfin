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
// disk, as the commit modes noted show. Its commit leaves the write-ahead
// log to the WAL writer: through Exec, the connection does not write the
// log itself; as usual, it does.
//
// Not at each usual commit, though: PostgreSQL writes in one go the
// records of all the commits waiting, whichever connection's, and a commit
// whose record another connection or the WAL writer wrote first has
// nothing left to write. On a busy server, as the other packages' tests
// make it, half the commits or more may find so. The rows are therefore
// written in pairs, one through Exec, one as usual, under the same load,
// until the usual ones had the connection write the log 10 times, in 10
// pairs on a quiet server: the Exec ones, as many, had it write the log at
// most twice, when the log's buffers were full. Another flush only takes
// writes away, from the usual commits; it never adds any to the Exec ones.
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

	var unsyncedWrites, syncedWrites int64
	var want []string
	writes := logWrites()
	for i := 0; syncedWrites < 10 && i < 200; i++ {
		tag, err := Exec(ctx, pool, "INSERT INTO written VALUES ($1)", i)
		if err != nil || tag.RowsAffected() != 1 {
			t.Fatalf("%v, %v", tag, err)
		}
		unsynced := logWrites()
		unsyncedWrites += unsynced - writes
		if _, err := pool.Exec(ctx, "INSERT INTO written VALUES ($1)", i); err != nil {
			t.Fatal(err)
		}
		writes = logWrites()
		syncedWrites += writes - unsynced
		want = append(want, "off", "on")
	}

	if got := modes("written"); !slices.Equal(got, want) {
		t.Errorf("commit modes: %v", got)
	}
	if syncedWrites < 10 || unsyncedWrites > 2 {
		t.Errorf("the connection wrote the log %d times for %d rows written through Exec, %d for as many written as usual",
			unsyncedWrites, len(want)/2, syncedWrites)
	}
}
