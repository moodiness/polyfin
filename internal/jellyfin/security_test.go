package jellyfin

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

func TestWrongPasswordsBlockSignInsFromApps(t *testing.T) {
	// The test signs in wrong on purpose, more often than the server lets an
	// address fail.
	s := newTestServer(t, 100)
	alice := s.user("alice", nil)
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	admin := s.signIn("admin", "admin-tv")
	policy := func() UserPolicy {
		t.Helper()
		var user UserDto
		if status := s.get(t, "/Users/"+alice.ID.String(), admin, &user); status != http.StatusOK {
			t.Fatalf("reading alice: %d", status)
		}
		return user.Policy
	}
	signIn := func(password string) (int, string) {
		t.Helper()
		status, body := s.call(http.MethodPost, "/Users/AuthenticateByName", app("alice-tv", ""), map[string]string{"Username": "alice", "Pw": password})
		return status, string(body)
	}
	// Without a limit, as by default, the policy says so.
	if got := policy(); got.LoginAttemptsBeforeLockout != -1 || got.InvalidLoginAttemptCount != 0 {
		t.Errorf("by default: %d attempts before lockout, %d made", got.LoginAttemptsBeforeLockout, got.InvalidLoginAttemptCount)
	}

	s.setting(t, func(settings *accounts.Settings) { settings.LoginAttempts = 3 })
	wrongStatus, wrongAnswer := signIn("wrong password")
	if wrongStatus != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d %s", wrongStatus, wrongAnswer)
	}
	if got := policy(); got.LoginAttemptsBeforeLockout != 3 || got.InvalidLoginAttemptCount != 1 || got.IsDisabled {
		t.Errorf("after a wrong password: %d attempts before lockout, %d made, disabled %v",
			got.LoginAttemptsBeforeLockout, got.InvalidLoginAttemptCount, got.IsDisabled)
	}
	signIn("wrong password")
	signIn("wrong password")
	// The right password gets the same answer as a wrong one.
	if status, answer := signIn("correct horse"); status != wrongStatus || answer != wrongAnswer {
		t.Errorf("right password of a blocked account: %d %s, want %d %s", status, answer, wrongStatus, wrongAnswer)
	}
	blocked := policy()
	if blocked.InvalidLoginAttemptCount != 3 || blocked.IsDisabled {
		t.Errorf("blocked: %d made, disabled %v", blocked.InvalidLoginAttemptCount, blocked.IsDisabled)
	}
	// An administrator's app posting the policy back, its count reset,
	// leaves the block alone: the admin interface's Unblock ends it.
	blocked.InvalidLoginAttemptCount, blocked.LoginAttemptsBeforeLockout = 0, 10
	raw, _ := json.Marshal(blocked)
	if status, body := s.postRaw("/Users/"+alice.ID.String()+"/Policy", app("admin-tv", admin), string(raw)); status != http.StatusNoContent {
		t.Fatalf("posting the policy: %d %s", status, body)
	}
	if got := policy(); got.InvalidLoginAttemptCount != 3 || got.LoginAttemptsBeforeLockout != 3 {
		t.Errorf("after posting the policy: %d attempts before lockout, %d made", got.LoginAttemptsBeforeLockout, got.InvalidLoginAttemptCount)
	}
	if status, _ := signIn("correct horse"); status != http.StatusUnauthorized {
		t.Errorf("signed in after posting the policy: %d", status)
	}
	if _, err := s.store.Unblock(t.Context(), alice.ID); err != nil {
		t.Fatal(err)
	}
	if status, answer := signIn("correct horse"); status != http.StatusOK {
		t.Fatalf("after unblocking: %d %s", status, answer)
	}
	if got := policy(); got.InvalidLoginAttemptCount != 0 {
		t.Errorf("after signing in: %d made", got.InvalidLoginAttemptCount)
	}
}

// The devices the server signs out for being unused lose their socket and
// their SyncPlay group, as any device signed out.
func TestUnusedDevicesLoseTheirSocket(t *testing.T) {
	p := newSyncPlayers(t)
	if _, err := p.pool.Exec(t.Context(), "UPDATE devices SET last_activity_at = now() - interval '31 days' WHERE device_id = 'bob-tv-id'"); err != nil {
		t.Fatal(err)
	}
	p.setting(t, func(settings *accounts.Settings) { settings.InactiveDeviceDays = 30 })
	if signedOut, err := p.store.SignOutInactiveDevices(t.Context()); err != nil || signedOut != 1 {
		t.Fatalf("signed out %d devices: %v", signedOut, err)
	}
	p.bob.ended(t)
	p.alice.expect(t, "UserLeft")
	var me UserDto
	if status := p.get(t, "/Users/Me", p.bob.token, &me); status != http.StatusUnauthorized {
		t.Errorf("bob's token: %d", status)
	}
	if status := p.get(t, "/Users/Me", p.alice.token, &me); status != http.StatusOK {
		t.Errorf("alice's token: %d", status)
	}
}
