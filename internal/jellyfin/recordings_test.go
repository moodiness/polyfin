package jellyfin

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/moodiness/polyfin/internal/activity"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/recordings"
	"github.com/moodiness/polyfin/internal/stremio"
)

// recordingServer is a test server that records into dir, analyzing with
// ffprobe; an empty dir leaves recording off.
func recordingServer(t *testing.T, ffprobe, dir string) (testServer, *recordings.Service) {
	t.Helper()
	var service *recordings.Service
	s := newProbingServer(t, 10, ffprobe, func(o *Options, pool *pgxpool.Pool) {
		service = recordings.New(recordings.Config{DB: pool, Dir: dir, Guide: o.Library, Recorder: o.Playback, Users: o.Accounts,
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), RetryDelay: time.Second})
		o.Recordings = service
	})
	return s, service
}

// recordingMember installs addon and signs in a member allowed to record
// when manages is set.
func recordingMember(t *testing.T, s testServer, addonURL string, manages bool) (string, accounts.User) {
	t.Helper()
	user := s.user("member", func(c *accounts.UserChanges) { c.LiveTvManagement = &manages })
	if _, err := s.addons.Install(t.Context(), addons.Shared(), addonURL, false); err != nil {
		t.Fatal(err)
	}
	return s.signIn("member", "tv"), user
}

// timerKeys and seriesTimerKeys are the properties of Jellyfin's
// TimerInfoDto and SeriesTimerInfoDto (BaseTimerInfoDto included).
var (
	baseTimerKeys = []string{"Id", "Type", "ServerId", "ExternalId", "ChannelId", "ExternalChannelId", "ChannelName",
		"ChannelPrimaryImageTag", "ProgramId", "ExternalProgramId", "Name", "Overview", "StartDate", "EndDate", "ServiceName",
		"Priority", "PrePaddingSeconds", "PostPaddingSeconds", "IsPrePaddingRequired", "ParentBackdropItemId",
		"ParentBackdropImageTags", "IsPostPaddingRequired", "KeepUntil"}
	timerKeys       = append(slices.Clone(baseTimerKeys), "Status", "SeriesTimerId", "ExternalSeriesTimerId", "RunTimeTicks", "ProgramInfo")
	seriesTimerKeys = append(slices.Clone(baseTimerKeys), "RecordAnyTime", "SkipEpisodesInLibrary", "RecordAnyChannel", "KeepUpTo",
		"RecordNewOnly", "Days", "DayPattern", "ImageTags", "ParentThumbItemId", "ParentThumbImageTag", "ParentPrimaryImageItemId",
		"ParentPrimaryImageTag")
)

// checkKeys reports the properties of object that Jellyfin's DTO does not
// have, and those of required it lacks.
func checkKeys(t *testing.T, what string, object map[string]any, known []string, required ...string) {
	t.Helper()
	for key := range maps.Keys(object) {
		if !slices.Contains(known, key) {
			t.Errorf("%s has %s, which Jellyfin's does not", what, key)
		}
	}
	for _, key := range required {
		if _, ok := object[key]; !ok {
			t.Errorf("%s lacks %s", what, key)
		}
	}
}

func guidePrograms(t *testing.T, s testServer, token string) map[string]BaseItemDto {
	t.Helper()
	var programs QueryResult
	if status := s.get(t, "/LiveTv/Programs", token, &programs); status != http.StatusOK {
		t.Fatalf("programmes: %d", status)
	}
	byName := map[string]BaseItemDto{}
	for _, p := range programs.Items {
		byName[p.Name] = p
	}
	return byName
}

func timerDefaults(t *testing.T, s testServer, token, program string) map[string]any {
	t.Helper()
	status, body := s.call(http.MethodGet, "/LiveTv/Timers/Defaults?programId="+program, app("tv", token), nil)
	var defaults map[string]any
	if status != http.StatusOK || json.Unmarshal(body, &defaults) != nil {
		t.Fatalf("timer defaults: %d %s", status, body)
	}
	return defaults
}

