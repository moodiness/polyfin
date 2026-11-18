package jellyfin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/preferences"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
	"github.com/moodiness/polyfin/internal/throttle"
)

// auxiliaryServer serves sign-in and the auxiliary routes only.
type auxiliaryServer struct {
	t     *testing.T
	store *accounts.Store
	url   string
}

func newAuxiliaryServer(t *testing.T) auxiliaryServer {
	t.Helper()
	pool := migrated.New(t)
	store, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{Options: Options{
		ServerID:    testServerID,
		Accounts:    store,
		SignIns:     throttle.New(10, time.Minute),
		Preferences: preferences.New(pool),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, configurations: cache.New[accounts.ID, UserConfiguration](10, time.Hour)}
	h.now = time.Now
	rt := &router{}
	rt.handle(http.MethodPost, "/Users/AuthenticateByName", http.HandlerFunc(h.authenticateByName))
	h.auxiliaryRoutes(rt)
	server := httptest.NewServer(rt)
	t.Cleanup(server.Close)
	return auxiliaryServer{t: t, store: store, url: server.URL}
}

func (s auxiliaryServer) user(name string, administrator bool) (accounts.User, string) {
	s.t.Helper()
	user, err := s.store.CreateUser(s.t.Context(), accounts.NewUser{Name: name, Password: "correct horse", IsAdministrator: administrator})
	if err != nil {
		s.t.Fatal(err)
	}
	status, body := s.call(http.MethodPost, "/Users/AuthenticateByName", app(name, ""),
		`{"Username":"`+name+`","Pw":"correct horse"}`)
	if status != http.StatusOK {
		s.t.Fatalf("sign-in of %s: %d %s", name, status, body)
	}
	var result AuthenticationResult
	_ = json.Unmarshal(body, &result)
	return user, app(name, result.AccessToken)
}

// call sends a request with a raw JSON body, if any.
func (s auxiliaryServer) call(method, path, authorization, body string) (int, []byte) {
	s.t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, _ := http.NewRequestWithContext(s.t.Context(), method, s.url+path, reader)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := testClient.Do(request)
	if err != nil {
		s.t.Fatal(err)
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)
	return response.StatusCode, payload
}

// sameJSON reports whether two JSON documents are equal, ignoring key order
// in objects.
func sameJSON(t *testing.T, want string, got []byte) bool {
	t.Helper()
	var expected, actual any
	if err := json.Unmarshal([]byte(want), &expected); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(got, &actual); err != nil {
		t.Fatalf("%v in %s", err, got)
	}
	return reflect.DeepEqual(expected, actual)
}

func customPrefKeys(t *testing.T, body []byte) []string {
	t.Helper()
	var dto struct{ CustomPrefs json.RawMessage }
	if err := json.Unmarshal(body, &dto); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(dto.CustomPrefs))
	_, _ = decoder.Token()
	var keys []string
	for decoder.More() {
		key, _ := decoder.Token()
		keys = append(keys, key.(string))
		var value any
		_ = decoder.Decode(&value)
	}
	return keys
}

// Responses recorded from Jellyfin 12.2.
const (
	unsavedUserSettings = `{"Id":"3ce5b65d-e116-d731-65d1-efc4a30ec35c","SortBy":"SortName","RememberIndexing":false,` +
		`"PrimaryImageHeight":250,"PrimaryImageWidth":250,"CustomPrefs":{"chromecastVersion":"stable",` +
		`"skipForwardLength":"30000","skipBackLength":"10000","enableNextVideoInfoOverlay":"False","tvhome":null,` +
		`"dashboardTheme":null},"ScrollDirection":"Horizontal","ShowBackdrop":true,"RememberSorting":false,` +
		`"SortOrder":"Ascending","ShowSidebar":false,"Client":"emby"}`
	savedRequest = `{"ViewType":"Poster","SortBy":"DateCreated","IndexBy":"Genre","RememberIndexing":true,` +
		`"PrimaryImageHeight":111,"PrimaryImageWidth":222,"CustomPrefs":{"zeta":"1","alpha":"2",` +
		`"homesection1":"LatestMedia","homesection0":"resume"},"ScrollDirection":"Vertical","ShowBackdrop":false,` +
		`"RememberSorting":true,"SortOrder":"Descending","ShowSidebar":true}`
	savedResponse = `{"Id":"f3e2b573-da15-8c96-aabe-05f6a90b2e6b","SortBy":"DateCreated","RememberIndexing":true,` +
		`"PrimaryImageHeight":250,"PrimaryImageWidth":250,"CustomPrefs":{"homesection1":"latestmedia",` +
		`"homesection0":"resume","chromecastVersion":"stable","skipForwardLength":"15000","skipBackLength":"15000",` +
		`"enableNextVideoInfoOverlay":"True","tvhome":"","dashboardTheme":"","alpha":"2","zeta":"1"},` +
		`"ScrollDirection":"Vertical","ShowBackdrop":false,"RememberSorting":true,"SortOrder":"Descending",` +
		`"ShowSidebar":true,"Client":"probeaux"}`
)

