package jellyfin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/stremio"
)

// tvAddon serves a live TV catalog of two channels, the first of which
// streams the HLS files in dir (a fixed playlist when dir is empty), and,
// when guide is set, a Native EPG guide for today: a programme airing now
// and the next. guides counts the guide pages asked for.
type tvAddon struct {
	url    string
	guides atomic.Int32
}

func newTVAddon(t *testing.T, guide bool, dir string) *tvAddon {
	t.Helper()
	a := &tvAddon{}
	var server *httptest.Server
	now := time.Now().UTC()
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		channels := []stremio.Meta{
			{ID: "tv:one", Type: "tv", Name: "One", Poster: server.URL + "/logo.png"},
			{ID: "tv:two", Type: "tv", Name: "Two", Logo: server.URL + "/logo.png"},
		}
		switch {
		case path == "/manifest.json":
			manifest := stremio.Manifest{ID: "tv", Name: "TV", Version: "1", Types: []string{"tv"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "tv", ID: "channels", Name: "Channels", Extra: []stremio.Extra{{Name: "date"}}}}}
			manifest.BehaviorHints.EpgProvider = guide
			_ = json.NewEncoder(w).Encode(manifest)
		case strings.HasPrefix(path, "/catalog/tv/channels/date="):
			a.guides.Add(1)
			if strings.Contains(path, now.Format(time.DateOnly)) {
				channels[0].Videos = []stremio.Video{
					{ID: "now", Title: "Now", Overview: "The news.", Genres: []string{"News"}, StartTime: now.Add(-time.Hour).Format(time.RFC3339), EndTime: now.Add(time.Hour).Format(time.RFC3339)},
					{ID: "next", Title: "Next", Genres: []string{"Movie"}, StartTime: now.Add(time.Hour).Format(time.RFC3339), EndTime: now.Add(3 * time.Hour).Format(time.RFC3339)},
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metasDetailed": channels})
		case path == "/catalog/tv/channels.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": channels})
		case path == "/stream/tv/tv%3Aone.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{{Name: "Live", URL: server.URL + "/live/one.m3u8"}}})
		case path == "/live/one.m3u8" && dir == "":
			_, _ = io.WriteString(w, "#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:7\n#EXTINF:2,\nseg7.ts\n")
		case path == "/live/seg7.ts" && dir == "":
			http.ServeContent(w, r, "seg7.ts", time.Time{}, strings.NewReader("segment bytes"))
		case strings.HasPrefix(path, "/live/") && dir != "":
			http.ServeFile(w, r, filepath.Join(dir, strings.TrimPrefix(path, "/live/")))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	a.url = server.URL + "/manifest.json"
	return a
}

// tuned installs a TV addon, whose live TV catalog the installation
// enables, and signs a member in.
func tuned(t *testing.T, addon *tvAddon) (testServer, string, accounts.User) {
	t.Helper()
	s := newTestServer(t, 10)
	user := s.user("member", nil)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), addon.url, false); err != nil {
		t.Fatal(err)
	}
	return s, s.signIn("member", "tv"), user
}

