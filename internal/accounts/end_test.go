package accounts

import (
	"errors"
	"testing"
	"time"
)

// From its end date, an account cannot sign in, its devices and admin
// sessions are refused, and the end is returned once to be recorded; a
// later date brings it all back, devices included, and Never too.
func TestAnAccountIsRefusedFromItsEndDateUntilALaterOne(t *testing.T) {
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

// Each account an invite creates ends the invite's days after it is
// created; without them, it never ends.
func TestAnInvitesAccountsEndAfterItsDays(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	now := time.Now()
	store.now = func() time.Time { return now }
	creator := mustCreate(t, store, NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true})

	if _, _, err := store.CreateInvite(ctx, NewInvite{MaxUses: 1, CreatedBy: creator.ID, AccountDays: MaxInviteDays + 1}); !errors.Is(err, ErrInvalidInviteAccountEnd) {
		t.Errorf("an end past the bound: %v", err)
	}
	ending := mustInvite(t, store, NewInvite{MaxUses: 2, CreatedBy: creator.ID, AccountDays: 7})
	guest, invite, err := store.AcceptInvite(ctx, ending, "guest", "battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if want := Day(now).AddDate(0, 0, 7); guest.EndsOn == nil || !guest.EndsOn.Equal(want) || invite.AccountDays != 7 {
		t.Errorf("guest ends on %v through an invite of %d days, want %v", guest.EndsOn, invite.AccountDays, want)
	}
	plain := mustInvite(t, store, NewInvite{MaxUses: 1, CreatedBy: creator.ID})
	if member, _, err := store.AcceptInvite(ctx, plain, "member", "battery staple"); err != nil || member.EndsOn != nil {
		t.Errorf("member through an invite without an end: %+v %v", member.EndsOn, err)
	}
}
