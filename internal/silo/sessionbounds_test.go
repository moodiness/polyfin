package silo

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The bounds of the playback sessions kept: how many a user and the
// server keep, what each keeps of the app's own words, and when they are
// walked.

// kept counts the sessions the harness's server keeps.
func (h *harness) kept() int {
	h.server.sessions.mu.Lock()
	defer h.server.sessions.mu.Unlock()
	return len(h.server.sessions.byID)
}

// session is a session the harness's server keeps, nil for none.
func (h *harness) session(id string) *session {
	h.server.sessions.mu.Lock()
	defer h.server.sessions.mu.Unlock()
	return h.server.sessions.byID[id]
}

// plays reports whether a session's media are served.
func (h *harness) plays(session string) bool {
	h.t.Helper()
	return h.do(http.MethodGet, "/api/v2/stream/"+session, nil).Code == http.StatusOK
}

// refusedAt is where a validation problem tells the request went wrong,
// empty for any other answer.
func refusedAt(w *httptest.ResponseRecorder) string {
	var p problemJSON
	if w.Code != http.StatusUnprocessableEntity || json.Unmarshal(w.Body.Bytes(), &p) != nil || len(p.Errors) != 1 ||
		!strings.HasSuffix(p.Type, "/validation_failed") || p.Errors[0].Code != "out_of_range" {
		return ""
	}
	return p.Errors[0].Location
}

// names are n different names of size bytes each.
func names(n, size int) []string {
	list := make([]string, n)
	for i := range list {
		number := strconv.Itoa(i)
		list[i] = strings.Repeat("x", size-len(number)) + number
	}
	return list
}

// stored is a session of a user as the store keeps one, last used at seen.
func stored(id string, user accounts.ID, seen time.Time) *session {
	return &session{id: id, user: user, seen: seen, replans: map[string][]byte{}}
}

// A user keeps sessionsKept playbacks. One more forgets the one unused
// the longest and stops its conversion; a playback already stopped goes
// before any other, and is not stopped again.
func TestAUserKeepsTheirLastPlaybacks(t *testing.T) {
	t.Parallel()
	h := newHarness(t, playable)
	var started []string
	for range sessionsKept {
		session, _ := h.start(nil)
		started = append(started, session)
		*h.clock = h.clock.Add(time.Second)
	}
	if kept := h.kept(); kept != sessionsKept {
		t.Fatalf("%d sessions kept of %d started", kept, sessionsKept)
	}
	// The first is used again: the second is the one unused the longest.
	if !h.plays(started[0]) {
		t.Fatal("the first session does not play")
	}
	*h.clock = h.clock.Add(time.Second)
	second := h.session(started[1]).playSession
	latest, _ := h.start(nil)
	if h.session(started[1]) != nil || h.plays(started[1]) {
		t.Error("the session unused the longest is still kept")
	}
	if kept := h.kept(); kept != sessionsKept || !h.plays(started[0]) || !h.plays(started[2]) || !h.plays(latest) {
		t.Errorf("%d sessions kept; the others no longer play", kept)
	}
	stops := h.jellyfin.called(http.MethodDelete, "/Videos/ActiveEncodings")
	if len(stops) != 1 || stops[0].query.Get("playSessionId") != second || second == "" {
		t.Errorf("the conversions stopped: %+v, want that of play session %q", stops, second)
	}

	// The latest stops: it goes first, though it was used last.
	*h.clock = h.clock.Add(time.Second)
	if w := h.do(http.MethodDelete, "/api/v2/playback/"+latest, map[string]any{"installation_id": h.accounts.device.ID.String(),
		"stop_id": "stop-1"}); w.Code != http.StatusOK {
		t.Fatalf("stop: %d %s", w.Code, w.Body)
	}
	*h.clock = h.clock.Add(time.Second)
	h.start(nil)
	if h.session(latest) != nil {
		t.Error("the stopped session is still kept")
	}
	if h.session(started[2]) == nil || h.kept() != sessionsKept {
		t.Errorf("a session in use went before the stopped one; %d kept", h.kept())
	}
	if stops := h.jellyfin.called(http.MethodDelete, "/Videos/ActiveEncodings"); len(stops) != 1 {
		t.Errorf("a stopped session was stopped again: %+v", stops)
	}

	// However many a user starts, on one device, they keep as many.
	for range 3 * sessionsKept {
		*h.clock = h.clock.Add(time.Second)
		h.start(nil)
	}
	if kept := h.kept(); kept != sessionsKept {
		t.Errorf("%d sessions kept after %d more starts", kept, 3*sessionsKept)
	}
	if again := h.session(started[0]); again != nil {
		t.Error("the first session outlived the starts that followed")
	}
}

