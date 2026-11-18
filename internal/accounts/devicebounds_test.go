package accounts

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// A sign-in from a device whose identifier, name, app or version passes
// its bound is refused before anything is stored; at the bounds, it signs
// in.
func TestASignInStoresNothingOfADevicePastItsBounds(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	for name, change := range map[string]func(*DeviceInfo){
		"identifier": func(d *DeviceInfo) { d.DeviceID = strings.Repeat("i", MaxDeviceIDBytes+1) },
		"name":       func(d *DeviceInfo) { d.DeviceName = strings.Repeat("n", MaxDeviceTextBytes+1) },
		"app":        func(d *DeviceInfo) { d.Client = strings.Repeat("c", MaxDeviceTextBytes+1) },
		"version":    func(d *DeviceInfo) { d.ClientVersion = strings.Repeat("v", MaxDeviceTextBytes+1) },
	} {
		info := device("tv")
		change(&info)
		if _, _, err := store.SignInDevice(ctx, alice.ID, info); !errors.Is(err, ErrInvalidDevice) {
			t.Errorf("a device whose %s passes its bound: %v", name, err)
		}
	}
	var devices, seen int
	if err := store.db.QueryRow(ctx, "SELECT (SELECT count(*) FROM devices), (SELECT count(*) FROM seen_devices)").Scan(&devices, &seen); err != nil {
		t.Fatal(err)
	}
	if devices != 0 || seen != 0 {
		t.Errorf("refused sign-ins left %d devices and %d identifiers seen", devices, seen)
	}

	atBounds := DeviceInfo{DeviceID: strings.Repeat("i", MaxDeviceIDBytes), DeviceName: strings.Repeat("n", MaxDeviceTextBytes),
		Client: strings.Repeat("c", MaxDeviceTextBytes), ClientVersion: strings.Repeat("v", MaxDeviceTextBytes), RemoteAddress: "192.0.2.1"}
	token, _, err := store.SignInDevice(ctx, alice.ID, atBounds)
	if err != nil {
		t.Fatalf("a device at its bounds: %v", err)
	}
	if signedIn, _, err := store.DeviceByToken(ctx, token, "192.0.2.1"); err != nil || signedIn.DeviceInfo != atBounds {
		t.Errorf("the device at its bounds: %v, kept as sent: %t", err, signedIn.DeviceInfo == atBounds)
	}
}

