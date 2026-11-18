package jellyfin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Wrong passwords from one address block that address for the account,
// the addresses of one IPv6 /64 together, and leave every other address
// signing in. An app the account signed in from before signs in from the
// blocked address too; one it never used does not.
func TestWrongPasswordsFromAppsBlockTheirAddressAlone(t *testing.T) {
	t.Parallel()
	// The test signs in wrong on purpose, more often than the server lets an
	// address fail.
	s := newTestServer(t, 100)
	alice := s.user("alice", nil)
	s.user("bob", nil)
	s.setting(t, func(settings *accounts.Settings) { settings.LoginAttempts = 3 })
	knowProxies(t, s, "127.0.0.1/32")
	call := behindProxy(t, s)
	signIn := func(address, device, name, password string) int {
		t.Helper()
		status, _ := call(address, http.MethodPost, "/Users/AuthenticateByName", app(device, ""), map[string]string{"Username": name, "Pw": password})
		return status
	}
	const stranger, home = "203.0.113.3", "198.51.100.4"
	if status := signIn(home, "alice-tv", "alice", "correct horse"); status != http.StatusOK {
		t.Fatalf("alice's first sign-in: %d", status)
	}

	for range 3 {
		if status := signIn(stranger, "pc", "alice", "wrong password"); status != http.StatusUnauthorized {
			t.Fatalf("wrong password: %d", status)
		}
	}
	if status := signIn(stranger, "pc", "alice", "correct horse"); status != http.StatusUnauthorized {
		t.Errorf("the right password from the blocked address: %d", status)
	}
	if status := signIn(home, "alice-phone", "alice", "correct horse"); status != http.StatusOK {
		t.Errorf("the right password from another address, on a new device: %d", status)
	}
	if status := signIn(stranger, "alice-tv", "alice", "correct horse"); status != http.StatusOK {
		t.Errorf("the right password from the blocked address, on a device the account used: %d", status)
	}
	if status := signIn(stranger, "alice-tv", "alice", "wrong password"); status != http.StatusUnauthorized {
		t.Errorf("a wrong password from the blocked address, on a device the account used: %d", status)
	}
	// The account still shows the block of that address.
	if user, err := s.store.User(t.Context(), alice.ID); err != nil || user.InvalidLoginAttempts != 4 || user.BlockedUntil == nil {
		t.Errorf("alice shows %d attempts, blocked until %v, %v", user.InvalidLoginAttempts, user.BlockedUntil, err)
	}

	for _, address := range []string{"2001:db8:1:2::1", "2001:db8:1:2:aaaa::2", "2001:db8:1:2:ffff:ffff:ffff:ffff"} {
		if status := signIn(address, "pc", "bob", "wrong password"); status != http.StatusUnauthorized {
			t.Fatalf("wrong password from %s: %d", address, status)
		}
	}
	if status := signIn("2001:db8:1:2::99", "bob-tv", "bob", "correct horse"); status != http.StatusUnauthorized {
		t.Errorf("the right password from another address of the blocked network: %d", status)
	}
	if status := signIn("2001:db8:1:3::1", "bob-tv", "bob", "correct horse"); status != http.StatusOK {
		t.Errorf("the right password from another network: %d", status)
	}
}

// The current password given to change one's password from an app counts
// as a wrong password at sign-in does: the one that makes the limit blocks
// the client for the account and is told, and the right one is then
// refused as a wrong one until the block ends.
func TestWrongCurrentPasswordsFromAppsCountTowardTheBlock(t *testing.T) {
	t.Parallel()
	told := &alerts{}
	// The test sends wrong passwords on purpose, more often than the server
	// lets an address fail.
	s := newProbingServer(t, 100, "ffprobe-not-installed", told.configure)
	s.setting(t, func(settings *accounts.Settings) { settings.LoginAttempts = 3 })
	alice := s.user("alice", nil)
	token := app("tv", s.signIn("alice", "tv"))
	change := func(current string) (int, string) {
		t.Helper()
		status, body := s.call(http.MethodPost, "/Users/Password", token, map[string]string{"CurrentPw": current, "NewPw": "another horse"})
		return status, string(body)
	}

	for i := range 3 {
		if status, body := change("guess"); status != http.StatusForbidden || !strings.Contains(body, wrongCurrentPassword) {
			t.Fatalf("wrong current password %d: %d %s", i+1, status, body)
		}
	}
	user, err := s.store.User(t.Context(), alice.ID)
	if err != nil || user.InvalidLoginAttempts != 3 || !user.Blocked(time.Now()) {
		t.Fatalf("alice after three wrong current passwords: %d attempts, blocked until %v, %v", user.InvalidLoginAttempts, user.BlockedUntil, err)
	}
	if blocked, _ := told.counts(); blocked != 1 {
		t.Fatalf("%d blocks told, want 1", blocked)
	}
	if a := told.blocked[0]; a.DeviceID != "tv-id" || a.DeviceName != "tv" || a.Client != "Test App" || a.RemoteAddress != "127.0.0.1" || told.users[0] != "alice" {
		t.Errorf("the attempt told: %+v of %s", a, told.users[0])
	}
	// The right one gets the answer of a wrong one, and changes nothing.
	if status, body := change("correct horse"); status != http.StatusForbidden || !strings.Contains(body, wrongCurrentPassword) {
		t.Fatalf("the right current password from the blocked client: %d %s", status, body)
	}
	if blocked, _ := told.counts(); blocked != 1 {
		t.Errorf("%d blocks told after the right current password, want 1", blocked)
	}
	// The device, which the account used, still signs in with the
	// password it had.
	status, body := s.call(http.MethodPost, "/Users/AuthenticateByName", app("tv", ""), map[string]string{"Username": "alice", "Pw": "correct horse"})
	var result AuthenticationResult
	if status != http.StatusOK || json.Unmarshal(body, &result) != nil {
		t.Fatalf("signing in on the device with the password kept: %d %s", status, body)
	}
	token = app("tv", result.AccessToken)

	if _, err := s.store.Unblock(t.Context(), alice.ID); err != nil {
		t.Fatal(err)
	}
	if status, body := change("correct horse"); status != http.StatusNoContent {
		t.Fatalf("the right current password once unblocked: %d %s", status, body)
	}
}
