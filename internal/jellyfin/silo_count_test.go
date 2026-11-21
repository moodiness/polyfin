package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/silo/silotest"
	"github.com/moodiness/polyfin/internal/stremio"
)

// The count the Android app's library bar shows: a library's total, from
// a page of one title, unless total_exact is false (see
// silotest.CheckLibraryCount).

// countedCatalog is an addon of one movie catalog, Shelf, of count movies,
// a page of 100 at a time.
func countedCatalog(t *testing.T, count int) string {
	t.Helper()
	skip := regexp.MustCompile(`skip=(\d+)`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "shelf", Name: "Shelf", Version: "1", Types: []string{"movie"},
				Resources: []stremio.Resource{{Name: "catalog"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "shelf", Name: "Shelf", Extra: []stremio.Extra{{Name: "skip"}}}}})
		case strings.HasPrefix(path, "/catalog/movie/shelf"):
			from := 0
			if match := skip.FindStringSubmatch(path); match != nil {
				from, _ = strconv.Atoi(match[1])
			}
			metas := []stremio.Meta{}
			for i := from; i < min(from+100, count); i++ {
				metas = append(metas, stremio.Meta{ID: fmt.Sprintf("tt%07d", 8000000+i), Type: "movie", Name: fmt.Sprintf("Movie %03d", i)})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// count reads the library's first title in the sort asked, as the
// Android app reads its count, narrowed by facets, and returns the total
// and whether it is exact, once the page meets the dock's rule for a
// library of titles.
func (x shelf) count(t *testing.T, library, sort string, facets silotest.Facets, titles int) (int, bool) {
	t.Helper()
	status, data := x.do(t, http.MethodGet, "/api/v2/catalog?"+silotest.AppleCatalog(library, facets, sort, 1).Encode(), x.token, x.profile, nil)
	var page struct {
		Total      int  `json:"total"`
		TotalExact bool `json:"total_exact"`
	}
	if status != http.StatusOK || json.Unmarshal(data, &page) != nil {
		t.Fatalf("the count of %s by %q: %d %s", library, sort, status, data)
	}
	if err := silotest.CheckLibraryCount(data, titles); err != nil {
		t.Errorf("the count of %s by %q: %v", library, sort, err)
	}
	return page.Total, page.TotalExact
}

// An addon's catalog read in part tells its total is not exact, by title,
// so the Android app shows no count rather than the titles read so far.
// By date added, the listing sorts the titles read, and reads the rest of
// the catalog in the background: its total is not exact while the catalog
// is read in part, and exact, every title, once it is read whole.
func TestSiloAppsCountOfAnAddonCatalogReadInPart(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), countedCatalog(t, 250), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "movie", CatalogID: "shelf"}}); err != nil {
		t.Fatal(err)
	}
	x := newShelf(t, s, "movies")
	for _, sort := range []string{"", "-added_at", "title"} {
		total, exact := x.count(t, x.library, sort, silotest.Facets{}, 250)
		if sort == "-added_at" && exact && total == 250 {
			continue
		}
		if exact || total >= 250 {
			t.Errorf("by %q: %d, exact %v", sort, total, exact)
		}
	}
}

// A catalog that fits on a page, in any order, and an IPTV source's
// movies give their exact total. Filtered, a first page of one title
// stops reading once it has more than one, as filtered pages do: its
// total is the titles kept so far, not marked exact.
func TestSiloAppsCountOfWholeLibraries(t *testing.T) {
	t.Parallel()
	f := newFiltering(t)
	x := shelf{siloApps: f.siloApps, member: f.member, token: f.token, profile: f.profile, account: f.account}
	films := f.numbers["Films"]
	for _, tc := range []struct {
		sort          string
		facets        silotest.Facets
		titles, total int
		exact         bool
	}{{"", silotest.Facets{}, 5, 5, true}, {"title", silotest.Facets{}, 5, 5, true},
		{"", silotest.Facets{Genres: []string{"Action"}}, 2, 2, false}, {"title", silotest.Facets{Genres: []string{"Action"}}, 2, 2, false}} {
		if total, exact := x.count(t, films, tc.sort, tc.facets, tc.titles); exact != tc.exact || total != tc.total {
			t.Errorf("Films by %q, %+v: %d, exact %v", tc.sort, tc.facets, total, exact)
		}
	}
	names := make([]string, 140)
	for i := range names {
		names[i] = fmt.Sprintf("Box movie %03d", i)
	}
	stored := newStoredShelf(t, names, 0)
	for _, sort := range []string{"", "-added_at", "title"} {
		if total, exact := stored.count(t, stored.library, sort, silotest.Facets{}, len(names)); !exact || total != len(names) {
			t.Errorf("IPTV movies by %q: %d, exact %v", sort, total, exact)
		}
	}
}
