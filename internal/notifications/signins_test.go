package notifications

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// A blocked account is told to the server's targets and administrators'
// own, linking to the user's page, and to the user's own targets, with the
// attempt that blocked it; nobody else's targets hear of it.
func TestAccountBlockedIsToldToAdministratorsAndTheUser(t *testing.T) {
	h := newHarness(t)
	ctx := t.Context()
	settings := h.store.Settings()
	settings.PublicAddress, settings.ServerName = "https://media.example.org", "Home"
	if _, err := h.store.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	h.Service.zone = time.UTC
	other, err := h.store.CreateUser(ctx, accounts.NewUser{Name: "other", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	h.add(t, nil, h.webhook("/server", AccountBlocked))
	h.add(t, &h.admin, h.webhook("/admin", AccountBlocked))
	h.add(t, &h.member, h.webhook("/member", AccountBlocked))
	h.add(t, &h.member, h.webhook("/member-episodes", NewEpisode))
	h.add(t, &other, h.webhook("/other", AccountBlocked))

	blocked := h.member
	until := time.Date(2026, 10, 4, 12, 15, 0, 0, time.UTC)
	blocked.BlockedUntil, blocked.InvalidLoginAttempts = &until, 5
	h.AccountBlocked(blocked, accounts.DeviceInfo{DeviceID: "pc-1", DeviceName: "Chrome", Client: "Jellyfin Web", ClientVersion: "10.11.0",
		RemoteAddress: "203.0.113.5"})
	server := h.targets.wait(t, "/server", 1)[0].body
	admin := h.targets.wait(t, "/admin", 1)[0].body
	member := h.targets.wait(t, "/member", 1)[0].body
	h.idle(t)
	if got := len(h.targets.at("/other")) + len(h.targets.at("/member-episodes")); got != 0 {
		t.Errorf("%d messages to targets that are not told", got)
	}
	for path, body := range map[string]map[string]any{"/server": server, "/admin": admin} {
		who, _ := body["user"].(map[string]any)
		signIn, _ := body["signIn"].(map[string]any)
		block, _ := body["block"].(map[string]any)
		if body["type"] != AccountBlocked || body["title"] != "Account blocked: member" || who["name"] != "member" ||
			body["url"] != "https://media.example.org/admin/users/"+h.member.ID.String() {
			t.Errorf("%s: %v", path, body)
		}
		if signIn["address"] != "203.0.113.5" || signIn["app"] != "Jellyfin Web" || signIn["appVersion"] != "10.11.0" || signIn["device"] != "Chrome" ||
			signIn["deviceId"] != "pc-1" || block["until"] != "2026-10-04T12:15:00Z" || block["attempts"] != 5.0 {
			t.Errorf("%s: the attempt %v, the block %v", path, signIn, block)
		}
		if want := "5 wrong passwords in a row: member cannot sign in until 12:15. Last attempt from 203.0.113.5, with Jellyfin Web on Chrome."; !strings.HasPrefix(body["message"].(string), want) {
			t.Errorf("%s: message %q", path, body["message"])
		}
	}
	if member["title"] != "Your account is blocked" || member["url"] != "https://media.example.org/admin/me/account#password" ||
		!strings.Contains(member["message"].(string), "your account member cannot sign in until 12:15") {
		t.Errorf("the member's message: %v", member)
	}
}

// A sign-in on a new device is told to the user's own targets only: not
// to the server's, which cannot choose it, nor to administrators'.
func TestNewDeviceIsToldToTheUserOnly(t *testing.T) {
	h := newHarness(t)
	if _, err := h.Create(t.Context(), nil, h.webhook("/server", NewDevice)); !errors.Is(err, ErrInvalidEvents) {
		t.Errorf("a server target choosing new devices: %v", err)
	}
	h.add(t, &h.admin, h.webhook("/admin", NewDevice))
	h.add(t, &h.member, h.webhook("/member", NewDevice))
	h.NewDevice(h.member, accounts.Device{DeviceInfo: accounts.DeviceInfo{DeviceID: "phone-1", DeviceName: "Phone", Client: "Example App",
		ClientVersion: "0.15", RemoteAddress: "198.51.100.7"}})
	body := h.targets.wait(t, "/member", 1)[0].body
	h.idle(t)
	if got := len(h.targets.at("/admin")); got != 0 {
		t.Errorf("an administrator was told of a member's device %d times", got)
	}
	signIn, _ := body["signIn"].(map[string]any)
	if body["type"] != NewDevice || body["title"] != "Sign-in on a new device" || signIn["device"] != "Phone" || signIn["app"] != "Example App" ||
		signIn["address"] != "198.51.100.7" || body["block"] != nil {
		t.Errorf("the member's message: %v", body)
	}
	if want := "Example App on Phone signed in to member from 198.51.100.7. If it was not you, change your password: a new password signs every other device out."; body["message"] != want {
		t.Errorf("message %q, want %q", body["message"], want)
	}
}
