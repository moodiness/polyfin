package jellyfin

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/bandwidth"
	"github.com/moodiness/polyfin/internal/metrics"
	"github.com/moodiness/polyfin/internal/plays"
)

// An answer counts its bytes for its playback when it succeeds; an error
// counts none; a redirection to the source counts none and marks the
// playback as read by the app itself, until an answer sends bytes again.
func TestMeteredAnswersCountSuccessesAndTellRedirections(t *testing.T) {
	t.Parallel()
	h := &Handler{dlna: &dlnaPlays{}}
	h.Sent = bandwidth.New()
	device := accounts.ID{1}
	answer := func(status int, location, body string) {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/Videos/item/stream", nil)
		w := h.meteredDevice(httptest.NewRecorder(), request, device)
		defer w.release()
		if location != "" {
			w.Header().Set("Location", location)
		}
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	key := device.String()
	answer(http.StatusPartialContent, "", "media")
	if total, redirected := h.Sent.Total(key), h.Sent.Redirected(key); total != 5 || redirected {
		t.Errorf("after a success: %d bytes, redirected %v; want 5, false", total, redirected)
	}
	answer(http.StatusNotFound, "", "not found")
	if total := h.Sent.Total(key); total != 5 {
		t.Errorf("after an error: %d bytes, want 5", total)
	}
	answer(http.StatusFound, "http://source.test/file", "<a>moved</a>")
	if total, redirected := h.Sent.Total(key), h.Sent.Redirected(key); total != 5 || !redirected {
		t.Errorf("after a redirection: %d bytes, redirected %v; want 5, true", total, redirected)
	}
	answer(http.StatusOK, "", "more")
	if total, redirected := h.Sent.Total(key), h.Sent.Redirected(key); total != 9 || redirected {
		t.Errorf("after another success: %d bytes, redirected %v; want 9, false", total, redirected)
	}
}

// A direct play's bytes count for the device whose token its requests
// carry, and its version's for the version: the live session tells both
// rates and the addon the version comes from, the playback keeps what its
// device was sent, and another device's play of the same file counts
// apart.
func TestDirectPlaysCountWhatTheyMoveForTheirDevice(t *testing.T) {
	t.Parallel()
	sent, received := bandwidth.New(), bandwidth.New()
	var mu sync.Mutex
	var changes []plays.Change
	tracker := plays.NewTracker(func(c plays.Change) {
		mu.Lock()
		defer mu.Unlock()
		changes = append(changes, c)
	})
	p := playingOn(t, newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, _ *pgxpool.Pool) {
		o.Sent, o.Plays = sent, tracker
		o.Playback.CountReceived(received)
	}))
	version := p.versions[1].ID.String()
	// The test addon serves the version's file as these bytes.
	const file = "\x1a\x45\xdf\xa3 media bytes"
	if status, body := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token), map[string]any{"MediaSourceId": version}); status != http.StatusOK {
		t.Fatalf("playback info: %d %s", status, body)
	}
	if status, body := p.call(http.MethodPost, "/Sessions/Playing", app("tv", p.token),
		map[string]any{"ItemId": p.movie, "MediaSourceId": version, "PositionTicks": 0, "PlayMethod": "DirectPlay"}); status != http.StatusNoContent {
		t.Fatalf("start: %d %s", status, body)
	}
	sessions, err := p.handler.LiveSessions(t.Context())
	if err != nil || len(sessions) != 1 {
		t.Fatalf("live sessions: %+v %v", sessions, err)
	}
	device := sessions[0].Device.ID.String()

	// A rate needs two samples: the meters are sampled before the play
	// reads the file, then after, as Run does every second.
	sent.Release(sent.Hold(device))
	received.Release(received.Hold(version))
	sent.Sample()
	received.Sample()
	stream := p.url + "/Videos/" + p.movie + "/stream?static=true&mediaSourceId=" + version
	// Other tests send direct plays at the same time: the counter grows
	// by this play's bytes at least.
	directPlay := map[string]string{"delivery": metrics.DeliveryDirectPlay}
	before := metrics.Value("polyfin_bytes_sent_total", directPlay)
	if response, body := fetchURL(t, stream+"&ApiKey="+p.token, nil); response.StatusCode != http.StatusOK || body != file {
		t.Fatalf("direct play: %d %q", response.StatusCode, body)
	}
	sent.Sample()
	received.Sample()
	if counted := metrics.Value("polyfin_bytes_sent_total", directPlay) - before; counted < float64(len(file)) {
		t.Errorf("polyfin_bytes_sent_total{delivery=direct_play} grew by %v, want %d at least", counted, len(file))
	}

	sessions, err = p.handler.LiveSessions(t.Context())
	if err != nil || len(sessions) != 1 {
		t.Fatalf("live sessions: %+v %v", sessions, err)
	}
	tv := sessions[0]
	if tv.Sent <= 0 || tv.Received <= 0 || tv.Redirected {
		t.Errorf("the play moves %v B/s to the app and %v B/s from the source, redirected %v: want both above 0", tv.Sent, tv.Received, tv.Redirected)
	}
	if tv.Origin != (Origin{Kind: OriginAddon, Name: "Streams"}) {
		t.Errorf("origin %+v, want the addon Streams", tv.Origin)
	}
	if total := sent.Total(device); total != int64(len(file)) {
		t.Errorf("the device was sent %d bytes, want %d", total, len(file))
	}
	if total, rate := p.handler.Bandwidth(); total != tv.Sent || rate != tv.Received {
		t.Errorf("server totals %v and %v, want the one play's %v and %v", total, rate, tv.Sent, tv.Received)
	}

	// Another device playing the same file counts for itself.
	phone := p.signIn("member", "phone")
	if response, body := fetchURL(t, stream+"&ApiKey="+phone, nil); response.StatusCode != http.StatusOK || body != file {
		t.Fatalf("the phone's direct play: %d %q", response.StatusCode, body)
	}
	if total := sent.Total(device); total != int64(len(file)) {
		t.Errorf("the tv's total became %d bytes with the phone's play, want %d", total, len(file))
	}

	if status, body := p.call(http.MethodPost, "/Sessions/Playing/Stopped", app("tv", p.token),
		map[string]any{"ItemId": p.movie, "MediaSourceId": version, "PositionTicks": 600_000_000}); status != http.StatusNoContent {
		t.Fatalf("stop: %d %s", status, body)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(changes) != 2 || changes[1].Kind != plays.Stopped || changes[1].Playback.Sent != int64(len(file)) {
		t.Errorf("changes %+v, want a start and a stop with the %d bytes sent", changes, len(file))
	}
}
