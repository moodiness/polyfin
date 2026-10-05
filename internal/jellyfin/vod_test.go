package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
)

// vodProvider is an Xtream account (user / secret) with two movies, extra
// older ones, and a series, whose files are file, counting its details
// requests.
type vodProvider struct {
	url  string
	mu   sync.Mutex
	asks map[string]int
}

func (p *vodProvider) count(action string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.asks[action]
}

func newVODProvider(t *testing.T, file string, extra int) *vodProvider {
	t.Helper()
	p := &vodProvider{asks: map[string]int{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if strings.HasPrefix(r.URL.Path, "/movie/user/secret/") || strings.HasPrefix(r.URL.Path, "/series/user/secret/") {
			if file == "" {
				http.NotFound(w, r)
				return
			}
			http.ServeFile(w, r, file)
			return
		}
		if r.URL.Path != "/player_api.php" || query.Get("username") != "user" || query.Get("password") != "secret" {
			http.NotFound(w, r)
			return
		}
		p.mu.Lock()
		p.asks[query.Get("action")]++
		p.mu.Unlock()
		var answer any
		switch query.Get("action") {
		case "":
			answer = map[string]any{"user_info": map[string]any{"auth": 1, "allowed_output_formats": []string{"ts"}}}
		case "get_vod_categories":
			answer = []any{map[string]any{"category_id": "1", "category_name": "Action"}}
		case "get_series_categories":
			answer = []any{map[string]any{"category_id": "2", "category_name": "Drama"}}
		case "get_vod_streams":
			movies := []any{
				map[string]any{"name": "EN - Zeb Quest (2019) 4K", "stream_id": 41, "category_id": "1", "container_extension": "mkv", "added": "1700000000",
					"tmdb": "1000", "stream_icon": "https://posters.example/zeb.jpg"},
				map[string]any{"name": "EN - Orb Tale (2001)", "stream_id": 42, "category_id": "1", "container_extension": "mp4", "added": "1600000000"},
			}
			for i := range extra {
				movies = append(movies, map[string]any{"name": fmt.Sprintf("Filler %d", i), "stream_id": 1000 + i, "category_id": "1",
					"container_extension": "mkv", "added": fmt.Sprint(1500000000 - i)})
			}
			answer = movies
		case "get_series":
			answer = []any{map[string]any{"name": "Quill Days", "series_id": 7, "category_id": "2", "plot": "A listed show.", "last_modified": "1"}}
		case "get_vod_info":
			answer = map[string]any{"info": map[string]any{"description": "The provider's plot of " + query.Get("vod_id"), "duration_secs": 6000,
				"genre": "Adventure"}, "movie_data": map[string]any{"container_extension": "mkv"}}
		case "get_series_info":
			answer = map[string]any{"info": map[string]any{"plot": "A detailed show."}, "episodes": map[string]any{
				"1": []any{
					map[string]any{"id": "701", "episode_num": 1, "season": 1, "title": "Quill Days - S01E01 - Pilot", "container_extension": "mkv"},
					map[string]any{"id": "702", "episode_num": 2, "season": 1, "title": "Quill Days - S01E02 - Second", "container_extension": "mkv"},
				},
				"2": []any{map[string]any{"id": "711", "episode_num": 1, "season": 2, "title": "Quill Days - S02E01 - Back", "container_extension": "mkv"}},
			}}
		default:
			answer = []any{}
		}
		_ = json.NewEncoder(w).Encode(answer)
	}))
	t.Cleanup(server.Close)
	p.url = server.URL
	return p
}

