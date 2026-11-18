package playback

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/source"
	"github.com/moodiness/polyfin/internal/stremio"
)

// These tests read sources confined to public addresses with the real
// ffprobe, through the real Service. A server on the loopback interface
// stands for a host of the server's local network: FFmpeg and ffprobe
// read through Polyfin alone, so that it is asked nothing, and what names
// it is asked of Polyfin's opener, which refuses it.

// localHost stands for a host of the server's local network. It records
// what it is asked.
type localHost struct {
	*httptest.Server
	mu    sync.Mutex
	asked []string
}

func newLocalHost(t *testing.T, files map[string][]byte) *localHost {
	t.Helper()
	local := &localHost{}
	local.Server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		local.mu.Lock()
		local.asked = append(local.asked, r.Method+" "+r.URL.Path+" by "+r.UserAgent())
		local.mu.Unlock()
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
	}))
	local.Start()
	t.Cleanup(local.Close)
	return local
}

func (h *localHost) requests() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.asked)
}

// confiningOpener is the opener of these tests: it records what Polyfin
// itself asks for, and refuses, as the confining dialer does, a confined
// request to an address that is not public, before passing the others to
// next; those next does not know are fetched.
type confiningOpener struct {
	next source.Opener

	mu      sync.Mutex
	asked   []opened
	refused []string
}

// opened is a request Polyfin made.
type opened struct {
	target   string
	header   http.Header
	confined bool
}

func (o *confiningOpener) Open(ctx context.Context, method, target string, header http.Header, confined bool) (*http.Response, error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	local := false
	if ip := net.ParseIP(parsed.Hostname()); ip != nil {
		local = ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}
	o.mu.Lock()
	o.asked = append(o.asked, opened{target, header.Clone(), confined})
	if local && confined {
		o.refused = append(o.refused, target)
	}
	o.mu.Unlock()
	switch {
	case local && confined:
		return nil, stremio.ErrPrivateNetwork
	case local:
		request, err := http.NewRequestWithContext(ctx, method, target, nil)
		if err != nil {
			return nil, err
		}
		request.Header = header.Clone()
		return http.DefaultTransport.RoundTrip(request)
	}
	return o.next.Open(ctx, method, target, header, confined)
}

// refusedOne reports whether Polyfin asked for an address under origin
// within the confinement, and was refused it.
func (o *confiningOpener) refusedOne(origin string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.ContainsFunc(o.refused, func(target string) bool { return strings.HasPrefix(target, origin) })
}

// requests are the requests Polyfin made.
func (o *confiningOpener) requests() []opened {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.asked)
}

// readThroughPolyfin fails the test unless the local host was asked
// nothing, every request of Polyfin's was confined, and, when named is
// set, Polyfin was asked for what names the local host and refused it:
// its address was a link of the loopback interface's to FFmpeg.
func readThroughPolyfin(t *testing.T, local *localHost, opener *confiningOpener, named bool) {
	t.Helper()
	if asked := local.requests(); len(asked) > 0 {
		t.Errorf("the local host was asked: %q", asked)
	}
	if named && !opener.refusedOne(local.URL) {
		t.Errorf("Polyfin was not asked for the local host's address: FFmpeg was given no link for it")
	}
	requests := opener.requests()
	if len(requests) == 0 || slices.ContainsFunc(requests, func(request opened) bool { return !request.confined }) {
		t.Errorf("a request of Polyfin's was not confined, of %d", len(requests))
	}
}

