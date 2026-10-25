package admin

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/desktop"
	"github.com/moodiness/polyfin/internal/updatetool"
)

// activityNames lists the names of the activity entries of type kind.
func activityNames(t *testing.T, admin browser, kind string) []string {
	t.Helper()
	names := []string{}
	for _, entry := range activityOf(t, admin, kind) {
		name, _ := entry["name"].(string)
		names = append(names, name)
	}
	return names
}

// Restart stops Polyfin for an administrator who confirms it with their
// password, once a minute, recording who did; while nothing Polyfin
// knows of starts it again, the request must say anyway.
func TestRestartIsAnAdministratorsConfirmedAction(t *testing.T) {
	t.Parallel()
	restarts := make(chan struct{}, 10)
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.Restart = func() { restarts <- struct{}{} }
		o.Health.Started = time.Now().Add(-time.Hour)
	})
	admin := api.signedIn("root", true)
	member := api.signedIn("alice", false)
	right := map[string]any{"password": "correct horse", "anyway": true}

	if status, _, _ := member.call(http.MethodPost, "/server/restart", right); status != http.StatusForbidden {
		t.Errorf("member: %d", status)
	}
	if status, _, _ := member.call(http.MethodGet, "/server", nil); status != http.StatusForbidden {
		t.Errorf("member reads the server's actions: %d", status)
	}
	var state serverJSON
	if admin.raw(http.MethodGet, "/server", "", &state); state.Restart != restartUnknown || state.Version != "1.2.3" || state.StartedAt.IsZero() {
		t.Errorf("server: %+v", state)
	}
	// Nothing known starts Polyfin again: the warning must have been seen.
	if status, body, _ := admin.call(http.MethodPost, "/server/restart", map[string]any{"password": "correct horse"}); status != http.StatusConflict ||
		body["error"] != "restart_unconfirmed" {
		t.Errorf("without anyway: %d %v", status, body)
	}
	if status, body, _ := admin.call(http.MethodPost, "/server/restart", map[string]any{"password": "wrong horse", "anyway": true}); status != http.StatusForbidden ||
		body["error"] != "wrong_password" {
		t.Errorf("wrong password: %d %v", status, body)
	}
	if status, body, _ := admin.call(http.MethodPost, "/server/restart", map[string]any{"anyway": true}); status != http.StatusForbidden {
		t.Errorf("no password: %d %v", status, body)
	}
	if len(restarts) != 0 || len(activityNames(t, admin, "ServerRestarted")) != 0 {
		t.Fatal("a refused request restarted Polyfin")
	}

	status, body, _ := admin.call(http.MethodPost, "/server/restart", right)
	if status != http.StatusAccepted || body["startedAt"] != state.StartedAt.Format(time.RFC3339Nano) {
		t.Fatalf("restart: %d %v", status, body)
	}
	<-restarts
	if names := activityNames(t, admin, "ServerRestarted"); len(names) != 1 || names[0] != "root restarted Polyfin" {
		t.Errorf("activity: %v", names)
	}
	// Once a minute.
	status, body, response := admin.call(http.MethodPost, "/server/restart", right)
	if status != http.StatusTooManyRequests || body["error"] != "too_soon" || response.Header.Get("Retry-After") == "" {
		t.Errorf("again at once: %d %v", status, body)
	}
	later := time.Now().Add(time.Minute + time.Second)
	api.clock.Store(&later)

	// Confirmed in the settings, Polyfin restarts without anyway.
	settings := api.store.Settings()
	settings.RestartConfirmed = true
	if _, err := api.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	if admin.raw(http.MethodGet, "/server", "", &state); state.Restart != restartConfirmed {
		t.Errorf("confirmed: %+v", state)
	}
	if status, body, _ := admin.call(http.MethodPost, "/server/restart", map[string]any{"password": "correct horse"}); status != http.StatusAccepted {
		t.Errorf("confirmed restart: %d %v", status, body)
	}
	<-restarts
}

