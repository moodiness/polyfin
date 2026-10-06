package jellyfin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// recordedDownloads are Jellyfin 12.2's answers to downloads, recorded by
// scripts/jellyfin-fixtures.sh.
func recordedDownloads(t *testing.T) map[string]struct {
	Status             int
	ContentDisposition string
	Body               any
} {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.2", "downloads", "answers.json"))
	if err != nil {
		t.Fatal(err)
	}
	var answers map[string]struct {
		Status             int
		ContentDisposition string
		Body               any
	}
	if err := json.Unmarshal(data, &answers); err != nil {
		t.Fatal(err)
	}
	return answers
}

// fetchURL gets target with header and returns the answer and its body.
func fetchURL(t *testing.T, target string, header http.Header) (*http.Response, string) {
	t.Helper()
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
	for name, values := range header {
		request.Header[name] = values
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	return response, string(body)
}

// A title's identifier stands for the version its grant names, else for its
// first version that has not failed, as in item details and PlaybackInfo.
func TestDownloadsSkipFailedVersions(t *testing.T) {
	p := playing(t)
	second := "attachment; filename=Movie.1080p.mp4; filename*=UTF-8''Movie.1080p.mp4"
	userItem := "/Users/" + p.user.ID.String() + "/Items/"
	var movie BaseItemDto
	p.get(t, userItem+p.movie, p.token, &movie)
	// The grant in the second version's Path names it.
	streamURL, err := url.Parse((*movie.MediaSources)[1].Path)
	if err != nil {
		t.Fatal(err)
	}
	grant := url.QueryEscape(streamURL.Query().Get(grantParameter))
	if response, _ := fetchURL(t, p.url+"/Items/"+p.movie+"/Download?"+grantParameter+"="+grant, nil); response.Header.Get("Content-Disposition") != second {
		t.Errorf("with the second version's grant: %d %v", response.StatusCode, response.Header)
	}

	// PlaybackInfo fails to analyze the first version, as ffprobe is not
	// installed in tests, and plays the second.
	if status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token),
		map[string]any{"UserId": p.user.ID.String(), "DeviceProfile": p.profile(t, "jellyfin-web-chrome")}); status != http.StatusOK {
		t.Fatalf("PlaybackInfo: %d %s", status, data)
	}
	response, body := fetchURL(t, p.url+"/Items/"+p.movie+"/Download?ApiKey="+p.token, nil)
	if response.StatusCode != http.StatusOK || body != "\x1a\x45\xdf\xa3 media bytes" || response.Header.Get("Content-Disposition") != second {
		t.Errorf("title after its first version failed: %d %v %q", response.StatusCode, response.Header, body)
	}
	// Path names the same file, in details and in listings.
	p.get(t, userItem+p.movie, p.token, &movie)
	if movie.Path != "Movie.1080p.mp4" {
		t.Errorf("detail path: %q", movie.Path)
	}
	var views QueryResult
	p.get(t, "/UserViews", p.token, &views)
	var page QueryResult
	p.get(t, "/Items?ParentId="+views.Items[0].Id+"&fields=CanDownload,Path", p.token, &page)
	for _, item := range page.Items {
		if item.Id == p.movie && (item.Path != "Movie.1080p.mp4" || item.CanDownload == nil || !*item.CanDownload) {
			t.Errorf("listed path: %q, can download %v", item.Path, item.CanDownload)
		}
	}
}