// The attribute lists of a playlist's tags are written as FFmpeg reads
// them: an address that follows an unquoted value ended by white space,
// or a quoted one holding an escaped quote, is a link to FFmpeg, which
// asks Polyfin for it within the source's confinement.
func TestAPlaylistsAddressesAreAskedOfPolyfinHoweverItsTagsAreWritten(t *testing.T) {
	t.Parallel()
	ffprobe, segments := hlsSegments(t)
	for _, tc := range []struct {
		name string
		// tag is the line, given the local host's address.
		tag func(local string) string
	}{
		{"a key after an unquoted value ended by a space", func(local string) string {
			return "#EXT-X-KEY:METHOD=AES-128,KEYFORMAT=identity URI=" + local + "/key\n"
		}},
		{"a key after a quoted value holding an escaped quote", func(local string) string {
			return `#EXT-X-KEY:METHOD=AES-128,X="a\",Y=",URI=` + local + `/key,Z="` + "\n"
		}},
		{"an initialization section after an unquoted value ended by a tab", func(local string) string {
			return "#EXT-X-MAP:BYTERANGE=720@0\tURI=" + local + "/init.mp4\n"
		}},
		{"an initialization section after a quoted value holding an escaped quote", func(local string) string {
			return `#EXT-X-MAP:X="a\",Y=",URI=` + local + `/init.mp4,Z="` + "\n"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			local := newLocalHost(t, map[string][]byte{"/key": []byte("0123456789abcdef"), "/init.mp4": segments[0]})
			channel := liveChannel(segments, tc.tag(local.URL), func(n int) string { return "s" + strconv.Itoa(n) + ".ts" }, func(n int) *http.Response {
				return sized(http.StatusOK, "video/mp2t", segments[n])
			})
			opener := &confiningOpener{next: channel}
			s := newService(t, opener, ffprobe, nil)
			version := library.Version{ID: accounts.ID{0xA1, 1}, Name: "Zeb Sport · Live", URL: "https://tv.example/live/index.m3u8", Confined: true}
			// The key and the section are refused: the analysis fails or
			// not, what was asked is what matters.
			_, err := s.AnalyzeLive(t.Context(), version)
			t.Logf("the analysis: %v", err)
			readThroughPolyfin(t, local, opener, true)
		})
	}
}

// A rendition's playlist named after an unquoted value ended by a space is
// a link to FFmpeg too: the tracks of a local host's playlist are not
// among those the analysis keeps.
func TestARenditionsPlaylistIsAskedOfPolyfinHoweverItsTagIsWritten(t *testing.T) {
	t.Parallel()
	ffprobe, segments := hlsSegments(t)
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	// A track only the local host has: MP2 audio at 22.05 kHz.
	dir := t.TempDir()
	if output, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "sine=frequency=330:sample_rate=22050:duration=4",
		"-c:a", "mp2", "-b:a", "64k", "-f", "mpegts", filepath.Join(dir, "local.ts")).CombinedOutput(); err != nil {
		t.Fatalf("encoding: %v %s", err, output)
	}
	audio, err := os.ReadFile(filepath.Join(dir, "local.ts"))
	if err != nil {
		t.Fatal(err)
	}
	local := newLocalHost(t, map[string][]byte{
		"/cam/audio.m3u8": []byte("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:4\n#EXT-X-MEDIA-SEQUENCE:0\n#EXTINF:4.000,\nlocal.ts\n#EXT-X-ENDLIST\n"),
		"/cam/local.ts":   audio,
	})
	var video strings.Builder
	video.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n#EXT-X-MEDIA-SEQUENCE:0\n")
	files := map[string]func() *http.Response{}
	for n := range segments {
		fmt.Fprintf(&video, "#EXTINF:1.000,\ns%d.ts\n", n)
		files["s"+strconv.Itoa(n)+".ts"] = func() *http.Response { return sized(http.StatusOK, "video/mp2t", segments[n]) }
	}
	video.WriteString("#EXT-X-ENDLIST\n")
	files["video.m3u8"] = func() *http.Response {
		return sized(http.StatusOK, "application/vnd.apple.mpegurl", []byte(video.String()))
	}
	master := "#EXTM3U\n" +
		"#EXT-X-MEDIA:TYPE=AUDIO URI=" + local.URL + "/cam/audio.m3u8,GROUP-ID=\"a\",NAME=\"x\",DEFAULT=YES\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=200000,AUDIO=\"a\"\nvideo.m3u8\n"
	opener := &confiningOpener{next: &hlsProvider{playlist: master, files: files}}
	s := newService(t, opener, ffprobe, nil)
	version := library.Version{ID: accounts.ID{0xA1, 2}, Name: "Zeb Sport · Live", URL: "https://tv.example/live/index.m3u8", Confined: true}
	analysis, err := s.AnalyzeLive(t.Context(), version)
	t.Logf("the analysis: %v", err)
	for _, stream := range analysis.Streams {
		if stream.Codec == "mp2" {
			t.Errorf("the analysis kept lists the local host's track: %+v", stream)
		}
	}
	readThroughPolyfin(t, local, opener, true)
}

// dashFFprobe is POLYFIN_TEST_FFMPEG and the ffprobe beside it when they
// have a DASH demuxer, as the FFmpeg of Polyfin's image has; the test is
// skipped otherwise.
func dashFFprobe(t *testing.T) (ffmpeg, ffprobe string) {
	t.Helper()
	ffmpeg = os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	ffprobe = filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	if output, err := exec.Command(ffprobe, "-hide_banner", "-h", "demuxer=dash").Output(); err != nil || !strings.Contains(string(output), "Demuxer dash") {
		t.Skip("the ffprobe of POLYFIN_TEST_FFMPEG has no DASH demuxer")
	}
	return ffmpeg, ffprobe
}

