package database

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/moodiness/polyfin/internal/testdb"
)

func step(version int, name, sql string) migration {
	return migration{version: version, name: name, sql: sql, checksum: name + ":" + sql}
}

func TestMigrateCreatesAStableServerIdentity(t *testing.T) {
	pool := testdb.New(t)
	ctx := t.Context()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	first, err := ServerID(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(first) {
		t.Fatalf("server ID %q is not 32 lowercase hex characters", first)
	}
	// A restart runs Migrate again: the identity clients know must survive it.
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if again, _ := ServerID(ctx, pool); again != first {
		t.Fatalf("server ID changed from %s to %s", first, again)
	}
}

func TestMigrateRefusesAnEditedMigration(t *testing.T) {
	pool := testdb.New(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, []migration{step(1, "a", "CREATE TABLE a (x int)")}); err != nil {
		t.Fatal(err)
	}
	err := migrate(ctx, pool, []migration{step(1, "a", "CREATE TABLE a (x bigint)")})
	if err == nil || !strings.Contains(err.Error(), "changed after it was applied") {
		t.Fatalf("expected an edited-migration error, got %v", err)
	}
}

func TestMigrateRefusesASchemaFromANewerBuild(t *testing.T) {
	pool := testdb.New(t)
	ctx := t.Context()
	newer := []migration{step(1, "a", "CREATE TABLE a (x int)"), step(2, "b", "CREATE TABLE b (x int)")}
	if err := migrate(ctx, pool, newer); err != nil {
		t.Fatal(err)
	}
	err := migrate(ctx, pool, newer[:1])
	if err == nil || !strings.Contains(err.Error(), "upgrade Polyfin") {
		t.Fatalf("expected a downgrade error, got %v", err)
	}
}

func TestMigrateRollsBackAFailedMigration(t *testing.T) {
	pool := testdb.New(t)
	ctx := t.Context()
	err := migrate(ctx, pool, []migration{
		step(1, "a", "CREATE TABLE a (x int)"),
		step(2, "b", "CREATE TABLE b (x int); SELECT * FROM missing"),
	})
	if err == nil || !strings.Contains(err.Error(), "0002_b") {
		t.Fatalf("expected migration 0002 to fail, got %v", err)
	}
	var applied int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&applied); err != nil {
		t.Fatal(err)
	}
	var tableB *string
	if err := pool.QueryRow(ctx, "SELECT to_regclass('b')::text").Scan(&tableB); err != nil {
		t.Fatal(err)
	}
	if applied != 1 || tableB != nil {
		t.Fatalf("partial migration kept: %d applied, table b = %v", applied, tableB)
	}
}

func TestConcurrentStartsApplyEachMigrationOnce(t *testing.T) {
	pool := testdb.New(t)
	ctx := t.Context()
	// Not idempotent: a second application would fail.
	migrations := []migration{step(1, "a", "CREATE TABLE a (x int)")}
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Go(func() { errs[i] = migrate(ctx, pool, migrations) })
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMigrationsRejectsBadSets(t *testing.T) {
	file := &fstest.MapFile{Data: []byte("SELECT 1")}
	for name, files := range map[string]fstest.MapFS{
		"gap":       {"0001_a.sql": file, "0003_c.sql": file},
		"duplicate": {"0001_a.sql": file, "0001_b.sql": file},
		"not first": {"0002_b.sql": file},
		"bad name":  {"0001_a.sql": file, "2_b.sql": file},
	} {
		if _, err := loadMigrations(files); err == nil {
			t.Errorf("%s: migration set was accepted", name)
		}
	}
}

func TestConnectFailsFastOnRejectedCredentials(t *testing.T) {
	parsed, err := url.Parse(testdb.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	parsed.User = url.UserPassword(parsed.User.Username(), "not-the-password")
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	_, err = Connect(ctx, parsed.String(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected an immediate authentication error, got %v", err)
	}
}

func TestConnectStopsWaitingWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 1500*time.Millisecond)
	defer cancel()
	_, err := Connect(ctx, "postgresql://polyfin@127.0.0.1:1/polyfin?connect_timeout=1",
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected the wait to end with the context, got %v", err)
	}
}
