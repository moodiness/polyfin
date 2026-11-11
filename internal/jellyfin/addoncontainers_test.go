package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/homerows"
	"github.com/moodiness/polyfin/internal/smartlists"
	"github.com/moodiness/polyfin/internal/stremio"
)

// ratedCollectionsAddon serves a catalog of collections, in order: Family,
// of a catalog of Kids (G); Nights, of a catalog of Adults (R); Rated,
// rated R itself, of Family's catalog; Outer, which groups a collection of
// Family's catalog; and Empty, which groups nothing. Catalog rows carry no
// rating: the titles' descriptions do.
func ratedCollectionsAddon(t *testing.T) string {
	t.Helper()
	titles := map[string]stremio.Meta{
		"tt1": {ID: "tt1", Type: "movie", Name: "Kids", Extras: &stremio.Extras{Certification: "G"}},
		"tt3": {ID: "tt3", Type: "movie", Name: "Adults", Extras: &stremio.Extras{Certification: "R"}},
	}
	row := func(id string) stremio.Meta {
		m := titles[id]
		m.Extras = nil
		return m
	}
	catalogs := map[string][]stremio.Meta{
		"collection/groups": {
			{ID: "group:family", Type: "collection", Name: "Family"}, {ID: "group:nights", Type: "collection", Name: "Nights"},
			{ID: "group:rated", Type: "collection", Name: "Rated"}, {ID: "group:outer", Type: "collection", Name: "Outer"},
			{ID: "group:empty", Type: "collection", Name: "Empty"},
		},
		"movie/family": {row("tt1")},
		"movie/nights": {row("tt3")},
	}
	family := []stremio.CollectionSource{{Type: "movie", CatalogID: "family"}}
	collections := map[string]stremio.Meta{
		"group:family": {Collection: &stremio.Collection{Sources: family}},
		"group:nights": {Collection: &stremio.Collection{Sources: []stremio.CollectionSource{{Type: "movie", CatalogID: "nights"}}}},
		"group:rated":  {Collection: &stremio.Collection{Sources: family}, Extras: &stremio.Extras{Certification: "R"}},
		"group:outer":  {Collection: &stremio.Collection{Items: []stremio.Meta{{ID: "group:inner", Type: "collection", Name: "Inner"}}}},
		"group:inner":  {Collection: &stremio.Collection{Sources: family}},
		"group:empty":  {Collection: &stremio.Collection{}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.EscapedPath(), ".json")
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
		switch {
		case path == "/manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "ratedgroups", Name: "Rated groups", Version: "1",
				Types: []string{"movie", "collection"}, IDPrefixes: []string{"tt", "group:"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "collection", ID: "groups", Name: "Groups"},
					{Type: "movie", ID: "family", Name: "Family movies"}, {Type: "movie", ID: "nights", Name: "Night movies"}}})
		case len(parts) == 3 && parts[0] == "catalog":
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": catalogs[parts[1]+"/"+parts[2]]})
		case len(parts) == 3 && parts[0] == "meta" && parts[1] == "collection":
			id, _ := url.PathUnescape(parts[2])
			meta, ok := collections[id]
			if !ok {
				http.NotFound(w, r)
				return
			}
			meta.ID, meta.Type, meta.Name = id, "collection", id
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
		case len(parts) == 3 && parts[0] == "meta":
			meta, ok := titles[parts[2]]
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

