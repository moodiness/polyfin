package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/activity"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/quickconnect"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/testdb"
	"github.com/moodiness/polyfin/internal/throttle"
)

const setupCode = "ABCD-EFGH"

type pinger struct{}

func (pinger) Ping(context.Context) error { return nil }

type testAPI struct {
	t            *testing.T
	url          string
	store        *accounts.Store
	quickConnect *quickconnect.Store
	// clock, once set, is the time users' allowed hours are checked
	// against.
	clock *atomic.Pointer[time.Time]
}

// testDeps are what a test completing the options may use.
type testDeps struct {
	pool   *pgxpool.Pool
	client *stremio.Client
	addons *addons.Store
}

// newTestAPI serves the admin API; configure, if any, completes its options.
func newTestAPI(t *testing.T, failures int, configure ...func(*Options, testDeps)) testAPI {
	t.Helper()
	pool := testdb.New(t)
	if err := database.Migrate(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	store, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	quickConnect := quickconnect.New()
	clock := new(atomic.Pointer[time.Time])
	client := stremio.NewClient("test")
	addonStore := addons.New(pool, client)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	channels := iptv.New(pool, addonStore, client, logger, store.Settings)
	lib := library.New(pool, addonStore, client, logger, store.Settings)
	lib.UseIPTV(channels)
	options := Options{
		Version:      "1.2.3",
		ServerID:     "0123456789abcdef0123456789abcdef",
		Database:     pinger{},
		Accounts:     store,
		Addons:       addonStore,
		QuickConnect: quickConnect,
		SignIns:      throttle.New(failures, time.Minute),
		SetupCode:    setupCode,
		Logger:       logger,
		Activity:     activity.New(pool, store.Settings, logger),
		Now: func() time.Time {
			if at := clock.Load(); at != nil {
				return *at
			}
			return time.Now()
		},
		Guides: lib,
		IPTV:   channels,
	}
	for _, c := range configure {
		c(&options, testDeps{pool: pool, client: client, addons: addonStore})
	}
	server := httptest.NewServer(New(options))
	t.Cleanup(server.Close)
	return testAPI{t: t, url: server.URL, store: store, quickConnect: quickConnect, clock: clock}
}

// browser is a client with its own cookie jar.
type browser struct {
	api    testAPI
	client *http.Client
}

func (api testAPI) browser() browser {
	jar, _ := cookiejar.New(nil)
	return browser{api: api, client: &http.Client{Jar: jar}}
}

func (b browser) call(method, path string, body any, header ...string) (int, map[string]any, *http.Response) {
	b.api.t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, _ := json.Marshal(body)
		reader = bytes.NewReader(encoded)
	}
	request, _ := http.NewRequestWithContext(b.api.t.Context(), method, b.api.url+"/admin/api"+path, reader)
	request.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(header); i += 2 {
		request.Header.Set(header[i], header[i+1])
	}
	response, err := b.client.Do(request)
	if err != nil {
		b.api.t.Fatal(err)
	}
	defer response.Body.Close()
	var decoded map[string]any
	_ = json.NewDecoder(response.Body).Decode(&decoded)
	return response.StatusCode, decoded, response
}

func (api testAPI) signedIn(name string, administrator bool) browser {
	api.t.Helper()
	if _, err := api.store.CreateUser(api.t.Context(), accounts.NewUser{Name: name, Password: "correct horse", IsAdministrator: administrator}); err != nil {
		api.t.Fatal(err)
	}
	b := api.browser()
	if status, body, _ := b.call(http.MethodPost, "/session", map[string]string{"name": name, "password": "correct horse"}); status != http.StatusOK {
		api.t.Fatalf("sign-in of %s: %d %v", name, status, body)
	}
	return b
}