// dashVideo encodes two seconds of video as DASH with FFmpeg and returns
// its initialization and its media segment, by the names a manifest of
// videoManifest gives them.
func dashVideo(t *testing.T, ffmpeg string) map[string][]byte {
	t.Helper()
	dir := t.TempDir()
	if output, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=176x144:rate=25:duration=2",
		"-c:v", "libx264", "-g", "25", "-pix_fmt", "yuv420p", "-f", "dash", "-seg_duration", "2", "-use_template", "1", "-use_timeline", "0",
		"-init_seg_name", "init.m4s", "-media_seg_name", "chunk-$Number$.m4s", filepath.Join(dir, "manifest.mpd")).CombinedOutput(); err != nil {
		t.Fatalf("encoding: %v %s", err, output)
	}
	files := map[string][]byte{}
	for _, name := range []string{"init.m4s", "chunk-1.m4s"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		files[name] = data
	}
	return files
}

// videoManifest is a DASH manifest of the video of dashVideo: its root
// written root, and its representation holding what, a BaseURL and a
// template for one.
func videoManifest(root, representation string) string {
	return `<` + root + ` xmlns="urn:mpeg:dash:schema:mpd:2011" xmlns:d="urn:mpeg:dash:schema:mpd:2011" profiles="urn:mpeg:dash:profile:isoff-live:2011" type="static" mediaPresentationDuration="PT2.0S" minBufferTime="PT2.0S">
 <Period id="0" start="PT0.0S">
  <AdaptationSet id="0" contentType="video" segmentAlignment="true">
   ` + representation + `
  </AdaptationSet>
 </Period>
</` + root + `>
`
}

// templated is a representation whose files are under base, an element
// written element, named by a template.
func templated(element, base string) string {
	return `<Representation id="0" mimeType="video/mp4" codecs="avc1.64000b" bandwidth="100000" width="176" height="144">
    <` + element + `>` + base + `</` + element + `>
    <SegmentTemplate timescale="1000000" duration="2000000" initialization="init.m4s" media="chunk-$Number$.m4s" startNumber="1"/>
   </Representation>`
}

// manifestHost answers a channel's DASH manifest at any address, and the
// files beside it by their names: the first answer is first, the later
// ones manifest when it is set, each of the type given.
type manifestHost struct {
	first, manifest string
	contentType     string
	files           map[string][]byte

	mu    sync.Mutex
	asked int
}

func (h *manifestHost) Open(_ context.Context, _, target string, _ http.Header, _ bool) (*http.Response, error) {
	if name := target[strings.LastIndexByte(target, '/')+1:]; strings.HasSuffix(name, ".m4s") {
		file, ok := h.files[name]
		if !ok {
			return sized(http.StatusNotFound, "text/plain", []byte("no such file")), nil
		}
		return sized(http.StatusOK, "video/mp4", file), nil
	}
	h.mu.Lock()
	h.asked++
	manifest := h.first
	if h.asked > 1 && h.manifest != "" {
		manifest = h.manifest
	}
	h.mu.Unlock()
	return sized(http.StatusOK, h.contentType, []byte(manifest)), nil
}

