package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// trailerFile is the MP4 file the trailer addon serves as a trailer.
var trailerFile = filepath.Join("..", "container", "testdata", "faststart.mp4")

// trailerAddon serves a movie whose description gives a trailer on a
// video site and a trailer stream linking to an MP4 file it serves, and
// one stream of the movie.
func trailerAddon(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	movie := func() stremio.Meta {
		return stremio.Meta{ID: "tt5000", Type: "movie", Name: "Trailered", Year: "2010", Extras: &stremio.Extras{Certification: "PG"},
			Trailers:       []stremio.Trailer{{Source: "dQw4w9WgXcQ", Type: "Trailer"}},
			TrailerStreams: []stremio.Stream{{Title: "Teaser", URL: server.URL + "/trailers/teaser.mp4"}}}
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "trailers", Name: "Trailers", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie()}})
		case path == "/meta/movie/tt5000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie()})
		case path == "/stream/movie/tt5000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{
				{Name: "Source 1080p", URL: server.URL + "/files/movie.mkv"}}})
		case path == "/trailers/teaser.mp4":
			http.ServeFile(w, r, trailerFile)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// An addon's trailer on a video site is a remote trailer, and its link to
// a video a local trailer: a Trailer item of the movie, which plays its
// link as one of the addon's streams, relayed from the addon's server on
// the loopback interface. The movie's own versions stay its streams.
func TestAddonTrailers(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	s := newProbingServer(t, 10, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"))
	user := s.user("member", nil)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), trailerAddon(t), false); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	var page QueryResult
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	if len(page.Items) != 1 {
		t.Fatalf("the movies: %+v", page.Items)
	}
	movie := page.Items[0].Id

	var details BaseItemDto
	s.get(t, "/Users/"+user.ID.String()+"/Items/"+movie, token, &details)
	if details.LocalTrailerCount == nil || *details.LocalTrailerCount != 1 {
		t.Errorf("local trailers counted: %v", details.LocalTrailerCount)
	}
	if details.RemoteTrailers == nil || len(*details.RemoteTrailers) != 1 ||
		(*details.RemoteTrailers)[0] != (MediaUrl{Url: "https://www.youtube.com/watch?v=dQw4w9WgXcQ", Name: "Trailer"}) {
		t.Errorf("remote trailers: %+v", details.RemoteTrailers)
	}
	if details.MediaSources == nil || len(*details.MediaSources) != 1 {
		t.Errorf("the movie's versions: %+v", details.MediaSources)
	}

	var trailers []BaseItemDto
	s.get(t, "/Items/"+movie+"/LocalTrailers?userId="+user.ID.String(), token, &trailers)
	if len(trailers) != 1 {
		t.Fatalf("local trailers: %+v", trailers)
	}
	trailer := trailers[0]
	if trailer.Name != "Teaser" || trailer.Type != "Trailer" || trailer.ExtraType != "Trailer" || trailer.MediaType != "Video" || trailer.ParentId != movie {
		t.Errorf("the trailer: %+v", trailer)
	}
	var features []BaseItemDto
	if s.get(t, "/Items/"+movie+"/SpecialFeatures", token, &features); len(features) != 0 {
		t.Errorf("special features: %+v", features)
	}
	var opened BaseItemDto
	s.get(t, "/Users/"+user.ID.String()+"/Items/"+trailer.Id, token, &opened)
	if opened.Id != trailer.Id || opened.Type != "Trailer" || opened.ExtraType != "Trailer" || opened.ParentId != movie {
		t.Errorf("the trailer's details: %+v", opened)
	}

	profile, err := os.ReadFile(filepath.Join(playbackFixtures, "profiles", "jellyfin-web-chrome.json"))
	if err != nil {
		t.Fatal(err)
	}
	status, data := s.call(http.MethodPost, "/Items/"+trailer.Id+"/PlaybackInfo", app("tv", token), map[string]any{
		"UserId": user.ID.String(), "DeviceProfile": json.RawMessage(profile)})
	var info playbackInfoResponse
	if err := json.Unmarshal(data, &info); status != http.StatusOK || err != nil || len(info.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %d %s", status, data)
	}
	file, err := os.ReadFile(trailerFile)
	if err != nil {
		t.Fatal(err)
	}
	// As jellyfin-web asks a source it plays directly: by its container.
	source := info.MediaSources[0]
	if source.Container != "mp4" || !source.SupportsDirectPlay {
		t.Errorf("the trailer's source: %+v", source)
	}
	stream := s.url + "/Videos/" + trailer.Id + "/stream." + source.Container + "?static=true&mediaSourceId=" + source.Id + "&playSessionId=" + info.PlaySessionId
	response, body := fetchURL(t, stream, nil)
	if response.StatusCode != http.StatusOK || body != string(file) || response.Header.Get("Content-Type") != "video/mp4" {
		t.Errorf("the trailer's stream: %d, %d bytes, %s", response.StatusCode, len(body), response.Header.Get("Content-Type"))
	}

	// A user whose parental control hides the movie reaches none of its
	// trailers.
	child := s.user("child", func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{MaxRating: new(0)} })
	childToken := s.signIn("child", "tv")
	for _, path := range []string{"/Users/" + child.ID.String() + "/Items/" + trailer.Id, "/Items/" + movie + "/LocalTrailers"} {
		if status, data := s.call(http.MethodGet, path, app("tv", childToken), nil); status != http.StatusNotFound {
			t.Errorf("%s for a child: %d %s", path, status, data)
		}
	}
}
