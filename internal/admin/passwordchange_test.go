package admin

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/moodiness/polyfin/internal/forwarded"
)

// The current password given to change one's password is limited as a
// password at sign-in is: wrong ones count toward the client's failed
// attempts, 429 once too many, and a right one with a new password that
// is refused counts for nothing.
func TestOwnPasswordChangeLimitsWrongCurrentPasswords(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 2)
	alice := api.signedIn("alice", false)
	change := func(current, next string) (int, any, string) {
		t.Helper()
		status, body, response := alice.call(http.MethodPut, "/account/password", map[string]string{"currentPassword": current, "newPassword": next})
		return status, body["error"], response.Header.Get("Retry-After")
	}
	// More often than the limit: none of these is a failed attempt.
	for i := range 5 {
		if status, code, _ := change("correct horse", "x"); status != http.StatusBadRequest || code != "invalid_password" {
			t.Fatalf("the right current password with a new one too short, %d: %d %v", i+1, status, code)
		}
	}
	for i := range 2 {
		if status, code, _ := change("a guess", "another horse"); status != http.StatusForbidden || code != "wrong_password" {
			t.Fatalf("wrong current password %d: %d %v", i+1, status, code)
		}
	}
	if status, code, retry := change("a guess", "another horse"); status != http.StatusTooManyRequests || code != "too_many_attempts" || retry == "" {
		t.Fatalf("the third wrong current password: %d %v, Retry-After %q, want 429", status, code, retry)
	}
	if status, code, _ := change("correct horse", "another horse"); status != http.StatusTooManyRequests {
		t.Errorf("the right current password past the limit: %d %v, want 429", status, code)
	}
	if _, err := api.store.Authenticate(t.Context(), "alice", "correct horse"); err != nil {
		t.Errorf("alice's password after the attempts: %v", err)
	}
}

// Wrong current passwords count for the user too, whichever client sends
// them: past the limit, another address is refused as well.
func TestOwnPasswordChangeLimitsWrongCurrentPasswordsOfEveryClient(t *testing.T) {
	t.Parallel()
	var options *Options
	api := newTestAPI(t, 2, func(o *Options, _ testDeps) { options = o })
	api.createUser("alice", false)
	// The same API behind a reverse proxy on 127.0.0.1, a known proxy, as
	// Polyfin serves it.
	proxy := httptest.NewServer(forwarded.New(api.store.KnownProxies, nil).Handler(New(*options)))
	t.Cleanup(proxy.Close)
	settings := api.store.Settings()
	settings.KnownProxies = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}
	if _, err := api.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	b := api.browser()
	b.api.url = proxy.URL
	from := func(address, method, path string, body any) (int, any) {
		t.Helper()
		status, answer, _ := b.call(method, path, body, "X-Forwarded-For", address)
		return status, answer["error"]
	}
	const first, second = "203.0.113.5", "198.51.100.7"
	if status, code := from(first, http.MethodPost, "/session", map[string]string{"name": "alice", "password": "correct horse"}); status != http.StatusOK {
		t.Fatalf("signing in: %d %v", status, code)
	}
	guess := map[string]string{"currentPassword": "a guess", "newPassword": "another horse"}
	for i := range 2 {
		if status, code := from(first, http.MethodPut, "/account/password", guess); status != http.StatusForbidden || code != "wrong_password" {
			t.Fatalf("wrong current password %d: %d %v", i+1, status, code)
		}
	}
	if status, code := from(second, http.MethodPut, "/account/password", guess); status != http.StatusTooManyRequests || code != "too_many_attempts" {
		t.Errorf("a wrong current password from another address, past the user's limit: %d %v, want 429", status, code)
	}
}

// Wrong current passwords count toward the block of the client for the
// account, with those of its sign-ins: the one that makes the limit blocks
// it, the right one is then refused as a wrong one, and Unblock ends it.
func TestOwnPasswordChangeCountsTowardTheBlock(t *testing.T) {
	t.Parallel()
	// The test sends wrong passwords on purpose, more often than the server
	// lets an address fail.
	api := newTestAPI(t, 100)
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	path := "/users/" + api.userID("member").String()
	settings := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "legacyAuthorization": false, "language": "en", "loginAttempts": 3}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK {
		t.Fatalf("setting the limit: %d %v", status, body)
	}
	change := func(current string) (int, any) {
		t.Helper()
		status, body, _ := member.call(http.MethodPut, "/account/password", map[string]string{"currentPassword": current, "newPassword": "another horse"})
		return status, body["error"]
	}
	blockedUntil := func() any {
		t.Helper()
		_, body, _ := administrator.call(http.MethodPatch, path, map[string]any{})
		return body["blockedUntil"]
	}

	// One wrong password at sign-in and two wrong current passwords make
	// three in a row from the client.
	if status, body, _ := api.browser().call(http.MethodPost, "/session", map[string]string{"name": "member", "password": "a guess"}); status != http.StatusUnauthorized {
		t.Fatalf("a wrong password at sign-in: %d %v", status, body)
	}
	if status, code := change("a guess"); status != http.StatusForbidden || code != "wrong_password" || blockedUntil() != nil {
		t.Fatalf("the second wrong password: %d %v, blocked until %v", status, code, blockedUntil())
	}
	if status, code := change("a guess"); status != http.StatusForbidden || code != "wrong_password" || blockedUntil() == nil {
		t.Fatalf("the third wrong password: %d %v, blocked until %v", status, code, blockedUntil())
	}
	if status, code := change("correct horse"); status != http.StatusForbidden || code != "wrong_password" {
		t.Fatalf("the right current password from the blocked client: %d %v", status, code)
	}
	if _, err := api.store.Authenticate(t.Context(), "member", "correct horse"); err != nil {
		t.Errorf("the member's password after the refused change: %v", err)
	}
	if status, body, _ := administrator.call(http.MethodPost, path+"/unblock", nil); status != http.StatusOK || body["blockedUntil"] != nil {
		t.Fatalf("unblocking: %d %v", status, body)
	}
	if status, code := change("correct horse"); status != http.StatusNoContent {
		t.Errorf("the right current password once unblocked: %d %v", status, code)
	}
}
