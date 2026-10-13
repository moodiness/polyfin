package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// mixedCollectionsAddon serves a collection catalog, Sets, of two
// collections grouping a movie catalog of two movies and a series catalog
// of two series: Mixed groups both catalogs, and Nested groups two
// collections, one of each catalog.
func mixedCollectionsAddon(t *testing.T) string {
	t.Helper()
	titles := func(kind string) []stremio.Meta {
		var result []stremio.Meta
		for i := range 2 {
			result = append(result, stremio.Meta{ID: fmt.Sprintf("tt%s%d", kind, i+1), Type: kind, Name: fmt.Sprintf("%s %d", kind, i+1),
				Released: fmt.Sprintf("20%02d-05-04T00:00:00.000Z", 10+i), Runtime: "1h 30min"})
		}
		return result
	}
	collections := map[string]stremio.Meta{
		"col:mixed": {ID: "col:mixed", Type: "collection", Name: "Mixed", Collection: &stremio.Collection{Sources: []stremio.CollectionSource{
			{Type: "movie", CatalogID: "top"}, {Type: "series", CatalogID: "shows"}}}},
		"col:nested": {ID: "col:nested", Type: "collection", Name: "Nested", Collection: &stremio.Collection{Items: []stremio.Meta{
			{ID: "col:movies", Name: "Movies"}, {ID: "col:series", Name: "Series"}}}},
		"col:movies": {ID: "col:movies", Type: "collection", Name: "Movies",
			Collection: &stremio.Collection{Sources: []stremio.CollectionSource{{Type: "movie", CatalogID: "top"}}}},
		"col:series": {ID: "col:series", Type: "collection", Name: "Series",
			Collection: &stremio.Collection{Sources: []stremio.CollectionSource{{Type: "series", CatalogID: "shows"}}}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		id, _ := url.PathUnescape(strings.TrimSuffix(path[strings.LastIndex(path, "/")+1:], ".json"))
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "mixed", Name: "Mixed", Version: "1", Types: []string{"movie", "series", "collection"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "collection", ID: "sets", Name: "Sets"}, {Type: "movie", ID: "top", Name: "Top"},
					{Type: "series", ID: "shows", Name: "Shows"}}})
		case strings.HasPrefix(path, "/catalog/collection/sets"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{
				{ID: "col:mixed", Type: "collection", Name: "Mixed"}, {ID: "col:nested", Type: "collection", Name: "Nested"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": titles("movie")})
		case strings.HasPrefix(path, "/catalog/series/shows"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": titles("series")})
		case strings.HasPrefix(path, "/meta/collection/") && collections[id].ID != "":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": collections[id]})
		case strings.HasPrefix(path, "/meta/"):
			for _, meta := range append(titles("movie"), titles("series")...) {
				if meta.ID == id {
					_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
					return
				}
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// The titles of a collection library listed recursively, as apps list a
// library's titles, answer the filters of what the user did with them as a
// library of titles answers them: the titles kept, paged, out of a total
// that counts them. An unwatched listing listed none, out of 1, its one
// collection, before.
func TestCollectionLibraryTitlesAnswerTheUserFilters(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), mixedCollectionsAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "collection", CatalogID: "sets"},
	}); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	titles := "/Items?ParentId=" + views.Items[0].Id + "&Recursive=true"
	var all QueryResult
	s.get(t, titles+"&IncludeItemTypes=Movie,Series", token, &all)
	ids := map[string]string{}
	for _, item := range all.Items {
		ids[item.Name] = item.Id
	}
	if len(ids) != 4 || all.TotalRecordCount != 4 {
		t.Fatalf("the library's titles: %v of %d", itemNames(all.Items), all.TotalRecordCount)
	}
	send := func(path string, body any, want int) {
		t.Helper()
		if status, answer := s.call(http.MethodPost, path, app("tv", token), body); status != want {
			t.Fatalf("%s: %d %s", path, status, answer)
		}
	}
	send("/UserPlayedItems/"+ids["movie 1"], nil, http.StatusOK)
	send("/UserFavoriteItems/"+ids["series 2"], nil, http.StatusOK)
	send("/Sessions/Playing", map[string]any{"ItemId": ids["movie 2"], "PositionTicks": 0}, http.StatusNoContent)
	send("/Sessions/Playing/Progress", map[string]any{"ItemId": ids["movie 2"], "PositionTicks": 27_000_000_000}, http.StatusNoContent)

	// listed reads a listing page by page, two titles at a time, and
	// checks that every page has the same total, which counts the titles.
	listed := func(query string) []string {
		t.Helper()
		var names []string
		total := -1
		for start := 0; start < 10; start += 2 {
			var page QueryResult
			if status := s.get(t, fmt.Sprintf("%s&%s&StartIndex=%d&Limit=2", titles, query, start), token, &page); status != http.StatusOK {
				t.Fatalf("%s from %d: %d", query, start, status)
			}
			if total >= 0 && page.TotalRecordCount != total {
				t.Errorf("%s: total %d from %d, %d before", query, page.TotalRecordCount, start, total)
			}
			total = page.TotalRecordCount
			names = append(names, itemNames(page.Items)...)
			if len(page.Items) < 2 {
				break
			}
		}
		if total != len(names) {
			t.Errorf("%s: %v, out of %d", query, names, total)
		}
		slices.Sort(names)
		return names
	}
	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"IncludeItemTypes=Movie,Series&Filters=IsUnplayed", []string{"movie 2", "series 1", "series 2"}},
		{"IncludeItemTypes=Movie,Series&isPlayed=false", []string{"movie 2", "series 1", "series 2"}},
		{"IncludeItemTypes=Movie&Filters=IsUnplayed", []string{"movie 2"}},
		{"IncludeItemTypes=Movie,Series&Filters=IsPlayed", []string{"movie 1"}},
		{"IncludeItemTypes=Movie,Series&isPlayed=true", []string{"movie 1"}},
		{"IncludeItemTypes=Movie,Series&Filters=IsFavorite", []string{"series 2"}},
		{"IncludeItemTypes=Movie,Series&isFavorite=true", []string{"series 2"}},
		{"IncludeItemTypes=Movie,Series&Filters=IsResumable", []string{"movie 2"}},
	} {
		if got := listed(tc.query); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %v, want %v", tc.query, got, tc.want)
		}
	}
}

