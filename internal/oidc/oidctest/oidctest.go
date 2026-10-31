// Package oidctest is an OpenID Connect provider for tests: an httptest
// issuer with its discovery document, its keys (JWKS), an authorization
// endpoint that signs in at once whoever SignInAs names, and a token
// endpoint that checks the client, the redirect URI and the PKCE
// verifier. Its keys are made for each provider.
package oidctest

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"
)

// Provider is a running provider. Its fields are set before it is used.
type Provider struct {
	// URL is the issuer.
	URL string
	// ClientID and ClientSecret are the client the token endpoint accepts,
	// the secret checked when not empty.
	ClientID     string
	ClientSecret string

	mu sync.Mutex
	// alg is the algorithm tokens are signed with, RS256 or ES256.
	alg     string
	rsaKey  *rsa.PrivateKey
	ecKey   *ecdsa.PrivateKey
	kid     string
	claims  map[string]any
	edit    func(claims map[string]any)
	forge   bool
	codes   map[string]grant
	keySets int
	tokens  int
}

// grant is a code the authorization endpoint gave.
type grant struct {
	claims      map[string]any
	nonce       string
	redirectURI string
	challenge   string
}

// New starts a provider whose client is clientID and secret, signing with
// RS256; it stops with the test.
func New(t testing.TB, clientID, secret string) *Provider {
	t.Helper()
	p := &Provider{ClientID: clientID, ClientSecret: secret, alg: "RS256", codes: map[string]grant{}}
	p.newKeys()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", p.discovery)
	mux.HandleFunc("GET /jwks", p.jwks)
	mux.HandleFunc("GET /authorize", p.authorize)
	mux.HandleFunc("POST /token", p.token)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	p.URL = server.URL
	return p
}

func (p *Provider) newKeys() {
	var err error
	if p.rsaKey, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
		panic(err)
	}
	if p.ecKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader); err != nil {
		panic(err)
	}
	p.kid = random()
}

func random() string {
	raw := make([]byte, 12)
	_, _ = rand.Read(raw)
	return base64.RawURLEncoding.EncodeToString(raw)
}

// SignInAs has the authorization endpoint sign in someone with claims,
// sub, and the claims the provider adds to any token: iss, aud, exp, iat
// and nonce.
func (p *Provider) SignInAs(sub string, claims map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.claims = map[string]any{"sub": sub}
	for name, value := range claims {
		p.claims[name] = value
	}
}

// Use has tokens signed with alg, RS256 or ES256.
func (p *Provider) Use(alg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.alg = alg
}

// Edit has edit change the claims of the tokens given from now on, before
// they are signed.
func (p *Provider) Edit(edit func(claims map[string]any)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.edit = edit
}

// Forge has tokens signed with keys the provider never published, under
// the published key ID.
func (p *Provider) Forge() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.forge = true
}

// Rotate replaces the provider's keys, under a new key ID.
func (p *Provider) Rotate() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.newKeys()
}

// KeySets is how many times the keys were fetched.
func (p *Provider) KeySets() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.keySets
}

// Tokens is how many ID tokens the token endpoint gave.
func (p *Provider) Tokens() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tokens
}

func (p *Provider) discovery(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                p.URL,
		"authorization_endpoint":                p.URL + "/authorize",
		"token_endpoint":                        p.URL + "/token",
		"jwks_uri":                              p.URL + "/jwks",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256", "ES256"},
		"scopes_supported":                      []string{"openid", "profile", "email", "groups"},
	})
}

func (p *Provider) jwks(w http.ResponseWriter, _ *http.Request) {
	p.mu.Lock()
	p.keySets++
	public := p.rsaKey.PublicKey
	point := p.ecKey.PublicKey
	kid := p.kid
	p.mu.Unlock()
	encode := base64.RawURLEncoding.EncodeToString
	ecBytes, _ := point.Bytes()
	writeJSON(w, http.StatusOK, map[string]any{"keys": []map[string]any{
		{"kty": "RSA", "kid": kid, "use": "sig", "alg": "RS256", "n": encode(public.N.Bytes()),
			"e": encode(big.NewInt(int64(public.E)).Bytes())},
		{"kty": "EC", "kid": kid, "use": "sig", "alg": "ES256", "crv": "P-256", "x": encode(ecBytes[1:33]), "y": encode(ecBytes[33:])},
	}})
}

