// Package secrets encrypts the keys, secrets and tokens Polyfin stores in
// its database with POLYFIN_SECRET_KEY, so that a copy of the database
// alone does not hand them out. A value is sealed with AES-256-GCM under a
// nonce of its own and marked with a version prefix, which tells sealed
// values from the plaintext ones stored without a key.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

// prefix starts every sealed value: the scheme and its version.
const prefix = "enc:v1:"

// KeySize is the length of a key, in bytes.
const KeySize = 32

var (
	// ErrInvalidKey reports a key that is neither 32 bytes in base64 nor
	// 64 hexadecimal digits.
	ErrInvalidKey = errors.New("not 32 bytes in base64 (openssl rand -base64 32) or 64 hexadecimal digits")
	// ErrUnreadable reports a sealed value the key cannot open: no key,
	// another key than the one it was sealed with, or a damaged value.
	ErrUnreadable = errors.New("sealed with another key, or damaged")
)

// ParseKey reads a key written as base64 (padded or not, standard or URL
// alphabet) or as hexadecimal.
func ParseKey(text string) ([]byte, error) {
	text = strings.TrimSpace(text)
	if len(text) == 2*KeySize {
		if key, err := hex.DecodeString(text); err == nil {
			return key, nil
		}
	}
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if key, err := encoding.DecodeString(text); err == nil && len(key) == KeySize {
			return key, nil
		}
	}
	return nil, ErrInvalidKey
}

// Box seals and opens values with a key. A nil Box has no key: it stores
// values as they are, and cannot open sealed ones.
type Box struct {
	aead cipher.AEAD
}

// New returns a box sealing with key, KeySize bytes; nil for no key.
func New(key []byte) (*Box, error) {
	if key == nil {
		return nil, nil
	}
	if len(key) != KeySize {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Enabled reports whether b has a key.
func (b *Box) Enabled() bool { return b != nil }

// Sealed reports whether a stored value is sealed.
func Sealed(stored string) bool { return strings.HasPrefix(stored, prefix) }

// Seal returns what to store for value: sealed with the key, or value
// itself without one. Empty stays empty, for no value.
func (b *Box) Seal(value string) string {
	if b == nil || value == "" {
		return value
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		// crypto/rand does not fail on the systems Go supports.
		panic(err)
	}
	return prefix + base64.RawStdEncoding.EncodeToString(b.aead.Seal(nonce, nonce, []byte(value), nil))
}

// Open returns the value stored: a plaintext value as it is, a sealed one
// opened, or ErrUnreadable when the key cannot open it.
func (b *Box) Open(stored string) (string, error) {
	if !Sealed(stored) {
		return stored, nil
	}
	if b == nil {
		return "", ErrUnreadable
	}
	data, err := base64.RawStdEncoding.DecodeString(stored[len(prefix):])
	size := b.aead.NonceSize()
	if err != nil || len(data) < size {
		return "", ErrUnreadable
	}
	value, err := b.aead.Open(nil, data[:size], data[size:], nil)
	if err != nil {
		return "", ErrUnreadable
	}
	return string(value), nil
}
