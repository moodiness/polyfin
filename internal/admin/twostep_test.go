package admin

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/totp"
)

// A test uses two codes of an app at most: the one that sets it up, at
// now, then the next step's, which stays within the drift for more than a
// step. Recovery codes do the rest.

// code is the code of secret a number of steps from now.
func code(t *testing.T, secret string, steps int) string {
	t.Helper()
	c, err := totp.Code(secret, time.Now().Add(time.Duration(steps)*totp.Step))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// stringList reads a JSON list of strings.
func stringList(t *testing.T, value any) []string {
	t.Helper()
	list, _ := value.([]any)
	result := make([]string, len(list))
	for i, item := range list {
		result[i], _ = item.(string)
	}
	return result
}

// turnOnTwoStep sets two-step sign-in up from My account of a signed-in
// browser, and returns the secret and the recovery codes.
func turnOnTwoStep(t *testing.T, b browser) (string, []string) {
	t.Helper()
	status, setup, _ := b.call(http.MethodPost, "/account/two-step/setup", nil)
	secret, _ := setup["secret"].(string)
	if status != http.StatusOK || secret == "" {
		t.Fatalf("setup: %d %v", status, setup)
	}
	status, body, _ := b.call(http.MethodPost, "/account/two-step/confirm", map[string]string{"code": code(t, secret, 0)})
	codes := stringList(t, body["recoveryCodes"])
	if status != http.StatusOK || len(codes) != accounts.RecoveryCodes {
		t.Fatalf("confirm: %d %v", status, body)
	}
	return secret, codes
}

// password signs in with a password in a new browser, and returns it with
// the answer.
func (api testAPI) password(name string) (browser, int, map[string]any) {
	b := api.browser()
	status, body, _ := b.call(http.MethodPost, "/session", map[string]string{"name": name, "password": "correct horse"})
	return b, status, body
}

func TestTwoStepSetupOnlyAfterAConfirmedCode(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	alice := api.signedIn("alice", false)

	if status, body, _ := alice.call(http.MethodGet, "/account/two-step", nil); status != http.StatusOK || body["on"] != false || body["required"] != false {
		t.Fatalf("before: %d %v", status, body)
	}
	if status, body, _ := alice.call(http.MethodPost, "/account/two-step/confirm", map[string]string{"code": "123456"}); status != http.StatusConflict ||
		body["error"] != "two_step_not_started" {
		t.Errorf("confirmed before a setup: %d %v", status, body)
	}
	status, setup, _ := alice.call(http.MethodPost, "/account/two-step/setup", nil)
	secret, _ := setup["secret"].(string)
	uri, _ := setup["uri"].(string)
	qr := stringList(t, setup["qr"])
	if status != http.StatusOK || len(secret) != 32 || !strings.HasPrefix(uri, "otpauth://totp/Polyfin:alice?") || !strings.Contains(uri, secret) {
		t.Fatalf("setup: %d %v", status, setup)
	}
	if len(qr) < 21 || len(qr[0]) != len(qr) || !strings.HasPrefix(qr[0], "1111111") {
		t.Errorf("QR code: %v", qr)
	}
	if status, body, _ := alice.call(http.MethodPost, "/account/two-step/confirm", map[string]string{"code": code(t, secret, -3)}); status != http.StatusBadRequest ||
		body["error"] != "invalid_code" {
		t.Errorf("a wrong code: %d %v", status, body)
	}
	if _, body, _ := alice.call(http.MethodGet, "/account/two-step", nil); body["on"] != false {
		t.Errorf("on after a wrong code: %v", body)
	}
	// Signing in still takes the password alone.
	if _, status, body := api.password("alice"); status != http.StatusOK || body["user"] == nil {
		t.Errorf("sign-in during the setup: %d %v", status, body)
	}
	status, body, _ := alice.call(http.MethodPost, "/account/two-step/confirm", map[string]string{"code": code(t, secret, 0)})
	if status != http.StatusOK || len(stringList(t, body["recoveryCodes"])) != accounts.RecoveryCodes {
		t.Fatalf("confirm: %d %v", status, body)
	}
	_, body, _ = alice.call(http.MethodGet, "/account/two-step", nil)
	if body["on"] != true || body["recoveryCodes"] != float64(accounts.RecoveryCodes) || body["unreadable"] != false {
		t.Errorf("after: %v", body)
	}
	if _, found := body["secret"]; found {
		t.Errorf("the secret is answered once set up: %v", body)
	}
	if status, body, _ := alice.call(http.MethodPost, "/account/two-step/setup", nil); status != http.StatusConflict || body["error"] != "two_step_on" {
		t.Errorf("a new secret while on: %d %v", status, body)
	}
}

// After the password, no session exists until the code; the step belongs
// to the browser that entered the password, and expires.
func TestTwoStepSignIn(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	alice := api.signedIn("alice", false)
	secret, recovery := turnOnTwoStep(t, alice)

	b, status, body := api.password("alice")
	if status != http.StatusOK || body["twoStep"] != "code" || body["user"] != nil {
		t.Fatalf("password: %d %v", status, body)
	}
	if status, _, _ := b.call(http.MethodGet, "/session", nil); status != http.StatusUnauthorized {
		t.Errorf("a session before the code: %d", status)
	}
	if status, _, _ := b.call(http.MethodGet, "/account/two-step", nil); status != http.StatusUnauthorized {
		t.Errorf("My account before the code: %d", status)
	}
	next := code(t, secret, 1)
	if status, body, _ := api.browser().call(http.MethodPost, "/session/two-step", map[string]string{"code": next}); status != http.StatusUnauthorized ||
		body["error"] != "two_step_expired" {
		t.Errorf("the code from another browser: %d %v", status, body)
	}
	if status, body, _ := b.call(http.MethodPost, "/session/two-step", map[string]string{"code": "000000"}); status != http.StatusBadRequest ||
		body["error"] != "invalid_code" {
		t.Errorf("a wrong code: %d %v", status, body)
	}
	status, body, _ = b.call(http.MethodPost, "/session/two-step", map[string]string{"code": next})
	if user, _ := body["user"].(map[string]any); status != http.StatusOK || user["name"] != "alice" {
		t.Fatalf("the code: %d %v", status, body)
	}
	if status, _, _ := b.call(http.MethodGet, "/session", nil); status != http.StatusOK {
		t.Errorf("no session after the code: %d", status)
	}
	if status, _, _ := b.call(http.MethodPost, "/session/two-step", map[string]string{"code": next}); status != http.StatusUnauthorized {
		t.Errorf("the step used again: %d", status)
	}

	// The same code again, in a new sign-in, is refused.
	b, _, _ = api.password("alice")
	if status, body, _ := b.call(http.MethodPost, "/session/two-step", map[string]string{"code": next}); status != http.StatusBadRequest {
		t.Errorf("a code replayed: %d %v", status, body)
	}
	// A recovery code signs in, once.
	if status, body, _ := b.call(http.MethodPost, "/session/two-step", map[string]string{"code": recovery[3]}); status != http.StatusOK {
		t.Errorf("a recovery code: %d %v", status, body)
	}
	b, _, _ = api.password("alice")
	if status, _, _ := b.call(http.MethodPost, "/session/two-step", map[string]string{"code": recovery[3]}); status != http.StatusBadRequest {
		t.Errorf("a recovery code used again: %d", status)
	}
	if entries := activityOf(t, api.signedIn("root", true), "UserRecoveryCodeUsed"); len(entries) != 1 || entries[0]["severity"] != "Warning" {
		t.Errorf("recovery codes used in the activity log: %v", entries)
	}

	// The step expires.
	later := time.Now().Add(stepLifetime + time.Minute)
	api.clock.Store(&later)
	if status, body, _ := b.call(http.MethodPost, "/session/two-step", map[string]string{"code": recovery[4]}); status != http.StatusUnauthorized ||
		body["error"] != "two_step_expired" {
		t.Errorf("an expired step: %d %v", status, body)
	}
}

// activityOf reads the entries of a type in the activity log, as an
// administrator.
func activityOf(t *testing.T, administrator browser, kind string) []map[string]any {
	t.Helper()
	status, body, _ := administrator.call(http.MethodGet, "/activity?type="+kind, nil)
	if status != http.StatusOK {
		t.Fatalf("activity: %d %v", status, body)
	}
	items, _ := body["items"].([]any)
	entries := make([]map[string]any, len(items))
	for i, item := range items {
		entries[i], _ = item.(map[string]any)
	}
	return entries
}

// Wrong codes count as wrong passwords do: past the limit, even the right
// code is refused for a while.
func TestWrongCodesAreThrottled(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 2)
	alice := api.signedIn("alice", false)
	_, recovery := turnOnTwoStep(t, alice)
	b, _, _ := api.password("alice")
	for range 2 {
		if status, _, _ := b.call(http.MethodPost, "/session/two-step", map[string]string{"code": "000000"}); status != http.StatusBadRequest {
			t.Fatalf("a wrong code: %d", status)
		}
	}
	status, body, response := b.call(http.MethodPost, "/session/two-step", map[string]string{"code": recovery[0]})
	if status != http.StatusTooManyRequests || body["error"] != "too_many_attempts" || response.Header.Get("Retry-After") == "" {
		t.Errorf("the right code past the limit: %d %v", status, body)
	}
	// A right password between wrong codes forgets none of them.
	if status, _, _ := api.browser().call(http.MethodPost, "/session", map[string]string{"name": "alice", "password": "correct horse"}); status != http.StatusTooManyRequests {
		t.Errorf("the password past the limit: %d", status)
	}
}

