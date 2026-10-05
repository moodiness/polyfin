package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/trackers"
)

// revealed returns the names of the reveals in the activity log.
func revealed(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(t.Context(), "SELECT name || ' ' || coalesce(short_overview, '') FROM activity_log WHERE type = 'SecretRevealed' ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	return names
}

// Administrators read the server's saved secrets again on demand, which
// the activity log records without the value; no one else can.
func TestServerSecretsAreRevealedToAdministratorsOnly(t *testing.T) {
	var pool *pgxpool.Pool
	api := newTestAPI(t, 10, func(_ *Options, deps testDeps) { pool = deps.pool })
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	settings := api.store.Settings()
	settings.PublicMetaDBKey, settings.TraktClientID, settings.TraktClientSecret = "pm-revealed-key", "trakt-id", "trakt-revealed-secret"
	if _, err := api.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}

	for name, want := range map[string]string{"publicMetaDbKey": "pm-revealed-key", "traktClientSecret": "trakt-revealed-secret"} {
		status, body, response := administrator.call(http.MethodPost, "/settings/secrets/"+name+"/reveal", nil)
		if status != http.StatusOK || body["value"] != want || response.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("%s: %d %v %q", name, status, body, response.Header.Get("Cache-Control"))
		}
	}
	for path, want := range map[string]string{"/settings/secrets/theIntroDbKey/reveal": "not_set", "/settings/secrets/customCss/reveal": "not_found"} {
		if status, body, _ := administrator.call(http.MethodPost, path, nil); status != http.StatusNotFound || body["error"] != want {
			t.Errorf("%s: %d %v", path, status, body)
		}
	}
	if status, body, _ := member.call(http.MethodPost, "/settings/secrets/publicMetaDbKey/reveal", nil); status != http.StatusForbidden || body["value"] != nil {
		t.Errorf("member: %d %v", status, body)
	}
	if status, _, _ := api.browser().call(http.MethodPost, "/settings/secrets/publicMetaDbKey/reveal", nil); status != http.StatusUnauthorized {
		t.Errorf("signed out: %d", status)
	}
	// Reading them is a POST: a GET is not routed.
	if status, body, _ := administrator.call(http.MethodGet, "/settings/secrets/publicMetaDbKey/reveal", nil); status == http.StatusOK || body["value"] != nil {
		t.Errorf("GET: %d %v", status, body)
	}

	log := revealed(t, pool)
	if len(log) != 2 || !strings.Contains(strings.Join(log, "\n"), "administrator revealed the PublicMetaDB key") ||
		!strings.Contains(strings.Join(log, "\n"), "administrator revealed the Trakt client secret") {
		t.Errorf("activity log: %q", log)
	}
	if strings.Contains(strings.Join(log, "\n"), "revealed-") {
		t.Error("the activity log holds a value")
	}
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); func() bool {
		encoded, _ := json.Marshal(body)
		return strings.Contains(string(encoded), "revealed-")
	}() {
		t.Error("the settings hold a secret")
	}
}

