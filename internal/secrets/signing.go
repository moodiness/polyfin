package secrets

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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

// fingerprintPurpose starts what Fingerprint derives from, apart from
// what signs (see signingPurpose).
const fingerprintPurpose = "polyfin.fingerprint"

// Fingerprint returns a keyed hash of value, HMAC-SHA256 under the key in
// hexadecimal, which tells two sealed values equal, as a unique index
// does, without telling them: Seal gives each value a nonce of its own.
// Without a key, it is empty, for none.
func (b *Box) Fingerprint(value string) string {
	if b == nil {
		return ""
	}
	mac := hmac.New(sha256.New, b.key)
	mac.Write([]byte(fingerprintPurpose))
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}
