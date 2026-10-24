package accounts

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

// The theme chosen decides what the web player gets: the bundled
// LumaaGlaass, by default, nothing for jellyfin-web's own look, or the
// Custom texts, which are kept while another theme is chosen, so that
// choosing Custom again brings them back.
func TestTheThemeChoiceKeepsTheCustomTexts(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	settings := store.Settings()
	if settings.WebTheme != WebThemeLumaaGlaass || settings.ServedCss() != LumaaGlaassCss || settings.ServedJs() != LumaaGlaassJs {
		t.Fatalf("a new server serves %q, %q and %q", settings.WebTheme, settings.ServedCss(), settings.ServedJs())
	}
	settings.WebTheme, settings.CustomCss, settings.CustomJs = WebThemeCustom, ".mine {}", "console.log('mine')"
	saved, err := store.UpdateSettings(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ServedCss() != ".mine {}" || saved.ServedJs() != "console.log('mine')" {
		t.Errorf("Custom serves %q and %q", saved.ServedCss(), saved.ServedJs())
	}
	for _, tc := range []struct{ theme, css, js string }{
		{WebThemeNone, "", ""},
		{WebThemeLumaaGlaass, LumaaGlaassCss, LumaaGlaassJs},
		{WebThemeCustom, ".mine {}", "console.log('mine')"},
	} {
		settings := store.Settings()
		settings.WebTheme = tc.theme
		if _, err := store.UpdateSettings(ctx, settings); err != nil {
			t.Fatal(err)
		}
		got := store.Settings()
		if got.ServedCss() != tc.css || got.ServedJs() != tc.js || got.CustomCss != ".mine {}" || got.CustomJs != "console.log('mine')" {
			t.Errorf("%s: serves %q and %q, keeps %q and %q", tc.theme, got.ServedCss(), got.ServedJs(), got.CustomCss, got.CustomJs)
		}
		var stored string
		if err := store.db.QueryRow(ctx, "SELECT web_theme FROM settings").Scan(&stored); err != nil || stored != tc.theme {
			t.Errorf("%s: stored %q, %v", tc.theme, stored, err)
		}
	}
	settings = store.Settings()
	settings.WebTheme = "another"
	if _, err := store.UpdateSettings(ctx, settings); !errors.Is(err, ErrInvalidWebTheme) {
		t.Errorf("an unknown theme: %v", err)
	}
}

// The Custom texts are replaced only when they still are what was read,
// with what runs in the same transaction, or not at all.
func TestCustomTextsAreReplacedOnlyAsTheyWereRead(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	settings := store.Settings()
	settings.WebTheme, settings.CustomCss, settings.CustomJs, settings.ServerName = WebThemeCustom, "a {}", "b()", "Kept name"
	if _, err := store.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceCustomTexts(ctx, "other {}", "b()", "c {}", "d()", nil); !errors.Is(err, ErrCustomTextsChanged) {
		t.Errorf("texts changed since: %v", err)
	}
	failed := errors.New("failed")
	if _, err := store.ReplaceCustomTexts(ctx, "a {}", "b()", "c {}", "d()", func(pgx.Tx) error { return failed }); !errors.Is(err, failed) {
		t.Errorf("what runs with it failed: %v", err)
	}
	var css string
	if err := store.db.QueryRow(ctx, "SELECT custom_css FROM settings").Scan(&css); err != nil || css != "a {}" || store.Settings().CustomCss != "a {}" {
		t.Errorf("after refusals: %q stored, %q in memory", css, store.Settings().CustomCss)
	}
	got, err := store.ReplaceCustomTexts(ctx, "a {}", "b()", "c {}", "d()", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.CustomCss != "c {}" || got.CustomJs != "d()" || got.ServerName != "Kept name" || store.Settings().CustomJs != "d()" {
		t.Errorf("replaced: %+v", got)
	}
	reloaded, err := store.loadSettings(ctx)
	if err != nil || reloaded.CustomCss != "c {}" || reloaded.CustomJs != "d()" || reloaded.WebTheme != WebThemeCustom {
		t.Errorf("stored: %q %q %q %v", reloaded.CustomCss, reloaded.CustomJs, reloaded.WebTheme, err)
	}
}
