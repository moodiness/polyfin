package jellyfin

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

func TestRootFolderCountsAndSuggestionsAnswerAsJellyfin(t *testing.T) {
	t.Parallel()
	e := newEdited(t)
	var views QueryResult
	e.get(t, "/UserViews", e.member, &views)
	for _, path := range []string{"/Items/Root", "/Users/" + e.memberID + "/Items/Root", "/Items/" + rootFolderID.String()} {
		status, body := e.call(http.MethodGet, path, app("tv", e.member), nil)
		if status != http.StatusOK {
			t.Fatalf("%s: %d %s", path, status, body)
		}
		// Polyfin's root folder has no path on disk.
		matchesFixture(t, "root-folder", body, shapeRules{absent: map[string][]string{"Path": {"root-folder"}, "ChannelId": {"root-folder"},
			"ParentId": {"root-folder"}}, returned: map[string][]string{"CollectionType": {"root-folder"}}})
		var root BaseItemDto
		_ = json.Unmarshal(body, &root)
		if root.Type != "UserRootFolder" || root.Name != "Media Folders" || root.Id != rootFolderID.String() || *root.ChildCount != len(views.Items) {
			t.Errorf("%s: %s", path, body)
		}
	}
	var children QueryResult
	if status := e.get(t, "/Items?ParentId="+rootFolderID.String(), e.member, &children); status != http.StatusOK ||
		!slices.Equal(itemNames(children.Items), itemNames(views.Items)) {
		t.Errorf("root's children: %d %v, views %v", status, itemNames(children.Items), itemNames(views.Items))
	}

	status, body := e.call(http.MethodGet, "/Items/Counts", app("tv", e.member), nil)
	if status != http.StatusOK {
		t.Fatalf("counts: %d %s", status, body)
	}
	matchesFixture(t, "item-counts", body, shapeRules{})
	var counts ItemCounts
	_ = json.Unmarshal(body, &counts)
	if counts.MovieCount != 3 || counts.SeriesCount != 1 || counts.ItemCount != 4 {
		t.Errorf("counts: %s", body)
	}
	// The child does not count what they may not see.
	e.get(t, "/Items/Counts?userId="+e.childID, e.child, &counts)
	if counts.MovieCount != 2 || counts.SeriesCount != 0 {
		t.Errorf("child's counts: %+v", counts)
	}
	if status, _ := e.call(http.MethodPost, "/Users/"+e.memberID+"/FavoriteItems/"+e.ids["Allowed"], app("tv", e.member), nil); status != http.StatusOK {
		t.Fatalf("favorite: %d", status)
	}
	e.get(t, "/Items/Counts?isFavorite=true", e.member, &counts)
	if counts.MovieCount != 1 || counts.ItemCount != 1 {
		t.Errorf("favorite counts: %+v", counts)
	}
	e.get(t, "/Items/Counts?isFavorite=false", e.member, &counts)
	if counts.MovieCount != 2 || counts.SeriesCount != 1 {
		t.Errorf("counts of the others: %+v", counts)
	}

	for _, path := range []string{"/Items/Suggestions?type=Movie&limit=2", "/Users/" + e.memberID + "/Suggestions?type=Movie&limit=2"} {
		var suggestions QueryResult
		if status := e.get(t, path, e.member, &suggestions); status != http.StatusOK || len(suggestions.Items) != 2 ||
			slices.ContainsFunc(suggestions.Items, func(d BaseItemDto) bool { return d.Type != "Movie" }) {
			t.Errorf("%s: %d %v", path, status, suggestions.Items)
		}
	}
	var all QueryResult
	e.get(t, "/Items/Suggestions?mediaType=Video&enableTotalRecordCount=true", e.member, &all)
	if all.TotalRecordCount != 3 || len(all.Items) != 3 {
		t.Errorf("video suggestions: %d %v", all.TotalRecordCount, itemNames(all.Items))
	}
}

// emptySearchSuggestions is what jellyfin-web asks for the Suggestions of
// its search page before anything is typed.
const emptySearchSuggestions = "/Items?sortBy=IsFavoriteOrLiked&sortBy=Random&includeItemTypes=Movie,Series,MusicArtist&limit=20&recursive=true"

