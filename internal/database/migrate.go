package database

import (
	"cmp"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var embedded embed.FS

// migrationLock is the advisory lock key held while migrating, so several
// Polyfin processes starting together apply each migration once.
const migrationLock int64 = 0x706f6c7966696e // "polyfin"

var migrationName = regexp.MustCompile(`^([0-9]{4})_([a-z0-9_]+)\.sql$`)

// superseded lists, by version, the checksums of migrations as earlier
// releases shipped them, before they were edited: a database that applied
// one is accepted, and Migrate records it as this build ships it. An edit
// listed here must leave the schema the earlier one did, wherever the
// earlier one succeeded.
var superseded = map[int][]string{
	// 0239_catalog_renames as Polyfin 0.72.0 shipped it, which failed on
	// databases holding rows its keys do not match: the current one
	// deletes them first, and deletes nothing where it succeeded.
	239: {"31eea78c9bad0c4fa99bd34e4b3b4a8a2119deb606af69b1bfff92fb8912054b"},
}

type migration struct {
	version  int
	name     string
	sql      string
	checksum string
}

// Migrate brings the schema to the version this build ships, logging into
// logger while it waits for another Polyfin migrating the same database,
// each migration it applies, and the sessions a migration waits for.
//
// Each migration runs in its own transaction. Polyfin refuses to start when a
// migration it already applied was edited, unless superseded lists the
// checksum it was applied with, or when the database was migrated by a
// newer Polyfin: both would leave the schema in a state the code does not
// expect.
func Migrate(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	migrations, err := shippedMigrations()
	if err != nil {
		return err
	}
	return migrator{logger: logger}.migrate(ctx, pool, migrations)
}

// The pace of Migrate's logs. While another Polyfin holds the migration
// lock, Migrate tries it again every lockPoll, and says that it waits at
// once, then every waitLog. A migration still running blockedAfter its
// start is looked at for the sessions whose locks it waits for, then
// every waitLog.
const (
	lockPoll     = time.Second
	waitLog      = 30 * time.Second
	blockedAfter = 10 * time.Second
)

// migrator applies migrations, logging into logger. Its zero durations
// stand for lockPoll, waitLog and blockedAfter; tests set shorter ones.
type migrator struct {
	logger                    *slog.Logger
	poll, every, blockedAfter time.Duration
}

// Fingerprint identifies the schema Migrate brings a database to: it
// changes with every migration added or edited. Tests name the database
// they copy migrated databases from after it.
func Fingerprint() (string, error) {
	migrations, err := shippedMigrations()
	if err != nil {
		return "", err
	}
	sum := sha256.New()
	for _, m := range migrations {
		fmt.Fprintf(sum, "%d %s %s\n", m.version, m.name, m.checksum)
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func shippedMigrations() ([]migration, error) {
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		return nil, err
	}
	return loadMigrations(files)
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

// migrate applies the migrations the database lacks, under the migration
// lock (see lock). A database that applied none, being created, logs each
// migration at Debug, and one line at Info once done; an upgrade logs
// each at Info. Once it applied any, the pool's connections are opened
// again, for their listing statements to be prepared on the schema as it
// is now (see prepareListings).
func (r migrator) migrate(ctx context.Context, pool *pgxpool.Pool, migrations []migration) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if err := r.lock(ctx, conn); err != nil {
		return fmt.Errorf("lock the schema: %w", err)
	}
	defer func() {
		if _, unlockErr := conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", migrationLock); unlockErr != nil {
			// A pooled connection must not keep the lock: discard it.
			_ = conn.Conn().Close(context.WithoutCancel(ctx))
		}
	}()

	if _, err := conn.Exec(ctx, createSchemaMigrations); err != nil {
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

	if err := checkApplied(applied, migrations); err != nil {
		return err
	}
	// What checkApplied accepted but differs is a superseded checksum.
	for _, m := range migrations {
		if checksum, done := applied[m.version]; done && checksum != m.checksum {
			if _, err := conn.Exec(ctx, "UPDATE schema_migrations SET checksum = $2 WHERE version = $1", m.version, m.checksum); err != nil {
				return fmt.Errorf("record migration %04d_%s as this build ships it: %w", m.version, m.name, err)
			}
		}
	}

	pending := 0
	for _, m := range migrations {
		if _, done := applied[m.version]; !done {
			pending++
		}
	}
	if pending == 0 {
		return nil
	}
	level := slog.LevelInfo
	if len(applied) == 0 {
		level = slog.LevelDebug
	}
	r.logger.Info("Migrating the database", "pending", pending, "applied", len(applied))
	began := time.Now()
	for _, m := range migrations {
		if _, done := applied[m.version]; done {
			continue
		}
		name := fmt.Sprintf("%04d_%s", m.version, m.name)
		start := time.Now()
		stop := r.watchLocks(ctx, pool, conn.Conn().PgConn().PID(), name)
		err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, m.sql); err != nil {
				return err
			}
			_, err := tx.Exec(ctx,
				"INSERT INTO schema_migrations (version, name, checksum) VALUES ($1, $2, $3)",
				m.version, m.name, m.checksum)
			return err
		})
		stop()
		if err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		r.logger.Log(ctx, level, "Applied a database migration", "migration", name, "duration", time.Since(start).Round(time.Millisecond))
	}
	r.logger.Info("The database is migrated", "migrations", pending, "duration", time.Since(began).Round(time.Millisecond))
	pool.Reset()
	return nil
}

