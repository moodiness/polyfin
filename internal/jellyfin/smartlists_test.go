package jellyfin

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/smartlists"
)

// makeSmartList saves a smart list of groupedAddon's catalogs, given as
// "type/id", of the server's or of owner's own, and returns its item.
func (s testServer) makeSmartList(t *testing.T, owner *accounts.ID, name, shownAs string, catalogs []string, sort string,
	rules ...smartlists.Rule) string {
	t.Helper()
	installed, err := s.addons.Addons(t.Context(), addons.Shared())
	if err != nil || len(installed) != 1 {
		t.Fatalf("addons: %v %v", installed, err)
	}
	list := smartlists.List{Owner: owner, Name: name, ShownAs: shownAs, Sort: sort,
		Rules: smartlists.Rule{Field: smartlists.FieldGroup, Match: smartlists.MatchAll, Rules: rules}}
	for _, c := range catalogs {
		kind, id, _ := strings.Cut(c, "/")
		list.Catalogs = append(list.Catalogs, smartlists.Catalog{AddonID: installed[0].ID, Type: kind, ID: id})
	}
	saved, err := smartlists.New(s.pool).Create(t.Context(), list)
	if err != nil {
		t.Fatal(err)
	}
	return library.SmartListItemID(saved.ID).String()
}

// listingNames lists a folder's children as token's user.
func (s testServer) listingNames(t *testing.T, token, path string) []string {
	t.Helper()
	var page QueryResult
	if status := s.get(t, path, token, &page); status != http.StatusOK {
		t.Fatalf("%s: %d", path, status)
	}
	return itemNames(page.Items)
}

// A smart list shown as a library is a view, with its home row: one
// without type when it draws from movies and series, as the Watchlist. It
// keeps its own order, and a title the user plays leaves a list of titles
// not played.
func TestASmartListShownAsALibraryIsAViewWithAHomeRow(t *testing.T) {
	t.Parallel()
	s, _, titles := grouping(t)
	s.user("member", nil)
	token := s.signIn("member", "tv")
	before := s.makeSmartList(t, nil, "Before 2002", smartlists.ShownAsLibrary, []string{"movie/top", "series/shows"}, smartlists.SortName,
		smartlists.Rule{Field: smartlists.FieldYear, Max: new(2001.0)},
		smartlists.Rule{Field: smartlists.FieldState, Value: smartlists.StateUnplayed})
	movies := s.makeSmartList(t, nil, "Movies", smartlists.ShownAsLibrary, []string{"movie/top"}, smartlists.SortCatalog)

	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	byName := map[string]BaseItemDto{}
	for _, view := range views.Items {
		byName[view.Name] = view
	}
	if view := byName["Before 2002"]; view.Id != before || view.Type != "CollectionFolder" || view.CollectionType != "" {
		t.Fatalf("views: %+v", views.Items)
	}
	if view := byName["Movies"]; view.Id != movies || view.CollectionType != "movies" {
		t.Errorf("a list of movies: %+v", view)
	}
	if got := s.listingNames(t, token, "/Items?ParentId="+before); !slices.Equal(got, []string{"Kids", "Teens"}) {
		t.Errorf("listing: %v", got)
	}
	// As jellyfin-web lists a library: the list's order holds.
	if got := s.listingNames(t, token, "/Items?ParentId="+before+"&Recursive=true&IncludeItemTypes=Movie,Series&SortBy=PremiereDate&SortOrder=Ascending"); !slices.Equal(got, []string{"Kids", "Teens"}) {
		t.Errorf("listing sorted by date: %v", got)
	}
	var latest []BaseItemDto
	s.get(t, "/Items/Latest?ParentId="+before+"&Limit=1", token, &latest)
	if got := itemNames(latest); !slices.Equal(got, []string{"Kids"}) {
		t.Errorf("home row: %v", got)
	}
	s.call(http.MethodPost, "/UserPlayedItems/"+titles.teens, app("tv", token), nil)
	if got := s.listingNames(t, token, "/Items?ParentId="+before); !slices.Equal(got, []string{"Kids"}) {
		t.Errorf("once Teens is played: %v", got)
	}
}

