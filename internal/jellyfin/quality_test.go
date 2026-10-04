package jellyfin

import (
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/stremio"
)

// group sets a user's quality group.
func (s testServer) group(t *testing.T, user accounts.User, height int) {
	t.Helper()
	s.limit(t, user, func(c *accounts.UserChanges) { c.QualityGroup = &height })
}

// sized stores an analysis of a version with its video another size: the
// movie's first version is a real Matroska file, which remuxes read; the
// second is H.264 and AAC in MP4.
func (p playbackSetup) sized(t *testing.T, version int, width, height int) {
	t.Helper()
	var analysis media.Analysis
	if version == 0 {
		analysis = p.remuxable(t)
	} else {
		probe, err := os.ReadFile(filepath.Join(playbackFixtures, "probes", "h264-aac-mp4.json"))
		if err != nil {
			t.Fatal(err)
		}
		if analysis, err = media.Parse(probe); err != nil {
			t.Fatal(err)
		}
		analysis.Remote = true
	}
	for i := range analysis.Streams {
		if analysis.Streams[i].Type == "video" {
			analysis.Streams[i].Width, analysis.Streams[i].Height = width, height
		}
	}
	p.storeAnalysis(t, p.versions[version], analysis)
}

// videoHeightOf is the height a media source describes its video at.
func videoHeightOf(source MediaSourceInfo) int {
	for _, stream := range source.MediaStreams {
		if stream.Type == "Video" && stream.Height != nil {
			return *stream.Height
		}
	}
	return 0
}

// indexed stores a keyframe index for the movie's second version, which
// streaming it over HLS needs: its file is not a real MP4.
func (p playbackSetup) indexed(t *testing.T) {
	t.Helper()
	index := binary.AppendVarint(binary.AppendVarint(nil, 0), 6_000_000)
	if _, err := p.pool.Exec(t.Context(), "INSERT INTO media_keyframes (version_id, keyframes) VALUES ($1, $2)", p.versions[1].ID, index); err != nil {
		t.Fatal(err)
	}
}

// fetchStatus fetches the first bytes of a URL.
func fetchStatus(t *testing.T, target string) int {
	t.Helper()
	response, _ := fetchURL(t, target, http.Header{"Range": {"bytes=0-3"}})
	return response.StatusCode
}

// needsEncoders skips the rest of a test when tests are given no FFmpeg:
// Polyfin converts video with the encoders FFmpeg has, so without one a
// version taller than the group plays nowhere.
func needsEncoders(t *testing.T) {
	t.Helper()
	if os.Getenv("POLYFIN_TEST_FFMPEG") == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set: Polyfin converts video with the encoders FFmpeg has")
	}
}

// The movie's first version is 4K, its second 1080p: a group offers the
// versions that fit, in their order, and when none fits, all of them,
// the closest to the group first, converted down.
func TestQualityGroupsLeaveOutTallerVersions(t *testing.T) {
	p := playing(t)
	p.sized(t, 0, 3840, 2160)
	p.sized(t, 1, 1920, 1080)
	p.indexed(t)
	chrome := p.profile(t, "jellyfin-web-chrome")
	tall, fitting := p.versions[0].ID.String(), p.versions[1].ID.String()
	etags := func(answer playbackAnswer) []string {
		var tags []string
		for _, source := range answer.MediaSources {
			tags = append(tags, source.ETag)
		}
		return tags
	}

	// Original, the default: both, the first playing as it is.
	answer := p.ask(t, p.token, p.movie, chrome, nil)
	if !slices.Equal(etags(answer), []string{tall, fitting}) || !answer.MediaSources[0].SupportsDirectPlay {
		t.Fatalf("Original: %+v", answer)
	}

	// 1080p: only the version that fits, under the title's identifier.
	p.group(t, p.user, 1080)
	answer = p.ask(t, p.token, p.movie, chrome, nil)
	if !slices.Equal(etags(answer), []string{fitting}) || answer.MediaSources[0].Id != p.movie || !answer.MediaSources[0].SupportsDirectPlay {
		t.Errorf("1080p: %+v", answer)
	}
	if asked := p.ask(t, p.token, p.movie, chrome, map[string]any{"MediaSourceId": tall}); !asked.refused() {
		t.Errorf("the 4K version asked for under 1080p: %+v", asked)
	}
	var details BaseItemDto
	p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &details)
	if details.MediaSources == nil || len(*details.MediaSources) != 1 || (*details.MediaSources)[0].ETag != fitting {
		t.Errorf("details under 1080p: %+v", details.MediaSources)
	}

	// 720p: none fits, so both are kept, the 1080p one first, converted
	// down to 720 lines, as Jellyfin reports a resolution limit; the other
	// is not offered as it is.
	needsEncoders(t)
	p.group(t, p.user, 720)
	answer = p.ask(t, p.token, p.movie, chrome, nil)
	if !slices.Equal(etags(answer), []string{fitting, tall}) {
		t.Fatalf("720p: %+v", answer)
	}
	first, other := answer.MediaSources[0], answer.MediaSources[1]
	if first.SupportsDirectPlay || !strings.Contains(first.TranscodingUrl, "VideoResolutionNotSupported") ||
		!strings.HasSuffix(first.TranscodingUrl, "&allowVideoStreamCopy=false") || videoHeightOf(first) != 720 {
		t.Errorf("720p, the version played: direct play %v, height %d, %s", first.SupportsDirectPlay, videoHeightOf(first), first.TranscodingUrl)
	}
	if other.SupportsDirectPlay || other.SupportsDirectStream {
		t.Errorf("720p, the other version offered as it is: %+v", other)
	}
}

