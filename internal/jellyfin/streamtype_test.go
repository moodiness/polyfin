package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// misnamedAddon offers a movie in two versions of Matroska files whose
// host answers video/mp4: one named as an MP4 file, one with no extension.
func misnamedAddon(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	meta := func() stremio.Meta {
		return stremio.Meta{ID: "tt3000", Type: "movie", Name: "Misnamed", Runtime: "2h", Year: "2008", Genres: []string{"Drama"}}
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "misnamed", Name: "Misnamed", Version: "1", Types: []string{"movie"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{meta()}})
		case path == "/meta/movie/tt3000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta()})
		case strings.HasPrefix(path, "/stream/movie/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{
				{Name: "Source 1080p", URL: server.URL + "/files/named", BehaviorHints: stremio.StreamBehavior{Filename: "Misnamed.1080p.mp4",
					VideoSize: stremio.Number64(len(mediaBytes))}},
				{Name: "Source 720p", URL: server.URL + "/files/bare", BehaviorHints: stremio.StreamBehavior{Filename: "Misnamed 720p",
					VideoSize: stremio.Number64(len(mediaBytes))}},
			}})
		case strings.HasPrefix(path, "/files/"):
			w.Header().Set("Content-Type", "video/mp4")
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader(mediaBytes))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// A version analyzed as Matroska is served as Matroska, whatever its file
// name says and its host answers, as its media sources describe it: to
// Jellyfin apps, and to Silo's apps, whose plan tells the same type.
func TestAnAnalyzedVersionIsServedAsItsContainer(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	user := s.user("member", nil)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), misnamedAddon(t), false); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	if len(page.Items) != 1 {
		t.Fatalf("titles: %+v", page.Items)
	}
	p := playbackSetup{testServer: s, token: token, user: user, movie: page.Items[0].Id}
	p.versions = p.versionsOf(t, p.movie)
	p.listed(t, user, p.movie)
	for _, version := range p.versions {
		p.analyzed(t, version, "remote-h264-aac-mkv")
	}
	for n, version := range p.versions {
		response, _ := fetchURL(t, s.url+"/Videos/"+p.movie+"/stream?static=true&mediaSourceId="+version.ID.String(),
			http.Header{"Authorization": {app("tv", token)}})
		if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "video/x-matroska" {
			t.Errorf("version %d (%s): %d %s", n, version.Filename, response.StatusCode, response.Header.Get("Content-Type"))
		}
	}

	allowSilo(t, s)
	a := servingSilo(t, s)
	siloToken, profile := a.signIn(t, "member")
	installation := a.installation(t, siloToken, profile)
	for n := range p.versions {
		file := a.fileOf(t, p.movie, siloToken, profile, n)
		status, data := a.do(t, http.MethodPost, "/api/v2/playback/start", siloToken, profile,
			siloStart(file, profile, installation, []string{"mkv", "mp4"}, []string{"h264"}, []string{"aac"}, nil))
		var d struct {
			Plan struct {
				Delivery string `json:"delivery"`
				Stream   struct {
					URL      string `json:"url"`
					MimeType string `json:"mime_type"`
				} `json:"stream"`
			} `json:"playback_plan"`
		}
		if status != http.StatusCreated || json.Unmarshal(data, &d) != nil || d.Plan.Delivery != "original_http" ||
			d.Plan.Stream.MimeType != "video/x-matroska" {
			t.Fatalf("version %d's plan: %d %s", n, status, data)
		}
		response, _ := fetchURL(t, a.site+d.Plan.Stream.URL, http.Header{"Authorization": {"Bearer " + siloToken}})
		if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != d.Plan.Stream.MimeType {
			t.Errorf("version %d's stream for Silo's apps: %d %s, the plan says %s", n, response.StatusCode,
				response.Header.Get("Content-Type"), d.Plan.Stream.MimeType)
		}
	}
}
