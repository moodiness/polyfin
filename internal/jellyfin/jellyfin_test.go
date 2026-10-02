package jellyfin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/preferences"
	"github.com/moodiness/polyfin/internal/quickconnect"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/testdb"
	"github.com/moodiness/polyfin/internal/throttle"
)

const testServerID = "0123456789abcdef0123456789abcdef"

type testServer struct {
	t       *testing.T
	store   *accounts.Store
	addons  *addons.Store
	library *library.Service
	pool    *pgxpool.Pool
	url     string
}

func newTestServer(t *testing.T, failures int) testServer {
	t.Helper()
	pool := testdb.New(t)
	if err := database.Migrate(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	store, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := stremio.NewClient("test")
	addonStore := addons.New(pool, client)
	secret, err := database.Secret(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	// Tests seed analyses instead of running ffprobe, which CI lacks.
	player, err := playback.New(pool, client, "ffprobe-not-installed", playback.NewSigner(secret), logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = player.Close() })
	lib := library.New(pool, addonStore, client, logger, func() string { return "en" })
	server := httptest.NewServer(New(Options{
		ServerID:      testServerID,
		Accounts:      store,
		QuickConnect:  quickconnect.New(),
		SignIns:       throttle.New(failures, time.Minute),
		WebSocketPort: 8096,
		Library:       lib,
		Stremio:       client,
		Playback:      player,
		Preferences:   preferences.New(pool),
		Logger:        logger,
	}))
	t.Cleanup(server.Close)
	return testServer{t: t, store: store, addons: addonStore, library: lib, pool: pool, url: server.URL}
}

func (s testServer) user(name string, change func(*accounts.UserChanges)) accounts.User {
	s.t.Helper()
	user, err := s.store.CreateUser(s.t.Context(), accounts.NewUser{Name: name, Password: "correct horse", IsHidden: true})
	if err != nil {
		s.t.Fatal(err)
	}
	if change != nil {
		var changes accounts.UserChanges
		change(&changes)
		if user, err = s.store.UpdateUser(s.t.Context(), user.ID, changes, nil); err != nil {
			s.t.Fatal(err)
		}
	}
	return user
}

func app(device, token string) string {
	header := fmt.Sprintf(`MediaBrowser Client="Test App", Device="%s", DeviceId="%s-id", Version="1.0.0"`, device, device)
	if token != "" {
		header += fmt.Sprintf(`, Token="%s"`, token)
	}
	return header
}

// call sends a request and returns the status and body.
func (s testServer) call(method, path, authorization string, body any) (int, []byte) {
	s.t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, _ := json.Marshal(body)
		reader = bytes.NewReader(encoded)
	}
	request, _ := http.NewRequestWithContext(s.t.Context(), method, s.url+path, reader)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		s.t.Fatal(err)
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)
	return response.StatusCode, payload
}

func (s testServer) signIn(name, device string) string {
	s.t.Helper()
	status, body := s.call(http.MethodPost, "/Users/AuthenticateByName", app(device, ""),
		map[string]string{"Username": name, "Pw": "correct horse"})
	if status != http.StatusOK {
		s.t.Fatalf("sign-in of %s: %d %s", name, status, body)
	}
	var result AuthenticationResult
	_ = json.Unmarshal(body, &result)
	return result.AccessToken
}

func TestSignInAndSignOut(t *testing.T) {
	s := newTestServer(t, 10)
	s.user("alice", nil)
	s.user("carol", func(c *accounts.UserChanges) { yes := true; c.IsDisabled = &yes })

	for _, tc := range []struct {
		name          string
		authorization string
		body          map[string]string
		status        int
	}{
		{"no app identity", "", map[string]string{"Username": "alice", "Pw": "correct horse"}, http.StatusBadRequest},
		{"wrong password", app("tv", ""), map[string]string{"Username": "alice", "Pw": "wrong horse"}, http.StatusUnauthorized},
		{"unknown user", app("tv", ""), map[string]string{"Username": "nobody", "Pw": "correct horse"}, http.StatusUnauthorized},
		{"disabled user", app("tv", ""), map[string]string{"Username": "carol", "Pw": "correct horse"}, http.StatusForbidden},
		// Apps send the body in Jellyfin's PascalCase; some send camelCase.
		{"camelCase body", app("tv", ""), map[string]string{"username": "ALICE", "pw": "correct horse"}, http.StatusOK},
	} {
		if status, body := s.call(http.MethodPost, "/Users/AuthenticateByName", tc.authorization, tc.body); status != tc.status {
			t.Errorf("%s: got %d %s, want %d", tc.name, status, body, tc.status)
		}
	}

	first := s.signIn("alice", "tv")
	if status, _ := s.call(http.MethodGet, "/Users/Me", app("tv", first), nil); status != http.StatusOK {
		t.Fatalf("/Users/Me with a fresh token: %d", status)
	}
	if status, _ := s.call(http.MethodGet, "/Users/Me?ApiKey="+first, "", nil); status != http.StatusOK {
		t.Fatalf("/Users/Me with ApiKey: %d", status)
	}
	second := s.signIn("alice", "tv")
	if status, _ := s.call(http.MethodGet, "/Users/Me", app("tv", first), nil); status != http.StatusUnauthorized {
		t.Errorf("the device's previous token still works: %d", status)
	}
	if status, _ := s.call(http.MethodPost, "/Sessions/Logout", app("tv", second), nil); status != http.StatusNoContent {
		t.Fatalf("logout: %d", status)
	}
	if status, _ := s.call(http.MethodGet, "/Users/Me", app("tv", second), nil); status != http.StatusUnauthorized {
		t.Errorf("token still works after logout: %d", status)
	}
}