// threeHeightsAddon serves a movie with three versions, labeled 2160p,
// 1080p and 720p in that order, each a real Matroska file.
func threeHeightsAddon(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		meta := stremio.Meta{ID: "tt3000", Type: "movie", Name: "Movie", Runtime: "2h"}
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "heights", Name: "Heights", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{meta}})
		case path == "/meta/movie/tt3000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
		case path == "/stream/movie/tt3000.json":
			var streams []stremio.Stream
			for _, label := range []string{"2160p", "1080p", "720p"} {
				streams = append(streams, stremio.Stream{Name: "Source " + label, Description: "WEB-DL", URL: server.URL + "/files/" + label + ".mkv",
					BehaviorHints: stremio.StreamBehavior{Filename: "Movie." + label + ".mkv"}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": streams})
		case strings.HasPrefix(path, "/files/"):
			http.ServeFile(w, r, filepath.Join("..", "container", "testdata", "forced.mkv"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// When no version fits, the one converted is the closest to the group, so
// as to read and decode no more than needed: by their labels before they
// are analyzed, by their analyses after.
func TestQualityGroupsConvertTheClosestVersionWhenNoneFits(t *testing.T) {
	s := newTestServer(t, 10)
	user := s.user("member", nil)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), threeHeightsAddon(t), false); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	if len(page.Items) != 1 {
		t.Fatalf("items: %+v", page.Items)
	}
	movie := page.Items[0].Id
	id, _ := accounts.ParseID(movie)
	versions, err := s.library.Versions(t.Context(), user, id)
	if err != nil || len(versions) != 3 {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	want := []string{versions[2].ID.String(), versions[1].ID.String(), versions[0].ID.String()}
	s.group(t, user, 480)

	var details BaseItemDto
	s.get(t, "/Users/"+user.ID.String()+"/Items/"+movie, token, &details)
	var listed []string
	for _, source := range *details.MediaSources {
		listed = append(listed, source.ETag)
	}
	if !slices.Equal(listed, want) {
		t.Errorf("details, by labels: %v, want 720p, 1080p, 2160p %v", listed, want)
	}

	needsEncoders(t)
	info, err := os.Stat(filepath.Join("..", "container", "testdata", "forced.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	for i, height := range []int{2160, 1080, 720} {
		analysis := media.Analysis{Format: "matroska,webm", Duration: 15008 * time.Millisecond, Size: info.Size(), Bitrate: 25_000, Remote: true,
			Streams: []media.Stream{
				{Index: 0, Type: "audio", Codec: "opus", Default: true, Channels: 1, SampleRate: 8000, ChannelLayout: "mono"},
				{Index: 1, Type: "video", Codec: "h264", Profile: "High", Level: 10, Width: height * 16 / 9, Height: height,
					FrameRate: 24, AverageRate: 24, PixelFormat: "yuv420p", BitDepth: 8},
			}}
		data, _ := json.Marshal(analysis)
		if _, err := s.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", versions[i].ID, data); err != nil {
			t.Fatal(err)
		}
	}
	chrome, err := os.ReadFile(filepath.Join(playbackFixtures, "profiles", "jellyfin-web-chrome.json"))
	if err != nil {
		t.Fatal(err)
	}
	answer := s.ask(t, token, movie, chrome, nil)
	var offered []string
	for _, source := range answer.MediaSources {
		offered = append(offered, source.ETag)
	}
	if !slices.Equal(offered, want) {
		t.Fatalf("PlaybackInfo, by analyses: %v, want 720p, 1080p, 2160p %v", offered, want)
	}
	first := answer.MediaSources[0]
	if first.SupportsDirectPlay || !strings.Contains(first.TranscodingUrl, "VideoResolutionNotSupported") ||
		!strings.HasSuffix(first.TranscodingUrl, "&allowVideoStreamCopy=false") || videoHeightOf(first) != 480 {
		t.Errorf("the 720p version: direct play %v, height %d, %s", first.SupportsDirectPlay, videoHeightOf(first), first.TranscodingUrl)
	}
}

// Versions left out are not tried, nor read to analyze them: a version
// that fits and cannot be read gives way to none taller.
func TestQualityGroupsSkipTallerVersionsWhenTrying(t *testing.T) {
	for _, test := range []struct {
		name, probe string
		group       int
		runs        int
		played      int // -1 for none
	}{
		{"Original", "exec cat \"$0.json\"\n", 0, 1, 0},
		{"1080p", "exec cat \"$0.json\"\n", 1080, 1, 1},
		{"1080p, the version that fits unreadable", unreadableFirst, 1080, 1, -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			probe := scriptedProbe(t, test.probe)
			p := playingOn(t, newProbingServer(t, 10, probe.path))
			p.setting(t, func(settings *accounts.Settings) { settings.VersionAttempts = 2 })
			p.group(t, p.user, test.group)
			// The other title's versions, 2160p and 1080p by their labels,
			// were never analyzed.
			versions := p.versionsOf(t, p.remote)
			answer := p.ask(t, p.token, p.remote, p.profile(t, "jellyfin-web-chrome"), nil)
			switch {
			case probe.runs() != test.runs:
				t.Errorf("%d versions analyzed, want %d", probe.runs(), test.runs)
			case test.played < 0 && !answer.refused():
				t.Errorf("played: %+v", answer)
			case test.played >= 0 && (len(answer.MediaSources) == 0 || answer.MediaSources[0].ETag != versions[test.played].ID.String()):
				t.Errorf("played: %+v, want version %d", answer, test.played)
			}
			if _, analyzed := p.handler.Playback.Analyzed(t.Context(), versions[0].ID); test.group != 0 && analyzed {
				t.Error("the 2160p version was analyzed")
			}
		})
	}
}

// Converted video is scaled to the lower of the group and the server's cap;
// a version that fits plays as it is, and no taller one is sent at full
// size, even from kept or made-up URLs.
func TestQualityGroupsConvertDownAndKeepTallerVersionsFromPlayingAsTheyAre(t *testing.T) {
	p := playing(t)
	p.sized(t, 0, 3840, 2160)
	p.sized(t, 1, 1920, 1080)
	p.indexed(t)
	chrome := p.profile(t, "jellyfin-web-chrome")
	tall, closest := p.versions[0].ID.String(), p.versions[1].ID.String()
	stream := func(version string) string {
		return p.url + "/Videos/" + p.movie + "/stream?static=true&mediaSourceId=" + version + "&ApiKey=" + p.token
	}
	// A remux copying the 4K version's video, kept from before the group.
	copied := firstSource(t, p.ask(t, p.token, p.movie, chrome, map[string]any{"MediaSourceId": p.movie, "EnableDirectPlay": false})).TranscodingUrl
	if copied == "" || strings.Contains(copied, "allowVideoStreamCopy=false") {
		t.Fatalf("remux without a group: %q", copied)
	}

	// 4K fits a 4K group: it plays as it is.
	p.group(t, p.user, 2160)
	if source := firstSource(t, p.ask(t, p.token, p.movie, chrome, nil)); source.ETag != tall || !source.SupportsDirectPlay {
		t.Errorf("4K group: %+v", source)
	}
	if status := fetchStatus(t, stream(tall)); status != http.StatusPartialContent {
		t.Errorf("stream under a 4K group: %d", status)
	}

	// Under 720p, both versions are taller: neither is sent as it is, nor
	// copied, even from kept or made-up URLs.
	p.group(t, p.user, 720)
	if status := fetchStatus(t, stream(tall)); status != http.StatusForbidden {
		t.Errorf("stream of the 4K version under 720p: %d", status)
	}
	if status, _, _ := fetchText(t, p.url+copied); status != http.StatusBadRequest {
		t.Errorf("remux copying the 4K version's video under 720p: %d", status)
	}
	// The title's own identifier stands for its first version that fits.
	p.group(t, p.user, 1080)
	if status := fetchStatus(t, stream(p.movie)); status != http.StatusPartialContent {
		t.Errorf("stream of the title under 1080p: %d", status)
	}

	// Under 720p, the 1080p version, the closer, is converted down to the
	// lower of the group and the server's cap, its video described at the
	// size sent.
	needsEncoders(t)
	for _, test := range []struct {
		cap, height int
		resolution  string
	}{{0, 720, "RESOLUTION=1280x720"}, {1080, 720, "RESOLUTION=1280x720"}, {480, 480, "RESOLUTION=852x480"}} {
		p.setting(t, func(settings *accounts.Settings) { settings.MaxConversionHeight = test.cap })
		p.group(t, p.user, 720)
		source := firstSource(t, p.ask(t, p.token, p.movie, chrome, nil))
		if source.ETag != closest || source.SupportsDirectPlay || videoHeightOf(source) != test.height {
			t.Errorf("cap %d: height %d, %+v", test.cap, videoHeightOf(source), source)
			continue
		}
		if status, _, playlist := fetchText(t, p.url+source.TranscodingUrl); status != http.StatusOK || !strings.Contains(playlist, test.resolution) {
			t.Errorf("cap %d: %d\n%s", test.cap, status, playlist)
		}
	}
}

// A user who may not have video converted is refused versions taller than
// their group as other conversions are refused, never sent them as they
// are; with the conversion allowed, they play converted.
func TestQualityGroupsRefuseTallerVersionsWithoutConversion(t *testing.T) {
	p := playing(t)
	p.sized(t, 0, 3840, 2160)
	p.sized(t, 1, 1920, 1080)
	chrome := p.profile(t, "jellyfin-web-chrome")
	p.group(t, p.user, 720)
	p.permit(t, p.user, false, true, true)
	if answer := p.ask(t, p.token, p.movie, chrome, nil); !answer.refused() {
		t.Errorf("without the user's permission: %+v", answer)
	}
	if answer := p.playbackInfo(t, "tv", p.token, p.movie, nil); !answer.refused() {
		t.Errorf("without the user's permission, nor a device profile: %+v", answer)
	}
	p.permit(t, p.user, true, true, true)
	p.switches(t, false, true)
	if answer := p.ask(t, p.token, p.movie, chrome, nil); !answer.refused() {
		t.Errorf("with the server's conversion off: %+v", answer)
	}
	// The version that fits under 1080p plays as it is all the same.
	p.group(t, p.user, 1080)
	if source := firstSource(t, p.ask(t, p.token, p.movie, chrome, nil)); source.ETag != p.versions[1].ID.String() || !source.SupportsDirectPlay {
		t.Errorf("1080p with the server's conversion off: %+v", source)
	}
	// Both allowed again, under 720p, a version plays converted.
	needsEncoders(t)
	p.switches(t, true, true)
	p.group(t, p.user, 720)
	if answer := p.ask(t, p.token, p.movie, chrome, nil); len(answer.MediaSources) == 0 || answer.MediaSources[0].SupportsDirectPlay {
		t.Errorf("with conversion: %+v", answer)
	}
}

// A channel's stream taller than the group is converted down to it, once
// analyzed.
func TestQualityGroupsConvertLiveTvDown(t *testing.T) {
	addon := newTVAddon(t, false, "")
	s, token, user := tuned(t, addon)
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	channel := channels.Items[0].Id
	liveAnalysis(t, s, user, channel)
	id, _ := accounts.ParseID(channel)
	versions, _ := s.library.Versions(t.Context(), user, id)
	var analysis media.Analysis
	var data []byte
	if err := s.pool.QueryRow(t.Context(), "SELECT analysis FROM media_analyses WHERE version_id = $1", versions[0].ID).Scan(&data); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &analysis); err != nil {
		t.Fatal(err)
	}
	for i := range analysis.Streams {
		if analysis.Streams[i].Type == "video" {
			analysis.Streams[i].Width, analysis.Streams[i].Height = 1920, 1080
		}
	}
	data, _ = json.Marshal(analysis)
	if _, err := s.pool.Exec(t.Context(), "UPDATE media_analyses SET analysis = $2 WHERE version_id = $1", versions[0].ID, data); err != nil {
		t.Fatal(err)
	}
	chrome, err := os.ReadFile(filepath.Join(playbackFixtures, "profiles", "jellyfin-web-chrome.json"))
	if err != nil {
		t.Fatal(err)
	}
	stream := s.url + "/Videos/" + channel + "/stream?static=true&ApiKey=" + token
	s.group(t, user, 1080)
	if answer := s.ask(t, token, channel, chrome, nil); len(answer.MediaSources) != 1 || !answer.MediaSources[0].SupportsDirectPlay {
		t.Fatalf("1080p: %+v", answer)
	}
	// Under 720p, the stream is never sent as it is, and the user who may
	// not convert is refused it.
	s.group(t, user, 720)
	if status := fetchStatus(t, stream); status != http.StatusForbidden {
		t.Errorf("the stream as it is under 720p: %d", status)
	}
	s.permit(t, user, false, true, true)
	if answer := s.ask(t, token, channel, chrome, nil); !answer.refused() {
		t.Errorf("720p without conversion: %+v", answer)
	}
	// With the conversion allowed, it is converted down to 720 lines.
	needsEncoders(t)
	s.permit(t, user, true, true, true)
	answer := s.ask(t, token, channel, chrome, nil)
	if len(answer.MediaSources) != 1 {
		t.Fatalf("720p: %+v", answer)
	}
	source := answer.MediaSources[0]
	if source.SupportsDirectPlay || !strings.Contains(source.TranscodingUrl, "VideoResolutionNotSupported") ||
		!strings.HasSuffix(source.TranscodingUrl, "&allowVideoStreamCopy=false") || videoHeightOf(source) != 720 {
		t.Errorf("720p: direct play %v, height %d, %s", source.SupportsDirectPlay, videoHeightOf(source), source.TranscodingUrl)
	}
}

// Downloads are never converted: only versions that fit download.
func TestQualityGroupsLimitDownloads(t *testing.T) {
	p := playing(t)
	p.sized(t, 0, 3840, 2160)
	p.sized(t, 1, 1920, 1080)
	download := func(item string) (int, string) {
		t.Helper()
		response, _ := fetchURL(t, p.url+"/Items/"+item+"/Download?ApiKey="+p.token, nil)
		return response.StatusCode, response.Header.Get("Content-Disposition")
	}
	canDownload := func() bool {
		var details BaseItemDto
		p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &details)
		return details.CanDownload != nil && *details.CanDownload
	}
	if status, name := download(p.movie); status != http.StatusOK || !strings.Contains(name, "Movie.2160p.mkv") || !canDownload() {
		t.Fatalf("Original: %d %q", status, name)
	}
	p.group(t, p.user, 1080)
	if status, _ := download(p.versions[0].ID.String()); status != http.StatusForbidden {
		t.Errorf("the 4K version under 1080p: %d", status)
	}
	if status, _ := download(p.versions[1].ID.String()); status != http.StatusOK {
		t.Errorf("the 1080p version under 1080p: %d", status)
	}
	if status, name := download(p.movie); status != http.StatusOK || !strings.Contains(name, "Movie.1080p.mp4") || !canDownload() {
		t.Errorf("the title under 1080p: %d %q", status, name)
	}
	p.group(t, p.user, 720)
	for _, item := range []string{p.movie, p.versions[1].ID.String()} {
		if status, _ := download(item); status != http.StatusForbidden {
			t.Errorf("%s under 720p: %d", item, status)
		}
	}
	if canDownload() {
		t.Error("the title can be downloaded under 720p")
	}
}

// Jellyfin's policy has no quality group: apps neither see it nor reset it.
func TestPolicyKeepsTheQualityGroup(t *testing.T) {
	s := newTestServer(t, 10)
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	member := s.user("member", nil)
	adminToken := s.signIn("admin", "tv")
	s.group(t, member, 1080)
	path := "/Users/" + member.ID.String()
	var user struct{ Policy map[string]any }
	if status := s.get(t, path, adminToken, &user); status != http.StatusOK {
		t.Fatalf("user: %d", status)
	}
	for name := range user.Policy {
		if strings.Contains(strings.ToLower(name), "quality") || strings.Contains(strings.ToLower(name), "height") {
			t.Errorf("the policy shows %s", name)
		}
	}
	encoded, _ := json.Marshal(user.Policy)
	for _, body := range []string{string(encoded), `{"IsHidden":true}`} {
		if status, answer := s.postRaw(path+"/Policy", app("tv", adminToken), body); status != http.StatusNoContent {
			t.Fatalf("policy: %d %s", status, answer)
		}
		if stored, err := s.store.User(t.Context(), member.ID); err != nil || stored.QualityGroup != 1080 {
			t.Errorf("after posting %.40s…: %+v %v", body, stored.QualityGroup, err)
		}
	}
}
