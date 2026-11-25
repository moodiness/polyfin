// Package unsynced runs the writes that need not wait for the disk.
package unsynced

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// off turns synchronous_commit off for the transaction it runs in, the
// function rather than SET LOCAL, which warns, in the server's log too,
// when it is not in a transaction block, as in a batch's.
const off = "SELECT set_config('synchronous_commit', 'off', true)"

// Exec runs sql, one statement in a transaction of its own, whose commit
// does not wait for the write-ahead log to reach the disk. Only a
// statement that writes nothing but what may be lost takes it: a cache,
// which is fetched again when it misses, or an activity time, which the
// next request writes again. A crash of PostgreSQL or of its machine
// loses the last moments of such writes, a few hundred milliseconds
// (three times wal_writer_delay at most), and nothing else: every other
// commit still waits for the disk, and with it for every write before it.
// While the disk is slow, the statement no longer waits for it.
//
// The setting goes with the statement, in the same round trip, as the
// first query of a batch, whose transaction it lasts: the connection goes
// back to the pool as it was. It takes the pool, never a transaction,
// whose other writes the setting would reach.
func Exec(ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) (pgconn.CommandTag, error) {
	batch := &pgx.Batch{}
	batch.Queue(off)
	var tag pgconn.CommandTag
	batch.Queue(sql, args...).Exec(func(written pgconn.CommandTag) error {
		tag = written
		return nil
	})
	err := pool.SendBatch(ctx, batch).Close()
	return tag, err
}
