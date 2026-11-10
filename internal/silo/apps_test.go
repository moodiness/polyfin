package silo

import (
	"encoding/json"
	"fmt"
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
var richVersion = versionWith()

// versionWith answers as richVersion does, with more streams after its
// own.
func versionWith(more ...any) func(http.ResponseWriter, call) {
	return func(w http.ResponseWriter, c call) { answerVersion(w, c, more) }
}

func answerVersion(w http.ResponseWriter, c call, more []any) {
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
	streams = append(streams, more...)
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

// The embedded rows of a plan's subtitles name the streams the watch
// detail gives their tracks: the Apple app finds a pick's row by them.
func TestInventoryNamesTheWatchDetailsStreams(t *testing.T) {
	t.Parallel()
	h := newHarness(t, richVersion)
	var detail struct {
		Versions []struct {
			Subtitles []struct {
				Index *int `json:"index"`
			} `json:"subtitle_tracks"`
		} `json:"versions"`
	}
	w := h.do(http.MethodGet, "/api/v2/watch/"+testVersion, nil)
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &detail) != nil || len(detail.Versions) != 1 {
		t.Fatalf("watch: %d %s", w.Code, w.Body)
	}
	var indexes []int
	for _, track := range detail.Versions[0].Subtitles {
		if track.Index != nil {
			indexes = append(indexes, *track.Index)
		}
	}
	for _, more := range []map[string]any{nil, {"subtitle_track_index": 2}} {
		w := h.startAnswer(more)
		if err := silotest.CheckSubtitleStreams(w.Body.Bytes(), indexes); w.Code != http.StatusCreated || err != nil {
			t.Errorf("a start asking %v: %d %v", more, w.Code, err)
		}
	}
}

// A file's own subtitle FFmpeg did not recognize is listed unplayable,
// and asking it is refused as Silo refuses it; a replan refused leaves
// the playback as it was.
func TestUnrecognizedSubtitlesAreRefused(t *testing.T) {
	t.Parallel()
	h := newHarness(t, versionWith(map[string]any{"Type": "Subtitle", "Index": 6, "Language": "jpn"}))
	file := h.file()
	refused := func(label string, w *httptest.ResponseRecorder, status int) {
		t.Helper()
		var d struct {
			Outcome  string `json:"outcome"`
			Terminal struct {
				Reason string `json:"reason"`
			} `json:"terminal"`
		}
		if w.Code != status || json.Unmarshal(w.Body.Bytes(), &d) != nil || d.Outcome != "adaptation_unavailable" ||
			d.Terminal.Reason != "subtitle_codec_unsupported" {
			t.Errorf("%s: %d %s", label, w.Code, w.Body)
		}
	}
	refused("a start asking it", h.startAnswer(map[string]any{"subtitle_track_index": 3}), http.StatusCreated)
	w := h.startAnswer(nil)
	if err := silotest.CheckDecision(w.Body.Bytes(), []string{file}); w.Code != http.StatusCreated || err != nil {
		t.Fatalf("a start: %d %v", w.Code, err)
	}
	var d struct {
		SessionID string `json:"session_id"`
		Plan      struct {
			Subtitle struct {
				Inventory []struct {
					Unplayable  bool `json:"unplayable"`
					StreamIndex *int `json:"stream_index"`
				} `json:"inventory"`
			} `json:"subtitle"`
		} `json:"playback_plan"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &d)
	if rows := d.Plan.Subtitle.Inventory; len(rows) != 4 || !rows[3].Unplayable || rows[3].StreamIndex == nil || *rows[3].StreamIndex != 5 ||
		rows[1].Unplayable || rows[2].Unplayable {
		t.Fatalf("inventory: %s", w.Body)
	}
	asked := len(h.jellyfin.called(http.MethodPost, "/Items/"+testVersion+"/PlaybackInfo"))
	refused("a replan asking it", h.replan(d.SessionID, "replan-1", map[string]any{
		"subtitle": map[string]any{"id": "file:" + file + ":subtitle:3", "index": 3}}), http.StatusOK)
	if now := len(h.jellyfin.called(http.MethodPost, "/Items/"+testVersion+"/PlaybackInfo")); now != asked {
		t.Errorf("the refused replan asked PlaybackInfo")
	}
	if w := h.replan(d.SessionID, "replan-2", map[string]any{}); w.Code != http.StatusOK || silotest.CheckDecision(w.Body.Bytes(), []string{file}) != nil {
		t.Errorf("the playback after the refusal: %d %s", w.Code, w.Body)
	}
}

// testLibrary is the Jellyfin view of browsing's library.
const testLibrary = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

// browsing answers the Jellyfin API for a library of five movies, and two
// episodes the user is watching: the first in a season with a poster of
// its own, the second's in one without.
func browsing(w http.ResponseWriter, c call) {
	movies := []any{}
	for n := range 5 {
		movies = append(movies, map[string]any{"Id": fmt.Sprintf("%032x", n+1), "Name": "Movie " + strconv.Itoa(n+1), "Type": "Movie"})
	}
	episode := func(id, season, parent string) map[string]any {
		return map[string]any{"Id": id, "Name": "Episode", "Type": "Episode", "SeriesId": "cccccccccccccccccccccccccccccccc",
			"SeriesName": "Show", "SeasonId": season, "SeriesPrimaryImageTag": "series-poster", "ParentPrimaryImageItemId": parent,
			"ParentPrimaryImageTag": "parent-poster", "ParentBackdropItemId": "cccccccccccccccccccccccccccccccc",
			"ParentBackdropImageTags": []string{"series-backdrop"}, "ImageTags": map[string]string{"Primary": "still"}}
	}
	switch c.path {
	case "/UserViews":
		jsonAnswer(w, map[string]any{"Items": []any{map[string]any{"Id": testLibrary, "Name": "Films", "CollectionType": "movies"}}})
	case "/Polyfin/HomeRows", "/Shows/NextUp":
		jsonAnswer(w, map[string]any{"Items": []any{}})
	case "/Users/Me":
		jsonAnswer(w, map[string]any{"Configuration": map[string]any{}})
	case "/Items/Latest":
		jsonAnswer(w, movies)
	case "/Items":
		jsonAnswer(w, map[string]any{"Items": movies, "TotalRecordCount": len(movies)})
	case "/UserItems/Resume":
		jsonAnswer(w, map[string]any{"Items": []any{
			episode("dddddddddddddddddddddddddddddddd", "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"),
			episode("ffffffffffffffffffffffffffffffff", "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", "cccccccccccccccccccccccccccccccc")}})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// A section read with item_limit lists at most that many cards a
// section, and tells, of one section, that it left cards out; Home's
// capabilities say the reads take it.
func TestSectionReadsTakeItemLimit(t *testing.T) {
	t.Parallel()
	h := newHarness(t, browsing)
	read := func(path string, into any) []byte {
		t.Helper()
		w := h.do(http.MethodGet, path, nil)
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), into) != nil {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
		return w.Body.Bytes()
	}
	var capabilities struct {
		Max int `json:"section_item_limit_max"`
	}
	if data := read("/api/v2/home/capabilities", &capabilities); silotest.CheckHomeCapabilities(data) != nil || capabilities.Max != 100 {
		t.Errorf("Home's capabilities: %s", data)
	}
	var libraries struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	read("/api/v2/user/libraries", &libraries)
	if len(libraries.Items) != 1 {
		t.Fatalf("libraries: %+v", libraries)
	}
	library := libraries.Items[0].ID
	type section struct {
		ID         string            `json:"id"`
		Items      []json.RawMessage `json:"items"`
		ItemLimit  int               `json:"item_limit"`
		TotalCount int               `json:"total_count"`
		HasMore    *bool             `json:"has_more"`
	}
	for _, path := range []string{"/api/v2/home/sections", "/api/v2/library/" + library + "/sections"} {
		var doc struct {
			Sections []section `json:"sections"`
		}
		data := read(path+"?item_limit=1", &doc)
		if err := silotest.CheckSections(data, 1); err != nil || len(doc.Sections) != 2 {
			t.Errorf("%s: %v %s", path, err, data)
		}
		for _, sec := range doc.Sections {
			if want := map[string]int{"continue_watching": 2, "resume-" + library: 2}[sec.ID]; want == 0 && sec.TotalCount != 5 ||
				want != 0 && sec.TotalCount != want || sec.ItemLimit != 1 || sec.HasMore != nil {
				t.Errorf("%s: section %s cut to %d of %d, item_limit %d, has_more %v", path, sec.ID, len(sec.Items), sec.TotalCount,
					sec.ItemLimit, sec.HasMore)
			}
		}
	}
	for _, tc := range []struct {
		query string
		cards int
		more  bool
	}{{"?item_limit=2", 2, true}, {"?item_limit=5", 5, false}, {"", 5, false}} {
		path := "/api/v2/home/sections/latest-" + library + "/items" + tc.query
		var sec section
		data := read(path, &sec)
		limit, _ := strconv.Atoi(strings.TrimPrefix(tc.query, "?item_limit="))
		if err := silotest.CheckSection(data, "latest-"+library, limit); err != nil || len(sec.Items) != tc.cards ||
			(sec.HasMore != nil) != tc.more || sec.TotalCount != 5 {
			t.Errorf("%s: %v %s", path, err, data)
		}
	}
	for _, limit := range []string{"0", "101", "some"} {
		if w := h.do(http.MethodGet, "/api/v2/home/sections?item_limit="+limit, nil); w.Code != http.StatusUnprocessableEntity {
			t.Errorf("item_limit %s: %d %s", limit, w.Code, w.Body)
		}
	}
}

// An episode's card carries its series' poster and backdrop, and its
// season's poster when the season has one: the apps draw them in place of
// the episode's still.
func TestEpisodeCardsCarryTheirParentsArtwork(t *testing.T) {
	t.Parallel()
	h := newHarness(t, browsing)
	var images struct {
		EpisodeParentArtwork bool `json:"episode_parent_artwork"`
	}
	w := h.do(http.MethodGet, "/api/v2/images/capabilities", nil)
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &images) != nil || !images.EpisodeParentArtwork {
		t.Errorf("images: %d %s", w.Code, w.Body)
	}
	w = h.do(http.MethodGet, "/api/v2/home/sections/continue_watching/items?poster_width=300", nil)
	var sec struct {
		Items []struct {
			SeriesPoster   string `json:"series_poster_url"`
			SeriesBackdrop string `json:"series_backdrop_url"`
			SeasonPoster   string `json:"season_poster_url"`
		} `json:"items"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &sec) != nil || len(sec.Items) != 2 {
		t.Fatalf("Continue Watching: %d %s", w.Code, w.Body)
	}
	if err := silotest.CheckSection(w.Body.Bytes(), "continue_watching", 0); err != nil {
		t.Error(err)
	}
	series, season := "/api/v2/artwork/cccccccccccccccccccccccccccccccc/", "/api/v2/artwork/eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee/"
	for n, want := range [][3]string{
		{series + "Primary?tag=series-poster&width=300", series + "Backdrop?tag=series-backdrop", season + "Primary?tag=parent-poster&width=300"},
		{series + "Primary?tag=series-poster&width=300", series + "Backdrop?tag=series-backdrop", ""},
	} {
		if got := [3]string{sec.Items[n].SeriesPoster, sec.Items[n].SeriesBackdrop, sec.Items[n].SeasonPoster}; got != want {
			t.Errorf("episode %d: %q, want %q", n, got, want)
		}
	}
}

// The documents the apps read of features Polyfin does not offer hold the
// keys the contract requires: no reset of an Apple push installation, no
// Discover rows of requests, no box sets in a library or a search, and
// the members the contract added since, all off or empty.
func TestDocumentsSayWhatPolyfinDoesNotOffer(t *testing.T) {
	t.Parallel()
	h := newHarness(t, browsing)
	for _, tc := range []struct{ path, key string }{{"/api/v2/devices/push/apple/capabilities", "installation_reset_available"},
		{"/api/v2/requests/status", "discover_rows_supported"}, {"/api/v2/catalog/search/capabilities", "box_sets"},
		{"/api/v2/capabilities/downloads", "batch_version"}, {"/api/v2/diagnostics/capabilities", "accepted_crash_sources"},
		{"/api/v2/requests/status", "remove_failed_supported"},
		{"/api/v2/notifications/push/devices/capabilities", "binds_login_session"},
		{"/api/v2/settings/contract/capabilities", "supports_overlay_images"}, {"/api/v2/settings/overlay-config", "images"},
		{"/api/v2/settings/overlay-config", "image_uploads_available"}} {
		var doc map[string]any
		w := h.do(http.MethodGet, tc.path, nil)
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &doc) != nil {
			t.Errorf("%s: %d %s", tc.path, w.Code, w.Body)
			continue
		}
		if list, isList := doc[tc.key].([]any); doc[tc.key] != false && (!isList || len(list) != 0) {
			t.Errorf("%s: %s is %v", tc.path, tc.key, doc[tc.key])
		}
	}
	w := h.do(http.MethodGet, "/api/v2/user/libraries", nil)
	var libraries struct {
		Items []map[string]any `json:"items"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &libraries) != nil || len(libraries.Items) != 1 ||
		libraries.Items[0]["box_sets_enabled"] != false || silotest.CheckLibraries(w.Body.Bytes()) != nil {
		t.Errorf("libraries: %d %s", w.Code, w.Body)
	}
}
