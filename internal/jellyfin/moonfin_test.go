package jellyfin

import (
	"bufio"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/moonfin"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// moonfinServer is a test server with an administrator, root, and two
// members, alice and bob, whose users it returns by name. configure, if
// any, completes the handler's options.
func moonfinServer(t *testing.T, configure ...func(*Options, *pgxpool.Pool)) (testServer, map[string]accounts.User) {
	t.Helper()
	s := newProbingServer(t, 10, "ffprobe-not-installed", configure...)
	users := map[string]accounts.User{
		"root":  s.user("root", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) }),
		"alice": s.user("alice", nil),
		"bob":   s.user("bob", nil),
	}
	return s, users
}

// moonfinJSON calls path and decodes its JSON answer.
func moonfinJSON(t *testing.T, s testServer, method, path, auth string, body any) (int, map[string]any) {
	t.Helper()
	status, data := s.call(method, path, auth, body)
	var answer map[string]any
	_ = json.Unmarshal(data, &answer)
	return status, answer
}

// Moonfin's apps find the companion, with the features Polyfin serves and
// the administrator's defaults for the device's kind, and those it does
// not serve turned off.
func TestMoonfinPingTellsWhatIsServed(t *testing.T) {
	t.Parallel()
	// A TMDB client, which the ping never calls.
	s, _ := moonfinServer(t, func(o *Options, _ *pgxpool.Pool) {
		o.TMDB = tmdb.New("http://127.0.0.1:1", "http://127.0.0.1:1", "test", o.Accounts.Settings)
	})
	alice := app("phone", s.signIn("alice", "phone"))
	if err := s.handler.Moonfin.SetDefaults(t.Context(), moonfin.Global, moonfin.Settings{"clock": "always", "theme": "dark"}); err != nil {
		t.Fatal(err)
	}
	if err := s.handler.Moonfin.SetDefaults(t.Context(), moonfin.TV, moonfin.Settings{"theme": "oled"}); err != nil {
		t.Fatal(err)
	}
	status, ping := moonfinJSON(t, s, http.MethodGet, "/Moonfin/Ping?profile=tv", alice, nil)
	if status != http.StatusOK || ping["installed"] != true || ping["settingsSyncEnabled"] != true || ping["version"] != "1.2.3" {
		t.Fatalf("ping: %d %v", status, ping)
	}
	defaults, _ := ping["defaultSettings"].(map[string]any)
	if defaults["theme"] != "oled" || defaults["clock"] != "always" {
		t.Errorf("TV defaults: %v", defaults)
	}
	for _, flag := range []string{"mdblistAvailable", "tmdbAvailable", "tmdbReviewsSupported", "seerrEnabled",
		"recommendationsSupported", "pushSupported", "calendarAvailable", "smartSearchEnabled"} {
		if ping[flag] != false {
			t.Errorf("%s: %v", flag, ping[flag])
		}
	}
	if ping["clientLogSupported"] != true {
		t.Errorf("clientLogSupported: %v", ping["clientLogSupported"])
	}
	s.setting(t, func(settings *accounts.Settings) {
		settings.MDBListRatingsKey, settings.TMDBKey = "mdblist-key", "tmdb-key"
	})
	if _, ping = moonfinJSON(t, s, http.MethodGet, "/Moonfin/Ping", alice, nil); ping["mdblistAvailable"] != true ||
		ping["tmdbReviewsSupported"] != true || ping["tmdbAvailable"] != false {
		t.Errorf("with the server's keys: %v", ping)
	}
	if keys, _ := json.Marshal(ping); strings.Contains(string(keys), "mdblist-key") || strings.Contains(string(keys), "tmdb-key") {
		t.Error("the ping gives the server's keys")
	}
	if status, _ := s.call(http.MethodGet, "/Moonfin/Ping", "", nil); status != http.StatusUnauthorized {
		t.Errorf("signed out: %d", status)
	}
}

