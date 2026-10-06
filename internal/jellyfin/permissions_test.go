package jellyfin

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
)

// switches sets the server's conversion switch.
func (s testServer) switches(t *testing.T, transcoding bool) {
	t.Helper()
	settings := s.store.Settings()
	settings.Transcoding = transcoding
	if _, err := s.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
}

// permit sets a user's own permissions.
func (s testServer) permit(t *testing.T, user accounts.User, video, audio, download bool) {
	t.Helper()
	changes := accounts.UserChanges{VideoTranscoding: &video, AudioTranscoding: &audio, ContentDownloading: &download}
	if _, err := s.store.UpdateUser(t.Context(), user.ID, changes, nil); err != nil {
		t.Fatal(err)
	}
}

// playbackAnswer is a PlaybackInfo answer, whether it offers sources or
// none.
type playbackAnswer struct {
	MediaSources  []MediaSourceInfo
	PlaySessionId string
	ErrorCode     string
}

// ask posts PlaybackInfo for an item with a recorded device profile and
// more of PlaybackInfoDto's fields.
func (s testServer) ask(t *testing.T, token, item string, profile json.RawMessage, more map[string]any) playbackAnswer {
	t.Helper()
	body := map[string]any{"MaxStreamingBitrate": 120_000_000, "DeviceProfile": profile}
	maps.Copy(body, more)
	status, data := s.call(http.MethodPost, "/Items/"+item+"/PlaybackInfo", app("tv", token), body)
	var answer playbackAnswer
	if err := json.Unmarshal(data, &answer); status != http.StatusOK || err != nil {
		t.Fatalf("PlaybackInfo: %d %s", status, data)
	}
	return answer
}

func (a playbackAnswer) refused() bool {
	return a.ErrorCode == "NoCompatibleStream" && len(a.MediaSources) == 0
}

// The minimal profile direct plays H.264 and AAC in MP4, and takes AAC only
// when streamed: the movie's first version, Opus and H.264 in Matroska,
// needs its audio converted; the second, H.264 and AAC in MP4, plays as it
// is.
func TestConversionOffPlaysTheVersionsThatNeedNone(t *testing.T) {
	p := playing(t)
	p.remuxable(t)
	minimal, chrome := p.profile(t, "minimal"), p.profile(t, "jellyfin-web-chrome")
	converted := p.ask(t, p.token, p.movie, minimal, nil)
	if len(converted.MediaSources) != 2 || converted.MediaSources[0].Id != p.movie ||
		!strings.HasSuffix(converted.MediaSources[0].TranscodingUrl, "&allowAudioStreamCopy=false") {
		t.Fatalf("with conversion allowed: %+v", converted)
	}
	for _, off := range []struct {
		name string
		set  func(on bool)
	}{
		{"server", func(on bool) { p.switches(t, on) }},
		{"user", func(on bool) { p.permit(t, p.user, on, on, true) }},
	} {
		off.set(false)
		// The next version, which plays as it is, is chosen.
		chosen := p.ask(t, p.token, p.movie, minimal, nil)
		if len(chosen.MediaSources) == 0 || chosen.MediaSources[0].Id != p.versions[1].ID.String() ||
			!chosen.MediaSources[0].SupportsDirectPlay || chosen.MediaSources[0].TranscodingUrl != "" {
			t.Errorf("%s off: chosen %+v", off.name, chosen)
		}
		// The version that needs a conversion is not played when asked by
		// its own identifier; the title's names none, and plays the next.
		if asked := p.ask(t, p.token, p.movie, minimal, map[string]any{"MediaSourceId": p.versions[0].ID.String()}); !asked.refused() {
			t.Errorf("%s off: the version needing a conversion: %+v", off.name, asked)
		}
		if asked := p.ask(t, p.token, p.movie, minimal, map[string]any{"MediaSourceId": p.movie}); len(asked.MediaSources) != 1 ||
			asked.MediaSources[0].Id != p.versions[1].ID.String() {
			t.Errorf("%s off: the version needing a conversion: %+v", off.name, asked)
		}
		// Neither direct play nor a remux that copies the tracks changes.
		if other := p.ask(t, p.token, p.movie, minimal, map[string]any{"MediaSourceId": p.versions[1].ID.String()}); len(other.MediaSources) != 1 ||
			!other.MediaSources[0].SupportsDirectPlay {
			t.Errorf("%s off: the version playing as it is: %+v", off.name, other)
		}
		copied := p.ask(t, p.token, p.movie, chrome, map[string]any{"MediaSourceId": p.movie, "EnableDirectPlay": false})
		if len(copied.MediaSources) != 1 || copied.MediaSources[0].TranscodingUrl == "" ||
			strings.Contains(copied.MediaSources[0].TranscodingUrl, "StreamCopy=false") {
			t.Errorf("%s off: a remux copying the tracks: %+v", off.name, copied)
		}
		// Only one version: none plays.
		if only := p.ask(t, p.token, p.movie, minimal, map[string]any{"MediaSourceId": p.versions[0].ID.String(), "EnableDirectPlay": false}); !only.refused() {
			t.Errorf("%s off: %+v", off.name, only)
		}
		off.set(true)
	}
}

