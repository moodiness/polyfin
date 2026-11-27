package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The migration keeps the entries written before as they were, without a
// phrase, and keeps a phrase only as a JSON object.
func TestActivityPhrasesMigrationKeepsTheEntriesWrittenBefore(t *testing.T) {
	t.Parallel()
	pool := testdb.Empty(t)
	ctx := t.Context()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "activity_phrases" })
	if at < 0 {
		t.Fatal("no activity_phrases migration")
	}
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO activity_log (name, short_overview, type) VALUES ('Connexion de Sam', 'Adresse : 192.0.2.1', 'AuthenticationSucceeded')"); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	var name, short string
	var phrases int
	if err := pool.QueryRow(ctx, "SELECT name, short_overview, num_nonnulls(name_phrase, short_overview_phrase) FROM activity_log").Scan(&name, &short, &phrases); err != nil {
		t.Fatal(err)
	}
	if name != "Connexion de Sam" || short != "Adresse : 192.0.2.1" || phrases != 0 {
		t.Errorf("an entry from before: %q, %q, %d phrases", name, short, phrases)
	}
	for _, phrase := range []string{`"signedIn"`, `[]`, `null`} {
		if _, err := pool.Exec(ctx, "INSERT INTO activity_log (name, type, name_phrase) VALUES ('x', 'x', $1::jsonb)", phrase); err == nil {
			t.Errorf("kept the phrase %s", phrase)
		}
		if _, err := pool.Exec(ctx, "INSERT INTO activity_log (name, type, short_overview_phrase) VALUES ('x', 'x', $1::jsonb)", phrase); err == nil {
			t.Errorf("kept the short overview phrase %s", phrase)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO activity_log (name, type, name_phrase, short_overview_phrase)
		VALUES ('x', 'x', '{"key": "signedIn", "args": [{"text": "Sam"}]}', '{"key": "address", "args": [{"text": "192.0.2.1"}]}')`); err != nil {
		t.Fatal(err)
	}
}
