package keystore

import (
	"encoding/base64"
	"errors"
	"testing"
)

// fakeStore is a key store in memory, which can fail.
type fakeStore struct {
	secrets map[string]string
	getErr  error
	setErr  error
	sets    int
}

func (f *fakeStore) Get(service, account string) (string, error) {
	if f.getErr != nil {
		return "", f.getErr
	}
	secret, ok := f.secrets[service+"/"+account]
	if !ok {
		return "", ErrNotFound
	}
	return secret, nil
}

func (f *fakeStore) Set(service, account, secret string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.sets++
	f.secrets[service+"/"+account] = secret
	return nil
}

// The first start makes a key the server accepts and stores it; the next
// ones read it back.
func TestSecretKeyIsMadeOnceThenReused(t *testing.T) {
	store := &fakeStore{secrets: map[string]string{}}
	account := Account("/home/a/.local/share/polyfin")
	key, created, err := SecretKey(store, account)
	if err != nil || !created {
		t.Fatalf("first SecretKey: created %v, %v", created, err)
	}
	if b, err := base64.StdEncoding.DecodeString(key); err != nil || len(b) != 32 {
		t.Errorf("key %q is not 32 bytes in base64", key)
	}
	if store.secrets[Service+"/"+account] != key {
		t.Error("the key was not stored under the service and the account")
	}
	again, created, err := SecretKey(store, account)
	if err != nil || created || again != key {
		t.Fatalf("second SecretKey: %q created %v, %v", again, created, err)
	}
	if store.sets != 1 {
		t.Errorf("the key was written %d times", store.sets)
	}
	// Another data folder has a key of its own.
	other, _, err := SecretKey(store, Account("/tmp/other"))
	if err != nil || other == key {
		t.Errorf("another data folder got %q, %v", other, err)
	}
}

// A store that cannot be read, or holds a malformed key, makes no new key.
func TestSecretKeyNeverReplacesAKey(t *testing.T) {
	failing := &fakeStore{secrets: map[string]string{}, getErr: errors.New("locked")}
	if _, _, err := SecretKey(failing, "a"); err == nil || failing.sets != 0 {
		t.Errorf("a store that cannot be read: %v, %d writes", err, failing.sets)
	}
	malformed := &fakeStore{secrets: map[string]string{Service + "/a": "short"}}
	if _, _, err := SecretKey(malformed, "a"); err == nil || malformed.sets != 0 {
		t.Errorf("a malformed key: %v, %d writes", err, malformed.sets)
	}
	readOnly := &fakeStore{secrets: map[string]string{}, setErr: errors.New("denied")}
	if _, _, err := SecretKey(readOnly, "a"); err == nil {
		t.Error("a store that cannot be written gave a key")
	}
	hexKey := &fakeStore{secrets: map[string]string{Service + "/a": "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"}}
	if key, created, err := SecretKey(hexKey, "a"); err != nil || created || key == "" {
		t.Errorf("a key in hexadecimal: %q %v %v", key, created, err)
	}
}

// The key of another server replaces the one made here; a malformed one
// is refused.
func TestReplace(t *testing.T) {
	store := &fakeStore{secrets: map[string]string{}}
	if _, _, err := SecretKey(store, "a"); err != nil {
		t.Fatal(err)
	}
	other := base64.StdEncoding.EncodeToString(make([]byte, 32))
	if err := Replace(store, "a", other); err != nil {
		t.Fatal(err)
	}
	if key, created, err := SecretKey(store, "a"); err != nil || created || key != other {
		t.Errorf("after Replace: %q %v %v", key, created, err)
	}
	if key, err := Read(store, "a"); err != nil || key != other {
		t.Errorf("Read = %q, %v", key, err)
	}
	if _, err := Read(store, "none"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Read of no key = %v", err)
	}
	if err := Replace(store, "a", "not a key"); err == nil || store.secrets[Service+"/a"] != other {
		t.Error("a malformed key replaced the stored one")
	}
}
