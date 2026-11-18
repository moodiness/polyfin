package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/database"
)

// A restore takes from a backup the rows of Polyfin's tables and the
// values of its sequences, and nothing else. They go into the schema
// Polyfin's own migrations make, those the backup tells it applied: no
// table, function, trigger, default nor index is made from the backup,
// and none of its statements is sent to the database. Whatever the file
// holds, the database only ever runs statements Polyfin wrote, on names
// it checked against that schema.

// appliedIn reads the migrations the backup at path applied, their
// checksums by version.
func (s *Service) appliedIn(ctx context.Context, path string) (map[int]string, error) {
	data, err := s.pgRestore(ctx, "--data-only", "--file=-", "--table=schema_migrations", path)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotABackup, err)
	}
	applied := map[int]string{}
	err = newScript(bytes.NewReader(data)).eachRow(func(table string, row map[string]string) error {
		version, err := strconv.Atoi(row["version"])
		if table != "schema_migrations" || err != nil {
			return ErrNotABackup
		}
		applied[version] = row["checksum"]
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotABackup, err)
	}
	if len(applied) == 0 {
		return nil, ErrNotABackup
	}
	return applied, nil
}

// builtRelation selects, as c, the relations of public that the database
// user owns and no extension does: those dropAll drops, so, after it,
// those the migrations made.
const builtRelation = `c.relnamespace = 'public'::regnamespace
	AND c.relowner = (SELECT oid FROM pg_roles WHERE rolname = current_user)
	AND NOT EXISTS (SELECT FROM pg_depend d WHERE d.classid = 'pg_class'::regclass AND d.objid = c.oid AND d.deptype = 'e')`

// restoreSchema is the schema a restore made with Polyfin's migrations,
// ready for a backup's rows.
type restoreSchema struct {
	// tables holds, by table, the columns a backup may fill; sequences
	// the sequences it may set.
	tables    map[string]map[string]bool
	sequences map[string]bool
	// after holds the statements putting back the foreign keys and the
	// triggers set aside while the rows load.
	after []string
}

// buildSchema makes, in tx, whose database dropAll emptied, the schema of
// a Polyfin that applied the migrations applied, and readies it for a
// backup's rows:
//   - the rows the migrations added are deleted, and the sequences start
//     again: the backup holds the rows and the values to have;
//   - the foreign keys are dropped: a backup's tables come in no order
//     they would accept. PostgreSQL checks each of them over the whole
//     tables when finish adds them back, as it wrote them from this
//     schema;
//   - the triggers are turned off: they answer what Polyfin changes,
//     and a backup's rows were answered when they were first written.
//
// Owning the tables is enough for each of these.
func buildSchema(ctx context.Context, tx pgx.Tx, applied map[int]string) (*restoreSchema, error) {
	// pg_restore's script lifts the session's timeouts before its
	// statements: none of it is run, so Polyfin lifts them, for the
	// transaction, where the server has them.
	if _, err := tx.Exec(ctx, `SELECT set_config(name, '0', true) FROM pg_settings
		WHERE name IN ('statement_timeout', 'lock_timeout', 'idle_in_transaction_session_timeout', 'transaction_timeout')`); err != nil {
		return nil, err
	}
	if err := database.MigrateAs(ctx, tx, applied); err != nil {
		return nil, err
	}
	schema := &restoreSchema{tables: map[string]map[string]bool{}, sequences: map[string]bool{}}
	// A generated column is computed, never filled.
	rows, _ := tx.Query(ctx, `SELECT c.relname::text, a.attname::text FROM pg_class c JOIN pg_attribute a ON a.attrelid = c.oid
		WHERE c.relkind = 'r' AND a.attnum > 0 AND NOT a.attisdropped AND a.attgenerated = '' AND `+builtRelation)
	var table, column string
	if _, err := pgx.ForEachRow(rows, []any{&table, &column}, func() error {
		if schema.tables[table] == nil {
			schema.tables[table] = map[string]bool{}
		}
		schema.tables[table][column] = true
		return nil
	}); err != nil {
		return nil, err
	}
	rows, _ = tx.Query(ctx, `SELECT c.relname::text FROM pg_class c WHERE c.relkind = 'S' AND `+builtRelation)
	var sequence string
	if _, err := pgx.ForEachRow(rows, []any{&sequence}, func() error {
		schema.sequences[sequence] = true
		return nil
	}); err != nil {
		return nil, err
	}
	rows, _ = tx.Query(ctx, `
		SELECT format('ALTER TABLE public.%I DROP CONSTRAINT %I', c.relname, k.conname),
			format('ALTER TABLE public.%I ADD CONSTRAINT %I %s', c.relname, k.conname, pg_get_constraintdef(k.oid))
		FROM pg_constraint k JOIN pg_class c ON c.oid = k.conrelid
		WHERE k.contype = 'f' AND k.conparentid = 0 AND `+builtRelation+`
		UNION ALL
		SELECT format('ALTER TABLE public.%I DISABLE TRIGGER %I', c.relname, t.tgname),
			format('ALTER TABLE public.%I ENABLE %sTRIGGER %I', c.relname,
				CASE t.tgenabled WHEN 'R' THEN 'REPLICA ' WHEN 'A' THEN 'ALWAYS ' ELSE '' END, t.tgname)
		FROM pg_trigger t JOIN pg_class c ON c.oid = t.tgrelid
		WHERE NOT t.tgisinternal AND t.tgenabled <> 'D' AND `+builtRelation+`
		ORDER BY 1`)
	var before []string
	var aside, back string
	if _, err := pgx.ForEachRow(rows, []any{&aside, &back}, func() error {
		before, schema.after = append(before, aside), append(schema.after, back)
		return nil
	}); err != nil {
		return nil, err
	}
	for _, statement := range before {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return nil, err
		}
	}
	if len(schema.tables) > 0 {
		names := make([]string, 0, len(schema.tables))
		for table := range schema.tables {
			names = append(names, pgx.Identifier{"public", table}.Sanitize())
		}
		if _, err := tx.Exec(ctx, "TRUNCATE "+strings.Join(names, ", ")+" RESTART IDENTITY"); err != nil {
			return nil, err
		}
	}
	return schema, nil
}

