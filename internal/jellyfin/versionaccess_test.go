package jellyfin

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/stremio"
)

// accessAddon serves the movie tt1000, with a catalog listing it when
// catalog is set, and a stream for each file named, of the same size
// whatever the addon, so that two addons listing a name list the same
// file. Each stream carries a subtitle file of its own, whose text names
// the addon. files counts the requests for the files' bytes.
type accessAddon struct {
	manifest, url string
	files         *atomic.Int32
}

func newAccessAddon(t *testing.T, name string, catalog bool, files ...string) accessAddon {
	t.Helper()
	var server *httptest.Server
	var fetched atomic.Int32
	movie := stremio.Meta{ID: "tt1000", Type: "movie", Name: "Movie", Runtime: "2h", Year: "2008"}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			manifest := stremio.Manifest{ID: "access." + strings.ToLower(strings.ReplaceAll(name, " ", ".")), Name: name, Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "stream"}}}
			if catalog {
				manifest.Resources = append(manifest.Resources, stremio.Resource{Name: "catalog"}, stremio.Resource{Name: "meta"})
				manifest.Catalogs = []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}
			}
			_ = json.NewEncoder(w).Encode(manifest)
		case catalog && strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case catalog && path == "/meta/movie/tt1000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case path == "/stream/movie/tt1000.json":
			var streams []stremio.Stream
			for _, file := range files {
				streams = append(streams, stremio.Stream{Name: file, Description: "English 1080p", URL: server.URL + "/files/" + file + ".mkv",
					BehaviorHints: stremio.StreamBehavior{Filename: file + ".mkv", VideoSize: 4e9},
					Subtitles:     []stremio.Subtitle{{ID: stremio.Text(name + file), Lang: "eng", URL: server.URL + "/subtitles/" + file + ".srt"}}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": streams})
		case strings.HasPrefix(path, "/subtitles/"):
			_, _ = w.Write([]byte("1\n00:00:01,000 --> 00:00:02,000\nWords from " + name + "\n"))
		case strings.HasPrefix(path, "/files/"):
			fetched.Add(1)
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader("\x1a\x45\xdf\xa3 media bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return accessAddon{manifest: server.URL + "/manifest.json", url: server.URL, files: &fetched}
}

// accessSetup is the movie tt1000, listed by the server's addon Access
// Server, which streams the file Shared, and by the own addon of owner,
// an administrator, which streams Shared too and Private; member uses the
// server's sources.
type accessSetup struct {
	testServer
	server, own   accessAddon
	ownAddon      addons.Addon
	owner, member accounts.User
	movie         string
	movieID       accounts.ID
}

func accessOn(t *testing.T) accessSetup {
	t.Helper()
	s := newTestServer(t, 10)
	a := accessSetup{testServer: s, server: newAccessAddon(t, "Access Server", true, "Shared"),
		own: newAccessAddon(t, "Access Own", false, "Shared", "Private")}
	if _, err := s.addons.Install(t.Context(), addons.Shared(), a.server.manifest, false); err != nil {
		t.Fatal(err)
	}
	a.owner = s.user("owner", func(c *accounts.UserChanges) { c.IsAdministrator, c.PersonalAddons = new(true), new(true) })
	var err error
	if a.ownAddon, err = s.addons.Install(t.Context(), addons.Personal(a.owner.ID), a.own.manifest, false); err != nil {
		t.Fatal(err)
	}
	a.member = s.user("member", nil)
	s.permit(t, a.member, true, true, true)
	var views QueryResult
	token := s.signIn("member", "access-setup")
	s.get(t, "/UserViews", token, &views)
	for _, view := range views.Items {
		var page QueryResult
		if s.get(t, "/Items?ParentId="+view.Id, token, &page); len(page.Items) == 1 {
			a.movie = page.Items[0].Id
		}
	}
	if a.movieID, err = accounts.ParseID(a.movie); err != nil {
		t.Fatalf("no movie in %+v", views.Items)
	}
	return a
}

// accessVersion is the version of the file named among versions.
func accessVersion(t *testing.T, versions []library.Version, file string) library.Version {
	t.Helper()
	for _, version := range versions {
		if version.Filename == file+".mkv" {
			return version
		}
	}
	t.Fatalf("no %s among %+v", file, versions)
	return library.Version{}
}

// hlsURL is the master playlist of an audio conversion of version, as
// PlaybackInfo's TranscodingUrl gives it to the user signed in with token
// on device: its play session is signed for the user and the version.
func (a accessSetup) hlsURL(t *testing.T, device, token string, version library.Version) string {
	t.Helper()
	body := map[string]any{"MaxStreamingBitrate": 120_000_000, "DeviceProfile": playbackSetup{}.profile(t, "minimal"),
		"MediaSourceId": version.ID.String()}
	status, data := a.call(http.MethodPost, "/Items/"+a.movie+"/PlaybackInfo", app(device, token), body)
	var answer playbackAnswer
	if err := json.Unmarshal(data, &answer); status != http.StatusOK || err != nil || len(answer.MediaSources) == 0 ||
		answer.MediaSources[0].TranscodingUrl == "" {
		t.Fatalf("no conversion of %s: %d %s", version.Filename, status, data)
	}
	return a.url + answer.MediaSources[0].TranscodingUrl
}

// remuxableFile stores for version the analysis of a file the minimal
// profile converts, with its keyframe index, which HLS needs.
func (a accessSetup) remuxableFile(t *testing.T, version library.Version) {
	t.Helper()
	playbackSetup{testServer: a.testServer}.remuxableVersion(t, version, true)
}

// A version only one user's own addon lists is theirs alone: another user
// is refused it on every path, as a version that does not exist, and no
// byte of it is read for them; the version of a file they share keeps
// only the mirrors their sources list.
func TestOneUsersOwnAddonVersionsAreRefusedToOthers(t *testing.T) {
	t.Parallel()
	a := accessOn(t)
	memberToken, ownerToken := a.signIn("member", "tv"), a.signIn("owner", "phone")
	// The owner lists the movie first: their listing is remembered.
	versions, err := a.library.Versions(t.Context(), a.owner, a.movieID)
	if err != nil || len(versions) != 2 {
		t.Fatalf("the owner's versions: %+v %v", versions, err)
	}
	private, shared := accessVersion(t, versions, "Private"), accessVersion(t, versions, "Shared")
	if len(shared.Mirrors) != 1 || !strings.HasPrefix(shared.Mirrors[0].URL, a.own.url) {
		t.Fatalf("the owner's Shared has mirrors %+v, want their own addon's link", shared.Mirrors)
	}
	a.remuxableFile(t, private)
	a.remuxableFile(t, shared)

	if _, err := a.library.Version(t.Context(), a.member, a.movieID, private.ID); !errors.Is(err, library.ErrNotFound) {
		t.Errorf("the member's Private: %v, want ErrNotFound", err)
	}
	if mine, err := a.library.Version(t.Context(), a.member, a.movieID, shared.ID); err != nil ||
		!strings.HasPrefix(mine.URL, a.server.url) || len(mine.Mirrors) != 0 {
		t.Errorf("the member's Shared: %s with mirrors %+v, %v; want the server addon's link alone", mine.URL, mine.Mirrors, err)
	}
	answer := a.playbackInfo(t, "tv", memberToken, a.movie, map[string]any{"MediaSourceId": private.ID.String()})
	for _, source := range answer.MediaSources {
		if source.Id == private.ID.String() || strings.Contains(source.Name, "Private") {
			t.Errorf("PlaybackInfo offered the member %s", source.Name)
		}
	}
	refused := func(path string, served func(status int, body string) bool) {
		t.Helper()
		if status, body := a.call(http.MethodGet, path, app("tv", memberToken), nil); served(status, string(body)) {
			t.Errorf("%s served the member's request: %d %.80q", path, status, body)
		}
	}
	ok := func(status int, _ string) bool { return status < 300 }
	refused("/Videos/"+a.movie+"/stream?static=true&MediaSourceId="+private.ID.String(), ok)
	refused("/Videos/"+a.movie+"/stream.mkv?static=true&tag="+private.ID.String(), ok)
	refused("/Items/"+private.ID.String()+"/Download", ok)
	refused("/Videos/"+a.movie+"/"+private.ID.String()+"/Subtitles/0/Stream.srt", func(status int, body string) bool {
		return status < 300 || strings.Contains(body, "Access Own")
	})
	refused("/Videos/"+a.movie+"/"+private.ID.String()+"/Attachments/0", func(_ int, body string) bool {
		return !strings.Contains(body, "MediaSource "+private.ID.String()+" not found")
	})
	sharedHLS := a.hlsURL(t, "tv", memberToken, shared)
	if status, _, body := fetchText(t, sharedHLS); status != http.StatusOK {
		t.Fatalf("the member's own conversion: %d %s", status, body)
	}
	// The same URL, made up for Private.
	privateHLS := a.withGrant(t, strings.ReplaceAll(sharedHLS, shared.ID.String(), private.ID.String()), a.member, private)
	if status, _, body := fetchText(t, privateHLS); status < 400 {
		t.Errorf("the member's conversion of Private: %d %.80q", status, body)
	}
	if fetched := a.own.files.Load(); fetched != 0 {
		t.Errorf("%d requests to the owner's addon for files", fetched)
	}

	// The member plays the file their sources list; the owner plays theirs.
	if status, body := a.call(http.MethodGet, "/Videos/"+a.movie+"/stream?static=true&MediaSourceId="+shared.ID.String(),
		app("tv", memberToken), nil); status >= 300 {
		t.Errorf("the member's Shared: %d %s", status, body)
	}
	if status, body := a.call(http.MethodGet, "/Videos/"+a.movie+"/stream?static=true&MediaSourceId="+private.ID.String(),
		app("phone", ownerToken), nil); status >= 300 {
		t.Errorf("the owner's Private: %d %s", status, body)
	}
}

// withGrant is target with its play session signed for user and version.
func (a accessSetup) withGrant(t *testing.T, target string, user accounts.User, version library.Version) string {
	t.Helper()
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	for name := range query {
		if strings.EqualFold(name, "playSessionId") {
			query.Set(name, a.handler.Playback.Signer().Sign(playback.Grant{Version: version.ID, User: user.ID}))
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

// A user who turns the server's sources off gets no version of a server
// addon, a URL kept from before included; one whose own addon lists the
// same file gets it from their addon, its mirror, also over HLS after a
// restart, while another user's listing is the one remembered.
func TestServerSourcesOffLeaveTheServersVersions(t *testing.T) {
	t.Parallel()
	a := accessOn(t)
	carol := a.user("carol", nil)
	carolToken, ownerToken := a.signIn("carol", "laptop"), a.signIn("owner", "phone")
	versions, err := a.library.Versions(t.Context(), a.owner, a.movieID)
	if err != nil || len(versions) != 2 {
		t.Fatalf("the owner's versions: %+v %v", versions, err)
	}
	shared := accessVersion(t, versions, "Shared")
	a.remuxableFile(t, shared)
	carolHLS, ownerHLS := a.hlsURL(t, "laptop", carolToken, shared), a.hlsURL(t, "phone", ownerToken, shared)
	for _, user := range []accounts.User{carol, a.owner} {
		a.limit(t, user, func(c *accounts.UserChanges) { c.UseSharedAddons = new(false) })
	}
	carol, _ = a.store.User(t.Context(), carol.ID)
	owner, _ := a.store.User(t.Context(), a.owner.ID)

	if _, err := a.library.Version(t.Context(), carol, a.movieID, shared.ID); !errors.Is(err, library.ErrNotFound) {
		t.Errorf("Carol's Shared: %v, want ErrNotFound", err)
	}
	if status, body := a.call(http.MethodGet, "/Videos/"+a.movie+"/stream?static=true&MediaSourceId="+shared.ID.String(),
		app("laptop", carolToken), nil); status < 300 {
		t.Errorf("Carol's stream of Shared: %d %.80q", status, body)
	}
	if status, _, body := fetchText(t, carolHLS); status < 400 {
		t.Errorf("Carol's kept conversion of Shared: %d %.80q", status, body)
	}
	if answer := a.playbackInfo(t, "laptop", carolToken, a.movie, nil); len(answer.MediaSources) != 0 {
		t.Errorf("PlaybackInfo offered Carol %+v", answer.MediaSources)
	}

	// The owner's own addon lists Shared: the mirror of the version
	// remembered is theirs.
	mine, err := a.library.Version(t.Context(), owner, a.movieID, shared.ID)
	if err != nil || !strings.HasPrefix(mine.URL, a.own.url) || mine.Origin.Addon != a.ownAddon.ID || len(mine.Mirrors) != 0 {
		t.Errorf("the owner's Shared: %s from %s with mirrors %+v, %v; want their addon's link alone", mine.URL, mine.Origin.Addon, mine.Mirrors, err)
	}

	// A restart: the member lists the movie first, and the owner's kept
	// conversion plays their own addon's link.
	options := a.handler.Options
	options.Library = library.New(a.pool, a.addons, options.Stremio, options.Logger, a.store.Settings)
	options.Library.SetFollowUps()
	restarted := New(options)
	server := httptest.NewServer(restarted)
	t.Cleanup(server.Close)
	if _, err := options.Library.Versions(t.Context(), a.member, a.movieID); err != nil {
		t.Fatal(err)
	}
	if status, _, body := fetchText(t, strings.Replace(ownerHLS, a.url, server.URL, 1)); status != http.StatusOK {
		t.Errorf("the owner's kept conversion after a restart: %d %s", status, body)
	}
	if mine, err := options.Library.Version(t.Context(), owner, a.movieID, shared.ID); err != nil || !strings.HasPrefix(mine.URL, a.own.url) {
		t.Errorf("the owner's Shared after a restart: %s, %v; want their addon's link", mine.URL, err)
	}
	if status, _, body := fetchText(t, strings.Replace(carolHLS, a.url, server.URL, 1)); status < 400 {
		t.Errorf("Carol's kept conversion after a restart: %d %.80q", status, body)
	}
}
