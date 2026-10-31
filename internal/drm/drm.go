// Package drm reads how an IPTV list says a stream is encrypted, and the
// ClearKey keys Polyfin decrypts it with: those the list gives, or those a
// ClearKey licence server answers (W3C Encrypted Media Extensions, Clear
// Key). Keys are secrets: they go nowhere but to FFmpeg.
package drm

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
)

// ClearKey is the licence type of a stream decrypted with keys its list,
// or a licence server, gives.
const ClearKey = "clearkey"

// Protection is how a stream's list says it is encrypted (#KODIPROP
// lines): the manifest type and the licence type it names, and for
// ClearKey, its keys or the licence server that gives them, asked with
// LicenseHeaders on top of the stream's own.
type Protection struct {
	// Manifest is the manifest type the list names, lower-cased ("mpd",
	// "hls"), empty when it names none.
	Manifest string `json:"manifest,omitempty"`
	// License is ClearKey, or the licence type as the list names it,
	// lower-cased; empty when it names none.
	License        string            `json:"license,omitempty"`
	Keys           []Key             `json:"keys,omitempty"`
	LicenseURL     string            `json:"licenseUrl,omitempty"`
	LicenseHeaders map[string]string `json:"licenseHeaders,omitempty"`
}

// Key is a ClearKey key: its key ID and its value, 32 lower-case
// hexadecimal digits each, as FFmpeg takes them.
type Key struct {
	ID    string `json:"kid"`
	Value string `json:"key"`
}

// Unsupported reports whether the stream needs a licence Polyfin cannot
// have: any licence type but ClearKey. A server has no content decryption
// module, which the other types need.
func (p *Protection) Unsupported() bool {
	return p != nil && p.License != "" && p.License != ClearKey
}

// Decrypted reports whether the stream is decrypted with ClearKey keys:
// given by its list, or by a licence server.
func (p *Protection) Decrypted() bool {
	return p != nil && p.License == ClearKey && (len(p.Keys) > 0 || p.LicenseURL != "")
}

// Name is how the licence type is told to an administrator.
func (p *Protection) Name() string {
	switch {
	case p == nil:
		return ""
	case strings.Contains(p.License, "widevine"):
		return "Widevine"
	case strings.Contains(p.License, "playready"):
		return "PlayReady"
	case strings.Contains(p.License, "wiseplay"):
		return "WisePlay"
	case strings.Contains(p.License, "fairplay"):
		return "FairPlay"
	}
	return p.License
}

// LicenseType normalizes a licence type as lists name it: ClearKey for
// org.w3.clearkey or clearkey, else the name lower-cased.
func LicenseType(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "org.w3.clearkey", "clearkey":
		return ClearKey
	}
	return name
}

// Option is the value of FFmpeg's cenc_decryption_keys option, the keys
// by key ID: kid=key pairs apart by colons.
func Option(keys []Key) string {
	pairs := make([]string, len(keys))
	for i, k := range keys {
		pairs[i] = k.ID + "=" + k.Value
	}
	return strings.Join(pairs, ":")
}

// ParseKeys reads ClearKey keys as lists give them: kid:key pairs apart by
// commas, each half in hexadecimal (a key ID may be written as a UUID) or
// base64url (base64 accepted, padded or not); or a JSON Web Key set, as a
// licence server answers it (see ParseLicense); or a JSON object of key
// values by key ID. It reports false when text is none of these, or a key
// is not 16 bytes.
func ParseKeys(text string) ([]Key, bool) {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "{") {
		if keys, err := ParseLicense([]byte(text)); err == nil {
			return keys, true
		}
		var pairs map[string]string
		if json.Unmarshal([]byte(text), &pairs) != nil || len(pairs) == 0 {
			return nil, false
		}
		var keys []Key
		for id, value := range pairs {
			key, ok := pair(id, value)
			if !ok {
				return nil, false
			}
			keys = append(keys, key)
		}
		slices.SortFunc(keys, func(a, b Key) int { return strings.Compare(a.ID, b.ID) })
		return keys, true
	}
	var keys []Key
	for part := range strings.SplitSeq(text, ",") {
		id, value, found := strings.Cut(strings.TrimSpace(part), ":")
		if !found {
			return nil, false
		}
		key, ok := pair(id, value)
		if !ok {
			return nil, false
		}
		keys = append(keys, key)
	}
	return keys, len(keys) > 0
}

// pair reads a key ID and its key value.
func pair(id, value string) (Key, bool) {
	kid, ok := Bytes(id)
	if !ok {
		return Key{}, false
	}
	key, ok := Bytes(value)
	if !ok {
		return Key{}, false
	}
	return Key{ID: hex.EncodeToString(kid), Value: hex.EncodeToString(key)}, true
}

// Bytes reads a 16-byte key ID or key value written in hexadecimal, as a
// UUID or not, or in base64url or base64, padded or not.
func Bytes(text string) ([]byte, bool) {
	text = strings.TrimSpace(text)
	if len(text) == 36 && strings.Count(text, "-") == 4 {
		text = strings.ReplaceAll(text, "-", "")
	}
	if len(text) == 32 {
		if raw, err := hex.DecodeString(text); err == nil {
			return raw, true
		}
	}
	trimmed := strings.TrimRight(text, "=")
	for _, encoding := range []*base64.Encoding{base64.RawURLEncoding, base64.RawStdEncoding} {
		if raw, err := encoding.DecodeString(trimmed); err == nil && len(raw) == 16 {
			return raw, true
		}
	}
	return nil, false
}

// LicenseRequest is the ClearKey licence request for key IDs: a JSON
// object of their base64url encodings, for a temporary licence (W3C
// Encrypted Media Extensions, 9.1.3 License Request Format).
func LicenseRequest(ids []string) []byte {
	request := struct {
		Kids []string `json:"kids"`
		Type string   `json:"type"`
	}{Kids: []string{}, Type: "temporary"}
	for _, id := range ids {
		if raw, ok := Bytes(id); ok {
			request.Kids = append(request.Kids, base64.RawURLEncoding.EncodeToString(raw))
		}
	}
	data, _ := json.Marshal(request)
	return data
}

// ErrNoLicense reports a licence that holds no key: not a JSON Web Key
// set, or none of its keys a symmetric key of 16 bytes.
var ErrNoLicense = errors.New("the licence holds no ClearKey key")

// ParseLicense reads a ClearKey licence: a JSON Web Key set of symmetric
// keys ("kty" "oct"), each key value ("k") and key ID ("kid") in base64url
// (W3C Encrypted Media Extensions, 9.1.4 License Format). Keys of other
// types, or not 16 bytes, are left out.
func ParseLicense(data []byte) ([]Key, error) {
	var license struct {
		Keys []struct {
			Type  string `json:"kty"`
			Value string `json:"k"`
			ID    string `json:"kid"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(data), &license); err != nil {
		return nil, ErrNoLicense
	}
	var keys []Key
	for _, k := range license.Keys {
		if k.Type != "" && k.Type != "oct" {
			continue
		}
		if key, ok := pair(k.ID, k.Value); ok {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil, ErrNoLicense
	}
	return keys, nil
}