var liveShapes = shapeRules{
	dynamic: []string{"ImageTags", "ImageBlurHashes", "ProviderIds", "RequiredHttpHeaders"},
	absent: map[string][]string{
		// Polyfin's views and channels have no path, and Jellyfin's sort
		// name of its own views is not Polyfin's to give.
		"Path": {"*"}, "ForcedSortName": {"*"},
		// Jellyfin's views have a null parent, which Polyfin leaves out.
		"ParentId": {"livetv-view"},
		// The recorded channel played as a transcoding, the test one as it
		// is; Polyfin relays the headers sources need instead of naming them.
		"TranscodingUrl": {"livetv-playback-info"}, "TranscodingContainer": {"livetv-playback-info"},
		// Polyfin opens nothing ahead of playback (RequiresOpening is
		// false), and has no tuner settings.
		"LiveStreamId": {"*"}, "AnalyzeDurationMs": {"*"}, "FallbackMaxStreamingBitrate": {"*"}, "Size": {"*"},
		// Polyfin's analysis gave no profile level for the seeded stream.
		"DefaultAudioStreamIndex": {"*"},
	},
	returned: map[string][]string{
		// The recorded channel had no guide, and Polyfin sends what every
		// item carries.
		"CurrentProgram": {"*"},
		"Name":           {"*"}, "ETag": {"*"}, "VideoType": {"*"},
		// Jellyfin leaves these out of programmes; Polyfin sends them on
		// every item, and images whether asked or not.
		"IsFolder":     {"livetv-program", "livetv-guided-programs", "livetv-guided-recommended"},
		"LocationType": {"livetv-program", "livetv-guided-programs", "livetv-guided-recommended"},
		"ImageTags":    {"livetv-guided-programs"}, "BackdropImageTags": {"livetv-channels", "livetv-guided-programs"},
		"PrimaryImageAspectRatio": {"livetv-channel-items"}, "Container": {"livetv-channel"},
		// The recorded stream had no language and its bitrate was unknown.
		"BitRate": {"livetv-playback-info"}, "IsAVC": {"livetv-playback-info"}, "Language": {"livetv-playback-info"},
	},
}

func compareLive(t *testing.T, fixture string, body []byte) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.1", fixture+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var want, got any
	_ = json.Unmarshal(raw, &want)
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("%s: %v in %s", fixture, err, body)
	}
	for _, difference := range compareShapes(fixture, want, got, liveShapes) {
		t.Error(difference)
	}
}

func TestTVCatalogsListAsChannels(t *testing.T) {
	addon := newTVAddon(t, true, "")
	s, token, _ := tuned(t, addon)

	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	if len(views.Items) != 1 || views.Items[0].CollectionType != "livetv" || views.Items[0].Type != "UserView" {
		t.Fatalf("views: a live TV catalog must give the Live TV view and no library: %+v", views.Items)
	}
	_, view := s.call(http.MethodGet, "/Items/"+views.Items[0].Id, app("tv", token), nil)
	_, body := s.call(http.MethodGet, "/UserViews", app("tv", token), nil)
	var raw struct{ Items []json.RawMessage }
	_ = json.Unmarshal(body, &raw)
	compareLive(t, "livetv-view", raw.Items[0])
	if !strings.Contains(string(view), `"livetv"`) {
		t.Errorf("the Live TV view by id: %s", view)
	}

	status, body := s.call(http.MethodGet, "/LiveTv/Channels?fields=PrimaryImageAspectRatio&startIndex=0&enableImageTypes=Primary", app("tv", token), nil)
	var channels QueryResult
	if status != http.StatusOK || json.Unmarshal(body, &channels) != nil || channels.TotalRecordCount != 2 {
		t.Fatalf("channels: %d %s", status, body)
	}
	compareLive(t, "livetv-channels", body)
	one, two := channels.Items[0], channels.Items[1]
	if one.Name != "One" || one.Type != "TvChannel" || one.Number != "1" || two.ChannelNumber != "2" || one.MediaType != "Video" {
		t.Errorf("channels are numbered in catalog order: %+v / %+v", one, two)
	}
	// A channel's logo is its poster, or its logo when it has no poster.
	if one.ImageTags["Primary"] == "" || two.ImageTags["Primary"] == "" {
		t.Errorf("channel logos: %v %v", one.ImageTags, two.ImageTags)
	}
	if one.CurrentProgram == nil || one.CurrentProgram.Name != "Now" || two.CurrentProgram != nil {
		t.Errorf("current programmes: %+v %+v", one.CurrentProgram, two.CurrentProgram)
	}

	_, body = s.call(http.MethodGet, "/LiveTv/Channels/"+one.Id, app("tv", token), nil)
	compareLive(t, "livetv-channel", body)
	_, body = s.call(http.MethodGet, "/Items?recursive=true&includeItemTypes=TvChannel", app("tv", token), nil)
	compareLive(t, "livetv-channel-items", body)
	// The player's details of a channel, and listings of channels, carry
	// what it airs now, as Jellyfin's do.
	var listed QueryResult
	var detail BaseItemDto
	if json.Unmarshal(body, &listed) != nil || len(listed.Items) != 2 || listed.Items[0].CurrentProgram == nil {
		t.Errorf("channel items without their current programme: %s", body)
	}
	_, body = s.call(http.MethodGet, "/Items/"+one.Id, app("tv", token), nil)
	if json.Unmarshal(body, &detail) != nil || detail.CurrentProgram == nil || detail.CurrentProgram.Name != "Now" {
		t.Errorf("channel details without their current programme: %s", body)
	}

	minEnd := url.QueryEscape(time.Now().UTC().Format(time.RFC3339))
	status, body = s.call(http.MethodGet, "/LiveTv/Programs?channelIds="+one.Id+"&MinEndDate="+minEnd+"&SortBy=StartDate&EnableImages=false&EnableUserData=false", app("tv", token), nil)
	var programs QueryResult
	if status != http.StatusOK || json.Unmarshal(body, &programs) != nil || len(programs.Items) != 2 {
		t.Fatalf("programmes: %d %s", status, body)
	}
	compareLive(t, "livetv-guided-programs", body)
	if programs.Items[0].Name != "Now" || programs.Items[0].IsNews == nil || programs.Items[1].IsMovie == nil ||
		*programs.Items[0].ChannelId != one.Id {
		t.Errorf("programmes, by start time with their categories: %s", body)
	}
	_, body = s.call(http.MethodGet, "/LiveTv/Programs?IsMovie=true", app("tv", token), nil)
	if json.Unmarshal(body, &programs) != nil || len(programs.Items) != 1 || programs.Items[0].Name != "Next" {
		t.Errorf("movies: %s", body)
	}
	_, body = s.call(http.MethodGet, "/LiveTv/Programs/Recommended?IsAiring=true&Fields=ChannelInfo", app("tv", token), nil)
	if json.Unmarshal(body, &programs) != nil || len(programs.Items) != 1 || programs.Items[0].ChannelName != "One" {
		t.Errorf("airing now: %s", body)
	}
	compareLive(t, "livetv-guided-recommended", body)
	status, body = s.call(http.MethodGet, "/LiveTv/Programs/"+programs.Items[0].Id, app("tv", token), nil)
	if status != http.StatusOK {
		t.Fatalf("programme: %d %s", status, body)
	}
	compareLive(t, "livetv-program", body)
}