// A user's bound is their own: the sessions one user starts forget none
// of another's.
func TestAUsersPlaybacksLeaveThoseOfOthers(t *testing.T) {
	t.Parallel()
	store := newSessions()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	quiet, busy := accounts.ID{1}, accounts.ID{9}
	for n := range 3 {
		store.add(stored("quiet-"+strconv.Itoa(n), quiet, now.Add(time.Duration(n)*time.Second)))
	}
	var forgotten []string
	for n := range sessionsKept + 10 {
		for _, gone := range store.add(stored("busy-"+strconv.Itoa(n), busy, now.Add(time.Minute+time.Duration(n)*time.Second))) {
			forgotten = append(forgotten, gone.id)
		}
	}
	if len(forgotten) != 10 || forgotten[0] != "busy-0" || forgotten[9] != "busy-9" {
		t.Errorf("forgotten: %v, want the ten first of the user who started them", forgotten)
	}
	later := now.Add(time.Hour)
	for n := range 3 {
		if _, done, ok := store.get("quiet-"+strconv.Itoa(n), quiet, later); ok {
			done()
		} else {
			t.Errorf("the other user's session %d is gone", n)
		}
	}
	if len(store.byID) != sessionsKept+3 || len(store.byUser[busy]) != sessionsKept || len(store.byUser[quiet]) != 3 {
		t.Errorf("%d sessions kept, %d and %d a user", len(store.byID), len(store.byUser[busy]), len(store.byUser[quiet]))
	}
	// A session is its user's alone.
	if _, _, ok := store.get("quiet-0", busy, later); ok {
		t.Error("a user got another's session")
	}
}

// The server keeps maxSessions playbacks of all its users: one more
// forgets the one unused the longest, whoever started it.
func TestTheServerKeepsABoundedNumberOfPlaybacks(t *testing.T) {
	t.Parallel()
	store := newSessions()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	// Each user stays under their own bound.
	const each = sessionsKept / 2
	user := func(n int) accounts.ID { return accounts.ID{1, byte(n / each >> 8), byte(n / each)} }
	for n := range maxSessions {
		if gone := store.add(stored("session-"+strconv.Itoa(n), user(n), now.Add(time.Duration(n)*time.Second))); len(gone) != 0 {
			t.Fatalf("session %d forgot %d others", n, len(gone))
		}
	}
	later := now.Add(time.Hour)
	// The first is used: the second is the one unused the longest.
	if _, done, ok := store.get("session-0", user(0), later); ok {
		done()
	} else {
		t.Fatal("the first session is gone")
	}
	gone := store.add(stored("one-more", accounts.ID{2}, later.Add(time.Second)))
	if len(gone) != 1 || gone[0].id != "session-1" {
		t.Errorf("forgotten: %d sessions, want the one unused the longest", len(gone))
	}
	if len(store.byID) != maxSessions || store.byID["session-0"] == nil || store.byID["one-more"] == nil {
		t.Errorf("%d sessions kept, want %d with the first and the last", len(store.byID), maxSessions)
	}
	users := 0
	for _, mine := range store.byUser {
		users += len(mine)
	}
	if users != maxSessions {
		t.Errorf("%d sessions listed by user, want %d", users, maxSessions)
	}
}

// A session forgotten for the server's bound is another user's: the
// request that took its place stops nothing in that user's name.
func TestAPlaybackForgottenForTheServersBoundIsNotStoppedInItsUsersName(t *testing.T) {
	t.Parallel()
	h := newHarness(t, playable)
	const each = sessionsKept / 2
	for n := range maxSessions {
		other := stored("session-"+strconv.Itoa(n), accounts.ID{7, byte(n / each >> 8), byte(n / each)}, h.clock.Add(time.Duration(n)*time.Second))
		other.playSession = "play-" + strconv.Itoa(n)
		h.server.sessions.add(other)
	}
	*h.clock = h.clock.Add(time.Hour)
	session, _ := h.start(nil)
	if kept := h.kept(); kept != maxSessions || h.session("session-0") != nil || !h.plays(session) {
		t.Errorf("%d sessions kept, the one unused the longest among them: %t", kept, h.session("session-0") != nil)
	}
	if stops := h.jellyfin.called(http.MethodDelete, "/Videos/ActiveEncodings"); len(stops) != 0 {
		t.Errorf("stopped as another user: %+v", stops)
	}
}