// The empty search page's Suggestions list titles picked at random from the
// user's libraries, as /Items/Suggestions picks them, their favorite and
// liked ones first: only what they may see, in the libraries they see,
// without the titles they hid.
func TestEmptySearchSuggestionsListTheTitlesTheUserSees(t *testing.T) {
	t.Parallel()
	e := newEdited(t)
	e.user("calm", func(c *accounts.UserChanges) { c.BlockedGenres = &[]string{"crime"} })
	suggested := func(token string) []string {
		t.Helper()
		var page QueryResult
		if status := e.get(t, emptySearchSuggestions, token, &page); status != http.StatusOK || page.TotalRecordCount != len(page.Items) {
			t.Fatalf("suggestions: %d %d %v", status, page.TotalRecordCount, itemNames(page.Items))
		}
		return itemNames(page.Items)
	}
	same := func(got []string, want ...string) bool {
		return len(got) == len(want) && !slices.ContainsFunc(want, func(name string) bool { return !slices.Contains(got, name) })
	}
	if got := suggested(e.member); !same(got, "Allowed", "Restricted", "Unrated", "Show") {
		t.Errorf("member's suggestions: %v", got)
	}
	if got := suggested(e.child); !same(got, "Allowed", "Unrated") {
		t.Errorf("child's suggestions: %v", got)
	}
	if got := suggested(e.signIn("calm", "phone")); !same(got, "Allowed", "Unrated") {
		t.Errorf("suggestions without the blocked genre: %v", got)
	}

	// A favorite and a liked title come first.
	for _, path := range []string{"/Users/" + e.memberID + "/FavoriteItems/" + e.ids["Unrated"], "/UserItems/" + e.ids["Show"] + "/Rating?likes=true"} {
		if status, body := e.call(http.MethodPost, path, app("tv", e.member), nil); status != http.StatusOK {
			t.Fatalf("%s: %d %s", path, status, body)
		}
	}
	for range 3 {
		if got := suggested(e.member); len(got) != 4 || !same(got[:2], "Unrated", "Show") {
			t.Errorf("favorite and liked first: %v", got)
		}
	}

	// A hidden title and a hidden library are left out.
	if status, body := e.call(http.MethodPost, "/Polyfin/UserItems/"+e.ids["Restricted"]+"/Hidden", app("tv", e.member), nil); status != http.StatusNoContent {
		t.Fatalf("hiding a title: %d %s", status, body)
	}
	member, _ := accounts.ParseID(e.memberID)
	shows, _ := accounts.ParseID(e.ids["Shows"])
	if _, err := e.store.UpdateUser(t.Context(), member, accounts.UserChanges{HiddenLibraries: &[]accounts.ID{shows}}, nil); err != nil {
		t.Fatal(err)
	}
	if got := suggested(e.member); !slices.Equal(got, []string{"Unrated", "Allowed"}) {
		t.Errorf("suggestions without the hidden title and library: %v", got)
	}
}

// Other listings without a folder still answer none: only the empty search
// page's Suggestions are answered from the user's libraries.
func TestOtherListingsWithoutAFolderStayEmpty(t *testing.T) {
	t.Parallel()
	e := newEdited(t)
	for _, path := range []string{
		"/Items?sortBy=DateCreated&sortOrder=Descending&includeItemTypes=Movie,Series&limit=20&recursive=true",
		"/Items?sortBy=Random&includeItemTypes=Movie,Episode&limit=20&recursive=true",
		"/Items?sortBy=Random&limit=20&recursive=true",
		"/Items?sortBy=IsFavoriteOrLiked&sortBy=Random&includeItemTypes=Movie,Series&limit=20",
		"/Items?includeItemTypes=Movie,Series&recursive=true",
	} {
		var page QueryResult
		if status := e.get(t, path, e.member, &page); status != http.StatusOK || len(page.Items) != 0 || page.TotalRecordCount != 0 {
			t.Errorf("%s: %d %d %v", path, status, page.TotalRecordCount, itemNames(page.Items))
		}
	}
}
