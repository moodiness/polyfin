package accounts

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

// ID identifies a user or a device. Jellyfin apps exchange identifiers as
// 32 lowercase hexadecimal characters and sometimes send them hyphenated.
type ID [16]byte

// ErrInvalidID reports a value that is not a 32-character hexadecimal ID,
// hyphenated or not.
var ErrInvalidID = errors.New("invalid ID")

// ParseID accepts the plain and the hyphenated forms, in any letter case.
func ParseID(value string) (ID, error) {
	var id ID
	if len(value) == 36 {
		if value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
			return id, ErrInvalidID
		}
		value = strings.ReplaceAll(value, "-", "")
	}
	if len(value) != 32 {
		return id, ErrInvalidID
	}
	if _, err := hex.Decode(id[:], []byte(value)); err != nil {
		return id, ErrInvalidID
	}
	return id, nil
}

func (id ID) String() string {
	return hex.EncodeToString(id[:])
}

// newToken returns a random access token in the 32-hexadecimal-character
// form Jellyfin apps expect, and the hash stored in its place.
func newToken() (token string, hash []byte) {
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	token = hex.EncodeToString(raw[:])
	return token, hashToken(token)
}

// hashToken accepts tokens in any letter case, like the identifiers above.
func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(strings.ToLower(token)))
	return sum[:]
}
