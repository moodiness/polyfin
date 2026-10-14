package jellyfin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/media"
)

// shareSetup is a member allowed to share, on a server that allows it,
// with the share routes served beside the Jellyfin API.
type shareSetup struct {
	playbackSetup
	site string
	// played counts the share_played notifications.
	played *atomic.Int32
	// analysis is the movie's first version's, which streams over HLS.
	analysis media.Analysis
}

func sharing(t *testing.T) shareSetup {
	t.Helper()
	p := playing(t)
	analysis := p.remuxable(t)
	p.setting(t, func(settings *accounts.Settings) { settings.ShareLinks = true })
	p.limit(t, p.user, func(c *accounts.UserChanges) { c.CanShare = new(true) })
	played := new(atomic.Int32)
	// Set before any request: the handler reads it as guests play.
	p.handler.SharePlayed = func(accounts.ShareLink) { played.Add(1) }
	mux := http.NewServeMux()
	mux.Handle("/share/", p.handler.Share())
	mux.Handle("/", p.handler)
	site := httptest.NewServer(mux)
	t.Cleanup(site.Close)
	return shareSetup{playbackSetup: p, site: site.URL, played: played, analysis: analysis}
}

// link makes a link to the movie for viewers at once, and returns its
// token and ID.
func (s shareSetup) link(t *testing.T, viewers int) (string, accounts.ID) {
	t.Helper()
	item, _ := accounts.ParseID(s.movie)
	link, token, err := s.store.CreateShareLink(t.Context(), accounts.NewShareLink{User: s.user.ID, Item: item, Kind: accounts.ShareMovie,
		Title: "Movie (2008)", Expiry: accounts.DefaultShareLinkExpiry, MaxViewers: viewers})
	if err != nil {
		t.Fatal(err)
	}
	return token, link.ID
}

// noRedirects is a client that reports a redirect rather than following
// it: no guest route ever sends one, to a source least of all.
var noRedirects = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

// guest sends a request as a guest's browser does, without credentials.
func (s shareSetup) guest(t *testing.T, method, path string, body any) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = strings.NewReader(string(data))
	}
	request, _ := http.NewRequestWithContext(t.Context(), method, s.site+path, reader)
	response, err := noRedirects.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		t.Errorf("%s %s redirected: %d to %q", method, path, response.StatusCode, response.Header.Get("Location"))
	}
	return response.StatusCode, string(data)
}

// start starts a guest's playback, and returns its play session.
func (s shareSetup) start(t *testing.T, token string) (int, string) {
	t.Helper()
	status, body := s.guest(t, http.MethodPost, "/share/"+token+"/play", nil)
	var answer struct{ Session, Error string }
	_ = json.Unmarshal([]byte(body), &answer)
	if status == http.StatusOK {
		return status, answer.Session
	}
	return status, answer.Error
}

