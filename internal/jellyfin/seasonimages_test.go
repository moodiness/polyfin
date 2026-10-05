package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Seasons show the poster the metadata addon lists for them, by their
// place among the series' seasons, as Jellyfin shows a season's own image:
// in ImageTags, the series' in SeriesPrimaryImageTag. A season without one
// shows its series' poster, as before; so do its episodes as parents.
func TestSeasonsShowTheirOwnImages(t *testing.T) {
	var server *httptest.Server
	var metaRequests atomic.Int32
	show := func() stremio.Meta {
		return stremio.Meta{ID: "tt0200", Type: "series", Name: "Show", Poster: server.URL + "/poster.jpg",
			Background: server.URL + "/backdrop.jpg", ReleaseInfo: "2010-2012",
			Extras: &stremio.Extras{SeasonPosters: stremio.Posters{server.URL + "/s1.jpg", "", server.URL + "/s3.jpg"}},
			Videos: []stremio.Video{
				{ID: "tt0200:1:1", Title: "Pilot", Season: 1, Episode: 1, Released: "2010-06-16T00:00:00.000Z"},
				{ID: "tt0200:2:1", Title: "Return", Season: 2, Episode: 1, Released: "2011-06-16T00:00:00.000Z"},
				{ID: "tt0200:3:1", Title: "End", Season: 3, Episode: 1, Released: "2012-06-16T00:00:00.000Z"},
			}}
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "shows", Name: "Shows", Version: "1", Types: []string{"series"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs:  []stremio.Catalog{{Type: "series", ID: "shows", Name: "Shows"}}})
		case strings.HasPrefix(path, "/catalog/series/shows"):
			preview := show()
			preview.Videos, preview.Extras = nil, nil
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{preview}})
		case path == "/meta/series/tt0200.json":
			metaRequests.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": show()})
		case strings.HasSuffix(path, ".jpg"):
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpeg " + strings.TrimSuffix(strings.TrimPrefix(path, "/"), ".jpg")))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	s := newTestServer(t, 10)
	member := s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), server.URL+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "shows"}}); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	series := page.Items[0].Id
	user := member.ID.String()

	tags := map[string]string{}
	for _, image := range []string{"poster", "s1", "s3"} {
		tags[image] = library.ImageTag(server.URL + "/" + image + ".jpg")
	}
	var seasons QueryResult
	s.get(t, "/Shows/"+series+"/Seasons?userId="+user+"&Fields=PrimaryImageAspectRatio", token, &seasons)
	want := []string{"s1", "poster", "s3"}
	if len(seasons.Items) != len(want) {
		t.Fatalf("seasons: %+v", seasons.Items)
	}
	for i, season := range seasons.Items {
		if season.ImageTags["Primary"] != tags[want[i]] || season.SeriesPrimaryImageTag != tags["poster"] || season.PrimaryImageAspectRatio == nil {
			t.Errorf("season %d: image tags %v, series tag %q", *season.IndexNumber, season.ImageTags, season.SeriesPrimaryImageTag)
		}
	}

	// Described as Jellyfin 12.1 describes a season.
	status, body := s.call(http.MethodGet, "/Users/"+user+"/Items/"+seasons.Items[0].Id, app("tv", token), nil)
	raw, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.1", "season.json"))
	if err != nil {
		t.Fatal(err)
	}
	var recorded, got any
	if err := json.Unmarshal(raw, &recorded); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &got); status != http.StatusOK || err != nil {
		t.Fatalf("season: %d %s", status, body)
	}
	for _, difference := range compareShapes("season", recorded, got, browseShapes) {
		t.Error(difference)
	}

	// Episodes show their season's poster as their parent's, else their
	// series'.
	for i, season := range seasons.Items {
		var episodes QueryResult
		s.get(t, "/Shows/"+series+"/Episodes?seasonId="+season.Id+"&userId="+user, token, &episodes)
		parent := season.Id
		if want[i] == "poster" {
			parent = series
		}
		if len(episodes.Items) != 1 || episodes.Items[0].ParentPrimaryImageItemId != parent || episodes.Items[0].ParentPrimaryImageTag != tags[want[i]] {
			t.Errorf("episode of season %d: %+v", *season.IndexNumber, episodes.Items)
		}
	}

	// Each season's image route serves its own image, under its tag,
	// without asking the addon for the series again.
	asked := metaRequests.Load()
	for i, season := range seasons.Items {
		status, header, body := s.send(http.MethodGet, "/Items/"+season.Id+"/Images/Primary?tag="+season.ImageTags["Primary"], "", "", "")
		if status != http.StatusOK || string(body) != "jpeg "+want[i] || header.Get("ETag") != `"`+tags[want[i]]+`"` {
			t.Errorf("image of season %d: %d %q %s", *season.IndexNumber, status, body, header.Get("ETag"))
		}
	}
	if metaRequests.Load() != asked {
		t.Errorf("the images asked the addon for the series: %d requests, %d before", metaRequests.Load(), asked)
	}
}