// The On Now rows ask what airs now: only today's guide is read for it,
// not the week ahead.
func TestProgrammesAiringNowReadTodaysGuideOnly(t *testing.T) {
	addon := newTVAddon(t, true, "")
	s, token, _ := tuned(t, addon)
	var programs QueryResult
	s.get(t, "/LiveTv/Programs/Recommended?IsAiring=true", token, &programs)
	if len(programs.Items) != 1 || programs.Items[0].Name != "Now" {
		t.Fatalf("airing now: %+v", programs.Items)
	}
	if n := addon.guides.Load(); n > 2 {
		t.Errorf("%d guide pages read for what airs now", n)
	}
}

// jellyfin-web's guide posts its query once its page of channels makes a
// long URL, with the channels, the sort and the fields as comma-separated
// strings, which Jellyfin reads as lists.
func TestGuidePostsChannelsAsOneString(t *testing.T) {
	addon := newTVAddon(t, true, "")
	s, token, _ := tuned(t, addon)
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	if len(channels.Items) != 2 {
		t.Fatalf("channels: %+v", channels.Items)
	}
	now := time.Now().UTC()
	status, body := s.call(http.MethodPost, "/LiveTv/Programs", app("web", token), map[string]any{
		"UserId":                 "",
		"MaxStartDate":           now.Add(24 * time.Hour).Format("2006-01-02T15:04:05.000Z"),
		"MinEndDate":             now.Format("2006-01-02T15:04:05.000Z"),
		"channelIds":             channels.Items[0].Id + "," + strings.Repeat("0", 32) + ",",
		"ImageTypeLimit":         1,
		"EnableImages":           false,
		"SortBy":                 "StartDate",
		"EnableTotalRecordCount": false,
		"EnableUserData":         false,
		"Fields":                 "IsHD",
	})
	var programs QueryResult
	if status != http.StatusOK || json.Unmarshal(body, &programs) != nil || !slices.Equal(programNames(programs), []string{"Now", "Next"}) {
		t.Fatalf("the guide's posted query: %d %s", status, body)
	}
	// Genres are separated by "|", as their names may hold commas.
	status, body = s.call(http.MethodPost, "/LiveTv/Programs", app("web", token), map[string]any{"Genres": "Talk, Late|News"})
	if status != http.StatusOK || json.Unmarshal(body, &programs) != nil || !slices.Equal(programNames(programs), []string{"Now"}) {
		t.Errorf("posted genres: %d %s", status, body)
	}
}

