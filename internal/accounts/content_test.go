package accounts

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// on returns the next day from 2026-10-04 that is weekday, at hour:minute
// in the server's local time zone.
func on(weekday time.Weekday, hour, minute, second int) time.Time {
	day := time.Date(2026, time.October, 4, hour, minute, second, 0, time.Local)
	for day.Weekday() != weekday {
		day = day.AddDate(0, 0, 1)
	}
	return day
}

func TestAccessSchedulesAllowTheirHours(t *testing.T) {
	if !(User{}).AllowedAt(on(time.Monday, 3, 0, 0)) {
		t.Error("a user without schedules is refused")
	}
	workdays := User{AccessSchedules: []AccessSchedule{{Day: "Weekday", StartHour: 9, EndHour: 17}}}
	weekends := User{AccessSchedules: []AccessSchedule{{Day: "Weekend", StartHour: 0, EndHour: 24}}}
	evenings := User{AccessSchedules: []AccessSchedule{
		{Day: "Everyday", StartHour: 20, EndHour: 24},
		{Day: "Monday", StartHour: 6.5, EndHour: 8},
	}}
	for _, tc := range []struct {
		user          User
		at            time.Time
		allowed       bool
		justification string
	}{
		{workdays, on(time.Monday, 9, 0, 0), true, "a weekday at the start hour"},
		{workdays, on(time.Friday, 12, 30, 0), true, "a weekday in the middle of the day"},
		{workdays, on(time.Wednesday, 17, 0, 0), true, "a weekday at the end hour"},
		{workdays, on(time.Wednesday, 17, 0, 1), false, "a weekday just after the end hour"},
		{workdays, on(time.Tuesday, 8, 59, 59), false, "a weekday just before the start hour"},
		{workdays, on(time.Saturday, 12, 0, 0), false, "a Saturday"},
		{workdays, on(time.Sunday, 12, 0, 0), false, "a Sunday"},
		{weekends, on(time.Saturday, 0, 0, 0), true, "Saturday at midnight"},
		{weekends, on(time.Sunday, 23, 59, 59), true, "late on Sunday"},
		{weekends, on(time.Monday, 12, 0, 0), false, "a Monday"},
		{weekends, on(time.Friday, 23, 59, 59), false, "late on Friday"},
		{evenings, on(time.Tuesday, 21, 0, 0), true, "every evening"},
		{evenings, on(time.Thursday, 19, 59, 0), false, "before the evening"},
		{evenings, on(time.Monday, 7, 0, 0), true, "Monday's half-hour start"},
		{evenings, on(time.Monday, 6, 29, 0), false, "before Monday's half-hour start"},
		{evenings, on(time.Tuesday, 7, 0, 0), false, "Monday's hours on a Tuesday"},
		{evenings, on(time.Wednesday, 0, 30, 0), false, "after midnight: the evening ends at 24"},
	} {
		if got := tc.user.AllowedAt(tc.at); got != tc.allowed {
			t.Errorf("%s (%s): allowed=%v", tc.justification, tc.at.Format("Mon 15:04:05"), got)
		}
	}
	// Each day of the week stands for itself only.
	for i, day := range ScheduleDays[:7] {
		user := User{AccessSchedules: []AccessSchedule{{Day: day, StartHour: 0, EndHour: 24}}}
		for weekday := time.Sunday; weekday <= time.Saturday; weekday++ {
			if got := user.AllowedAt(on(weekday, 12, 0, 0)); got != (int(weekday) == i) {
				t.Errorf("%s schedule on %s: allowed=%v", day, weekday, got)
			}
		}
	}
}

