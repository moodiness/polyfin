package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// bigCollectionAddon serves a movie catalog, Big, of count titles in one
// page, and a collection catalog whose one collection groups Big.
func bigCollectionAddon(t *testing.T, count int) string {
	t.Helper()
	movies := make([]stremio.Meta, count)
	for i := range movies {
		movies[i] = stremio.Meta{ID: fmt.Sprintf("tt%07d", i), Type: "movie", Name: fmt.Sprintf("Movie %d", i)}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "big", Name: "Big", Version: "1", Types: []string{"movie", "collection"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "big", Name: "Big"}, {Type: "collection", ID: "groups", Name: "Groups"}}})
		case strings.HasPrefix(path, "/catalog/movie/big"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": movies})
		case strings.HasPrefix(path, "/catalog/collection/groups"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{{ID: "group:all", Type: "collection", Name: "All"}}})
		case strings.HasPrefix(path, "/meta/collection/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": stremio.Meta{ID: "group:all", Type: "collection", Name: "All",
				Collection: &stremio.Collection{Sources: []stremio.CollectionSource{{Type: "movie", CatalogID: "big"}}}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// A library or a collection listed without a limit, as jellyfin-web lists
// a collection's page, lists every title up to WholeListing, or up to its
// library's maximum; with a limit, as it asks.
func TestWholeListingsListMoreThanAPage(t *testing.T) {
	s := newTestServer(t, 10)
	s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), bigCollectionAddon(t, 250), false)
	if err != nil {
		t.Fatal(err)
	}
	libraries := func(most *int) {
		t.Helper()
		if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
			{AddonID: addon.ID, CatalogType: "movie", CatalogID: "big"},
			{AddonID: addon.ID, CatalogType: "collection", CatalogID: "groups", MaxItems: most},
		}); err != nil {
			t.Fatal(err)
		}
	}
	libraries(nil)
	token := s.signIn("member", "web")
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	ids := map[string]string{}
	for _, view := range views.Items {
		ids[view.Name] = view.Id
	}
	var groups QueryResult
	s.get(t, "/Items?ParentId="+ids["Groups"], token, &groups)
	if len(groups.Items) != 1 {
		t.Fatalf("the collections: %+v", groups.Items)
	}
	collection := groups.Items[0].Id
	listed := func(query string) (int, int) {
		t.Helper()
		var page QueryResult
		if status := s.get(t, "/Items?"+query, token, &page); status != http.StatusOK {
			t.Fatalf("%s: %d", query, status)
		}
		return len(page.Items), page.TotalRecordCount
	}
	for name, tc := range map[string]struct {
		query        string
		items, total int
	}{
		"a collection without a limit": {"ParentId=" + collection, 250, 250},
		"a collection with a limit":    {"ParentId=" + collection + "&Limit=100", 100, 250},
		"a library without a limit":    {"ParentId=" + ids["Big"], 250, 250},
		"a library from an index, all": {"ParentId=" + ids["Big"] + "&StartIndex=200", 50, 250},
	} {
		if items, total := listed(tc.query); items != tc.items || total != tc.total {
			t.Errorf("%s: %d items of %d, want %d of %d", name, items, total, tc.items, tc.total)
		}
	}
	// A library limited to fewer titles lists at most as many in its
	// collections, a limit asked or not.
	most := 120
	libraries(&most)
	if items, total := listed("ParentId=" + collection); items != 120 || total != 120 {
		t.Errorf("a collection of a library of 120 titles at most: %d items of %d", items, total)
	}
	if items, total := listed("ParentId=" + collection + "&StartIndex=100&Limit=100"); items != 20 || total != 120 {
		t.Errorf("its last page: %d items of %d", items, total)
	}
}
