package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/media"
)

// directPlayOffInfo asks PlaybackInfo for media source source of item on
// the app of profile, with direct play turned off, and returns the source
// it answers.
func directPlayOffInfo(t *testing.T, p playbackSetup, item, source, profile string) MediaSourceInfo {
	t.Helper()
	status, data := p.call(http.MethodPost, "/Items/"+item+"/PlaybackInfo", app("tv", p.token), map[string]any{
		"UserId": p.user.ID.String(), "MediaSourceId": source, "MaxStreamingBitrate": 120_000_000, "EnableDirectPlay": false,
		"DeviceProfile": p.profile(t, profile)})
	var response playbackInfoResponse
	if err := json.Unmarshal(data, &response); status != http.StatusOK || err != nil || len(response.MediaSources) != 1 {
		t.Fatalf("%d %s", status, data)
	}
	return response.MediaSources[0]
}

// transcodeReasons are the TranscodeReasons of a TranscodingUrl.
func transcodeReasons(t *testing.T, target string) string {
	t.Helper()
	query, err := url.ParseQuery(strings.SplitN(target, "?", 2)[len(strings.SplitN(target, "?", 2))-1])
	if err != nil {
		t.Fatal(err)
	}
	return query.Get("TranscodeReasons")
}

// The Jellyfin app for Android asks again with direct play turned off
// after two playback errors within 30 seconds (#542): a file its
// integrated player plays as it is is converted for its MPEG-TS profile,
// which takes H.264 only, with DirectPlayError beside the codec's reason,
// as Jellyfin 12.4 answers, and its name says it is the app's request.
func TestDirectPlayTurnedOffByTheAppIsAReason(t *testing.T) {
	t.Parallel()
	if os.Getenv("POLYFIN_TEST_FFMPEG") == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set: Polyfin converts with the encoders FFmpeg has")
	}
	p := playing(t)
	// The file the addon serves, described as #542's: HEVC Main 10 in SDR
	// and E-AC-3 in 5.1.
	analysis := p.remuxable(t)
	analysis.Bitrate = 2_306_513
	analysis.Streams = []media.Stream{
		{Index: 0, Type: "audio", Codec: "eac3", Default: true, Channels: 6, SampleRate: 48000, ChannelLayout: "5.1(side)", Bitrate: 640_000},
		{Index: 1, Type: "video", Codec: "hevc", Profile: "Main 10", Level: 120, Width: 1920, Height: 960, FrameRate: 24, AverageRate: 24,
			PixelFormat: "yuv420p10le", BitDepth: 10, ColorTransfer: "bt709", ColorPrimaries: "bt709", ColorSpace: "bt709"},
	}
	data, _ := json.Marshal(analysis)
	if _, err := p.pool.Exec(t.Context(), "UPDATE media_analyses SET analysis = $2 WHERE version_id = $1", p.versions[0].ID, data); err != nil {
		t.Fatal(err)
	}
	source := directPlayOffInfo(t, p, p.movie, p.movie, "jellyfin-android-integrated")
	if source.SupportsDirectPlay || source.TranscodingContainer != "ts" || !strings.Contains(source.TranscodingUrl, "&VideoCodec=h264&") {
		t.Fatalf("source: %+v", source)
	}
	if got := transcodeReasons(t, source.TranscodingUrl); got != "VideoCodecNotSupported,DirectPlayError" {
		t.Errorf("TranscodeReasons %q, want VideoCodecNotSupported,DirectPlayError", got)
	}
	if !strings.HasSuffix(source.Name, "(converted: app’s request)") {
		t.Errorf("name %q", source.Name)
	}
}