func TestSetupCreatesTheFirstAdministratorOnce(t *testing.T) {
	api := newTestAPI(t, 10)
	b := api.browser()
	if _, status, _ := b.call(http.MethodGet, "/status", nil); status["setupRequired"] != true {
		t.Fatalf("status before setup: %v", status)
	}
	account := map[string]string{"name": "admin", "password": "correct horse", "language": "fr"}

	account["setupCode"] = "WXYZ-WXYZ"
	if status, body, _ := b.call(http.MethodPost, "/setup", account); status != http.StatusBadRequest || body["error"] != "invalid_setup_code" {
		t.Fatalf("wrong setup code: %d %v", status, body)
	}
	// The code is read from a log: letter case and the dash do not matter.
	account["setupCode"] = "abcdefgh"
	status, body, response := b.call(http.MethodPost, "/setup", account)
	if status != http.StatusCreated {
		t.Fatalf("setup: %d %v", status, body)
	}
	if got := api.store.Settings().Language; got != "fr" {
		t.Errorf("server language after a setup in French: %q", got)
	}
	cookie := response.Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/admin" {
		t.Errorf("session cookie is not HttpOnly, SameSite=Strict, Path=/admin: %+v", cookie)
	}
	if status, body, _ := b.call(http.MethodGet, "/session", nil); status != http.StatusOK {
		t.Errorf("session after setup: %d %v", status, body)
	}
	if status, body, _ := api.browser().call(http.MethodPost, "/setup", account); status != http.StatusConflict || body["error"] != "setup_complete" {
		t.Errorf("second setup: %d %v", status, body)
	}
	if _, status, _ := b.call(http.MethodGet, "/status", nil); status["setupRequired"] != false {
		t.Errorf("status after setup: %v", status)
	}
}

// The admin app links to the web client only when Polyfin serves one.
func TestStatusTellsWhetherTheWebClientIsServed(t *testing.T) {
	pool := testdb.New(t)
	if err := database.Migrate(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	store, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	for _, served := range []bool{false, true} {
		h := New(Options{Database: pinger{}, Accounts: store, WebClient: served, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/admin/api/status", nil))
		var status map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &status); err != nil || status["webClient"] != served {
			t.Errorf("web client served %v: status %d %s", served, recorder.Code, recorder.Body)
		}
	}
}

func TestSetupIgnoresLanguagesTheServerDoesNotSpeak(t *testing.T) {
	api := newTestAPI(t, 10)
	account := map[string]string{"name": "admin", "password": "correct horse", "setupCode": setupCode, "language": "de"}
	if status, body, _ := api.browser().call(http.MethodPost, "/setup", account); status != http.StatusCreated {
		t.Fatalf("setup: %d %v", status, body)
	}
	if got := api.store.Settings().Language; got != "en" {
		t.Errorf("server language after a setup in German: %q", got)
	}
}

func TestSettingsLanguage(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); body["language"] != "en" {
		t.Errorf("default settings: %v", body)
	}
	settings := map[string]any{"serverName": "Maison", "quickConnectEnabled": true, "legacyAuthorization": false, "language": "fr"}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK || body["language"] != "fr" {
		t.Fatalf("saving French: %d %v", status, body)
	}
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); body["language"] != "fr" || body["serverName"] != "Maison" {
		t.Errorf("settings after saving: %v", body)
	}
	for _, language := range []any{"de", "", nil} {
		settings["language"] = language
		if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusBadRequest || body["error"] != "invalid_language" {
			t.Errorf("language %v: %d %v", language, status, body)
		}
	}
	if got := api.store.Settings().Language; got != "fr" {
		t.Errorf("a refused language changed the settings: %q", got)
	}
}

func TestSettingsPlaybackSwitches(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); body["chapters"] != true || body["prepareAhead"] != false {
		t.Errorf("default settings: %v", body)
	}
	settings := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "legacyAuthorization": false, "language": "en",
		"chapters": false, "prepareAhead": true}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK ||
		body["chapters"] != false || body["prepareAhead"] != true {
		t.Fatalf("saving the switches: %d %v", status, body)
	}
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); body["chapters"] != false || body["prepareAhead"] != true {
		t.Errorf("settings after saving: %v", body)
	}
	// A page or script older than the switches leaves them as they are.
	delete(settings, "chapters")
	delete(settings, "prepareAhead")
	settings["serverName"] = "Maison"
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK ||
		body["serverName"] != "Maison" || body["chapters"] != false || body["prepareAhead"] != true {
		t.Errorf("saving without the switches: %d %v", status, body)
	}
	if got := api.store.Settings(); got.Chapters || !got.PrepareAhead {
		t.Errorf("stored switches after a save without them: %+v", got)
	}
}