// Without a recordings folder, Polyfin answers as a server that records
// nothing, as it did before recording existed.
func TestRecordingRoutesWithoutAFolder(t *testing.T) {
	addon := newTVAddon(t, true, "")
	s, _ := recordingServer(t, "ffprobe-not-installed", "")
	token, _ := recordingMember(t, s, addon.url, true)
	for _, path := range []string{"/LiveTv/Recordings", "/LiveTv/Recordings/Folders", "/LiveTv/Timers", "/LiveTv/SeriesTimers"} {
		var result struct {
			Items            []any
			TotalRecordCount int
		}
		if status := s.get(t, path, token, &result); status != http.StatusOK || result.Items == nil || len(result.Items) != 0 || result.TotalRecordCount != 0 {
			t.Errorf("%s without a folder: %d %+v", path, status, result)
		}
	}
	program := guidePrograms(t, s, token)["Next"]
	if program.TimerId != "" {
		t.Errorf("a programme has a timer without a folder: %+v", program)
	}
	defaults := timerDefaults(t, s, token, program.Id)
	if status, body := s.call(http.MethodPost, "/LiveTv/Timers", app("tv", token), defaults); status != http.StatusBadRequest {
		t.Errorf("a timer made without a folder: %d %s", status, body)
	}
	if status, body := s.call(http.MethodPost, "/LiveTv/SeriesTimers", app("tv", token), defaults); status != http.StatusBadRequest {
		t.Errorf("a series timer made without a folder: %d %s", status, body)
	}
	unknown := "0123456789abcdef0123456789abcdef"
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{http.MethodGet, "/LiveTv/Timers/" + unknown, http.StatusNoContent},
		{http.MethodDelete, "/LiveTv/Timers/" + unknown, http.StatusNotFound},
		{http.MethodGet, "/LiveTv/SeriesTimers/" + unknown, http.StatusNotFound},
		{http.MethodDelete, "/LiveTv/SeriesTimers/" + unknown, http.StatusNotFound},
		{http.MethodGet, "/LiveTv/Recordings/" + unknown, http.StatusNotFound},
		{http.MethodDelete, "/LiveTv/Recordings/" + unknown, http.StatusNotFound},
		{http.MethodGet, "/LiveTv/LiveRecordings/" + unknown + "/stream", http.StatusNotFound},
	} {
		if status, body := s.call(tc.method, tc.path, app("tv", token), nil); status != tc.status {
			t.Errorf("%s %s without a folder: %d %s, want %d", tc.method, tc.path, status, body, tc.status)
		}
	}
}

