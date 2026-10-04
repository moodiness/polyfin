package accounts

import (
	"errors"
	"testing"
)

// Recordings start Jellyfin's way, without padding, and are kept until
// deleted; the paddings and the days stay in range.
func TestRecordingSettingsStayInRange(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	got := store.Settings()
	if got.RecordingPrePadding != DefaultRecordingPrePadding || got.RecordingPostPadding != DefaultRecordingPostPadding ||
		got.RecordingRetentionDays != DefaultRecordingRetentionDays || DefaultRecordingPrePadding != 0 || DefaultRecordingRetentionDays != 0 {
		t.Errorf("defaults: %+v", got)
	}
	for _, tc := range []struct {
		name   string
		change func(*Settings)
		err    error
	}{
		{"negative padding before", func(s *Settings) { s.RecordingPrePadding = -1 }, ErrInvalidRecordingPadding},
		{"padding before too long", func(s *Settings) { s.RecordingPrePadding = MaxRecordingPadding + 1 }, ErrInvalidRecordingPadding},
		{"padding after too long", func(s *Settings) { s.RecordingPostPadding = MaxRecordingPadding + 1 }, ErrInvalidRecordingPadding},
		{"negative days", func(s *Settings) { s.RecordingRetentionDays = -1 }, ErrInvalidRecordingRetentionDays},
		{"too many days", func(s *Settings) { s.RecordingRetentionDays = MaxRecordingRetentionDays + 1 }, ErrInvalidRecordingRetentionDays},
	} {
		changed := store.Settings()
		tc.change(&changed)
		if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, tc.err) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.err)
		}
	}
	changed := store.Settings()
	changed.RecordingPrePadding, changed.RecordingPostPadding, changed.RecordingRetentionDays = MaxRecordingPadding, 90, MaxRecordingRetentionDays
	if _, err := store.UpdateSettings(ctx, changed); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, store.db)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Settings(); got != changed {
		t.Errorf("after reopening: %+v, want %+v", got, changed)
	}
	// The administrator's setup keeps them.
	if _, err := store.CreateFirstAdministrator(ctx, "admin", "correct horse", "fr"); err != nil {
		t.Fatal(err)
	}
	if after := store.Settings(); after.RecordingPrePadding != MaxRecordingPadding || after.RecordingPostPadding != 90 {
		t.Errorf("after the setup: %+v", after)
	}
}

// Administrators may record Live TV unless that is taken away; other users
// may not unless given it.
func TestOnlyAdministratorsRecordByDefault(t *testing.T) {
	store := newStore(t)
	admin := mustCreate(t, store, NewUser{Name: "boss", Password: "correct horse", IsAdministrator: true})
	member := mustCreate(t, store, NewUser{Name: "member", Password: "correct horse"})
	if !admin.LiveTvManagement || member.LiveTvManagement {
		t.Fatalf("may record: administrator %v, member %v", admin.LiveTvManagement, member.LiveTvManagement)
	}
	yes := true
	updated, err := store.UpdateUser(t.Context(), member.ID, UserChanges{LiveTvManagement: &yes}, nil)
	if err != nil || !updated.LiveTvManagement {
		t.Fatalf("given the permission: %v %+v", err, updated)
	}
	// A change that leaves it out keeps it.
	if updated, err = store.UpdateUser(t.Context(), member.ID, UserChanges{IsHidden: &yes}, nil); err != nil || !updated.LiveTvManagement {
		t.Errorf("after another change: %v %+v", err, updated)
	}
}
