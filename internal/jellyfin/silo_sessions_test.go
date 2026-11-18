package jellyfin

import (
	"net/http"
	"strings"
	"testing"
)

// A member's Silo app keeps the last playbacks it started, on the
// Jellyfin API itself: of eighteen starts of one title on one device, the
// sixteen last still play and the two first are unknown; and a start
// that tells more codecs than a list may hold is refused, and forgets
// none.
func TestSiloAppsKeepAMembersLastPlaybacks(t *testing.T) {
	t.Parallel()
	p := playing(t)
	allowSilo(t, p.testServer)
	a := servingSilo(t, p.testServer)
	token, profile := a.signIn(t, "member")
	installation := a.installation(t, token, profile)
	file := a.fileOf(t, p.movie, token, profile, 1)
	// kept is how many playbacks a user keeps (sessionsKept, in silo).
	const kept, starts = 16, 18
	var sessions []string
	for n := range starts {
		d := a.start(t, token, profile, siloStart(file, profile, installation, []string{"mp4"}, []string{"h264"}, []string{"aac"}, nil))
		if d.Outcome != "playable" || d.SessionID == "" {
			t.Fatalf("start %d: %+v", n, d)
		}
		sessions = append(sessions, d.SessionID)
	}
	// 100,000 empty codecs are 300 kB of JSON.
	status, data := a.do(t, http.MethodPost, "/api/v2/playback/start", token, profile,
		siloStart(file, profile, installation, []string{"mp4"}, []string{"h264"}, append(make([]string, 100_000), "aac"), nil))
	if status != http.StatusUnprocessableEntity || problemCode(data) != "validation_failed" {
		t.Errorf("a start telling 100,001 audio codecs: %d %.300s", status, data)
	}
	for n, session := range sessions {
		status, body := a.fetch(t, "/api/v2/stream/"+session, token)
		if n < starts-kept && (status != http.StatusNotFound || problemCode([]byte(body)) != "not_found") {
			t.Errorf("session %d of %d, past the %d a user keeps: %d", n, starts, kept, status)
		}
		if n >= starts-kept && (status != http.StatusOK || !strings.Contains(body, "media bytes")) {
			t.Errorf("session %d of %d, among the last %d: %d", n, starts, kept, status)
		}
	}
}
