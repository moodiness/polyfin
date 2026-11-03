package silo

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/silo/silotest"
)

// renditionFeatures are the features an app sends to have every embedded
// text track as a rendition of a converted or repackaged stream.
var renditionFeatures = []string{subtitleRenditions, repackagedRenditions, allSubtitleRenditions}

// renditionVersion answers as richVersion does, with an English ASS track
// after the file's SRT one, and the playlists and segments of its
// conversion: the file's streams are numbered 0 to 5 by FFmpeg, its
// subtitle tracks 2 (the picture one), 3 and 4 in their container.
func renditionVersion(t *testing.T) *harness {
	t.Helper()
	more := map[string]any{"Type": "Subtitle", "Index": 6, "Codec": "ass", "Language": "eng", "Title": "English \"SDH\"",
		"IsTextSubtitleStream": true, "DeliveryMethod": "External",
		"DeliveryUrl": "/Videos/" + testVersion + "/" + testVersion + "/Subtitles/6/0/Stream.vtt?ApiKey=" + testToken}
	h := newHarness(t, func(w http.ResponseWriter, c call) {
		w.Header().Set("Cache-Control", "no-cache")
		switch {
		case strings.HasSuffix(c.path, "/master.m3u8"):
			_, _ = io.WriteString(w, "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-INDEPENDENT-SEGMENTS\n"+
				"#EXT-X-STREAM-INF:BANDWIDTH=4500000,CODECS=\"avc1.64001f,mp4a.40.2\",RESOLUTION=1280x720\nmain.m3u8?PlaySessionId=play-1\n"+
				"#EXT-X-STREAM-INF:BANDWIDTH=1600000,CODECS=\"avc1.64001e,mp4a.40.2\",RESOLUTION=854x480\nhls1/main1/main.m3u8?PlaySessionId=play-1\n")
		case strings.Contains(c.path, "/hls1/subtitles") && strings.HasSuffix(c.path, "/main.m3u8"):
			_, _ = io.WriteString(w, "#EXTM3U\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:6\n#EXT-X-MEDIA-SEQUENCE:0\n"+
				"#EXTINF:6.000000,\n0.vtt?PlaySessionId=play-1\n#EXTINF:6.000000,\n1.vtt?PlaySessionId=play-1\n#EXT-X-ENDLIST\n")
		case strings.HasSuffix(c.path, "/main.m3u8"):
			_, _ = io.WriteString(w, "#EXTM3U\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:6\n#EXT-X-MEDIA-SEQUENCE:0\n"+
				"#EXT-X-MAP:URI=\"hls1/main/-1.mp4?PlaySessionId=play-1\"\n#EXTINF:6.000000,\nhls1/main/0.mp4?PlaySessionId=play-1\n"+
				"#EXTINF:6.000000,\nhls1/main/1.mp4?PlaySessionId=play-1\n#EXT-X-ENDLIST\n")
		case strings.HasSuffix(c.path, ".vtt"):
			w.Header().Set("Content-Type", "text/vtt")
			_, _ = io.WriteString(w, "WEBVTT\nX-TIMESTAMP-MAP=MPEGTS:900000,LOCAL:00:00:00.000\n\n00:00:01.000 --> 00:00:02.000\n"+c.path+"\n")
		case strings.HasSuffix(c.path, ".mp4"):
			_, _ = io.WriteString(w, "segment "+c.path)
		default:
			answerVersion(w, c, []any{more})
		}
	})
	h.server.options.ContainerTracks = func(_ context.Context, playSession string) map[int]uint64 {
		if !strings.HasPrefix(playSession, "play-") {
			return nil
		}
		return map[int]uint64{0: 1, 1: 2, 2: 3, 3: 4, 4: 5, 5: 6}
	}
	return h
}

// get fetches media of a session, as the apps' players do.
func (h *harness) get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	return h.do(http.MethodGet, path, nil)
}

// asked is the last call the fake Jellyfin API received at path, with the
// play session it named.
func (h *harness) asked(t *testing.T, path string) call {
	t.Helper()
	calls := h.jellyfin.called(http.MethodGet, path)
	if len(calls) == 0 {
		t.Fatalf("%s was not asked", path)
	}
	return calls[len(calls)-1]
}