// Polyfin Desktop starts Polyfin again: its restart needs no anyway.
func TestRestartUnderPolyfinDesktop(t *testing.T) {
	t.Parallel()
	restarts := make(chan struct{}, 1)
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.Restart = func() { restarts <- struct{}{} }
		o.Desktop = true
		o.Health.Started = time.Now().Add(-time.Hour)
	})
	admin := api.signedIn("root", true)
	var state serverJSON
	if admin.raw(http.MethodGet, "/server", "", &state); state.Restart != restartDesktop {
		t.Errorf("server: %+v", state)
	}
	if status, body, _ := admin.call(http.MethodPost, "/server/restart", map[string]any{"password": "correct horse"}); status != http.StatusAccepted {
		t.Fatalf("restart: %d %v", status, body)
	}
	<-restarts
}

// The limit holds across a restart: the new Polyfin refuses a restart
// within a minute of its start as the old one did within a minute of the
// last, while the rest of the admin app, and the other actions, work at
// once.
func TestRestartIsRefusedWithinAMinuteOfStart(t *testing.T) {
	t.Parallel()
	restarts := make(chan struct{}, 1)
	tool, received := fakeTool(t, http.StatusOK, "ok", false)
	started := time.Now()
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.Restart = func() { restarts <- struct{}{} }
		o.Health.Started = started
		o.UpdateTool = updatetool.New("Polyfin/1.2.3")
	})
	saveTool(t, api, tool.URL+"/v1/update", "")
	admin := api.signedIn("root", true)
	right := map[string]any{"password": "correct horse", "anyway": true}

	status, body, response := admin.call(http.MethodPost, "/server/restart", right)
	if status != http.StatusTooManyRequests || body["error"] != "too_soon" || response.Header.Get("Retry-After") == "" {
		t.Errorf("restart at start-up: %d %v", status, body)
	}
	if len(restarts) != 0 || len(activityNames(t, admin, "ServerRestarted")) != 0 {
		t.Fatal("Polyfin restarted at start-up")
	}
	for _, path := range []string{"/status", "/server", "/session", "/health", "/activity"} {
		if status, body, _ := admin.call(http.MethodGet, path, nil); status != http.StatusOK {
			t.Errorf("%s at start-up: %d %v", path, status, body)
		}
	}
	if status, body, _ := admin.call(http.MethodPost, "/server/update-tool", map[string]any{"password": "correct horse"}); status != http.StatusOK || len(received()) != 1 {
		t.Errorf("update now at start-up: %d %v", status, body)
	}

	later := started.Add(time.Minute + time.Second)
	api.clock.Store(&later)
	if status, body, _ := admin.call(http.MethodPost, "/server/restart", right); status != http.StatusAccepted {
		t.Fatalf("restart a minute after start-up: %d %v", status, body)
	}
	<-restarts
}

