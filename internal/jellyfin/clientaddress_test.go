package jellyfin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/forwarded"
)

// through calls s as Polyfin serves it, through forwarded, over a
// connection from the address connection, with the headers given: what a
// reverse proxy at that address sends, or a client with no proxy.
func through(t *testing.T, s testServer, connection string) func(headers map[string]string, path, authorization string, body any) int {
	t.Helper()
	handler := forwarded.New(s.store.KnownProxies, nil).Handler(s.handler)
	return func(headers map[string]string, path, authorization string, body any) int {
		t.Helper()
		encoded, _ := json.Marshal(body)
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, bytes.NewReader(encoded))
		request.RemoteAddr = connection
		request.Header.Set("Content-Type", "application/json")
		if authorization != "" {
			request.Header.Set("Authorization", authorization)
		}
		for name, value := range headers {
			request.Header.Set(name, value)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder.Code
	}
}

// resetPINs counts the password reset PINs still valid.
func resetPINs(t *testing.T, s testServer) int {
	t.Helper()
	pins, err := s.store.PasswordResetPINs(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return len(pins)
}

// A reverse proxy on the local network that is missing from Known proxies
// does not make its clients local: through it, an account without remote
// access is refused and a forgotten password makes no PIN, whichever
// forwarded header the proxy sends and whatever address it names. The same
// address without forwarded headers is a client of the local network.
func TestRequestsThroughAnUnlistedProxyAreOutsideTheLocalNetwork(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 100)
	if known := s.store.Settings().KnownProxies; len(known) != 0 {
		t.Fatalf("known proxies by default: %v", known)
	}
	s.user("homebody", func(c *accounts.UserChanges) { c.RemoteAccess = new(false) })
	call := through(t, s, "172.18.0.1:41000")
	signIn := map[string]string{"Username": "homebody", "Pw": "correct horse"}
	forgot := map[string]string{"EnteredUsername": "homebody"}

	for _, headers := range []map[string]string{
		{"X-Forwarded-For": "203.0.113.9", "X-Forwarded-Proto": "https"},
		{"X-Forwarded-For": "192.168.1.50"},
		{"X-Real-IP": "192.168.1.50"},
		{"Forwarded": "for=192.168.1.50"},
	} {
		if status := call(headers, "/Users/AuthenticateByName", app("laptop", ""), signIn); status != http.StatusForbidden {
			t.Errorf("%v: sign-in without remote access %d, want 403", headers, status)
		}
		if status := call(headers, "/Users/ForgotPassword", "", forgot); status != http.StatusOK {
			t.Errorf("%v: forgotten password %d", headers, status)
		}
		if made := resetPINs(t, s); made != 0 {
			t.Fatalf("%v: %d PINs made through the proxy", headers, made)
		}
	}

	if status := call(nil, "/Users/AuthenticateByName", app("laptop", ""), signIn); status != http.StatusOK {
		t.Errorf("the local network, no forwarded header: sign-in %d, want 200", status)
	}
	call(nil, "/Users/ForgotPassword", "", forgot)
	if made := resetPINs(t, s); made != 1 {
		t.Errorf("the local network, no forwarded header: %d PINs, want 1", made)
	}
}

// Behind a known proxy that names its client in X-Real-IP alone, an
// X-Forwarded-For the client wrote itself names nobody: the request is
// not local, so an account without remote access stays refused and no PIN
// is made, and the failed sign-ins of such requests count together, at
// the proxy's address, whatever address each one names.
func TestAClientsOwnForwardedForNamesNoClientBehindAKnownProxy(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 2)
	knowProxies(t, s, "172.30.0.2/32")
	s.user("homebody", func(c *accounts.UserChanges) { c.RemoteAccess = new(false) })
	s.user("alice", nil)
	call := through(t, s, "172.30.0.2:41000")
	signIn := map[string]string{"Username": "homebody", "Pw": "correct horse"}
	forgot := map[string]string{"EnteredUsername": "homebody"}

	for _, headers := range []map[string]string{
		{"X-Real-IP": "203.0.113.9"},
		{"X-Real-IP": "203.0.113.9", "X-Forwarded-For": "192.168.1.50"},
		{"X-Real-IP": "203.0.113.9", "X-Forwarded-For": "unknown"},
	} {
		if status := call(headers, "/Users/ForgotPassword", "", forgot); status != http.StatusOK {
			t.Errorf("%v: forgotten password %d", headers, status)
		}
		if made := resetPINs(t, s); made != 0 {
			t.Errorf("%v: %d PINs made from the internet", headers, made)
		}
	}
	if status := call(map[string]string{"X-Real-IP": "203.0.113.9"}, "/Users/AuthenticateByName", app("laptop", ""), signIn); status != http.StatusForbidden {
		t.Errorf("from the internet without remote access: %d, want 403", status)
	}
	if status := call(map[string]string{"X-Real-IP": "203.0.113.10", "X-Forwarded-For": "192.168.1.50"}, "/Users/AuthenticateByName", app("laptop", ""),
		signIn); status != http.StatusForbidden {
		t.Errorf("with an X-Forwarded-For of the client's own: %d, want 403", status)
	}

	// Wrong passwords, each with another address in X-Forwarded-For: the
	// limit of 2 failed sign-ins holds, since all count at one address.
	wrong := map[string]string{"Username": "alice", "Pw": "wrong horse"}
	for guess := 1; guess <= 8; guess++ {
		headers := map[string]string{"X-Real-IP": "203.0.113.11", "X-Forwarded-For": "198.51.100." + strconv.Itoa(guess)}
		status := call(headers, "/Users/AuthenticateByName", app("laptop", ""), wrong)
		if guess > 2 && status != http.StatusTooManyRequests {
			t.Fatalf("guess %d: %d, want 429", guess, status)
		}
	}
	// A client the proxy alone names keeps its own count.
	if status := call(map[string]string{"X-Real-IP": "203.0.113.12"}, "/Users/AuthenticateByName", app("laptop", ""),
		map[string]string{"Username": "alice", "Pw": "correct horse"}); status != http.StatusOK {
		t.Errorf("a client named by X-Real-IP alone: %d, want 200", status)
	}
}
