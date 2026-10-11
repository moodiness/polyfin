package jellyfin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A file of a local folder linked to a title is one more of its versions,
// beside the addons' streams, named after its folder, and Polyfin serves
// it from the disk with byte ranges.
func TestLocalFilesAreVersions(t *testing.T) {
	var folders *localfiles.Service
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, slog.New(slog.NewTextHandler(io.Discard, nil)),
			o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
		folders.UseFiles(o.Playback.LocalFiles(folders.Open), o.Playback.LocalInputs(folders.Input))
	})
	p := playingOn(t, s)
	file, err := os.ReadFile(filepath.Join("..", "container", "testdata", "forced.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Movie (2008) 1080p.mkv"), file, 0o644); err != nil {
		t.Fatal(err)
	}
	addon, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Shelf", Path: dir, Kind: localfiles.KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	// The streaming addon offers no search: the file is linked by hand.
	if err := folders.Link(t.Context(), addon.ID, "Movie (2008) 1080p.mkv", "tt1000"); err != nil {
		t.Fatal(err)
	}

	var movie BaseItemDto
	p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie)
	if movie.MediaSources == nil || len(*movie.MediaSources) != 3 {
		t.Fatalf("media sources: %+v", movie.MediaSources)
	}
	local := (*movie.MediaSources)[2]
	if local.Name != "Shelf · 1080p · 52 KB" || local.Path == "" {
		t.Errorf("local version: %+v", local)
	}
	response, body := fetchURL(t, p.url+"/Items/"+local.Id+"/Download?ApiKey="+p.token, http.Header{"Range": {"bytes=100-199"}})
	if response.StatusCode != http.StatusPartialContent || body != string(file[100:200]) ||
		response.Header.Get("Content-Range") != "bytes 100-199/52341" {
		t.Errorf("a range of the file: %d %v", response.StatusCode, response.Header)
	}
	// The folder's library lists the title, and the file once gone is
	// not offered any more.
	var views QueryResult
	p.get(t, "/UserViews", p.token, &views)
	shelf := ""
	for _, view := range views.Items {
		if view.Name == "Shelf" {
			shelf = view.Id
		}
	}
	var page QueryResult
	if p.get(t, "/Items?ParentId="+shelf, p.token, &page); len(page.Items) != 1 || page.Items[0].Id != p.movie {
		t.Errorf("the folder's library: %+v", page.Items)
	}
	if err := os.Remove(filepath.Join(dir, "Movie (2008) 1080p.mkv")); err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie)
	for _, source := range *movie.MediaSources {
		if strings.HasPrefix(source.Name, "Shelf") {
			t.Errorf("a version of a file gone: %+v", source)
		}
	}
}

// A Blu-ray disc of a local folder is one version, its largest stream,
// named after the disc's folder rather than its stream: apps show and
// download it under that name.
func TestLocalDiscsAreNamedAfterTheirFolder(t *testing.T) {
	var folders *localfiles.Service
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, slog.New(slog.NewTextHandler(io.Discard, nil)),
			o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
		folders.UseFiles(o.Playback.LocalFiles(folders.Open), o.Playback.LocalInputs(folders.Input))
	})
	p := playingOn(t, s)
	file, err := os.ReadFile(filepath.Join("..", "container", "testdata", "forced.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	stream := filepath.Join(dir, "Movie (2008)", "BDMV", "STREAM")
	if err := os.MkdirAll(stream, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stream, "00001.m2ts"), file, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stream, "00000.m2ts"), file[:1000], 0o644); err != nil {
		t.Fatal(err)
	}
	addon, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Shelf", Path: dir, Kind: localfiles.KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if err := folders.Link(t.Context(), addon.ID, "Movie (2008)", "tt1000"); err != nil {
		t.Fatal(err)
	}

	userItem := "/Users/" + p.user.ID.String() + "/Items/"
	var movie BaseItemDto
	p.get(t, userItem+p.movie, p.token, &movie)
	if movie.MediaSources == nil || len(*movie.MediaSources) != 3 {
		t.Fatalf("media sources: %+v", movie.MediaSources)
	}
	local := (*movie.MediaSources)[2]
	if local.Name != "Shelf · 52 KB" || *local.Size != int64(len(file)) {
		t.Errorf("local version: %+v", local)
	}
	var version BaseItemDto
	if p.get(t, userItem+local.Id, p.token, &version); version.Path != "Movie (2008).m2ts" {
		t.Errorf("the disc's name: %q", version.Path)
	}
	response, body := fetchURL(t, p.url+"/Items/"+local.Id+"/Download?ApiKey="+p.token, http.Header{"Range": {"bytes=100-199"}})
	if response.StatusCode != http.StatusPartialContent || body != string(file[100:200]) {
		t.Errorf("a range of the largest stream: %d %v", response.StatusCode, response.Header)
	}
}