func TestDownloadsServeAVersionAsAFile(t *testing.T) {
	p := playing(t)
	recorded := recordedDownloads(t)
	fetch := func(path string, header http.Header) (*http.Response, string) {
		t.Helper()
		return fetchURL(t, p.url+path, header)
	}
	apiKey := "?ApiKey=" + p.token

	// The title stands for its first version, whose file name it is saved
	// under, as Jellyfin names a download after its file. The addon's
	// source is on the loopback interface: Polyfin relays it.
	file, err := os.ReadFile(filepath.Join("..", "container", "testdata", "forced.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	response, body := fetch("/Items/"+p.movie+"/Download"+apiKey, nil)
	if response.StatusCode != recorded["Movie with the token in ApiKey"].Status || body != string(file) ||
		response.Header.Get("Content-Type") != "video/x-matroska" ||
		response.Header.Get("Content-Disposition") != "attachment; filename=Movie.2160p.mkv; filename*=UTF-8''Movie.2160p.mkv" {
		t.Errorf("title: %d %v, %d bytes", response.StatusCode, response.Header, len(body))
	}
	response, body = fetch("/Items/"+p.movie+"/Download"+apiKey, http.Header{"Range": {"bytes=0-99"}})
	if response.StatusCode != recorded["Movie, a byte range"].Status || body != string(file[:100]) ||
		!strings.HasPrefix(response.Header.Get("Content-Range"), "bytes 0-99/") {
		t.Errorf("range: %d %v", response.StatusCode, response.Header)
	}
	// A version downloads by its own identifier, as Streamyfin asks for
	// the media source it picked.
	response, body = fetch("/Items/"+p.versions[1].ID.String()+"/Download", http.Header{"Authorization": {app("tv", p.token)}})
	if response.StatusCode != http.StatusOK || body != "\x1a\x45\xdf\xa3 media bytes" || response.Header.Get("Content-Type") != "video/mp4" ||
		response.Header.Get("Content-Disposition") != "attachment; filename=Movie.1080p.mp4; filename*=UTF-8''Movie.1080p.mp4" {
		t.Errorf("version: %d %v %q", response.StatusCode, response.Header, body)
	}
	// The grant of a media source's Path stands for the user, as on the
	// stream route.
	var movie BaseItemDto
	p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie)
	streamURL, err := url.Parse((*movie.MediaSources)[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if response, _ = fetch("/Items/"+p.movie+"/Download?"+grantParameter+"="+url.QueryEscape(streamURL.Query().Get(grantParameter)), nil); response.StatusCode != http.StatusOK {
		t.Errorf("with a grant: %d", response.StatusCode)
	}

	var views QueryResult
	p.get(t, "/UserViews", p.token, &views)
	for key, path := range map[string]string{
		"Anonymous":    "/Items/" + p.movie + "/Download",
		"Wrong token":  "/Items/" + p.movie + "/Download?ApiKey=" + strings.Repeat("0", 32),
		"Unknown item": "/Items/" + strings.Repeat("ab", 16) + "/Download" + apiKey,
		"Not an id":    "/Items/nothing/Download" + apiKey,
		// What has no version, a library here as a series in Jellyfin.
		"Series": "/Items/" + views.Items[0].Id + "/Download" + apiKey,
	} {
		response, body := fetch(path, nil)
		want := recorded[key]
		if response.StatusCode != want.Status || response.Header.Get("Content-Disposition") != "" {
			t.Errorf("%s: %d %v, Jellyfin %d", key, response.StatusCode, response.Header, want.Status)
		}
		// Error bodies keep Jellyfin's, but for their trace identifiers.
		switch expected := want.Body.(type) {
		case string:
			if body != expected {
				t.Errorf("%s: body %q, Jellyfin %q", key, body, expected)
			}
		case map[string]any:
			var got map[string]any
			if err := json.Unmarshal([]byte(body), &got); err != nil || got["title"] != expected["title"] || got["status"] != expected["status"] {
				t.Errorf("%s: body %s, Jellyfin %v", key, body, expected)
			}
		}
	}
}

func TestMoviesWithVersionsCanBeDownloaded(t *testing.T) {
	p := playing(t)
	data, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.2", "downloads", "can-download.json"))
	if err != nil {
		t.Fatal(err)
	}
	var recorded struct {
		MovieDetail, SeriesDetail, ListingWithoutTheFieldHasIt bool
	}
	if err := json.Unmarshal(data, &recorded); err != nil {
		t.Fatal(err)
	}
	userItem := "/Users/" + p.user.ID.String() + "/Items/"
	var movie, version, view BaseItemDto
	p.get(t, userItem+p.movie, p.token, &movie)
	if movie.CanDownload == nil || *movie.CanDownload != recorded.MovieDetail || movie.Path != "Movie.2160p.mkv" {
		t.Errorf("title: can download %v, path %q", movie.CanDownload, movie.Path)
	}
	// jellyfin-web names the file it downloads after Path.
	p.get(t, userItem+p.versions[1].ID.String(), p.token, &version)
	if version.CanDownload == nil || !*version.CanDownload || version.Path != "Movie.1080p.mp4" {
		t.Errorf("version: can download %v, path %q", version.CanDownload, version.Path)
	}
	var views QueryResult
	p.get(t, "/UserViews", p.token, &views)
	p.get(t, userItem+views.Items[0].Id, p.token, &view)
	if view.CanDownload == nil || *view.CanDownload != recorded.SeriesDetail || view.Path != "" {
		t.Errorf("library: can download %v, path %q", view.CanDownload, view.Path)
	}

	listing := func(query string) map[string]map[string]any {
		t.Helper()
		var page struct{ Items []map[string]any }
		p.get(t, "/Items?ParentId="+views.Items[0].Id+query, p.token, &page)
		byID := map[string]map[string]any{}
		for _, item := range page.Items {
			byID[item["Id"].(string)] = item
		}
		return byID
	}
	listed := listing("&fields=CanDownload,Path")
	if item := listed[p.movie]; item["CanDownload"] != true || item["Path"] != "Movie.2160p.mkv" {
		t.Errorf("listed title: %v %v", item["CanDownload"], item["Path"])
	}
	// The other title's versions were never listed: a listing does not ask
	// addons for them.
	if item := listed[p.remote]; item["CanDownload"] != false || item["Path"] != nil {
		t.Errorf("listed title never opened: %v %v", item["CanDownload"], item["Path"])
	}
	if _, ok := listing("")[p.movie]["CanDownload"]; ok != recorded.ListingWithoutTheFieldHasIt {
		t.Errorf("listing without the field: CanDownload sent %v", ok)
	}
}
