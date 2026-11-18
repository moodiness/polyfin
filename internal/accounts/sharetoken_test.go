package accounts

import (
	"strings"
	"testing"
)

// A share token is told by its shape before any link is read: an ID then
// a secret, in base64 for URLs.
func TestShareTokensAreToldByTheirShape(t *testing.T) {
	t.Parallel()
	token := linkToken(ID{1}, make([]byte, linkSecretSize))
	if !WellFormedShareToken(token) {
		t.Errorf("a token as links bear them is not well formed: %q", token)
	}
	for name, text := range map[string]string{
		"nothing":            "",
		"a word":             "x",
		"a character short":  token[1:],
		"a character longer": token + "A",
		"padded":             token + "=",
		"another alphabet":   strings.Repeat("+", len(token)),
	} {
		if WellFormedShareToken(text) {
			t.Errorf("%s is well formed: %q", name, text)
		}
	}
}
