package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// nestedCollectionsAddon serves a collection catalog, Genres, whose one
// collection, Animation, groups two collections, Movies and Series, each
// grouping a catalog of two titles: movies of 2012 and 2017, series of 2002
// and 2004.
func nestedCollectionsAddon(t *testing.T) string {
	t.Helper()
	titles := func(kind, prefix string, years ...int) []stremio.Meta {
		var result []stremio.Meta
		for i, year := range years {
			result = append(result, stremio.Meta{ID: fmt.Sprintf("%s%d", prefix, i+1), Type: kind, Name: fmt.Sprintf("%s %d", kind, i+1),
				Released: fmt.Sprintf("%d-05-04T00:00:00.000Z", year)})
		}
		return result
	}
	metas := map[string]stremio.Meta{
		"col:anim": {ID: "col:anim", Type: "collection", Name: "Animation", Collection: &stremio.Collection{Items: []stremio.Meta{
			{ID: "col:anim:movies", Name: "Movies"}, {ID: "col:anim:series", Name: "Series"}}}},
		"col:anim:movies": {ID: "col:anim:movies", Type: "collection", Name: "Movies",
			Collection: &stremio.Collection{Sources: []stremio.CollectionSource{{Type: "movie", CatalogID: "films"}}}},
		"col:anim:series": {ID: "col:anim:series", Type: "collection", Name: "Series",
			Collection: &stremio.Collection{Sources: []stremio.CollectionSource{{Type: "series", CatalogID: "shows"}}}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "nested", Name: "Nested", Version: "1", Types: []string{"movie", "series", "collection"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "collection", ID: "genres", Name: "Genres"}, {Type: "movie", ID: "films", Name: "Films"},
					{Type: "series", ID: "shows", Name: "Shows"}}})
		case strings.HasPrefix(path, "/catalog/collection/genres"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{{ID: "col:anim", Type: "collection", Name: "Animation"}}})
		case strings.HasPrefix(path, "/catalog/movie/films"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": titles("movie", "ttm", 2012, 2017)})
		case strings.HasPrefix(path, "/catalog/series/shows"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": titles("series", "tts", 2002, 2004)})
		case strings.HasPrefix(path, "/meta/collection/"):
			id, _ := url.PathUnescape(strings.TrimSuffix(strings.TrimPrefix(path, "/meta/collection/"), ".json"))
			meta, ok := metas[id]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// Streamyfin lists a collection for its movies, series and seasons,
// recursively: a collection that groups other collections then lists their
// titles, as Jellyfin's recursive listing does, rather than nothing; sorted
// as asked when listed whole, as a saga's movies by premiere date. Listed as
// jellyfin-web lists it, it still shows those collections.
func TestStreamyfinListsTheTitlesOfNestedCollections(t *testing.T) {
	s := newTestServer(t, 10)
	s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), nestedCollectionsAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "collection", CatalogID: "genres"},
	}); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "web")
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	var genres QueryResult
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &genres)
	if len(genres.Items) != 1 {
		t.Fatalf("the genres: %+v", genres.Items)
	}
	animation := genres.Items[0].Id
	names := func(result QueryResult) string {
		var listed []string
		for _, item := range result.Items {
			listed = append(listed, item.Name+":"+item.Type)
		}
		return strings.Join(listed, ", ")
	}
	titlesOf := func(query string) (int, string, int) {
		t.Helper()
		var result QueryResult
		status := s.get(t, "/Items?ParentId="+animation+"&Fields=ItemCounts,PrimaryImageAspectRatio,CanDelete,MediaSourceCount"+
			"&Recursive=true&IncludeItemTypes=Movie,Series,Season&"+query, token, &result)
		return status, names(result), result.TotalRecordCount
	}
	for _, tc := range []struct{ query, want string }{
		// Streamyfin's request: whole, sorted by premiere date.
		{"Limit=18&StartIndex=0&SortBy=PremiereDate&SortOrder=Ascending", "series 1:Series, series 2:Series, movie 1:Movie, movie 2:Movie"},
		// Unsorted, the catalogs' titles one of each in turn.
		{"Limit=18&StartIndex=0", "movie 1:Movie, series 1:Series, movie 2:Movie, series 2:Series"},
		// In pages, the catalogs' order, which pages sorted one by one would mix up.
		{"Limit=2&StartIndex=0&SortBy=PremiereDate&SortOrder=Ascending", "movie 1:Movie, series 1:Series"},
	} {
		if status, got, total := titlesOf(tc.query); status != http.StatusOK || got != tc.want || total != 4 {
			t.Errorf("Streamyfin's listing with %s: %d %s, total %d; want %s", tc.query, status, got, total, tc.want)
		}
	}
	var web QueryResult
	if status := s.get(t, "/Items?ParentId="+animation, token, &web); status != http.StatusOK ||
		names(web) != fmt.Sprintf("%s:BoxSet, %s:BoxSet", "Movies", "Series") {
		t.Errorf("jellyfin-web's listing: %d %s", status, names(web))
	}
}
