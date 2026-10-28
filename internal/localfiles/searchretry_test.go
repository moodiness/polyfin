package localfiles

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/stremio"
)

// failingSearch is a metadata addon whose search catalog answers status
// to its first failures searches, then finds The Lone Ranger; searches
// counts them.
func failingSearch(t *testing.T, status, failures int) (manifest string, searches *atomic.Int32) {
	t.Helper()
	searches = &atomic.Int32{}
	show := stremio.Meta{ID: "tt0041038", Type: "series", Name: "The Lone Ranger", Year: "1949–1957"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "flaky", Name: "Flaky", Version: "1", Types: []string{"series"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "series", ID: "top", Name: "Top", Extra: []stremio.Extra{{Name: "search"}}}}})
		case strings.HasPrefix(path, "/catalog/series/top/search="):
			if int(searches.Add(1)) <= failures {
				http.Error(w, "failing", status)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{show}})
		case path == "/meta/series/tt0041038.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": show})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json", searches
}

// A search the metadata addon fails (HTTP 5xx) is tried again within the
// scan, and the file is matched when the addon answers then.
func TestSearchesAnAddonFailedAreTriedAgain(t *testing.T) {
	t.Parallel()
	manifest, searches := failingSearch(t, http.StatusGatewayTimeout, 1)
	e := newEnvWith(t, manifest)
	rel := "The Lone Ranger (1949)/Season 01/The Lone Ranger S01E01.mkv"
	addon, _ := e.folder(KindShows, map[string]int{rel: 10})
	if got := e.files(addon.ID)[rel]; got != "tt0041038" {
		t.Errorf("the file once the addon answers: %s", got)
	}
	if got := searches.Load(); got != 2 {
		t.Errorf("searches: %d", got)
	}
	folder, err := e.service.Folder(t.Context(), addon.ID)
	if err != nil || folder.SearchFailed != 0 || folder.SearchRetryAt != nil {
		t.Errorf("the folder: %d failed, retry at %v, %v", folder.SearchFailed, folder.SearchRetryAt, err)
	}
}

// A search the addon keeps failing is tried twice more, then left to the
// next scan; one it refuses (HTTP 4xx) is not tried again within the scan.
func TestSearchesTriedAgainAtMostTwice(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		status int
		want   int32
	}{{http.StatusBadGateway, 3}, {http.StatusNotFound, 1}} {
		manifest, searches := failingSearch(t, tc.status, 100)
		e := newEnvWith(t, manifest)
		rel := "The Lone Ranger (1949)/Season 01/The Lone Ranger S01E01.mkv"
		addon, _ := e.folder(KindShows, map[string]int{rel: 10})
		if got := e.files(addon.ID)[rel]; got != "unmatched: "+reasonSearchFailed {
			t.Errorf("HTTP %d: the file: %s", tc.status, got)
		}
		if got := searches.Load(); got != tc.want {
			t.Errorf("HTTP %d: searches %d, want %d", tc.status, got, tc.want)
		}
	}
}
