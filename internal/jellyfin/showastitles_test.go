package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// vaultAddon is a fake addon of a store's downloads, answering as an
// addon that lists them under a type of its own does: its catalog's
// titles are downloads, whose descriptions list their files as videos,
// numbered by season and episode as their names tell, or not at all, and
// a download without files; the stream resource answers one stream for
// each video, and for the download without files. Every file is clip. It
// counts the stream requests by identifier.
type vaultAddon struct {
	url string
	mu  sync.Mutex
	// asks counts the stream requests by identifier.
	asks map[string]int
}

const (
	vaultShow  = "vault:store:ridge"
	vaultLoose = "vault:store:loose"
	vaultLone  = "vault:store:lone"
	// vaultSingle is a download of one file its name numbers nothing of,
	// a movie's; vaultEpisode one of a single numbered episode.
	vaultSingle  = "vault:store:single"
	vaultEpisode = "vault:store:episode"
)

// vaultShowVideos are the show's videos in its description's order, with
// the place they are shown at, season and episode: the second episode
// first, then the first, a video its name numbers nothing of, which comes
// after the first season's, and the second season's.
var vaultShowVideos = []struct {
	id, title       string
	season, episode int
	shownAt         [2]int
}{
	{vaultShow + ":1", "Ridge.Show.S01E02.mkv", 1, 2, [2]int{1, 2}},
	{vaultShow + ":0", "Ridge.Show.S01E01.mkv", 1, 1, [2]int{1, 1}},
	{vaultShow + ":3", "Ridge.Show.Extras.mkv", 0, 0, [2]int{1, 3}},
	{vaultShow + ":2", "Ridge.Show.S02E01.mkv", 2, 1, [2]int{2, 1}},
}

func newVaultAddon(t *testing.T, clip string) *vaultAddon {
	t.Helper()
	a := &vaultAddon{asks: map[string]int{}}
	var server *httptest.Server
	describe := func(id string) (stremio.Meta, bool) {
		switch id {
		case vaultShow:
			meta := stremio.Meta{ID: vaultShow, Type: "library", Name: "Ridge Show", Description: "A download"}
			for _, v := range vaultShowVideos {
				meta.Videos = append(meta.Videos, stremio.Video{ID: v.id, Title: v.title, Season: stremio.Number(v.season),
					Episode: stremio.Number(v.episode)})
			}
			return meta, true
		case vaultLoose:
			meta := stremio.Meta{ID: vaultLoose, Type: "library", Name: "Loose Files", Description: "A download"}
			for n := range 3 {
				meta.Videos = append(meta.Videos, stremio.Video{ID: fmt.Sprintf("%s:%d", vaultLoose, n), Title: fmt.Sprintf("Loose.Part.%d.mkv", n+1)})
			}
			return meta, true
		case vaultLone:
			return stremio.Meta{ID: vaultLone, Type: "library", Name: "Lone Clip", Description: "A file"}, true
		case vaultSingle:
			return stremio.Meta{ID: vaultSingle, Type: "library", Name: "Single Movie", Videos: []stremio.Video{
				{ID: vaultSingle + ":0", Title: "Single.Movie.2024.1080p.mkv"}}}, true
		case vaultEpisode:
			return stremio.Meta{ID: vaultEpisode, Type: "library", Name: "Episode Only", Videos: []stremio.Video{
				{ID: vaultEpisode + ":0", Title: "Episode.Only.S03E05.mkv", Season: 3, Episode: 5}}}, true
		}
		return stremio.Meta{}, false
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "org.example.vault", Name: "Vault", Version: "1",
				Types: []string{"library"},
				Resources: []stremio.Resource{
					{Name: "stream", Types: []string{"library"}, IDPrefixes: []string{"vault:"}},
					{Name: "catalog", Types: []string{"library"}},
					{Name: "meta", Types: []string{"library"}, IDPrefixes: []string{"vault:"}},
				},
				Catalogs: []stremio.Catalog{{Type: "library", ID: "f00d.vault::library.store", Name: "Vault", Extra: []stremio.Extra{
					{Name: "search"}, {Name: "skip"}}}}})
		case strings.HasPrefix(path, "/catalog/library/"):
			if strings.Contains(path, "skip=") {
				_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{}})
				return
			}
			var metas []stremio.Meta
			for _, id := range []string{vaultShow, vaultLoose, vaultLone, vaultSingle, vaultEpisode} {
				meta, _ := describe(id)
				meta.Videos = nil
				if !strings.Contains(path, "search=") || strings.Contains(path, strings.Fields(meta.Name)[0]) {
					metas = append(metas, meta)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		case strings.HasPrefix(path, "/meta/library/"):
			id, _ := url.PathUnescape(strings.TrimSuffix(strings.TrimPrefix(path, "/meta/library/"), ".json"))
			meta, ok := describe(id)
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
		case strings.HasPrefix(path, "/stream/library/"):
			id, _ := url.PathUnescape(strings.TrimSuffix(strings.TrimPrefix(path, "/stream/library/"), ".json"))
			a.mu.Lock()
			a.asks[id]++
			a.mu.Unlock()
			file := strings.ReplaceAll(id, ":", ".") + ".mp4"
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{{Name: "Vault", Description: file,
				URL: server.URL + "/files/" + file, BehaviorHints: stremio.StreamBehavior{Filename: file}}}})
		case strings.HasPrefix(path, "/files/"):
			http.ServeFile(w, r, clip)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	a.url = server.URL + "/manifest.json"
	return a
}