// A start whose lists of containers or codecs, attempt identifier or
// quality pass their bounds is refused, and nothing is kept nor asked of
// the Jellyfin API; at the bounds it plays, and the session keeps what
// plans read alone.
func TestAStartPastItsBoundsIsRefused(t *testing.T) {
	t.Parallel()
	h := newHarness(t, playable)
	capabilities := func(list string, value []string) map[string]any {
		told := map[string]any{"containers": []string{"mp4"}, "codecs_video": []string{"h264"}, "codecs_audio": []string{"aac"}}
		told[list] = value
		return map[string]any{"client_capabilities": told}
	}
	delivery := func(name, list string, value []string) map[string]any {
		return map[string]any{"client_playback_context": map[string]any{"deliveries": map[string]any{name: map[string]any{
			"enabled": true, "supported_on_device": true, list: value}}}}
	}
	long := strings.Repeat("x", maxIdentifier+1)
	for location, more := range map[string]map[string]any{
		"body.client_capabilities.containers":   capabilities("containers", names(maxNames+1, 4)),
		"body.client_capabilities.codecs_video": capabilities("codecs_video", names(1, maxName+1)),
		// 300,000 empty names are 900 kB of JSON, under the bound of a body.
		"body.client_capabilities.codecs_audio":                                          capabilities("codecs_audio", make([]string, 300_000)),
		"body.client_playback_context.deliveries.original_http.containers":               delivery("original_http", "containers", names(maxNames+1, 4)),
		"body.client_playback_context.deliveries.original_http.video_codecs":             delivery("original_http", "video_codecs", names(1, maxName+1)),
		"body.client_playback_context.deliveries.original_http.audio_decode_codecs":      delivery("original_http", "audio_decode_codecs", names(maxNames+1, 4)),
		"body.client_playback_context.deliveries.hls.audio_passthrough_codecs":           delivery("hls", "audio_passthrough_codecs", names(maxNames+1, 4)),
		"body.client_playback_context.deliveries.hls.video_codecs":                       delivery("hls", "video_codecs", names(1, maxName+1)),
		"body.client_playback_context.deliveries.hls.containers":                         delivery("hls", "containers", names(maxNames+1, 4)),
		"body.playback_attempt_id":                                                       {"playback_attempt_id": long},
		"body.quality_preference":                                                        {"quality_preference": long},
		"body.client_playback_context.deliveries.original_http.audio_passthrough_codecs": delivery("original_http", "audio_passthrough_codecs", names(maxNames+1, 4)),
	} {
		if w := h.startAnswer(more); refusedAt(w) != location {
			t.Errorf("%s: %d %.300s", location, w.Code, w.Body)
		}
	}
	if kept := h.kept(); kept != 0 {
		t.Errorf("%d sessions kept of starts refused", kept)
	}
	h.jellyfin.mu.Lock()
	asked := len(h.jellyfin.calls)
	h.jellyfin.mu.Unlock()
	if asked != 0 {
		t.Errorf("%d calls of the Jellyfin API for starts refused", asked)
	}

	// At the bounds, with a delivery plans do not read past them.
	audio := names(2*maxNames, maxName)
	full := map[string]any{"enabled": true, "supported_on_device": true, "containers": names(maxNames, maxName),
		"video_codecs": names(maxNames, maxName), "audio_decode_codecs": audio[:maxNames], "audio_passthrough_codecs": audio[maxNames:]}
	session, _ := h.start(map[string]any{"playback_attempt_id": long[1:], "quality_preference": long[1:],
		"client_capabilities": map[string]any{"containers": names(maxNames, maxName), "codecs_video": names(maxNames, maxName),
			"codecs_audio": names(maxNames, maxName), "max_resolution": strings.Repeat("p", 100_000)},
		"client_playback_context": map[string]any{"deliveries": map[string]any{"original_http": full, "hls": full,
			"another": map[string]any{"enabled": true, "video_codecs": names(10*maxNames, 2*maxName)}}}})
	found := h.session(session)
	if found == nil || !h.plays(session) {
		t.Fatal("a start at its bounds does not play")
	}
	// A list is kept as its names, each once, with a comma between them;
	// the audio codecs are those decoded and those passed through.
	list := maxNames*(maxName+1) - 1
	if c := found.choice.client; len(c.containers) != list || len(c.video) != list || len(c.audio) != 2*list+1 ||
		c.hlsVideo != "h264" || c.hlsAudio != "aac" || c.segments != "ts" || c.height != 0 {
		t.Errorf("kept of the app: %d, %d and %d bytes of names, HLS %q %q %q, height %d", len(c.containers), len(c.video), len(c.audio),
			c.hlsVideo, c.hlsAudio, c.segments, c.height)
	}
	if len(found.attempt) != maxIdentifier || len(found.choice.quality) != maxIdentifier {
		t.Errorf("kept: an attempt of %d bytes, a quality of %d", len(found.attempt), len(found.choice.quality))
	}
}

