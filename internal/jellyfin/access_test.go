package jellyfin

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// limit changes a user's playback and access limits.
func (s testServer) limit(t *testing.T, user accounts.User, change func(*accounts.UserChanges)) {
	t.Helper()
	var changes accounts.UserChanges
	change(&changes)
	if _, err := s.store.UpdateUser(t.Context(), user.ID, changes, nil); err != nil {
		t.Fatal(err)
	}
}

// playbackInfo posts PlaybackInfo from a device.
func (s testServer) playbackInfo(t *testing.T, device, token, item string, body map[string]any) playbackAnswer {
	t.Helper()
	if body == nil {
		body = map[string]any{}
	}
	status, data := s.call(http.MethodPost, "/Items/"+item+"/PlaybackInfo", app(device, token), body)
	var answer playbackAnswer
	if err := json.Unmarshal(data, &answer); status != http.StatusOK || err != nil {
		t.Fatalf("PlaybackInfo from %s: %d %s", device, status, data)
	}
	return answer
}

func TestPlaybacksAtOnceRefuseOneMoreDevice(t *testing.T) {
	p := playing(t)
	tokens := map[string]string{"tv": p.token, "phone": p.signIn("member", "phone"), "tablet": p.signIn("member", "tablet")}
	report := func(device, path string) {
		t.Helper()
		body := map[string]any{"ItemId": p.movie, "MediaSourceId": p.versions[1].ID.String(), "PositionTicks": 0, "PlayMethod": "DirectPlay"}
		if status, data := p.call(http.MethodPost, path, app(device, tokens[device]), body); status != http.StatusNoContent {
			t.Fatalf("%s from %s: %d %s", path, device, status, data)
		}
	}
	plays := func(device string) bool {
		t.Helper()
		answer := p.playbackInfo(t, device, tokens[device], p.movie, nil)
		if answer.ErrorCode == "RateLimitExceeded" && len(answer.MediaSources) == 0 {
			return false
		}
		if answer.ErrorCode != "" || len(answer.MediaSources) == 0 {
			t.Fatalf("PlaybackInfo from %s: %+v", device, answer)
		}
		return true
	}
	report("tv", "/Sessions/Playing")
	report("phone", "/Sessions/Playing")
	playing := 0
	for _, session := range p.sessions(t, p.token, "") {
		if session.NowPlayingItem != nil {
			playing++
		}
	}
	if playing != 2 {
		t.Fatalf("%d sessions playing", playing)
	}
	// No limit by default.
	if !plays("tablet") {
		t.Error("a third device was refused without a limit")
	}
	p.limit(t, p.user, func(c *accounts.UserChanges) { c.MaxPlaybacks = new(2) })
	if plays("tablet") {
		t.Error("a third device played at a limit of 2")
	}
	// The devices playing go on to another title.
	for _, device := range []string{"tv", "phone"} {
		if !plays(device) {
			t.Errorf("%s, already playing, was refused", device)
		}
	}
	// Once a device stops, another may play.
	report("phone", "/Sessions/Playing/Stopped")
	if !plays("tablet") {
		t.Error("the tablet was refused once the phone stopped")
	}
	report("phone", "/Sessions/Playing")
	p.limit(t, p.user, func(c *accounts.UserChanges) { c.MaxPlaybacks = new(3) })
	if !plays("tablet") {
		t.Error("a third device was refused at a limit of 3")
	}
}

// withBitrate stores the analysis of a recorded clip for a version, with
// another overall bitrate.
func (p playbackSetup) withBitrate(t *testing.T, version library.Version, clip string, bitrate int64) {
	t.Helper()
	probe, err := os.ReadFile(filepath.Join(playbackFixtures, "probes", clip+".json"))
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := media.Parse(probe)
	if err != nil {
		t.Fatal(err)
	}
	analysis.Remote, analysis.Bitrate = true, bitrate
	p.storeAnalysis(t, version, analysis)
}

// storeAnalysis replaces the analysis of a version.
func (p playbackSetup) storeAnalysis(t *testing.T, version library.Version, analysis media.Analysis) {
	t.Helper()
	data, _ := json.Marshal(analysis)
	if _, err := p.pool.Exec(t.Context(), "DELETE FROM media_analyses WHERE version_id = $1", version.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := p.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", version.ID, data); err != nil {
		t.Fatal(err)
	}
}

