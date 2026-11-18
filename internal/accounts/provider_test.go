package accounts

import (
	"errors"
	"testing"
	"time"
)

const testIssuer = "https://id.example.org"

// at is someone at the test provider.
func at(subject, name, email string) ProviderIdentity {
	return ProviderIdentity{Issuer: testIssuer, Subject: subject, Name: name, Email: email}
}

// allowLink gives user the permission to be linked at the next sign-in
// through the provider.
func allowLink(t *testing.T, store *Store, user User) {
	t.Helper()
	if _, err := store.UpdateUser(t.Context(), user.ID, UserChanges{ProviderLinkAllowed: new(true)}, nil); err != nil {
		t.Fatal(err)
	}
}

// session opens an admin session of user and returns its token's hash.
func session(t *testing.T, store *Store, user User) []byte {
	t.Helper()
	token, _, err := store.CreateAdminSession(t.Context(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	return HashAdminToken(token)
}

// An account is found by its link alone. Someone not linked gets no
// existing account by the name their claim or their email gives, an
// administrator's or a member's, whether accounts are created or not, and
// no account is made under a name that is taken; once linked, they get
// their account whatever their claims say.
func TestProviderUserFindsAccountsByTheirLinkAlone(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	root := mustCreate(t, store, NewUser{Name: "root", Password: "correct horse", IsAdministrator: true})
	carol := mustCreate(t, store, NewUser{Name: "carol@example.org", Password: "correct horse"})
	for name, identity := range map[string]ProviderIdentity{
		"an administrator's name":       at("sub-1", "Root", ""),
		"a member's name, as the email": at("sub-2", `CORP\carol`, "Carol@example.org"),
		"a member's name, as the claim": {Issuer: testIssuer, Subject: "sub-3", Name: "carol@example.org", Administrator: true},
	} {
		for _, create := range []bool{false, true} {
			if user, created, err := store.ProviderUser(ctx, identity, create); !errors.Is(err, ErrNoProviderAccount) {
				t.Errorf("%s, create %v: %v (account %q, created %v), want ErrNoProviderAccount", name, create, err, user.Name, created)
			}
		}
	}
	users, err := store.Users(ctx)
	if err != nil || len(users) != 2 {
		t.Fatalf("accounts afterwards: %d %v, want the two", len(users), err)
	}
	for _, user := range users {
		if user.ProviderIssuer != "" || user.IsAdministrator != (user.ID == root.ID) {
			t.Errorf("%s: linked at %q, administrator %v", user.Name, user.ProviderIssuer, user.IsAdministrator)
		}
	}

	// A free name is given to a new account, which the person then keeps
	// whatever their claims become.
	dave, created, err := store.ProviderUser(ctx, at("sub-dave", "dave", ""), true)
	if err != nil || !created || dave.Name != "dave" || !dave.ProviderOnly || dave.ProviderIssuer != testIssuer {
		t.Fatalf("a free name: %+v, created %v, %v", dave, created, err)
	}
	again, created, err := store.ProviderUser(ctx, at("sub-dave", "root", "carol@example.org"), true)
	if err != nil || created || again.ID != dave.ID {
		t.Errorf("linked, under other claims: account %q, created %v, %v", again.Name, created, err)
	}
	if !dave.ProviderLinked(Settings{ProviderIssuer: testIssuer}) || dave.ProviderLinked(Settings{ProviderIssuer: "https://other.example.org"}) ||
		carol.ProviderLinked(Settings{}) {
		t.Error("ProviderLinked does not follow the issuer set up")
	}
}

// The permission an administrator gives links the account to the next
// person signed in under its name, by the claim or the verified email,
// once. It is not given for an administrator nor for an account with
// two-step sign-in, and links neither when the account became one since.
func TestProviderUserLinksAnAllowedAccountOnce(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	root := mustCreate(t, store, NewUser{Name: "root", Password: "correct horse", IsAdministrator: true})
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	carol := mustCreate(t, store, NewUser{Name: "carol@example.org", Password: "correct horse"})
	allowLink(t, store, alice)
	allowLink(t, store, carol)
	linked, created, err := store.ProviderUser(ctx, at("sub-alice", "Alice", ""), false)
	if err != nil || created || linked.ID != alice.ID || linked.ProviderIssuer != testIssuer || linked.ProviderLinkAllowed {
		t.Fatalf("by the claim: %+v, created %v, %v", linked, created, err)
	}
	if _, _, err := store.ProviderUser(ctx, at("sub-another", "alice", ""), true); !errors.Is(err, ErrNoProviderAccount) {
		t.Errorf("someone else under the name: %v", err)
	}
	// The permission given again does not move the link to someone else.
	allowLink(t, store, alice)
	if _, _, err := store.ProviderUser(ctx, at("sub-another", "alice", ""), false); !errors.Is(err, ErrNoProviderAccount) {
		t.Errorf("someone else under the name of a linked account: %v", err)
	}
	if linked, _, err = store.ProviderUser(ctx, at("sub-carol", "nobody", "Carol@Example.org"), false); err != nil || linked.ID != carol.ID {
		t.Errorf("by the email: %q %v", linked.Name, err)
	}

	bob := mustCreate(t, store, NewUser{Name: "bob", Password: "correct horse"})
	twoStepOn(t, store, bob, time.Now())
	for name, user := range map[string]User{"an administrator": root, "an account with two-step sign-in": bob} {
		if _, err := store.UpdateUser(ctx, user.ID, UserChanges{ProviderLinkAllowed: new(true)}, nil); !errors.Is(err, ErrProviderLinkRefused) {
			t.Errorf("the permission for %s: %v", name, err)
		}
		if _, err := store.UpdateUser(ctx, user.ID, UserChanges{ProviderLinkAllowed: new(false)}, nil); err != nil {
			t.Errorf("the permission taken from %s: %v", name, err)
		}
	}
	promoted := mustCreate(t, store, NewUser{Name: "erin", Password: "correct horse"})
	careful := mustCreate(t, store, NewUser{Name: "frank", Password: "correct horse"})
	allowLink(t, store, promoted)
	allowLink(t, store, careful)
	if _, err := store.UpdateUser(ctx, promoted.ID, UserChanges{IsAdministrator: new(true)}, nil); err != nil {
		t.Fatal(err)
	}
	twoStepOn(t, store, careful, time.Now())
	for _, user := range []User{promoted, careful} {
		if _, _, err := store.ProviderUser(ctx, at("sub-"+user.Name, user.Name, ""), false); !errors.Is(err, ErrNoProviderAccount) {
			t.Errorf("%s, changed since the permission: %v", user.Name, err)
		}
		if got := mustUser(t, store, user.ID); got.ProviderIssuer != "" {
			t.Errorf("%s was linked", user.Name)
		}
	}
}

// LinkProvider links the account of the session that asked, an
// administrator's with two-step sign-in included, and nothing more: it
// needs that session still open, gives no rights, and takes no person
// from another account nor an account from another person.
func TestLinkProviderLinksTheAccountOfItsSession(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	root := mustCreate(t, store, NewUser{Name: "root", Password: "correct horse", IsAdministrator: true})
	twoStepOn(t, store, root, time.Now())
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	bob := mustCreate(t, store, NewUser{Name: "bob", Password: "correct horse"})
	rootSession, aliceSession, bobSession := session(t, store, root), session(t, store, alice), session(t, store, bob)

	inGroup := ProviderIdentity{Issuer: testIssuer, Subject: "sub-alice", Name: "root", Administrator: true}
	for name, hash := range map[string][]byte{"no session": nil, "an unknown session": HashAdminToken("made up"), "another account's session": bobSession} {
		if _, err := store.LinkProvider(ctx, alice.ID, hash, inGroup); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: %v, want ErrNotFound", name, err)
		}
	}
	linked, err := store.LinkProvider(ctx, alice.ID, aliceSession, inGroup)
	if err != nil || linked.ID != alice.ID || linked.ProviderIssuer != testIssuer || linked.IsAdministrator {
		t.Fatalf("alice: %+v %v", linked, err)
	}
	if found, created, err := store.ProviderUser(ctx, at("sub-alice", "whoever", ""), false); err != nil || created || found.ID != alice.ID {
		t.Errorf("alice, through the provider: %q %v", found.Name, err)
	}
	if linked, err = store.LinkProvider(ctx, root.ID, rootSession, at("sub-root", "", "")); err != nil || linked.ProviderIssuer != testIssuer {
		t.Errorf("an administrator with two-step sign-in: %v", err)
	}

	// One person, one account; one account, one person.
	if _, err := store.LinkProvider(ctx, bob.ID, bobSession, at("sub-alice", "", "")); !errors.Is(err, ErrProviderLinked) {
		t.Errorf("a person linked to another account: %v", err)
	}
	if _, err := store.LinkProvider(ctx, alice.ID, aliceSession, at("sub-other", "", "")); !errors.Is(err, ErrProviderLinked) {
		t.Errorf("an account linked to someone else: %v", err)
	}
	if _, err := store.LinkProvider(ctx, alice.ID, aliceSession, at("sub-alice", "", "")); err != nil {
		t.Errorf("the same link again: %v", err)
	}
	if got := mustUser(t, store, bob.ID); got.ProviderIssuer != "" {
		t.Error("bob was linked")
	}
	// A link the former provider left is replaced.
	moved := ProviderIdentity{Issuer: "https://new.example.org", Subject: "sub-alice-new"}
	if linked, err = store.LinkProvider(ctx, alice.ID, aliceSession, moved); err != nil || linked.ProviderIssuer != moved.Issuer {
		t.Errorf("at another issuer: %q %v", linked.ProviderIssuer, err)
	}

	// The session ended, or the account disabled, since the link was asked.
	if err := store.DeleteAdminSession(ctx, bobSession); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LinkProvider(ctx, bob.ID, bobSession, at("sub-bob", "", "")); !errors.Is(err, ErrNotFound) {
		t.Errorf("a session signed out: %v", err)
	}
	carl := mustCreate(t, store, NewUser{Name: "carl", Password: "correct horse"})
	carlSession := session(t, store, carl)
	if _, err := store.UpdateUser(ctx, carl.ID, UserChanges{Password: new("another horse")}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LinkProvider(ctx, carl.ID, carlSession, at("sub-carl", "", "")); !errors.Is(err, ErrNotFound) {
		t.Errorf("a session a new password signed out: %v", err)
	}
	if got := mustUser(t, store, carl.ID); got.ProviderIssuer != "" {
		t.Error("carl was linked")
	}
}
