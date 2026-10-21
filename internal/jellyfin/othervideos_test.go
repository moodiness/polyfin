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
	"github.com/moodiness/polyfin/internal/stremio"
)

// storeAddon is a fake addon of a cloud store's files, answering as such
// addons do: one catalog of type other, whose titles are a download with
// its files as videos, each embedding its stream, and a file alone, with
// no videos; a stream resource that answers a notice for the videos, and
// the file alone's stream for it. Every file is clip. It counts the stream
// requests by identifier.
type storeAddon struct {
	url  string
	mu   sync.Mutex
	asks map[string]int
}

const (
	storeDownload = "bx:store:ad:k3j9q1"
	storeSingle   = "bx:store:ad:p7x2m4"
	storeVideos   = 36
)

// storeVideoID is the identifier of the download's video n, from 1.
func storeVideoID(n int) string {
	return fmt.Sprintf("%s:%05d:Holiday.Clips.2024.Part.%02d.1080p.WEB.x264.mp4", storeDownload, n, n)
}

func newStoreAddon(t *testing.T, clip string) *storeAddon {
	t.Helper()
	info, err := os.Stat(clip)
	if err != nil {
		t.Fatal(err)
	}
	a := &storeAddon{asks: map[string]int{}}
	var server *httptest.Server
	stream := func(name, file string) stremio.Stream {
		return stremio.Stream{Name: "Boxcloud", Description: name + "\n" + file, URL: server.URL + "/files/" + file,
			BehaviorHints: stremio.StreamBehavior{Filename: file, VideoSize: stremio.Number(info.Size()), BingeGroup: "boxcloud|store"}}
	}
	download := func() stremio.Meta {
		meta := stremio.Meta{ID: storeDownload, Type: "other", Name: "Holiday clips", Description: "A download", Released: "2024-08-02T10:00:00.000Z"}
		for n := 1; n <= storeVideos; n++ {
			file := fmt.Sprintf("Holiday.Clips.2024.Part.%02d.1080p.WEB.x264.mp4", n)
			meta.Videos = append(meta.Videos, stremio.Video{ID: storeVideoID(n), Title: fmt.Sprintf("Part %02d", n),
				Released: "2024-08-02T10:00:00.000Z", Streams: []stremio.Stream{stream(fmt.Sprintf("Part %02d", n), file)}})
		}
		return meta
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "org.example.boxcloud", Name: "Boxcloud", Version: "1",
				Types: []string{"movie", "series", "anime", "other"},
				Resources: []stremio.Resource{
					{Name: "stream", Types: []string{"movie", "series", "anime", "other"}, IDPrefixes: []string{"tt", "tmdb", "tvdb", "kitsu", "bx:store:"}},
					{Name: "catalog", Types: []string{"other"}},
					{Name: "meta", Types: []string{"other"}, IDPrefixes: []string{"bx:store:"}},
				},
				Catalogs: []stremio.Catalog{{Type: "other", ID: "a1b2c3d.bx:store:ad", Name: "Downloads", Extra: []stremio.Extra{
					{Name: "search"}, {Name: "skip"}, {Name: "genre", Options: []string{"Downloads", "Torrents"}}}}}})
		case strings.HasPrefix(path, "/catalog/other/a1b2c3d.bx%3Astore%3Aad"):
			if strings.Contains(path, "skip=") {
				_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{}})
				return
			}
			metas := []stremio.Meta{
				{ID: storeDownload, Type: "other", Name: "Holiday clips", Poster: server.URL + "/poster.jpg", Description: "A download"},
				{ID: storeSingle, Type: "other", Name: "Single clip", Poster: server.URL + "/poster.jpg", Description: "A file"},
			}
			if strings.Contains(path, "search=") {
				metas = slices.DeleteFunc(metas, func(m stremio.Meta) bool { return !strings.Contains(path, strings.Fields(m.Name)[0]) })
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		case path == "/meta/other/"+strings.ReplaceAll(storeDownload, ":", "%3A")+".json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": download()})
		case path == "/meta/other/"+strings.ReplaceAll(storeSingle, ":", "%3A")+".json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": stremio.Meta{ID: storeSingle, Type: "other", Name: "Single clip", Description: "A file"}})
		case strings.HasPrefix(path, "/stream/"):
			id := strings.TrimSuffix(path[strings.LastIndex(path, "/")+1:], ".json")
			id = strings.ReplaceAll(id, "%3A", ":")
			a.mu.Lock()
			a.asks[id]++
			a.mu.Unlock()
			if id == storeSingle {
				_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{stream("Single", "Single.Clip.mp4")}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{{Name: "Boxcloud", Title: "Open the file in the store",
				ExternalURL: "https://boxcloud.example/store"}}})
		case strings.HasPrefix(path, "/files/"):
			http.ServeFile(w, r, clip)
		case path == "/poster.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpeg bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	a.url = server.URL + "/manifest.json"
	return a
}

