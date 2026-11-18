package playback

import (
	"cmp"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// ErrInvalidGrant reports a grant that Polyfin did not sign.
var ErrInvalidGrant = errors.New("invalid playback grant")

// Grant lets a player fetch one version for one user without credentials.
// Players fetch media with URLs that carry no token, and play sessions are
// named by the server: both are grants Polyfin signs, so they stay valid
// across restarts and cannot be forged. What reads a grant decides how
// long it stands after it was signed.
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
	// Signed is when the grant was signed, to the second; zero for a
	// grant that carries no time, as those signed before grants carried
	// one.
	Signed time.Time
}

// A grant is its version, its user, its flags, its device when it has
// one, the time it was signed when it tells it, and a nonce, then their
// MAC. A grant without a device or without a time is as grants were
// before they had one, so that those given out before still verify. The
// time is in seconds since 1970, on 32 bits: until 2106.
const (
	grantPayload = 16 + 16 + 1 + 8 // version, user, flags, nonce
	grantDevice  = 16
	grantTime    = 4
	grantMAC     = 16

	grantRelay     = 1
	grantHasDevice = 2
	grantHasTime   = 4
)

// grantDomain is what the MAC of a signer's grants starts with, unless
// the signer was given a domain of its own (see Signer.Domain).
const grantDomain = "polyfin.playback.grant"

// Signer signs and checks grants with the server's secret.
type Signer struct {
	secret []byte
	// domain is what the MAC of the grants starts with, grantDomain when
	// empty.
	domain string
}

// NewSigner returns a signer using secret, which must stay private.
func NewSigner(secret []byte) Signer {
	return Signer{secret: secret}
}

// Domain returns a signer of the same secret whose grants are those of
// the service name only: neither s nor a signer of another domain
// verifies them, and it verifies none of theirs. A grant given out by a
// service that asks for less than the others, as DLNA does on the local
// network, then opens nothing elsewhere. Links and marks have purposes of
// their own and are the same in every domain.
func (s Signer) Domain(name string) Signer {
	s.domain = "polyfin." + name + ".grant"
	return s
}

// Sign encodes a grant as a URL-safe token. Two signatures of the same grant
// differ, so that each play session has its own identifier.
func (s Signer) Sign(grant Grant) string {
	var token [grantPayload + grantDevice + grantTime + grantMAC]byte
	copy(token[0:16], grant.Version[:])
	copy(token[16:32], grant.User[:])
	if grant.Relay {
		token[32] |= grantRelay
	}
	nonce := 33
	if grant.Device != (accounts.ID{}) {
		token[32] |= grantHasDevice
		copy(token[nonce:nonce+grantDevice], grant.Device[:])
		nonce += grantDevice
	}
	if !grant.Signed.IsZero() {
		token[32] |= grantHasTime
		binary.BigEndian.PutUint32(token[nonce:nonce+grantTime], uint32(grant.Signed.Unix()))
		nonce += grantTime
	}
	size := nonce + 8
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
	if raw[32]&grantHasTime != 0 {
		size += grantTime
	}
	if len(raw) != size+grantMAC || !hmac.Equal(raw[size:], s.mac(raw[:size])) {
		return Grant{}, ErrInvalidGrant
	}
	var grant Grant
	copy(grant.Version[:], raw[0:16])
	copy(grant.User[:], raw[16:32])
	grant.Relay = raw[32]&grantRelay != 0
	next := 33
	if raw[32]&grantHasDevice != 0 {
		copy(grant.Device[:], raw[next:next+grantDevice])
		next += grantDevice
	}
	if raw[32]&grantHasTime != 0 {
		grant.Signed = time.Unix(int64(binary.BigEndian.Uint32(raw[next:next+grantTime])), 0)
	}
	return grant, nil
}

func (s Signer) mac(payload []byte) []byte {
	h := hmac.New(sha256.New, s.secret)
	h.Write([]byte(cmp.Or(s.domain, grantDomain) + "\x00"))
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
