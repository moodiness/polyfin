package trackers

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// clock is a time the tests move on by hand.
type clock struct {
	mu sync.Mutex
	at time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(d)
}

// musicHarness is a harness whose time moves only when the test moves it.
func musicHarness(t *testing.T) (harness, *clock) {
	t.Helper()
	h := newHarness(t)
	c := &clock{at: clockStart}
	h.now = c.now
	// What the time of a report makes stale is not under test here.
	h.timing.stale = 24 * time.Hour
	return h, c
}

// tune is a 200-second song: it is scrobbled after 100 seconds.
var tune = Song{Item: accounts.ID{20}, Artist: "The Artist", Title: "A Song", Album: "An Album", AlbumArtist: "Various",
	TrackNumber: 3, DurationMS: 200_000, ISRC: "XX0000000001"}

// hear reports a playback of song at position seconds.
func hear(event Event, song Song, seconds float64, paused bool) Listening {
	return Listening{Event: event, Device: device, Position: time.Duration(seconds * float64(time.Second)), PositionKnown: true,
		Paused: paused, Song: song}
}

// lastFMCalls lists the calls of method Last.fm received so far.
func (f *fakes) lastFMCalls(method string) []url.Values {
	var calls []url.Values
	for _, r := range f.sent("lastfm", "/2.0/") {
		params := r.form
		if r.method == http.MethodGet {
			params = r.query
		}
		if params.Get("method") == method {
			calls = append(calls, params)
		}
	}
	return calls
}

// waitLastFM waits until Last.fm received n calls of method.
func (f *fakes) waitLastFM(t *testing.T, n int, method string) []url.Values {
	t.Helper()
	eventually(t, strconv.Itoa(n)+" "+method, func() bool { return len(f.lastFMCalls(method)) >= n })
	return f.lastFMCalls(method)
}