// A replan whose identifier, quality or lists pass their bounds is
// refused and changes nothing; the identifiers a session keeps are those
// of the answers it keeps, of maxIdentifier bytes each at most.
func TestAReplanPastItsBoundsIsRefused(t *testing.T) {
	t.Parallel()
	h := newHarness(t, playable)
	session, _ := h.start(nil)
	found := h.session(session)
	before := found.choice
	long := strings.Repeat("x", maxIdentifier+1)
	for location, more := range map[string]map[string]any{
		"body.replan_request_id":  {"replan_request_id": long},
		"body.quality_preference": {"quality_preference": long},
		"body.client_capabilities.codecs_audio": {"client_capabilities": map[string]any{"containers": []string{"mp4"},
			"codecs_video": []string{"h264"}, "codecs_audio": names(maxNames+1, 4)}},
		"body.client_playback_context.deliveries.hls.video_codecs": {"client_playback_context": map[string]any{"deliveries": map[string]any{
			"hls": map[string]any{"enabled": true, "video_codecs": names(1, maxName+1)}}}},
	} {
		if w := h.replanWith(session, "replan-refused", more); refusedAt(w) != location {
			t.Errorf("%s: %d %.300s", location, w.Code, w.Body)
		}
	}
	if len(found.replanOrder) != 0 || len(found.replans) != 0 || found.choice != before {
		t.Errorf("replans refused left %d answers, and the choice %+v", len(found.replanOrder), found.choice)
	}

	var last string
	for n := range maxReplans + 4 {
		id := names(n+1, maxIdentifier)[n]
		last = planID(t, h.replan(session, id, map[string]any{}))
		if again := planID(t, h.replan(session, id, map[string]any{})); again != last {
			t.Fatalf("replan %d is not replayed", n)
		}
	}
	bytesKept := 0
	for _, id := range found.replanOrder {
		bytesKept += len(id)
	}
	if kept := len(found.replanOrder); kept == 0 || kept > maxReplans || len(found.replans) != kept || bytesKept != kept*maxIdentifier ||
		found.replanSize > replanBytes {
		t.Errorf("%d identifiers kept for %d answers, %d bytes of identifiers and %d of answers", kept, len(found.replans), bytesKept,
			found.replanSize)
	}
}

// A session keeps replanBytes of answers at most, and maxReplans answers:
// the oldest go first, and an answer longer than replanBytes is kept
// alone, until the next one.
func TestReplanAnswersKeptAreBoundedInBytes(t *testing.T) {
	t.Parallel()
	found := stored("session", accounts.ID{1}, time.Time{})
	const size = replanBytes/4 + 1
	for n := range 10 {
		found.keepReplan("replan-"+strconv.Itoa(n), bytes.Repeat([]byte{'x'}, size))
	}
	kept := 0
	for _, id := range found.replanOrder {
		kept += len(found.replans[id])
	}
	if len(found.replanOrder) != 3 || len(found.replans) != 3 || kept != 3*size || found.replanSize != kept {
		t.Errorf("%d answers kept, %d bytes of them, counted as %d", len(found.replanOrder), kept, found.replanSize)
	}
	if found.replans["replan-9"] == nil || found.replans["replan-6"] != nil {
		t.Errorf("kept: %v, want the last three", found.replanOrder)
	}
	found.keepReplan("too-long", make([]byte, replanBytes+1))
	if found.replans["too-long"] == nil || len(found.replanOrder) != 1 || len(found.replans) != 1 || found.replanSize != replanBytes+1 {
		t.Errorf("an answer past the bound: %v, %d bytes", found.replanOrder, found.replanSize)
	}
	found.keepReplan("whole", make([]byte, replanBytes))
	if len(found.replanOrder) != 1 || found.replans["whole"] == nil || found.replanSize != replanBytes {
		t.Errorf("an answer of the bound: %v, %d bytes", found.replanOrder, found.replanSize)
	}
	for n := range maxReplans + 8 {
		found.keepReplan("short-"+strconv.Itoa(n), []byte{'x'})
	}
	if len(found.replanOrder) != maxReplans || len(found.replans) != maxReplans || found.replanSize != maxReplans ||
		found.replanOrder[0] != "short-8" {
		t.Errorf("%d short answers kept, %d bytes, the oldest %s", len(found.replanOrder), found.replanSize, found.replanOrder[0])
	}
}

