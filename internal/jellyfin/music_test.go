package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/activity"
	"github.com/moodiness/polyfin/internal/addons"
)

// fakeEclipse is an Eclipse addon serving two albums, one explicit track,
// an artist page, a search, and streams of a generated FLAC tone that
// expire once expiring is set, or fail with HTTP 502 once failing is set.
// content, set before it is installed, is its manifest's contentType: an
// audiobook addon's streams come with chapters; rowless, set before it is
// installed, leaves its manifest without catalog rows. queries records the
// settings every request carried, catalogs counts the requests for catalog
// rows.
type fakeEclipse struct {
	url      string
	content  string
	rowless  bool
	streams  atomic.Int32
	catalogs atomic.Int32
	expiring atomic.Bool
	failing  atomic.Bool
	mu       sync.Mutex
	queries  []string
}

func newFakeEclipse(t *testing.T, tone string) *fakeEclipse {
	t.Helper()
	f := &fakeEclipse{}
	var server *httptest.Server
	track := func(id, title, album, albumID string, explicit bool) map[string]any {
		return map[string]any{"id": id, "title": title, "artist": "Tone Quartet", "album": album, "albumId": albumID,
			"duration": 10, "artworkURL": server.URL + "/cover.jpg", "explicit": explicit, "format": "flac"}
	}
	albums := func() map[string]map[string]any {
		return map[string]map[string]any{
			"sine": {"id": "sine", "title": "Sine Studies", "artist": "Tone Quartet", "year": "2021", "artworkURL": server.URL + "/cover.jpg",
				"tracks": []any{track("t1", "First Light", "", "", false), track("t2", "Second Wind", "", "", false)}},
			"loud": {"id": "loud", "title": "Loud Songs", "artist": "Tone Quartet", "year": 2022, "artworkURL": server.URL + "/cover.jpg",
				"tracks": []any{track("t3", "Rude Words", "", "", true)}},
		}
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Settings travel with every resource request; the manifest
		// describes them.
		if r.URL.Path != "/cover.jpg" && r.URL.Path != "/tone.flac" && !strings.HasSuffix(r.URL.Path, "/manifest.json") {
			f.mu.Lock()
			f.queries = append(f.queries, r.URL.Query().Get("quality"))
			f.mu.Unlock()
		}
		answer := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		path := strings.TrimPrefix(r.URL.Path, "/token")
		if strings.HasPrefix(path, "/catalog") {
			f.catalogs.Add(1)
		}
		switch {
		case path == "/manifest.json":
			manifest := map[string]any{"id": "com.example.tones", "name": "Tones", "version": "1.0.0",
				"resources": []string{"search", "stream", "catalog", "settings"}, "types": []string{"track", "album", "artist"},
				"settings": []any{map[string]any{"key": "quality", "type": "select", "label": "Quality", "default": "high", "perNetwork": true,
					"options": []any{map[string]string{"value": "high", "label": "High"}, map[string]string{"value": "low", "label": "Low"}}}},
				"catalogs": []any{map[string]string{"id": "new", "type": "album", "name": "New Releases"},
					map[string]string{"id": "top", "type": "track", "name": "Top Songs"}}}
			if f.content != "" {
				manifest["contentType"] = f.content
			}
			if f.rowless {
				manifest["resources"], manifest["catalogs"] = []string{"search", "stream", "settings"}, []any{}
			}
			answer(manifest)
		case path == "/catalog/new":
			answer(map[string]any{"items": []any{albums()["sine"], albums()["loud"]}})
		case path == "/catalog/top":
			answer(map[string]any{"items": []any{track("t1", "First Light", "Sine Studies", "sine", false), track("t3", "Rude Words", "Loud Songs", "loud", true)}})
		case strings.HasPrefix(path, "/album/"):
			if album, ok := albums()[strings.TrimPrefix(path, "/album/")]; ok {
				answer(album)
				return
			}
			http.NotFound(w, r)
		case path == "/artist/quartet":
			answer(map[string]any{"id": "quartet", "name": "Tone Quartet", "artworkURL": server.URL + "/cover.jpg", "bio": "Four tones.",
				"genres": []string{"Electronic"}, "topTracks": []any{track("t1", "First Light", "Sine Studies", "sine", false)},
				"albums": []any{albums()["sine"], albums()["loud"]}})
		case path == "/search":
			answer(map[string]any{"tracks": []any{track("t2", "Second Wind", "Sine Studies", "sine", false)},
				"albums": []any{albums()["sine"]}, "artists": []any{map[string]string{"id": "quartet", "name": "Tone Quartet"}}})
		case strings.HasPrefix(path, "/stream/"):
			if f.failing.Load() {
				http.Error(w, "bad gateway", http.StatusBadGateway)
				return
			}
			n := f.streams.Add(1)
			reply := map[string]any{"url": fmt.Sprintf("%s/tone.flac?n=%d", server.URL, n), "codec": "flac", "container": "flac",
				"manifest": "none", "sampleRate": 44100, "bitDepth": 16}
			if f.content == "audiobook" {
				reply["chapters"] = []any{map[string]any{"title": "Opening", "startTime": 0}, map[string]any{"title": "Ending", "startTime": 5}}
			}
			if f.expiring.Load() {
				reply["expiresAt"] = time.Now().Add(-time.Hour).Unix()
			}
			answer(reply)
		case path == "/tone.flac":
			http.ServeFile(w, r, tone)
		case path == "/cover.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpeg"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	f.url = server.URL + "/token/"
	return f
}

// tone generates a ten-second stereo FLAC tone.
func tone(t *testing.T) (string, string) {
	t.Helper()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	path := filepath.Join(t.TempDir(), "tone.flac")
	if out, err := exec.Command(ffmpeg, "-nostdin", "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=10",
		"-ac", "2", "-c:a", "flac", path).CombinedOutput(); err != nil {
		t.Fatalf("tone: %v %s", err, out)
	}
	return ffmpeg, path
}

func TestEclipseAddonPlaysInJellyfinMusicApps(t *testing.T) {
	ffmpeg, flac := tone(t)
	s := newProbingServer(t, 10, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"))
	addon := newFakeEclipse(t, flac)
	installed, err := s.addons.Install(t.Context(), addons.Shared(), addon.url, false)
	if err != nil {
		t.Fatal(err)
	}
	// Its two rows and My music.
	if installed.Kind != addons.KindEclipse || len(installed.Manifest.Catalogs) != 3 {
		t.Fatalf("installed as %s with %d catalogs", installed.Kind, len(installed.Manifest.Catalogs))
	}
	if _, err := s.addons.SetSettings(t.Context(), addons.Shared(), installed.ID, map[string]string{"quality": "low"}); err != nil {
		t.Fatal(err)
	}
	s.user("listener", nil)
	child := s.user("child", func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{MaxRating: new(10)} })
	token, childToken := s.signIn("listener", "web"), s.signIn("child", "web")

	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	ids := map[string]string{}
	for _, view := range views.Items {
		if view.CollectionType != "music" {
			t.Errorf("%s is a %q library", view.Name, view.CollectionType)
		}
		ids[view.Name] = view.Id
	}
	albumsLibrary := ids["New Releases"]
	var albums QueryResult
	s.get(t, "/Items?ParentId="+albumsLibrary+"&IncludeItemTypes=MusicAlbum&Recursive=true&SortBy=SortName", token, &albums)
	if len(albums.Items) != 2 || albums.Items[0].Name != "Loud Songs" || albums.Items[0].Type != "MusicAlbum" || !albums.Items[0].IsFolder {
		t.Fatalf("albums: %+v", albums.Items)
	}
	var childAlbums QueryResult
	s.get(t, "/Items?ParentId="+albumsLibrary+"&IncludeItemTypes=MusicAlbum&Recursive=true", childToken, &childAlbums)
	if len(childAlbums.Items) != 1 || childAlbums.Items[0].Name != "Sine Studies" {
		t.Errorf("a user limited to a rating sees explicit albums: %+v", childAlbums.Items)
	}
	sine := albums.Items[1].Id
	var songs QueryResult
	s.get(t, "/Items?ParentId="+sine+"&Fields=AudioInfo", token, &songs)
	if len(songs.Items) != 2 || songs.Items[0].Type != "Audio" || songs.Items[0].MediaType != "Audio" || songs.Items[0].AlbumId != sine ||
		songs.Items[0].ArtistItems == nil || (*songs.Items[0].ArtistItems)[0].Name != "Tone Quartet" {
		t.Fatalf("songs: %+v", songs.Items)
	}
	var tracks QueryResult
	s.get(t, "/Items?ParentId="+albumsLibrary+"&IncludeItemTypes=Audio&Recursive=true", childToken, &tracks)
	if len(tracks.Items) != 2 {
		t.Errorf("the child's songs of the albums: %d", len(tracks.Items))
	}
	var found QueryResult
	s.get(t, "/Items?searchTerm=wind&IncludeItemTypes=Audio&Recursive=true", token, &found)
	if len(found.Items) != 1 || found.Items[0].Name != "Second Wind" {
		t.Errorf("search: %+v", found.Items)
	}
	var artists QueryResult
	s.get(t, "/Artists?ParentId="+albumsLibrary, token, &artists)
	if len(artists.Items) != 1 || artists.Items[0].Type != "MusicArtist" {
		t.Errorf("artists: %+v", artists.Items)
	}
	if status, _ := s.call(http.MethodGet, "/Audio/"+songs.Items[0].Id+"/Lyrics", app("web", token), nil); status != http.StatusNotFound {
		t.Errorf("lyrics: %d", status)
	}
	var mix QueryResult
	s.get(t, "/Items/"+songs.Items[0].Id+"/InstantMix", token, &mix)
	// The song, then the other songs of its album and of its artist.
	if len(mix.Items) != 3 || mix.Items[0].Id != songs.Items[0].Id {
		t.Errorf("instant mix: %+v", mix.Items)
	}

	// Direct play with a profile taking FLAC, decided without probing.
	song := songs.Items[0].Id
	profile := map[string]any{"MaxStreamingBitrate": 140000000,
		"DirectPlayProfiles":  []any{map[string]string{"Type": "Audio", "Container": "flac"}},
		"TranscodingProfiles": []any{map[string]string{"Type": "Audio", "Container": "mp3", "AudioCodec": "mp3", "Protocol": "http", "Context": "Streaming"}}}
	var info playbackInfoResponse
	status, body := s.call(http.MethodPost, "/Items/"+song+"/PlaybackInfo", app("web", token), map[string]any{"DeviceProfile": profile})
	if status != http.StatusOK || json.Unmarshal(body, &info) != nil || len(info.MediaSources) != 1 || !info.MediaSources[0].SupportsDirectPlay {
		t.Fatalf("direct play: %d %s", status, body)
	}
	// Without FLAC, the track converts with FFmpeg to MP3.
	profile["DirectPlayProfiles"] = []any{map[string]string{"Type": "Audio", "Container": "mp3"}}
	status, body = s.call(http.MethodPost, "/Items/"+song+"/PlaybackInfo", app("web", token), map[string]any{"DeviceProfile": profile})
	if status != http.StatusOK || json.Unmarshal(body, &info) != nil || info.MediaSources[0].SupportsDirectPlay ||
		!strings.Contains(info.MediaSources[0].TranscodingUrl, "AudioCodec=mp3") {
		t.Fatalf("conversion: %d %s", status, body)
	}
	response, err := http.Get(s.url + info.MediaSources[0].TranscodingUrl)
	if err != nil {
		t.Fatal(err)
	}
	converted := make([]byte, 3)
	_, _ = response.Body.Read(converted)
	response.Body.Close()
	if response.Header.Get("Content-Type") != "audio/mpeg" || !(string(converted) == "ID3" || converted[0] == 0xff) {
		t.Errorf("converted: %s %q", response.Header.Get("Content-Type"), converted)
	}

	// Every request carried the chosen setting.
	addon.mu.Lock()
	for _, quality := range addon.queries {
		if quality != "low" {
			t.Errorf("a request carried quality=%q", quality)
		}
	}
	addon.mu.Unlock()

	// An expired link is asked for again.
	addon.expiring.Store(true)
	before := addon.streams.Load()
	if _, err := s.library.Versions(t.Context(), child, mustID(t, songs.Items[1].Id)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.library.Versions(t.Context(), child, mustID(t, songs.Items[1].Id)); err != nil {
		t.Fatal(err)
	}
	if addon.streams.Load()-before != 2 {
		t.Errorf("an expired stream was not renewed: %d requests", addon.streams.Load()-before)
	}

	// Reports update the user's data and the activity log.
	report := map[string]any{"ItemId": song, "MediaSourceId": song, "PositionTicks": 0}
	s.call(http.MethodPost, "/Sessions/Playing", app("web", token), report)
	report["PositionTicks"] = 60_000_000
	s.call(http.MethodPost, "/Sessions/Playing/Stopped", app("web", token), report)
	var data UserItemData
	s.get(t, "/UserItems/"+song+"/UserData", token, &data)
	if !data.Played || data.PlayCount != 1 || data.PlaybackPositionTicks != 0 {
		t.Errorf("user data after playing a song: %+v", data)
	}
	entries, err := s.activity.Entries(t.Context(), activity.Query{})
	if err != nil || !strings.Contains(fmt.Sprint(entries), "Tone Quartet - First Light") {
		t.Errorf("activity: %v %v", entries, err)
	}
}
