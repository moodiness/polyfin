package playback

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// vodOrigin serves the files of a finished HLS stream by path, byte
// ranges included, to requests that carry its Referer only, and records
// each request.
type vodOrigin struct {
	files map[string]string

	mu       sync.Mutex
	requests []string
	refused  int
}

func (o *vodOrigin) Open(ctx context.Context, method, target string, header http.Header, _ bool) (*http.Response, error) {
	address, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	o.mu.Lock()
	o.requests = append(o.requests, address.Path+" "+header.Get("Range"))
	refused := header.Get("Referer") != "https://site.example/"
	if refused {
		o.refused++
	}
	o.mu.Unlock()
	body, ok := o.files[address.Path]
	switch {
	case refused:
		return sized(http.StatusForbidden, "text/html", []byte("<html>forbidden</html>")), nil
	case !ok:
		return sized(http.StatusNotFound, "text/plain", []byte("no such file")), nil
	}
	request := httptest.NewRequestWithContext(ctx, method, target, nil)
	request.Header = header.Clone()
	recorder := httptest.NewRecorder()
	if strings.HasSuffix(address.Path, ".m3u8") {
		recorder.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	}
	http.ServeContent(recorder, request, "", time.Time{}, strings.NewReader(body))
	response := recorder.Result()
	response.Request = request
	return response, nil
}