// TestGuideAnswersAsJellyfinWithoutData checks the answers of a live TV
// catalog that publishes no guide: empty lists, as Jellyfin gives without
// guide data, without asking the addon for a guide.
func TestGuideAnswersAsJellyfinWithoutData(t *testing.T) {
	addon := newTVAddon(t, false, "")
	s, token, _ := tuned(t, addon)
	for fixture, path := range map[string]string{
		"livetv-guide-info":        "/LiveTv/GuideInfo",
		"livetv-recommended":       "/LiveTv/Programs/Recommended?IsAiring=true&limit=12&EnableTotalRecordCount=false&Fields=ChannelInfo,PrimaryImageAspectRatio",
		"livetv-programs":          "/LiveTv/Programs?HasAired=false&limit=9&IsMovie=true&EnableTotalRecordCount=false&Fields=ChannelInfo",
		"livetv-recordings":        "/LiveTv/Recordings?IsInProgress=true",
		"livetv-recording-folders": "/LiveTv/Recordings/Folders",
		"livetv-timers":            "/LiveTv/Timers?IsActive=false&IsScheduled=true",
		"livetv-series-timers":     "/LiveTv/SeriesTimers?SortBy=SortName&SortOrder=Ascending",
		"livetv-info":              "/LiveTv/Info",
	} {
		status, body := s.call(http.MethodGet, path, app("tv", token), nil)
		if status != http.StatusOK {
			t.Fatalf("%s: %d %s", path, status, body)
		}
		compareLive(t, fixture, body)
		if strings.HasPrefix(fixture, "livetv-programs") && strings.Contains(string(body), `"Id"`) {
			t.Errorf("%s lists programmes without a guide: %s", path, body)
		}
	}
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	if len(channels.Items) != 2 || channels.Items[0].CurrentProgram != nil {
		t.Errorf("channels without a guide: %+v", channels.Items)
	}
	if n := addon.guides.Load(); n != 0 {
		t.Errorf("an addon without a guide was asked for %d guide pages", n)
	}
}

// liveAnalysis seeds the analysis of a channel's HLS stream: H.264 and AAC,
// as ffprobe reads a live playlist.
func liveAnalysis(t *testing.T, s testServer, user accounts.User, channel string) {
	t.Helper()
	id, _ := accounts.ParseID(channel)
	versions, err := s.library.Versions(t.Context(), user, id)
	if err != nil || len(versions) != 1 {
		t.Fatalf("channel streams: %v %v", versions, err)
	}
	probe, err := os.ReadFile(filepath.Join(playbackFixtures, "probes", "h264-aac-mp4.json"))
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := media.Parse(probe)
	if err != nil {
		t.Fatal(err)
	}
	analysis.Format, analysis.Duration, analysis.Remote = "hls", 0, true
	data, _ := json.Marshal(analysis)
	if _, err := s.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", versions[0].ID, data); err != nil {
		t.Fatal(err)
	}
}

