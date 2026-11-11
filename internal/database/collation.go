package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Collation is how the collation version of the server's database stands:
// the version PostgreSQL recorded when the database was made or last
// refreshed, and the one the system's collation library provides now.
// After an update that brings another version of that library, text
// indexes built under the old one may not follow the new order: lookups
// then miss rows, and unique indexes let duplicates in.
type Collation struct {
	// Database is the database's name.
	Database string
	// Provider is its locale provider: "libc", "icu" or "builtin".
	Provider string
	// Recorded is the version recorded, empty when none is, and Actual
	// the version the provider gives now, empty when it gives none, as
	// for the C and POSIX locales and on some systems.
	Recorded string
	Actual   string
}

// Changed tells whether the collation version changed since it was
// recorded: only when both versions are known and differ, so that a
// database without a recorded version, or a provider that gives none, is
// never taken for a changed one.
func (c Collation) Changed() bool {
	return c.Recorded != "" && c.Actual != "" && c.Recorded != c.Actual
}

// ReadCollation reads how the collation version of the database db is
// connected to stands, from the catalog: one cheap query.
func ReadCollation(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) (Collation, error) {
	var c Collation
	err := db.QueryRow(ctx, `SELECT datname,
			CASE datlocprovider WHEN 'c' THEN 'libc' WHEN 'i' THEN 'icu' WHEN 'b' THEN 'builtin' ELSE datlocprovider::text END,
			coalesce(datcollversion, ''), coalesce(pg_database_collation_actual_version(oid), '')
		FROM pg_database WHERE datname = current_database()`).Scan(&c.Database, &c.Provider, &c.Recorded, &c.Actual)
	if err != nil {
		return Collation{}, fmt.Errorf("read the database's collation version: %w", err)
	}
	return c, nil
}