// The movie's first version, Opus and H.264 in Matroska, is made 3 Mbit/s;
// its second, H.264 and AAC in MP4, 1 Mbit/s. Both play as they are in
// jellyfin-web.
func TestMaxBitrateConvertsFallsBackOrRefuses(t *testing.T) {
	p := playing(t)
	remuxable := p.remuxable(t)
	remuxable.Bitrate = 3_000_000
	p.storeAnalysis(t, p.versions[0], remuxable)
	p.withBitrate(t, p.versions[1], "h264-aac-mp4", 1_000_000)
	heavy, light := p.movie, p.versions[1].ID.String()
	chrome := p.profile(t, "jellyfin-web-chrome")
	first := func(answer playbackAnswer) MediaSourceInfo {
		t.Helper()
		if len(answer.MediaSources) == 0 {
			t.Fatalf("no source: %+v", answer)
		}
		return answer.MediaSources[0]
	}
	listed := func(answer playbackAnswer, id string) MediaSourceInfo {
		t.Helper()
		i := slices.IndexFunc(answer.MediaSources, func(s MediaSourceInfo) bool { return s.Id == id })
		if i < 0 {
			t.Fatalf("%s not listed: %+v", id, answer)
		}
		return answer.MediaSources[i]
	}
	setLimit := func(bitrate int) { p.limit(t, p.user, func(c *accounts.UserChanges) { c.MaxBitrate = &bitrate }) }
	fetch := func(target string) int {
		t.Helper()
		request, _ := http.NewRequest(http.MethodGet, p.url+target, nil)
		request.Header.Set("Range", "bytes=0-3")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		return response.StatusCode
	}
	stream := func(version string) string {
		return "/Videos/" + p.movie + "/stream?static=true&mediaSourceId=" + version + "&ApiKey=" + p.token
	}

	// Without a limit, the first version plays as it is.
	if source := first(p.ask(t, p.token, p.movie, chrome, nil)); source.Id != heavy || !source.SupportsDirectPlay {
		t.Fatalf("without a limit: %+v", source)
	}
	copied := first(p.ask(t, p.token, p.movie, chrome, map[string]any{"MediaSourceId": heavy, "EnableDirectPlay": false})).TranscodingUrl
	if copied == "" || strings.Contains(copied, "StreamCopy=false") {
		t.Fatalf("remux without a limit: %q", copied)
	}
	if status := fetch(stream(heavy)); status != http.StatusPartialContent {
		t.Fatalf("stream without a limit: %d", status)
	}
	if status, _, _ := fetchText(t, p.url+copied); status != http.StatusOK {
		t.Fatalf("remux without a limit: %d", status)
	}

	// Below the first version, without video conversion: the lighter one
	// plays, and the heavier one is never offered as it is.
	setLimit(2_000_000)
	p.permit(t, p.user, false, true, true)
	answer := p.ask(t, p.token, p.movie, chrome, nil)
	if source := first(answer); source.Id != light || !source.SupportsDirectPlay {
		t.Errorf("fallback to the lighter version: %+v", source)
	}
	if other := listed(answer, heavy); other.SupportsDirectPlay || other.SupportsDirectStream {
		t.Errorf("the heavier version listed as playable: %+v", other)
	}
	if asked := p.ask(t, p.token, p.movie, chrome, map[string]any{"MediaSourceId": heavy}); !asked.refused() {
		t.Errorf("the heavier version asked for: %+v", asked)
	}
	// An app without a device profile is told to play the lighter one.
	if source := first(p.playbackInfo(t, "tv", p.token, p.movie, nil)); source.Id != light {
		t.Errorf("without a device profile: %+v", source)
	}
	// Kept or made-up URLs of the heavier version are refused.
	if status := fetch(stream(heavy)); status != http.StatusForbidden {
		t.Errorf("stream of the heavier version: %d", status)
	}
	if status := fetch(stream(light)); status != http.StatusPartialContent {
		t.Errorf("stream of the lighter version: %d", status)
	}
	if status, _, _ := fetchText(t, p.url+copied); status != http.StatusBadRequest {
		t.Errorf("remux copying the heavier version's video: %d", status)
	}

	// Below both: nothing plays.
	setLimit(500_000)
	if refused := p.ask(t, p.token, p.movie, chrome, nil); !refused.refused() {
		t.Errorf("below every version: %+v", refused)
	}

	if os.Getenv("POLYFIN_TEST_FFMPEG") == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set: Polyfin converts video with the encoders FFmpeg has")
	}
	// With video conversion, the first version is converted down to the
	// limit, or to the app's when it is lower.
	setLimit(2_000_000)
	p.permit(t, p.user, true, true, true)
	for _, tc := range []struct {
		app  int
		want string
	}{{120_000_000, "VideoBitrate=1872000&"}, {1_500_000, "VideoBitrate=1372000&"}} {
		source := first(p.ask(t, p.token, p.movie, chrome, map[string]any{"MaxStreamingBitrate": tc.app}))
		if source.Id != heavy || source.SupportsDirectPlay || !strings.Contains(source.TranscodingUrl, tc.want) ||
			!strings.HasSuffix(source.TranscodingUrl, "&allowVideoStreamCopy=false") || !strings.Contains(source.TranscodingUrl, "ContainerBitrateExceedsLimit") {
			t.Errorf("app limit %d: %+v", tc.app, source)
		}
	}
	setLimit(0)
	if source := first(p.ask(t, p.token, p.movie, chrome, nil)); source.Id != heavy || !source.SupportsDirectPlay {
		t.Errorf("once the limit is lifted: %+v", source)
	}
}

