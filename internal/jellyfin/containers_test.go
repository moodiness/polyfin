package jellyfin

import (
	"net/http"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// A collection or a playlist none of whose titles a user may see is
// hidden from them, whatever hides the titles; one without titles still
// shows. Listings count and page what they show, its own pages answer as
// missing, and a listing's rating filter reaches the titles inside.
func TestContainersWithoutVisibleTitlesAreHidden(t *testing.T) {
	t.Parallel()
	e := newEdited(t)
	e.user("curator", func(c *accounts.UserChanges) { c.IsAdministrator, c.CollectionManagement = new(true), new(true) })
	curator := e.signIn("curator", "tv")
	restricted := e.ids["Restricted"]
	adults := e.createCollection(t, curator, "name=Adults&ids="+restricted)
	mixed := e.createCollection(t, curator, "name=Mixed&ids="+e.ids["Allowed"]+","+restricted)
	e.createCollection(t, curator, "name=Empty")
	adultList := e.createPlaylist(t, curator, map[string]any{"Name": "Late", "Ids": []string{restricted}, "IsPublic": true})
	mixedList := e.createPlaylist(t, curator, map[string]any{"Name": "Mix", "Ids": []string{e.ids["Allowed"], restricted}, "IsPublic": true})
	names := func(token, path string) ([]string, int) {
		t.Helper()
		return e.filtered(t, token, path)
	}
	collections := "/Items?includeItemTypes=BoxSet&Recursive=true"
	if got, total := names(e.member, collections); !slices.Equal(got, []string{"Adults", "Empty", "Mixed"}) || total != 3 {
		t.Errorf("the member's collections: %v, counted %d", got, total)
	}
	if got, total := names(e.child, collections); !slices.Equal(got, []string{"Empty", "Mixed"}) || total != 2 {
		t.Errorf("the child's collections: %v, counted %d", got, total)
	}
	var page QueryResult
	e.get(t, collections+"&SortBy=SortName&StartIndex=1&Limit=1", e.child, &page)
	if len(page.Items) != 1 || page.Items[0].Name != "Mixed" || page.TotalRecordCount != 2 {
		t.Errorf("the child's second collection: %v, counted %d", itemNames(page.Items), page.TotalRecordCount)
	}
	// The highest rating an app asks hides them from anyone.
	if got, total := names(e.member, collections+"&maxOfficialRating=G"); !slices.Equal(got, []string{"Empty"}) || total != 1 {
		t.Errorf("under G: %v, counted %d", got, total)
	}
	// It reaches the titles of a collection and of a playlist.
	if got, total := names(e.member, "/Items?ParentId="+mixed+"&maxOfficialRating=PG-13"); !slices.Equal(got, []string{"Allowed"}) || total != 1 {
		t.Errorf("the mixed collection under PG-13: %v, counted %d", got, total)
	}
	if got, total := names(e.member, "/Items?ParentId="+mixedList+"&maxOfficialRating=PG-13"); !slices.Equal(got, []string{"Allowed"}) || total != 1 {
		t.Errorf("the mixed playlist under PG-13: %v, counted %d", got, total)
	}
	playlistsAsked := "/Items?includeItemTypes=Playlist&Recursive=true"
	if got, total := names(e.child, playlistsAsked); !slices.Contains(got, "Mix") || slices.Contains(got, "Late") || total != len(got) {
		t.Errorf("the child's playlists: %v, counted %d", got, total)
	}
	if got, _ := names(e.member, playlistsAsked); !slices.Contains(got, "Late") {
		t.Errorf("the member's playlists: %v", got)
	}
	for _, id := range []string{adults, adultList} {
		if status, _ := e.item(t, e.child, e.childID, id); status != http.StatusNotFound {
			t.Errorf("the child's page of %s: %d", id, status)
		}
		if status, _ := e.item(t, e.member, e.memberID, id); status != http.StatusOK {
			t.Errorf("the member's page of %s: %d", id, status)
		}
		e.expectStatus(t, http.StatusNotFound, http.MethodGet, "/Items?ParentId="+id, e.child, nil)
	}
	e.expectStatus(t, http.StatusNotFound, http.MethodGet, "/Playlists/"+adultList+"/Items", e.child, nil)
}