func TestConversionPermissionsApplyPerKind(t *testing.T) {
	p := playing(t)
	p.remuxable(t)
	audio := map[string]any{"MediaSourceId": p.versions[0].ID.String()}
	minimal := p.profile(t, "minimal")

	// The video copied, the audio converted.
	p.permit(t, p.user, false, true, true)
	if answer := p.ask(t, p.token, p.movie, minimal, audio); len(answer.MediaSources) != 1 ||
		!strings.HasSuffix(answer.MediaSources[0].TranscodingUrl, "&allowAudioStreamCopy=false") {
		t.Errorf("audio converted, video conversion not allowed: %+v", answer)
	}
	p.permit(t, p.user, true, false, true)
	if answer := p.ask(t, p.token, p.movie, minimal, audio); !answer.refused() {
		t.Errorf("audio converted, audio conversion not allowed: %+v", answer)
	}

	if os.Getenv("POLYFIN_TEST_FFMPEG") == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set: Polyfin converts video with the encoders FFmpeg has")
	}
	// The video converted, the audio copied, as jellyfin-web asks when a
	// file failed to play as it is.
	video := map[string]any{"MediaSourceId": p.versions[0].ID.String(), "EnableDirectPlay": false, "AllowVideoStreamCopy": false}
	chrome := p.profile(t, "jellyfin-web-chrome")
	if answer := p.ask(t, p.token, p.movie, chrome, video); len(answer.MediaSources) != 1 ||
		!strings.HasSuffix(answer.MediaSources[0].TranscodingUrl, "&allowVideoStreamCopy=false") {
		t.Errorf("video converted, audio conversion not allowed: %+v", answer)
	}
	p.permit(t, p.user, false, true, true)
	if answer := p.ask(t, p.token, p.movie, chrome, video); !answer.refused() {
		t.Errorf("video converted, video conversion not allowed: %+v", answer)
	}
}

// Burning an image subtitle in converts the video: when that is not
// allowed, the subtitle is left out and the file plays as it is.
func TestImageSubtitlesAreLeftOutWhenTheVideoMayNotBeConverted(t *testing.T) {
	if os.Getenv("POLYFIN_TEST_FFMPEG") == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set: without an encoder, no subtitle is burned in anyway")
	}
	p := playing(t)
	// An English PGS track, after the addon's file: stream 3 for Jellyfin.
	p.remuxable(t, media.Stream{Index: 2, Type: "subtitle", Codec: "hdmv_pgs_subtitle", Language: "eng", Width: 64, Height: 64})
	request := map[string]any{"MediaSourceId": p.movie, "SubtitleStreamIndex": 3}
	chrome := p.profile(t, "jellyfin-web-chrome")
	track := func(source MediaSourceInfo) string {
		if i := slices.IndexFunc(source.MediaStreams, func(s playback.MediaStream) bool { return s.Index == 3 }); i >= 0 {
			return source.MediaStreams[i].DeliveryMethod
		}
		return ""
	}
	if burned := p.ask(t, p.token, p.movie, chrome, request); len(burned.MediaSources) != 1 || track(burned.MediaSources[0]) != "Encode" {
		t.Fatalf("burned in when allowed: %+v", burned)
	}
	for _, off := range []func(on bool){
		func(on bool) { p.permit(t, p.user, on, true, true) },
		func(on bool) { p.switches(t, on) },
	} {
		off(false)
		answer := p.ask(t, p.token, p.movie, chrome, request)
		if len(answer.MediaSources) != 1 {
			t.Fatalf("burning not allowed: %+v", answer)
		}
		if source := answer.MediaSources[0]; !source.SupportsDirectPlay || source.TranscodingUrl != "" || track(source) != "Drop" {
			t.Errorf("burning not allowed: direct play %v, track %s, TranscodingUrl %q", source.SupportsDirectPlay, track(source), source.TranscodingUrl)
		}
		off(true)
	}
}