func TestSettingsCatalogLimits(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); body["catalogLimit"] != float64(accounts.DefaultCatalogLimit) ||
		body["channelLimit"] != float64(accounts.DefaultChannelLimit) {
		t.Errorf("default settings: %v", body)
	}
	settings := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "legacyAuthorization": false, "language": "en",
		"catalogLimit": 5000, "channelLimit": 30000}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK ||
		body["catalogLimit"] != float64(5000) || body["channelLimit"] != float64(30000) {
		t.Fatalf("saving the limits: %d %v", status, body)
	}
	if _, body, _ := administrator.call(http.MethodGet, "/settings", nil); body["catalogLimit"] != float64(5000) || body["channelLimit"] != float64(30000) {
		t.Errorf("settings after saving: %v", body)
	}
	// A save without a limit keeps it.
	delete(settings, "catalogLimit")
	settings["channelLimit"] = 40000
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK ||
		body["catalogLimit"] != float64(5000) || body["channelLimit"] != float64(40000) {
		t.Errorf("saving without the catalog limit: %d %v", status, body)
	}
	delete(settings, "channelLimit")
	settings["catalogLimit"] = 6000
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK ||
		body["catalogLimit"] != float64(6000) || body["channelLimit"] != float64(40000) {
		t.Errorf("saving without the channel limit: %d %v", status, body)
	}
	for _, tc := range []struct {
		key   string
		value int
		code  string
	}{
		{"catalogLimit", accounts.MinCatalogLimit - 1, "invalid_catalog_limit"},
		{"catalogLimit", accounts.MaxCatalogLimit + 1, "invalid_catalog_limit"},
		{"channelLimit", accounts.MinChannelLimit - 1, "invalid_channel_limit"},
		{"channelLimit", accounts.MaxChannelLimit + 1, "invalid_channel_limit"},
	} {
		refused := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "legacyAuthorization": false, "language": "en", tc.key: tc.value}
		if status, body, _ := administrator.call(http.MethodPut, "/settings", refused); status != http.StatusBadRequest || body["error"] != tc.code {
			t.Errorf("%s %d: %d %v", tc.key, tc.value, status, body)
		}
	}
	if got := api.store.Settings(); got.CatalogLimit != 6000 || got.ChannelLimit != 40000 {
		t.Errorf("a refused limit changed the settings: %+v", got)
	}
}

