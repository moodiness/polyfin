package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The migration keeps the administrator's messages to Moonfin's apps as
// they may be shown, and a message's chosen users while both last.
func TestMoonfinMessagesMigrationKeepsMessagesAndTheirUsers(t *testing.T) {
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
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "moonfin_messages" })
	if at < 0 {
		t.Fatal("no moonfin_messages migration")
	}
	if err := migrate(ctx, pool, migrations[:at+1]); err != nil {
		t.Fatal(err)
	}
	var user, message string
	if err := pool.QueryRow(ctx, "INSERT INTO users (name, password_hash) VALUES ('member', 'x') RETURNING id").Scan(&user); err != nil {
		t.Fatal(err)
	}
	insert := `INSERT INTO moonfin_messages (position, title, body, color, delivery, audience, start_at, end_at, link_label, link_url,
		created_at, updated_at) VALUES (1, $1, $2, $3, $4, $5, $6, $7, $8, $9, now(), now()) RETURNING id`
	if err := pool.QueryRow(ctx, insert, "Hello", "**Hi**", "blue", "popup", "users", "2026-12-01T09:00:00Z", "2026-12-02T09:00:00Z",
		"Open", "https://example.com").Scan(&message); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO moonfin_message_users (message_id, user_id) VALUES ($1, $2)", message, user); err != nil {
		t.Fatal(err)
	}
	for name, values := range map[string][]any{
		"no title or body":        {"", "", "blue", "inbox", "all", nil, nil, "", ""},
		"an unknown color":        {"Hi", "", "pink", "inbox", "all", nil, nil, "", ""},
		"an unknown delivery":     {"Hi", "", "blue", "push", "all", nil, nil, "", ""},
		"an unknown audience":     {"Hi", "", "blue", "inbox", "guests", nil, nil, "", ""},
		"an end before the start": {"Hi", "", "blue", "inbox", "all", "2026-12-02T09:00:00Z", "2026-12-01T09:00:00Z", "", ""},
		"a label without a link":  {"Hi", "", "blue", "inbox", "all", nil, nil, "Open", ""},
	} {
		var id string
		if err := pool.QueryRow(ctx, insert, values...).Scan(&id); err == nil {
			t.Errorf("%s was kept", name)
		}
	}
	if _, err := pool.Exec(ctx, "DELETE FROM users WHERE id = $1", user); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM moonfin_message_users").Scan(&left); err != nil || left != 0 {
		t.Errorf("users left after their user: %d %v", left, err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM moonfin_messages").Scan(&left); err != nil || left != 1 {
		t.Errorf("the message after its user: %d %v", left, err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		t.Fatal(err)
	}
}
