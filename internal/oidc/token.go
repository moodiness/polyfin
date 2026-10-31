package oidc

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// The signing algorithms accepted: RS256, which every provider offers
// (OpenID Connect Core 1.0, section 15.1), and ES256.
const (
	rs256 = "RS256"
	es256 = "ES256"
)

// minRSABits is the smallest RSA key accepted.
const minRSABits = 2048

// key is one of the provider's signing keys, RSA or P-256.
type key struct {
	id  string
	alg string
	rsa *rsa.PublicKey
	ec  *ecdsa.PublicKey
}

// signs reports whether k may have signed a token with alg and the key ID
// kid: a token naming no key may be signed by any key of its kind.
func (k key) signs(alg, kid string) bool {
	if kid != "" && k.id != kid {
		return false
	}
	if k.alg != "" && k.alg != alg {
		return false
	}
	return (alg == rs256 && k.rsa != nil) || (alg == es256 && k.ec != nil)
}

// verifies reports whether signature signs signed under k with alg.
func (k key) verifies(alg string, signed, signature []byte) bool {
	digest := sha256.Sum256(signed)
	switch {
	case alg == rs256 && k.rsa != nil:
		return rsa.VerifyPKCS1v15(k.rsa, crypto.SHA256, digest[:], signature) == nil
	case alg == es256 && k.ec != nil:
		// JWS gives R and S, 32 bytes each (RFC 7518, section 3.4).
		if len(signature) != 64 {
			return false
		}
		r, s := new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])
		return ecdsa.Verify(k.ec, digest[:], r, s)
	}
	return false
}

// Verify checks an ID token (OpenID Connect Core 1.0, section 3.1.3.7):
// a JWS signed with RS256 or ES256 by one of the issuer's keys, fetched
// again when the token names a key they lack; issued by cfg.Issuer, for
// cfg.ClientID (and, holding other audiences or an azp, authorized for
// it); not expired and not issued in the future, Skew allowed either way;
// holding nonce and a subject. It returns the token's claims.
func (c *Client) Verify(ctx context.Context, cfg Config, raw, nonce string) (Claims, error) {
	if _, err := c.discover(ctx, cfg.Issuer); err != nil {
		return nil, err
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("%w: not a signed token", ErrToken)
	}
	var header struct {
		Alg  string   `json:"alg"`
		Kid  string   `json:"kid"`
		Crit []string `json:"crit"`
	}
	if decodePart(parts[0], &header) != nil {
		return nil, fmt.Errorf("%w: unreadable header", ErrToken)
	}
	if header.Alg != rs256 && header.Alg != es256 {
		return nil, fmt.Errorf("%w: unsupported algorithm", ErrToken)
	}
	if len(header.Crit) > 0 {
		return nil, fmt.Errorf("%w: critical extensions", ErrToken)
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("%w: unreadable signature", ErrToken)
	}
	keys, err := c.keysFor(ctx, cfg.Issuer, header.Alg, header.Kid)
	if err != nil {
		return nil, err
	}
	signed := []byte(parts[0] + "." + parts[1])
	verified := false
	for _, k := range keys {
		if k.verifies(header.Alg, signed, signature) {
			verified = true
			break
		}
	}
	if !verified {
		return nil, fmt.Errorf("%w: bad signature", ErrToken)
	}
	var claims Claims
	if decodePart(parts[1], &claims) != nil {
		return nil, fmt.Errorf("%w: unreadable claims", ErrToken)
	}
	if err := claims.check(cfg, nonce, c.now()); err != nil {
		return nil, err
	}
	return claims, nil
}

// decodePart reads a base64url part of a token as JSON, numbers kept as
// json.Number.
func decodePart(part string, into any) error {
	raw, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return decoder.Decode(into)
}

// keysFor returns the issuer's keys that may have signed a token with alg
// and kid. When none may, the keys are fetched again, unless they were
// fetched less than keysAgain ago.
func (c *Client) keysFor(ctx context.Context, issuer, alg, kid string) ([]key, error) {
	c.mu.Lock()
	p := c.current
	if p == nil || p.issuer != issuer {
		c.mu.Unlock()
		return nil, ErrDiscovery
	}
	found := matching(p.keys, alg, kid)
	recent := !p.keysAt.IsZero() && c.now().Sub(p.keysAt) < keysAgain
	address := p.metadata.JWKSURI
	c.mu.Unlock()
	if len(found) > 0 {
		return found, nil
	}
	if recent {
		return nil, fmt.Errorf("%w: unknown key", ErrToken)
	}
	var set struct {
		Keys []jwk `json:"keys"`
	}
	if err := c.get(ctx, address, &set); err != nil {
		return nil, err
	}
	keys := make([]key, 0, len(set.Keys))
	for _, entry := range set.Keys {
		if k, ok := entry.key(); ok {
			keys = append(keys, k)
		}
	}
	c.mu.Lock()
	if c.current == p {
		p.keys, p.keysAt = keys, c.now()
	}
	c.mu.Unlock()
	if found = matching(keys, alg, kid); len(found) == 0 {
		return nil, fmt.Errorf("%w: unknown key", ErrToken)
	}
	return found, nil
}

