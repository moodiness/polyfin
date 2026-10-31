package jellyfin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// failoverFFmpeg is the FFmpeg of POLYFIN_TEST_FFMPEG, and the ffprobe
// beside it, which analyses run.
func failoverFFmpeg(t *testing.T) (ffmpeg, ffprobe string) {
	t.Helper()
	ffmpeg = os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set: a play goes on from another version through FFmpeg")
	}
	return ffmpeg, filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
}

// failoverClip makes 30 seconds of a moving picture, in colour or in
// grey, 320 × 240 at 24 frames a second, with a keyframe every gop
// frames, about 2 Mb/s, and stereo AAC at 48 kHz, in MP4 with its index
// first: two versions of one title, as two releases are.
func failoverClip(t *testing.T, ffmpeg string, grey bool, gop int) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "clip.mp4")
	picture := "testsrc2=size=320x240:rate=24:duration=30"
	if grey {
		picture += ",hue=s=0"
	}
	out, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", picture, "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=30",
		"-ac", "2", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-g", strconv.Itoa(gop), "-keyint_min", strconv.Itoa(gop),
		"-sc_threshold", "0", "-b:v", "2M", "-minrate", "2M", "-maxrate", "2M", "-bufsize", "1M", "-c:a", "aac", "-b:a", "96k",
		"-movflags", "+faststart", "-shortest", path).CombinedOutput()
	if err != nil {
		t.Fatalf("making a clip: %v: %s", err, out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// dyingHost serves data with ranges, but never its bytes from limit on,
// which its answers hold until kill is called: the host then goes down,
// cutting them, and closing every connection after at once.
type dyingHost struct {
	*httptest.Server
	kill func()
}

func newDyingHost(t *testing.T, data []byte, limit int) dyingHost {
	t.Helper()
	killed := make(chan struct{})
	var dead atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dead.Load() {
			if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
				_ = conn.Close()
			}
			return
		}
		http.ServeContent(&heldWriter{ResponseWriter: w, left: -1, limit: limit, killed: killed, dead: &dead, r: r}, r, "", time.Time{},
			bytes.NewReader(data))
	}))
	t.Cleanup(server.Close)
	var once sync.Once
	return dyingHost{Server: server, kill: func() {
		once.Do(func() {
			dead.Store(true)
			close(killed)
		})
	}}
}

// heldWriter writes an answer up to the byte limit of the file, then a
// trickle, a kilobyte every two seconds, so that the connection does not
// stall, until the host is killed, which cuts it.
type heldWriter struct {
	http.ResponseWriter
	r      *http.Request
	limit  int
	left   int
	killed chan struct{}
	dead   *atomic.Bool
}

func (w *heldWriter) Write(p []byte) (int, error) {
	if w.left < 0 {
		start := 0
		if value, ok := strings.CutPrefix(w.r.Header.Get("Range"), "bytes="); ok {
			start, _ = strconv.Atoi(strings.Split(value, "-")[0])
		}
		w.left = max(w.limit-start, 0)
	}
	if len(p) <= w.left {
		w.left -= len(p)
		return w.ResponseWriter.Write(p)
	}
	total := len(p)
	_, _ = w.ResponseWriter.Write(p[:w.left])
	p, w.left = p[w.left:], 0
	w.ResponseWriter.(http.Flusher).Flush()
	for {
		select {
		case <-w.killed:
			panic(http.ErrAbortHandler)
		case <-w.r.Context().Done():
			panic(http.ErrAbortHandler)
		case <-time.After(2 * time.Second):
		}
		if len(p) == 0 {
			return total, nil
		}
		n := min(len(p), 1024)
		_, _ = w.ResponseWriter.Write(p[:n])
		w.ResponseWriter.(http.Flusher).Flush()
		p = p[n:]
	}
}

// steadyHost serves data with ranges, counting the requests.
func steadyHost(t *testing.T, data []byte) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

// failoverAddon lists one movie, with the streams given.
func failoverAddon(t *testing.T, id string, streams []stremio.Stream) string {
	t.Helper()
	movie := stremio.Meta{ID: "tt7160", Type: "movie", Name: "Failover", Runtime: "1 min", Year: "2026"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			manifest := stremio.Manifest{ID: id, Name: id, Version: "1", Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "stream"}}}
			if id == "catalog" {
				manifest.Resources = append(manifest.Resources, stremio.Resource{Name: "catalog"}, stremio.Resource{Name: "meta"})
				manifest.Catalogs = []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}
			}
			_ = json.NewEncoder(w).Encode(manifest)
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case path == "/meta/movie/tt7160.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case path == "/stream/movie/tt7160.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": streams})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// failoverSetup is a server whose analyses run ffprobe, with a member
// signed in, and the movie the addons installed list.
type failoverSetup struct {
	testServer
	token, movie string
}

