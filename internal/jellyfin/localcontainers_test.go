package jellyfin

import (
	"encoding/json"
	"io"
	"log/slog"
	"math"
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

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/stremio"
)

// containerSeconds is how long the files of the container tests last, at
// 25 frames a second.
const containerSeconds = 12

// A local MPEG program stream (.mpg, .vob) or AVI file has no keyframe
// index Polyfin reads, yet plays over HLS on jellyfin-web and on the
// Android TV app, which take neither container as it is: its video
// converted and cut every few seconds, as an MPEG-TS file's is, even
// H.264 the app takes. The last segment and one in the middle, read
// first, as seeks read them, hold the frames they hold read in order; all
// the segments hold each of the file's frames once, from its start. So
// does an MPEG-TS file of MPEG-2, whose first keyframe a seek near the
// start lands on, as it does in an MPEG program stream, on jellyfin-web:
// the Android TV app plays MPEG-TS as it is.
func TestLocalUnindexedContainersPlayOverHLS(t *testing.T) {
	t.Parallel()
	ffmpeg, ffprobe := discFFmpeg(t, "-demuxers", "avi")
	both := []string{"jellyfin-web-12.4-chrome", "androidtv"}
	files := []struct {
		name, video string
		args        []string
		apps        []string
	}{
		{"Movie (2008).mpg", "mpeg2video", []string{"-c:v", "mpeg2video", "-b:v", "1M", "-g", "12", "-bf", "2", "-c:a", "mp2"}, both},
		{"Movie (2008).vob", "mpeg2video", []string{"-c:v", "mpeg2video", "-b:v", "1M", "-g", "15", "-bf", "2", "-c:a", "ac3", "-f", "vob"}, both},
		{"Movie (2008).avi", "mpeg4", []string{"-c:v", "mpeg4", "-vtag", "XVID", "-q:v", "4", "-g", "250", "-bf", "2", "-c:a", "libmp3lame"}, both},
		{"Movie (2008) h264.avi", "h264", []string{"-c:v", "libx264", "-preset", "veryfast", "-g", "100", "-pix_fmt", "yuv420p", "-c:a", "libmp3lame"}, both},
		{"Movie (2008).ts", "mpeg2video", []string{"-c:v", "mpeg2video", "-b:v", "1M", "-g", "12", "-bf", "2", "-c:a", "mp2"}, both[:1]},
	}
	for _, file := range files {
		t.Run(file.name, func(t *testing.T) {
			t.Parallel()
			p, item, local := localContainerFile(t, ffprobe, containerFile(t, ffmpeg, file.name, file.args...))
			for _, app := range file.apps {
				source := playContainer(t, p, item, local, p.profile(t, app))
				if !strings.Contains(source.TranscodingUrl, "allowVideoStreamCopy=false") {
					t.Errorf("%s: the video is not converted: %s", app, source.TranscodingUrl)
				}
				if got := tracks(source); !slices.Contains(got, "Video "+file.video+" ") {
					t.Errorf("%s: tracks %q", app, got)
				}
				uris := segmentURIs(t, p, source.TranscodingUrl)
				sought := map[int][]byte{}
				for _, n := range []int{len(uris) - 1, len(uris) / 2} {
					sought[n] = getSegment(t, p, uris[n])
				}
				// Fragmented MP4 segments decode after their initialization
				// segment, listed first.
				var init []byte
				if strings.Contains(uris[0], "/-1.") {
					init = getSegment(t, p, uris[0])
				}
				segments := make([][]byte, len(uris))
				for i, uri := range uris {
					segments[i] = getSegment(t, p, uri)
				}
				for n, segment := range sought {
					times, _ := frameTimes(t, ffprobe, decoded(t, ffmpeg, init, segment))
					if inOrder, _ := frameTimes(t, ffprobe, decoded(t, ffmpeg, init, segments[n])); len(times) == 0 || !slices.Equal(times, inOrder) {
						t.Errorf("%s: segment %d from a seek: %d frames from %v, in order %d from %v", app, n, len(times), times[:min(1, len(times))],
							len(inOrder), inOrder[:min(1, len(inOrder))])
					}
				}
				times, _ := frameTimes(t, ffprobe, decoded(t, ffmpeg, segments...))
				if len(times) != containerSeconds*25 || times[0] < 10 || times[0] > 10.1 {
					t.Fatalf("%s: the segments hold %d frames from %v", app, len(times), times[:min(1, len(times))])
				}
				for i, at := range times {
					if want := times[0] + float64(i)/25; math.Abs(at-want) > 0.002 {
						t.Fatalf("%s: frame %d at %.4f s, not %.4f s", app, i, at, want)
					}
				}
			}
		})
	}
}