func TestUserContentIsStoredNormalized(t *testing.T) {
	store := newStore(t)
	user := mustCreate(t, store, NewUser{Name: "child", Password: "correct horse"})
	if user.HiddenLibraries == nil || len(user.HiddenLibraries) != 0 || user.BlockedGenres == nil || len(user.BlockedGenres) != 0 ||
		user.AccessSchedules == nil || len(user.AccessSchedules) != 0 || user.Restricted() {
		t.Fatalf("new user's content settings: %+v %+v %+v", user.HiddenLibraries, user.BlockedGenres, user.AccessSchedules)
	}
	library, other := ID{1}, ID{2}
	updated, err := store.UpdateUser(t.Context(), user.ID, UserChanges{
		HiddenLibraries: &[]ID{library, other, library},
		BlockedGenres:   &[]string{" Horror ", "horror", "Thriller"},
		AccessSchedules: &[]AccessSchedule{{Day: "weekday", StartHour: 7.5, EndHour: 20}, {Day: "SATURDAY", StartHour: 0, EndHour: 24}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantSchedules := []AccessSchedule{{Day: "Weekday", StartHour: 7.5, EndHour: 20}, {Day: "Saturday", StartHour: 0, EndHour: 24}}
	if !slices.Equal(updated.HiddenLibraries, []ID{library, other}) || !slices.Equal(updated.BlockedGenres, []string{"Horror", "Thriller"}) ||
		!slices.Equal(updated.AccessSchedules, wantSchedules) || !updated.Restricted() {
		t.Errorf("stored settings: %+v %+v %+v", updated.HiddenLibraries, updated.BlockedGenres, updated.AccessSchedules)
	}
	// Other changes keep them, and devices load the user with them.
	updated, err = store.UpdateUser(t.Context(), user.ID, UserChanges{IsHidden: new(true)}, nil)
	if err != nil || len(updated.HiddenLibraries) != 2 || len(updated.BlockedGenres) != 2 || len(updated.AccessSchedules) != 2 {
		t.Errorf("settings after another change: %+v %v", updated, err)
	}
	token, _, err := store.SignInDevice(t.Context(), user.ID, device("tv"))
	if err != nil {
		t.Fatal(err)
	}
	if _, signed, err := store.DeviceByToken(t.Context(), token, "192.0.2.1"); err != nil || !slices.Equal(signed.AccessSchedules, wantSchedules) {
		t.Errorf("signed-in user's schedules: %+v %v", signed.AccessSchedules, err)
	}

	tooMany := make([]string, MaxBlockedGenres+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("Genre %d", i)
	}
	for name, genres := range map[string][]string{
		"empty":       {" "},
		"too long":    {strings.Repeat("a", MaxGenreLength+1)},
		"too many":    tooMany,
		"unprintable": {"Hor\nror"},
	} {
		if _, err := store.UpdateUser(t.Context(), user.ID, UserChanges{BlockedGenres: &genres}, nil); !errors.Is(err, ErrInvalidBlockedGenres) {
			t.Errorf("%s genre: %v", name, err)
		}
	}
	for name, schedule := range map[string]AccessSchedule{
		"unknown day":      {Day: "Someday", StartHour: 1, EndHour: 2},
		"negative start":   {Day: "Sunday", StartHour: -1, EndHour: 2},
		"end after 24":     {Day: "Sunday", StartHour: 1, EndHour: 24.5},
		"end before start": {Day: "Sunday", StartHour: 20, EndHour: 6},
		"empty":            {Day: "Sunday", StartHour: 6, EndHour: 6},
	} {
		if _, err := store.UpdateUser(t.Context(), user.ID, UserChanges{AccessSchedules: &[]AccessSchedule{schedule}}, nil); !errors.Is(err, ErrInvalidAccessSchedule) {
			t.Errorf("%s: %v", name, err)
		}
	}
	many := slices.Repeat([]AccessSchedule{{Day: "Sunday", StartHour: 1, EndHour: 2}}, MaxAccessSchedules+1)
	if _, err := store.UpdateUser(t.Context(), user.ID, UserChanges{AccessSchedules: &many}, nil); !errors.Is(err, ErrInvalidAccessSchedule) {
		t.Errorf("too many schedules: %v", err)
	}
	// Refused changes leave the settings as they were.
	if kept, _ := store.User(t.Context(), user.ID); !slices.Equal(kept.AccessSchedules, wantSchedules) || len(kept.BlockedGenres) != 2 {
		t.Errorf("settings after refused changes: %+v", kept)
	}
	// Clearing them.
	updated, err = store.UpdateUser(t.Context(), user.ID, UserChanges{HiddenLibraries: &[]ID{}, BlockedGenres: &[]string{}, AccessSchedules: &[]AccessSchedule{}}, nil)
	if err != nil || len(updated.HiddenLibraries)+len(updated.BlockedGenres)+len(updated.AccessSchedules) != 0 || updated.AccessSchedules == nil {
		t.Errorf("cleared settings: %+v %v", updated, err)
	}
}

func TestUserContentRangesAreCheckedByTheDatabase(t *testing.T) {
	store := newStore(t)
	user := mustCreate(t, store, NewUser{Name: "child", Password: "correct horse"})
	for value, valid := range map[string]bool{
		`[{"day": "Weekend", "start": 0, "end": 24}, {"day": "Monday", "start": 6.5, "end": 8}]`: true,
		`[{"day": "Weekend", "start": 0, "end": 24.5}]`:                                          false,
		`[{"day": "Weekend", "start": -1, "end": 2}]`:                                            false,
		`[{"day": "Weekend", "start": 8, "end": 6}]`:                                             false,
		`[{"day": "Someday", "start": 1, "end": 2}]`:                                             false,
		`[{"day": "Sunday"}]`:                                                                    false,
		`{"day": "Sunday", "start": 1, "end": 2}`:                                                false,
	} {
		_, err := store.db.Exec(t.Context(), "UPDATE users SET access_schedules = $2 WHERE id = $1", user.ID, value)
		if (err == nil) != valid {
			t.Errorf("%s: %v", value, err)
		}
	}
	if _, err := store.db.Exec(t.Context(), "UPDATE users SET blocked_genres = array_fill('Horror'::text, ARRAY[$2::int]) WHERE id = $1",
		user.ID, MaxBlockedGenres+1); err == nil {
		t.Error("too many blocked genres stored")
	}
}