// failoverServer installs addons, the first with the catalog, and signs
// a member in.
func failoverServer(t *testing.T, ffprobe string, addonURLs ...string) failoverSetup {
	t.Helper()
	s := newProbingServer(t, 10, ffprobe)
	s.user("member", nil)
	for _, addon := range addonURLs {
		if _, err := s.addons.Install(t.Context(), addons.Shared(), addon, false); err != nil {
			t.Fatal(err)
		}
	}
	token := s.signIn("member", "tv")
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	var page QueryResult
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	if len(page.Items) != 1 {
		t.Fatalf("items: %+v", page.Items)
	}
	return failoverSetup{testServer: s, token: token, movie: page.Items[0].Id}
}

func recordedProfile(t *testing.T, name string) json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(playbackFixtures, "profiles", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// colourOf is how coloured the first frame of a segment is, its
// initialization segment first when given: the mean distance of its
// chroma from grey.
func colourOf(t *testing.T, ffmpeg string, parts ...string) float64 {
	t.Helper()
	path := filepath.Join(t.TempDir(), "segment")
	if err := os.WriteFile(path, []byte(strings.Join(parts, "")), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(ffmpeg, "-v", "error", "-i", path, "-frames:v", "1", "-vf", "scale=16:16,format=yuv444p", "-f", "rawvideo", "-").Output()
	if err != nil || len(out) != 3*256 {
		t.Fatalf("decoding a segment: %v (%d bytes)", err, len(out))
	}
	var total float64
	for _, c := range out[256:] {
		total += float64(max(c, 128) - min(c, 128))
	}
	return total / 512
}

// segmentsOf are the segment URIs of a media playlist, in order, and the
// indexes among them that follow an EXT-X-DISCONTINUITY.
func segmentsOf(playlist string) (uris []string, breaks []int) {
	discontinuity := false
	for line := range strings.SplitSeq(playlist, "\n") {
		switch {
		case line == "#EXT-X-DISCONTINUITY":
			discontinuity = true
		case line != "" && !strings.HasPrefix(line, "#"):
			if discontinuity {
				breaks = append(breaks, len(uris))
			}
			uris, discontinuity = append(uris, line), false
		}
	}
	return uris, breaks
}

// fetchSegment asks for a segment as a player does: again while the
// server is busy (503), for a minute at most.
func fetchSegment(t *testing.T, target string) string {
	t.Helper()
	deadline := time.Now().Add(time.Minute)
	for {
		status, _, body := fetchText(t, target)
		if status == http.StatusOK {
			return body
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: %d %s", target, status, body)
		}
		time.Sleep(time.Second)
	}
}

// failoverPlay plays the movie over HLS as profile asks, more given to
// PlaybackInfo, until segment upTo, kills the first host, and plays it to
// its end: it returns the media playlist after, the segments, and the
// initialization segment, empty for MPEG-TS.
func failoverPlay(t *testing.T, profile string, more map[string]any, upTo int, extension string) (string, []string, string) {
	t.Helper()
	ffmpeg, ffprobe := failoverFFmpeg(t)
	colour, grey := failoverClip(t, ffmpeg, false, 48), failoverClip(t, ffmpeg, true, 72)
	first := newDyingHost(t, colour, len(colour)/2)
	second, requests := steadyHost(t, grey)
	p := failoverServer(t, ffprobe,
		failoverAddon(t, "catalog", []stremio.Stream{{Name: "Colour", URL: first.URL + "/colour.mp4",
			BehaviorHints: stremio.StreamBehavior{Filename: "Failover.2026.1080p.Colour.mp4", VideoSize: stremio.Number(len(colour))}}}),
		failoverAddon(t, "other", []stremio.Stream{{Name: "Grey", URL: second.URL + "/grey.mp4",
			BehaviorHints: stremio.StreamBehavior{Filename: "Failover.2026.1080p.Grey.mp4", VideoSize: stremio.Number(len(grey))}}}))
	body := map[string]any{"MediaSourceId": p.movie, "EnableDirectPlay": false}
	for key, value := range more {
		body[key] = value
	}
	answer := p.ask(t, p.token, p.movie, recordedProfile(t, profile), body)
	if len(answer.MediaSources) != 1 || answer.MediaSources[0].TranscodingUrl == "" || !strings.HasPrefix(answer.MediaSources[0].Name, "Colour") {
		t.Fatalf("PlaybackInfo: %+v", answer)
	}
	base, query, _ := strings.Cut(p.url+answer.MediaSources[0].TranscodingUrl, "master.m3u8?")
	_, _, playlist := fetchText(t, base+"main.m3u8?"+query)
	uris, _ := segmentsOf(playlist)
	if len(uris) <= upTo+2 {
		t.Fatalf("playlist:\n%s", playlist)
	}
	init := ""
	if extension == "mp4" {
		init = fetchSegment(t, base+"hls1/main/-1.mp4?"+query)
	}
	segments := make([]string, len(uris))
	for n := 0; n <= upTo; n++ {
		segments[n] = fetchSegment(t, base+"hls1/main/"+strconv.Itoa(n)+"."+extension+"?"+query)
	}
	first.kill()
	for n := upTo + 1; n < len(uris); n++ {
		segments[n] = fetchSegment(t, base+"hls1/main/"+strconv.Itoa(n)+"."+extension+"?"+query)
	}
	_ = requests
	_, _, after := fetchText(t, base+"main.m3u8?"+query)
	return after, segments, init
}

// checkSwitch checks that the segments of a play are the first version's
// until one, from which they are the second's, the playlist marking it
// alone with a discontinuity, and that the segments decode across it.
func checkSwitch(t *testing.T, playlist string, segments []string, init string, upTo int) {
	t.Helper()
	ffmpeg, _ := failoverFFmpeg(t)
	switched := -1
	for n, segment := range segments {
		colour := colourOf(t, ffmpeg, init, segment)
		coloured := colour > 8
		switch {
		case switched < 0 && !coloured:
			switched = n
		case switched >= 0 && coloured:
			t.Errorf("segment %d is the first version's, after the switch at %d", n, switched)
		}
	}
	if switched <= upTo {
		t.Fatalf("switched at segment %d, played to %d before the host went down", switched, upTo)
	}
	if _, breaks := segmentsOf(playlist); len(breaks) != 1 || breaks[0] != switched {
		t.Errorf("discontinuities %v, switched at %d:\n%s", breaks, switched, playlist)
	}
	// The segments around the switch play one after the other, the player
	// keeping the initialization segment it has.
	path := filepath.Join(t.TempDir(), "across")
	if err := os.WriteFile(path, []byte(init+segments[switched-1]+segments[switched]), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(ffmpeg, "-v", "error", "-i", path, "-f", "null", "-").CombinedOutput()
	if err != nil || len(bytes.TrimSpace(out)) > 0 {
		t.Errorf("decoding across the switch: %v: %s", err, out)
	}
}

// A play repackaged into MPEG-TS segments, as Android TV asks for, goes on
// from the next version once its host goes down: the segments keep coming,
// the next ones from the other file, converted into the codec and size of
// the first, with one discontinuity in the playlist.
func TestARepackagedPlayGoesOnFromTheNextVersion(t *testing.T) {
	t.Parallel()
	playlist, segments, _ := failoverPlay(t, "androidtv", nil, 2, "ts")
	checkSwitch(t, playlist, segments, "", 2)
}

// A converted play in fragmented MP4 segments, as jellyfin-web asks for in
// Chrome, goes on the same way, with the same conversion: the player
// keeps the initialization segment it has.
func TestAConvertedPlayGoesOnFromTheNextVersion(t *testing.T) {
	t.Parallel()
	playlist, segments, init := failoverPlay(t, "jellyfin-web-chrome", map[string]any{"AllowVideoStreamCopy": false}, 1, "mp4")
	if init == "" {
		t.Fatal("no initialization segment")
	}
	checkSwitch(t, playlist, segments, init, 1)
}

// A play never goes on from an MPEG program stream or an AVI file, which
// it would read without their searching from before the time asked and
// their start shifted to zero: it passes over both, coloured as the first
// version, to the grey version after them.
func TestAPlayGoesOnFromNoUnindexedFile(t *testing.T) {
	t.Parallel()
	ffmpeg, ffprobe := failoverFFmpeg(t)
	colour, grey := failoverClip(t, ffmpeg, false, 48), failoverClip(t, ffmpeg, true, 72)
	program := failoverCopy(t, ffmpeg, colour, "mpg", "-c:v", "mpeg2video", "-b:v", "2M", "-g", "12", "-c:a", "mp2")
	avi := failoverCopy(t, ffmpeg, colour, "avi", "-c:v", "mpeg4", "-q:v", "4", "-c:a", "libmp3lame")
	first := newDyingHost(t, colour, len(colour)/2)
	hosts := map[string][]byte{"colour.mpg": program, "colour.avi": avi, "grey.mp4": grey}
	urls := map[string]string{}
	for name, data := range hosts {
		host, _ := steadyHost(t, data)
		urls[name] = host.URL + "/" + name
	}
	stream := func(name, url, filename string, data []byte) []stremio.Stream {
		return []stremio.Stream{{Name: name, URL: url, BehaviorHints: stremio.StreamBehavior{Filename: filename, VideoSize: stremio.Number(len(data))}}}
	}
	p := failoverServer(t, ffprobe,
		failoverAddon(t, "catalog", stream("Colour", first.URL+"/colour.mp4", "Failover.2026.1080p.Colour.mp4", colour)),
		failoverAddon(t, "program", stream("Program", urls["colour.mpg"], "Failover.2026.1080p.Program.mpg", program)),
		failoverAddon(t, "avi", stream("AVI", urls["colour.avi"], "Failover.2026.1080p.AVI.avi", avi)),
		failoverAddon(t, "other", stream("Grey", urls["grey.mp4"], "Failover.2026.1080p.Grey.mp4", grey)))
	answer := p.ask(t, p.token, p.movie, recordedProfile(t, "androidtv"), map[string]any{"MediaSourceId": p.movie, "EnableDirectPlay": false})
	if len(answer.MediaSources) != 1 || answer.MediaSources[0].TranscodingUrl == "" || !strings.HasPrefix(answer.MediaSources[0].Name, "Colour") {
		t.Fatalf("PlaybackInfo: %+v", answer)
	}
	base, query, _ := strings.Cut(p.url+answer.MediaSources[0].TranscodingUrl, "master.m3u8?")
	_, _, playlist := fetchText(t, base+"main.m3u8?"+query)
	uris, _ := segmentsOf(playlist)
	const upTo = 2
	if len(uris) <= upTo+2 {
		t.Fatalf("playlist:\n%s", playlist)
	}
	segments := make([]string, len(uris))
	for n := range segments {
		if n == upTo+1 {
			first.kill()
		}
		segments[n] = fetchSegment(t, base+"hls1/main/"+strconv.Itoa(n)+".ts?"+query)
	}
	_, _, after := fetchText(t, base+"main.m3u8?"+query)
	checkSwitch(t, after, segments, "", upTo)
}

// failoverCopy is clip in the container of extension, encoded with args.
func failoverCopy(t *testing.T, ffmpeg string, clip []byte, extension string, args ...string) []byte {
	t.Helper()
	dir := t.TempDir()
	in, out := filepath.Join(dir, "clip.mp4"), filepath.Join(dir, "copy."+extension)
	if err := os.WriteFile(in, clip, 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(ffmpeg, slices.Concat([]string{"-v", "error", "-i", in}, args, []string{out})...).CombinedOutput(); err != nil {
		t.Fatalf("making a copy: %v: %s", err, output)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// A repackaged play whose host goes down reads on from the copy of the
// file another addon lists, from where it stopped: the segments keep
// coming, all from the same file, without a discontinuity.
func TestARepackagedPlayGoesOnFromAMirror(t *testing.T) {
	t.Parallel()
	ffmpeg, ffprobe := failoverFFmpeg(t)
	clip := failoverClip(t, ffmpeg, false, 48)
	first := newDyingHost(t, clip, len(clip)/2)
	second, requests := steadyHost(t, clip)
	hints := stremio.StreamBehavior{Filename: "Failover.2026.1080p.mp4", VideoSize: stremio.Number(len(clip))}
	p := failoverServer(t, ffprobe,
		failoverAddon(t, "catalog", []stremio.Stream{{Name: "First", URL: first.URL + "/clip.mp4", BehaviorHints: hints}}),
		failoverAddon(t, "other", []stremio.Stream{{Name: "Copy", URL: second.URL + "/token/clip.mp4", BehaviorHints: hints}}))
	answer := p.ask(t, p.token, p.movie, recordedProfile(t, "androidtv"), map[string]any{"MediaSourceId": p.movie, "EnableDirectPlay": false})
	if len(answer.MediaSources) != 1 || answer.MediaSources[0].TranscodingUrl == "" {
		t.Fatalf("PlaybackInfo: %+v", answer)
	}
	base, query, _ := strings.Cut(p.url+answer.MediaSources[0].TranscodingUrl, "master.m3u8?")
	_, _, playlist := fetchText(t, base+"main.m3u8?"+query)
	uris, _ := segmentsOf(playlist)
	for n := range uris {
		if n == 3 {
			first.kill()
		}
		segment := fetchSegment(t, base+"hls1/main/"+strconv.Itoa(n)+".ts?"+query)
		if colourOf(t, ffmpeg, segment) <= 8 {
			t.Errorf("segment %d is not the file's", n)
		}
	}
	if requests.Load() == 0 {
		t.Error("the copy was not read")
	}
	if _, _, after := fetchText(t, base+"main.m3u8?"+query); strings.Contains(after, "#EXT-X-DISCONTINUITY") {
		t.Errorf("a discontinuity:\n%s", after)
	}
}
