package accounts

import (
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

// From its end date, an account cannot sign in, its devices and admin
// sessions are refused, and the end is returned once to be recorded; a
// later date brings it all back, devices included, and Never too.
func TestAnAccountIsRefusedFromItsEndDateUntilALaterOne(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	now := time.Now()
	store.now = func() time.Time { return now }
	today := Day(now)
	guest := mustCreate(t, store, NewUser{Name: "guest", Password: "correct horse"})
	token, _, err := store.SignInDevice(ctx, guest.ID, device("phone"))
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := store.CreateAdminSession(ctx, guest.ID)
	if err != nil {
		t.Fatal(err)
	}
	end := func(on time.Time) {
		t.Helper()
		if _, err := store.UpdateUser(ctx, guest.ID, UserChanges{EndsOn: &on}, nil); err != nil {
			t.Fatal(err)
		}
	}
	refused := func(want bool) {
		t.Helper()
		_, signInErr := store.Authenticate(ctx, "guest", "correct horse")
		_, sessionErr := store.AdminSession(ctx, session)
		if got := [3]bool{errors.Is(signInErr, ErrDisabled), !signedIn(ctx, store, token), errors.Is(sessionErr, ErrNotFound)}; got != [3]bool{want, want, want} {
			t.Errorf("sign-in, device and admin session refused: %v, want all %v", got, want)
		}
	}
	recorded := func(want int) {
		t.Helper()
		ended, err := store.EndedAccounts(ctx)
		if err != nil || len(ended) != want || want == 1 && ended[0].ID != guest.ID {
			t.Errorf("ended accounts: %+v %v, want %d", ended, err, want)
		}
	}

	end(today.AddDate(0, 0, 1))
	refused(false)
	recorded(0)

	end(today)
	refused(true)
	recorded(1)
	recorded(0)

	end(today.AddDate(0, 0, 1))
	refused(false)
	end(today.AddDate(0, 0, -3))
	refused(true)
	recorded(1)

	end(time.Time{})
	refused(false)
	if user, err := store.User(ctx, guest.ID); err != nil || user.EndsOn != nil {
		t.Errorf("after Never: %+v %v", user.EndsOn, err)
	}
}

// An administrator's account never ends: it cannot be given an end date,
// and making a user an administrator clears theirs.
func TestAnAdministratorNeverEnds(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	admin := mustCreate(t, store, NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true})
	member := mustCreate(t, store, NewUser{Name: "member", Password: "correct horse"})
	next := Day(time.Now()).AddDate(0, 0, 7)

	if _, err := store.UpdateUser(ctx, admin.ID, UserChanges{EndsOn: &next}, nil); !errors.Is(err, ErrAdministratorEnds) {
		t.Errorf("an end date for an administrator: %v", err)
	}
	if _, err := store.UpdateUser(ctx, member.ID, UserChanges{IsAdministrator: new(true), EndsOn: &next}, nil); !errors.Is(err, ErrAdministratorEnds) {
		t.Errorf("an end date for a user made an administrator: %v", err)
	}
	if user, err := store.UpdateUser(ctx, member.ID, UserChanges{EndsOn: &next}, nil); err != nil || user.EndsOn == nil || !user.EndsOn.Equal(next) {
		t.Fatalf("a member's end date: %+v %v", user.EndsOn, err)
	}
	if user, err := store.UpdateUser(ctx, member.ID, UserChanges{IsAdministrator: new(true)}, nil); err != nil || user.EndsOn != nil {
		t.Errorf("made an administrator, the member keeps their end date: %+v %v", user.EndsOn, err)
	}
	// A former administrator may be given one.
	if user, err := store.UpdateUser(ctx, member.ID, UserChanges{IsAdministrator: new(false), EndsOn: &next}, nil); err != nil || user.EndsOn == nil {
		t.Errorf("an administrator made a member with an end date: %+v %v", user.EndsOn, err)
	}
}

// An account an invite creates ends exactly the invite's days of 24 hours
// after its creation, to the minute, as the link's own expiry counts: used
// at 18:40, an invite of 1 day gives an account allowed at 18:39 the next
// day and refused at 18:40, and one of 7 days the same 7 days later.
// Without days, it never ends.
func TestAnInvitesAccountsEndToTheMinuteAfterItsDays(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	var clock atomic.Int64
	at := func(when time.Time) { clock.Store(when.UnixNano()) }
	store.now = func() time.Time { return time.Unix(0, clock.Load()) }
	used := time.Date(2026, time.November, 8, 18, 40, 0, 0, time.Local)
	at(used)
	creator := mustCreate(t, store, NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true})

	if _, _, err := store.CreateInvite(ctx, NewInvite{MaxUses: 1, CreatedBy: creator.ID, AccountDays: MaxInviteDays + 1}); !errors.Is(err, ErrInvalidInviteAccountEnd) {
		t.Errorf("an end past the bound: %v", err)
	}
	for _, days := range []int{1, 7} {
		at(used)
		name := fmt.Sprintf("guest%d", days)
		token := mustInvite(t, store, NewInvite{MaxUses: 1, CreatedBy: creator.ID, AccountDays: days})
		if _, invite, err := store.AcceptInvite(ctx, token, name, "battery staple"); err != nil || invite.AccountDays != days {
			t.Fatalf("joining through an invite of %d days: %d %v", days, invite.AccountDays, err)
		}
		end := used.Add(time.Duration(days) * 24 * time.Hour)
		for _, step := range []struct {
			at      time.Time
			refused bool
		}{{used, false}, {end.Add(-time.Minute), false}, {end, true}, {end.Add(time.Hour), true}} {
			at(step.at)
			_, err := store.Authenticate(ctx, name, "battery staple")
			if refused := errors.Is(err, ErrDisabled); refused != step.refused || err != nil && !refused {
				t.Errorf("%s at %v: %v, want refused %v", name, step.at, err, step.refused)
			}
		}
	}
	at(used)
	plain := mustInvite(t, store, NewInvite{MaxUses: 1, CreatedBy: creator.ID})
	if member, _, err := store.AcceptInvite(ctx, plain, "member", "battery staple"); err != nil || member.EndsOn != nil || member.EndsAt != nil {
		t.Errorf("member through an invite without an end: %v %v %v", member.EndsOn, member.EndsAt, err)
	}
}

