package secrets_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/secrets"
)

// Without a key, Polyfin signs with the secret it stores, as it did. With
// one, it signs with HMAC-SHA256 of the stored secret under the key, for
// that purpose alone: 32 bytes that are not the stored ones, the same at
// each start, and others once the stored secret or the key changes. The
// derivation is written out here: changing it would end, at an update,
// everything a server with a key has signed.
func TestTheSigningSecretIsDerivedFromTheKeyAndTheStoredSecret(t *testing.T) {
	t.Parallel()
	stored := bytes.Repeat([]byte{1}, 32)
	if got := (*secrets.Box)(nil).SigningSecret(stored); !bytes.Equal(got, stored) {
		t.Errorf("without a key: %x, want the stored secret", got)
	}
	key := bytes.Repeat([]byte{7}, secrets.KeySize)
	box, err := secrets.New(key)
	if err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("polyfin.signing"))
	mac.Write(stored)
	want := mac.Sum(nil)
	got := box.SigningSecret(stored)
	if !bytes.Equal(got, want) || len(got) != 32 || bytes.Equal(got, stored) {
		t.Fatalf("with a key: %x, want %x", got, want)
	}
	if again := box.SigningSecret(stored); !bytes.Equal(again, got) {
		t.Errorf("derived again: %x, want %x", again, got)
	}
	if other := box.SigningSecret(bytes.Repeat([]byte{2}, 32)); bytes.Equal(other, got) {
		t.Error("another stored secret derives the same signing secret")
	}
	// The box keeps its own copy of the key.
	key[0] ^= 0xff
	if after := box.SigningSecret(stored); !bytes.Equal(after, got) {
		t.Error("the signing secret follows the bytes the box was made from")
	}
}

// Two servers that store the same secret, as a server and the copy made
// from its backup do, and have different keys, refuse each other's
// grants, links and marks, and take their own; so does one with a key and
// one without.
func TestServersWithOneStoredSecretAndDifferentKeysRefuseEachOthersGrants(t *testing.T) {
	t.Parallel()
	stored := bytes.Repeat([]byte{1}, 32)
	signers := map[string]playback.Signer{}
	for name, key := range map[string][]byte{
		"no key":      nil,
		"one key":     bytes.Repeat([]byte{7}, secrets.KeySize),
		"another key": bytes.Repeat([]byte{8}, secrets.KeySize),
	} {
		box, err := secrets.New(key)
		if err != nil {
			t.Fatal(err)
		}
		signers[name] = playback.NewSigner(box.SigningSecret(stored))
	}
	grant := playback.Grant{Version: accounts.ID{1, 2, 3}, User: accounts.ID{9, 8, 7}}
	for signs, signer := range signers {
		token := signer.Sign(grant)
		link := signer.Link(token, "https://source.example/segment.ts")
		mark := signer.Mark("purpose", "value")
		for verifies, verifier := range signers {
			got, err := verifier.Verify(token)
			own := signs == verifies
			if own && (err != nil || got != grant) {
				t.Errorf("the server with %s refuses its own grant: %+v %v", signs, got, err)
			}
			if !own && !errors.Is(err, playback.ErrInvalidGrant) {
				t.Errorf("the server with %s takes a grant of the one with %s: %v", verifies, signs, err)
			}
			if verifier.VerifyLink(token, "https://source.example/segment.ts", link) != own {
				t.Errorf("the server with %s, a link of the one with %s: want %v", verifies, signs, own)
			}
			if verifier.VerifyMark("purpose", "value", mark) != own {
				t.Errorf("the server with %s, a mark of the one with %s: want %v", verifies, signs, own)
			}
		}
	}
}
