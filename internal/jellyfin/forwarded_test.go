package jellyfin

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/forwarded"
)

// behindProxy serves s as Polyfin serves it, through forwarded, and
// returns a call through it from the client at address, as a reverse
// proxy on 127.0.0.1 forwards it.
func behindProxy(t *testing.T, s testServer) func(client, method, path, authorization string, body any) (int, []byte) {
	t.Helper()
	server := httptest.NewServer(forwarded.New(s.store.KnownProxies).Handler(s.handler))
	t.Cleanup(server.Close)
	return func(client, method, path, authorization string, body any) (int, []byte) {
		t.Helper()
		var reader io.Reader
		if body != nil {
			encoded, _ := json.Marshal(body)
			reader = bytes.NewReader(encoded)
		}
		request, _ := http.NewRequestWithContext(t.Context(), method, server.URL+path, reader)
		request.Header.Set("X-Forwarded-For", client)
		if authorization != "" {
			request.Header.Set("Authorization", authorization)
		}
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		payload, _ := io.ReadAll(response.Body)
		return response.StatusCode, payload
	}
}

func knowProxies(t *testing.T, s testServer, networks ...string) {
	t.Helper()
	s.setting(t, func(settings *accounts.Settings) {
		settings.KnownProxies = nil
		for _, network := range networks {
			settings.KnownProxies = append(settings.KnownProxies, netip.MustParsePrefix(network))
		}
	})
}

// Behind a proxy that is not known, every client shares its failed
// sign-ins; once known, each client has its own, and Sessions shows each
// device's address.
func TestSignInsBehindAKnownProxy(t *testing.T) {
	s := newTestServer(t, 10)
	s.user("root", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	s.user("alice", nil)
	call := behindProxy(t, s)
	wrong := map[string]string{"Username": "alice", "Pw": "wrong horse"}
	right := map[string]string{"Username": "alice", "Pw": "correct horse"}
	signIn := func(client, device string, body map[string]string) (int, string) {
		t.Helper()
		status, payload := call(client, http.MethodPost, "/Users/AuthenticateByName", app(device, ""), body)
		var result AuthenticationResult
		_ = json.Unmarshal(payload, &result)
		return status, result.AccessToken
	}

	// No proxy known: client A's 10 failures refuse client B.
	for range 10 {
		signIn("203.0.113.1", "tv-a", wrong)
	}
	if status, _ := signIn("198.51.100.2", "tv-b", right); status != http.StatusTooManyRequests {
		t.Errorf("client B behind an unknown proxy: %d, want 429", status)
	}

	// 127.0.0.1 known: client C's failures refuse C alone.
	knowProxies(t, s, "127.0.0.1/32")
	for range 10 {
		signIn("203.0.113.3", "tv-c", wrong)
	}
	if status, _ := signIn("203.0.113.3", "tv-c", right); status != http.StatusTooManyRequests {
		t.Errorf("client C after its failures: %d, want 429", status)
	}
	status, alice := signIn("198.51.100.4", "tv-d", right)
	if status != http.StatusOK {
		t.Fatalf("client D: %d", status)
	}
	_, root := signIn("2001:db8::5", "dashboard", map[string]string{"Username": "root", "Pw": "correct horse"})
	// A request of alice's from elsewhere moves her device there.
	if status, _ := call("192.0.2.6", http.MethodGet, "/Users/Me", app("tv-d", alice), nil); status != http.StatusOK {
		t.Fatalf("/Users/Me: %d", status)
	}
	status, payload := call("2001:db8::5", http.MethodGet, "/Sessions", app("dashboard", root), nil)
	var sessions []struct{ DeviceName, RemoteEndPoint string }
	if status != http.StatusOK || json.Unmarshal(payload, &sessions) != nil {
		t.Fatalf("/Sessions: %d %s", status, payload)
	}
	addresses := map[string]string{}
	for _, session := range sessions {
		addresses[session.DeviceName] = session.RemoteEndPoint
	}
	if addresses["tv-d"] != "192.0.2.6" || addresses["dashboard"] != "2001:db8::5" {
		t.Errorf("addresses: %v", addresses)
	}
}

// Behind a proxy in the local network, a client on the internet is outside
// it once the proxy is known: /System/Endpoint says so, and a forgotten
// password makes no PIN.
func TestLocalNetworkBehindAKnownProxy(t *testing.T) {
	s, _, member := administrated(t)
	call := behindProxy(t, s)
	endpoint := func(client string) string {
		t.Helper()
		_, payload := call(client, http.MethodGet, "/System/Endpoint", member, nil)
		return strings.TrimSpace(string(payload))
	}
	pins := func() int {
		var count int
		_ = s.pool.QueryRow(t.Context(), "SELECT count(*) FROM password_reset_pins").Scan(&count)
		return count
	}
	forgot := func(client string) {
		t.Helper()
		if status, payload := call(client, http.MethodPost, "/Users/ForgotPassword", "", map[string]string{"EnteredUsername": "alice"}); status != http.StatusOK ||
			!strings.Contains(string(payload), `"Action":"PinCode"`) {
			t.Fatalf("forgot password: %d %s", status, payload)
		}
	}

	// The proxy unknown, every client is on the server's machine: the
	// problem.
	if got := endpoint("203.0.113.7"); got != `{"IsLocal":true,"IsInNetwork":true}` {
		t.Errorf("behind an unknown proxy: %s", got)
	}
	knowProxies(t, s, "127.0.0.0/8")
	if got := endpoint("203.0.113.7"); got != `{"IsLocal":false,"IsInNetwork":false}` {
		t.Errorf("a client on the internet: %s", got)
	}
	if got := endpoint("192.168.1.20"); got != `{"IsLocal":false,"IsInNetwork":true}` {
		t.Errorf("a client in the local network: %s", got)
	}
	forgot("203.0.113.7")
	if pins() != 0 {
		t.Fatal("a PIN was made for a client on the internet")
	}
	forgot("192.168.1.20")
	if pins() != 1 {
		t.Errorf("%d PINs for a client in the local network", pins())
	}
}
