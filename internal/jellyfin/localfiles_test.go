package jellyfin

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
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
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/playback"
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

// localDisc is a test server, its ffprobe at ffprobe, with a local folder
// holding the disc populate puts in the disc folder "Movie (2008)", linked
// to the movie, and the disc's media source in the movie's details.
func localDisc(t *testing.T, ffprobe string, populate func(dir string)) (playbackSetup, MediaSourceInfo) {
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
	populate(filepath.Join(dir, "Movie (2008)"))
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

// localDVD is localDisc with the test DVD of package localfiles.
func localDVD(t *testing.T, ffprobe string) (playbackSetup, MediaSourceInfo) {
	t.Helper()
	return localDisc(t, ffprobe, func(dir string) {
		source := filepath.Join("..", "localfiles", "testdata", "dvd", "VIDEO_TS")
		entries, err := os.ReadDir(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "VIDEO_TS"), 0o755); err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			data, err := os.ReadFile(filepath.Join(source, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "VIDEO_TS", e.Name()), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	})
}

// localBluRay is localDisc with the test Blu-ray disc of package
// localfiles, whose main playlist plays two clips, 190 seconds in all.
func localBluRay(t *testing.T, ffprobe string) (playbackSetup, MediaSourceInfo) {
	t.Helper()
	return localDisc(t, ffprobe, func(dir string) {
		archive, err := os.Open(filepath.Join("..", "localfiles", "testdata", "bluray.tar.gz"))
		if err != nil {
			t.Fatal(err)
		}
		defer archive.Close()
		unzipped, err := gzip.NewReader(archive)
		if err != nil {
			t.Fatal(err)
		}
		files := tar.NewReader(unzipped)
		for {
			header, err := files.Next()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if header.Typeflag != tar.TypeReg {
				continue
			}
			target := filepath.Join(dir, filepath.FromSlash(header.Name))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(files)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	})
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

// A Blu-ray disc in a local folder is its main playlist, sized as both its
// clips, which is not one file: neither played as it is nor downloaded.
func TestLocalBluRaysAreNotOneFile(t *testing.T) {
	p, local := localBluRay(t, "ffprobe-not-installed")
	if local.Size == nil || *local.Size != 2893824+2838528 || local.SupportsDirectPlay || local.SupportsDirectStream {
		t.Errorf("the disc's version: %+v", local)
	}
	var version BaseItemDto
	if p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+local.Id, p.token, &version); version.Path != "Movie (2008).m2ts" {
		t.Errorf("the disc's name: %q", version.Path)
	}
	if response, _ := fetchURL(t, p.url+"/Items/"+local.Id+"/Download?ApiKey="+p.token, nil); response.StatusCode != http.StatusBadRequest {
		t.Errorf("a download of the disc: %d", response.StatusCode)
	}
}

// discFFmpeg is the FFmpeg of POLYFIN_TEST_FFMPEG, with its ffprobe beside
// it, skipping the test when it is not set or lacks the demuxer or
// protocol name that reads a disc: list is -demuxers or -protocols.
func discFFmpeg(t *testing.T, list, name string) (ffmpeg, ffprobe string) {
	t.Helper()
	ffmpeg = os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	output, err := exec.Command(ffmpeg, "-hide_banner", list).Output()
	if err != nil || !slices.Contains(strings.Fields(string(output)), name) {
		t.Skipf("the FFmpeg of POLYFIN_TEST_FFMPEG has no %s", name)
	}
	return ffmpeg, filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
}

// playDisc asks PlaybackInfo for the disc's media source on jellyfin-web,
// checks it plays over HLS only, for want, and returns it.
func playDisc(t *testing.T, p playbackSetup, local MediaSourceInfo, want time.Duration) MediaSourceInfo {
	t.Helper()
	status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token), map[string]any{
		"UserId": p.user.ID.String(), "MediaSourceId": local.Id, "DeviceProfile": p.profile(t, "jellyfin-web-chrome")})
	var info playbackInfoResponse
	if err := json.Unmarshal(data, &info); status != http.StatusOK || err != nil || len(info.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %d %s", status, data)
	}
	source := info.MediaSources[0]
	if source.Id != local.Id || source.SupportsDirectPlay || source.SupportsDirectStream || !source.SupportsTranscoding ||
		source.TranscodingUrl == "" || source.RunTimeTicks == nil || (time.Duration(*source.RunTimeTicks)*100-want).Abs() > 100*time.Millisecond {
		t.Fatalf("the disc's source: %+v", source)
	}
	return source
}

