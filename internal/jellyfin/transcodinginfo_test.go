package jellyfin

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/stremio"
)

// hevcSetup is a movie whose local version holds HEVC Main 10 SDR video
// and E-AC-3 5.1 audio in Matroska, as in #542, which Chromium browsers
// play with the video copied and the audio converted.
type hevcSetup struct {
	playbackSetup
	local MediaSourceInfo
}

// newHEVCSetup makes the file with the FFmpeg of POLYFIN_TEST_FFMPEG and
// links it to the movie, beside the addon's two versions.
func newHEVCSetup(t *testing.T) hevcSetup {
	t.Helper()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set: the version is streamed by FFmpeg")
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "Movie (2008)"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "Movie (2008)", "Movie (2008).mkv")
	if out, err := exec.Command(ffmpeg, "-v", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24:duration=12",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=12:sample_rate=48000", "-map", "0:v", "-map", "1:a",
		"-c:v", "libx265", "-preset", "ultrafast", "-profile:v", "main10", "-pix_fmt", "yuv420p10le",
		"-x265-params", "keyint=24:min-keyint=24:log-level=error", "-color_primaries", "bt709", "-color_trc", "bt709", "-colorspace", "bt709",
		"-c:a", "eac3", "-b:a", "384k", "-ac", "6", "-metadata:s:a:0", "language=eng", "-disposition:a:0", "default", file).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	var folders *localfiles.Service
	s := newProbingServer(t, 10, ffprobe, func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, o.Logger, o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
		folders.UseFiles(o.Playback.LocalFiles(folders.Open), o.Playback.LocalInputs(folders))
	})
	p := playingOn(t, s)
	addon, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Shelf", Path: dir, Kind: localfiles.KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if err := folders.Link(t.Context(), addon.ID, "Movie (2008)/Movie (2008).mkv", "tt1000"); err != nil {
		t.Fatal(err)
	}
	var movie BaseItemDto
	p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie)
	if movie.MediaSources == nil || len(*movie.MediaSources) != 3 {
		t.Fatalf("media sources: %+v", movie.MediaSources)
	}
	h := hevcSetup{playbackSetup: p, local: (*movie.MediaSources)[2]}
	// Analyzed ahead: a 12-second video would stand in for the movie's two
	// hours.
	analysis, err := media.Prober{Path: ffprobe, Timeout: time.Minute}.Probe(t.Context(), file)
	if err != nil {
		t.Fatal(err)
	}
	analysis.Remote = true
	encoded, _ := json.Marshal(analysis)
	if _, err := p.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", localVersion(t, p, h.local).ID, encoded); err != nil {
		t.Fatal(err)
	}
	return h
}

