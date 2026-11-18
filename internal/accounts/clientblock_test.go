package accounts

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// sentFrom is who sends a password: the client at address, with the device
// identifier device when its app names one.
func sentFrom(address, device string) DeviceInfo {
	return DeviceInfo{RemoteAddress: address, DeviceID: device}
}

// blockedBy reports whether err is the wrong password that blocked its
// client.
func blockedBy(err error) bool {
	var blocked *BlockError
	return errors.As(err, &blocked)
}

// keptClients lists the clients whose wrong passwords are kept for user,
// the one whose last wrong password is the oldest first.
func keptClients(t *testing.T, store *Store, user ID) []string {
	t.Helper()
	var clients []string
	err := store.db.QueryRow(t.Context(),
		"SELECT coalesce(array_agg(client ORDER BY last_at, client), '{}') FROM wrong_passwords WHERE user_id = $1", user).Scan(&clients)
	if err != nil {
		t.Fatal(err)
	}
	return clients
}

func TestBlockClientIsTheAddressOrItsIPv6Network(t *testing.T) {
	t.Parallel()
	for address, want := range map[string]string{
		"203.0.113.5":             "203.0.113.5",
		"::ffff:203.0.113.5":      "203.0.113.5",
		"2001:db8:1:2:3:4:5:6":    "2001:db8:1:2::/64",
		"2001:db8:1:2::":          "2001:db8:1:2::/64",
		"2001:db8:1:3::1":         "2001:db8:1:3::/64",
		"fe80::1%eth0":            "fe80::/64",
		"::1":                     "::/64",
		"a host without address":  "a host without address",
		"":                        "",
		"2001:db8:1:2:3:4:5:6:77": "2001:db8:1:2:3:4:5:6:77",
	} {
		if got := blockClient(address); got != want {
			t.Errorf("%q counts with %q, want %q", address, got, want)
		}
	}
}

// Wrong passwords from one address block that address for the account and
// nobody else: whoever sends them again as soon as each block ends keeps
// the account's owner out from nowhere else. The account shows the block,
// which ends by itself after its time.
func TestWrongPasswordsFromOneAddressLeaveTheOthersSigningIn(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	start := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	advance := clockAt(store, start)
	root := mustCreate(t, store, NewUser{Name: "root", Password: "correct horse", IsAdministrator: true})
	setLoginAttempts(t, store, MinLoginAttempts)
	const stranger, owner = "203.0.113.5", "198.51.100.7"
	signIn := func(address, password string) error {
		_, err := store.AuthenticateFrom(ctx, "root", password, sentFrom(address, ""))
		return err
	}

	for cycle := range 4 {
		blockedAt := start.Add(time.Duration(cycle) * LoginBlock)
		for i := range MinLoginAttempts {
			err := signIn(stranger, "not the password")
			if !errors.Is(err, ErrInvalidCredentials) || blockedBy(err) != (i == MinLoginAttempts-1) {
				t.Fatalf("cycle %d, wrong password %d: %v, blocking %v", cycle, i+1, err, blockedBy(err))
			}
		}
		for _, wait := range []time.Duration{0, LoginBlock - time.Second} {
			advance(wait)
			if err := signIn(stranger, "correct horse"); !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("cycle %d: the right password from the blocked address: %v", cycle, err)
			}
			if err := signIn(owner, "correct horse"); err != nil {
				t.Fatalf("cycle %d: the right password from another address during the block: %v", cycle, err)
			}
			// The other address's sign-in ends no block but its own.
			user, err := store.User(ctx, root.ID)
			if err != nil || user.InvalidLoginAttempts != MinLoginAttempts || user.BlockedUntil == nil ||
				!user.BlockedUntil.Equal(blockedAt.Add(LoginBlock)) {
				t.Fatalf("cycle %d: the account shows %d attempts, blocked until %v, %v", cycle, user.InvalidLoginAttempts, user.BlockedUntil, err)
			}
		}
		advance(time.Second)
	}
	// The last block ended by itself.
	if err := signIn(stranger, "correct horse"); err != nil {
		t.Fatalf("the right password once the block ended: %v", err)
	}
	if user, err := store.User(ctx, root.ID); err != nil || user.InvalidLoginAttempts != 0 || user.BlockedUntil != nil {
		t.Errorf("after the block: %d attempts, blocked until %v, %v", user.InvalidLoginAttempts, user.BlockedUntil, err)
	}
	if kept := keptClients(t, store, root.ID); len(kept) != 0 {
		t.Errorf("clients kept after the block: %v", kept)
	}
}