type renditionDecision struct {
	SessionID string `json:"session_id"`
	Plan      struct {
		Delivery  string `json:"delivery"`
		ExpiresAt string `json:"expires_at"`
		Stream    struct {
			URL string `json:"url"`
		} `json:"stream"`
		Subtitle struct {
			Mode         string        `json:"mode"`
			Artifact     *struct{}     `json:"artifact"`
			HLSRendition *hlsRendition `json:"hls_rendition"`
			Inventory    []struct {
				ContainerTrackID string              `json:"container_track_id"`
				HLSRendition     *inventoryRendition `json:"hls_rendition"`
				Native           *bool               `json:"native_selectable"`
			} `json:"inventory"`
		} `json:"subtitle"`
	} `json:"playback_plan"`
}

// decided checks a decision against the apps' rules, and reads it.
func decided(t *testing.T, label string, w *httptest.ResponseRecorder, status int, file string) renditionDecision {
	t.Helper()
	var d renditionDecision
	if w.Code != status || json.Unmarshal(w.Body.Bytes(), &d) != nil || d.Plan.Delivery == "" {
		t.Fatalf("%s: %d %s", label, w.Code, w.Body)
	}
	if err := silotest.CheckDecision(w.Body.Bytes(), []string{file}); err != nil {
		t.Errorf("%s: %v", label, err)
	}
	return d
}

