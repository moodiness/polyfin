package appimport

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// waitSignedOut waits until the fake app signed out n sessions, and
// returns their secrets, sorted.
func (h connectionHarness) waitSignedOut(t *testing.T, n int) []string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		signedOut := h.source.signedOutSecrets()
		if len(signedOut) >= n {
			slices.Sort(signedOut)
			return signedOut
		}
		if time.Now().After(deadline) {
			t.Fatalf("signed out: %v, want %d", signedOut, n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Deleting a user signs out, in the background, the session Polyfin opened
// for their connection and the one of the account they hold for an
// import; a key a user pasted is only forgotten, and other users'
// connections stay.
func TestDeletingAUserSignsOutWhatPolyfinOpened(t *testing.T) {
	t.Parallel()
	h := newConnectionHarness(t)
	h.store.OnDelete(h.Leaving)
	root, alice, bob := h.user(t, "root", true), h.user(t, "alice", false), h.user(t, "bob", false)
	account := Account{Profiles: []Profile{{ID: "1"}}}
	h.source.set(func(f *fakeSource) {
		f.account = account
		f.accepted["alice's"], f.accepted["bob's pasted"], f.accepted["root's"] = true, true, true
	})
	h.connected(t, alice, alice, account, SignIn{Secret: "alice's", Owned: true}, Choice{Profile: "1"})
	h.connected(t, bob, bob, account, SignIn{Secret: "bob's pasted"}, Choice{Profile: "1"})
	h.connected(t, root, root, account, SignIn{Secret: "root's", Owned: true}, Choice{Profile: "1"})
	h.Hold(alice.ID, Account{App: "stremio", SignIn: &SignIn{Secret: "alice's held", Owned: true}})

	for _, user := range []accounts.User{alice, bob} {
		if err := h.store.DeleteUser(t.Context(), user.ID); err != nil {
			t.Fatal(err)
		}
	}
	if signedOut := h.waitSignedOut(t, 2); !slices.Equal(signedOut, []string{"alice's", "alice's held"}) {
		t.Errorf("signed out: %v", signedOut)
	}
	if c := h.connectionOf(t, root); c.Problem != "" {
		t.Errorf("root's connection: %+v", c)
	}
	var left int
	if err := h.pool.QueryRow(t.Context(), "SELECT count(*) FROM app_connections").Scan(&left); err != nil || left != 1 {
		t.Errorf("%d connections left: %v", left, err)
	}
	if signedOut := h.source.signedOutSecrets(); len(signedOut) != 2 {
		t.Errorf("signed out: %v", signedOut)
	}
}

// A sign-out that never answers or fails leaves the deletion done at once;
// the failure is logged without the sign-in, and stopping waits for the
// sign-outs under way a few seconds at most.
func TestDeletingAUserNeverWaitsForTheApp(t *testing.T) {
	t.Parallel()
	h := newConnectionHarness(t)
	h.store.OnDelete(h.Leaving)
	h.closeWait = 50 * time.Millisecond
	dave, erin := h.user(t, "dave", false), h.user(t, "erin", false)
	account := Account{Profiles: []Profile{{ID: "1"}}}
	h.source.set(func(f *fakeSource) {
		f.account, f.accepted["dave's"], f.accepted["erin's"] = account, true, true
	})
	h.connected(t, dave, dave, account, SignIn{Secret: "dave's", Owned: true}, Choice{Profile: "1"})
	h.connected(t, erin, erin, account, SignIn{Secret: "erin's", Owned: true}, Choice{Profile: "1"})

	h.source.set(func(f *fakeSource) { f.hang = true })
	if err := h.store.DeleteUser(t.Context(), dave.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.User(t.Context(), dave.ID); !errors.Is(err, accounts.ErrNotFound) {
		t.Errorf("Dave after his deletion: %v", err)
	}
	h.source.set(func(f *fakeSource) { f.hang, f.signOutFails = false, true })
	if err := h.store.DeleteUser(t.Context(), erin.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(h.log.String(), "could not be signed out") {
		if time.Now().After(deadline) {
			t.Fatalf("no failure logged: %s", h.log.String())
		}
		time.Sleep(5 * time.Millisecond)
	}

	start := time.Now()
	h.Close()
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("stopping waited %v for an app that does not answer", took)
	}
	if log := h.log.String(); strings.Contains(log, "dave's") || strings.Contains(log, "erin's") {
		t.Errorf("the log holds a sign-in: %s", log)
	}
}

// Stopping Polyfin signs out the sessions of the accounts held for an
// import, before it ends; a key a user pasted is only forgotten.
func TestStoppingSignsOutTheAccountsHeld(t *testing.T) {
	t.Parallel()
	h := newConnectionHarness(t)
	bob, carol := h.user(t, "bob", false), h.user(t, "carol", false)
	h.Hold(bob.ID, Account{App: "stremio", SignIn: &SignIn{Secret: "bob's held", Owned: true}})
	h.Hold(carol.ID, Account{App: "stremio", SignIn: &SignIn{Secret: "carol's pasted"}})
	h.Close()
	if signedOut := h.source.signedOutSecrets(); !slices.Equal(signedOut, []string{"bob's held"}) {
		t.Errorf("signed out: %v", signedOut)
	}
}