func TestTwoStepRequiredForAdministrators(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	root := api.signedIn("root", true)
	api.signedIn("member", false)
	device, _, err := api.store.SignInDevice(t.Context(), mustUser(t, api, "root").ID, accounts.DeviceInfo{DeviceID: "web", DeviceName: "Browser",
		Client: "Jellyfin Web", ClientVersion: "10.11.0", RemoteAddress: "192.0.2.1"})
	if err != nil {
		t.Fatal(err)
	}
	settings := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "legacyAuthorization": false, "language": "en",
		"twoStepAdministrators": true}
	if status, body, _ := root.call(http.MethodPut, "/settings", settings); status != http.StatusOK || body["twoStepAdministrators"] != true {
		t.Fatalf("saving: %d %v", status, body)
	}
	// The session open stays.
	if status, body, _ := root.call(http.MethodGet, "/account/two-step", nil); status != http.StatusOK || body["required"] != true || body["on"] != false {
		t.Errorf("the open session: %d %v", status, body)
	}
	// A member signs in with their password alone.
	if _, status, body := api.password("member"); status != http.StatusOK || body["user"] == nil {
		t.Errorf("member: %d %v", status, body)
	}
	// An administrator without it sets it up before their session opens.
	b, status, body := api.password("root")
	setup, _ := body["setup"].(map[string]any)
	secret, _ := setup["secret"].(string)
	if status != http.StatusOK || body["twoStep"] != "setup" || secret == "" || len(stringList(t, setup["qr"])) == 0 {
		t.Fatalf("password: %d %v", status, body)
	}
	if status, _, _ := b.call(http.MethodGet, "/session", nil); status != http.StatusUnauthorized {
		t.Errorf("a session before the setup: %d", status)
	}
	// Nor does a Jellyfin app's token open one.
	if status, _, _ := api.browser().call(http.MethodPost, "/session/jellyfin", nil, "Authorization", `MediaBrowser Token="`+device+`"`); status != http.StatusUnauthorized {
		t.Errorf("a session from a Jellyfin token: %d", status)
	}
	if status, _, _ := b.call(http.MethodPost, "/session/two-step", map[string]string{"code": code(t, secret, -3)}); status != http.StatusBadRequest {
		t.Errorf("a wrong code: %d", status)
	}
	status, body, _ = b.call(http.MethodPost, "/session/two-step", map[string]string{"code": code(t, secret, 0)})
	if status != http.StatusOK || body["user"] == nil || len(stringList(t, body["recoveryCodes"])) != accounts.RecoveryCodes {
		t.Fatalf("setup: %d %v", status, body)
	}
	if status, body, _ := b.call(http.MethodGet, "/account/two-step", nil); status != http.StatusOK || body["on"] != true {
		t.Errorf("after the setup: %d %v", status, body)
	}
	if status, body, _ := b.call(http.MethodPost, "/account/two-step/off", map[string]string{"password": "correct horse"}); status != http.StatusConflict ||
		body["error"] != "two_step_required" {
		t.Errorf("turned off while required: %d %v", status, body)
	}
	// From now on, the code.
	_, status, body = api.password("root")
	if status != http.StatusOK || body["twoStep"] != "code" {
		t.Errorf("password after the setup: %d %v", status, body)
	}
}

