package admin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/appimport"
	"github.com/moodiness/polyfin/internal/appimport/nuvio"
	"github.com/moodiness/polyfin/internal/appimport/nuvio/nuviotest"
	stremioimport "github.com/moodiness/polyfin/internal/appimport/stremio"
	"github.com/moodiness/polyfin/internal/appimport/stremio/stremiotest"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
)

// appImportAPI serves the admin API with imports from apps, the Stremio
// accounts on a fake Stremio, and returns the library, the user data and
// the addons they write to, and the fake.
func appImportAPI(t *testing.T) (testAPI, *library.Service, *userdata.Store, *addons.Store, *stremiotest.Fake) {
	t.Helper()
	var lib *library.Service
	var data *userdata.Store
	var installed *addons.Store
	fake := stremiotest.New(t)
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		lib = library.New(deps.pool, deps.addons, deps.client, logger, o.Accounts.Settings)
		data, installed = userdata.New(deps.pool), deps.addons
		o.AppImport = appimport.New(appimport.Options{Settings: o.Accounts.Settings, Titles: lib, UserData: data, Addons: deps.addons,
			Logger: logger})
		t.Cleanup(o.AppImport.Close)
		o.Nuvio = nuvio.New(nuvio.Options{Version: "1.2.3"})
		o.Stremio = stremioimport.New(stremioimport.Options{Metas: stremiotest.SampleMetas(), Version: "1.2.3", API: fake.API,
			Link: fake.Link})
	})
	return api, lib, data, installed, fake
}