// The addresses of one IPv6 /64, which one client holds whole, count
// together; another /64 is another client.
func TestAddressesOfOneIPv6NetworkAreBlockedTogether(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	setLoginAttempts(t, store, 3)
	for i, address := range []string{"2001:db8:1:2::1", "2001:db8:1:2:aaaa::2", "2001:db8:1:2:ffff:ffff:ffff:ffff"} {
		_, err := store.AuthenticateFrom(ctx, "alice", "wrong password", sentFrom(address, ""))
		if !errors.Is(err, ErrInvalidCredentials) || blockedBy(err) != (i == 2) {
			t.Fatalf("wrong password %d, from %s: %v, blocking %v", i+1, address, err, blockedBy(err))
		}
	}
	if _, err := store.AuthenticateFrom(ctx, "alice", "correct horse", sentFrom("2001:db8:1:2::99", "")); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("the right password from another address of the blocked network: %v", err)
	}
	if _, err := store.AuthenticateFrom(ctx, "alice", "correct horse", sentFrom("2001:db8:1:3::1", "")); err != nil {
		t.Errorf("the right password from another network: %v", err)
	}
	if kept := keptClients(t, store, alice.ID); !slices.Equal(kept, []string{"2001:db8:1:2::/64"}) {
		t.Errorf("clients kept: %v", kept)
	}
}

// A sign-in naming a device identifier the account signed in from before
// is not refused by its client's block. It still needs the right password,
// a wrong one from it still counts, and the block of the address lasts its
// time for everyone else.
func TestADeviceTheAccountUsedPassesItsClientsBlock(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	advance := clockAt(store, time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC))
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	bob := mustCreate(t, store, NewUser{Name: "bob", Password: "correct horse"})
	for user, id := range map[ID]string{alice.ID: "alice-tv", bob.ID: "bob-tv"} {
		if _, _, err := store.SignInDevice(ctx, user, device(id)); err != nil {
			t.Fatal(err)
		}
	}
	setLoginAttempts(t, store, 3)
	const address = "203.0.113.5"
	signIn := func(password, device string) error {
		_, err := store.AuthenticateFrom(ctx, "alice", password, sentFrom(address, device))
		return err
	}
	for i := range 3 {
		if err := signIn("wrong password", "never-seen"); blockedBy(err) != (i == 2) {
			t.Fatalf("wrong password %d: %v", i+1, err)
		}
	}
	for _, c := range []struct {
		device string
		passes bool
	}{
		{"", false},
		{"never-seen", false},
		// Another account's device is not this account's.
		{"bob-tv", false},
		{"alice-tv", true},
	} {
		err := signIn("correct horse", c.device)
		if c.passes && err != nil || !c.passes && !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("the right password during the block, from the device %q: %v", c.device, err)
		}
	}
	// A wrong password from the device is refused and counted, and blocks
	// nothing again.
	if err := signIn("wrong password", "alice-tv"); !errors.Is(err, ErrInvalidCredentials) || blockedBy(err) {
		t.Errorf("a wrong password from the device during the block: %v", err)
	}
	if user, err := store.User(ctx, alice.ID); err != nil || user.InvalidLoginAttempts != 4 || !user.Blocked(store.now()) {
		t.Errorf("after it: %d attempts, blocked until %v, %v", user.InvalidLoginAttempts, user.BlockedUntil, err)
	}
	// The device's sign-in did not end the block of its address.
	if err := signIn("correct horse", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("the right password without a device, after the device signed in: %v", err)
	}
	advance(LoginBlock)
	if err := signIn("correct horse", "never-seen"); err != nil {
		t.Errorf("another device once the block ended: %v", err)
	}
}