func mustUser(t *testing.T, api testAPI, name string) accounts.User {
	t.Helper()
	users, err := api.store.Users(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range users {
		if user.Name == name {
			return user
		}
	}
	t.Fatalf("no user %s", name)
	return accounts.User{}
}

// An administrator turns off the two-step sign-in of a user who lost their
// device, which the activity log records.
func TestAdministratorTurnsOffTwoStep(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	root := api.signedIn("root", true)
	alice := api.signedIn("alice", false)
	turnOnTwoStep(t, alice)
	turnOnTwoStep(t, root)
	id := mustUser(t, api, "alice").ID.String()

	if status, _, _ := alice.call(http.MethodPost, "/users/"+id+"/two-step/off", nil); status != http.StatusForbidden {
		t.Errorf("by a member: %d", status)
	}
	status, body, _ := root.call(http.MethodPost, "/users/"+id+"/two-step/off", nil)
	if status != http.StatusOK || body["twoStep"] != false || body["name"] != "alice" {
		t.Fatalf("turned off: %d %v", status, body)
	}
	if status, body, _ := root.call(http.MethodPost, "/users/"+id+"/two-step/off", nil); status != http.StatusConflict || body["error"] != "two_step_off" {
		t.Errorf("turned off again: %d %v", status, body)
	}
	rootID := mustUser(t, api, "root").ID.String()
	if status, body, _ := root.call(http.MethodPost, "/users/"+rootID+"/two-step/off", nil); status != http.StatusConflict || body["error"] != "two_step_own" {
		t.Errorf("their own: %d %v", status, body)
	}
	entries := activityOf(t, root, "UserTwoStepOff")
	if len(entries) != 1 || entries[0]["name"] != "root turned two-step sign-in off for alice" || entries[0]["severity"] != "Warning" {
		t.Errorf("activity: %v", entries)
	}
	// The sessions open stay; the password alone signs in again.
	if status, _, _ := alice.call(http.MethodGet, "/session", nil); status != http.StatusOK {
		t.Errorf("the open session: %d", status)
	}
	if _, status, body := api.password("alice"); status != http.StatusOK || body["user"] == nil {
		t.Errorf("password after: %d %v", status, body)
	}
}

// A user turns their own off, or makes new recovery codes, with a code or
// their password.
func TestOwnTwoStepChangesAskACodeOrThePassword(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	alice := api.signedIn("alice", false)
	_, recovery := turnOnTwoStep(t, alice)

	if status, body, _ := alice.call(http.MethodPost, "/account/two-step/recovery-codes", map[string]string{"password": "wrong horse"}); status != http.StatusForbidden ||
		body["error"] != "wrong_password" {
		t.Errorf("a wrong password: %d %v", status, body)
	}
	status, body, _ := alice.call(http.MethodPost, "/account/two-step/recovery-codes", map[string]string{"password": "correct horse"})
	fresh := stringList(t, body["recoveryCodes"])
	if status != http.StatusOK || len(fresh) != accounts.RecoveryCodes {
		t.Fatalf("new codes: %d %v", status, body)
	}
	b, _, _ := api.password("alice")
	if status, _, _ := b.call(http.MethodPost, "/session/two-step", map[string]string{"code": recovery[0]}); status != http.StatusBadRequest {
		t.Errorf("a former recovery code: %d", status)
	}
	if status, body, _ := alice.call(http.MethodPost, "/account/two-step/off", map[string]string{"code": "000000"}); status != http.StatusBadRequest ||
		body["error"] != "invalid_code" {
		t.Errorf("off with a wrong code: %d %v", status, body)
	}
	if status, body, _ := alice.call(http.MethodPost, "/account/two-step/off", map[string]string{"code": fresh[0]}); status != http.StatusNoContent {
		t.Fatalf("off with a recovery code: %d %v", status, body)
	}
	if _, body, _ := alice.call(http.MethodGet, "/account/two-step", nil); body["on"] != false {
		t.Errorf("after: %v", body)
	}
}

// A secret POLYFIN_SECRET_KEY no longer reads, as after a restore under
// another key, lets its user in with a recovery code, and an administrator
// turns it off.
func TestUnreadableTwoStepSecret(t *testing.T) {
	t.Parallel()
	before, _ := secrets.New(bytes.Repeat([]byte{1}, secrets.KeySize))
	after, _ := secrets.New(bytes.Repeat([]byte{2}, secrets.KeySize))
	var former *accounts.Store
	api := newTestAPI(t, 10, func(o *Options, d testDeps) {
		var err error
		if former, err = accounts.Open(t.Context(), d.pool, accounts.Sealing(before)); err != nil {
			t.Fatal(err)
		}
		if o.Accounts, err = accounts.Open(t.Context(), d.pool, accounts.Sealing(after)); err != nil {
			t.Fatal(err)
		}
	})
	root := api.signedIn("root", true)
	alice, err := former.CreateUser(t.Context(), accounts.NewUser{Name: "alice", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	setup, err := former.StartTwoStep(t.Context(), alice)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := former.ConfirmTwoStep(t.Context(), alice.ID, code(t, setup.Secret, 0))
	if err != nil {
		t.Fatal(err)
	}

	b, _, body := api.password("alice")
	if body["twoStep"] != "code" {
		t.Fatalf("password: %v", body)
	}
	if status, _, _ := b.call(http.MethodPost, "/session/two-step", map[string]string{"code": code(t, setup.Secret, 1)}); status != http.StatusBadRequest {
		t.Errorf("a code under another key: %d", status)
	}
	if status, body, _ := b.call(http.MethodPost, "/session/two-step", map[string]string{"code": recovery[0]}); status != http.StatusOK {
		t.Fatalf("a recovery code under another key: %d %v", status, body)
	}
	if _, body, _ := b.call(http.MethodGet, "/account/two-step", nil); body["unreadable"] != true || body["on"] != true {
		t.Errorf("state: %v", body)
	}
	if status, body, _ := root.call(http.MethodPost, "/users/"+alice.ID.String()+"/two-step/off", nil); status != http.StatusOK || body["twoStep"] != false {
		t.Errorf("turned off: %d %v", status, body)
	}
}