// An app that opts in gets the embedded text tracks of a converted
// version as renditions of its HLS stream, named on their rows; the master
// playlist lists them under those names, by their container numbers, and
// its playlists and segments are those of the conversion, under the
// addresses the apps fetch. Switching among them, or turning them off,
// keeps the stream; another subtitle, or a player that could not offer
// them, gets files.
func TestSubtitlesAreRenditionsOfTheStream(t *testing.T) {
	t.Parallel()
	h := renditionVersion(t)
	file := h.file()
	w := h.startAnswer(map[string]any{"client_features": renditionFeatures, "quality_preference": "720p", "subtitle_track_index": 2})
	d := decided(t, "the start", w, http.StatusCreated, file)
	rows := d.Plan.Subtitle.Inventory
	if d.Plan.Delivery != converted || d.Plan.Subtitle.Mode != "render" || d.Plan.Subtitle.Artifact != nil || len(rows) != 4 {
		t.Fatalf("plan: %s", w.Body)
	}
	if got := *d.Plan.Subtitle.HLSRendition; got != (hlsRendition{GroupID: "subs", Name: "en", Format: "vtt", MimeType: "text/vtt", Index: 1}) {
		t.Errorf("the selected rendition: %+v", got)
	}
	// The file beside the video and the picture track keep their file or
	// burn-in; every embedded row names its container track.
	if rows[0].HLSRendition != nil || rows[0].ContainerTrackID != "" || rows[1].HLSRendition != nil || rows[1].ContainerTrackID != "4" ||
		rows[2].ContainerTrackID != "5" || rows[3].ContainerTrackID != "6" {
		t.Errorf("inventory: %s", w.Body)
	}
	if *rows[2].HLSRendition != (inventoryRendition{GroupID: "subs", Name: "en", Index: 1}) ||
		*rows[3].HLSRendition != (inventoryRendition{GroupID: "subs", Name: "English 'SDH'", Index: 2}) {
		t.Errorf("renditions: %+v %+v", rows[2].HLSRendition, rows[3].HLSRendition)
	}
	for _, row := range rows {
		if row.Native != nil {
			t.Errorf("native_selectable %v", *row.Native)
		}
	}
	base := d.Plan.Stream.URL[:strings.LastIndex(d.Plan.Stream.URL, "/")+1]

	master := h.get(t, d.Plan.Stream.URL)
	if master.Code != http.StatusOK || master.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("master playlist: %d %s", master.Code, master.Body)
	}
	if err := silotest.CheckMultivariant(master.Body.Bytes(), w.Body.Bytes()); err != nil {
		t.Errorf("master playlist: %v\n%s", err, master.Body)
	}
	want := "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-INDEPENDENT-SEGMENTS\n" +
		"#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID=\"subs\",NAME=\"en\",LANGUAGE=\"en\",DEFAULT=YES,AUTOSELECT=YES,FORCED=NO,URI=\"master.m3u8?silo_rendition=subtitles&silo_track=5\"\n" +
		"#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID=\"subs\",NAME=\"English 'SDH'\",LANGUAGE=\"en\",DEFAULT=NO,AUTOSELECT=YES,FORCED=NO,URI=\"master.m3u8?silo_rendition=subtitles&silo_track=6\"\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=4500000,CODECS=\"avc1.64001f,mp4a.40.2\",RESOLUTION=1280x720,SUBTITLES=\"subs\"\nmaster.m3u8?silo_rendition=video\n"
	if master.Body.String() != want {
		t.Errorf("master playlist:\n%s\nwant\n%s", master.Body, want)
	}
	if c := h.asked(t, "/videos/"+testVersion+"/master.m3u8"); c.query.Get("PlaySessionId") != "play-1" {
		t.Errorf("the conversion's master playlist was asked as %v", c.query)
	}

	video := h.get(t, base+"master.m3u8?silo_rendition=video")
	if err := silotest.CheckMediaPlaylist(video.Body.Bytes(), d.Plan.Stream.URL); video.Code != http.StatusOK || err != nil ||
		!strings.Contains(video.Body.String(), "#EXT-X-MAP:URI=\"segment/-1.mp4\"\n") || !strings.Contains(video.Body.String(), "\nsegment/1.mp4\n") {
		t.Errorf("video playlist: %d %v\n%s", video.Code, err, video.Body)
	}
	subtitles := h.get(t, base+"master.m3u8?silo_rendition=subtitles&silo_track=6")
	if err := silotest.CheckMediaPlaylist(subtitles.Body.Bytes(), d.Plan.Stream.URL); subtitles.Code != http.StatusOK || err != nil ||
		!strings.Contains(subtitles.Body.String(), "\nsegment/sub_6_0.vtt\n#EXTINF:6.000000,\nsegment/sub_6_1.vtt\n") {
		t.Errorf("subtitle playlist: %d %v\n%s", subtitles.Code, err, subtitles.Body)
	}
	h.asked(t, "/videos/"+testVersion+"/hls1/subtitles6/main.m3u8")
	segment := h.get(t, base+"segment/sub_6_1.vtt")
	if err := silotest.CheckSubtitleSegment(segment.Body.Bytes()); segment.Code != http.StatusOK || err != nil ||
		!strings.Contains(segment.Body.String(), "/hls1/subtitles6/1.vtt") {
		t.Errorf("subtitle segment: %d %v\n%s", segment.Code, err, segment.Body)
	}
	if c := h.asked(t, "/videos/"+testVersion+"/hls1/subtitles6/1.vtt"); c.query.Get("PlaySessionId") != "play-1" {
		t.Errorf("the subtitle segment was asked as %v", c.query)
	}
	if got := h.get(t, base+"segment/0.mp4"); got.Code != http.StatusOK || got.Body.String() != "segment /videos/"+testVersion+"/hls1/main/0.mp4" {
		t.Errorf("video segment: %d %s", got.Code, got.Body)
	}
	for _, path := range []string{"segment/sub_9_0.vtt", "segment/sub_0.vtt", "master.m3u8?silo_rendition=subtitles&silo_track=9",
		"master.m3u8?silo_rendition=subtitles", "master.m3u8?silo_rendition=audio", "main.m3u8?PlaySessionId=play-1", "hls1/main/0.mp4"} {
		if got := h.get(t, base+path); got.Code != http.StatusNotFound {
			t.Errorf("%s: %d", path, got.Code)
		}
	}

	// Another rendition, then none, at the quality playing: the stream
	// plays on.
	pick := func(id string, tracks map[string]any) *httptest.ResponseRecorder {
		return h.replanWith(d.SessionID, id, map[string]any{"selected_tracks": tracks, "quality_preference": "720p"})
	}
	infos := len(h.jellyfin.called(http.MethodPost, "/Items/"+testVersion+"/PlaybackInfo"))
	stops := len(h.jellyfin.called(http.MethodDelete, "/Videos/ActiveEncodings"))
	switched := decided(t, "a switch to the other rendition", pick("replan-1", map[string]any{
		"subtitle": map[string]any{"id": "file:" + file + ":subtitle:3", "index": 3}}), http.StatusOK, file)
	if switched.Plan.Stream.URL != d.Plan.Stream.URL || switched.Plan.ExpiresAt == "" || switched.Plan.ExpiresAt != d.Plan.ExpiresAt ||
		switched.Plan.Subtitle.HLSRendition == nil || switched.Plan.Subtitle.HLSRendition.Index != 2 {
		t.Errorf("the switch: %+v", switched.Plan)
	}
	off := decided(t, "subtitles turned off", pick("replan-2", map[string]any{}), http.StatusOK, file)
	if off.Plan.Subtitle.Mode != "off" || off.Plan.Subtitle.HLSRendition != nil || off.Plan.Subtitle.Inventory[2].HLSRendition == nil ||
		off.Plan.ExpiresAt != d.Plan.ExpiresAt {
		t.Errorf("off: %+v", off.Plan.Subtitle)
	}
	if now := len(h.jellyfin.called(http.MethodPost, "/Items/"+testVersion+"/PlaybackInfo")); now != infos {
		t.Errorf("switching renditions asked PlaybackInfo again")
	}
	if stopped := h.jellyfin.called(http.MethodDelete, "/Videos/ActiveEncodings"); len(stopped) != stops {
		t.Errorf("switching renditions stopped the conversion")
	}
	if master := h.get(t, d.Plan.Stream.URL); strings.Contains(master.Body.String(), "DEFAULT=YES") {
		t.Errorf("with subtitles off, a rendition is shown by default:\n%s", master.Body)
	}

	// The file beside the video is a file.
	sidecar := decided(t, "the file beside the video", pick("replan-3", map[string]any{
		"subtitle": map[string]any{"id": "file:" + file + ":subtitle:0", "index": 0}}), http.StatusOK, file)
	if sidecar.Plan.Subtitle.Artifact == nil || sidecar.Plan.Subtitle.HLSRendition != nil ||
		len(h.jellyfin.called(http.MethodPost, "/Items/"+testVersion+"/PlaybackInfo")) == infos {
		t.Errorf("the file beside the video: %+v", sidecar.Plan.Subtitle)
	}
	// A player that could not offer the renditions gets files from now on.
	failed := decided(t, "renditions failed", h.replanWith(d.SessionID, "replan-4", map[string]any{"operation": "failure_recovery",
		"quality_preference": "720p",
		"failure":            map[string]any{"classification": renditionFailed},
		"selected_tracks":    map[string]any{"subtitle": map[string]any{"id": "file:" + file + ":subtitle:2", "index": 2}}}), http.StatusOK, file)
	if failed.Plan.Subtitle.Artifact == nil || failed.Plan.Subtitle.HLSRendition != nil || failed.Plan.Subtitle.Inventory[2].HLSRendition != nil {
		t.Errorf("after the renditions failed: %+v", failed.Plan.Subtitle)
	}
	if video := h.get(t, d.Plan.Stream.URL); silotest.CheckMediaPlaylist(video.Body.Bytes(), d.Plan.Stream.URL) != nil {
		t.Errorf("the master playlist without renditions is not the video playlist:\n%s", video.Body)
	}
}