func TestFailedSignInsAreThrottled(t *testing.T) {
	s := newTestServer(t, 2)
	s.user("alice", nil)
	wrong := map[string]string{"Username": "alice", "Pw": "wrong horse"}
	for range 2 {
		s.call(http.MethodPost, "/Users/AuthenticateByName", app("tv", ""), wrong)
	}
	right := map[string]string{"Username": "alice", "Pw": "correct horse"}
	if status, _ := s.call(http.MethodPost, "/Users/AuthenticateByName", app("tv", ""), right); status != http.StatusTooManyRequests {
		t.Errorf("sign-in after the failure limit: got %d, want 429", status)
	}
}

func TestQuickConnect(t *testing.T) {
	s := newTestServer(t, 10)
	alice := s.user("alice", nil)
	phone := s.signIn("alice", "phone")
	tv := app("tv", "")

	status, body := s.call(http.MethodPost, "/QuickConnect/Initiate", tv, nil)
	if status != http.StatusOK {
		t.Fatalf("initiate: %d %s", status, body)
	}
	var initiated QuickConnectResult
	_ = json.Unmarshal(body, &initiated)
	if len(initiated.Code) != 6 || len(initiated.Secret) != 64 || initiated.DeviceName != "tv" {
		t.Fatalf("unexpected request: %+v", initiated)
	}
	secret := map[string]string{"Secret": initiated.Secret}
	if status, _ := s.call(http.MethodPost, "/Users/AuthenticateWithQuickConnect", tv, secret); status != http.StatusNotFound {
		t.Errorf("sign-in before approval: got %d, want 404", status)
	}
	if status, _ := s.call(http.MethodPost, "/QuickConnect/Authorize?code="+initiated.Code, "", nil); status != http.StatusUnauthorized {
		t.Errorf("anonymous approval: got %d, want 401", status)
	}
	if status, _ := s.call(http.MethodPost, "/QuickConnect/Authorize?Code="+initiated.Code, app("phone", phone), nil); status != http.StatusOK {
		t.Fatalf("approval: %d", status)
	}
	_, body = s.call(http.MethodGet, "/QuickConnect/Connect?secret="+initiated.Secret, "", nil)
	var polled QuickConnectResult
	if json.Unmarshal(body, &polled) != nil || !polled.Authenticated {
		t.Fatalf("poll after approval: %s", body)
	}
	status, body = s.call(http.MethodPost, "/Users/AuthenticateWithQuickConnect", tv, secret)
	var result AuthenticationResult
	if status != http.StatusOK || json.Unmarshal(body, &result) != nil {
		t.Fatalf("sign-in after approval: %d %s", status, body)
	}
	if result.User.Id != alice.ID.String() || result.SessionInfo.DeviceName != "tv" {
		t.Errorf("signed in as %s on %s, want alice on tv", result.User.Name, result.SessionInfo.DeviceName)
	}

	if _, err := s.store.UpdateSettings(t.Context(), accounts.Settings{ServerName: "Polyfin", Language: "en"}); err != nil {
		t.Fatal(err)
	}
	if _, body := s.call(http.MethodGet, "/QuickConnect/Enabled", "", nil); strings.TrimSpace(string(body)) != "false" {
		t.Errorf("Enabled after turning Quick Connect off: %s", body)
	}
	if status, _ := s.call(http.MethodPost, "/QuickConnect/Initiate", tv, nil); status != http.StatusUnauthorized {
		t.Errorf("initiate with Quick Connect off: got %d, want 401", status)
	}
}

func TestLegacyAuthorizationFollowsTheSetting(t *testing.T) {
	s := newTestServer(t, 10)
	s.user("alice", nil)
	token := s.signIn("alice", "tv")
	if status, _ := s.call(http.MethodGet, "/Users/Me?api_key="+token, "", nil); status != http.StatusUnauthorized {
		t.Errorf("api_key by default: got %d, want 401", status)
	}
	if _, err := s.store.UpdateSettings(t.Context(), accounts.Settings{ServerName: "Polyfin", LegacyAuthorization: true, Language: "en"}); err != nil {
		t.Fatal(err)
	}
	if status, _ := s.call(http.MethodGet, "/Users/Me?api_key="+token, "", nil); status != http.StatusOK {
		t.Errorf("api_key with legacy authorization on: got %d, want 200", status)
	}
}