// Polyfin Desktop's update is offered while Polyfin Desktop runs Polyfin
// and offers a version newer than the server, to administrators, and is
// installed at their confirmed request, through the running app.
func TestDesktopUpdateIsOfferedAndRequested(t *testing.T) {
	t.Parallel()
	data := t.TempDir()
	folders := desktop.Folders{Data: data}
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.Desktop, o.DataDir = true, data
	})
	admin := api.signedIn("root", true)
	member := api.signedIn("alice", false)
	request := map[string]any{"password": "correct horse", "version": "1.3.0"}
	offered := func() *desktopUpdateJSON {
		t.Helper()
		var state serverJSON
		if status := admin.raw(http.MethodGet, "/server", "", &state); status != http.StatusOK {
			t.Fatalf("server: %d", status)
		}
		return state.DesktopUpdate
	}

	if offered() != nil {
		t.Error("an update is offered before Polyfin Desktop offers one")
	}
	if status, body, _ := admin.call(http.MethodPost, "/server/desktop-update", request); status != http.StatusConflict || body["error"] != "no_update" {
		t.Errorf("nothing offered: %d %v", status, body)
	}
	// Polyfin Desktop offers the version this server runs: nothing new.
	if err := desktop.WriteOffer(folders, &desktop.Offered{Version: "1.2.3"}); err != nil {
		t.Fatal(err)
	}
	if offered() != nil {
		t.Error("the running version is offered")
	}
	if err := desktop.WriteOffer(folders, &desktop.Offered{Version: "1.3.0", Notes: "https://github.com/moodiness/polyfin/releases/tag/v1.3.0"}); err != nil {
		t.Fatal(err)
	}
	if got := offered(); got == nil || got.Version != "1.3.0" || got.Notes == "" || got.Installing || got.Failed {
		t.Fatalf("offer: %+v", got)
	}
	if status, _, _ := member.call(http.MethodPost, "/server/desktop-update", request); status != http.StatusForbidden {
		t.Errorf("member: %d", status)
	}
	if status, body, _ := admin.call(http.MethodPost, "/server/desktop-update", map[string]any{"password": "correct horse", "version": "1.4.0"}); status != http.StatusConflict ||
		body["error"] != "no_update" {
		t.Errorf("another version: %d %v", status, body)
	}
	if status, body, _ := admin.call(http.MethodPost, "/server/desktop-update", map[string]any{"password": "wrong horse", "version": "1.3.0"}); status != http.StatusForbidden {
		t.Errorf("wrong password: %d %v", status, body)
	}
	// Polyfin Desktop does not run: its record is missing.
	if status, body, _ := admin.call(http.MethodPost, "/server/desktop-update", request); status != http.StatusConflict || body["error"] != "no_update" {
		t.Errorf("no app: %d %v", status, body)
	}
	if _, err := os.Stat(folders.InstallRequest()); err == nil {
		t.Error("a request was written without an app")
	}

	if err := os.WriteFile(folders.Instance(), []byte("4242 app-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Minute + time.Second)
	api.clock.Store(&later)
	if status, body, _ := admin.call(http.MethodPost, "/server/desktop-update", request); status != http.StatusAccepted || body["version"] != "1.3.0" {
		t.Fatalf("install: %d %v", status, body)
	}
	if version, ok := desktop.InstallRequested(folders, "app-token"); !ok || version != "1.3.0" {
		t.Errorf("request: %q %v", version, ok)
	}
	if names := activityNames(t, admin, "ServerUpdateStarted"); len(names) != 1 || names[0] != "root had Polyfin Desktop install Polyfin 1.3.0" {
		t.Errorf("activity: %v", names)
	}
	if status, _, _ := admin.call(http.MethodPost, "/server/desktop-update", request); status != http.StatusTooManyRequests {
		t.Errorf("again at once: %d", status)
	}
	// While Polyfin Desktop downloads it, it is not asked again.
	if err := desktop.WriteOffer(folders, &desktop.Offered{Version: "1.3.0", Installing: true, Percent: 40}); err != nil {
		t.Fatal(err)
	}
	if got := offered(); got == nil || !got.Installing || got.Percent != 40 {
		t.Errorf("installing: %+v", got)
	}
	evenLater := later.Add(time.Minute + time.Second)
	api.clock.Store(&evenLater)
	if status, body, _ := admin.call(http.MethodPost, "/server/desktop-update", request); status != http.StatusConflict || body["error"] != "no_update" {
		t.Errorf("while installing: %d %v", status, body)
	}
}

// Elsewhere than in Polyfin Desktop, its update is neither offered nor
// installed, whatever the data folder holds.
func TestDesktopUpdateOutsideDesktop(t *testing.T) {
	t.Parallel()
	data := t.TempDir()
	folders := desktop.Folders{Data: data}
	if err := desktop.WriteOffer(folders, &desktop.Offered{Version: "1.3.0"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(folders.Instance(), []byte("4242 app-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) { o.DataDir = data })
	admin := api.signedIn("root", true)
	var state serverJSON
	if admin.raw(http.MethodGet, "/server", "", &state); state.DesktopUpdate != nil || state.Restart != restartUnknown {
		t.Errorf("server: %+v", state)
	}
	if status, body, _ := admin.call(http.MethodPost, "/server/desktop-update", map[string]any{"password": "correct horse", "version": "1.3.0"}); status != http.StatusNotFound ||
		body["error"] != "not_desktop" {
		t.Errorf("install: %d %v", status, body)
	}
	if _, err := os.Stat(folders.InstallRequest()); err == nil {
		t.Error("a request was written")
	}
}

// toolRequest is what the fake update tool received.
type toolRequest struct {
	method, uri, authorization, userAgent, body string
}

// fakeTool is an update tool that records its requests and answers with
// answer, or holds its answer until the test ends while hold is set.
func fakeTool(t *testing.T, status int, answer string, hold bool) (*httptest.Server, func() []toolRequest) {
	t.Helper()
	var mu sync.Mutex
	var got []toolRequest
	release := make(chan struct{})
	tool := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, toolRequest{r.Method, r.RequestURI, r.Header.Get("Authorization"), r.Header.Get("User-Agent"), string(body)})
		mu.Unlock()
		if hold {
			<-release
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, answer)
	}))
	t.Cleanup(func() {
		close(release)
		tool.Close()
	})
	return tool, func() []toolRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]toolRequest(nil), got...)
	}
}

