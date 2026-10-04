package jellyfin

import (
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/thumbnails"
)

// The playlist of a version's tiles is written as Jellyfin writes it.
func TestTrickplayPlaylistFormat(t *testing.T) {
	version, _ := accounts.ParseID("0123456789abcdef0123456789abcdef")
	got := trickplayPlaylist(thumbnails.Info{Width: 320, Height: 180, TileWidth: 10, TileHeight: 10, ThumbnailCount: 250, Interval: 10000},
		version, "token")
	want := `#EXTM3U
#EXT-X-TARGETDURATION:3
#EXT-X-VERSION:7
#EXT-X-MEDIA-SEQUENCE:1
#EXT-X-PLAYLIST-TYPE:VOD
#EXT-X-IMAGES-ONLY
#EXTINF:1000,
#EXT-X-TILES:RESOLUTION=320x180,LAYOUT=10x10,DURATION=10
0.jpg?MediaSourceId=0123456789abcdef0123456789abcdef&ApiKey=token
#EXTINF:1000,
#EXT-X-TILES:RESOLUTION=320x180,LAYOUT=10x10,DURATION=10
1.jpg?MediaSourceId=0123456789abcdef0123456789abcdef&ApiKey=token
#EXTINF:500,
#EXT-X-TILES:RESOLUTION=320x180,LAYOUT=10x10,DURATION=10
2.jpg?MediaSourceId=0123456789abcdef0123456789abcdef&ApiKey=token
#EXT-X-ENDLIST
`
	if got != want {
		t.Errorf("playlist\n%s\nwant\n%s", got, want)
	}
	// Fractions of a second, to the millisecond as .NET's "0.###" writes
	// them.
	got = trickplayPlaylist(thumbnails.Info{Width: 240, Height: 136, TileWidth: 10, TileHeight: 10, ThumbnailCount: 7, Interval: 2345},
		version, "token")
	if !strings.Contains(got, "#EXT-X-TARGETDURATION:1\n") || !strings.Contains(got, "#EXTINF:16.415,\n") ||
		!strings.Contains(got, ",DURATION=2.345\n") {
		t.Errorf("fractional playlist\n%s", got)
	}
}

// trickplaySetup is a movie whose first version has thumbnails.
type trickplaySetup struct {
	playbackSetup
	info  thumbnails.Info
	tiles [][]byte
}

func withThumbnails(t *testing.T) trickplaySetup {
	t.Helper()
	p := playing(t)
	p.setting(t, func(s *accounts.Settings) { s.Trickplay, s.ChapterImages = true, true })
	s := trickplaySetup{playbackSetup: p,
		info:  thumbnails.Info{Width: 320, Height: 180, TileWidth: 10, TileHeight: 10, ThumbnailCount: 150, Interval: 10000, Bandwidth: 2400},
		tiles: [][]byte{[]byte("\xff\xd8 tile 0"), []byte("\xff\xd8 tile 1")}}
	movie, _ := accounts.ParseID(p.movie)
	if err := p.handler.Thumbnails.SaveTrickplay(t.Context(), p.versions[0].ID, movie, s.info, s.tiles); err != nil {
		t.Fatal(err)
	}
	return s
}

