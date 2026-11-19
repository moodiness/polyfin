package jellyfin

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
)

// servedSetup is a member signed in on three devices, tv, phone and
// tablet, none of which reports anything, with the address each asks the
// movie's analyzed version at.
type servedSetup struct {
	playbackSetup
	tokens  map[string]string
	version string
}

func served(t *testing.T) servedSetup {
	t.Helper()
	p := playing(t)
	return servedSetup{playbackSetup: p, version: p.versions[1].ID.String(),
		tokens: map[string]string{"tv": p.token, "phone": p.signIn("member", "phone"), "tablet": p.signIn("member", "tablet")}}
}

// stream asks the version as its file from a device, with its token
// alone, and returns the status.
func (s servedSetup) stream(device string) int {
	s.t.Helper()
	status, _ := s.call(http.MethodGet, "/Videos/"+s.movie+"/stream.mp4?static=true&mediaSourceId="+s.version+"&api_key="+s.tokens[device], "", nil)
	return status
}

// plays reports whether a device's stream was served, and fails for an
// answer that is neither the file nor the refusal of a limit reached.
func (s servedSetup) plays(t *testing.T, device string) bool {
	t.Helper()
	switch status := s.stream(device); status {
	case http.StatusOK:
		return true
	case http.StatusTooManyRequests:
		return false
	default:
		t.Fatalf("the stream of the %s: %d", device, status)
		return false
	}
}

// offered reports whether PlaybackInfo offers a device the version, and
// fails for an answer that is neither that nor RateLimitExceeded.
func (s servedSetup) offered(t *testing.T, device string) (playbackAnswer, bool) {
	t.Helper()
	answer := s.playbackInfo(t, device, s.tokens[device], s.movie, map[string]any{"MediaSourceId": s.version})
	if answer.ErrorCode == "RateLimitExceeded" && len(answer.MediaSources) == 0 {
		return answer, false
	}
	if answer.ErrorCode != "" || len(answer.MediaSources) == 0 || answer.PlaySessionId == "" {
		t.Fatalf("PlaybackInfo from the %s: %+v", device, answer)
	}
	return answer, true
}