// A Blu-ray disc never plays as it is: the app turning direct play off
// changes nothing, and its reasons stay those of its streaming, without
// the direct-play check, which its MPEG-TS would fail in Chrome.
func TestDiscsSkipTheDirectPlayCheckWhenTheAppTurnsItOff(t *testing.T) {
	t.Parallel()
	p, local := localBluRay(t, "ffprobe-not-installed")
	id, err := accounts.ParseID(local.Id)
	if err != nil {
		t.Fatal(err)
	}
	analysis := media.Analysis{Format: "mpegts", Duration: 190 * time.Second, Bitrate: 8_000_000,
		Streams: []media.Stream{
			{Index: 0, Type: "video", Codec: "h264", Profile: "High", Level: 40, Width: 1920, Height: 1080, FrameRate: 24, AverageRate: 24,
				PixelFormat: "yuv420p", BitDepth: 8},
			{Index: 1, Type: "audio", Codec: "ac3", Default: true, Channels: 6, SampleRate: 48000, Bitrate: 448_000},
		}}
	data, _ := json.Marshal(analysis)
	if _, err := p.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", id, data); err != nil {
		t.Fatal(err)
	}
	source := directPlayOffInfo(t, p, p.movie, local.Id, "jellyfin-web-chrome")
	if source.SupportsDirectPlay || source.TranscodingSubProtocol != "hls" {
		t.Fatalf("source: %+v", source)
	}
	if got := transcodeReasons(t, source.TranscodingUrl); got != "AudioCodecNotSupported" {
		t.Errorf("TranscodeReasons %q, want AudioCodecNotSupported", got)
	}
}

// A DASH channel never plays as it is either: the app turning direct play
// off leaves its reasons as they were, without the direct-play check,
// which its DASH manifest would fail. A channel an app plays as it is
// gets DirectPlayError, as a file does.
func TestDASHChannelsSkipTheDirectPlayCheckWhenTheAppTurnsItOff(t *testing.T) {
	t.Parallel()
	provider := newIPTVProvider(t)
	s := newTestServer(t, 10)
	user := s.user("member", nil)
	token := s.signIn("member", "tv")
	addon, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Provider", Guide: provider.url + "/epg.xml",
		Account: iptv.Account{Kind: addons.KindM3U, URL: provider.url + "/get.php?username=user&password=secret"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.library.RefreshGuide(t.Context(), addons.Shared(), addons.LibraryKey{AddonID: addon.ID, CatalogType: "tv", CatalogID: "channels"}); err != nil {
		t.Fatal(err)
	}
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	if len(channels.Items) != 2 {
		t.Fatalf("channels: %+v", channels.Items)
	}
	profile, err := os.ReadFile(filepath.Join(playbackFixtures, "profiles", "jellyfin-web-chrome.json"))
	if err != nil {
		t.Fatal(err)
	}
	for i, format := range []string{"dash", "hls"} {
		channel := channels.Items[i].Id
		id, _ := accounts.ParseID(channel)
		versions, err := s.library.Versions(t.Context(), user, id)
		if err != nil || len(versions) != 1 {
			t.Fatalf("channel streams: %v %v", versions, err)
		}
		analysis := media.Analysis{Format: format, Remote: true, Streams: []media.Stream{
			{Index: 0, Type: "video", Codec: "h264", Profile: "High", Level: 40, Width: 1280, Height: 720, FrameRate: 25, AverageRate: 25,
				PixelFormat: "yuv420p", BitDepth: 8},
			{Index: 1, Type: "audio", Codec: "aac", Default: true, Channels: 2, SampleRate: 48000},
		}}
		data, _ := json.Marshal(analysis)
		if _, err := s.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", versions[0].ID, data); err != nil {
			t.Fatal(err)
		}
		status, body := s.call(http.MethodPost, "/Items/"+channel+"/PlaybackInfo", app("tv", token),
			map[string]any{"DeviceProfile": json.RawMessage(profile), "IsPlayback": true, "AutoOpenLiveStream": true, "EnableDirectPlay": false})
		var info playbackInfoResponse
		if status != http.StatusOK || json.Unmarshal(body, &info) != nil || len(info.MediaSources) != 1 {
			t.Fatalf("PlaybackInfo of a %s channel: %d %s", format, status, body)
		}
		source := info.MediaSources[0]
		if source.SupportsDirectPlay || source.TranscodingUrl == "" {
			t.Fatalf("a %s channel: %+v", format, source)
		}
		if got := transcodeReasons(t, source.TranscodingUrl); got != "DirectPlayError" {
			t.Errorf("a %s channel's TranscodeReasons %q, want DirectPlayError", format, got)
		}
	}
}