// Items describe their versions' thumbnails as Jellyfin does, by media
// source and width, in details and in listings that ask for them.
func TestTrickplayInItems(t *testing.T) {
	s := withThumbnails(t)
	want := map[string]map[string]map[string]any{s.versions[0].ID.String(): {"320": {
		"Width": float64(320), "Height": float64(180), "TileWidth": float64(10), "TileHeight": float64(10),
		"ThumbnailCount": float64(150), "Interval": float64(10000), "Bandwidth": float64(2400)}}}
	trickplay := func(what string, body []byte) (any, bool) {
		t.Helper()
		var item map[string]json.RawMessage
		if err := json.Unmarshal(body, &item); err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		raw, ok := item["Trickplay"]
		var value map[string]map[string]map[string]any
		_ = json.Unmarshal(raw, &value)
		return value, ok
	}
	userItem := "/Users/" + s.user.ID.String() + "/Items/"
	_, body := s.call(http.MethodGet, userItem+s.movie, app("tv", s.token), nil)
	if got, _ := trickplay("details", body); !reflect.DeepEqual(got, any(want)) {
		t.Errorf("details: %v", got)
	}
	// jellyfin-web opens the version played as an item.
	_, body = s.call(http.MethodGet, userItem+s.versions[0].ID.String(), app("tv", s.token), nil)
	if got, _ := trickplay("version", body); !reflect.DeepEqual(got, any(want)) {
		t.Errorf("version: %v", got)
	}
	var views QueryResult
	s.get(t, "/UserViews", s.token, &views)
	listing := func(query string) []byte {
		_, body := s.call(http.MethodGet, "/Items?Ids="+s.movie+query, app("tv", s.token), nil)
		var page struct{ Items []json.RawMessage }
		_ = json.Unmarshal(body, &page)
		if len(page.Items) != 1 {
			t.Fatalf("listing: %s", body)
		}
		return page.Items[0]
	}
	if got, _ := trickplay("listing", listing("&Fields=Chapters,MediaSources,Trickplay")); !reflect.DeepEqual(got, any(want)) {
		t.Errorf("listing asking for them: %v", got)
	}
	if _, sent := trickplay("listing without the field", listing("")); sent {
		t.Error("a listing not asking for thumbnails got them")
	}
	// Turned off, they are hidden.
	s.setting(t, func(settings *accounts.Settings) { settings.Trickplay = false })
	_, body = s.call(http.MethodGet, userItem+s.movie, app("tv", s.token), nil)
	if got, sent := trickplay("turned off", body); !sent || len(got.(map[string]map[string]map[string]any)) != 0 {
		t.Errorf("turned off: %v", got)
	}
}