func livePlaybackInfo(t *testing.T, s testServer, token, channel, profile string) MediaSourceInfo {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(playbackFixtures, "profiles", profile+".json"))
	if err != nil {
		t.Fatal(err)
	}
	status, body := s.call(http.MethodPost, "/Items/"+channel+"/PlaybackInfo", app("tv", token),
		map[string]any{"DeviceProfile": json.RawMessage(data), "IsPlayback": true, "AutoOpenLiveStream": true})
	var info playbackInfoResponse
	if status != http.StatusOK || json.Unmarshal(body, &info) != nil || len(info.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo of a channel: %d %s", status, body)
	}
	raw, _ := os.ReadFile(filepath.Join("testdata", "jellyfin-12.1", "livetv", "playback-info.json"))
	var recorded struct{ Opened json.RawMessage }
	_ = json.Unmarshal(raw, &recorded)
	var want, got any
	_ = json.Unmarshal(recorded.Opened, &want)
	_ = json.Unmarshal(body, &got)
	for _, difference := range compareShapes("livetv-playback-info", want, got, liveShapes) {
		t.Error(difference)
	}
	return info.MediaSources[0]
}

// Channels have no rating: a user under parental control lists and opens
// them as everyone does, from the server's TV catalogs.
func TestChannelsStayWithUsersUnderParentalControl(t *testing.T) {
	addon := newTVAddon(t, true, "")
	s, _, _ := tuned(t, addon)
	s.user("child", func(c *accounts.UserChanges) {
		c.Parental = &accounts.ParentalControl{MaxRating: new(13), BlockUnrated: []string{"Movie", "Series"}}
	})
	token := s.signIn("child", "tablet")
	var channels QueryResult
	if status := s.get(t, "/LiveTv/Channels", token, &channels); status != http.StatusOK || channels.TotalRecordCount != 2 {
		t.Fatalf("channels of a user under parental control: %d %+v", status, channels)
	}
	if status, body := s.call(http.MethodGet, "/LiveTv/Channels/"+channels.Items[0].Id, app("tablet", token), nil); status != http.StatusOK {
		t.Errorf("a channel opened by a user under parental control: %d %s", status, body)
	}
}

func TestChannelPlaysItsHLSStreamDirectly(t *testing.T) {
	addon := newTVAddon(t, false, "")
	s, token, user := tuned(t, addon)
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	channel := channels.Items[0].Id
	liveAnalysis(t, s, user, channel)

	source := livePlaybackInfo(t, s, token, channel, "jellyfin-web-chrome")
	if !source.IsInfiniteStream || !source.SupportsDirectPlay || source.RunTimeTicks != nil || source.RequiresOpening {
		t.Fatalf("a live source the app plays as it is: %+v", source)
	}
	// The test addon is on a local address: the playlist is relayed, its
	// segments named through Polyfin.
	response, err := http.Get(source.Path)
	if err != nil {
		t.Fatal(err)
	}
	playlist, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if !strings.Contains(string(playlist), "#EXT-X-MEDIA-SEQUENCE:7") || !strings.Contains(string(playlist), "/Videos/"+channel+"/live/seg7.ts?") {
		t.Fatalf("relayed playlist: %s", playlist)
	}
	var segment string
	for line := range strings.Lines(string(playlist)) {
		if strings.HasPrefix(line, "http") {
			segment = strings.TrimSpace(line)
		}
	}
	response, err = http.Get(segment)
	if err != nil {
		t.Fatal(err)
	}
	bytes, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || string(bytes) != "segment bytes" {
		t.Errorf("relayed segment: %d %q", response.StatusCode, bytes)
	}
	// Parts of a file, as EXT-X-BYTERANGE asks them, are relayed as parts.
	request, _ := http.NewRequest(http.MethodGet, segment, nil)
	request.Header.Set("Range", "bytes=0-6")
	if response, err := http.DefaultClient.Do(request); err != nil {
		t.Error(err)
	} else {
		part, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusPartialContent || string(part) != "segment" || response.Header.Get("Content-Range") != "bytes 0-6/13" {
			t.Errorf("relayed byte range: %d %q %v", response.StatusCode, part, response.Header)
		}
	}
	// A link Polyfin did not sign relays nothing.
	forged := strings.Replace(segment, "target=", "target=aHR0cDovL2V4YW1wbGUuY29tLw", 1)
	if response, err := http.Get(forged); err != nil || response.StatusCode != http.StatusUnauthorized {
		t.Errorf("forged link: %v %v", response, err)
	}

	// A playing channel appears in the device's session.
	report := map[string]any{"ItemId": channel, "MediaSourceId": source.Id, "PlayMethod": "DirectPlay"}
	if status, body := s.call(http.MethodPost, "/Sessions/Playing", app("tv", token), report); status != http.StatusNoContent {
		t.Fatalf("playback report: %d %s", status, body)
	}
	var sessions []SessionInfo
	s.get(t, "/Sessions", token, &sessions)
	if len(sessions) != 1 || sessions[0].NowPlayingItem == nil || sessions[0].NowPlayingItem.Type != "TvChannel" {
		t.Errorf("sessions while a channel plays: %+v", sessions)
	}

	// A channel plays only while the user has the live TV catalog that
	// lists it.
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), nil); err != nil {
		t.Fatal(err)
	}
	if status, _ := s.call(http.MethodPost, "/Items/"+channel+"/PlaybackInfo", app("tv", token), map[string]any{}); status != http.StatusNotFound {
		t.Errorf("PlaybackInfo of a channel whose catalog was removed: %d", status)
	}
	if response, err := http.Get(segment); err != nil || response.StatusCode != http.StatusNotFound {
		t.Errorf("a segment of a channel whose catalog was removed: %v %v", response, err)
	}
}