// lock takes the migration lock on conn, the session migrating. While
// another session holds it, another Polyfin migrating the same database
// as a rule, lock tries again every poll until ctx ends, and logs that it
// waits, with the session holding the lock, at once and then every
// waitLog.
func (r migrator) lock(ctx context.Context, conn *pgxpool.Conn) error {
	began := time.Now()
	var logged time.Time
	for {
		var locked bool
		if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", migrationLock).Scan(&locked); err != nil {
			return err
		}
		if locked {
			if !logged.IsZero() {
				r.logger.Info("Another Polyfin finished migrating", "waited", time.Since(began).Round(time.Second))
			}
			return nil
		}
		if logged.IsZero() || time.Since(logged) >= cmp.Or(r.every, waitLog) {
			attrs := []any{"waited", time.Since(began).Round(time.Second)}
			// The session holding the lock; none when it let go meanwhile.
			var pid int32
			var host string
			if conn.QueryRow(ctx, `SELECT l.pid, coalesce(host(a.client_addr), '')
				FROM pg_locks l LEFT JOIN pg_stat_activity a ON a.pid = l.pid
				WHERE l.locktype = 'advisory' AND l.granted AND l.objsubid = 1
					AND l.database = (SELECT oid FROM pg_database WHERE datname = current_database())
					AND l.classid::bigint = $1 AND l.objid::bigint = $2`,
				migrationLock>>32, migrationLock&0xffffffff).Scan(&pid, &host) == nil {
				attrs = append(attrs, "holder_pid", pid)
				if host != "" {
					attrs = append(attrs, "holder_host", host)
				}
			}
			r.logger.Info("Waiting for another Polyfin to finish migrating", attrs...)
			logged = time.Now()
		}
		timer := time.NewTimer(cmp.Or(r.poll, lockPoll))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// watchLocks looks, blockedAfter the migration name started and then
// every waitLog until stop, for the sessions holding the locks the
// migration, run by the backend pid, waits for, as pg_blocking_pids
// tells, and logs them: their pid, application, state and host, never
// their statements. stop returns once it no longer looks.
func (r migrator) watchLocks(ctx context.Context, pool *pgxpool.Pool, pid uint32, name string) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	began := time.Now()
	go func() {
		defer close(done)
		timer := time.NewTimer(cmp.Or(r.blockedAfter, blockedAfter))
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			rows, _ := pool.Query(ctx, `SELECT a.pid, coalesce(a.application_name, ''), coalesce(a.state, ''), coalesce(host(a.client_addr), '')
				FROM pg_stat_activity a WHERE a.pid = ANY(pg_blocking_pids($1)) ORDER BY a.pid`, int64(pid))
			var blocking []string
			var holder int32
			var application, state, host string
			_, err := pgx.ForEachRow(rows, []any{&holder, &application, &state, &host}, func() error {
				session := fmt.Sprintf("pid %d", holder)
				for _, detail := range []string{application, state, host} {
					if detail != "" {
						session += ", " + detail
					}
				}
				blocking = append(blocking, session)
				return nil
			})
			if err == nil && len(blocking) > 0 {
				r.logger.Warn("A database migration waits for locks other sessions hold", "migration", name,
					"waited", time.Since(began).Round(time.Second), "blocking", blocking)
			}
			timer.Reset(cmp.Or(r.every, waitLog))
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

// createSchemaMigrations makes the table recording the migrations applied.
const createSchemaMigrations = `CREATE TABLE IF NOT EXISTS schema_migrations (
		version integer PRIMARY KEY,
		name text NOT NULL,
		checksum text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`

// MigrateAs makes, in tx's transaction, the schema of a database that
// applied the migrations applied, their checksums by version, as
// CheckApplied takes them: it applies those migrations as this build
// ships them, in order, to a database holding none of Polyfin's tables,
// and records them. A restore makes the schema of a backup with it,
// before it loads the backup's rows, so that a failure rolls the whole
// restore back; Migrate applies the other migrations afterwards.
func MigrateAs(ctx context.Context, tx pgx.Tx, applied map[int]string) error {
	migrations, err := shippedMigrations()
	if err != nil {
		return err
	}
	if err := checkApplied(applied, migrations); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, createSchemaMigrations); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	for _, m := range migrations {
		if _, done := applied[m.version]; !done {
			continue
		}
		if _, err := tx.Exec(ctx, m.sql); err != nil {
			return fmt.Errorf("apply migration %04d_%s: %w", m.version, m.name, err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version, name, checksum) VALUES ($1, $2, $3)",
			m.version, m.name, m.checksum); err != nil {
			return fmt.Errorf("apply migration %04d_%s: %w", m.version, m.name, err)
		}
	}
	return nil
}

// ErrNewerSchema reports a database a newer Polyfin migrated, and
// ErrChangedMigration one whose applied migrations differ from those this
// build ships: Polyfin refuses to start on either.
var (
	ErrNewerSchema      = errors.New("the database schema is newer than this Polyfin build")
	ErrChangedMigration = errors.New("a migration was changed after it was applied")
)

// schemaError tells what differs, and is one of the errors above.
type schemaError struct {
	kind    error
	message string
}

func (e *schemaError) Error() string { return e.message }
func (e *schemaError) Unwrap() error { return e.kind }

// CheckApplied checks the migrations a database applied, their checksums by
// version as schema_migrations holds them, against those this build
// ships, as Migrate does before migrating, superseded checksums accepted:
// it fails with ErrNewerSchema or ErrChangedMigration, saying what
// differs.
func CheckApplied(applied map[int]string) error {
	migrations, err := shippedMigrations()
	if err != nil {
		return err
	}
	return checkApplied(applied, migrations)
}

// checkApplied checks the applied versions in order, so that its error
// names the same version from one start to the next: the newest when the
// database is ahead of this build, else the first changed migration.
func checkApplied(applied map[int]string, migrations []migration) error {
	known := len(migrations)
	versions := slices.Sorted(maps.Keys(applied))
	if len(versions) > 0 && versions[len(versions)-1] > known {
		return &schemaError{ErrNewerSchema, fmt.Sprintf("the database schema is at version %d but this Polyfin build only knows %d: upgrade Polyfin",
			versions[len(versions)-1], known)}
	}
	for _, version := range versions {
		if version < 1 {
			return &schemaError{ErrNewerSchema,
				fmt.Sprintf("the database schema is at version %d but this Polyfin build only knows %d: upgrade Polyfin", version, known)}
		}
		if m := migrations[version-1]; m.checksum != applied[version] && !slices.Contains(superseded[version], applied[version]) {
			return &schemaError{ErrChangedMigration, fmt.Sprintf("migration %04d_%s was changed after it was applied to the database", m.version, m.name)}
		}
	}
	return nil
}
