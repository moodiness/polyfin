package admin

import (
	"maps"
	"net/http"
	"slices"
	"testing"
	"time"
)

// How often connected accounts are imported is one of the intervals
// offered, 6 hours by default; a PUT leaving it out keeps it, one not
// offered is refused with its code, and the next import of a connection
// follows it at once.
func TestSettingsImportInterval(t *testing.T) {
	t.Parallel()
	a := newAppConnectionsAPI(t)
	stremioSample(a.fake, "https://first.example/u/secret/manifest.json")
	administrator := a.signedIn("administrator", true)
	_, body, _ := administrator.call(http.MethodGet, "/settings", nil)
	bounds, _ := body["bounds"].(map[string]any)
	interval, _ := bounds["importEveryMinutes"].(map[string]any)
	choices, _ := interval["choices"].([]any)
	if body["importEveryMinutes"] != 360.0 || !slices.Equal(choices, []any{15.0, 30.0, 60.0, 120.0, 180.0, 360.0, 720.0, 1440.0}) {
		t.Errorf("default %v, bounds %v", body["importEveryMinutes"], interval)
	}

	// Bob stays connected to Stremio: his next import is 6 hours after
	// the first.
	bob := a.signedIn("bob", false)
	_, read, _ := bob.call(http.MethodPost, "/account/app-import/stremio/password",
		map[string]any{"email": "bob@example.com", "password": "bob's password", "stayConnected": true})
	if status, body, _ := bob.call(http.MethodPost, "/account/app-import", map[string]any{"key": read["key"], "profile": "1"}); status != http.StatusOK {
		t.Fatalf("the import: %d %v", status, body)
	}
	appImportDone(t, bob, "/account/app-import")
	nextAfterLast := func() time.Duration {
		t.Helper()
		c := connectionIdle(t, bob, "/account/app-import", "stremio")
		last, _ := c["lastImport"].(map[string]any)
		lastAt, _ := last["at"].(string)
		nextAt, _ := c["nextImportAt"].(string)
		from, err1 := time.Parse(time.RFC3339, lastAt)
		to, err2 := time.Parse(time.RFC3339, nextAt)
		if err1 != nil || err2 != nil {
			t.Fatalf("the connection: %v", c)
		}
		return to.Sub(from)
	}
	if got := nextAfterLast(); got != 6*time.Hour {
		t.Errorf("next import by default: %v after the last", got)
	}

	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	settings := maps.Clone(base)
	settings["importEveryMinutes"] = 15
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK || body["importEveryMinutes"] != 15.0 {
		t.Fatalf("saving 15 minutes: %d %v", status, body)
	}
	if got := nextAfterLast(); got != 15*time.Minute {
		t.Errorf("next import at 15 minutes: %v after the last", got)
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK || body["importEveryMinutes"] != 15.0 {
		t.Errorf("left out: %d %v", status, body)
	}
	for _, minutes := range []int{0, 14, 45, 1441} {
		settings := maps.Clone(base)
		settings["importEveryMinutes"] = minutes
		if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusBadRequest ||
			body["error"] != "invalid_import_every_minutes" {
			t.Errorf("%d minutes: %d %v", minutes, status, body)
		}
	}
	if got := a.store.Settings().ImportEveryMinutes; got != 15 {
		t.Errorf("saved: %d minutes", got)
	}
}
