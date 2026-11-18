package jellyfin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/source"
	"github.com/moodiness/polyfin/internal/stremio"
)

// What the addresses of the server's sources hold: the key an addon that
// resolves streams on request was configured with, in the path of each
// stream, and an IPTV account's name and password, in the path of each
// channel, movie and episode, as Xtream servers and their lists write
// them.
const (
	keptKey      = "ADDON-KEY-7f3c9a1e0123456789"
	keptAccount  = "household42"
	keptPassword = "ACCOUNT-pw-5d2b8c"
	keptFile     = "\x1a\x45\xdf\xa3 the bytes of a media file, asked whole and in ranges"

	keptResolved = "/resolve/" + keptKey + "/Played.mp4"
	keptPlaylist = "/resolve/" + keptKey + "/Listed.m3u8"
	keptLive     = "/live/" + keptAccount + "/" + keptPassword + "/"
	keptMovie    = "/movie/" + keptAccount + "/" + keptPassword + "/41.mp4"
	keptEpisode  = "/series/" + keptAccount + "/" + keptPassword + "/701.mp4"
)

// keptSegment is a file of an HLS stream: MPEG-TS packets.
func keptSegment() []byte {
	segment := make([]byte, 188*8)
	for i := 0; i < len(segment); i += 188 {
		segment[i] = 0x47
	}
	return segment
}

// serverSources serves an addon whose two movies stream from addresses
// holding its key, one a file and one a finished HLS playlist, and an
// IPTV list whose HLS channel, MPEG-TS channel, movie and episode stream
// from addresses holding its account. The streams are on publicSource,
// which the test server reaches here (see reachedAt): the playlists name
// their files by a relative address and by a whole one.
func serverSources(t *testing.T) (manifest, list, host string) {
	t.Helper()
	metas := []stremio.Meta{
		{ID: "tt3000", Type: "movie", Name: "Played", Runtime: "2h", Year: "2010", Released: "2010-01-01T00:00:00.000Z"},
		{ID: "tt3001", Type: "movie", Name: "Listed", Runtime: "2h", Year: "2011", Released: "2011-01-01T00:00:00.000Z"},
	}
	streams := map[string]stremio.Stream{
		"tt3000": {Name: "Resolved", URL: "http://" + publicSource + keptResolved, BehaviorHints: stremio.StreamBehavior{Filename: "Played.mp4"}},
		"tt3001": {Name: "Resolved", URL: "http://" + publicSource + keptPlaylist},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		id := strings.TrimSuffix(path[strings.LastIndex(path, "/")+1:], ".json")
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "resolver", Name: "Resolver", Version: "1", Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		case strings.HasPrefix(path, "/meta/movie/") && streams[id].URL != "":
			for _, meta := range metas {
				if meta.ID == id {
					_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
				}
			}
		case strings.HasPrefix(path, "/stream/movie/") && streams[id].URL != "":
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{streams[id]}})
		case path == keptResolved, path == keptMovie, path == keptEpisode:
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader(keptFile))
		case path == keptPlaylist:
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = fmt.Fprintf(w, "#EXTM3U\n#EXT-X-TARGETDURATION:4\n#EXTINF:4,\nfirst.ts\n#EXTINF:4,\nhttp://%s/resolve/%s/second.ts\n#EXT-X-ENDLIST\n",
				publicSource, keptKey)
		case path == "/get.php":
			_, _ = fmt.Fprintf(w, "#EXTM3U\r\n"+
				"#EXTINF:-1 tvg-id=\"one\" group-title=\"News\",Channel One\r\nhttp://%[1]s%[2]s55.m3u8\r\n"+
				"#EXTINF:-1 tvg-id=\"two\" group-title=\"News\",Channel Two\r\nhttp://%[1]s%[2]s56.ts\r\n"+
				"#EXTINF:-1 group-title=\"Movies\",Orb Tale (2001)\r\nhttp://%[1]s%[3]s\r\n"+
				"#EXTINF:-1 group-title=\"Series\",Quill Days S01E01\r\nhttp://%[1]s%[4]s\r\n", publicSource, keptLive, keptMovie, keptEpisode)
		case path == keptLive+"55.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = fmt.Fprintf(w, "#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:7\n#EXTINF:2,\nseg7.ts\n#EXTINF:2,\nhttp://%s%sseg8.ts\n", publicSource, keptLive)
		case strings.HasSuffix(path, ".ts"):
			w.Header().Set("Content-Type", "video/mp2t")
			_, _ = w.Write(keptSegment())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json", server.URL + "/get.php?username=" + keptAccount + "&password=" + keptPassword,
		strings.TrimPrefix(server.URL, "http://")
}

