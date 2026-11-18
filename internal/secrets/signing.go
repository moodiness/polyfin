package secrets

import (
	"crypto/hmac"
	"crypto/sha256"
)

// signingPurpose starts what SigningSecret derives from: the key signs
// for this purpose only.
const signingPurpose = "polyfin.signing"

// SigningSecret returns the secret Polyfin signs with, from the one its
// database stores (see database.Secret). With a key, it is derived from
// both, HMAC-SHA256 under the key, so that a copy of the database alone
// signs nothing; without one, it is the stored secret itself. The stored
// secret keeps its bytes either way: what signs changes with it, and with
// the key.
func (b *Box) SigningSecret(stored []byte) []byte {
	if b == nil {
		return stored
	}
	mac := hmac.New(sha256.New, b.key)
	mac.Write([]byte(signingPurpose))
	mac.Write(stored)
	return mac.Sum(nil)
}