// A listing of an addon's collection narrowed by item type lists what it
// keeps, a page at a time, out of a total that counts it: none for BoxSet,
// as the collection lists titles, never the collections within it; its
// movies alone, or its series, whether it groups catalogs or collections.
// A collection library listed without Recursive, by its collections,
// counts none for titles. They answered no item, or pages short, out of
// every title or collection, before.
func TestTypedCollectionListingsCountWhatTheyKeep(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), mixedCollectionsAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "collection", CatalogID: "sets"},
	}); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views, sets QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &sets)
	if got := itemNames(sets.Items); !slices.Equal(got, []string{"Mixed", "Nested"}) {
		t.Fatalf("the collections: %v", got)
	}
	check := func(parent, name, query string, want []string) {
		t.Helper()
		var names []string
		for start := range 6 {
			var page QueryResult
			if status := s.get(t, fmt.Sprintf("/Items?ParentId=%s&%s&StartIndex=%d&Limit=1", parent, query, start), token, &page); status != http.StatusOK {
				t.Fatalf("%s with %s from %d: %d", name, query, start, status)
			}
			if page.TotalRecordCount != len(want) {
				t.Errorf("%s with %s from %d: total %d, want %d", name, query, start, page.TotalRecordCount, len(want))
			}
			if len(page.Items) == 0 {
				break
			}
			names = append(names, itemNames(page.Items)...)
		}
		slices.Sort(names)
		if !slices.Equal(names, want) {
			t.Errorf("%s with %s: %v, want %v", name, query, names, want)
		}
	}
	for _, collection := range sets.Items {
		check(collection.Id, collection.Name, "IncludeItemTypes=BoxSet", nil)
		check(collection.Id, collection.Name, "IncludeItemTypes=Movie", []string{"movie 1", "movie 2"})
		check(collection.Id, collection.Name, "IncludeItemTypes=Series", []string{"series 1", "series 2"})
	}
	check(views.Items[0].Id, "the collection library", "IncludeItemTypes=Movie", nil)
	check(views.Items[0].Id, "the collection library", "IncludeItemTypes=BoxSet", []string{"Mixed", "Nested"})
	check(views.Items[0].Id, "the collection library", "IncludeItemTypes=Series&Recursive=true", []string{"series 1", "series 2"})
}
