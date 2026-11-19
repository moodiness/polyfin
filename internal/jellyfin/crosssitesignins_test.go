package jellyfin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// fromSite returns a call to s from the client at 203.0.113.20 that the
// browser tells came from site (Sec-Fetch-Site), or tells nothing of when
// site is empty, as an app.
func fromSite(t *testing.T, s testServer) func(site, method, target, authorization string, body any) int {
	return func(site, method, target, authorization string, body any) int {
		t.Helper()
		encoded, _ := json.Marshal(body)
		request := httptest.NewRequestWithContext(t.Context(), method, target, bytes.NewReader(encoded))
		request.RemoteAddr = "203.0.113.20:50000"
		request.Header.Set("Content-Type", "application/json")
		if authorization != "" {
			request.Header.Set("Authorization", authorization)
		}
		if site != "" {
			request.Header.Set("Sec-Fetch-Site", site)
		}
		recorder := httptest.NewRecorder()
		s.handler.ServeHTTP(recorder, request)
		if recorder.Code == http.StatusTooManyRequests && recorder.Header().Get("Retry-After") == "" {
			t.Errorf("%s %s: 429 without Retry-After", method, target)
		}
		return recorder.Code
	}
}

// Wrong passwords and PINs that a page of another site had the browser
// send, as many as the limit, leave the client's own sign-ins as they
// were: the right password signs in from the same address, from the same
// origin and from an app.
func TestOtherSitesFailedSignInsNeverRefuseTheClientsOwn(t *testing.T) {
	t.Parallel()
	const limit = 3
	s := newTestServer(t, limit)
	s.user("alice", nil)
	call := fromSite(t, s)
	signIn := func(site, password string) int {
		t.Helper()
		return call(site, http.MethodPost, "/Users/AuthenticateByName", app("browser", ""), map[string]string{"Username": "alice", "Pw": password})
	}
	for i := range limit - 1 {
		if status := signIn("cross-site", "wrong horse"); status != http.StatusUnauthorized {
			t.Fatalf("wrong password %d from another site: %d, want 401", i+1, status)
		}
	}
	if status := call("cross-site", http.MethodPost, "/Users/ForgotPassword/Pin", "", map[string]string{"Pin": "00000000"}); status != http.StatusNotFound {
		t.Fatalf("wrong PIN from another site: %d, want 404", status)
	}
	if status := signIn("same-origin", "correct horse"); status != http.StatusOK {
		t.Errorf("the right password from the same origin after %d failures from another site: %d, want 200", limit, status)
	}
	if status := signIn("", "correct horse"); status != http.StatusOK {
		t.Errorf("the right password from an app after %d failures from another site: %d, want 200", limit, status)
	}
}

// Failures that a page of another site had the browser send meet a limit
// of their own, wrong passwords and PINs together: past it, every attempt
// from another site is refused, the right password included, and the
// client's own wrong passwords still count from none toward theirs.
func TestOtherSitesFailedSignInsMeetTheirOwnLimit(t *testing.T) {
	t.Parallel()
	const limit = 3
	s := newTestServer(t, limit)
	s.user("alice", nil)
	call := fromSite(t, s)
	signIn := func(site, password string) int {
		t.Helper()
		return call(site, http.MethodPost, "/Users/AuthenticateByName", app("browser", ""), map[string]string{"Username": "alice", "Pw": password})
	}
	pin := func(site string) int {
		t.Helper()
		return call(site, http.MethodPost, "/Users/ForgotPassword/Pin", "", map[string]string{"Pin": "00000000"})
	}
	if status := pin("cross-site"); status != http.StatusNotFound {
		t.Fatalf("wrong PIN from another site: %d, want 404", status)
	}
	if status := signIn("cross-site", "wrong horse"); status != http.StatusUnauthorized {
		t.Fatalf("wrong password from another site: %d, want 401", status)
	}
	if status := pin("cross-site"); status != http.StatusNotFound {
		t.Fatalf("second wrong PIN from another site: %d, want 404", status)
	}
	if status := signIn("cross-site", "correct horse"); status != http.StatusTooManyRequests {
		t.Errorf("the right password from another site past its limit: %d, want 429", status)
	}
	if status := pin("cross-site"); status != http.StatusTooManyRequests {
		t.Errorf("a PIN from another site past its limit: %d, want 429", status)
	}
	for i := range limit {
		if status := signIn("same-origin", "wrong horse"); status != http.StatusUnauthorized {
			t.Fatalf("the client's own wrong password %d: %d, want 401", i+1, status)
		}
	}
	if status := signIn("same-origin", "correct horse"); status != http.StatusTooManyRequests {
		t.Errorf("the client's own sign-in past its own limit: %d, want 429", status)
	}
}

// Wrong passwords that a page of another site had the browser send count
// toward the block of the account as any other: the one that makes the
// limit blocks the client's address for the account, which its own
// sign-in then meets (401, not the 429 of the client's limit), except
// from a device the account used.
func TestOtherSitesWrongPasswordsStillCountTowardTheBlock(t *testing.T) {
	t.Parallel()
	const limit = 3
	s := newTestServer(t, limit)
	alice := s.user("alice", nil)
	s.setting(t, func(settings *accounts.Settings) { settings.LoginAttempts = limit })
	call := fromSite(t, s)
	signIn := func(site, device, password string) int {
		t.Helper()
		return call(site, http.MethodPost, "/Users/AuthenticateByName", app(device, ""), map[string]string{"Username": "alice", "Pw": password})
	}
	if status := signIn("", "alice-tv", "correct horse"); status != http.StatusOK {
		t.Fatalf("alice's first sign-in: %d", status)
	}
	for i := range limit {
		if status := signIn("cross-site", "browser", "wrong horse"); status != http.StatusUnauthorized {
			t.Fatalf("wrong password %d from another site: %d, want 401", i+1, status)
		}
	}
	if user, err := s.store.User(t.Context(), alice.ID); err != nil || user.InvalidLoginAttempts != limit || user.BlockedUntil == nil {
		t.Fatalf("alice shows %d attempts, blocked until %v, %v", user.InvalidLoginAttempts, user.BlockedUntil, err)
	}
	if status := signIn("same-origin", "browser", "correct horse"); status != http.StatusUnauthorized {
		t.Errorf("the right password from the blocked address, on a new device: %d, want 401", status)
	}
	if status := signIn("", "alice-tv", "correct horse"); status != http.StatusOK {
		t.Errorf("the right password from the blocked address, on a device the account used: %d, want 200", status)
	}
}
