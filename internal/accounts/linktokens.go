package accounts

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
)

// The links Polyfin gives guests, invite links and share links, carry a
// token: the link's ID then a random secret, in base64 for URLs. Only the
// secret's hash is kept, compared in constant time.

// linkSecretSize is the size of the random part of a link's token, as
// large as an admin session token's.
const linkSecretSize = 32

// newLinkSecret makes the random part of a new link's token, and the hash
// kept of it.
func newLinkSecret() ([]byte, [sha256.Size]byte) {
	secret := make([]byte, linkSecretSize)
	_, _ = rand.Read(secret)
	return secret, sha256.Sum256(secret)
}

// linkToken is the token of a link: its ID then its secret.
func linkToken(id ID, secret []byte) string {
	return base64.RawURLEncoding.EncodeToString(append(id[:], secret...))
}

// parseLinkToken splits a token into the link's ID and its secret.
func parseLinkToken(token string) (ID, []byte, bool) {
	var id ID
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != len(id)+linkSecretSize {
		return id, nil, false
	}
	copy(id[:], raw)
	return id, raw[len(id):], true
}

// secretMatches reports whether secret is the one whose hash is stored,
// compared in constant time.
func secretMatches(secret, stored []byte) bool {
	hash := sha256.Sum256(secret)
	return subtle.ConstantTimeCompare(hash[:], stored) == 1
}
