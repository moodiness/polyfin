package jellyfin

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/remuxdb"
	"github.com/moodiness/polyfin/internal/stremio"
)

// The files of the labelled movie: one stream each, its labels its file
// name, and the ending item details give it for jellyfin-web in Chrome.
var labelledFiles = []struct {
	filename, english, french string
}{
	// Estimated from the labels: H.264 and AAC in MP4 plays as it is.
	{"Movie.1080p.WEB-DL.AAC2.0.H.264.mp4", "(likely direct play)", "(lecture directe probable)"},
	// Dolby Vision over HDR10, which Chrome cannot show, whatever its
	// profile, and which the test server, without FFmpeg, cannot tone map.
	{"Movie.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265.mkv", "(likely cannot play here)", "(probablement illisible ici)"},
	// HEVC Chrome takes, 8 or 10 bits, with DTS it does not.
	{"Movie.1080p.BluRay.DTS-HD.MA.5.1.x265.mkv", "(likely converted: audio)", "(conversion probable\u202f: audio)"},
	// No codec named: no ending.
	{"Movie.1080p.WEB-DL.mkv", "", ""},
	// Two audio tracks, AAC played as it is, AC-3 converted: which one
	// plays decides, so no ending rather than a guess.
	{"Movie.1080p.MULTi.AAC2.0.AC3.5.1.x264.mkv", "", ""},
	// Described: its description, HEVC and AAC, wins over its labels'
	// E-AC-3.
	{"Movie.Described.2160p.HEVC.DDP5.1.mkv", "(direct play)", "(lecture directe)"},
	// Analyzed as H.264 and AAC in MP4: the analysis wins over the labels'
	// DTS and over its description's TrueHD.
	{"Movie.Analysed.1080p.HEVC.DTS.mkv", "(direct play)", "(lecture directe)"},
}

// labelledDescriptions is what a fake RemuxDB knows of the labelled
// movie's files: the described one and the analyzed one.
const labelledDescriptions = `[
  {"container": "matroska,webm", "duration": 60, "size": 60000000,
   "sources": [{"kind": "torrent", "filename": "Movie.Described.2160p.HEVC.DDP5.1.mkv"}],
   "tracks": [
     {"kind": "video", "idx": 0, "codec": "hevc", "profile": "Main 10", "width": 3840, "height": 2160, "is_default": true},
     {"kind": "audio", "idx": 1, "codec": "aac", "channels": 2, "is_default": true}]},
  {"container": "matroska,webm", "duration": 60, "size": 60000000,
   "sources": [{"kind": "torrent", "filename": "Movie.Analysed.1080p.HEVC.DTS.mkv"}],
   "tracks": [
     {"kind": "video", "idx": 0, "codec": "hevc", "profile": "Main 10", "width": 1920, "height": 1080, "is_default": true},
     {"kind": "audio", "idx": 1, "codec": "truehd", "channels": 8, "is_default": true}]}
]`

