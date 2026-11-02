package jellyfin

import (
	"encoding/json"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// An H.264 MPEG-TS file of a local folder or a network share plays in
// jellyfin-web 12.4 over HLS with its video copied, cut on the keyframes
// read from the whole file at its first play, even for a user who may not
// have video converted: its segments, in order, hold each of its frames
// once, as the file holds them, at its times shifted to start at zero, its
// audio starting 21 ms before its video (see hls.timestampOffset); a later
// segment read first, from a seek, starts on its keyframe, at the second
// its playlist gives, and holds the same frames as in order. Its version
// is then named as it plays, copied.
func TestLocalMPEGTSFilesPlayCopied(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	file := filepath.Join(t.TempDir(), "movie.ts")
	// 12.5 minutes, a tenth of the movie's runtime at least (see
	// playback.standIn), of 10 frames a second, a keyframe every 4.5
	// seconds, which segments of 4 seconds do not fall on, and B-frames.
	const seconds, rate = 750, 10
	if out, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=128x72:rate="+strconv.Itoa(rate), "-f", "lavfi", "-i", "sine=sample_rate=48000",
		"-t", strconv.Itoa(seconds), "-c:v", "libx264", "-preset", "veryfast", "-bf", "3", "-g", "1000", "-sc_threshold", "0",
		"-force_key_frames", "expr:gte(t,n_forced*4.5)", "-c:a", "aac", "-f", "mpegts", file).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	pictures := frameHashes(t, ffmpeg, file)
	// Where the video starts in the file's times shifted to start at zero:
	// its audio starts first.
	output, err := exec.Command(ffprobe, "-v", "error", "-show_entries", "format=start_time:stream=start_time", "-of", "csv=p=0", file).Output()
	if err != nil {
		t.Fatal(err)
	}
	var startTimes []float64
	for line := range strings.Lines(string(output)) {
		if at, err := strconv.ParseFloat(strings.Trim(strings.TrimSpace(line), ","), 64); err == nil {
			startTimes = append(startTimes, at)
		}
	}
	videoStart := startTimes[0] - slices.Min(startTimes)
	for _, share := range []bool{false, true} {
		t.Run(map[bool]string{false: "local folder", true: "network share"}[share], func(t *testing.T) {
			t.Parallel()
			p, local := localDiscOn(t, ffprobe, func(at string) {
				data, err := os.ReadFile(file)
				if err == nil {
					err = os.WriteFile(at+".ts", data, 0o644)
				}
				if err != nil {
					t.Fatal(err)
				}
			}, share)
			profile := p.profile(t, "jellyfin-web-12.4-chrome")
			// Its first play, by a user who may not have video converted.
			if _, err := p.store.UpdateUser(t.Context(), p.user.ID, accounts.UserChanges{VideoTranscoding: new(false)}, nil); err != nil {
				t.Fatal(err)
			}
			source := playItemOn(t, p, p.movie, local, seconds*time.Second, profile)
			if strings.Contains(source.TranscodingUrl, "allowVideoStreamCopy=false") {
				t.Fatalf("the video is converted: %s", source.TranscodingUrl)
			}
			if got := tracks(source); !slices.Equal(got[1:], []string{"Video h264 ", "Audio aac "}) {
				t.Errorf("tracks: %q", got)
			}
			uris := segmentURIs(t, p, source.TranscodingUrl)
			starts := segmentStarts(t, p, source.TranscodingUrl)
			if len(starts) != len(uris)-1 || len(starts) < 5 {
				t.Fatalf("%d segments, starting at %v", len(uris)-1, starts)
			}
			later := len(uris) * 2 / 3
			seek := playItemOn(t, p, p.movie, local, seconds*time.Second, profile)
			seekURIs := segmentURIs(t, p, seek.TranscodingUrl)
			sought, _ := frameTimes(t, ffprobe, decoded(t, ffmpeg, getSegment(t, p, seekURIs[0]), getSegment(t, p, seekURIs[later])))

			segments := make([][]byte, len(uris))
			for i, uri := range uris {
				segments[i] = getSegment(t, p, uri)
			}
			played := decoded(t, ffmpeg, segments...)
			times, codec := frameTimes(t, ffprobe, played)
			if len(times) != seconds*rate || codec != "h264" {
				t.Fatalf("the segments hold %d %s frames", len(times), codec)
			}
			for i, at := range times {
				if want := 10 + videoStart + float64(i)/rate; math.Abs(at-want) > 0.002 {
					t.Fatalf("frame %d at %.4f s, not %.4f s", i, at, want)
				}
			}
			if got := frameHashes(t, ffmpeg, played); !slices.Equal(got, pictures) {
				t.Errorf("the video was not copied: %d pictures, %d the same as the file's", len(got), countSame(got, pictures))
			}
			inOrder, _ := frameTimes(t, ffprobe, decoded(t, ffmpeg, segments[0], segments[later]))
			if len(sought) == 0 || !slices.Equal(sought, inOrder) {
				t.Errorf("segment %d from a seek: %d frames, in order %d", later-1, len(sought), len(inOrder))
			}
			// The segment sought starts on a keyframe, where the playlist says.
			if first := sought[slices.IndexFunc(sought, func(at float64) bool { return at > 10+starts[later-1]-0.5 })]; math.Abs(first-10-starts[later-1]) > 0.01 {
				t.Errorf("segment %d starts at %.3f s, its playlist at %.3f s", later-1, first-10, starts[later-1])
			}
			var movie BaseItemDto
			p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie)
			for _, s := range *movie.MediaSources {
				if s.Id == local.Id && (strings.Contains(s.Name, "converted") || strings.Contains(s.Name, "cannot play")) {
					t.Errorf("the version is named %q", s.Name)
				}
			}
		})
	}
}

