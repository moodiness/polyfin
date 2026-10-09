package database

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The web player theme becomes the default of the custom CSS and script:
// a server upgraded with both still empty gets what a new server gets, and
// a server that set either keeps both as they were, never half a theme.
func TestThemeMigrationFillsOnlyServersWithoutCustomCode(t *testing.T) {
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "web_player_theme" })
	if at < 0 {
		t.Fatal("no web_player_theme migration")
	}
	read := func(t *testing.T, pool *pgxpool.Pool) (css, js string) {
		t.Helper()
		if err := pool.QueryRow(t.Context(), "SELECT custom_css, custom_js FROM settings").Scan(&css, &js); err != nil {
			t.Fatal(err)
		}
		return css, js
	}
	fresh := testdb.New(t)
	if err := migrate(t.Context(), fresh, migrations); err != nil {
		t.Fatal(err)
	}
	themeCss, themeJs := read(t, fresh)
	if themeCss == "" || themeJs == "" {
		t.Fatalf("a new server's custom code: %q, %q", themeCss, themeJs)
	}
	for name, tc := range map[string]struct{ css, js, wantCss, wantJs string }{
		"none":       {"", "", themeCss, themeJs},
		"own CSS":    {"body { color: red; }", "", "body { color: red; }", ""},
		"own script": {"", "console.log('mine')", "", "console.log('mine')"},
		"both":       {"a {}", "b()", "a {}", "b()"},
	} {
		t.Run(name, func(t *testing.T) {
			pool := testdb.New(t)
			ctx := t.Context()
			if err := migrate(ctx, pool, migrations[:at]); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, "UPDATE settings SET custom_css = $1, custom_js = $2", tc.css, tc.js); err != nil {
				t.Fatal(err)
			}
			if err := migrate(ctx, pool, migrations); err != nil {
				t.Fatal(err)
			}
			if css, js := read(t, pool); css != tc.wantCss || js != tc.wantJs {
				t.Errorf("the custom code became %q and %q, want %q and %q", css, js, tc.wantCss, tc.wantJs)
			}
		})
	}
}