// The page shows its title and no other; the guest plays over HLS through
// Polyfin, and no response redirects.
func TestAShareLinkOpensItsTitleAlone(t *testing.T) {
	t.Parallel()
	s := sharing(t)
	token, _ := s.link(t, 1)
	status, page := s.guest(t, http.MethodGet, "/share/"+token, nil)
	if status != http.StatusOK || !strings.Contains(page, "<h1>Movie</h1>") || strings.Contains(page, "Remote") ||
		strings.Contains(page, s.remote) {
		t.Fatalf("page: %d\n%s", status, page)
	}
	status, session := s.start(t, token)
	if status != http.StatusOK || !strings.HasPrefix(session, "share") {
		t.Fatalf("start: %d %s", status, session)
	}
	if s.played.Load() != 1 {
		t.Errorf("share_played told %d times", s.played.Load())
	}
	base := "/share/" + token + "/play/" + session + "/"
	status, master := s.guest(t, http.MethodGet, base+"master.m3u8", nil)
	if status != http.StatusOK || !strings.Contains(master, "#EXTM3U") || strings.Contains(master, "http") || strings.Contains(master, "?") {
		t.Fatalf("master playlist: %d\n%s", status, master)
	}
	status, media := s.guest(t, http.MethodGet, base+"main.m3u8", nil)
	if status != http.StatusOK || !strings.Contains(media, "hls1/main/") {
		t.Fatalf("media playlist: %d\n%s", status, media)
	}
	// The play session is signed for no one: the API's own HLS routes
	// refuse it.
	if status, _ := s.guest(t, http.MethodGet, "/Videos/"+s.movie+"/master.m3u8?playSessionId="+session, nil); status != http.StatusUnauthorized {
		t.Errorf("the API's playlist with a guest's session: %d", status)
	}
	// The guest cannot have the title downloaded, nor another title.
	for _, path := range []string{"/share/" + token + "/play/" + session + "/stream.mkv", "/share/" + token + "/download",
		"/share/" + token + "/play/" + session + "/hls1/main/notanumber.mp4"} {
		if status, _ := s.guest(t, http.MethodGet, path, nil); status < 400 {
			t.Errorf("%s: %d", path, status)
		}
	}
	// Another play within the hour is not told again.
	if status, _ := s.guest(t, http.MethodPost, base+"stop", nil); status != http.StatusNoContent {
		t.Errorf("stop: %d", status)
	}
	if status, _ := s.start(t, token); status != http.StatusOK || s.played.Load() != 1 {
		t.Errorf("a second play: %d, told %d times", status, s.played.Load())
	}
	if link, err := s.store.ShareLinks(t.Context(), &s.user.ID); err != nil || len(link) != 1 || link[0].Plays != 2 {
		t.Errorf("plays counted: %+v %v", link, err)
	}
}

// A share token opens none of the API's routes, however it is sent.
func TestOtherRoutesRefuseAShareToken(t *testing.T) {
	t.Parallel()
	s := sharing(t)
	token, _ := s.link(t, 1)
	for _, path := range []string{"/Users/Me", "/Items?Recursive=true", "/UserViews", "/Items/" + s.movie + "/PlaybackInfo", "/Sessions"} {
		for name, send := range map[string]func(*http.Request){
			"api_key":       func(r *http.Request) { r.URL.RawQuery += "&api_key=" + token },
			"X-Emby-Token":  func(r *http.Request) { r.Header.Set("X-Emby-Token", token) },
			"Authorization": func(r *http.Request) { r.Header.Set("Authorization", app("browser", token)) },
		} {
			request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, s.site+path, nil)
			send(request)
			response, err := noRedirects.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusUnauthorized {
				t.Errorf("%s with the token as %s: %d", path, name, response.StatusCode)
			}
		}
	}
	// Admin and Polyfin routes take none either.
	if status, _ := s.guest(t, http.MethodGet, "/Polyfin/Items/"+s.movie+"/Share?api_key="+token, nil); status != http.StatusUnauthorized {
		t.Errorf("the script's route with a share token: %d", status)
	}
}