// An app that takes only the selected track as a rendition gets it alone,
// unnamed on the rows; one that takes none gets files, and the video
// playlist as the master playlist; a subtitle burned in, or a file played
// as it is, has no rendition. Every file's own track names its container
// track.
func TestRenditionsAsTheAppNegotiated(t *testing.T) {
	t.Parallel()
	h := renditionVersion(t)
	file := h.file()
	w := h.startAnswer(map[string]any{"client_features": []string{subtitleRenditions}, "quality_preference": "720p", "subtitle_track_index": 3})
	d := decided(t, "the selected track alone", w, http.StatusCreated, file)
	if got := d.Plan.Subtitle.HLSRendition; got == nil || *got != (hlsRendition{GroupID: "subs", Name: "Subtitles", Format: "vtt", MimeType: "text/vtt"}) ||
		d.Plan.Subtitle.Artifact != nil || d.Plan.Subtitle.Inventory[3].HLSRendition != nil {
		t.Fatalf("the single rendition: %s", w.Body)
	}
	base := d.Plan.Stream.URL[:strings.LastIndex(d.Plan.Stream.URL, "/")+1]
	master := h.get(t, d.Plan.Stream.URL)
	if err := silotest.CheckMultivariant(master.Body.Bytes(), w.Body.Bytes()); err != nil ||
		!strings.Contains(master.Body.String(), `URI="master.m3u8?silo_rendition=subtitles"`) {
		t.Errorf("master playlist: %v\n%s", err, master.Body)
	}
	if got := h.get(t, base+"master.m3u8?silo_rendition=subtitles"); !strings.Contains(got.Body.String(), "\nsegment/sub_1.vtt\n") {
		t.Errorf("subtitle playlist:\n%s", got.Body)
	}
	if got := h.get(t, base+"segment/sub_1.vtt"); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "/hls1/subtitles6/1.vtt") {
		t.Errorf("subtitle segment: %d %s", got.Code, got.Body)
	}

	none := decided(t, "no renditions", h.startAnswer(map[string]any{"quality_preference": "720p", "subtitle_track_index": 2,
		"playback_attempt_id": "attempt-2"}), http.StatusCreated, file)
	if none.Plan.Subtitle.Artifact == nil || none.Plan.Subtitle.HLSRendition != nil {
		t.Errorf("without renditions: %+v", none.Plan.Subtitle)
	}
	if video := h.get(t, none.Plan.Stream.URL); silotest.CheckMediaPlaylist(video.Body.Bytes(), none.Plan.Stream.URL) != nil {
		t.Errorf("the master playlist without renditions:\n%s", video.Body)
	}

	burned := decided(t, "a picture subtitle burned in", h.startAnswer(map[string]any{"client_features": renditionFeatures,
		"subtitle_track_index": 1, "playback_attempt_id": "attempt-3"}), http.StatusCreated, file)
	if burned.Plan.Subtitle.Mode != "burn_in" || burned.Plan.Subtitle.HLSRendition != nil || burned.Plan.Subtitle.Inventory[2].HLSRendition != nil {
		t.Errorf("burned in: %+v", burned.Plan.Subtitle)
	}

	original := decided(t, "played as it is", h.startAnswer(map[string]any{"client_features": renditionFeatures,
		"subtitle_track_index": 2, "playback_attempt_id": "attempt-4"}), http.StatusCreated, file)
	rows := original.Plan.Subtitle.Inventory
	if original.Plan.Delivery != originalHTTP || original.Plan.Subtitle.HLSRendition != nil || rows[2].HLSRendition != nil ||
		rows[1].ContainerTrackID != "4" || rows[2].ContainerTrackID != "5" || rows[3].ContainerTrackID != "6" || rows[0].ContainerTrackID != "" {
		t.Errorf("played as it is: %+v", original.Plan)
	}
}