func TestSettingsContent(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	defaults := map[string]any{"skipButtons": true, "similarTitles": true, "playedPercent": float64(90), "resumePercent": float64(5),
		"versionListMinutes": float64(10), "catalogRefreshMinutes": float64(10)}
	_, body, _ := administrator.call(http.MethodGet, "/settings", nil)
	for key, want := range defaults {
		if body[key] != want {
			t.Errorf("default %s: %v, want %v", key, body[key], want)
		}
	}
	base := func() map[string]any {
		return map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "legacyAuthorization": false, "language": "en"}
	}
	saved := map[string]any{"skipButtons": false, "similarTitles": false, "playedPercent": float64(95), "resumePercent": float64(20),
		"versionListMinutes": float64(60), "catalogRefreshMinutes": float64(720)}
	settings := base()
	for key, value := range saved {
		settings[key] = value
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK {
		t.Fatalf("saving: %d %v", status, body)
	}
	_, body, _ = administrator.call(http.MethodGet, "/settings", nil)
	for key, want := range saved {
		if body[key] != want {
			t.Errorf("saved %s: %v, want %v", key, body[key], want)
		}
	}
	// A save that leaves them out keeps them.
	status, body, _ := administrator.call(http.MethodPut, "/settings", base())
	if status != http.StatusOK {
		t.Fatalf("saving without them: %d %v", status, body)
	}
	for key, want := range saved {
		if body[key] != want {
			t.Errorf("saved without them, %s: %v, want %v", key, body[key], want)
		}
	}
	changed := base()
	changed["versionListMinutes"] = 5
	if status, body, _ := administrator.call(http.MethodPut, "/settings", changed); status != http.StatusOK ||
		body["versionListMinutes"] != float64(5) || body["catalogRefreshMinutes"] != float64(720) || body["playedPercent"] != float64(95) ||
		body["resumePercent"] != float64(20) || body["skipButtons"] != false || body["similarTitles"] != false {
		t.Errorf("saving the list life only: %d %v", status, body)
	}
	for _, tc := range []struct {
		values map[string]any
		code   string
	}{
		{map[string]any{"playedPercent": accounts.MinPlayedPercent - 1}, "invalid_played_percent"},
		{map[string]any{"playedPercent": accounts.MaxPlayedPercent + 1}, "invalid_played_percent"},
		{map[string]any{"resumePercent": accounts.MinResumePercent - 1}, "invalid_resume_percent"},
		{map[string]any{"resumePercent": accounts.MaxResumePercent + 1}, "invalid_resume_percent"},
		{map[string]any{"playedPercent": 50, "resumePercent": 50}, "resume_not_below_played"},
		{map[string]any{"versionListMinutes": accounts.MinVersionListMinutes - 1}, "invalid_version_list_minutes"},
		{map[string]any{"versionListMinutes": accounts.MaxVersionListMinutes + 1}, "invalid_version_list_minutes"},
		{map[string]any{"catalogRefreshMinutes": accounts.MinCatalogRefreshMinutes - 1}, "invalid_catalog_refresh_minutes"},
		{map[string]any{"catalogRefreshMinutes": accounts.MaxCatalogRefreshMinutes + 1}, "invalid_catalog_refresh_minutes"},
	} {
		refused := base()
		for key, value := range tc.values {
			refused[key] = value
		}
		if status, body, _ := administrator.call(http.MethodPut, "/settings", refused); status != http.StatusBadRequest || body["error"] != tc.code {
			t.Errorf("%v: %d %v", tc.values, status, body)
		}
	}
	if got := api.store.Settings(); got.PlayedPercent != 95 || got.ResumePercent != 20 || got.VersionListMinutes != 5 || got.CatalogRefreshMinutes != 720 {
		t.Errorf("a refused value changed the settings: %+v", got)
	}
}

func TestAccessRequiresTheRightRole(t *testing.T) {
	api := newTestAPI(t, 10)
	anonymous := api.browser()
	if status, body, _ := anonymous.call(http.MethodGet, "/account/devices", nil); status != http.StatusUnauthorized || body["error"] != "unauthenticated" {
		t.Errorf("anonymous: %d %v", status, body)
	}
	member := api.signedIn("member", false)
	if status, _, _ := member.call(http.MethodGet, "/account/devices", nil); status != http.StatusOK {
		t.Errorf("member on own devices: %d", status)
	}
	for _, path := range []string{"/users", "/settings"} {
		if status, body, _ := member.call(http.MethodGet, path, nil); status != http.StatusForbidden || body["error"] != "forbidden" {
			t.Errorf("member on %s: %d %v", path, status, body)
		}
	}
	administrator := api.signedIn("administrator", true)
	if status, _, _ := administrator.call(http.MethodGet, "/users", nil); status != http.StatusOK {
		t.Errorf("administrator on /users: %d", status)
	}
	if status, _, _ := administrator.call(http.MethodDelete, "/session", nil); status != http.StatusNoContent {
		t.Fatalf("sign-out: %d", status)
	}
	if status, _, _ := administrator.call(http.MethodGet, "/users", nil); status != http.StatusUnauthorized {
		t.Errorf("after sign-out: %d", status)
	}
}

func TestCrossOriginChangesAreRefused(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	user := map[string]any{"name": "mallory", "password": "correct horse"}
	if status, body, _ := administrator.call(http.MethodPost, "/users", user, "Origin", "https://evil.example"); status != http.StatusForbidden || body["error"] != "cross_origin" {
		t.Errorf("cross-origin creation: %d %v", status, body)
	}
	if status, _, _ := administrator.call(http.MethodPost, "/users", user, "Origin", api.url); status != http.StatusCreated {
		t.Errorf("same-origin creation: %d", status)
	}
}