// Each user reads their own MDBList and PublicMetaDB keys again; the
// tokens of Trakt and Simkl are never handed out.
func TestOwnTrackingKeysAreRevealedToTheirOwnerOnly(t *testing.T) {
	services := trackingServices(t)
	var pool *pgxpool.Pool
	api := newTestAPI(t, 10, func(options *Options, deps testDeps) {
		pool = deps.pool
		tracker := trackers.New(trackers.Options{DB: deps.pool, Settings: options.Accounts.Settings, Version: "test", Logger: options.Logger,
			URLs: map[string]string{trackers.Trakt: services + "/trakt", trackers.MDBList: services + "/mdblist", trackers.PublicMetaDB: services + "/publicmetadb"}})
		t.Cleanup(tracker.Close)
		options.Trackers = tracker
	})
	member := api.signedIn("member", false)
	other := api.signedIn("other", false)
	if status, body, _ := member.call(http.MethodPost, "/account/tracking/mdblist/key/reveal", nil); status != http.StatusNotFound || body["error"] != "not_set" {
		t.Errorf("not connected: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPost, "/account/tracking/mdblist", map[string]string{"key": "good"}); status != http.StatusOK {
		t.Fatalf("connecting: %d %v", status, body)
	}
	status, body, response := member.call(http.MethodPost, "/account/tracking/mdblist/key/reveal", nil)
	if status != http.StatusOK || body["value"] != "good" || response.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("own key: %d %v", status, body)
	}
	if status, body, _ := other.call(http.MethodPost, "/account/tracking/mdblist/key/reveal", nil); status != http.StatusNotFound || body["value"] != nil {
		t.Errorf("another user: %d %v", status, body)
	}
	// A Trakt connection holds tokens, which are never revealed.
	if _, err := pool.Exec(t.Context(), `INSERT INTO tracking_connections (user_id, service, token, refresh_token, connected_at)
		SELECT id, 'trakt', 'trakt-access', 'trakt-refresh', now() FROM users WHERE name = 'member'`); err != nil {
		t.Fatal(err)
	}
	for _, service := range []string{"trakt", "simkl"} {
		if status, body, _ := member.call(http.MethodPost, "/account/tracking/"+service+"/key/reveal", nil); status != http.StatusBadRequest ||
			body["error"] != "not_revealable" || body["value"] != nil {
			t.Errorf("%s: %d %v", service, status, body)
		}
	}
	if status, body, _ := member.call(http.MethodPost, "/account/tracking/other/key/reveal", nil); status != http.StatusNotFound || body["error"] != "not_found" {
		t.Errorf("unknown service: %d %v", status, body)
	}
	if status, _, _ := api.browser().call(http.MethodPost, "/account/tracking/mdblist/key/reveal", nil); status != http.StatusUnauthorized {
		t.Errorf("signed out: %d", status)
	}
	if log := revealed(t, pool); len(log) != 1 || !strings.HasPrefix(log[0], "member revealed their MDBList API key") {
		t.Errorf("activity log: %q", log)
	}
}

// Health tells whether keys are stored unencrypted, and which the key
// cannot decrypt.
func TestHealthReportsStoredSecrets(t *testing.T) {
	var pool *pgxpool.Pool
	var box *secrets.Box
	api := newTestAPI(t, 10, func(options *Options, deps testDeps) {
		pool = deps.pool
		options.Health.Secrets = func(ctx context.Context) (secrets.Report, error) { return box.Inspect(ctx, deps.pool) }
	})
	administrator := api.signedIn("administrator", true)
	stored := func() map[string]any {
		t.Helper()
		_, body, _ := administrator.call(http.MethodGet, "/health", nil)
		report, _ := body["secrets"].(map[string]any)
		return report
	}
	if got := stored(); got["encrypted"] != false || got["plaintext"] != float64(0) || len(got["unreadable"].([]any)) != 0 {
		t.Errorf("nothing stored: %v", got)
	}
	settings := api.store.Settings()
	settings.PublicMetaDBKey = "pm-plain"
	if _, err := api.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	if got := stored(); got["encrypted"] != false || got["plaintext"] != float64(1) {
		t.Errorf("a key stored unencrypted: %v", got)
	}
	// A key sealed with another key than the server's.
	key, _ := secrets.ParseKey(strings.Repeat("ab", 32))
	other, _ := secrets.New(key)
	if _, err := pool.Exec(t.Context(), "UPDATE settings SET theintrodb_key = $1", other.Seal("tidb-lost")); err != nil {
		t.Fatal(err)
	}
	got := stored()
	unreadable, _ := got["unreadable"].([]any)
	if len(unreadable) != 1 || unreadable[0].(map[string]any)["setting"] != "theIntroDbKey" {
		t.Errorf("an unreadable key: %v", got)
	}
	if encoded, _ := json.Marshal(got); strings.Contains(string(encoded), "pm-plain") || strings.Contains(string(encoded), "enc:v1") {
		t.Errorf("health holds a secret: %s", encoded)
	}
}