// keptSources is a test server with serverSources as the server's own,
// a member and an administrator who use them.
type keptSources struct {
	testServer
	member, administrator accounts.User
	// played and listed are the addon's movies, a file and a playlist;
	// playlist and stream the list's channels, HLS and MPEG-TS; movie and
	// episode its titles. Each has one version.
	played, listed, playlist, stream, movie, episode string
	versions                                         map[string]library.Version
}

func newKeptSources(t *testing.T) keptSources {
	t.Helper()
	manifest, list, host := serverSources(t)
	k := keptSources{versions: map[string]library.Version{}}
	k.testServer = newServerReaching(t, 10, "ffprobe-not-installed", func(o source.Opener) source.Opener { return reachedAt{Opener: o, local: host} })
	k.member = k.user("member", nil)
	administrator, err := k.store.CreateUser(t.Context(), accounts.NewUser{Name: "administrator", Password: "correct horse", IsAdministrator: true})
	if err != nil {
		t.Fatal(err)
	}
	k.administrator = administrator
	k.setting(t, func(settings *accounts.Settings) { settings.SyncAddonSubtitles = false })
	if _, err := k.addons.Install(t.Context(), addons.Shared(), manifest, false); err != nil {
		t.Fatal(err)
	}
	on := true
	if _, err := k.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Provider", Account: iptv.Account{Kind: addons.KindM3U, URL: list},
		Options: &iptv.OptionsPatch{Movies: &on, Series: &on}}, false); err != nil {
		t.Fatal(err)
	}
	token := k.signIn("member", "setup")
	// The titles are those of the libraries the member sees, and the
	// channels those of Live TV.
	queries := []string{"/LiveTv/Channels"}
	var views QueryResult
	k.get(t, "/UserViews", token, &views)
	for _, view := range views.Items {
		queries = append(queries, "/Items?ParentId="+view.Id)
	}
	titled := func(name string) string {
		t.Helper()
		var seen []string
		for _, query := range queries {
			var found QueryResult
			k.get(t, query, token, &found)
			for _, item := range found.Items {
				if item.Name == name {
					return item.Id
				}
			}
			seen = append(seen, itemNames(found.Items)...)
		}
		t.Fatalf("%s is not listed: %q", name, seen)
		return ""
	}
	k.played, k.listed, k.movie = titled("Played"), titled("Listed"), titled("Orb Tale")
	k.playlist, k.stream = titled("Channel One"), titled("Channel Two")
	show := titled("Quill Days")
	var seasons, episodes QueryResult
	k.get(t, "/Shows/"+show+"/Seasons?userId="+k.member.ID.String(), token, &seasons)
	if len(seasons.Items) != 1 {
		t.Fatalf("seasons: %+v", seasons.Items)
	}
	k.get(t, "/Shows/"+show+"/Episodes?userId="+k.member.ID.String()+"&seasonId="+seasons.Items[0].Id, token, &episodes)
	if len(episodes.Items) != 1 {
		t.Fatalf("episodes: %+v", episodes.Items)
	}
	k.episode = episodes.Items[0].Id
	for _, item := range []string{k.played, k.listed, k.playlist, k.stream, k.movie, k.episode} {
		versions, err := k.library.Versions(t.Context(), k.member, mustID(t, item))
		if err != nil || len(versions) != 1 {
			t.Fatalf("versions of %s: %+v %v", item, versions, err)
		}
		k.versions[item] = versions[0]
	}
	// As ffprobe would have found them: the addon's file, and the
	// channels' streams, which are analyzed before they play.
	playbackSetup{testServer: k.testServer, user: k.member}.analyzed(t, k.versions[k.played], "h264-aac-mp4")
	seedLiveAnalysis(t, k.testServer, k.versions[k.playlist].ID)
	k.analyzedAs(t, k.stream, "mpegts", 0)
	return k
}

// analyzedAs stores the analysis of an item's version as ffprobe would
// have found a stream of format, of a duration, 0 for one with no end.
func (k keptSources) analyzedAs(t *testing.T, item, format string, duration time.Duration) {
	t.Helper()
	probe, err := os.ReadFile(filepath.Join(playbackFixtures, "probes", "h264-aac-mp4.json"))
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := media.Parse(probe)
	if err != nil {
		t.Fatal(err)
	}
	analysis.Format, analysis.Duration, analysis.Remote = format, duration, true
	data, _ := json.Marshal(analysis)
	if _, err := k.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", k.versions[item].ID, data); err != nil {
		t.Fatal(err)
	}
}

