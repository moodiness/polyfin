package cluster

import (
	"bytes"
	"context"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// startedWithIndex starts a new cluster whose server's database holds a
// table with a text index, and reports the index's file number.
func startedWithIndex(t *testing.T, c *Cluster) string {
	t.Helper()
	ctx := context.Background()
	if _, err := c.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	stopAtEnd(t, c)
	execute(t, c.URL(), "CREATE TABLE names (name text)",
		"INSERT INTO names VALUES ('b'), ('B'), ('a'), ('Z')",
		"CREATE INDEX names_name ON names (name)")
	return indexFile(t, c)
}

func indexFile(t *testing.T, c *Cluster) string {
	t.Helper()
	return query(t, c.URL(), "SELECT relfilenode::text FROM pg_class WHERE relname = 'names_name'")
}

func execute(t *testing.T, url string, statements ...string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)
	for _, sql := range statements {
		if _, err := conn.Exec(ctx, sql); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
}

// warnings are the warnings a new connection to database gets.
func warnings(t *testing.T, c *Cluster, database string) []string {
	t.Helper()
	ctx := context.Background()
	config, err := pgx.ParseConfig(c.url(database))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	config.OnNotice = func(_ *pgconn.PgConn, n *pgconn.Notice) {
		if n.Severity == "WARNING" {
			got = append(got, n.Message)
		}
	}
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatalf("connect to %s: %v", database, err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	return got
}

// staleVersions lists the databases whose recorded collation version is
// not the actual one.
func staleVersions(t *testing.T, c *Cluster) string {
	t.Helper()
	return query(t, c.url("postgres"), `SELECT coalesce(string_agg(datname, ',' ORDER BY datname), '')
		FROM pg_database WHERE datallowconn AND datcollversion IS DISTINCT FROM pg_database_collation_actual_version(oid)`)
}

// A database whose collation version changed, as after an update that
// brings another ICU, has its indexes rebuilt and its version refreshed
// as the cluster starts, before the server's database is created, which
// a template1 whose version changed would refuse.
func TestChangedCollationVersionIsRefreshed(t *testing.T) {
	bin := postgresBin(t)
	ctx := context.Background()
	var logs bytes.Buffer
	c := newCluster(t, bin, t.TempDir())
	c.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	var calls []bool
	c.Rebuilding = func(on bool) { calls = append(calls, on) }
	before := startedWithIndex(t, c)

	execute(t, c.url("postgres"), "UPDATE pg_database SET datcollversion = '0' WHERE datname IN ('polyfin', 'template1')")
	if got := warnings(t, c, Database); len(got) != 1 || !strings.Contains(got[0], "collation version mismatch") {
		t.Fatalf("warnings before the refresh = %q", got)
	}
	if err := c.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if stale := staleVersions(t, c); stale != "" {
		t.Errorf("still stale: %s", stale)
	}
	if after := indexFile(t, c); after == before {
		t.Errorf("the index was not rebuilt: file %s", after)
	}
	for _, database := range []string{Database, "template1", "postgres"} {
		if got := warnings(t, c, database); len(got) != 0 {
			t.Errorf("warnings of %s after the refresh = %q", database, got)
		}
	}
	if !slices.Equal(calls, []bool{true, false}) {
		t.Errorf("Rebuilding calls = %v", calls)
	}
	actual := query(t, c.URL(), "SELECT pg_database_collation_actual_version(oid) FROM pg_database WHERE datname = current_database()")
	for _, database := range []string{Database, "template1"} {
		if want := "database=" + database + " from=0 to=" + actual + " took="; !strings.Contains(logs.String(), want) {
			t.Errorf("the log lacks %q:\n%s", want, logs.String())
		}
	}

	// The server's database is created only once template1 is refreshed.
	execute(t, c.url("postgres"), "DROP DATABASE polyfin", "UPDATE pg_database SET datcollversion = '0' WHERE datname = 'template1'")
	if err := c.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start without the server's database: %v", err)
	}
	if stale := staleVersions(t, c); stale != "" {
		t.Errorf("still stale: %s", stale)
	}
	if got := query(t, c.URL(), "SELECT current_database()"); got != Database {
		t.Errorf("database = %q", got)
	}
}

// When every collation version matches, the usual case, nothing is
// rebuilt.
func TestMatchingCollationVersionIsLeftAlone(t *testing.T) {
	bin := postgresBin(t)
	c := newCluster(t, bin, t.TempDir())
	var calls []bool
	c.Rebuilding = func(on bool) { calls = append(calls, on) }
	before := startedWithIndex(t, c)
	if err := c.refreshCollations(context.Background()); err != nil {
		t.Fatal(err)
	}
	if after := indexFile(t, c); after != before {
		t.Errorf("the index was rebuilt: file %s, then %s", before, after)
	}
	if len(calls) != 0 {
		t.Errorf("Rebuilding calls = %v", calls)
	}
	if stale := staleVersions(t, c); stale != "" {
		t.Errorf("stale: %s", stale)
	}
}
