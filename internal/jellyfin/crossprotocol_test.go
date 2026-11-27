package jellyfin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/source"
	"github.com/moodiness/polyfin/internal/stremio"
)

// dalvikAgent is the User-Agent of Android's HttpURLConnection, which the
// players on Android's default HTTP stack send, with no Authorization.
const dalvikAgent = "Dalvik/2.1.0 (Linux; U; Android 12; Pixel Build/SP1A.210812.016)"

// androidTVClient is the Authorization header of an Android TV Jellyfin
// client whose video player is on Android's default HTTP stack.
func androidTVClient(token string) string {
	header := `MediaBrowser Client="Android TV client", Device="Pixel Tablet", DeviceId="tv-1536", Version="1.1.2"`
	if token != "" {
		header += `, Token="` + token + `"`
	}
	return header
}

// securelyReached serves the https requests for publicSource from the
// TLS server secure, with a client that trusts it.
type securelyReached struct {
	source.Opener
	secure *httptest.Server
}

func (o securelyReached) Open(ctx context.Context, method, target string, header http.Header, confined bool) (*http.Response, error) {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "https" || parsed.Host != publicSource {
		return o.Opener.Open(ctx, method, target, header, confined)
	}
	parsed.Host = strings.TrimPrefix(o.secure.URL, "https://")
	request, err := http.NewRequestWithContext(ctx, method, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header = header.Clone()
	return o.secure.Client().Do(request)
}

// httpsSourceServer is a server whose one addon, the server's, lists one
// movie streamed from https://publicSource, served from a TLS server with
// file. It returns the server and the stream's address.
func httpsSourceServer(t *testing.T, file string) (s testServer, stream string) {
	t.Helper()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "", time.Time{}, strings.NewReader(file))
	}))
	t.Cleanup(secure.Close)
	stream = "https://" + publicSource + "/files/secure.mp4"
	meta := stremio.Meta{ID: "tt1536", Type: "movie", Name: "Secure", Runtime: "2h", Year: "2010", Released: "2010-01-01T00:00:00.000Z"}
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "secure", Name: "Secure", Version: "1", Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{meta}})
		case path == "/meta/movie/tt1536.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
		case path == "/stream/movie/tt1536.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{
				{Name: "Secure", URL: stream, BehaviorHints: stremio.StreamBehavior{Filename: "Secure.mp4"}},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(addon.Close)
	s = newServerReaching(t, 10, "ffprobe-not-installed", func(o source.Opener) source.Opener { return securelyReached{Opener: o, secure: secure} })
	if _, err := s.addons.Install(t.Context(), addons.Shared(), addon.URL+"/manifest.json", false); err != nil {
		t.Fatal(err)
	}
	s.setting(t, func(settings *accounts.Settings) { settings.SyncAddonSubtitles = false })
	return s, stream
}

// Players on Android's default HTTP stack never follow a redirect from
// http to https (#1536): an Android TV Jellyfin client's video player
// reaching Polyfin over http gets an https source through Polyfin, where
// browsers, and every player reaching Polyfin over https, are still sent
// to the source.
func TestPlayersThatRefuseCrossProtocolRedirectsGetTheSourceThroughPolyfin(t *testing.T) {
	t.Parallel()
	const file = "\x00\x00\x00\x18ftypmp42 media bytes of the secure source"
	s, stream := httpsSourceServer(t, file)
	// The administrator, owner of the server's addon, is sent to its
	// source by default.
	administrator := s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	token := s.signInAs(t, "admin", androidTVClient(""))
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	if len(page.Items) != 1 {
		t.Fatalf("items: %+v", page.Items)
	}
	movie := page.Items[0].Id
	s.listed(t, administrator, movie)
	id, _ := accounts.ParseID(movie)
	versions, err := s.library.Versions(t.Context(), administrator, id)
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	playbackSetup{testServer: s, user: administrator}.analyzed(t, versions[0], "h264-aac-mp4")

	answer := s.androidPlaybackInfo(t, androidTVClient(token), movie, "", "android-tv-exoplayer", 0)
	path := answer.MediaSources[0].Path
	if !answer.MediaSources[0].IsRemote || !strings.HasPrefix(path, s.url+"/Videos/") {
		t.Fatalf("the source: %+v", answer.MediaSources[0])
	}
	ask := func(header http.Header) (*http.Response, string) {
		t.Helper()
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		for name, values := range header {
			request.Header[name] = values
		}
		response, err := noRedirects.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }()
		body, _ := io.ReadAll(response.Body)
		return response, string(body)
	}
	// The video player: relayed.
	for _, agent := range []string{dalvikAgent, "Java/17.0.2"} {
		response, body := ask(http.Header{"User-Agent": {agent}})
		if (response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent) || body != file {
			t.Errorf("%s over http: %d to %q, %d bytes, want the source relayed", agent, response.StatusCode, response.Header.Get("Location"), len(body))
		}
	}
	// A browser, and the player over https: sent to the source.
	for name, header := range map[string]http.Header{
		"a browser":                 {"User-Agent": {"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0 Safari/537.36"}},
		"an OkHttp player":          {"User-Agent": {"okhttp/4.12.0"}},
		"the video player on https": {"User-Agent": {dalvikAgent}, "X-Forwarded-Proto": {"https"}},
	} {
		if response, _ := ask(header); response.StatusCode != http.StatusFound || response.Header.Get("Location") != stream {
			t.Errorf("%s: %d to %q, want a 302 to the source", name, response.StatusCode, response.Header.Get("Location"))
		}
	}
}

// The rule that decides the relay of the video, live channel and audio
// stream routes alike (see mustRelay): an https source is relayed over
// http to a player that refuses redirects across protocols, known by its
// client name or by its request's User-Agent, and to no other.
func TestMustRelayOnlyPlayersThatRefuseCrossProtocolRedirects(t *testing.T) {
	t.Parallel()
	secure, plain := library.Version{URL: "https://" + publicSource + "/a.mp4"}, library.Version{URL: "http://" + publicSource + "/a.mp4"}
	for _, c := range []struct {
		name, client, agent, proto string
		version                    library.Version
		want                       bool
	}{
		{"Android's default HTTP stack", "", dalvikAgent, "", secure, true},
		{"the JDK's", "", "Java/17.0.2", "", secure, true},
		{"Findroid", "Findroid", "okhttp/4.12.0", "", secure, true},
		{"an http source", "", dalvikAgent, "", plain, false},
		{"over https", "", dalvikAgent, "https", secure, false},
		{"Findroid over https", "Findroid", "", "https", secure, false},
		{"a browser", "Jellyfin Web", "Mozilla/5.0 (X11; Linux x86_64) Chrome/131.0", "", secure, false},
		{"an OkHttp player", "Android TV client", "okhttp/4.12.0", "", secure, false},
	} {
		request := httptest.NewRequest(http.MethodGet, "http://polyfin.test/Videos/x/stream", nil)
		request.Header.Set("User-Agent", c.agent)
		if c.proto != "" {
			request.Header.Set("X-Forwarded-Proto", c.proto)
		}
		request = request.WithContext(context.WithValue(request.Context(), callerKey{}, caller{Device: accounts.Device{Client: c.client}}))
		if got := mustRelay(request, c.version); got != c.want {
			t.Errorf("%s: relayed %v, want %v", c.name, got, c.want)
		}
	}
}
