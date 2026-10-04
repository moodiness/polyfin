package admin

import (
	"net/http"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

func TestSettingsSecurity(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); body["personalAddons"] != true || body["loginAttempts"] != float64(0) ||
		body["inactiveDeviceDays"] != float64(0) || body["detailedLog"] != false {
		t.Errorf("default settings: %v", body)
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "legacyAuthorization": false, "language": "en"}
	put := func(changes map[string]any) (int, map[string]any) {
		t.Helper()
		body := map[string]any{}
		for key, value := range base {
			body[key] = value
		}
		for key, value := range changes {
			body[key] = value
		}
		status, saved, _ := administrator.call(http.MethodPut, "/settings", body)
		return status, saved
	}
	changed := map[string]any{"personalAddons": false, "loginAttempts": 5, "inactiveDeviceDays": 90, "detailedLog": true}
	// Every other setting keeps its value: the stored settings are those
	// before, with the base and the security settings saved.
	want := api.store.Settings()
	want.ServerName, want.QuickConnectEnabled, want.LegacyAuthorization, want.Language = "Polyfin", true, false, "en"
	want.PersonalAddons, want.LoginAttempts, want.InactiveDeviceDays, want.DetailedLog = false, 5, 90, true
	if status, saved := put(changed); status != http.StatusOK || saved["personalAddons"] != false || saved["loginAttempts"] != float64(5) ||
		saved["inactiveDeviceDays"] != float64(90) || saved["detailedLog"] != true {
		t.Fatalf("saving: %d %v", status, saved)
	}
	if got := api.store.Settings(); got != want {
		t.Errorf("stored: %+v, want %+v", got, want)
	}
	// An admin app that does not know them keeps them, one by one too.
	if status, saved := put(nil); status != http.StatusOK || saved["personalAddons"] != false || saved["loginAttempts"] != float64(5) ||
		saved["inactiveDeviceDays"] != float64(90) || saved["detailedLog"] != true {
		t.Errorf("saving without them: %d %v", status, saved)
	}
	for key, value := range map[string]any{"personalAddons": true, "loginAttempts": 3, "inactiveDeviceDays": 1, "detailedLog": false} {
		want := value
		if n, ok := value.(int); ok {
			want = float64(n)
		}
		if status, saved := put(map[string]any{key: value}); status != http.StatusOK || saved[key] != want {
			t.Errorf("saving %s alone: %d %v", key, status, saved)
		}
	}
	if got := api.store.Settings(); !got.PersonalAddons || got.LoginAttempts != 3 || got.InactiveDeviceDays != 1 || got.DetailedLog {
		t.Errorf("stored after saving one by one: %+v", got)
	}
	for _, tc := range []struct {
		key   string
		value int
		code  string
	}{
		{"loginAttempts", 1, "invalid_login_attempts"},
		{"loginAttempts", accounts.MinLoginAttempts - 1, "invalid_login_attempts"},
		{"loginAttempts", accounts.MaxLoginAttempts + 1, "invalid_login_attempts"},
		{"loginAttempts", -1, "invalid_login_attempts"},
		{"inactiveDeviceDays", -1, "invalid_inactive_device_days"},
		{"inactiveDeviceDays", accounts.MaxInactiveDeviceDays + 1, "invalid_inactive_device_days"},
	} {
		if status, body := put(map[string]any{tc.key: tc.value}); status != http.StatusBadRequest || body["error"] != tc.code {
			t.Errorf("%s %d: %d %v", tc.key, tc.value, status, body)
		}
	}
	if got := api.store.Settings(); got.LoginAttempts != 3 || got.InactiveDeviceDays != 1 {
		t.Errorf("a refused value changed the settings: %+v", got)
	}
}

