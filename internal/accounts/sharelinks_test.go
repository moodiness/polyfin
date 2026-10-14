package accounts

import (
	"errors"
	"testing"
	"time"
)

// sharer is a member allowed to share, on a server that allows it.
func sharer(t *testing.T, store *Store) User {
	t.Helper()
	settings := store.Settings()
	settings.ShareLinks = true
	if _, err := store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	user := mustCreate(t, store, NewUser{Name: "alex", Password: "correct horse"})
	user, err := store.UpdateUser(t.Context(), user.ID, UserChanges{CanShare: new(true)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func newShareLink(user User) NewShareLink {
	return NewShareLink{User: user.ID, Item: ID{1}, Kind: ShareMovie, Title: "Example Movie (2008)", Expiry: DefaultShareLinkExpiry, MaxViewers: 1}
}

// Share links are off by default, for the server and for every user,
// administrators included; off, no link is made, and those made are
// refused without being deleted.
func TestShareLinksAreOffByDefault(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	if store.Settings().ShareLinks {
		t.Error("share links are on by default")
	}
	administrator := mustCreate(t, store, NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true})
	if administrator.CanShare {
		t.Error("an administrator may share by default")
	}
	if _, _, err := store.CreateShareLink(ctx, newShareLink(administrator)); !errors.Is(err, ErrShareLinksOff) {
		t.Errorf("a link without the permission: %v", err)
	}
	user := sharer(t, store)
	link, token, err := store.CreateShareLink(ctx, newShareLink(user))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ShareLinkByToken(ctx, token); err != nil {
		t.Fatalf("an active link: %v", err)
	}
	settings := store.Settings()
	settings.ShareLinks = false
	if _, err := store.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ShareLinkByToken(ctx, token); !errors.Is(err, ErrShareLinksOff) {
		t.Errorf("a link while the server does not share: %v", err)
	}
	if _, _, err := store.CreateShareLink(ctx, newShareLink(user)); !errors.Is(err, ErrShareLinksOff) {
		t.Errorf("a link made while the server does not share: %v", err)
	}
	if kept, err := store.ShareLink(ctx, link.ID); err != nil || kept.RevokedAt != nil {
		t.Errorf("the link is gone or revoked while sharing is off: %+v %v", kept, err)
	}
}

// Tokens are checked as an invite's: a guessed secret, a malformed token
// or another link's ID are unknown; a user who loses the permission, or is
// disabled, loses their links.
func TestShareLinkTokensAndRevocations(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	user := sharer(t, store)
	other := mustCreate(t, store, NewUser{Name: "sam", Password: "correct horse"})
	link, token, err := store.CreateShareLink(ctx, newShareLink(user))
	if err != nil {
		t.Fatal(err)
	}
	if link.Plays != 0 || link.MaxViewers != 1 || link.State(time.Now()) != ShareLinkActive {
		t.Errorf("made: %+v", link)
	}
	id, secret, _ := parseLinkToken(token)
	secret[0] ^= 1
	for name, guessed := range map[string]string{"guessed secret": linkToken(id, secret), "malformed": "not-a-token", "empty": ""} {
		if _, _, err := store.ShareLinkByToken(ctx, guessed); !errors.Is(err, ErrShareLinkUnknown) {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, bad := range []NewShareLink{
		{User: user.ID, Item: ID{1}, Kind: ShareMovie, Title: "x", Expiry: 2 * time.Hour, MaxViewers: 1},
		{User: user.ID, Item: ID{1}, Kind: ShareMovie, Title: "x", Expiry: time.Hour, MaxViewers: MaxShareLinkViewers + 1},
		{User: user.ID, Item: ID{1}, Kind: "series", Title: "x", Expiry: time.Hour, MaxViewers: 1},
	} {
		if _, _, err := store.CreateShareLink(ctx, bad); err == nil {
			t.Errorf("made %+v", bad)
		}
	}
	// Another user cannot revoke it; its owner can, once.
	if _, _, err := store.RevokeShareLink(ctx, link.ID, &other.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("another user's revocation: %v", err)
	}
	if _, revoked, err := store.RevokeShareLink(ctx, link.ID, &user.ID); err != nil || !revoked {
		t.Fatalf("revoking: %v %v", revoked, err)
	}
	if _, revoked, _ := store.RevokeShareLink(ctx, link.ID, nil); revoked {
		t.Error("revoked twice")
	}
	if _, _, err := store.ShareLinkByToken(ctx, token); !errors.Is(err, ErrShareLinkRevoked) {
		t.Errorf("a revoked link: %v", err)
	}
	for _, change := range []UserChanges{{CanShare: new(false)}, {IsDisabled: new(true)}} {
		user, err = store.UpdateUser(ctx, user.ID, UserChanges{CanShare: new(true), IsDisabled: new(false)}, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, token, err := store.CreateShareLink(ctx, newShareLink(user))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.UpdateUser(ctx, user.ID, change, nil); err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.ShareLinkByToken(ctx, token); !errors.Is(err, ErrShareLinkRevoked) {
			t.Errorf("after %+v: %v", change, err)
		}
	}
}

// A link expires at its time and is recorded once; share_played tells of
// a link at most once an hour.
func TestShareLinksExpireAndNotifyOnceAnHour(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	now := time.Now()
	store.now = func() time.Time { return now }
	user := sharer(t, store)
	link, token, err := store.CreateShareLink(ctx, NewShareLink{User: user.ID, Item: ID{1}, Kind: ShareEpisode, Title: "Series S01E02",
		Expiry: time.Hour, MaxViewers: 2})
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []bool{true, false} {
		if notify, err := store.ShareLinkToNotify(ctx, link.ID); err != nil || notify != want {
			t.Errorf("play %d: notify %v %v", i, notify, err)
		}
	}
	now = now.Add(59 * time.Minute)
	if notify, _ := store.ShareLinkToNotify(ctx, link.ID); notify {
		t.Error("told again within the hour")
	}
	if expired, err := store.ExpiredShareLinks(ctx); err != nil || len(expired) != 0 {
		t.Errorf("expired early: %+v %v", expired, err)
	}
	if _, _, err := store.ShareLinkByToken(ctx, token); err != nil {
		t.Errorf("a link before its expiry: %v", err)
	}
	now = now.Add(time.Minute)
	if notify, _ := store.ShareLinkToNotify(ctx, link.ID); !notify {
		t.Error("not told again after an hour")
	}
	if _, _, err := store.ShareLinkByToken(ctx, token); !errors.Is(err, ErrShareLinkExpired) {
		t.Errorf("an expired link: %v", err)
	}
	if expired, err := store.ExpiredShareLinks(ctx); err != nil || len(expired) != 1 || expired[0].ID != link.ID {
		t.Errorf("recorded: %+v %v", expired, err)
	}
	if expired, _ := store.ExpiredShareLinks(ctx); len(expired) != 0 {
		t.Errorf("recorded twice: %+v", expired)
	}
	if _, revoked, _ := store.RevokeShareLink(ctx, link.ID, nil); revoked {
		t.Error("an expired link was revoked")
	}
}
