package oidc_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/oidc"
	rule "github.com/moodiness/polyfin/internal/redirect"
)

// A token endpoint that answers with a 307 to another host, here the same
// server named localhost rather than 127.0.0.1, is not sent the form again
// there, with the code, the verifier and the client secret: the sign-in
// fails instead.
func TestTheTokenRequestIsNotSentAgainToAnotherHost(t *testing.T) {
	t.Parallel()
	var (
		mu     sync.Mutex
		bodies []string
	)
	var issuer, elsewhere string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case !strings.HasPrefix(r.Host, "127.0.0.1:"):
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			bodies = append(bodies, string(body))
			mu.Unlock()
			_, _ = io.WriteString(w, `{}`)
		case r.URL.Path == "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize",
				"token_endpoint": issuer + "/token", "jwks_uri": issuer + "/jwks", "token_endpoint_auth_methods_supported": []string{"client_secret_post"}})
		default:
			w.Header().Set("Location", elsewhere+r.URL.RequestURI())
			w.WriteHeader(http.StatusTemporaryRedirect)
		}
	}))
	t.Cleanup(server.Close)
	issuer, elsewhere = server.URL, strings.Replace(server.URL, "127.0.0.1", "localhost", 1)
	sender := &http.Client{Transport: &http.Transport{}, CheckRedirect: rule.Follow(rule.Default)}
	t.Cleanup(sender.CloseIdleConnections)
	client := oidc.New(sender, time.Now)

	cfg := oidc.Config{Issuer: issuer, ClientID: "polyfin", ClientSecret: "client secret", RedirectURI: redirect}
	if _, err := client.SignIn(t.Context(), cfg, "the-code", oidc.NewFlow()); !errors.Is(err, oidc.ErrExchange) {
		t.Errorf("a token endpoint redirecting to another host: %v, want ErrExchange", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 0 {
		t.Errorf("the other host was sent %q", bodies)
	}
}
