package admin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
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
// played one movie and Bob another, to the server's API key jellyfinKey, to
// Alice's own key, which /Users/Me answers with her, and to Alice and Bob
// signed in with their passwords. A session reads its own user's items
// whatever user is asked, as some servers answer, and lists the users only
// for Alice, the administrator, as Jellyfin does.
func fakeJellyfin(t *testing.T) string {
	t.Helper()
	var mu sync.Mutex
	sessions := map[string]string{}
	passwords := map[string]string{"Alice": "alice's password", "Bob": "bob's password"}
	played := map[string]string{"jf-alice": `{"Id":"m1","Name":"One","Type":"Movie","ProviderIds":{"Imdb":"tt0000001"},
		"UserData":{"Played":true,"PlayCount":1,"LastPlayedDate":"2026-09-01T20:00:00.0000000Z"}}`,
		"jf-bob": `{"Id":"m2","Name":"Two","Type":"Movie","ProviderIds":{"Imdb":"tt0000002"},"UserData":{"Played":true,"PlayCount":1}}`}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodPost && r.URL.Path == "/Users/AuthenticateByName" {
			var body struct{ Username, Pw string }
			if json.NewDecoder(r.Body).Decode(&body) != nil || !strings.Contains(r.Header.Get("Authorization"), `DeviceId="`) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if password, ok := passwords[body.Username]; !ok || password != body.Pw {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			token := "session-" + strconv.Itoa(len(sessions)+1)
			sessions[token] = "jf-" + strings.ToLower(body.Username)
			_, _ = io.WriteString(w, `{"User":{"Id":"`+sessions[token]+`"},"AccessToken":"`+token+`"}`)
			return
		}
		if r.Method != http.MethodGet && r.URL.Path != "/Sessions/Logout" {
			t.Errorf("Jellyfin was sent %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Path == "/System/Info/Public" {
			_, _ = io.WriteString(w, `{"Id":"f1e2d3","ServerName":"Home","Version":"10.10.7"}`)
			return
		}
		auth := r.Header.Get("Authorization")
		token := strings.TrimSuffix(strings.TrimPrefix(auth, `MediaBrowser Token="`), `"`)
		session, signedIn := sessions[token]
		alicesKey := auth == `MediaBrowser Token="`+aliceJellyfinKey+`"`
		if auth != `MediaBrowser Token="`+jellyfinKey+`"` && !alicesKey && !signedIn {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		user := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/Users/"), "/Items")
		if signedIn {
			user = session
		}
		switch {
		case r.URL.Path == "/Sessions/Logout":
			delete(sessions, token)
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/Users" && signedIn && session != "jf-alice":
			w.WriteHeader(http.StatusForbidden)
		case r.URL.Path == "/Users":
			_, _ = io.WriteString(w, `[{"Id":"jf-alice","Name":"Alice","Policy":{"IsAdministrator":true}},
				{"Id":"jf-bob","Name":"Bob","Policy":{"IsHidden":true}},{"Id":"jf-carol","Name":"CAROL","Policy":{}}]`)
		case r.URL.Path == "/Users/Me" && signedIn:
			_, _ = io.WriteString(w, `{"Id":"`+session+`"}`)
		case r.URL.Path == "/Users/Me" && alicesKey:
			_, _ = io.WriteString(w, `{"Id":"jf-alice","Name":"Alice"}`)
		case r.URL.Path == "/Users/Me":
			// The server's key is no user's.
			w.WriteHeader(http.StatusBadRequest)
		case strings.HasSuffix(r.URL.Path, "/Items") && r.URL.Query().Get("Filters") == "IsPlayed" && played[user] != "":
			_, _ = io.WriteString(w, `{"Items":[`+played[user]+`],"TotalRecordCount":1}`)
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

	jellyfinImportDone(t, admin)
	aliceID, _ := accounts.ParseID(created[0].(map[string]any)["id"].(string))
	if !playedMovie(t, lib, data, aliceID, "tt0000001") {
		t.Error("Alice's movie is not played")
	}
}

// jellyfinImportDone waits for the import running to end, and fails unless
// it is done.
func jellyfinImportDone(t *testing.T, admin browser) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, body, _ := admin.call(http.MethodGet, "/jellyfin-import", nil)
		if current, _ := body["import"].(map[string]any); current != nil && current["state"] != "running" {
			if current["state"] != "done" {
				t.Fatalf("import: %v", current)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the import never ended: %v", body)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// playedMovie tells whether user marked played the movie of IMDb identifier
// imdb.
func playedMovie(t *testing.T, lib *library.Service, data *userdata.Store, user accounts.ID, imdb string) bool {
	t.Helper()
	found, err := lib.Resolve(t.Context(), []library.TitleRef{{IMDb: imdb}})
	if err != nil || len(found[0]) == 0 {
		t.Fatalf("%s: %v %v", imdb, found, err)
	}
	got, err := data.Get(t.Context(), user, []accounts.ID{found[0][0].ID})
	if err != nil {
		t.Fatal(err)
	}
	return got[found[0][0].ID].Played
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

// Signed in as a user, an import reads another user's watch data signed in
// as them, with their password. A wrong one is refused before anyone is
// created.
func TestJellyfinImportSignsInAsEachUser(t *testing.T) {
	jellyfin := fakeJellyfin(t)
	api, lib, data := jellyfinImportAPI(t)
	admin := api.signedIn("root", true)
	asAlice := map[string]any{"name": "Alice", "password": "alice's password"}
	if status, body, _ := admin.call(http.MethodPost, "/jellyfin-import/users",
		map[string]any{"address": jellyfin, "account": map[string]any{"name": "Alice", "password": "wrong"}}); status != http.StatusBadRequest ||
		body["error"] != "jellyfin_sign_in_refused" {
		t.Errorf("a wrong password: %d %v", status, body)
	}
	if status, body, _ := admin.call(http.MethodPost, "/jellyfin-import/users", map[string]any{"address": jellyfin, "account": asAlice}); status != http.StatusOK ||
		body["keyOwner"] != "jf-alice" {
		t.Fatalf("users as Alice: %d %v", status, body)
	}

	// Bob keeps his password in Polyfin.
	newUser := func(name, password string) map[string]any {
		return map[string]any{"name": name, "password": password}
	}
	start := map[string]any{"address": jellyfin, "account": asAlice, "users": []map[string]any{
		{"jellyfinId": "jf-alice", "create": newUser("Alice", "correct horse"), "watchData": true},
		{"jellyfinId": "jf-bob", "create": newUser("Bob", "bob's password"), "watchData": true, "jellyfinPassword": "wrong"},
	}}
	if status, body, _ := admin.call(http.MethodPost, "/jellyfin-import", start); status != http.StatusBadRequest ||
		body["error"] != "jellyfin_password_refused" || body["jellyfinId"] != "jf-bob" {
		t.Errorf("Bob's wrong password: %d %v", status, body)
	}
	if all, err := api.store.Users(t.Context()); err != nil || len(all) != 1 {
		t.Errorf("users created by a refused import: %v %v", all, err)
	}

	start["users"].([]map[string]any)[1]["jellyfinPassword"] = "bob's password"
	status, body, _ := admin.call(http.MethodPost, "/jellyfin-import", start)
	created, _ := body["created"].([]any)
	if status != http.StatusOK || len(created) != 2 {
		t.Fatalf("start: %d %v", status, body)
	}
	raw, _ := json.Marshal(body)
	for _, password := range []string{"alice's password", "bob's password"} {
		if strings.Contains(string(raw), password) {
			t.Errorf("the answer holds %q", password)
		}
	}
	jellyfinImportDone(t, admin)
	aliceID, _ := accounts.ParseID(created[0].(map[string]any)["id"].(string))
	bobID, _ := accounts.ParseID(created[1].(map[string]any)["id"].(string))
	if !playedMovie(t, lib, data, aliceID, "tt0000001") || playedMovie(t, lib, data, aliceID, "tt0000002") {
		t.Error("Alice's movies")
	}
	if !playedMovie(t, lib, data, bobID, "tt0000002") || playedMovie(t, lib, data, bobID, "tt0000001") {
		t.Error("Bob's movies")
	}
}

// A user imports their own watch history under My account: signed in as
// themselves on the server, they get their own data, and see only their
// own import, which the administrator sees too. The password is never
// answered back, a refused one says so, and with the setting off the
// section is gone and the import refused.
func TestUsersImportTheirOwnWatchHistory(t *testing.T) {
	jellyfin := fakeJellyfin(t)
	api, lib, data := jellyfinImportAPI(t)
	admin := api.signedIn("root", true)
	bob, carol := api.signedIn("bob", false), api.signedIn("carol", false)
	asBob := map[string]any{"kind": "jellyfin", "address": jellyfin, "name": "Bob", "password": "wrong"}

	if status, body, _ := bob.call(http.MethodPost, "/account/server-import", asBob); status != http.StatusBadRequest ||
		body["error"] != "jellyfin_sign_in_refused" {
		t.Errorf("a wrong password: %d %v", status, body)
	}
	asBob["password"] = "bob's password"
	status, body, _ := bob.call(http.MethodPost, "/account/server-import", asBob)
	if status != http.StatusOK || body["import"] == nil {
		t.Fatalf("Bob's import: %d %v", status, body)
	}
	if raw, _ := json.Marshal(body); strings.Contains(string(raw), "bob's password") {
		t.Error("the answer holds Bob's password")
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, body, _ := bob.call(http.MethodGet, "/account/server-import", nil)
		if current, _ := body["import"].(map[string]any); body["enabled"] == true && current != nil && current["state"] != "running" {
			if current["state"] != "done" {
				t.Fatalf("Bob's import: %v", current)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Bob's import never ended: %v", body)
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, session, _ := bob.call(http.MethodGet, "/session", nil)
	bobID, err := accounts.ParseID(session["user"].(map[string]any)["id"].(string))
	if err != nil {
		t.Fatalf("Bob's session: %v %v", session, err)
	}
	if !playedMovie(t, lib, data, bobID, "tt0000002") || playedMovie(t, lib, data, bobID, "tt0000001") {
		t.Error("Bob's movies")
	}
	if _, body, _ := carol.call(http.MethodGet, "/account/server-import", nil); body["enabled"] != true || body["import"] != nil {
		t.Errorf("Carol sees %v", body)
	}
	_, body, _ = admin.call(http.MethodGet, "/jellyfin-import", nil)
	if current, _ := body["import"].(map[string]any); current == nil || current["own"] != true ||
		current["startedBy"].(map[string]any)["name"] != "bob" {
		t.Errorf("the administrator sees %v", body)
	}

	settings := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en", "serverImports": false}
	if status, body, _ := admin.call(http.MethodPut, "/settings", settings); status != http.StatusOK || body["serverImports"] != false {
		t.Fatalf("turning the setting off: %d %v", status, body)
	}
	if _, body, _ := bob.call(http.MethodGet, "/account/server-import", nil); body["enabled"] != false || body["import"] != nil {
		t.Errorf("with the setting off: %v", body)
	}
	if status, body, _ := bob.call(http.MethodPost, "/account/server-import", asBob); status != http.StatusForbidden ||
		body["error"] != "server_imports_disabled" {
		t.Errorf("importing with the setting off: %d %v", status, body)
	}
}