// localDVD is a test server, its ffprobe at ffprobe, with a local folder
// holding the test DVD of package localfiles in "Movie (2008)", linked to
// the movie, and the DVD's media source in the movie's details.
func localDVD(t *testing.T, ffprobe string) (playbackSetup, MediaSourceInfo) {
	t.Helper()
	var folders *localfiles.Service
	s := newProbingServer(t, 10, ffprobe, func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, slog.New(slog.NewTextHandler(io.Discard, nil)),
			o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
		folders.UseFiles(o.Playback.LocalFiles(folders.Open), o.Playback.LocalInputs(folders.Input))
	})
	p := playingOn(t, s)
	dir := t.TempDir()
	source := filepath.Join("..", "localfiles", "testdata", "dvd", "VIDEO_TS")
	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "Movie (2008)", "VIDEO_TS"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(source, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "Movie (2008)", "VIDEO_TS", e.Name()), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	addon, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Shelf", Path: dir, Kind: localfiles.KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if err := folders.Link(t.Context(), addon.ID, "Movie (2008)", "tt1000"); err != nil {
		t.Fatal(err)
	}
	var movie BaseItemDto
	p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie)
	if movie.MediaSources == nil || len(*movie.MediaSources) != 3 {
		t.Fatalf("media sources: %+v", movie.MediaSources)
	}
	return p, (*movie.MediaSources)[2]
}

// A DVD of a local folder is one version, named after its folder, which is
// not one file: apps are not offered to play it as it is, nor to download
// it, and its bytes are not served.
func TestLocalDVDsAreNotOneFile(t *testing.T) {
	p, local := localDVD(t, "ffprobe-not-installed")
	if local.Name != "Shelf · 520 KB" || local.Size == nil || *local.Size != 520192 || local.SupportsDirectPlay || local.SupportsDirectStream {
		t.Errorf("the DVD's version: %+v", local)
	}
	var version BaseItemDto
	if p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+local.Id, p.token, &version); version.Path != "Movie (2008).VOB" {
		t.Errorf("the DVD's name: %q", version.Path)
	}
	if response, _ := fetchURL(t, p.url+"/Items/"+local.Id+"/Download?ApiKey="+p.token, nil); response.StatusCode != http.StatusBadRequest {
		t.Errorf("a download of the DVD: %d", response.StatusCode)
	}
	if response, _ := fetchURL(t, p.url+"/Videos/"+local.Id+"/stream?static=true&ApiKey="+p.token, nil); response.StatusCode != http.StatusNotFound {
		t.Errorf("the DVD's bytes: %d", response.StatusCode)
	}
}

// dvdFFmpeg is the FFmpeg of POLYFIN_TEST_FFMPEG, with its ffprobe beside
// it, skipping the test when it is not set or reads no DVD (dvdvideo
// demuxer).
func dvdFFmpeg(t *testing.T) (ffmpeg, ffprobe string) {
	t.Helper()
	ffmpeg = os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	output, err := exec.Command(ffmpeg, "-hide_banner", "-demuxers").Output()
	if err != nil || !strings.Contains(string(output), " dvdvideo ") {
		t.Skip("the FFmpeg of POLYFIN_TEST_FFMPEG has no dvdvideo demuxer")
	}
	return ffmpeg, filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
}

