package admin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/notifications"
)

// The server's targets are an administrator's to see and change, each
// user's own their own: the secret part of an address is never answered,
// a member may not choose health events nor a target on the local network,
// and reaches no other owner's target.
func TestNotificationTargetsBelongToTheirOwner(t *testing.T) {
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		o.Notifications = notifications.New(notifications.Options{DB: deps.pool, Accounts: o.Accounts, Version: "1.2.3",
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
		t.Cleanup(o.Notifications.Close)
	})
	administrator := api.signedIn("admin", true)
	member := api.signedIn("member", false)

	secret := "https://hooks.example.org/api/webhooks/42/a-secret-token"
	status, created, _ := administrator.call(http.MethodPost, "/notifications/targets", map[string]any{"kind": "discord", "name": "Family",
		"address": secret, "events": []string{"health_problem", "new_episode"}})
	if status != http.StatusCreated || created["address"] != "https://hooks.example.org" || created["problem"] != nil {
		t.Fatalf("adding a server target: %d %v", status, created)
	}
	if events, _ := json.Marshal(created["events"]); string(events) != `["new_episode","health_problem"]` {
		t.Errorf("events: %s", events)
	}
	_, listed, response := administrator.call(http.MethodGet, "/notifications", nil)
	encoded, _ := json.Marshal(listed)
	if strings.Contains(string(encoded), "a-secret-token") || len(listed["targets"].([]any)) != 1 {
		t.Errorf("listed: %s", encoded)
	}
	if got := listed["events"].([]any); len(got) != len(notifications.Events) {
		t.Errorf("an administrator's events: %v", got)
	}
	if response.Header.Get("Content-Type") != "application/json" {
		t.Errorf("content type: %s", response.Header.Get("Content-Type"))
	}

	// Members reach their own targets only.
	if status, _, _ := member.call(http.MethodGet, "/notifications", nil); status != http.StatusForbidden {
		t.Errorf("a member reading the server's targets: %d", status)
	}
	id := created["id"].(string)
	if status, _, _ := member.call(http.MethodPatch, "/account/notifications/targets/"+id, map[string]any{"enabled": false}); status != http.StatusNotFound {
		t.Errorf("a member changing the server's target as their own: %d", status)
	}
	_, own, _ := member.call(http.MethodGet, "/account/notifications", nil)
	var events []string
	for _, event := range own["events"].([]any) {
		events = append(events, event.(string))
	}
	if slices.Contains(events, "health_problem") || !slices.Contains(events, "new_episode") || len(own["targets"].([]any)) != 0 {
		t.Errorf("a member's notifications: %v", own)
	}
	// Addresses are written as public IP addresses, which are checked
	// without asking DNS.
	for _, refused := range []struct {
		body map[string]any
		code string
	}{
		{map[string]any{"kind": "webhook", "name": "Mine", "address": "https://203.0.113.10/mine", "events": []string{"health_solved"}}, "invalid_events"},
		{map[string]any{"kind": "webhook", "name": "Mine", "address": "http://127.0.0.1:8080/hook", "events": []string{"new_episode"}}, "private_target_address"},
		{map[string]any{"kind": "ntfy", "name": "Mine", "address": "https://203.0.113.10", "topic": "has spaces"}, "invalid_topic"},
		{map[string]any{"kind": "webhook", "name": "Mine", "address": "ftp://203.0.113.10/"}, "invalid_target_address"},
	} {
		if status, body, _ := member.call(http.MethodPost, "/account/notifications/targets", refused.body); status != http.StatusBadRequest || body["error"] != refused.code {
			t.Errorf("%v: %d %v, want %s", refused.body, status, body, refused.code)
		}
	}
	// An ntfy target's token is never answered, only that it has one.
	status, ntfy, _ := member.call(http.MethodPost, "/account/notifications/targets", map[string]any{"kind": "ntfy", "name": "Phone",
		"address": "https://203.0.113.10/", "topic": "member-alerts", "token": "tk_secret-token", "events": []string{"new_episode"}})
	if status != http.StatusCreated || ntfy["address"] != "https://203.0.113.10" || ntfy["tokenSet"] != true || ntfy["token"] != nil {
		t.Errorf("a member's ntfy target: %d %v", status, ntfy)
	}
	status, changed, _ := member.call(http.MethodPatch, "/account/notifications/targets/"+ntfy["id"].(string), map[string]any{"token": ""})
	if status != http.StatusOK || changed["tokenSet"] != false || changed["topic"] != "member-alerts" {
		t.Errorf("removing the token: %d %v", status, changed)
	}
	if status, _, _ := member.call(http.MethodDelete, "/account/notifications/targets/"+ntfy["id"].(string), nil); status != http.StatusNoContent {
		t.Errorf("deleting: %d", status)
	}
	// Without a server, an ntfy target uses the public one.
	status, public, _ := administrator.call(http.MethodPost, "/account/notifications/targets", map[string]any{"kind": "ntfy", "name": "Phone",
		"topic": "admin-alerts"})
	if status != http.StatusCreated || public["address"] != notifications.DefaultNtfyServer || public["tokenSet"] != false {
		t.Errorf("an ntfy target without a server: %d %v", status, public)
	}
}

// The public address links in messages start with is an http or https
// address, kept without its trailing slash.
func TestPublicAddressSetting(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("admin", true)
	with := func(address string) map[string]any {
		return map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "legacyAuthorization": false, "language": "en",
			"publicAddress": address}
	}
	for _, refused := range []string{"media.example.org", "https://media.example.org/?a=b", "https://user:pass@media.example.org"} {
		if status, body, _ := administrator.call(http.MethodPut, "/settings", with(refused)); status != http.StatusBadRequest ||
			body["error"] != "invalid_public_address" {
			t.Errorf("%q: %d %v", refused, status, body)
		}
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", with(" https://media.example.org/polyfin/ ")); status != http.StatusOK ||
		body["publicAddress"] != "https://media.example.org/polyfin" {
		t.Errorf("a public address: %d %v", status, body["publicAddress"])
	}
}