// keptAnswer is what an app received for a request, redirections not
// followed.
type keptAnswer struct {
	status int
	header http.Header
	body   string
}

// ask sends a request as an app's player does, without credentials but
// those of its address, and reads the answer, at most its first limit
// bytes when limit is set: a channel's stream has no end.
func (k keptSources) ask(t *testing.T, method, target string, header http.Header, limit int) keptAnswer {
	t.Helper()
	if strings.HasPrefix(target, "/") {
		target = k.url + target
	}
	request, _ := http.NewRequestWithContext(t.Context(), method, target, nil)
	for name, values := range header {
		request.Header[name] = values
	}
	response, err := noRedirects.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body []byte
	if limit > 0 {
		body = make([]byte, limit)
		n, _ := io.ReadFull(response.Body, body)
		body = body[:n]
	} else {
		body, _ = io.ReadAll(response.Body)
	}
	return keptAnswer{status: response.StatusCode, header: response.Header, body: string(body)}
}

// keeps fails the test when an answer tells anything of the sources'
// addresses: their host, the addon's key, the account's name or its
// password, in a header or in the body.
func (a keptAnswer) keeps(t *testing.T, what string) {
	t.Helper()
	var headers strings.Builder
	_ = a.header.Write(&headers)
	for _, secret := range []string{publicSource, keptKey, keptAccount, keptPassword} {
		if strings.Contains(headers.String(), secret) || strings.Contains(a.body, secret) {
			t.Errorf("%s tells %q:\n%s\n%s", what, secret, headers.String(), a.body)
		}
	}
}

// relayed fails the test unless an answer is Polyfin's own, with want as
// its body when want is set, and tells nothing of the source.
func (a keptAnswer) relayed(t *testing.T, what string, status int, want string) {
	t.Helper()
	if a.status != status || a.header.Get("Location") != "" || want != "" && a.body != want {
		t.Errorf("%s: %d to %q, %q; want %d from Polyfin", what, a.status, a.header.Get("Location"), a.body, status)
	}
	a.keeps(t, what)
}

// sent fails the test unless an answer sends the app to address.
func (a keptAnswer) sent(t *testing.T, what, address string) {
	t.Helper()
	if a.status != http.StatusFound || a.header.Get("Location") != address {
		t.Errorf("%s: %d to %q, want a redirection to %s", what, a.status, a.header.Get("Location"), address)
	}
}

// playsRelayed asks for a relayed HLS playlist and each file it names, and
// fails the test unless every line is one of Polyfin's addresses of item,
// which answers the file.
func (k keptSources) playsRelayed(t *testing.T, what, target, item string, files int) {
	t.Helper()
	playlist := k.ask(t, http.MethodGet, target, nil, 0)
	playlist.relayed(t, what, http.StatusOK, "")
	named := 0
	for line := range strings.Lines(playlist.body) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		named++
		if !strings.HasPrefix(line, k.url+"/Videos/"+item+"/live/") {
			t.Errorf("%s names %q, want one of Polyfin's addresses", what, line)
			continue
		}
		file := k.ask(t, http.MethodGet, line, nil, 0)
		file.relayed(t, what+", a file it names", http.StatusOK, string(keptSegment()))
	}
	if named != files || !strings.HasPrefix(playlist.body, "#EXTM3U") {
		t.Errorf("%s names %d files, want %d:\n%s", what, named, files, playlist.body)
	}
}