func TestHLSRefusesConversionsThatAreNotAllowed(t *testing.T) {
	p := playing(t)
	p.remuxable(t)
	audio := p.ask(t, p.token, p.movie, p.profile(t, "minimal"), map[string]any{"MediaSourceId": p.movie}).MediaSources[0].TranscodingUrl
	copied := p.ask(t, p.token, p.movie, p.profile(t, "jellyfin-web-chrome"), map[string]any{"MediaSourceId": p.movie, "EnableDirectPlay": false}).MediaSources[0].TranscodingUrl
	if !strings.Contains(audio, "allowAudioStreamCopy=false") || copied == "" || strings.Contains(copied, "StreamCopy=false") {
		t.Fatalf("TranscodingUrls: %q, %q", audio, copied)
	}
	// A URL asking for the video converted, as made by hand.
	video := copied + "&allowVideoStreamCopy=false"
	segment := func(target string) string {
		base, query, _ := strings.Cut(target, "master.m3u8?")
		return base + "hls1/main/0.mp4?" + query
	}
	expect := func(when, target string, want int) {
		t.Helper()
		status, _, body := fetchText(t, p.url+target)
		if status != want || (want == http.StatusBadRequest && body != "Error processing request.") {
			t.Errorf("%s: %d %q, want %d", when, status, body, want)
		}
	}
	expect("audio conversion allowed", audio, http.StatusOK)
	for _, off := range []struct {
		name         string
		set          func(on bool)
		video, audio bool
	}{
		{"server off", func(on bool) { p.switches(t, on) }, false, false},
		{"user's video off", func(on bool) { p.permit(t, p.user, on, true, true) }, false, true},
		{"user's audio off", func(on bool) { p.permit(t, p.user, true, on, true) }, true, false},
	} {
		off.set(false)
		statuses := map[bool]int{true: http.StatusOK, false: http.StatusBadRequest}
		expect(off.name+", audio converted", audio, statuses[off.audio])
		expect(off.name+", video converted", video, statuses[off.video])
		if !off.audio {
			expect(off.name+", segment with the audio converted", segment(audio), http.StatusBadRequest)
		}
		expect(off.name+", tracks copied", copied, http.StatusOK)
		off.set(true)
	}
}

func TestLiveConversionsFollowThePermissions(t *testing.T) {
	addon := newTVAddon(t, false, "")
	s, token, user := tuned(t, addon)
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	channel := channels.Items[0].Id
	liveAnalysis(t, s, user, channel)
	profile, err := os.ReadFile(playbackFixtures + "/profiles/minimal.json")
	if err != nil {
		t.Fatal(err)
	}
	chrome, err := os.ReadFile(playbackFixtures + "/profiles/jellyfin-web-chrome.json")
	if err != nil {
		t.Fatal(err)
	}
	// The minimal profile takes no HLS as it is: the H.264 and AAC stream
	// is remuxed into MPEG-TS, its audio converted when the app asks.
	convert := map[string]any{"AllowAudioStreamCopy": false}
	converted := s.ask(t, token, channel, profile, convert)
	copied := s.ask(t, token, channel, profile, nil)
	if len(converted.MediaSources) != 1 || !strings.HasSuffix(converted.MediaSources[0].TranscodingUrl, "&allowAudioStreamCopy=false") ||
		len(copied.MediaSources) != 1 || copied.MediaSources[0].TranscodingUrl == "" || strings.Contains(copied.MediaSources[0].TranscodingUrl, "StreamCopy=false") {
		t.Fatalf("live sources: %+v, %+v", converted, copied)
	}
	master := func(target string) int {
		status, _ := s.call(http.MethodGet, target, "", nil)
		return status
	}
	if status := master(converted.MediaSources[0].TranscodingUrl); status != http.StatusOK {
		t.Fatalf("live conversion allowed: %d", status)
	}
	for _, off := range []struct {
		name string
		set  func(on bool)
	}{
		{"server", func(on bool) { s.switches(t, on) }},
		{"user", func(on bool) { s.permit(t, user, true, on, true) }},
	} {
		off.set(false)
		if answer := s.ask(t, token, channel, profile, convert); !answer.refused() {
			t.Errorf("%s off, live conversion asked: %+v", off.name, answer)
		}
		if answer := s.ask(t, token, channel, profile, nil); len(answer.MediaSources) != 1 || answer.MediaSources[0].TranscodingUrl == "" {
			t.Errorf("%s off, live remux: %+v", off.name, answer)
		}
		if status := master(converted.MediaSources[0].TranscodingUrl); status != http.StatusBadRequest {
			t.Errorf("%s off, live conversion: %d", off.name, status)
		}
		if status := master(copied.MediaSources[0].TranscodingUrl); status != http.StatusOK {
			t.Errorf("%s off, live remux: %d", off.name, status)
		}
		// A stream the app plays as it is stays.
		if answer := s.ask(t, token, channel, chrome, nil); len(answer.MediaSources) != 1 || !answer.MediaSources[0].SupportsDirectPlay {
			t.Errorf("%s off, live direct play: %+v", off.name, answer)
		}
		off.set(true)
	}
}

