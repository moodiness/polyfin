package admin

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/config"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/jellyfin"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/logs"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/source"
	"github.com/moodiness/polyfin/internal/tasks"
)

func randomID() accounts.ID {
	var id accounts.ID
	_, _ = rand.Read(id[:])
	return id
}

// fakeSessions plays the Jellyfin handler's sessions.
type fakeSessions struct {
	mu       sync.Mutex
	sessions []jellyfin.LiveSession
	// err is what commands answer.
	err      error
	stopped  []accounts.ID
	messages []string
}

func (f *fakeSessions) LiveSessions(context.Context) ([]jellyfin.LiveSession, error) {
	return f.sessions, nil
}

func (f *fakeSessions) StopPlayback(_ context.Context, by accounts.User, session accounts.ID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err == nil {
		f.stopped = append(f.stopped, session)
	}
	return f.err
}

func (f *fakeSessions) SendMessage(_ context.Context, by accounts.User, session accounts.ID, header, text string, timeout time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err == nil {
		f.messages = append(f.messages, by.Name+"|"+header+"|"+text+"|"+timeout.String())
	}
	return f.err
}

func TestDashboardRoutesAreForAdministratorsOnly(t *testing.T) {
	api := newTestAPI(t, 10)
	member := api.signedIn("alice", false)
	anonymous := api.browser()
	id := randomID().String()
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/sessions"},
		{http.MethodPost, "/sessions/" + id + "/stop"},
		{http.MethodPost, "/sessions/" + id + "/message"},
		{http.MethodGet, "/tasks"},
		{http.MethodPost, "/tasks/" + tasks.ID("Any") + "/run"},
		{http.MethodPost, "/tasks/" + tasks.ID("Any") + "/stop"},
		{http.MethodGet, "/timers"},
		{http.MethodGet, "/health"},
		{http.MethodPost, "/health/addons/" + id + "/check"},
		{http.MethodGet, "/logs"},
		{http.MethodGet, "/logs/download"},
		{http.MethodGet, "/variables"},
		{http.MethodGet, "/sources"},
	} {
		if status := member.raw(route.method, route.path, `{"text":"hi"}`, nil); status != http.StatusForbidden {
			t.Errorf("member %s %s: %d", route.method, route.path, status)
		}
		if status := anonymous.raw(route.method, route.path, `{"text":"hi"}`, nil); status != http.StatusUnauthorized {
			t.Errorf("anonymous %s %s: %d", route.method, route.path, status)
		}
	}
}