// appImportDone waits for the import into the account path reads to end,
// and fails unless it is done.
func appImportDone(t *testing.T, b browser, path string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, body, _ := b.call(http.MethodGet, path, nil)
		if current, _ := body["import"].(map[string]any); current != nil && current["state"] != "running" {
			if current["state"] != "done" {
				t.Fatalf("import: %v", current)
			}
			return current
		}
		if time.Now().After(deadline) {
			t.Fatalf("the import never ended: %v", body)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestUsersImportTheirNuvioAccount(t *testing.T) {
	t.Parallel()
	api, lib, data, _, _ := appImportAPI(t)
	backend := nuviotest.New(t)
	addon := localAddon(t)
	backend.Set(func(b *nuviotest.Backend) {
		b.Email, b.Password = "bob@example.com", "bob's password"
		b.Backup = nuviotest.Backup(1, nuviotest.Sample(addon, "https://second.example/u/other-secret"))
	})
	bob, carol := api.signedIn("bob", false), api.signedIn("carol", false)
	signIn := map[string]any{"backend": backend.URL, "email": "bob@example.com", "password": "wrong"}

	if _, body, _ := bob.call(http.MethodGet, "/account/app-import", nil); body["enabled"] != true || body["addonsAllowed"] != true ||
		body["import"] != nil {
		t.Errorf("before: %v", body)
	}
	if status, body, _ := bob.call(http.MethodPost, "/account/app-import/nuvio", signIn); status != http.StatusBadRequest ||
		body["error"] != "nuvio_sign_in_refused" {
		t.Errorf("a wrong password: %d %v", status, body)
	}
	signIn["password"] = "bob's password"
	status, read, _ := bob.call(http.MethodPost, "/account/app-import/nuvio", signIn)
	profiles, _ := read["profiles"].([]any)
	if status != http.StatusOK || read["app"] != "nuvio" || read["key"] == "" || len(profiles) != 2 {
		t.Fatalf("Bob's account: %d %v", status, read)
	}
	main := profiles[0].(map[string]any)
	if main["id"] != "1" || main["name"] != "Main" || main["entries"] != 8.0 || main["played"] != 4.0 || main["resumes"] != 3.0 ||
		main["library"] != 2.0 || profiles[1].(map[string]any)["name"] != "Kids" {
		t.Errorf("profiles: %v", profiles)
	}
	if addonsOffered := main["addons"].([]any); len(addonsOffered) != 2 || addonsOffered[0].(map[string]any)["name"] != "First" ||
		!strings.HasSuffix(addonsOffered[1].(map[string]any)["address"].(string), "/…/manifest.json") {
		t.Errorf("addons: %v", addonsOffered)
	}
	if raw, _ := json.Marshal(read); strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "bob's password") {
		t.Errorf("the answer holds a secret: %s", raw)
	}

	// Carol cannot import what Bob read.
	start := map[string]any{"key": read["key"], "profile": "1"}
	if status, body, _ := carol.call(http.MethodPost, "/account/app-import", start); status != http.StatusConflict ||
		body["error"] != "app_import_expired" {
		t.Errorf("Carol with Bob's key: %d %v", status, body)
	}
	status, body, _ := bob.call(http.MethodPost, "/account/app-import", start)
	if started, _ := body["import"].(map[string]any); status != http.StatusOK || started == nil || started["profile"] != "Main" {
		t.Fatalf("Bob's import: %d %v", status, body)
	}
	done := appImportDone(t, bob, "/account/app-import")
	// Played: the first movie, and S1E2; resumed: the second movie, S1E3
	// and S2E1; favorites: the first movie and the series; not matched:
	// one not found, one of another identifier.
	if done["read"] != 8.0 || done["played"] != 2.0 || done["resumed"] != 3.0 || done["favorites"] != 2.0 || done["unmatchedCount"] != 2.0 ||
		done["app"] != "nuvio" || done["user"].(map[string]any)["name"] != "bob" || done["startedBy"].(map[string]any)["name"] != "bob" {
		t.Errorf("Bob's result: %v", done)
	}
	reasons := map[string]string{}
	for _, u := range done["unmatched"].([]any) {
		entry := u.(map[string]any)
		reasons[entry["id"].(string)] = entry["reason"].(string)
	}
	if reasons["tmdb:999999"] != "not_found" || reasons["custom:1"] != "other_identifier" {
		t.Errorf("unmatched: %v", done["unmatched"])
	}
	bobID := api.userID("bob")
	if !playedMovie(t, lib, data, bobID, "tt0000001") || playedMovie(t, lib, data, bobID, "tt0000009") {
		t.Error("Bob's movies")
	}
	if backend.Open() != 0 {
		t.Error("a Nuvio session is still open")
	}
	if _, body, _ := carol.call(http.MethodGet, "/account/app-import", nil); body["import"] != nil {
		t.Errorf("Carol sees %v", body)
	}

	// Another version of the backup is refused, saying which.
	backend.Set(func(b *nuviotest.Backend) { b.Backup = nuviotest.Backup(2, map[string]any{}) })
	if status, body, _ := bob.call(http.MethodPost, "/account/app-import/nuvio", signIn); status != http.StatusBadGateway ||
		body["error"] != "nuvio_backup_unsupported" || body["version"] != 2.0 {
		t.Errorf("version 2: %d %v", status, body)
	}
}

func TestAppImportsFollowTheirPermissions(t *testing.T) {
	t.Parallel()
	api, _, _, installed, _ := appImportAPI(t)
	backend := nuviotest.New(t)
	addon := localAddon(t)
	backend.Set(func(b *nuviotest.Backend) {
		b.Email, b.Password = "bob@example.com", "bob's password"
		b.Backup = nuviotest.Backup(1, nuviotest.Sample(addon, "https://second.example/u/other-secret"))
	})
	admin, bob := api.signedIn("root", true), api.signedIn("bob", false)
	bobID := api.userID("bob")
	signIn := map[string]any{"backend": backend.URL, "email": "bob@example.com", "password": "bob's password"}
	userPath := "/users/" + bobID.String() + "/app-import"

	// The administrator's routes are theirs only.
	if status, _, _ := bob.call(http.MethodPost, userPath+"/nuvio", signIn); status != http.StatusForbidden {
		t.Errorf("a member on another user's import: %d", status)
	}
	if status, _, _ := bob.call(http.MethodGet, userPath, nil); status != http.StatusForbidden {
		t.Errorf("a member reading another user's import: %d", status)
	}

	// Bob's own addons are on the local network: his import does not
	// reach it.
	_, read, _ := bob.call(http.MethodPost, "/account/app-import/nuvio", signIn)
	if status, _, _ := bob.call(http.MethodPost, "/account/app-import", map[string]any{"key": read["key"], "profile": "1", "addons": []int{0}}); status != http.StatusOK {
		t.Fatalf("Bob's import: %d", status)
	}
	done := appImportDone(t, bob, "/account/app-import")
	if chosen := done["addons"].([]any); len(chosen) != 1 || chosen[0].(map[string]any)["problem"] != "private_network" {
		t.Errorf("Bob's addons: %v", chosen)
	}

	// With users' own addons off, none may be chosen.
	settings := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en", "personalAddons": false}
	if status, _, _ := admin.call(http.MethodPut, "/settings", settings); status != http.StatusOK {
		t.Fatalf("turning users' own addons off: %d", status)
	}
	_, read, _ = admin.call(http.MethodPost, userPath+"/nuvio", signIn)
	if _, body, _ := admin.call(http.MethodGet, userPath, nil); body["addonsAllowed"] != false {
		t.Errorf("addons allowed: %v", body)
	}
	if status, body, _ := admin.call(http.MethodPost, userPath, map[string]any{"key": read["key"], "profile": "1", "addons": []int{0}}); status != http.StatusForbidden ||
		body["error"] != "personal_addons_disabled" {
		t.Errorf("addons while users' own are off: %d %v", status, body)
	}

	// An administrator imports into Bob's account, the first addon among
	// them, even with the users' import turned off.
	settings["personalAddons"], settings["serverImports"] = true, false
	if status, _, _ := admin.call(http.MethodPut, "/settings", settings); status != http.StatusOK {
		t.Fatalf("turning users' imports off: %d", status)
	}
	if status, body, _ := admin.call(http.MethodPost, userPath, map[string]any{"key": read["key"], "profile": "1", "addons": []int{0}}); status != http.StatusOK {
		t.Fatalf("the administrator's import: %d %v", status, body)
	}
	done = appImportDone(t, admin, userPath)
	if chosen := done["addons"].([]any); len(chosen) != 1 || chosen[0].(map[string]any)["installed"] != true ||
		done["startedBy"].(map[string]any)["name"] != "root" || done["user"].(map[string]any)["name"] != "bob" {
		t.Errorf("the administrator's import: %v", done)
	}
	if own, err := installed.Addons(t.Context(), addons.Personal(bobID)); err != nil || len(own) != 1 || own[0].ManifestURL != addon {
		t.Errorf("Bob's addons: %v %v", own, err)
	}
	// Bob's section is gone, and he may not import himself any longer.
	if _, body, _ := bob.call(http.MethodGet, "/account/app-import", nil); body["enabled"] != false || body["import"] != nil {
		t.Errorf("with the setting off: %v", body)
	}
	if status, body, _ := bob.call(http.MethodPost, "/account/app-import/nuvio", signIn); status != http.StatusForbidden ||
		body["error"] != "server_imports_disabled" {
		t.Errorf("signing in with the setting off: %d %v", status, body)
	}
	if status, body, _ := bob.call(http.MethodPost, "/account/app-import", map[string]any{"key": "any", "profile": "1"}); status != http.StatusForbidden ||
		body["error"] != "server_imports_disabled" {
		t.Errorf("importing with the setting off: %d %v", status, body)
	}
}

// stremioSample puts the sample account on the fake (see
// stremiotest.Sample), with the addon at first, and returns the auth key
// the user's browser holds.
func stremioSample(fake *stremiotest.Fake, first string) string {
	var pasted string
	fake.Set(func(f *stremiotest.Fake) {
		f.Email, f.Password = "bob@example.com", "bob's password"
		f.Library = stremiotest.Sample()
		f.Addons = stremiotest.SampleAddons(first, "https://second.example/u/other-secret/manifest.json")
		pasted = f.Pasted
	})
	return pasted
}

// noStremioSecret fails when an answer holds a password or an auth key.
func noStremioSecret(t *testing.T, answer map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(answer)
	for _, secret := range []string{"secret", "bob's password", "issued-key", "pasted-key"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("the answer holds %q: %s", secret, raw)
		}
	}
}