// load puts into schema what the script holds, on tx: the rows of each
// COPY block, with a COPY statement of Polyfin's own naming a table and
// columns the schema has, and the value of each of its sequences. A
// script asking for anything else is not a backup of Polyfin's.
func (s *script) load(ctx context.Context, tx pgx.Tx, schema *restoreSchema) error {
	loaded := map[string]bool{}
	for {
		next, err := s.next()
		if errors.Is(err, errScript) {
			return fmt.Errorf("%w: %w", ErrNotABackup, err)
		}
		if err != nil {
			return err
		}
		switch next.kind {
		case scriptEnd:
			return nil
		case scriptEncoding:
			// The rows are in the encoding the script names, which
			// PostgreSQL converts from: it knows the name, or fails.
			if _, err := tx.Exec(ctx, "SELECT set_config('client_encoding', $1, true)", next.name); err != nil {
				return err
			}
		case scriptSequence:
			if !schema.sequences[next.name] {
				return fmt.Errorf("%w: it sets %q, which is no sequence of Polyfin's", ErrNotABackup, next.name)
			}
			if _, err := tx.Exec(ctx, "SELECT setval($1::text::regclass, $2, $3)",
				pgx.Identifier{"public", next.name}.Sanitize(), next.value, next.called); err != nil {
				return fmt.Errorf("restore %s: %w", next.name, err)
			}
		case scriptCopy:
			columns, known := schema.tables[next.name]
			if !known {
				return fmt.Errorf("%w: it holds rows of %q, which is no table of Polyfin's", ErrNotABackup, next.name)
			}
			if loaded[next.name] {
				return fmt.Errorf("%w: it holds the rows of %q twice", ErrNotABackup, next.name)
			}
			loaded[next.name] = true
			names := make([]string, len(next.columns))
			for i, column := range next.columns {
				if !columns[column] {
					return fmt.Errorf("%w: it holds a column %q of %q, which Polyfin's has not", ErrNotABackup, column, next.name)
				}
				names[i] = pgx.Identifier{column}.Sanitize()
			}
			statement := "COPY " + pgx.Identifier{"public", next.name}.Sanitize() + " (" + strings.Join(names, ", ") + ") FROM STDIN"
			if _, err := tx.Conn().PgConn().CopyFrom(ctx, &copyData{s: s}, statement); err != nil {
				return fmt.Errorf("restore %s: %w", next.name, err)
			}
		}
	}
}

// finish puts back, once the rows are loaded, the foreign keys and the
// triggers buildSchema set aside, and checks that the rows loaded tell of
// the migrations applied, those the schema was made with: Migrate goes on
// from them.
func (r *restoreSchema) finish(ctx context.Context, tx pgx.Tx, applied map[int]string) error {
	// A deferred check still pending would refuse the changes of its
	// table below: they all run now, as they would at the commit.
	if _, err := tx.Exec(ctx, "SET CONSTRAINTS ALL IMMEDIATE"); err != nil {
		return err
	}
	// Whatever encoding the rows were in, the statements are Polyfin's.
	if _, err := tx.Exec(ctx, "SELECT set_config('client_encoding', 'UTF8', true)"); err != nil {
		return err
	}
	for _, statement := range r.after {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return err
		}
	}
	rows, _ := tx.Query(ctx, "SELECT version, checksum FROM schema_migrations")
	loaded := map[int]string{}
	var version int
	var checksum string
	if _, err := pgx.ForEachRow(rows, []any{&version, &checksum}, func() error {
		loaded[version] = checksum
		return nil
	}); err != nil {
		return err
	}
	if !maps.Equal(loaded, applied) {
		return fmt.Errorf("%w: its migrations are not those it told of", ErrNotABackup)
	}
	return nil
}