// The playlist and the tiles are served to users who may open the title,
// within their hours, with a token in the header or the URL.
func TestTrickplayRoutes(t *testing.T) {
	s := withThumbnails(t)
	base := "/Videos/" + s.movie + "/Trickplay/320/"
	status, body := s.call(http.MethodGet, base+"tiles.m3u8", app("tv", s.token), nil)
	if want := trickplayPlaylist(s.info, s.versions[0].ID, s.token); status != http.StatusOK || string(body) != want {
		t.Errorf("playlist: %d\n%s", status, body)
	}
	// The tile URLs of the playlist, as players resolve them.
	for i, tile := range s.tiles {
		path := base + []string{"0", "1"}[i] + ".jpg?MediaSourceId=" + s.versions[0].ID.String() + "&ApiKey=" + s.token
		response, data := fetchURL(t, s.url+path, nil)
		if response.StatusCode != http.StatusOK || string(data) != string(tile) || response.Header.Get("Content-Type") != "image/jpeg" {
			t.Errorf("tile %d: %d %q %q", i, response.StatusCode, data, response.Header.Get("Content-Type"))
		}
	}
	// By the version opened as an item, or without a media source: the
	// title's thumbnails.
	for _, path := range []string{"/Videos/" + s.versions[0].ID.String() + "/Trickplay/320/1.jpg", base + "1.jpg"} {
		if status, data := s.call(http.MethodGet, path, app("tv", s.token), nil); status != http.StatusOK || string(data) != string(s.tiles[1]) {
			t.Errorf("%s: %d %q", path, status, data)
		}
	}
	for _, tc := range []struct {
		path, authorization string
		status              int
	}{
		{base + "2.jpg", app("tv", s.token), http.StatusNotFound},
		{"/Videos/" + s.movie + "/Trickplay/480/0.jpg", app("tv", s.token), http.StatusNotFound},
		{base + "0.jpg?MediaSourceId=" + s.versions[1].ID.String(), app("tv", s.token), http.StatusNotFound},
		{"/Videos/" + s.remote + "/Trickplay/320/0.jpg", app("tv", s.token), http.StatusNotFound},
		{base + "0.jpg", "", http.StatusUnauthorized},
		{base + "tiles.m3u8", app("tv", "wrong"), http.StatusUnauthorized},
		{base + "first.jpg", app("tv", s.token), http.StatusBadRequest},
		{"/Videos/" + s.movie + "/Trickplay/wide/0.jpg", app("tv", s.token), http.StatusBadRequest},
	} {
		if status, data := s.call(http.MethodGet, tc.path, tc.authorization, nil); status != tc.status {
			t.Errorf("%s: %d %s, want %d", tc.path, status, data, tc.status)
		}
	}

	// A user whose blocked genres hide the title gets none.
	kidUser := s.testServer.user("kid", func(c *accounts.UserChanges) { c.BlockedGenres = &[]string{"drama"} })
	kid := s.signIn("kid", "tablet")
	for _, file := range []string{"tiles.m3u8", "0.jpg"} {
		if status, _ := s.call(http.MethodGet, base+file, app("tablet", kid), nil); status != http.StatusNotFound {
			t.Errorf("%s for a user who may not open the title: %d", file, status)
		}
	}
	// An API key reads them as no one, or as the user it names.
	_, key, err := s.store.CreateAPIKey(t.Context(), "Script")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		query  string
		status int
	}{
		{"", http.StatusOK},
		{"&userId=" + s.user.ID.String(), http.StatusOK},
		{"&userId=" + kidUser.ID.String(), http.StatusNotFound},
	} {
		if status, _ := s.call(http.MethodGet, base+"0.jpg?ApiKey="+key+tc.query, "", nil); status != tc.status {
			t.Errorf("an API key%s: %d, want %d", tc.query, status, tc.status)
		}
	}
	// Neither does a user outside their hours.
	if _, err := s.store.UpdateUser(t.Context(), s.user.ID, accounts.UserChanges{
		AccessSchedules: &[]accounts.AccessSchedule{{Day: "Weekday", StartHour: 9, EndHour: 17}},
	}, nil); err != nil {
		t.Fatal(err)
	}
	var clock atomic.Pointer[time.Time]
	sunday := time.Date(2026, time.October, 4, 20, 0, 0, 0, time.Local)
	clock.Store(&sunday)
	s.handler.now = func() time.Time { return *clock.Load() }
	if status, _ := s.call(http.MethodGet, base+"0.jpg?ApiKey="+s.token, "", nil); status != http.StatusForbidden {
		t.Errorf("outside the hours: %d", status)
	}
	// Turned off, they are not served.
	s.handler.now = time.Now
	if _, err := s.store.UpdateUser(t.Context(), s.user.ID, accounts.UserChanges{AccessSchedules: &[]accounts.AccessSchedule{}}, nil); err != nil {
		t.Fatal(err)
	}
	s.setting(t, func(settings *accounts.Settings) { settings.Trickplay = false })
	if status, _ := s.call(http.MethodGet, base+"0.jpg", app("tv", s.token), nil); status != http.StatusNotFound {
		t.Errorf("turned off: %d", status)
	}
}

