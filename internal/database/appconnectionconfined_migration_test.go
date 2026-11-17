package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The app connections made before they kept their confinement stay
// confined unless an administrator made them: a member's backend reaches
// public addresses only, at every later import too.
func TestAppConnectionConfinedMigrationConfinesMembersConnections(t *testing.T) {
	t.Parallel()
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "app_connection_confined" })
	if at < 0 {
		t.Fatal("no app_connection_confined migration")
	}
	pool := testdb.Empty(t)
	ctx := t.Context()
	if err := migrate(ctx, pool, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	// Bob's own, one an administrator made for him, the administrator's
	// own, and Carol's, whose maker's account is gone.
	if _, err := pool.Exec(ctx, `WITH root AS (INSERT INTO users (name, password_hash, is_administrator) VALUES ('root', 'x', true) RETURNING id),
		bob AS (INSERT INTO users (name, password_hash, is_administrator) VALUES ('bob', 'x', false) RETURNING id),
		carol AS (INSERT INTO users (name, password_hash, is_administrator) VALUES ('carol', 'x', false) RETURNING id)
		INSERT INTO app_connections (user_id, app, sign_in, owned, connected_at, connected_by)
		SELECT bob.id, 'nuvio', 'bob own', true, now(), bob.id FROM bob
		UNION ALL SELECT bob.id, 'stremio', 'bob by root', true, now(), root.id FROM bob, root
		UNION ALL SELECT root.id, 'nuvio', 'root own', true, now(), root.id FROM root
		UNION ALL SELECT carol.id, 'nuvio', 'carol', true, now(), NULL FROM carol`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, "SELECT sign_in, confined FROM app_connections")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := map[string]bool{"bob own": true, "bob by root": false, "root own": false, "carol": true}
	for rows.Next() {
		var signIn string
		var confined bool
		if err := rows.Scan(&signIn, &confined); err != nil {
			t.Fatal(err)
		}
		if confined != want[signIn] {
			t.Errorf("%s: confined %v", signIn, confined)
		}
		delete(want, signIn)
	}
	if err := rows.Err(); err != nil || len(want) != 0 {
		t.Errorf("missing %v %v", want, err)
	}
}
