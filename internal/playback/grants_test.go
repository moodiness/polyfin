package playback

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

func TestGrantsRoundTripAndResistForgery(t *testing.T) {
	t.Parallel()
	signer := NewSigner([]byte("a secret of the server, 32 bytes"))
	grant := Grant{Version: accounts.ID{1, 2, 3}, User: accounts.ID{9, 8, 7}, Relay: true}
	first, second := signer.Sign(grant), signer.Sign(grant)
	if first == second {
		t.Error("two play sessions of the same grant share an identifier")
	}
	for _, token := range []string{first, second} {
		if got, err := signer.Verify(token); err != nil || got != grant {
			t.Fatalf("verify: %+v %v", got, err)
		}
	}
	tampered := []byte(first)
	tampered[3] ^= 1
	other := NewSigner([]byte("another server's secret, 32 byte"))
	for name, token := range map[string]string{
		"tampered":     string(tampered),
		"other server": other.Sign(grant),
		"truncated":    first[:len(first)-2],
		"not base64":   strings.Repeat("*", len(first)),
		"empty":        "",
	} {
		if _, err := signer.Verify(token); !errors.Is(err, ErrInvalidGrant) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

// A grant given to a device names it; one given to none is as grants were
// before they named devices, so that the addresses given out before still
// play.
func TestGrantsNameTheirDevice(t *testing.T) {
	t.Parallel()
	signer := NewSigner([]byte("a secret of the server, 32 bytes"))
	given := Grant{Version: accounts.ID{1, 2, 3}, User: accounts.ID{9, 8, 7}, Device: accounts.ID{4, 5, 6}}
	token := signer.Sign(given)
	if got, err := signer.Verify(token); err != nil || got != given {
		t.Fatalf("verify: %+v %v, want %+v", got, err, given)
	}
	tampered := []byte(token)
	tampered[len(token)/2] ^= 1
	if _, err := signer.Verify(string(tampered)); !errors.Is(err, ErrInvalidGrant) {
		t.Errorf("a grant whose device was changed: got %v", err)
	}
	// A grant signed before devices: version, user, flags, nonce, MAC.
	old := make([]byte, 16+16+1+8, 16+16+1+8+16)
	copy(old[0:16], given.Version[:])
	copy(old[16:32], given.User[:])
	old[32] = 1
	old = append(old, signer.mac(old)...)
	if got, err := signer.Verify(base64.RawURLEncoding.EncodeToString(old)); err != nil || got != (Grant{Version: given.Version, User: given.User, Relay: true}) {
		t.Errorf("a grant signed before devices: %+v %v", got, err)
	}
}

// A grant tells when it was signed, to the second, whether it names a
// device or not, and the time is signed with the rest. A grant signed
// with no time is as grants were before they told it, and verifies as
// telling none: what reads it decides what it is worth.
func TestGrantsTellWhenTheyWereSigned(t *testing.T) {
	t.Parallel()
	signer := NewSigner([]byte("a secret of the server, 32 bytes"))
	at := time.Date(2026, time.November, 18, 20, 30, 15, 987_000_000, time.UTC)
	for name, given := range map[string]Grant{
		"with a device":    {Version: accounts.ID{1, 2, 3}, User: accounts.ID{9, 8, 7}, Device: accounts.ID{4, 5, 6}, Relay: true, Signed: at},
		"without a device": {Version: accounts.ID{1, 2, 3}, User: accounts.ID{9, 8, 7}, Signed: at},
	} {
		token := signer.Sign(given)
		got, err := signer.Verify(token)
		if err != nil || !got.Signed.Equal(at.Truncate(time.Second)) {
			t.Fatalf("%s: signed at %v (%v), want %v", name, got.Signed, err, at.Truncate(time.Second))
		}
		got.Signed, given.Signed = time.Time{}, time.Time{}
		if got != given {
			t.Errorf("%s: %+v, want %+v", name, got, given)
		}
		raw, _ := base64.RawURLEncoding.DecodeString(token)
		start := len(raw) - grantMAC - 8 - grantTime
		for i := range grantTime {
			changed := bytes.Clone(raw)
			changed[start+i] ^= 1
			if _, err := signer.Verify(base64.RawURLEncoding.EncodeToString(changed)); !errors.Is(err, ErrInvalidGrant) {
				t.Errorf("%s: byte %d of the time changed: got %v", name, i, err)
			}
		}
		// The time taken out, and the flag with it.
		cut := append(bytes.Clone(raw[:start]), raw[start+grantTime:]...)
		cut[32] &^= grantHasTime
		if _, err := signer.Verify(base64.RawURLEncoding.EncodeToString(cut)); !errors.Is(err, ErrInvalidGrant) {
			t.Errorf("%s: the time cut out: got %v", name, err)
		}
	}
	untimed := Grant{Version: accounts.ID{1, 2, 3}, User: accounts.ID{9, 8, 7}, Device: accounts.ID{4, 5, 6}}
	token := signer.Sign(untimed)
	if raw, _ := base64.RawURLEncoding.DecodeString(token); len(raw) != grantPayload+grantDevice+grantMAC {
		t.Errorf("a grant signed with no time is %d bytes", len(raw))
	}
	if got, err := signer.Verify(token); err != nil || got != untimed || !got.Signed.IsZero() {
		t.Errorf("a grant signed with no time: %+v %v", got, err)
	}
}

// The grants of a domain are its own: the signer they were taken from
// and the other domains refuse them, and it refuses theirs. Links and
// marks are the same in every domain.
func TestGrantsOfADomainAreItsOwn(t *testing.T) {
	t.Parallel()
	server := NewSigner([]byte("a secret of the server, 32 bytes"))
	signers := map[string]Signer{"the server": server, "dlna": server.Domain("dlna"), "other": server.Domain("other"),
		"another server's dlna": NewSigner([]byte("another server's secret, 32 byte")).Domain("dlna")}
	grant := Grant{Version: accounts.ID{1, 2, 3}, User: accounts.ID{9, 8, 7}, Relay: true, Signed: time.Unix(1_800_000_000, 0)}
	for signs, signer := range signers {
		token := signer.Sign(grant)
		for verifies, verifier := range signers {
			got, err := verifier.Verify(token)
			if signs == verifies && (err != nil || !got.Signed.Equal(grant.Signed) || got.User != grant.User) {
				t.Errorf("%s refuses its own grant: %+v %v", signs, got, err)
			}
			if signs != verifies && !errors.Is(err, ErrInvalidGrant) {
				t.Errorf("%s takes a grant of %s: %v", verifies, signs, err)
			}
		}
	}
	dlna := signers["dlna"]
	if dlna.Mark("purpose", "value") != server.Mark("purpose", "value") || dlna.Link("grant", "target") != server.Link("grant", "target") {
		t.Error("a domain signs marks or links of its own")
	}
}
