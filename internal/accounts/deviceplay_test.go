package accounts

import (
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// How a device plays is kept for a signed-in device, replaced as it
// changes, bounded in size and forgotten as asked; a device signed out
// keeps nothing, even written after.
func TestDevicePlayIsKeptForSignedInDevices(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	token, tv, err := store.SignInDevice(ctx, alice.ID, device("tv"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.DevicePlay(ctx, tv.ID); ok || err != nil {
		t.Fatalf("before any: %v %v", ok, err)
	}
	bitrate := int64(120_000_000)
	first := DevicePlay{Profile: json.RawMessage(`{"Name":"first"}`), Bitrate: &bitrate}
	second := DevicePlay{Profile: json.RawMessage(`{"Name":"second"}`), FromCapabilities: true}
	for _, play := range []DevicePlay{first, second} {
		if err := store.SetDevicePlay(ctx, tv.ID, play); err != nil {
			t.Fatal(err)
		}
		kept, ok, err := store.DevicePlay(ctx, tv.ID)
		var keptName, wantName struct{ Name string }
		_ = json.Unmarshal(kept.Profile, &keptName)
		_ = json.Unmarshal(play.Profile, &wantName)
		if !ok || err != nil || keptName != wantName || kept.FromCapabilities != play.FromCapabilities ||
			(kept.Bitrate == nil) != (play.Bitrate == nil) || kept.Bitrate != nil && *kept.Bitrate != *play.Bitrate {
			t.Errorf("kept %+v %v %v, want %+v", kept, ok, err, play)
		}
	}
	large := DevicePlay{Profile: json.RawMessage(`{"Name":"` + strings.Repeat("x", MaxDevicePlayProfile) + `"}`)}
	if err := store.SetDevicePlay(ctx, tv.ID, large); !errors.Is(err, ErrDevicePlayTooLarge) {
		t.Errorf("a large profile: %v", err)
	}
	if err := store.ForgetDevicePlay(ctx, tv.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := store.DevicePlay(ctx, tv.ID); ok {
		t.Error("kept once forgotten")
	}
	if err := store.SignOutDevice(ctx, token); err != nil {
		t.Fatal(err)
	}
	if err := store.SetDevicePlay(ctx, tv.ID, first); err != nil {
		t.Errorf("for a device signed out: %v", err)
	}
	if _, ok, _ := store.DevicePlay(ctx, tv.ID); ok {
		t.Error("kept for a device signed out")
	}
}

// The sweep of unused devices forgets how the devices unused for
// UnusedDevicePlay play, even while unused devices stay signed in, and
// keeps how the others play.
func TestUnusedDevicesForgetHowTheyPlay(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	_, tv, _ := store.SignInDevice(ctx, alice.ID, device("tv"))
	phoneToken, phone, err := store.SignInDevice(ctx, alice.ID, device("phone"))
	if err != nil {
		t.Fatal(err)
	}
	play := DevicePlay{Profile: json.RawMessage(`{"Name":"app"}`)}
	for _, d := range []Device{tv, phone} {
		if err := store.SetDevicePlay(ctx, d.ID, play); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	later := clockAt(store, now)
	lastUsed := func(d Device, ago time.Duration) {
		t.Helper()
		if _, err := store.db.Exec(ctx, "UPDATE devices SET last_activity_at = $2 WHERE id = $1", d.ID, now.Add(-ago)); err != nil {
			t.Fatal(err)
		}
	}
	lastUsed(tv, UnusedDevicePlay-time.Hour)
	lastUsed(phone, UnusedDevicePlay+time.Hour)
	kept := func(d Device) bool {
		t.Helper()
		_, ok, err := store.DevicePlay(ctx, d.ID)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	sweep := func() {
		t.Helper()
		if err := store.SweepInactiveDevices(ctx, slog.New(slog.DiscardHandler)); err != nil {
			t.Fatal(err)
		}
	}
	sweep()
	if kept(phone) || !kept(tv) {
		t.Errorf("after the sweep: the phone's kept %v, the TV's %v", kept(phone), kept(tv))
	}
	later(2 * time.Hour)
	sweep()
	if kept(tv) {
		t.Error("the TV's kept once unused for as long")
	}
	if !signedIn(ctx, store, phoneToken) {
		t.Error("the sweep signed the phone out")
	}
}

// The latest profile a user's device of an app and name told in a
// PlaybackInfo stands for another of them that told none: those of other
// users, other names or apps, and profiles from capabilities alone, do
// not count.
func TestLatestDevicePlayOfTheSameApp(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	bob := mustCreate(t, store, NewUser{Name: "bob", Password: "correct horse"})
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	told := func(user ID, info DeviceInfo, name string, capabilities bool) {
		t.Helper()
		_, d, err := store.SignInDevice(ctx, user, info)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.SetDevicePlay(ctx, d.ID, DevicePlay{Profile: json.RawMessage(`{"Name":"` + name + `"}`), FromCapabilities: capabilities}); err != nil {
			t.Fatal(err)
		}
		at = at.Add(time.Minute)
		if _, err := store.db.Exec(ctx, "UPDATE device_play_profiles SET updated_at = $2 WHERE device_id = $1", d.ID, at); err != nil {
			t.Fatal(err)
		}
	}
	phone := device("phone")
	phone.DeviceName = "Phone"
	told(alice.ID, device("tv-1"), "older", false)
	told(alice.ID, device("tv-2"), "latest", false)
	told(alice.ID, device("tv-3"), "capabilities", true)
	told(alice.ID, phone, "phone", false)
	told(bob.ID, device("tv-4"), "bob", false)
	latest := func(user ID, client, name string) string {
		t.Helper()
		play, ok, err := store.LatestDevicePlay(ctx, user, client, name)
		if err != nil {
			t.Fatal(err)
		}
		var profile struct{ Name string }
		if ok {
			_ = json.Unmarshal(play.Profile, &profile)
		}
		return profile.Name
	}
	if got := latest(alice.ID, "Test", "TV"); got != "latest" {
		t.Errorf("alice's TV: %q, want latest", got)
	}
	if got := latest(alice.ID, "Test", "Phone"); got != "phone" {
		t.Errorf("alice's phone: %q", got)
	}
	if got := latest(alice.ID, "Other", "TV"); got != "" {
		t.Errorf("another app: %q, want none", got)
	}
}
