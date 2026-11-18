package accounts

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/logs"
)

// No line logs the settings, and the log would hide their secrets if one
// did: it knows a secret by what its name ends in, so a secret added to
// the settings under another kind of name fails here. The WireGuard
// configuration is a text of keys, each hidden by its own name.
func TestTheLogWouldHideEverySecretOfTheSettings(t *testing.T) {
	t.Parallel()
	var settings Settings
	for _, secret := range settings.secrets() {
		values := []string{"VALUE-of-" + secret.name}
		text := values[0]
		if secret.name == "torrentWireGuard" {
			values = []string{"WG-private-1", "WG-shared-2"}
			text = "[Interface]\nPrivateKey = " + values[0] + "\nAddress = 10.0.0.2/32\n[Peer]\nPresharedKey = " + values[1] + "\n"
		}
		body, err := json.Marshal(map[string]string{secret.name: text})
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range []string{secret.name + "=" + text, string(body), "settings=" + strings.ToUpper(secret.name[:1]) + secret.name[1:] + ":" + text} {
			redacted := logs.Redact(line)
			for _, value := range values {
				if strings.Contains(redacted, value) {
					t.Errorf("%s: the log would keep %q of %q", secret.name, value, line)
				}
			}
		}
	}
}