// While a version's tracks are unknown, item details end its name with
// how the app likely plays it, estimated from its labels, worded as an
// estimate, in the server language: only when the labels name both
// codecs and what they leave open changes nothing. A description, then
// an analysis, give the exact ending instead. The estimate names the
// version only: its track menus list no track the labels suggest.
func TestVersionNamesEstimateHowVersionsPlayFromTheirLabels(t *testing.T) {
	t.Parallel()
	descriptions := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/media/tt7160/versions" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, labelledDescriptions)
	}))
	t.Cleanup(descriptions.Close)
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, _ *pgxpool.Pool) {
		o.RemuxDB = remuxdb.New(testServerID, "test", slog.New(slog.DiscardHandler), o.Accounts.Settings)
	})
	s.setting(t, func(settings *accounts.Settings) { settings.RemuxDB, settings.RemuxDBURL = true, descriptions.URL })
	var streams []stremio.Stream
	for i, file := range labelledFiles {
		// A minute at 8 Mb/s, the movie's runtime.
		streams = append(streams, stremio.Stream{Name: "Addon", Description: file.filename, URL: fmt.Sprintf("http://127.0.0.1:9/%d", i),
			BehaviorHints: stremio.StreamBehavior{Filename: file.filename, VideoSize: 60_000_000}})
	}
	user := s.user("member", nil)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), failoverAddon(t, "catalog", streams), false); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	if len(page.Items) != 1 {
		t.Fatalf("titles: %+v", page.Items)
	}
	movie := page.Items[0].Id
	id, _ := accounts.ParseID(movie)
	versions, err := s.library.Versions(t.Context(), user, id)
	if err != nil || len(versions) != len(labelledFiles) {
		t.Fatalf("versions: %d, %v", len(versions), err)
	}
	byTag := map[string]string{}
	for _, version := range versions {
		byTag[version.ID.String()] = version.Filename
		if strings.Contains(version.Filename, "Analysed") {
			probe, err := os.ReadFile(filepath.Join(playbackFixtures, "probes", "h264-aac-mp4.json"))
			if err != nil {
				t.Fatal(err)
			}
			analysis, err := media.Parse(probe)
			if err != nil {
				t.Fatal(err)
			}
			analysis.Remote = true
			data, _ := json.Marshal(analysis)
			if _, err := s.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", version.ID, data); err != nil {
				t.Fatal(err)
			}
		}
	}
	details := func(what string) map[string]MediaSourceInfo {
		t.Helper()
		var dto BaseItemDto
		if status := s.get(t, "/Users/"+user.ID.String()+"/Items/"+movie, token, &dto); status != http.StatusOK || dto.MediaSources == nil {
			t.Fatalf("%s: %d", what, status)
		}
		sources := map[string]MediaSourceInfo{}
		for _, source := range *dto.MediaSources {
			sources[byTag[source.ETag]] = source
		}
		return sources
	}
	ending := func(source MediaSourceInfo) string {
		if i := strings.LastIndex(source.Name, " ("); i >= 0 && strings.HasSuffix(source.Name, ")") {
			return source.Name[i+1:]
		}
		return ""
	}
	// Before the device tells how it plays, nothing ends any name.
	for filename, source := range details("before a profile") {
		if got := ending(source); got != "" {
			t.Errorf("%s before a profile: %q", filename, got)
		}
	}
	if status, body := s.call(http.MethodPost, "/Sessions/Capabilities/Full", app("tv", token),
		map[string]any{"PlayableMediaTypes": []string{"Video"}, "DeviceProfile": deviceProfile(t, "jellyfin-web-12.4-chrome")}); status != http.StatusNoContent {
		t.Fatalf("capabilities: %d %s", status, body)
	}
	// The first details ask RemuxDB, waiting a second at most; both
	// details see its descriptions.
	details("describing")
	sources := details("in English")
	for _, file := range labelledFiles {
		if got := ending(sources[file.filename]); got != file.english {
			t.Errorf("%s: %q, want %q", file.filename, got, file.english)
		}
	}
	// The estimate is in the name alone: the version estimated to play as
	// it is lists no video or audio track, and no default audio track.
	estimated := sources[labelledFiles[0].filename]
	for _, stream := range estimated.MediaStreams {
		if stream.Type == "Video" || stream.Type == "Audio" {
			t.Errorf("estimated version lists a %s track: %+v", stream.Type, stream)
		}
	}
	if estimated.DefaultAudioStreamIndex != nil || estimated.Container != "mp4" {
		t.Errorf("estimated version: audio %v, container %s", estimated.DefaultAudioStreamIndex, estimated.Container)
	}
	s.setting(t, func(settings *accounts.Settings) { settings.Language = "fr" })
	sources = details("in French")
	for _, file := range labelledFiles {
		if got := ending(sources[file.filename]); got != file.french {
			t.Errorf("%s in French: %q, want %q", file.filename, got, file.french)
		}
	}
}