// The exact end an invite gives is kept as a time, not a date, through
// the account's other changes, and recorded once on being reached. A date
// set since replaces it, Never clears both, and so does making the user an
// administrator.
func TestAnExactEndStaysUntilADateOrNeverReplacesIt(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	var clock atomic.Int64
	at := func(when time.Time) { clock.Store(when.UnixNano()) }
	store.now = func() time.Time { return time.Unix(0, clock.Load()) }
	used := time.Date(2026, time.November, 8, 18, 40, 0, 0, time.Local)
	end := used.Add(24 * time.Hour)
	at(used)
	creator := mustCreate(t, store, NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true})
	join := func(name string) User {
		t.Helper()
		at(used)
		user, _, err := store.AcceptInvite(ctx, mustInvite(t, store, NewInvite{MaxUses: 1, CreatedBy: creator.ID, AccountDays: 1}), name, "battery staple")
		if err != nil {
			t.Fatal(err)
		}
		return user
	}
	exact := func(user User, err error, what string) {
		t.Helper()
		if err != nil || user.EndsOn != nil || user.EndsAt == nil || !user.EndsAt.Equal(end) {
			t.Errorf("%s: ends on %v at %v (%v), want at %v", what, user.EndsOn, user.EndsAt, err, end)
		}
	}
	recorded := func(want ...ID) {
		t.Helper()
		ended, err := store.EndedAccounts(ctx)
		got := make([]ID, len(ended))
		for i, user := range ended {
			got[i] = user.ID
		}
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("ended accounts: %v %v, want %v", got, err, want)
		}
	}
	guest := join("guest")
	exact(guest, nil, "joined")

	at(end.Add(-time.Minute))
	recorded()
	updated, err := store.UpdateUser(ctx, guest.ID, UserChanges{IsHidden: new(false)}, nil)
	exact(updated, err, "shown on the sign-in screen")
	at(end)
	recorded(guest.ID)
	recorded()
	updated, err = store.UpdateUser(ctx, guest.ID, UserChanges{IsHidden: new(true), Name: new("guest2")}, nil)
	exact(updated, err, "an unrelated change")
	if !updated.Ended(end) {
		t.Error("the guest's end is gone after an unrelated change")
	}
	recorded()

	date := Day(end).AddDate(0, 0, 3)
	updated, err = store.UpdateUser(ctx, guest.ID, UserChanges{EndsOn: &date}, nil)
	if err != nil || updated.EndsAt != nil || updated.EndsOn == nil || !updated.EndsOn.Equal(date) || updated.Ended(end) {
		t.Errorf("a date set: ends on %v at %v (%v), want on %v", updated.EndsOn, updated.EndsAt, err, date)
	}
	recorded()
	at(time.Date(2026, time.November, 12, 0, 0, 0, 0, time.Local))
	recorded(guest.ID)

	other := join("other")
	updated, err = store.UpdateUser(ctx, other.ID, UserChanges{EndsOn: new(time.Time{})}, nil)
	if err != nil || updated.EndsAt != nil || updated.EndsOn != nil {
		t.Errorf("Never: ends on %v at %v (%v)", updated.EndsOn, updated.EndsAt, err)
	}
	promoted := join("promoted")
	updated, err = store.UpdateUser(ctx, promoted.ID, UserChanges{IsAdministrator: new(true)}, nil)
	if err != nil || updated.EndsAt != nil || updated.EndsOn != nil {
		t.Errorf("made an administrator: ends on %v at %v (%v)", updated.EndsOn, updated.EndsAt, err)
	}
	at(end.Add(time.Hour))
	recorded()
}

// A date end applies from that date's start, in the server's time zone,
// and an exact one from its instant.
func TestEndedReadsADateOrAnExactTime(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.November, 9, 18, 40, 0, 0, time.Local)
	today, tomorrow := Day(now), Day(now).AddDate(0, 0, 1)
	for _, tc := range []struct {
		name string
		user User
		want bool
	}{
		{"never", User{}, false},
		{"today", User{EndsOn: &today}, true},
		{"tomorrow", User{EndsOn: &tomorrow}, false},
		{"a minute ago", User{EndsAt: new(now.Add(-time.Minute))}, true},
		{"now", User{EndsAt: new(now)}, true},
		{"in a minute", User{EndsAt: new(now.Add(time.Minute))}, false},
	} {
		if got := tc.user.Ended(now); got != tc.want {
			t.Errorf("%s: ended %v, want %v", tc.name, got, tc.want)
		}
	}
}