// streamAsks counts the stream requests for a title or video.
func (a *storeAddon) streamAsks(id string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.asks[id]
}

// embeddedSeriesAddon serves a series whose episodes embed their streams,
// and no stream resource.
func embeddedSeriesAddon(t *testing.T, clip string) string {
	t.Helper()
	var server *httptest.Server
	show := func() stremio.Meta {
		meta := stremio.Meta{ID: "tt0900", Type: "series", Name: "Embedded show", Released: "2020-01-01T00:00:00.000Z"}
		for n := 1; n <= 2; n++ {
			meta.Videos = append(meta.Videos, stremio.Video{ID: fmt.Sprintf("tt0900:1:%d", n), Title: fmt.Sprintf("Episode %d", n), Season: 1,
				Episode: stremio.Number(n), Released: "2020-01-01T00:00:00.000Z",
				Streams: []stremio.Stream{{Name: "Own 720p", URL: server.URL + fmt.Sprintf("/files/episode%d.mp4", n),
					BehaviorHints: stremio.StreamBehavior{Filename: fmt.Sprintf("Embedded.Show.S01E0%d.mp4", n)}}}})
		}
		return meta
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "org.example.ownshows", Name: "Own shows", Version: "1", Types: []string{"series"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "series", ID: "own", Name: "Own shows"}}})
		case strings.HasPrefix(path, "/catalog/series/own"):
			preview := show()
			preview.Videos = nil
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{preview}})
		case path == "/meta/series/tt0900.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": show()})
		case strings.HasPrefix(path, "/files/"):
			http.ServeFile(w, r, clip)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// A catalog of type other is a homevideos library: a title with videos is
