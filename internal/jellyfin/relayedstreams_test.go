package jellyfin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/source"
	"github.com/moodiness/polyfin/internal/stremio"
)

// pageFile is a web page holding a script, which a source answers where
// media is expected.
const pageFile = `<!doctype html><html><body><script>document.title = 'ran'</script></body></html>`

// A file a relayed playlist names is relayed with the type its source
// gave it only when that names media, else as bytes, and sandboxed; a web
// page, by its type or by its bytes, is not relayed at all: the player
// receives a 502, and the answer is told for the log.
func TestFilesOfARelayedPlaylistKeepOnlyAMediaType(t *testing.T) {
	t.Parallel()
	const segment = "G\x40\x11\x10\x00\x42\xf0\x25\x00\x01\xc1\x00\x00\xff\x01\xff"
	files := map[string]struct {
		contentType, body string
		// want is the type relayed, empty for a file that is not.
		want string
	}{
		"/hls/seg.ts":        {"video/mp2t", segment, "video/mp2t"},
		"/hls/seg.aac":       {"audio/aac", "\xff\xf1\x50\x80\x01\x3f\xfc", "audio/aac"},
		"/hls/init.mp4":      {"application/mp4", "\x00\x00\x00\x1cftypiso5\x00\x00\x00\x00", "application/mp4"},
		"/hls/subs.vtt":      {"text/vtt; charset=utf-8", "WEBVTT\n\n00:00.000 --> 00:02.000\nHello\n", "text/vtt; charset=utf-8"},
		"/hls/untyped.ts":    {"", segment, "application/octet-stream"},
		"/hls/bytes.ts":      {"application/octet-stream", segment, "application/octet-stream"},
		"/hls/odd.ts":        {"text/html; charset=utf-8", segment, "application/octet-stream"},
		"/hls/script.ts":     {"text/javascript", segment, "application/octet-stream"},
		"/hls/drawing.ts":    {"image/svg+xml", segment, "application/octet-stream"},
		"/hls/key.bin":       {"text/plain", "0123456789abcdef", "application/octet-stream"},
		"/hls/page.html":     {"text/html; charset=utf-8", pageFile, ""},
		"/hls/page.ts":       {"video/mp2t", pageFile, ""},
		"/hls/untyped.html":  {"", pageFile, ""},
		"/hls/text.html":     {"text/html", "document.title = 'ran'", ""},
		"/hls/fragment.html": {"application/octet-stream", "<script>document.title = 'ran'</script>", ""},
	}
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		// Set even when empty: the server then names no type at all.
		w.Header()["Content-Type"] = nil
		if file.contentType != "" {
			w.Header().Set("Content-Type", file.contentType)
		}
		_, _ = io.WriteString(w, file.body)
	}))
	t.Cleanup(host.Close)
	s := newTestServer(t, 10)
	version := library.Version{ID: accounts.ID{0xa1}, URL: host.URL + "/hls/index.m3u8"}
	for path, file := range files {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/Videos/0123456789abcdef0123456789abcdef/live/file.ts", nil)
		err := s.handler.Playback.ServeLive(recorder, request, version, host.URL+path, func(target, _ string) string { return target })
		header := recorder.Header()
		if !sandboxed(header) {
			t.Errorf("%s: the answer is not sandboxed: %q", path, header.Get("Content-Security-Policy"))
		}
		if file.want == "" {
			if recorder.Code != http.StatusBadGateway || strings.Contains(recorder.Body.String(), "script") || strings.Contains(recorder.Body.String(), "ran") ||
				strings.HasPrefix(header.Get("Content-Type"), "text/html") {
				t.Errorf("%s: %d %q %q, want a 502", path, recorder.Code, header.Get("Content-Type"), recorder.Body.String())
			}
			if err == nil {
				t.Errorf("%s: a page was not told", path)
			}
			continue
		}
		if recorder.Code != http.StatusOK || header.Get("Content-Type") != file.want || recorder.Body.String() != file.body {
			t.Errorf("%s: %d %q, %d bytes (%v), want %q", path, recorder.Code, header.Get("Content-Type"), recorder.Body.Len(), err, file.want)
		}
	}
}

