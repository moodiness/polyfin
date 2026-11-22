package jellyfin

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/playback"
)

// noMatroska is jellyfin-web's profile in a browser that plays no Matroska,
// as most do: the first version, a Matroska file, is repackaged over HLS.
func (p playbackSetup) noMatroska(t *testing.T) json.RawMessage {
	t.Helper()
	var profile map[string]any
	if err := json.Unmarshal(p.profile(t, "jellyfin-web-chrome"), &profile); err != nil {
		t.Fatal(err)
	}
	var plays []any
	for _, play := range profile["DirectPlayProfiles"].([]any) {
		if container, _ := play.(map[string]any)["Container"].(string); !strings.Contains(container, "mkv") && !strings.Contains(container, "webm") {
			plays = append(plays, play)
		}
	}
	profile["DirectPlayProfiles"] = plays
	data, _ := json.Marshal(profile)
	return data
}

// playInfo asks the movie's PlaybackInfo on the TV, as jellyfin-web's Play
// does, with body's fields besides.
func (p playbackSetup) playInfo(t *testing.T, profile json.RawMessage, body map[string]any) playbackInfoResponse {
	t.Helper()
	request := map[string]any{"UserId": p.user.ID.String(), "MediaSourceId": p.movie, "MaxStreamingBitrate": 120_000_000, "DeviceProfile": profile}
	for name, value := range body {
		request[name] = value
	}
	status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token), request)
	var response playbackInfoResponse
	if err := json.Unmarshal(data, &response); status != http.StatusOK || err != nil || len(response.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %d %s", status, data)
	}
	return response
}

// startedEarly waits until the play started early for the TV has its play
// session, and returns it.
func (p playbackSetup) startedEarly(t *testing.T) earlyPlay {
	t.Helper()
	var started earlyPlay
	eventually(t, "a play started early", func() bool {
		pr := p.handler.preparations
		pr.mu.Lock()
		defer pr.mu.Unlock()
		for _, play := range pr.early {
			started = *play
		}
		return started.session != ""
	})
	return started
}

// earlyPlays is how many plays started early are kept.
func (p playbackSetup) earlyPlays() int {
	pr := p.handler.preparations
	pr.mu.Lock()
	defer pr.mu.Unlock()
	return len(pr.early)
}

