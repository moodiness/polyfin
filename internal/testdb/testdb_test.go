package testdb

import (
	"context"
	"sync/atomic"
	"testing"

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