// withoutHEVC is a profile that neither plays nor takes HEVC.
func withoutHEVC(t *testing.T, profile json.RawMessage) json.RawMessage {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(profile, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, list := range []string{"DirectPlayProfiles", "TranscodingProfiles"} {
		for _, entry := range decoded[list].([]any) {
			entry := entry.(map[string]any)
			if codecs, ok := entry["VideoCodec"].(string); ok {
				entry["VideoCodec"] = strings.Join(slices.DeleteFunc(strings.Split(codecs, ","), func(c string) bool { return c == "hevc" }), ",")
			}
		}
	}
	data, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// play asks PlaybackInfo for version on the app of profile on device,
// reports it playing as the app would, and returns its media source and
// play session.
func (s hevcSetup) play(t *testing.T, device, token, version string, profile json.RawMessage) (MediaSourceInfo, string) {
	t.Helper()
	status, data := s.call(http.MethodPost, "/Items/"+s.movie+"/PlaybackInfo", app(device, token), map[string]any{
		"UserId": s.user.ID.String(), "MediaSourceId": version, "MaxStreamingBitrate": 120_000_000, "DeviceProfile": profile})
	var info playbackInfoResponse
	if err := json.Unmarshal(data, &info); status != http.StatusOK || err != nil || len(info.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %d %s", status, data)
	}
	source := info.MediaSources[0]
	method := "Transcode"
	if source.SupportsDirectPlay {
		method = "DirectPlay"
	}
	if status, body := s.call(http.MethodPost, "/Sessions/Playing", app(device, token), map[string]any{"ItemId": s.movie,
		"MediaSourceId": version, "PlaySessionId": info.PlaySessionId, "PlayMethod": method}); status != http.StatusNoContent {
		t.Fatalf("playing: %d %s", status, body)
	}
	return source, info.PlaySessionId
}

// segments reads the master and media playlists of a stream, then its
// initialization segment and first segment, as a player starts.
func (s hevcSetup) segments(t *testing.T, source MediaSourceInfo) {
	t.Helper()
	if source.SupportsDirectPlay || source.TranscodingSubProtocol != "hls" {
		t.Fatalf("not streamed over HLS: %s %v", source.Name, tracks(source))
	}
	uris := segmentURIs(t, s.playbackSetup, source.TranscodingUrl)
	getSegment(t, s.playbackSetup, uris[0])
	getSegment(t, s.playbackSetup, uris[1])
}

// session is the session of device, as jellyfin-web's Playback Info asks
// for it.
func (s hevcSetup) session(t *testing.T, device, token string) SessionInfo {
	t.Helper()
	status, data := s.call(http.MethodGet, "/Sessions?deviceId="+device+"-id", app(device, token), nil)
	var sessions []SessionInfo
	if err := json.Unmarshal(data, &sessions); status != http.StatusOK || err != nil || len(sessions) != 1 {
		t.Fatalf("sessions: %d %s", status, data)
	}
	return sessions[0]
}

// jellyfin-web 12.4 in Chrome takes HEVC but not E-AC-3: the HEVC video
// is copied and the audio converted to AAC. The device's session says so,
// as Jellyfin's does, for its Playback Info to tell "Direct streaming"
// with the reason, and keeps saying so while no encoding runs, until the
// device plays a file as it is.
func TestSessionsTellWhatAStreamCopiesAndConverts(t *testing.T) {
	t.Parallel()
	s := newHEVCSetup(t)
	chrome := s.profile(t, "jellyfin-web-12.4-chrome")
	source, session := s.play(t, "tv", s.token, s.local.Id, chrome)
	s.segments(t, source)
	got := s.session(t, "tv", s.token).TranscodingInfo
	want := TranscodingInfo{AudioCodec: "aac", VideoCodec: "hevc", Container: "mp4", IsVideoDirect: true, IsAudioDirect: false,
		Width: new(320), Height: new(180), AudioChannels: new(2), TranscodeReasons: []string{"AudioCodecNotSupported"}}
	if got == nil {
		t.Fatal("no TranscodingInfo")
	}
	if got.Bitrate == nil || *got.Bitrate <= 0 {
		t.Errorf("bitrate: %v", got.Bitrate)
	}
	got.Bitrate = nil
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("TranscodingInfo:\n got %+v\nwant %+v", *got, want)
	}

	// The encoding stops once the player is idle, and starts again at its
	// next request: the session keeps what was sent meanwhile.
	kept := *s.session(t, "tv", s.token).TranscodingInfo
	s.handler.Playback.StopRemux(session)
	if slices.ContainsFunc(s.handler.Playback.Encodings(), func(r hls.Running) bool { return r.Key.Session == session }) {
		t.Fatal("the encoding still runs")
	}
	if between := s.session(t, "tv", s.token).TranscodingInfo; between == nil || !reflect.DeepEqual(*between, kept) {
		t.Errorf("between encodings: %+v, want %+v", between, kept)
	}

	// The same device then plays a file as it is: nothing is sent but the
	// file.
	if status, body := s.call(http.MethodPost, "/Sessions/Playing/Stopped", app("tv", s.token), map[string]any{"ItemId": s.movie,
		"MediaSourceId": s.local.Id, "PlaySessionId": session}); status != http.StatusNoContent {
		t.Fatalf("stopped: %d %s", status, body)
	}
	if direct, _ := s.play(t, "tv", s.token, s.versions[1].ID.String(), chrome); !direct.SupportsDirectPlay {
		t.Fatalf("H.264/AAC in MP4 is not played as it is: %+v", direct)
	}
	if info := s.session(t, "tv", s.token); info.NowPlayingItem == nil || info.PlayState.PlayMethod != "DirectPlay" || info.TranscodingInfo != nil {
		t.Errorf("direct play: %s, TranscodingInfo %+v", info.PlayState.PlayMethod, info.TranscodingInfo)
	}
}

// An app without HEVC has the video converted to H.264 as well, on the
// processor here, and its session says both are converted, and why.
func TestSessionsTellAConvertedVideo(t *testing.T) {
	t.Parallel()
	s := newHEVCSetup(t)
	source, _ := s.play(t, "tv", s.token, s.local.Id, withoutHEVC(t, s.profile(t, "jellyfin-web-12.4-chrome")))
	s.segments(t, source)
	got := s.session(t, "tv", s.token).TranscodingInfo
	if got == nil {
		t.Fatal("no TranscodingInfo")
	}
	if got.IsVideoDirect || got.IsAudioDirect || got.VideoCodec != "h264" || got.AudioCodec != "aac" || got.Container != "mp4" ||
		got.HardwareAccelerationType != "none" || got.Width == nil || *got.Width != 320 || got.Bitrate == nil || *got.Bitrate <= 0 ||
		!slices.Equal(got.TranscodeReasons, []string{"VideoCodecNotSupported", "AudioCodecNotSupported"}) {
		t.Errorf("TranscodingInfo: %+v", *got)
	}
}

// Each play tells on the detailed log how it plays, and each encoding
// whether it copies or converts.
func TestPlaysLogHowTheyPlay(t *testing.T) {
	t.Parallel()
	s := newHEVCSetup(t)
	s.warnings.all.Store(true)
	chrome := s.profile(t, "jellyfin-web-12.4-chrome")
	laptop := s.signIn("member", "laptop")

	source, _ := s.play(t, "tv", s.token, s.local.Id, chrome)
	s.segments(t, source)
	if got := s.warnings.attrsOf("A version plays over HLS"); len(got) != 1 || got[0]["app"] != "Test App" || got[0]["video"] != "copied" ||
		got[0]["audio"] != "aac 2 ch" || got[0]["reasons"] != "AudioCodecNotSupported" || got[0]["item"] != s.movie {
		t.Errorf("the play copying the video: %v", got)
	}
	if got := s.warnings.attrsOf("Started a remux"); len(got) == 0 || got[0]["video"] != "copy" || got[0]["audio"] != "aac" {
		t.Errorf("the encoding copying the video: %v", got)
	}

	converted, _ := s.play(t, "laptop", laptop, s.local.Id, withoutHEVC(t, chrome))
	s.segments(t, converted)
	if got := s.warnings.attrsOf("A version plays over HLS"); len(got) != 2 || got[1]["video"] != "h264 with libx264" ||
		got[1]["audio"] != "aac 2 ch" || got[1]["reasons"] != "VideoCodecNotSupported,AudioCodecNotSupported" {
		t.Errorf("the play converting the video: %v", got)
	}
	if got := s.warnings.attrsOf("Started a remux"); !slices.ContainsFunc(got, func(a map[string]string) bool {
		return a["video"] == "libx264" && a["audio"] == "aac"
	}) {
		t.Errorf("the encoding converting the video: %v", got)
	}

	s.play(t, "tv", s.token, s.versions[1].ID.String(), chrome)
	if got := s.warnings.attrsOf("A version plays as it is"); len(got) != 1 || got[0]["item"] != s.movie {
		t.Errorf("the play as it is: %v", got)
	}
}