// With the switch off, without the permission, revoked or expired, a link
// answers 410 and plays nothing, the playback under way included; an
// unknown token answers 404.
func TestRefusedShareLinksPlayNothing(t *testing.T) {
	t.Parallel()
	s := sharing(t)
	token, id := s.link(t, 1)
	_, session := s.start(t, token)
	master := "/share/" + token + "/play/" + session + "/master.m3u8"
	refused := func(why string, code string) {
		t.Helper()
		if status, page := s.guest(t, http.MethodGet, "/share/"+token, nil); status != http.StatusGone || strings.Contains(page, "<video") {
			t.Errorf("%s: page %d", why, status)
		}
		if status, answer := s.start(t, token); status != http.StatusGone || answer != code {
			t.Errorf("%s: start %d %s", why, status, answer)
		}
		if status, _ := s.guest(t, http.MethodGet, master, nil); status != http.StatusGone {
			t.Errorf("%s: playlist %d", why, status)
		}
		if status, _ := s.guest(t, http.MethodGet, "/share/"+token+"/image/primary", nil); status != http.StatusGone {
			t.Errorf("%s: artwork %d", why, status)
		}
	}
	s.setting(t, func(settings *accounts.Settings) { settings.ShareLinks = false })
	refused("switch off", "off")
	s.setting(t, func(settings *accounts.Settings) { settings.ShareLinks = true })
	if status, _ := s.guest(t, http.MethodGet, "/share/"+token, nil); status != http.StatusOK {
		t.Errorf("switched on again: %d", status)
	}
	if _, err := s.pool.Exec(t.Context(), "UPDATE share_links SET expires_at = now() - interval '1 minute' WHERE id = $1", id); err != nil {
		t.Fatal(err)
	}
	refused("expired", "expired")
	other, otherID := s.link(t, 1)
	if _, _, err := s.store.RevokeShareLink(t.Context(), otherID, nil); err != nil {
		t.Fatal(err)
	}
	token = other
	master = "/share/" + token + "/play/" + session + "/master.m3u8"
	refused("revoked", "revoked")
	third, _ := s.link(t, 1)
	s.limit(t, s.user, func(c *accounts.UserChanges) { c.CanShare = new(false) })
	token = third
	refused("permission taken", "revoked")
	if status, _ := s.guest(t, http.MethodGet, "/share/"+strings.Repeat("A", 64), nil); status != http.StatusNotFound {
		t.Errorf("an unknown token: %d", status)
	}
}

// Guests count toward the sharer's playbacks at once, and a link takes as
// many guests as it allows.
func TestGuestsCountAsTheSharersPlaybacks(t *testing.T) {
	t.Parallel()
	s := sharing(t)
	token, _ := s.link(t, 2)
	single, _ := s.link(t, 1)
	if status, _ := s.start(t, single); status != http.StatusOK {
		t.Fatalf("first guest: %d", status)
	}
	if status, why := s.start(t, single); status != http.StatusTooManyRequests || why != "viewers" {
		t.Errorf("a second guest on a link for one: %d %s", status, why)
	}
	s.limit(t, s.user, func(c *accounts.UserChanges) { c.MaxPlaybacks = new(1) })
	if status, why := s.start(t, token); status != http.StatusTooManyRequests || why != "limit" {
		t.Errorf("a guest past the sharer's limit: %d %s", status, why)
	}
	if answer := s.playbackInfo(t, "tv", s.token, s.movie, nil); answer.ErrorCode != "RateLimitExceeded" {
		t.Errorf("the sharer's own device while a guest plays: %+v", answer)
	}
	s.limit(t, s.user, func(c *accounts.UserChanges) { c.MaxPlaybacks = new(2) })
	if status, _ := s.start(t, token); status != http.StatusOK {
		t.Errorf("a guest under a raised limit: %d", status)
	}
}

// A guest's playback changes nothing of the sharer's watch state.
func TestGuestsLeaveTheSharersResumePointAlone(t *testing.T) {
	t.Parallel()
	s := sharing(t)
	if status, _ := s.call(http.MethodPost, "/Sessions/Playing/Stopped", app("tv", s.token),
		map[string]any{"ItemId": s.movie, "PositionTicks": 36_000_000_000}); status != http.StatusNoContent {
		t.Fatalf("the sharer's report: %d", status)
	}
	resume := func() UserItemData {
		t.Helper()
		var item BaseItemDto
		s.get(t, "/Users/"+s.user.ID.String()+"/Items/"+s.movie, s.token, &item)
		return item.UserData
	}
	before := resume()
	if before.PlaybackPositionTicks != 36_000_000_000 {
		t.Fatalf("resume point: %+v", before)
	}
	token, _ := s.link(t, 1)
	_, session := s.start(t, token)
	base := "/share/" + token + "/play/" + session
	if status, _ := s.guest(t, http.MethodPost, base+"/progress", map[string]any{"position": 3000, "paused": false}); status != http.StatusNoContent {
		t.Errorf("progress: %d", status)
	}
	if status, _ := s.guest(t, http.MethodPost, base+"/stop", nil); status != http.StatusNoContent {
		t.Errorf("stop: %d", status)
	}
	if after := resume(); after.PlaybackPositionTicks != before.PlaybackPositionTicks || after.Played != before.Played ||
		after.PlayCount != before.PlayCount {
		t.Errorf("the guest changed the sharer's data: %+v, was %+v", after, before)
	}
}