// Each client's block is told once, by the wrong password that starts it,
// and Unblock ends every block of the account.
func TestEachClientsBlockIsToldOnceAndUnblockEndsThemAll(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	start := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	clockAt(store, start)
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	setLoginAttempts(t, store, 3)
	const first, second, third = "203.0.113.5", "203.0.113.6", "198.51.100.7"
	told := func(address, password string, times int) []*BlockError {
		t.Helper()
		var blocks []*BlockError
		for range times {
			_, err := store.AuthenticateFrom(ctx, "alice", password, sentFrom(address, ""))
			if !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("%q from %s: %v", password, address, err)
			}
			var blocked *BlockError
			if errors.As(err, &blocked) {
				blocks = append(blocks, blocked)
			}
		}
		return blocks
	}
	// The wrong passwords of one client count toward no other's block.
	if got := append(told(first, "wrong password", 2), told(second, "wrong password", 2)...); len(got) != 0 {
		t.Fatalf("2 wrong passwords from each of two addresses told %d blocks", len(got))
	}
	for _, address := range []string{first, second} {
		got := told(address, "wrong password", 1)
		if len(got) != 1 || got[0].User.ID != alice.ID || got[0].User.InvalidLoginAttempts != 3 || got[0].User.BlockedUntil == nil ||
			!got[0].User.BlockedUntil.Equal(start.Add(LoginBlock)) {
			t.Fatalf("the third wrong password from %s: %+v", address, got)
		}
	}
	for _, address := range []string{first, second} {
		if got := append(told(address, "wrong password", 3), told(address, "correct horse", 1)...); len(got) != 0 {
			t.Errorf("%d blocks told during the block of %s", len(got), address)
		}
	}
	if _, err := store.AuthenticateFrom(ctx, "alice", "correct horse", sentFrom(third, "")); err != nil {
		t.Errorf("a third address during both blocks: %v", err)
	}

	unblocked, err := store.Unblock(ctx, alice.ID)
	if err != nil || unblocked.InvalidLoginAttempts != 0 || unblocked.BlockedUntil != nil {
		t.Fatalf("unblocking: %d attempts, blocked until %v, %v", unblocked.InvalidLoginAttempts, unblocked.BlockedUntil, err)
	}
	for _, address := range []string{first, second} {
		if _, err := store.AuthenticateFrom(ctx, "alice", "correct horse", sentFrom(address, "")); err != nil {
			t.Errorf("%s after unblocking: %v", address, err)
		}
	}
	if kept := keptClients(t, store, alice.ID); len(kept) != 0 {
		t.Errorf("clients kept after unblocking: %v", kept)
	}
}

// An account keeps the wrong passwords of blockClients clients at most:
// the one whose last is the oldest gives way. A block that ended is
// forgotten at the account's next wrong password.
func TestTheClientsKeptForAnAccountAreBounded(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	advance := clockAt(store, time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC))
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	setLoginAttempts(t, store, MaxLoginAttempts)
	address := func(i int) string { return fmt.Sprintf("198.51.100.%d", i+1) }
	wrong := func(address string) {
		t.Helper()
		if _, err := store.AuthenticateFrom(ctx, "alice", "wrong password", sentFrom(address, "")); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("wrong password from %s: %v", address, err)
		}
	}
	const extra = 5
	for i := range blockClients + extra {
		wrong(address(i))
		advance(time.Second)
		if kept := keptClients(t, store, alice.ID); len(kept) > blockClients {
			t.Fatalf("%d clients kept after %d addresses, over the bound of %d", len(kept), i+1, blockClients)
		}
	}
	kept := keptClients(t, store, alice.ID)
	if len(kept) != blockClients || kept[0] != address(extra) || kept[len(kept)-1] != address(blockClients+extra-1) {
		t.Fatalf("%d clients kept, from %s to %s", len(kept), kept[0], kept[len(kept)-1])
	}
	// The first address gave way: its count starts again.
	wrong(address(0))
	var attempts int
	if err := store.db.QueryRow(ctx, "SELECT attempts FROM wrong_passwords WHERE user_id = $1 AND client = $2", alice.ID, address(0)).Scan(&attempts); err != nil || attempts != 1 {
		t.Errorf("the address that gave way counts %d, %v", attempts, err)
	}

	if _, err := store.Unblock(ctx, alice.ID); err != nil {
		t.Fatal(err)
	}
	setLoginAttempts(t, store, 3)
	for range 3 {
		wrong(address(0))
	}
	advance(LoginBlock)
	wrong(address(1))
	if kept := keptClients(t, store, alice.ID); !slices.Equal(kept, []string{address(1)}) {
		t.Errorf("clients kept once a block ended: %v", kept)
	}
	if user, err := store.User(ctx, alice.ID); err != nil || user.InvalidLoginAttempts != 1 || user.BlockedUntil != nil {
		t.Errorf("the account then shows %d attempts, blocked until %v, %v", user.InvalidLoginAttempts, user.BlockedUntil, err)
	}
}

