package jellyfin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
)

// A channel whose every stream names a DRM Polyfin does not support, in
// any of the properties lists write, leaves the line-up: apps neither
// list it, nor show its programmes in the guide, nor play it, and its
// source is never asked. A merged channel with a stream Polyfin can play
// stays listed.
func TestChannelsNeedingAnUnsupportedDRMLeaveTheLineup(t *testing.T) {
	t.Parallel()
	var asked atomic.Int32
	now := time.Now().UTC()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/get.php":
			_, _ = fmt.Fprintf(w, "#EXTM3U\n"+
				"#EXTINF:-1 tvg-id=\"zeb.zz\" tvg-chno=\"1\",Zeb One\n%[1]s/live/zeb.m3u8\n"+
				"#EXTINF:-1 tvg-id=\"moss.zz\" tvg-chno=\"2\",Moss One\n"+
				"#KODIPROP:inputstream.adaptive.drm_legacy=com.widevine.alpha|%[1]s/licence\n%[1]s/drm/moss.mpd\n"+
				"#EXTINF:-1 tvg-id=\"reed.zz\" tvg-chno=\"3\",Reed One\n"+
				"#KODIPROP:inputstream.adaptive.drm={\"com.microsoft.playready\":{}}\n%[1]s/drm/reed.mpd\n"+
				"#EXTINF:-1 tvg-id=\"kite.zz\" tvg-chno=\"4\",Kite One\n"+
				"#KODIPROP:inputstream.adaptive.license_type=com.widevine.alpha\n%[1]s/drm/kite.mpd\n"+
				"#EXTINF:-1 tvg-id=\"fern.zz\" tvg-chno=\"5\",Fern One FHD\n"+
				"#KODIPROP:inputstream.adaptive.drm_legacy=com.widevine.alpha\n%[1]s/drm/fern.mpd\n"+
				"#EXTINF:-1 tvg-id=\"fern.zz\" tvg-chno=\"5\",Fern One HD\n%[1]s/live/fern.m3u8\n", server.URL)
		case "/epg.xml":
			var b strings.Builder
			b.WriteString("<tv>")
			for _, name := range []string{"zeb", "moss", "reed", "kite", "fern"} {
				fmt.Fprintf(&b, `<channel id="%[1]s.zz"><display-name>%[1]s</display-name></channel>`, name)
				fmt.Fprintf(&b, `<programme channel="%s.zz" start="%s" stop="%s"><title>%s Tonight</title></programme>`, name,
					now.Add(-time.Hour).Format("20060102150405 -0700"), now.Add(time.Hour).Format("20060102150405 -0700"), name)
			}
			b.WriteString("</tv>")
			_, _ = fmt.Fprint(w, b.String())
		default:
			if strings.HasPrefix(r.URL.Path, "/drm/") || r.URL.Path == "/licence" {
				asked.Add(1)
			}
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	s := newTestServer(t, 10)
	s.user("member", nil)
	token := s.signIn("member", "tv")
	merged := iptv.ChannelsMerged
	addon, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Provider", Guide: server.URL + "/epg.xml",
		Account: iptv.Account{Kind: addons.KindM3U, URL: server.URL + "/get.php"}, Options: &iptv.OptionsPatch{Channels: &merged}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.library.RefreshGuide(t.Context(), addons.Shared(), addons.LibraryKey{AddonID: addon.ID, CatalogType: "tv", CatalogID: "channels"}); err != nil {
		t.Fatal(err)
	}

	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	var names []string
	for _, c := range channels.Items {
		names = append(names, c.Name)
	}
	if !slices.Equal(names, []string{"Zeb One", "Fern One"}) {
		t.Errorf("channels listed: %v", names)
	}
	programmes := guidePrograms(t, s, token)
	for name, want := range map[string]bool{"zeb Tonight": true, "fern Tonight": true, "moss Tonight": false, "reed Tonight": false, "kite Tonight": false} {
		if _, ok := programmes[name]; ok != want {
			t.Errorf("the guide shows %q: %v", name, ok)
		}
	}

	_, lineup, err := s.iptv.ListChannels(t.Context(), addons.Shared(), addon.ID, iptv.ChannelFilter{})
	if err != nil || len(lineup) != 5 {
		t.Fatalf("the line-up: %+v %v", lineup, err)
	}
	for _, c := range lineup {
		if c.Name != "Moss One" && c.Name != "Reed One" && c.Name != "Kite One" {
			continue
		}
		if status, body := s.call(http.MethodPost, "/Items/"+c.ID.String()+"/PlaybackInfo", app("tv", token), map[string]any{}); status != http.StatusNotFound {
			t.Errorf("PlaybackInfo of %s: %d %s", c.Name, status, body)
		}
	}
	if n := asked.Load(); n != 0 {
		t.Errorf("the sources of streams needing a DRM were asked %d times", n)
	}
}