func TestDownloadsFollowThePermissions(t *testing.T) {
	p := playing(t)
	userItem := "/Users/" + p.user.ID.String() + "/Items/"
	var movie BaseItemDto
	p.get(t, userItem+p.movie, p.token, &movie)
	streamURL, err := url.Parse((*movie.MediaSources)[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	grant := grantParameter + "=" + url.QueryEscape(streamURL.Query().Get(grantParameter))
	credentials := map[string]struct {
		query  string
		header http.Header
	}{
		"ApiKey": {"?ApiKey=" + p.token, nil},
		"header": {"", http.Header{"Authorization": {app("tv", p.token)}}},
		"grant":  {"?" + grant, nil},
	}
	var views QueryResult
	p.get(t, "/UserViews", p.token, &views)
	check := func(when string, allowed bool) {
		t.Helper()
		var detail BaseItemDto
		p.get(t, userItem+p.movie, p.token, &detail)
		if detail.CanDownload == nil || *detail.CanDownload != allowed || detail.Path != "Movie.2160p.mkv" {
			t.Errorf("%s: details can download %v, path %q", when, detail.CanDownload, detail.Path)
		}
		var page QueryResult
		p.get(t, "/Items?ParentId="+views.Items[0].Id+"&fields=CanDownload,Path", p.token, &page)
		for _, item := range page.Items {
			if item.Id == p.movie && (item.CanDownload == nil || *item.CanDownload != allowed || item.Path != "Movie.2160p.mkv") {
				t.Errorf("%s: listed can download %v, path %q", when, item.CanDownload, item.Path)
			}
		}
		for name, credential := range credentials {
			for _, item := range []string{p.movie, strings.Repeat("ab", 16)} {
				response, body := fetchURL(t, p.url+"/Items/"+item+"/Download"+credential.query, credential.header)
				switch {
				case !allowed && (response.StatusCode != http.StatusForbidden || body != ""):
					t.Errorf("%s, %s, %s: %d %q", when, name, item, response.StatusCode, body)
				case allowed && item == p.movie && response.StatusCode != http.StatusOK:
					t.Errorf("%s, %s: %d", when, name, response.StatusCode)
				}
			}
		}
	}
	check("allowed", true)
	p.permit(t, p.user, true, true, false)
	check("user off", false)
	p.permit(t, p.user, true, true, true)
	check("allowed again", true)
	// Turning downloads off for everyone takes the user's permission away
	// at once, for the apps already signed in too.
	if _, err := p.store.TurnOffDownloads(t.Context()); err != nil {
		t.Fatal(err)
	}
	check("off for everyone", false)
}

func TestPolicyCarriesConversionAndDownloadPermissions(t *testing.T) {
	s := newTestServer(t, 10)
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	member := s.user("member", nil)
	adminToken := s.signIn("admin", "tv")
	path := "/Users/" + member.ID.String()
	policy := func() map[string]any {
		t.Helper()
		var user struct{ Policy map[string]any }
		if status := s.get(t, path, adminToken, &user); status != http.StatusOK {
			t.Fatalf("user: %d", status)
		}
		return user.Policy
	}
	flags := func(p map[string]any) [4]any {
		return [4]any{p["EnableVideoPlaybackTranscoding"], p["EnableAudioPlaybackTranscoding"], p["EnableContentDownloading"], p["EnablePlaybackRemuxing"]}
	}
	if got := flags(policy()); got != [4]any{true, true, true, true} {
		t.Errorf("a new user's policy: %v", got)
	}
	// The administrator's app posts the whole policy it read, changed.
	read := policy()
	read["EnableVideoPlaybackTranscoding"], read["EnableContentDownloading"] = false, false
	encoded, _ := json.Marshal(read)
	if status, body := s.postRaw(path+"/Policy", app("tv", adminToken), string(encoded)); status != http.StatusNoContent {
		t.Fatalf("policy: %d %s", status, body)
	}
	if got := flags(policy()); got != [4]any{false, true, false, true} {
		t.Errorf("policy after the change: %v", got)
	}
	stored, err := s.store.User(t.Context(), member.ID)
	if err != nil || stored.VideoTranscoding || !stored.AudioTranscoding || stored.ContentDownloading {
		t.Errorf("stored: %+v %v", stored, err)
	}
	// The policy is the user's own: the server's switch does not show.
	s.switches(t, false)
	if got := flags(policy()); got != [4]any{false, true, false, true} {
		t.Errorf("policy with the server's conversion off: %v", got)
	}
	// Like Jellyfin's, a policy that leaves them out grants them.
	if status, body := s.postRaw(path+"/Policy", app("tv", adminToken), `{"IsHidden":true}`); status != http.StatusNoContent {
		t.Fatalf("partial policy: %d %s", status, body)
	}
	if got := flags(policy()); got != [4]any{true, true, true, true} {
		t.Errorf("policy after a policy without them: %v", got)
	}
}
