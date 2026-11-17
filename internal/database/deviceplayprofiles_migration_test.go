package database

import (
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The device profiles kept across restarts come after the devices they
// belong to: a device signed in before the migration keeps one once it
// runs, which goes with the device, or with its user. A profile is an
// object of bounded size.
func TestDevicePlayProfilesGoWithTheirDevice(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "device_play_profiles" })
	if at < 0 {
		t.Fatal("no device_play_profiles migration")
	}
	pool := testdb.Empty(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `WITH u AS (
			INSERT INTO users (name, password_hash) VALUES ('alice', 'x'), ('bob', 'x') RETURNING id, name
		)
		INSERT INTO devices (user_id, token_hash, device_id, device_name, client, client_version, remote_address)
		SELECT u.id, decode(md5(u.name || d.device_id), 'hex'), d.device_id, 'TV', 'Jellyfin Web', '12.4', '192.0.2.1'
		FROM u JOIN (VALUES ('alice', 'tv'), ('alice', 'phone'), ('bob', 'web')) d (name, device_id) ON d.name = u.name`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO device_play_profiles (device_id, profile, bitrate, from_capabilities)
		SELECT id, '{"Name": "web"}', CASE WHEN device_id = 'tv' THEN 120000000 END, device_id = 'phone' FROM devices`); err != nil {
		t.Fatal(err)
	}
	count := func() int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM device_play_profiles").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(); n != 3 {
		t.Fatalf("profiles: %d", n)
	}
	for _, profile := range []string{`[]`, `{"Name": "` + strings.Repeat("x", 131072) + `"}`} {
		if _, err := pool.Exec(ctx, `UPDATE device_play_profiles SET profile = $1::jsonb`, profile); err == nil {
			t.Errorf("a profile of %d bytes starting %.10s accepted", len(profile), profile)
		}
	}
	if _, err := pool.Exec(ctx, "DELETE FROM devices WHERE device_id = 'tv'"); err != nil {
		t.Fatal(err)
	}
	if n := count(); n != 2 {
		t.Errorf("profiles after a device was deleted: %d", n)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM users WHERE name = 'alice'"); err != nil {
		t.Fatal(err)
	}
	var left string
	if err := pool.QueryRow(ctx, "SELECT d.device_id FROM device_play_profiles p JOIN devices d ON d.id = p.device_id").Scan(&left); err != nil || left != "web" {
		t.Errorf("profiles after a user was deleted: %q %v", left, err)
	}
}