// streamAsks counts the stream requests for a title or video.
func (a *vaultAddon) streamAsks(id string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.asks[id]
}

// vaultLibrary is a server with the vault addon's catalog as a library,
// shown as movies and shows when titles is set, and a member browsing it.
type vaultLibrary struct {
	tracking
	vault  *vaultAddon
	choice addons.LibraryChoice
	view   string
	file   []byte
}

func newVaultLibrary(t *testing.T, titles bool) vaultLibrary {
	t.Helper()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	// Six minutes, past Jellyfin's shortest resumable runtime.
	clip := filepath.Join(t.TempDir(), "clip.mp4")
	if out, err := exec.Command(ffmpeg, "-v", "error", "-y", "-f", "lavfi", "-i", "testsrc=duration=361:size=64x48:rate=1", "-c:v", "mpeg4",
		clip).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	file, err := os.ReadFile(clip)
	if err != nil {
		t.Fatal(err)
	}
	s := newProbingServer(t, 10, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"))
	member := s.user("member", nil)
	vault := newVaultAddon(t, clip)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), vault.url, false)
	if err != nil {
		t.Fatal(err)
	}
	catalog := addon.Manifest.Catalogs[0]
	p := vaultLibrary{tracking: tracking{testServer: s, token: s.signIn("member", "tv"), user: member.ID.String()}, vault: vault, file: file,
		choice: addons.LibraryChoice{AddonID: addon.ID, CatalogType: catalog.Type, CatalogID: catalog.ID}}
	p.show(t, titles)
	var views QueryResult
	s.get(t, "/UserViews", p.token, &views)
	for _, view := range views.Items {
		if view.Name == "Vault" {
			p.view = view.Id
		}
	}
	if p.view == "" {
		t.Fatalf("the views: %+v", views.Items)
	}
	return p
}

// show switches the library to movies and shows, or to home videos, as
// the administration's page saves it.
func (p vaultLibrary) show(t *testing.T, titles bool) {
	t.Helper()
	before, err := p.addons.Libraries(t.Context(), addons.Shared())
	if err != nil {
		t.Fatal(err)
	}
	choice := p.choice
	choice.ShowAsTitles = titles
	after, err := p.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{choice})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.testServer.library.FollowTitles(t.Context(), before, after); err != nil {
		t.Fatal(err)
	}
}

// titles lists the library's titles as the apps that list only movies and
// shows ask for them, by name.
func (p vaultLibrary) titles(t *testing.T) map[string]BaseItemDto {
	t.Helper()
	var page QueryResult
	p.get(t, "/Items?ParentId="+p.view+"&Recursive=true&IncludeItemTypes=Movie,Series&SortBy=SortName&SortOrder=Ascending", p.token, &page)
	byName := map[string]BaseItemDto{}
	for _, item := range page.Items {
		byName[item.Name] = item
	}
	if len(page.Items) != page.TotalRecordCount {
		t.Errorf("listed %d of %d", len(page.Items), page.TotalRecordCount)
	}
	return byName
}