// A member who plays from one of the server's sources gets it through
// Polyfin, whatever the address asked: a server addon's file, and an IPTV
// source's channels, movie and episode. No answer of the play tells the
// source's address, where the addon's key or the account's name and
// password are: not PlaybackInfo, not the item's details, not the stream,
// whole, in ranges or its headers alone, not a download, not a line of a
// playlist. With Send apps to the source, the same plays are sent to
// their sources.
func TestAMemberPlaysTheServersSourcesThroughPolyfin(t *testing.T) {
	t.Parallel()
	k := newKeptSources(t)
	token := k.signIn("member", "tv")
	stream := func(item string) string {
		return "/Videos/" + item + "/stream?static=true&mediaSourceId=" + k.versions[item].ID.String() + "&ApiKey=" + token
	}

	// What the app is told before it plays.
	status, body := k.call(http.MethodPost, "/Items/"+k.played+"/PlaybackInfo", app("tv", token), map[string]any{})
	var info playbackInfoResponse
	if err := json.Unmarshal(body, &info); status != http.StatusOK || err != nil || len(info.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %d %s", status, body)
	}
	keptAnswer{body: string(body)}.keeps(t, "PlaybackInfo of the addon's movie")
	for name, item := range map[string]string{"the addon's movie": k.played, "the list's movie": k.movie, "the list's episode": k.episode,
		"the list's channel": k.playlist} {
		_, details := k.call(http.MethodGet, "/Users/"+k.member.ID.String()+"/Items/"+item, app("tv", token), nil)
		keptAnswer{body: string(details)}.keeps(t, "the details of "+name)
	}
	status, body = k.call(http.MethodPost, "/Items/"+k.playlist+"/PlaybackInfo", app("tv", token),
		map[string]any{"DeviceProfile": deviceProfile(t, "swiftfin-native"), "IsPlayback": true, "AutoOpenLiveStream": true})
	if status != http.StatusOK {
		t.Fatalf("PlaybackInfo of the channel: %d %s", status, body)
	}
	keptAnswer{body: string(body)}.keeps(t, "PlaybackInfo of the list's channel")

	// The addon's file: the address PlaybackInfo gave, and the one an app
	// builds with its token; whole, a range of it, and its headers.
	for name, target := range map[string]string{"the media source's Path": info.MediaSources[0].Path, "the stream route": stream(k.played)} {
		k.ask(t, http.MethodGet, target, nil, 0).relayed(t, "the addon's movie, "+name, http.StatusOK, keptFile)
		ranged := k.ask(t, http.MethodGet, target, http.Header{"Range": {"bytes=5-23"}}, 0)
		ranged.relayed(t, "a range of the addon's movie, "+name, http.StatusPartialContent, keptFile[5:24])
		if got := ranged.header.Get("Content-Range"); got != fmt.Sprintf("bytes 5-23/%d", len(keptFile)) {
			t.Errorf("a range of the addon's movie, %s: Content-Range %q", name, got)
		}
		head := k.ask(t, http.MethodHead, target, nil, 0)
		head.relayed(t, "the headers of the addon's movie, "+name, http.StatusOK, "")
		if got := head.header.Get("Content-Length"); got != fmt.Sprint(len(keptFile)) {
			t.Errorf("the headers of the addon's movie, %s: Content-Length %q", name, got)
		}
	}
	download := k.ask(t, http.MethodGet, "/Items/"+k.played+"/Download?ApiKey="+token, nil, 0)
	download.relayed(t, "a download of the addon's movie", http.StatusOK, keptFile)
	if !strings.Contains(download.header.Get("Content-Disposition"), "attachment") {
		t.Errorf("a download of the addon's movie: Content-Disposition %q", download.header.Get("Content-Disposition"))
	}

	// The IPTV source's movie and episode, whose addresses Polyfin builds
	// from the account.
	for name, item := range map[string]string{"the list's movie": k.movie, "the list's episode": k.episode} {
		k.ask(t, http.MethodGet, stream(item), nil, 0).relayed(t, name, http.StatusOK, keptFile)
		k.ask(t, http.MethodGet, stream(item), http.Header{"Range": {"bytes=5-23"}}, 0).relayed(t, "a range of "+name, http.StatusPartialContent, keptFile[5:24])
		k.ask(t, http.MethodGet, "/Items/"+item+"/Download?ApiKey="+token, nil, 0).relayed(t, "a download of "+name, http.StatusOK, keptFile)
	}

	// Its channels: an HLS playlist, whose lines are Polyfin's and answer
	// its files, and an MPEG-TS stream, read from the channel's feed.
	k.playsRelayed(t, "the list's HLS channel", stream(k.playlist), k.playlist, 2)
	feed := k.ask(t, http.MethodGet, stream(k.stream), nil, 188)
	feed.relayed(t, "the list's MPEG-TS channel", http.StatusOK, "")
	if len(feed.body) != 188 || feed.body[0] != 0x47 || feed.header.Get("Content-Type") != "video/mp2t" {
		t.Errorf("the list's MPEG-TS channel: %d bytes of %q", len(feed.body), feed.header.Get("Content-Type"))
	}

	// Send apps to the source: every play is sent to its source, an IPTV
	// source's too.
	k.setting(t, func(settings *accounts.Settings) { settings.SendAppsToSource = true })
	for item, address := range map[string]string{k.played: keptResolved, k.movie: keptMovie, k.episode: keptEpisode, k.playlist: keptLive + "55.m3u8",
		k.stream: keptLive + "56.ts"} {
		k.ask(t, http.MethodGet, stream(item), nil, 0).sent(t, "with Send apps to the source, "+k.versions[item].URL, "http://"+publicSource+address)
	}
	k.ask(t, http.MethodGet, info.MediaSources[0].Path, nil, 0).sent(t, "with Send apps to the source, the media source's Path", "http://"+publicSource+keptResolved)
	k.ask(t, http.MethodGet, "/Items/"+k.played+"/Download?ApiKey="+token, nil, 0).sent(t, "with Send apps to the source, a download",
		"http://"+publicSource+keptResolved)
}

