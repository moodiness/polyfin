package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/smartlists"
	"github.com/moodiness/polyfin/internal/stremio"
)

// collectionTypesOf is what /Polyfin/Items/{id}/CollectionTypes answers
// token's user, with its status.
func (s testServer) collectionTypesOf(t *testing.T, token, id string) ([]string, int) {
	t.Helper()
	var answer CollectionTypes
	status := s.get(t, "/Polyfin/Items/"+id+"/CollectionTypes", token, &answer)
	return answer.Types, status
}

// The web player's script offers a type filter on the page of a
// collection that holds several types of entries, and has the page list
// the type chosen alone: /Polyfin/Items/{id}/CollectionTypes tells the
// types a collection lists for the user, and its listing narrowed by
// IncludeItemTypes lists that type alone. Collections made by users, with
// the collections in them, and smart lists shown as collections both
// tell them; an item that is not a collection holds none.
func TestCollectionsTellTheTypesTheyHold(t *testing.T) {
	t.Parallel()
	s, admin, titles := grouping(t)
	inner := s.createCollection(t, admin, "name=Inner&ids="+titles.teens)
	picks := s.createCollection(t, admin, "name=Picks&ids="+titles.kids+","+titles.show+","+inner)
	movies := s.createCollection(t, admin, "name=Movies&ids="+titles.kids+","+titles.adults)
	mixed := s.makeSmartList(t, nil, "Mixed", smartlists.ShownAsCollection, []string{"movie/top", "series/shows"}, smartlists.SortName)
	top := s.makeSmartList(t, nil, "Top", smartlists.ShownAsCollection, []string{"movie/top"}, smartlists.SortName)
	s.user("member", nil)
	s.user("child", func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{MaxRating: new(10)} })
	member := s.signIn("member", "phone")
	child := s.signIn("child", "tablet")

	for name, tc := range map[string]struct {
		token, id string
		want      []string
	}{
		"a collection of a movie, a series and a collection": {member, picks, []string{"BoxSet", "Movie", "Series"}},
		// The series and the collection in it are out of the child's reach.
		"the same for a child":              {child, picks, []string{"Movie"}},
		"a collection of movies":            {member, movies, []string{"Movie"}},
		"a smart list of movies and series": {member, mixed, []string{"Movie", "Series"}},
		"a smart list of movies":            {member, top, []string{"Movie"}},
		"a movie":                           {member, titles.kids, []string{}},
	} {
		if got, status := s.collectionTypesOf(t, tc.token, tc.id); status != http.StatusOK || !slices.Equal(got, tc.want) {
			t.Errorf("%s: %d %q, want %q", name, status, got, tc.want)
		}
	}
	if _, status := s.collectionTypesOf(t, member, "0123456789abcdef0123456789abcdef"); status != http.StatusNotFound {
		t.Errorf("an unknown item: %d", status)
	}

	// jellyfin-web asks a collection's page without a limit; the script
	// adds the type chosen.
	for name, tc := range map[string]struct {
		id, types string
		want      []string
	}{
		"the collection's movies":      {picks, "Movie", []string{"Kids"}},
		"the collection's series":      {picks, "Series", []string{"Show"}},
		"the collection's collections": {picks, "BoxSet", []string{"Inner"}},
		"the smart list's movies":      {mixed, "Movie", []string{"Adults", "Kids", "Teens"}},
		"the smart list's series":      {mixed, "Series", []string{"Show"}},
	} {
		path := "/Items?ParentId=" + tc.id + "&Fields=ItemCounts,PrimaryImageAspectRatio,CanDelete,MediaSourceCount&IncludeItemTypes=" + tc.types
		if got := s.listingNames(t, member, path); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %q, want %q", name, got, tc.want)
		}
	}
}

// pastTheBoundAddon serves a catalog of count recent movies, in one page,
// and one of a single older series.
func pastTheBoundAddon(t *testing.T, count int) string {
	t.Helper()
	movies := make([]stremio.Meta, count)
	for i := range movies {
		movies[i] = stremio.Meta{ID: fmt.Sprintf("tt%07d", i), Type: "movie", Name: fmt.Sprintf("Movie %d", i), ReleaseInfo: "2020",
			Released: "2020-06-01T00:00:00.000Z"}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.EscapedPath(), ".json")
		switch {
		case path == "/manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "bound", Name: "Bound", Version: "1", Types: []string{"movie", "series"},
				Resources: []stremio.Resource{{Name: "catalog"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "recent", Name: "Recent"}, {Type: "series", ID: "old", Name: "Old"}}})
		case strings.HasPrefix(path, "/catalog/movie/recent"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": movies})
		case strings.HasPrefix(path, "/catalog/series/old"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{{ID: "tt9999999", Type: "series", Name: "Old Show",
				ReleaseInfo: "1990", Released: "1990-01-01T00:00:00.000Z"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// A collection's page lists its first titles only, up to WholeListing: a
// smart list of more recent movies than that, sorted by premiere date,
// shows none of its older series there. It still tells it holds series,
// and its page narrowed to them lists them.
func TestATypePastTheBoundOfACollectionsPageCounts(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.waitForWholeListings()
	if _, err := s.store.CreateUser(t.Context(), accounts.NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.Install(t.Context(), addons.Shared(), pastTheBoundAddon(t, 520), false); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("admin", "web")
	list := s.makeSmartList(t, nil, "Everything", smartlists.ShownAsCollection, []string{"movie/recent", "series/old"}, smartlists.SortPremiere)
	var page QueryResult
	if status := s.get(t, "/Items?ParentId="+list, token, &page); status != http.StatusOK || len(page.Items) != 500 ||
		slices.ContainsFunc(page.Items, func(item BaseItemDto) bool { return item.Type != "Movie" }) {
		t.Fatalf("the page: %d, %d items of %d", status, len(page.Items), page.TotalRecordCount)
	}
	if got, status := s.collectionTypesOf(t, token, list); status != http.StatusOK || !slices.Equal(got, []string{"Movie", "Series"}) {
		t.Errorf("the types: %d %q", status, got)
	}
	if got := s.listingNames(t, token, "/Items?ParentId="+list+"&IncludeItemTypes=Series"); !slices.Equal(got, []string{"Old Show"}) {
		t.Errorf("its series: %q", got)
	}
}