// episodes lists a series' episodes as "season.episode name" and their
// identifiers, in order.
func (p vaultLibrary) episodes(t *testing.T, series, query string) ([]string, []string) {
	t.Helper()
	var page QueryResult
	if status := p.get(t, "/Shows/"+series+"/Episodes?UserId="+p.user+query, p.token, &page); status != http.StatusOK {
		t.Fatalf("the episodes: %d", status)
	}
	var places, ids []string
	for _, episode := range page.Items {
		if episode.Type != "Episode" || episode.SeriesId != series || episode.IndexNumber == nil || episode.ParentIndexNumber == nil {
			t.Errorf("an episode: %+v", episode)
			continue
		}
		places = append(places, fmt.Sprintf("%d.%d %s", *episode.ParentIndexNumber, *episode.IndexNumber, episode.Name))
		ids = append(ids, episode.Id)
	}
	return places, ids
}

// A library of other videos shown as movies and shows has no content
// type, as a mixed library, so the apps that keep only libraries of
// movies and shows keep it; its titles with videos are series, of the
// seasons and episodes their videos' numbers tell, or their order, and a
// title without videos is a movie. They open, play and seek, and Next Up,
// Continue Watching, played marks, search and the type filters see them.
func TestTitlesShownAsMoviesAndShows(t *testing.T) {
	t.Parallel()
	p := newVaultLibrary(t, true)
	var views QueryResult
	p.get(t, "/UserViews", p.token, &views)
	for _, view := range views.Items {
		if view.Id == p.view && view.CollectionType != "" {
			t.Errorf("the library's type: %q", view.CollectionType)
		}
	}

	titles := p.titles(t)
	show, loose, lone := titles["Ridge Show"], titles["Loose Files"], titles["Lone Clip"]
	single, episodeOnly := titles["Single Movie"], titles["Episode Only"]
	// Several videos, or one numbered as an episode, make a show; no
	// video, or one numbered as none, a movie.
	if len(titles) != 5 || show.Type != "Series" || loose.Type != "Series" || episodeOnly.Type != "Series" || lone.Type != "Movie" ||
		single.Type != "Movie" {
		t.Fatalf("the titles: %+v", titles)
	}
	for kind, want := range map[string][]string{"Series": {episodeOnly.Id, loose.Id, show.Id}, "Movie": {lone.Id, single.Id}} {
		if got := p.list(t, "/Items?ParentId="+p.view+"&Recursive=true&IncludeItemTypes="+kind+"&SortBy=SortName"); !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", kind, got, want)
		}
	}

	// The seasons and episodes of the show, as its videos number them.
	var seasons QueryResult
	if status := p.get(t, "/Shows/"+show.Id+"/Seasons?UserId="+p.user, p.token, &seasons); status != http.StatusOK || len(seasons.Items) != 2 ||
		*seasons.Items[0].IndexNumber != 1 || *seasons.Items[1].IndexNumber != 2 {
		t.Fatalf("the seasons: %d %+v", status, seasons.Items)
	}
	places, episodes := p.episodes(t, show.Id, "")
	want := []string{"1.1 Ridge.Show.S01E01.mkv", "1.2 Ridge.Show.S01E02.mkv", "1.3 Ridge.Show.Extras.mkv", "2.1 Ridge.Show.S02E01.mkv"}
	if !slices.Equal(places, want) {
		t.Fatalf("the episodes: %v", places)
	}
	if second, _ := p.episodes(t, show.Id, "&SeasonId="+seasons.Items[1].Id); !slices.Equal(second, want[3:]) {
		t.Errorf("the second season: %v", second)
	}
	// Videos without numbers are episodes of season 1, in order.
	if places, _ := p.episodes(t, loose.Id, ""); !slices.Equal(places, []string{"1.1 Loose.Part.1.mkv", "1.2 Loose.Part.2.mkv", "1.3 Loose.Part.3.mkv"}) {
		t.Errorf("the loose files: %v", places)
	}
	if places, _ := p.episodes(t, episodeOnly.Id, ""); !slices.Equal(places, []string{"3.5 Episode.Only.S03E05.mkv"}) {
		t.Errorf("the single episode: %v", places)
	}
	var episode BaseItemDto
	if status := p.get(t, "/Users/"+p.user+"/Items/"+episodes[0], p.token, &episode); status != http.StatusOK || episode.Type != "Episode" ||
		episode.SeriesId != show.Id || episode.SeasonId != seasons.Items[0].Id {
		t.Fatalf("the first episode: %d %+v", status, episode)
	}

	// An episode plays its video's stream, and seeks in it; the movie
	// plays its own.
	answer := p.playbackInfo(t, "tv", p.token, episodes[0], nil)
	if len(answer.MediaSources) != 1 || p.vault.streamAsks(vaultShow+":0") == 0 {
		t.Fatalf("the episode's PlaybackInfo: %+v", answer)
	}
	source := answer.MediaSources[0]
	response, body := fetchURL(t, p.url+"/Videos/"+episodes[0]+"/stream?static=true&MediaSourceId="+source.Id+"&ApiKey="+p.token,
		http.Header{"Range": {"bytes=2000-2099"}})
	if response.StatusCode != http.StatusPartialContent || body != string(p.file[2000:2100]) {
		t.Errorf("a range of the episode: %d", response.StatusCode)
	}
	if answer := p.playbackInfo(t, "tv", p.token, lone.Id, nil); len(answer.MediaSources) != 1 || p.vault.streamAsks(vaultLone) == 0 {
		t.Errorf("the movie's PlaybackInfo: %+v", answer)
	}
	// A movie of one file plays that file's streams.
	if answer := p.playbackInfo(t, "tv", p.token, single.Id, nil); len(answer.MediaSources) != 1 || p.vault.streamAsks(vaultSingle+":0") == 0 ||
		p.vault.streamAsks(vaultSingle) != 0 {
		t.Errorf("the one-file movie's PlaybackInfo: %+v", answer)
	}

	// Played, the first episode leaves the second up next; stopped
	// halfway, the second resumes.
	p.mark(t, http.MethodPost, "/UserPlayedItems/"+episodes[0])
	if got := p.list(t, "/Shows/NextUp?UserId="+p.user); !slices.Equal(got, episodes[1:2]) {
		t.Errorf("Next Up: %v, want %v", got, episodes[1:2])
	}
	if data := p.userData(t, show.Id); data.UnplayedItemCount == nil || *data.UnplayedItemCount != 3 {
		t.Errorf("the show after one episode played: %+v", data)
	}
	p.report(t, "/Sessions/Playing", map[string]any{"ItemId": episodes[1], "PositionTicks": 0})
	p.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": episodes[1], "PositionTicks": 1_800_000_000})
	if got := p.list(t, "/UserItems/Resume?MediaTypes=Video"); !slices.Equal(got, episodes[1:2]) {
		t.Errorf("Continue Watching: %v", got)
	}
	// The movie played, the played filters tell it from the shows.
	p.mark(t, http.MethodPost, "/UserPlayedItems/"+lone.Id)
	for filter, want := range map[string][]string{"IsUnplayed": {episodeOnly.Id, loose.Id, show.Id, single.Id}, "IsPlayed": {lone.Id}} {
		got := p.list(t, "/Items?ParentId="+p.view+"&Recursive=true&IncludeItemTypes=Movie,Series&Filters="+filter)
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", filter, got, want)
		}
	}

	// Search finds them as movies and shows.
	if got := p.list(t, "/Items?searchTerm=Ridge&Recursive=true&IncludeItemTypes=Movie,Series"); !slices.Equal(got, []string{show.Id}) {
		t.Errorf("searching shows: %v", got)
	}
	if got := p.list(t, "/Items?searchTerm=Lone&Recursive=true&IncludeItemTypes=Movie"); !slices.Equal(got, []string{lone.Id}) {
		t.Errorf("searching movies: %v", got)
	}
}