func TestDisplayPreferencesDefaults(t *testing.T) {
	t.Parallel()
	s := newAuxiliaryServer(t)
	_, alice := s.user("alice", false)

	status, body := s.call(http.MethodGet, "/DisplayPreferences/usersettings?userId=&client=emby", alice, "")
	if status != http.StatusOK || !sameJSON(t, unsavedUserSettings, body) {
		t.Errorf("unsaved usersettings: %d %s", status, body)
	}
	// An item id is answered hyphenated, whatever form it came in.
	status, body = s.call(http.MethodGet, "/DisplayPreferences/0C19A6F54D50D8BBBBA00F8F4325DE45?CLIENT=emby", alice, "")
	want := strings.Replace(unsavedUserSettings, "3ce5b65d-e116-d731-65d1-efc4a30ec35c", "0c19a6f5-4d50-d8bb-bba0-0f8f4325de45", 1)
	if status != http.StatusOK || !sameJSON(t, want, body) {
		t.Errorf("unsaved item preferences: %d %s", status, body)
	}

	status, body = s.call(http.MethodGet, "/DisplayPreferences/usersettings", alice, "")
	var problem problemDetails
	_ = json.Unmarshal(body, &problem)
	if status != http.StatusBadRequest || len(problem.Errors["client"]) != 1 {
		t.Errorf("without client: %d %s", status, body)
	}
	if status, _ := s.call(http.MethodGet, "/DisplayPreferences/usersettings?client=emby", "", ""); status != http.StatusUnauthorized {
		t.Errorf("signed out: %d", status)
	}
}

func TestDisplayPreferencesRoundTrip(t *testing.T) {
	t.Parallel()
	s := newAuxiliaryServer(t)
	_, alice := s.user("alice", false)
	_, bob := s.user("bob", false)

	if status, body := s.call(http.MethodPost, "/DisplayPreferences/m1?client=probeaux", alice, savedRequest); status != http.StatusNoContent {
		t.Fatalf("save: %d %s", status, body)
	}
	status, body := s.call(http.MethodGet, "/DisplayPreferences/m1?client=probeaux", alice, "")
	if status != http.StatusOK || !sameJSON(t, savedResponse, body) {
		t.Errorf("saved preferences: %d %s", status, body)
	}
	wantKeys := []string{"homesection1", "homesection0", "chromecastVersion", "skipForwardLength", "skipBackLength",
		"enableNextVideoInfoOverlay", "tvhome", "dashboardTheme", "alpha", "zeta"}
	if keys := customPrefKeys(t, body); !reflect.DeepEqual(keys, wantKeys) {
		t.Errorf("CustomPrefs order: got %v, want %v", keys, wantKeys)
	}

	// Preferences belong to one user and one client.
	unsaved := strings.NewReplacer("3ce5b65d-e116-d731-65d1-efc4a30ec35c", "f3e2b573-da15-8c96-aabe-05f6a90b2e6b")
	for _, check := range []struct{ who, authorization, client string }{
		{"bob", bob, "probeaux"},
		{"alice on another client", alice, "other"},
	} {
		status, body := s.call(http.MethodGet, "/DisplayPreferences/m1?client="+check.client, check.authorization, "")
		want := strings.Replace(unsaved.Replace(unsavedUserSettings), `"Client":"emby"`, `"Client":"`+check.client+`"`, 1)
		if status != http.StatusOK || !sameJSON(t, want, body) {
			t.Errorf("%s: %d %s", check.who, status, body)
		}
	}

	// Saving again replaces everything but home sections, which merge.
	s.call(http.MethodPost, "/DisplayPreferences/m1?client=probeaux", alice,
		`{"CustomPrefs":{"homesection0":"livetv","skipForwardLength":"5000","tvhome":null,"beta":"3"}}`)
	_, body = s.call(http.MethodGet, "/DisplayPreferences/m1?client=probeaux", alice, "")
	var dto struct {
		SortBy      string
		CustomPrefs map[string]any
	}
	_ = json.Unmarshal(body, &dto)
	want := map[string]any{"homesection0": "livetv", "homesection1": "latestmedia", "chromecastVersion": "stable",
		"skipForwardLength": "5000", "skipBackLength": "15000", "enableNextVideoInfoOverlay": "True",
		"tvhome": nil, "dashboardTheme": "", "beta": "3"}
	if dto.SortBy != "SortName" || !reflect.DeepEqual(dto.CustomPrefs, want) {
		t.Errorf("second save: %s", body)
	}
}

