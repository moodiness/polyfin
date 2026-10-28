package jellyfin

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/preferences"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// Reset my preferences reaches the apps already signed in: the user's
// configuration, once read and remembered, is a new user's again from the
// next request, and another user's stays.
func TestResetPreferencesForgetsTheRememberedConfiguration(t *testing.T) {
	t.Parallel()
	pool := migrated.New(t)
	ctx := t.Context()
	prefs := preferences.New(pool)
	h := &Handler{Options: Options{Preferences: prefs, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))},
		configurations: cache.New[accounts.ID, UserConfiguration](10, time.Hour)}
	store, err := accounts.Open(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	var users []accounts.ID
	for _, name := range []string{"sam", "kim"} {
		user, err := store.CreateUser(ctx, accounts.NewUser{Name: name, Password: "correct horse"})
		if err != nil {
			t.Fatal(err)
		}
		if err := prefs.PutConfiguration(ctx, user.ID, json.RawMessage(`{"SubtitleMode":"Always","SubtitleLanguagePreference":"fre"}`)); err != nil {
			t.Fatal(err)
		}
		// Read once, as a request does: it is remembered.
		if configuration, err := h.userConfiguration(ctx, user.ID); err != nil || configuration.SubtitleLanguagePreference != "fre" {
			t.Fatalf("%s before: %+v %v", name, configuration, err)
		}
		users = append(users, user.ID)
	}

	if err := h.ResetPreferences(ctx, users[0]); err != nil {
		t.Fatal(err)
	}
	configuration, err := h.userConfiguration(ctx, users[0])
	if want := defaultUserConfiguration(); err != nil || configuration.SubtitleLanguagePreference != want.SubtitleLanguagePreference ||
		configuration.SubtitleMode != want.SubtitleMode {
		t.Errorf("after the reset: %+v %v, want a new user's", configuration, err)
	}
	if configuration, err := h.userConfiguration(ctx, users[1]); err != nil || configuration.SubtitleLanguagePreference != "fre" {
		t.Errorf("another user after the reset: %+v %v", configuration, err)
	}
}

// Apply this order to every user forgets the orders users saved, also for
// the apps already signed in, whose configuration was remembered, and
// leaves the rest of every configuration.
func TestClearingLibraryOrdersForgetsTheRememberedConfiguration(t *testing.T) {
	t.Parallel()
	pool := migrated.New(t)
	ctx := t.Context()
	prefs := preferences.New(pool)
	h := &Handler{Options: Options{Preferences: prefs, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))},
		configurations: cache.New[accounts.ID, UserConfiguration](10, time.Hour)}
	store, err := accounts.Open(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	saved := map[string]string{
		"sam": `{"SubtitleLanguagePreference":"fre","OrderedViews":["0123456789abcdef0123456789abcdef"]}`,
		"kim": `{"SubtitleLanguagePreference":"ger","OrderedViews":["fedcba9876543210fedcba9876543210","0123456789abcdef0123456789abcdef"]}`,
		"lee": `{"SubtitleLanguagePreference":"spa","OrderedViews":[]}`,
	}
	users := map[string]accounts.ID{}
	for name, value := range saved {
		user, err := store.CreateUser(ctx, accounts.NewUser{Name: name, Password: "correct horse"})
		if err != nil {
			t.Fatal(err)
		}
		if err := prefs.PutConfiguration(ctx, user.ID, json.RawMessage(value)); err != nil {
			t.Fatal(err)
		}
		// Read once, as a request does: it is remembered.
		if _, err := h.userConfiguration(ctx, user.ID); err != nil {
			t.Fatal(err)
		}
		users[name] = user.ID
	}
	if count, err := h.LibraryOrders(ctx); err != nil || count != 2 {
		t.Fatalf("users with an order before: %d %v", count, err)
	}

	if count, err := h.ClearLibraryOrders(ctx); err != nil || count != 2 {
		t.Fatalf("cleared: %d %v", count, err)
	}
	for name, language := range map[string]string{"sam": "fre", "kim": "ger", "lee": "spa"} {
		configuration, err := h.userConfiguration(ctx, users[name])
		if err != nil || len(configuration.OrderedViews) != 0 || configuration.SubtitleLanguagePreference != language {
			t.Errorf("%s after: %+v %v", name, configuration, err)
		}
	}
	if count, err := h.LibraryOrders(ctx); err != nil || count != 0 {
		t.Errorf("users with an order after: %d %v", count, err)
	}
}
