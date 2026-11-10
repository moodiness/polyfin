package database

import (
	"context"
	"io"
	"io/fs"
	"log/slog"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/tasks"
	"github.com/moodiness/polyfin/internal/testdb"
)

// The hours of the settings become the triggers of their tasks: an hour a
// daily trigger, while the defaults, -1 and the backup hour 4, write no
// row, so a fresh install keeps every task on its defaults; the columns
// are gone.
func TestTaskTriggersMigrationKeepsTheHours(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "task_triggers" })
	if at < 0 {
		t.Fatal("no task_triggers migration")
	}
	for _, tc := range []struct {
		name                   string
		fresh                  bool
		backup, read, emptying int
		want                   []string
	}{
		{"hours", false, 2, 3, 23, []string{
			`BackUpDatabase [{"time": 120, "type": "daily"}]`,
			`EmptySourceCache [{"time": 1380, "type": "daily"}]`,
			`ReadCollections [{"time": 180, "type": "daily"}]`,
		}},
		{"defaults", false, 4, -1, -1, nil},
		{"fresh install", true, 0, 0, 0, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := testdb.Empty(t)
			ctx := t.Context()
			if err := migrate(ctx, pool, migrations[:at]); err != nil {
				t.Fatal(err)
			}
			// A fresh install keeps the defaults its first migrations gave.
			if !tc.fresh {
				if _, err := pool.Exec(ctx, "UPDATE settings SET backup_hour = $1, collection_read_hour = $2, source_cache_empty_hour = $3",
					tc.backup, tc.read, tc.emptying); err != nil {
					t.Fatal(err)
				}
			}
			if err := migrate(ctx, pool, migrations); err != nil {
				t.Fatal(err)
			}
			rows, err := pool.Query(ctx, "SELECT task_key || ' ' || triggers::text FROM task_triggers ORDER BY task_key")
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for rows.Next() {
				var row string
				if err := rows.Scan(&row); err != nil {
					t.Fatal(err)
				}
				got = append(got, row)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("triggers %q, want %q", got, tc.want)
			}
			var columns int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_name = 'settings'
				AND column_name IN ('backup_hour', 'collection_read_hour', 'source_cache_empty_hour')`).Scan(&columns); err != nil {
				t.Fatal(err)
			}
			if columns != 0 {
				t.Errorf("%d hour columns left in settings", columns)
			}
			if tc.want != nil {
				return
			}
			registry := tasks.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
			for _, key := range []string{"BackUpDatabase", "EmptySourceCache", "ReadCollections"} {
				registry.Register(tasks.Task{Key: key, Text: map[string]tasks.Text{"en": {Name: key}}, Run: func(context.Context) error { return nil }})
			}
			if err := registry.Load(ctx, tasks.NewStore(pool)); err != nil {
				t.Fatal(err)
			}
			for _, info := range registry.Tasks("en") {
				if info.Custom {
					t.Errorf("%s: custom triggers %v", info.Key, info.Triggers)
				}
			}
		})
	}
}
