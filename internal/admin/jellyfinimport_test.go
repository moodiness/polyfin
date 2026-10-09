package admin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/jellyfinimport"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
)

const (
	jellyfinKey = "fedcba9876543210fedcba9876543210"
	// aliceJellyfinKey is Alice's own key, which reads her data only.
	aliceJellyfinKey = "alices-own-jellyfin-key"
)

// fakeJellyfin answers as a Jellyfin server with three users, Alice having
// played one movie, to the server's API key jellyfinKey and to Alice's own
// key, which /Users/Me answers with her.
func fakeJellyfin(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			t.Errorf("Jellyfin was sent %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Path == "/System/Info/Public" {
			_, _ = io.WriteString(w, `{"Id":"f1e2d3","ServerName":"Home","Version":"10.10.7"}`)
			return
		}
		auth := r.Header.Get("Authorization")
		alicesKey := auth == `MediaBrowser Token="`+aliceJellyfinKey+`"`
		if auth != `MediaBrowser Token="`+jellyfinKey+`"` && !alicesKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/Users":
			_, _ = io.WriteString(w, `[{"Id":"jf-alice","Name":"Alice","Policy":{"IsAdministrator":true}},
				{"Id":"jf-bob","Name":"Bob","Policy":{"IsHidden":true}},{"Id":"jf-carol","Name":"CAROL","Policy":{}}]`)
		case r.URL.Path == "/Users/Me" && alicesKey:
			_, _ = io.WriteString(w, `{"Id":"jf-alice","Name":"Alice"}`)
		case r.URL.Path == "/Users/Me":
			// The server's key is no user's.
			w.WriteHeader(http.StatusBadRequest)
		case r.URL.Path == "/Users/jf-alice/Items" && r.URL.Query().Get("Filters") == "IsPlayed":
			_, _ = io.WriteString(w, `{"Items":[{"Id":"m1","Name":"One","Type":"Movie","ProviderIds":{"Imdb":"tt0000001"},
				"UserData":{"Played":true,"PlayCount":1,"LastPlayedDate":"2026-09-01T20:00:00.0000000Z"}}],"TotalRecordCount":1}`)
		case strings.HasSuffix(r.URL.Path, "/Items"):
			_, _ = io.WriteString(w, `{"Items":[],"TotalRecordCount":0}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// jellyfinImportAPI serves the admin API with Jellyfin imports, and returns
// the library and user data they write to.
func jellyfinImportAPI(t *testing.T) (testAPI, *library.Service, *userdata.Store) {
	t.Helper()
	var lib *library.Service
	var data *userdata.Store
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		lib = library.New(deps.pool, deps.addons, deps.client, logger, o.Accounts.Settings)
		data = userdata.New(deps.pool)
		o.JellyfinImport = jellyfinimport.New(jellyfinimport.Options{Settings: o.Accounts.Settings, Titles: lib, UserData: data,
			Version: "1.2.3", Logger: logger})
		t.Cleanup(o.JellyfinImport.Close)
	})
	return api, lib, data
}

func TestJellyfinImportCreatesTheChosenUsersAndImportsTheirWatchData(t *testing.T) {
	jellyfin := fakeJellyfin(t)
	api, lib, data := jellyfinImportAPI(t)
	admin := api.signedIn("root", true)
	member := api.signedIn("member", false)
	carol, err := api.store.CreateUser(t.Context(), accounts.NewUser{Name: "carol", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	connection := map[string]any{"address": jellyfin, "apiKey": jellyfinKey}

	if status, _, _ := member.call(http.MethodPost, "/jellyfin-import/users", connection); status != http.StatusForbidden {
		t.Errorf("a member: %d", status)
	}
	if status, body, _ := admin.call(http.MethodPost, "/jellyfin-import/users", map[string]any{"address": jellyfin, "apiKey": "wrong"}); status != http.StatusBadRequest ||
		body["error"] != "jellyfin_key_refused" {
		t.Errorf("a wrong key: %d %v", status, body)
	}
	status, body, _ := admin.call(http.MethodPost, "/jellyfin-import/users", connection)
	users, _ := body["users"].([]any)
	if status != http.StatusOK || len(users) != 3 || body["keyOwner"] != nil {
		t.Fatalf("users: %d %v", status, body)
	}
	alice, carolListed := users[0].(map[string]any), users[2].(map[string]any)
	if alice["isAdministrator"] != true || alice["userId"] != nil || carolListed["userId"] != carol.ID.String() {
		t.Errorf("users: %v", users)
	}

	// One password breaks the rules: no user is created.
	newUser := func(name, password string, administrator bool) map[string]any {
		return map[string]any{"name": name, "password": password, "isAdministrator": administrator}
	}
	start := map[string]any{"address": jellyfin, "apiKey": jellyfinKey, "users": []map[string]any{
		{"jellyfinId": "jf-alice", "create": newUser("Alice", "correct horse", true), "watchData": true},
		{"jellyfinId": "jf-bob", "create": newUser("Bob", "short", false), "watchData": false},
		{"jellyfinId": "jf-carol", "userId": carol.ID.String(), "watchData": true},
	}}
	if status, body, _ := admin.call(http.MethodPost, "/jellyfin-import", start); status != http.StatusBadRequest ||
		body["error"] != "invalid_password" || body["jellyfinId"] != "jf-bob" {
		t.Errorf("a short password: %d %v", status, body)
	}
	if all, err := api.store.Users(t.Context()); err != nil || len(all) != 3 {
		t.Errorf("users created by a refused import: %v %v", all, err)
	}

	start["users"].([]map[string]any)[1]["create"] = newUser("Bob", "another horse", false)
	status, body, _ = admin.call(http.MethodPost, "/jellyfin-import", start)
	created, _ := body["created"].([]any)
	imported, _ := body["import"].(map[string]any)
	if status != http.StatusOK || len(created) != 2 || imported == nil {
		t.Fatalf("start: %d %v", status, body)
	}
	if first := created[0].(map[string]any); first["name"] != "Alice" || first["isAdministrator"] != true {
		t.Errorf("created: %v", created)
	}
	if raw, _ := json.Marshal(body); strings.Contains(string(raw), jellyfinKey) {
		t.Error("the answer holds the API key")
	}
	// Bob's account is created, without importing his watch data.
	if importedUsers := imported["users"].([]any); len(importedUsers) != 2 || importedUsers[0].(map[string]any)["jellyfinName"] != "Alice" ||
		importedUsers[1].(map[string]any)["userId"] != carol.ID.String() {
		t.Errorf("import: %v", imported)
	}
	if status, _, _ := api.browser().call(http.MethodPost, "/session", map[string]string{"name": "Bob", "password": "another horse"}); status != http.StatusOK {
		t.Errorf("Bob signs in: %d", status)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		_, body, _ = admin.call(http.MethodGet, "/jellyfin-import", nil)
		if current, _ := body["import"].(map[string]any); current != nil && current["state"] != "running" {
			if current["state"] != "done" {
				t.Fatalf("import: %v", current)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the import never ended: %v", body)
		}
		time.Sleep(10 * time.Millisecond)
	}
	aliceID, _ := accounts.ParseID(created[0].(map[string]any)["id"].(string))
	found, err := lib.Resolve(t.Context(), []library.TitleRef{{IMDb: "tt0000001"}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := data.Get(t.Context(), aliceID, []accounts.ID{found[0][0].ID})
	if d := got[found[0][0].ID]; err != nil || !d.Played {
		t.Errorf("Alice's movie: %+v %v", d, err)
	}
}

// A user's own key reads that user's watch data only: the admin API names
// its owner, and refuses another user's watch data before creating anyone.
func TestJellyfinImportWithAUserKeyImportsOnlyItsOwner(t *testing.T) {
	jellyfin := fakeJellyfin(t)
	api, _, _ := jellyfinImportAPI(t)
	admin := api.signedIn("root", true)
	connection := map[string]any{"address": jellyfin, "apiKey": aliceJellyfinKey}
	if status, body, _ := admin.call(http.MethodPost, "/jellyfin-import/users", connection); status != http.StatusOK || body["keyOwner"] != "jf-alice" {
		t.Fatalf("users with Alice's key: %d %v", status, body)
	}
	newUser := func(name string) map[string]any {
		return map[string]any{"name": name, "password": "correct horse"}
	}
	start := map[string]any{"address": jellyfin, "apiKey": aliceJellyfinKey, "users": []map[string]any{
		{"jellyfinId": "jf-alice", "create": newUser("Alice"), "watchData": true},
		{"jellyfinId": "jf-bob", "create": newUser("Bob"), "watchData": true},
	}}
	if status, body, _ := admin.call(http.MethodPost, "/jellyfin-import", start); status != http.StatusBadRequest ||
		body["error"] != "jellyfin_key_owner_only" || body["jellyfinId"] != "jf-bob" {
		t.Errorf("Bob's watch data with Alice's key: %d %v", status, body)
	}
	if all, err := api.store.Users(t.Context()); err != nil || len(all) != 1 {
		t.Errorf("users created by a refused import: %v %v", all, err)
	}
	// Bob's account alone comes over.
	start["users"].([]map[string]any)[1]["watchData"] = false
	status, body, _ := admin.call(http.MethodPost, "/jellyfin-import", start)
	created, _ := body["created"].([]any)
	imported, _ := body["import"].(map[string]any)
	if status != http.StatusOK || len(created) != 2 || imported == nil || len(imported["users"].([]any)) != 1 {
		t.Errorf("Alice's watch data and Bob's account: %d %v", status, body)
	}
}
