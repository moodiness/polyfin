package update

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// The release's manifest lists the size and SHA-256 of each package of
// a release, with when it was issued and when it expires, signed with
// ed25519 by the release workflow (cmd/polyfin-release-sign). The app
// installs no package whose manifest is not signed by one of the keys it
// carries, has expired, or gives another size or checksum than GitHub's.
//
// The file is JSON: "manifest", the manifest itself, and "signature",
// base64, the signature of the manifest's JSON without its insignificant
// whitespace, after manifestContext: the file may be indented.

// releaseKeys are the public keys a manifest's signature is checked
// against: base64 ed25519 keys, separated by commas. Two are carried, so
// that the signing key can change: the one the release workflow signs
// with, and a spare kept offline (see docs/desktop.md). Without them,
// the release workflow refuses to sign, and the app installs nothing. A
// build pointing at a fake sets them with -ldflags "-X github.com/moodiness/polyfin/internal/desktop/update.releaseKeys=…".
var releaseKeys = ""

// manifestContext comes before the manifest's bytes in what is signed:
// a signature made for anything else never passes for a manifest's.
const manifestContext = "polyfin release manifest v1\n"

// MaxManifest bounds the bytes of a manifest file.
const MaxManifest = 1 << 20

var (
	// ErrSignature is a manifest that is not signed by a key the app
	// carries, or was changed after it was signed.
	ErrSignature = errors.New("the release's manifest is not signed by Polyfin's release key")
	// ErrExpired is a manifest past its expiry time.
	ErrExpired = errors.New("the release's manifest has expired")
	// ErrNoManifest is a release that carries no manifest.
	ErrNoManifest = errors.New("the release carries no signed manifest")
)

// Manifest lists a release's packages.
type Manifest struct {
	// Version is the release's, without the v.
	Version string    `json:"version"`
	Issued  time.Time `json:"issued"`
	Expires time.Time `json:"expires"`
	Files   []File    `json:"files"`
}

// File is a package of the release: its name, size in bytes and SHA-256,
// in lowercase hexadecimal.
type File struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// File returns the entry of the package name.
func (m Manifest) File(name string) (File, bool) {
	for _, f := range m.Files {
		if f.Name == name {
			return f, true
		}
	}
	return File{}, false
}

// ManifestName is the name of the manifest the release of version
// carries.
func ManifestName(version string) string {
	return "polyfin-" + version + ".manifest.json"
}

// signedManifest is a manifest file.
type signedManifest struct {
	Manifest  json.RawMessage `json:"manifest"`
	Signature []byte          `json:"signature"`
}

// Sign writes the manifest file of m, signed with key.
func Sign(m Manifest, key ed25519.PrivateKey) ([]byte, error) {
	payload, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	signature := ed25519.Sign(key, append([]byte(manifestContext), payload...))
	data, err := json.MarshalIndent(signedManifest{Manifest: payload, Signature: signature}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Open checks the signature of the manifest file data against keys, and
// returns its manifest, whatever its expiry: ErrSignature when no key
// signed it as it is.
func Open(data []byte, keys []ed25519.PublicKey) (Manifest, error) {
	var file signedManifest
	if err := json.Unmarshal(data, &file); err != nil || len(file.Manifest) == 0 {
		return Manifest{}, fmt.Errorf("%w: it cannot be read", ErrSignature)
	}
	if len(file.Signature) != ed25519.SignatureSize {
		return Manifest{}, ErrSignature
	}
	signed := bytes.NewBufferString(manifestContext)
	if err := json.Compact(signed, file.Manifest); err != nil {
		return Manifest{}, ErrSignature
	}
	verified := false
	for _, key := range keys {
		if len(key) == ed25519.PublicKeySize && ed25519.Verify(key, signed.Bytes(), file.Signature) {
			verified = true
			break
		}
	}
	if !verified {
		return Manifest{}, ErrSignature
	}
	var m Manifest
	if err := json.Unmarshal(file.Manifest, &m); err != nil {
		return Manifest{}, fmt.Errorf("%w: its manifest cannot be read", ErrSignature)
	}
	return m, nil
}

// Verify checks the manifest file data as Open does, and refuses it with
// ErrExpired once now is past its expiry time.
func Verify(data []byte, keys []ed25519.PublicKey, now time.Time) (Manifest, error) {
	m, err := Open(data, keys)
	if err != nil {
		return Manifest{}, err
	}
	if !now.Before(m.Expires) {
		return Manifest{}, fmt.Errorf("%w on %s", ErrExpired, m.Expires.UTC().Format(time.RFC3339))
	}
	return m, nil
}

// ReleaseKeys are the public keys the app carries.
func ReleaseKeys() ([]ed25519.PublicKey, error) {
	return ParseKeys(releaseKeys)
}

// ParseKeys reads base64 ed25519 public keys separated by commas.
func ParseKeys(text string) ([]ed25519.PublicKey, error) {
	var keys []ed25519.PublicKey
	for _, field := range strings.Split(text, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(field)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("%q is not an ed25519 public key in base64", field)
		}
		keys = append(keys, ed25519.PublicKey(raw))
	}
	return keys, nil
}

// ParsePrivateKey reads an ed25519 private key: its 32-byte seed in
// base64, as polyfin-release-sign keygen writes it.
func ParsePrivateKey(text string) (ed25519.PrivateKey, error) {
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("the signing key is not an ed25519 seed in base64")
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// EncodeKey writes a public key in base64, as ParseKeys reads it.
func EncodeKey(key ed25519.PublicKey) string {
	return base64.StdEncoding.EncodeToString(key)
}
