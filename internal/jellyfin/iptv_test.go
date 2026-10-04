package jellyfin

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
)

// iptvProvider serves an M3U playlist of two channels, the first of which
// streams a fixed HLS playlist only to the user agent its list names, and
// an XMLTV guide naming that channel by its tvg-id only.
type iptvProvider struct {
	url         string
	refusedUser atomic.Int32
}

func newIPTVProvider(t *testing.T) *iptvProvider {
	t.Helper()
	p := &iptvProvider{}
	var server *httptest.Server
	now := time.Now().UTC()
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/get.php":
			_, _ = fmt.Fprintf(w, "#EXTM3U\r\n"+
				"#EXTINF:-1 tvg-id=\"zeb.zz\" tvg-chno=\"5\" tvg-logo=\"%[1]s/zeb.png\" group-title=\"News\",ZZ| Zeb One\r\n"+
				"#EXTVLCOPT:http-user-agent=ZebPlayer/2\r\n"+
				"%[1]s/live/one.m3u8\r\n"+
				"#EXTINF:-1 group-title=\"=== KIDS ===\",##### KIDS #####\r\n%[1]s/live/heading.ts\r\n"+
				"#EXTINF:-1 tvg-logo=\"%[1]s/orbe.png\" group-title=\"Kids\",Orbe Junior\r\n%[1]s/live/two.ts\r\n", server.URL)
		case "/epg.xml":
			_, _ = fmt.Fprintf(w, `<tv><channel id="zeb.zz"><display-name>Unrelated Name</display-name></channel>
				<programme channel="zeb.zz" start="%s" stop="%s"><title>Zeb Tonight</title></programme></tv>`,
				now.Add(-time.Hour).Format("20060102150405 -0700"), now.Add(time.Hour).Format("20060102150405 -0700"))
		case "/live/one.m3u8", "/live/seg7.ts":
			if r.Header.Get("User-Agent") != "ZebPlayer/2" {
				p.refusedUser.Add(1)
				http.Error(w, "unknown player", http.StatusForbidden)
				return
			}
			if strings.HasSuffix(r.URL.Path, ".m3u8") {
				_, _ = io.WriteString(w, "#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:7\n#EXTINF:2,\nseg7.ts\n")
				return
			}
			http.ServeContent(w, r, "seg7.ts", time.Time{}, strings.NewReader("segment bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	p.url = server.URL
	return p
}

// An M3U source lists its channels in Jellyfin apps with their numbers,
// logos and groups, plays them through the live path with the headers its
// list gives, shows the groups chosen, and takes its guide by tvg-id.
func TestIPTVSourcesListAndPlayChannels(t *testing.T) {
	provider := newIPTVProvider(t)
	s := newTestServer(t, 10)
	user := s.user("member", nil)
	token := s.signIn("member", "tv")
	addon, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Provider", Guide: provider.url + "/epg.xml",
		Account: iptv.Account{Kind: addons.KindM3U, URL: provider.url + "/get.php?username=user&password=secret"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.library.RefreshGuide(t.Context(), addons.Shared(), addons.LibraryKey{AddonID: addon.ID, CatalogType: "tv", CatalogID: "channels"}); err != nil {
		t.Fatal(err)
	}

	var channels QueryResult
	s.get(t, "/LiveTv/Channels?fields=Genres", token, &channels)
	if len(channels.Items) != 2 {
		t.Fatalf("channels: %+v", channels.Items)
	}
	zeb, orbe := channels.Items[0], channels.Items[1]
	if zeb.Name != "ZZ| Zeb One" || zeb.Number != "5" || zeb.ChannelNumber != "5" || zeb.ImageTags["Primary"] == "" ||
		zeb.Genres == nil || !slices.Equal(*zeb.Genres, []string{"News"}) {
		t.Errorf("first channel: %+v", zeb)
	}
	if orbe.Name != "Orbe Junior" || orbe.Number != "2" || orbe.Genres == nil || !slices.Equal(*orbe.Genres, []string{"Kids"}) {
		t.Errorf("second channel, numbered by its place: %+v", orbe)
	}
	if zeb.CurrentProgram == nil || zeb.CurrentProgram.Name != "Zeb Tonight" || orbe.CurrentProgram != nil {
		t.Errorf("the guide by tvg-id: %+v / %+v", zeb.CurrentProgram, orbe.CurrentProgram)
	}

	liveAnalysis(t, s, user, zeb.Id)
	source := livePlaybackInfo(t, s, token, zeb.Id, "jellyfin-web-chrome")
	if !source.IsInfiniteStream || !source.SupportsDirectPlay {
		t.Fatalf("a live source played as it is: %+v", source)
	}
	response, err := http.Get(source.Path)
	if err != nil {
		t.Fatal(err)
	}
	playlist, _ := io.ReadAll(response.Body)
	response.Body.Close()
	var segment string
	for line := range strings.Lines(string(playlist)) {
		if strings.HasPrefix(line, "http") {
			segment = strings.TrimSpace(line)
		}
	}
	if segment == "" {
		t.Fatalf("relayed playlist: %d %s", response.StatusCode, playlist)
	}
	response, err = http.Get(segment)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || string(body) != "segment bytes" || provider.refusedUser.Load() != 0 {
		t.Errorf("relayed segment: %d %q, %d requests without the list's user agent", response.StatusCode, body, provider.refusedUser.Load())
	}

	if err := s.iptv.Update(t.Context(), addons.Shared(), addon.ID, iptv.Changes{Groups: []string{"Kids"}}, false); err != nil {
		t.Fatal(err)
	}
	s.get(t, "/LiveTv/Channels", token, &channels)
	if len(channels.Items) != 1 || channels.Items[0].Name != "Orbe Junior" {
		t.Errorf("the groups chosen: %+v", channels.Items)
	}
	// A channel of a group no longer shown is no longer played.
	if status, _ := s.call(http.MethodPost, "/Items/"+zeb.Id+"/PlaybackInfo", app("tv", token), map[string]any{}); status != http.StatusNotFound {
		t.Errorf("PlaybackInfo of a channel no longer listed: %d", status)
	}
}

// A source of another user is not one's own, and a user without Live TV
// has no channels from it.
func TestIPTVSourcesFollowScopesAndLiveTv(t *testing.T) {
	provider := newIPTVProvider(t)
	s := newTestServer(t, 10)
	owner := s.user("owner", nil)
	s.user("other", nil)
	ownerToken := s.signIn("owner", "tv")
	otherToken := s.signIn("other", "tv")
	if _, err := s.iptv.Add(t.Context(), addons.Personal(owner.ID), iptv.NewSource{Name: "Mine",
		Account: iptv.Account{Kind: addons.KindM3U, URL: provider.url + "/get.php"}}, false); err != nil {
		t.Fatal(err)
	}
	var channels QueryResult
	if s.get(t, "/LiveTv/Channels", ownerToken, &channels); len(channels.Items) != 2 {
		t.Errorf("the owner's channels: %+v", channels.Items)
	}
	if s.get(t, "/LiveTv/Channels", otherToken, &channels); len(channels.Items) != 0 {
		t.Errorf("another user's channels: %+v", channels.Items)
	}
	if _, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Shared",
		Account: iptv.Account{Kind: addons.KindM3U, URL: provider.url + "/get.php"}}, false); err != nil {
		t.Fatal(err)
	}
	s.user("nolive", func(c *accounts.UserChanges) { c.LiveTv = new(false) })
	token := s.signIn("nolive", "tv")
	if status, body := s.call(http.MethodGet, "/LiveTv/Channels", app("tv", token), nil); status == http.StatusOK && strings.Contains(string(body), "Orbe") {
		t.Errorf("channels of a user without Live TV: %d %s", status, body)
	}
}
