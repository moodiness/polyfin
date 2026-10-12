// Package migrated gives tests a database of their own, migrated to the
// schema this build ships.
package migrated

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/testdb"
)

// New returns a pool on a migrated database, dropped when the test ends. It
// is a copy of a template database migrated once for every test binary and
// run, as long as the migrations do not change.
func New(t testing.TB) *pgxpool.Pool {
	t.Helper()
	fingerprint, err := database.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	return testdb.Copy(t, fingerprint[:16], database.Migrate)
}
