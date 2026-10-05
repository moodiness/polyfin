package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/trackers"
)

// trackingServices stands in for Trakt (/trakt), MDBList (/mdblist) and
// PublicMetaDB (/publicmetadb): MDBList takes the key "good", refuses
// "bad" and fails for any other; Trakt hands out a code to any app but
// "unknown-id".
func trackingServices(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/trakt/oauth/device/code":
			var body struct {
				ClientID string `json:"client_id"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.ClientID == "unknown-id" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"error":"invalid_client","error_description":"client not found"}`)
				return
			}
			_, _ = io.WriteString(w, `{"device_code":"secret-device","user_code":"ABCD1234","verification_url":"https://trakt.example/activate",
				"expires_in":600,"interval":5}`)
		case "/trakt/oauth/device/token":
			w.WriteHeader(http.StatusBadRequest)
		case "/mdblist/user":
			switch r.URL.Query().Get("apikey") {
			case "good":
				_, _ = io.WriteString(w, `{"username":"mdblist-member"}`)
			case "bad":
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"error":"Invalid API key"}`)
			default:
				w.WriteHeader(http.StatusServiceUnavailable)
			}
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func newTrackingAPI(t *testing.T) testAPI {
	t.Helper()
	services := trackingServices(t)
	return newTestAPI(t, 10, func(options *Options, deps testDeps) {
		tracker := trackers.New(trackers.Options{DB: deps.pool, Settings: options.Accounts.Settings, Version: "test", Logger: options.Logger,
			URLs: map[string]string{trackers.Trakt: services + "/trakt", trackers.MDBList: services + "/mdblist", trackers.PublicMetaDB: services + "/publicmetadb"}})
		t.Cleanup(tracker.Close)
		options.Trackers = tracker
	})
}

func TestOwnTrackingFollowsTheContract(t *testing.T) {
	api := newTrackingAPI(t)
	administrator := api.signedIn("admin", true)
	member := api.signedIn("member", false)
	other := api.signedIn("other", false)
	if status, _, _ := api.browser().call(http.MethodGet, "/account/tracking", nil); status != http.StatusUnauthorized {
		t.Errorf("signed out: %d", status)
	}

	status, body, _ := member.call(http.MethodGet, "/account/tracking", nil)
	want := `{"services":[
		{"service":"trakt","connection":"code","available":false,"connected":false,"account":null,"connectedAt":null,"lastSentAt":null,"problem":null,"code":null},
		{"service":"simkl","connection":"code","available":false,"connected":false,"account":null,"connectedAt":null,"lastSentAt":null,"problem":null,"code":null},
		{"service":"mdblist","connection":"key","available":true,"connected":false,"account":null,"connectedAt":null,"lastSentAt":null,"problem":null,"code":null},
		{"service":"publicmetadb","connection":"key","available":true,"connected":false,"account":null,"connectedAt":null,"lastSentAt":null,"problem":null,"code":null}]}`
	var expected map[string]any
	_ = json.Unmarshal([]byte(want), &expected)
	if encoded, _ := json.Marshal(body); status != http.StatusOK || string(encoded) != mustCompact(t, expected) {
		t.Fatalf("listing: %d %s", status, encoded)
	}

	for _, check := range []struct {
		method, path string
		body         any
		status       int
		code         string
	}{
		{http.MethodPost, "/account/tracking/other", map[string]string{}, http.StatusNotFound, "not_found"},
		{http.MethodDelete, "/account/tracking/other", nil, http.StatusNotFound, "not_found"},
		{http.MethodPost, "/account/tracking/trakt", map[string]string{}, http.StatusConflict, "not_available"},
		{http.MethodPost, "/account/tracking/mdblist", map[string]string{"key": "bad"}, http.StatusBadRequest, "invalid_key"},
		{http.MethodPost, "/account/tracking/mdblist", map[string]string{"key": "  "}, http.StatusBadRequest, "invalid_key"},
		{http.MethodPost, "/account/tracking/mdblist", map[string]string{"key": "down"}, http.StatusBadGateway, "service_unreachable"},
	} {
		if status, body, _ := member.call(check.method, check.path, check.body); status != check.status || body["error"] != check.code {
			t.Errorf("%s %s: %d %v", check.method, check.path, status, body)
		}
	}

	status, body, _ = member.call(http.MethodPost, "/account/tracking/mdblist", map[string]string{"key": "good"})
	encoded, _ := json.Marshal(body)
	connectedAt, _ := body["connectedAt"].(string)
	if parsed, err := time.Parse(time.RFC3339, connectedAt); status != http.StatusOK || body["connected"] != true || body["account"] != "mdblist-member" ||
		body["service"] != "mdblist" || err != nil || parsed.Location() != time.UTC || strings.Contains(string(encoded), "good") {
		t.Errorf("connected: %d %s", status, encoded)
	}
	// Only the member's own.
	if _, body, _ := other.call(http.MethodGet, "/account/tracking", nil); body["services"].([]any)[2].(map[string]any)["connected"] != false {
		t.Errorf("another user sees %v", body)
	}

	// An app Trakt does not know is not an outage.
	settings := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en",
		"traktClientId": "unknown-id", "traktClientSecret": "trakt-secret"}
	if status, _, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK {
		t.Fatalf("settings: %d", status)
	}
	if status, body, _ := member.call(http.MethodPost, "/account/tracking/trakt", map[string]string{}); status != http.StatusConflict || body["error"] != "app_refused" {
		t.Errorf("refused app: %d %v", status, body)
	}

	// With the Trakt app saved, a code waits for the user.
	settings["traktClientId"] = "trakt-id"
	if status, _, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK {
		t.Fatalf("settings: %d", status)
	}
	status, body, _ = member.call(http.MethodPost, "/account/tracking/trakt", map[string]string{})
	code, _ := body["code"].(map[string]any)
	expires, _ := code["expiresAt"].(string)
	if parsed, err := time.Parse(time.RFC3339, expires); status != http.StatusOK || body["available"] != true || body["connected"] != false ||
		code["userCode"] != "ABCD1234" || code["verificationUrl"] != "https://trakt.example/activate" || err != nil || time.Until(parsed) < 9*time.Minute {
		t.Errorf("code: %d %v", status, body)
	}
	if _, body, _ := member.call(http.MethodGet, "/account/tracking", nil); body["services"].([]any)[0].(map[string]any)["code"] == nil {
		t.Errorf("the waiting code is not listed: %v", body)
	}
	_, all, _ := member.call(http.MethodGet, "/account/tracking", nil)
	if encoded, _ := json.Marshal(all); strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "good") {
		t.Errorf("a secret is listed: %s", encoded)
	}

	for _, service := range []string{"mdblist", "trakt", "publicmetadb"} {
		if status, body, _ := member.call(http.MethodDelete, "/account/tracking/"+service, nil); status != http.StatusNoContent {
			t.Errorf("disconnecting %s: %d %v", service, status, body)
		}
	}
	_, body, _ = member.call(http.MethodGet, "/account/tracking", nil)
	for _, service := range body["services"].([]any) {
		if entry := service.(map[string]any); entry["connected"] != false || entry["code"] != nil {
			t.Errorf("after disconnecting: %v", entry)
		}
	}
}

func mustCompact(t *testing.T, value map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
