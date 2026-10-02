package database

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var embedded embed.FS

// migrationLock is the advisory lock key held while migrating, so several
// Polyfin processes starting together apply each migration once.
const migrationLock int64 = 0x706f6c7966696e // "polyfin"

var migrationName = regexp.MustCompile(`^([0-9]{4})_([a-z0-9_]+)\.sql$`)

type migration struct {
	version  int
	name     string
	sql      string
	checksum string
}

// Migrate brings the schema to the version this build ships.
//
// Each migration runs in its own transaction. Polyfin refuses to start when a
// migration it already applied was edited, or when the database was migrated
// by a newer Polyfin: both would leave the schema in a state the code does
// not expect.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		return err
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		return err
	}
	return migrate(ctx, pool, migrations)
}

func loadMigrations(files fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, err
	}
	var migrations []migration
	for _, entry := range entries {
		match := migrationName.FindStringSubmatch(entry.Name())
		if match == nil || entry.IsDir() {
			return nil, fmt.Errorf("migration %q is not named NNNN_name.sql", entry.Name())
		}
		body, err := fs.ReadFile(files, entry.Name())
		if err != nil {
			return nil, err
		}
		version, _ := strconv.Atoi(match[1])
		sum := sha256.Sum256(body)
		migrations = append(migrations, migration{
			version:  version,
			name:     match[2],
			sql:      string(body),
			checksum: hex.EncodeToString(sum[:]),
		})
	}
	slices.SortFunc(migrations, func(a, b migration) int { return a.version - b.version })
	for i, m := range migrations {
		if m.version != i+1 {
			return nil, fmt.Errorf("migration versions must run from 0001 without gaps or duplicates; found %04d_%s at position %d", m.version, m.name, i+1)
		}
	}
	return migrations, nil
}

func migrate(ctx context.Context, pool *pgxpool.Pool, migrations []migration) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLock); err != nil {
		return fmt.Errorf("lock the schema: %w", err)
	}
	defer func() {
		if _, unlockErr := conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", migrationLock); unlockErr != nil {
			// A pooled connection must not keep the lock: discard it.
			_ = conn.Conn().Close(context.WithoutCancel(ctx))
		}
	}()

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version integer PRIMARY KEY,
		name text NOT NULL,
		checksum text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	rows, _ := conn.Query(ctx, "SELECT version, checksum FROM schema_migrations")
	applied := map[int]string{}
	var version int
	var checksum string
	if _, err := pgx.ForEachRow(rows, []any{&version, &checksum}, func() error {
		applied[version] = checksum
		return nil
	}); err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}

	known := len(migrations)
	for version, checksum := range applied {
		if version > known || version < 1 {
			return fmt.Errorf("the database schema is at version %d but this Polyfin build only knows %d: upgrade Polyfin", version, known)
		}
		if m := migrations[version-1]; m.checksum != checksum {
			return fmt.Errorf("migration %04d_%s was changed after it was applied to the database", m.version, m.name)
		}
	}

	for _, m := range migrations {
		if _, done := applied[m.version]; done {
			continue
		}
		err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, m.sql); err != nil {
				return err
			}
			_, err := tx.Exec(ctx,
				"INSERT INTO schema_migrations (version, name, checksum) VALUES ($1, $2, $3)",
				m.version, m.name, m.checksum)
			return err
		})
		if err != nil {
			return fmt.Errorf("apply migration %04d_%s: %w", m.version, m.name, err)
		}
	}
	return nil
}
