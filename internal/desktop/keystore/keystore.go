// Package keystore keeps the POLYFIN_SECRET_KEY of Polyfin Desktop in the
// system's key store: the macOS keychain, Windows Credential Manager, or
// the Secret Service on Linux. The key is made at first start, and read
// back at each start after.
package keystore

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

// Service is the name the key is stored under. Its account names the
// data folder, so that a data folder used for development keeps a key of
// its own.
const Service = "Polyfin"

// ErrNotFound is a key the store does not hold.
var ErrNotFound = errors.New("the key store holds no such key")

// Store is a key store.
type Store interface {
	Get(service, account string) (string, error)
	Set(service, account, secret string) error
}

// System is the system's key store.
type System struct{}

// Get reads a secret, ErrNotFound when there is none.
func (System) Get(service, account string) (string, error) {
	secret, err := keyring.Get(service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return secret, err
}

// Set stores a secret, replacing the one stored before.
func (System) Set(service, account, secret string) error {
	return keyring.Set(service, account, secret)
}

// Account is the account the key of the data folder dir is stored as.
func Account(dir string) string {
	return "Secret key of " + dir
}

// SecretKey returns the key stored for account, making and storing one
// when the store holds none: it reports whether it made it. A store that
// cannot be read, or holds a key the server would refuse, returns an
// error and makes nothing, so that no key replaces one that exists.
func SecretKey(store Store, account string) (key string, created bool, err error) {
	key, err = store.Get(Service, account)
	switch {
	case err == nil:
		if !valid(key) {
			return "", false, errors.New("the key store holds a key that is not 32 bytes in base64")
		}
		return key, false, nil
	case !errors.Is(err, ErrNotFound):
		return "", false, fmt.Errorf("read the key store: %w", err)
	}
	key, err = newKey()
	if err != nil {
		return "", false, err
	}
	if err := store.Set(Service, account, key); err != nil {
		return "", false, fmt.Errorf("write to the key store: %w", err)
	}
	// What the store answers is what the next start reads.
	stored, err := store.Get(Service, account)
	if err != nil || stored != key {
		return "", false, fmt.Errorf("the key store did not keep the key: %w", err)
	}
	return key, true, nil
}

// Read returns the key stored for account, to copy it: ErrNotFound
// before the first start made it.
func Read(store Store, account string) (string, error) {
	key, err := store.Get(Service, account)
	if err != nil {
		return "", err
	}
	if !valid(key) {
		return "", errors.New("the key store holds a key that is not 32 bytes in base64")
	}
	return key, nil
}

// Replace stores key for account in place of the one stored: the key of
// another server, whose backup is restored here.
func Replace(store Store, account, key string) error {
	if !valid(key) {
		return errors.New("the key is not 32 bytes in base64, nor 64 hexadecimal digits")
	}
	if err := store.Set(Service, account, key); err != nil {
		return fmt.Errorf("write to the key store: %w", err)
	}
	return nil
}

// newKey is 32 random bytes in base64, as openssl rand -base64 32 makes.
func newKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// valid tells a key the server accepts: 32 bytes in base64, or in 64
// hexadecimal digits.
func valid(key string) bool {
	if b, err := base64.StdEncoding.DecodeString(key); err == nil && len(b) == 32 {
		return true
	}
	b, err := hex.DecodeString(key)
	return err == nil && len(b) == 32
}
