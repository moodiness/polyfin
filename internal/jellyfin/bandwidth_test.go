package jellyfin

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/bandwidth"
	"github.com/moodiness/polyfin/internal/metrics"
	"github.com/moodiness/polyfin/internal/plays"
	"github.com/moodiness/polyfin/internal/source"
	"github.com/moodiness/polyfin/internal/stremio"
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
	// The play alone reads the file. A version's first play has its speech
	// found in the background (see playback.Service.FindSpeech): with
	// FFmpeg, as CI has, that reads the whole small file into the source
	// cache, before the first sample when the runner is busy, and the play
	// then reads it from the cache, receiving nothing from the source.
	p.setting(t, func(s *accounts.Settings) { s.SyncAddonSubtitles = false })
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

// publicSource is the host of the source a play is redirected to: a
// public address, which the test server reaches at a local one (see
// reachedAt), so that no request leaves the machine.
const publicSource = "203.0.113.10"

// reachedAt sends the requests for publicSource to the server at local.
type reachedAt struct {
	source.Opener
	local string
}

func (o reachedAt) Open(ctx context.Context, method, target string, header http.Header, confined bool) (*http.Response, error) {
	if parsed, err := url.Parse(target); err == nil && parsed.Host == publicSource {
		parsed.Host = o.local
		target = parsed.String()
	}
	return o.Opener.Open(ctx, method, target, header, confined)
}

// directPlayAddon lists one movie with two streams: the first on the
// local network, which Polyfin relays, the second on publicSource, which
// Polyfin sends the app to. It returns its manifest's address and the
// host it listens at.
func directPlayAddon(t *testing.T, file string) (manifest, host string) {
	t.Helper()
	var server *httptest.Server
	movie := stremio.Meta{ID: "tt3000", Type: "movie", Name: "Played", Runtime: "2h", Year: "2010", Released: "2010-01-01T00:00:00.000Z"}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "direct", Name: "Direct", Version: "1", Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case path == "/meta/movie/tt3000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case path == "/stream/movie/tt3000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{
				{Name: "Relayed", URL: server.URL + "/files/relayed.mp4", BehaviorHints: stremio.StreamBehavior{Filename: "Played.relayed.mp4"}},
				{Name: "Redirected", URL: "http://" + publicSource + "/files/redirected.mp4", BehaviorHints: stremio.StreamBehavior{Filename: "Played.redirected.mp4"}},
			}})
		case strings.HasPrefix(path, "/files/"):
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader(file))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json", strings.TrimPrefix(server.URL, "http://")
}

