package jellyfin

import (
	"net/http"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
)

// alerts records what the handler tells of blocked accounts and new
// devices.
type alerts struct {
	mu      sync.Mutex
	blocked []accounts.DeviceInfo
	devices []accounts.Device
	users   []string
}

func (a *alerts) configure(o *Options, _ *pgxpool.Pool) {
	o.AccountBlocked = func(user accounts.User, attempt accounts.DeviceInfo) {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.blocked = append(a.blocked, attempt)
		a.users = append(a.users, user.Name)
	}
	o.NewDevice = func(user accounts.User, device accounts.Device) {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.devices = append(a.devices, device)
		a.users = append(a.users, user.Name)
	}
}

func (a *alerts) counts() (int, int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.blocked), len(a.devices)
}

// The wrong password that blocks an account is told once, with the app,
// device and address it came from; the wrong passwords during the block
// tell nothing more. A sign-in from a device the account never used is
// told, not the account's first device, nor one it used before.
func TestSignInsTellBlocksAndNewDevices(t *testing.T) {
	t.Parallel()
	told := &alerts{}
	// The test signs in wrong on purpose, more often than the server lets an
	// address fail.
	s := newProbingServer(t, 100, "ffprobe-not-installed", told.configure)
	s.user("alice", nil)
	s.setting(t, func(settings *accounts.Settings) { settings.LoginAttempts = 3 })
	signIn := func(device, password string) int {
		t.Helper()
		status, _ := s.call(http.MethodPost, "/Users/AuthenticateByName", app(device, ""), map[string]string{"Username": "alice", "Pw": password})
		return status
	}

	// The account's first device, then the same device again.
	for range 2 {
		if status := signIn("alice-tv", "correct horse"); status != http.StatusOK {
			t.Fatalf("signing in: %d", status)
		}
	}
	if blocked, devices := told.counts(); blocked+devices != 0 {
		t.Fatalf("the first device told %d blocks, %d devices", blocked, devices)
	}
	if status := signIn("alice-phone", "correct horse"); status != http.StatusOK {
		t.Fatalf("signing in: %d", status)
	}
	if status := signIn("alice-phone", "correct horse"); status != http.StatusOK {
		t.Fatalf("signing in: %d", status)
	}
	if _, devices := told.counts(); devices != 1 {
		t.Fatalf("%d new devices told, want 1", devices)
	}
	if d := told.devices[0]; d.DeviceID != "alice-phone-id" || d.DeviceName != "alice-phone" || d.Client != "Test App" || d.RemoteAddress != "127.0.0.1" ||
		told.users[0] != "alice" {
		t.Errorf("the new device: %+v of %s", d, told.users[0])
	}

	for range 5 {
		if status := signIn("intruder-pc", "wrong password"); status != http.StatusUnauthorized {
			t.Fatalf("wrong password: %d", status)
		}
	}
	blocked, _ := told.counts()
	if blocked != 1 {
		t.Fatalf("%d blocks told, want 1", blocked)
	}
	if a := told.blocked[0]; a.DeviceID != "intruder-pc-id" || a.DeviceName != "intruder-pc" || a.Client != "Test App" || a.ClientVersion != "1.0.0" ||
		a.RemoteAddress != "127.0.0.1" || told.users[1] != "alice" {
		t.Errorf("the attempt: %+v of %s", a, told.users[1])
	}
}