// An addon's collection none of whose titles a user may see, judged on the
// first page of its catalogs, or rated above their limit itself, is hidden
// from them, as one made by users is; so is a smart list shown as a
// collection whose read keeps no title for them. Listings count and page
// what they show, their own pages answer as missing, and the highest
// rating a listing asks hides them from anyone.
func TestAddonCollectionsAndSmartListsWithoutVisibleTitlesAreHidden(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	child := s.user("child", func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{MaxRating: new(13)} })
	addon, err := s.addons.Install(t.Context(), addons.Shared(), ratedCollectionsAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "collection", CatalogID: "groups"},
	}); err != nil {
		t.Fatal(err)
	}
	cartoons := s.makeSmartList(t, nil, "Cartoons", smartlists.ShownAsCollection, []string{"movie/family"}, smartlists.SortCatalog)
	late := s.makeSmartList(t, nil, "Late shows", smartlists.ShownAsCollection, []string{"movie/nights"}, smartlists.SortCatalog)
	memberToken, childToken := s.signIn("member", "tv"), s.signIn("child", "tablet")
	views := s.viewIDs(t, memberToken)
	groups := "/Items?ParentId=" + views["Groups"]
	listing := func(token, path string) ([]string, int) {
		t.Helper()
		var page QueryResult
		if status := s.get(t, path, token, &page); status != http.StatusOK {
			t.Fatalf("%s: %d", path, status)
		}
		return itemNames(page.Items), page.TotalRecordCount
	}
	all := []string{"Family", "Nights", "Rated", "Outer", "Empty"}
	var page QueryResult
	s.get(t, groups, memberToken, &page)
	ids := map[string]string{}
	for _, item := range page.Items {
		ids[item.Name] = item.Id
	}
	if got := itemNames(page.Items); !slices.Equal(got, all) || page.TotalRecordCount != len(all) {
		t.Fatalf("the member's collections: %v, counted %d", got, page.TotalRecordCount)
	}
	shown := []string{"Family", "Outer", "Empty"}
	if got, total := listing(childToken, groups); !slices.Equal(got, shown) || total != len(shown) {
		t.Errorf("the child's collections: %v, counted %d", got, total)
	}
	if got, total := listing(childToken, groups+"&StartIndex=1&Limit=1"); !slices.Equal(got, []string{"Outer"}) || total != len(shown) {
		t.Errorf("the child's second collection: %v, counted %d", got, total)
	}
	// The highest rating a listing asks hides them from anyone; its other
	// filters judge the titles, not whether a collection shows.
	if got, total := listing(memberToken, groups+"&maxOfficialRating=PG-13"); !slices.Equal(got, shown) || total != len(shown) {
		t.Errorf("the member's collections under PG-13: %v, counted %d", got, total)
	}
	if got, _ := listing(memberToken, groups+"&officialRatings=G"); !slices.Equal(got, all) {
		t.Errorf("the member's collections rated G: %v", got)
	}
	if got, _ := listing(childToken, "/Items?ParentId="+ids["Outer"]); !slices.Equal(got, []string{"Kids"}) {
		t.Errorf("the child's titles of Outer: %v", got)
	}
	// Apps other than jellyfin-web list them across the server.
	if got, _ := listing(childToken, "/Items?IncludeItemTypes=BoxSet&Recursive=true"); !slices.Equal(got, []string{"Cartoons", "Empty", "Family", "Outer"}) {
		t.Errorf("the child's collections across the server: %v", got)
	}
	collections := "/Items?ParentId=" + views["Collections"] + "&IncludeItemTypes=BoxSet&Recursive=true"
	if got, _ := listing(memberToken, collections); !slices.Equal(got, []string{"Cartoons", "Late shows"}) {
		t.Errorf("the member's Collections view: %v", got)
	}
	if got, total := listing(childToken, collections); !slices.Equal(got, []string{"Cartoons"}) || total != 1 {
		t.Errorf("the child's Collections view: %v, counted %d", got, total)
	}
	if got, _ := listing(childToken, "/Items?ParentId="+cartoons); !slices.Equal(got, []string{"Kids"}) {
		t.Errorf("the child's Cartoons: %v", got)
	}
	for _, id := range []string{ids["Nights"], ids["Rated"], late} {
		if status := s.get(t, "/Users/"+child.ID.String()+"/Items/"+id, childToken, nil); status != http.StatusNotFound {
			t.Errorf("the child's page of %s: %d", id, status)
		}
		if status := s.get(t, "/Users/"+member.ID.String()+"/Items/"+id, memberToken, nil); status != http.StatusOK {
			t.Errorf("the member's page of %s: %d", id, status)
		}
		s.expectStatus(t, http.StatusNotFound, http.MethodGet, "/Items?ParentId="+id, childToken, nil)
		s.expectStatus(t, http.StatusOK, http.MethodGet, "/Items?ParentId="+id, memberToken, nil)
	}
	// A hidden collection makes no home row.
	nights, _ := accounts.ParseID(ids["Nights"])
	familyID, _ := accounts.ParseID(ids["Family"])
	s.addRow(t, homerows.Row{Name: "Nights", Kind: homerows.KindCollection, Place: homerows.PlaceBottom, Collection: nights, Everyone: true})
	s.addRow(t, homerows.Row{Name: "Family", Kind: homerows.KindCollection, Place: homerows.PlaceBottom, Collection: familyID, Everyone: true})
	if got := rowSummary(s.customRowsOf(t, childToken)); !slices.Equal(got, []string{"Family @Bottom: Kids"}) {
		t.Errorf("the child's rows: %q", got)
	}
}
