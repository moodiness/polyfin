package jellyfin

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// idleClock has the server, and the sessions it keeps, read a clock that
// only moves on through the function returned.
func idleClock(s testServer) func(time.Duration) {
	var clock atomic.Pointer[time.Time]
	start := time.Now()
	clock.Store(&start)
	s.handler.now = func() time.Time { return *clock.Load() }
	return func(d time.Duration) {
		next := clock.Load().Add(d)
		clock.Store(&next)
	}
}

// reportAt sends a playback report of the movie from a device, at a
// position, paused or not, naming no version: the movie's runtime is the
// one positions are measured in.
func (p playbackSetup) reportAt(t *testing.T, device, token, path string, position time.Duration, paused bool) {
	t.Helper()
	body := map[string]any{"ItemId": p.movie, "PositionTicks": position.Nanoseconds() / 100,
		"IsPaused": paused, "PlayMethod": "DirectPlay"}
	if status, data := p.call(http.MethodPost, path, app(device, token), body); status != http.StatusNoContent {
		t.Fatalf("%s from %s: %d %s", path, device, status, data)
	}
}

// playsNow tells whether PlaybackInfo lets a device play the movie, rather
// than refusing it for the user's Playbacks at once.
func (p playbackSetup) playsNow(t *testing.T, device, token string) bool {
	t.Helper()
	answer := p.playbackInfo(t, device, token, p.movie, nil)
	if answer.ErrorCode == "RateLimitExceeded" && len(answer.MediaSources) == 0 {
		return false
	}
	if answer.ErrorCode != "" || len(answer.MediaSources) == 0 {
		t.Fatalf("PlaybackInfo from %s: %+v", device, answer)
	}
	return true
}

// playingDevices are the devices /Sessions shows playing.
func (p playbackSetup) playingDevices(t *testing.T) map[string]bool {
	t.Helper()
	playing := map[string]bool{}
	for _, session := range p.sessions(t, p.token, "") {
		if session.NowPlayingItem != nil {
			playing[session.DeviceName] = true
		}
	}
	return playing
}

// runtime is the movie's runtime.
func (p playbackSetup) runtime(t *testing.T) time.Duration {
	t.Helper()
	var item BaseItemDto
	p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &item)
	if item.RunTimeTicks == nil {
		t.Fatal("the movie has no runtime")
	}
	return time.Duration(*item.RunTimeTicks) * 100
}

// resumePoint is the user's resume point in the movie.
func (p playbackSetup) resumePoint(t *testing.T) time.Duration {
	t.Helper()
	status, body := p.call(http.MethodGet, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, app("tv", p.token), nil)
	var item BaseItemDto
	if err := json.Unmarshal(body, &item); status != http.StatusOK || err != nil {
		t.Fatalf("the movie: %d %s", status, body)
	}
	return time.Duration(item.UserData.PlaybackPositionTicks) * 100
}

// A device whose app stopped reporting, as a tab closed while it played,
// stops playing five minutes after its last report, as Jellyfin stops it:
// it no longer holds a place of its user's Playbacks at once, and its
// resume point is the position it last reported.
func TestAPlaybackWhoseAppStoppedReportingEndsAfterFiveMinutes(t *testing.T) {
	t.Parallel()
	p := playing(t)
	advance := idleClock(p.testServer)
	tablet, laptop := p.signIn("member", "tablet"), p.signIn("member", "laptop")
	p.limit(t, p.user, func(c *accounts.UserChanges) { c.MaxPlaybacks = new(1) })
	position := (p.runtime(t) / 2).Truncate(time.Second)

	p.reportAt(t, "tablet", tablet, "/Sessions/Playing", 0, false)
	advance(10 * time.Second)
	p.reportAt(t, "tablet", tablet, "/Sessions/Playing/Progress", position, false)
	// The tab closes: nothing more comes from it.
	advance(idlePlayback - time.Second)
	p.handler.endIdlePlayback(t.Context())
	if p.playsNow(t, "laptop", laptop) {
		t.Fatal("the laptop played while the tablet's last report was not five minutes old")
	}
	advance(2 * time.Second)
	p.handler.endIdlePlayback(t.Context())
	if p.playingDevices(t)["tablet"] {
		t.Error("/Sessions shows the tablet playing five minutes after its last report")
	}
	if !p.playsNow(t, "laptop", laptop) {
		t.Error("the laptop was refused five minutes after the tablet's last report")
	}
	if resume := p.resumePoint(t); resume != position {
		t.Errorf("the resume point is %v, not the position the tablet last reported, %v", resume, position)
	}
}

// Devices whose apps keep reporting play on, paused or not, as long as
// they report: apps report every ten seconds while they play, paused too.
func TestAPlaybackThatKeepsReportingPlaysOn(t *testing.T) {
	t.Parallel()
	p := playing(t)
	advance := idleClock(p.testServer)
	phone, laptop := p.signIn("member", "phone"), p.signIn("member", "laptop")
	p.limit(t, p.user, func(c *accounts.UserChanges) { c.MaxPlaybacks = new(2) })

	p.reportAt(t, "tv", p.token, "/Sessions/Playing", 0, false)
	p.reportAt(t, "phone", phone, "/Sessions/Playing", 0, false)
	for minute := 4; minute <= 16; minute += 4 {
		advance(4 * time.Minute)
		p.reportAt(t, "tv", p.token, "/Sessions/Playing/Progress", time.Duration(minute)*time.Second, false)
		p.reportAt(t, "phone", phone, "/Sessions/Playing/Progress", time.Second, true)
		p.handler.endIdlePlayback(t.Context())
	}
	if playing := p.playingDevices(t); !playing["tv"] || !playing["phone"] {
		t.Fatalf("devices playing after 16 minutes of reports: %v", playing)
	}
	if p.playsNow(t, "laptop", laptop) {
		t.Error("a third device played at a limit of 2")
	}
}

// A torrent the user's other device played in the web player no longer
// counts as read by it once that device stopped reporting for five
// minutes: the device the user plays on now moves on at the cap.
func TestATorrentOfADeviceThatStoppedReportingIsNoLongerReadByIt(t *testing.T) {
	t.Parallel()
	p := newCapTitles(t, 0)
	advance := idleClock(p.testServer)
	p.cap(t, 1)
	web, tv := p.signIn("member", "web"), p.signIn("member", "tv")
	// The tv plays Iron Coast, as jellyfin-web reports it, then its tab
	// closes.
	if answer := p.ask(t, "tv", tv, "Iron Coast"); answer.ErrorCode != "" {
		t.Fatalf("the tv's play answered %q", answer.ErrorCode)
	}
	p.reportAsWeb(t, "tv", tv, "/Sessions/Playing", "Iron Coast")
	// The user goes on playing it on the web device, then moves on.
	p.play(t, "web", web, "Iron Coast")
	p.report(t, "web", web, "/Sessions/Playing/Stopped", "Iron Coast", nil)
	advance(idlePlayback + time.Second)
	p.handler.endIdlePlayback(t.Context())

	if answer := p.ask(t, "web", web, "Signal Fire"); answer.ErrorCode != "" || len(answer.MediaSources) == 0 {
		t.Fatalf("the web device's next title answered %q, though the tv stopped reporting", answer.ErrorCode)
	}
	if p.running(t, "Iron Coast") {
		t.Error("the torrent the user left still runs")
	}
}
