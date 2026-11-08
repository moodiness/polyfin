package playback

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

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