// metadataAddon is a Stremio addon describing TMDB movies, counting its
// requests.
func metadataAddon(t *testing.T, asks *int) string {
	t.Helper()
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "test.metadata", "version": "1.0.0", "name": "Metadata", "types": []string{"movie"},
				"resources": []any{map[string]any{"name": "meta", "types": []string{"movie"}, "idPrefixes": []string{"tmdb:"}}}, "catalogs": []any{}})
		case "/meta/movie/tmdb:1000.json":
			mu.Lock()
			*asks++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": map[string]any{"id": "tmdb:1000", "type": "movie", "name": "Zeb Quest",
				"description": "The metadata addon's overview.", "poster": "https://images.example/zeb.jpg", "imdbRating": "7.9",
				"app_extras": map[string]any{"certification": "PG-13"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// views lists a user's views by collection type.
func (s testServer) viewsByType(t *testing.T, token string) map[string]string {
	t.Helper()
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	result := map[string]string{}
	for _, view := range views.Items {
		if view.CollectionType != "" {
			result[view.CollectionType] = view.Id
		}
	}
	return result
}

// An Xtream account's movies and series are libraries of ordinary
// titles in apps: listed and searched from the provider's lists alone,
// described with the provider's details once opened, and through a
// metadata addon when the source enriches them; played state survives a
// refresh; parental control sees them unrated.
func TestIPTVMoviesAndSeriesInApps(t *testing.T) {
	provider := newVODProvider(t, "", 0)
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	token := s.signIn("member", "tv")
	asks := 0
	if _, err := s.addons.Install(t.Context(), addons.Shared(), metadataAddon(t, &asks), false); err != nil {
		t.Fatal(err)
	}
	on := true
	addon, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Box", Account: iptv.Account{Kind: addons.KindXtream, Server: provider.url,
		Username: "user", Password: "secret"}, Options: &iptv.OptionsPatch{Movies: &on, Series: &on}}, false)
	if err != nil {
		t.Fatal(err)
	}
	views := s.viewsByType(t, token)
	if views["movies"] == "" || views["tvshows"] == "" {
		t.Fatalf("views: %v", views)
	}
	var movies QueryResult
	s.get(t, "/Items?ParentId="+views["movies"], token, &movies)
	if len(movies.Items) != 2 || movies.Items[0].Name != "Zeb Quest" || movies.Items[1].Name != "Orb Tale" {
		t.Fatalf("movies: %+v", movies.Items)
	}
	var found QueryResult
	s.get(t, "/Items?Recursive=true&IncludeItemTypes=Movie&searchTerm=orb", token, &found)
	if len(found.Items) != 1 || found.Items[0].Name != "Orb Tale" {
		t.Errorf("search: %+v", found.Items)
	}
	if provider.count("get_vod_info") != 0 || provider.count("get_series_info") != 0 || asks != 0 {
		t.Fatalf("details asked for listings: %v, metadata %d", provider.asks, asks)
	}

	// Opened: the provider's details, then the metadata addon's, once.
	zeb, orb := movies.Items[0].Id, movies.Items[1].Id
	var details BaseItemDto
	s.get(t, "/Users/"+member.ID.String()+"/Items/"+zeb, token, &details)
	if details.Overview == nil || *details.Overview != "The metadata addon's overview." || details.OfficialRating != "PG-13" ||
		details.ProductionYear == nil || *details.ProductionYear != 2019 {
		t.Errorf("an enriched movie: %+v", details)
	}
	s.get(t, "/Users/"+member.ID.String()+"/Items/"+orb, token, &details)
	if details.Overview == nil || *details.Overview != "The provider's plot of 42" {
		t.Errorf("a movie without a TMDB id: %+v", details.Overview)
	}
	s.get(t, "/Users/"+member.ID.String()+"/Items/"+zeb, token, &details)
	if provider.count("get_vod_info") != 2 || asks != 1 {
		t.Errorf("details asked %d times, metadata %d times", provider.count("get_vod_info"), asks)
	}
	versions, err := s.library.Versions(t.Context(), member, mustID(t, zeb))
	if err != nil || len(versions) != 1 || versions[0].Height != 2160 || !strings.Contains(versions[0].Name, "4K") ||
		versions[0].URL != provider.url+"/movie/user/secret/41.mkv" {
		t.Errorf("the movie's version: %+v %v", versions, err)
	}

	// Without enrichment, the provider's own description.
	off := false
	if err := s.iptv.Update(t.Context(), addons.Shared(), addon.ID, iptv.Changes{Options: &iptv.OptionsPatch{Enrichment: &off}}, false); err != nil {
		t.Fatal(err)
	}
	s.get(t, "/Users/"+member.ID.String()+"/Items/"+zeb, token, &details)
	if details.Overview == nil || *details.Overview != "The provider's plot of 41" {
		t.Errorf("a movie without enrichment: %+v", details.Overview)
	}

	// A series' seasons and episodes come from its details.
	var shows QueryResult
	s.get(t, "/Items?ParentId="+views["tvshows"], token, &shows)
	if len(shows.Items) != 1 || provider.count("get_series_info") != 0 {
		t.Fatalf("series: %+v after %d details", shows.Items, provider.count("get_series_info"))
	}
	var seasons, episodes QueryResult
	s.get(t, "/Shows/"+shows.Items[0].Id+"/Seasons?userId="+member.ID.String(), token, &seasons)
	s.get(t, "/Shows/"+shows.Items[0].Id+"/Episodes?userId="+member.ID.String()+"&seasonId="+seasons.Items[0].Id, token, &episodes)
	var titles []string
	for _, episode := range episodes.Items {
		titles = append(titles, episode.Name)
	}
	if len(seasons.Items) != 2 || !slices.Equal(titles, []string{"Pilot", "Second"}) || provider.count("get_series_info") != 1 {
		t.Errorf("seasons %d, episodes %q, after %d details", len(seasons.Items), titles, provider.count("get_series_info"))
	}
	episode, _ := accounts.ParseID(episodes.Items[1].Id)
	if versions, err := s.library.Versions(t.Context(), member, episode); err != nil || len(versions) != 1 ||
		versions[0].URL != provider.url+"/series/user/secret/702.mkv" {
		t.Errorf("an episode's version: %+v %v", versions, err)
	}

	// Played state keeps its title across a refresh.
	if status, _ := s.call(http.MethodPost, "/Users/"+member.ID.String()+"/PlayedItems/"+orb, app("tv", token), nil); status != http.StatusOK {
		t.Fatalf("marking played: %d", status)
	}
	if err := s.iptv.Refresh(t.Context(), addons.Shared(), addon.ID, false); err != nil {
		t.Fatal(err)
	}
	s.get(t, "/Items?ParentId="+views["movies"], token, &movies)
	if len(movies.Items) != 2 || movies.Items[1].Id != orb || !movies.Items[1].UserData.Played {
		t.Errorf("played after a refresh: %+v", movies.Items)
	}

	// Unrated, as the provider gives no rating: a user blocking unrated
	// movies does not see them.
	s.user("child", func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{BlockUnrated: []string{"Movie"}} })
	child := s.signIn("child", "tv")
	childViews := s.viewsByType(t, child)
	var hidden QueryResult
	s.get(t, "/Items?ParentId="+childViews["movies"], child, &hidden)
	if len(hidden.Items) != 0 {
		t.Errorf("unrated movies listed to a child: %+v", hidden.Items)
	}
}