func TestLiveSessionsTellHowEachPlaybackPlays(t *testing.T) {
	sessions := &fakeSessions{}
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) { o.Sessions = sessions })
	admin := api.signedIn("root", true)
	series := randomID()
	gpu := &hls.Hardware{Method: "cuda"}
	converted := jellyfin.LiveSession{
		User:   accounts.User{ID: randomID(), Name: "alice", QualityGroup: 1080},
		Device: accounts.Device{ID: randomID(), DeviceInfo: accounts.DeviceInfo{DeviceName: "Living room", Client: "Test App", ClientVersion: "2.0", RemoteAddress: "192.0.2.10"}},
		Playing: playback.NowPlaying{PlayState: playback.PlayState{PlayMethod: "Transcode", Position: 90 * time.Second, Paused: true},
			Started: time.Now().Add(-time.Minute)},
		Item: library.Item{ID: randomID(), Kind: library.KindEpisode, Name: "Pilot", SeriesID: series, SeriesName: "Harbor Lights",
			ParentIndexNumber: 1, IndexNumber: 2, Runtime: 50 * time.Minute},
		Found:        true,
		Controllable: true,
		Source: &media.Analysis{Bitrate: 20_000_000, Streams: []media.Stream{
			{Type: "video", Codec: "hevc", Width: 3840, Height: 2160}, {Type: "audio", Codec: "eac3"}}},
		Encodings: []hls.Running{{Key: hls.Key{Converts: true}, Opened: true,
			Video:      &hls.VideoEncoding{Encoder: "h264_nvenc", Width: 1920, Height: 1080, Bitrate: 8_000_000, Hardware: gpu},
			AudioCodec: "aac", AudioChannels: 2, AudioBitrate: 192_000}},
		Reasons: []string{"VideoCodecNotSupported", "ContainerBitrateExceedsLimit"},
	}
	direct := jellyfin.LiveSession{
		User:    accounts.User{ID: randomID(), Name: "bob"},
		Device:  accounts.Device{ID: randomID(), DeviceInfo: accounts.DeviceInfo{DeviceName: "Phone", Client: "Other App"}},
		Playing: playback.NowPlaying{PlayState: playback.PlayState{PlayMethod: "DirectPlay"}},
	}
	sessions.sessions = []jellyfin.LiveSession{converted, direct}

	var listed []liveSessionJSON
	if status := admin.raw(http.MethodGet, "/sessions", "", &listed); status != http.StatusOK || len(listed) != 2 {
		t.Fatalf("sessions: %d %+v", status, listed)
	}
	first := listed[0]
	if first.ID != converted.Device.ID.String() || first.User.Name != "alice" || first.User.QualityGroup != 1080 ||
		first.Device.Name != "Living room" || first.Device.App != "Test App" || !first.Controllable || !first.Paused || first.Position != 90 {
		t.Errorf("who and where: %+v", first)
	}
	if first.Item == nil || first.Item.Kind != "episode" || first.Item.SeriesName != "Harbor Lights" || first.Item.Season != 1 ||
		first.Item.Episode != 2 || first.Item.PosterID != series.String() || first.Item.Runtime != 3000 {
		t.Errorf("item: %+v", first.Item)
	}
	if first.Delivery != deliveryConversion || !slices.Equal(first.Reasons, converted.Reasons) || first.Video == nil ||
		first.Video.Hardware != "cuda" || first.Video.Encoder != "h264_nvenc" || first.Audio == nil || first.Audio.Codec != "aac" {
		t.Errorf("conversion: %+v %+v %+v", first, first.Video, first.Audio)
	}
	if first.Source == nil || first.Source.Height != 2160 || first.Source.VideoCodec != "hevc" || first.Sent == nil ||
		first.Sent.Height != 1080 || first.Sent.VideoCodec != "h264" || first.Sent.AudioCodec != "aac" || first.Sent.Bitrate != 8_192_000 {
		t.Errorf("source and sent: %+v %+v", first.Source, first.Sent)
	}
	second := listed[1]
	if second.Delivery != deliveryDirectPlay || second.Item != nil || second.Sent != nil || second.Video != nil || second.Reasons == nil {
		t.Errorf("direct play: %+v", second)
	}

	target := "/sessions/" + converted.Device.ID.String()
	if status := admin.raw(http.MethodPost, target+"/stop", "", nil); status != http.StatusNoContent {
		t.Errorf("stop: %d", status)
	}
	if status := admin.raw(http.MethodPost, target+"/message", `{"header":"Polyfin","text":"  Restarting in 5 minutes ","timeout":10}`, nil); status != http.StatusNoContent {
		t.Errorf("message: %d", status)
	}
	for _, body := range []string{`{"text":"  "}`, `{"text":"` + strings.Repeat("x", 501) + `"}`, `{"text":"hi","timeout":-1}`} {
		if status, answer, _ := admin.call(http.MethodPost, target+"/message", rawJSON(body)); status != http.StatusBadRequest || answer["error"] != "invalid_message" {
			t.Errorf("message %s: %d %v", body, status, answer)
		}
	}
	sessions.mu.Lock()
	if !slices.Equal(sessions.stopped, []accounts.ID{converted.Device.ID}) ||
		!slices.Equal(sessions.messages, []string{"root|Polyfin|Restarting in 5 minutes|10s"}) {
		t.Errorf("commands sent: %v %v", sessions.stopped, sessions.messages)
	}
	sessions.mu.Unlock()

	for err, want := range map[error]string{
		jellyfin.ErrNotControllable: "not_controllable",
		jellyfin.ErrControlRefused:  "remote_control_refused",
		accounts.ErrNotFound:        "not_found",
	} {
		sessions.mu.Lock()
		sessions.err = err
		sessions.mu.Unlock()
		if _, body, _ := admin.call(http.MethodPost, target+"/stop", nil); body["error"] != want {
			t.Errorf("%v: %v", err, body)
		}
	}
}

// rawJSON is a request body sent as written.
type rawJSON string

func (r rawJSON) MarshalJSON() ([]byte, error) { return []byte(r), nil }