// A user's own addons, turned off by the server or for the user, are kept
// but cannot be added, replaced, refreshed or turned on.
func TestPersonalAddonsTurnedOffAreRefused(t *testing.T) {
	api := newTestAPI(t, 10)
	manifestURL := localAddon(t)
	// Only administrators may install addons on the local network, as the
	// test's.
	administrator := api.signedIn("administrator", true)
	status, installed, _ := administrator.call(http.MethodPost, "/scopes/me/addons", map[string]string{"manifestUrl": manifestURL})
	if status != http.StatusCreated {
		t.Fatalf("installing an own addon: %d %v", status, installed)
	}
	_, shared, _ := administrator.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": manifestURL})
	self := "/users/" + api.userID("administrator").String()
	addon := "/scopes/me/addons/" + installed["id"].(string)
	refused := func(what string) {
		t.Helper()
		for _, request := range []struct {
			method, path string
			body         any
		}{
			{http.MethodPost, "/scopes/me/addons", map[string]string{"manifestUrl": manifestURL}},
			{http.MethodPatch, addon, map[string]any{"manifestUrl": manifestURL}},
			{http.MethodPost, addon + "/refresh", nil},
			{http.MethodPatch, addon, map[string]any{"enabled": true}},
		} {
			if status, body, _ := administrator.call(request.method, request.path, request.body); status != http.StatusForbidden || body["error"] != "personal_addons_disabled" {
				t.Errorf("%s: %s %s: %d %v", what, request.method, request.path, status, body)
			}
		}
		// The addon is kept, and may still be turned off.
		if status, body, _ := administrator.call(http.MethodPatch, addon, map[string]any{"enabled": false}); status != http.StatusOK || body["enabled"] != false {
			t.Errorf("%s: turning the addon off: %d %v", what, status, body)
		}
		if _, body, _ := administrator.call(http.MethodGet, "/account/addon-preferences", nil); body["personalAddons"] != false || body["useSharedAddons"] != true {
			t.Errorf("%s: preferences %v", what, body)
		}
		// The server's addons are not concerned.
		if status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/addons/"+shared["id"].(string)+"/refresh", nil); status != http.StatusOK {
			t.Errorf("%s: refreshing a server's addon: %d %v", what, status, body)
		}
	}
	allowed := func(what string) {
		t.Helper()
		if status, body, _ := administrator.call(http.MethodPatch, addon, map[string]any{"enabled": true}); status != http.StatusOK || body["enabled"] != true {
			t.Errorf("%s: turning the addon on: %d %v", what, status, body)
		}
		if status, body, _ := administrator.call(http.MethodPost, addon+"/refresh", nil); status != http.StatusOK {
			t.Errorf("%s: refreshing: %d %v", what, status, body)
		}
		if _, body, _ := administrator.call(http.MethodGet, "/account/addon-preferences", nil); body["personalAddons"] != true {
			t.Errorf("%s: preferences %v", what, body)
		}
	}
	allowed("by default")

	if status, body, _ := administrator.call(http.MethodPatch, self, map[string]any{"personalAddons": false}); status != http.StatusOK || body["personalAddons"] != false {
		t.Fatalf("turning the user's permission off: %d %v", status, body)
	}
	refused("user's permission off")
	// Other changes keep it.
	if status, body, _ := administrator.call(http.MethodPatch, self, map[string]any{"isHidden": true}); status != http.StatusOK || body["personalAddons"] != false {
		t.Errorf("another change: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPatch, self, map[string]any{"personalAddons": true}); status != http.StatusOK || body["personalAddons"] != true {
		t.Fatalf("turning the user's permission on: %d %v", status, body)
	}
	allowed("user's permission back on")

	settings := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "legacyAuthorization": false, "language": "en", "personalAddons": false}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK {
		t.Fatalf("turning the server's switch off: %d %v", status, body)
	}
	refused("server's switch off")
	settings["personalAddons"] = true
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK {
		t.Fatalf("turning the server's switch on: %d %v", status, body)
	}
	allowed("server's switch back on")
}

// userID is the identifier of the user named name.
func (api testAPI) userID(name string) accounts.ID {
	api.t.Helper()
	user, err := api.store.Authenticate(api.t.Context(), name, "correct horse")
	if err != nil {
		api.t.Fatal(err)
	}
	return user.ID
}

func TestBlockedAccountsAndUnblocking(t *testing.T) {
	// The test signs in wrong on purpose, more often than the server lets an
	// address fail.
	api := newTestAPI(t, 100)
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	path := "/users/" + api.userID("member").String()
	signIn := func(password string) (int, string) {
		t.Helper()
		status, body, _ := api.browser().call(http.MethodPost, "/session", map[string]string{"name": "member", "password": password})
		code, _ := body["error"].(string)
		return status, code
	}
	block := func() {
		t.Helper()
		for range 3 {
			if status, code := signIn("wrong password"); status != http.StatusUnauthorized || code != "invalid_credentials" {
				t.Fatalf("wrong password: %d %s", status, code)
			}
		}
	}
	blockedUntil := func() any {
		t.Helper()
		_, body, _ := administrator.call(http.MethodPatch, path, map[string]any{})
		return body["blockedUntil"]
	}

	// By default nothing blocks.
	block()
	if status, _ := signIn("correct horse"); status != http.StatusOK || blockedUntil() != nil {
		t.Fatalf("by default: %d, blocked until %v", status, blockedUntil())
	}
	settings := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "legacyAuthorization": false, "language": "en", "loginAttempts": 3}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK {
		t.Fatalf("setting the limit: %d %v", status, body)
	}

	block()
	// The right password gets the answer of a wrong one.
	if status, code := signIn("correct horse"); status != http.StatusUnauthorized || code != "invalid_credentials" {
		t.Errorf("right password of a blocked account: %d %s", status, code)
	}
	until, _ := blockedUntil().(string)
	if at, err := time.Parse(time.RFC3339, until); err != nil || time.Until(at) < 14*time.Minute || time.Until(at) > 16*time.Minute {
		t.Errorf("blocked until %q", until)
	}
	if status, _, _ := member.call(http.MethodPost, path+"/unblock", nil); status != http.StatusForbidden {
		t.Errorf("a member unblocking: %d", status)
	}
	if status, body, _ := administrator.call(http.MethodPost, "/users/0123456789abcdef0123456789abcdef/unblock", nil); status != http.StatusNotFound || body["error"] != "not_found" {
		t.Errorf("unblocking an unknown user: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPost, path+"/unblock", nil); status != http.StatusOK || body["blockedUntil"] != nil || body["name"] != "member" {
		t.Fatalf("unblocking: %d %v", status, body)
	}
	if status, _ := signIn("correct horse"); status != http.StatusOK {
		t.Errorf("after unblocking: %d", status)
	}

	// A new password set by an administrator ends the block too.
	block()
	if status, body, _ := administrator.call(http.MethodPatch, path, map[string]any{"password": "battery staple"}); status != http.StatusOK || body["blockedUntil"] != nil {
		t.Fatalf("new password: %d %v", status, body)
	}
	if status, _ := signIn("battery staple"); status != http.StatusOK {
		t.Errorf("after a new password: %d", status)
	}
}