// A DVD plays its main title, analyzed through FFmpeg's dvdvideo demuxer
// with its tracks, over HLS only, its video converted: the playlist's
// segments make the title's eight seconds.
func TestLocalDVDsPlayOverHLS(t *testing.T) {
	ffmpeg, ffprobe := dvdFFmpeg(t)
	p, local := localDVD(t, ffprobe)
	status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token), map[string]any{
		"UserId": p.user.ID.String(), "MediaSourceId": local.Id, "DeviceProfile": p.profile(t, "jellyfin-web-chrome")})
	var info playbackInfoResponse
	if err := json.Unmarshal(data, &info); status != http.StatusOK || err != nil || len(info.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %d %s", status, data)
	}
	source := info.MediaSources[0]
	if source.Id != local.Id || source.SupportsDirectPlay || source.SupportsDirectStream || !source.SupportsTranscoding ||
		source.TranscodingUrl == "" || source.RunTimeTicks == nil || *source.RunTimeTicks != 8*10_000_000 {
		t.Fatalf("the DVD's source: %+v", source)
	}
	var tracks []string
	for _, stream := range source.MediaStreams {
		tracks = append(tracks, stream.Type+" "+stream.Codec+" "+stream.Language)
	}
	// The movie's subtitle file, then the title's tracks.
	if !slices.Equal(tracks, []string{"Subtitle subrip fra", "Video mpeg2video ", "Audio ac3 eng", "Audio ac3 fra", "Subtitle DVDSUB eng"}) {
		t.Errorf("tracks: %q", tracks)
	}

	// The segments, joined, play the whole title.
	get := func(path string) []byte {
		t.Helper()
		status, body := p.call(http.MethodGet, path, "", nil)
		if status != http.StatusOK {
			t.Fatalf("%s: %d %s", path, status, body)
		}
		return body
	}
	base := source.TranscodingUrl[:strings.Index(source.TranscodingUrl, "master.m3u8")]
	resolve := func(uri string) string {
		if strings.HasPrefix(uri, "/") {
			return uri
		}
		return base + uri
	}
	var playlist string
	for line := range strings.Lines(string(get(source.TranscodingUrl))) {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			playlist = resolve(line)
			break
		}
	}
	// An app seeking to the last segment first has FFmpeg seek in the
	// title; the segments, then read in order and joined, play it whole.
	var uris []string
	for line := range strings.Lines(string(get(playlist))) {
		line = strings.TrimSpace(line)
		if uri, ok := strings.CutPrefix(line, `#EXT-X-MAP:URI="`); ok {
			uris = append(uris, resolve(strings.TrimSuffix(uri, `"`)))
		} else if line != "" && !strings.HasPrefix(line, "#") {
			uris = append(uris, resolve(line))
		}
	}
	if len(uris) < 3 || len(get(uris[len(uris)-1])) == 0 {
		t.Fatalf("the last segment of %q", uris)
	}
	var joined []byte
	for _, uri := range uris {
		joined = append(joined, get(uri)...)
	}
	file := filepath.Join(t.TempDir(), "played")
	if err := os.WriteFile(file, joined, 0o644); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(ffmpeg, "-hide_banner", "-v", "error", "-i", file, "-f", "null", "-").CombinedOutput()
	if err != nil {
		t.Fatalf("the segments do not decode: %v %s", err, output)
	}
	// The segments keep the title's times, shifted: their video frames
	// tell how long they play, at the title's rate.
	output, err = exec.Command(ffprobe, "-v", "error", "-select_streams", "v:0", "-count_packets", "-show_entries", "stream=nb_read_packets",
		"-of", "csv=p=0", file).Output()
	frames, _ := strconv.Atoi(strings.TrimSpace(string(output)))
	if seconds := float64(frames) * 1001 / 30000; err != nil || seconds < 7.5 || seconds > 8.5 {
		t.Errorf("the segments play %d frames: %v", frames, err)
	}
}