// An app saves its profiles and reads them back resolved: its device's
// own settings over the user's global ones over the administrator's
// defaults, each user's apart; HEAD tells whether a user has settings.
func TestMoonfinSettingsSyncBetweenDevices(t *testing.T) {
	t.Parallel()
	s, _ := moonfinServer(t)
	phone, tv := app("phone", s.signIn("alice", "phone")), app("tv", s.signIn("alice", "tv"))
	bob := app("bob-tv", s.signIn("bob", "bob-tv"))
	if status, _ := s.call(http.MethodHead, "/Moonfin/Settings", phone, nil); status != http.StatusNotFound {
		t.Errorf("HEAD before any save: %d", status)
	}
	if err := s.handler.Moonfin.SetDefaults(t.Context(), moonfin.Global, moonfin.Settings{"clock": "always", "theme": "light"}); err != nil {
		t.Fatal(err)
	}
	status, saved := moonfinJSON(t, s, http.MethodPost, "/Moonfin/Settings/Profile/global", phone,
		map[string]any{"profile": map[string]any{"theme": "dark", "navbarPosition": "top"}, "clientId": "moonfin-flutter"})
	if status != http.StatusOK || saved["Success"] != true || saved["Created"] != true {
		t.Fatalf("save: %d %v", status, saved)
	}
	if status, saved = moonfinJSON(t, s, http.MethodPost, "/Moonfin/Settings/Profile/TV", tv,
		map[string]any{"profile": map[string]any{"navbarPosition": "left"}, "clientId": "moonfin-flutter"}); status != http.StatusOK || saved["Created"] != false {
		t.Fatalf("TV save: %d %v", status, saved)
	}
	if status, _ := s.call(http.MethodHead, "/Moonfin/Settings", phone, nil); status != http.StatusOK {
		t.Errorf("HEAD after a save: %d", status)
	}
	_, resolved := moonfinJSON(t, s, http.MethodGet, "/Moonfin/Settings/Resolved/tv", tv, nil)
	if resolved["theme"] != "dark" || resolved["navbarPosition"] != "left" || resolved["clock"] != "always" {
		t.Errorf("resolved for the TV: %v", resolved)
	}
	_, resolved = moonfinJSON(t, s, http.MethodGet, "/Moonfin/Settings/Resolved/mobile", phone, nil)
	if resolved["navbarPosition"] != "top" {
		t.Errorf("resolved for the phone: %v", resolved)
	}
	_, resolved = moonfinJSON(t, s, http.MethodGet, "/Moonfin/Settings/Resolved/desktop", bob, nil)
	if resolved["theme"] != "light" || resolved["navbarPosition"] != nil {
		t.Errorf("bob, who never synced: %v", resolved)
	}
	_, env := moonfinJSON(t, s, http.MethodGet, "/Moonfin/Settings", phone, nil)
	if env["schemaVersion"] != float64(2) || env["lastUpdatedBy"] != "moonfin-flutter" || env["tv"] == nil {
		t.Errorf("the envelope: %v", env)
	}
	if status, _ := s.call(http.MethodGet, "/Moonfin/Settings/Resolved/watch", phone, nil); status != http.StatusBadRequest {
		t.Errorf("an unknown profile: %d", status)
	}
	if status, _ := s.call(http.MethodDelete, "/Moonfin/Settings/Profile/global", phone, nil); status != http.StatusBadRequest {
		t.Errorf("deleting the global profile: %d", status)
	}
	if status, _ := s.call(http.MethodDelete, "/Moonfin/Settings/Profile/tv", phone, nil); status != http.StatusOK {
		t.Errorf("deleting the TV profile: %d", status)
	}
	if _, resolved = moonfinJSON(t, s, http.MethodGet, "/Moonfin/Settings/Resolved/tv", tv, nil); resolved["navbarPosition"] != "top" {
		t.Errorf("the TV without its profile: %v", resolved)
	}
	status, saved = moonfinJSON(t, s, http.MethodPost, "/Moonfin/Settings", phone,
		map[string]any{"settings": map[string]any{"global": map[string]any{"theme": "amoled"}}, "mergeMode": "replace"})
	if status != http.StatusOK {
		t.Fatalf("replace: %d %v", status, saved)
	}
	if _, resolved = moonfinJSON(t, s, http.MethodGet, "/Moonfin/Settings/Resolved/global", phone, nil); resolved["theme"] != "amoled" || resolved["navbarPosition"] != nil {
		t.Errorf("after a replace: %v", resolved)
	}
	if status, _ := s.call(http.MethodPost, "/Moonfin/Settings/Profile/global", phone, map[string]any{"clientId": "x"}); status != http.StatusBadRequest {
		t.Errorf("a save without a profile: %d", status)
	}
	if status, _ := s.call(http.MethodDelete, "/Moonfin/Settings", phone, nil); status != http.StatusOK {
		t.Errorf("delete: %d", status)
	}
	if status, _ := s.call(http.MethodGet, "/Moonfin/Settings", phone, nil); status != http.StatusNotFound {
		t.Errorf("after the delete: %d", status)
	}
}