func TestDisplayPreferencesRejectsUnreadableSettings(t *testing.T) {
	t.Parallel()
	s := newAuxiliaryServer(t)
	_, alice := s.user("alice", false)
	for body, wantStatus := range map[string]int{
		`{"CustomPrefs":{"chromecastVersion":"nightly"}}`:       http.StatusBadRequest,
		`{"CustomPrefs":{"skipForwardLength":"soon"}}`:          http.StatusInternalServerError,
		`{"CustomPrefs":{"enableNextVideoInfoOverlay":"yes"}}`:  http.StatusInternalServerError,
		`{"CustomPrefs":{"homesectionX":"resume"}}`:             http.StatusInternalServerError,
		`{"CustomPrefs":null}`:                                  http.StatusBadRequest,
		`{"ShowBackdrop":"true"}`:                               http.StatusBadRequest,
		`{"SortOrder":"Sideways"}`:                              http.StatusBadRequest,
		`[]`:                                                    http.StatusBadRequest,
		`null`:                                                  http.StatusBadRequest,
		`{"SortBy":"Name"} trailing`:                            http.StatusBadRequest,
		`{"SortBy":"Name","CustomPrefs":{"a":{"nested":true}}}`: http.StatusBadRequest,
	} {
		if status, response := s.call(http.MethodPost, "/DisplayPreferences/p?client=web", alice, body); status != wantStatus {
			t.Errorf("%s: got %d %s, want %d", body, status, response, wantStatus)
		}
	}
	// Nothing was saved.
	_, body := s.call(http.MethodGet, "/DisplayPreferences/p?client=web", alice, "")
	if !strings.Contains(string(body), `"skipForwardLength":"30000"`) {
		t.Errorf("a refused update was saved: %s", body)
	}
}

func TestDisplayPreferencesOfAnotherUser(t *testing.T) {
	t.Parallel()
	s := newAuxiliaryServer(t)
	alice, aliceApp := s.user("alice", false)
	_, bobApp := s.user("bob", false)
	_, adminApp := s.user("admin", true)
	s.call(http.MethodPost, "/DisplayPreferences/home?client=web", aliceApp, `{"SortBy":"Name","CustomPrefs":{}}`)

	path := "/DisplayPreferences/home?client=web&userId=" + alice.ID.String()
	if status, _ := s.call(http.MethodGet, path, bobApp, ""); status != http.StatusForbidden {
		t.Errorf("another user's preferences: got %d, want 403", status)
	}
	if status, _ := s.call(http.MethodPost, path, bobApp, `{"SortBy":"Random"}`); status != http.StatusForbidden {
		t.Errorf("saving another user's preferences: got %d, want 403", status)
	}
	status, body := s.call(http.MethodGet, path, adminApp, "")
	if status != http.StatusOK || !strings.Contains(string(body), `"SortBy":"Name"`) {
		t.Errorf("administrator reading a user's preferences: %d %s", status, body)
	}
	// Bob's own preferences, named by his id or not, stay his.
	if _, body := s.call(http.MethodGet, "/DisplayPreferences/home?client=web", bobApp, ""); !strings.Contains(string(body), `"SortBy":"SortName"`) {
		t.Errorf("bob sees alice's preferences: %s", body)
	}
	unknown := "/DisplayPreferences/home?client=web&userId=00000000000000000000000000000001"
	if status, _ := s.call(http.MethodGet, unknown, adminApp, ""); status != http.StatusInternalServerError {
		t.Errorf("unknown user: got %d, want 500", status)
	}
	if status, _ := s.call(http.MethodGet, "/DisplayPreferences/home?client=web&userId=nobody", adminApp, ""); status != http.StatusBadRequest {
		t.Errorf("malformed user id: got %d, want 400", status)
	}
}

