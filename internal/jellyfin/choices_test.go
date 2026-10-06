package jellyfin

import (
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// scriptedProbe is a fake ffprobe that counts its runs, then runs body,
// where $n is the number of the run, from 1. `exec cat "$0.json"` answers
// the recorded analysis.
func scriptedProbe(t *testing.T, body string) fakeProbe {
	t.Helper()
	f := newFakeProbe(t, true)
	script := "#!/bin/sh\necho run >> \"$0.runs\"\nn=$(wc -l < \"$0.runs\")\n" + body
	if err := os.WriteFile(f.path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return f
}

// A source that does not answer holds up PlaybackInfo only for a while
// once the next version is ready: that version plays, and the first, still
// being read, is left out of the list.
func TestSlowAnalysesGiveWayToTheNextVersion(t *testing.T) {
	probe := scriptedProbe(t, "[ $n -gt 1 ] || exec sleep 60\nexec cat \"$0.json\"\n")
	p := playingOn(t, newProbingServer(t, 10, probe.path))
	p.setting(t, func(settings *accounts.Settings) { settings.AnalysisTimeout = accounts.MinAnalysisTimeout })
	started := time.Now()
	answer := p.ask(t, p.token, p.movie, p.profile(t, "jellyfin-web-chrome"), nil)
	elapsed := time.Since(started)
	// The second version's analysis is known.
	if len(answer.MediaSources) != 1 || answer.MediaSources[0].Id != p.versions[1].ID.String() || probe.runs() != 1 {
		t.Fatalf("after a source that did not answer: %d runs, %+v", probe.runs(), answer)
	}
	// The analysis timeout, longer, would still be waiting.
	if elapsed < versionPatience || elapsed > versionPatience+time.Second {
		t.Errorf("PlaybackInfo answered after %s, want about %s", elapsed, versionPatience)
	}
}

// unreadableFirst makes a scripted ffprobe fail its first run, as on a
// source that is not media, and answer the next ones.
const unreadableFirst = "[ $n -gt 1 ] || { echo 'Invalid data found when processing input' >&2; exit 1; }\nexec cat \"$0.json\"\n"

// unreadable makes a scripted ffprobe fail every run.
const unreadable = "echo 'Invalid data found when processing input' >&2\nexit 1\n"

// When the app chooses no version, PlaybackInfo analyzes VersionAttempts
// of them at most, at once; the others play only when analyzed before.
func TestVersionAttemptsBoundTheVersionsAnalyzed(t *testing.T) {
	for _, attempts := range []int{1, 2, 3} {
		probe := scriptedProbe(t, unreadable)
		p := playingOn(t, newProbingServer(t, 10, probe.path))
		p.setting(t, func(settings *accounts.Settings) { settings.VersionAttempts = attempts })
		// Unlike the first title's, the other title's two versions were
		// not analyzed before, and cannot be read.
		p.versionsOf(t, p.remote)
		answer := p.ask(t, p.token, p.remote, p.profile(t, "jellyfin-web-chrome"), nil)
		if runs := probe.runs(); runs != min(attempts, 2) || !answer.refused() {
			t.Errorf("%d attempts: %d versions analyzed, %+v", attempts, runs, answer)
		}
	}
	// The first title's second version, analyzed before, plays however
	// few may be analyzed now.
	probe := scriptedProbe(t, unreadable)
	p := playingOn(t, newProbingServer(t, 10, probe.path))
	p.setting(t, func(settings *accounts.Settings) { settings.VersionAttempts = 1 })
	if source := firstSource(t, p.ask(t, p.token, p.movie, p.profile(t, "jellyfin-web-chrome"), nil)); source.Id != p.versions[1].ID.String() || probe.runs() != 1 {
		t.Errorf("one attempt: %d runs, %+v", probe.runs(), source)
	}
}

// threeStreamsTV is an addon whose live TV catalog has one channel with
// three streams.
func threeStreamsTV(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "tv", Name: "TV", Version: "1", Types: []string{"tv"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "tv", ID: "channels", Name: "Channels"}}})
		case "/catalog/tv/channels.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{{ID: "tv:one", Type: "tv", Name: "One"}}})
		case "/stream/tv/tv%3Aone.json":
			var streams []stremio.Stream
			for n := range 3 {
				streams = append(streams, stremio.Stream{Name: "Live " + strconv.Itoa(n), URL: server.URL + "/live/" + strconv.Itoa(n) + ".m3u8"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": streams})
		case "/live/0.m3u8", "/live/1.m3u8", "/live/2.m3u8":
			// Live playlists: their start is checked before ffprobe reads them.
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXTINF:2,\nsegment.ts\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// A channel's streams are tried as a title's versions are.
func TestVersionAttemptsBoundTheStreamsOfAChannel(t *testing.T) {
	chrome, err := os.ReadFile(filepath.Join(playbackFixtures, "profiles", "jellyfin-web-chrome.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, attempts := range []int{1, 2} {
		probe := scriptedProbe(t, unreadableFirst)
		s := newProbingServer(t, 10, probe.path)
		s.user("member", nil)
		if _, err := s.addons.Install(t.Context(), addons.Shared(), threeStreamsTV(t), false); err != nil {
			t.Fatal(err)
		}
		token := s.signIn("member", "tv")
		s.setting(t, func(settings *accounts.Settings) { settings.VersionAttempts = attempts })
		var channels QueryResult
		s.get(t, "/LiveTv/Channels", token, &channels)
		if len(channels.Items) != 1 {
			t.Fatalf("channels: %+v", channels.Items)
		}
		answer := s.ask(t, token, channels.Items[0].Id, chrome, nil)
		switch {
		case probe.runs() != attempts:
			t.Errorf("%d attempts: %d streams analyzed", attempts, probe.runs())
		case attempts == 1 && !answer.refused():
			t.Errorf("one attempt, on a stream that cannot be read: %+v", answer)
		case attempts == 2 && (len(answer.MediaSources) != 1 || !strings.HasSuffix(answer.MediaSources[0].Name, "1")):
			t.Errorf("two attempts: %+v", answer)
		}
	}
}

// firstSource is the version PlaybackInfo chose.
func firstSource(t *testing.T, answer playbackAnswer) MediaSourceInfo {
	t.Helper()
	if len(answer.MediaSources) == 0 {
		t.Fatalf("no version plays: %+v", answer)
	}
	return answer.MediaSources[0]
}

// The minimal profile direct plays H.264 and AAC in MP4: the first version,
// Opus and H.264 in Matroska, needs its audio converted; the second plays
// as it is.
func TestPreferDirectPlayChoosesAVersionThatNeedsNoConversion(t *testing.T) {
	p := playing(t)
	p.remuxable(t)
	// The second version's keyframe index, which a remux copying its tracks
	// needs.
	index := binary.AppendVarint(binary.AppendVarint(nil, 0), 6_000_000)
	if _, err := p.pool.Exec(t.Context(), "INSERT INTO media_keyframes (version_id, keyframes) VALUES ($1, $2)", p.versions[1].ID, index); err != nil {
		t.Fatal(err)
	}
	minimal := p.profile(t, "minimal")
	if source := firstSource(t, p.ask(t, p.token, p.movie, minimal, nil)); source.Id != p.movie ||
		!strings.HasSuffix(source.TranscodingUrl, "&allowAudioStreamCopy=false") {
		t.Errorf("by default, the first version plays, converted: %+v", source)
	}
	p.setting(t, func(settings *accounts.Settings) { settings.PreferDirectPlay = true })
	// The first version, passed over, is left out rather than listed after
	// the second: the versions keep their order.
	answer := p.ask(t, p.token, p.movie, minimal, nil)
	if source := firstSource(t, answer); source.Id != p.versions[1].ID.String() || !source.SupportsDirectPlay || source.TranscodingUrl != "" ||
		len(answer.MediaSources) != 1 {
		t.Errorf("preferring direct play: %+v", answer)
	}
	// Repackaged with its tracks copied, a version needs no conversion
	// either.
	if source := firstSource(t, p.ask(t, p.token, p.movie, minimal, map[string]any{"EnableDirectPlay": false})); source.Id != p.versions[1].ID.String() ||
		source.TranscodingUrl == "" || strings.Contains(source.TranscodingUrl, "StreamCopy=false") {
		t.Errorf("preferring a copy: %+v", source)
	}
	// The version the app asks for plays, converted as it needs.
	if source := firstSource(t, p.ask(t, p.token, p.movie, minimal, map[string]any{"MediaSourceId": p.versions[0].ID.String()})); source.Id != p.movie ||
		!strings.HasSuffix(source.TranscodingUrl, "&allowAudioStreamCopy=false") {
		t.Errorf("the version asked for: %+v", source)
	}
	// When every version needs a conversion, the first plays, as without
	// the setting.
	if source := firstSource(t, p.ask(t, p.token, p.movie, minimal, map[string]any{"EnableDirectPlay": false, "AllowAudioStreamCopy": false})); source.Id != p.movie {
		t.Errorf("no version without conversion: %+v", source)
	}
}

// A remux copying the tracks needs the version's keyframe index: a version
// whose index cannot be read is not preferred.
func TestPreferDirectPlaySkipsRemuxesThatCannotStart(t *testing.T) {
	p := playing(t)
	p.remuxable(t)
	p.setting(t, func(settings *accounts.Settings) { settings.PreferDirectPlay = true })
	if source := firstSource(t, p.ask(t, p.token, p.movie, p.profile(t, "minimal"), map[string]any{"EnableDirectPlay": false})); source.Id != p.movie ||
		!strings.HasSuffix(source.TranscodingUrl, "&allowAudioStreamCopy=false") {
		t.Errorf("chosen: %+v", source)
	}
}

// A version above the user's bitrate limit never counts as one the app
// plays without conversion: the first version plays, its audio converted,
// rather than the second, 3 Mbit/s, which the app would play as it is.
func TestPreferDirectPlayKeepsUnderTheUsersBitrateLimit(t *testing.T) {
	p := playing(t)
	p.remuxable(t)
	p.withBitrate(t, p.versions[1], "h264-aac-mp4", 3_000_000)
	minimal := p.profile(t, "minimal")
	p.setting(t, func(settings *accounts.Settings) { settings.PreferDirectPlay = true })
	if source := firstSource(t, p.ask(t, p.token, p.movie, minimal, nil)); source.Id != p.versions[1].ID.String() || !source.SupportsDirectPlay {
		t.Fatalf("without a limit: %+v", source)
	}
	p.limit(t, p.user, func(c *accounts.UserChanges) { c.MaxBitrate = new(2_000_000) })
	if source := firstSource(t, p.ask(t, p.token, p.movie, minimal, nil)); source.Id != p.movie ||
		!strings.HasSuffix(source.TranscodingUrl, "&allowAudioStreamCopy=false") {
		t.Errorf("under a limit: %+v", source)
	}
	// An app without a device profile plays any version as it is: the
	// first, 3 Mbit/s, is passed over for the second, 1 Mbit/s.
	q := playing(t)
	heavy := q.remuxable(t)
	heavy.Bitrate = 3_000_000
	q.storeAnalysis(t, q.versions[0], heavy)
	q.withBitrate(t, q.versions[1], "h264-aac-mp4", 1_000_000)
	q.setting(t, func(settings *accounts.Settings) { settings.PreferDirectPlay = true })
	q.limit(t, q.user, func(c *accounts.UserChanges) { c.MaxBitrate = new(2_000_000) })
	if source := firstSource(t, q.playbackInfo(t, "tv", q.token, q.movie, nil)); source.Id != q.versions[1].ID.String() {
		t.Errorf("without a device profile, under a limit: %+v", source)
	}
}

func TestConversionsAtOnceAreLimited(t *testing.T) {
	if os.Getenv("POLYFIN_TEST_FFMPEG") == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set: Polyfin converts video with the encoders FFmpeg has")
	}
	p := playing(t)
	p.remuxable(t)
	guest := p.testServer.user("guest", nil)
	guestToken := p.signIn("guest", "phone")
	if _, err := p.addons.Install(t.Context(), addons.Shared(), newTVAddon(t, false, "").url, false); err != nil {
		t.Fatal(err)
	}
	var channels QueryResult
	p.get(t, "/LiveTv/Channels", guestToken, &channels)
	if len(channels.Items) == 0 {
		t.Fatal("no channel")
	}
	channel := channels.Items[0].Id
	liveAnalysis(t, p.testServer, guest, channel)
	chrome, minimal := p.profile(t, "jellyfin-web-chrome"), p.profile(t, "minimal")
	video := map[string]any{"MediaSourceId": p.movie, "EnableDirectPlay": false, "AllowVideoStreamCopy": false}
	liveVideo := map[string]any{"AllowVideoStreamCopy": false}
	converted := func(token, item string, profile json.RawMessage, more map[string]any) (string, bool) {
		t.Helper()
		answer := p.ask(t, token, item, profile, more)
		if len(answer.MediaSources) == 0 {
			return "", false
		}
		target := answer.MediaSources[0].TranscodingUrl
		return target, strings.Contains(target, "allowVideoStreamCopy=false")
	}
	segment := func(target string, n int) int {
		t.Helper()
		base, query, _ := strings.Cut(target, "master.m3u8?")
		status, _, _ := fetchText(t, p.url+base+"hls1/main/"+strconv.Itoa(n)+".mp4?"+query)
		return status
	}

	p.setting(t, func(settings *accounts.Settings) { settings.MaxConversions = 1 })
	// Planned while no video is converted, so before the limit is reached.
	guestURL, guestConverted := converted(guestToken, p.movie, chrome, video)
	memberURL, memberConverted := converted(p.token, p.movie, chrome, video)
	if _, live := converted(guestToken, channel, minimal, liveVideo); !guestConverted || !memberConverted || !live {
		t.Fatalf("below the limit: %q, %q, live %v", guestURL, memberURL, live)
	}
	if status := segment(memberURL, 0); status != http.StatusOK {
		t.Fatalf("the member's first segment: %d", status)
	}

	// At the limit, no other video conversion is planned, files and live
	// alike, and none starts.
	if answer := p.ask(t, guestToken, p.movie, chrome, video); !answer.refused() {
		t.Errorf("a file at the limit: %+v", answer)
	}
	if answer := p.ask(t, guestToken, channel, minimal, liveVideo); !answer.refused() {
		t.Errorf("a channel at the limit: %+v", answer)
	}
	if status := segment(guestURL, 0); status != http.StatusServiceUnavailable {
		t.Errorf("a conversion starting past the limit: %d", status)
	}
	// Copies go on.
	if target, convertedVideo := converted(guestToken, p.movie, chrome, map[string]any{"MediaSourceId": p.movie, "EnableDirectPlay": false}); target == "" || convertedVideo {
		t.Errorf("a copy at the limit: %q", target)
	}
	if target, convertedVideo := converted(guestToken, channel, minimal, nil); target == "" || convertedVideo {
		t.Errorf("a live copy at the limit: %q", target)
	}
	// The playback running goes on: its next segments, a seek, and the
	// title asked again, as apps do to switch tracks.
	for _, n := range []int{1, 2, 0} {
		if status := segment(memberURL, n); status != http.StatusOK {
			t.Errorf("the member's segment %d at the limit: %d", n, status)
		}
	}
	if _, again := converted(p.token, p.movie, chrome, video); !again {
		t.Error("the member's title asked again at the limit is not converted")
	}

	// A higher limit applies at once.
	p.setting(t, func(settings *accounts.Settings) { settings.MaxConversions = 2 })
	if status := segment(guestURL, 0); status != http.StatusOK {
		t.Errorf("a conversion below a raised limit: %d", status)
	}
}

// Converted video is scaled down to fit the height cap's 16:9 frame,
// keeping its shape; copied video keeps its size.
func TestConvertedVideoKeepsUnderTheHeightCap(t *testing.T) {
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set: Polyfin converts video with the encoders FFmpeg has")
	}
	p := playing(t)
	// The file is 64 × 64; its analysis says 1920 × 800, which FFmpeg
	// scales as the analysis asks.
	analysis := p.remuxable(t)
	analysis.Streams[1].Width, analysis.Streams[1].Height = 1920, 800
	data, _ := json.Marshal(analysis)
	if _, err := p.pool.Exec(t.Context(), "UPDATE media_analyses SET analysis = $2 WHERE version_id = $1", p.versions[0].ID, data); err != nil {
		t.Fatal(err)
	}
	chrome := p.profile(t, "jellyfin-web-chrome")
	video := map[string]any{"MediaSourceId": p.movie, "EnableDirectPlay": false, "AllowVideoStreamCopy": false}
	copied := map[string]any{"MediaSourceId": p.movie, "EnableDirectPlay": false}
	master := func(more map[string]any) (string, string) {
		t.Helper()
		answer := p.ask(t, p.token, p.movie, chrome, more)
		if len(answer.MediaSources) != 1 || answer.MediaSources[0].TranscodingUrl == "" {
			t.Fatalf("PlaybackInfo: %+v", answer)
		}
		target := answer.MediaSources[0].TranscodingUrl
		_, _, playlist := fetchText(t, p.url+target)
		return target, playlist
	}
	if _, playlist := master(video); !strings.Contains(playlist, "RESOLUTION=1920x800") {
		t.Errorf("converted without a cap:\n%s", playlist)
	}
	p.setting(t, func(settings *accounts.Settings) { settings.MaxConversionHeight = 480 })
	// 854 × 480 at most, at the bitrate of 480p, 2 Mb/s, at most 3 Mb/s
	// with the audio's allowance.
	target, playlist := master(video)
	if !strings.Contains(playlist, "BANDWIDTH=3640000,") || !strings.Contains(playlist, "RESOLUTION=854x354") {
		t.Errorf("converted under 480 lines:\n%s", playlist)
	}
	if _, playlist := master(copied); !strings.Contains(playlist, "RESOLUTION=1920x800") {
		t.Errorf("copied under 480 lines:\n%s", playlist)
	}
	base, query, _ := strings.Cut(p.url+target, "master.m3u8?")
	status, _, init := fetchText(t, base+"hls1/main/-1.mp4?"+query)
	_, _, first := fetchText(t, base+"hls1/main/0.mp4?"+query)
	path := filepath.Join(t.TempDir(), "0.mp4")
	if err := os.WriteFile(path, []byte(init+first), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := exec.Command(filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), "-v", "error", "-select_streams", "v",
		"-show_entries", "stream=width,height", "-of", "csv=p=0", path).Output()
	if got := strings.TrimSpace(string(out)); status != http.StatusOK || got != "854,354" {
		t.Errorf("segment: %d, video %q", status, got)
	}
}