func TestTasksAreListedRunAndStopped(t *testing.T) {
	registry := tasks.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	registry.Register(tasks.Task{Key: "Wait", Category: tasks.CategoryMaintenance,
		Text: map[string]tasks.Text{"en": {Name: "Wait"}, "fr": {Name: "Attendre"}},
		Run:  func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }})
	registry.Register(tasks.Task{Key: "Hourly", Category: tasks.CategoryLibrary, Interval: time.Hour,
		Text: map[string]tasks.Text{"en": {Name: "Hourly"}},
		Run:  func(context.Context) error { return errors.New("addon down") }})
	registry.Start(t.Context())
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) { o.Tasks = registry })
	admin := api.signedIn("root", true)

	list := func() map[string]taskJSON {
		t.Helper()
		var listed []taskJSON
		if status := admin.raw(http.MethodGet, "/tasks?language=fr", "", &listed); status != http.StatusOK {
			t.Fatalf("tasks: %d", status)
		}
		byKey := map[string]taskJSON{}
		for _, task := range listed {
			byKey[task.Key] = task
		}
		return byKey
	}
	listed := list()
	if wait := listed["Wait"]; wait.Name != "Attendre" || wait.Category != "Maintenance" || wait.State != "Idle" || wait.Next != nil || wait.Last != nil {
		t.Errorf("wait: %+v", wait)
	}
	if hourly := listed["Hourly"]; hourly.Interval != 3600 || hourly.Next == nil || hourly.Category != "Médiathèque" {
		t.Errorf("hourly: %+v", hourly)
	}

	id := tasks.ID("Wait")
	if status := admin.raw(http.MethodPost, "/tasks/"+id+"/run", "", nil); status != http.StatusNoContent {
		t.Fatalf("run: %d", status)
	}
	waitUntil(t, func() bool { return list()["Wait"].State == "Running" })
	if status := admin.raw(http.MethodPost, "/tasks/"+id+"/stop", "", nil); status != http.StatusNoContent {
		t.Fatalf("stop: %d", status)
	}
	waitUntil(t, func() bool { last := list()["Wait"].Last; return last != nil && last.Status == "Cancelled" })
	if status := admin.raw(http.MethodPost, "/tasks/"+tasks.ID("Missing")+"/run", "", nil); status != http.StatusNotFound {
		t.Errorf("unknown task: %d", status)
	}
}

func waitUntil(t *testing.T, done func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !done(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
	}
}

func TestLogsAreFollowedAndDownloadedRedacted(t *testing.T) {
	ring := logs.NewRing(logs.Capacity)
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) { o.Logs = ring })
	admin := api.signedIn("root", true)
	_, _ = ring.Write([]byte(`level=WARN msg="An addon failed" url=https://user:pw@addon.example/secret-config/manifest.json token=abc123` + "\n"))

	var page struct {
		Lines []string `json:"lines"`
		Next  uint64   `json:"next"`
	}
	if status := admin.raw(http.MethodGet, "/logs", "", &page); status != http.StatusOK || len(page.Lines) == 0 {
		t.Fatalf("logs: %d %+v", status, page)
	}
	all := strings.Join(page.Lines, "\n")
	for _, secret := range []string{"secret-config", "abc123", "pw@"} {
		if strings.Contains(all, secret) {
			t.Errorf("the log shows %q: %s", secret, all)
		}
	}
	if !strings.Contains(all, "https://addon.example/…") {
		t.Errorf("the addon's host is missing: %s", all)
	}
	// Following the log returns the new lines only.
	_, _ = ring.Write([]byte("level=INFO msg=\"Polyfin started\"\n"))
	next := page.Next
	if status := admin.raw(http.MethodGet, "/logs?after="+strconv.FormatUint(next, 10), "", &page); status != http.StatusOK ||
		len(page.Lines) != 1 || !strings.Contains(page.Lines[0], "Polyfin started") || page.Next != next+1 {
		t.Errorf("new lines: %d %+v", status, page)
	}
	if status := admin.raw(http.MethodGet, "/logs?limit=0", "", nil); status != http.StatusBadRequest {
		t.Errorf("limit 0: %d", status)
	}

	request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, api.url+"/admin/api/logs/download", nil)
	response, err := admin.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Disposition"), "attachment") ||
		strings.Contains(string(body), "abc123") || !strings.Contains(string(body), "Polyfin started") {
		t.Errorf("download: %d %v %s", response.StatusCode, response.Header, body)
	}
}

