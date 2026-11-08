package testdb

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// counted returns a template key of the test's own, dropped when the test
// ends, and a prepare that makes a table of one row and counts its runs.
func counted(t *testing.T) (string, func(context.Context, *pgxpool.Pool) error, *atomic.Int32) {
	t.Helper()
	url := URL(t)
	key := random()
	t.Cleanup(func() {
		if err := exec(context.Background(), url, "DROP DATABASE IF EXISTS "+pgx.Identifier{templatePrefix + key}.Sanitize()); err != nil {
			t.Errorf("drop the template: %v", err)
		}
	})
	runs := new(atomic.Int32)
	return key, func(ctx context.Context, pool *pgxpool.Pool) error {
		runs.Add(1)
		_, err := pool.Exec(ctx, "CREATE TABLE prepared (n int); INSERT INTO prepared VALUES (1)")
		return err
	}, runs
}

func rows(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM prepared").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Copies hold what prepare made, run once for all of them, and each test's
// writes stay in its own copy, dropped when it ends.
//
//nolint:tparallel // the copies group waits for its parallel copies before the counts are read
func TestCopiesShareOnePreparation(t *testing.T) {
	t.Parallel()
	key, prepare, runs := counted(t)
	names := make([]string, 3)
	t.Run("copies", func(t *testing.T) {
		for i := range names {
			t.Run("copy", func(t *testing.T) {
				t.Parallel()
				pool := Copy(t, key, prepare)
				if _, err := pool.Exec(t.Context(), "INSERT INTO prepared VALUES (2)"); err != nil {
					t.Fatal(err)
				}
				if n := rows(t, pool); n != 2 {
					t.Errorf("%d rows, want the prepared one and this copy's", n)
				}
				var options []string
				if err := pool.QueryRow(t.Context(), "SELECT reloptions FROM pg_class WHERE relname = 'prepared'").Scan(&options); err != nil ||
					len(options) != 1 || options[0] != "autovacuum_enabled=false" {
					t.Errorf("table options %q, want autovacuum off: %v", options, err)
				}
				names[i] = pool.Config().ConnConfig.Database
			})
		}
	})
	if n := runs.Load(); n != 1 {
		t.Errorf("prepared %d times", n)
	}
	conn, err := pgx.Connect(t.Context(), URL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	var left int
	if err := conn.QueryRow(t.Context(), "SELECT count(*) FROM pg_database WHERE datname = ANY($1)", names).Scan(&left); err != nil || left != 0 {
		t.Errorf("%d copies left after their tests: %v", left, err)
	}
}

// A template dropped meanwhile, as the tests of another branch drop those
// they find too old, is made again.
func TestADroppedTemplateIsMadeAgain(t *testing.T) {
	t.Parallel()
	key, prepare, runs := counted(t)
	Copy(t, key, prepare)
	if err := exec(t.Context(), URL(t), "DROP DATABASE "+pgx.Identifier{templatePrefix + key}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if n := rows(t, Copy(t, key, prepare)); n != 1 || runs.Load() != 2 {
		t.Errorf("after the template was dropped: %d rows, prepared %d times", n, runs.Load())
	}
}

// A failed preparation leaves no template behind: the next test prepares
// again.
func TestAFailedPreparationIsTriedAgain(t *testing.T) {
	t.Parallel()
	key, prepare, runs := counted(t)
	failing := func(ctx context.Context, pool *pgxpool.Pool) error {
		runs.Add(1)
		_, err := pool.Exec(ctx, "SELECT * FROM missing")
		return err
	}
	if err := makeTemplate(t.Context(), URL(t), templatePrefix+key, failing); err == nil {
		t.Fatal("a failed preparation made a template")
	}
	if n := rows(t, Copy(t, key, prepare)); n != 1 || runs.Load() != 2 {
		t.Errorf("after a failed preparation: %d rows, prepared %d times", n, runs.Load())
	}
}

// Making a template drops the copies stopped test binaries left behind,
// once dated more than templateLife ago and used by no connection. A copy
// without a date, from before copies were dated, is kept.
func TestOldCopiesAreSwept(t *testing.T) {
	t.Parallel()
	url := URL(t)
	now := time.Now()
	old := now.Add(-templateLife - time.Hour)
	made := func(at time.Time) string {
		name := copyPrefix + random()
		ident := pgx.Identifier{name}.Sanitize()
		statements := []string{"CREATE DATABASE " + ident}
		if !at.IsZero() {
			statements = append(statements, dated(ident, at))
		}
		if err := exec(t.Context(), url, statements...); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := exec(context.Background(), url, "DROP DATABASE IF EXISTS "+ident+" WITH (FORCE)"); err != nil {
				t.Errorf("drop %s: %v", name, err)
			}
		})
		return name
	}
	stale := made(old)
	recent := made(now.Add(-time.Hour))
	// Dated old only once connected to: tests making their templates
	// meanwhile, in this binary or another, sweep the old copies no
	// connection uses, and would drop it first.
	used := made(time.Time{})
	undated := made(time.Time{})
	config, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.Database = used
	conn, err := pgx.ConnectConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	if err := exec(t.Context(), url, dated(pgx.Identifier{used}.Sanitize(), old)); err != nil {
		t.Fatal(err)
	}

	key, prepare, _ := counted(t)
	pool := Copy(t, key, prepare)
	var date string
	if err := pool.QueryRow(t.Context(), "SELECT coalesce(shobj_description(oid, 'pg_database'), '') FROM pg_database WHERE datname = current_database()").Scan(&date); err != nil {
		t.Fatal(err)
	}
	if at, err := time.Parse(time.RFC3339, date); err != nil || at.Before(now.Add(-time.Minute)) || at.After(time.Now().Add(time.Minute)) {
		t.Errorf("the copy is dated %q, not when it was made: %v", date, err)
	}

	found, _ := conn.Query(t.Context(), "SELECT datname FROM pg_database WHERE datname = ANY($1)", []string{stale, recent, used, undated})
	left, err := pgx.CollectRows(found, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(left, stale) {
		t.Error("an old copy no connection uses was kept")
	}
	for name, which := range map[string]string{recent: "a recent copy", used: "an old copy in use", undated: "a copy without a date"} {
		if !slices.Contains(left, name) {
			t.Errorf("%s was dropped", which)
		}
	}
}