// Opening a title on a device whose profile is known starts its play
// early: its PlaybackInfo then answers with that play session, whose HLS
// requests take over the remux under way. The play is kept: the title
// opened again starts nothing early meanwhile, until another title is
// opened. A play from elsewhere than the start does not take the play
// started early, and ends it; so does opening another title.
func TestAPlayGoesOnFromThePlayStartedEarly(t *testing.T) {
	t.Parallel()
	if os.Getenv("POLYFIN_TEST_FFMPEG") == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set: the remux runs FFmpeg")
	}
	p := playing(t)
	p.remuxable(t)
	p.setting(t, func(settings *accounts.Settings) { settings.PrepareAhead = true })
	profile := p.noMatroska(t)
	// The TV told how it plays, playing another title.
	p.call(http.MethodPost, "/Items/"+p.remote+"/PlaybackInfo", app("tv", p.token),
		map[string]any{"UserId": p.user.ID.String(), "MaxStreamingBitrate": 120_000_000, "DeviceProfile": profile})

	p.detailsOn(t, "tv", p.token, p.movie)
	started := p.startedEarly(t)
	if started.version != p.versions[0].ID || len(p.handler.Playback.Encodings()) != 0 {
		t.Errorf("started early: version %s, want %s; listed among the encodings: %d", started.version, p.versions[0].ID, len(p.handler.Playback.Encodings()))
	}
	answer := p.playInfo(t, profile, nil)
	target := answer.MediaSources[0].TranscodingUrl
	if answer.PlaySessionId != started.session || !strings.Contains(target, "/master.m3u8?") {
		t.Fatalf("PlaybackInfo: session %s, want %s; %s", answer.PlaySessionId, started.session, target)
	}
	base, query, _ := strings.Cut(p.url+target, "master.m3u8?")
	for _, path := range []string{"master.m3u8", "main.m3u8", "hls1/main/-1.mp4", "hls1/main/0.mp4"} {
		if status, _, _ := fetchText(t, base+path+"?"+query); status != http.StatusOK {
			t.Errorf("%s: %d", path, status)
		}
	}
	if running := p.handler.Playback.Encodings(); len(running) != 1 || running[0].Key.Session != started.session {
		t.Errorf("encodings: %+v, want the one started early", running)
	}
	if kept := p.startedEarly(t); kept.session != started.session || !kept.taken {
		t.Errorf("kept once played: %+v, want the play started early, taken", kept)
	}

	// The title played opened again starts nothing early; another title
	// opened ends what was kept.
	p.detailsOn(t, "tv", p.token, p.movie)
	time.Sleep(200 * time.Millisecond)
	if kept := p.startedEarly(t); kept.session != started.session {
		t.Errorf("the title played started early again: %s", kept.session)
	}
	p.detailsOn(t, "tv", p.token, p.remote)
	if p.earlyPlays() != 0 {
		t.Error("the play kept outlived the opening of another title")
	}

	// A resume does not take the play started early: it ends.
	p.detailsOn(t, "tv", p.token, p.movie)
	again := p.startedEarly(t)
	if resumed := p.playInfo(t, profile, map[string]any{"StartTimeTicks": int64(5 * time.Second / 100)}); resumed.PlaySessionId == again.session ||
		p.earlyPlays() != 0 {
		t.Errorf("a resume played the play started early: %s", resumed.PlaySessionId)
	}

	// Another title opened ends the one before.
	p.detailsOn(t, "tv", p.token, p.movie)
	p.startedEarly(t)
	p.detailsOn(t, "tv", p.token, p.remote)
	if p.earlyPlays() != 0 {
		t.Error("the play started early outlived the opening of another title")
	}
}

// Play pressed before the title's play started early began, as when Play
// comes as the version's analysis ends, starts FFmpeg as its PlaybackInfo
// answers, in its own play session, which its HLS requests take over; the
// details the app opens meanwhile start nothing more early. FFmpeg starts
// once from the start.
func TestAPlayBeforeItsEarlyStartStartsAsItsPlaybackInfoAnswers(t *testing.T) {
	t.Parallel()
	if os.Getenv("POLYFIN_TEST_FFMPEG") == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set: the remux runs FFmpeg")
	}
	p := playing(t)
	p.remuxable(t)
	profile := p.noMatroska(t)
	// The TV told how it plays, playing another title.
	p.call(http.MethodPost, "/Items/"+p.remote+"/PlaybackInfo", app("tv", p.token),
		map[string]any{"UserId": p.user.ID.String(), "MaxStreamingBitrate": 120_000_000, "DeviceProfile": profile})
	p.setting(t, func(settings *accounts.Settings) { settings.PrepareAhead = true })
	p.warnings.all.Store(true)
	logged := func(message string) int { return len(p.warnings.attrsOf(message)) }

	answer := p.playInfo(t, profile, nil)
	target := answer.MediaSources[0].TranscodingUrl
	if !strings.Contains(target, "/master.m3u8?") {
		t.Fatalf("PlaybackInfo: %s", target)
	}
	eventually(t, "FFmpeg started as PlaybackInfo answered", func() bool { return logged("Started an encoding ahead of its play") == 1 })
	p.detailsOn(t, "tv", p.token, p.movie)
	time.Sleep(200 * time.Millisecond)
	if kept := p.startedEarly(t); kept.session != answer.PlaySessionId || !kept.taken {
		t.Errorf("kept: %+v, want the PlaybackInfo's play", kept)
	}

	base, query, _ := strings.Cut(p.url+target, "master.m3u8?")
	for _, path := range []string{"master.m3u8", "main.m3u8", "hls1/main/-1.mp4", "hls1/main/0.mp4"} {
		if status, _, _ := fetchText(t, base+path+"?"+query); status != http.StatusOK {
			t.Errorf("%s: %d", path, status)
		}
	}
	if running := p.handler.Playback.Encodings(); len(running) != 1 || running[0].Key.Session != answer.PlaySessionId {
		t.Errorf("encodings: %+v, want the PlaybackInfo's", running)
	}
	fromStart := 0
	for _, attrs := range p.warnings.attrsOf("Started a remux") {
		if attrs["from"] == "0" {
			fromStart++
		}
	}
	if started, took := logged("Started an encoding ahead of its play"), logged("A play took over the encoding started ahead of it"); started != 1 ||
		took != 1 || fromStart != 1 {
		t.Errorf("started ahead %d, taken over %d, FFmpeg runs from the start %d, want 1 each", started, took, fromStart)
	}
}