func matching(keys []key, alg, kid string) []key {
	var found []key
	for _, k := range keys {
		if k.signs(alg, kid) {
			found = append(found, k)
		}
	}
	return found
}

// jwk is a JSON Web Key (RFC 7517), RSA or EC.
type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

// key reads a signing key: an RSA key of minRSABits at least, or a P-256
// key. Keys for encryption, of other kinds or curves, or malformed, are
// left out.
func (j jwk) key() (key, bool) {
	if j.Use != "" && j.Use != "sig" {
		return key{}, false
	}
	k := key{id: j.Kid, alg: j.Alg}
	switch j.Kty {
	case "RSA":
		n, errN := base64.RawURLEncoding.DecodeString(j.N)
		e, errE := base64.RawURLEncoding.DecodeString(j.E)
		if errN != nil || errE != nil || len(e) == 0 || len(e) > 4 {
			return key{}, false
		}
		exponent := int(new(big.Int).SetBytes(e).Int64())
		modulus := new(big.Int).SetBytes(n)
		if exponent < 3 || exponent%2 == 0 || modulus.BitLen() < minRSABits {
			return key{}, false
		}
		k.rsa = &rsa.PublicKey{N: modulus, E: exponent}
	case "EC":
		x, errX := base64.RawURLEncoding.DecodeString(j.X)
		y, errY := base64.RawURLEncoding.DecodeString(j.Y)
		if j.Crv != "P-256" || errX != nil || errY != nil || len(x) != 32 || len(y) != 32 {
			return key{}, false
		}
		public, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), append(append([]byte{4}, x...), y...))
		if err != nil {
			return key{}, false
		}
		k.ec = public
	default:
		return key{}, false
	}
	return k, true
}

// Claims are the claims of an ID token that passed every check.
type Claims map[string]any

// String is the claim name when it is a string, else empty.
func (c Claims) String(name string) string {
	value, _ := c[name].(string)
	return value
}

// Has reports whether the claim name is value, or a list holding it.
func (c Claims) Has(name, value string) bool {
	switch claim := c[name].(type) {
	case string:
		return claim == value
	case []any:
		for _, entry := range claim {
			if entry == value {
				return true
			}
		}
	}
	return false
}

// Subject is the sub claim, which names the person at the provider for
// good.
func (c Claims) Subject() string {
	return c.String("sub")
}

// Email is the email claim, only when the provider says it is verified
// (email_verified true): an email it does not vouch for names nobody.
func (c Claims) Email() string {
	if verified, _ := c["email_verified"].(bool); !verified {
		return ""
	}
	return c.String("email")
}

// check checks the claims of a token whose signature is right.
func (c Claims) check(cfg Config, nonce string, now time.Time) error {
	if c.String("iss") != cfg.Issuer {
		return fmt.Errorf("%w: another issuer", ErrToken)
	}
	audiences := 0
	switch aud := c["aud"].(type) {
	case string:
		audiences = 1
		if aud != cfg.ClientID {
			return fmt.Errorf("%w: another audience", ErrToken)
		}
	case []any:
		audiences = len(aud)
		if !c.Has("aud", cfg.ClientID) {
			return fmt.Errorf("%w: another audience", ErrToken)
		}
	default:
		return fmt.Errorf("%w: no audience", ErrToken)
	}
	if azp, ok := c["azp"]; ok || audiences > 1 {
		if azp != cfg.ClientID {
			return fmt.Errorf("%w: authorized for another party", ErrToken)
		}
	}
	expires, ok := c.time("exp")
	if !ok {
		return fmt.Errorf("%w: no expiry", ErrToken)
	}
	if !now.Before(expires.Add(Skew)) {
		return fmt.Errorf("%w: expired", ErrToken)
	}
	issued, ok := c.time("iat")
	if !ok {
		return fmt.Errorf("%w: no issue time", ErrToken)
	}
	if issued.After(now.Add(Skew)) {
		return fmt.Errorf("%w: issued in the future", ErrToken)
	}
	if subtle.ConstantTimeCompare([]byte(c.String("nonce")), []byte(nonce)) != 1 || nonce == "" {
		return fmt.Errorf("%w: another nonce", ErrToken)
	}
	if c.Subject() == "" {
		return fmt.Errorf("%w: no subject", ErrToken)
	}
	return nil
}

// time reads the claim name as a time in seconds since the epoch.
func (c Claims) time(name string) (time.Time, bool) {
	number, ok := c[name].(json.Number)
	if !ok {
		return time.Time{}, false
	}
	seconds, err := number.Float64()
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(int64(seconds), 0), true
}
