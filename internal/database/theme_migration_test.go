package database

import (
	"io/fs"
	"slices"
	"strings"
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
	if err := migrate(t.Context(), fresh, beforeThemeChoice(t, migrations)); err != nil {
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
			if err := migrate(ctx, pool, beforeThemeChoice(t, migrations)); err != nil {
				t.Fatal(err)
			}
			if css, js := read(t, pool); css != tc.wantCss || js != tc.wantJs {
				t.Errorf("the custom code became %q and %q, want %q and %q", css, js, tc.wantCss, tc.wantJs)
			}
		})
	}
}

// The theme moves from the commit it was pinned to, to its main branch, on
// servers whose two fields still held it: a server that changed either
// keeps both as they were, never a stylesheet and a script from two
// versions of the theme.
func TestThemeMainBranchMigrationMovesOnlyTheUnchangedTheme(t *testing.T) {
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "theme_main_branch" })
	if at < 0 {
		t.Fatal("no theme_main_branch migration")
	}
	read := func(t *testing.T, pool *pgxpool.Pool) (css, js string) {
		t.Helper()
		if err := pool.QueryRow(t.Context(), "SELECT custom_css, custom_js FROM settings").Scan(&css, &js); err != nil {
			t.Fatal(err)
		}
		return css, js
	}
	pinned := testdb.New(t)
	if err := migrate(t.Context(), pinned, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	pinnedCss, pinnedJs := read(t, pinned)
	fresh := testdb.New(t)
	if err := migrate(t.Context(), fresh, beforeThemeChoice(t, migrations)); err != nil {
		t.Fatal(err)
	}
	mainCss, mainJs := read(t, fresh)
	if mainCss == pinnedCss || mainJs == pinnedJs {
		t.Fatalf("a new server's custom code is still the pinned theme: %q, %q", mainCss, mainJs)
	}
	for name, tc := range map[string]struct{ css, js, wantCss, wantJs string }{
		"unchanged":  {pinnedCss, pinnedJs, mainCss, mainJs},
		"own CSS":    {"a {}", pinnedJs, "a {}", pinnedJs},
		"own script": {pinnedCss, "b()", pinnedCss, "b()"},
		"emptied":    {"", "", "", ""},
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
			if err := migrate(ctx, pool, beforeThemeChoice(t, migrations)); err != nil {
				t.Fatal(err)
			}
			if css, js := read(t, pool); css != tc.wantCss || js != tc.wantJs {
				t.Errorf("the custom code became %q and %q, want %q and %q", css, js, tc.wantCss, tc.wantJs)
			}
		})
	}
}

// The default script turns the theme's home carousel off on servers whose
// script was still the previous default: a server that changed its script
// keeps it, and the stylesheet is never touched.
func TestThemeCarouselOffMigrationChangesOnlyTheUnchangedScript(t *testing.T) {
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "theme_carousel_off" })
	if at < 0 {
		t.Fatal("no theme_carousel_off migration")
	}
	read := func(t *testing.T, pool *pgxpool.Pool) (css, js string) {
		t.Helper()
		if err := pool.QueryRow(t.Context(), "SELECT custom_css, custom_js FROM settings").Scan(&css, &js); err != nil {
			t.Fatal(err)
		}
		return css, js
	}
	before := testdb.New(t)
	if err := migrate(t.Context(), before, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	themeCss, carouselJs := read(t, before)
	// Up to this migration only: later ones move the theme on again.
	fresh := testdb.New(t)
	if err := migrate(t.Context(), fresh, migrations[:at+1]); err != nil {
		t.Fatal(err)
	}
	freshCss, freshJs := read(t, fresh)
	if freshCss != themeCss || !strings.Contains(freshJs, "homeCarousel: false") ||
		strings.Replace(freshJs, "homeCarousel: false", "homeCarousel: true", 1) != carouselJs {
		t.Fatalf("a new server's custom code: %q, %q", freshCss, freshJs)
	}
	edited := strings.Replace(carouselJs, "collectionFilter: true", "collectionFilter: false", 1)
	for name, tc := range map[string]struct{ css, js, wantCss, wantJs string }{
		"unchanged":     {themeCss, carouselJs, themeCss, freshJs},
		"own CSS":       {"a {}", carouselJs, "a {}", freshJs},
		"edited script": {themeCss, edited, themeCss, edited},
		"own script":    {themeCss, "b()", themeCss, "b()"},
		"emptied":       {"", "", "", ""},
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
			if err := migrate(ctx, pool, migrations[:at+1]); err != nil {
				t.Fatal(err)
			}
			if css, js := read(t, pool); css != tc.wantCss || js != tc.wantJs {
				t.Errorf("the custom code became %q and %q, want %q and %q", css, js, tc.wantCss, tc.wantJs)
			}
		})
	}
}

