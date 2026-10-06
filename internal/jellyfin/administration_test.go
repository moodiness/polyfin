package jellyfin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/activity"
	"github.com/moodiness/polyfin/internal/tasks"
)

// matchesFixture compares body with a recorded Jellyfin 12.2 answer.
func matchesFixture(t *testing.T, fixture string, body []byte, rules shapeRules) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.2", fixture+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var want, got any
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("%s: %v in %s", fixture, err, body)
	}
	for _, difference := range compareShapes(fixture, want, got, rules) {
		t.Error(difference)
	}
}

// administrated is a test server with an administrator signed in on
// "dashboard" and a member, alice, on "tv".
func administrated(t *testing.T) (s testServer, admin, member string) {
	t.Helper()
	s = newTestServer(t, 10)
	s.user("root", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	s.user("alice", nil)
	return s, app("dashboard", s.signIn("root", "dashboard")), app("tv", s.signIn("alice", "tv"))
}

func TestAdministrationAnswersMatchJellyfinAndNeedAnAdministrator(t *testing.T) {
	s, admin, member := administrated(t)
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	s.tasks.Register(tasks.Task{Key: "Probe", Category: tasks.CategoryMaintenance,
		Text: map[string]tasks.Text{"en": {Name: "Probe", Description: "Does nothing."}}, Run: func(context.Context) error { return nil }})
	s.tasks.Start(ctx)
	_ = s.tasks.Run(tasks.ID("Probe"))
	for deadline := time.Now().Add(5 * time.Second); ; {
		if info, _ := s.tasks.Task(tasks.ID("Probe"), "en"); info.Last != nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if status, _ := s.call(http.MethodPost, "/Auth/Keys?app=Requests", admin, nil); status != http.StatusNoContent {
		t.Fatalf("create key: %d", status)
	}
	if status, _ := s.call(http.MethodPost, "/Devices/Options?id=tv-id", admin, map[string]string{"CustomName": "Living room"}); status != http.StatusNoContent {
		t.Fatalf("name device: %d", status)
	}
	optional := shapeRules{
		absent:   map[string][]string{"StorageType": {"system-storage"}, "DeviceId": {"system-storage"}, "ShortOverview": {"activity-log"}},
		returned: map[string][]string{"ShortOverview": {"activity-log"}, "CustomName": {"devices"}},
	}
	for fixture, path := range map[string]string{
		"api-keys":                 "/Auth/Keys",
		"auth-providers":           "/Auth/Providers",
		"password-reset-providers": "/Auth/PasswordResetProviders",
		"devices":                  "/Devices",
		"device-info":              "/Devices/Info?id=tv-id",
		"device-options":           "/Devices/Options?id=tv-id",
		"activity-log":             "/System/ActivityLog/Entries?limit=5",
		"scheduled-task":           "/ScheduledTasks/" + tasks.ID("Probe"),
		"system-storage":           "/System/Info/Storage",
		"log-files":                "/System/Logs",
		"server-configuration":     "/System/Configuration",
	} {
		status, body := s.call(http.MethodGet, path, admin, nil)
		if status != http.StatusOK {
			t.Errorf("%s: %d %s", path, status, body)
			continue
		}
		matchesFixture(t, fixture, body, optional)
		// Like Jellyfin, only the server configuration is anyone's.
		status, body = s.call(http.MethodGet, path, member, nil)
		if want := http.StatusForbidden; fixture == "server-configuration" && status != http.StatusOK || fixture != "server-configuration" && (status != want || len(body) != 0) {
			t.Errorf("%s for a member: %d %q", path, status, body)
		}
	}
	for _, path := range []string{"/Users/New", "/ScheduledTasks/Running/" + tasks.ID("Probe")} {
		if status, body := s.call(http.MethodPost, path, member, map[string]string{"Name": "x"}); status != http.StatusForbidden || len(body) != 0 {
			t.Errorf("POST %s for a member: %d %q", path, status, body)
		}
	}
	_, body := s.postRaw("/ClientLog/Document", member, "app log")
	matchesFixture(t, "client-log", body, shapeRules{})
	_, body = s.call(http.MethodPost, "/Users/ForgotPassword", "", map[string]string{"EnteredUsername": "alice"})
	matchesFixture(t, "forgot-password", body, shapeRules{})
	var pin string
	if err := s.pool.QueryRow(t.Context(), "SELECT pin FROM password_reset_pins").Scan(&pin); err != nil {
		t.Fatal(err)
	}
	_, body = s.call(http.MethodPost, "/Users/ForgotPassword/Pin", "", map[string]string{"Pin": pin})
	matchesFixture(t, "pin-redeem", body, shapeRules{})
}

// jellyfin-web's dashboard asks the plugins' pages for its menu: there are
// none, as on a Jellyfin server without plugins, and only administrators
// may ask.
func TestNoPluginConfigurationPages(t *testing.T) {
	s, admin, member := administrated(t)
	path := "/web/ConfigurationPages?enableInMainMenu=true"
	if status, body := s.call(http.MethodGet, path, admin, nil); status != http.StatusOK || strings.TrimSpace(string(body)) != "[]" {
		t.Errorf("for an administrator: %d %s", status, body)
	}
	if status, body := s.call(http.MethodGet, path, member, nil); status != http.StatusForbidden || len(body) != 0 {
		t.Errorf("for a member: %d %q", status, body)
	}
}

func TestAPIKeysActAsAnAdministratorUntilRevoked(t *testing.T) {
	s, admin, _ := administrated(t)
	if status, body := s.call(http.MethodPost, "/Auth/Keys", admin, nil); status != http.StatusBadRequest || !strings.Contains(string(body), `"app"`) {
		t.Errorf("without app: %d %s", status, body)
	}
	s.call(http.MethodPost, "/Auth/Keys?app=Requests", admin, nil)
	list := func() AuthenticationInfo {
		var keys listResult[AuthenticationInfo]
		_, body := s.call(http.MethodGet, "/Auth/Keys", admin, nil)
		_ = json.Unmarshal(body, &keys)
		if len(keys.Items) != 1 || keys.Items[0].AppName != "Requests" {
			t.Fatalf("keys: %s", body)
		}
		return keys.Items[0]
	}
	key := list().AccessToken
	// The key is shown once; later listings show its identifier.
	shown := list()
	if shown.AccessToken == key {
		t.Error("the key was shown twice")
	}
	stored, _ := s.store.APIKeys(t.Context())
	if shown.AccessToken != stored[0].ID.String() {
		t.Errorf("listed %s, want the identifier", shown.AccessToken)
	}
	var hashed []byte
	_ = s.pool.QueryRow(t.Context(), "SELECT token_hash FROM api_keys").Scan(&hashed)
	if strings.Contains(string(hashed), key) {
		t.Error("the key is stored as is")
	}

	header := `MediaBrowser Token="` + key + `"`
	for _, request := range []struct {
		path, authorization string
		want                int
	}{
		{"/System/ActivityLog/Entries", header, http.StatusOK},
		{"/System/ActivityLog/Entries?ApiKey=" + key, "", http.StatusOK},
		// api_key is a legacy form, refused by default as Jellyfin does.
		{"/System/ActivityLog/Entries?api_key=" + key, "", http.StatusUnauthorized},
		// A key has no user.
		{"/Users/Me", header, http.StatusBadRequest},
	} {
		if status, body := s.call(http.MethodGet, request.path, request.authorization, nil); status != request.want {
			t.Errorf("%s: %d %s, want %d", request.path, status, body, request.want)
		}
	}
	if list().DateLastActivity == (Time{}) {
		t.Error("the key's last use is not recorded")
	}
	// A WebSocket belongs to a signed-in device: a key gets none.
	request := httptest.NewRequest(http.MethodGet, "/socket?api_key="+key, nil)
	request.Header.Set("Authorization", header)
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Connection", "Upgrade")
	recorder := httptest.NewRecorder()
	s.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Errorf("socket: %d", recorder.Code)
	}

	// Revoked by its identifier, as listed, or by the key itself.
	if status, _ := s.call(http.MethodDelete, "/Auth/Keys/"+shown.AccessToken, admin, nil); status != http.StatusNoContent {
		t.Errorf("revoke: %d", status)
	}
	if status, _ := s.call(http.MethodGet, "/System/ActivityLog/Entries", header, nil); status != http.StatusUnauthorized {
		t.Errorf("revoked key: %d", status)
	}
	_, other, _ := s.store.CreateAPIKey(t.Context(), "Script")
	s.call(http.MethodDelete, "/Auth/Keys/"+other, admin, nil)
	if status, _ := s.call(http.MethodGet, "/Users", `MediaBrowser Token="`+other+`"`, nil); status != http.StatusUnauthorized {
		t.Errorf("key revoked by itself: %d", status)
	}
}

func TestAPIKeysBrowseForTheUserTheyName(t *testing.T) {
	s, _, ids := browsing(t)
	_, key, _ := s.store.CreateAPIKey(t.Context(), "Requests")
	header := `MediaBrowser Token="` + key + `"`
	member, _ := s.store.Users(t.Context())
	var memberID accounts.ID
	for _, user := range member {
		if user.Name == "member" {
			memberID = user.ID
		}
	}
	hidden, _ := accounts.ParseID(ids["Groups"])
	if _, err := s.store.UpdateUser(t.Context(), memberID, accounts.UserChanges{HiddenLibraries: &[]accounts.ID{hidden}}, nil); err != nil {
		t.Fatal(err)
	}
	count := func(path string) int {
		t.Helper()
		var result QueryResult
		status, body := s.call(http.MethodGet, path, header, nil)
		if status != http.StatusOK {
			t.Fatalf("%s: %d %s", path, status, body)
		}
		_ = json.Unmarshal(body, &result)
		return len(result.Items)
	}
	// Without a user, the server's libraries; for a user, theirs.
	if got := count("/Library/MediaFolders"); got != 3 {
		t.Errorf("media folders: %d", got)
	}
	if got := count("/UserViews?userId=" + memberID.String()); got != 2 {
		t.Errorf("member's views through the key: %d", got)
	}
	if got := count("/Library/MediaFolders?isHidden=true"); got != 0 {
		t.Errorf("hidden media folders: %d", got)
	}
}

func TestDevicesAreListedRenamedAndSignedOut(t *testing.T) {
	s, admin, alice := administrated(t)
	s.user("bob", nil)
	bob := app("tv", s.signIn("bob", "tv"))
	phone := app("phone", s.signIn("alice", "phone"))
	var devices listResult[DeviceInfoDto]
	_, body := s.call(http.MethodGet, "/Devices", admin, nil)
	_ = json.Unmarshal(body, &devices)
	if devices.TotalRecordCount != 4 || strings.Contains(string(body), "AccessToken") {
		t.Errorf("devices: %s", body)
	}
	if status, _ := s.call(http.MethodGet, "/Devices/Options?id=tv-id", admin, nil); status != http.StatusNotFound {
		t.Errorf("options never set: %d", status)
	}
	if status, _ := s.call(http.MethodGet, "/Devices/Info", admin, nil); status != http.StatusBadRequest {
		t.Errorf("info without id: %d", status)
	}
	// One unknown device leaves every device signed in.
	if status, _ := s.call(http.MethodDelete, "/Devices?id=tv-id&id=nowhere", admin, nil); status != http.StatusBadRequest {
		t.Errorf("unknown device: %d", status)
	}
	if status, _ := s.call(http.MethodDelete, "/Devices?id=tv-id", admin, nil); status != http.StatusNoContent {
		t.Errorf("delete: %d", status)
	}
	for name, header := range map[string]string{"alice's tv": alice, "bob's tv": bob} {
		if status, _ := s.call(http.MethodGet, "/Users/Me", header, nil); status != http.StatusUnauthorized {
			t.Errorf("%s still signed in: %d", name, status)
		}
	}
	if status, _ := s.call(http.MethodGet, "/Users/Me", phone, nil); status != http.StatusOK {
		t.Errorf("alice's phone signed out: %d", status)
	}
}

func TestUsersAreManagedFromJellyfinApps(t *testing.T) {
	s, admin, alice := administrated(t)
	status, body := s.call(http.MethodPost, "/Users/New", admin, map[string]string{"Name": "dave", "Password": "long enough"})
	var dave UserDto
	_ = json.Unmarshal(body, &dave)
	if status != http.StatusOK || dave.Policy.IsAdministrator || !dave.Policy.IsHidden {
		t.Fatalf("create: %d %s", status, body)
	}
	for name, request := range map[string]map[string]string{
		"taken name":     {"Name": "Dave", "Password": "long enough"},
		"short password": {"Name": "erin", "Password": "short"},
		"no password":    {"Name": "erin"},
	} {
		if status, _ := s.call(http.MethodPost, "/Users/New", admin, request); status != http.StatusBadRequest {
			t.Errorf("%s: %d", name, status)
		}
	}
	if status, body := s.call(http.MethodPost, "/Users/New", admin, map[string]string{}); status != http.StatusBadRequest || !strings.Contains(string(body), "missing required properties") {
		t.Errorf("without name: %d %s", status, body)
	}

	update := func(header, id string, change func(*UserDto)) (int, []byte) {
		dto := dave
		change(&dto)
		return s.call(http.MethodPost, "/Users?userId="+id, header, dto)
	}
	if status, _ := update(admin, dave.Id, func(d *UserDto) { d.Name = "david" }); status != http.StatusNoContent {
		t.Errorf("rename: %d", status)
	}
	if user, _ := s.store.User(t.Context(), mustID(t, dave.Id)); user.Name != "david" {
		t.Errorf("renamed to %q", user.Name)
	}
	if status, body := update(alice, dave.Id, func(d *UserDto) { d.Name = "x" }); status != http.StatusForbidden || string(body) != "\"User update not allowed.\"\n" {
		t.Errorf("member renaming another: %d %s", status, body)
	}
	if status, body := update(admin, dave.Id, func(d *UserDto) { d.Policy.AuthenticationProviderId = "" }); status != http.StatusBadRequest || !strings.Contains(string(body), "Policy.AuthenticationProviderId") {
		t.Errorf("without providers: %d %s", status, body)
	}

	root, _ := s.store.Users(t.Context())
	for _, user := range root {
		if user.Name == "root" {
			if status, _ := s.call(http.MethodDelete, "/Users/"+user.ID.String(), admin, nil); status != http.StatusBadRequest {
				t.Errorf("last administrator deleted: %d", status)
			}
		}
	}
	if status, _ := s.call(http.MethodDelete, "/Users/"+dave.Id, admin, nil); status != http.StatusNoContent {
		t.Errorf("delete: %d", status)
	}
	if status, _ := s.call(http.MethodDelete, "/Users/"+dave.Id, admin, nil); status != http.StatusNotFound {
		t.Errorf("deleted twice: %d", status)
	}
	page, _ := s.activity.Entries(t.Context(), activity.Query{})
	types := map[string]bool{}
	for _, entry := range page.Entries {
		types[entry.Type] = true
	}
	for _, want := range []string{"UserCreated", "UserPolicyUpdated", "UserDeleted", "AuthenticationSucceeded"} {
		if !types[want] {
			t.Errorf("no %s entry in %v", want, types)
		}
	}
}

func TestForgottenPasswordPin(t *testing.T) {
	s, _, alice := administrated(t)
	ask := func(remote string) ForgotPasswordResult {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/Users/ForgotPassword", strings.NewReader(`{"EnteredUsername":"Alice"}`))
		request.Header.Set("Content-Type", "application/json")
		request.RemoteAddr = remote
		recorder := httptest.NewRecorder()
		s.handler.ServeHTTP(recorder, request)
		var result ForgotPasswordResult
		if recorder.Code != http.StatusOK || json.Unmarshal(recorder.Body.Bytes(), &result) != nil || result.Action != "PinCode" {
			t.Fatalf("forgot password: %d %s", recorder.Code, recorder.Body)
		}
		return result
	}
	pins := func() int {
		var count int
		_ = s.pool.QueryRow(t.Context(), "SELECT count(*) FROM password_reset_pins").Scan(&count)
		return count
	}
	// From outside the local network, the same answer, and no PIN.
	ask("203.0.113.7:5000")
	if pins() != 0 {
		t.Fatal("a PIN was made from outside the local network")
	}
	result := ask("192.168.1.20:5000")
	if until := time.Until(time.Time(result.PinExpirationDate)); until < 29*time.Minute || until > 31*time.Minute {
		t.Errorf("valid for %v", until)
	}
	var pin string
	_ = s.pool.QueryRow(t.Context(), "SELECT pin FROM password_reset_pins").Scan(&pin)
	if status, _ := s.call(http.MethodPost, "/Users/ForgotPassword/Pin", "", map[string]string{"Pin": "00-00-00-00"}); status != http.StatusNotFound {
		t.Errorf("wrong PIN: %d", status)
	}
	typed := strings.ReplaceAll(pin, "-", "")
	status, body := s.call(http.MethodPost, "/Users/ForgotPassword/Pin", "", map[string]string{"Pin": typed})
	if status != http.StatusOK || string(body) != "{\"Success\":true,\"UsersReset\":[\"alice\"]}\n" {
		t.Fatalf("redeem: %d %s", status, body)
	}
	// The password is the PIN as typed; the old sign-ins are gone.
	if _, err := s.store.Authenticate(t.Context(), "alice", typed); err != nil {
		t.Errorf("sign in with the PIN: %v", err)
	}
	if status, _ := s.call(http.MethodGet, "/Users/Me", alice, nil); status != http.StatusUnauthorized {
		t.Errorf("old token: %d", status)
	}
	if status, _ := s.call(http.MethodPost, "/Users/ForgotPassword/Pin", "", map[string]string{"Pin": typed}); status != http.StatusNotFound {
		t.Errorf("PIN used twice: %d", status)
	}
	// An expired PIN is refused.
	ask("127.0.0.1:5000")
	_ = s.pool.QueryRow(t.Context(), "SELECT pin FROM password_reset_pins").Scan(&pin)
	if _, err := s.pool.Exec(t.Context(), "UPDATE password_reset_pins SET expires_at = now() - interval '1 minute'"); err != nil {
		t.Fatal(err)
	}
	if status, _ := s.call(http.MethodPost, "/Users/ForgotPassword/Pin", "", map[string]string{"Pin": pin}); status != http.StatusNotFound {
		t.Errorf("expired PIN: %d", status)
	}
}

func TestActivityLogIsWrittenAndPaged(t *testing.T) {
	s, admin, _ := administrated(t)
	s.call(http.MethodPost, "/Users/AuthenticateByName", app("tv", ""), map[string]string{"Username": "alice", "Pw": "wrong password"})
	entries := func(query string) listResult[ActivityLogEntry] {
		t.Helper()
		var page listResult[ActivityLogEntry]
		status, body := s.call(http.MethodGet, "/System/ActivityLog/Entries"+query, admin, nil)
		if status != http.StatusOK || json.Unmarshal(body, &page) != nil {
			t.Fatalf("%s: %d %s", query, status, body)
		}
		return page
	}
	all := entries("")
	if all.TotalRecordCount != 3 || all.Items[0].Type != "AuthenticationFailed" || all.Items[0].Severity != "Error" || all.Items[0].UserId != zeroID {
		t.Fatalf("entries: %+v", all)
	}
	if page := entries("?startIndex=1&limit=1"); len(page.Items) != 1 || page.Items[0].Id != all.Items[1].Id || page.TotalRecordCount != 3 || page.StartIndex != 1 {
		t.Errorf("second page: %+v", page)
	}
	if page := entries("?hasUserId=false"); page.TotalRecordCount != 1 {
		t.Errorf("without user: %+v", page)
	}
	if page := entries("?minDate=" + time.Now().Add(time.Hour).UTC().Format(time.RFC3339)); page.TotalRecordCount != 0 {
		t.Errorf("from an hour on: %+v", page)
	}
}

func TestScheduledTasksRunStopAndKeepTheirTriggers(t *testing.T) {
	s, admin, _ := administrated(t)
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	s.tasks.Register(tasks.Task{Key: "Wait", Category: tasks.CategoryMaintenance, Interval: time.Hour,
		Text: map[string]tasks.Text{"en": {Name: "Wait"}}, Run: func(ctx context.Context) error { <-ctx.Done(); return nil }})
	s.tasks.Start(ctx)
	id := tasks.ID("Wait")
	task := func() TaskInfo {
		var info TaskInfo
		_, body := s.call(http.MethodGet, "/ScheduledTasks/"+id, admin, nil)
		_ = json.Unmarshal(body, &info)
		return info
	}
	if info := task(); info.State != tasks.Idle || len(info.Triggers) != 1 || info.Triggers[0].IntervalTicks != int64(time.Hour/100) {
		t.Errorf("before: %+v", info)
	}
	s.call(http.MethodPost, "/ScheduledTasks/Running/"+id, admin, nil)
	if info := task(); info.State != tasks.Running {
		t.Errorf("started: %+v", info)
	}
	s.call(http.MethodDelete, "/ScheduledTasks/Running/"+id, admin, nil)
	for deadline := time.Now().Add(5 * time.Second); task().LastExecutionResult == nil && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
	}
	if info := task(); info.State != tasks.Idle || info.LastExecutionResult == nil || info.LastExecutionResult.Status != tasks.Cancelled {
		t.Errorf("stopped: %+v", info)
	}
	if status, _ := s.call(http.MethodPost, "/ScheduledTasks/"+id+"/Triggers", admin, []TaskTriggerInfo{}); status != http.StatusBadRequest {
		t.Errorf("triggers changed: %d", status)
	}
	if status, _ := s.call(http.MethodGet, "/ScheduledTasks/nothing", admin, nil); status != http.StatusNotFound {
		t.Errorf("unknown task: %d", status)
	}
}