func TestMaxBitrateAppliesToLiveTv(t *testing.T) {
	addon := newTVAddon(t, false, "")
	s, token, user := tuned(t, addon)
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	channel := channels.Items[0].Id
	liveAnalysis(t, s, user, channel)
	chrome, err := os.ReadFile(playbackFixtures + "/profiles/jellyfin-web-chrome.json")
	if err != nil {
		t.Fatal(err)
	}
	if answer := s.ask(t, token, channel, chrome, nil); len(answer.MediaSources) != 1 || !answer.MediaSources[0].SupportsDirectPlay {
		t.Fatalf("without a limit: %+v", answer)
	}
	// The stream, at 1.1 Mbit/s, is above the limit, and its video may not
	// be converted.
	s.limit(t, user, func(c *accounts.UserChanges) { c.MaxBitrate, c.VideoTranscoding = new(500_000), new(false) })
	if answer := s.ask(t, token, channel, chrome, nil); !answer.refused() {
		t.Errorf("above the limit: %+v", answer)
	}
}

func TestLiveTvOffHidesTheViewAndRefusesTheRoutes(t *testing.T) {
	addon := newTVAddon(t, false, "")
	s, token, user := tuned(t, addon)
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	if len(channels.Items) == 0 {
		t.Fatal("no channel")
	}
	channel := channels.Items[0].Id
	liveAnalysis(t, s, user, channel)
	views := func() []string {
		t.Helper()
		var result QueryResult
		if status := s.get(t, "/UserViews", token, &result); status != http.StatusOK {
			t.Fatalf("views: %d", status)
		}
		var types []string
		for _, view := range result.Items {
			types = append(types, view.CollectionType)
		}
		return types
	}
	routes := []string{"/LiveTv/Info", "/LiveTv/GuideInfo", "/LiveTv/Channels", "/LiveTv/Channels/" + channel, "/LiveTv/Programs",
		"/LiveTv/Programs/Recommended", "/LiveTv/Recordings", "/LiveTv/Recordings/Folders", "/LiveTv/Timers", "/LiveTv/SeriesTimers"}
	status := func(method, path string) int {
		t.Helper()
		var body any
		if method == http.MethodPost {
			body = map[string]any{}
		}
		status, _ := s.call(method, path, app("tv", token), body)
		return status
	}
	if got := views(); !slices.Contains(got, "livetv") {
		t.Fatalf("views with Live TV: %v", got)
	}
	for _, path := range routes {
		if got := status(http.MethodGet, path); got != http.StatusOK {
			t.Errorf("%s with Live TV: %d", path, got)
		}
	}

	s.limit(t, user, func(c *accounts.UserChanges) { c.LiveTv = new(false) })
	if got := views(); slices.Contains(got, "livetv") {
		t.Errorf("views without Live TV: %v", got)
	}
	for _, path := range routes {
		if got := status(http.MethodGet, path); got != http.StatusForbidden {
			t.Errorf("%s without Live TV: %d, want 403", path, got)
		}
	}
	if got := status(http.MethodPost, "/LiveTv/Programs"); got != http.StatusForbidden {
		t.Errorf("POST /LiveTv/Programs without Live TV: %d", got)
	}
	// The channels cannot be reached another way, nor played.
	for _, path := range []string{"/Items/" + channel, "/Users/" + user.ID.String() + "/Items/" + channel} {
		if got := status(http.MethodGet, path); got != http.StatusNotFound {
			t.Errorf("%s without Live TV: %d", path, got)
		}
	}
	if got := status(http.MethodPost, "/Items/"+channel+"/PlaybackInfo"); got != http.StatusNotFound {
		t.Errorf("PlaybackInfo of a channel without Live TV: %d", got)
	}
	var listed QueryResult
	if got := s.get(t, "/Items?includeItemTypes=TvChannel&recursive=true", token, &listed); got != http.StatusOK || len(listed.Items) != 0 {
		t.Errorf("channels listed without Live TV: %d %+v", got, listed.Items)
	}
	var info LiveTvInfo
	s.limit(t, user, func(c *accounts.UserChanges) { c.LiveTv = new(true) })
	s.get(t, "/LiveTv/Info", token, &info)
	if !slices.Contains(info.EnabledUsers, user.ID.String()) {
		t.Errorf("enabled users with Live TV back: %v", info.EnabledUsers)
	}
}