// playedFrames reads the HLS playback of a transcoding URL, its last
// segment first, as an app seeking there does, then every segment in
// order, and returns how many video frames they hold together, which
// decode, and when they start: Polyfin shifts the title's times by 10
// seconds (see hls.timestampOffset).
func playedFrames(t *testing.T, p playbackSetup, ffmpeg, ffprobe, transcoding string) (frames int, start float64) {
	t.Helper()
	get := func(path string) []byte {
		t.Helper()
		status, body := p.call(http.MethodGet, path, "", nil)
		if status != http.StatusOK {
			t.Fatalf("%s: %d %s", path, status, body)
		}
		return body
	}
	base := transcoding[:strings.Index(transcoding, "master.m3u8")]
	resolve := func(uri string) string {
		if strings.HasPrefix(uri, "/") {
			return uri
		}
		return base + uri
	}
	var playlist string
	for line := range strings.Lines(string(get(transcoding))) {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			playlist = resolve(line)
			break
		}
	}
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
	// tell how long they play.
	output, err = exec.Command(ffprobe, "-v", "error", "-select_streams", "v:0", "-count_packets", "-show_entries", "stream=nb_read_packets",
		"-of", "csv=p=0", file).Output()
	if err != nil {
		t.Fatal(err)
	}
	frames, _ = strconv.Atoi(strings.TrimSpace(string(output)))
	output, err = exec.Command(ffprobe, "-v", "error", "-show_entries", "format=start_time", "-of", "csv=p=0", file).Output()
	if err != nil {
		t.Fatal(err)
	}
	start, _ = strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	return frames, start
}

// tracks lists a media source's tracks, by type, codec and language.
func tracks(source MediaSourceInfo) []string {
	var list []string
	for _, stream := range source.MediaStreams {
		list = append(list, stream.Type+" "+stream.Codec+" "+stream.Language)
	}
	return list
}

// A DVD plays its main title, analyzed through FFmpeg's dvdvideo demuxer
// with its tracks, over HLS only, its video converted: the playlist's
// segments make the title's eight seconds, even read from a seek.
func TestLocalDVDsPlayOverHLS(t *testing.T) {
	ffmpeg, ffprobe := discFFmpeg(t, "-demuxers", "dvdvideo")
	p, local := localDVD(t, ffprobe)
	source := playDisc(t, p, local, 8*time.Second)
	// The movie's subtitle file, then the title's tracks.
	if got := tracks(source); !slices.Equal(got, []string{"Subtitle subrip fra", "Video mpeg2video ", "Audio ac3 eng", "Audio ac3 fra", "Subtitle DVDSUB eng"}) {
		t.Errorf("tracks: %q", got)
	}
	frames, start := playedFrames(t, p, ffmpeg, ffprobe, source.TranscodingUrl)
	if seconds := float64(frames) * 1001 / 30000; seconds < 7.5 || seconds > 8.5 || start != 10 {
		t.Errorf("the segments play %.1f s from %.3f s", seconds, start)
	}
}

// A Blu-ray disc plays its main playlist through FFmpeg's bluray protocol,
// both its clips, over HLS only, its video converted, starting at zero
// though its clips start at 600 seconds.
func TestLocalBluRaysPlayTheirMainPlaylist(t *testing.T) {
	ffmpeg, ffprobe := discFFmpeg(t, "-protocols", "bluray")
	p, local := localBluRay(t, ffprobe)
	source := playDisc(t, p, local, 190*time.Second)
	if got := tracks(source); !slices.Equal(got, []string{"Subtitle subrip fra", "Video h264 ", "Audio ac3 "}) {
		t.Errorf("tracks: %q", got)
	}
	frames, start := playedFrames(t, p, ffmpeg, ffprobe, source.TranscodingUrl)
	if seconds := float64(frames) / 24; seconds < 189 || seconds > 191 || start != 10 {
		t.Errorf("the segments play %.1f s from %.3f s", seconds, start)
	}
}

