package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The targets that chose the weekly summary receive the year in review
// too; the others, and the month in review, stay as they were.
func TestReviewsMigrationChoosesTheYearWhereTheWeekIs(t *testing.T) {
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
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "reviews" })
	if at < 0 {
		t.Fatal("no reviews migration")
	}
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO notification_targets (kind, name, address, secret, events) VALUES
		('webhook', 'weekly', 'https://example.org', 'x', '{new_episode,weekly_summary}'),
		('webhook', 'episodes', 'https://example.org', 'x', '{new_episode}'),
		('webhook', 'nothing', 'https://example.org', 'x', '{}')`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, "SELECT name, array_to_string(events, ',') FROM notification_targets")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	var name, events string
	for rows.Next() {
		if err := rows.Scan(&name, &events); err != nil {
			t.Fatal(err)
		}
		got[name] = events
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"weekly": "new_episode,weekly_summary,yearly_summary", "episodes": "new_episode", "nothing": ""}
	for target, events := range want {
		if got[target] != events {
			t.Errorf("%s: %q, want %q", target, got[target], events)
		}
	}
}
