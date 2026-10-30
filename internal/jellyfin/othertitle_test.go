package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// mislabelledAddon lists a series whose second episode's streams are its
// own file, another episode's and the season's pack, and whose first
// episode's only stream is another episode's.
func mislabelledAddon(t *testing.T) string {
	t.Helper()
	show := stremio.Meta{ID: "tt6100", Type: "series", Name: "Harbor Tales", Videos: []stremio.Video{
		{ID: "tt6100:1:1", Title: "First", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00.000Z"},
		{ID: "tt6100:1:2", Title: "Second", Season: 1, Episode: 2, Released: "2020-01-08T00:00:00.000Z"},
	}}
	files := map[string][]string{
		"tt6100:1:1": {"Harbor.Tales.S01E05.1080p.mkv"},
		"tt6100:1:2": {"Harbor.Tales.S01E02.1080p.mkv", "Harbor.Tales.S01E03.1080p.mkv", "Harbor.Tales.S01.1080p.WEB.mkv"},
	}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.Path; {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "mislabelled", Name: "Mislabelled", Version: "1",
				Types: []string{"series"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "series", ID: "shows", Name: "Shows"}}})
		case strings.HasPrefix(path, "/catalog/series/shows"):
			preview := show
			preview.Videos = nil
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{preview}})
		case path == "/meta/series/tt6100.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": show})
		case strings.HasPrefix(path, "/stream/series/"):
			var streams []stremio.Stream
			for i, file := range files[strings.TrimSuffix(strings.TrimPrefix(path, "/stream/series/"), ".json")] {
				streams = append(streams, stremio.Stream{Name: []string{"One", "Two", "Three"}[i] + " 1080p", URL: server.URL + "/files/" + file,
					BehaviorHints: stremio.StreamBehavior{Filename: file, VideoSize: 2e9}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": streams})
		case strings.HasPrefix(path, "/files/"):
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader("\x1a\x45\xdf\xa3 media bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// An episode's release naming another episode is no longer offered, in
// item details or PlaybackInfo, without any setting, while its own file
// and the season's pack are; an episode whose every release names another
// keeps them, as the hide rules never hide every version.
func TestVersionsNamingAnotherEpisodeAreHidden(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	user := s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), mislabelledAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "shows"}}); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views, page, listed QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	s.get(t, "/Shows/"+page.Items[0].Id+"/Episodes", token, &listed)
	ids := map[string]string{}
	for _, episode := range listed.Items {
		ids[episode.Name] = episode.Id
	}
	s.listed(t, user, ids["First"], ids["Second"])
	detailed := func(name string) []string {
		t.Helper()
		var dto BaseItemDto
		if status := s.get(t, "/Users/"+user.ID.String()+"/Items/"+ids[name], token, &dto); status != http.StatusOK || dto.MediaSources == nil {
			t.Fatalf("details of %s: %d", name, status)
		}
		return firstWords(*dto.MediaSources)
	}
	if got := detailed("Second"); !slices.Equal(got, []string{"One", "Three"}) {
		t.Errorf("details of the second episode: %v", got)
	}
	if got := detailed("First"); !slices.Equal(got, []string{"One"}) {
		t.Errorf("details of the first episode: %v", got)
	}
	id, _ := accounts.ParseID(ids["Second"])
	versions, err := s.library.Versions(t.Context(), user, id)
	if err != nil || len(versions) != 3 {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	p := playbackSetup{testServer: s, token: token, user: user, movie: ids["Second"]}
	for _, version := range versions {
		p.analyzed(t, version, "h264-aac-mp4")
	}
	chrome := p.profile(t, "jellyfin-web-chrome")
	if answer := p.ask(t, token, p.movie, chrome, nil); !slices.Equal(firstWords(answer.MediaSources), []string{"One", "Three"}) {
		t.Errorf("PlaybackInfo: %v", firstWords(answer.MediaSources))
	}
	if answer := p.ask(t, token, p.movie, chrome, map[string]any{"MediaSourceId": versions[1].ID.String()}); !answer.refused() {
		t.Errorf("the third episode's release asked for: %+v", answer)
	}
}