func TestLogsAreServedRedacted(t *testing.T) {
	s, admin, member := administrated(t)
	_, _ = s.logs.Write([]byte(`level=INFO msg="An addon failed" url=https://addon.example/secret-config/manifest.json token=abc123` + "\n"))
	status, body := s.call(http.MethodGet, "/System/Logs/Log?name=POLYFIN.log", admin, nil)
	if status != http.StatusOK || !strings.Contains(string(body), "https://addon.example/…") ||
		strings.Contains(string(body), "secret-config") || strings.Contains(string(body), "abc123") {
		t.Errorf("log: %d %s", status, body)
	}
	if status, body := s.call(http.MethodGet, "/System/Logs/Log?name=other.log", admin, nil); status != http.StatusNotFound || string(body) != "\"Log file not found.\"\n" {
		t.Errorf("unknown log: %d %s", status, body)
	}
	if status, _ := s.call(http.MethodGet, "/System/Logs/Log", admin, nil); status != http.StatusBadRequest {
		t.Errorf("without name: %d", status)
	}
	if status, _ := s.postRaw("/ClientLog/Document", member, strings.Repeat("a", maxClientLog+1)); status != http.StatusRequestEntityTooLarge {
		t.Errorf("large app log: %d", status)
	}
}

func TestServerConfigurationAppliesPolyfinSettings(t *testing.T) {
	s, admin, member := administrated(t)
	status, _ := s.call(http.MethodPost, "/System/Configuration", admin, map[string]any{
		"ServerName": "Den", "UICulture": "fr-FR", "MinResumePct": 10, "MaxResumePct": 80, "EnableMetrics": true, "QuickConnectAvailable": false})
	if status != http.StatusNoContent {
		t.Fatalf("save: %d", status)
	}
	settings := s.store.Settings()
	if settings.ServerName != "Den" || settings.Language != "fr" || settings.ResumePercent != 10 || settings.PlayedPercent != 80 || settings.QuickConnectEnabled {
		t.Errorf("settings: %+v", settings)
	}
	var configuration ServerConfiguration
	_, body := s.call(http.MethodGet, "/System/Configuration", member, nil)
	_ = json.Unmarshal(body, &configuration)
	if configuration.ServerName != "Den" || configuration.UICulture != "fr" || configuration.MaxResumePct != 80 || configuration.MinResumeDurationSeconds != 300 {
		t.Errorf("configuration: %+v", configuration)
	}
	if status, _ := s.call(http.MethodPost, "/System/Configuration", admin, map[string]any{"MinResumePct": 90}); status != http.StatusBadRequest {
		t.Errorf("resume above played: %d", status)
	}
	if status, _ := s.call(http.MethodPost, "/System/Configuration", member, map[string]any{"ServerName": "Mine"}); status != http.StatusForbidden {
		t.Errorf("member: %d", status)
	}
	if status, body := s.call(http.MethodGet, "/System/Configuration/branding", member, nil); status != http.StatusOK || string(body) != "{\"SplashscreenEnabled\":false}\n" {
		t.Errorf("branding: %d %s", status, body)
	}
}