// A finished HLS stream an addon gives is read through Polyfin as a whole
// (#1197): its master playlist, served on the loopback interface to
// ffprobe and FFmpeg, and relayed to a player, names a variant's playlist,
// an audio rendition's, and through them a key, an initialization section
// and segments, each read as EXT-X-BYTERANGE asks, all on Polyfin's
// addresses, fetched from the source with the header it needs. The tags
// stay as they were: the playlists still end.
func TestFinishedPlaylistsNameEveryFileThroughPolyfin(t *testing.T) {
	t.Parallel()
	rendition := "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-TARGETDURATION:2\n" +
		"#EXT-X-KEY:METHOD=AES-128,URI=\"../keys/k.bin\",IV=0x1\n#EXT-X-MAP:URI=\"init.mp4\",BYTERANGE=\"4@0\"\n" +
		"#EXTINF:2.000,\n#EXT-X-BYTERANGE:6@4\nmedia.mp4\n#EXTINF:2.000,\n#EXT-X-BYTERANGE:6@10\nmedia.mp4\n#EXT-X-ENDLIST\n"
	origin := &vodOrigin{files: map[string]string{
		"/vod/master.m3u8": "#EXTM3U\n#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"a\",NAME=\"English\",URI=\"audio/index.m3u8\"\n" +
			"#EXT-X-STREAM-INF:BANDWIDTH=800000,AUDIO=\"a\"\nhttps://cdn.example/vod/video/index.m3u8\n",
		"/vod/video/index.m3u8": rendition,
		"/vod/audio/index.m3u8": rendition,
		"/vod/keys/k.bin":       "0123456789abcdef",
		"/vod/video/init.mp4":   "INIT",
		"/vod/video/media.mp4":  "INITfirst-second",
		"/vod/audio/init.mp4":   "init",
		"/vod/audio/media.mp4":  "initFIRST-SECOND",
	}}
	s := newService(t, origin, "ffprobe-not-installed", nil)
	version := library.Version{ID: accounts.ID{11}, URL: "https://cdn.example/vod/master.m3u8", Headers: map[string]string{"Referer": "https://site.example/"}}
	// get fetches a playlist or a file as FFmpeg or a player does: the
	// range EXT-X-BYTERANGE gives, when given.
	client := &http.Client{}
	get := func(address, asked string) (int, string) {
		t.Helper()
		var response *http.Response
		if strings.HasPrefix(address, "http://") {
			request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, address, nil)
			if asked != "" {
				request.Header.Set("Range", asked)
			}
			var err error
			if response, err = client.Do(request); err != nil {
				t.Fatal(err)
			}
		} else {
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, address, nil)
			if asked != "" {
				request.Header.Set("Range", asked)
			}
			recorder := httptest.NewRecorder()
			s.relayedFile(recorder, request, version, address)
			response = recorder.Result()
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		return response.StatusCode, string(body)
	}
	// read reads a playlist and every file it names, as FFmpeg's HLS
	// demuxer does, and returns what each file gave, by the tag or line
	// naming it.
	var read func(t *testing.T, playlist, prefix string) []string
	read = func(t *testing.T, playlist, prefix string) []string {
		t.Helper()
		var files []string
		asked := ""
		for line := range strings.Lines(playlist) {
			line = strings.TrimSpace(line)
			uri := line
			switch {
			case strings.HasPrefix(line, "#EXT-X-BYTERANGE:"):
				length, offset, _ := strings.Cut(strings.TrimPrefix(line, "#EXT-X-BYTERANGE:"), "@")
				asked = byteRange(t, length, offset)
				continue
			case strings.HasPrefix(line, "#"):
				_, quoted, ok := strings.Cut(line, `URI="`)
				if !ok {
					continue
				}
				uri, _, _ = strings.Cut(quoted, `"`)
				if _, attribute, ok := strings.Cut(line, `BYTERANGE="`); ok {
					length, offset, _ := strings.Cut(strings.TrimSuffix(attribute, `"`), "@")
					asked = byteRange(t, length, offset)
				}
			case line == "":
				continue
			}
			if !strings.HasPrefix(uri, prefix) {
				t.Fatalf("an address that is not Polyfin's: %q", line)
			}
			status, body := get(uri, asked)
			asked = ""
			if status != http.StatusOK && status != http.StatusPartialContent {
				t.Fatalf("%s: %d %s", uri, status, body)
			}
			if strings.HasPrefix(body, "#EXTM3U") {
				if strings.Contains(body, "#EXTINF") && !strings.Contains(body, "#EXT-X-ENDLIST\n") {
					t.Errorf("a media playlist that no longer ends: %s", body)
				}
				files = append(files, read(t, body, prefix)...)
				continue
			}
			files = append(files, body)
		}
		return files
	}
	// The audio rendition comes first, as the master playlist names it.
	want := []string{"0123456789abcdef", "init", "FIRST-", "SECOND", "0123456789abcdef", "INIT", "first-", "second"}

	// FFmpeg and ffprobe read it on the loopback interface.
	src := s.open(version)
	defer src.Release()
	target, release := s.loopback.register(version, src)
	defer release()
	status, master := get(target, "bytes=0-")
	if status != http.StatusOK || !strings.HasPrefix(master, "#EXTM3U") {
		t.Fatalf("the master playlist on the loopback interface: %d %s", status, master)
	}
	loopback := "http://" + s.loopback.listener.Addr().String() + "/live/"
	if got := read(t, master, loopback); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("read on the loopback interface: %q, want %q", got, want)
	}

	// A player, which it is relayed to, reads it at the links it is given.
	s.analyses.Put(version.ID, media.Analysis{Format: "hls", Duration: 4 * time.Second})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/Videos/x/stream", nil)
	if err := s.Serve(recorder, request, version, Delivery{Relay: true, Link: relayedLink}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, recorder.Body.String(), "/relayed/"); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("relayed to a player: %q, want %q", got, want)
	}

	origin.mu.Lock()
	defer origin.mu.Unlock()
	if origin.refused != 0 {
		t.Errorf("%d requests without the source's header: %q", origin.refused, origin.requests)
	}
}

// byteRange is the Range header asking for length bytes from offset.
func byteRange(t *testing.T, length, offset string) string {
	t.Helper()
	n, err := strconv.Atoi(length)
	at, err2 := strconv.Atoi(offset)
	if err != nil || err2 != nil {
		t.Fatalf("byte range %s@%s", length, offset)
	}
	return "bytes=" + strconv.Itoa(at) + "-" + strconv.Itoa(at+n-1)
}

// relayedLink names a file a playlist relayed to a player names, as the
// player's server would.
func relayedLink(target, name string) string {
	return "/relayed/" + url.PathEscape(name) + "?target=" + url.QueryEscape(target)
}

// relayedFile serves a file relayedLink names, as the player's server
// does (see ServeLive).
func (s *Service) relayedFile(w http.ResponseWriter, r *http.Request, version library.Version, address string) {
	parsed, _ := url.Parse(address)
	_ = s.ServeLive(w, r, version, parsed.Query().Get("target"), relayedLink)
}
