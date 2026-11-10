package accounts

import (
	"errors"
	"testing"
)

// 7 backups are kept by default, from 1 to 90; the database refuses what
// the store would.
func TestBackupSettingsStayInRange(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	if got := store.Settings(); got.BackupsKept != DefaultBackupsKept || DefaultBackupsKept != 7 {
		t.Errorf("default: %d", got.BackupsKept)
	}
	for _, kept := range []int{0, -1, MaxBackupsKept + 1} {
		changed := store.Settings()
		changed.BackupsKept = kept
		if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, ErrInvalidBackupsKept) {
			t.Errorf("%d kept: %v", kept, err)
		}
	}
	for _, kept := range []int{MinBackupsKept, MaxBackupsKept} {
		changed := store.Settings()
		changed.BackupsKept = kept
		if _, err := store.UpdateSettings(ctx, changed); err != nil {
			t.Fatal(err)
		}
		reopened, err := Open(ctx, store.db)
		if err != nil {
			t.Fatal(err)
		}
		if got := reopened.Settings().BackupsKept; got != kept {
			t.Errorf("after reopening: %d, want %d", got, kept)
		}
	}
	for _, column := range []string{"backups_kept = 0", "backups_kept = 91"} {
		if _, err := store.db.Exec(ctx, "UPDATE settings SET "+column); err == nil {
			t.Errorf("the database took %s", column)
		}
	}
}