// The H.264 of an MPEG program stream or of an AVI file is converted over
// HLS, though jellyfin-web takes it: the version's name in item details
// says so, as PlaybackInfo plays it.
func TestLocalUnindexedVersionsAreNamedAsTheyPlay(t *testing.T) {
	t.Parallel()
	ffmpeg, ffprobe := discFFmpeg(t, "-demuxers", "avi")
	h264 := []string{"-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p"}
	for name, args := range map[string][]string{
		"Movie (2008).mpg": slices.Concat(h264, []string{"-c:a", "mp2", "-f", "vob"}),
		"Movie (2008).avi": slices.Concat(h264, []string{"-c:a", "libmp3lame"}),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, item, local := localContainerFile(t, ffprobe, containerFile(t, ffmpeg, name, args...))
			source := playContainer(t, p, item, local, p.profile(t, "jellyfin-web-12.4-chrome"))
			var dto BaseItemDto
			p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+item, p.token, &dto)
			if dto.MediaSources == nil || len(*dto.MediaSources) != 1 || (*dto.MediaSources)[0].Name != local.Name+" (converted)" ||
				source.Name != local.Name+" (converted)" {
				t.Errorf("named %+v in item details, %q by PlaybackInfo", dto.MediaSources, source.Name)
			}
		})
	}
}

// A local MPEG program stream is served as video/mpeg to an app playing
// it as it is.
func TestLocalProgramStreamsAreServedAsMPEG(t *testing.T) {
	t.Parallel()
	ffmpeg, ffprobe := discFFmpeg(t, "-demuxers", "mpeg")
	p, _, local := localContainerFile(t, ffprobe, containerFile(t, ffmpeg, "Movie (2008).mpg", "-c:v", "mpeg2video", "-c:a", "mp2"))
	response, _ := fetchURL(t, p.url+"/Videos/"+local.Id+"/stream?static=true&ApiKey="+p.token, nil)
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "video/mpeg" {
		t.Errorf("the file: %d %v", response.StatusCode, response.Header)
	}
}

// containerFile makes containerSeconds of 25 frames a second and a tone,
// encoded with args, in the container of name's extension, with ffmpeg.
func containerFile(t *testing.T, ffmpeg, name string, args ...string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), name)
	duration := "duration=" + strconv.Itoa(containerSeconds)
	cmd := exec.Command(ffmpeg, slices.Concat([]string{"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=25:" + duration,
		"-f", "lavfi", "-i", "sine=" + duration}, args, []string{file})...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	return file
}

// localContainerFile is a test server, its ffprobe at ffprobe, with a
// movies folder holding the file made, which no addon matches: a plain
// movie, the item returned, whose runtime is the file's own. Its media
// source is the file's.
func localContainerFile(t *testing.T, ffprobe, made string) (playbackSetup, string, MediaSourceInfo) {
	t.Helper()
	var folders *localfiles.Service
	s := newProbingServer(t, 10, ffprobe, func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, slog.New(slog.NewTextHandler(io.Discard, nil)),
			o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
		folders.UseFiles(o.Playback.LocalFiles(folders.Open), o.Playback.LocalInputs(folders))
	})
	p := playingOn(t, s)
	dir := t.TempDir()
	if err := os.Rename(made, filepath.Join(dir, filepath.Base(made))); err != nil {
		t.Fatal(err)
	}
	addon, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Shelf", Path: dir, Kind: localfiles.KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	var views, page QueryResult
	p.get(t, "/UserViews", p.token, &views)
	for _, view := range views.Items {
		if view.Name == "Shelf" {
			p.get(t, "/Items?ParentId="+view.Id+"&Recursive=true&IncludeItemTypes=Movie", p.token, &page)
		}
	}
	if len(page.Items) != 1 {
		t.Fatalf("the folder's movies: %v", itemNames(page.Items))
	}
	var movie BaseItemDto
	p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+page.Items[0].Id, p.token, &movie)
	if movie.MediaSources == nil || len(*movie.MediaSources) != 1 {
		t.Fatalf("media sources: %+v", movie.MediaSources)
	}
	return p, movie.Id, (*movie.MediaSources)[0]
}

// playContainer asks PlaybackInfo for item's media source local on the
// app of profile, checks it plays over HLS only, for containerSeconds,
// and returns it. FFmpeg's encoders leave the audio up to a tenth of a
// second longer than the video.
func playContainer(t *testing.T, p playbackSetup, item string, local MediaSourceInfo, profile json.RawMessage) MediaSourceInfo {
	t.Helper()
	status, data := p.call(http.MethodPost, "/Items/"+item+"/PlaybackInfo", app("tv", p.token), map[string]any{
		"UserId": p.user.ID.String(), "MediaSourceId": local.Id, "DeviceProfile": profile})
	var info playbackInfoResponse
	if err := json.Unmarshal(data, &info); status != http.StatusOK || err != nil || len(info.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %d %s", status, data)
	}
	source := info.MediaSources[0]
	if source.Id != local.Id || source.SupportsDirectPlay || source.SupportsDirectStream || !source.SupportsTranscoding || source.TranscodingUrl == "" ||
		source.RunTimeTicks == nil || (time.Duration(*source.RunTimeTicks)*100-containerSeconds*time.Second).Abs() > 200*time.Millisecond {
		t.Fatalf("the file's source: %+v", source)
	}
	return source
}
