package playback

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"

	"github.com/moodiness/polyfin/internal/accounts"
)

// ErrInvalidGrant reports a grant that Polyfin did not sign.
var ErrInvalidGrant = errors.New("invalid playback grant")

// Grant lets a player fetch one version for one user without credentials.
// Players fetch media with URLs that carry no token, and play sessions are
// named by the server: both are grants Polyfin signs, so they stay valid
// across restarts and cannot be forged.
type Grant struct {
	Version accounts.ID
	User    accounts.ID
	// Relay makes Polyfin relay the bytes instead of redirecting the player
	// to the source, for players that cannot follow the redirect.
	Relay bool
}

const (
	grantPayload = 16 + 16 + 1 + 8 // version, user, flags, nonce
	grantMAC     = 16
)

// Signer signs and checks grants with the server's secret.
type Signer struct {
	secret []byte
}

// NewSigner returns a signer using secret, which must stay private.
func NewSigner(secret []byte) Signer {
	return Signer{secret: secret}
}

// Sign encodes a grant as a URL-safe token. Two signatures of the same grant
// differ, so that each play session has its own identifier.
func (s Signer) Sign(grant Grant) string {
	var token [grantPayload + grantMAC]byte
	copy(token[0:16], grant.Version[:])
	copy(token[16:32], grant.User[:])
	if grant.Relay {
		token[32] = 1
	}
	_, _ = rand.Read(token[33:grantPayload])
	copy(token[grantPayload:], s.mac(token[:grantPayload]))
	return base64.RawURLEncoding.EncodeToString(token[:])
}

// Verify decodes a token signed by Sign.
func (s Signer) Verify(token string) (Grant, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != grantPayload+grantMAC || !hmac.Equal(raw[grantPayload:], s.mac(raw[:grantPayload])) {
		return Grant{}, ErrInvalidGrant
	}
	var grant Grant
	copy(grant.Version[:], raw[0:16])
	copy(grant.User[:], raw[16:32])
	grant.Relay = raw[32]&1 == 1
	return grant, nil
}

func (s Signer) mac(payload []byte) []byte {
	h := hmac.New(sha256.New, s.secret)
	h.Write([]byte("polyfin.playback.grant\x00"))
	h.Write(payload)
	return h.Sum(nil)[:grantMAC]
}

// Link signs the address of a file of a live stream that the player of a
// grant, given as signed, may fetch through Polyfin: the playlists and
// segments a channel's playlist names.
func (s Signer) Link(grant, target string) string {
	h := hmac.New(sha256.New, s.secret)
	h.Write([]byte("polyfin.playback.link\x00" + grant + "\x00" + target))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil)[:grantMAC])
}

// VerifyLink reports whether signature is Link's for grant and target.
func (s Signer) VerifyLink(grant, target, signature string) bool {
	return hmac.Equal([]byte(s.Link(grant, target)), []byte(signature))
}
