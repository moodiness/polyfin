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
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
)

// vodProvider is an Xtream account (user / secret) with two movies and a
// series, whose files are file, counting its details requests.
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

func newVODProvider(t *testing.T, file string) *vodProvider {
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
			answer = []any{
				map[string]any{"name": "EN - Zeb Quest (2019) 4K", "stream_id": 41, "category_id": "1", "container_extension": "mkv", "added": "1700000000",
					"tmdb": "1000", "stream_icon": "https://posters.example/zeb.jpg"},
				map[string]any{"name": "EN - Orb Tale (2001)", "stream_id": 42, "category_id": "1", "container_extension": "mkv", "added": "1600000000"},
			}
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
	provider := newVODProvider(t, "")
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

// A provider's movie file plays through the usual path: analyzed by
// ffprobe, then remuxed or sent as it is, with FFmpeg.
func TestIPTVMoviePlaysThroughFFmpeg(t *testing.T) {
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	file := filepath.Join(t.TempDir(), "movie.mkv")
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24:duration=8",
		"-f", "lavfi", "-i", "sine=duration=8", "-c:v", "libx264", "-g", "24", "-pix_fmt", "yuv420p", "-c:a", "aac", file)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	provider := newVODProvider(t, file)
	s := newProbingServer(t, 10, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"))
	member := s.user("member", nil)
	token := s.signIn("member", "tv")
	on, off := true, false
	if _, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Box", Account: iptv.Account{Kind: addons.KindXtream, Server: provider.url,
		Username: "user", Password: "secret"}, Options: &iptv.OptionsPatch{Movies: &on, LiveTv: &off}}, false); err != nil {
		t.Fatal(err)
	}
	views := s.viewsByType(t, token)
	var movies QueryResult
	s.get(t, "/Items?ParentId="+views["movies"], token, &movies)
	if len(movies.Items) != 2 {
		t.Fatalf("movies: %+v", movies.Items)
	}
	profile, err := os.ReadFile(filepath.Join(playbackFixtures, "profiles", "swiftfin-native.json"))
	if err != nil {
		t.Fatal(err)
	}
	status, body := s.call(http.MethodPost, "/Items/"+movies.Items[0].Id+"/PlaybackInfo", app("tv", token),
		map[string]any{"UserId": member.ID.String(), "DeviceProfile": json.RawMessage(profile)})
	var info playbackInfoResponse
	if err := json.Unmarshal(body, &info); status != http.StatusOK || err != nil || len(info.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %d %s", status, body)
	}
	source := info.MediaSources[0]
	var height int
	for _, stream := range source.MediaStreams {
		if stream.Type == "Video" && stream.Height != nil {
			height = *stream.Height
		}
	}
	if height != 180 || source.Container == "" {
		t.Fatalf("the analyzed file: %+v", source)
	}
	if source.TranscodingUrl == "" {
		t.Fatalf("an MKV file for an MP4 player is not remuxed: %+v", source)
	}
	get := func(path string) (int, string) {
		status, body := s.call(http.MethodGet, path, "", nil)
		return status, string(body)
	}
	status, master := get(source.TranscodingUrl)
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
	base := source.TranscodingUrl[:strings.Index(source.TranscodingUrl, "master.m3u8")]
	deadline := time.Now().Add(30 * time.Second)
	var segment string
	for time.Now().Before(deadline) {
		status, playlist := get(base + media)
		for line := range strings.Lines(playlist) {
			if line = strings.TrimSpace(line); status == http.StatusOK && line != "" && !strings.HasPrefix(line, "#") {
				segment = line
				break
			}
		}
		if segment != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if segment == "" {
		t.Fatal("no segment listed")
	}
	if !strings.HasPrefix(segment, "/") && !strings.HasPrefix(segment, "http") {
		segment = base + segment
	}
	if status, data := get(segment); status != http.StatusOK || len(data) == 0 {
		t.Errorf("first segment %s: %d, %d bytes", segment, status, len(data))
	}
	_ = fmt.Sprint
}