// vodFile makes eight seconds of H.264 and AAC in container (by its
// extension) with the FFmpeg of POLYFIN_TEST_FFMPEG, skipping the test
// without one.
func vodFile(t *testing.T, name string) (string, string) {
	t.Helper()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	file := filepath.Join(t.TempDir(), name)
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24:duration=20",
		"-f", "lavfi", "-i", "sine=duration=20", "-c:v", "libx264", "-g", "48", "-pix_fmt", "yuv420p", "-c:a", "aac", file)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	return ffmpeg, file
}

// vodPlayback adds an Xtream source of movies served as file to a
// probing test server, and asks PlaybackInfo for its first movie.
func vodPlayback(t *testing.T, ffmpeg, file string, request map[string]any, user func(*accounts.UserChanges)) (testServer, string, playbackInfoResponse) {
	t.Helper()
	provider := newVODProvider(t, file, 0)
	s := newProbingServer(t, 10, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"))
	member := s.user("member", user)
	token := s.signIn("member", "tv")
	on, off := true, false
	if _, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Box", Account: iptv.Account{Kind: addons.KindXtream, Server: provider.url,
		Username: "user", Password: "secret"}, Options: &iptv.OptionsPatch{Movies: &on, LiveTv: &off}}, false); err != nil {
		t.Fatal(err)
	}
	var movies QueryResult
	s.get(t, "/Items?ParentId="+s.viewsByType(t, token)["movies"], token, &movies)
	if len(movies.Items) != 2 {
		t.Fatalf("movies: %+v", movies.Items)
	}
	body := map[string]any{"UserId": member.ID.String()}
	for key, value := range request {
		body[key] = value
	}
	status, data := s.call(http.MethodPost, "/Items/"+movies.Items[0].Id+"/PlaybackInfo", app("tv", token), body)
	var info playbackInfoResponse
	if err := json.Unmarshal(data, &info); status != http.StatusOK || err != nil {
		t.Fatalf("PlaybackInfo: %d %s", status, data)
	}
	return s, token, info
}

