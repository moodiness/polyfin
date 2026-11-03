package jellyfin

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
)

// proxyGuide serves an XMLTV guide of three channels, each airing a
// programme from an hour ago to an hour from now, the guide giving each an
// icon (an image at /icon/<id>.png of the same server).
func proxyGuide(w http.ResponseWriter, base string) {
	now := time.Now().UTC()
	from, to := now.Add(-time.Hour).Format("20060102150405 -0700"), now.Add(time.Hour).Format("20060102150405 -0700")
	_, _ = io.WriteString(w, "<tv>")
	for _, c := range []struct{ id, name string }{{"zeb.zz", "Zeb News"}, {"orb.zz", "Orb Sport"}, {"kids.zz", "Kids Box"}} {
		_, _ = fmt.Fprintf(w, `<channel id="%[1]s"><display-name>%[2]s</display-name><icon src="%[3]s/icon/%[1]s.png"/></channel>
			<programme channel="%[1]s" start="%[4]s" stop="%[5]s"><title>%[2]s Live</title></programme>`, c.id, c.name, base, from, to)
	}
	_, _ = io.WriteString(w, "</tv>")
}

// proxyServer serves what a proxy that manages channels serves: an M3U
// playlist numbering a channel 104.5, giving Kids Box no logo, and the
// guide of proxyGuide; and the icons, as PNG images.
func proxyServer(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/output/m3u":
			_, _ = fmt.Fprintf(w, "#EXTM3U\n"+
				"#EXTINF:-1 tvg-id=\"zeb.zz\" tvg-chno=\"104\" tvg-logo=\"%[1]s/logo/zeb.png\" group-title=\"News\",Zeb News\n%[1]s/stream/zeb\n"+
				"#EXTINF:-1 tvg-id=\"orb.zz\" tvg-chno=\"104.5\" tvg-logo=\"%[1]s/logo/orb.png\" group-title=\"Sports\",Orb Sport\n%[1]s/stream/orb\n"+
				"#EXTINF:-1 tvg-id=\"kids.zz\" tvg-chno=\"103\" group-title=\"Kids\",Kids Box\n%[1]s/stream/kids\n", server.URL)
		case r.URL.Path == "/output/epg":
			proxyGuide(w, server.URL)
		case strings.HasPrefix(r.URL.Path, "/icon/"), strings.HasPrefix(r.URL.Path, "/logo/"):
			w.Header().Set("Content-Type", "image/png")
			_, _ = io.WriteString(w, "\x89PNG\r\n\x1a\n"+r.URL.Path)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// addProxy adds the proxy's playlist and guide as an M3U source.
func addProxy(t *testing.T, s testServer, base string) addons.LibraryKey {
	t.Helper()
	addon, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Proxy", Guide: base + "/output/epg",
		Account: iptv.Account{Kind: addons.KindM3U, URL: base + "/output/m3u"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	key := addons.LibraryKey{AddonID: addon.ID, CatalogType: "tv", CatalogID: "channels"}
	if err := s.library.RefreshGuide(t.Context(), addons.Shared(), key); err != nil {
		t.Fatal(err)
	}
	return key
}

// A decimal number a playlist gives ("104.5") shows as written in apps,
// as Jellyfin's ChannelNumber does, and orders the channel after 104: in
// the guide, and among the programmes airing now.
func TestDecimalChannelNumbersShowInApps(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("member", nil)
	token := s.signIn("member", "tv")
	addProxy(t, s, proxyServer(t))

	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	numbers, names := map[string]string{}, map[string]string{}
	for _, c := range channels.Items {
		numbers[c.Name], names[c.Id] = c.Number+"/"+c.ChannelNumber, c.Name
	}
	if numbers["Orb Sport"] != "104.5/104.5" || numbers["Zeb News"] != "104/104" || numbers["Kids Box"] != "103/103" {
		t.Errorf("channel numbers: %v", numbers)
	}
	for _, path := range []string{"/LiveTv/Programs", "/LiveTv/Programs/Recommended?IsAiring=true"} {
		var programs QueryResult
		s.get(t, path, token, &programs)
		var order []string
		for _, p := range programs.Items {
			if p.ChannelId != nil {
				order = append(order, names[*p.ChannelId])
			}
		}
		if !slices.Equal(order, []string{"Kids Box", "Zeb News", "Orb Sport"}) {
			t.Errorf("%s: programmes by channel %q", path, order)
		}
	}
}

// PlaybackInfo of a channel whose source is slow to send its first bytes,
// as a proxy buffering the channel for its first viewer is, keeps the app
// waiting: it sends its answer's status, then spaces, which JSON allows
// before the answer, until the source answered.
func TestASlowChannelKeepsPlaybackInfoWaiting(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/output/m3u":
			_, _ = fmt.Fprintf(w, "#EXTM3U\n#EXTINF:-1 tvg-chno=\"101\",Cold Channel\nhttp://%s/stream/cold\n", r.Host)
		case "/stream/cold":
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			w.Header().Set("Content-Type", "video/mp2t")
			packet := make([]byte, 188)
			packet[0] = 0x47
			for range 64 {
				if _, err := w.Write(packet); err != nil {
					return
				}
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(source.Close)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	s := newTestServer(t, 10)
	s.user("member", nil)
	token := s.signIn("member", "tv")
	if _, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Proxy",
		Account: iptv.Account{Kind: addons.KindM3U, URL: source.URL + "/output/m3u"}}, false); err != nil {
		t.Fatal(err)
	}
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	if len(channels.Items) != 1 {
		t.Fatalf("channels: %+v", channels.Items)
	}
	profile, err := os.ReadFile(filepath.Join(playbackFixtures, "profiles", "swiftfin-native.json"))
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"DeviceProfile": json.RawMessage(profile), "IsPlayback": true, "AutoOpenLiveStream": true})
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, s.url+"/Items/"+channels.Items[0].Id+"/PlaybackInfo",
		strings.NewReader(string(payload)))
	request.Header.Set("Authorization", app("tv", token))
	request.Header.Set("Content-Type", "application/json")
	answered := make(chan *http.Response, 1)
	go func() {
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Error(err)
			close(answered)
			return
		}
		answered <- response
	}()
	// The source holds its first bytes: the answer's status and a space
	// come first.
	var response *http.Response
	select {
	case response = <-answered:
	case <-time.After(3 * playbackInfoPatience):
		t.Fatal("PlaybackInfo sent nothing while the source was silent")
	}
	if response == nil {
		t.FailNow()
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	first, err := reader.ReadByte()
	if response.StatusCode != http.StatusOK || err != nil || first != ' ' {
		t.Fatalf("PlaybackInfo's first byte: %d %q %v", response.StatusCode, first, err)
	}
	close(release)
	var answer map[string]json.RawMessage
	if err := json.NewDecoder(reader).Decode(&answer); err != nil {
		t.Fatalf("PlaybackInfo's answer after the spaces: %v", err)
	}
	if _, ok := answer["MediaSources"]; !ok {
		t.Errorf("PlaybackInfo's answer: %v", answer)
	}
}