// The subtitle files beside a local file are subtitle streams of its
// version alone, after its title's files, with the language, title and
// flags their names tell, and Polyfin serves them in the format each app
// asks for, French accents of Windows-1252 included.
func TestLocalSubtitleFilesAreTracks(t *testing.T) {
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
	for name, data := range map[string]string{
		"Movie (2008) 1080p.mkv":           string(file),
		"Movie (2008) 1080p.fr.srt":        "1\r\n00:00:01,000 --> 00:00:02,500\r\nD\xe9j\xe0 vu, \x93cool\x94\r\n",
		"Movie (2008) 1080p.en.forced.srt": "1\n00:00:03,000 --> 00:00:04,000\nSigns\n",
		"Movie (2008) 1080p.en.sdh.ass": "[Script Info]\nScriptType: v4.00+\n\n[V4+ Styles]\n" +
			"Format: Name, Fontname, Fontsize\nStyle: Default,Arial,20\n\n[Events]\n" +
			"Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n" +
			"Dialogue: 0,0:00:05.00,0:00:06.00,Default,,0,0,0,,{\\i1}[door creaks]{\\i0}\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
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
	if err := folders.Link(t.Context(), addon.ID, "Movie (2008) 1080p.mkv", "tt1000"); err != nil {
		t.Fatal(err)
	}

	var movie BaseItemDto
	p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie)
	if movie.MediaSources == nil || len(*movie.MediaSources) != 3 {
		t.Fatalf("media sources: %+v", movie.MediaSources)
	}
	type track struct {
		language, codec, title    string
		forced, hearing, external bool
	}
	externals := func(streams []playback.MediaStream) []track {
		var tracks []track
		for _, stream := range streams {
			if stream.IsExternal {
				tracks = append(tracks, track{stream.Language, stream.Codec, stream.DisplayTitle, stream.IsForced, stream.IsHearingImpaired, true})
			}
		}
		return tracks
	}
	// The addon's French file, the title's, comes first; then the local
	// file's own, by name.
	addonFile := track{"fra", "subrip", "French - SUBRIP - External", false, false, true}
	want := []track{addonFile,
		{"eng", "subrip", "English - Forced - SUBRIP - External", true, false, true},
		{"eng", "ass", "English - Hearing Impaired - ASS - External", false, true, true},
		{"fra", "subrip", "French - SUBRIP - External", false, false, true},
	}
	local := (*movie.MediaSources)[2]
	if got := externals(local.MediaStreams); !slices.Equal(got, want) {
		t.Errorf("the local version's subtitle files: %+v", got)
	}
	if got := externals((*movie.MediaSources)[0].MediaStreams); !slices.Equal(got, []track{addonFile}) {
		t.Errorf("another version's: %+v", got)
	}

	// PlaybackInfo plays the local version as it is, once analyzed.
	movieID, _ := accounts.ParseID(p.movie)
	versions, err := p.library.Versions(t.Context(), p.user, movieID)
	if i := slices.IndexFunc(versions, func(v library.Version) bool { return v.ID.String() == local.Id }); err != nil || i < 0 {
		t.Fatalf("versions: %+v %v", versions, err)
	} else {
		p.analyzed(t, versions[i], "h264-aac-mp4")
	}
	status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token), map[string]any{
		"UserId": p.user.ID.String(), "MediaSourceId": local.Id, "DeviceProfile": p.profile(t, "jellyfin-web-chrome")})
	var info playbackInfoResponse
	if err := json.Unmarshal(data, &info); status != http.StatusOK || err != nil || len(info.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %d %s", status, data)
	}
	source := info.MediaSources[0]
	if got := externals(source.MediaStreams); !slices.Equal(got, want) {
		t.Errorf("PlaybackInfo's subtitle files: %+v", got)
	}
	french := source.MediaStreams[3]
	if french.DeliveryMethod != "External" || !strings.Contains(french.DeliveryUrl, "/Subtitles/3/0/Stream.vtt") {
		t.Errorf("delivery: %s %s", french.DeliveryMethod, french.DeliveryUrl)
	}
	subtitle := func(index, format string) string {
		t.Helper()
		response, body := fetchURL(t, p.url+"/Videos/"+p.movie+"/"+local.Id+"/Subtitles/"+index+"/0/Stream."+format+"?ApiKey="+p.token, nil)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("subtitle %s as %s: %d %s", index, format, response.StatusCode, body)
		}
		return body
	}
	if got := subtitle("3", "vtt"); got != "WEBVTT\n\n00:00:01.000 --> 00:00:02.500\nDéjà vu, “cool”\n" {
		t.Errorf("as WebVTT: %q", got)
	}
	if got := subtitle("3", "srt"); got != "1\n00:00:01,000 --> 00:00:02,500\nDéjà vu, “cool”\n" {
		t.Errorf("as SubRip: %q", got)
	}
	if got := subtitle("2", "ass"); !strings.Contains(got, `{\i1}[door creaks]{\i0}`) || !strings.Contains(got, "Style: Default,Arial,20") {
		t.Errorf("an ASS file as ASS keeps its styles: %q", got)
	}
	if got := subtitle("1", "vtt"); !strings.Contains(got, "Signs") {
		t.Errorf("the forced file: %q", got)
	}
}
