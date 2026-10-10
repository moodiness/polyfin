package accounts

import (
	"errors"
	"testing"
)

// The weekly summary is sent on Monday at 9:00 by default, and otherwise
// on a day from 0 (Sunday) to 6 (Saturday), at an hour from 0 to 23; the
// database refuses what the store would.
func TestWeeklySummaryDayAndHourStayInRange(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	if got := store.Settings(); got.WeeklySummaryDay != 1 || got.WeeklySummaryHour != 9 {
		t.Errorf("default: day %d at %d", got.WeeklySummaryDay, got.WeeklySummaryHour)
	}
	for _, day := range []int{-1, 7} {
		changed := store.Settings()
		changed.WeeklySummaryDay = day
		if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, ErrInvalidWeeklySummaryDay) {
			t.Errorf("day %d: %v", day, err)
		}
	}
	for _, hour := range []int{-1, 24} {
		changed := store.Settings()
		changed.WeeklySummaryHour = hour
		if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, ErrInvalidWeeklySummaryHour) {
			t.Errorf("hour %d: %v", hour, err)
		}
	}
	for _, when := range [][2]int{{0, 0}, {6, 23}} {
		changed := store.Settings()
		changed.WeeklySummaryDay, changed.WeeklySummaryHour = when[0], when[1]
		if _, err := store.UpdateSettings(ctx, changed); err != nil {
			t.Fatal(err)
		}
		reopened, err := Open(ctx, store.db)
		if err != nil {
			t.Fatal(err)
		}
		if got := reopened.Settings(); got.WeeklySummaryDay != when[0] || got.WeeklySummaryHour != when[1] {
			t.Errorf("after reopening: day %d at %d, want %v", got.WeeklySummaryDay, got.WeeklySummaryHour, when)
		}
	}
	for _, column := range []string{"weekly_summary_day = -1", "weekly_summary_day = 7", "weekly_summary_hour = -1", "weekly_summary_hour = 24"} {
		if _, err := store.db.Exec(ctx, "UPDATE settings SET "+column); err == nil {
			t.Errorf("the database took %s", column)
		}
	}
}
