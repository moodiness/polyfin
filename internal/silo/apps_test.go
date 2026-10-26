package silo

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/silo/silotest"
)

// richVersion answers the Jellyfin API for a version whose streams are
// numbered as Polyfin numbers them: a subtitle file beside it first, then
// the file's video, two audio tracks, a picture subtitle and a text one.
// It plays as it is unless conversion is asked: a picture subtitle burned
// in, the file failed to play, or a bitrate below the file's. The user
// prefers French subtitles, always.
func richVersion(w http.ResponseWriter, c call) {
	streams := []any{
		map[string]any{"Type": "Subtitle", "Index": 0, "Codec": "srt", "Language": "fre", "IsExternal": true, "IsTextSubtitleStream": true,
			"DeliveryMethod": "External", "DeliveryUrl": "/Videos/" + testVersion + "/" + testVersion + "/Subtitles/0/0/Stream.vtt?ApiKey=" + testToken},
		map[string]any{"Type": "Video", "Index": 1, "Codec": "h264", "Width": 3840, "Height": 2160},
		map[string]any{"Type": "Audio", "Index": 2, "Codec": "aac", "Language": "eng", "Channels": 2},
		map[string]any{"Type": "Audio", "Index": 3, "Codec": "aac", "Language": "fre", "Channels": 2},
		map[string]any{"Type": "Subtitle", "Index": 4, "Codec": "PGSSUB", "Language": "eng"},
		map[string]any{"Type": "Subtitle", "Index": 5, "Codec": "subrip", "Language": "eng", "IsTextSubtitleStream": true,
			"DeliveryMethod": "External", "DeliveryUrl": "/Videos/" + testVersion + "/" + testVersion + "/Subtitles/5/0/Stream.vtt?ApiKey=" + testToken},
	}
	source := map[string]any{"Id": testVersion, "Container": "mp4", "RunTimeTicks": int64(7200) * 10_000_000, "Bitrate": 20_000_000,
		"DefaultAudioStreamIndex": 3, "DefaultSubtitleStreamIndex": 5, "MediaStreams": streams}
	switch {
	case strings.HasSuffix(c.path, "/PlaybackInfo"):
		bitrate, _ := c.body["MaxStreamingBitrate"].(float64)
		if c.body["EnableDirectPlay"] == false || c.body["SubtitleStreamIndex"] == float64(4) || bitrate > 0 && bitrate < 20_000_000 {
			source["TranscodingUrl"] = "/videos/" + testVersion + "/master.m3u8?PlaySessionId=play-1&VideoCodec=h264&AudioCodec=aac&ApiKey=" + testToken
			source["TranscodingContainer"] = "mp4"
		} else {
			source["SupportsDirectPlay"] = true
		}
		jsonAnswer(w, map[string]any{"PlaySessionId": "play-" + strconv.Itoa(int(time.Now().UnixNano())), "MediaSources": []any{source}})
	case c.path == "/Items/"+testVersion:
		jsonAnswer(w, map[string]any{"Id": testVersion, "Name": "Movie", "Type": "Movie", "MediaSources": []any{source}})
	case c.path == "/Users/Me":
		jsonAnswer(w, map[string]any{"Configuration": map[string]any{"SubtitleMode": "Always", "SubtitleLanguagePreference": "fre"}})
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// lastPlaybackInfo is the last PlaybackInfo the API asked.
func (h *harness) lastPlaybackInfo() call {
	h.t.Helper()
	asked := h.jellyfin.called(http.MethodPost, "/Items/"+testVersion+"/PlaybackInfo")
	if len(asked) == 0 {
		h.t.Fatal("PlaybackInfo was not asked")
	}
	return asked[len(asked)-1]
}

// The capabilities document meets the rules the apps check before they
// start a playback.
func TestCapabilitiesMeetTheAppsRules(t *testing.T) {
	t.Parallel()
	h := newHarness(t, playable)
	w := h.do(http.MethodGet, "/api/v2/playback/capabilities", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("capabilities: %d %s", w.Code, w.Body)
	}
	if err := silotest.CheckCapabilities(w.Body.Bytes()); err != nil {
		t.Error(err)
	}
}

// Every decision meets the rules the apps check before they play it: a
// start's played as it is, with a subtitle beside it, the replans' of
// each kind, and a refusal; each replan's plan attempt key is new.
func TestDecisionsMeetTheAppsRules(t *testing.T) {
	t.Parallel()
	h := newHarness(t, richVersion)
	file := h.file()
	attempted := map[string]bool{}
	check := func(label string, w *httptest.ResponseRecorder, status int) map[string]any {
		t.Helper()
		if w.Code != status {
			t.Fatalf("%s: %d %s", label, w.Code, w.Body)
		}
		if err := silotest.CheckDecision(w.Body.Bytes(), []string{file}); err != nil {
			t.Errorf("%s: %v", label, err)
		}
		var d map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &d)
		plan, _ := d["playback_plan"].(map[string]any)
		key, _ := plan["plan_attempt_key"].(string)
		if attempted[key] {
			t.Errorf("%s: plan attempt key %s again", label, key)
		}
		attempted[key] = true
		return plan
	}
	if plan := check("a start played as it is", h.startAnswer(nil), http.StatusCreated); plan["delivery"] != "original_http" {
		t.Errorf("played as %v", plan["delivery"])
	}
	w := h.startAnswer(map[string]any{"subtitle_track_index": 0})
	plan := check("a start with a subtitle beside the video", w, http.StatusCreated)
	if subtitle, _ := plan["subtitle"].(map[string]any); subtitle["mode"] != "render" {
		t.Errorf("the subtitle: %v", plan["subtitle"])
	}
	session, _ := plan["session_id"].(string)
	plan = check("a replan burning a picture subtitle in", h.replan(session, "replan-1", map[string]any{
		"subtitle": map[string]any{"id": "file:" + file + ":subtitle:1", "index": 1}}), http.StatusOK)
	if subtitle, _ := plan["subtitle"].(map[string]any); plan["delivery"] != "server_transcode_hls" || subtitle["mode"] != "burn_in" {
		t.Errorf("burned in: %v, subtitle %v", plan["delivery"], plan["subtitle"])
	}
	check("a replan after a failure", h.replanWith(session, "replan-2", map[string]any{"operation": "failure_recovery"}), http.StatusOK)
	check("a replan to a lower quality", h.replanWith(session, "replan-3", map[string]any{"operation": "quality_change",
		"quality_preference": "720p"}), http.StatusOK)
	refused := newHarness(t, func(w http.ResponseWriter, c call) {
		if strings.HasSuffix(c.path, "/PlaybackInfo") {
			jsonAnswer(w, map[string]any{"PlaySessionId": "play-nothing", "MediaSources": []any{}, "ErrorCode": "NoCompatibleStream"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	w = refused.startAnswer(nil)
	if err := silotest.CheckDecision(w.Body.Bytes(), nil); w.Code != http.StatusCreated || err != nil {
		t.Errorf("a refusal: %d %v", w.Code, err)
	}
}

// The apps ask tracks by their place, and name them by identifiers of
// their own: among the subtitles, the files beside the video come first.
func TestTracksAreThoseTheAppsCount(t *testing.T) {
	t.Parallel()
	h := newHarness(t, richVersion)
	file := h.file()
	session, d := h.start(map[string]any{"audio_track_id": "file:" + file + ":audio:0", "audio_track_index": 0, "subtitle_track_index": 0})
	if asked := h.lastPlaybackInfo(); asked.body["AudioStreamIndex"] != float64(2) || asked.body["SubtitleStreamIndex"] != float64(0) {
		t.Errorf("asked audio %v, subtitle %v", asked.body["AudioStreamIndex"], asked.body["SubtitleStreamIndex"])
	}
	plan, _ := d["playback_plan"].(map[string]any)
	selected, _ := plan["selected_tracks"].(map[string]any)
	audio, _ := selected["audio"].(map[string]any)
	subtitle, _ := selected["subtitle"].(map[string]any)
	if audio["id"] != "file:"+file+":audio:0" || subtitle["id"] != "file:"+file+":subtitle:0" || subtitle["index"] != 0.0 {
		t.Errorf("selected %v", selected)
	}
	decision, _ := plan["subtitle"].(map[string]any)
	inventory, _ := decision["inventory"].([]any)
	var listed []string
	for _, entry := range inventory {
		row, _ := entry.(map[string]any)
		listed = append(listed, row["track_id"].(string)+" "+row["source"].(string)+" "+row["language"].(string))
	}
	if want := []string{"file:" + file + ":subtitle:0 external fr", "file:" + file + ":subtitle:1 embedded en",
		"file:" + file + ":subtitle:2 embedded en"}; !slices.Equal(listed, want) {
		t.Errorf("inventory %q, want %q", listed, want)
	}
	if w := h.replan(session, "replan-1", map[string]any{"audio": map[string]any{"id": "file:" + file + ":audio:1", "index": 1},
		"subtitle": map[string]any{"id": "file:" + file + ":subtitle:2"}}); w.Code != http.StatusOK {
		t.Fatalf("replan: %d %s", w.Code, w.Body)
	}
	if asked := h.lastPlaybackInfo(); asked.body["AudioStreamIndex"] != float64(3) || asked.body["SubtitleStreamIndex"] != float64(5) {
		t.Errorf("the replan asked audio %v, subtitle %v", asked.body["AudioStreamIndex"], asked.body["SubtitleStreamIndex"])
	}
}

// A start that asks no subtitle shows none, as the apps mean it: they
// choose the subtitle from the user's preferences themselves.
func TestAStartWithoutASubtitleShowsNone(t *testing.T) {
	t.Parallel()
	h := newHarness(t, richVersion)
	_, d := h.start(nil)
	if asked := h.lastPlaybackInfo(); asked.body["SubtitleStreamIndex"] != float64(-1) {
		t.Errorf("asked subtitle %v", asked.body["SubtitleStreamIndex"])
	}
	plan, _ := d["playback_plan"].(map[string]any)
	selected, _ := plan["selected_tracks"].(map[string]any)
	if subtitle, _ := plan["subtitle"].(map[string]any); subtitle["mode"] != "off" || selected["subtitle"] != nil {
		t.Errorf("subtitle %v, selected %v", plan["subtitle"], selected["subtitle"])
	}
}

// A quality the plans offer keeps to its bitrate as well as its height.
func TestQualityRungsKeepToTheirBitrate(t *testing.T) {
	t.Parallel()
	h := newHarness(t, richVersion)
	_, d := h.start(map[string]any{"quality_preference": "720p"})
	if asked := h.lastPlaybackInfo(); asked.body["MaxStreamingBitrate"] != float64(4_000_000) {
		t.Errorf("asked bitrate %v", asked.body["MaxStreamingBitrate"])
	}
	plan, _ := d["playback_plan"].(map[string]any)
	if plan["delivery"] != "server_transcode_hls" || plan["active_quality"] != "720p" {
		t.Errorf("played %v as %v", plan["delivery"], plan["active_quality"])
	}
}

// The watch detail tells the tracks as the apps count them, the audio a
// playback plays unless they ask another, and the user's subtitle
// preferences, which the apps choose a start's subtitle by.
func TestWatchDetailTellsTracksAndPreferences(t *testing.T) {
	t.Parallel()
	h := newHarness(t, richVersion)
	w := h.do(http.MethodGet, "/api/v2/watch/"+testVersion, nil)
	var detail struct {
		Versions []struct {
			EffectiveAudio *int `json:"effective_audio_track_index"`
			Subtitles      []struct {
				Index    *int   `json:"index"`
				External bool   `json:"external"`
				Language string `json:"language"`
			} `json:"subtitle_tracks"`
		} `json:"versions"`
		Language string `json:"effective_subtitle_language"`
		Mode     string `json:"effective_subtitle_mode"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &detail) != nil || len(detail.Versions) != 1 {
		t.Fatalf("watch: %d %s", w.Code, w.Body)
	}
	version := detail.Versions[0]
	var tracks []string
	for _, s := range version.Subtitles {
		index := "file"
		if s.Index != nil {
			index = strconv.Itoa(*s.Index)
		}
		tracks = append(tracks, index+" "+strconv.FormatBool(s.External)+" "+s.Language)
	}
	if want := []string{"file true fr", "3 false en", "4 false en"}; !slices.Equal(tracks, want) {
		t.Errorf("subtitles %q, want %q", tracks, want)
	}
	if version.EffectiveAudio == nil || *version.EffectiveAudio != 1 {
		t.Errorf("effective audio %v", version.EffectiveAudio)
	}
	if detail.Language != "fr" || detail.Mode != "always" {
		t.Errorf("subtitle preferences %q %q", detail.Language, detail.Mode)
	}
}

// A stop of a stopped playback is a replay of the stop that ended it.
func TestAStopOfAStoppedPlaybackIsAReplay(t *testing.T) {
	t.Parallel()
	h := newHarness(t, playable)
	session, _ := h.start(nil)
	installation := h.accounts.device.ID.String()
	for _, stop := range []string{"stop-1", "stop-2"} {
		w := h.do(http.MethodDelete, "/api/v2/playback/"+session, map[string]any{"installation_id": installation, "stop_id": stop})
		var answer struct {
			Outcome string `json:"outcome"`
			StopID  string `json:"stop_id"`
		}
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &answer) != nil || answer.StopID != "stop-1" {
			t.Errorf("%s: %d %s", stop, w.Code, w.Body)
		}
	}
	if stopped := h.jellyfin.called(http.MethodPost, "/Sessions/Playing/Stopped"); len(stopped) != 1 {
		t.Errorf("stopped %d times", len(stopped))
	}
}

// The apps' diagnostics of their playbacks are taken; the failures they
// tell are logged.
func TestRouteEventsAreTaken(t *testing.T) {
	t.Parallel()
	h := newHarness(t, playable)
	event := func(id, kind, failure string) {
		t.Helper()
		body := map[string]any{"event_id": id, "event": kind, "installation_id": h.accounts.device.ID.String(),
			"playback_attempt_id": "attempt-1", "protocol_version": 3, "diagnostics": map[string]any{"decoder": "x"}}
		if failure != "" {
			body["failure_classification"] = failure
		}
		w := h.do(http.MethodPost, "/api/v2/playback/route-events", body)
		var receipt struct {
			EventID string `json:"event_id"`
			Outcome string `json:"outcome"`
		}
		if w.Code != http.StatusAccepted || json.Unmarshal(w.Body.Bytes(), &receipt) != nil || receipt.EventID != id ||
			receipt.Outcome != "accepted" {
			t.Errorf("%s: %d %s", id, w.Code, w.Body)
		}
	}
	event("event-1", "first_frame", "")
	event("event-2", "plan_failed", "decoder_unsupported")
	if log := h.log.String(); !strings.Contains(log, "failure=decoder_unsupported") || strings.Contains(log, "first_frame") {
		t.Errorf("log: %s", log)
	}
}

// The operations the apps ask that Polyfin does not serve are logged,
// their method and path without identifiers nor query, each once an
// hour.
func TestUnknownOperationsAreLogged(t *testing.T) {
	t.Parallel()
	h := newHarness(t, playable)
	ask := func() {
		t.Helper()
		if w := h.do(http.MethodGet, "/api/v2/notifications/123?token=secret", nil); w.Code != http.StatusNotFound {
			t.Errorf("%d %s", w.Code, w.Body)
		}
	}
	logged := func() int { return strings.Count(h.log.String(), "does not serve") }
	ask()
	ask()
	if log := h.log.String(); logged() != 1 || !strings.Contains(log, "method=GET path=/api/v2/notifications/{id}") ||
		strings.Contains(log, "secret") {
		t.Errorf("log: %s", log)
	}
	*h.clock = h.clock.Add(time.Hour)
	ask()
	if logged() != 2 {
		t.Errorf("an hour later: %s", h.log.String())
	}
}
