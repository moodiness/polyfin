package database

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/moodiness/polyfin/internal/testdb"
)

func step(version int, name, sql string) migration {
	return migration{version: version, name: name, sql: sql, checksum: name + ":" + sql}
}

func TestMigrateCreatesAStableServerIdentity(t *testing.T) {
	t.Parallel()
	pool := testdb.Empty(t)
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
	t.Parallel()
	pool := testdb.Empty(t)
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
	t.Parallel()
	pool := testdb.Empty(t)
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

// A database several versions ahead names its newest version, the same at
// every start, and the first of several changed migrations is the one
// named.
func TestSchemaErrorsNameTheSameVersionEachTime(t *testing.T) {
	t.Parallel()
	shipped := []migration{step(1, "a", "SELECT 1"), step(2, "b", "SELECT 2")}
	ahead := map[int]string{1: shipped[0].checksum, 2: shipped[1].checksum}
	for version := 3; version <= 8; version++ {
		ahead[version] = "later"
	}
	changed := map[int]string{1: "edited", 2: "edited"}
	for range 20 {
		if err := checkApplied(ahead, shipped); !errors.Is(err, ErrNewerSchema) ||
			!strings.Contains(err.Error(), "at version 8 but this Polyfin build only knows 2") {
			t.Fatalf("a database at version 8: %v", err)
		}
		if err := checkApplied(changed, shipped); !errors.Is(err, ErrChangedMigration) || !strings.Contains(err.Error(), "0001_a") {
			t.Fatalf("two changed migrations: %v", err)
		}
	}
}

// A database that applied 0239_catalog_renames as Polyfin 0.72.0 shipped
// it is accepted, and recorded as this build ships it; any other checksum
// is still refused.
func TestMigrateRecordsASupersededChecksumAsShipped(t *testing.T) {
	t.Parallel()
	pool := testdb.Empty(t)
	ctx := t.Context()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	shipped, err := shippedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	applied := func() map[int]string {
		t.Helper()
		rows, _ := pool.Query(ctx, "SELECT version, checksum FROM schema_migrations")
		checksums := map[int]string{}
		var version int
		var checksum string
		if _, err := pgx.ForEachRow(rows, []any{&version, &checksum}, func() error {
			checksums[version] = checksum
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return checksums
	}
	const released = "31eea78c9bad0c4fa99bd34e4b3b4a8a2119deb606af69b1bfff92fb8912054b"
	if !slices.Contains(superseded[239], released) || shipped[238].checksum == released {
		t.Fatalf("0239 as 0.72.0 shipped it is not superseded: %q", superseded[239])
	}
	if _, err := pool.Exec(ctx, "UPDATE schema_migrations SET checksum = $1 WHERE version = 239", released); err != nil {
		t.Fatal(err)
	}
	if err := CheckApplied(applied()); err != nil {
		t.Fatalf("CheckApplied with 0.72.0's 0239: %v", err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate with 0.72.0's 0239: %v", err)
	}
	if checksum := applied()[239]; checksum != shipped[238].checksum {
		t.Errorf("0239 recorded as %s, want %s", checksum, shipped[238].checksum)
	}

	if _, err := pool.Exec(ctx, "UPDATE schema_migrations SET checksum = 'edited' WHERE version = 239"); err != nil {
		t.Fatal(err)
	}
	if err := CheckApplied(applied()); !errors.Is(err, ErrChangedMigration) {
		t.Errorf("CheckApplied with an edited 0239: %v", err)
	}
	if err := Migrate(ctx, pool); !errors.Is(err, ErrChangedMigration) || !strings.Contains(err.Error(), "0239_catalog_renames") {
		t.Errorf("Migrate with an edited 0239: %v", err)
	}
}

func TestMigrateRollsBackAFailedMigration(t *testing.T) {
	t.Parallel()
	pool := testdb.Empty(t)
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
	t.Parallel()
	pool := testdb.Empty(t)
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 1500*time.Millisecond)
	defer cancel()
	_, err := Connect(ctx, "postgresql://polyfin@127.0.0.1:1/polyfin?connect_timeout=1",
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected the wait to end with the context, got %v", err)
	}
}

// The pool keeps to maxConns whatever the number of CPUs, and the URL's
// pool_max_conns, in either form, sets another bound. Neither reaches
// PostgreSQL as a setting.
func TestPoolIsCappedUnlessTheURLSaysOtherwise(t *testing.T) {
	t.Parallel()
	for connection, want := range map[string]int32{
		"postgresql://polyfin:secret@db:5432/polyfin":                                   maxConns,
		"postgresql://polyfin:secret@db:5432/polyfin?sslmode=disable":                   maxConns,
		"postgresql://polyfin:secret@db:5432/polyfin?pool_max_conns=3":                  3,
		"postgresql://polyfin:secret@db:5432/polyfin?sslmode=disable&pool_max_conns=40": 40,
		"host=db user=polyfin dbname=polyfin pool_max_conns=5":                          5,
		"host=db user=polyfin dbname=polyfin":                                           maxConns,
	} {
		config, err := poolConfig(connection)
		if err != nil {
			t.Fatalf("%s: %v", connection, err)
		}
		if config.MaxConns != want {
			t.Errorf("%s: MaxConns = %d, want %d", connection, config.MaxConns, want)
		}
		if _, ok := config.ConnConfig.RuntimeParams["pool_max_conns"]; ok {
			t.Errorf("%s: pool_max_conns is sent as a setting", connection)
		}
		if got := config.ConnConfig.RuntimeParams["application_name"]; got != "polyfin" {
			t.Errorf("%s: application_name = %q", connection, got)
		}
	}
	for _, connection := range []string{"postgresql://db/polyfin?pool_max_conns=0", "postgresql://db/polyfin?pool_max_conns=many"} {
		if _, err := poolConfig(connection); err == nil {
			t.Errorf("%s was accepted", connection)
		}
	}
}

// The pool Connect opens has the bound, and a URL's own.
func TestConnectCapsThePool(t *testing.T) {
	t.Parallel()
	address := testdb.URL(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool, err := Connect(t.Context(), address, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if got := pool.Stat().MaxConns(); got != maxConns {
		t.Errorf("MaxConns = %d, want %d", got, maxConns)
	}
	parsed, err := url.Parse(address)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("pool_max_conns", "2")
	parsed.RawQuery = query.Encode()
	small, err := Connect(t.Context(), parsed.String(), logger)
	if err != nil {
		t.Fatal(err)
	}
	defer small.Close()
	if got := small.Stat().MaxConns(); got != 2 {
		t.Errorf("MaxConns with pool_max_conns=2 = %d", got)
	}
}
