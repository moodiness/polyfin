package database

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/testdb"
)

// prepares counts the statements prepared one by one on a pool.
type prepares struct{ n atomic.Int32 }

func (p *prepares) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	return ctx
}

func (*prepares) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (p *prepares) TracePrepareStart(ctx context.Context, _ *pgx.Conn, data pgx.TracePrepareStartData) context.Context {
	p.n.Add(1)
	return ctx
}

func (*prepares) TracePrepareEnd(context.Context, *pgx.Conn, pgx.TracePrepareEndData) {}

// A listing statement is prepared on each new connection as it opens:
// its first run there prepares nothing more. Another statement is
// prepared at its first run, as before.
func TestListingStatementsArePreparedAsAConnectionOpens(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	empty := testdb.Empty(t)
	if err := Migrate(t.Context(), empty, logger); err != nil {
		t.Fatal(err)
	}
	listed := Listing("SELECT version, name FROM schema_migrations WHERE version = $1 AND name = $2")
	config, err := poolConfig(testdb.CopyURL(t, empty), logger)
	if err != nil {
		t.Fatal(err)
	}
	counted := &prepares{}
	config.ConnConfig.Tracer = counted
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version int
	if err := pool.QueryRow(t.Context(), listed, 1, "init").Scan(&version); err != nil && err != pgx.ErrNoRows {
		t.Fatal(err)
	}
	if n := counted.n.Load(); n != 0 {
		t.Errorf("a listing statement prepared %d times at its first run", n)
	}
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM schema_migrations WHERE version > $1", 0).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if n := counted.n.Load(); n != 1 {
		t.Errorf("another statement prepared %d times at its first run, want 1", n)
	}
}

// A connection the pool opens is handed out outside any transaction,
// whether its listing statements prepare and run, one fails to prepare,
// as in a database not migrated yet, or one fails as it runs: it locks
// rows and writes as any other.
//
//nolint:paralleltest,tparallel // replaces the statements Listing registered, which every pool of the package prepares
func TestListingStatementsLeaveANewConnectionIdle(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	empty := testdb.Empty(t)
	if err := Migrate(t.Context(), empty, logger); err != nil {
		t.Fatal(err)
	}
	url := testdb.CopyURL(t, empty)
	prepares := "SELECT version, name FROM schema_migrations WHERE version = $1"
	for name, statements := range map[string][]string{
		"all prepare":          {prepares},
		"one fails to prepare": {prepares, "SELECT 1 FROM no_such_table WHERE $1"},
		"one fails as it runs": {prepares, "SELECT version FROM schema_migrations WHERE version = $1 OR 1 / 0 = 1"},
	} {
		t.Run(name, func(t *testing.T) {
			registerOnly(t, statements...)
			config, err := poolConfig(url, logger)
			if err != nil {
				t.Fatal(err)
			}
			pool, err := pgxpool.NewWithConfig(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			conn, err := pool.Acquire(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Release()
			if status := conn.Conn().PgConn().TxStatus(); status != 'I' {
				t.Fatalf("a new connection's transaction status is %q, want 'I'", status)
			}
			var version int
			if err := conn.QueryRow(t.Context(), "SELECT version FROM schema_migrations ORDER BY version LIMIT 1 FOR UPDATE").Scan(&version); err != nil {
				t.Fatal(err)
			}
			if _, err := conn.Exec(t.Context(), "CREATE TEMP TABLE written (n int)"); err != nil {
				t.Fatal(err)
			}
			if _, err := conn.Exec(t.Context(), "INSERT INTO written VALUES ($1)", version); err != nil {
				t.Fatal(err)
			}
			if status := conn.Conn().PgConn().TxStatus(); status != 'I' {
				t.Errorf("the connection's transaction status is %q after it wrote, want 'I'", status)
			}
		})
	}
}

// registerOnly leaves registered only the listing statements sqls until t
// ends.
func registerOnly(t *testing.T, sqls ...string) {
	listing.mu.Lock()
	registered := listing.statements
	listing.statements = nil
	listing.mu.Unlock()
	t.Cleanup(func() {
		listing.mu.Lock()
		listing.statements = registered
		listing.mu.Unlock()
	})
	for _, sql := range sqls {
		Listing(sql)
	}
}

func TestParametersCountsTheHighestNamed(t *testing.T) {
	t.Parallel()
	for sql, want := range map[string]int{
		"SELECT 1":                       0,
		"SELECT $1 WHERE a = $2":         2,
		"SELECT $2, $10, $3 WHERE $1":    10,
		"SELECT unnest($1::uuid[]), $$x": 1,
	} {
		if got := parameters(sql); got != want {
			t.Errorf("%q: %d parameters, want %d", sql, got, want)
		}
	}
}
