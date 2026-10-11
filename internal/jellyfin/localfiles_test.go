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
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/testsubtitles"
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
// holding the disc populate puts in it, at "Movie (2008)", which is linked
// to the movie, and the disc's media source in the movie's details.
func localDisc(t *testing.T, ffprobe string, populate func(at string)) (playbackSetup, MediaSourceInfo) {
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
	unit := "Movie (2008)"
	if entries, err := os.ReadDir(dir); err == nil && len(entries) == 1 && !entries[0].IsDir() {
		// A disc image is its own unit.
		unit = entries[0].Name()
	}
	if err := folders.Link(t.Context(), addon.ID, unit, "tt1000"); err != nil {
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

// playedFrames reads the HLS playback of a transcoding URL (see played),
// and returns how many video frames its segments hold together, which
// decode, and when they start: Polyfin shifts the title's times by 10
// seconds (see hls.timestampOffset).
func playedFrames(t *testing.T, p playbackSetup, ffmpeg, ffprobe, transcoding string) (frames int, start float64) {
	t.Helper()
	file := played(t, p, transcoding)
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

// played reads the HLS playback of a transcoding URL, its last segment
// first, as an app seeking there does, then every segment in order, and
// returns the file of the segments joined.
func played(t *testing.T, p playbackSetup, transcoding string) string {
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
	return file
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

// pictureProfile is jellyfin-web's Chrome profile as jellyfin-web 12.2
// sends it where it draws picture subtitles itself: PGS as files, VobSub
// as Matroska files.
func pictureProfile(t *testing.T, p playbackSetup) json.RawMessage {
	t.Helper()
	var profile map[string]any
	if err := json.Unmarshal(p.profile(t, "jellyfin-web-chrome"), &profile); err != nil {
		t.Fatal(err)
	}
	profile["SubtitleProfiles"] = append(profile["SubtitleProfiles"].([]any), map[string]any{"Format": "pgssub", "Method": "External"},
		map[string]any{"Format": "vobsub", "Container": "mks", "Method": "External"})
	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// localPictureFiles is a test server, its ffprobe at ffprobe, with a local
// folder holding "Movie (2008)", whose video, from video, is linked to the
// movie, beside a PGS file, "Movie (2008).en.forced.sup", and a VobSub
// pair, "Movie (2008).sdh.idx" and ".sub", whose .idx file tells English,
// the red rectangle of each shown from 1 to 3 seconds and from 5 to 7. It
// returns the PGS file and the local version's media source.
func localPictureFiles(t *testing.T, ffprobe string, video []byte) (playbackSetup, []byte, MediaSourceInfo) {
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
	rectangle := testsubtitles.Rectangle{CanvasWidth: 320, CanvasHeight: 180, X: 60, Y: 120, Width: 200, Height: 40,
		Shown: [][2]time.Duration{{time.Second, 3 * time.Second}, {5 * time.Second, 7 * time.Second}}}
	pgs := rectangle.PGS()
	idx, sub := rectangle.VobSub()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "Movie (2008)"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"Movie (2008).mkv": video, "Movie (2008).en.forced.sup": pgs, "Movie (2008).sdh.idx": []byte(idx),
		"Movie (2008).sdh.sub": sub} {
		if err := os.WriteFile(filepath.Join(dir, "Movie (2008)", name), data, 0o644); err != nil {
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
	if err := folders.Link(t.Context(), addon.ID, "Movie (2008)/Movie (2008).mkv", "tt1000"); err != nil {
		t.Fatal(err)
	}
	var movie BaseItemDto
	p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie)
	if movie.MediaSources == nil || len(*movie.MediaSources) != 3 {
		t.Fatalf("media sources: %+v", movie.MediaSources)
	}
	return p, pgs, (*movie.MediaSources)[2]
}

// pictureInfo asks PlaybackInfo for the local version on profile, with
// the subtitle stream subtitle chosen, -1 for the user's preference.
func pictureInfo(t *testing.T, p playbackSetup, local MediaSourceInfo, profile json.RawMessage, subtitle int) MediaSourceInfo {
	t.Helper()
	body := map[string]any{"UserId": p.user.ID.String(), "MediaSourceId": local.Id, "DeviceProfile": profile}
	if subtitle >= 0 {
		body["SubtitleStreamIndex"] = subtitle
	}
	status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token), body)
	var info playbackInfoResponse
	if err := json.Unmarshal(data, &info); status != http.StatusOK || err != nil || len(info.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %d %s", status, data)
	}
	return info.MediaSources[0]
}

// localVersion is the version of the movie a local media source stands
// for.
func localVersion(t *testing.T, p playbackSetup, local MediaSourceInfo) library.Version {
	t.Helper()
	movieID, _ := accounts.ParseID(p.movie)
	versions, err := p.library.Versions(t.Context(), p.user, movieID)
	i := slices.IndexFunc(versions, func(v library.Version) bool { return v.ID.String() == local.Id })
	if err != nil || i < 0 {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	return versions[i]
}

// The picture subtitle files beside a local file, a PGS file and a VobSub
// pair, are subtitle streams of its version, PGSSUB and DVDSUB as Jellyfin
// names them, with the language and flags of their names, or of the .idx
// file. An app that draws PGS takes the PGS file as it is, by ranges; a
// VobSub pair is not one file, nor are pictures text.
func TestLocalPictureSubtitleFilesAreTracks(t *testing.T) {
	file, err := os.ReadFile(filepath.Join("..", "container", "testdata", "forced.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	p, pgs, local := localPictureFiles(t, "ffprobe-not-installed", file)
	p.analyzed(t, localVersion(t, p, local), "h264-aac-mp4")
	type track struct {
		codec, title     string
		forced, hearing  bool
		text             bool
		delivery, format string
	}
	files := func(source MediaSourceInfo) []track {
		var tracks []track
		for _, stream := range source.MediaStreams {
			if stream.IsExternal {
				_, format, _ := strings.Cut(stream.DeliveryUrl, "/0/Stream.")
				format, _, _ = strings.Cut(format, "?")
				tracks = append(tracks, track{stream.Codec, stream.DisplayTitle, stream.IsForced, stream.IsHearingImpaired, stream.IsTextSubtitleStream,
					stream.DeliveryMethod, format})
			}
		}
		return tracks
	}
	addonFile := track{"subrip", "French - SUBRIP - External", false, false, true, "External", "vtt"}
	pgsFile := track{"PGSSUB", "English - Forced - PGSSUB - External", true, false, false, "External", "pgssub"}
	vobSub := track{"DVDSUB", "English - Hearing Impaired - DVDSUB - External", false, true, false, "Drop", ""}
	if got := files(pictureInfo(t, p, local, pictureProfile(t, p), -1)); !slices.Equal(got, []track{addonFile, pgsFile, vobSub}) {
		t.Errorf("the files, for an app that draws PGS: %+v", got)
	}
	subtitle := func(index, format string, header http.Header) (*http.Response, string) {
		return fetchURL(t, p.url+"/Videos/"+p.movie+"/"+local.Id+"/Subtitles/"+index+"/0/Stream."+format+"?ApiKey="+p.token, header)
	}
	if response, body := subtitle("1", "pgssub", nil); response.StatusCode != http.StatusOK || body != string(pgs) {
		t.Errorf("the PGS file: %d, %d bytes", response.StatusCode, len(body))
	}
	if response, body := subtitle("1", "pgssub", http.Header{"Range": {"bytes=2-5"}}); response.StatusCode != http.StatusPartialContent ||
		body != string(pgs[2:6]) || response.Header.Get("Content-Range") != "bytes 2-5/"+strconv.Itoa(len(pgs)) {
		t.Errorf("a range of the PGS file: %d %v", response.StatusCode, response.Header)
	}
	for _, c := range []struct{ index, format string }{{"1", "vtt"}, {"2", "pgssub"}, {"2", "sub"}} {
		if response, _ := subtitle(c.index, c.format, nil); response.StatusCode != http.StatusInternalServerError {
			t.Errorf("subtitle %s as %s: %d", c.index, c.format, response.StatusCode)
		}
	}
}

// A picture subtitle file chosen on an app that cannot draw it is burned
// into the video FFmpeg converts, reading the file through the loopback
// interface, a VobSub pair's .sub file beside its .idx file: the pair
// always, as it is not one file, the PGS file on an app that does not draw
// PGS.
func TestLocalPictureSubtitleFilesAreBurnedIn(t *testing.T) {
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	video := filepath.Join(t.TempDir(), "video.mkv")
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "8", "-c:v", "libx264", "-preset", "veryfast", "-g", "48",
		"-c:a", "aac", video).CombinedOutput(); err != nil {
		t.Fatalf("make the video: %v: %s", err, out)
	}
	data, err := os.ReadFile(video)
	if err != nil {
		t.Fatal(err)
	}
	p, _, local := localPictureFiles(t, ffprobe, data)
	// Analyzed ahead: an 8-second video would stand in for the movie's two
	// hours.
	analysis, err := media.Prober{Path: ffprobe, Timeout: time.Minute}.Probe(t.Context(), video)
	if err != nil {
		t.Fatal(err)
	}
	analysis.Remote = true
	encoded, _ := json.Marshal(analysis)
	if _, err := p.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", localVersion(t, p, local).ID, encoded); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		profile  json.RawMessage
		subtitle int
	}{{pictureProfile(t, p), 2}, {p.profile(t, "jellyfin-web-chrome"), 1}} {
		source := pictureInfo(t, p, local, c.profile, c.subtitle)
		if !strings.Contains(source.TranscodingUrl, "SubtitleStreamIndex="+strconv.Itoa(c.subtitle)+"&") ||
			!strings.Contains(source.TranscodingUrl, "SubtitleMethod=Encode") || !strings.Contains(source.TranscodingUrl, "allowVideoStreamCopy=false") ||
			source.MediaStreams[c.subtitle].DeliveryMethod != "Encode" {
			t.Fatalf("subtitle %d chosen: %s %s", c.subtitle, source.MediaStreams[c.subtitle].DeliveryMethod, source.TranscodingUrl)
		}
		file := played(t, p, source.TranscodingUrl)
		// The color of a pixel inside the rectangle, at a time of the
		// title, shifted as the segments are (see hls.timestampOffset).
		for _, at := range []struct {
			seconds float64
			red     bool
		}{{2, true}, {4, false}, {6, true}} {
			out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-copyts", "-i", file,
				"-vf", "select=gte(t\\,"+strconv.FormatFloat(at.seconds+10, 'f', 3, 64)+"),scale=160:90,format=rgb24,crop=1:1:80:70",
				"-frames:v", "1", "-f", "rawvideo", "-pix_fmt", "rgb24", "-").Output()
			if err != nil || len(out) != 3 {
				t.Fatalf("subtitle %d, pixel at %.0f s: %v", c.subtitle, at.seconds, err)
			}
			if red := out[0] > 160 && out[1] < 90 && out[2] < 90; red != at.red {
				t.Errorf("subtitle %d at %.0f s: color %v, subtitle shown %v", c.subtitle, at.seconds, out, at.red)
			}
		}
	}
}

// localImage is localDisc with the test image name of package localfiles,
// dvd or bluray, as the file "Movie (2008).iso".
func localImage(t *testing.T, ffprobe, name string) (playbackSetup, MediaSourceInfo) {
	t.Helper()
	return localDisc(t, ffprobe, func(at string) {
		archive, err := os.Open(filepath.Join("..", "localfiles", "testdata", name+".iso.gz"))
		if err != nil {
			t.Fatal(err)
		}
		defer archive.Close()
		unzipped, err := gzip.NewReader(archive)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(unzipped)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(at+".iso", data, 0o644); err != nil {
			t.Fatal(err)
		}
	})
}

// A disc image plays its main title from the image itself, over HLS only:
// a DVD image through the dvdvideo demuxer, a Blu-ray image through the
// bluray protocol, each whole.
func TestLocalDiscImagesPlay(t *testing.T) {
	t.Run("dvd", func(t *testing.T) {
		ffmpeg, ffprobe := discFFmpeg(t, "-demuxers", "dvdvideo")
		p, local := localImage(t, ffprobe, "dvd")
		var version BaseItemDto
		if p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+local.Id, p.token, &version); version.Path != "Movie (2008).iso" || local.SupportsDirectPlay {
			t.Errorf("the image's version: %q %+v", version.Path, local)
		}
		source := playDisc(t, p, local, 8*time.Second)
		if got := tracks(source); !slices.Equal(got, []string{"Subtitle subrip fra", "Video mpeg2video ", "Audio ac3 eng", "Audio ac3 fra", "Subtitle DVDSUB eng"}) {
			t.Errorf("tracks: %q", got)
		}
		frames, start := playedFrames(t, p, ffmpeg, ffprobe, source.TranscodingUrl)
		if seconds := float64(frames) * 1001 / 30000; seconds < 7.5 || seconds > 8.5 || start != 10 {
			t.Errorf("the segments play %.1f s from %.3f s", seconds, start)
		}
	})
	t.Run("bluray", func(t *testing.T) {
		ffmpeg, ffprobe := discFFmpeg(t, "-protocols", "bluray")
		p, local := localImage(t, ffprobe, "bluray")
		source := playDisc(t, p, local, 190*time.Second)
		if got := tracks(source); !slices.Equal(got, []string{"Subtitle subrip fra", "Video h264 ", "Audio ac3 "}) {
			t.Errorf("tracks: %q", got)
		}
		frames, start := playedFrames(t, p, ffmpeg, ffprobe, source.TranscodingUrl)
		if seconds := float64(frames) / 24; seconds < 189 || seconds > 191 || start != 10 {
			t.Errorf("the segments play %.1f s from %.3f s", seconds, start)
		}
	})
}