// The Apple app sends its token only to a stream's master playlist, under
// silo_rendition, and to segment/{name} beside it: the Jellyfin API's own
// playlist addresses, which converted plans named before, it does not
// fetch.
func TestTheAppleAppFetchesOnlyTheStreamsAddresses(t *testing.T) {
	t.Parallel()
	stream := "/api/v2/playback/transcode/0b5f0c4e-1f7a-4c39-9c39-4f0e3b0a7d11/master.m3u8"
	for address, want := range map[string]bool{
		stream: true, "master.m3u8?silo_rendition=video": true, "master.m3u8?silo_rendition=subtitles": true,
		"master.m3u8?silo_rendition=subtitles&silo_track=3": true, "segment/0.ts": true, "segment/-1.mp4": true, "segment/sub_3_12.vtt": true,
		"segment/0.ts?seek=10": true, "master.m3u8?silo_rendition=video&silo_track=3": false, "master.m3u8?silo_rendition=subtitles&silo_track=03": false,
		"master.m3u8?silo_rendition=audio": false, "main.m3u8?PlaySessionId=play-1": false, "hls1/main/0.ts?PlaySessionId=play-1": false,
		"segment/0.ts?PlaySessionId=play-1": false, "hls1/subtitles5/main.m3u8": false, "/api/v2/stream/other": false,
	} {
		if got := silotest.MediaAddressAllowed(stream, address); got != want {
			t.Errorf("%s: %v, want %v", address, got, want)
		}
	}
}
