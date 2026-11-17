package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Version names end with how the device asking plays each version known:
// PlaybackInfo names every version it lists for the profile sent; item
// details name them for the device's last PlaybackInfo, and not for a
// device that never played. Ids and ETags stay. The names follow the
// server language, conversions the user may not have, and the setting.
func TestVersionNamesSayHowTheyPlay(t *testing.T) {
	t.Parallel()
	p := playing(t)
	// The first version plays with its AC3 audio converted for Chrome; the
	// second, H.264 and AAC in MP4, as it is.
	p.analyzed(t, p.versions[0], "h264-ac3-srt-mkv")
	p.keyframed(t, p.versions[0])
	userItem := "/Users/" + p.user.ID.String() + "/Items/" + p.movie
	names := func(sources []MediaSourceInfo) []string {
		result := make([]string, len(sources))
		for i, source := range sources {
			result[i] = source.Name
		}
		return result
	}
	details := func(what, token string, want ...string) []MediaSourceInfo {
		t.Helper()
		var dto BaseItemDto
		if status := p.get(t, userItem, token, &dto); status != http.StatusOK || dto.MediaSources == nil {
			t.Fatalf("%s: %d", what, status)
		}
		if got := names(*dto.MediaSources); !slices.Equal(got, want) {
			t.Errorf("%s: %q, want %q", what, got, want)
		}
		return *dto.MediaSources
	}
	first, second := "Source 2160p · REMUX · HEVC", "Source 1080p · WEB-DL"
	before := details("before any play", p.token, first, second)

	chrome := p.profile(t, "jellyfin-web-chrome")
	answer := p.ask(t, p.token, p.movie, chrome, nil)
	if got := names(answer.MediaSources); !slices.Equal(got, []string{first + " (converted: audio)", second + " (direct play)"}) {
		t.Errorf("PlaybackInfo: %q", got)
	}
	after := details("after a play", p.token, first+" (converted: audio)", second+" (direct play)")
	for i := range after {
		if after[i].Id != before[i].Id || after[i].ETag != before[i].ETag {
			t.Errorf("source %d: %s %s, before %s %s", i, after[i].Id, after[i].ETag, before[i].Id, before[i].ETag)
		}
	}
	phone := p.signIn("member", "phone")
	details("another device", phone, first, second)

	p.setting(t, func(s *accounts.Settings) { s.Language = "fr" })
	details("in French", p.token, first+" (converti\u202f: audio)", second+" (lecture directe)")
	p.setting(t, func(s *accounts.Settings) { s.Language, s.Transcoding = "en", false })
	details("without conversion", p.token, first+" (cannot play here)", second+" (direct play)")
	p.setting(t, func(s *accounts.Settings) { s.Transcoding, s.NameHowVersionsPlay = true, false })
	details("the setting off", p.token, first, second)
	if answer := p.ask(t, p.token, p.movie, chrome, nil); !slices.Equal(names(answer.MediaSources), []string{first, second}) {
		t.Errorf("PlaybackInfo with the setting off: %q", names(answer.MediaSources))
	}

	// Taller than the user's quality group, a 1080p version is converted
	// down to it.
	needsEncoders(t)
	q := playing(t)
	q.sized(t, 1, 1920, 1080)
	q.indexed(t)
	q.group(t, q.user, 720)
	q.ask(t, q.token, q.movie, chrome, nil)
	var dto BaseItemDto
	q.get(t, "/Users/"+q.user.ID.String()+"/Items/"+q.movie, q.token, &dto)
	at := slices.IndexFunc(*dto.MediaSources, func(s MediaSourceInfo) bool { return s.ETag == q.versions[1].ID.String() })
	if at < 0 || (*dto.MediaSources)[at].Name != second+" (converted: quality limit)" {
		t.Errorf("over the quality group: %q", names(*dto.MediaSources))
	}
}

// restarted is the test server as after a restart: a new handler over the
// same database and services, which remembers nothing in memory.
func (s testServer) restarted(t *testing.T) testServer {
	t.Helper()
	handler := New(s.handler.Options)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	s.handler, s.url = handler, server.URL
	return s
}

