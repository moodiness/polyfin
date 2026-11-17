package accounts

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
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