func TestUsersImportTheirStremioAccount(t *testing.T) {
	t.Parallel()
	api, lib, data, _, fake := appImportAPI(t)
	pasted := stremioSample(fake, localAddon(t))
	bob, carol := api.signedIn("bob", false), api.signedIn("carol", false)

	// The link code first: Bob opens the link, and the page waits.
	status, link, _ := bob.call(http.MethodPost, "/account/app-import/stremio/link", nil)
	if status != http.StatusOK || link["code"] == "" || link["link"] != fake.Link+"/"+link["code"].(string) {
		t.Fatalf("link: %d %v", status, link)
	}
	noStremioSecret(t, link)
	if status, body, _ := bob.call(http.MethodPost, "/account/app-import/stremio/link/poll", map[string]any{}); status != http.StatusAccepted ||
		body["pending"] != true {
		t.Errorf("before the approval: %d %v", status, body)
	}
	fake.Approve()
	// Bob's code is not Carol's.
	if status, body, _ := carol.call(http.MethodPost, "/account/app-import/stremio/link/poll", map[string]any{}); status != http.StatusConflict ||
		body["error"] != "stremio_link_expired" {
		t.Errorf("Carol polling: %d %v", status, body)
	}
	status, read, _ := bob.call(http.MethodPost, "/account/app-import/stremio/link/poll", map[string]any{})
	profiles, _ := read["profiles"].([]any)
	if status != http.StatusOK || read["app"] != "stremio" || read["key"] == "" || len(profiles) != 1 {
		t.Fatalf("Bob's account: %d %v", status, read)
	}
	noStremioSecret(t, read)
	profile := profiles[0].(map[string]any)
	if profile["id"] != "1" || profile["name"] != "" || profile["entries"] != 11.0 || profile["played"] != 6.0 || profile["resumes"] != 2.0 ||
		profile["library"] != 3.0 {
		t.Errorf("profile: %v", profile)
	}
	if offered := profile["addons"].([]any); len(offered) != 2 || offered[0].(map[string]any)["name"] != "First" ||
		!strings.HasSuffix(offered[1].(map[string]any)["address"].(string), "/…/manifest.json") {
		t.Errorf("addons: %v", offered)
	}
	if fake.Open() != 0 {
		t.Error("the session the link code opened is still open")
	}
	if status, body, _ := bob.call(http.MethodPost, "/account/app-import/stremio/link/poll", map[string]any{}); status != http.StatusConflict ||
		body["error"] != "stremio_link_expired" {
		t.Errorf("polled again: %d %v", status, body)
	}

	// The password, then a key pasted, which is never signed out.
	signIn := map[string]any{"email": "bob@example.com", "password": "wrong"}
	if status, body, _ := bob.call(http.MethodPost, "/account/app-import/stremio/password", signIn); status != http.StatusBadRequest ||
		body["error"] != "stremio_sign_in_refused" {
		t.Errorf("a wrong password: %d %v", status, body)
	}
	signIn["password"] = "bob's password"
	if status, body, _ := bob.call(http.MethodPost, "/account/app-import/stremio/password", signIn); status != http.StatusOK ||
		body["app"] != "stremio" {
		t.Errorf("the password: %d %v", status, body)
	}
	if status, body, _ := bob.call(http.MethodPost, "/account/app-import/stremio/key", map[string]any{"key": "wrong"}); status != http.StatusBadRequest ||
		body["error"] != "stremio_key_refused" {
		t.Errorf("a wrong key: %d %v", status, body)
	}
	status, read, _ = bob.call(http.MethodPost, "/account/app-import/stremio/key", map[string]any{"key": pasted})
	if status != http.StatusOK || read["app"] != "stremio" {
		t.Fatalf("the key: %d %v", status, read)
	}
	noStremioSecret(t, read)
	for _, request := range fake.Requests() {
		if request.Path == "/api/logout" && request.AuthKey == pasted {
			t.Error("the pasted key was signed out")
		}
	}
	if fake.Open() != 0 {
		t.Error("the password's session is still open")
	}

	status, body, _ := bob.call(http.MethodPost, "/account/app-import", map[string]any{"key": read["key"], "profile": "1"})
	if started, _ := body["import"].(map[string]any); status != http.StatusOK || started == nil || started["app"] != "stremio" {
		t.Fatalf("Bob's import: %d %v", status, body)
	}
	done := appImportDone(t, bob, "/account/app-import")
	// Played: the first and the removed movie, and S1E1 to S1E3; resumed:
	// the second movie and S2E1; favorites: the first movie and the
	// series; reported: the series watched against an episode Polyfin's
	// addons do not list, one of another identifier, one not found.
	if done["read"] != 11.0 || done["played"] != 5.0 || done["resumed"] != 2.0 || done["favorites"] != 2.0 || done["unmatchedCount"] != 3.0 {
		t.Errorf("Bob's result: %v", done)
	}
	reasons := map[string]string{}
	for _, u := range done["unmatched"].([]any) {
		entry := u.(map[string]any)
		reasons[entry["id"].(string)] = entry["reason"].(string)
	}
	if reasons["tt0200"] != "episode_list" || reasons["custom:1"] != "other_identifier" || reasons["tmdb:999999"] != "not_found" {
		t.Errorf("unmatched: %v", done["unmatched"])
	}
	bobID := api.userID("bob")
	if !playedMovie(t, lib, data, bobID, "tt0000001") || !playedMovie(t, lib, data, bobID, "tt0000003") ||
		playedMovie(t, lib, data, bobID, "tt0000002") {
		t.Error("Bob's movies")
	}
}

