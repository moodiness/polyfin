package accounts

import (
	"errors"
	"testing"
)

// The database is backed up at 4 by default, at an hour from 0 to 23,
// and 7 backups are kept by default, from 1 to 90; the database refuses
// what the store would.
func TestBackupSettingsStayInRange(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	if got := store.Settings(); got.BackupHour != DefaultBackupHour || DefaultBackupHour != 4 ||
		got.BackupsKept != DefaultBackupsKept || DefaultBackupsKept != 7 {
		t.Errorf("defaults: %d %d", got.BackupHour, got.BackupsKept)
	}
	for _, hour := range []int{-1, 24} {
		changed := store.Settings()
		changed.BackupHour = hour
		if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, ErrInvalidBackupHour) {
			t.Errorf("hour %d: %v", hour, err)
		}
	}
	for _, kept := range []int{0, -1, MaxBackupsKept + 1} {
		changed := store.Settings()
		changed.BackupsKept = kept
		if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, ErrInvalidBackupsKept) {
			t.Errorf("%d kept: %v", kept, err)
		}
	}
	for _, values := range [][2]int{{0, MinBackupsKept}, {23, MaxBackupsKept}} {
		changed := store.Settings()
		changed.BackupHour, changed.BackupsKept = values[0], values[1]
		if _, err := store.UpdateSettings(ctx, changed); err != nil {
			t.Fatal(err)
		}
		reopened, err := Open(ctx, store.db)
		if err != nil {
			t.Fatal(err)
		}
		if got := reopened.Settings(); got.BackupHour != values[0] || got.BackupsKept != values[1] {
			t.Errorf("after reopening: %d %d, want %v", got.BackupHour, got.BackupsKept, values)
		}
	}
	for _, column := range []string{"backup_hour = 24", "backups_kept = 0", "backups_kept = 91"} {
		if _, err := store.db.Exec(ctx, "UPDATE settings SET "+column); err == nil {
			t.Errorf("the database took %s", column)
		}
	}
}