// A smart list shown as a collection is a BoxSet of the Collections view,
// which shows once there is one, and whose titles no app adds or removes.
func TestASmartListShownAsACollectionIsABoxSetThatRefusesTitles(t *testing.T) {
	t.Parallel()
	s, token, titles := grouping(t)
	dramas := s.makeSmartList(t, nil, "Dramas", smartlists.ShownAsCollection, []string{"movie/top", "series/shows"}, smartlists.SortPremiere,
		smartlists.Rule{Field: smartlists.FieldGenre, Value: "Drama"})
	views := s.viewIDs(t, token)
	if _, ok := views["Dramas"]; ok {
		t.Error("a list shown as a collection is a view")
	}
	var page QueryResult
	s.get(t, "/Items?ParentId="+views["Collections"]+"&IncludeItemTypes=BoxSet&Recursive=true", token, &page)
	if len(page.Items) != 1 || page.Items[0].Id != dramas || page.Items[0].Type != "BoxSet" || page.TotalRecordCount != 1 {
		t.Fatalf("Collections view: %+v", page.Items)
	}
	var item BaseItemDto
	if s.get(t, "/Items/"+dramas, token, &item); item.Type != "BoxSet" || item.Name != "Dramas" {
		t.Errorf("description: %+v", item)
	}
	if got := s.listingNames(t, token, "/Items?ParentId="+dramas); !slices.Equal(got, []string{"Show", "Adults"}) {
		t.Errorf("titles: %v", got)
	}
	// Apps other than jellyfin-web list it among every collection.
	s.get(t, "/Items?IncludeItemTypes=BoxSet&Recursive=true", token, &page)
	if !slices.Contains(itemNames(page.Items), "Dramas") {
		t.Errorf("across the server: %v", itemNames(page.Items))
	}
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		if status, body := s.call(method, "/Collections/"+dramas+"/Items?ids="+titles.kids, app("tv", token), nil); status != http.StatusForbidden || len(body) != 0 {
			t.Errorf("%s a title: %d %s", method, status, body)
		}
	}
	if got := s.listingNames(t, token, "/Items?ParentId="+dramas); !slices.Equal(got, []string{"Show", "Adults"}) {
		t.Errorf("titles after the refusals: %v", got)
	}
}

func TestAUsersOwnSmartListIsNotSeenByOthers(t *testing.T) {
	t.Parallel()
	s, _, _ := grouping(t)
	member := s.user("member", nil)
	s.user("other", nil)
	memberToken, otherToken := s.signIn("member", "tv"), s.signIn("other", "tv")
	mine := s.makeSmartList(t, &member.ID, "Mine", smartlists.ShownAsLibrary, []string{"movie/top"}, smartlists.SortName)
	shelf := s.makeSmartList(t, &member.ID, "My shelf", smartlists.ShownAsCollection, []string{"movie/top"}, smartlists.SortName)
	if views := s.viewIDs(t, memberToken); views["Mine"] != mine || views["Collections"] == "" {
		t.Fatalf("the member's views: %v", views)
	}
	if got := s.listingNames(t, memberToken, "/Items?ParentId="+mine); !slices.Equal(got, []string{"Adults", "Kids", "Teens"}) {
		t.Errorf("the member's listing: %v", got)
	}
	views := s.viewIDs(t, otherToken)
	if _, ok := views["Mine"]; ok {
		t.Errorf("another user's views: %v", views)
	}
	if _, ok := views["Collections"]; ok {
		t.Errorf("another user sees the Collections view of the member's list: %v", views)
	}
	for _, id := range []string{mine, shelf} {
		if status := s.get(t, "/Items/"+id, otherToken, nil); status != http.StatusNotFound {
			t.Errorf("another user describes %s: %d", id, status)
		}
		if status := s.get(t, "/Items?ParentId="+id, otherToken, nil); status == http.StatusOK {
			t.Errorf("another user lists %s: %d", id, status)
		}
	}
}