// Video sent to guests is converted down to 1080p at most, and to the
// sharer's quality group; a version taller than either is never copied.
func TestGuestsGetVideoUnder1080pAndTheSharersGroup(t *testing.T) {
	t.Parallel()
	if os.Getenv("POLYFIN_TEST_FFMPEG") == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set: Polyfin converts video with the encoders FFmpeg has")
	}
	s := sharing(t)
	tall := s.analysis
	tall.Streams = slices.Clone(tall.Streams)
	for i := range tall.Streams {
		if tall.Streams[i].Type == "video" {
			tall.Streams[i].Width, tall.Streams[i].Height = 3840, 2160
		}
	}
	// Both versions are 2160p: none fits 1080p, and the first, which
	// streams over HLS, plays converted down.
	s.storeAnalysis(t, s.versions[0], tall)
	s.storeAnalysis(t, s.versions[1], tall)
	resolution := regexp.MustCompile(`RESOLUTION=(\d+)x(\d+)`)
	sent := func(token string) (int, string) {
		t.Helper()
		status, session := s.start(t, token)
		if status != http.StatusOK {
			return status, session
		}
		var params string
		for _, play := range s.handler.guests.all() {
			if play.id == session {
				params = play.params
			}
		}
		if !strings.Contains(params, "allowVideoStreamCopy=false") {
			t.Errorf("video copied for a guest: %s", params)
		}
		// The guest's query changes nothing of what is sent.
		_, master := s.guest(t, http.MethodGet, "/share/"+token+"/play/"+session+"/master.m3u8?allowVideoStreamCopy=true&videoCodec=hevc", nil)
		match := resolution.FindStringSubmatch(master)
		if match == nil {
			t.Fatalf("no resolution in\n%s", master)
		}
		height, _ := strconv.Atoi(match[2])
		_, _ = s.guest(t, http.MethodPost, "/share/"+token+"/play/"+session+"/stop", nil)
		return status, strconv.Itoa(height)
	}
	token, _ := s.link(t, 1)
	if status, height := sent(token); status != http.StatusOK || height != "1080" {
		t.Errorf("without a group: %d, %sp", status, height)
	}
	s.group(t, s.user, 720)
	if status, height := sent(token); status != http.StatusOK || height != "720" {
		t.Errorf("in a 720p group: %d, %sp", status, height)
	}
	// A sharer who may not have video converted shares nothing taller.
	s.permit(t, s.user, false, true, false)
	if status, why := s.start(t, token); status == http.StatusOK {
		t.Errorf("a tall version without conversion: %d %s", status, why)
	}
}

// Guests' playbacks end once they ask for nothing for a while.
func TestStaleGuestsEnd(t *testing.T) {
	t.Parallel()
	s := sharing(t)
	token, _ := s.link(t, 1)
	if status, _ := s.start(t, token); status != http.StatusOK {
		t.Fatal(status)
	}
	stale := time.Now().Add(-guestStale - time.Second)
	s.handler.guests.mu.Lock()
	for _, play := range s.handler.guests.plays {
		play.seen = stale
	}
	s.handler.guests.mu.Unlock()
	s.handler.endStaleGuests()
	if status, _ := s.start(t, token); status != http.StatusOK {
		t.Errorf("a guest after a stale one: %d", status)
	}
}
