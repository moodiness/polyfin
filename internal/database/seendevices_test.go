package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The devices signed in when the migration runs count as seen, so that
// updating tells no sign-in on a new device.
func TestSeenDevicesMigrationCountsTheDevicesSignedIn(t *testing.T) {
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
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "seen_devices" })
	if at < 0 {
		t.Fatal("no seen_devices migration")
	}
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `WITH u AS (INSERT INTO users (name, password_hash) VALUES ('alice', 'x'), ('bob', 'x') RETURNING id, name)
		INSERT INTO devices (user_id, token_hash, device_id, device_name, client, client_version, remote_address)
		SELECT id, decode(md5(name || d), 'hex'), d, d, 'Test App', '1', '' FROM u, unnest(ARRAY['tv', 'phone']) AS d WHERE name = 'alice'`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	var seen []string
	rows, err := pool.Query(ctx, "SELECT u.name || ' ' || s.device_id FROM seen_devices s JOIN users u ON u.id = s.user_id ORDER BY 1")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var row string
		if err := rows.Scan(&row); err != nil {
			t.Fatal(err)
		}
		seen = append(seen, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(seen, []string{"alice phone", "alice tv"}) {
		t.Errorf("seen: %v", seen)
	}
}
