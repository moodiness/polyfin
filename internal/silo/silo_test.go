package silo

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// These tests answer the Jellyfin API's calls with a fake, which records
// them; the tests of the package against the real Jellyfin API are in
// package jellyfin (silo_test.go).

// fakeAccounts are the accounts of one user signed in on one device.
type fakeAccounts struct {
	mu       sync.Mutex
	user     accounts.User
	device   accounts.Device
	tokens   map[string]bool
	notFound bool
}

func (f *fakeAccounts) Settings() accounts.Settings {
	return accounts.Settings{SiloApps: true, Language: "en"}
}

func (f *fakeAccounts) DeviceByToken(_ context.Context, token, _ string) (accounts.Device, accounts.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.notFound || !f.tokens[token] {
		return accounts.Device{}, accounts.User{}, accounts.ErrNotFound
	}
	return f.device, f.user, nil
}

func (f *fakeAccounts) SetupRequired(context.Context) (bool, error) { return false, nil }

func (f *fakeAccounts) SignOutDevice(_ context.Context, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.tokens, token)
	return nil
}

// call is a call the fake Jellyfin API received.
type call struct {
	method, path string
	query        url.Values
	body         map[string]any
}

// fakeJellyfin answers the Jellyfin API's calls with answer, and records
// them.
type fakeJellyfin struct {
	mu     sync.Mutex
	calls  []call
	answer func(w http.ResponseWriter, c call)
}

func (f *fakeJellyfin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c := call{method: r.Method, path: r.URL.Path, query: r.URL.Query()}
	data, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(data, &c.body)
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()
	f.answer(w, c)
}

func (f *fakeJellyfin) called(method, path string) []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	var found []call
	for _, c := range f.calls {
		if c.method == method && c.path == path {
			found = append(found, c)
		}
	}
	return found
}