// The default theme is the copy Polyfin bundles, served by the web client,
// on new servers and on servers whose two fields still held the theme from
// a CDN at its main branch. A server that changed either keeps both, never
// a stylesheet and a script from two versions of the theme.
func TestThemeBundledMigrationMovesOnlyTheUnchangedTheme(t *testing.T) {
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "theme_bundled" })
	if at < 0 {
		t.Fatal("no theme_bundled migration")
	}
	read := func(t *testing.T, pool *pgxpool.Pool) (css, js string) {
		t.Helper()
		if err := pool.QueryRow(t.Context(), "SELECT custom_css, custom_js FROM settings").Scan(&css, &js); err != nil {
			t.Fatal(err)
		}
		return css, js
	}
	before := testdb.New(t)
	if err := migrate(t.Context(), before, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	cdnCss, cdnJs := read(t, before)
	// Up to this migration only: later ones may move the theme on again.
	fresh := testdb.New(t)
	if err := migrate(t.Context(), fresh, migrations[:at+1]); err != nil {
		t.Fatal(err)
	}
	bundledCss, bundledJs := read(t, fresh)
	for _, code := range []string{bundledCss, bundledJs} {
		if strings.Contains(code, "://") || !strings.Contains(code, "'polyfin/themes/lumaaglaass/") {
			t.Fatalf("a new server's custom code does not load the bundled theme: %q", code)
		}
	}
	if !strings.Contains(cdnJs, "homeCarousel: false") || !strings.Contains(bundledJs, "homeCarousel: false") {
		t.Fatalf("the theme's options changed: %q, then %q", cdnJs, bundledJs)
	}
	edited := strings.Replace(cdnJs, "homeCarousel: false", "homeCarousel: true", 1)
	for name, tc := range map[string]struct{ css, js, wantCss, wantJs string }{
		"unchanged":     {cdnCss, cdnJs, bundledCss, bundledJs},
		"own CSS":       {"a {}", cdnJs, "a {}", cdnJs},
		"edited CSS":    {cdnCss + "\nbody { color: red; }", cdnJs, cdnCss + "\nbody { color: red; }", cdnJs},
		"edited script": {cdnCss, edited, cdnCss, edited},
		"own script":    {cdnCss, "b()", cdnCss, "b()"},
		"emptied":       {"", "", "", ""},
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
			if err := migrate(ctx, pool, migrations[:at+1]); err != nil {
				t.Fatal(err)
			}
			if css, js := read(t, pool); css != tc.wantCss || js != tc.wantJs {
				t.Errorf("the custom code became %q and %q, want %q and %q", css, js, tc.wantCss, tc.wantJs)
			}
		})
	}
}

// beforeThemeChoice returns the migrations before theme_choice, after
// which the custom code no longer holds the bundled theme: the tests of
// the migrations that moved it look at what they left.
func beforeThemeChoice(t *testing.T, migrations []migration) []migration {
	t.Helper()
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "theme_choice" })
	if at < 0 {
		t.Fatal("no theme_choice migration")
	}
	return migrations[:at]
}

// The theme becomes a choice: a server whose two texts still loaded the
// bundled theme chooses it, with both texts emptied, one with both empty
// chooses jellyfin-web's own look, and any other keeps its texts as
// Custom, the theme it had.
func TestThemeChoiceMigrationChoosesWhatTheTextsLoaded(t *testing.T) {
	files, err := fs.Sub(embedded, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(migrations, func(m migration) bool { return m.name == "theme_choice" })
	if at < 0 {
		t.Fatal("no theme_choice migration")
	}
	read := func(t *testing.T, pool *pgxpool.Pool) (theme, css, js string) {
		t.Helper()
		if err := pool.QueryRow(t.Context(), "SELECT web_theme, custom_css, custom_js FROM settings").Scan(&theme, &css, &js); err != nil {
			t.Fatal(err)
		}
		return theme, css, js
	}
	before := testdb.New(t)
	if err := migrate(t.Context(), before, migrations[:at]); err != nil {
		t.Fatal(err)
	}
	var bundledCss, bundledJs string
	if err := before.QueryRow(t.Context(), "SELECT custom_css, custom_js FROM settings").Scan(&bundledCss, &bundledJs); err != nil {
		t.Fatal(err)
	}
	fresh := testdb.New(t)
	if err := migrate(t.Context(), fresh, migrations[:at+1]); err != nil {
		t.Fatal(err)
	}
	if theme, css, js := read(t, fresh); theme != "lumaaglaass" || css != "" || js != "" {
		t.Fatalf("a new server: %q, %q, %q", theme, css, js)
	}
	edited := strings.Replace(bundledJs, "homeCarousel: false", "homeCarousel: true", 1)
	for name, tc := range map[string]struct{ css, js, wantTheme, wantCss, wantJs string }{
		"bundled theme": {bundledCss, bundledJs, "lumaaglaass", "", ""},
		"emptied":       {"", "", "none", "", ""},
		"edited script": {bundledCss, edited, "custom", bundledCss, edited},
		"own CSS only":  {"a {}", "", "custom", "a {}", ""},
		"own theme":     {"@import url('https://themes.example/a.css');", "b()", "custom", "@import url('https://themes.example/a.css');", "b()"},
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
			if err := migrate(ctx, pool, migrations[:at+1]); err != nil {
				t.Fatal(err)
			}
			if theme, css, js := read(t, pool); theme != tc.wantTheme || css != tc.wantCss || js != tc.wantJs {
				t.Errorf("became %q with %q and %q, want %q with %q and %q", theme, css, js, tc.wantTheme, tc.wantCss, tc.wantJs)
			}
		})
	}
}
