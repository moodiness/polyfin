package playback

import (
	"errors"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

func TestGrantsRoundTripAndResistForgery(t *testing.T) {
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
