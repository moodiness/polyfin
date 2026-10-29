package drm

import (
	"encoding/json"
	"slices"
	"testing"
)

const (
	kid   = "0123456789abcdef0123456789abcdef"
	key   = "00112233445566778899aabbccddeeff"
	kid2  = "fedcba9876543210fedcba9876543210"
	key2  = "ffeeddccbbaa99887766554433221100"
	kid64 = "ASNFZ4mrze8BI0VniavN7w" // kid in base64url
	key64 = "ABEiM0RVZneImaq7zN3u_w" // key in base64url
)

// Lists give ClearKey keys in several forms; each reads as the same keys,
// by key ID, as FFmpeg takes them.
func TestKeysAsListsGiveThem(t *testing.T) {
	one := []Key{{ID: kid, Value: key}}
	two := []Key{{ID: kid, Value: key}, {ID: kid2, Value: key2}}
	for _, tc := range []struct {
		name, text string
		want       []Key
	}{
		{"hexadecimal", kid + ":" + key, one},
		{"upper case, spaced", "  0123456789ABCDEF0123456789ABCDEF:00112233445566778899AABBCCDDEEFF ", one},
		{"a key ID written as a UUID", "01234567-89ab-cdef-0123-456789abcdef:" + key, one},
		{"base64url", kid64 + ":" + key64, one},
		{"base64, padded", "ASNFZ4mrze8BI0VniavN7w==:ABEiM0RVZneImaq7zN3u/w==", one},
		{"several, apart by commas", kid + ":" + key + ", " + kid2 + ":" + key2, two},
		{"a JSON Web Key set", `{"keys":[{"kty":"oct","k":"` + key64 + `","kid":"` + kid64 + `"}],"type":"temporary"}`, one},
		{"a JSON object of keys by key ID", `{"` + kid2 + `":"` + key2 + `","` + kid + `":"` + key + `"}`, two},
	} {
		got, ok := ParseKeys(tc.text)
		if !ok || !slices.Equal(got, tc.want) {
			t.Errorf("%s: %v %v, want %v", tc.name, got, ok, tc.want)
		}
	}
	for _, text := range []string{"", kid, kid + ":" + key[:30], kid + ":" + key + ",", "https://licence.example/key", `{"keys":[]}`, "zz" + kid[2:] + ":" + key} {
		if keys, ok := ParseKeys(text); ok {
			t.Errorf("%q read as keys: %v", text, keys)
		}
	}
	if option := Option(two); option != kid+"="+key+":"+kid2+"="+key2 {
		t.Errorf("FFmpeg's option: %q", option)
	}
}

// A ClearKey licence request names the key IDs in base64url, for a
// temporary licence; the licence answers a JSON Web Key set, whose
// symmetric keys of 16 bytes are read.
func TestClearKeyLicence(t *testing.T) {
	var request struct {
		Kids []string `json:"kids"`
		Type string   `json:"type"`
	}
	if err := json.Unmarshal(LicenseRequest([]string{kid, kid2}), &request); err != nil ||
		!slices.Equal(request.Kids, []string{kid64, "_ty6mHZUMhD-3LqYdlQyEA"}) || request.Type != "temporary" {
		t.Errorf("licence request: %+v %v", request, err)
	}
	keys, err := ParseLicense([]byte(`{"keys":[{"kty":"RSA","k":"x","kid":"y"},{"kty":"oct","k":"` + key64 + `","kid":"` + kid64 + `"}]}`))
	if err != nil || !slices.Equal(keys, []Key{{ID: kid, Value: key}}) {
		t.Errorf("licence: %v %v", keys, err)
	}
	for _, answer := range []string{`<html>`, `{"keys":[]}`, `{"keys":[{"kty":"oct","k":"c2hvcnQ","kid":"` + kid64 + `"}]}`} {
		if keys, err := ParseLicense([]byte(answer)); err == nil {
			t.Errorf("%s read as a licence: %v", answer, keys)
		}
	}
}

// ClearKey is the only licence Polyfin decrypts with; any other type is
// unsupported, as a server has no content decryption module.
func TestLicenceTypes(t *testing.T) {
	for _, name := range []string{"org.w3.clearkey", "ClearKey"} {
		if p := (&Protection{License: LicenseType(name), Keys: []Key{{ID: kid, Value: key}}}); p.Unsupported() || !p.Decrypted() {
			t.Errorf("%s: unsupported %v, decrypted %v", name, p.Unsupported(), p.Decrypted())
		}
	}
	for _, name := range []string{"com.widevine.alpha", "com.microsoft.playready", "com.huawei.wiseplay"} {
		if p := (&Protection{License: LicenseType(name), Keys: []Key{{ID: kid, Value: key}}}); !p.Unsupported() || p.Decrypted() {
			t.Errorf("%s: unsupported %v, decrypted %v", name, p.Unsupported(), p.Decrypted())
		}
	}
	var none *Protection
	if none.Unsupported() || none.Decrypted() || (&Protection{Manifest: "mpd"}).Unsupported() {
		t.Error("a stream its list says nothing of is unsupported or decrypted")
	}
}
