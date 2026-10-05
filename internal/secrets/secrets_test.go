package secrets

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func box(t *testing.T, key string) *Box {
	t.Helper()
	parsed, err := ParseKey(key)
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(parsed)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestKeysAreReadAsWritten(t *testing.T) {
	want := []byte("0123456789abcdef0123456789abcdef")
	for _, text := range []string{
		"MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		" MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY\n",
		"3031323334353637383961626364656630313233343536373839616263646566",
	} {
		if key, err := ParseKey(text); err != nil || !bytes.Equal(key, want) {
			t.Errorf("%q: %x %v", text, key, err)
		}
	}
	for _, text := range []string{"", "short", "MDEyMzQ1Njc4OWFiY2RlZg==", strings.Repeat("z", 64)} {
		if _, err := ParseKey(text); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("%q accepted: %v", text, err)
		}
	}
}

func TestSealedValuesOpenWithTheirKeyOnly(t *testing.T) {
	b := box(t, "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	const value = "pm-Xk9mN2pQrS7tUvWx3yZaB4cD5eF6gH7iJ8kL9mN0oP1qR2sT3uV4wXyZ5aB"
	sealed := b.Seal(value)
	if !Sealed(sealed) || strings.Contains(sealed, value) || strings.Contains(sealed, "pm-") {
		t.Fatalf("sealed: %q", sealed)
	}
	if again := b.Seal(value); again == sealed {
		t.Error("two seals of one value are the same: the nonce is reused")
	}
	if opened, err := b.Open(sealed); err != nil || opened != value {
		t.Errorf("opened %q, %v", opened, err)
	}
	// The longest key stored still fits the database's checks.
	if long := b.Seal(strings.Repeat("k", 256)); len(long) > 512 {
		t.Errorf("a 256-byte key seals into %d bytes", len(long))
	}

	other := box(t, "3031323334353637383961626364656630313233343536373839616263646500")
	var none *Box
	for name, tc := range map[string]struct {
		b      *Box
		stored string
	}{
		"another key": {other, sealed},
		"no key":      {none, sealed},
		"damaged":     {b, sealed[:len(sealed)-4] + "AAAA"},
		"cut short":   {b, prefix + "AAAA"},
		"not base64":  {b, prefix + "%%%"},
	} {
		if opened, err := tc.b.Open(tc.stored); !errors.Is(err, ErrUnreadable) || opened != "" {
			t.Errorf("%s: %q, %v", name, opened, err)
		}
	}

	// Without a key, values are stored and read as they are; empty is no
	// value, with or without one.
	if stored := none.Seal(value); stored != value || none.Enabled() {
		t.Errorf("without a key: %q", stored)
	}
	if opened, err := b.Open(value); err != nil || opened != value {
		t.Errorf("plaintext with a key: %q, %v", opened, err)
	}
	if b.Seal("") != "" {
		t.Error("empty is sealed")
	}
}