// Timers are made from the defaults Timers/Defaults gives for a programme,
// as jellyfin-web makes them, and read, changed and cancelled as
// Jellyfin's are.
func TestTimersAsJellyfinKeepsThem(t *testing.T) {
	addon := newTVAddon(t, true, "")
	s, _ := recordingServer(t, "ffprobe-not-installed", t.TempDir())
	s.setting(t, func(settings *accounts.Settings) {
		settings.RecordingPrePadding, settings.RecordingPostPadding = 60, 120
	})
	token, _ := recordingMember(t, s, addon.url, true)
	next := guidePrograms(t, s, token)["Next"]

	defaults := timerDefaults(t, s, token, next.Id)
	checkKeys(t, "the timer defaults", defaults, seriesTimerKeys, "Id", "ChannelId", "ProgramId", "Days", "DayPattern")
	if defaults["PrePaddingSeconds"] != 60.0 || defaults["PostPaddingSeconds"] != 120.0 || defaults["ProgramId"] != next.Id ||
		defaults["Name"] != "Next" || defaults["DayPattern"] != "Daily" || defaults["RecordAnyTime"] != true || defaults["KeepUntil"] != "UntilDeleted" {
		t.Errorf("timer defaults: %v", defaults)
	}
	if status, body := s.call(http.MethodPost, "/LiveTv/Timers", app("tv", token), defaults); status != http.StatusNoContent {
		t.Fatalf("a timer made: %d %s", status, body)
	}
	// The activity log tells who scheduled it.
	if page, err := s.activity.Entries(t.Context(), activity.Query{}); err != nil ||
		!slices.ContainsFunc(page.Entries, func(e activity.Entry) bool { return e.Type == "RecordingScheduled" && strings.Contains(e.Name, "Next") }) {
		t.Errorf("activity after a timer was made: %+v %v", page.Entries, err)
	}
	// A programme has one timer.
	if status, _ := s.call(http.MethodPost, "/LiveTv/Timers", app("tv", token), defaults); status != http.StatusBadRequest {
		t.Errorf("a second timer for the programme: %d", status)
	}

	status, body := s.call(http.MethodGet, "/LiveTv/Timers?isScheduled=true", app("tv", token), nil)
	var list struct {
		Items            []map[string]any
		TotalRecordCount int
	}
	if status != http.StatusOK || json.Unmarshal(body, &list) != nil || len(list.Items) != 1 || list.TotalRecordCount != 1 {
		t.Fatalf("timers: %d %s", status, body)
	}
	timer := list.Items[0]
	checkKeys(t, "a timer", timer, timerKeys, "Id", "Type", "ServerId", "ChannelId", "ProgramId", "StartDate", "EndDate",
		"Status", "ServiceName", "KeepUntil", "RunTimeTicks", "ProgramInfo")
	if timer["Type"] != "Timer" || timer["Status"] != "New" || timer["ProgramId"] != next.Id || timer["ChannelId"] != *next.ChannelId ||
		timer["ServiceName"] != "Emby" || timer["Name"] != "Next" || timer["PrePaddingSeconds"] != 60.0 || timer["ChannelName"] != "One" {
		t.Errorf("timer: %v", timer)
	}
	id := timer["Id"].(string)
	if info, _ := timer["ProgramInfo"].(map[string]any); info["TimerId"] != id || info["Type"] != "Program" || info["Status"] != "New" {
		t.Errorf("a timer's programme: %v", info)
	}
	if status := s.get(t, "/LiveTv/Timers?isActive=true", token, &list); status != http.StatusOK || len(list.Items) != 0 {
		t.Errorf("active timers before any records: %+v", list)
	}
	// The programme shows its timer, in the guide and in its details.
	if program := guidePrograms(t, s, token)["Next"]; program.TimerId != id || program.Status != "New" {
		t.Errorf("a scheduled programme in the guide: %+v", program)
	}
	var detail BaseItemDto
	if status := s.get(t, "/Items/"+next.Id, token, &detail); status != http.StatusOK || detail.TimerId != id {
		t.Errorf("a scheduled programme's details: %d %+v", status, detail.TimerId)
	}

	// Its padding changes.
	timer["PrePaddingSeconds"], timer["PostPaddingSeconds"] = 0, 300
	if status, body := s.call(http.MethodPost, "/LiveTv/Timers/"+id, app("tv", token), timer); status != http.StatusNoContent {
		t.Fatalf("a timer changed: %d %s", status, body)
	}
	var changed TimerInfoDto
	if status := s.get(t, "/LiveTv/Timers/"+id, token, &changed); status != http.StatusOK || changed.PrePaddingSeconds != 0 || changed.PostPaddingSeconds != 300 {
		t.Errorf("a changed timer: %d %+v", status, changed)
	}
	// Cancelled, it is gone, as Jellyfin answers it: no content.
	if status, _ := s.call(http.MethodDelete, "/LiveTv/Timers/"+id, app("tv", token), nil); status != http.StatusNoContent {
		t.Fatalf("a timer cancelled: %d", status)
	}
	if status, _ := s.call(http.MethodGet, "/LiveTv/Timers/"+id, app("tv", token), nil); status != http.StatusNoContent {
		t.Errorf("a cancelled timer: %d", status)
	}
	if status, _ := s.call(http.MethodDelete, "/LiveTv/Timers/"+id, app("tv", token), nil); status != http.StatusNotFound {
		t.Errorf("a timer cancelled twice: %d", status)
	}
	if program := guidePrograms(t, s, token)["Next"]; program.TimerId != "" {
		t.Errorf("a programme whose timer was cancelled: %+v", program)
	}
}

