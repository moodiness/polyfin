package accounts

import (
	"errors"
	"testing"
	"time"
)

// Connected accounts are imported every 6 hours by default, and otherwise
// at one of the intervals offered, from 15 minutes to 24 hours; the
// database refuses what the store would.
func TestImportIntervalIsOneOfTheChoices(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	if got := store.Settings().ImportEvery(); got != 6*time.Hour {
		t.Errorf("default: %v", got)
	}
	for _, minutes := range []int{0, 14, 45, 1441, -15} {
		changed := store.Settings()
		changed.ImportEveryMinutes = minutes
		if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, ErrInvalidImportEveryMinutes) {
			t.Errorf("%d minutes: %v", minutes, err)
		}
	}
	for _, minutes := range []int{15, 1440} {
		changed := store.Settings()
		changed.ImportEveryMinutes = minutes
		if _, err := store.UpdateSettings(ctx, changed); err != nil {
			t.Fatal(err)
		}
		reopened, err := Open(ctx, store.db)
		if err != nil {
			t.Fatal(err)
		}
		if got := reopened.Settings().ImportEvery(); got != time.Duration(minutes)*time.Minute {
			t.Errorf("after reopening: %v, want %d minutes", got, minutes)
		}
	}
	for _, minutes := range []string{"14", "45", "1441"} {
		if _, err := store.db.Exec(ctx, "UPDATE settings SET import_every_minutes = "+minutes); err == nil {
			t.Errorf("the database took %s minutes", minutes)
		}
	}
}
