package accounts

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

// settingsOf is what a user's page sets on user, without their name,
// password and administrator status, and without what changes from one
// account to the next.
func settingsOf(user User) User {
	user.ID, user.Name, user.IsAdministrator, user.CreatedAt = ID{}, "", false, time.Time{}
	return user
}

// model is an administrator whose every setting differs from a new user's.
func model(t *testing.T, store *Store) User {
	t.Helper()
	user := mustCreate(t, store, NewUser{Name: "model", Password: "correct horse", IsAdministrator: true})
	changes := UserChanges{
		IsHidden:             new(false),
		Parental:             &ParentalControl{MaxRating: new(13), MaxSubRating: new(0), BlockUnrated: []string{"Movie"}},
		VideoTranscoding:     new(false),
		AudioTranscoding:     new(false),
		ContentDownloading:   new(false),
		PersonalAddons:       new(false),
		MaxPlaybacks:         new(2),
		MaxBitrate:           new(8_000_000),
		LiveTv:               new(false),
		SyncPlay:             new(SyncPlayJoin),
		RemoteControl:        new(false),
		HiddenLibraries:      &[]ID{{1}, {2}},
		BlockedGenres:        &[]string{"Horror"},
		AccessSchedules:      &[]AccessSchedule{{Day: "Saturday", StartHour: 8, EndHour: 20}},
		CollectionManagement: new(true),
		SubtitleManagement:   new(false),
		LiveTvManagement:     new(true),
		QualityGroup:         new(720),
	}
	user, err := store.UpdateUser(t.Context(), user.ID, changes, nil)
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func mustInvite(t *testing.T, store *Store, invite NewInvite) string {
	t.Helper()
	_, token, err := store.CreateInvite(t.Context(), invite)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// An account created through an invite copies the model's settings, but
// never its administrator status; without a model, it has those of a user
// created from Create a user, hidden from the sign-in screen.
func TestInvitedAccountsCopyTheirModelOrTakeNewUserDefaults(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	creator := mustCreate(t, store, NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true})
	modelUser := model(t, store)

	modelled := mustInvite(t, store, NewInvite{MaxUses: 1, Days: 7, Model: &modelUser.ID, CreatedBy: creator.ID})
	guest, invite, err := store.AcceptInvite(ctx, modelled, "guest", "battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if guest.IsAdministrator || guest.Name != "guest" || invite.CreatedBy == nil || invite.CreatedBy.Name != "admin" {
		t.Errorf("guest %+v through invite %+v", guest, invite)
	}
	if got, want := settingsOf(guest), settingsOf(modelUser); !reflect.DeepEqual(got, want) {
		t.Errorf("guest's settings:\n%+v\nwant the model's:\n%+v", got, want)
	}
	if _, err := store.Authenticate(ctx, "guest", "battery staple"); err != nil {
		t.Errorf("the guest cannot sign in: %v", err)
	}

	plain := mustInvite(t, store, NewInvite{MaxUses: 1, CreatedBy: creator.ID})
	member, _, err := store.AcceptInvite(ctx, plain, "member", "battery staple")
	if err != nil {
		t.Fatal(err)
	}
	created := mustCreate(t, store, NewUser{Name: "created", Password: "battery staple", IsHidden: true})
	if got, want := settingsOf(member), settingsOf(created); !reflect.DeepEqual(got, want) {
		t.Errorf("member's settings:\n%+v\nwant a new user's:\n%+v", got, want)
	}
}

// Two guests racing on the last use of an invite create one account.
func TestTheLastUseOfAnInviteGoesToOneGuest(t *testing.T) {
	store := newStore(t)
	creator := mustCreate(t, store, NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true})
	token := mustInvite(t, store, NewInvite{MaxUses: 1, Days: 1, CreatedBy: creator.ID})

	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i, name := range []string{"first", "second"} {
		wg.Go(func() { _, _, errs[i] = store.AcceptInvite(t.Context(), token, name, "battery staple") })
	}
	wg.Wait()
	created, usedUp := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			created++
		case errors.Is(err, ErrInviteUsedUp):
			usedUp++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	users, err := store.Users(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if created != 1 || usedUp != 1 || len(users) != 2 {
		t.Errorf("%d created, %d used up, %d users; want one guest created", created, usedUp, len(users))
	}
}

// An invite refuses guests once used up, expired or revoked, or once its
// model is deleted; a token that is no invite's, a secret guessed for a
// known invite included, is unknown. A guest refused for their name uses
// nothing up.
func TestInvitesRefuseGuestsOnceUsedUpExpiredOrRevoked(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	creator := mustCreate(t, store, NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true})
	now := time.Now()
	store.now = func() time.Time { return now }

	twice := mustInvite(t, store, NewInvite{MaxUses: 2, Days: 1, CreatedBy: creator.ID})
	if _, _, err := store.AcceptInvite(ctx, twice, "admin", "battery staple"); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("a taken name: %v", err)
	}
	for _, name := range []string{"one", "two"} {
		if _, _, err := store.AcceptInvite(ctx, twice, name, "battery staple"); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}

	expiring := mustInvite(t, store, NewInvite{MaxUses: 1, Days: 1, CreatedBy: creator.ID})
	revoked, revokedToken, err := store.CreateInvite(ctx, NewInvite{MaxUses: 1, CreatedBy: creator.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RevokeInvite(ctx, revoked.ID); err != nil {
		t.Fatal(err)
	}
	doomed := mustCreate(t, store, NewUser{Name: "doomed", Password: "correct horse"})
	orphan := mustInvite(t, store, NewInvite{MaxUses: 1, Model: &doomed.ID, CreatedBy: creator.ID})
	if err := store.DeleteUser(ctx, doomed.ID); err != nil {
		t.Fatal(err)
	}
	id, secret, _ := parseInviteToken(expiring)
	secret[0] ^= 1
	guessed := inviteToken(id, secret)

	now = now.Add(24 * time.Hour)
	for _, tc := range []struct {
		name, token string
		want        error
	}{
		{"used up", twice, ErrInviteUsedUp},
		{"expired", expiring, ErrInviteExpired},
		{"revoked", revokedToken, ErrInviteRevoked},
		{"model deleted", orphan, ErrInviteRevoked},
		{"guessed secret", guessed, ErrInviteUnknown},
		{"malformed", "not-a-token", ErrInviteUnknown},
	} {
		if _, err := store.InviteByToken(ctx, tc.token); !errors.Is(err, tc.want) {
			t.Errorf("%s: read %v, want %v", tc.name, err, tc.want)
		}
		if _, _, err := store.AcceptInvite(ctx, tc.token, "late "+tc.name, "battery staple"); !errors.Is(err, tc.want) {
			t.Errorf("%s: accepted %v, want %v", tc.name, err, tc.want)
		}
	}
	users, err := store.Users(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 3 {
		t.Errorf("%d users, want the administrator and the two guests", len(users))
	}
}
