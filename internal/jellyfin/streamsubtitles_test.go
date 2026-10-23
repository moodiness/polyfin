package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// captionedAddon serves one drama, "Captioned", whose streams carry their
// own subtitles, and no subtitles resource: a link with an English and a
// French file, a link with none, and a torrent with a Spanish file. Its
// subtitle links hold the generation the addon is at: a link of an earlier
// generation has expired and answers 403. Its video links always play: one
// that failed would have Polyfin renew it in the background, asking the
// addon for its streams while a test moves the generation and the clock.
func captionedAddon(t *testing.T, generation *atomic.Int64) (manifest, host string) {
	t.Helper()
	var server *httptest.Server
	movie := stremio.Meta{ID: "tt3000", Type: "movie", Name: "Captioned", Year: "2008", Genres: []string{"Drama"}}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := generation.Load()
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "captioned", Name: "Captioned streams", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case path == "/meta/movie/tt3000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case path == "/stream/movie/tt3000.json":
			subtitle := func(lang string) stremio.Subtitle {
				return stremio.Subtitle{ID: stremio.Text(lang), Lang: lang, URL: fmt.Sprintf("%s/subs/%s.srt?token=%d", server.URL, lang, current)}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{
				{Name: "Captioned link", URL: fmt.Sprintf("%s/files/movie.mp4?token=%d", server.URL, current),
					BehaviorHints: stremio.StreamBehavior{Filename: "Captioned.mp4", VideoSize: 3_000_000_000},
					Subtitles:     []stremio.Subtitle{subtitle("eng"), subtitle("fre")}},
				{Name: "Plain link", URL: server.URL + "/files/plain.mp4"},
				{Name: "Captioned torrent", InfoHash: strings.Repeat("ab", 20), Subtitles: []stremio.Subtitle{subtitle("spa")}},
			}})
		case strings.HasPrefix(path, "/subs/"):
			if r.URL.Query().Get("token") != fmt.Sprint(current) {
				http.Error(w, "expired", http.StatusForbidden)
				return
			}
			lines := map[string]string{"eng": "Hello", "fre": "Bonjour", "spa": "Hola"}
			lang := strings.TrimSuffix(strings.TrimPrefix(path, "/subs/"), ".srt")
			_, _ = fmt.Fprintf(w, "1\r\n00:00:01,000 --> 00:00:04,000\r\n%s %d\r\n", lines[lang], current)
		case strings.HasPrefix(path, "/files/"):
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader("\x1a\x45\xdf\xa3 media bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json", strings.TrimPrefix(server.URL, "http://")
}

// installCaptioned installs the captioned addon on s, its links at
// generation, and returns the drama's identifier and the addon's host.
func installCaptioned(t *testing.T, s testServer, generation *atomic.Int64, token string) (movie, host string) {
	t.Helper()
	manifest, host := captionedAddon(t, generation)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), manifest, false); err != nil {
		t.Fatal(err)
	}
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	return page.Items[0].Id, host
}

// subtitleText fetches a subtitle URL Polyfin gives, and returns its
// status and text.
func (s testServer) subtitleText(t *testing.T, address string) (int, string) {
	t.Helper()
	status, body := s.call(http.MethodGet, strings.TrimPrefix(address, s.url), "", nil)
	return status, string(body)
}