// saveTool saves the update tool's address and token.
func saveTool(t *testing.T, api testAPI, address, token string) {
	t.Helper()
	settings := api.store.Settings()
	settings.UpdateToolURL, settings.UpdateToolToken = address, token
	if _, err := api.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
}

// Update now sends a POST to the update tool's address, with its token,
// at an administrator's confirmed request, once a minute, and answers
// what the tool said, whatever it said.
func TestUpdateNowAsksTheUpdateTool(t *testing.T) {
	t.Parallel()
	tool, received := fakeTool(t, http.StatusAccepted, "Updating 2 containers\n", false)
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) { o.UpdateTool = updatetool.New("Polyfin/1.2.3") })
	admin := api.signedIn("root", true)
	member := api.signedIn("alice", false)
	right := map[string]any{"password": "correct horse"}

	var state serverJSON
	if admin.raw(http.MethodGet, "/server", "", &state); state.UpdateTool {
		t.Error("a tool before one is set")
	}
	if status, body, _ := admin.call(http.MethodPost, "/server/update-tool", right); status != http.StatusConflict || body["error"] != "no_update_tool" {
		t.Errorf("no tool: %d %v", status, body)
	}
	saveTool(t, api, tool.URL+"/v1/update?scope=media", "s3cret-token")
	if admin.raw(http.MethodGet, "/server", "", &state); !state.UpdateTool {
		t.Error("the tool set is not told")
	}
	if status, _, _ := member.call(http.MethodPost, "/server/update-tool", right); status != http.StatusForbidden {
		t.Errorf("member: %d", status)
	}
	if status, _, _ := admin.call(http.MethodPost, "/server/update-tool", map[string]any{"password": "wrong horse"}); status != http.StatusForbidden {
		t.Errorf("wrong password: %d", status)
	}
	if got := received(); len(got) != 0 {
		t.Fatalf("refused requests reached the tool: %+v", got)
	}

	status, body, _ := admin.call(http.MethodPost, "/server/update-tool", right)
	if status != http.StatusOK || body["status"] != float64(http.StatusAccepted) || body["body"] != "Updating 2 containers" {
		t.Fatalf("update now: %d %v", status, body)
	}
	got := received()
	if len(got) != 1 || got[0] != (toolRequest{http.MethodPost, "/v1/update?scope=media", "Bearer s3cret-token", "Polyfin/1.2.3", ""}) {
		t.Errorf("the tool received %+v", got)
	}
	if names := activityNames(t, admin, "ServerUpdateStarted"); len(names) != 1 || names[0] != "root asked the update tool to update now" {
		t.Errorf("activity: %v", names)
	}
	if status, _, _ := admin.call(http.MethodPost, "/server/update-tool", right); status != http.StatusTooManyRequests || len(received()) != 1 {
		t.Errorf("again at once: %d", status)
	}

	// Without a token, no Authorization header; a refusal is answered as
	// the tool said it.
	refusing, refusals := fakeTool(t, http.StatusUnauthorized, "invalid token", false)
	saveTool(t, api, refusing.URL+"/v1/update", "")
	later := time.Now().Add(time.Minute + time.Second)
	api.clock.Store(&later)
	status, body, _ = admin.call(http.MethodPost, "/server/update-tool", right)
	if status != http.StatusOK || body["status"] != float64(http.StatusUnauthorized) || body["body"] != "invalid token" {
		t.Errorf("refused: %d %v", status, body)
	}
	if got := refusals(); len(got) != 1 || got[0].authorization != "" {
		t.Errorf("without a token: %+v", got)
	}
}