// A series timer records the programmes of a title; cancelling it
// cancels its timers.
func TestSeriesTimersMakeTimers(t *testing.T) {
	addon := newTVAddon(t, true, "")
	s, _ := recordingServer(t, "ffprobe-not-installed", t.TempDir())
	token, _ := recordingMember(t, s, addon.url, true)
	next := guidePrograms(t, s, token)["Next"]
	defaults := timerDefaults(t, s, token, next.Id)
	defaults["RecordAnyChannel"], defaults["KeepUpTo"] = true, 3
	if status, body := s.call(http.MethodPost, "/LiveTv/SeriesTimers", app("tv", token), defaults); status != http.StatusNoContent {
		t.Fatalf("a series timer made: %d %s", status, body)
	}
	status, body := s.call(http.MethodGet, "/LiveTv/SeriesTimers", app("tv", token), nil)
	var series struct{ Items []map[string]any }
	if status != http.StatusOK || json.Unmarshal(body, &series) != nil || len(series.Items) != 1 {
		t.Fatalf("series timers: %d %s", status, body)
	}
	st := series.Items[0]
	checkKeys(t, "a series timer", st, seriesTimerKeys, "Id", "Type", "Days", "RecordAnyChannel", "KeepUpTo")
	if st["Type"] != "SeriesTimer" || st["Name"] != "Next" || st["RecordAnyChannel"] != true || st["KeepUpTo"] != 3.0 || st["DayPattern"] != "Daily" {
		t.Errorf("series timer: %v", st)
	}
	id := st["Id"].(string)
	var timers struct{ Items []TimerInfoDto }
	s.get(t, "/LiveTv/Timers?seriesTimerId="+id, token, &timers)
	if len(timers.Items) != 1 || timers.Items[0].ProgramId != next.Id || timers.Items[0].SeriesTimerId != id {
		t.Fatalf("the series timer's timers: %+v", timers.Items)
	}
	if program := guidePrograms(t, s, token)["Next"]; program.SeriesTimerId != id || program.TimerId == "" {
		t.Errorf("a programme of a series timer: %+v", program)
	}
	// Changed, it keeps its timers.
	st["RecordAnyTime"] = false
	if status, body := s.call(http.MethodPost, "/LiveTv/SeriesTimers/"+id, app("tv", token), st); status != http.StatusNoContent {
		t.Fatalf("a series timer changed: %d %s", status, body)
	}
	var changed SeriesTimerInfoDto
	if s.get(t, "/LiveTv/SeriesTimers/"+id, token, &changed); changed.RecordAnyTime {
		t.Errorf("a changed series timer: %+v", changed)
	}
	if s.get(t, "/LiveTv/Timers", token, &timers); len(timers.Items) != 1 {
		t.Errorf("timers of a changed series timer: %+v", timers.Items)
	}
	if status, _ := s.call(http.MethodDelete, "/LiveTv/SeriesTimers/"+id, app("tv", token), nil); status != http.StatusNoContent {
		t.Fatalf("a series timer cancelled: %d", status)
	}
	if s.get(t, "/LiveTv/Timers", token, &timers); len(timers.Items) != 0 {
		t.Errorf("timers of a cancelled series timer: %+v", timers.Items)
	}
	if status, _ := s.call(http.MethodGet, "/LiveTv/SeriesTimers/"+id, app("tv", token), nil); status != http.StatusNotFound {
		t.Errorf("a cancelled series timer: %d", status)
	}
}

// Recording needs Jellyfin's EnableLiveTvManagement, given to
// administrators, and stays within the user's parental control and
// allowed hours.
func TestRecordingNeedsPermission(t *testing.T) {
	addon := newTVAddon(t, true, "")
	s, _ := recordingServer(t, "ffprobe-not-installed", t.TempDir())
	token, member := recordingMember(t, s, addon.url, false)
	next := guidePrograms(t, s, token)["Next"]
	defaults := timerDefaults(t, s, token, next.Id)
	for _, route := range [][2]string{{http.MethodPost, "/LiveTv/Timers"}, {http.MethodPost, "/LiveTv/SeriesTimers"},
		{http.MethodDelete, "/LiveTv/Timers/0123456789abcdef0123456789abcdef"}, {http.MethodDelete, "/LiveTv/Recordings/0123456789abcdef0123456789abcdef"}} {
		if status, _ := s.call(route[0], route[1], app("tv", token), defaults); status != http.StatusForbidden {
			t.Errorf("%s %s without the permission: %d", route[0], route[1], status)
		}
	}
	// Its policy says so, and an administrator's policy grants it.
	var me UserDto
	if s.get(t, "/Users/Me", token, &me); me.Policy.EnableLiveTvManagement {
		t.Errorf("a member's policy: %+v", me.Policy)
	}
	admin, err := s.store.CreateUser(t.Context(), accounts.NewUser{Name: "boss", Password: "correct horse", IsAdministrator: true})
	if err != nil || !admin.LiveTvManagement {
		t.Fatalf("a new administrator: %v %+v", err, admin)
	}
	// Granted in the policy, as jellyfin-web grants it.
	adminToken := s.signIn("boss", "web")
	policy := map[string]any{"EnableLiveTvManagement": true, "EnableLiveTvAccess": true, "IsHidden": true}
	if status, body := s.call(http.MethodPost, "/Users/"+member.ID.String()+"/Policy", app("web", adminToken), policy); status != http.StatusNoContent {
		t.Fatalf("policy: %d %s", status, body)
	}
	if status, body := s.call(http.MethodPost, "/LiveTv/Timers", app("tv", token), defaults); status != http.StatusNoContent {
		t.Fatalf("a timer made once allowed: %d %s", status, body)
	}

	// A user blocking a programme's genre cannot record it, nor sees its
	// timer.
	s.user("teen", func(c *accounts.UserChanges) {
		yes := true
		c.LiveTvManagement, c.BlockedGenres = &yes, &[]string{"news"}
	})
	teen := s.signIn("teen", "phone")
	now := guidePrograms(t, s, teen)["Now"]
	if status, _ := s.call(http.MethodGet, "/LiveTv/Timers/Defaults?programId="+now.Id, app("phone", teen), nil); status != http.StatusNotFound {
		t.Errorf("defaults for a blocked programme: %d", status)
	}
	nowDefaults := timerDefaults(t, s, token, now.Id)
	if status, _ := s.call(http.MethodPost, "/LiveTv/Timers", app("phone", teen), nowDefaults); status != http.StatusBadRequest {
		t.Errorf("a blocked programme recorded: %d", status)
	}
	if status, _ := s.call(http.MethodPost, "/LiveTv/Timers", app("tv", token), nowDefaults); status != http.StatusNoContent {
		t.Fatalf("a timer for the news: %d", status)
	}
	var timers struct{ Items []TimerInfoDto }
	if s.get(t, "/LiveTv/Timers", teen, &timers); len(timers.Items) != 1 || timers.Items[0].Name != "Next" {
		t.Errorf("timers seen by a user blocking the news: %+v", timers.Items)
	}

	// Outside their allowed hours, a user schedules nothing: their only
	// day is three days away, whatever the time zone.
	night := s.user("night", func(c *accounts.UserChanges) { yes := true; c.LiveTvManagement = &yes })
	nightToken := s.signIn("night", "lamp")
	away := time.Now().Add(72 * time.Hour).Weekday().String()
	if _, err := s.store.UpdateUser(t.Context(), night.ID, accounts.UserChanges{
		AccessSchedules: &[]accounts.AccessSchedule{{Day: away, StartHour: 0, EndHour: 24}}}, nil); err != nil {
		t.Fatal(err)
	}
	if status, _ := s.call(http.MethodPost, "/LiveTv/Timers", app("lamp", nightToken), defaults); status != http.StatusForbidden {
		t.Errorf("a timer made outside the user's hours: %d", status)
	}
}