// a Folder of Video items, in its description's order, a title without is
// a Video. The videos play the streams their description embeds, the
// addon not being asked for them, directly and with their file's name and
// size; the title alone plays the stream resource's. Watch state, resume
// points and Continue Watching work as for other videos, and the folder
// counts its videos watched. Series' episodes play their embedded streams
// too. Search, jellyfin-web's Videos view, parental control and hidden
// titles see them.
func TestOtherTitlesAreFoldersOfVideos(t *testing.T) {
	t.Parallel()
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
	store := newStoreAddon(t, clip)
	var choices []addons.LibraryChoice
	for _, manifest := range []string{store.url, embeddedSeriesAddon(t, clip)} {
		addon, err := s.addons.Install(t.Context(), addons.Shared(), manifest, false)
		if err != nil {
			t.Fatal(err)
		}
		catalog := addon.Manifest.Catalogs[0]
		choices = append(choices, addons.LibraryChoice{AddonID: addon.ID, CatalogType: catalog.Type, CatalogID: catalog.ID})
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), choices); err != nil {
		t.Fatal(err)
	}
	tr := tracking{testServer: s, token: s.signIn("member", "tv"), user: member.ID.String()}
	var views QueryResult
	s.get(t, "/UserViews", tr.token, &views)
	library, shows := "", ""
	for _, view := range views.Items {
		switch view.Name {
		case "Downloads":
			library = view.Id
			if view.CollectionType != "homevideos" {
				t.Errorf("the library's type: %q", view.CollectionType)
			}
		case "Own shows":
			shows = view.Id
		}
	}

	// jellyfin-web's Folders view: the download is a folder, the file
	// alone a video.
	var titles QueryResult
	s.get(t, "/Items?ParentId="+library+"&Recursive=false&IncludeItemTypes=Folder,Photo,PhotoAlbum,Video&Fields=ChildCount", tr.token, &titles)
	if len(titles.Items) != 2 || titles.TotalRecordCount != 2 {
		t.Fatalf("the library: %+v", titles)
	}
	folder, single := titles.Items[0], titles.Items[1]
	if folder.Name != "Holiday clips" || folder.Type != "Folder" || !folder.IsFolder || folder.ChildCount == nil || *folder.ChildCount != storeVideos ||
		folder.UserData.UnplayedItemCount == nil || *folder.UserData.UnplayedItemCount != storeVideos {
		t.Errorf("the download: %+v", folder)
	}
	if single.Name != "Single clip" || single.Type != "Video" || single.MediaType != "Video" || single.IsFolder {
		t.Errorf("the file alone: %+v", single)
	}

	// The folder lists its videos in order.
	var videos QueryResult
	s.get(t, "/Items?ParentId="+folder.Id+"&Fields=ParentId", tr.token, &videos)
	if len(videos.Items) != storeVideos || videos.TotalRecordCount != storeVideos {
		t.Fatalf("the videos: %d of %d", len(videos.Items), videos.TotalRecordCount)
	}
	for i, video := range videos.Items {
		if want := fmt.Sprintf("Part %02d", i+1); video.Name != want || video.Type != "Video" || video.MediaType != "Video" || video.ParentId != folder.Id {
			t.Errorf("video %d: %+v", i+1, video)
		}
	}
	third := videos.Items[2].Id
	var ancestors []BaseItemDto
	s.get(t, "/Items/"+third+"/Ancestors", tr.token, &ancestors)
	if len(ancestors) < 2 || ancestors[0].Id != folder.Id || ancestors[1].Id != library {
		t.Errorf("ancestors: %+v", ancestors)
	}

	// A video plays its embedded stream, named and sized as listed,
	// without asking the addon's stream resource.
	answer := s.playbackInfo(t, "tv", tr.token, third, nil)
	if len(answer.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %+v", answer)
	}
	source := answer.MediaSources[0]
	if !source.SupportsDirectPlay || source.Size == nil || *source.Size != int64(len(file)) || !strings.Contains(source.Name, "Boxcloud") {
		t.Errorf("the video's version: %+v", source)
	}
	if asked := store.streamAsks(storeVideoID(3)); asked != 0 {
		t.Errorf("the addon was asked %d times for the video's streams", asked)
	}
	response, body := fetchURL(t, s.url+"/Videos/"+third+"/stream?static=true&MediaSourceId="+source.Id+"&ApiKey="+tr.token,
		http.Header{"Range": {"bytes=2000-2099"}})
	if response.StatusCode != http.StatusPartialContent || body != string(file[2000:2100]) {
		t.Errorf("a range of the video: %d", response.StatusCode)
	}

	// The file alone plays the stream resource's stream.
	answer = s.playbackInfo(t, "tv", tr.token, single.Id, nil)
	if len(answer.MediaSources) != 1 || store.streamAsks(storeSingle) != 1 {
		t.Errorf("the file alone: %+v, asked %d times", answer, store.streamAsks(storeSingle))
	}

	// Stopped halfway, a video resumes and shows in Continue Watching;
	// another marked played counts in its folder.
	tr.report(t, "/Sessions/Playing", map[string]any{"ItemId": third, "MediaSourceId": source.Id, "PositionTicks": 0})
	tr.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": third, "MediaSourceId": source.Id, "PositionTicks": 1_800_000_000})
	var resume QueryResult
	s.get(t, "/UserItems/Resume?MediaTypes=Video", tr.token, &resume)
	if len(resume.Items) != 1 || resume.Items[0].Id != third || resume.Items[0].UserData.PlaybackPositionTicks != 1_800_000_000 {
		t.Errorf("resume: %+v", resume.Items)
	}
	tr.mark(t, http.MethodPost, "/UserPlayedItems/"+videos.Items[0].Id)
	if data := tr.userData(t, videos.Items[0].Id); !data.Played {
		t.Errorf("played: %+v", data)
	}
	if data := tr.userData(t, folder.Id); data.UnplayedItemCount == nil || *data.UnplayedItemCount != storeVideos-1 || data.Played {
		t.Errorf("the folder after one video played: %+v", data)
	}
	s.get(t, "/Items?ParentId="+library, tr.token, &titles)
	if data := titles.Items[0].UserData; data.UnplayedItemCount == nil || *data.UnplayedItemCount != storeVideos-1 {
		t.Errorf("the folder listed after one video played: %+v", data)
	}
	tr.mark(t, http.MethodPost, "/UserPlayedItems/"+folder.Id)
	if data := tr.userData(t, folder.Id); data.UnplayedItemCount == nil || *data.UnplayedItemCount != 0 || !data.Played {
		t.Errorf("the folder marked played: %+v", data)
	}
	if data := tr.userData(t, videos.Items[storeVideos-1].Id); !data.Played {
		t.Errorf("the folder's last video once it is marked played: %+v", data)
	}

	// jellyfin-web's Videos view lists the videos through the folder.
	var all QueryResult
	s.get(t, "/Items?ParentId="+library+"&Recursive=true&IncludeItemTypes=Video&SortBy=SortName&SortOrder=Ascending", tr.token, &all)
	if len(all.Items) != storeVideos+1 || all.TotalRecordCount != storeVideos+1 {
		t.Errorf("the Videos view: %d of %d", len(all.Items), all.TotalRecordCount)
	}

	// Searched, as folders or videos.
	var found QueryResult
	s.get(t, "/Items?searchTerm=Holiday&Recursive=true&IncludeItemTypes=Folder", tr.token, &found)
	if len(found.Items) != 1 || found.Items[0].Id != folder.Id {
		t.Errorf("searching folders: %+v", found.Items)
	}
	s.get(t, "/Items?searchTerm=Single&Recursive=true&IncludeItemTypes=Video", tr.token, &found)
	if len(found.Items) != 1 || found.Items[0].Id != single.Id {
		t.Errorf("searching videos: %+v", found.Items)
	}

	// A series' episodes play the streams they embed.
	var series, episodes QueryResult
	s.get(t, "/Items?ParentId="+shows, tr.token, &series)
	if len(series.Items) != 1 {
		t.Fatalf("the shows: %+v", series.Items)
	}
	s.get(t, "/Shows/"+series.Items[0].Id+"/Episodes", tr.token, &episodes)
	if len(episodes.Items) != 2 {
		t.Fatalf("the episodes: %+v", episodes.Items)
	}
	answer = s.playbackInfo(t, "tv", tr.token, episodes.Items[1].Id, nil)
	if len(answer.MediaSources) != 1 || !strings.Contains(answer.MediaSources[0].Name, "Own 720p") {
		t.Fatalf("the episode's PlaybackInfo: %+v", answer)
	}
	response, body = fetchURL(t, s.url+"/Videos/"+episodes.Items[1].Id+"/stream?static=true&MediaSourceId="+answer.MediaSources[0].Id+
		"&ApiKey="+tr.token, http.Header{"Range": {"bytes=0-99"}})
	if response.StatusCode != http.StatusPartialContent || body != string(file[:100]) {
		t.Errorf("a range of the episode: %d", response.StatusCode)
	}

	// Unrated, they are Other to parental control.
	child := s.user("child", func(c *accounts.UserChanges) {
		c.Parental = &accounts.ParentalControl{BlockUnrated: []string{"Other"}}
	})
	childToken := s.signIn("child", "tv")
	var childTitles QueryResult
	s.get(t, "/Items?ParentId="+library, childToken, &childTitles)
	var item BaseItemDto
	if len(childTitles.Items) != 0 || s.get(t, "/Users/"+child.ID.String()+"/Items/"+third, childToken, &item) != http.StatusNotFound {
		t.Errorf("a child blocking Other: %+v", childTitles.Items)
	}

	// Hiding a video hides its folder, with its videos.
	if status, data := tr.call(http.MethodPost, "/Polyfin/UserItems/"+third+"/Hidden", app("tv", tr.token), nil); status != http.StatusNoContent {
		t.Fatalf("hiding: %d %s", status, data)
	}
	s.get(t, "/Items?ParentId="+library, tr.token, &titles)
	if len(titles.Items) != 1 || titles.Items[0].Id != single.Id {
		t.Errorf("the library once the folder is hidden: %+v", titles.Items)
	}
	s.get(t, "/UserItems/Resume?MediaTypes=Video", tr.token, &resume)
	if len(resume.Items) != 0 {
		t.Errorf("resume once the folder is hidden: %+v", resume.Items)
	}
}