// What an app keeps as display preferences is bounded: a body, a value or
// a client's name over its bound is refused and nothing is kept of it,
// under whatever identifier and client the request names. Home sections
// merge into the ones kept, and cannot grow a value past the bound either.
// What apps do keep is kept and read back as before.
func TestDisplayPreferencesOverABoundAreRefused(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	alice := s.user("alice", nil)
	header := app("laptop", s.signIn("alice", "laptop"))
	rows := func() int {
		t.Helper()
		var count int
		if err := s.pool.QueryRow(t.Context(), "SELECT count(*) FROM display_preferences WHERE user_id = $1", alice.ID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}

	// A megabyte under each new identifier and client, then one byte more
	// than the bound.
	for i, size := range []int{maxBody - 64, maxBody - 64, preferences.MaxValueBytes - 28} {
		body := `{"CustomPrefs":{"filler":"` + strings.Repeat("A", size) + `"}}`
		path := fmt.Sprintf("/DisplayPreferences/view-%d?client=app-%d", i, i)
		if status, answer := s.postRaw(path, header, body); status != http.StatusRequestEntityTooLarge {
			t.Errorf("a body of %d bytes: %d %.80s, want 413", len(body), status, answer)
		}
	}
	long := strings.Repeat("c", preferences.MaxClientBytes+1)
	for _, method := range []string{http.MethodPost, http.MethodGet} {
		var body any
		if method == http.MethodPost {
			body = json.RawMessage(savedRequest)
		}
		status, answer := s.call(method, "/DisplayPreferences/home?client="+long, header, body)
		var problem problemDetails
		_ = json.Unmarshal(answer, &problem)
		if status != http.StatusBadRequest || len(problem.Errors["client"]) != 1 {
			t.Errorf("%s with a client of %d bytes: %d %s, want 400 for client", method, len(long), status, answer)
		}
	}
	if n := rows(); n != 0 {
		t.Fatalf("the refused requests left %d rows", n)
	}

	sections := func(from, to int) string {
		var body strings.Builder
		body.WriteString(`{"CustomPrefs":{"homesection` + fmt.Sprint(from) + `":"resume"`)
		for index := from + 1; index < to; index++ {
			fmt.Fprintf(&body, `,"homesection%d":"resume"`, index)
		}
		body.WriteString(`}}`)
		return body.String()
	}
	if status, answer := s.postRaw("/DisplayPreferences/home?client=emby", header, sections(0, 2000)); status != http.StatusNoContent {
		t.Fatalf("2000 home sections: %d %s", status, answer)
	}
	if status, answer := s.postRaw("/DisplayPreferences/home?client=emby", header, sections(2000, 4000)); status != http.StatusBadRequest {
		t.Errorf("2000 home sections more: %d %.80s, want 400", status, answer)
	}
	_, answer := s.call(http.MethodGet, "/DisplayPreferences/home?client=emby", header, nil)
	if !strings.Contains(string(answer), `"homesection1999":"resume"`) || strings.Contains(string(answer), `"homesection2000"`) {
		t.Errorf("after a refused value, the home sections are not the ones kept before: %.200s", answer)
	}

	// An app keeps its settings under one identifier, each view's filters
	// among them, as jellyfin-web does. A client's name of the bound's
	// length is kept.
	client := strings.Repeat("c", preferences.MaxClientBytes)
	filters := map[string]string{}
	for view := range 60 {
		filters[fmt.Sprintf("%032x-movies", view)] = `{"SortBy":"SortName,ProductionYear","SortOrder":"Ascending","IsUnplayed":true,"Genres":"Drama|Comedy"}`
	}
	settings, _ := json.Marshal(map[string]any{"SortBy": "SortName", "CustomPrefs": filters, "ShowBackdrop": true, "Client": client})
	if status, answer := s.postRaw("/DisplayPreferences/usersettings?client="+client, header, string(settings)); status != http.StatusNoContent {
		t.Fatalf("an app's settings of %d bytes: %d %s", len(settings), status, answer)
	}
	status, answer := s.call(http.MethodGet, "/DisplayPreferences/usersettings?client="+client, header, nil)
	var read struct {
		Client      string
		CustomPrefs map[string]*string
	}
	if err := json.Unmarshal(answer, &read); err != nil || status != http.StatusOK || read.Client != client {
		t.Fatalf("an app's settings read back: %d %v %.200s", status, err, answer)
	}
	for key, value := range filters {
		if got := read.CustomPrefs[key]; got == nil || *got != value {
			t.Errorf("%s was not kept as saved", key)
		}
	}
	if n := rows(); n != 2 {
		t.Errorf("alice keeps %d rows, want 2", n)
	}
}

func TestEndpointClassifiesTheClientAddress(t *testing.T) {
	t.Parallel()
	h := &Handler{}
	for address, want := range map[string]string{
		"127.0.0.1:5000":        `{"IsLocal":true,"IsInNetwork":true}`,
		"[::1]:5000":            `{"IsLocal":true,"IsInNetwork":true}`,
		"[::ffff:127.0.0.1]:80": `{"IsLocal":true,"IsInNetwork":true}`,
		"192.168.1.20:5000":     `{"IsLocal":false,"IsInNetwork":true}`,
		"10.1.2.3:5000":         `{"IsLocal":false,"IsInNetwork":true}`,
		"172.31.255.1:5000":     `{"IsLocal":false,"IsInNetwork":true}`,
		"169.254.10.1:5000":     `{"IsLocal":false,"IsInNetwork":true}`,
		"[fe80::1%en0]:5000":    `{"IsLocal":false,"IsInNetwork":true}`,
		"[fd12:3456::1]:5000":   `{"IsLocal":false,"IsInNetwork":true}`,
		"172.32.0.1:5000":       `{"IsLocal":false,"IsInNetwork":false}`,
		"203.0.113.9:5000":      `{"IsLocal":false,"IsInNetwork":false}`,
		"[2001:db8::1]:5000":    `{"IsLocal":false,"IsInNetwork":false}`,
		"[::ffff:8.8.8.8]:5000": `{"IsLocal":false,"IsInNetwork":false}`,
	} {
		request := httptest.NewRequest(http.MethodGet, "/System/Endpoint", nil)
		request.RemoteAddr = address
		recorder := httptest.NewRecorder()
		h.endpointInfo(recorder, request)
		if got := strings.TrimSpace(recorder.Body.String()); got != want {
			t.Errorf("%s: got %s, want %s", address, got, want)
		}
	}
}

func TestBitrateTest(t *testing.T) {
	t.Parallel()
	s := newAuxiliaryServer(t)
	_, alice := s.user("alice", false)
	// Jellyfin sends the size rounded up to a power of two, at least 16.
	for query, want := range map[string]int{
		"":                128 << 10,
		"?size=1":         16,
		"?size=16":        16,
		"?SIZE=17":        32,
		"?size=0x10":      16,
		"?size=%2010%20":  16,
		"?size=1048577":   2 << 20,
		"?size=10&size=9": 16,
	} {
		status, body := s.call(http.MethodGet, "/Playback/BitrateTest"+query, alice, "")
		if status != http.StatusOK || len(body) != want {
			t.Errorf("%q: got %d with %d bytes, want %d bytes", query, status, len(body), want)
		}
	}
	for _, size := range []string{"0", "-1", "100000001", "abc", "", "10.0"} {
		status, body := s.call(http.MethodGet, "/Playback/BitrateTest?size="+size, alice, "")
		var problem problemDetails
		_ = json.Unmarshal(body, &problem)
		if status != http.StatusBadRequest || len(problem.Errors["size"]) != 1 {
			t.Errorf("size %q: got %d %s, want a validation problem", size, status, body)
		}
	}
	if status, _ := s.call(http.MethodGet, "/Playback/BitrateTest?size=10", "", ""); status != http.StatusUnauthorized {
		t.Errorf("signed out: got %d, want 401", status)
	}
}