// A local MPEG-TS file whose keyframes are not read when a play asks, as
// a large file on a slow share, has that play wait a few seconds at most:
// it plays converted, cut every few seconds, and keeps that cut once the
// read, which goes on, is done; the next play copies its video.
func TestLocalMPEGTSFilesPlayConvertedUntilTheirKeyframesAreRead(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	if runtime.GOOS == "windows" {
		t.Skip("the slow ffprobe is a shell script")
	}
	dir := t.TempDir()
	// ffprobe, its read of a whole file held until the test releases it, or
	// ends.
	release := filepath.Join(dir, "release")
	t.Cleanup(func() { _ = os.WriteFile(release, nil, 0o644) })
	ffprobe := filepath.Join(dir, "ffprobe")
	script := "#!/bin/sh\ncase \"$*\" in *packet=pts_time*) while [ ! -e '" + release + "' ] && [ -d '" + dir + "' ]; do sleep 0.05; done;; esac\nexec '" +
		filepath.Join(filepath.Dir(ffmpeg), "ffprobe") + "' \"$@\"\n"
	if err := os.WriteFile(ffprobe, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "movie.ts")
	if out, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=128x72:rate=10", "-f", "lavfi", "-i", "sine=sample_rate=48000",
		"-t", "750", "-c:v", "libx264", "-preset", "veryfast", "-g", "1000", "-sc_threshold", "0",
		"-force_key_frames", "expr:gte(t,n_forced*4.5)", "-c:a", "aac", "-f", "mpegts", file).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	p, local := localDiscOn(t, ffprobe, func(at string) {
		data, err := os.ReadFile(file)
		if err == nil {
			err = os.WriteFile(at+".ts", data, 0o644)
		}
		if err != nil {
			t.Fatal(err)
		}
	}, false)
	profile := p.profile(t, "jellyfin-web-12.4-chrome")
	first := playItemOn(t, p, p.movie, local, 750*time.Second, profile)
	if !strings.Contains(first.TranscodingUrl, "allowVideoStreamCopy=false") {
		t.Fatalf("the play waiting for the read copies the video: %s", first.TranscodingUrl)
	}
	grid := segmentStarts(t, p, first.TranscodingUrl)
	if len(grid) < 3 || grid[1] != 2 || grid[2] != 6 {
		t.Fatalf("the first play is not cut every few seconds: %v", grid[:min(3, len(grid))])
	}
	var movie BaseItemDto
	p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie)
	for _, s := range *movie.MediaSources {
		if s.Id == local.Id && !strings.HasSuffix(s.Name, "(converted)") {
			t.Errorf("while its keyframes are read, the version is named %q", s.Name)
		}
	}

	if err := os.WriteFile(release, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	// The next play waits for the read, released: it copies the video, cut
	// on its keyframes.
	next := playItemOn(t, p, p.movie, local, 750*time.Second, profile)
	if strings.Contains(next.TranscodingUrl, "allowVideoStreamCopy=false") {
		t.Fatalf("the play after the read converts the video: %s", next.TranscodingUrl)
	}
	if starts := segmentStarts(t, p, next.TranscodingUrl); len(starts) < 2 || math.Abs(starts[1]-4.5) > 0.1 {
		t.Errorf("the next play is not cut on the keyframes: %v", starts[:min(2, len(starts))])
	}
	// The first play keeps its cut: its playlist, and its segments, still
	// converted.
	if again := segmentStarts(t, p, first.TranscodingUrl); !slices.Equal(again, grid) {
		t.Errorf("the first play's segments changed: %v, then %v", grid[:3], again[:min(3, len(again))])
	}
	uris := segmentURIs(t, p, first.TranscodingUrl)
	times, codec := frameTimes(t, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), decoded(t, ffmpeg, getSegment(t, p, uris[0]), getSegment(t, p, uris[3])))
	if codec != "h264" || len(times) == 0 || math.Abs(times[0]-10-grid[2]) > 0.15 {
		t.Errorf("the first play's segment 2: %s from %v", codec, times[:min(1, len(times))])
	}
}