func TestSyncPlayAccessLevelsMatchJellyfin(t *testing.T) {
	s := newTestServer(t, 10)
	s.user("alice", nil)
	bobUser := s.user("bob", func(c *accounts.UserChanges) { c.SyncPlay = new(accounts.SyncPlayJoin) })
	s.user("carol", func(c *accounts.UserChanges) { c.SyncPlay = new(accounts.SyncPlayNone) })
	alice, bob, carol := s.syncPlayApp(t, "alice", "alice-tv"), s.syncPlayApp(t, "bob", "bob-tv"), s.syncPlayApp(t, "carol", "carol-tv")
	status, answer := alice.send(t, s, http.MethodPost, "/SyncPlay/New", map[string]any{"GroupName": "Movie night"})
	var group GroupInfoDto
	if status != http.StatusOK || json.Unmarshal(answer, &group) != nil {
		t.Fatalf("alice creating a group: %d %s", status, answer)
	}
	type request struct {
		method, path string
		body         any
	}
	newGroup := request{http.MethodPost, "/SyncPlay/New", map[string]any{"GroupName": "Another"}}
	join := request{http.MethodPost, "/SyncPlay/Join", map[string]any{"GroupId": group.GroupId}}
	list := request{http.MethodGet, "/SyncPlay/List", nil}
	one := request{http.MethodGet, "/SyncPlay/" + group.GroupId, nil}
	ping := request{http.MethodPost, "/SyncPlay/Ping", map[string]any{"Ping": 10}}
	leave := request{http.MethodPost, "/SyncPlay/Leave", nil}
	pause := request{http.MethodPost, "/SyncPlay/Pause", nil}
	expect := func(a syncPlayApp, r request, want int) {
		t.Helper()
		if got, body := a.send(t, s, r.method, r.path, r.body); got != want {
			t.Errorf("%s %s by %s: %d %s, want %d", r.method, r.path, a.device, got, body, want)
		}
	}

	// None: every SyncPlay request is refused.
	for _, r := range []request{newGroup, join, list, one, ping, leave, pause} {
		expect(carol, r, http.StatusForbidden)
	}
	// JoinGroups: no group of one's own, but listing and joining.
	expect(bob, newGroup, http.StatusForbidden)
	expect(bob, list, http.StatusOK)
	expect(bob, one, http.StatusOK)
	expect(bob, join, http.StatusNoContent)
	// Taken away in a group, SyncPlay still lets bob act in it and leave,
	// as Jellyfin does while the user is in a group, but not list groups.
	s.limit(t, bobUser, func(c *accounts.UserChanges) { c.SyncPlay = new(accounts.SyncPlayNone) })
	expect(bob, ping, http.StatusNoContent)
	expect(bob, pause, http.StatusNoContent)
	expect(bob, list, http.StatusForbidden)
	expect(bob, leave, http.StatusNoContent)
	expect(bob, ping, http.StatusForbidden)
	expect(bob, join, http.StatusForbidden)
}