// Capabilities with a list past its bound, in entries or in the bytes of
// one, are refused, and the device keeps those it had; at the bounds, they
// are kept.
func TestCapabilitiesPastTheirBoundsAreNotStored(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	token, tv, err := store.SignInDevice(ctx, alice.ID, device("tv"))
	if err != nil {
		t.Fatal(err)
	}
	kept := func() Capabilities {
		t.Helper()
		signedIn, _, err := store.DeviceByToken(ctx, token, "192.0.2.1")
		if err != nil {
			t.Fatal(err)
		}
		return signedIn.Capabilities
	}
	same := func(a, b Capabilities) bool {
		return slices.Equal(a.PlayableMediaTypes, b.PlayableMediaTypes) && slices.Equal(a.SupportedCommands, b.SupportedCommands) &&
			a.SupportsMediaControl == b.SupportsMediaControl && a.SupportsPersistentIdentifier == b.SupportsPersistentIdentifier
	}
	usual := Capabilities{PlayableMediaTypes: []string{"Audio", "Video"}, SupportedCommands: []string{"VolumeUp", "Mute"}, SupportsMediaControl: true}
	if err := store.SetCapabilities(ctx, tv.ID, usual); err != nil || !same(kept(), usual) {
		t.Fatalf("usual capabilities: %v, kept %+v", err, kept())
	}

	tooMany := slices.Repeat([]string{"Mute"}, MaxCapabilityEntries+1)
	tooLong := []string{strings.Repeat("m", MaxCapabilityBytes+1)}
	for name, capabilities := range map[string]Capabilities{
		"too many media types": {PlayableMediaTypes: tooMany},
		"too many commands":    {SupportedCommands: tooMany},
		"a long media type":    {PlayableMediaTypes: tooLong},
		"a long command":       {SupportedCommands: tooLong},
	} {
		if err := store.SetCapabilities(ctx, tv.ID, capabilities); !errors.Is(err, ErrInvalidCapabilities) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if !same(kept(), usual) {
		t.Errorf("after refused capabilities, the device keeps %+v", kept())
	}

	full := slices.Repeat([]string{strings.Repeat("m", MaxCapabilityBytes)}, MaxCapabilityEntries)
	atBounds := Capabilities{PlayableMediaTypes: full, SupportedCommands: full}
	if err := store.SetCapabilities(ctx, tv.ID, atBounds); err != nil || !same(kept(), atBounds) {
		t.Errorf("capabilities at their bounds: %v, kept: %t", err, same(kept(), atBounds))
	}
}

// A user keeps MaxUserDevices devices signed in. One more signs out the
// device whose last activity is the oldest, whenever it signed in: the
// first device signed in, used since, stays. Another user's devices are
// not counted, and a device signing in again signs no one out.
func TestASignInPastAUsersBoundSignsOutTheDeviceUsedLongestAgo(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	var told []ID
	store.OnSignOut(func(devices []ID) { told = append(told, devices...) })
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	bob := mustCreate(t, store, NewUser{Name: "bob", Password: "correct horse"})
	bobs, _, err := store.SignInDevice(ctx, bob.ID, device("device-000"))
	if err != nil {
		t.Fatal(err)
	}
	tokens := make([]string, MaxUserDevices)
	ids := make([]ID, MaxUserDevices)
	for i := range MaxUserDevices {
		token, signedIn, err := store.SignInDevice(ctx, alice.ID, device(fmt.Sprintf("device-%03d", i)))
		if err != nil {
			t.Fatal(err)
		}
		tokens[i], ids[i] = token, signedIn.ID
		// Each was last used a minute after the one before, the first
		// two hours ago.
		if _, err := store.db.Exec(ctx, "UPDATE devices SET last_activity_at = now() - interval '2 hours' + $2::int * interval '1 minute' WHERE id = $1",
			signedIn.ID, i); err != nil {
			t.Fatal(err)
		}
	}
	count := func(user ID) int {
		t.Helper()
		var n int
		if err := store.db.QueryRow(ctx, "SELECT count(*) FROM devices WHERE user_id = $1", user).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(alice.ID); n != MaxUserDevices || len(told) != 0 {
		t.Fatalf("at the bound: %d devices, %d signed out", n, len(told))
	}
	// The first device asks for something, as one that is playing does:
	// its activity is written, the latest of all.
	if !signedIn(ctx, store, tokens[0]) {
		t.Fatal("the first device is not signed in")
	}

	if _, _, err := store.SignInDevice(ctx, alice.ID, device("one-more")); err != nil {
		t.Fatal(err)
	}
	if n := count(alice.ID); n != MaxUserDevices {
		t.Errorf("past the bound, the user keeps %d devices", n)
	}
	if signedIn(ctx, store, tokens[1]) {
		t.Error("the device used longest ago is still signed in")
	}
	if !signedIn(ctx, store, tokens[0]) || !signedIn(ctx, store, tokens[2]) {
		t.Error("a device used since was signed out")
	}
	if !slices.Equal(told, []ID{ids[1]}) {
		t.Errorf("told of the sign-out of %v, want %v", told, ids[1])
	}
	if !signedIn(ctx, store, bobs) || count(bob.ID) != 1 {
		t.Error("another user's device was signed out")
	}

	// A device signing in again takes its own row.
	if _, _, err := store.SignInDevice(ctx, alice.ID, device("device-050")); err != nil {
		t.Fatal(err)
	}
	if n := count(alice.ID); n != MaxUserDevices || len(told) != 1 || !signedIn(ctx, store, tokens[2]) {
		t.Errorf("a device signing in again: %d devices, %d signed out", n, len(told))
	}
}

// A user who held more devices than the bound before it existed keeps
// them until their next sign-in, which brings them down to the bound: the
// most recently used stay.
func TestAUserPastTheBoundIsBroughtToItAtTheirNextSignIn(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	var told []ID
	store.OnSignOut(func(devices []ID) { told = append(told, devices...) })
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	const before = MaxUserDevices + 50
	if _, err := store.db.Exec(ctx, `INSERT INTO devices (user_id, token_hash, device_id, device_name, client, client_version, remote_address, last_activity_at)
		SELECT $1, sha256(convert_to('old-' || n, 'UTF8')), 'old-' || n, 'TV', 'Test', '1.0', '192.0.2.1', now() - n * interval '1 minute'
		FROM generate_series(1, $2::int) n`, alice.ID, before); err != nil {
		t.Fatal(err)
	}
	listed, err := store.Devices(ctx, alice.ID)
	if err != nil || len(listed) != before {
		t.Fatalf("before the sign-in: %d devices, %v", len(listed), err)
	}

	if _, _, err := store.SignInDevice(ctx, alice.ID, device("new")); err != nil {
		t.Fatal(err)
	}
	listed, err = store.Devices(ctx, alice.ID)
	if err != nil || len(listed) != MaxUserDevices {
		t.Fatalf("after the sign-in: %d devices, %v", len(listed), err)
	}
	if len(told) != before+1-MaxUserDevices {
		t.Errorf("told of %d sign-outs, want %d", len(told), before+1-MaxUserDevices)
	}
	// Most recently active first: the new device, then the old ones in
	// the order of their minutes.
	for i, kept := range listed {
		want := fmt.Sprintf("old-%d", i)
		if i == 0 {
			want = "new"
		}
		if kept.DeviceID != want {
			t.Fatalf("device %d kept is %s, want %s", i, kept.DeviceID, want)
		}
	}
}

// New capabilities make their own device's token be read again, and leave
// what every other token resolved to: a change made behind the server's
// back shows which were read. A lookup running meanwhile keeps nothing.
func TestCapabilitiesForgetTheSignInOfTheirOwnDeviceOnly(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	now := time.Now()
	store.now = func() time.Time { return now }
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	tvToken, tv, err := store.SignInDevice(ctx, alice.ID, device("tv"))
	if err != nil {
		t.Fatal(err)
	}
	phoneToken, _, err := store.SignInDevice(ctx, alice.ID, device("phone"))
	if err != nil {
		t.Fatal(err)
	}
	name := func(token string) string {
		t.Helper()
		signedIn, _, err := store.DeviceByToken(ctx, token, "192.0.2.1")
		if err != nil {
			t.Fatal(err)
		}
		return signedIn.DeviceName
	}
	if name(tvToken) != "TV" || name(phoneToken) != "TV" {
		t.Fatal("the devices do not resolve")
	}
	if _, err := store.db.Exec(ctx, "UPDATE devices SET device_name = 'read again'"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetCapabilities(ctx, tv.ID, Capabilities{SupportedCommands: []string{"Mute"}}); err != nil {
		t.Fatal(err)
	}
	if got := name(phoneToken); got != "TV" {
		t.Errorf("another device's token was read again: %q", got)
	}
	signedIn, _, err := store.DeviceByToken(ctx, tvToken, "192.0.2.1")
	if err != nil || signedIn.DeviceName != "read again" || !slices.Equal(signedIn.Capabilities.SupportedCommands, []string{"Mute"}) {
		t.Errorf("the device's own token: %q %v %v", signedIn.DeviceName, signedIn.Capabilities.SupportedCommands, err)
	}

	generation := store.signIns.current()
	store.signIns.forgetDevice(tv.ID)
	store.signIns.keepDevice("read before", resolvedDevice{device: tv, at: now}, generation)
	if _, ok := store.signIns.device("read before", now); ok {
		t.Error("a device read before its capabilities changed was kept")
	}
}