// A direct play's bytes count for its play whatever address the app
// fetches: the media source's Path, which carries a grant and no token,
// as the apps that play it as given fetch it; the stream route the app
// builds, with its token as ApiKey, as api_key with or without legacy
// authorization, or in the Authorization header; or with the play
// session alone. A relayed source shows the play's rate to the app, and
// its history keeps the bytes; a source the app is sent to shows that
// the app reads it itself, and its history keeps none. The header makes
// Polyfin relay even a public source (see stream).
func TestDirectPlaysCountForTheirPlayWhateverTheAddress(t *testing.T) {
	t.Parallel()
	const file = "\x1a\x45\xdf\xa3 media bytes"
	type address struct {
		target string
		header http.Header
	}
	type play struct {
		path, stream, device, token, session string
	}
	for _, form := range []struct {
		name    string
		legacy  bool
		relays  bool
		address func(p play) address
	}{
		{name: "the media source's Path", address: func(p play) address { return address{target: p.path} }},
		{name: "ApiKey", address: func(p play) address { return address{target: p.stream + "&ApiKey=" + p.token} }},
		{name: "api_key without legacy authorization", address: func(p play) address { return address{target: p.stream + "&api_key=" + p.token} }},
		{name: "api_key with legacy authorization", legacy: true, address: func(p play) address { return address{target: p.stream + "&api_key=" + p.token} }},
		{name: "the Authorization header", relays: true, address: func(p play) address {
			return address{target: p.stream, header: http.Header{"Authorization": {app(p.device, p.token)}}}
		}},
		{name: "the play session", address: func(p play) address {
			return address{target: p.stream + "&playSessionId=" + url.QueryEscape(p.session)}
		}},
	} {
		t.Run(form.name, func(t *testing.T) {
			t.Parallel()
			manifest, host := directPlayAddon(t, file)
			sent := bandwidth.New()
			var mu sync.Mutex
			var stopped []plays.Change
			tracker := plays.NewTracker(func(c plays.Change) {
				mu.Lock()
				defer mu.Unlock()
				if c.Kind == plays.Stopped {
					stopped = append(stopped, c)
				}
			})
			s := newServerReaching(t, 10, "ffprobe-not-installed", func(o source.Opener) source.Opener { return reachedAt{Opener: o, local: host} },
				func(o *Options, _ *pgxpool.Pool) { o.Sent, o.Plays = sent, tracker })
			user := s.user("member", nil)
			if _, err := s.addons.Install(t.Context(), addons.Shared(), manifest, false); err != nil {
				t.Fatal(err)
			}
			s.setting(t, func(settings *accounts.Settings) {
				settings.SyncAddonSubtitles = false
				settings.LegacyAuthorization = form.legacy
				settings.PlaybackHistory = true
			})
			token := s.signIn("member", "tv")
			var views, page QueryResult
			s.get(t, "/UserViews", token, &views)
			s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
			movie := page.Items[0].Id
			p := playbackSetup{testServer: s, token: token, user: user, movie: movie}
			p.versions = p.versionsOf(t, movie)
			p.listed(t, user, movie)
			for _, version := range p.versions {
				p.analyzed(t, version, "h264-aac-mp4")
			}
			history := plays.NewHistory(s.pool, s.store.Settings, slog.New(slog.DiscardHandler))

			// Each source plays on its own device, whose rate is its own.
			for i, redirected := range []bool{false, !form.relays} {
				version, name := p.versions[i].ID.String(), []string{"tv", "phone"}[i]
				token := s.signIn("member", name)
				status, body := s.call(http.MethodPost, "/Items/"+movie+"/PlaybackInfo", app(name, token), map[string]any{"MediaSourceId": version})
				var info playbackInfoResponse
				if err := json.Unmarshal(body, &info); status != http.StatusOK || err != nil || len(info.MediaSources) != 1 {
					t.Fatalf("playback info of %s: %d %s", p.versions[i].Name, status, body)
				}
				if status, body := s.call(http.MethodPost, "/Sessions/Playing", app(name, token), map[string]any{"ItemId": movie, "MediaSourceId": version,
					"PlaySessionId": info.PlaySessionId, "PositionTicks": 0, "PlayMethod": "DirectPlay"}); status != http.StatusNoContent {
					t.Fatalf("start: %d %s", status, body)
				}
				sessions, err := s.handler.LiveSessions(t.Context())
				if err != nil || len(sessions) != 1 {
					t.Fatalf("live sessions: %+v %v", sessions, err)
				}
				device := sessions[0].Device.ID.String()
				// A rate needs two samples: before the play reads, and after.
				// Now playing adds the meter of the play session to the
				// device's, as that of a guest or a download.
				sent.Release(sent.Hold(device))
				sent.Release(sent.Hold(info.PlaySessionId))
				sent.Sample()
				a := form.address(play{path: info.MediaSources[0].Path, device: name, token: token, session: info.PlaySessionId,
					stream: s.url + "/Videos/" + movie + "/stream?static=true&mediaSourceId=" + version})
				request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, a.target, nil)
				for name, values := range a.header {
					request.Header[name] = values
				}
				response, err := noRedirects.Do(request)
				if err != nil {
					t.Fatal(err)
				}
				got, _ := io.ReadAll(response.Body)
				response.Body.Close()
				switch {
				case redirected && (response.StatusCode != http.StatusFound || response.Header.Get("Location") != "http://"+publicSource+"/files/redirected.mp4"):
					t.Fatalf("%s: %d to %q, want a redirection to the source", p.versions[i].Name, response.StatusCode, response.Header.Get("Location"))
				case !redirected && (response.StatusCode != http.StatusOK || string(got) != file):
					t.Fatalf("%s: %d %q, want the file", p.versions[i].Name, response.StatusCode, got)
				}
				sent.Sample()

				sessions, err = s.handler.LiveSessions(t.Context())
				if err != nil || len(sessions) != 1 {
					t.Fatalf("live sessions: %+v %v", sessions, err)
				}
				tv := sessions[0]
				if redirected && (tv.Sent != 0 || !tv.Redirected) || !redirected && (tv.Sent <= 0 || tv.Redirected) {
					t.Errorf("%s: the play sends %v B/s to the app, redirected %v; want %v", p.versions[i].Name, tv.Sent, tv.Redirected,
						map[bool]string{true: "none, redirected", false: "some, not redirected"}[redirected])
				}
				if status, body := s.call(http.MethodPost, "/Sessions/Playing/Stopped", app(name, token), map[string]any{"ItemId": movie, "MediaSourceId": version,
					"PlaySessionId": info.PlaySessionId, "PositionTicks": 600_000_000}); status != http.StatusNoContent {
					t.Fatalf("stop: %d %s", status, body)
				}
				mu.Lock()
				stop := stopped[len(stopped)-1]
				mu.Unlock()
				// The play lasted no time: it is kept as one that played
				// long enough.
				stop.Playback.Played = plays.MinPlayed
				history.Record(t.Context(), stop)
				var kept *int64
				if err := s.pool.QueryRow(t.Context(), "SELECT sent_bytes FROM playback_history ORDER BY ended_at DESC, started_at DESC LIMIT 1").Scan(&kept); err != nil {
					t.Fatal(err)
				}
				switch {
				case redirected && kept != nil:
					t.Errorf("%s: the history keeps %d bytes sent, want none", p.versions[i].Name, *kept)
				case !redirected && (kept == nil || *kept != int64(len(file))):
					t.Errorf("%s: the history keeps %v bytes sent, want %d", p.versions[i].Name, kept, len(file))
				}
			}
		})
	}
}