const (
	testToken   = "0123456789abcdef0123456789abcdef"
	testVersion = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

type harness struct {
	t        *testing.T
	server   *Server
	accounts *fakeAccounts
	jellyfin *fakeJellyfin
	clock    *time.Time
	profile  string
}

func newHarness(t *testing.T, answer func(w http.ResponseWriter, c call)) *harness {
	t.Helper()
	user := accounts.User{ID: accounts.ID{1}, Name: "member", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	h := &harness{t: t, accounts: &fakeAccounts{user: user, tokens: map[string]bool{testToken: true},
		device: accounts.Device{ID: accounts.ID{2}, UserID: user.ID, DeviceInfo: accounts.DeviceInfo{DeviceID: "pixel", DeviceName: "Pixel",
			Client: "Silo", ClientVersion: "1"}}},
		jellyfin: &fakeJellyfin{answer: answer}, profile: user.ID.String()}
	// A Wednesday at noon.
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.Local)
	h.clock = &now
	h.server = New(Options{ServerID: "server", Version: "test", Accounts: h.accounts, Jellyfin: h.jellyfin, DB: migrated.New(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), now: func() time.Time { return *h.clock }})
	return h
}

// do sends a request to the API as an app does.
func (h *harness) do(method, path string, body any, header ...string) *httptest.ResponseRecorder {
	h.t.Helper()
	var reader io.Reader = http.NoBody
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	r := httptest.NewRequestWithContext(h.t.Context(), method, path, reader)
	r.Header.Set("Authorization", "Bearer "+testToken)
	r.Header.Set("X-Profile-Id", h.profile)
	r.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(header); i += 2 {
		r.Header.Set(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	h.server.ServeHTTP(w, r)
	return w
}

// jsonAnswer writes v as JSON.
func jsonAnswer(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// playable answers PlaybackInfo with a version played as it is, and the
// reports and media of its playback.
func playable(w http.ResponseWriter, c call) {
	switch {
	case strings.HasSuffix(c.path, "/PlaybackInfo"):
		jsonAnswer(w, map[string]any{"PlaySessionId": "play-" + strconv.Itoa(int(time.Now().UnixNano())), "MediaSources": []any{map[string]any{
			"Id": testVersion, "Container": "mp4", "SupportsDirectPlay": true, "RunTimeTicks": int64(7200) * 10_000_000,
			"MediaStreams": []any{map[string]any{"Type": "Video", "Index": 0, "Codec": "h264"},
				map[string]any{"Type": "Audio", "Index": 1, "Codec": "aac"}, map[string]any{"Type": "Audio", "Index": 2, "Codec": "ac3"},
				map[string]any{"Type": "Audio", "Index": 3, "Codec": "dts"}}}}})
	case strings.HasPrefix(c.path, "/Videos/") && strings.HasSuffix(c.path, "/stream"):
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = io.WriteString(w, "media bytes")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// start starts a playback of the test version, returning the session.
func (h *harness) start(more map[string]any) (string, map[string]any) {
	h.t.Helper()
	number, err := h.server.numbers.number(h.t.Context(), accounts.ID{0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa,
		0xaa, 0xaa, 0xaa, 0xaa})
	if err != nil {
		h.t.Fatal(err)
	}
	body := map[string]any{"protocol_version": 3, "file_id": number, "profile_id": h.profile, "installation_id": h.accounts.device.ID.String(),
		"playback_attempt_id": "attempt-1", "quality_preference": "auto",
		"client_capabilities": map[string]any{"containers": []string{"mp4"}, "codecs_video": []string{"h264"}, "codecs_audio": []string{"aac"}}}
	for key, value := range more {
		body[key] = value
	}
	w := h.do(http.MethodPost, "/api/v2/playback/start", body)
	var d map[string]any
	if w.Code != http.StatusCreated || json.Unmarshal(w.Body.Bytes(), &d) != nil {
		h.t.Fatalf("start: %d %s", w.Code, w.Body)
	}
	session, _ := d["session_id"].(string)
	return session, d
}

func (h *harness) replan(session, id string, tracks map[string]any) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPost, "/api/v2/playback/"+session+"/replan", map[string]any{"installation_id": h.accounts.device.ID.String(),
		"playback_attempt_id": "attempt-1", "replan_request_id": id, "operation": "track_change", "quality_preference": "auto",
		"position_seconds": 10, "selected_tracks": tracks,
		"client_capabilities": map[string]any{"containers": []string{"mp4"}, "codecs_video": []string{"h264"}, "codecs_audio": []string{"aac"}}})
}

func planID(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var d struct {
		Plan struct {
			PlanID string `json:"plan_id"`
		} `json:"playback_plan"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &d) != nil {
		t.Fatalf("replan: %d %s", w.Code, w.Body)
	}
	return d.Plan.PlanID
}

// Replans name the session and its media: no cache keeps them, the replay
// of a replan included.
func TestReplansAreNotStored(t *testing.T) {
	t.Parallel()
	h := newHarness(t, playable)
	session, _ := h.start(nil)
	for _, label := range []string{"first answer", "replay"} {
		if w := h.replan(session, "replan-1", map[string]any{}); w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s: %d, Cache-Control %q", label, w.Code, w.Header().Get("Cache-Control"))
		}
	}
}

// The media of a session are the user's: no shared cache keeps them,
// whatever the Jellyfin API says of them.
func TestSessionMediaAreNotStored(t *testing.T) {
	t.Parallel()
	h := newHarness(t, playable)
	session, _ := h.start(nil)
	w := h.do(http.MethodGet, "/api/v2/stream/"+session, nil)
	if w.Code != http.StatusOK || w.Body.String() != "media bytes" || w.Header().Get("Cache-Control") != "private, no-store" {
		t.Errorf("the file: %d %q, Cache-Control %q", w.Code, w.Body, w.Header().Get("Cache-Control"))
	}
	converting := newHarness(t, func(w http.ResponseWriter, c call) {
		switch {
		case strings.HasSuffix(c.path, "/PlaybackInfo"):
			jsonAnswer(w, map[string]any{"PlaySessionId": "play-1", "MediaSources": []any{map[string]any{"Id": testVersion, "Container": "mkv",
				"TranscodingUrl": "/videos/" + testVersion + "/master.m3u8?PlaySessionId=play-1&ApiKey=" + testToken, "TranscodingContainer": "mp4",
				"MediaStreams": []any{map[string]any{"Type": "Video", "Index": 0, "Codec": "hevc"}}}}})
		case strings.HasSuffix(c.path, ".m3u8") || strings.HasSuffix(c.path, ".mp4"):
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = io.WriteString(w, "#EXTM3U\n")
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
	session, _ = converting.start(nil)
	for _, path := range []string{"master.m3u8", "main.m3u8?PlaySessionId=play-1", "hls1/main/0.mp4?PlaySessionId=play-1"} {
		w := converting.do(http.MethodGet, "/api/v2/playback/transcode/"+session+"/"+path, nil)
		if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "private, no-store" {
			t.Errorf("%s: %d, Cache-Control %q", path, w.Code, w.Header().Get("Cache-Control"))
		}
	}
	for _, c := range converting.jellyfin.calls {
		if c.query.Get("ApiKey") != "" {
			t.Errorf("the token went in a relayed address: %s?%s", c.path, c.query.Encode())
		}
	}
}

// A session keeps the answers of its last replans only.
func TestReplanAnswersAreBounded(t *testing.T) {
	t.Parallel()
	h := newHarness(t, playable)
	session, _ := h.start(nil)
	first := planID(t, h.replan(session, "replan-0", map[string]any{}))
	var last string
	for i := 1; i < 100; i++ {
		last = planID(t, h.replan(session, "replan-"+strconv.Itoa(i), map[string]any{}))
	}
	if again := planID(t, h.replan(session, "replan-99", map[string]any{})); again != last {
		t.Errorf("the last replan is not replayed: %s then %s", last, again)
	}
	if again := planID(t, h.replan(session, "replan-0", map[string]any{})); again == first {
		t.Errorf("the first of 100 replans is still kept")
	}
}

// Sessions nobody used for a day are forgotten as new ones start.
func TestUnusedSessionsAreForgottenAsOthersStart(t *testing.T) {
	t.Parallel()
	h := newHarness(t, playable)
	h.start(nil)
	h.start(map[string]any{"playback_attempt_id": "attempt-2"})
	*h.clock = h.clock.Add(25 * time.Hour)
	h.start(map[string]any{"playback_attempt_id": "attempt-3"})
	h.server.sessions.mu.Lock()
	kept := len(h.server.sessions.byID)
	h.server.sessions.mu.Unlock()
	if kept != 1 {
		t.Errorf("%d sessions kept", kept)
	}
}

// A stop without a sequence never moves the position back behind a
// sample already applied.
func TestAStopWithoutSequenceKeepsTheLastSample(t *testing.T) {
	t.Parallel()
	h := newHarness(t, playable)
	session, _ := h.start(nil)
	installation := h.accounts.device.ID.String()
	if w := h.do(http.MethodPost, "/api/v2/playback/"+session+"/progress", map[string]any{"installation_id": installation, "sequence": 5,
		"position": 300.0, "is_paused": false}); w.Code != http.StatusOK {
		t.Fatalf("progress: %d %s", w.Code, w.Body)
	}
	if w := h.do(http.MethodDelete, "/api/v2/playback/"+session, map[string]any{"installation_id": installation, "stop_id": "stop-1",
		"position": 10.0}); w.Code != http.StatusOK {
		t.Fatalf("stop: %d %s", w.Code, w.Body)
	}
	stopped := h.jellyfin.called(http.MethodPost, "/Sessions/Playing/Stopped")
	if len(stopped) != 1 || stopped[0].body["PositionTicks"] != float64(300*10_000_000) {
		t.Errorf("stopped at %v", stopped)
	}
}

// A replan that fails leaves the session as it was: the next one plans
// from the tracks last planned.
func TestAFailedReplanChangesNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(w http.ResponseWriter, c call) {
		if strings.HasSuffix(c.path, "/PlaybackInfo") && c.body["AudioStreamIndex"] == float64(3) {
			_, _ = io.WriteString(w, "not JSON")
			return
		}
		playable(w, c)
	})
	session, _ := h.start(nil)
	if w := h.replan(session, "replan-1", map[string]any{"audio": map[string]any{"id": "2"}}); w.Code != http.StatusOK {
		t.Fatalf("replan to track 2: %d %s", w.Code, w.Body)
	}
	if w := h.replan(session, "replan-2", map[string]any{"audio": map[string]any{"id": "3"}}); w.Code != http.StatusInternalServerError {
		t.Fatalf("failing replan: %d %s", w.Code, w.Body)
	}
	if w := h.replan(session, "replan-3", map[string]any{}); w.Code != http.StatusOK {
		t.Fatalf("replan: %d %s", w.Code, w.Body)
	}
	asked := h.jellyfin.called(http.MethodPost, "/Items/"+testVersion+"/PlaybackInfo")
	if last := asked[len(asked)-1]; last.body["AudioStreamIndex"] != float64(2) {
		t.Errorf("the replan after a failure asked audio %v", last.body["AudioStreamIndex"])
	}
}

// A start that plays nothing stops what PlaybackInfo prepared for it.
func TestAStartThatPlaysNothingStopsItsPlaySession(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(w http.ResponseWriter, c call) {
		if strings.HasSuffix(c.path, "/PlaybackInfo") {
			jsonAnswer(w, map[string]any{"PlaySessionId": "play-nothing", "MediaSources": []any{}, "ErrorCode": "NoCompatibleStream"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	_, d := h.start(nil)
	if d["outcome"] != "adaptation_unavailable" {
		t.Fatalf("decision: %v", d)
	}
	stopped := h.jellyfin.called(http.MethodDelete, "/Videos/ActiveEncodings")
	if len(stopped) != 1 || stopped[0].query.Get("playSessionId") != "play-nothing" {
		t.Errorf("encodings stopped: %v", stopped)
	}
}

// The token leaves a Jellyfin address even when its query is malformed,
// and the play session that authorizes the address stays.
func TestTokensLeaveMalformedAddresses(t *testing.T) {
	t.Parallel()
	for _, target := range []string{"/Videos/x/Subtitles/0/0/Stream.vtt?bad=%zz&ApiKey=secret&PlaySessionId=play-1",
		"/Videos/x/stream?API_KEY=secret;PlaySessionId=play-1;x=%"} {
		if got := withoutKey(target); strings.Contains(got, "secret") || !strings.Contains(got, "PlaySessionId=play-1") {
			t.Errorf("%s: %s", target, got)
		}
	}
}

// Numbers asked twice in one call are given once.
func TestNumbersOfRepeatedIDs(t *testing.T) {
	t.Parallel()
	n := newNumbers(migrated.New(t))
	a, b := accounts.ID{1}, accounts.ID{2}
	found, err := n.of(t.Context(), a, b, a, b, a)
	if err != nil || len(found) != 2 || found[a] == found[b] || found[a] <= 0 {
		t.Errorf("numbers: %v %v", found, err)
	}
}

// Episodes are numbered with one lookup, however many they are.
func TestEpisodesAreNumberedAtOnce(t *testing.T) {
	t.Parallel()
	const series = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	h := newHarness(t, func(w http.ResponseWriter, c call) {
		var items []any
		for i := range 40 {
			id := strings.Repeat(strconv.FormatInt(int64(i%16), 16), 30) + strconv.Itoa(10+i)
			items = append(items, map[string]any{"Id": id, "Name": "Episode", "Type": "Episode", "IndexNumber": i + 1, "ParentIndexNumber": 1,
				"MediaSources": []any{map[string]any{"Id": id, "Container": "mkv"}}})
		}
		jsonAnswer(w, map[string]any{"Items": items, "TotalRecordCount": len(items)})
	})
	w := h.do(http.MethodGet, "/api/v2/catalog/series/"+series+"/seasons/1/episodes", nil)
	if w.Code != http.StatusOK || strings.Count(w.Body.String(), `"file_id"`) != 40 {
		t.Fatalf("episodes: %d %s", w.Code, w.Body)
	}
	if trips := h.server.numbers.trips.Load(); trips > 2 {
		t.Errorf("%d round trips to number 40 episodes", trips)
	}
}

// Sign-in answers a session ended meanwhile as one that never was.
func TestSignInOfAnAccountEndedMeanwhile(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(w http.ResponseWriter, c call) { jsonAnswer(w, map[string]any{"AccessToken": "fresh"}) })
	h.accounts.notFound = true
	w := h.do(http.MethodPost, "/api/v2/auth/login", map[string]any{"username": "member", "password": "secret"})
	if w.Code != http.StatusUnauthorized || problemCode(w) != "invalid_token" {
		t.Errorf("login: %d %s", w.Code, w.Body)
	}
}

func problemCode(w *httptest.ResponseRecorder) string {
	var p struct{ Type string }
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	return p.Type[strings.LastIndex(p.Type, "/")+1:]
}

// Outside their hours, a user's apps are refused, as Jellyfin's are, but
// may sign out.
func TestOutsideTheirHoursAppsAreRefusedButSignOut(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(w http.ResponseWriter, c call) { w.WriteHeader(http.StatusForbidden) })
	h.accounts.user.AccessSchedules = []accounts.AccessSchedule{{Day: "Sunday", StartHour: 9, EndHour: 17}}
	for _, path := range []string{"/api/v2/account/me", "/api/v2/profiles"} {
		if w := h.do(http.MethodGet, path, nil); w.Code != http.StatusForbidden {
			t.Errorf("%s outside the hours: %d", path, w.Code)
		}
	}
	if w := h.do(http.MethodPost, "/api/v2/auth/logout", nil); w.Code != http.StatusNoContent {
		t.Errorf("logout outside the hours: %d %s", w.Code, w.Body)
	}
	if w := h.do(http.MethodGet, "/api/v2/account/me", nil); w.Code != http.StatusUnauthorized {
		t.Errorf("after logout: %d", w.Code)
	}
}

// A profile named in another form of the user's ID is the user's.
func TestProfileIDsInAnyForm(t *testing.T) {
	t.Parallel()
	h := newHarness(t, playable)
	hyphenated := strings.ToUpper(h.profile[:8] + "-" + h.profile[8:12] + "-" + h.profile[12:16] + "-" + h.profile[16:20] + "-" + h.profile[20:])
	if w := h.do(http.MethodGet, "/api/v2/playback/capabilities", nil, "X-Profile-Id", hyphenated); w.Code != http.StatusOK {
		t.Errorf("%s: %d %s", hyphenated, w.Code, w.Body)
	}
}

// A source that fails reaches the app as unavailable for now, not as the
// server's failure.
func TestFailingSourcesAreUnavailable(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(w http.ResponseWriter, c call) { w.WriteHeader(http.StatusBadGateway) })
	w := h.do(http.MethodGet, "/api/v2/home/sections", nil)
	if w.Code != http.StatusServiceUnavailable || problemCode(w) != "unavailable" || w.Header().Get("Retry-After") == "" {
		t.Errorf("a failing source: %d %s, Retry-After %q", w.Code, w.Body, w.Header().Get("Retry-After"))
	}
}

// system/info names every link the contract requires; a problem's
// instance is the request's ID, sent in X-Request-ID.
func TestSystemInfoLinksAndRequestIDs(t *testing.T) {
	t.Parallel()
	h := newHarness(t, playable)
	var info struct {
		Links map[string]string `json:"links"`
	}
	if w := h.do(http.MethodGet, "/api/v2/system/info", nil); json.Unmarshal(w.Body.Bytes(), &info) != nil {
		t.Fatalf("info: %s", w.Body)
	}
	for _, link := range []string{"openapi", "capabilities", "identity"} {
		if info.Links[link] == "" {
			t.Errorf("links.%s is empty", link)
		}
	}
	w := h.do(http.MethodGet, "/api/v2/requests", nil)
	var p struct{ Instance string }
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	if id := w.Header().Get("X-Request-ID"); id == "" || p.Instance != "urn:silo:request:"+id {
		t.Errorf("instance %q, X-Request-ID %q", p.Instance, id)
	}
}