// Without playback prepared ahead, nothing starts early.
func TestNothingStartsEarlyWithoutPreparation(t *testing.T) {
	t.Parallel()
	p := playing(t)
	p.remuxable(t)
	profile := p.noMatroska(t)
	p.call(http.MethodPost, "/Items/"+p.remote+"/PlaybackInfo", app("tv", p.token),
		map[string]any{"UserId": p.user.ID.String(), "MaxStreamingBitrate": 120_000_000, "DeviceProfile": profile})
	p.detailsOn(t, "tv", p.token, p.movie)
	time.Sleep(200 * time.Millisecond)
	if p.earlyPlays() != 0 {
		t.Error("a play started early with playback prepared ahead off")
	}
}

// A device keeps one play started early: the same title opened again
// within earlyIdle keeps it, another replaces it, and so does the same
// title once earlyIdle passed. A PlaybackInfo goes on from it only under
// the same grant, else with a session of its own; and those no Play took
// are forgotten after earlyKept.
func TestOnePlayStartedEarlyPerDevice(t *testing.T) {
	t.Parallel()
	now := time.Now()
	p := newPreparations(nil)
	p.now = func() time.Time { return now }
	owner := earlyOwner{user: accounts.ID{1}, device: accounts.ID{2}}
	title, other, version := accounts.ID{3}, accounts.ID{4}, accounts.ID{5}
	stopped := 0
	play := func(title accounts.ID) *earlyPlay {
		return &earlyPlay{title: title, version: version, stop: func() { stopped++ }}
	}
	first := play(title)
	if ended, ok := p.claimEarly(owner, first); !ok || len(ended) != 0 {
		t.Fatalf("first claim: %v %v", ended, ok)
	}
	if _, ok := p.claimEarly(owner, play(title)); ok {
		t.Error("the same title opened again started again")
	}
	now = now.Add(earlyIdle)
	again := play(title)
	if ended, ok := p.claimEarly(owner, again); !ok || len(ended) != 1 || ended[0] != first {
		t.Errorf("the same title after earlyIdle: %v %v", ended, ok)
	}
	next := play(other)
	if ended, ok := p.claimEarly(owner, next); !ok || len(ended) != 1 || ended[0] != again {
		t.Errorf("another title: %v %v", ended, ok)
	}
	if p.leaveEarly(owner, other) != nil || p.leaveEarly(owner, title) != next {
		t.Error("leaving the title opened, or another, did not leave as expected")
	}

	grant := playback.Grant{Version: version, User: owner.user, Device: owner.device}
	started := play(title)
	p.claimEarly(owner, started)
	if !p.reserveEarly(owner, started, grant, "session-1") || p.reserveEarly(owner, play(title), grant, "session-2") {
		t.Error("only the play kept records its session")
	}
	relayed := grant
	relayed.Relay = true
	if session, early, ended := p.takeEarly(title, relayed, true, true, "own-1"); session != "own-1" || early || ended != started {
		t.Errorf("another grant: %q %v %v", session, early, ended)
	}
	now = now.Add(earlyIdle)
	reserved := play(title)
	p.claimEarly(owner, reserved)
	p.reserveEarly(owner, reserved, grant, "session-1")
	signed := grant
	signed.Signed = now
	if session, early, ended := p.takeEarly(title, signed, true, true, "own-2"); session != "session-1" || !early || ended != nil {
		t.Errorf("the same grant: %q %v %v", session, early, ended)
	}
	if session, early, _ := p.takeEarly(title, grant, true, true, "own-3"); session != "own-3" || early {
		t.Error("a play started early was taken twice")
	}

	now = now.Add(earlyKept)
	elsewhere := earlyOwner{user: owner.user, device: accounts.ID{6}}
	if ended, _ := p.claimEarly(elsewhere, play(title)); len(ended) != 1 {
		t.Errorf("a play started early long ago was kept: %v", ended)
	}
}

