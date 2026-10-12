// Package testdb gives tests an isolated PostgreSQL schema, or a database of
// their own copied from a template.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// URL returns POLYFIN_TEST_DATABASE_URL and skips the test when it is unset.
// CI always sets it, so database tests never skip there.
func URL(t testing.TB) string {
	t.Helper()
	url := os.Getenv("POLYFIN_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("POLYFIN_TEST_DATABASE_URL is not set")
	}
	return url
}

// maxConns bounds each test's pool. Tests run in parallel, packages too, and
// they share the server's max_connections, 100 by default: CI runs four
// packages of four tests at a time, each with a pool.
const maxConns = 4

// New returns a pool whose search_path is a fresh schema, dropped when the
// test ends, so tests never see each other's tables.
func New(t testing.TB) *pgxpool.Pool {
	t.Helper()
	url := URL(t)
	ctx := t.Context()

	schema := pgx.Identifier{"test_" + random()}.Sanitize()
	if err := exec(ctx, url, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		if err := exec(context.Background(), url, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})
	return open(t, url, func(config *pgxpool.Config) { config.ConnConfig.RuntimeParams["search_path"] = schema })
}

// templatePrefix starts the names of template databases; their key ends
// them.
const templatePrefix = "polyfin_template_"

// templateLife is how long a template database of another key is kept: a
// branch whose tests have not used theirs for that long most likely has
// none to run any more. One still running makes it again.
const templateLife = 24 * time.Hour

// templates holds, by name, the template databases this test binary found
// or made.
var templates sync.Map // string → *template

type template struct {
	mu    sync.Mutex
	ready bool
}

// Copy returns a pool on a database of its own, dropped when the test ends,
// copied from the template database that key names. The first test needing
// it, in any test binary, makes it with prepare; it then serves later runs
// too. Copying takes a fraction of the time preparing would: key must
// change whenever prepare would give another database.
func Copy(t testing.TB, key string, prepare func(context.Context, *pgxpool.Pool) error) *pgxpool.Pool {
	t.Helper()
	url := URL(t)
	ctx := t.Context()
	source := templatePrefix + key
	name := "polyfin_test_" + random()
	create := "CREATE DATABASE " + pgx.Identifier{name}.Sanitize() + " TEMPLATE " + pgx.Identifier{source}.Sanitize()
	for attempt := 1; ; attempt++ {
		if err := makeTemplate(ctx, url, source, prepare); err != nil {
			t.Fatalf("make the template database: %v", err)
		}
		err := exec(ctx, url, create)
		if err == nil {
			break
		}
		// The tests of another branch may have dropped it as too old:
		// it is made again.
		if pgErr := (*pgconn.PgError)(nil); attempt < 3 && errors.As(err, &pgErr) && pgErr.Code == "3D000" {
			forget(source)
			continue
		}
		t.Fatalf("copy the template database: %v", err)
	}
	t.Cleanup(func() {
		if err := exec(context.Background(), url, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Errorf("drop test database: %v", err)
		}
	})
	return open(t, url, func(config *pgxpool.Config) { config.ConnConfig.Database = name })
}

// makeTemplate makes sure the template database name exists, making it
// with prepare when it does not.
func makeTemplate(ctx context.Context, url, name string, prepare func(context.Context, *pgxpool.Pool) error) error {
	value, _ := templates.LoadOrStore(name, &template{})
	tpl := value.(*template)
	tpl.mu.Lock()
	defer tpl.mu.Unlock()
	if tpl.ready {
		return nil
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(context.Background()) }()
	// Test binaries starting together make it once: the others wait, then
	// find it. Closing the connection releases the lock.
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock(hashtext($1))", name); err != nil {
		return err
	}
	var exists bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT FROM pg_database WHERE datname = $1)", name).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if err := buildTemplate(ctx, conn, url, name, prepare); err != nil {
			return err
		}
	}
	tpl.ready = true
	return nil
}

// buildTemplate makes the template database name under another name, so
// that it exists under its own only once prepared and dated, then drops the
// templates of other keys made more than templateLife ago.
func buildTemplate(ctx context.Context, conn *pgx.Conn, url, name string, prepare func(context.Context, *pgxpool.Pool) error) error {
	building := pgx.Identifier{name + "_building"}.Sanitize()
	// A test binary stopped while making it may have left it.
	if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+building+" WITH (FORCE)"); err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+building); err != nil {
		return err
	}
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		return err
	}
	config.ConnConfig.Database = name + "_building"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return err
	}
	err = prepare(ctx, pool)
	if err == nil {
		_, err = pool.Exec(ctx, autovacuumOff)
	}
	pool.Close()
	if err != nil {
		return err
	}
	ident := pgx.Identifier{name}.Sanitize()
	now := time.Now().UTC()
	// The comment dates it for the tests of other branches. No one
	// connects to it: a database being copied must have no connection.
	// Both are set before it takes its name: a template found undated
	// counts as old, and the test binaries making others at the same time
	// would drop it.
	for _, statement := range []string{
		"COMMENT ON DATABASE " + building + " IS '" + now.Format(time.RFC3339) + "'",
		"ALTER DATABASE " + building + " WITH ALLOW_CONNECTIONS false",
		"ALTER DATABASE " + building + " RENAME TO " + ident,
	} {
		if _, err := conn.Exec(ctx, statement); err != nil {
			return err
		}
	}
	rows, _ := conn.Query(ctx, `SELECT datname FROM pg_database
		WHERE starts_with(datname, $1) AND datname <> $2 AND NOT datname LIKE '%\_building'
		AND coalesce(shobj_description(oid, 'pg_database'), '') < $3`,
		templatePrefix, name, now.Add(-templateLife).Format(time.RFC3339))
	old, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	for _, other := range old {
		if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{other}.Sanitize()); err != nil {
			return fmt.Errorf("drop the old template database %s: %w", other, err)
		}
	}
	return nil
}

// autovacuumOff turns autovacuum off for every table: it comes at once to
// a new database, and its analyses would change query plans at random
// moments of the tests, those of the time bounds among them.
const autovacuumOff = `DO $$
DECLARE
	t regclass;
BEGIN
	FOR t IN SELECT c.oid FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind = 'r' AND n.nspname NOT IN ('pg_catalog', 'information_schema') AND n.nspname NOT LIKE 'pg\_toast%'
	LOOP
		EXECUTE format('ALTER TABLE %s SET (autovacuum_enabled = false)', t);
	END LOOP;
END
$$`

// forget has the next Copy look for the template database name again.
func forget(name string) {
	if value, ok := templates.Load(name); ok {
		tpl := value.(*template)
		tpl.mu.Lock()
		tpl.ready = false
		tpl.mu.Unlock()
	}
}

// open returns a pool on url, bounded to maxConns and closed when the test
// ends, configured by configure.
func open(t testing.TB, url string, configure func(*pgxpool.Config)) *pgxpool.Pool {
	t.Helper()
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = maxConns
	configure(config)
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// exec runs statement on a connection of its own, closed at once: the test
// keeps only its pool's.
func exec(ctx context.Context, url, statement string) error {
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(context.Background()) }()
	_, err = conn.Exec(ctx, statement)
	return err
}

// random returns 16 random hexadecimal digits.
func random() string {
	suffix := make([]byte, 8)
	_, _ = rand.Read(suffix)
	return hex.EncodeToString(suffix)
}