// A tool that cannot be reached, or does not answer in time, is told as
// such, without the address, which may hold a secret.
func TestUpdateNowTellsWhyTheToolDidNotAnswer(t *testing.T) {
	t.Parallel()
	silent, _ := fakeTool(t, http.StatusOK, "", true)
	client := updatetool.New("Polyfin/1.2.3")
	client.Timeout = 200 * time.Millisecond
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) { o.UpdateTool = client })
	admin := api.signedIn("root", true)
	right := map[string]any{"password": "correct horse"}

	saveTool(t, api, silent.URL+"/v1/update", "s3cret-token")
	if status, body, _ := admin.call(http.MethodPost, "/server/update-tool", right); status != http.StatusBadGateway || body["error"] != "update_tool_timeout" {
		t.Errorf("silent: %d %v", status, body)
	}

	gone := httptest.NewServer(http.NotFoundHandler())
	address := gone.URL + "/v1/update?token=in-the-query"
	gone.Close()
	saveTool(t, api, address, "")
	later := time.Now().Add(time.Minute + time.Second)
	api.clock.Store(&later)
	status, body, _ := admin.call(http.MethodPost, "/server/update-tool", right)
	message, _ := body["message"].(string)
	if status != http.StatusBadGateway || body["error"] != "update_tool_unreachable" || message == "" || strings.Contains(message, "in-the-query") {
		t.Errorf("unreachable: %d %v", status, body)
	}
}

// The update tool's address is checked, and its token, a secret, is
// never sent back: only whether one is saved, and it is revealed on
// demand, as the other secrets are.
func TestUpdateToolSettings(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	admin := api.signedIn("root", true)
	for _, address := range []string{"ftp://updater/v1/update", "http://user:pw@updater/v1/update", "http://updater/v1/update#x", "updater:8080"} {
		if status, body, _ := admin.call(http.MethodPut, "/settings", map[string]any{"serverName": "Polyfin", "language": "en", "updateToolUrl": address}); status != http.StatusBadRequest ||
			body["error"] != "invalid_update_tool_url" {
			t.Errorf("%s: %d %v", address, status, body)
		}
	}
	if status, body, _ := admin.call(http.MethodPut, "/settings", map[string]any{"serverName": "Polyfin", "language": "en", "updateToolToken": "two words"}); status != http.StatusBadRequest ||
		body["error"] != "invalid_update_tool_token" {
		t.Errorf("token with a space: %d %v", status, body)
	}
	status, body, _ := admin.call(http.MethodPut, "/settings", map[string]any{"serverName": "Polyfin", "language": "en",
		"updateToolUrl": " http://updater:8080/v1/update ", "updateToolToken": "s3cret-token", "restartConfirmed": true})
	if status != http.StatusOK || body["updateToolUrl"] != "http://updater:8080/v1/update" || body["updateToolTokenSet"] != true || body["restartConfirmed"] != true {
		t.Fatalf("save: %d %v", status, body)
	}
	_, settings, _ := admin.call(http.MethodGet, "/settings", nil)
	if _, sent := settings["updateToolToken"]; sent || strings.Contains(strings.Join(activityNames(t, admin, ""), " "), "s3cret") {
		t.Errorf("the token is sent back: %v", settings)
	}
	if status, body, _ := admin.call(http.MethodPost, "/settings/secrets/updateToolToken/reveal", nil); status != http.StatusOK || body["value"] != "s3cret-token" {
		t.Errorf("reveal: %d %v", status, body)
	}
	if names := activityNames(t, admin, "SecretRevealed"); len(names) != 1 || names[0] != "root revealed the update tool's token" {
		t.Errorf("activity: %v", names)
	}
	// Leaving the token out keeps it; empty removes it.
	admin.call(http.MethodPut, "/settings", map[string]any{"serverName": "Polyfin", "language": "en"})
	if api.store.Settings().UpdateToolToken != "s3cret-token" {
		t.Error("the token was not kept")
	}
	admin.call(http.MethodPut, "/settings", map[string]any{"serverName": "Polyfin", "language": "en", "updateToolToken": ""})
	if api.store.Settings().UpdateToolToken != "" {
		t.Error("the token was not removed")
	}
}