// Play pressed at once meets the play started early as it starts, both
// having waited for the same analysis, in one of three orders; FFmpeg
// starts once in each. Before the play started early began, the
// PlaybackInfo answers with a session of its own, whose FFmpeg it starts
// (see startWithPlay), and nothing starts early for the title meanwhile,
// whatever its version. Before the play started early set its session,
// so too, and the play started early starts nothing. Once it set it, as
// its FFmpeg is about to start, the PlaybackInfo answers with it, and the
// play started early keeps it once its FFmpeg started.
func TestAPlayPressedAsItsEarlyPlayStartsGoesOnOnce(t *testing.T) {
	t.Parallel()
	owner := earlyOwner{user: accounts.ID{1}, device: accounts.ID{2}}
	title, version, other := accounts.ID{3}, accounts.ID{4}, accounts.ID{5}
	grant := playback.Grant{Version: version, User: owner.user, Device: owner.device, Signed: time.Now()}
	play := func(version accounts.ID, stopped *int) *earlyPlay {
		return &earlyPlay{title: title, version: version, stop: func() { *stopped++ }}
	}
	t.Run("before it began", func(t *testing.T) {
		t.Parallel()
		p := newPreparations(nil)
		stopped := 0
		if session, early, ended := p.takeEarly(title, grant, true, true, "own"); session != "own" || early || ended != nil {
			t.Fatalf("PlaybackInfo: %q %v %v", session, early, ended)
		}
		for _, v := range []accounts.ID{version, other} {
			if _, ok := p.claimEarly(owner, play(v, &stopped)); ok {
				t.Errorf("a play of version %v started early for the title being played", v)
			}
		}
		if !p.answeredWith(owner, "own") {
			t.Error("the PlaybackInfo's session was not kept as the play's")
		}
	})
	t.Run("before its session", func(t *testing.T) {
		t.Parallel()
		p := newPreparations(nil)
		stopped := 0
		deciding := play(version, &stopped)
		p.claimEarly(owner, deciding)
		session, early, ended := p.takeEarly(title, grant, true, true, "own")
		if session != "own" || early || ended != deciding {
			t.Fatalf("PlaybackInfo: %q %v %v", session, early, ended)
		}
		if p.reserveEarly(owner, deciding, grant, "early") || !p.answeredWith(owner, "own") {
			t.Error("the play started early went on once the PlaybackInfo answered with its own session")
		}
	})
	t.Run("once its session is set", func(t *testing.T) {
		t.Parallel()
		p := newPreparations(nil)
		stopped := 0
		starting := play(version, &stopped)
		p.claimEarly(owner, starting)
		if !p.reserveEarly(owner, starting, grant, "early") {
			t.Fatal("the play started early could not set its session")
		}
		if session, early, ended := p.takeEarly(title, grant, true, true, "own"); session != "early" || !early || ended != nil {
			t.Fatalf("PlaybackInfo: %q %v %v", session, early, ended)
		}
		if !p.keepsEarly(owner, starting) || p.answeredWith(owner, "own") || stopped != 0 {
			t.Error("the play started early was not kept once the PlaybackInfo went on from it")
		}
		if _, ok := p.claimEarly(owner, play(version, &stopped)); ok {
			t.Error("a play started early again for the title being played")
		}
	})
}
