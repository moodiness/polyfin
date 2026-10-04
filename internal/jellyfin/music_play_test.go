package jellyfin

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// fetch gets a URL of the test server, absolute or relative, without
// following redirects.
func (s testServer) fetch(t *testing.T, target string) (*http.Response, []byte) {
	t.Helper()
	if strings.HasPrefix(target, "/") {
		target = s.url + target
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Get(target)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	return response, body
}

// firstSong lists the songs of the addon's album row and returns the first.
func firstSong(t *testing.T, s testServer, token, library string) string {
	t.Helper()
	var songs QueryResult
	s.get(t, "/Items?ParentId="+library+"&IncludeItemTypes=Audio&Recursive=true&SortBy=SortName", token, &songs)
	if len(songs.Items) == 0 {
		t.Fatal("no songs")
	}
	return songs.Items[0].Id
}

// hlsProfile is an app that plays MP3 as it is and takes the rest as AAC
// in fragmented MP4 HLS segments, as jellyfin-web does.
var hlsProfile = map[string]any{"MaxStreamingBitrate": 140000000, "MusicStreamingTranscodingBitrate": 192000,
	"DirectPlayProfiles": []any{map[string]string{"Type": "Audio", "Container": "mp3"}},
	"TranscodingProfiles": []any{map[string]string{"Type": "Audio", "Container": "mp4", "AudioCodec": "aac", "Protocol": "hls",
		"Context": "Streaming", "MaxAudioChannels": "2", "MinSegments": "1"}}}

func TestTracksConvertToHLSSegments(t *testing.T) {
	ffmpeg, flac := tone(t)
	s := newProbingServer(t, 10, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"))
	token, _, views := listening(t, s, newFakeEclipse(t, flac).url)
	song := firstSong(t, s, token, views["New Releases"])
	var info playbackInfoResponse
	status, body := s.call(http.MethodPost, "/Items/"+song+"/PlaybackInfo", app("web", token), map[string]any{"DeviceProfile": hlsProfile})
	if status != http.StatusOK || json.Unmarshal(body, &info) != nil || len(info.MediaSources) != 1 {
		t.Fatalf("playback info: %d %s", status, body)
	}
	source := info.MediaSources[0]
	if source.SupportsDirectPlay || source.TranscodingSubProtocol != "hls" || source.TranscodingContainer != "mp4" ||
		!strings.Contains(source.TranscodingUrl, "AudioCodec=aac") || !strings.Contains(source.TranscodingUrl, "AudioBitrate=192000") {
		t.Fatalf("decision: %s", body)
	}
	response, master := s.fetch(t, source.TranscodingUrl)
	if response.StatusCode != http.StatusOK || !bytes.Contains(master, []byte(`CODECS="mp4a.40.2"`)) {
		t.Fatalf("master: %d %s", response.StatusCode, master)
	}
	base := strings.TrimSuffix(source.TranscodingUrl[:strings.Index(source.TranscodingUrl, "?")], "master.m3u8")
	media := strings.TrimSpace(string(master[bytes.LastIndexByte(bytes.TrimSpace(master), '\n')+1:]))
	response, playlist := s.fetch(t, base+media)
	if response.StatusCode != http.StatusOK || bytes.Count(playlist, []byte("#EXTINF:3.000000")) != 3 || !bytes.Contains(playlist, []byte("#EXT-X-MAP")) {
		t.Fatalf("media playlist: %d %s", response.StatusCode, playlist)
	}
	var segments []string
	for line := range strings.Lines(string(playlist)) {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, "hls1/") {
			segments = append(segments, line)
		}
		if uri, ok := strings.CutPrefix(line, `#EXT-X-MAP:URI="`); ok {
			segments = append([]string{strings.TrimSuffix(uri, `"`)}, segments...)
		}
	}
	response, init := s.fetch(t, base+segments[0])
	if response.StatusCode != http.StatusOK || !bytes.Contains(init[:min(len(init), 16)], []byte("ftyp")) || !bytes.Contains(init, []byte("mp4a")) {
		t.Fatalf("initialization segment: %d %d bytes", response.StatusCode, len(init))
	}
	response, segment := s.fetch(t, base+segments[2])
	if response.StatusCode != http.StatusOK || !bytes.Contains(segment, []byte("moof")) || len(segment) < 10_000 {
		t.Fatalf("second segment: %d %d bytes", response.StatusCode, len(segment))
	}

	// Without the conversion permission, the song does not play on the
	// app, as Jellyfin answers.
	s.store.UpdateUser(t.Context(), mustUser(t, s, "listener").ID, accounts.UserChanges{AudioTranscoding: new(false)}, nil)
	status, body = s.call(http.MethodPost, "/Items/"+song+"/PlaybackInfo", app("web", token), map[string]any{"DeviceProfile": hlsProfile})
	if status != http.StatusOK || !bytes.Contains(body, []byte(`"ErrorCode":"NoCompatibleStream"`)) {
		t.Errorf("without conversion: %d %s", status, body)
	}
	if response, _ := s.fetch(t, base+segments[2]); response.StatusCode != http.StatusForbidden {
		t.Errorf("a segment without conversion: %d", response.StatusCode)
	}
}

func mustUser(t *testing.T, s testServer, name string) accounts.User {
	t.Helper()
	users, err := s.store.Users(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range users {
		if user.Name == name {
			return user
		}
	}
	t.Fatalf("no user %s", name)
	return accounts.User{}
}

func TestUniversalAudioPlaysAsItIsOrConverted(t *testing.T) {
	ffmpeg, flac := tone(t)
	s := newProbingServer(t, 10, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"))
	token, user, views := listening(t, s, newFakeEclipse(t, flac).url)
	song := firstSong(t, s, token, views["New Releases"])
	universal := func(query url.Values) (*http.Response, []byte) {
		query.Set("UserId", user)
		query.Set("ApiKey", token)
		return s.fetch(t, "/Audio/"+song+"/universal?"+query.Encode())
	}
	// The app takes FLAC: the song as it is, relayed from the addon's
	// local address.
	response, body := universal(url.Values{"Container": {"opus,mp3|mp3,flac,webma"}, "TranscodingContainer": {"mp4"},
		"TranscodingProtocol": {"hls"}, "AudioCodec": {"aac"}})
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "audio/flac" || !bytes.HasPrefix(body, []byte("fLaC")) {
		t.Fatalf("as it is: %d %s %q", response.StatusCode, response.Header.Get("Content-Type"), body[:min(len(body), 8)])
	}
	// It does not: an HLS master playlist converting to AAC.
	response, body = universal(url.Values{"Container": {"opus,mp3|mp3"}, "TranscodingContainer": {"mp4"},
		"TranscodingProtocol": {"hls"}, "AudioCodec": {"aac"}})
	if response.StatusCode != http.StatusOK || !bytes.Contains(body, []byte("#EXT-X-STREAM-INF")) || !bytes.Contains(body, []byte("audioCodec=aac")) {
		t.Fatalf("HLS: %d %s", response.StatusCode, body)
	}
	// Or a progressive MP3 stream.
	response, body = universal(url.Values{"Container": {"opus"}, "TranscodingContainer": {"mp3"}, "TranscodingProtocol": {"http"},
		"AudioCodec": {"mp3"}})
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "audio/mpeg" || len(body) < 10_000 {
		t.Fatalf("MP3: %d %s %d bytes", response.StatusCode, response.Header.Get("Content-Type"), len(body))
	}
}

func TestAudiobookAddonsMakeBooksLibraries(t *testing.T) {
	s := newTestServer(t, 10)
	addon := newFakeEclipse(t, "")
	addon.content = "audiobook"
	token, _, views := listening(t, s, addon.url)
	var result QueryResult
	s.get(t, "/UserViews", token, &result)
	for _, view := range result.Items {
		if view.CollectionType != "books" {
			t.Errorf("%s is a %q library", view.Name, view.CollectionType)
		}
	}
	var books QueryResult
	s.get(t, "/Items?ParentId="+views["Top Songs"], token, &books)
	if len(books.Items) != 2 || books.Items[0].Type != "AudioBook" || books.Items[0].MediaType != "Audio" {
		t.Fatalf("books: %+v", books.Items)
	}
	var book BaseItemDto
	s.get(t, "/Items/"+books.Items[0].Id, token, &book)
	if book.Chapters == nil || len(*book.Chapters) != 2 || (*book.Chapters)[1].Name != "Ending" || (*book.Chapters)[1].StartPositionTicks != 50_000_000 {
		t.Errorf("chapters: %+v", book.Chapters)
	}
	status, body := s.call(http.MethodGet, "/Items/"+books.Items[0].Id, app("web", token), nil)
	if status != http.StatusOK {
		t.Fatal(status)
	}
	matchesFixture(t, "music/audiobook", body, shapeRules{dynamic: []string{"ImageTags", "ImageBlurHashes", "ProviderIds"},
		absent: map[string][]string{
			"Path": {"*"},
			// Described from the stream reply, as songs are (see musicShapes).
			"Bitrate": {"*"}, "Size": {"*"}, "BitRate": {"*"}, "Channels": {"*"}, "ChannelLayout": {"*"}, "TimeBase": {"*"},
			"Profile": {"*"}, "Language": {"*"}, "LocalizedLanguage": {"*"},
			// The addon gives these books no year; Jellyfin dated its file
			// with the zero date.
			"PremiereDate": {"*"},
		},
		returned: map[string][]string{
			// The addon's books have covers, a year and an album name; the
			// recorded file had none.
			"PrimaryImageAspectRatio": {"*"}, "ProductionYear": {"*"}, "AlbumId": {"*"}, "AlbumPrimaryImageTag": {"*"},
			"IndexNumber": {"*"}, "BitDepth": {"*"}, "ImageTag": {"*"},
		}})
}

func TestUsersOwnMusicAddonsStayOnPublicAddresses(t *testing.T) {
	s := newTestServer(t, 10)
	addon := newFakeEclipse(t, "")
	member := s.user("member", nil)
	admin := s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	// Installing from a local address is refused to a member.
	if _, err := s.addons.Install(t.Context(), addons.Personal(member.ID), addon.url, true); err == nil {
		t.Fatal("a member installed an addon on a local address")
	}
	// Installed anyway, as when the addon moved there, it is not reached
	// on the member's behalf; an administrator's own is.
	for _, user := range []accounts.User{member, admin} {
		if _, err := s.addons.Install(t.Context(), addons.Personal(user.ID), addon.url, false); err != nil {
			t.Fatal(err)
		}
	}
	for name, want := range map[string]int{"member": http.StatusBadGateway, "admin": http.StatusOK} {
		token := s.signIn(name, "web")
		var views QueryResult
		s.get(t, "/UserViews", token, &views)
		if len(views.Items) != 2 {
			t.Fatalf("%s's views: %+v", name, views.Items)
		}
		if status, body := s.call(http.MethodGet, "/Items?ParentId="+views.Items[0].Id, app("web", token), nil); status != want {
			t.Errorf("%s's own music: %d %s", name, status, body)
		}
	}
	// Other users do not see them.
	s.user("other", nil)
	var views QueryResult
	s.get(t, "/UserViews", s.signIn("other", "web"), &views)
	if len(views.Items) != 0 {
		t.Errorf("another user's views: %+v", views.Items)
	}
}

func TestHiddenMusicLibrariesAreNotListed(t *testing.T) {
	s := newTestServer(t, 10)
	token, _, views := listening(t, s, newFakeEclipse(t, "").url)
	hidden := mustID(t, views["New Releases"])
	s.store.UpdateUser(t.Context(), mustUser(t, s, "listener").ID, accounts.UserChanges{HiddenLibraries: &[]accounts.ID{hidden}}, nil)
	var result QueryResult
	s.get(t, "/UserViews", token, &result)
	if len(result.Items) != 1 || result.Items[0].Name != "Top Songs" {
		t.Errorf("views: %+v", result.Items)
	}
	for _, path := range []string{"/Items?ParentId=" + views["New Releases"], "/Artists?ParentId=" + views["New Releases"]} {
		if status, body := s.call(http.MethodGet, path, app("web", token), nil); status != http.StatusUnauthorized {
			t.Errorf("%s: %d %s", path, status, body)
		}
	}
	// Its albums stay reachable through the other library, as titles do.
	var albums QueryResult
	s.get(t, "/Items?ParentId="+views["Top Songs"]+"&IncludeItemTypes=MusicAlbum&Recursive=true", token, &albums)
	if len(albums.Items) != 2 {
		t.Errorf("albums of the visible library: %+v", albums.Items)
	}
}