func TestPublicUsersListsOnlyVisibleEnabledUsers(t *testing.T) {
	s := newTestServer(t, 10)
	no, yes := false, true
	s.user("hidden", nil)
	s.user("visible", func(c *accounts.UserChanges) { c.IsHidden = &no })
	s.user("disabled", func(c *accounts.UserChanges) { c.IsHidden, c.IsDisabled = &no, &yes })
	_, body := s.call(http.MethodGet, "/Users/Public", "", nil)
	var users []UserDto
	if err := json.Unmarshal(body, &users); err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].Name != "visible" {
		t.Errorf("public users: %s", body)
	}
}

// TestResponsesMatchJellyfin compares the JSON structure of Polyfin's
// responses with fixtures recorded from Jellyfin 12.1 by
// scripts/jellyfin-fixtures.sh: same keys at every level and same types.
func TestResponsesMatchJellyfin(t *testing.T) {
	s := newTestServer(t, 10)
	s.user("alice", nil)
	_, authentication := s.call(http.MethodPost, "/Users/AuthenticateByName", app("tv", ""),
		map[string]string{"Username": "alice", "Pw": "correct horse"})
	var result AuthenticationResult
	_ = json.Unmarshal(authentication, &result)
	signedIn := app("tv", result.AccessToken)
	_, quickConnect := s.call(http.MethodPost, "/QuickConnect/Initiate", app("phone", ""), nil)
	_, publicInfo := s.call(http.MethodGet, "/System/Info/Public", "", nil)
	_, systemInfo := s.call(http.MethodGet, "/System/Info", signedIn, nil)
	_, user := s.call(http.MethodGet, "/Users/Me", signedIn, nil)
	_, branding := s.call(http.MethodGet, "/Branding/Configuration", "", nil)

	for fixture, body := range map[string][]byte{
		"authentication-result":  authentication,
		"quick-connect-result":   quickConnect,
		"public-system-info":     publicInfo,
		"system-info":            systemInfo,
		"user":                   user,
		"branding-configuration": branding,
	} {
		raw, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.1", fixture+".json"))
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
		for _, difference := range compareShapes(fixture, want, got, shapeRules{}) {
			t.Error(difference)
		}
	}
}

// shapeRules relaxes a shape comparison where Polyfin cannot have the data
// Jellyfin had when the fixture was recorded.
type shapeRules struct {
	// dynamic names objects keyed by data, such as image types: their keys
	// are not compared.
	dynamic []string
	// absent maps a field Polyfin leaves out to the fixtures concerned, or
	// "*" for all; returned maps a field Polyfin sends that the recorded
	// items lacked.
	absent, returned map[string][]string
}

// allowed reports whether rules list key for the fixture path belongs to.
func allowed(rules map[string][]string, path, key string) bool {
	fixture, _, _ := strings.Cut(path, ".")
	fixture, _, _ = strings.Cut(fixture, "[")
	return slices.Contains(rules[key], "*") || slices.Contains(rules[key], fixture)
}

func compareShapes(path string, want, got any, rules shapeRules) []string {
	switch want := want.(type) {
	case map[string]any:
		object, ok := got.(map[string]any)
		if !ok {
			return []string{fmt.Sprintf("%s: got %T, want an object", path, got)}
		}
		if key := path[strings.LastIndexAny(path, ".]")+1:]; slices.Contains(rules.dynamic, key) {
			return nil
		}
		var differences []string
		keys := make([]string, 0, len(want)+len(object))
		for key := range want {
			keys = append(keys, key)
		}
		for key := range object {
			if _, known := want[key]; !known {
				keys = append(keys, key)
			}
		}
		slices.Sort(keys)
		for _, key := range keys {
			expected, inWant := want[key]
			actual, inGot := object[key]
			switch {
			case !inGot:
				if !allowed(rules.absent, path, key) {
					differences = append(differences, fmt.Sprintf("%s.%s: missing", path, key))
				}
			case !inWant:
				if !allowed(rules.returned, path, key) {
					differences = append(differences, fmt.Sprintf("%s.%s: not returned by Jellyfin", path, key))
				}
			default:
				differences = append(differences, compareShapes(path+"."+key, expected, actual, rules)...)
			}
		}
		return differences
	case []any:
		array, ok := got.([]any)
		if !ok {
			return []string{fmt.Sprintf("%s: got %T, want an array", path, got)}
		}
		if len(want) > 0 && len(array) > 0 {
			return compareShapes(path+"[0]", want[0], array[0], rules)
		}
		return nil
	default:
		if fmt.Sprintf("%T", want) != fmt.Sprintf("%T", got) {
			return []string{fmt.Sprintf("%s: got %T, want %T", path, got, want)}
		}
		return nil
	}
}