// A stop's identifier, which the session keeps to answer the stop again,
// is bounded as the others are.
func TestAStopPastItsBoundIsRefused(t *testing.T) {
	t.Parallel()
	h := newHarness(t, playable)
	session, _ := h.start(nil)
	stop := func(id string) *httptest.ResponseRecorder {
		return h.do(http.MethodDelete, "/api/v2/playback/"+session, map[string]any{"installation_id": h.accounts.device.ID.String(), "stop_id": id})
	}
	if w := stop(strings.Repeat("x", maxIdentifier+1)); refusedAt(w) != "body.stop_id" {
		t.Errorf("a stop past the bound: %d %.300s", w.Code, w.Body)
	}
	if !h.plays(session) || len(h.jellyfin.called(http.MethodPost, "/Sessions/Playing/Stopped")) != 0 {
		t.Error("a stop refused stopped the playback")
	}
	if w := stop(strings.Repeat("x", maxIdentifier)); w.Code != http.StatusOK || h.plays(session) {
		t.Errorf("a stop at the bound: %d %s", w.Code, w.Body)
	}
}

// A request of a session looks it up and walks none: the sessions are
// walked for those unused as one starts, once per sweepInterval at most.
// A session unused for sessionLifetime is no longer served, and is gone
// once they are walked; one in use stays.
func TestRequestsOfASessionWalkNoOther(t *testing.T) {
	t.Parallel()
	store := newSessions()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	viewer := accounts.ID{1}
	store.add(stored("viewer", viewer, now))
	const others = 500
	for n := range others {
		store.add(stored("other-"+strconv.Itoa(n), accounts.ID{2, byte(n / 4)}, now.Add(time.Duration(n)*time.Millisecond)))
	}
	if store.walks != 1 {
		t.Errorf("%d walks for %d starts within %v", store.walks, others+1, sweepInterval)
	}
	// A request a minute, for 23 hours.
	at := now
	for range 23 * 60 {
		at = at.Add(time.Minute)
		_, done, ok := store.get("viewer", viewer, at)
		if !ok {
			t.Fatal("the session in use is gone")
		}
		done()
	}
	if store.walks != 1 || len(store.byID) != others+1 {
		t.Errorf("%d walks after the requests of one session, %d sessions kept", store.walks, len(store.byID))
	}
	// Two hours later the others were unused for a day: none is served,
	// before any walk.
	at = at.Add(2 * time.Hour)
	if _, _, ok := store.get("other-0", accounts.ID{2, 0}, at); ok {
		t.Error("a session unused for a day is served")
	}
	if _, done, ok := store.get("viewer", viewer, at); ok {
		done()
	} else {
		t.Error("the session in use is no longer served")
	}
	if store.walks != 1 || len(store.byID) != others+1 {
		t.Errorf("%d walks, %d sessions kept", store.walks, len(store.byID))
	}
	// The next start walks them, and those that follow within the
	// interval do not.
	for n := range 5 {
		store.add(stored("next-"+strconv.Itoa(n), accounts.ID{3}, at.Add(time.Duration(n)*time.Minute)))
	}
	if store.walks != 2 {
		t.Errorf("%d walks, want one more for the starts of %v", store.walks-1, sweepInterval)
	}
	if len(store.byID) != 6 || store.byID["viewer"] == nil || len(store.byUser) != 2 {
		t.Errorf("%d sessions kept of %d users, want the one in use and the five just started", len(store.byID), len(store.byUser))
	}
	store.add(stored("later", accounts.ID{3}, at.Add(sweepInterval)))
	if store.walks != 3 {
		t.Errorf("%d walks, want one more once %v passed", store.walks, sweepInterval)
	}
}

