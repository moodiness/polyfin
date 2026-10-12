package accounts

import (
	"errors"
	"testing"
	"time"
)

// The wrong password that blocks an account is reported once per block,
// with the account blocked until the end of the block; another block
// after it ends is reported again.
func TestTheWrongPasswordThatBlocksIsReportedOnce(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	start := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	advance := clockAt(store, start)
	mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	setLoginAttempts(t, store, 3)
	blocks := func(password string, times int) []*BlockError {
		t.Helper()
		var found []*BlockError
		for range times {
			_, err := store.Authenticate(ctx, "alice", password)
			if !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("%q: %v", password, err)
			}
			var blocked *BlockError
			if errors.As(err, &blocked) {
				found = append(found, blocked)
			}
		}
		return found
	}

	if got := blocks("wrong password", 2); len(got) != 0 {
		t.Fatalf("2 wrong passwords reported a block")
	}
	got := blocks("wrong password", 1)
	if len(got) != 1 || got[0].User.Name != "alice" || got[0].User.InvalidLoginAttempts != 3 || got[0].User.BlockedUntil == nil ||
		!got[0].User.BlockedUntil.Equal(start.Add(LoginBlock)) {
		t.Fatalf("the third wrong password: %+v", got)
	}
	// During the block, neither a wrong password nor the right one reports
	// it again.
	advance(LoginBlock - time.Second)
	if got := append(blocks("wrong password", 3), blocks("correct horse", 1)...); len(got) != 0 {
		t.Fatalf("%d blocks reported during the block", len(got))
	}
	advance(time.Second)
	if got := blocks("wrong password", 2); len(got) != 0 {
		t.Fatalf("the first wrong passwords after the block reported one")
	}
	if got := blocks("wrong password", 1); len(got) != 1 || !got[0].User.BlockedUntil.Equal(start.Add(2*LoginBlock)) {
		t.Fatalf("the next block: %+v", got)
	}
}

// A device the account never signed in from is unfamiliar, unless it is
// the account's first; a device it used before is not, even once signed
// out. Each account has its own devices.
func TestDevicesNeverUsedAreUnfamiliar(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	bob := mustCreate(t, store, NewUser{Name: "bob", Password: "correct horse"})
	signIn := func(user User, device string) (string, bool) {
		t.Helper()
		token, signedIn, err := store.SignInDevice(ctx, user.ID, DeviceInfo{DeviceID: device, DeviceName: device, Client: "Test App"})
		if err != nil {
			t.Fatal(err)
		}
		return token, signedIn.Unfamiliar
	}
	for _, step := range []struct {
		user       User
		device     string
		unfamiliar bool
	}{
		{alice, "tv", false}, // alice's first device
		{alice, "tv", false},
		{alice, "phone", true},
		{alice, "phone", false},
		{bob, "phone", false}, // bob's first device, which alice used
		{bob, "laptop", true},
	} {
		if _, got := signIn(step.user, step.device); got != step.unfamiliar {
			t.Errorf("%s on %s: unfamiliar %v, want %v", step.user.Name, step.device, got, step.unfamiliar)
		}
	}
	token, _ := signIn(alice, "tablet")
	if err := store.SignOutDevice(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, unfamiliar := signIn(alice, "tablet"); unfamiliar {
		t.Errorf("a device signed out came back unfamiliar")
	}
}
