package oidc_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/oidc"
	"github.com/moodiness/polyfin/internal/oidc/oidctest"
)

const redirect = "https://polyfin.example/admin/api/provider/callback"

// clock is a time a test moves.
type clock struct {
	mu sync.Mutex
	at time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(d)
}

func setup(t *testing.T) (*oidctest.Provider, *oidc.Client, oidc.Config, *clock) {
	t.Helper()
	provider := oidctest.New(t, "polyfin", "client secret")
	provider.SignInAs("subject-1", map[string]any{"preferred_username": "alice", "groups": []string{"media-admins"}})
	at := &clock{at: time.Now()}
	return provider, oidc.New(http.DefaultClient, at.now), oidc.Config{Issuer: provider.URL, ClientID: "polyfin",
		ClientSecret: "client secret", RedirectURI: redirect, Groups: true}, at
}

// signIn goes through the flow as a browser does: to the provider's page,
// which sends it back with a code and the state, then exchanges the code.
func signIn(t *testing.T, client *oidc.Client, cfg oidc.Config) (oidc.Claims, error) {
	t.Helper()
	flow := oidc.NewFlow()
	address, err := client.AuthorizationURL(t.Context(), cfg, flow)
	if err != nil {
		t.Fatal(err)
	}
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := noRedirect.Get(address)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	back, err := url.Parse(response.Header.Get("Location"))
	if err != nil || response.StatusCode != http.StatusFound {
		t.Fatalf("the provider answered %d", response.StatusCode)
	}
	if !strings.HasPrefix(back.String(), redirect+"?") || back.Query().Get("state") != flow.State {
		t.Fatalf("sent back to %s", back.Redacted())
	}
	return client.SignIn(t.Context(), cfg, back.Query().Get("code"), flow)
}

