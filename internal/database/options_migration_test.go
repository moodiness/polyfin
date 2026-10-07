package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/testdb"
)

// optionsMigration names the migration that changes the defaults of the
// settings, removes chapters and downloads, and marks the settings
// POLYFIN_HWACCEL and POLYFIN_SEGMENTS still have to give.
const optionsMigration = "options_and_addon_changes"

// laterOptions are the settings a later migration (settings_from_environment)
// adds to those the environment still has to give.
var laterOptions = []string{"cache_size_gb", "vaapi_device", "recording", "backups", "detailed_log"}

// beforeOptions migrates a fresh schema up to the migration changing the
// options, runs prepare on it, then applies the rest.
func beforeOptions(t *testing.T, prepare string) *pgxpool.Pool {
	t.Helper()
	pool := testdb.New(t)
	ctx := t.Context()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == optionsMigration })
	if at < 0 {
		t.Fatalf("no migration named %s", optionsMigration)
	}
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, prepare); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	return pool
}

// migratedOptions are the settings the options migration changes.
type migratedOptions struct {
	prepareAhead          bool
	analysisTimeout       int
	catalogRefreshMinutes int
	hardware              string
	order, off, pending   []string
}

func readOptions(t *testing.T, pool *pgxpool.Pool) migratedOptions {
	t.Helper()
	var o migratedOptions
	if err := pool.QueryRow(t.Context(), "SELECT prepare_ahead, analysis_timeout, catalog_refresh_minutes, hardware_acceleration, "+
		"segment_order, segment_sources_off, environment_pending FROM settings").
		Scan(&o.prepareAhead, &o.analysisTimeout, &o.catalogRefreshMinutes, &o.hardware, &o.order, &o.off, &o.pending); err != nil {
		t.Fatal(err)
	}
	return o
}

// downloading returns the users who may download, by name.
func downloading(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, _ := pool.Query(t.Context(), "SELECT name FROM users WHERE content_downloading ORDER BY name")
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return names
}

const twoUsers = `INSERT INTO users (name, password_hash) VALUES ('alice', ''), ('bob', '');
	UPDATE users SET content_downloading = false WHERE name = 'bob';`

func TestTheOptionsMigrationMovesTheOldDefaults(t *testing.T) {
	// A server on every old default: downloads off for everyone, the GPU
	// and the order of the segment databases left to the environment.
	pool := beforeOptions(t, twoUsers+`UPDATE settings SET downloads = false, chapters = false,
		prepare_ahead = false, analysis_timeout = 45, catalog_refresh_minutes = 10,
		hardware_acceleration = '', segment_order = '{}'`)
	got := readOptions(t, pool)
	if !got.prepareAhead || got.analysisTimeout != 20 || got.catalogRefreshMinutes != 60 {
		t.Errorf("defaults: prepare %v, analysis %d s, catalogs every %d min", got.prepareAhead, got.analysisTimeout, got.catalogRefreshMinutes)
	}
	// Nobody could download: nobody can.
	if names := downloading(t, pool); len(names) != 0 {
		t.Errorf("users who may download: %v", names)
	}
	// The GPU and the order hold the defaults until the environment is
	// copied in, which they wait for, with the databases turned off.
	if got.hardware != "auto" || !slices.Equal(got.order, []string{"theintrodb", "introdb", "publicmetadb"}) || len(got.off) != 0 ||
		!slices.Equal(got.pending, append([]string{"hardware_acceleration", "segment_order", "segment_sources_off"}, laterOptions...)) {
		t.Errorf("before the environment is copied: %+v", got)
	}
	var columns []string
	rows, _ := pool.Query(t.Context(), "SELECT column_name FROM information_schema.columns "+
		"WHERE table_schema = current_schema() AND table_name = 'settings' AND column_name IN ('chapters', 'downloads')")
	for rows.Next() {
		var name string
		_ = rows.Scan(&name)
		columns = append(columns, name)
	}
	if rows.Err() != nil || len(columns) != 0 {
		t.Errorf("columns left: %v %v", columns, rows.Err())
	}
	// '' no longer follows POLYFIN_HWACCEL, and every database is in the
	// order.
	for _, change := range []string{"hardware_acceleration = ''", "segment_order = '{}'",
		"segment_order = '{theintrodb,introdb}'", "segment_sources_off = '{introdb,introdb}'", "segment_sources_off = '{other}'",
		"environment_pending = '{language}'"} {
		if _, err := pool.Exec(t.Context(), "UPDATE settings SET "+change); err == nil {
			t.Errorf("the database took %s", change)
		}
	}
}

func TestTheOptionsMigrationKeepsWhatWasChosen(t *testing.T) {
	pool := beforeOptions(t, twoUsers+`UPDATE settings SET downloads = true,
		analysis_timeout = 30, catalog_refresh_minutes = 15,
		hardware_acceleration = 'vaapi', segment_order = '{publicmetadb,introdb,theintrodb}'`)
	got := readOptions(t, pool)
	if got.analysisTimeout != 30 || got.catalogRefreshMinutes != 15 {
		t.Errorf("chosen values: analysis %d s, catalogs every %d min", got.analysisTimeout, got.catalogRefreshMinutes)
	}
	// Each user keeps their own permission.
	if names := downloading(t, pool); !slices.Equal(names, []string{"alice"}) {
		t.Errorf("users who may download: %v", names)
	}
	// Only the databases turned off are still to come from the
	// environment, which alone chose them.
	if got.hardware != "vaapi" || !slices.Equal(got.order, []string{"publicmetadb", "introdb", "theintrodb"}) ||
		!slices.Equal(got.pending, append([]string{"segment_sources_off"}, laterOptions...)) {
		t.Errorf("chosen GPU and order: %+v", got)
	}
}
