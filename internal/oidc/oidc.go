// Package oidc signs people in with an OpenID Connect provider, with the
// standard library alone: the provider's discovery document and keys
// (JWKS), the authorization code flow with PKCE, a state and a nonce, and
// the checks of the ID token, signed with RS256 or ES256 (OpenID Connect
// Core 1.0, sections 3.1 and 3.1.3.7).
//
// Nothing here logs, and no error holds a token, a code or a secret: an
// error tells which check failed, never what was checked.
package oidc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	// Skew is the difference between Polyfin's clock and the provider's
	// that the ID token's times allow.
	Skew = 2 * time.Minute
	// maxAnswer bounds what the provider's answers are read up to.
	maxAnswer = 1 << 20
	// metadataLife is how long the discovery document is kept.
	metadataLife = time.Hour
	// keysAgain is how long after the keys were fetched a token signed
	// with a key they lack fetches them again: a provider rotating its
	// keys is followed at once, and tokens naming made-up keys cannot have
	// Polyfin ask the provider at every attempt.
	keysAgain = 30 * time.Second
)

var (
	// ErrDiscovery reports a provider whose discovery document or keys
	// could not be read, or do not describe the issuer configured.
	ErrDiscovery = errors.New("the provider's discovery failed")
	// ErrExchange reports a code the provider's token endpoint refused, or
	// an answer without an ID token.
	ErrExchange = errors.New("the provider refused the code")
	// ErrToken reports an ID token refused: its form, its signature, or
	// one of its claims. Wrapped errors tell which.
	ErrToken = errors.New("invalid ID token")
)

// Config is the provider and Polyfin's client at it.
type Config struct {
	// Issuer is the provider's issuer identifier, exactly as its discovery
	// document and its ID tokens give it.
	Issuer string
	// ClientID and ClientSecret are Polyfin's client at the provider; a
	// public client has no secret, PKCE standing for it.
	ClientID     string
	ClientSecret string
	// RedirectURI is where the provider sends the browser back, exactly as
	// registered at the provider.
	RedirectURI string
	// Groups asks for the groups scope too, when the provider lists it.
	Groups bool
}

// Flow is what a sign-in keeps between leaving for the provider and coming
// back: the state that ties the return to its start, the nonce the ID
// token must hold, and the PKCE code verifier.
type Flow struct {
	State    string
	Nonce    string
	Verifier string
}

// NewFlow makes a flow of fresh random values.
func NewFlow() Flow {
	return Flow{State: random(), Nonce: random(), Verifier: random()}
}

// random is 32 random bytes in base64url: 43 characters, a valid PKCE
// verifier (RFC 7636, section 4.1).
func random() string {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	return base64.RawURLEncoding.EncodeToString(raw)
}

// challenge is the S256 code challenge of verifier (RFC 7636, section 4.2).
func challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// metadata is what Polyfin reads of the discovery document (OpenID Connect
// Discovery 1.0, section 3).
type metadata struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	JWKSURI               string   `json:"jwks_uri"`
	ScopesSupported       []string `json:"scopes_supported"`
	TokenAuthMethods      []string `json:"token_endpoint_auth_methods_supported"`
}

// provider is what Polyfin keeps of the provider: its discovery document
// and its keys.
type provider struct {
	issuer   string
	metadata metadata
	fetched  time.Time
	keys     []key
	keysAt   time.Time
}

// Client follows one provider at a time, the one configured last, and
// keeps its discovery document and keys.
type Client struct {
	http *http.Client
	now  func() time.Time
	mu   sync.Mutex
	// current is the provider of the last issuer asked; its fields are
	// read and changed under mu.
	current *provider
}

// New returns a client that reaches the provider with client and tells
// the time with now.
func New(client *http.Client, now func() time.Time) *Client {
	return &Client{http: client, now: now}
}