// The authorization code flow with PKCE gives the claims of the person
// signed in, with tokens signed with RS256 as with ES256; the provider's
// page gets the flow's state, nonce and S256 challenge, and the scopes it
// lists.
func TestSignInGivesTheClaims(t *testing.T) {
	t.Parallel()
	for _, alg := range []string{"RS256", "ES256"} {
		provider, client, cfg, _ := setup(t)
		provider.Use(alg)
		claims, err := signIn(t, client, cfg)
		if err != nil {
			t.Fatalf("%s: %v", alg, err)
		}
		if claims.Subject() != "subject-1" || claims.String("preferred_username") != "alice" || !claims.Has("groups", "media-admins") {
			t.Errorf("%s: claims %v", alg, claims)
		}
	}
	_, client, cfg, _ := setup(t)
	flow := oidc.NewFlow()
	address, err := client.AuthorizationURL(t.Context(), cfg, flow)
	if err != nil {
		t.Fatal(err)
	}
	query := must(url.Parse(address)).Query()
	if query.Get("state") != flow.State || query.Get("nonce") != flow.Nonce || query.Get("code_challenge_method") != "S256" ||
		query.Get("code_challenge") == "" || query.Get("code_challenge") == flow.Verifier || query.Get("redirect_uri") != redirect ||
		query.Get("scope") != "openid profile email groups" {
		t.Errorf("authorization request %v", query)
	}
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

// Every check of the ID token refuses a token that fails it: a signature
// of a key the provider never published, another audience, another
// issuer, an expiry past, an issue time to come, both beyond the skew,
// another nonce, or an algorithm other than RS256 and ES256.
func TestIDTokensFailingACheckAreRefused(t *testing.T) {
	t.Parallel()
	for name, spoil := range map[string]func(*oidctest.Provider){
		"forged signature":     func(p *oidctest.Provider) { p.Forge() },
		"forged ES256":         func(p *oidctest.Provider) { p.Use("ES256"); p.Forge() },
		"wrong audience":       edit(func(c map[string]any) { c["aud"] = "another-client" }),
		"audience among more":  edit(func(c map[string]any) { c["aud"] = []string{"another-client", "polyfin"} }),
		"another party":        edit(func(c map[string]any) { c["azp"] = "another-client" }),
		"wrong issuer":         edit(func(c map[string]any) { c["iss"] = "https://elsewhere.example" }),
		"expired":              edit(func(c map[string]any) { c["exp"] = time.Now().Add(-oidc.Skew - time.Minute).Unix() }),
		"no expiry":            edit(func(c map[string]any) { delete(c, "exp") }),
		"issued in the future": edit(func(c map[string]any) { c["iat"] = time.Now().Add(oidc.Skew + time.Minute).Unix() }),
		"no issue time":        edit(func(c map[string]any) { delete(c, "iat") }),
		"another nonce":        edit(func(c map[string]any) { c["nonce"] = "another" }),
		"no subject":           edit(func(c map[string]any) { delete(c, "sub") }),
	} {
		provider, client, cfg, _ := setup(t)
		spoil(provider)
		if _, err := signIn(t, client, cfg); !errors.Is(err, oidc.ErrToken) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// Within the skew, both times pass.
	provider, client, cfg, _ := setup(t)
	provider.Edit(func(c map[string]any) {
		c["exp"] = time.Now().Add(-oidc.Skew + time.Minute).Unix()
		c["iat"] = time.Now().Add(oidc.Skew - time.Minute).Unix()
	})
	if _, err := signIn(t, client, cfg); err != nil {
		t.Errorf("times within the skew: %v", err)
	}
	// Unsigned and HMAC tokens are refused whatever they hold.
	encode := base64.RawURLEncoding.EncodeToString
	payload := encode([]byte(`{"iss":"` + provider.URL + `","aud":"polyfin","sub":"x","nonce":"n","exp":9999999999,"iat":1}`))
	for _, alg := range []string{"none", "HS256"} {
		token := encode([]byte(`{"alg":"`+alg+`"}`)) + "." + payload + "." + encode([]byte("signature"))
		if _, err := client.Verify(t.Context(), cfg, token, "n"); !errors.Is(err, oidc.ErrToken) {
			t.Errorf("%s: %v", alg, err)
		}
	}
}

func edit(change func(map[string]any)) func(*oidctest.Provider) {
	return func(p *oidctest.Provider) { p.Edit(change) }
}

// The keys are fetched once and kept; a token naming a key they lack has
// them fetched again, as when the provider rotates its keys, but not
// within 30 seconds of the last fetch, so that made-up key IDs do not
// have Polyfin ask the provider at every attempt.
func TestKeysAreKeptAndFetchedAgainForAnUnknownKey(t *testing.T) {
	t.Parallel()
	provider, client, cfg, at := setup(t)
	for range 2 {
		if _, err := signIn(t, client, cfg); err != nil {
			t.Fatal(err)
		}
	}
	if provider.KeySets() != 1 {
		t.Errorf("keys fetched %d times for two sign-ins", provider.KeySets())
	}
	provider.Rotate()
	if _, err := signIn(t, client, cfg); !errors.Is(err, oidc.ErrToken) || provider.KeySets() != 1 {
		t.Errorf("a new key right after a fetch: %v, %d fetches", err, provider.KeySets())
	}
	at.add(31 * time.Second)
	if _, err := signIn(t, client, cfg); err != nil || provider.KeySets() != 2 {
		t.Errorf("a new key later: %v, %d fetches", err, provider.KeySets())
	}
}

// A discovery document naming another issuer is refused.
func TestDiscoveryMustNameTheIssuer(t *testing.T) {
	t.Parallel()
	_, client, cfg, _ := setup(t)
	cfg.Issuer += "/"
	if _, err := client.AuthorizationURL(context.Background(), cfg, oidc.NewFlow()); !errors.Is(err, oidc.ErrDiscovery) {
		t.Errorf("another issuer: %v", err)
	}
}
