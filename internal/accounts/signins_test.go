package accounts

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestResolvedTokensAreKeptBriefly(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	now := time.Now()
	store.now = func() time.Time { return now }
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	token, _, err := store.SignInDevice(ctx, alice.ID, device("tv"))
	if err != nil {
		t.Fatal(err)
	}
	if _, user, err := store.DeviceByToken(ctx, token, "192.0.2.1"); err != nil || user.Name != "alice" {
		t.Fatalf("first lookup: %v %v", user.Name, err)
	}
	// A change made behind the server's back is not seen while the token
	// is remembered: the lookup did not ask the database.
	if _, err := store.db.Exec(ctx, "UPDATE users SET name = 'renamed' WHERE id = $1", alice.ID); err != nil {
		t.Fatal(err)
	}
	now = now.Add(signInLife - time.Second)
	if _, user, err := store.DeviceByToken(ctx, token, "192.0.2.1"); err != nil || user.Name != "alice" {
		t.Errorf("lookup within the life: %v %v", user.Name, err)
	}
	now = now.Add(time.Second)
	if _, user, err := store.DeviceByToken(ctx, token, "192.0.2.1"); err != nil || user.Name != "renamed" {
		t.Errorf("lookup past the life: %v %v", user.Name, err)
	}
}

func TestActivityIsWrittenOnceAMinuteFromRememberedTokens(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	now := time.Now()
	store.now = func() time.Time { return now }
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	token, signedIn, err := store.SignInDevice(ctx, alice.ID, device("tv"))
	if err != nil {
		t.Fatal(err)
	}
	written := func() time.Time {
		t.Helper()
		var device, user time.Time
		if err := store.db.QueryRow(ctx, `SELECT d.last_activity_at, u.last_activity_at FROM devices d JOIN users u ON u.id = d.user_id
			WHERE d.id = $1`, signedIn.ID).Scan(&device, &user); err != nil {
			t.Fatal(err)
		}
		if !device.Equal(user) {
			t.Errorf("device active at %v, user at %v", device, user)
		}
		return device
	}
	start := written()
	if _, _, err := store.DeviceByToken(ctx, token, "192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(20 * time.Second)
	if _, _, err := store.DeviceByToken(ctx, token, "192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	if got := written(); !got.Equal(start) {
		t.Errorf("activity written within a minute: %v, then %v", start, got)
	}
	now = start.Add(activityResolution + time.Second)
	device, user, err := store.DeviceByToken(ctx, token, "192.0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	if got := written(); !got.Equal(now.Truncate(time.Microsecond)) {
		t.Errorf("activity after a minute: %v, want %v", got, now)
	}
	if !device.LastActivityAt.Equal(now) || user.LastActivityAt == nil || !user.LastActivityAt.Equal(now) {
		t.Errorf("activity answered: %v %v", device.LastActivityAt, user.LastActivityAt)
	}
	// The remembered device knows it was just written: no second write.
	now = now.Add(time.Second)
	if _, _, err := store.DeviceByToken(ctx, token, "192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	if got := written(); !got.Equal(now.Add(-time.Second).Truncate(time.Microsecond)) {
		t.Errorf("activity written again within a minute: %v", got)
	}
	// A new address is written at once.
	if device, _, err := store.DeviceByToken(ctx, token, "198.51.100.7"); err != nil || device.RemoteAddress != "198.51.100.7" {
		t.Errorf("new address: %v %v", device.RemoteAddress, err)
	}
	var address string
	if err := store.db.QueryRow(ctx, "SELECT remote_address FROM devices WHERE id = $1", signedIn.ID).Scan(&address); err != nil || address != "198.51.100.7" {
		t.Errorf("address stored: %q %v", address, err)
	}
}

func TestEveryChangeIsSeenFromTheNextRequest(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	type signed struct {
		user   User
		token  string
		device Device
	}
	count := 0
	newSigned := func() signed {
		t.Helper()
		count++
		user := mustCreate(t, store, NewUser{Name: fmt.Sprintf("user%d", count), Password: "correct horse"})
		token, device, err := store.SignInDevice(ctx, user.ID, DeviceInfo{DeviceID: fmt.Sprintf("tv%d", count), DeviceName: "TV",
			Client: "Test", ClientVersion: "1.0", RemoteAddress: "192.0.2.1"})
		if err != nil {
			t.Fatal(err)
		}
		// The token is remembered before the change.
		if _, _, err := store.DeviceByToken(ctx, token, "192.0.2.1"); err != nil {
			t.Fatal(err)
		}
		return signed{user: user, token: token, device: device}
	}
	refused := func(what string, token string) {
		t.Helper()
		if _, _, err := store.DeviceByToken(ctx, token, "192.0.2.1"); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: token still works (%v)", what, err)
		}
	}
	lookup := func(token string) (Device, User) {
		t.Helper()
		device, user, err := store.DeviceByToken(ctx, token, "192.0.2.1")
		if err != nil {
			t.Fatal(err)
		}
		return device, user
	}

	s := newSigned()
	if err := store.SignOutDevice(ctx, s.token); err != nil {
		t.Fatal(err)
	}
	refused("signed out", s.token)

	s = newSigned()
	if err := store.RevokeDevice(ctx, s.user.ID, s.device.ID); err != nil {
		t.Fatal(err)
	}
	refused("revoked", s.token)

	s = newSigned()
	if _, err := store.SignOutDeviceID(ctx, s.device.DeviceID); err != nil {
		t.Fatal(err)
	}
	refused("signed out by device identifier", s.token)

	s = newSigned()
	if _, err := store.UpdateUser(ctx, s.user.ID, UserChanges{IsDisabled: new(true)}, nil); err != nil {
		t.Fatal(err)
	}
	refused("disabled", s.token)

	s = newSigned()
	if err := store.DeleteUser(ctx, s.user.ID); err != nil {
		t.Fatal(err)
	}
	refused("deleted", s.token)

	s = newSigned()
	if _, _, err := store.SignInDevice(ctx, s.user.ID, DeviceInfo{DeviceID: s.device.DeviceID, DeviceName: "TV", Client: "Test",
		ClientVersion: "1.0"}); err != nil {
		t.Fatal(err)
	}
	refused("signed in again on the same device", s.token)

	s = newSigned()
	other, otherDevice, err := store.SignInDevice(ctx, s.user.ID, device("phone"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ChangePasswordFromDevice(ctx, s.user.ID, "correct horse", "battery staple", otherDevice.ID); err != nil {
		t.Fatal(err)
	}
	refused("password changed on another device", s.token)
	lookup(other)

	s = newSigned()
	settings := store.Settings()
	settings.InactiveDeviceDays = 1
	store.settings.Store(&settings)
	if _, err := store.db.Exec(ctx, "UPDATE devices SET last_activity_at = now() - interval '2 days' WHERE id = $1", s.device.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SignOutInactiveDevices(ctx); err != nil {
		t.Fatal(err)
	}
	refused("signed out for inactivity", s.token)

	s = newSigned()
	if _, user := lookup(s.token); !user.UseSharedAddons || !user.VideoTranscoding {
		t.Errorf("a new user: %+v", user)
	}
	if _, err := store.UpdateUser(ctx, s.user.ID, UserChanges{VideoTranscoding: new(false), LiveTv: new(false), UseSharedAddons: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	if _, user := lookup(s.token); user.VideoTranscoding || user.LiveTv || user.UseSharedAddons {
		t.Errorf("permissions changed: %+v", user)
	}
	if err := store.SetCapabilities(ctx, s.device.ID, Capabilities{PlayableMediaTypes: []string{"Video"}, SupportsMediaControl: true}); err != nil {
		t.Fatal(err)
	}
	if device, _ := lookup(s.token); !device.Capabilities.SupportsMediaControl {
		t.Errorf("capabilities reported: %+v", device.Capabilities)
	}
	if _, err := store.db.Exec(ctx, "UPDATE users SET invalid_login_attempts = 2 WHERE id = $1", s.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Unblock(ctx, s.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, user := lookup(s.token); user.InvalidLoginAttempts != 0 {
		t.Errorf("unblocked: %d wrong passwords", user.InvalidLoginAttempts)
	}
}

func TestRevokedAPIKeysStopAtOnce(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	now := time.Now()
	store.now = func() time.Time { return now }
	for _, revoke := range []func(APIKey, string) error{
		func(key APIKey, _ string) error { return store.RevokeAPIKey(ctx, key.ID) },
		func(_ APIKey, token string) error { return store.RevokeAPIKeyByToken(ctx, token) },
	} {
		key, token, err := store.CreateAPIKey(ctx, "Tool")
		if err != nil {
			t.Fatal(err)
		}
		if got, err := store.APIKeyByToken(ctx, token); err != nil || got.ID != key.ID || got.LastUsedAt == nil {
			t.Fatalf("key: %v %v", got, err)
		}
		if err := revoke(key, token); err != nil {
			t.Fatal(err)
		}
		if _, err := store.APIKeyByToken(ctx, token); !errors.Is(err, ErrNotFound) {
			t.Errorf("revoked key: %v", err)
		}
	}
	// A key in use is remembered, and its use written once a minute.
	key, token, err := store.CreateAPIKey(ctx, "Tool")
	if err != nil {
		t.Fatal(err)
	}
	used := func() time.Time {
		t.Helper()
		var at time.Time
		if err := store.db.QueryRow(ctx, "SELECT last_used_at FROM api_keys WHERE id = $1", key.ID).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at
	}
	first := now
	if _, err := store.APIKeyByToken(ctx, token); err != nil {
		t.Fatal(err)
	}
	now = now.Add(20 * time.Second)
	if got, err := store.APIKeyByToken(ctx, token); err != nil || !got.LastUsedAt.Equal(first) || !used().Equal(first.Truncate(time.Microsecond)) {
		t.Errorf("use written within a minute: %v %v", got.LastUsedAt, err)
	}
	now = first.Add(activityResolution)
	if got, err := store.APIKeyByToken(ctx, token); err != nil || !got.LastUsedAt.Equal(now) || !used().Equal(now.Truncate(time.Microsecond)) {
		t.Errorf("use after a minute: %v %v", got.LastUsedAt, err)
	}
}

func TestALookupStartedBeforeAChangeKeepsNothing(t *testing.T) {
	var c signIns
	now := time.Now()
	generation := c.current()
	c.forget()
	c.keepDevice("token", resolvedDevice{at: now}, generation)
	c.keepKey("key", APIKey{}, now, generation)
	if _, ok := c.device("token", now); ok {
		t.Error("a device read before a change was kept")
	}
	if _, ok := c.key("key", now); ok {
		t.Error("a key read before a change was kept")
	}
	c.keepDevice("token", resolvedDevice{at: now}, c.current())
	if _, ok := c.device("token", now); !ok {
		t.Error("a device read after the change was not kept")
	}
}

func TestRememberedTokensAreBounded(t *testing.T) {
	var c signIns
	now := time.Now()
	for i := range maxSignIns {
		c.keepDevice(fmt.Sprint(i), resolvedDevice{at: now.Add(-signInLife)}, 0)
	}
	c.keepDevice("fresh", resolvedDevice{at: now}, 0)
	if len(c.devices) != 1 {
		t.Errorf("expired tokens kept past the bound: %d", len(c.devices))
	}
	for i := range maxSignIns - 1 {
		c.keepDevice(fmt.Sprint(i), resolvedDevice{at: now}, 0)
	}
	c.keepDevice("one more", resolvedDevice{at: now}, 0)
	if len(c.devices) > maxSignIns {
		t.Errorf("%d tokens kept, bound %d", len(c.devices), maxSignIns)
	}
	if _, ok := c.device("one more", now); !ok {
		t.Error("the newest token was not kept")
	}
}
