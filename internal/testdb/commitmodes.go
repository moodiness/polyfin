package testdb

import (
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WatchCommitModes has each statement that inserts or updates rows of the
// tables, in the database of a pool of Copy or Empty, note the
// synchronous_commit its transaction runs with, read by a trigger in that
// transaction, which is the setting its commit follows. It returns what
// was noted of a table so far, in order.
func WatchCommitModes(t testing.TB, pool *pgxpool.Pool, tables ...string) func(table string) []string {
	t.Helper()
	ctx := t.Context()
	statements := []string{
		"CREATE TABLE IF NOT EXISTS test_commit_modes (n bigserial PRIMARY KEY, table_name text NOT NULL, mode text NOT NULL)",
		`CREATE OR REPLACE FUNCTION test_note_commit_mode() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			INSERT INTO test_commit_modes (table_name, mode) VALUES (TG_TABLE_NAME, current_setting('synchronous_commit'));
			RETURN NULL;
		END $$`,
	}
	for _, table := range tables {
		statements = append(statements, "CREATE TRIGGER test_note_commit_mode AFTER INSERT OR UPDATE ON "+pgx.Identifier{table}.Sanitize()+
			" FOR EACH STATEMENT EXECUTE FUNCTION test_note_commit_mode()")
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	return func(table string) []string {
		t.Helper()
		rows, err := pool.Query(t.Context(), "SELECT mode FROM test_commit_modes WHERE table_name = $1 ORDER BY n", table)
		if err != nil {
			t.Fatal(err)
		}
		modes, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatal(err)
		}
		return modes
	}
}