func TestRemoteControlOfOtherUsersFollowsTheFlag(t *testing.T) {
	s := newTestServer(t, 10)
	s.user("alice", nil)
	bob := s.user("bob", nil)
	administrator, err := s.store.CreateUser(t.Context(), accounts.NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true})
	if err != nil {
		t.Fatal(err)
	}
	aliceTV := s.remoteApp(t, "alice", "tv", queryMediaControl, true)
	bobPhone := s.remoteApp(t, "bob", "phone", noMediaControl, false)
	console := s.remoteApp(t, "admin", "console", noMediaControl, false)
	message := func(from remoteApp, device string) int {
		t.Helper()
		status, _ := s.call(http.MethodPost, "/Sessions/"+aliceTV.session+"/Message", app(device, from.token), map[string]string{"Text": "hello"})
		return status
	}
	controllable := func(from remoteApp, device string, user accounts.User) []string {
		t.Helper()
		return sessionIDs(s.sessions(t, from.token, "controllableByUserId="+user.ID.String()))
	}
	// By default, a member controls their own apps only, an administrator
	// everyone's.
	if status := message(bobPhone, "phone"); status != http.StatusForbidden {
		t.Errorf("bob by default: %d", status)
	}
	if status := message(console, "console"); status != http.StatusNoContent {
		t.Errorf("the administrator by default: %d", status)
	}
	if got := controllable(console, "console", administrator); !slices.Equal(got, []string{aliceTV.session}) {
		t.Errorf("the administrator may control %v", got)
	}
	if got := controllable(bobPhone, "phone", bob); len(got) != 0 {
		t.Errorf("bob may control %v", got)
	}

	s.limit(t, bob, func(c *accounts.UserChanges) { c.RemoteControl = new(true) })
	s.limit(t, administrator, func(c *accounts.UserChanges) { c.RemoteControl = new(false) })
	if status := message(bobPhone, "phone"); status != http.StatusNoContent {
		t.Errorf("bob allowed: %d", status)
	}
	if got := controllable(bobPhone, "phone", bob); !slices.Equal(got, []string{aliceTV.session}) {
		t.Errorf("bob allowed may control %v", got)
	}
	if status := message(console, "console"); status != http.StatusForbidden {
		t.Errorf("the administrator without it: %d", status)
	}
	if got := controllable(console, "console", administrator); len(got) != 0 {
		t.Errorf("the administrator without it may control %v", got)
	}
}

func TestPolicyCarriesPlaybackAndAccessLimits(t *testing.T) {
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
	limits := func(p map[string]any) [5]any {
		return [5]any{p["MaxActiveSessions"], p["RemoteClientBitrateLimit"], p["EnableLiveTvAccess"], p["SyncPlayAccess"], p["EnableRemoteControlOfOtherUsers"]}
	}
	post := func(body string, want int) {
		t.Helper()
		if status, answer := s.postRaw(path+"/Policy", app("tv", adminToken), body); status != want {
			t.Fatalf("policy %s: %d %s, want %d", body, status, answer, want)
		}
	}
	defaults := [5]any{0.0, 0.0, true, "CreateAndJoinGroups", false}
	if got := limits(policy()); got != defaults {
		t.Errorf("a new user's policy: %v", got)
	}
	// The administrator's app posts the whole policy it read, changed.
	read := policy()
	read["MaxActiveSessions"], read["RemoteClientBitrateLimit"], read["EnableLiveTvAccess"] = 3, 4_000_000, false
	read["SyncPlayAccess"], read["EnableRemoteControlOfOtherUsers"] = "JoinGroups", true
	encoded, _ := json.Marshal(read)
	post(string(encoded), http.StatusNoContent)
	if got := limits(policy()); got != [5]any{3.0, 4_000_000.0, false, "JoinGroups", true} {
		t.Errorf("policy after the change: %v", got)
	}
	stored, err := s.store.User(t.Context(), member.ID)
	if err != nil || stored.MaxPlaybacks != 3 || stored.MaxBitrate != 4_000_000 || stored.LiveTv || stored.SyncPlay != accounts.SyncPlayJoin || !stored.RemoteControl {
		t.Errorf("stored: %+v %v", stored, err)
	}
	// Like Jellyfin, enumerations are read without regard to case, or by
	// number, and limits of zero or less are none.
	post(`{"SyncPlayAccess":"none","MaxActiveSessions":-1,"RemoteClientBitrateLimit":-5}`, http.StatusNoContent)
	if got := limits(policy()); got != [5]any{0.0, 0.0, true, "None", false} {
		t.Errorf("policy with a lowercase name: %v", got)
	}
	post(`{"SyncPlayAccess":1,"MaxActiveSessions":20}`, http.StatusNoContent)
	if got := limits(policy()); got[0] != 20.0 || got[3] != "JoinGroups" {
		t.Errorf("policy with a number: %v", got)
	}
	// A policy that leaves them out sets Jellyfin's defaults.
	post(`{"IsHidden":true}`, http.StatusNoContent)
	if got := limits(policy()); got != defaults {
		t.Errorf("policy after a policy without them: %v", got)
	}
	for _, refused := range []string{`{"SyncPlayAccess":"Sometimes"}`, `{"SyncPlayAccess":3}`, `{"MaxActiveSessions":21}`, `{"RemoteClientBitrateLimit":2147483648}`} {
		post(refused, http.StatusBadRequest)
	}
	if got := limits(policy()); got != defaults {
		t.Errorf("policy after refused ones: %v", got)
	}
}