// The link a relayed playlist gives for a file that is a web page, which
// carries its own grant, answers a 502 to whoever opens it, never the
// page; a segment beside it is relayed.
func TestAPageARelayedPlaylistNamesIsNotRelayed(t *testing.T) {
	t.Parallel()
	const segment = "G\x40\x11\x10\x00\x42\xf0\x25\x00\x01\xc1\x00\x00\xff\x01\xff"
	dir := t.TempDir()
	for name, content := range map[string]string{
		"one.m3u8":  "#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:7\n#EXTINF:2,\nseg7.html\n#EXTINF:2,\nseg8.ts\n",
		"seg7.html": pageFile,
		"seg8.ts":   segment,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s, token, user := tuned(t, newTVAddon(t, false, dir))
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	channel := channels.Items[0].Id
	liveAnalysis(t, s, user, channel)
	opened := livePlaybackInfo(t, s, token, channel, "jellyfin-web-chrome")
	_, playlist := fetchURL(t, opened.Path, nil)
	var links []string
	for line := range strings.Lines(playlist) {
		if strings.HasPrefix(line, "http") {
			links = append(links, strings.TrimSpace(line))
		}
	}
	if len(links) != 2 || !strings.Contains(links[0], "/Videos/"+channel+"/live/") {
		t.Fatalf("the relayed playlist: %s", playlist)
	}
	// Opened with no credentials but the link's own, as a browser given
	// the link would, as a page.
	navigation := http.Header{"Sec-Fetch-Mode": {"navigate"}, "Sec-Fetch-Dest": {"document"}, "Accept": {"text/html"}}
	for _, header := range []http.Header{nil, navigation} {
		response, body := fetchURL(t, links[0], header)
		if response.StatusCode != http.StatusBadGateway || strings.Contains(body, "script") || strings.HasPrefix(response.Header.Get("Content-Type"), "text/html") {
			t.Errorf("the page: %d %q %q, want a 502", response.StatusCode, response.Header.Get("Content-Type"), body)
		}
		response, body = fetchURL(t, links[1], header)
		if contentType := response.Header.Get("Content-Type"); response.StatusCode != http.StatusOK || body != segment ||
			contentType != "application/octet-stream" && !strings.HasPrefix(contentType, "video/") || !sandboxed(response.Header) {
			t.Errorf("the segment: %d %q %q, %d bytes", response.StatusCode, contentType, response.Header.Get("Content-Security-Policy"), len(body))
		}
	}
}

// pageStreamAddon lists one movie whose one stream answers a web page,
// asked with a header of the addon's, so that Polyfin relays it.
func pageStreamAddon(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	movie := stremio.Meta{ID: "page:1", Type: "movie", Name: "Page"}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "page", Name: "Page", Version: "1", Types: []string{"movie"},
				IDPrefixes: []string{"page:"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case strings.HasPrefix(path, "/meta/movie/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case strings.HasPrefix(path, "/stream/movie/"):
			stream := stremio.Stream{Name: "Page", URL: server.URL + "/play"}
			stream.BehaviorHints.NotWebReady = true
			stream.BehaviorHints.ProxyHeaders = &stremio.ProxyHeaders{Request: map[string]string{"X-Asked": "1"}}
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{stream}})
		case path == "/play":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, pageFile)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// A relayed stream is named a media type whatever its source says: the
// stream route names its own, and a relay that names none keeps the
// source's only when it names media. A web page a source answers there
// is so never a page of Polyfin's, and the answer is sandboxed.
func TestARelayedStreamKeepsAMediaType(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("member", nil)
	token := s.signIn("member", "tv")
	// The page is relayed as its source wrote it: with Send apps to the
	// source at owners only, a member is refused a stream that starts as
	// text, which may be a playlist naming its source.
	s.setting(t, func(settings *accounts.Settings) { settings.SendToSource = accounts.SendToSourceEveryone })
	host := pageStreamAddon(t)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), host+"/manifest.json", false); err != nil {
		t.Fatal(err)
	}
	var page QueryResult
	if status := s.get(t, "/Items?ParentId="+s.viewIDs(t, token)["Top"], token, &page); status != http.StatusOK || len(page.Items) != 1 {
		t.Fatalf("the library: %d %+v", status, page.Items)
	}
	response, body := fetchURL(t, s.url+"/Videos/"+page.Items[0].Id+"/stream?api_key="+token, nil)
	if response.StatusCode != http.StatusOK || body != pageFile || response.Header.Get("Content-Type") != "video/x-matroska" {
		t.Errorf("the stream: %d %q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	if !sandboxed(response.Header) {
		t.Errorf("the stream is not sandboxed: %q", response.Header.Get("Content-Security-Policy"))
	}

	// A relay that names no type, as the audio route's without an
	// extension.
	version := library.Version{ID: accounts.ID{0xa2}, URL: host + "/play"}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/Audio/0123456789abcdef0123456789abcdef/stream", nil)
	if err := s.handler.Playback.Serve(recorder, request, version, playback.Delivery{Relay: true}); err != nil {
		t.Fatal(err)
	}
	if got := recorder.Header().Get("Content-Type"); recorder.Code != http.StatusOK || got != "application/octet-stream" || !sandboxed(recorder.Header()) {
		t.Errorf("a relay that names no type: %d %q %q", recorder.Code, got, recorder.Header().Get("Content-Security-Policy"))
	}
}

