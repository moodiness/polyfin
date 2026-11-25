package moonfin

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// newStore is a store of a database of its own, with users of names.
func newStore(t *testing.T, names ...string) (*Store, []accounts.ID) {
	t.Helper()
	pool := migrated.New(t)
	ids := make([]accounts.ID, len(names))
	for i, name := range names {
		if err := pool.QueryRow(t.Context(), "INSERT INTO users (name, password_hash) VALUES ($1, 'x') RETURNING id", name).Scan(&ids[i]); err != nil {
			t.Fatal(err)
		}
	}
	return New(pool), ids
}

// told reports whether changed was told, at once.
func toldNow(changed <-chan struct{}) bool {
	select {
	case <-changed:
		return true
	case <-time.After(time.Second):
		return false
	}
}

// Settings saved are kept for their user alone, resolved with the
// administrator's defaults, and their watchers are told of a save that
// changed them, never of one that did not.
func TestStoredSettingsResolveAndTellTheirChanges(t *testing.T) {
	t.Parallel()
	s, ids := newStore(t, "sam", "robin")
	sam, robin := ids[0], ids[1]
	ctx := t.Context()
	if _, err := s.Envelope(ctx, sam); !errors.Is(err, ErrNotFound) {
		t.Fatalf("settings before any save: %v", err)
	}
	changed, stop := s.Watch(sam)
	defer stop()

	created, err := s.SaveProfile(ctx, sam, Global, Settings{"theme": "dark"}, "phone")
	if err != nil || !created {
		t.Fatalf("first save: %v %v", created, err)
	}
	if !toldNow(changed) {
		t.Error("the first save was not told")
	}
	if created, err = s.SaveProfile(ctx, sam, Global, Settings{"theme": "dark"}, "tv"); err != nil || created {
		t.Fatalf("second save: %v %v", created, err)
	}
	select {
	case <-changed:
		t.Error("a save that changed nothing was told")
	default:
	}

	if err := s.SetDefaults(ctx, Global, Settings{"theme": "light", "clock": "always", "tmdbApiKey": "never"}); err != nil {
		t.Fatal(err)
	}
	resolved, err := s.Resolved(ctx, sam, TV)
	if err != nil || resolved["theme"] != "dark" || resolved["clock"] != "always" || resolved["tmdbApiKey"] != nil {
		t.Errorf("sam's TV: %v %v", resolved, err)
	}
	if resolved, err = s.Resolved(ctx, robin, Mobile); err != nil || resolved["theme"] != "light" {
		t.Errorf("robin, who never synced: %v %v", resolved, err)
	}

	if _, err := s.SaveProfile(ctx, sam, TV, Settings{"theme": "oled"}, "tv"); err != nil {
		t.Fatal(err)
	}
	sync, err := s.SyncOf(ctx, sam)
	if err != nil || !sync.Synced || sync.Settings != 2 || strings.Join(sync.Profiles, ",") != "global,tv" || sync.UpdatedBy != "tv" {
		t.Errorf("sam's sync: %+v %v", sync, err)
	}
	if err := s.DeleteProfile(ctx, sam, TV); err != nil {
		t.Fatal(err)
	}
	if resolved, _ = s.Resolved(ctx, sam, TV); resolved["theme"] != "dark" {
		t.Errorf("sam's TV without its profile: %v", resolved)
	}
	if err := s.DeleteProfile(ctx, sam, Global); !errors.Is(err, ErrInvalidProfile) {
		t.Errorf("deleting the global profile: %v", err)
	}
	if sync, _ = s.SyncOf(ctx, robin); sync.Synced {
		t.Errorf("robin's sync: %+v", sync)
	}
	if err := s.Delete(ctx, sam); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Envelope(ctx, sam); !errors.Is(err, ErrNotFound) {
		t.Errorf("settings after their deletion: %v", err)
	}
}

// Defaults are taken from a user's profile without their own keys and
// hidden titles, and applied to every user who synced.
func TestDefaultsTakenFromAUserAndApplied(t *testing.T) {
	t.Parallel()
	s, ids := newStore(t, "morgan", "sam", "robin")
	morgan, sam := ids[0], ids[1]
	ctx := t.Context()
	if _, err := s.SaveProfile(ctx, morgan, Global, Settings{"theme": "dark", "mdblistApiKey": "morgan's",
		hiddenNextUp: `{"x":"2026-11-01T00:00:00Z"}`}, "web"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveProfile(ctx, sam, Global, Settings{"theme": "light", "clock": "never"}, "web"); err != nil {
		t.Fatal(err)
	}
	if err := s.CopyToDefaults(ctx, ids[2], Global, TV); !errors.Is(err, ErrNotFound) {
		t.Errorf("copying from a user without settings: %v", err)
	}
	if err := s.CopyToDefaults(ctx, morgan, Global, TV); err != nil {
		t.Fatal(err)
	}
	defaults, err := s.Defaults(ctx)
	if err != nil || len(defaults[TV]) != 1 || defaults[TV]["theme"] != "dark" || len(defaults[Global]) != 0 {
		t.Fatalf("defaults: %v %v", defaults, err)
	}
	if err := s.SetDefaults(ctx, Global, Settings{"clock": "always"}); err != nil {
		t.Fatal(err)
	}
	if applied, err := s.Apply(ctx, false); err != nil || applied != 2 {
		t.Fatalf("applied to %d: %v", applied, err)
	}
	env, _ := s.Envelope(ctx, sam)
	if got := profileOf(env, Global); got["clock"] != "always" || got["theme"] != "light" {
		t.Errorf("sam's global: %v", got)
	}
	if got := profileOf(env, TV); got["theme"] != "dark" {
		t.Errorf("sam's TV: %v", got)
	}
	if applied, err := s.Apply(ctx, true); err != nil || applied != 2 {
		t.Fatalf("reset %d: %v", applied, err)
	}
	env, _ = s.Envelope(ctx, morgan)
	if got := profileOf(env, Global); got["theme"] != nil || got["mdblistApiKey"] != "morgan's" || got[hiddenNextUp] == nil || got["clock"] != "always" {
		t.Errorf("morgan's global after a reset: %v", got)
	}
	if err := s.SetDefaults(ctx, TV, Settings{}); err != nil {
		t.Fatal(err)
	}
	if defaults, _ = s.Defaults(ctx); len(defaults[TV]) != 0 {
		t.Errorf("TV defaults emptied: %v", defaults[TV])
	}
	if err := s.SetDefaults(ctx, TV, Settings{"big": strings.Repeat("x", MaxDefaultsBytes)}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("defaults too large: %v", err)
	}
}