// A channel's DASH manifest is rewritten as FFmpeg reads it: whatever the
// case of its elements' names, wherever FFmpeg reads a template's
// addresses, and whatever comes before its MPD, at each time FFmpeg reads
// it. What it names on a local host is asked of Polyfin, within the
// source's confinement.
func TestAManifestsAddressesAreAskedOfPolyfinHoweverItIsWritten(t *testing.T) {
	t.Parallel()
	ffmpeg, ffprobe := dashFFprobe(t)
	files := dashVideo(t, ffmpeg)
	plain := videoManifest("MPD", templated("BaseURL", "https://tv.example/live/"))
	for _, tc := range []struct {
		name string
		// host is the channel's, given the local host's address.
		host func(local string) *manifestHost
	}{
		{"a BaseURL in lower case", func(local string) *manifestHost {
			return &manifestHost{first: videoManifest("MPD", templated("baseurl", local+"/nvr/")), contentType: "application/dash+xml"}
		}},
		{"a root in lower case and a BaseURL in upper case, told by its bytes", func(local string) *manifestHost {
			return &manifestHost{first: strings.ReplaceAll(videoManifest("MPD", templated("BASEURL", local+"/nvr/")), "MPD", "mpd"), contentType: "text/plain"}
		}},
		{"a template's addresses in a segment list", func(local string) *manifestHost {
			return &manifestHost{first: videoManifest("MPD", `<SegmentList initialization="`+local+`/nvr/init.m4s" media="`+local+`/nvr/chunk-$Number$.m4s"/>
   <Representation id="0" mimeType="video/mp4" codecs="avc1.64000b" bandwidth="100000" width="176" height="144">
    <SegmentTemplate timescale="1000000" duration="2000000" startNumber="1"/>
   </Representation>`), contentType: "application/dash+xml"}
		}},
		{"an MPD after two thousand bytes of a comment", func(local string) *manifestHost {
			return &manifestHost{first: "<!--" + strings.Repeat(" ", 2000) + "-->\n" + videoManifest("MPD", templated("BaseURL", local+"/nvr/")),
				contentType: "application/octet-stream"}
		}},
		{"another document once it was told a manifest", func(local string) *manifestHost {
			// What FFmpeg's probe takes for a manifest, by its comment, and
			// its XML library reads as one, by its root's name within its
			// namespace: the loopback interface tells neither by its first
			// bytes.
			return &manifestHost{first: plain, contentType: "application/octet-stream",
				manifest: strings.Repeat(" ", 1500) + "<!-- <MPD urn:mpeg:dash:profile:isoff-live:2011 -->\n" +
					videoManifest("d:MPD", templated("d:BaseURL", local+"/nvr/"))}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			local := newLocalHost(t, map[string][]byte{"/nvr/init.m4s": files["init.m4s"], "/nvr/chunk-1.m4s": files["chunk-1.m4s"]})
			opener := &confiningOpener{next: tc.host(local.URL)}
			s := newService(t, opener, ffprobe, nil)
			version := library.Version{ID: accounts.ID{0xA1, 4}, Name: "Zeb Sport · Live", URL: "https://tv.example/live/manifest.mpd", Confined: true}
			analysis, err := s.AnalyzeLive(t.Context(), version)
			t.Logf("the analysis: %v", err)
			if err == nil {
				t.Errorf("the analysis kept is of the local host's video: %+v", analysis.Streams)
			}
			readThroughPolyfin(t, local, opener, true)
		})
	}

	// The same manifest, of a public host, plays: its files are asked of
	// Polyfin, within the confinement.
	t.Run("public addresses", func(t *testing.T) {
		t.Parallel()
		local := newLocalHost(t, nil)
		opener := &confiningOpener{next: &manifestHost{first: plain, contentType: "application/dash+xml", files: files}}
		s := newService(t, opener, ffprobe, nil)
		version := library.Version{ID: accounts.ID{0xA1, 5}, Name: "Zeb Sport · Live", URL: "https://tv.example/live/manifest.mpd", Confined: true}
		analysis, err := s.AnalyzeLive(t.Context(), version)
		if err != nil {
			t.Fatalf("the analysis: %v", err)
		}
		if video, ok := LiveVideo(analysis); !DASH(analysis) || !ok || video.Width != 176 || video.Height != 144 {
			t.Errorf("the analysis kept is not of the channel's video: %+v", analysis)
		}
		readThroughPolyfin(t, local, opener, false)
		for _, name := range []string{"init.m4s", "chunk-1.m4s"} {
			if !slices.ContainsFunc(opener.requests(), func(request opened) bool { return request.target == "https://tv.example/live/"+name }) {
				t.Errorf("Polyfin was not asked for %s", name)
			}
		}
	})
}

// A version that is a file is read by the demuxers of one file: a file
// that is a DASH manifest is not read as one, so that ffprobe asks for
// nothing it names, and its analysis fails.
func TestAFileThatIsAManifestIsNotReadAsOne(t *testing.T) {
	t.Parallel()
	ffmpeg, ffprobe := dashFFprobe(t)
	files := dashVideo(t, ffmpeg)
	local := newLocalHost(t, map[string][]byte{"/nvr/init.m4s": files["init.m4s"], "/nvr/chunk-1.m4s": files["chunk-1.m4s"]})
	manifest := []byte(`<?xml version="1.0" encoding="utf-8"?>` + "\n" + videoManifest("MPD", templated("BaseURL", local.URL+"/nvr/")))
	opener := &confiningOpener{next: &fileSource{data: manifest}}
	s := newService(t, opener, ffprobe, nil)
	version := library.Version{ID: accounts.ID{0xA1, 3}, Name: "Movie.2026.1080p.mkv", URL: "https://cdn.example/movie.mkv",
		Size: int64(len(manifest)), Confined: true}
	analysis, err := s.Analyze(t.Context(), version)
	t.Logf("the analysis: %v", err)
	if err == nil {
		t.Errorf("the file was analyzed: %+v", analysis)
	}
	readThroughPolyfin(t, local, opener, false)
}

