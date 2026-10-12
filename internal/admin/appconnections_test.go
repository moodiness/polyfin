package admin

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/appimport"
	"github.com/moodiness/polyfin/internal/appimport/nuvio"
	"github.com/moodiness/polyfin/internal/appimport/nuvio/nuviotest"
	stremioimport "github.com/moodiness/polyfin/internal/appimport/stremio"
	"github.com/moodiness/polyfin/internal/appimport/stremio/stremiotest"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/userdata"
)

// syncBuffer is a log written from several goroutines.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// appConnectionsAPI is the admin API with imports from apps that keep
// connections, sealed, the Stremio accounts on a fake Stremio, logging
// everything to log.
type appConnectionsAPI struct {
	testAPI
	lib       *library.Service
	data      *userdata.Store
	installed *addons.Store
	fake      *stremiotest.Fake
	log       *syncBuffer
	pool      *pgxpool.Pool
}

func newAppConnectionsAPI(t *testing.T) appConnectionsAPI {
	t.Helper()
	a := appConnectionsAPI{fake: stremiotest.New(t), log: &syncBuffer{}}
	box, err := secrets.New(bytes.Repeat([]byte{9}, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	a.testAPI = newTestAPI(t, 10, func(o *Options, deps testDeps) {
		logger := slog.New(slog.NewTextHandler(a.log, &slog.HandlerOptions{Level: slog.LevelDebug}))
		o.Logger = logger
		a.lib = library.New(deps.pool, deps.addons, deps.client, logger, o.Accounts.Settings)
		a.data, a.installed, a.pool = userdata.New(deps.pool), deps.addons, deps.pool
		o.Nuvio = nuvio.New(nuvio.Options{Version: "1.2.3"})
		o.Stremio = stremioimport.New(stremioimport.Options{Metas: stremiotest.SampleMetas(), Version: "1.2.3", API: a.fake.API,
			Link: a.fake.Link})
		o.AppImport = appimport.New(appimport.Options{Settings: o.Accounts.Settings, Titles: a.lib, UserData: a.data, Addons: deps.addons,
			Logger: logger, DB: deps.pool, Secrets: box,
			Sources: map[string]appimport.Source{nuvio.App: o.Nuvio, stremioimport.App: o.Stremio}})
		t.Cleanup(o.AppImport.Close)
	})
	return a
}

// connectionOf is the connection to app path lists, nil when none.
func connectionOf(t *testing.T, b browser, path, app string) map[string]any {
	t.Helper()
	_, body, _ := b.call(http.MethodGet, path, nil)
	list, ok := body["connections"].([]any)
	if !ok {
		t.Fatalf("no connections: %v", body)
	}
	for _, c := range list {
		if c.(map[string]any)["app"] == app {
			return c.(map[string]any)
		}
	}
	return nil
}

// connectionIdle waits for the import of the connection to app path
// lists to end, and returns the connection.
func connectionIdle(t *testing.T, b browser, path, app string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		c := connectionOf(t, b, path, app)
		if c != nil && c["importing"] == false {
			return c
		}
		if time.Now().After(deadline) {
			t.Fatalf("the import of %s never ended: %v", app, c)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// checkSecrets fails the test when a sealed sign-in is stored in plain, or
// the log holds what one keeps.
func (a appConnectionsAPI) checkSecrets(t *testing.T, kept ...string) {
	t.Helper()
	rows, err := a.pool.Query(t.Context(), "SELECT sign_in FROM app_connections")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var signIn string
		if err := rows.Scan(&signIn); err != nil {
			t.Fatal(err)
		}
		if !secrets.Sealed(signIn) {
			t.Errorf("a sign-in is stored unsealed: %q", signIn)
		}
	}
	for _, secret := range kept {
		if secret != "" && strings.Contains(a.log.String(), secret) {
			t.Errorf("the log holds %q", secret)
		}
	}
}

// noSecretIn fails the test when answer holds one of kept.
func noSecretIn(t *testing.T, answer any, kept ...string) {
	t.Helper()
	raw, _ := json.Marshal(answer)
	for _, secret := range append(kept, "issued-key", "refresh-token", "access-token") {
		if secret != "" && strings.Contains(string(raw), secret) {
			t.Errorf("the answer holds %q: %s", secret, raw)
		}
	}
}

func TestUsersStayConnectedToStremio(t *testing.T) {
	t.Parallel()
	a := newAppConnectionsAPI(t)
	stremioSample(a.fake, "https://first.example/u/secret/manifest.json")
	bob := a.signedIn("bob", false)
	bobID := a.userID("bob")

	// Bob signs in with his password to stay connected, and imports.
	status, read, _ := bob.call(http.MethodPost, "/account/app-import/stremio/password",
		map[string]any{"email": "bob@example.com", "password": "bob's password", "stayConnected": true})
	if status != http.StatusOK || read["stayConnected"] != true {
		t.Fatalf("signing in: %d %v", status, read)
	}
	noSecretIn(t, read, "bob's password")
	if status, body, _ := bob.call(http.MethodPost, "/account/app-import", map[string]any{"key": read["key"], "profile": "1"}); status != http.StatusOK {
		t.Fatalf("the import: %d %v", status, body)
	}
	appImportDone(t, bob, "/account/app-import")
	c := connectionIdle(t, bob, "/account/app-import", "stremio")
	last, _ := c["lastImport"].(map[string]any)
	if c["account"] != "bob@example.com" || c["problem"] != nil || c["paused"] != false || c["nextImportAt"] == nil || last == nil ||
		last["played"] != 5.0 || c["connectedBy"].(map[string]any)["name"] != "bob" {
		t.Fatalf("the connection: %v", c)
	}
	_, answer, _ := bob.call(http.MethodGet, "/account/app-import", nil)
	noSecretIn(t, answer)
	// The session Polyfin opened stays open.
	if a.fake.Open() != 1 {
		t.Errorf("%d sessions open", a.fake.Open())
	}
	var key string
	for _, r := range a.fake.Requests() {
		if r.Path == "/api/datastoreGet" {
			key = r.AuthKey
		}
	}

	// The account plays another movie: Import now reads it with the key
	// kept, and installs no addon.
	a.fake.Set(func(f *stremiotest.Fake) {
		f.Library = append(stremiotest.Sample(), map[string]any{"_id": "tt0000077", "type": "movie", "name": "Later", "removed": false,
			"temp": false, "_mtime": "2026-10-10T20:00:00.000Z", "state": map[string]any{"lastWatched": "2026-10-10T20:00:00.000Z",
				"timesWatched": 1}})
	})
	if status, body, _ := bob.call(http.MethodPost, "/account/app-connections/stremio/import", nil); status != http.StatusOK ||
		body["connection"] == nil {
		t.Fatalf("import now: %d %v", status, body)
	}
	connectionIdle(t, bob, "/account/app-import", "stremio")
	if !playedMovie(t, a.lib, a.data, bobID, "tt0000077") {
		t.Error("the later import did not mark the movie played")
	}
	reads := 0
	for _, r := range a.fake.Requests() {
		if r.Path == "/api/datastoreGet" {
			reads++
			if r.AuthKey != key {
				t.Error("a later import signed in again")
			}
		}
	}
	if reads != 2 {
		t.Errorf("%d reads", reads)
	}
	if installed, err := a.installed.Addons(t.Context(), addons.Personal(bobID)); err != nil || len(installed) != 0 {
		t.Errorf("Bob's addons: %+v %v", installed, err)
	}
	a.checkSecrets(t, key, "bob's password")

	// Stremio refuses the key: the connection stops, with its reason.
	a.fake.Revoke()
	bob.call(http.MethodPost, "/account/app-connections/stremio/import", nil)
	c = connectionIdle(t, bob, "/account/app-import", "stremio")
	if c["problem"] != "sign_in_refused" || c["nextImportAt"] != nil || c["lastImport"].(map[string]any)["problem"] != "sign_in_refused" {
		t.Errorf("refused: %v", c)
	}
	if status, body, _ := bob.call(http.MethodPost, "/account/app-connections/stremio/import", nil); status != http.StatusConflict ||
		body["error"] != "app_connection_stopped" {
		t.Errorf("importing a stopped connection: %d %v", status, body)
	}

	// Bob signs in again, by a link code: the connection runs again.
	if status, _, _ := bob.call(http.MethodPost, "/account/app-import/stremio/link", nil); status != http.StatusOK {
		t.Fatalf("link: %d", status)
	}
	a.fake.Approve()
	_, read, _ = bob.call(http.MethodPost, "/account/app-import/stremio/link/poll", map[string]any{"stayConnected": true})
	if status, body, _ := bob.call(http.MethodPost, "/account/app-import", map[string]any{"key": read["key"], "profile": "1"}); status != http.StatusOK {
		t.Fatalf("the import: %d %v", status, body)
	}
	appImportDone(t, bob, "/account/app-import")
	if c := connectionIdle(t, bob, "/account/app-import", "stremio"); c["problem"] != nil {
		t.Errorf("signed in again: %v", c)
	}
	if a.fake.Open() != 1 {
		t.Errorf("%d sessions open", a.fake.Open())
	}

	// Disconnecting signs out the session Polyfin opened, and forgets it.
	if status, _, _ := bob.call(http.MethodDelete, "/account/app-connections/stremio", nil); status != http.StatusNoContent {
		t.Fatalf("disconnecting: %d", status)
	}
	if a.fake.Open() != 0 {
		t.Errorf("%d sessions open", a.fake.Open())
	}
	if c := connectionOf(t, bob, "/account/app-import", "stremio"); c != nil {
		t.Errorf("after disconnecting: %v", c)
	}
	var count int
	if err := a.pool.QueryRow(t.Context(), "SELECT count(*) FROM app_connections").Scan(&count); err != nil || count != 0 {
		t.Errorf("%d connections left: %v", count, err)
	}
	if status, _, _ := bob.call(http.MethodPost, "/account/app-connections/stremio/import", nil); status != http.StatusNotFound {
		t.Errorf("importing after disconnecting: %d", status)
	}
	for _, r := range a.fake.Requests() {
		if r.Path == "/api/logout" && r.AuthKey != "" && strings.Contains(a.log.String(), r.AuthKey) {
			t.Error("the log holds a key")
		}
	}
}

func TestAPastedStremioKeyIsKeptButNeverSignedOut(t *testing.T) {
	t.Parallel()
	a := newAppConnectionsAPI(t)
	pasted := stremioSample(a.fake, "https://first.example/u/secret/manifest.json")
	bob := a.signedIn("bob", false)

	_, read, _ := bob.call(http.MethodPost, "/account/app-import/stremio/key", map[string]any{"key": pasted, "stayConnected": true})
	if status, body, _ := bob.call(http.MethodPost, "/account/app-import", map[string]any{"key": read["key"], "profile": "1"}); status != http.StatusOK {
		t.Fatalf("the import: %d %v", status, body)
	}
	appImportDone(t, bob, "/account/app-import")
	c := connectionIdle(t, bob, "/account/app-import", "stremio")
	noSecretIn(t, c, pasted)
	if status, _, _ := bob.call(http.MethodDelete, "/account/app-connections/stremio", nil); status != http.StatusNoContent {
		t.Fatalf("disconnecting: %d", status)
	}
	for _, r := range a.fake.Requests() {
		if r.Path == "/api/logout" {
			t.Error("a pasted key was signed out")
		}
	}
	a.fake.Set(func(f *stremiotest.Fake) {
		if f.Pasted != pasted {
			t.Error("the browser's key no longer works")
		}
	})
	a.checkSecrets(t, pasted)
}

func TestUsersStayConnectedToNuvio(t *testing.T) {
	t.Parallel()
	a := newAppConnectionsAPI(t)
	backend := nuviotest.New(t)
	backend.Set(func(b *nuviotest.Backend) {
		b.Email, b.Password = "bob@example.com", "bob's password"
		b.Backup = nuviotest.Backup(1, nuviotest.Sample("https://first.example/u/secret/manifest.json",
			"https://second.example/u/other-secret"))
	})
	bob := a.signedIn("bob", false)

	_, read, _ := bob.call(http.MethodPost, "/account/app-import/nuvio",
		map[string]any{"backend": backend.URL, "email": "bob@example.com", "password": "bob's password", "stayConnected": true})
	noSecretIn(t, read, "bob's password")
	if status, body, _ := bob.call(http.MethodPost, "/account/app-import", map[string]any{"key": read["key"], "profile": "2"}); status != http.StatusOK {
		t.Fatalf("the import: %d %v", status, body)
	}
	appImportDone(t, bob, "/account/app-import")
	c := connectionIdle(t, bob, "/account/app-import", "nuvio")
	if c["account"] != "bob@example.com" || c["lastImport"] == nil || backend.Open() != 1 {
		t.Fatalf("the connection: %v, %d open", c, backend.Open())
	}
	first := backend.Refresh()

	// Each import renews the session, and keeps the new refresh token: the
	// next one reads with it.
	for i := range 2 {
		bob.call(http.MethodPost, "/account/app-connections/nuvio/import", nil)
		c = connectionIdle(t, bob, "/account/app-import", "nuvio")
		last := c["lastImport"].(map[string]any)
		if c["problem"] != nil || last["problem"] != nil || last["read"] != 1.0 {
			t.Fatalf("import %d: %v", i, c)
		}
	}
	if !backend.Spent(first) || backend.Open() != 1 {
		t.Errorf("the session was not renewed: %d open", backend.Open())
	}
	if !playedMovie(t, a.lib, a.data, a.userID("bob"), "tt0000009") || playedMovie(t, a.lib, a.data, a.userID("bob"), "tt0000001") {
		t.Error("the Kids profile is not the one imported")
	}
	noSecretIn(t, c)
	a.checkSecrets(t, first, backend.Refresh(), "bob's password")

	// Nuvio ending the session stops the connection.
	backend.EndSessions()
	bob.call(http.MethodPost, "/account/app-connections/nuvio/import", nil)
	if c := connectionIdle(t, bob, "/account/app-import", "nuvio"); c["problem"] != "sign_in_refused" {
		t.Errorf("refused: %v", c)
	}

	// Signed in again then disconnected, the session is signed out.
	_, read, _ = bob.call(http.MethodPost, "/account/app-import/nuvio",
		map[string]any{"backend": backend.URL, "email": "bob@example.com", "password": "bob's password", "stayConnected": true})
	bob.call(http.MethodPost, "/account/app-import", map[string]any{"key": read["key"], "profile": "1"})
	appImportDone(t, bob, "/account/app-import")
	connectionIdle(t, bob, "/account/app-import", "nuvio")
	if backend.Open() != 1 {
		t.Errorf("%d sessions open", backend.Open())
	}
	if status, _, _ := bob.call(http.MethodDelete, "/account/app-connections/nuvio", nil); status != http.StatusNoContent {
		t.Fatalf("disconnecting: %d", status)
	}
	if backend.Open() != 0 {
		t.Errorf("%d sessions open after disconnecting", backend.Open())
	}
}

func TestAppConnectionsFollowTheirPermissions(t *testing.T) {
	t.Parallel()
	a := newAppConnectionsAPI(t)
	stremioSample(a.fake, "https://first.example/u/secret/manifest.json")
	admin, bob, carol := a.signedIn("root", true), a.signedIn("bob", false), a.signedIn("carol", false)
	userPath := "/users/" + a.userID("bob").String()

	// An administrator connects Bob's account for him.
	_, read, _ := admin.call(http.MethodPost, userPath+"/app-import/stremio/password",
		map[string]any{"email": "bob@example.com", "password": "bob's password", "stayConnected": true})
	if status, body, _ := admin.call(http.MethodPost, userPath+"/app-import", map[string]any{"key": read["key"], "profile": "1"}); status != http.StatusOK {
		t.Fatalf("the administrator's import: %d %v", status, body)
	}
	appImportDone(t, admin, userPath+"/app-import")
	if c := connectionIdle(t, admin, userPath+"/app-import", "stremio"); c["connectedBy"].(map[string]any)["name"] != "root" {
		t.Errorf("for the administrator: %v", c)
	}
	if c := connectionIdle(t, bob, "/account/app-import", "stremio"); c["connectedBy"].(map[string]any)["name"] != "root" {
		t.Errorf("for Bob: %v", c)
	}
	if c := connectionOf(t, carol, "/account/app-import", "stremio"); c != nil {
		t.Errorf("Carol sees %v", c)
	}
	// A member has no administrator's routes, nor other apps.
	for _, call := range [][2]string{{http.MethodPost, userPath + "/app-connections/stremio/import"},
		{http.MethodDelete, userPath + "/app-connections/stremio"}} {
		if status, _, _ := bob.call(call[0], call[1], nil); status != http.StatusForbidden {
			t.Errorf("a member on %s %s: %d", call[0], call[1], status)
		}
	}
	if status, _, _ := bob.call(http.MethodPost, "/account/app-connections/other/import", nil); status != http.StatusNotFound {
		t.Errorf("another app: %d", status)
	}
	if status, _, _ := carol.call(http.MethodPost, "/account/app-connections/stremio/import", nil); status != http.StatusNotFound {
		t.Errorf("Carol imports Bob's: %d", status)
	}

	// The setting off pauses Bob's connection: he sees it, may not import
	// it, but may disconnect it; the administrator may import it still.
	settings := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en", "serverImports": false}
	if status, _, _ := admin.call(http.MethodPut, "/settings", settings); status != http.StatusOK {
		t.Fatalf("turning users' imports off: %d", status)
	}
	if c := connectionOf(t, bob, "/account/app-import", "stremio"); c == nil || c["paused"] != true || c["nextImportAt"] != nil {
		t.Errorf("paused: %v", c)
	}
	if status, body, _ := bob.call(http.MethodPost, "/account/app-connections/stremio/import", nil); status != http.StatusForbidden ||
		body["error"] != "server_imports_disabled" {
		t.Errorf("Bob imports with the setting off: %d %v", status, body)
	}
	if status, _, _ := admin.call(http.MethodPost, userPath+"/app-connections/stremio/import", nil); status != http.StatusOK {
		t.Errorf("the administrator imports with the setting off: %d", status)
	}
	connectionIdle(t, admin, userPath+"/app-import", "stremio")
	if status, _, _ := bob.call(http.MethodDelete, "/account/app-connections/stremio", nil); status != http.StatusNoContent {
		t.Errorf("Bob disconnects with the setting off: %d", status)
	}
	if a.fake.Open() != 0 {
		t.Errorf("%d sessions open", a.fake.Open())
	}
}