// authorize signs in at once whoever SignInAs named, and sends the
// browser back with a code; it needs PKCE with S256.
func (p *Provider) authorize(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if query.Get("response_type") != "code" || query.Get("client_id") != p.ClientID || query.Get("code_challenge_method") != "S256" ||
		query.Get("code_challenge") == "" || query.Get("redirect_uri") == "" {
		http.Error(w, "bad authorization request", http.StatusBadRequest)
		return
	}
	p.mu.Lock()
	code := random()
	p.codes[code] = grant{claims: p.claims, nonce: query.Get("nonce"), redirectURI: query.Get("redirect_uri"), challenge: query.Get("code_challenge")}
	p.mu.Unlock()
	back, err := url.Parse(query.Get("redirect_uri"))
	if err != nil {
		http.Error(w, "bad redirect URI", http.StatusBadRequest)
		return
	}
	values := back.Query()
	values.Set("code", code)
	values.Set("state", query.Get("state"))
	back.RawQuery = values.Encode()
	http.Redirect(w, r, back.String(), http.StatusFound)
}

// token exchanges a code once, for the client that asked for it, with its
// redirect URI and its PKCE verifier.
func (p *Provider) token(w http.ResponseWriter, r *http.Request) {
	if r.ParseForm() != nil || r.PostForm.Get("grant_type") != "authorization_code" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	id, secret, basic := r.BasicAuth()
	if basic {
		id, _ = url.QueryUnescape(id)
		secret, _ = url.QueryUnescape(secret)
	} else {
		id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	if id != p.ClientID || (p.ClientSecret != "" && secret != p.ClientSecret) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_client"})
		return
	}
	p.mu.Lock()
	code := r.PostForm.Get("code")
	g, ok := p.codes[code]
	delete(p.codes, code)
	p.mu.Unlock()
	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	if !ok || g.redirectURI != r.PostForm.Get("redirect_uri") || base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}
	now := time.Now()
	claims := map[string]any{"iss": p.URL, "aud": p.ClientID, "exp": now.Add(5 * time.Minute).Unix(), "iat": now.Unix(), "nonce": g.nonce}
	for name, value := range g.claims {
		claims[name] = value
	}
	p.mu.Lock()
	edit := p.edit
	p.tokens++
	p.mu.Unlock()
	if edit != nil {
		edit(claims)
	}
	writeJSON(w, http.StatusOK, map[string]any{"access_token": random(), "token_type": "Bearer", "expires_in": 300,
		"id_token": p.Sign(claims)})
}

// Sign makes an ID token of claims, signed as the provider signs them now.
func (p *Provider) Sign(claims map[string]any) string {
	p.mu.Lock()
	alg, kid, forge := p.alg, p.kid, p.forge
	rsaKey, ecKey := p.rsaKey, p.ecKey
	p.mu.Unlock()
	if forge {
		rsaKey, _ = rsa.GenerateKey(rand.Reader, 2048)
		ecKey, _ = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	}
	header, _ := json.Marshal(map[string]string{"alg": alg, "kid": kid, "typ": "JWT"})
	payload, _ := json.Marshal(claims)
	encode := base64.RawURLEncoding.EncodeToString
	signed := encode(header) + "." + encode(payload)
	digest := sha256.Sum256([]byte(signed))
	var signature []byte
	switch alg {
	case "ES256":
		r, s, err := ecdsa.Sign(rand.Reader, ecKey, digest[:])
		if err != nil {
			panic(err)
		}
		signature = make([]byte, 64)
		r.FillBytes(signature[:32])
		s.FillBytes(signature[32:])
	default:
		var err error
		if signature, err = rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, digest[:]); err != nil {
			panic(err)
		}
	}
	return signed + "." + encode(signature)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(mustJSON(value))))
	w.WriteHeader(status)
	_, _ = w.Write(mustJSON(value))
}

func mustJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}
