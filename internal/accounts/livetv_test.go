package accounts

import (
	"errors"
	"testing"
)

// Live TV lists and guides are fetched again every 12 hours, as before
// the setting, and every 1 to 168 hours once it is set.
func TestLiveTvRefreshHoursStayInRange(t *testing.T) {
	t.Parallel()
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

// Each watched channel keeps its last 30 minutes for pause and rewind by
// default, from 0 (off) to 240, and all buffers take at most 20 GB by
// default, from 1 to 1000.
func TestLiveTvBufferStaysInRange(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	if got := store.Settings(); got.LiveTvBufferMinutes != 30 || got.LiveTvBufferGigabytes != 20 ||
		DefaultLiveTvBufferMinutes != 30 || DefaultLiveTvBufferGigabytes != 20 {
		t.Errorf("default: %d minutes, %d GB", got.LiveTvBufferMinutes, got.LiveTvBufferGigabytes)
	}
	for _, minutes := range []int{-1, MaxLiveTvBufferMinutes + 1} {
		changed := store.Settings()
		changed.LiveTvBufferMinutes = minutes
		if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, ErrInvalidLiveTvBufferMinutes) {
			t.Errorf("%d minutes: %v", minutes, err)
		}
	}
	for _, gigabytes := range []int{0, -1, MaxLiveTvBufferGigabytes + 1} {
		changed := store.Settings()
		changed.LiveTvBufferGigabytes = gigabytes
		if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, ErrInvalidLiveTvBufferGigabytes) {
			t.Errorf("%d GB: %v", gigabytes, err)
		}
	}
	for _, bounds := range [][2]int{{MinLiveTvBufferMinutes, MinLiveTvBufferGigabytes}, {MaxLiveTvBufferMinutes, MaxLiveTvBufferGigabytes}} {
		changed := store.Settings()
		changed.LiveTvBufferMinutes, changed.LiveTvBufferGigabytes = bounds[0], bounds[1]
		if _, err := store.UpdateSettings(ctx, changed); err != nil {
			t.Fatal(err)
		}
		reopened, err := Open(ctx, store.db)
		if err != nil {
			t.Fatal(err)
		}
		if got := reopened.Settings(); got.LiveTvBufferMinutes != bounds[0] || got.LiveTvBufferGigabytes != bounds[1] {
			t.Errorf("after reopening: %d minutes, %d GB, want %v", got.LiveTvBufferMinutes, got.LiveTvBufferGigabytes, bounds)
		}
	}
}