// A file that is an SDP description is not read as one either: ffprobe
// opens no port for what the description names, and reads nothing of what
// is sent there.
func TestAFileThatIsASessionDescriptionOpensNoPort(t *testing.T) {
	t.Parallel()
	ffprobe, segments := hlsSegments(t)
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	stream := filepath.Join(t.TempDir(), "stream.ts")
	if err := os.WriteFile(stream, bytes.Join(segments, nil), 0o600); err != nil {
		t.Fatal(err)
	}
	// An even port nothing listens on: RTP's, RTCP taking the next.
	port := 0
	for port == 0 {
		probe, err := net.ListenPacket("udp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		if candidate := probe.LocalAddr().(*net.UDPAddr).Port; candidate%2 == 0 {
			port = candidate
		}
		_ = probe.Close()
	}
	description := fmt.Sprintf("v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=stream\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=video %d RTP/AVP 33\r\na=rtpmap:33 MP2T/90000\r\n", port)
	opener := &confiningOpener{next: &fileSource{data: []byte(description)}}
	s := newService(t, opener, ffprobe, nil)
	version := library.Version{ID: accounts.ID{0xA1, 6}, Name: "Movie.2026.1080p.mkv", URL: "https://cdn.example/movie.mkv",
		Size: int64(len(description)), Confined: true}

	// A sender of the local network: MPEG-TS over RTP to the port, again
	// and again until the analysis ends, which an ffprobe that opened the
	// port would read.
	sending, stop := context.WithCancel(t.Context())
	sent := make(chan struct{})
	go func() {
		defer close(sent)
		for sending.Err() == nil {
			_ = exec.CommandContext(sending, ffmpeg, "-v", "quiet", "-re", "-i", stream, "-c", "copy", "-f", "rtp_mpegts",
				"rtp://127.0.0.1:"+strconv.Itoa(port)).Run()
			select {
			case <-sending.Done():
			case <-time.After(50 * time.Millisecond):
			}
		}
	}()
	analysis, err := s.Analyze(t.Context(), version)
	stop()
	<-sent
	t.Logf("the analysis: %v", err)
	if err == nil {
		t.Errorf("the file was analyzed, as %q: %+v", analysis.Format, analysis.Streams)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("ffprobe waited for a stream: %v", err)
	}
}

// FFmpeg and ffprobe are given the loopback interface as their proxy: it
// answers for its own addresses alone, so that an address they would
// fetch themselves is refused there.
func TestTheLoopbackInterfaceAnswersForItsOwnAddressesAlone(t *testing.T) {
	t.Parallel()
	data := bytes.Repeat([]byte("media"), 1000)
	s := newService(t, &fileSource{data: data}, "ffprobe", nil)
	version := library.Version{ID: accounts.ID{0xA1, 7}, Name: "Movie.2026.1080p.mkv", URL: "https://cdn.example/movie.mkv", Size: int64(len(data))}
	src := s.open(version)
	defer src.Release()
	target, release := s.loopback.register(version, src)
	defer release()
	own, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: own.Host})}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	get := func(address string) (int, []byte) {
		t.Helper()
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, address, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		return response.StatusCode, body
	}
	if status, body := get(target); status != http.StatusOK || !bytes.Equal(body, data) {
		t.Errorf("its own address, asked as a proxy is: HTTP %d, %d bytes", status, len(body))
	}
	// The same path on another host, which the interface would answer
	// were it asked for the path alone.
	for _, address := range []string{"http://192.168.1.10" + own.Path, "http://192.168.1.10:" + own.Port() + own.Path, "http://cdn.example/movie.mkv"} {
		if status, _ := get(address); status != http.StatusForbidden {
			t.Errorf("%s: HTTP %d, want it refused", address, status)
		}
	}
	// A tunnel, which a client asks its proxy for an HTTPS address.
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://192.168.1.10/movie.mkv", nil)
	if err != nil {
		t.Fatal(err)
	}
	if response, err := client.Do(request); err == nil {
		response.Body.Close()
		t.Errorf("a tunnel to another host was opened: HTTP %d", response.StatusCode)
	} else if !strings.Contains(err.Error(), "Forbidden") {
		t.Errorf("a tunnel to another host: %v, want it refused", err)
	}
}