// Chapters whose image was made carry its tag, and the image is served as
// Jellyfin serves images, by the item or the version opened.
func TestChapterImages(t *testing.T) {
	s := withThumbnails(t)
	probe, err := os.ReadFile("testdata/jellyfin-12.1/chapters/probe.json")
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := media.Parse(probe)
	if err != nil || len(analysis.Chapters) < 2 {
		t.Fatalf("recorded analysis: %d chapters, %v", len(analysis.Chapters), err)
	}
	stored, _ := json.Marshal(analysis)
	if _, err := s.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", s.versions[0].ID, stored); err != nil {
		t.Fatal(err)
	}
	chapters := func() []map[string]any {
		t.Helper()
		var item struct{ Chapters []map[string]any }
		_, body := s.call(http.MethodGet, "/Users/"+s.user.ID.String()+"/Items/"+s.movie, app("tv", s.token), nil)
		if err := json.Unmarshal(body, &item); err != nil {
			t.Fatal(err)
		}
		return item.Chapters
	}
	for _, chapter := range chapters() {
		if _, tagged := chapter["ImageTag"]; tagged || chapter["ImageDateModified"] != "0001-01-01T00:00:00.0000000Z" {
			t.Errorf("a chapter without an image: %v", chapter)
		}
	}
	movie, _ := accounts.ParseID(s.movie)
	images := make([][]byte, len(analysis.Chapters))
	for i := range images {
		images[i] = []byte("\xff\xd8 chapter " + string(rune('0'+i)))
	}
	if err := s.handler.Thumbnails.SaveChapterImages(t.Context(), s.versions[0].ID, movie, images); err != nil {
		t.Fatal(err)
	}
	got := chapters()
	if len(got) != len(images) {
		t.Fatalf("chapters %v", got)
	}
	for i, chapter := range got {
		tag, _ := chapter["ImageTag"].(string)
		if tag == "" || chapter["ImageDateModified"] == "0001-01-01T00:00:00.0000000Z" {
			t.Errorf("chapter %d: %v", i, chapter)
			continue
		}
		// jellyfin-web asks by the item, or by the version playing, with
		// the tag and no credentials.
		for _, id := range []string{s.movie, s.versions[0].ID.String()} {
			response, data := fetchURL(t, s.url+"/Items/"+id+"/Images/Chapter/"+string(rune('0'+i))+"?maxWidth=400&tag="+tag, nil)
			if response.StatusCode != http.StatusOK || string(data) != string(images[i]) || response.Header.Get("ETag") != `"`+tag+`"` ||
				response.Header.Get("Content-Type") != "image/jpeg" {
				t.Errorf("chapter %d by %s: %d %q", i, id, response.StatusCode, data)
			}
		}
	}
	if response, _ := fetchURL(t, s.url+"/Items/"+s.movie+"/Images/Chapter/"+string(rune('0'+len(images))), nil); response.StatusCode != http.StatusNotFound {
		t.Errorf("a chapter past the last: %d", response.StatusCode)
	}
	s.setting(t, func(settings *accounts.Settings) { settings.ChapterImages = false })
	if _, tagged := chapters()[0]["ImageTag"]; tagged {
		t.Error("turned off, chapters keep their tags")
	}
	if response, _ := fetchURL(t, s.url+"/Items/"+s.movie+"/Images/Chapter/0", nil); response.StatusCode != http.StatusNotFound {
		t.Errorf("turned off: %d", response.StatusCode)
	}
}

// A version that starts playing gets its thumbnails in the background,
// read from its source's keyframes once its playback stopped.
func TestPlaybackStartMakesThumbnails(t *testing.T) {
	if os.Getenv("POLYFIN_TEST_FFMPEG") == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	p := playing(t)
	p.setting(t, func(s *accounts.Settings) { s.Trickplay, s.TrickplayInterval = true, 5 })
	// The first version serves forced.mkv: 15 s of 64x64 video.
	analysis := media.Analysis{Format: "matroska,webm", Duration: 15008 * time.Millisecond,
		Streams: []media.Stream{{Index: 0, Type: "audio", Codec: "opus"}, {Index: 1, Type: "video", Codec: "h264", Width: 64, Height: 64}}}
	stored, _ := json.Marshal(analysis)
	if _, err := p.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", p.versions[0].ID, stored); err != nil {
		t.Fatal(err)
	}
	if status, data := p.call(http.MethodPost, "/Sessions/Playing", app("tv", p.token),
		map[string]any{"ItemId": p.movie, "MediaSourceId": p.versions[0].ID.String(), "PositionTicks": 0, "PlayMethod": "DirectPlay"}); status != http.StatusNoContent {
		t.Fatalf("report: %d %s", status, data)
	}
	movie, _ := accounts.ParseID(p.movie)
	// Nothing is read from the source while the title plays.
	time.Sleep(300 * time.Millisecond)
	if manifest, _ := p.handler.Thumbnails.Manifest(t.Context(), movie); len(manifest) != 0 {
		t.Fatalf("thumbnails made during the playback: %v", manifest)
	}
	if status, data := p.call(http.MethodPost, "/Sessions/Playing/Stopped", app("tv", p.token),
		map[string]any{"ItemId": p.movie, "MediaSourceId": p.versions[0].ID.String(), "PositionTicks": 50_000_000}); status != http.StatusNoContent {
		t.Fatalf("stop report: %d %s", status, data)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		manifest, err := p.handler.Thumbnails.Manifest(t.Context(), movie)
		if err != nil {
			t.Fatal(err)
		}
		if info, ok := manifest[p.versions[0].ID][320]; ok {
			if info.ThumbnailCount != 4 || info.Height != 320 {
				t.Errorf("thumbnails %+v", info)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Error("no thumbnails were made")
}