// The current password given to change a password counts as a password at
// sign-in does: with those of sign-ins from the same client, toward the
// same block, told the same way; while its client is blocked the right one
// is refused as a wrong one, from a signed-in device too.
func TestWrongCurrentPasswordsCountAsWrongPasswordsAtSignIn(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	start := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	clockAt(store, start)
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	_, tv, err := store.SignInDevice(ctx, alice.ID, device("alice-tv"))
	if err != nil {
		t.Fatal(err)
	}
	const address, elsewhere = "203.0.113.5", "198.51.100.7"
	change := func(current, next, address string) error {
		return store.ChangePasswordAt(ctx, alice.ID, current, next, nil, address)
	}
	signsIn := func(address, password string) bool {
		t.Helper()
		_, err := store.AuthenticateFrom(ctx, "alice", password, sentFrom(address, ""))
		if err != nil && !errors.Is(err, ErrInvalidCredentials) {
			t.Fatal(err)
		}
		return err == nil
	}

	// Without a limit, as by default, nothing counts.
	for range MaxLoginAttempts + 1 {
		if err := change("wrong password", "battery staple", address); !errors.Is(err, ErrWrongPassword) || blockedBy(err) {
			t.Fatalf("a wrong current password without a limit: %v", err)
		}
	}
	if kept := keptClients(t, store, alice.ID); len(kept) != 0 {
		t.Fatalf("clients kept without a limit: %v", kept)
	}

	setLoginAttempts(t, store, 3)
	if signsIn(address, "wrong password") || signsIn(address, "wrong password") {
		t.Fatal("a wrong password signed in")
	}
	err = change("wrong password", "battery staple", address)
	var blocked *BlockError
	if !errors.Is(err, ErrWrongPassword) || errors.Is(err, ErrInvalidCredentials) || !errors.As(err, &blocked) ||
		blocked.User.ID != alice.ID || blocked.User.InvalidLoginAttempts != 3 || blocked.User.BlockedUntil == nil ||
		!blocked.User.BlockedUntil.Equal(start.Add(LoginBlock)) {
		t.Fatalf("the wrong current password that makes three in a row: %v, %+v", err, blocked)
	}
	if err := change("correct horse", "battery staple", address); !errors.Is(err, ErrWrongPassword) || blockedBy(err) {
		t.Errorf("the right current password from the blocked client: %v", err)
	}
	if err := store.ChangePasswordFromDeviceAt(ctx, alice.ID, "correct horse", "battery staple", tv.ID, address); !errors.Is(err, ErrWrongPassword) || blockedBy(err) {
		t.Errorf("the right current password from a signed-in device of the blocked client: %v", err)
	}
	if signsIn(address, "correct horse") || !signsIn(elsewhere, "correct horse") {
		t.Fatal("the password changed, or the block is not of the client alone")
	}

	// From another client the right current password changes it, which
	// ends every block of the account.
	if err := change("correct horse", "battery staple", elsewhere); err != nil {
		t.Fatalf("the right current password from another client: %v", err)
	}
	if user, err := store.User(ctx, alice.ID); err != nil || user.InvalidLoginAttempts != 0 || user.BlockedUntil != nil || !signsIn(address, "battery staple") {
		t.Errorf("after the change: %d attempts, blocked until %v, %v", user.InvalidLoginAttempts, user.BlockedUntil, err)
	}

	// A right current password starts the count again, even when the new
	// password is refused.
	for range 2 {
		if err := change("wrong password", "another staple", address); !errors.Is(err, ErrWrongPassword) || blockedBy(err) {
			t.Fatalf("a wrong current password: %v", err)
		}
	}
	if err := change("battery staple", "short", address); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("the right current password with a new one too short: %v", err)
	}
	for range 2 {
		if err := change("wrong password", "another staple", address); !errors.Is(err, ErrWrongPassword) || blockedBy(err) {
			t.Fatalf("a wrong current password after a right one: %v", err)
		}
	}
	if !signsIn(address, "battery staple") {
		t.Error("blocked by wrong current passwords that were not in a row")
	}
}

// A sign-in verifies one hash whatever the name and the password's length,
// so that its time tells nothing of the name: the account's for a password
// that fits, the hash that stands for an unknown account for a name that
// matches none and for a password over the bound, which is never right.
func TestEverySignInVerifiesOneHash(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	var stored string
	if err := store.db.QueryRow(ctx, "SELECT password_hash FROM users WHERE id = $1", alice.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	var verified []string
	store.verifying = func(hash string) { verified = append(verified, hash) }
	fits, over := strings.Repeat("x", maxPasswordLength), strings.Repeat("x", maxPasswordLength+1)
	for _, c := range []struct {
		name, password, against string
		signsIn                 bool
	}{
		{"alice", "correct horse", stored, true},
		{"alice", "wrong password", stored, false},
		{"alice", fits, stored, false},
		{"alice", over, unknownUserHash, false},
		{"nobody", "wrong password", unknownUserHash, false},
		{"nobody", fits, unknownUserHash, false},
		{"nobody", over, unknownUserHash, false},
	} {
		verified = nil
		_, err := store.Authenticate(ctx, c.name, c.password)
		if c.signsIn && err != nil || !c.signsIn && !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("%s with a password of %d bytes: %v", c.name, len(c.password), err)
		}
		if !slices.Equal(verified, []string{c.against}) {
			t.Errorf("%s with a password of %d bytes verified %d hashes, want one: the account's %v",
				c.name, len(c.password), len(verified), c.against == stored)
		}
	}
}