func TestVariablesHideSecrets(t *testing.T) {
	environ := []string{"POLYFIN_DATABASE_URL=postgresql://polyfin:s3cret@db.internal:5432/media", "POLYFIN_API_KEY=k3y"}
	cfg, err := config.Load(func(name string) string {
		if name == "POLYFIN_DATABASE_URL" {
			return "postgresql://polyfin:s3cret@db.internal:5432/media"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) { o.Variables = config.Variables(environ, cfg) })
	admin := api.signedIn("root", true)
	var variables []variableJSON
	if status := admin.raw(http.MethodGet, "/variables", "", &variables); status != http.StatusOK {
		t.Fatalf("variables: %d", status)
	}
	byName := map[string]variableJSON{}
	for _, v := range variables {
		byName[v.Name] = v
		if strings.Contains(v.Value, "s3cret") || strings.Contains(v.Value, "k3y") {
			t.Errorf("%s shows a secret: %q", v.Name, v.Value)
		}
	}
	if db := byName["POLYFIN_DATABASE_URL"]; db.Value != "db.internal:5432/media" || !db.Set {
		t.Errorf("database: %+v", db)
	}
	if key := byName["POLYFIN_API_KEY"]; !key.Hidden || key.Value != "" || !key.Set || key.Known {
		t.Errorf("key: %+v", key)
	}
	if listen := byName["POLYFIN_LISTEN"]; listen.Value != ":8096" || listen.Set {
		t.Errorf("default: %+v", listen)
	}
}

func TestHealthDescribesTheServerFromWhatItRecords(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cacheDir := t.TempDir()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		ffmpeg = "ffmpeg-not-installed"
	}
	encoder, err := hls.NewManager(ffmpeg, t.TempDir(), logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(encoder.Close)
	encoder.LimitConversions(func() int { return 3 })
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		sources, err := source.New(cacheDir, 1<<30, deps.client, logger)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = sources.Close() })
		o.Health = HealthSources{Addons: deps.client, Cache: sources, CacheDir: cacheDir, Encoder: encoder,
			DatabaseSize: func(ctx context.Context) (int64, error) { return database.Size(ctx, deps.pool) }}
	})
	admin := api.signedIn("root", true)
	status, installed, _ := admin.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": localAddon(t)})
	if status != http.StatusCreated {
		t.Fatalf("install: %d %v", status, installed)
	}

	var health healthJSON
	if status := admin.raw(http.MethodGet, "/health", "", &health); status != http.StatusOK {
		t.Fatalf("health: %d", status)
	}
	if health.Process.Version != "1.2.3" || health.Process.Goroutines == 0 || health.Process.Memory == 0 || health.Process.StartedAt.IsZero() {
		t.Errorf("process: %+v", health.Process)
	}
	if !health.Database.Reachable || health.Database.Size == nil || *health.Database.Size <= 0 {
		t.Errorf("database: %+v", health.Database)
	}
	if health.Cache == nil || health.Cache.Limit != 1<<30 || health.Cache.Used != 0 {
		t.Errorf("cache: %+v", health.Cache)
	}
	if len(health.Disks) != 1 || health.Disks[0].Folder != "cache" || health.Disks[0].Free <= 0 {
		t.Errorf("disks: %+v", health.Disks)
	}
	if health.Transcoder == nil || health.Transcoder.Limit != 3 || health.Transcoder.Conversions != 0 || health.Thumbnails != nil {
		t.Errorf("transcoder: %+v", health.Transcoder)
	}
	if len(health.Addons) != 1 || health.Addons[0].ID != installed["id"] || health.Addons[0].Requests != 1 ||
		health.Addons[0].LastSuccessAt == nil || health.Addons[0].Failure != "" {
		t.Fatalf("addons: %+v", health.Addons)
	}

	// A check asks the addon once, and not again within a minute.
	var checked addonHealthJSON
	if status := admin.raw(http.MethodPost, "/health/addons/"+health.Addons[0].ID+"/check", "", &checked); status != http.StatusOK || checked.Requests != 2 {
		t.Errorf("check: %d %+v", status, checked)
	}
	if status, body, response := admin.call(http.MethodPost, "/health/addons/"+health.Addons[0].ID+"/check", nil); status != http.StatusTooManyRequests ||
		body["error"] != "too_soon" || response.Header.Get("Retry-After") == "" {
		t.Errorf("second check: %d %v", status, body)
	}
	if status := admin.raw(http.MethodPost, "/health/addons/"+randomID().String()+"/check", "", nil); status != http.StatusNotFound {
		t.Errorf("unknown addon: %d", status)
	}
}

func TestActivityIsFilteredAndPaged(t *testing.T) {
	api := newTestAPI(t, 10)
	admin := api.signedIn("root", true)
	api.browser().call(http.MethodPost, "/session", map[string]string{"name": "root", "password": "wrong password"})
	admin.call(http.MethodPost, "/users", map[string]any{"name": "bob", "password": "correct horse"})
	admin.call(http.MethodPost, "/users", map[string]any{"name": "carol", "password": "correct horse"})
	var page struct {
		Items []activityEntryJSON `json:"items"`
		Total int                 `json:"total"`
	}
	if admin.raw(http.MethodGet, "/activity?severity=Warning,Error", "", &page); page.Total != 1 || page.Items[0].Type != "AuthenticationFailed" {
		t.Errorf("problems: %+v", page)
	}
	if admin.raw(http.MethodGet, "/activity?type=UserCreated&limit=1&start=1", "", &page); page.Total != 2 || len(page.Items) != 1 ||
		!strings.Contains(page.Items[0].Name, "bob") {
		t.Errorf("second user created: %+v", page)
	}
}

func TestTimersTellWhenRecordingIsOff(t *testing.T) {
	api := newTestAPI(t, 10)
	admin := api.signedIn("root", true)
	_, body, _ := admin.call(http.MethodGet, "/timers", nil)
	if body["available"] != false || body["timers"] == nil {
		t.Errorf("timers: %v", body)
	}
}