func TestChannelPlaysThroughFFmpeg(t *testing.T) {
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	dir := t.TempDir()
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24:duration=12",
		"-f", "lavfi", "-i", "sine=duration=12", "-c:v", "libx264", "-g", "24", "-pix_fmt", "yuv420p", "-c:a", "aac",
		"-f", "hls", "-hls_time", "2", "-hls_list_size", "0", filepath.Join(dir, "one.m3u8"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	addon := newTVAddon(t, false, dir)
	s, token, user := tuned(t, addon)
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	channel := channels.Items[0].Id
	liveAnalysis(t, s, user, channel)

	// Swiftfin's AVPlayer profile lists no HLS direct play: FFmpeg remuxes
	// the stream into fragmented MP4 segments.
	source := livePlaybackInfo(t, s, token, channel, "swiftfin-native")
	if source.SupportsDirectPlay || source.TranscodingUrl == "" || source.TranscodingContainer != "mp4" {
		t.Fatalf("a live source remuxed: %+v", source)
	}
	get := func(path string) (int, string) {
		status, body := s.call(http.MethodGet, path, "", nil)
		return status, string(body)
	}
	status, master := get(source.TranscodingUrl)
	if status != http.StatusOK || !strings.Contains(master, "live.m3u8?") {
		t.Fatalf("master playlist: %d %s", status, master)
	}
	base := source.TranscodingUrl[:strings.Index(source.TranscodingUrl, "master.m3u8")]
	query := source.TranscodingUrl[strings.Index(source.TranscodingUrl, "?"):]
	status, media := get(base + "live.m3u8" + query)
	if status != http.StatusOK || !strings.Contains(media, "#EXTINF") || !strings.Contains(media, `URI="hls/live/init.mp4?`) {
		t.Fatalf("live playlist: %d %s", status, media)
	}
	var first string
	for line := range strings.Lines(media) {
		if strings.HasPrefix(line, "hls/live/") {
			first = strings.TrimSpace(line)
			break
		}
	}
	if status, segment := get(base + first); status != http.StatusOK || len(segment) == 0 {
		t.Fatalf("live segment %s: %d", first, status)
	}
	// Leaving stops FFmpeg: its segments are gone.
	session := source.TranscodingUrl[strings.Index(source.TranscodingUrl, "PlaySessionId=")+len("PlaySessionId="):]
	session = session[:strings.Index(session, "&")]
	if status, _ := s.call(http.MethodDelete, "/Videos/ActiveEncodings?playSessionId="+session, app("tv", token), nil); status != http.StatusNoContent {
		t.Fatalf("stopping: %d", status)
	}
	if status, _ := get(base + first); status != http.StatusNotFound {
		t.Errorf("a segment after the app left: %d", status)
	}
}