func TestUserManagementErrors(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	_, created, _ := administrator.call(http.MethodPost, "/users", map[string]any{"name": "alice", "password": "correct horse"})
	for _, tc := range []struct {
		method, path string
		body         any
		status       int
		code         string
	}{
		{http.MethodPost, "/users", map[string]any{"name": "ALICE", "password": "correct horse"}, http.StatusConflict, "name_taken"},
		{http.MethodPost, "/users", map[string]any{"name": "bob", "password": "short"}, http.StatusBadRequest, "invalid_password"},
		{http.MethodPost, "/users", map[string]any{"name": "b/b", "password": "correct horse"}, http.StatusBadRequest, "invalid_name"},
		{http.MethodPatch, "/users/" + created["id"].(string), map[string]any{"name": "administrator"}, http.StatusConflict, "name_taken"},
		{http.MethodPatch, "/users/00000000000000000000000000000000", map[string]any{"isHidden": false}, http.StatusNotFound, "not_found"},
	} {
		if status, body, _ := administrator.call(tc.method, tc.path, tc.body); status != tc.status || body["error"] != tc.code {
			t.Errorf("%s %s: got %d %v, want %d %s", tc.method, tc.path, status, body, tc.status, tc.code)
		}
	}
	_, me, _ := administrator.call(http.MethodGet, "/session", nil)
	selfID := me["user"].(map[string]any)["id"].(string)
	if status, body, _ := administrator.call(http.MethodPatch, "/users/"+selfID, map[string]any{"isAdministrator": false}); body["error"] != "last_administrator" {
		t.Errorf("demoting the last administrator: %d %v", status, body)
	}
	// Changing one's own password from the users page keeps this session.
	if status, _, _ := administrator.call(http.MethodPatch, "/users/"+selfID, map[string]any{"password": "battery staple"}); status != http.StatusOK {
		t.Fatalf("own password change: %d", status)
	}
	if status, _, _ := administrator.call(http.MethodGet, "/session", nil); status != http.StatusOK {
		t.Errorf("own password change signed this session out: %d", status)
	}
}

func TestQuickConnectApproval(t *testing.T) {
	api := newTestAPI(t, 10)
	member := api.signedIn("member", false)
	request, err := api.quickConnect.Initiate("tv-id", "Living room TV", "Swiftfin", "1.3")
	if err != nil {
		t.Fatal(err)
	}
	if status, body, _ := member.call(http.MethodGet, "/quick-connect/000000x", nil); status != http.StatusNotFound || body["error"] != "unknown_code" {
		t.Errorf("unknown code: %d %v", status, body)
	}
	status, body, _ := member.call(http.MethodGet, "/quick-connect/"+request.Code, nil)
	if status != http.StatusOK || body["deviceName"] != "Living room TV" || body["appName"] != "Swiftfin" {
		t.Fatalf("lookup: %d %v", status, body)
	}
	if status, _, _ := member.call(http.MethodPost, "/quick-connect", map[string]string{"code": request.Code}); status != http.StatusNoContent {
		t.Fatalf("approval: %d", status)
	}
	approved, _ := api.quickConnect.BySecret(request.Secret)
	_, session, _ := member.call(http.MethodGet, "/session", nil)
	if approved.User == nil || approved.User.String() != session["user"].(map[string]any)["id"] {
		t.Errorf("request approved for %v, want the signed-in member", approved.User)
	}
}

func TestFailedSignInsAreThrottled(t *testing.T) {
	api := newTestAPI(t, 2)
	api.signedIn("alice", false)
	b := api.browser()
	for range 2 {
		b.call(http.MethodPost, "/session", map[string]string{"name": "alice", "password": "wrong horse"})
	}
	status, body, response := b.call(http.MethodPost, "/session", map[string]string{"name": "alice", "password": "correct horse"})
	if status != http.StatusTooManyRequests || body["error"] != "too_many_attempts" || response.Header.Get("Retry-After") == "" {
		t.Errorf("sign-in after the failure limit: %d %v", status, body)
	}
}