// device is the identifier of a device of the member.
func (s servedSetup) device(t *testing.T, name string) accounts.ID {
	t.Helper()
	devices, err := s.store.Devices(t.Context(), s.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, device := range devices {
		if device.DeviceName == name {
			return device.ID
		}
	}
	t.Fatalf("no device %s among %+v", name, devices)
	return accounts.ID{}
}

// places is how many places are kept, held or not.
func (s servedSetup) places() int {
	s.handler.served.mu.Lock()
	defer s.handler.served.mu.Unlock()
	return len(s.handler.served.places)
}

// At a limit of one, a device that is sent the file holds the place
// though no app reports anything: PlaybackInfo, the stream, the HLS
// playlists and segments, the audio route and a converted download all
// refuse a second device, with or without a PlaybackInfo before, while
// the first goes on with every one of them. Two minutes after the last
// answer to the first, the second plays, and the first is the one refused.
// Nothing is kept once nobody is sent anything.
func TestPlaybacksAtOnceCountDevicesThatReportNothing(t *testing.T) {
	t.Parallel()
	s := served(t)
	advance := idleClock(s.testServer)
	s.setting(t, func(settings *accounts.Settings) { settings.ConvertedDownloads = true })
	s.limit(t, s.user, func(c *accounts.UserChanges) { c.MaxPlaybacks = new(1) })
	// Each device asks PlaybackInfo while nothing plays: both are offered
	// the version, and hold a play session from then on.
	sessions := map[string]string{}
	for _, device := range []string{"tv", "phone"} {
		answer, ok := s.offered(t, device)
		if !ok {
			t.Fatalf("PlaybackInfo refused the %s while nothing played", device)
		}
		sessions[device] = answer.PlaySessionId
	}
	if n := s.places(); n != 0 {
		t.Fatalf("%d places kept after PlaybackInfo alone, want none: asking is not playing", n)
	}
	// What a device asks of the routes that send media, each with the
	// status its own playback gets once the limit let it through: a
	// segment in another container than the playlist's is not found, the
	// movie is no track, and HEAD of a converted download sends nothing.
	asks := func(device string) map[string]int {
		t.Helper()
		query := "?playSessionId=" + url.QueryEscape(sessions[device]) + "&mediaSourceId=" + s.version
		token := "&ApiKey=" + s.tokens[device]
		statuses := map[string]int{}
		for name, request := range map[string]struct{ method, path string }{
			"master playlist":    {http.MethodGet, "/Videos/" + s.movie + "/master.m3u8" + query},
			"media playlist":     {http.MethodGet, "/Videos/" + s.movie + "/main.m3u8" + query},
			"segment":            {http.MethodGet, "/Videos/" + s.movie + "/hls1/main/0.none" + query},
			"live segment":       {http.MethodGet, "/Videos/" + s.movie + "/hls/live/0.ts" + query},
			"audio":              {http.MethodGet, "/Audio/" + s.movie + "/stream.mp3?static=true" + token},
			"universal audio":    {http.MethodGet, "/Audio/" + s.movie + "/universal?container=mp3" + token},
			"converted download": {http.MethodHead, "/Videos/" + s.movie + "/stream.mp4" + query + "&videoCodec=h264" + token},
		} {
			statuses[name], _ = s.call(request.method, request.path, "", nil)
		}
		return statuses
	}
	own := map[string]int{"segment": http.StatusNotFound, "live segment": http.StatusNotFound, "audio": http.StatusNotFound,
		"universal audio": http.StatusNotFound, "converted download": http.StatusOK}

	if !s.plays(t, "tv") {
		t.Fatal("the tv's stream was refused while nothing played")
	}
	member, err := s.store.User(t.Context(), s.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Guests and DLNA devices are admitted on this count.
	if playing, err := s.handler.devicesPlaying(t.Context(), member, accounts.ID{}); err != nil || playing != 1 {
		t.Errorf("%d devices playing while the tv is sent the file (%v), want 1", playing, err)
	}
	if playing, err := s.handler.devicesPlaying(t.Context(), member, s.device(t, "tv")); err != nil || playing != 0 {
		t.Errorf("%d devices playing but the tv (%v), want 0", playing, err)
	}
	if _, ok := s.offered(t, "phone"); ok {
		t.Error("PlaybackInfo offered the phone a playback while the tv, which reports nothing, was sent the file")
	}
	if s.plays(t, "phone") {
		t.Error("the phone's stream was served while the tv was sent the file, at a limit of 1")
	}
	for name, status := range asks("phone") {
		if status != http.StatusTooManyRequests {
			t.Errorf("the phone's %s while the tv was sent the file: %d, want 429", name, status)
		}
	}
	// The tv goes on: another range of its file, another PlaybackInfo,
	// and every route, none refused by its own playback.
	if response, _ := fetchURL(t, s.url+"/Videos/"+s.movie+"/stream.mp4?static=true&mediaSourceId="+s.version+"&api_key="+s.tokens["tv"],
		http.Header{"Range": {"bytes=4-"}}); response.StatusCode != http.StatusPartialContent {
		t.Errorf("the tv's next range of its own stream: %d", response.StatusCode)
	}
	if _, ok := s.offered(t, "tv"); !ok {
		t.Error("PlaybackInfo refused the tv, the device playing")
	}
	for name, status := range asks("tv") {
		if want, exact := own[name]; status == http.StatusTooManyRequests || exact && status != want {
			t.Errorf("the tv's own %s: %d, want %d (0: anything but 429)", name, status, want)
		}
	}
	// A second short of two minutes after the last answer to the tv, it
	// still holds the place; at two minutes it holds it no more.
	advance(servedStale - time.Second)
	if s.plays(t, "phone") {
		t.Error("the phone's stream was served a second before the tv's place ended")
	}
	advance(time.Second)
	if !s.plays(t, "phone") {
		t.Fatal("the phone's stream was refused two minutes after the last answer to the tv")
	}
	if s.plays(t, "tv") {
		t.Error("the tv's stream was served while the phone, which took the place, was sent the file")
	}
	if n := s.places(); n != 1 {
		t.Errorf("%d places kept while the phone alone is sent the file, want 1", n)
	}
	advance(servedStale)
	if playing, err := s.handler.devicesPlaying(t.Context(), member, accounts.ID{}); err != nil || playing != 0 {
		t.Errorf("%d devices playing two minutes after the last answer (%v), want 0", playing, err)
	}
	if n := s.places(); n != 0 {
		t.Errorf("%d places kept once nobody is sent anything, want none", n)
	}
}

// A device whose app reports its playback holds the place by its reports
// alone, on the media routes as in PlaybackInfo; its stop report frees
// the place at once, though it was sent the file a moment before, so that
// the user goes on from it on another device.
func TestAStopReportFreesTheDevicesPlaceAtOnce(t *testing.T) {
	t.Parallel()
	s := served(t)
	s.limit(t, s.user, func(c *accounts.UserChanges) { c.MaxPlaybacks = new(1) })
	report := func(path string) {
		t.Helper()
		body := map[string]any{"ItemId": s.movie, "MediaSourceId": s.version, "PositionTicks": 0, "PlayMethod": "DirectPlay"}
		if status, data := s.call(http.MethodPost, path, app("tv", s.tokens["tv"]), body); status != http.StatusNoContent {
			t.Fatalf("%s from the tv: %d %s", path, status, data)
		}
	}
	report("/Sessions/Playing")
	if s.plays(t, "phone") {
		t.Fatal("the phone's stream was served while the tv reported playing, at a limit of 1")
	}
	if !s.plays(t, "tv") {
		t.Fatal("the tv's stream was refused, though it is the device playing")
	}
	report("/Sessions/Playing/Stopped")
	if !s.plays(t, "phone") {
		t.Error("the phone's stream was refused right after the tv reported its stop")
	}
	// The phone reports nothing: it holds the place it was sent the file
	// at, and the tv, which stopped, is the one refused.
	if _, ok := s.offered(t, "tv"); ok {
		t.Error("PlaybackInfo offered the tv a playback while the phone was sent the file")
	}
}

// An answer still being sent keeps its device's place however long it
// lasts, as a direct play's does while its app reads; once it ends the
// place lasts two minutes more. The files of a relayed playlist ask for
// the place of the device their grant was given to.
func TestAnAnswerBeingSentKeepsItsDevicesPlace(t *testing.T) {
	t.Parallel()
	s := served(t)
	advance := idleClock(s.testServer)
	s.limit(t, s.user, func(c *accounts.UserChanges) { c.MaxPlaybacks = new(1) })
	member, err := s.store.User(t.Context(), s.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	tv, phone := s.device(t, "tv"), s.device(t, "phone")
	// The other movie's version is known as an HLS playlist, whose files
	// Polyfin relays: one of them is asked with a grant given to a device.
	other := s.versionsOf(t, s.remote)[1]
	s.storeAnalysis(t, other, media.Analysis{Format: "hls", Remote: true})
	file := func(device accounts.ID) int {
		t.Helper()
		signer := s.handler.Playback.Signer()
		// Signed as the handler signs, with its time: a grant that tells
		// none is refused.
		grant, target := s.handler.grant(playback.Grant{Version: other.ID, User: member.ID, Device: device, Relay: true}), s.url+"/System/Ping"
		status, _ := s.call(http.MethodGet, "/Videos/"+s.remote+"/live/file.ts?"+url.Values{grantParameter: {grant},
			"target": {base64.RawURLEncoding.EncodeToString([]byte(target))}, "sign": {signer.Link(grant, target)}}.Encode(), "", nil)
		return status
	}
	// An answer to the tv that lasts until the test ends it.
	sending, end, ended := make(chan struct{}), make(chan struct{}), make(chan struct{})
	answer := s.handler.meteredFile(func(w http.ResponseWriter, r *http.Request) {
		if !s.handler.admitPlay(w, r, member, tv) {
			return
		}
		w.WriteHeader(http.StatusOK)
		close(sending)
		<-end
	})
	go func() {
		defer close(ended)
		answer(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/Videos/"+s.movie+"/stream.mp4", nil))
	}()
	select {
	case <-sending:
	case <-ended:
		t.Fatal("the tv's answer was refused while nothing played")
	}
	advance(time.Hour)
	if s.plays(t, "phone") {
		t.Error("the phone's stream was served while an answer was being sent to the tv for an hour")
	}
	if status := file(phone); status != http.StatusTooManyRequests {
		t.Errorf("a relayed playlist's file for the phone while the tv is sent an answer: %d, want 429", status)
	}
	if status := file(tv); status == http.StatusTooManyRequests {
		t.Error("a relayed playlist's file for the tv was refused by the tv's own playback")
	}
	close(end)
	<-ended
	advance(servedStale - time.Second)
	if s.plays(t, "phone") {
		t.Error("the phone's stream was served a second before the tv's place ended")
	}
	advance(time.Second)
	if !s.plays(t, "phone") {
		t.Error("the phone's stream was refused two minutes after the tv's answer ended")
	}
}

// Without a limit nothing is counted and nothing kept: a member and an
// administrator play on every device at once, as before. A user's limit
// counts their own devices alone.
func TestOnlyUsersWithALimitOfPlaybacksAtOnceAreCounted(t *testing.T) {
	t.Parallel()
	s := served(t)
	s.testServer.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	s.testServer.user("limited", func(c *accounts.UserChanges) { c.MaxPlaybacks = new(1) })
	s.tokens["admin tv"], s.tokens["admin phone"] = s.signIn("admin", "admin tv"), s.signIn("admin", "admin phone")
	s.tokens["limited tv"], s.tokens["limited phone"] = s.signIn("limited", "limited tv"), s.signIn("limited", "limited phone")
	for _, device := range []string{"tv", "phone", "tablet", "admin tv", "admin phone"} {
		if !s.plays(t, device) {
			t.Errorf("the stream of the %s was refused, though its user has no limit", device)
		}
	}
	if n := s.places(); n != 0 {
		t.Errorf("%d places kept for users without a limit, want none", n)
	}
	// Five devices of others are sent the file: none counts for this user.
	if !s.plays(t, "limited tv") {
		t.Error("a user's first device was refused at a limit of 1 while other users played")
	}
	if s.plays(t, "limited phone") {
		t.Error("a user's second device was served at a limit of 1")
	}
	if n := s.places(); n != 1 {
		t.Errorf("%d places kept, want the one of the user with a limit", n)
	}
	for _, device := range []string{"phone", "admin phone"} {
		if !s.plays(t, device) {
			t.Errorf("the stream of the %s was refused by another user's limit", device)
		}
	}
}

// Of many devices asking for a user's last place at the same instant, one
// gets it, and it keeps it for its own next answers; a device whose app
// reports playing is not counted twice; an answer that failed keeps no
// place, nor does one that began before its app's stop report.
func TestAPlaceIsTakenByOneDeviceAtATime(t *testing.T) {
	t.Parallel()
	plays, user, now := newServedPlays(), accounts.ID{1}, time.Unix(1_700_000_000, 0)
	key := func(device byte) servedKey { return servedKey{user: user, device: accounts.ID{device}} }
	var admitted atomic.Int32
	var winner atomic.Value
	var wg sync.WaitGroup
	for device := byte(1); device <= 50; device++ {
		wg.Go(func() {
			if answer, ok := plays.admit(key(device), 1, nil, now); ok {
				admitted.Add(1)
				winner.Store(answer)
			}
		})
	}
	wg.Wait()
	if n := admitted.Load(); n != 1 {
		t.Fatalf("%d devices of 50 got the last place, want 1", n)
	}
	first := winner.Load().(servedAnswer)
	again, ok := plays.enter(first.key, now)
	if !ok {
		t.Fatal("the device that holds the place was not let in for its next answer")
	}
	other := key(51)
	if other == first.key {
		other = key(52)
	}
	if _, ok := plays.enter(other, now); ok {
		t.Error("a device that holds no place was let in without being counted")
	}
	// The other user's devices, and a device its app reports playing, take
	// no room.
	if n := plays.playing(accounts.ID{2}, accounts.ID{}, nil, now); n != 0 {
		t.Errorf("%d devices of another user playing, want 0", n)
	}
	if n := plays.playing(user, accounts.ID{}, []accounts.ID{first.key.device}, now); n != 0 {
		t.Errorf("%d devices playing beside the one its app reports, want 0: it is the same device", n)
	}
	// Its app reports a stop while both answers are being sent: the place
	// lasts as long as they do, whatever their outcome.
	plays.stopped(user, first.key.device)
	plays.leave(first, true, now)
	if n := plays.playing(user, accounts.ID{}, nil, now); n != 1 {
		t.Errorf("%d devices playing while an answer is still being sent after a stop report, want 1", n)
	}
	plays.leave(again, true, now)
	if n := plays.playing(user, accounts.ID{}, nil, now); n != 0 {
		t.Errorf("%d devices playing once the answers begun before a stop report ended, want 0", n)
	}
	// An answer that failed keeps no place; one that went out keeps it
	// for servedStale, to the second.
	failed, ok := plays.admit(other, 1, nil, now)
	if !ok {
		t.Fatal("a device was refused the place nobody held")
	}
	plays.leave(failed, false, now)
	if n := len(plays.places); n != 0 {
		t.Errorf("%d places kept after an answer that failed, want none", n)
	}
	sent, _ := plays.admit(other, 1, nil, now)
	plays.leave(sent, true, now)
	if n := plays.playing(user, accounts.ID{}, nil, now.Add(servedStale-time.Second)); n != 1 {
		t.Errorf("%d devices playing a second before the place ends, want 1", n)
	}
	if n := plays.playing(user, accounts.ID{}, nil, now.Add(servedStale)); n != 0 || len(plays.places) != 0 {
		t.Errorf("%d devices playing and %d places kept two minutes after the answer, want none", n, len(plays.places))
	}
}