// Switching a library between home videos and movies and shows gives
// users' played marks, resume points and favorites to the items its
// titles and videos become, both ways; the items they were are no longer
// found.
func TestShowingTitlesKeepsWatchState(t *testing.T) {
	t.Parallel()
	p := newVaultLibrary(t, false)
	var page QueryResult
	p.get(t, "/Items?ParentId="+p.view, p.token, &page)
	folders := map[string]BaseItemDto{}
	for _, item := range page.Items {
		folders[item.Name] = item
	}
	folder, lone, singleFolder := folders["Ridge Show"], folders["Lone Clip"], folders["Single Movie"]
	if folder.Type != "Folder" || lone.Type != "Video" || singleFolder.Type != "Folder" {
		t.Fatalf("the home videos: %+v", page.Items)
	}
	videos := p.list(t, "/Items?ParentId="+folder.Id)
	if len(videos) != len(vaultShowVideos) {
		t.Fatalf("the folder's videos: %v", videos)
	}
	// The video of S01E01 played, S01E02's stopped halfway, the folder and
	// the file alone favorites.
	p.mark(t, http.MethodPost, "/UserPlayedItems/"+videos[1])
	p.report(t, "/Sessions/Playing", map[string]any{"ItemId": videos[0], "PositionTicks": 0})
	p.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": videos[0], "PositionTicks": 1_800_000_000})
	p.mark(t, http.MethodPost, "/UserFavoriteItems/"+folder.Id)
	p.mark(t, http.MethodPost, "/UserFavoriteItems/"+lone.Id)
	// The one-file download's video played.
	singleVideo := p.list(t, "/Items?ParentId="+singleFolder.Id)
	if len(singleVideo) != 1 {
		t.Fatalf("the one-file download: %v", singleVideo)
	}
	p.mark(t, http.MethodPost, "/UserPlayedItems/"+singleVideo[0])

	p.show(t, true)
	titles := p.titles(t)
	show, movie := titles["Ridge Show"], titles["Lone Clip"]
	single := titles["Single Movie"]
	if show.Type != "Series" || movie.Type != "Movie" || !show.UserData.IsFavorite || !movie.UserData.IsFavorite || single.Type != "Movie" ||
		!single.UserData.Played {
		t.Fatalf("the titles once shown as movies and shows: %+v", titles)
	}
	_, episodes := p.episodes(t, show.Id, "")
	if data := p.userData(t, episodes[0]); !data.Played {
		t.Errorf("S01E01: %+v", data)
	}
	if data := p.userData(t, episodes[1]); data.Played || data.PlaybackPositionTicks != 1_800_000_000 {
		t.Errorf("S01E02: %+v", data)
	}
	if got := p.list(t, "/UserItems/Resume?MediaTypes=Video"); !slices.Equal(got, episodes[1:2]) {
		t.Errorf("Continue Watching: %v", got)
	}
	if got := p.list(t, "/Shows/NextUp?UserId="+p.user); !slices.Equal(got, episodes[1:2]) {
		t.Errorf("Next Up: %v", got)
	}
	for _, id := range []string{folder.Id, videos[1], lone.Id} {
		var item BaseItemDto
		if status := p.get(t, "/Users/"+p.user+"/Items/"+id, p.token, &item); status != http.StatusNotFound {
			t.Errorf("a former home video: %d %+v", status, item)
		}
	}

	// Back to home videos, with the extras video played meanwhile.
	p.mark(t, http.MethodPost, "/UserPlayedItems/"+episodes[2])
	p.show(t, false)
	if data := p.userData(t, videos[2]); !data.Played {
		t.Errorf("the extras video: %+v", data)
	}
	if data := p.userData(t, singleVideo[0]); !data.Played {
		t.Errorf("the one-file download's video: %+v", data)
	}
	if data := p.userData(t, folder.Id); !data.IsFavorite {
		t.Errorf("the folder: %+v", data)
	}
	if got := p.list(t, "/UserItems/Resume?MediaTypes=Video"); !slices.Equal(got, videos[:1]) {
		t.Errorf("Continue Watching: %v", got)
	}
	var item BaseItemDto
	if status := p.get(t, "/Users/"+p.user+"/Items/"+show.Id, p.token, &item); status != http.StatusNotFound {
		t.Errorf("the former series: %d", status)
	}
}