// AuthorizationURL is the provider's page the browser goes to: the
// authorization code flow with PKCE (S256), flow's state and nonce, and
// the scopes openid, profile and email, those two when the provider lists
// them or lists no scope, and groups when cfg asks for it and the provider
// lists it.
func (c *Client) AuthorizationURL(ctx context.Context, cfg Config, flow Flow) (string, error) {
	m, err := c.discover(ctx, cfg.Issuer)
	if err != nil {
		return "", err
	}
	scopes := []string{"openid"}
	for _, scope := range []string{"profile", "email"} {
		if len(m.ScopesSupported) == 0 || slices.Contains(m.ScopesSupported, scope) {
			scopes = append(scopes, scope)
		}
	}
	if cfg.Groups && slices.Contains(m.ScopesSupported, "groups") {
		scopes = append(scopes, "groups")
	}
	address, err := url.Parse(m.AuthorizationEndpoint)
	if err != nil {
		return "", ErrDiscovery
	}
	query := address.Query()
	query.Set("response_type", "code")
	query.Set("client_id", cfg.ClientID)
	query.Set("redirect_uri", cfg.RedirectURI)
	query.Set("scope", strings.Join(scopes, " "))
	query.Set("state", flow.State)
	query.Set("nonce", flow.Nonce)
	query.Set("code_challenge", challenge(flow.Verifier))
	query.Set("code_challenge_method", "S256")
	address.RawQuery = query.Encode()
	return address.String(), nil
}

// SignIn exchanges the code the provider gave the browser for an ID token,
// with flow's code verifier, and returns the token's claims once it passes
// every check (see Verify).
func (c *Client) SignIn(ctx context.Context, cfg Config, code string, flow Flow) (Claims, error) {
	m, err := c.discover(ctx, cfg.Issuer)
	if err != nil {
		return nil, err
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {cfg.RedirectURI},
		"code_verifier": {flow.Verifier},
		"client_id":     {cfg.ClientID},
	}
	// client_secret_basic is the default method (OpenID Connect Discovery
	// 1.0, token_endpoint_auth_methods_supported); a provider that lists
	// only client_secret_post gets the secret in the form.
	post := cfg.ClientSecret != "" && len(m.TokenAuthMethods) > 0 &&
		!slices.Contains(m.TokenAuthMethods, "client_secret_basic") && slices.Contains(m.TokenAuthMethods, "client_secret_post")
	if post {
		form.Set("client_secret", cfg.ClientSecret)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, m.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, ErrDiscovery
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	if cfg.ClientSecret != "" && !post {
		// RFC 6749, section 2.3.1: both form-encoded before the encoding.
		request.SetBasicAuth(url.QueryEscape(cfg.ClientID), url.QueryEscape(cfg.ClientSecret))
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: the token endpoint did not answer", ErrExchange)
	}
	defer response.Body.Close()
	var answer struct {
		IDToken string `json:"id_token"`
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: the token endpoint answered %d", ErrExchange, response.StatusCode)
	}
	if json.NewDecoder(io.LimitReader(response.Body, maxAnswer)).Decode(&answer) != nil || answer.IDToken == "" {
		return nil, fmt.Errorf("%w: no ID token", ErrExchange)
	}
	return c.Verify(ctx, cfg, answer.IDToken, flow.Nonce)
}

// discover returns the provider's discovery document, fetched again once
// metadataLife old. The document must name the issuer configured,
// exactly, and give its endpoints.
func (c *Client) discover(ctx context.Context, issuer string) (metadata, error) {
	c.mu.Lock()
	p := c.current
	if p != nil && p.issuer == issuer && c.now().Sub(p.fetched) < metadataLife {
		m := p.metadata
		c.mu.Unlock()
		return m, nil
	}
	c.mu.Unlock()
	var m metadata
	if err := c.get(ctx, strings.TrimSuffix(issuer, "/")+"/.well-known/openid-configuration", &m); err != nil {
		return metadata{}, err
	}
	if m.Issuer != issuer || !web(m.AuthorizationEndpoint) || !web(m.TokenEndpoint) || !web(m.JWKSURI) {
		return metadata{}, ErrDiscovery
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current == nil || c.current.issuer != issuer || c.current.metadata.JWKSURI != m.JWKSURI {
		c.current = &provider{issuer: issuer}
	}
	c.current.metadata, c.current.fetched = m, c.now()
	return m, nil
}

// web reports whether address is an http or https address with a host.
func web(address string) bool {
	parsed, err := url.Parse(address)
	return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != ""
}

// get reads the JSON document at address into into.
func (c *Client) get(ctx context.Context, address string, into any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return ErrDiscovery
	}
	request.Header.Set("Accept", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return ErrDiscovery
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ErrDiscovery
	}
	if json.NewDecoder(io.LimitReader(response.Body, maxAnswer)).Decode(into) != nil {
		return ErrDiscovery
	}
	return nil
}