// keptPlay is what the database keeps of how a user's device plays: the
// audio codec of its profile's first direct play profile, its bitrate,
// whether its capabilities told it and when it was written; false when
// nothing is kept.
type keptPlay struct {
	audio            string
	bitrate          *int64
	fromCapabilities bool
	at               time.Time
}

func (s testServer) keptPlay(t *testing.T, user, device string) (keptPlay, bool) {
	t.Helper()
	var kept keptPlay
	err := s.pool.QueryRow(t.Context(), `SELECT coalesce(p.profile->'DirectPlayProfiles'->0->>'AudioCodec', ''), p.bitrate, p.from_capabilities, p.updated_at
		FROM device_play_profiles p JOIN devices d ON d.id = p.device_id JOIN users u ON u.id = d.user_id
		WHERE u.name = $1 AND d.device_id = $2`, user, device+"-id").Scan(&kept.audio, &kept.bitrate, &kept.fromCapabilities, &kept.at)
	if err != nil {
		return keptPlay{}, false
	}
	return kept, true
}

func (s testServer) keptPlays(t *testing.T) int {
	t.Helper()
	var n int
	if err := s.pool.QueryRow(t.Context(), "SELECT count(*) FROM device_play_profiles").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// detailsNames are the names item details give a title's versions.
func (s testServer) detailsNames(t *testing.T, user accounts.User, token, item string) []string {
	t.Helper()
	var dto BaseItemDto
	if status := s.get(t, "/Users/"+user.ID.String()+"/Items/"+item, token, &dto); status != http.StatusOK || dto.MediaSources == nil {
		t.Fatalf("details: %d", status)
	}
	names := make([]string, len(*dto.MediaSources))
	for i, source := range *dto.MediaSources {
		names[i] = source.Name
	}
	return names
}

// A device's last profile and bitrate are kept across restarts: after a
// restart, item details name the versions as before it, without a play.
// The profile is written when it changes only, and a changed one replaces
// it (see TestProfilesOverTheirBoundsAreNotKept for one too large to
// keep). API keys keep none, and a device signed out or a user deleted
// leaves none behind.
func TestVersionNamesSurviveARestart(t *testing.T) {
	t.Parallel()
	p := playing(t)
	p.analyzed(t, p.versions[0], "h264-ac3-srt-mkv")
	p.keyframed(t, p.versions[0])
	plain := p.detailsNames(t, p.user, p.token, p.movie)
	if _, ok := p.keptPlay(t, "member", "tv"); ok {
		t.Fatal("a profile kept before any play")
	}
	p.ask(t, p.token, p.movie, p.profile(t, "jellyfin-web-chrome"), nil)
	chrome := p.detailsNames(t, p.user, p.token, p.movie)
	if slices.Equal(chrome, plain) {
		t.Fatalf("details after a play name nothing: %q", chrome)
	}
	kept, ok := p.keptPlay(t, "member", "tv")
	if !ok || kept.audio == "" || kept.bitrate == nil || *kept.bitrate != 120_000_000 || kept.fromCapabilities {
		t.Fatalf("kept after a play: %+v %v", kept, ok)
	}

	r := p
	r.testServer = p.restarted(t)
	if got := r.detailsNames(t, r.user, r.token, r.movie); !slices.Equal(got, chrome) {
		t.Errorf("after a restart: %q, want %q", got, chrome)
	}
	// The same profile again writes nothing.
	r.ask(t, r.token, r.movie, r.profile(t, "jellyfin-web-chrome"), nil)
	if again, _ := r.keptPlay(t, "member", "tv"); !again.at.Equal(kept.at) {
		t.Errorf("the same profile written again: %v, then %v", kept.at, again.at)
	}
	// Another replaces it: the second version, AAC in MP4, is then
	// streamed rather than played as it is.
	r.ask(t, r.token, r.movie, r.profile(t, "minimal-no-aac"), map[string]any{"MaxStreamingBitrate": 8_000_000})
	noAAC := r.detailsNames(t, r.user, r.token, r.movie)
	if slices.Equal(noAAC, chrome) {
		t.Fatalf("details for another profile: %q", noAAC)
	}
	if kept, _ := r.keptPlay(t, "member", "tv"); kept.audio != "mp3" || kept.bitrate == nil || *kept.bitrate != 8_000_000 {
		t.Errorf("kept after another profile: %+v", kept)
	}
	r.testServer = r.restarted(t)
	if got := r.detailsNames(t, r.user, r.token, r.movie); !slices.Equal(got, noAAC) {
		t.Errorf("after another restart: %q, want %q", got, noAAC)
	}

	// An API key keeps nothing.
	_, key, err := r.store.CreateAPIKey(t.Context(), "Script")
	if err != nil {
		t.Fatal(err)
	}
	r.call(http.MethodPost, "/Items/"+r.movie+"/PlaybackInfo?userId="+r.user.ID.String(), `MediaBrowser Token="`+key+`"`,
		map[string]any{"DeviceProfile": r.profile(t, "minimal")})
	if n := r.keptPlays(t); n != 1 {
		t.Errorf("profiles kept after an API key's play: %d", n)
	}
	// A device signed out keeps none.
	if status, _ := r.call(http.MethodPost, "/Sessions/Logout", app("tv", r.token), nil); status != http.StatusNoContent {
		t.Fatalf("sign-out: %d", status)
	}
	if n := r.keptPlays(t); n != 0 {
		t.Errorf("profiles kept after a sign-out: %d", n)
	}
	// Nor a user deleted.
	phone := r.signIn("member", "phone")
	r.ask(t, phone, r.movie, r.profile(t, "jellyfin-web-chrome"), nil)
	if _, ok := r.keptPlay(t, "member", "phone"); !ok {
		t.Fatal("no profile kept for another device")
	}
	if err := r.store.DeleteUser(t.Context(), r.user.ID); err != nil {
		t.Fatal(err)
	}
	if n := r.keptPlays(t); n != 0 {
		t.Errorf("profiles kept after the user was deleted: %d", n)
	}
}

// A profile a session's capabilities carry is kept across restarts too,
// until a PlaybackInfo's replaces it, which capabilities sent after a
// restart leave in place.
func TestCapabilitiesProfileSurvivesARestart(t *testing.T) {
	t.Parallel()
	p := playing(t)
	p.analyzed(t, p.versions[0], "h264-ac3-srt-mkv")
	p.keyframed(t, p.versions[0])
	capabilities := func(s testServer, profile json.RawMessage) {
		t.Helper()
		if status, body := s.call(http.MethodPost, "/Sessions/Capabilities/Full", app("tv", p.token),
			map[string]any{"PlayableMediaTypes": []string{"Video"}, "DeviceProfile": profile}); status != http.StatusNoContent {
			t.Fatalf("capabilities: %d %s", status, body)
		}
	}
	plain := p.detailsNames(t, p.user, p.token, p.movie)
	capabilities(p.testServer, p.profile(t, "jellyfin-web-chrome"))
	chrome := p.detailsNames(t, p.user, p.token, p.movie)
	if slices.Equal(chrome, plain) {
		t.Fatalf("details after capabilities name nothing: %q", chrome)
	}
	if kept, ok := p.keptPlay(t, "member", "tv"); !ok || !kept.fromCapabilities || kept.bitrate != nil {
		t.Fatalf("kept from capabilities: %+v %v", kept, ok)
	}
	r := p
	r.testServer = p.restarted(t)
	if got := r.detailsNames(t, r.user, r.token, r.movie); !slices.Equal(got, chrome) {
		t.Errorf("after a restart: %q, want %q", got, chrome)
	}
	r.ask(t, r.token, r.movie, r.profile(t, "minimal-no-aac"), nil)
	noAAC := r.detailsNames(t, r.user, r.token, r.movie)
	r.testServer = r.restarted(t)
	capabilities(r.testServer, r.profile(t, "jellyfin-web-chrome"))
	if got := r.detailsNames(t, r.user, r.token, r.movie); !slices.Equal(got, noAAC) {
		t.Errorf("capabilities after a restart replaced PlaybackInfo's profile: %q, want %q", got, noAAC)
	}
	if kept, _ := r.keptPlay(t, "member", "tv"); kept.audio != "mp3" || kept.fromCapabilities {
		t.Errorf("kept: %+v", kept)
	}
}
