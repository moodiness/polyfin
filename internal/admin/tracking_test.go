package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/trackers"
	"github.com/moodiness/polyfin/internal/userdata"
)

// trackingServices stands in for Trakt (/trakt), MDBList (/mdblist),
// PublicMetaDB (/publicmetadb) and Last.fm (/lastfm): MDBList takes the
// key "good", refuses "bad" and fails for any other; Trakt hands out a
// code to any app but "unknown-id"; Last.fm hands out a request token
// nobody allows.
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
		case "/mdblist/sync/playback":
			_, _ = io.WriteString(w, `[]`)
		case "/lastfm/2.0/":
			if r.URL.Query().Get("method") == "auth.getToken" {
				_, _ = io.WriteString(w, `{"token":"request-token"}`)
				return
			}
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":14,"message":"Unauthorized Token - This token has not been authorized"}`)
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
			URLs: map[string]string{trackers.Trakt: services + "/trakt", trackers.MDBList: services + "/mdblist", trackers.PublicMetaDB: services + "/publicmetadb",
				trackers.LastFM: services + "/lastfm/2.0/"},
			Titles:   library.New(deps.pool, deps.addons, deps.client, options.Logger, options.Accounts.Settings),
			UserData: userdata.New(deps.pool)})
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
		{"service":"trakt","connection":"code","music":false,"available":false,"connected":false,"account":null,"connectedAt":null,"lastSentAt":null,"problem":null,"code":null,"importHistory":false,"importing":false,"lastImport":null},
		{"service":"simkl","connection":"code","music":false,"available":false,"connected":false,"account":null,"connectedAt":null,"lastSentAt":null,"problem":null,"code":null,"importHistory":false,"importing":false,"lastImport":null},
		{"service":"mdblist","connection":"key","music":false,"available":true,"connected":false,"account":null,"connectedAt":null,"lastSentAt":null,"problem":null,"code":null,"importHistory":false,"importing":false,"lastImport":null},
		{"service":"publicmetadb","connection":"key","music":false,"available":true,"connected":false,"account":null,"connectedAt":null,"lastSentAt":null,"problem":null,"code":null,"importHistory":false,"importing":false,"lastImport":null},
		{"service":"lastfm","connection":"signin","music":true,"available":false,"connected":false,"account":null,"connectedAt":null,"lastSentAt":null,"problem":null,"code":null,"importHistory":false,"importing":false,"lastImport":null},
		{"service":"listenbrainz","connection":"key","music":true,"available":true,"connected":false,"account":null,"connectedAt":null,"lastSentAt":null,"problem":null,"code":null,"importHistory":false,"importing":false,"lastImport":null}]}`
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
		{http.MethodPost, "/account/tracking/lastfm", map[string]string{}, http.StatusConflict, "not_available"},
		{http.MethodPost, "/account/tracking/lastfm/key/reveal", nil, http.StatusBadRequest, "not_revealable"},
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

	// With Last.fm's API account saved, a sign-in waits for the user.
	settings["lastFmApiKey"], settings["lastFmSecret"] = "lastfm-key", "lastfm-shared"
	if status, _, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK {
		t.Fatalf("settings: %d", status)
	}
	status, body, _ = member.call(http.MethodPost, "/account/tracking/lastfm", map[string]string{})
	code, _ = body["code"].(map[string]any)
	if status != http.StatusOK || body["connection"] != "signin" || body["available"] != true || body["connected"] != false || code["userCode"] != "" ||
		code["verificationUrl"] != "https://www.last.fm/api/auth/?api_key=lastfm-key&token=request-token" {
		t.Errorf("sign-in: %d %v", status, body)
	}

	for _, service := range []string{"mdblist", "trakt", "publicmetadb", "lastfm"} {
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

func TestHistoryImportFollowsTheContract(t *testing.T) {
	api := newTrackingAPI(t)
	member := api.signedIn("member", false)
	other := api.signedIn("other", false)
	if status, body, _ := member.call(http.MethodPost, "/account/tracking/mdblist", map[string]string{"key": "good"}); status != http.StatusOK ||
		body["importHistory"] != false || body["importing"] != false || body["lastImport"] != nil {
		t.Fatalf("connected: %d %v", status, body)
	}
	for _, check := range []struct {
		method, path string
		body         any
		status       int
		code         string
	}{
		{http.MethodPatch, "/account/tracking/other", map[string]bool{"importHistory": true}, http.StatusNotFound, "not_found"},
		{http.MethodPost, "/account/tracking/other/import", nil, http.StatusNotFound, "not_found"},
		{http.MethodPatch, "/account/tracking/mdblist", map[string]string{}, http.StatusBadRequest, "invalid_request"},
		{http.MethodPatch, "/account/tracking/publicmetadb", map[string]bool{"importHistory": true}, http.StatusConflict, "not_connected"},
		{http.MethodPost, "/account/tracking/publicmetadb/import", nil, http.StatusConflict, "not_connected"},
		{http.MethodPost, "/account/tracking/mdblist/import", nil, http.StatusConflict, "import_off"},
		// The music services have no history.
		{http.MethodPatch, "/account/tracking/listenbrainz", map[string]bool{"importHistory": true}, http.StatusNotFound, "not_found"},
		{http.MethodPost, "/account/tracking/lastfm/import", nil, http.StatusNotFound, "not_found"},
	} {
		if status, body, _ := member.call(check.method, check.path, check.body); status != check.status || body["error"] != check.code {
			t.Errorf("%s %s: %d %v", check.method, check.path, status, body)
		}
	}

	if status, body, _ := member.call(http.MethodPatch, "/account/tracking/mdblist", map[string]bool{"importHistory": true}); status != http.StatusOK ||
		body["importHistory"] != true || body["service"] != "mdblist" {
		t.Fatalf("turned on: %d %v", status, body)
	}
	var last map[string]any
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, body, _ := member.call(http.MethodGet, "/account/tracking", nil)
		entry := body["services"].([]any)[2].(map[string]any)
		if entry["importing"] == false && entry["lastImport"] != nil {
			last = entry["lastImport"].(map[string]any)
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the import never ended: %v", entry)
		}
		time.Sleep(20 * time.Millisecond)
	}
	at, _ := last["at"].(string)
	if parsed, err := time.Parse(time.RFC3339, at); err != nil || parsed.Location() != time.UTC || last["played"] != 0.0 || last["resumed"] != 0.0 ||
		last["unmapped"] != 0.0 || last["problem"] != nil || len(last) != 5 {
		t.Errorf("last import: %v", last)
	}
	if status, body, _ := member.call(http.MethodPost, "/account/tracking/mdblist/import", nil); status != http.StatusAccepted || body["importHistory"] != true {
		t.Errorf("import now: %d %v", status, body)
	}
	if _, body, _ := other.call(http.MethodGet, "/account/tracking", nil); body["services"].([]any)[2].(map[string]any)["importHistory"] != false {
		t.Errorf("another user sees %v", body)
	}
	if status, body, _ := member.call(http.MethodPatch, "/account/tracking/mdblist", map[string]bool{"importHistory": false}); status != http.StatusOK ||
		body["importHistory"] != false {
		t.Errorf("turned off: %d %v", status, body)
	}
}