func deviceProfile(t *testing.T, name string) json.RawMessage {
	t.Helper()
	profile, err := os.ReadFile(filepath.Join(playbackFixtures, "profiles", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

// fetchSegments reads a remux's master playlist, its media playlist, and
// segments, from the first to the one at n, as apps seeking do.
func fetchSegments(t *testing.T, s testServer, transcoding string, segments ...int) {
	t.Helper()
	get := func(path string) (int, string) {
		status, body := s.call(http.MethodGet, path, "", nil)
		return status, string(body)
	}
	status, master := get(transcoding)
	if status != http.StatusOK || !strings.Contains(master, ".m3u8") {
		t.Fatalf("master playlist: %d %s", status, master)
	}
	var media string
	for line := range strings.Lines(master) {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			media = line
			break
		}
	}
	base := transcoding[:strings.Index(transcoding, "master.m3u8")]
	status, playlist := get(base + media)
	var listed []string
	for line := range strings.Lines(playlist) {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			listed = append(listed, line)
		}
	}
	if status != http.StatusOK || !strings.Contains(playlist, "#EXT-X-ENDLIST") || len(listed) < 3 {
		t.Fatalf("media playlist: %d %s", status, playlist)
	}
	for _, n := range segments {
		segment := listed[n]
		if !strings.HasPrefix(segment, "/") && !strings.HasPrefix(segment, "http") {
			segment = base + segment
		}
		if status, data := get(segment); status != http.StatusOK || len(data) == 0 {
			t.Errorf("segment %d: %d, %d bytes", n, status, len(data))
		}
	}
}

// A provider's movie file plays through the usual path: analyzed by
// ffprobe, remuxed for an app that cannot play its container, converted
// when the app asks for it.
func TestIPTVMoviePlaysThroughFFmpeg(t *testing.T) {
	ffmpeg, file := vodFile(t, "movie.mkv")
	s, _, info := vodPlayback(t, ffmpeg, file, map[string]any{"DeviceProfile": deviceProfile(t, "swiftfin-native")}, nil)
	if len(info.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %+v", info)
	}
	source := info.MediaSources[0]
	var height int
	for _, stream := range source.MediaStreams {
		if stream.Type == "Video" && stream.Height != nil {
			height = *stream.Height
		}
	}
	if height != 180 || !source.SupportsTranscoding || source.TranscodingUrl == "" {
		t.Fatalf("an MKV file for an MP4 player is not remuxed: %+v", source)
	}
	fetchSegments(t, s, source.TranscodingUrl, 0, 2)

	s, _, info = vodPlayback(t, ffmpeg, file, map[string]any{"DeviceProfile": deviceProfile(t, "jellyfin-web-chrome"), "EnableDirectPlay": false,
		"AllowVideoStreamCopy": false}, nil)
	if len(info.MediaSources) != 1 || !info.MediaSources[0].SupportsTranscoding || info.MediaSources[0].TranscodingUrl == "" {
		t.Fatalf("a conversion asked for: %+v", info)
	}
	fetchSegments(t, s, info.MediaSources[0].TranscodingUrl, 0)
}

// An MPEG-TS movie, which has no index to remux it by, plays converted,
// cut every few seconds and seekable, as Jellyfin plays such files; a
// user who may not have video converted is told no stream suits, rather
// than offered one that fails.
func TestIPTVTransportStreamMoviesPlayConverted(t *testing.T) {
	ffmpeg, file := vodFile(t, "movie.ts")
	s, _, info := vodPlayback(t, ffmpeg, file, map[string]any{"DeviceProfile": deviceProfile(t, "jellyfin-web-chrome")}, nil)
	if len(info.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %+v", info)
	}
	source := info.MediaSources[0]
	if source.SupportsDirectPlay || !source.SupportsTranscoding || source.TranscodingUrl == "" || source.RunTimeTicks == nil ||
		*source.RunTimeTicks < 19*10_000_000 {
		t.Fatalf("an MPEG-TS movie: %+v", source)
	}
	fetchSegments(t, s, source.TranscodingUrl, 0, 2)

	_, _, refused := vodPlayback(t, ffmpeg, file, map[string]any{"DeviceProfile": deviceProfile(t, "jellyfin-web-chrome")},
		func(c *accounts.UserChanges) { c.VideoTranscoding = new(false) })
	if len(refused.MediaSources) != 0 {
		t.Errorf("an MPEG-TS movie offered to a user who may not convert video: %+v", refused.MediaSources)
	}
}

// A provider's movies are paged and counted whole in apps, beyond the
// catalog limit addons' catalogs are read to.
func TestIPTVLibrariesArePagedWhole(t *testing.T) {
	provider := newVODProvider(t, "", 2498)
	s := newTestServer(t, 10)
	s.user("member", nil)
	token := s.signIn("member", "tv")
	on := true
	if _, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Box", Account: iptv.Account{Kind: addons.KindXtream, Server: provider.url,
		Username: "user", Password: "secret"}, Options: &iptv.OptionsPatch{Movies: &on}}, false); err != nil {
		t.Fatal(err)
	}
	views := s.viewsByType(t, token)
	var page QueryResult
	s.get(t, "/Items?ParentId="+views["movies"]+"&StartIndex=2450&Limit=100", token, &page)
	if page.TotalRecordCount != 2500 || len(page.Items) != 50 || page.Items[49].Name != "Filler 2497" {
		t.Errorf("the last page: %d of %d", len(page.Items), page.TotalRecordCount)
	}
	s.get(t, "/Items?ParentId="+views["movies"]+"&Limit=10", token, &page)
	if page.TotalRecordCount != 2500 || len(page.Items) != 10 || page.Items[0].Name != "Zeb Quest" {
		t.Errorf("the first page: %d of %d", len(page.Items), page.TotalRecordCount)
	}
}