// A browser that opens a stream's address as a page is not sent to the
// address the source chose: the bytes are relayed, with a media type,
// sandboxed. A player, which sends no such headers, is still sent to a
// source it can fetch itself.
func TestANavigationIsNotRedirectedToASource(t *testing.T) {
	t.Parallel()
	const file = "\x1a\x45\xdf\xa3 media bytes"
	manifest, host := directPlayAddon(t, file)
	s := newServerReaching(t, 10, "ffprobe-not-installed", func(o source.Opener) source.Opener { return reachedAt{Opener: o, local: host} })
	member := s.user("member", nil)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), manifest, false); err != nil {
		t.Fatal(err)
	}
	// A member is sent to a server addon's source only with Send apps to
	// the source at everyone.
	s.setting(t, func(settings *accounts.Settings) {
		settings.SyncAddonSubtitles = false
		settings.SendToSource = accounts.SendToSourceEveryone
	})
	token := s.signIn("member", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	movie := page.Items[0].Id
	s.listed(t, member, movie)

	// The address of the stream on the public host, as item details give
	// it: Polyfin's own, with a grant, no token.
	var details BaseItemDto
	if status := s.get(t, "/Users/"+member.ID.String()+"/Items/"+movie, token, &details); status != http.StatusOK || details.MediaSources == nil {
		t.Fatalf("details: %d %+v", status, details.MediaSources)
	}
	var link string
	for _, listed := range *details.MediaSources {
		if listed.Name == "Redirected" {
			link = listed.Path
		}
	}
	parsed, err := url.Parse(link)
	if err != nil || !strings.HasPrefix(link, s.url+"/Videos/") || parsed.Query().Get(grantParameter) == "" || strings.Contains(link, token) {
		t.Fatalf("the address in the item's details: %q", link)
	}
	ask := func(header http.Header) (*http.Response, string) {
		t.Helper()
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, link, nil)
		for name, values := range header {
			request.Header[name] = values
		}
		response, err := noRedirects.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		return response, string(body)
	}
	source := "http://" + publicSource + "/files/redirected.mp4"
	// A player.
	if response, _ := ask(nil); response.StatusCode != http.StatusFound || response.Header.Get("Location") != source {
		t.Errorf("a player: %d to %q, want a 302 to the source", response.StatusCode, response.Header.Get("Location"))
	}
	// A <video> element of a page.
	if response, _ := ask(http.Header{"Sec-Fetch-Mode": {"no-cors"}, "Sec-Fetch-Dest": {"video"}, "Sec-Fetch-Site": {"same-origin"}}); response.StatusCode != http.StatusFound {
		t.Errorf("a video element: %d, want a 302 to the source", response.StatusCode)
	}
	// A browser that opens the address: a link followed, an address
	// typed, a frame.
	for name, header := range map[string]http.Header{
		"a link followed":          {"Sec-Fetch-Mode": {"navigate"}, "Sec-Fetch-Dest": {"document"}, "Sec-Fetch-Site": {"cross-site"}, "Accept": {"text/html,application/xhtml+xml"}},
		"a frame":                  {"Sec-Fetch-Mode": {"navigate"}, "Sec-Fetch-Dest": {"iframe"}, "Sec-Fetch-Site": {"cross-site"}},
		"a document, by its place": {"Sec-Fetch-Dest": {"document"}},
	} {
		response, body := ask(header)
		if response.StatusCode/100 == 3 || response.Header.Get("Location") != "" {
			t.Errorf("%s: %d to %q, want no redirect", name, response.StatusCode, response.Header.Get("Location"))
			continue
		}
		if response.StatusCode != http.StatusOK || body != file || response.Header.Get("Content-Type") != "video/mp4" || !sandboxed(response.Header) {
			t.Errorf("%s: %d %q %q, %d bytes, want the file relayed", name, response.StatusCode, response.Header.Get("Content-Type"),
				response.Header.Get("Content-Security-Policy"), len(body))
		}
	}
}