// A playback in use outlives one left unused, through the API: the one
// unused for a day answers as unknown, and goes as the next one starts.
func TestAPlaybackInUseOutlivesOneLeftUnused(t *testing.T) {
	t.Parallel()
	h := newHarness(t, playable)
	unused, _ := h.start(nil)
	used, _ := h.start(map[string]any{"playback_attempt_id": "attempt-2"})
	*h.clock = h.clock.Add(23 * time.Hour)
	if !h.plays(used) {
		t.Fatal("the session does not play 23 hours later")
	}
	*h.clock = h.clock.Add(2 * time.Hour)
	if w := h.do(http.MethodGet, "/api/v2/stream/"+unused, nil); w.Code != http.StatusNotFound || problemCode(w) != "not_found" {
		t.Errorf("the session unused for 25 hours: %d %s", w.Code, w.Body)
	}
	if !h.plays(used) {
		t.Error("the session used 2 hours before no longer plays")
	}
	h.start(map[string]any{"playback_attempt_id": "attempt-3"})
	if kept := h.kept(); kept != 2 || h.session(unused) != nil || h.session(used) == nil {
		t.Errorf("%d sessions kept, the unused one among them: %t", kept, h.session(unused) != nil)
	}
}

// What the Jellyfin API is told of the app is what its capabilities say,
// at a start and at each replan: the containers and codecs it plays as
// they are, named as the Jellyfin API names them, each once; those of the
// HLS it plays converted; and the tallest picture it shows.
func TestPlansTellTheJellyfinAPIWhatTheAppPlays(t *testing.T) {
	t.Parallel()
	h := newHarness(t, playable)
	session, _ := h.start(map[string]any{
		"client_capabilities": map[string]any{"containers": []string{"MP4", "matroska", " mp4 "}, "codecs_video": []string{"h264"},
			"codecs_audio": []string{"mp3"}, "max_resolution": "1080P"},
		"client_playback_context": map[string]any{"deliveries": map[string]any{
			"original_http": map[string]any{"enabled": true, "supported_on_device": true, "video_codecs": []string{"h264", " HEVC ", "h264"},
				"audio_decode_codecs": []string{"aac", "ac3"}, "audio_passthrough_codecs": []string{"eac3", "AAC"}},
			"hls": map[string]any{"enabled": true, "containers": []string{"fmp4"}, "video_codecs": []string{"hevc", "vp9"},
				"audio_decode_codecs": []string{"opus"}, "audio_passthrough_codecs": []string{"ac3"}}}}})
	want := "plays mp4:h264,hevc:aac,ac3,eac3 mkv:h264,hevc:aac,ac3,eac3; converted mp4:h264,hevc:aac,ac3,opus; height 1080"
	if got := h.told(); got != want {
		t.Errorf("a start told %q, want %q", got, want)
	}
	// A replan after a failure converts, for the client it tells then.
	if w := h.replanWith(session, "replan-1", map[string]any{"operation": "failure_recovery", "client_capabilities": map[string]any{
		"containers": []string{"mkv"}, "codecs_video": []string{"av1"}, "codecs_audio": []string{"flac"}}}); w.Code != http.StatusOK {
		t.Fatalf("replan: %d %s", w.Code, w.Body)
	}
	if got, want := h.told(), "plays; converted ts:h264:aac; height "; got != want {
		t.Errorf("a replan told %q, want %q", got, want)
	}
}

// told is what the device profile of the last PlaybackInfo asked says of
// the client: the containers it plays as they are, each with its video
// and audio codecs, those of the stream it plays converted, and the
// height its picture keeps under.
func (h *harness) told() string {
	h.t.Helper()
	h.jellyfin.mu.Lock()
	defer h.jellyfin.mu.Unlock()
	for i := len(h.jellyfin.calls) - 1; i >= 0; i-- {
		if !strings.HasSuffix(h.jellyfin.calls[i].path, "/PlaybackInfo") {
			continue
		}
		var profile struct {
			DirectPlayProfiles, TranscodingProfiles []struct{ Container, VideoCodec, AudioCodec string }
			CodecProfiles                           []struct{ Conditions []struct{ Value string } }
		}
		data, _ := json.Marshal(h.jellyfin.calls[i].body["DeviceProfile"])
		if err := json.Unmarshal(data, &profile); err != nil || len(profile.TranscodingProfiles) != 1 {
			h.t.Fatalf("the device profile: %v: %s", err, data)
		}
		said := "plays"
		for _, direct := range profile.DirectPlayProfiles {
			said += " " + direct.Container + ":" + direct.VideoCodec + ":" + direct.AudioCodec
		}
		stream := profile.TranscodingProfiles[0]
		said += "; converted " + stream.Container + ":" + stream.VideoCodec + ":" + stream.AudioCodec + "; height "
		for _, codec := range profile.CodecProfiles {
			for _, condition := range codec.Conditions {
				said += condition.Value
			}
		}
		return said
	}
	h.t.Fatal("PlaybackInfo was not asked")
	return ""
}