// An administrator, who configured the server's addons, is still sent to
// their source, and gets an IPTV source through Polyfin as everyone
// does: its addresses are built from the account, which the admin app
// shows to no one.
func TestAnAdministratorIsSentToAServerAddonNotToAnIPTVSource(t *testing.T) {
	t.Parallel()
	k := newKeptSources(t)
	token := k.signIn("administrator", "tv")
	stream := func(item string) string {
		return "/Videos/" + item + "/stream?static=true&mediaSourceId=" + k.versions[item].ID.String() + "&ApiKey=" + token
	}
	k.ask(t, http.MethodGet, stream(k.played), nil, 0).sent(t, "the addon's movie", "http://"+publicSource+keptResolved)
	k.ask(t, http.MethodGet, "/Items/"+k.played+"/Download?ApiKey="+token, nil, 0).sent(t, "a download of the addon's movie", "http://"+publicSource+keptResolved)
	// A playlist the addon lists is the addon's too.
	k.ask(t, http.MethodGet, stream(k.listed), nil, 0).sent(t, "the addon's playlist", "http://"+publicSource+keptPlaylist)

	for name, item := range map[string]string{"the list's movie": k.movie, "the list's episode": k.episode} {
		k.ask(t, http.MethodGet, stream(item), nil, 0).relayed(t, name, http.StatusOK, keptFile)
		k.ask(t, http.MethodGet, "/Items/"+item+"/Download?ApiKey="+token, nil, 0).relayed(t, "a download of "+name, http.StatusOK, keptFile)
	}
	k.playsRelayed(t, "the list's HLS channel", stream(k.playlist), k.playlist, 2)
	feed := k.ask(t, http.MethodGet, stream(k.stream), nil, 188)
	feed.relayed(t, "the list's MPEG-TS channel", http.StatusOK, "")
	if len(feed.body) != 188 || feed.body[0] != 0x47 {
		t.Errorf("the list's MPEG-TS channel: %d bytes", len(feed.body))
	}
}

// A playlist's lines are addresses: one a server addon lists reaches a
// member only with each line named at Polyfin's own address, which
// relays its file. Until its analysis tells that it is a playlist, and
// on the routes that name no file for it, a download, it is refused
// rather than passed on as its source wrote it.
func TestAPlaylistReachesAMemberWithPolyfinsAddressesOnly(t *testing.T) {
	t.Parallel()
	k := newKeptSources(t)
	token := k.signIn("member", "tv")
	stream := "/Videos/" + k.listed + "/stream?static=true&mediaSourceId=" + k.versions[k.listed].ID.String() + "&ApiKey=" + token
	download := "/Items/" + k.listed + "/Download?ApiKey=" + token

	// Not analyzed yet: its first bytes are text, not media.
	for name, target := range map[string]string{"the stream route": stream, "a download": download} {
		refused := k.ask(t, http.MethodGet, target, nil, 0)
		refused.relayed(t, "a playlist not analyzed yet, "+name, http.StatusNotFound, "")
		if strings.Contains(refused.body, "#EXT") {
			t.Errorf("a playlist not analyzed yet, %s, is passed on:\n%s", name, refused.body)
		}
	}

	// Analyzed: a finished playlist, relayed where its files are named.
	k.analyzedAs(t, k.listed, "hls", 8*time.Second)
	k.playsRelayed(t, "the addon's playlist", stream, k.listed, 2)
	k.ask(t, http.MethodGet, download, nil, 0).relayed(t, "a download of the addon's playlist", http.StatusNotFound, "")
}