func TestStremioImportsFollowTheirPermissions(t *testing.T) {
	t.Parallel()
	api, _, _, _, fake := appImportAPI(t)
	pasted := stremioSample(fake, localAddon(t))
	admin, bob := api.signedIn("root", true), api.signedIn("bob", false)
	userPath := "/users/" + api.userID("bob").String() + "/app-import"

	// The administrator's routes are theirs only.
	for _, route := range []string{"/stremio/link", "/stremio/link/poll", "/stremio/password", "/stremio/key"} {
		if status, _, _ := bob.call(http.MethodPost, userPath+route, map[string]any{"key": pasted}); status != http.StatusForbidden {
			t.Errorf("a member on %s: %d", route, status)
		}
	}

	// An administrator imports into Bob's account, even with the users'
	// import turned off, while Bob may not any longer.
	settings := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en", "serverImports": false}
	if status, _, _ := admin.call(http.MethodPut, "/settings", settings); status != http.StatusOK {
		t.Fatalf("turning users' imports off: %d", status)
	}
	for _, route := range []string{"/stremio/link", "/stremio/link/poll", "/stremio/password", "/stremio/key"} {
		if status, body, _ := bob.call(http.MethodPost, "/account/app-import"+route, map[string]any{"key": pasted}); status != http.StatusForbidden ||
			body["error"] != "server_imports_disabled" {
			t.Errorf("Bob on %s with the setting off: %d %v", route, status, body)
		}
	}
	if status, _, _ := admin.call(http.MethodPost, userPath+"/stremio/link", nil); status != http.StatusOK {
		t.Errorf("the administrator's link code: %d", status)
	}
	fake.Approve()
	_, read, _ := admin.call(http.MethodPost, userPath+"/stremio/link/poll", map[string]any{})
	if status, body, _ := admin.call(http.MethodPost, userPath, map[string]any{"key": read["key"], "profile": "1"}); status != http.StatusOK {
		t.Fatalf("the administrator's import: %d %v", status, body)
	}
	done := appImportDone(t, admin, userPath)
	if done["app"] != "stremio" || done["startedBy"].(map[string]any)["name"] != "root" || done["user"].(map[string]any)["name"] != "bob" ||
		done["played"] != 5.0 {
		t.Errorf("the administrator's import: %v", done)
	}
}
