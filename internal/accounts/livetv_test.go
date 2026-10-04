package accounts

import (
	"errors"
	"testing"
)

// Live TV lists and guides are fetched again every 12 hours, as before
// the setting, and every 1 to 168 hours once it is set.
func TestLiveTvRefreshHoursStayInRange(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	if got := store.Settings().LiveTvRefreshHours; got != DefaultLiveTvRefreshHours || DefaultLiveTvRefreshHours != 12 {
		t.Errorf("default: %d", got)
	}
	for _, hours := range []int{0, -1, MaxLiveTvRefreshHours + 1} {
		changed := store.Settings()
		changed.LiveTvRefreshHours = hours
		if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, ErrInvalidLiveTvRefreshHours) {
			t.Errorf("%d hours: %v", hours, err)
		}
	}
	for _, hours := range []int{MinLiveTvRefreshHours, MaxLiveTvRefreshHours} {
		changed := store.Settings()
		changed.LiveTvRefreshHours = hours
		if _, err := store.UpdateSettings(ctx, changed); err != nil {
			t.Fatal(err)
		}
		reopened, err := Open(ctx, store.db)
		if err != nil {
			t.Fatal(err)
		}
		if got := reopened.Settings().LiveTvRefreshHours; got != hours {
			t.Errorf("after reopening: %d, want %d", got, hours)
		}
	}
	if _, err := store.CreateFirstAdministrator(ctx, "admin", "correct horse", "fr"); err != nil {
		t.Fatal(err)
	}
	if got := store.Settings().LiveTvRefreshHours; got != MaxLiveTvRefreshHours {
		t.Errorf("after the setup: %d", got)
	}
}