// signedByLastFMRules reports whether params carry the signature Last.fm
// documents: the MD5 of every parameter but format and api_sig, sorted by
// name, name then value, then the shared secret.
func signedByLastFMRules(params url.Values, secret string) bool {
	var names []string
	for name := range params {
		if name != "format" && name != "api_sig" {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	var text strings.Builder
	for _, name := range names {
		text.WriteString(name + params.Get(name))
	}
	sum := md5.Sum([]byte(text.String() + secret))
	return params.Get("api_sig") == hex.EncodeToString(sum[:])
}

// listens lists the bodies ListenBrainz received of listenType.
func (f *fakes) listens(listenType string) []map[string]any {
	var found []map[string]any
	for _, r := range f.sent("listenbrainz", "/1/submit-listens") {
		if body, ok := r.body.(map[string]any); ok && body["listen_type"] == listenType {
			found = append(found, body)
		}
	}
	return found
}

func (f *fakes) waitListens(t *testing.T, n int, listenType string) []map[string]any {
	t.Helper()
	eventually(t, strconv.Itoa(n)+" "+listenType, func() bool { return len(f.listens(listenType)) >= n })
	return f.listens(listenType)
}

func TestLastFMIsConnectedThroughItsSignInPage(t *testing.T) {
	h, _ := musicHarness(t)
	alice := h.user(t, "alice")
	sessions := 0
	h.f.on(http.MethodGet, "/lastfm/2.0/", func(r request) answer {
		if !signedByLastFMRules(r.query, "lastfm-secret") || r.query.Get("api_key") != "lastfm-key" || r.query.Get("format") != "json" {
			return answer{status: http.StatusForbidden, body: `{"error":13,"message":"Invalid method signature supplied"}`}
		}
		switch r.query.Get("method") {
		case "auth.getToken":
			return answer{status: http.StatusOK, body: `{"token":"request-token"}`}
		case "auth.getSession":
			if r.query.Get("token") != "request-token" {
				return answer{status: http.StatusForbidden, body: `{"error":4,"message":"Invalid authentication token supplied"}`}
			}
			// Twice not allowed yet, then allowed.
			if sessions++; sessions < 3 {
				return answer{status: http.StatusForbidden, body: `{"error":14,"message":"Unauthorized Token - This token has not been authorized"}`}
			}
			return answer{status: http.StatusOK, body: `{"session":{"name":"alice-fm","key":"session-key","subscriber":0}}`}
		}
		return answer{status: http.StatusBadRequest, body: `{"error":3,"message":"Invalid Method"}`}
	})

	status, err := h.StartCode(t.Context(), alice, LastFM)
	if err != nil {
		t.Fatal(err)
	}
	if status.Code == nil || status.Code.UserCode != "" ||
		status.Code.VerificationURL != "https://www.last.fm/api/auth/?api_key=lastfm-key&token=request-token" || !status.BySignIn {
		t.Fatalf("sign-in: %+v %+v", status, status.Code)
	}
	// The signature of the first call, worked out by hand.
	sum := md5.Sum([]byte("api_keylastfm-keymethodauth.getTokenlastfm-secret"))
	if got := h.f.lastFMCalls("auth.getToken")[0].Get("api_sig"); got != hex.EncodeToString(sum[:]) {
		t.Errorf("auth.getToken signed %q", got)
	}
	eventually(t, "the connection", func() bool { return h.status(t, alice, LastFM).Connected })
	status = h.status(t, alice, LastFM)
	if status.Account != "alice-fm" || status.Code != nil || status.Problem != "" {
		t.Errorf("connected: %+v", status)
	}
	if c, ok, err := h.connection(t.Context(), alice, LastFM); err != nil || !ok || c.token != "session-key" {
		t.Errorf("stored session: %q %v %v", c.token, ok, err)
	}
	// The session key is never handed out.
	if _, err := h.Key(t.Context(), alice, LastFM); !errors.Is(err, ErrUnknownService) {
		t.Errorf("session key revealed: %v", err)
	}

	// An API key Last.fm refuses is the administrator's to fix.
	h.f.reply(http.MethodGet, "/lastfm/2.0/", http.StatusForbidden, `{"error":10,"message":"Invalid API key - You must be granted a valid key by last.fm"}`)
	if _, err := h.StartCode(t.Context(), alice, LastFM); !errors.Is(err, ErrAppRefused) {
		t.Errorf("refused API key: %v", err)
	}
	// Without the API account, Last.fm is not offered.
	settings := h.store.Settings()
	settings.LastFMSecret = ""
	if _, err := h.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	if _, err := h.StartCode(t.Context(), alice, LastFM); !errors.Is(err, ErrNotAvailable) {
		t.Errorf("without a secret: %v", err)
	}
}

func TestListenBrainzIsConnectedWithAValidatedToken(t *testing.T) {
	h, _ := musicHarness(t)
	alice := h.user(t, "alice")
	h.f.on(http.MethodGet, "/listenbrainz/1/validate-token", func(r request) answer {
		if r.header.Get("Authorization") == "Token good-token" {
			return answer{status: http.StatusOK, body: `{"code":200,"message":"Token valid.","valid":true,"user_name":"alice-lb"}`}
		}
		return answer{status: http.StatusOK, body: `{"code":200,"message":"Token invalid.","valid":false}`}
	})
	if _, err := h.ConnectKey(t.Context(), alice, ListenBrainz, "bad-token"); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("invalid token: %v", err)
	}
	if h.status(t, alice, ListenBrainz).Connected {
		t.Error("an invalid token was kept")
	}
	status, err := h.ConnectKey(t.Context(), alice, ListenBrainz, " good-token ")
	if err != nil || !status.Connected || status.Account != "alice-lb" || !status.Music {
		t.Fatalf("connected: %+v %v", status, err)
	}
	if token, err := h.Key(t.Context(), alice, ListenBrainz); err != nil || token != "good-token" {
		t.Errorf("stored token: %q %v", token, err)
	}
	h.f.reply(http.MethodGet, "/listenbrainz/1/validate-token", http.StatusServiceUnavailable, ``)
	if _, err := h.ConnectKey(t.Context(), alice, ListenBrainz, "other-token"); !errors.Is(err, ErrUnreachable) {
		t.Errorf("down: %v", err)
	}
	// Music services have no history to import.
	if _, err := h.SetImport(t.Context(), alice, ListenBrainz, true); !errors.Is(err, ErrUnknownService) {
		t.Errorf("import: %v", err)
	}
}

func TestASongIsScrobbledOnceHalfwayThrough(t *testing.T) {
	h, c := musicHarness(t)
	alice := h.connectAll(t, "alice")
	h.Listen(alice, hear(Started, tune, 0, false))
	h.f.waitLastFM(t, 1, "track.updateNowPlaying")
	h.f.waitListens(t, 1, "playing_now")
	c.advance(50 * time.Second)
	h.Listen(alice, hear(Progressed, tune, 50, false))
	c.advance(49 * time.Second)
	h.Listen(alice, hear(Progressed, tune, 99, false))
	h.idle(t)
	if len(h.f.lastFMCalls("track.scrobble")) != 0 || len(h.f.listens("single")) != 0 {
		t.Fatal("scrobbled before half the song played")
	}
	c.advance(2 * time.Second)
	h.Listen(alice, hear(Progressed, tune, 101, false))
	c.advance(80 * time.Second)
	h.Listen(alice, hear(Stopped, tune, 181, false))
	h.idle(t)

	nowPlaying := h.f.lastFMCalls("track.updateNowPlaying")
	scrobbles := h.f.lastFMCalls("track.scrobble")
	if len(nowPlaying) != 1 || len(scrobbles) != 1 {
		t.Fatalf("Last.fm: %d now playing, %d scrobbles", len(nowPlaying), len(scrobbles))
	}
	for name, want := range map[string]string{"artist": "The Artist", "track": "A Song", "album": "An Album", "albumArtist": "Various",
		"trackNumber": "3", "duration": "200", "sk": "lastfm-alice", "api_key": "lastfm-key"} {
		if got := nowPlaying[0].Get(name); got != want {
			t.Errorf("now playing %s: %q, want %q", name, got, want)
		}
	}
	started := strconv.FormatInt(clockStart.Unix(), 10)
	for name, want := range map[string]string{"artist[0]": "The Artist", "track[0]": "A Song", "album[0]": "An Album",
		"albumArtist[0]": "Various", "trackNumber[0]": "3", "duration[0]": "200", "timestamp[0]": started, "sk": "lastfm-alice"} {
		if got := scrobbles[0].Get(name); got != want {
			t.Errorf("scrobble %s: %q, want %q", name, got, want)
		}
	}
	for _, call := range append(nowPlaying, scrobbles...) {
		if !signedByLastFMRules(call, "lastfm-secret") {
			t.Errorf("%s badly signed: %v", call.Get("method"), call)
		}
	}

	playing, singles := h.f.listens("playing_now"), h.f.listens("single")
	if len(playing) != 1 || len(singles) != 1 {
		t.Fatalf("ListenBrainz: %d playing now, %d listens", len(playing), len(singles))
	}
	metadata := `{"artist_name":"The Artist","track_name":"A Song","release_name":"An Album","additional_info":{"media_player":"Polyfin",` +
		`"media_player_version":"1.2.3","submission_client":"Polyfin","submission_client_version":"1.2.3","duration_ms":200000,"tracknumber":3,"isrc":"XX0000000001"}}`
	sameJSON(t, "playing now", playing[0], `{"listen_type":"playing_now","payload":[{"track_metadata":`+metadata+`}]}`)
	sameJSON(t, "listen", singles[0], `{"listen_type":"single","payload":[{"listened_at":`+started+`,"track_metadata":`+metadata+`}]}`)
	if r := h.f.sent("listenbrainz", "/1/submit-listens")[0]; r.header.Get("Authorization") != "Token listenbrainz-alice" {
		t.Errorf("ListenBrainz authorized with %q", r.header.Get("Authorization"))
	}
	if status := h.status(t, alice, LastFM); status.LastSentAt == nil {
		t.Errorf("Last.fm: %+v", status)
	}
}

func TestALongSongIsScrobbledAfterFourMinutes(t *testing.T) {
	h, c := musicHarness(t)
	alice := h.connectAll(t, "alice")
	long := tune
	long.DurationMS = (20 * time.Minute).Milliseconds()
	h.Listen(alice, hear(Started, long, 0, false))
	c.advance(239 * time.Second)
	h.Listen(alice, hear(Progressed, long, 239, false))
	h.idle(t)
	if len(h.f.lastFMCalls("track.scrobble")) != 0 {
		t.Fatal("scrobbled before 4 minutes")
	}
	c.advance(time.Second)
	h.Listen(alice, hear(Progressed, long, 240, false))
	h.f.waitLastFM(t, 1, "track.scrobble")
	h.f.waitListens(t, 1, "single")
}

func TestPausesAndSeeksDoNotCountAsPlayed(t *testing.T) {
	h, c := musicHarness(t)
	alice := h.connectAll(t, "alice")
	h.Listen(alice, hear(Started, tune, 0, false))
	// A seek past the middle 10 seconds in: 10 seconds played.
	c.advance(10 * time.Second)
	h.Listen(alice, hear(Progressed, tune, 150, false))
	// Paused 10 seconds later, for five minutes: 20 seconds played.
	c.advance(10 * time.Second)
	h.Listen(alice, hear(Progressed, tune, 160, true))
	c.advance(5 * time.Minute)
	h.Listen(alice, hear(Progressed, tune, 160, false))
	// A pause the player did not report: the song stays put for five
	// minutes.
	c.advance(10 * time.Second)
	h.Listen(alice, hear(Progressed, tune, 170, false))
	c.advance(5 * time.Minute)
	h.Listen(alice, hear(Progressed, tune, 170, false))
	// A seek back, then 20 seconds more: 50 seconds played in all, of a
	// song needing 100.
	c.advance(5 * time.Second)
	h.Listen(alice, hear(Progressed, tune, 0, false))
	c.advance(20 * time.Second)
	h.Listen(alice, hear(Stopped, tune, 20, false))
	// An app that sends no positions: only the time between its reports
	// while not paused counts, 30 and 30 seconds around a five-minute
	// pause.
	unplaced := func(event Event, paused bool) Listening {
		return Listening{Event: event, Device: device, Paused: paused, Song: tune}
	}
	h.Listen(alice, unplaced(Started, false))
	c.advance(30 * time.Second)
	h.Listen(alice, unplaced(Progressed, true))
	c.advance(5 * time.Minute)
	h.Listen(alice, unplaced(Progressed, false))
	c.advance(30 * time.Second)
	h.Listen(alice, unplaced(Stopped, false))
	h.idle(t)
	if n := len(h.f.lastFMCalls("track.scrobble")); n != 0 {
		t.Errorf("%d scrobbles of songs played 50 and 60 seconds", n)
	}
	if n := len(h.f.listens("single")); n != 0 {
		t.Errorf("%d listens of songs played 50 and 60 seconds", n)
	}
	// Played again from the start, the same song counts anew.
	h.Listen(alice, hear(Started, tune, 0, false))
	c.advance(100 * time.Second)
	h.Listen(alice, hear(Stopped, tune, 100, false))
	h.f.waitLastFM(t, 1, "track.scrobble")
	h.f.waitListens(t, 1, "single")
}

func TestShortSongsAreNotScrobbled(t *testing.T) {
	h, c := musicHarness(t)
	alice := h.connectAll(t, "alice")
	jingle := tune
	jingle.DurationMS = 25_000
	h.Listen(alice, hear(Started, jingle, 0, false))
	c.advance(25 * time.Second)
	h.Listen(alice, hear(Stopped, jingle, 25, false))
	h.idle(t)
	if len(h.f.lastFMCalls("track.updateNowPlaying")) != 1 || len(h.f.listens("playing_now")) != 1 {
		t.Error("the short song was not playing")
	}
	if len(h.f.lastFMCalls("track.scrobble")) != 0 || len(h.f.listens("single")) != 0 {
		t.Error("a 25-second song was scrobbled")
	}
}

func TestFailedScrobblesAreSentAgainOnce(t *testing.T) {
	h, c := musicHarness(t)
	alice := h.connectAll(t, "alice")
	var mu sync.Mutex
	failures := map[string]int{}
	fail := func(service string) bool {
		mu.Lock()
		defer mu.Unlock()
		failures[service]++
		return failures[service] == 1
	}
	h.f.on(http.MethodPost, "/lastfm/2.0/", func(r request) answer {
		if r.form.Get("method") == "track.scrobble" && fail(LastFM) {
			return answer{status: http.StatusServiceUnavailable, body: `{"error":16,"message":"There was a temporary error processing your request."}`}
		}
		return answer{status: http.StatusOK, body: `{"scrobbles":{"@attr":{"accepted":1,"ignored":0}}}`}
	})
	h.f.on(http.MethodPost, "/listenbrainz/1/submit-listens", func(r request) answer {
		if body, _ := r.body.(map[string]any); body["listen_type"] == "single" && fail(ListenBrainz) {
			return answer{status: http.StatusInternalServerError, body: `{"code":500,"error":"Something went wrong"}`}
		}
		return answer{status: http.StatusOK, body: `{"status":"ok"}`}
	})
	h.Listen(alice, hear(Started, tune, 0, false))
	c.advance(150 * time.Second)
	h.Listen(alice, hear(Stopped, tune, 150, false))
	scrobbles := h.f.waitLastFM(t, 2, "track.scrobble")
	singles := h.f.waitListens(t, 2, "single")
	h.idle(t)
	if len(h.f.lastFMCalls("track.scrobble")) != 2 || len(h.f.listens("single")) != 2 {
		t.Errorf("sent %d scrobbles and %d listens, want 2 each", len(h.f.lastFMCalls("track.scrobble")), len(h.f.listens("single")))
	}
	if scrobbles[1].Get("timestamp[0]") != strconv.FormatInt(clockStart.Unix(), 10) {
		t.Errorf("sent again with %q", scrobbles[1].Get("timestamp[0]"))
	}
	sameJSON(t, "listen sent again", singles[1], mustJSON(singles[0]))
	var queued int
	if err := h.db.QueryRow(t.Context(), "SELECT count(*) FROM tracking_events WHERE user_id = $1", alice).Scan(&queued); err != nil || queued != 0 {
		t.Errorf("%d changes left queued (%v)", queued, err)
	}
}

func TestARefusedSessionAsksToConnectAgain(t *testing.T) {
	h, c := musicHarness(t)
	alice := h.connectAll(t, "alice")
	h.f.on(http.MethodPost, "/lastfm/2.0/", func(r request) answer {
		if r.form.Get("method") == "track.scrobble" {
			return answer{status: http.StatusForbidden, body: `{"error":9,"message":"Invalid session key - Please re-authenticate"}`}
		}
		return answer{status: http.StatusOK, body: `{}`}
	})
	h.f.on(http.MethodPost, "/listenbrainz/1/submit-listens", func(r request) answer {
		if body, _ := r.body.(map[string]any); body["listen_type"] == "single" {
			return answer{status: http.StatusUnauthorized, body: `{"code":401,"error":"Invalid authorization token."}`}
		}
		return answer{status: http.StatusOK, body: `{"status":"ok"}`}
	})
	h.Listen(alice, hear(Started, tune, 0, false))
	c.advance(150 * time.Second)
	h.Listen(alice, hear(Stopped, tune, 150, false))
	h.f.waitLastFM(t, 1, "track.scrobble")
	h.f.waitListens(t, 1, "single")
	h.idle(t)
	for _, service := range []string{LastFM, ListenBrainz} {
		if status := h.status(t, alice, service); status.Problem != ProblemReconnect {
			t.Errorf("%s: %+v", service, status)
		}
	}
	// Nothing more goes to them, while the other services keep theirs.
	h.Listen(alice, hear(Started, tune, 0, false))
	c.advance(150 * time.Second)
	h.Listen(alice, hear(Stopped, tune, 150, false))
	h.Playback(alice, play(Started, movie, 0, false, false))
	h.f.wait(t, 1, "trakt", "/scrobble/start")
	h.idle(t)
	if len(h.f.lastFMCalls("track.updateNowPlaying")) != 1 || len(h.f.lastFMCalls("track.scrobble")) != 1 || len(h.f.listens("single")) != 1 {
		t.Error("sent to a refused connection")
	}
	if status := h.status(t, alice, Trakt); status.Problem != "" {
		t.Errorf("Trakt: %+v", status)
	}
}

func TestSongsAndMoviesGoToTheirOwnServicesOnly(t *testing.T) {
	h, c := musicHarness(t)
	alice := h.connectAll(t, "alice")
	h.connectAll(t, "bob")
	h.Playback(alice, play(Started, movie, 0, false, false))
	h.Playback(alice, play(Stopped, movie, 96, false, true))
	h.Listen(alice, hear(Started, tune, 0, false))
	c.advance(150 * time.Second)
	h.Listen(alice, hear(Stopped, tune, 150, false))
	h.f.wait(t, 1, "trakt", "/scrobble/stop")
	h.f.waitLastFM(t, 1, "track.scrobble")
	h.idle(t)
	for _, service := range []string{"trakt", "simkl", "mdblist"} {
		if n := len(h.f.sent(service, "/scrobble/start")); n != 1 {
			t.Errorf("%s: %d starts, want the movie's only", service, n)
		}
	}
	for _, r := range h.f.sent("lastfm", "/2.0/") {
		if strings.HasPrefix(r.form.Get("method"), "track.") && r.form.Get("track") != "A Song" && r.form.Get("track[0]") != "A Song" {
			t.Errorf("Last.fm got %v", r.form)
		}
		if r.form.Get("sk") != "lastfm-alice" {
			t.Errorf("Last.fm got another user's call: %v", r.form)
		}
	}
	if n := len(h.f.sent("listenbrainz", "/1/submit-listens")); n != 2 {
		t.Errorf("ListenBrainz: %d submissions, want the song's 2", n)
	}
	for _, r := range h.f.sent("listenbrainz", "/1/submit-listens") {
		if r.header.Get("Authorization") != "Token listenbrainz-alice" {
			t.Errorf("ListenBrainz got another user's call: %v", r.header)
		}
	}
}

func TestSongOfNeedsAnArtistAndATitle(t *testing.T) {
	album := accounts.ID{30}
	track := library.Item{ID: accounts.ID{20}, Kind: library.KindTrack, Name: "A Song", Album: "An Album", AlbumID: album, ParentID: album,
		IndexNumber: 3, Artists: []library.Credit{{Name: "The Artist"}}, AlbumArtist: &library.Credit{Name: "the artist"}}
	song, ok := SongOf(track, 200*time.Second)
	if !ok || song.Artist != "The Artist" || song.Title != "A Song" || song.TrackNumber != 3 || song.AlbumArtist != "" || song.DurationMS != 200_000 {
		t.Errorf("album track: %+v %v", song, ok)
	}
	// Listed in a playlist, its index is not a track number.
	listed := track
	listed.ParentID = accounts.ID{40}
	if song, _ := SongOf(listed, 0); song.TrackNumber != 0 {
		t.Errorf("playlist entry numbered %d", song.TrackNumber)
	}
	for name, item := range map[string]library.Item{
		"no artist":  {ID: track.ID, Kind: library.KindTrack, Name: "A Song"},
		"no title":   {ID: track.ID, Kind: library.KindTrack, Name: " ", Artists: track.Artists},
		"audiobook":  {ID: track.ID, Kind: library.KindAudiobook, Name: "A Book", Artists: track.Artists},
		"not a song": {ID: track.ID, Kind: library.KindMovie, Name: "A Movie", Artists: track.Artists},
	} {
		if _, ok := SongOf(item, time.Minute); ok {
			t.Errorf("%s: identified", name)
		}
	}
}