// A local MPEG-TS file whose keyframes cannot be used, as one joined from
// two recordings whose timestamps go back where the second starts, plays
// as before: over HLS in jellyfin-web, cut every few seconds, its video
// converted, and not for a user who may not have video converted.
func TestLocalMPEGTSFilesWithoutUsableKeyframesPlayConverted(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	dir := t.TempDir()
	var joined []byte
	for _, part := range []struct{ name, seconds string }{{"first.ts", "20"}, {"second.ts", "750"}} {
		path := filepath.Join(dir, part.name)
		if out, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=128x72:rate=10", "-f", "lavfi", "-i", "sine=sample_rate=48000",
			"-t", part.seconds, "-c:v", "libx264", "-preset", "veryfast", "-g", "20", "-c:a", "aac", "-f", "mpegts", path).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		joined = append(joined, data...)
	}
	p, local := localDiscOn(t, ffprobe, func(at string) {
		if err := os.WriteFile(at+".ts", joined, 0o644); err != nil {
			t.Fatal(err)
		}
	}, false)
	profile := p.profile(t, "jellyfin-web-12.4-chrome")
	source := playItemOn(t, p, p.movie, local, 750*time.Second, profile)
	if !strings.Contains(source.TranscodingUrl, "allowVideoStreamCopy=false") {
		t.Fatalf("the video is copied: %s", source.TranscodingUrl)
	}
	uris := segmentURIs(t, p, source.TranscodingUrl)
	if _, codec := frameTimes(t, ffprobe, decoded(t, ffmpeg, getSegment(t, p, uris[0]), getSegment(t, p, uris[1]))); codec != "h264" {
		t.Errorf("the first segment's video: %q", codec)
	}
	if _, err := p.store.UpdateUser(t.Context(), p.user.ID, accounts.UserChanges{VideoTranscoding: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token), map[string]any{
		"UserId": p.user.ID.String(), "MediaSourceId": local.Id, "DeviceProfile": profile})
	var refused playbackInfoResponse
	if err := json.Unmarshal(data, &refused); status != http.StatusOK || err != nil || len(refused.MediaSources) != 0 {
		t.Errorf("without video conversion: %d %s", status, data)
	}
}

// frameHashes are the hashes of the decoded pictures of file's video, in
// order: the same as a file's when its video was copied, where a
// conversion would change them.
func frameHashes(t *testing.T, ffmpeg, file string) []string {
	t.Helper()
	output, err := exec.Command(ffmpeg, "-v", "error", "-i", file, "-map", "0:v:0", "-f", "framemd5", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	var hashes []string
	for line := range strings.Lines(string(output)) {
		if fields := strings.Split(strings.TrimSpace(line), ","); !strings.HasPrefix(line, "#") && len(fields) == 6 {
			hashes = append(hashes, strings.TrimSpace(fields[5]))
		}
	}
	return hashes
}

// countSame counts the hashes a and b have the same at the same place.
func countSame(a, b []string) int {
	same := 0
	for i := range min(len(a), len(b)) {
		if a[i] == b[i] {
			same++
		}
	}
	return same
}

// segmentStarts are the times the media playlist of a transcoding URL
// starts its segments at, in seconds, from its durations.
func segmentStarts(t *testing.T, p playbackSetup, transcoding string) []float64 {
	t.Helper()
	base := transcoding[:strings.Index(transcoding, "master.m3u8")]
	var playlist string
	for line := range strings.Lines(string(getSegment(t, p, transcoding))) {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			playlist = line
			if !strings.HasPrefix(line, "/") {
				playlist = base + line
			}
			break
		}
	}
	var starts []float64
	at := 0.0
	for line := range strings.Lines(string(getSegment(t, p, playlist))) {
		if duration, ok := strings.CutPrefix(strings.TrimSpace(line), "#EXTINF:"); ok {
			seconds, err := strconv.ParseFloat(strings.TrimSuffix(strings.Split(duration, ",")[0], ","), 64)
			if err != nil {
				t.Fatalf("%q: %v", line, err)
			}
			starts = append(starts, at)
			at += seconds
		}
	}
	return starts
}
