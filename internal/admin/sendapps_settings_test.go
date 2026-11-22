package admin

import (
	"maps"
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Send apps to the source is owners only on a new server: only an
// administrator who saves another choice lets every app be sent to the
// sources' addresses, or none. A save that leaves it out, as a page or
// script older than the setting sends, keeps it as it is; a choice the
// server does not know is refused.
func TestSendAppsToTheSourceIsOwnersOnlyUntilSaved(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); body["sendToSource"] != accounts.SendToSourceOwners {
		t.Errorf("default: %v", body["sendToSource"])
	}
	if got := api.store.Settings().SendToSource; got != accounts.SendToSourceOwners {
		t.Errorf("stored by default: %q", got)
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "legacyAuthorization": false, "language": "en"}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || body["sendToSource"] != accounts.SendToSourceOwners {
		t.Errorf("a save without it: %d %v", status, body["sendToSource"])
	}
	for _, choice := range []string{accounts.SendToSourceEveryone, accounts.SendToSourceNone, accounts.SendToSourceOwners} {
		saved := maps.Clone(base)
		saved["sendToSource"] = choice
		if status, body, _ := administrator.call(http.MethodPut, "/settings", saved); status != http.StatusOK || body["sendToSource"] != choice {
			t.Fatalf("saving %s: %d %v", choice, status, body)
		}
		if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || body["sendToSource"] != choice {
			t.Errorf("a save without it, once %s: %d %v", choice, status, body["sendToSource"])
		}
		if got := api.store.Settings().SendToSource; got != choice {
			t.Errorf("stored once saved %s: %q", choice, got)
		}
	}
	unknown := maps.Clone(base)
	unknown["sendToSource"] = "admins"
	if status, body, _ := administrator.call(http.MethodPut, "/settings", unknown); status != http.StatusBadRequest || body["error"] != "invalid_send_to_source" {
		t.Errorf("an unknown choice: %d %v", status, body)
	}
	member := api.signedIn("member", false)
	none := maps.Clone(base)
	none["sendToSource"] = accounts.SendToSourceNone
	if status, _, _ := member.call(http.MethodPut, "/settings", none); status != http.StatusForbidden {
		t.Errorf("a member's save: %d", status)
	}
	if got := api.store.Settings().SendToSource; got != accounts.SendToSourceOwners {
		t.Errorf("after a member's save: %q", got)
	}
}
