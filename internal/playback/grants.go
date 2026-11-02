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
	// Device is the sign-in the grant was given to, zero when it was
	// given to none: the media requests made with it are that device's
	// playback, as those carrying its token are.
	Device accounts.ID
	// Relay makes Polyfin relay the bytes instead of redirecting the player
	// to the source, for players that cannot follow the redirect.
	Relay bool
}

// A grant is its version, its user, its flags, its device when it has
// one, and a nonce, then their MAC. A grant without a device is as grants
// were before they had one, so that those given out before still verify.
const (
	grantPayload = 16 + 16 + 1 + 8 // version, user, flags, nonce
	grantDevice  = 16
	grantMAC     = 16

	grantRelay     = 1
	grantHasDevice = 2
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
	var token [grantPayload + grantDevice + grantMAC]byte
	copy(token[0:16], grant.Version[:])
	copy(token[16:32], grant.User[:])
	if grant.Relay {
		token[32] |= grantRelay
	}
	nonce, size := 33, grantPayload
	if grant.Device != (accounts.ID{}) {
		token[32] |= grantHasDevice
		copy(token[33:33+grantDevice], grant.Device[:])
		nonce, size = 33+grantDevice, grantPayload+grantDevice
	}
	_, _ = rand.Read(token[nonce:size])
	copy(token[size:], s.mac(token[:size]))
	return base64.RawURLEncoding.EncodeToString(token[:size+grantMAC])
}

// Verify decodes a token signed by Sign.
func (s Signer) Verify(token string) (Grant, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) < grantPayload+grantMAC {
		return Grant{}, ErrInvalidGrant
	}
	size := grantPayload
	if raw[32]&grantHasDevice != 0 {
		size += grantDevice
	}
	if len(raw) != size+grantMAC || !hmac.Equal(raw[size:], s.mac(raw[:size])) {
		return Grant{}, ErrInvalidGrant
	}
	var grant Grant
	copy(grant.Version[:], raw[0:16])
	copy(grant.User[:], raw[16:32])
	grant.Relay = raw[32]&grantRelay != 0
	if raw[32]&grantHasDevice != 0 {
		copy(grant.Device[:], raw[33:33+grantDevice])
	}
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

// Mark signs value as one Polyfin gave out for purpose, such as the
// address of an image it listed, for VerifyMark to tell it when it comes
// back.
func (s Signer) Mark(purpose, value string) string {
	h := hmac.New(sha256.New, s.secret)
	h.Write([]byte("polyfin.mark\x00" + purpose + "\x00" + value))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil)[:grantMAC])
}

// VerifyMark reports whether signature is Mark's for purpose and value.
func (s Signer) VerifyMark(purpose, value, signature string) bool {
	return hmac.Equal([]byte(s.Mark(purpose, value)), []byte(signature))
}