// The companion is served as the server's switch says, unless a user is
// set apart: to a user it is not served to, every route answers 404, as a
// server without it does.
func TestMoonfinIsServedAsTheServerAndEachUserSay(t *testing.T) {
	t.Parallel()
	s, users := moonfinServer(t)
	alice, bob := app("phone", s.signIn("alice", "phone")), app("tv", s.signIn("bob", "tv"))
	served := func(auth string) bool {
		t.Helper()
		status, _ := s.call(http.MethodGet, "/Moonfin/Ping", auth, nil)
		return status == http.StatusOK
	}
	if !served(alice) || !served(bob) {
		t.Fatal("not served by default")
	}
	setApart := func(name, choice string) {
		t.Helper()
		if _, err := s.store.UpdateUser(t.Context(), users[name].ID, accounts.UserChanges{Moonfin: &choice}, nil); err != nil {
			t.Fatal(err)
		}
	}
	setApart("bob", accounts.MoonfinOff)
	if !served(alice) || served(bob) {
		t.Error("bob set apart off")
	}
	if status, _ := s.call(http.MethodPost, "/Moonfin/Settings/Profile/global", bob, map[string]any{"profile": map[string]any{"a": 1}}); status != http.StatusNotFound {
		t.Errorf("a save by bob: %d", status)
	}
	s.setting(t, func(settings *accounts.Settings) { settings.Moonfin = false })
	setApart("bob", accounts.MoonfinOn)
	if served(alice) || !served(bob) {
		t.Error("server off, bob set apart on")
	}
	setApart("bob", accounts.MoonfinServer)
	if served(bob) {
		t.Error("bob back to the server's choice")
	}
}

// Another user's settings are an administrator's alone to read and save.
func TestMoonfinUserSettingsAreForAdministrators(t *testing.T) {
	t.Parallel()
	s, users := moonfinServer(t)
	root, alice := app("dashboard", s.signIn("root", "dashboard")), app("phone", s.signIn("alice", "phone"))
	path := "/Moonfin/Settings/" + users["bob"].ID.String()
	body := map[string]any{"settings": map[string]any{"global": map[string]any{"theme": "dark"}}}
	if status, _ := s.call(http.MethodPost, path, alice, body); status != http.StatusForbidden {
		t.Errorf("a member saving another's settings: %d", status)
	}
	if status, _ := s.call(http.MethodGet, path, root, nil); status != http.StatusNotFound {
		t.Errorf("bob before any save: %d", status)
	}
	if status, _ := s.call(http.MethodPost, path, root, body); status != http.StatusOK {
		t.Errorf("an administrator saving bob's settings: %d", status)
	}
	if status, env := moonfinJSON(t, s, http.MethodGet, path, root, nil); status != http.StatusOK || env["global"] == nil {
		t.Errorf("bob's settings: %d %v", status, env)
	}
	if status, _ := s.call(http.MethodGet, path, alice, nil); status != http.StatusForbidden {
		t.Errorf("a member reading another's settings: %d", status)
	}
	if status, _ := s.call(http.MethodGet, "/Moonfin/Settings/0123456789abcdef0123456789abcdef", root, nil); status != http.StatusNotFound {
		t.Errorf("an unknown user: %d", status)
	}
}

// An app's open stream is told when a save of another device changes the
// user's settings.
func TestMoonfinStreamTellsOfChanges(t *testing.T) {
	t.Parallel()
	s, _ := moonfinServer(t)
	tv, phone := app("tv", s.signIn("alice", "tv")), app("phone", s.signIn("alice", "phone"))
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, s.url+"/Moonfin/Settings/Stream", nil)
	request.Header.Set("Authorization", tv)
	response, err := testClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream: %d %s", response.StatusCode, response.Header.Get("Content-Type"))
	}
	lines := make(chan string, 16)
	go func() {
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	// The stream is open once its first comment arrives.
	if line := <-lines; !strings.HasPrefix(line, ":") {
		t.Fatalf("first line: %q", line)
	}
	if status, _ := s.call(http.MethodPost, "/Moonfin/Settings/Profile/global", phone, map[string]any{"profile": map[string]any{"theme": "dark"}}); status != http.StatusOK {
		t.Fatalf("save: %d", status)
	}
	deadline := time.After(10 * time.Second)
	for {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatal("the stream ended")
			}
			if line == `data: {"type":"settingsUpdated"}` {
				return
			}
		case <-deadline:
			t.Fatal("no settingsUpdated")
		}
	}
}

// Polyfin keeps no custom themes, and takes app logs as Jellyfin's route
// does.
func TestMoonfinThemesAndAppLogs(t *testing.T) {
	t.Parallel()
	s, _ := moonfinServer(t)
	alice := app("phone", s.signIn("alice", "phone"))
	if status, body := s.call(http.MethodGet, "/Moonfin/Themes", alice, nil); status != http.StatusOK || strings.TrimSpace(string(body)) != "[]" {
		t.Errorf("themes: %d %s", status, body)
	}
	status, body := s.call(http.MethodPost, "/Moonfin/ClientLog/Document", alice, "a log")
	if status != http.StatusOK || !strings.Contains(string(body), "FileName") {
		t.Errorf("log: %d %s", status, body)
	}
}
