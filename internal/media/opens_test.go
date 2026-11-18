package media

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// option is the value an option of FFmpeg's has in options, empty when it
// is not among them.
func option(options []string, name string) string {
	if i := slices.Index(options, name); i >= 0 && i+1 < len(options) {
		return options[i+1]
	}
	return ""
}

// following are demuxers of FFmpeg that open the addresses, files or
// ports named by what they read, or run it.
var following = []string{"hls", "applehttp", "dash", "concat", "imf", "sdp", "rtsp", "rtp", "sap", "image2", "image2pipe",
	"webm_dash_manifest", "avisynth", "vapoursynth", "lavfi", "dvdvideo"}

// An input on the loopback interface is given the protocols it needs, the
// demuxers of one file, with the playlist's or the manifest's only when
// Polyfin found one, and the interface as its proxy. A path, a disc or
// another address gets no option.
func TestTheOptionsOfALoopbackInputNameWhatItMayOpen(t *testing.T) {
	t.Parallel()
	for _, address := range []string{"/media/movie.mkv", `C:\media\movie.mkv`, "bluray:/media/disc", "https://cdn.example/movie.mkv",
		"http://192.168.1.10:8080/movie.mkv", "http://127.0.0.1/movie.mkv"} {
		if got := Opens(address, File); got != nil {
			t.Errorf("Opens(%q) = %q, want no option", address, got)
		}
	}
	for _, tc := range []struct {
		name      string
		reading   Reading
		protocols string
		demuxer   string
	}{
		{"a file", File, "http,tcp", ""},
		{"a playlist", Playlist, "http,tcp,crypto,data", "hls"},
		{"a manifest", Manifest, "http,tcp", "dash"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			options := Opens("http://127.0.0.1:4242/0123abcd/movie.mkv", tc.reading)
			if got := option(options, "-protocol_whitelist"); got != tc.protocols {
				t.Errorf("the protocols are %q, want %q", got, tc.protocols)
			}
			if got := option(options, "-http_proxy"); got != "http://127.0.0.1:4242" {
				t.Errorf("the proxy is %q, want the loopback interface", got)
			}
			demuxers := strings.Split(option(options, "-format_whitelist"), ",")
			for _, name := range following {
				if slices.Contains(demuxers, name) != (name == tc.demuxer) {
					t.Errorf("the demuxer %s among those given: %t", name, name != tc.demuxer)
				}
			}
			for _, name := range []string{"matroska", "webm", "mov", "mp4", "mpegts", "mpeg", "avi", "flv", "asf", "ogg", "wav", "mp3", "flac",
				"aac", "ac3", "eac3", "dts", "truehd", "h264", "hevc", "srt", "ass", "webvtt", "sup", "vobsub"} {
				if !slices.Contains(demuxers, name) {
					t.Errorf("the demuxer %s is not among those given", name)
				}
			}
		})
	}
}

// host is an HTTP server on the loopback interface that records what it
// is asked: the path of a request for itself, the whole address of one
// for another host, which a client asks its proxy, and which it refuses.
type host struct {
	*httptest.Server
	files map[string]string

	mu    sync.Mutex
	asked []string
}

func newHost(t *testing.T, files map[string]string) *host {
	t.Helper()
	h := &host{files: files}
	h.Server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		own := r.URL.Host == "" || r.URL.Host == h.Listener.Addr().String()
		h.mu.Lock()
		if own {
			h.asked = append(h.asked, r.URL.Path)
		} else {
			h.asked = append(h.asked, r.URL.String())
		}
		h.mu.Unlock()
		body, ok := h.files[r.URL.Path]
		switch {
		case !own:
			http.Error(w, "another host", http.StatusForbidden)
		case !ok:
			http.NotFound(w, r)
		default:
			if strings.HasPrefix(body, "#EXTM3U") {
				// FFmpeg takes a playlist for one by its type, or its name.
				w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			}
			_, _ = w.Write([]byte(body))
		}
	}))
	h.Start()
	t.Cleanup(h.Close)
	return h
}

func (h *host) requests() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.asked)
}

// testProber runs the ffprobe beside POLYFIN_TEST_FFMPEG.
func testProber(t *testing.T) Prober {
	t.Helper()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	return Prober{Path: filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), Timeout: 20 * time.Second}
}

