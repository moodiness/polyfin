package main

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/desktop/update"
)

// keyPair makes a key with keygen, and returns its private key's file
// contents and its public key.
func keyPair(t *testing.T) (string, ed25519.PublicKey) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "release.key")
	var out bytes.Buffer
	if err := run([]string{"keygen", "-out", path}, os.Getenv, nil, time.Now(), &out); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/' {
		t.Errorf("the private key's file: %v, %v", info, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), strings.TrimSpace(string(data))) {
		t.Error("keygen printed the private key")
	}
	fields := strings.Fields(out.String())
	keys, err := update.ParseKeys(fields[len(fields)-1])
	if err != nil || len(keys) != 1 {
		t.Fatalf("the public key printed: %v, %v", keys, err)
	}
	if err := run([]string{"keygen", "-out", path}, os.Getenv, nil, time.Now(), &out); err == nil {
		t.Error("keygen replaced a key")
	}
	return string(data), keys[0]
}

// The workflow's manifest lists the packages, signed with the key of the
// secret, which must be one the app carries; verify checks it and the
// packages; renew re-dates it only once its signature checks, even after
// it expired.
func TestManifestVerifyAndRenew(t *testing.T) {
	private, public := keyPair(t)
	_, spare := keyPair(t)
	keys := []ed25519.PublicKey{spare, public}
	env := func(name string) string {
		if name == KeyVariable {
			return private
		}
		return ""
	}
	dir := t.TempDir()
	packages := []string{filepath.Join(dir, "Polyfin-Desktop-1.4.0-macos-arm64.dmg"), filepath.Join(dir, "polyfin-desktop_1.4.0_amd64.deb")}
	for i, path := range packages {
		if err := os.WriteFile(path, bytes.Repeat([]byte{byte(i)}, 1000+i), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	manifest := filepath.Join(dir, update.ManifestName("1.4.0"))
	issued := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	var out bytes.Buffer
	args := append([]string{"manifest", "-version", "1.4.0", "-out", manifest}, packages...)
	if err := run(args, env, keys, issued, &out); err != nil {
		t.Fatal(err)
	}
	if err := run(append([]string{"verify", "-in", manifest}, packages...), env, keys, issued.Add(time.Hour), &out); err != nil {
		t.Errorf("verify: %v", err)
	}
	data, _ := os.ReadFile(manifest)
	m, err := update.Verify(data, keys, issued)
	if err != nil || len(m.Files) != 2 || m.Files[1].Size != 1001 || !m.Expires.Equal(issued.Add(defaultValidity)) {
		t.Fatalf("manifest = %+v, %v", m, err)
	}

	// Another key than those the app carries signs nothing.
	if err := run(args, env, []ed25519.PublicKey{spare}, issued, &out); err == nil || !strings.Contains(err.Error(), "not one of those") {
		t.Errorf("an unknown key: %v", err)
	}
	if err := run([]string{"key"}, env, []ed25519.PublicKey{spare}, issued, &out); err == nil {
		t.Error("key accepted an unknown key")
	}
	if err := run([]string{"key"}, env, keys, issued, &out); err != nil {
		t.Errorf("key: %v", err)
	}
	// A package changed since is caught.
	if err := os.WriteFile(packages[0], []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(append([]string{"verify", "-in", manifest}, packages...), env, keys, issued, &out); err == nil {
		t.Error("verify accepted a changed package")
	}

	// Expired, it is renewed for another 30 days from now.
	later := issued.Add(40 * 24 * time.Hour)
	if err := run([]string{"verify", "-in", manifest}, env, keys, later, &out); !errors.Is(err, update.ErrExpired) {
		t.Errorf("verify once expired: %v", err)
	}
	renewed := filepath.Join(dir, "renewed.json")
	if err := run([]string{"renew", "-in", manifest, "-out", renewed}, env, keys, later, &out); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(renewed)
	if again, err := update.Verify(data, keys, later); err != nil || !again.Issued.Equal(later) || len(again.Files) != 2 ||
		again.Files[0] != m.Files[0] {
		t.Errorf("renewed = %+v, %v", again, err)
	}

	// A manifest edited since it was signed is not renewed.
	tampered := filepath.Join(dir, "tampered.json")
	original, _ := os.ReadFile(manifest)
	if err := os.WriteFile(tampered, bytes.Replace(original, []byte(`1001`), []byte(`1002`), 1), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"renew", "-in", tampered, "-out", renewed}, env, keys, later, &out); !errors.Is(err, update.ErrSignature) {
		t.Errorf("renew of a tampered manifest: %v", err)
	}
}