// recordingAddon serves a live TV catalog of one channel whose stream is
// the HLS playlist in dir, and whose guide has one programme, from start to
// end.
func recordingAddon(t *testing.T, dir string, start, end time.Time) string {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		channels := []stremio.Meta{{ID: "tv:rec", Type: "tv", Name: "Rec"}}
		switch {
		case path == "/manifest.json":
			manifest := stremio.Manifest{ID: "rec", Name: "Rec", Version: "1", Types: []string{"tv"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "tv", ID: "channels", Name: "Channels", Extra: []stremio.Extra{{Name: "date"}}}}}
			manifest.BehaviorHints.EpgProvider = true
			_ = json.NewEncoder(w).Encode(manifest)
		case strings.HasPrefix(path, "/catalog/tv/channels/date="):
			if strings.Contains(path, start.UTC().Format(time.DateOnly)) {
				channels[0].Videos = []stremio.Video{{ID: "show", Title: "Show", Genres: []string{"Series"},
					StartTime: start.UTC().Format(time.RFC3339), EndTime: end.UTC().Format(time.RFC3339)}}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metasDetailed": channels})
		case path == "/catalog/tv/channels.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": channels})
		case path == "/stream/tv/tv%3Arec.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{{Name: "Live", URL: server.URL + "/live/one.m3u8"}}})
		case strings.HasPrefix(path, "/live/"):
			http.ServeFile(w, r, filepath.Join(dir, strings.TrimPrefix(path, "/live/")))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// A programme is recorded with FFmpeg from its channel's stream, read as
