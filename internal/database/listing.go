package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5"
)

// pgx prepares each statement on a connection the first time it runs
// there, one round trip each, and the pool opens its connections as
// Polyfin starts. The first home screen after a start would then prepare
// each statement of its listings, on each connection it runs on, while
// it answers. The statements listings run on every request are prepared
// instead on each new connection as it opens, all in one pipeline (see
// prepareListings).

// listing holds the statements Listing registered.
var listing struct {
	mu         sync.Mutex
	statements []listingStatement
}

// listingStatement is a statement Listing registered, with the number of
// its parameters.
type listingStatement struct {
	sql    string
	params int
}

// Listing registers sql, a statement the listings of every request run,
// to be prepared on each new connection of the pools Connect opens, and
// returns it. Packages register theirs as they initialize. sql only
// reads, and reads nothing when its parameters are all NULL: preparing
// runs it so.
func Listing(sql string) string {
	listing.mu.Lock()
	defer listing.mu.Unlock()
	listing.statements = append(listing.statements, listingStatement{sql: sql, params: parameters(sql)})
	return sql
}

// parameters counts the parameters of sql: the highest $n it names.
func parameters(sql string) int {
	most := 0
	for i := 0; i < len(sql); i++ {
		if sql[i] != '$' {
			continue
		}
		n := 0
		for i+1 < len(sql) && sql[i+1] >= '0' && sql[i+1] <= '9' {
			i++
			n = n*10 + int(sql[i]-'0')
		}
		most = max(most, n)
	}
	return most
}

// prepareListings returns the pool's AfterConnect: it prepares the
// statements Listing registered on a new connection, all in one round
// trip, and runs them once with NULL parameters, which reads nothing, in
// another, inside a read-only transaction, so that none can write, which
// the same round trip rolls back: pgx keeps a statement a batch prepared,
// as it keeps one a query prepared. A statement that cannot be prepared,
// as in a database not migrated yet, is prepared as it is first used, as
// are the others. The connection is handed out outside any transaction,
// or not at all.
func prepareListings(logger *slog.Logger) func(context.Context, *pgx.Conn) error {
	return func(ctx context.Context, conn *pgx.Conn) error {
		listing.mu.Lock()
		statements := listing.statements
		listing.mu.Unlock()
		if len(statements) == 0 {
			return nil
		}
		batch := &pgx.Batch{QueuedQueries: make([]*pgx.QueuedQuery, 0, len(statements)+2)}
		batch.BeginTx(pgx.TxOptions{AccessMode: pgx.ReadOnly})
		for _, statement := range statements {
			batch.Queue(statement.sql, make([]any, statement.params)...)
		}
		batch.Queue("ROLLBACK")
		results := conn.SendBatch(ctx, batch)
		tx := results.Tx()
		if err := results.Close(); err != nil {
			if conn.IsClosed() {
				return err
			}
			logger.Debug("The listing statements were not prepared on a new connection", "error", err)
		}
		// The queued ROLLBACK ended the transaction, unless a statement
		// failed as it ran and the server skipped the rest of the batch.
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			return fmt.Errorf("roll back the transaction that prepared the listing statements: %w", err)
		}
		if status := conn.PgConn().TxStatus(); status != 'I' {
			return fmt.Errorf("a new connection is left in a transaction (status %q) once its listing statements are prepared", status)
		}
		return nil
	}
}