// The subtitles an addon lists with a stream are that version's own, for a
// link as for a torrent: listed for it alone on the item and in
// PlaybackInfo, after the title's, and served through Polyfin, which never
// gives apps the addon's address. A user whose blocked genres hide the
// title gets none of them.
func TestStreamSubtitlesAreTheirVersions(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	proxy := torrenttest.StartProxy(t, true, "", "")
	s.setting(t, func(settings *accounts.Settings) {
		settings.TorrentStreams = true
		settings.TorrentProxy = proxy.Address()
	})
	user := s.user("member", func(c *accounts.UserChanges) { c.TorrentStreams = new(true) })
	s.user("calm", func(c *accounts.UserChanges) { c.BlockedGenres = &[]string{"drama"} })
	token := s.signIn("member", "tv")
	var generation atomic.Int64
	generation.Store(1)
	movie, host := installCaptioned(t, s, &generation, token)
	s.listed(t, user, movie)

	var details BaseItemDto
	if status := s.get(t, "/Users/"+user.ID.String()+"/Items/"+movie, token, &details); status != http.StatusOK || details.MediaSources == nil {
		t.Fatalf("details: %d", status)
	}
	languages := map[string][]string{}
	ids := map[string]string{}
	for _, source := range *details.MediaSources {
		ids[source.Name] = source.Id
		languages[source.Name] = []string{}
		for _, stream := range source.MediaStreams {
			if stream.Type == "Subtitle" && stream.IsExternal {
				languages[source.Name] = append(languages[source.Name], stream.Language)
			}
		}
	}
	want := map[string][]string{"Captioned link": {"eng", "fra"}, "Plain link": {}, "Captioned torrent": {"spa"}}
	for name, langs := range want {
		if got, ok := languages[name]; !ok || strings.Join(got, ",") != strings.Join(langs, ",") {
			t.Errorf("subtitles of %q on the item: %q, want %q", name, got, langs)
		}
	}

	playbackSetup{testServer: s}.analyzed(t, versionNamed(t, s, user, movie, "Captioned link"), "h264-aac-mp4")
	status, data := s.call(http.MethodPost, "/Items/"+movie+"/PlaybackInfo", app("tv", token), map[string]any{"UserId": user.ID.String(),
		"MediaSourceId": ids["Captioned link"], "DeviceProfile": playbackSetup{testServer: s}.profile(t, "jellyfin-web-chrome")})
	var info playbackInfoResponse
	if err := json.Unmarshal(data, &info); status != http.StatusOK || err != nil || len(info.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %d %s", status, data)
	}
	texts := map[string]string{}
	for _, stream := range info.MediaSources[0].MediaStreams {
		if stream.Type != "Subtitle" || !stream.IsExternal {
			continue
		}
		if stream.DeliveryMethod != "External" || strings.Contains(stream.DeliveryUrl, host) {
			t.Errorf("subtitle %s delivered %s at %s", stream.Language, stream.DeliveryMethod, stream.DeliveryUrl)
			continue
		}
		status, text := s.subtitleText(t, stream.DeliveryUrl)
		if status != http.StatusOK {
			t.Errorf("subtitle %s: %d %s", stream.Language, status, text)
		}
		texts[stream.Language] = text
	}
	if !strings.Contains(texts["eng"], "Hello 1") || !strings.Contains(texts["fra"], "Bonjour 1") || len(texts) != 2 {
		t.Errorf("subtitles served in PlaybackInfo: %q", texts)
	}

	torrentFile := "/Videos/" + movie + "/" + ids["Captioned torrent"] + "/Subtitles/0/0/Stream.srt?ApiKey="
	if status, text := s.subtitleText(t, torrentFile+token); status != http.StatusOK || !strings.Contains(text, "Hola 1") {
		t.Errorf("the torrent's subtitle: %d %q", status, text)
	}
	if status, text := s.subtitleText(t, torrentFile+s.signIn("calm", "tv")); status == http.StatusOK {
		t.Errorf("the subtitle of a title hidden by a blocked genre was served: %q", text)
	}
}

// A stream's subtitle links last as long as the version list: once it
// expired, the addon is asked again, and its subtitles are served from the
// links it gives now, never from the expired ones nor from what they gave,
// as files and as HLS renditions alike.
func TestExpiredStreamSubtitlesAreNotServedStale(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	var elapsed atomic.Int64
	s.library.SetClock(func() time.Time { return time.Now().Add(time.Duration(elapsed.Load())) })
	user := s.user("member", nil)
	token := s.signIn("member", "tv")
	var generation atomic.Int64
	generation.Store(1)
	movie, _ := installCaptioned(t, s, &generation, token)
	s.listed(t, user, movie)
	version := versionNamed(t, s, user, movie, "Captioned link")
	p := playbackSetup{testServer: s}
	p.remuxableVersion(t, version, true)
	// A player that takes subtitles from the playlist, with jellyfin-web's
	// codecs.
	var profile map[string]any
	if err := json.Unmarshal(p.profile(t, "jellyfin-web-chrome"), &profile); err != nil {
		t.Fatal(err)
	}
	profile["SubtitleProfiles"] = []map[string]string{{"Format": "vtt", "Method": "Hls"}, {"Format": "srt", "Method": "External"}}
	status, data := s.call(http.MethodPost, "/Items/"+movie+"/PlaybackInfo", app("tv", token), map[string]any{"UserId": user.ID.String(),
		"MediaSourceId": version.ID.String(), "EnableDirectPlay": false, "SubtitleStreamIndex": 0, "DeviceProfile": profile})
	var info playbackInfoResponse
	if err := json.Unmarshal(data, &info); status != http.StatusOK || err != nil || len(info.MediaSources) != 1 || info.MediaSources[0].TranscodingUrl == "" {
		t.Fatalf("PlaybackInfo: %d %s", status, data)
	}
	transcoding := info.MediaSources[0].TranscodingUrl
	segment := s.url + strings.Split(transcoding, "master.m3u8")[0] + "hls1/subtitles0/0.vtt?" + strings.SplitN(transcoding, "?", 2)[1]
	file := func(index int) string {
		return fmt.Sprintf("/Videos/%s/%s/Subtitles/%d/0/Stream.srt?ApiKey=%s", movie, version.ID, index, token)
	}
	if status, text := s.subtitleText(t, file(0)); status != http.StatusOK || !strings.Contains(text, "Hello 1") {
		t.Fatalf("English before the list expired: %d %q", status, text)
	}
	// The addon's links change, the old ones expire, and so does the list.
	generation.Store(2)
	elapsed.Store(int64(time.Duration(s.store.Settings().VersionListMinutes)*time.Minute + time.Minute))
	if status, _, text := fetchText(t, segment); status != http.StatusOK || !strings.Contains(text, "Hello 2") {
		t.Errorf("English rendition after the list expired: %d %q, want Hello 2", status, text)
	}
	for index, want := range []string{"Hello 2", "Bonjour 2"} {
		if status, text := s.subtitleText(t, file(index)); status != http.StatusOK || !strings.Contains(text, want) {
			t.Errorf("subtitle %d after the list expired: %d %q, want %q", index, status, text, want)
		}
	}
}

