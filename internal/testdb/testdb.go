// Package testdb gives tests an isolated PostgreSQL schema.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
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

// New returns a pool whose search_path is a fresh schema, dropped when the
// test ends, so tests never see each other's tables.
func New(t testing.TB) *pgxpool.Pool {
	t.Helper()
	url := URL(t)
	ctx := t.Context()

	suffix := make([]byte, 8)
	_, _ = rand.Read(suffix)
	schema := pgx.Identifier{"test_" + hex.EncodeToString(suffix)}.Sanitize()

	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect to the test database: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		cleanup := context.Background()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
		_ = admin.Close(cleanup)
	})

	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