// live playback reads it, then plays as a file, directly or remuxed into
// HLS, and is deleted with its file.
func TestProgrammeIsRecordedAndPlays(t *testing.T) {
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	if _, err := os.Stat(ffprobe); err != nil {
		t.Skip("no ffprobe next to POLYFIN_TEST_FFMPEG")
	}
	// A live playlist: no end, so FFmpeg waits for more until the
	// recording ends.
	stream := t.TempDir()
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24:duration=12",
		"-f", "lavfi", "-i", "sine=duration=12", "-c:v", "libx264", "-g", "24", "-pix_fmt", "yuv420p", "-c:a", "aac",
		"-f", "hls", "-hls_time", "2", "-hls_list_size", "0", "-hls_flags", "omit_endlist", filepath.Join(stream, "one.m3u8"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	now := time.Now()
	addonURL := recordingAddon(t, stream, now.Add(-time.Second), now.Add(6*time.Second))
	folder := t.TempDir()
	s, service := recordingServer(t, ffprobe, folder)
	token, user := recordingMember(t, s, addonURL, true)
	// Thumbnails are on, yet recordings get none.
	s.setting(t, func(settings *accounts.Settings) { settings.Trickplay, settings.ChapterImages = true, true })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { service.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })

	show := guidePrograms(t, s, token)["Show"]
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	liveAnalysis(t, s, user, channels.Items[0].Id)
	if status, body := s.call(http.MethodPost, "/LiveTv/Timers", app("tv", token), timerDefaults(t, s, token, show.Id)); status != http.StatusNoContent {
		t.Fatalf("a timer made: %d %s", status, body)
	}

	// It records at once: it is under way, its timer active.
	var recording BaseItemDto
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		var active QueryResult
		s.get(t, "/LiveTv/Recordings?isInProgress=true", token, &active)
		if len(active.Items) == 1 {
			recording = active.Items[0]
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if recording.Status != "InProgress" || recording.TimerId == "" || recording.Type != "Video" || recording.Name != "Show" {
		t.Fatalf("a recording under way: %+v", recording)
	}
	var timers struct{ Items []TimerInfoDto }
	if s.get(t, "/LiveTv/Timers?isActive=true", token, &timers); len(timers.Items) != 1 || timers.Items[0].Status != "InProgress" {
		t.Errorf("the active timer: %+v", timers.Items)
	}
	// Its file streams as it is written.
	status, live := s.call(http.MethodGet, "/LiveTv/LiveRecordings/"+recording.TimerId+"/stream", app("tv", token), nil)
	if status != http.StatusOK || len(live) == 0 || live[0] != 0x47 {
		t.Errorf("the recording under way: %d, %d bytes", status, len(live))
	}

	// Once the programme ends, it is one Matroska file.
	for id := recording.Id; time.Now().Before(deadline.Add(20 * time.Second)); time.Sleep(200 * time.Millisecond) {
		recording = BaseItemDto{}
		s.get(t, "/LiveTv/Recordings/"+id, token, &recording)
		if recording.Status == "" {
			break
		}
	}
	if recording.Trickplay != nil {
		t.Errorf("a recording with a Trickplay field: %+v", *recording.Trickplay)
	}
	if recording.Status != "" || recording.MediaSources == nil || len(*recording.MediaSources) != 1 || recording.CanDelete == nil || !*recording.CanDelete {
		t.Fatalf("a finished recording: %+v", recording)
	}
	files, _ := filepath.Glob(filepath.Join(folder, "*"))
	if len(files) != 1 || filepath.Ext(files[0]) != ".mkv" {
		t.Fatalf("recordings folder: %v", files)
	}
	var folders QueryResult
	if s.get(t, "/LiveTv/Recordings/Folders", token, &folders); len(folders.Items) != 1 || folders.Items[0].Type != "CollectionFolder" {
		t.Fatalf("recording folders: %+v", folders)
	}
	var listed QueryResult
	if s.get(t, "/Items?parentId="+folders.Items[0].Id, token, &listed); len(listed.Items) != 1 || listed.Items[0].Id != recording.Id {
		t.Errorf("the recordings folder lists: %+v", listed)
	}

	// It plays as it is on an app that plays Matroska.
	profile, _ := os.ReadFile(filepath.Join(playbackFixtures, "profiles", "androidtv.json"))
	status, body := s.call(http.MethodPost, "/Items/"+recording.Id+"/PlaybackInfo", app("tv", token),
		map[string]any{"DeviceProfile": json.RawMessage(profile)})
	var info playbackInfoResponse
	if status != http.StatusOK || json.Unmarshal(body, &info) != nil || len(info.MediaSources) != 1 || !info.MediaSources[0].SupportsDirectPlay {
		t.Fatalf("PlaybackInfo of a recording: %d %s", status, body)
	}
	if info.MediaSources[0].RunTimeTicks == nil || *info.MediaSources[0].RunTimeTicks < int64(3*time.Second/100) {
		t.Errorf("the recording's runtime: %+v", info.MediaSources[0].RunTimeTicks)
	}
	status, file := s.call(http.MethodGet, strings.TrimPrefix(info.MediaSources[0].Path, s.url), "", nil)
	if status != http.StatusOK || len(file) < 4 || string(file[:4]) != "\x1a\x45\xdf\xa3" {
		t.Errorf("the recording's file: %d, %d bytes", status, len(file))
	}
	// It is remuxed into HLS for an app that does not.
	profile, _ = os.ReadFile(filepath.Join(playbackFixtures, "profiles", "swiftfin-native.json"))
	status, body = s.call(http.MethodPost, "/Items/"+recording.Id+"/PlaybackInfo", app("tv", token),
		map[string]any{"DeviceProfile": json.RawMessage(profile)})
	info = playbackInfoResponse{}
	if status != http.StatusOK || json.Unmarshal(body, &info) != nil || len(info.MediaSources) != 1 || info.MediaSources[0].TranscodingUrl == "" {
		t.Fatalf("PlaybackInfo of a recording remuxed: %d %s", status, body)
	}
	transcoding := info.MediaSources[0].TranscodingUrl
	base, query, _ := strings.Cut(transcoding, "master.m3u8")
	if status, master := s.call(http.MethodGet, transcoding, "", nil); status != http.StatusOK || !strings.Contains(string(master), "main.m3u8") {
		t.Fatalf("master playlist: %d %s", status, master)
	}
	status, media := s.call(http.MethodGet, base+"main.m3u8"+query, "", nil)
	if status != http.StatusOK || !strings.Contains(string(media), "#EXT-X-ENDLIST") {
		t.Fatalf("media playlist: %d %s", status, media)
	}
	var first string
	for line := range strings.Lines(string(media)) {
		if strings.HasPrefix(line, "hls1/") {
			first = strings.TrimSpace(line)
			break
		}
	}
	if status, segment := s.call(http.MethodGet, base+first, "", nil); status != http.StatusOK || len(segment) == 0 {
		t.Errorf("a segment of the recording: %d", status)
	}
	// Its file is served as Jellyfin serves an item's, under the download
	// permissions, and saved under its name.
	status, whole := s.call(http.MethodGet, "/Items/"+recording.Id+"/File", app("tv", token), nil)
	if status != http.StatusOK || len(whole) != len(file) {
		t.Errorf("the recording's file: %d, %d bytes", status, len(whole))
	}
	request, _ := http.NewRequest(http.MethodGet, s.url+"/Items/"+recording.Id+"/Download", nil)
	request.Header.Set("Authorization", app("tv", token))
	if response, err := http.DefaultClient.Do(request); err != nil || response.StatusCode != http.StatusOK ||
		!strings.Contains(response.Header.Get("Content-Disposition"), ".mkv") {
		t.Errorf("the recording downloaded: %v %v", response, err)
	} else {
		response.Body.Close()
	}
	s.setting(t, func(settings *accounts.Settings) { settings.Downloads = false })
	if status, _ := s.call(http.MethodGet, "/Items/"+recording.Id+"/File", app("tv", token), nil); status != http.StatusForbidden {
		t.Errorf("the recording's file while downloads are off: %d", status)
	}
	s.setting(t, func(settings *accounts.Settings) { settings.Downloads = true })
	// Other app routes answer without failing; subtitle files are refused.
	for _, route := range [][2]string{{http.MethodGet, "/Items/" + recording.Id + "/Images"}, {http.MethodPost, "/Items/" + recording.Id + "/Refresh"}} {
		if status, body := s.call(route[0], route[1], app("tv", token), nil); status >= http.StatusInternalServerError {
			t.Errorf("%s %s on a recording: %d %s", route[0], route[1], status, body)
		}
	}
	upload := map[string]any{"Language": "eng", "Format": "srt", "Data": "MQowMDowMDowMSwwMDAgLS0+IDAwOjAwOjAyLDAwMApIaQo="}
	if status, _ := s.call(http.MethodPost, "/Videos/"+recording.Id+"/Subtitles", app("tv", token), upload); status != http.StatusBadRequest && status != http.StatusForbidden {
		t.Errorf("a subtitle file for a recording: %d", status)
	}
	admin, err := s.store.CreateUser(t.Context(), accounts.NewUser{Name: "boss", Password: "correct horse", IsAdministrator: true})
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	if _, err := s.store.UpdateUser(t.Context(), admin.ID, accounts.UserChanges{LiveTv: &yes}, nil); err != nil {
		t.Fatal(err)
	}
	if status, _ := s.call(http.MethodPost, "/Videos/"+recording.Id+"/Subtitles", app("web", s.signIn("boss", "web")), upload); status != http.StatusBadRequest {
		t.Errorf("a subtitle file for a recording, by an administrator: %d", status)
	}

	// Deleted, it is gone with its file.
	if status, _ := s.call(http.MethodDelete, "/LiveTv/Recordings/"+recording.Id, app("tv", token), nil); status != http.StatusNoContent {
		t.Fatalf("a recording deleted: %d", status)
	}
	if files, _ := filepath.Glob(filepath.Join(folder, "*")); len(files) != 0 {
		t.Errorf("files of a deleted recording: %v", files)
	}
	if status, _ := s.call(http.MethodGet, "/LiveTv/Recordings/"+recording.Id, app("tv", token), nil); status != http.StatusNotFound {
		t.Errorf("a deleted recording: %d", status)
	}
	if page, err := s.activity.Entries(t.Context(), activity.Query{}); err != nil ||
		!slices.ContainsFunc(page.Entries, func(e activity.Entry) bool { return e.Type == "RecordingDeleted" && strings.Contains(e.Name, "Show") }) {
		t.Errorf("activity after a recording was deleted: %+v %v", page.Entries, err)
	}
}

// Programmes fed by an XMLTV guide are scheduled like Native EPG ones:
// timers, series timers matching their title on their channel at their
// time of day, and the timers they show.
func TestXMLTVProgrammesAreScheduled(t *testing.T) {
	addon := newTVAddon(t, true, "")
	s, _ := recordingServer(t, "ffprobe-not-installed", t.TempDir())
	token, _ := recordingMember(t, s, addon.url, true)
	now := time.Now().UTC().Truncate(time.Minute)
	at := func(d time.Duration) string { return now.Add(d).Format("20060102150405 -0700") }
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="two-x"><display-name>Two</display-name></channel>
  <programme channel="two-x" start="%s" stop="%s"><title>Talk Show</title><sub-title>Pilot</sub-title></programme>
  <programme channel="two-x" start="%s" stop="%s"><title>Talk Show</title></programme>
  <programme channel="two-x" start="%s" stop="%s"><title>Talk Show</title></programme>
</tv>`, at(time.Hour), at(2*time.Hour), at(5*time.Hour), at(6*time.Hour), at(25*time.Hour), at(26*time.Hour))
	guide := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, body) }))
	t.Cleanup(guide.Close)
	libraries, err := s.addons.Libraries(t.Context(), addons.Shared())
	if err != nil || len(libraries) != 1 {
		t.Fatal(libraries, err)
	}
	key := addons.LibraryKey{AddonID: libraries[0].AddonID, CatalogType: "tv", CatalogID: "channels"}
	if _, err := s.addons.SetGuide(t.Context(), addons.Shared(), key, guide.URL+"/epg"); err != nil {
		t.Fatal(err)
	}
	if err := s.library.RefreshGuide(t.Context(), addons.Shared(), key); err != nil {
		t.Fatal(err)
	}
	var shows []BaseItemDto
	var programs QueryResult
	s.get(t, "/LiveTv/Programs", token, &programs)
	for _, p := range programs.Items {
		if p.Name == "Talk Show" {
			shows = append(shows, p)
		}
	}
	if len(shows) != 3 {
		t.Fatalf("guide programmes: %+v", programs.Items)
	}
	defaults := timerDefaults(t, s, token, shows[0].Id)
	if defaults["Name"] != "Talk Show" || defaults["ChannelId"] != *shows[0].ChannelId {
		t.Errorf("defaults for a guide programme: %v", defaults)
	}
	if status, body := s.call(http.MethodPost, "/LiveTv/Timers", app("tv", token), defaults); status != http.StatusNoContent {
		t.Fatalf("a timer for a guide programme: %d %s", status, body)
	}
	var detail BaseItemDto
	if s.get(t, "/LiveTv/Programs/"+shows[0].Id, token, &detail); detail.TimerId == "" || detail.EpisodeTitle != "Pilot" {
		t.Errorf("a scheduled guide programme: %+v", detail)
	}
	defaults["RecordAnyTime"] = false
	if status, body := s.call(http.MethodPost, "/LiveTv/SeriesTimers", app("tv", token), defaults); status != http.StatusNoContent {
		t.Fatalf("a series timer for a guide programme: %d %s", status, body)
	}
	var timers struct{ Items []TimerInfoDto }
	s.get(t, "/LiveTv/Timers", token, &timers)
	var scheduled []string
	for _, timer := range timers.Items {
		if timer.SeriesTimerId == "" {
			t.Errorf("a timer outside the series: %+v", timer)
		}
		scheduled = append(scheduled, timer.ProgramId)
	}
	// The one five hours later airs at another time of day.
	if !slices.Equal(scheduled, []string{shows[0].Id, shows[2].Id}) {
		t.Errorf("timers of the series: %v, programmes %s %s %s", scheduled, shows[0].Id, shows[1].Id, shows[2].Id)
	}
	s.get(t, "/LiveTv/Programs", token, &programs)
	for _, p := range programs.Items {
		if p.Id == shows[1].Id && (p.TimerId != "" || p.SeriesTimerId == "") || p.Id == shows[2].Id && p.TimerId == "" {
			t.Errorf("a guide programme of the series: %+v", p)
		}
	}
}
