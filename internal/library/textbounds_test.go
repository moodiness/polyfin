package library

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// The names and overviews an addon gives are bounded, on a character, in
// its libraries' names, its listings, the records and search.
func TestAddonsNamesAndOverviewsAreBounded(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	// 2 MiB of two-byte characters, after one byte: no bound falls
	// between two characters.
	long := "x" + strings.Repeat("é", 1<<20)
	search := []stremio.Extra{{Name: "search", IsRequired: true}}
	e.install(addons.Shared(), &fakeAddon{
		manifest: stremio.Manifest{ID: "long", Name: "Long", Version: "1", Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: long}, {Type: "movie", ID: "search", Name: "Search", Extra: search}}},
		catalogs: map[string][]stremio.Meta{
			"movie/top":    {{ID: "tt0000001", Type: "movie", Name: long, Description: long}},
			"movie/search": {{ID: "tt0000001", Type: "movie", Name: long, Description: long}},
		},
	})
	check := func(where string, item Item) {
		t.Helper()
		if len(item.Name) > stremio.MaxName || !utf8.ValidString(item.Name) || !strings.HasPrefix(long, item.Name) || item.Name == "" {
			t.Errorf("%s: a name of %d bytes, valid %v", where, len(item.Name), utf8.ValidString(item.Name))
		}
		if len(item.Overview) > stremio.MaxOverview || !utf8.ValidString(item.Overview) || !strings.HasPrefix(long, item.Overview) {
			t.Errorf("%s: an overview of %d bytes, valid %v", where, len(item.Overview), utf8.ValidString(item.Overview))
		}
	}
	libraries, err := e.service.Libraries(t.Context(), e.member)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(libraries, func(l Item) bool { return strings.HasPrefix(l.Name, "xé") })
	if i < 0 {
		t.Fatalf("no library named after the catalog in %d", len(libraries))
	}
	if name := libraries[i].Name; len(name) > stremio.MaxName || !utf8.ValidString(name) {
		t.Errorf("a library named in %d bytes, valid %v", len(name), utf8.ValidString(name))
	}
	page, err := e.service.Children(t.Context(), e.member, libraries[i].ID, 0, 10, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("listing: %d items, %v", len(page.Items), err)
	}
	check("listing", page.Items[0])
	recorded, err := e.service.Item(t.Context(), e.member, page.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	check("record", recorded)
	var name, overview int
	if err := e.service.db.QueryRow(t.Context(), `SELECT max(octet_length(data->'meta'->>'name')),
		max(octet_length(coalesce(data->'meta'->>'description', ''))) FROM items WHERE kind = 'movie'`).Scan(&name, &overview); err != nil {
		t.Fatal(err)
	}
	if name > stremio.MaxName || overview > stremio.MaxOverview {
		t.Errorf("stored: a name of %d bytes, an overview of %d", name, overview)
	}
	found, err := e.service.Search(t.Context(), e.member, "x", []Kind{KindMovie}, 10)
	if err != nil || len(found) != 1 {
		t.Fatalf("search: %d found, %v", len(found), err)
	}
	check("search", found[0])
}