// The subtitles listed with a stream a series' description embeds for an
// episode are that version's too, and last as long as the version list:
// past it, they are served from the description given again.
func TestEmbeddedStreamSubtitlesFollowTheDescription(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	var elapsed atomic.Int64
	s.library.SetClock(func() time.Time { return time.Now().Add(time.Duration(elapsed.Load())) })
	user := s.user("member", nil)
	token := s.signIn("member", "tv")
	var generation atomic.Int64
	generation.Store(1)
	var server *httptest.Server
	show := func() stremio.Meta {
		token := generation.Load()
		return stremio.Meta{ID: "tt3100", Type: "series", Name: "Captioned show", Released: "2020-01-01T00:00:00.000Z",
			Videos: []stremio.Video{{ID: "tt3100:1:1", Title: "Pilot", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00.000Z",
				Streams: []stremio.Stream{{Name: "Own 720p", URL: fmt.Sprintf("%s/files/pilot.mp4?token=%d", server.URL, token),
					BehaviorHints: stremio.StreamBehavior{Filename: "Captioned.Show.S01E01.mp4", VideoSize: 1_000_000},
					Subtitles:     []stremio.Subtitle{{ID: "eng", Lang: "eng", URL: fmt.Sprintf("%s/subs/eng.srt?token=%d", server.URL, token)}}}}}}}
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "captionedshows", Name: "Captioned shows", Version: "1", Types: []string{"series"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "series", ID: "own", Name: "Own shows"}}})
		case strings.HasPrefix(path, "/catalog/series/own"):
			preview := show()
			preview.Videos = nil
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{preview}})
		case path == "/meta/series/tt3100.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": show()})
		case path == "/subs/eng.srt":
			if r.URL.Query().Get("token") != fmt.Sprint(generation.Load()) {
				http.Error(w, "expired", http.StatusForbidden)
				return
			}
			_, _ = fmt.Fprintf(w, "1\r\n00:00:01,000 --> 00:00:04,000\r\nHello %d\r\n", generation.Load())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), server.URL+"/manifest.json", false); err != nil {
		t.Fatal(err)
	}
	var views, series, episodes QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &series)
	s.get(t, "/Shows/"+series.Items[0].Id+"/Episodes", token, &episodes)
	if len(episodes.Items) != 1 {
		t.Fatalf("the episodes: %+v", episodes.Items)
	}
	pilot := episodes.Items[0].Id
	s.listed(t, user, pilot)
	version := versionNamed(t, s, user, pilot, "Own 720p")
	file := fmt.Sprintf("/Videos/%s/%s/Subtitles/0/0/Stream.srt?ApiKey=%s", pilot, version.ID, token)
	if status, text := s.subtitleText(t, file); status != http.StatusOK || !strings.Contains(text, "Hello 1") {
		t.Fatalf("the episode's subtitle: %d %q", status, text)
	}
	generation.Store(2)
	elapsed.Store(int64(time.Duration(s.store.Settings().VersionListMinutes)*time.Minute + time.Minute))
	if status, text := s.subtitleText(t, file); status != http.StatusOK || !strings.Contains(text, "Hello 2") {
		t.Errorf("the episode's subtitle after the list expired: %d %q, want Hello 2", status, text)
	}
}

// versionNamed is the version of an item named name.
func versionNamed(t *testing.T, s testServer, user accounts.User, item, name string) library.Version {
	t.Helper()
	id, _ := accounts.ParseID(item)
	versions, err := s.library.Versions(t.Context(), user, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range versions {
		if version.Name == name {
			return version
		}
	}
	t.Fatalf("no version %q among %+v", name, versions)
	return library.Version{}
}