// manifestOf is a DASH manifest of one video whose files are under base.
func manifestOf(base string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" profiles="urn:mpeg:dash:profile:isoff-live:2011" type="static" mediaPresentationDuration="PT2.0S" minBufferTime="PT2.0S">
 <Period id="0" start="PT0.0S">
  <AdaptationSet id="0" contentType="video">
   <Representation id="0" mimeType="video/mp4" codecs="avc1.64000b" bandwidth="100000" width="176" height="144">
    <BaseURL>` + base + `</BaseURL>
    <SegmentTemplate timescale="1000000" duration="2000000" initialization="init.m4s" media="chunk-$Number$.m4s" startNumber="1"/>
   </Representation>
  </AdaptationSet>
 </Period>
</MPD>
`
}

// The bytes of a file choose its demuxer among those that read the one
// file they are given: a file that is a playlist, a manifest or a list of
// files to join is not read as one, so that ffprobe asks for nothing it
// names, of the file's host or of another.
func TestTheBytesOfAFileChooseNoDemuxerThatOpensWhatTheyName(t *testing.T) {
	t.Parallel()
	prober := testProber(t)
	for _, tc := range []struct {
		name string
		file func(other string) string
	}{
		{"an HLS playlist", func(other string) string {
			return "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n#EXT-X-MEDIA-SEQUENCE:0\n" +
				"#EXT-X-KEY:METHOD=AES-128,URI=\"" + other + "/key\"\n#EXTINF:1.000,\n" + other + "/s0.ts\n#EXTINF:1.000,\ns1.ts\n#EXT-X-ENDLIST\n"
		}},
		{"a DASH manifest", func(other string) string { return manifestOf(other + "/nvr/") }},
		{"a DASH manifest of its own host", func(string) string { return manifestOf("nvr/") }},
		{"a list of files to join", func(string) string { return "ffconcat version 1.0\nfile 's0.ts'\nfile 's1.ts'\n" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			other := newHost(t, nil)
			own := newHost(t, map[string]string{"/movie.mkv": tc.file(other.URL)})
			_, err := prober.Probe(t.Context(), own.URL+"/movie.mkv")
			t.Logf("ffprobe: %v", err)
			if err == nil {
				t.Errorf("the file was analyzed")
			}
			if asked := other.requests(); len(asked) > 0 {
				t.Errorf("another host was asked for %q", asked)
			}
			asked := own.requests()
			if len(asked) == 0 || slices.ContainsFunc(asked, func(target string) bool { return target != "/movie.mkv" }) {
				t.Errorf("the file's host was asked for %q, want the file alone", asked)
			}
		})
	}
}

// What a playlist or a manifest names on another host than the loopback
// interface, an address its rewriting would have left, is asked of the
// interface, FFmpeg's proxy, which refuses it: the host itself is asked
// nothing.
//
//nolint:paralleltest,tparallel // sets no_proxy, for whose hosts FFmpeg skips its proxy
func TestAnAddressLeftInAPlaylistIsAskedOfTheLoopbackInterface(t *testing.T) {
	t.Setenv("no_proxy", "")
	prober := testProber(t)
	dash := false
	if out, err := exec.Command(prober.Path, "-hide_banner", "-h", "demuxer=dash").Output(); err == nil {
		dash = strings.Contains(string(out), "Demuxer dash")
	}
	for _, tc := range []struct {
		name    string
		reading Reading
		file    func(other string) string
		named   string
	}{
		{"a segment", Playlist, func(other string) string {
			return "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n#EXT-X-MEDIA-SEQUENCE:0\n#EXTINF:1.000,\n" + other + "/s0.ts\n#EXT-X-ENDLIST\n"
		}, "/s0.ts"},
		{"a key", Playlist, func(other string) string {
			return "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n#EXT-X-MEDIA-SEQUENCE:0\n" +
				"#EXT-X-KEY:METHOD=AES-128,URI=\"" + other + "/key\"\n#EXTINF:1.000,\ns0.ts\n#EXT-X-ENDLIST\n"
		}, "/key"},
		{"a manifest's file", Manifest, func(other string) string { return manifestOf(other + "/nvr/") }, "/nvr/init.m4s"},
		// Where FFmpeg's dash demuxer reads an address a manifest's
		// rewriting must know of: a template's, in the attributes of a
		// segment list, and one whose scheme its representation's
		// identifier spells, filled in once the address is resolved.
		{"a template's address in a segment list", Manifest, func(other string) string {
			return strings.NewReplacer(`<Representation `, `<SegmentList initialization="`+other+`/nvr/init.m4s" media="`+other+`/nvr/chunk-$Number$.m4s"/>
   <Representation `, ` initialization="init.m4s" media="chunk-$Number$.m4s"`, ``).Replace(manifestOf("nvr/"))
		}, "/nvr/init.m4s"},
		{"a template whose scheme an identifier spells", Manifest, func(other string) string {
			return strings.NewReplacer(`id="0" mimeType`, `id="http" mimeType`,
				`initialization="init.m4s"`, `initialization="$RepresentationID$://`+strings.TrimPrefix(other, "http://")+`/nvr/init.m4s"`).Replace(manifestOf("nvr/"))
		}, "/nvr/init.m4s"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.reading == Manifest && !dash {
				t.Skip("the ffprobe of POLYFIN_TEST_FFMPEG has no DASH demuxer")
			}
			other := newHost(t, nil)
			relay := newHost(t, map[string]string{"/live/stream": tc.file(other.URL)})
			reading := prober
			reading.Reads = tc.reading
			_, err := reading.ProbeLive(t.Context(), relay.URL+"/live/stream")
			t.Logf("ffprobe: %v", err)
			if asked := other.requests(); len(asked) > 0 {
				t.Errorf("the other host was asked for %q", asked)
			}
			if asked := relay.requests(); !slices.Contains(asked, other.URL+tc.named) {
				t.Errorf("the loopback interface was asked for %q, want %s among them", asked, other.URL+tc.named)
			}
		})
	}
}
