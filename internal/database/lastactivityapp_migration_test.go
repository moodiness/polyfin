package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// Users active before the app of the last activity was kept count their
// most recently active device's; those without a device, or never
// active, have none.
func TestLastActivityAppMigrationTakesTheLatestDevice(t *testing.T) {
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "last_activity_app" })
	if at < 0 {
		t.Fatal("no last_activity_app migration")
	}
	pool := testdb.New(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `WITH u AS (
			INSERT INTO users (name, password_hash, last_activity_at) VALUES
				('alice', 'x', now()), ('bob', 'x', now()), ('carol', 'x', NULL), ('dave', 'x', now())
			RETURNING id, name
		)
		INSERT INTO devices (user_id, token_hash, device_id, device_name, client, client_version, remote_address, last_activity_at)
		SELECT u.id, decode(md5(u.name || d.device_id), 'hex'), d.device_id, 'TV', d.client, '1.0', '192.0.2.1', now() - d.age
		FROM u JOIN (VALUES
			('alice', 'tv', 'Jellyfin Android TV', interval '1 minute'),
			('alice', 'phone', 'Jellyfin Mobile', interval '2 days'),
			('bob', 'web', 'Jellyfin Web', interval '1 hour'),
			('carol', 'web', 'Jellyfin Web', interval '1 hour')
		) d (name, device_id, client, age) ON d.name = u.name`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	rows, err := pool.Query(ctx, "SELECT name, last_activity_app FROM users")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name, app string
		if err := rows.Scan(&name, &app); err != nil {
			t.Fatal(err)
		}
		got[name] = app
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"alice": "Jellyfin Android TV", "bob": "Jellyfin Web", "carol": "", "dave": ""}
	for name, app := range want {
		if got[name] != app {
			t.Errorf("%s: app %q, want %q", name, got[name], app)
		}
	}
}
