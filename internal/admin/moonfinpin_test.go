package admin

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// A user sets, changes and removes their Moonfin PIN under My account with
// the current one, whose wrong guesses wait as in Moonfin's apps; an
// administrator sees on the user's page whether they have one and their
// wrong guesses, and sets a new one, which starts the count again, or
// removes it, without the current one. No answer holds a PIN.
func TestMoonfinPinsOnUsersPagesAndMyAccount(t *testing.T) {
	t.Parallel()
	api, _ := moonfinAPI(t)
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	path := "/users/" + api.userID("member").String() + "/moonfin/pin"
	var answers strings.Builder
	call := func(b browser, method, path string, body any) (int, map[string]any, *http.Response) {
		t.Helper()
		status, answer, response := b.call(method, path, body)
		text, _ := json.Marshal(answer)
		answers.Write(text)
		return status, answer, response
	}

	if status, body, _ := call(member, http.MethodGet, "/account/moonfin/pin", nil); status != http.StatusOK || body["set"] != false {
		t.Errorf("before any PIN: %d %v", status, body)
	}
	if status, body, _ := call(member, http.MethodPut, "/account/moonfin/pin", map[string]any{"pin": "12a4"}); status != http.StatusBadRequest ||
		body["error"] != "invalid_moonfin_pin" {
		t.Errorf("a PIN with a letter: %d %v", status, body)
	}
	if status, body, _ := call(member, http.MethodPut, "/account/moonfin/pin", map[string]any{"pin": "4821"}); status != http.StatusOK || body["set"] != true {
		t.Fatalf("set: %d %v", status, body)
	}
	if status, body, _ := call(member, http.MethodPut, "/account/moonfin/pin", map[string]any{"pin": "7777"}); status != http.StatusForbidden ||
		body["error"] != "wrong_moonfin_pin" {
		t.Errorf("a change without the current PIN: %d %v", status, body)
	}
	if _, body, _ := call(member, http.MethodGet, "/account/moonfin/pin", nil); body["wrongGuesses"] != 1.0 || body["waitUntil"] != nil {
		t.Errorf("a wrong guess counted: %v", body)
	}
	if status, body, _ := call(member, http.MethodPut, "/account/moonfin/pin", map[string]any{"pin": "7777", "currentPin": "4821"}); status != http.StatusOK ||
		body["wrongGuesses"] != 0.0 {
		t.Fatalf("a change with the current PIN: %d %v", status, body)
	}
	for i := range 5 {
		if status, body, _ := call(member, http.MethodDelete, "/account/moonfin/pin", map[string]any{"currentPin": "0000"}); status != http.StatusForbidden {
			t.Fatalf("free guess %d: %d %v", i+1, status, body)
		}
	}
	status, body, response := call(member, http.MethodDelete, "/account/moonfin/pin", map[string]any{"currentPin": "0000"})
	if status != http.StatusTooManyRequests || body["error"] != "moonfin_pin_wait" || response.Header.Get("Retry-After") != "30" {
		t.Errorf("past the free guesses: %d %v %q", status, body, response.Header.Get("Retry-After"))
	}
	if status, body, _ := call(member, http.MethodDelete, "/account/moonfin/pin", map[string]any{"currentPin": "7777"}); status != http.StatusTooManyRequests {
		t.Errorf("the right PIN while waiting: %d %v", status, body)
	}
	if status, _, _ := call(member, http.MethodGet, path, nil); status != http.StatusForbidden {
		t.Errorf("a member reading a user's PIN: %d", status)
	}
	if status, body, _ := call(administrator, http.MethodGet, path, nil); status != http.StatusOK || body["set"] != true ||
		body["wrongGuesses"] != 6.0 || body["waitUntil"] == nil {
		t.Errorf("on the user's page: %d %v", status, body)
	}
	if status, body, _ := call(administrator, http.MethodPut, path, map[string]any{"pin": "13579"}); status != http.StatusBadRequest ||
		body["error"] != "invalid_moonfin_pin" {
		t.Errorf("a PIN of five digits: %d %v", status, body)
	}
	if status, body, _ := call(administrator, http.MethodPut, path, map[string]any{"pin": "1357"}); status != http.StatusOK || body["set"] != true ||
		body["wrongGuesses"] != 0.0 || body["waitUntil"] != nil {
		t.Fatalf("an administrator setting a new PIN: %d %v", status, body)
	}
	if status, body, _ := call(member, http.MethodPut, "/account/moonfin/pin", map[string]any{"pin": "2468", "currentPin": "1357"}); status != http.StatusOK {
		t.Errorf("the member changing the PIN the administrator set: %d %v", status, body)
	}
	if status, body, _ := call(administrator, http.MethodDelete, path, nil); status != http.StatusOK || body["set"] != false {
		t.Errorf("an administrator removing it: %d %v", status, body)
	}
	if status, _, _ := call(administrator, http.MethodGet, "/users/0123456789abcdef0123456789abcdef/moonfin/pin", nil); status != http.StatusNotFound {
		t.Errorf("an unknown user: %d", status)
	}
	// A PIN as a word of its own: a time's fraction may hold its digits.
	for _, secret := range []string{`\b4821\b`, `\b7777\b`, `\b1357\b`, `\b2468\b`, `argon2`} {
		if regexp.MustCompile(secret).MatchString(answers.String()) {
			t.Errorf("an answer holds %s", secret)
		}
	}
}
