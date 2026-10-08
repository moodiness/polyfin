package jellyfin

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// A library of an addon's collections is told to apps as a library of
// mixed content, without content type: Jellyfin apps leave libraries of
// collections out of their home rows of latest items, and give one to the
// others. Its row lists its collections; and listed as jellyfin-web lists
// a library without type, by its folders, movies and series, it lists its
// collections too.
func TestCollectionLibrariesGetHomeRows(t *testing.T) {
	s, token, ids := browsing(t)
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	types := map[string]string{}
	for _, view := range views.Items {
		types[view.Name] = view.CollectionType
	}
	if len(types) != 3 || types["Groups"] != "" || types["Top"] != "movies" || types["Shows"] != "tvshows" {
		t.Errorf("content types: %v", types)
	}
	var latest []BaseItemDto
	if status := s.get(t, "/Items/Latest?ParentId="+ids["Groups"], token, &latest); status != http.StatusOK || len(latest) != 1 || latest[0].Type != "BoxSet" {
		t.Errorf("the collection library's row: %d %+v", status, latest)
	}
	for _, query := range []string{
		"IncludeItemTypes=Folder,Movie,Series&Recursive=false",
		"IncludeItemTypes=BoxSet&Recursive=true",
		"ExcludeItemTypes=Folder",
	} {
		var listed QueryResult
		if status := s.get(t, "/Items?ParentId="+ids["Groups"]+"&"+query, token, &listed); status != http.StatusOK || len(listed.Items) != 1 || listed.Items[0].Type != "BoxSet" {
			t.Errorf("the collection library listed with %s: %d %+v", query, status, listed.Items)
		}
	}
}

// Jellyfin lists every BoxSet in a listing of collections across the
// server, as Strand asks for its shelves: Polyfin lists there the
// collections of the user's collection libraries, with those made by users,
// by name, and each opens on its titles. jellyfin-web asks the same for its
// Add to collection dialog, where only collections made by users take a
// title: it gets those alone. A library hidden from the user keeps its
// collections out.
func TestCollectionsAcrossTheServerHoldTheAddonsCollections(t *testing.T) {
	s, token, ids := browsing(t)
	if _, err := s.store.CreateUser(t.Context(), accounts.NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true}); err != nil {
		t.Fatal(err)
	}
	admin := s.signIn("admin", "tv")
	var movies, groups QueryResult
	s.get(t, "/Items?ParentId="+ids["Top"], admin, &movies)
	s.get(t, "/Items?ParentId="+ids["Groups"], token, &groups)
	if len(movies.Items) == 0 || len(groups.Items) != 1 {
		t.Fatalf("titles %v, collections %v", itemNames(movies.Items), itemNames(groups.Items))
	}
	s.createCollection(t, admin, "name=Picks&ids="+movies.Items[0].Id)
	var me UserDto
	s.get(t, "/Users/Me", token, &me)
	listed := func(authorization, query string) []BaseItemDto {
		t.Helper()
		status, body := s.call(http.MethodGet, "/Users/"+me.Id+"/Items?IncludeItemTypes=BoxSet&Recursive=true"+query, authorization, nil)
		var page QueryResult
		if status != http.StatusOK || json.Unmarshal(body, &page) != nil {
			t.Fatalf("collections across the server with %s: %d %s", query, status, body)
		}
		return page.Items
	}
	strand := listed(app("phone", token), "&EnableTotalRecordCount=false&SortBy=SortName&SortOrder=Ascending")
	if got := itemNames(strand); !slices.Equal(got, []string{"Group", "Picks"}) {
		t.Fatalf("Strand's listing: %v", got)
	}
	if strand[0].Id != groups.Items[0].Id || strand[0].Type != "BoxSet" {
		t.Errorf("the addon's collection: %+v, its library lists %+v", strand[0], groups.Items[0])
	}
	var titles QueryResult
	s.get(t, "/Users/"+me.Id+"/Items?ParentId="+strand[0].Id+"&Recursive=true&IncludeItemTypes=Movie,Series,Video,MusicVideo&SortBy=SortName&Limit=50", token, &titles)
	if len(titles.Items) == 0 {
		t.Error("the addon's collection listed across the server opens on no title")
	}
	if got := itemNames(listed(app("phone", token), "&SortBy=SortName&SortOrder=Descending")); !slices.Equal(got, []string{"Picks", "Group"}) {
		t.Errorf("by name, descending: %v", got)
	}
	if got := itemNames(listed(app("phone", token), "&SortBy=SortName&StartIndex=1&Limit=1")); !slices.Equal(got, []string{"Picks"}) {
		t.Errorf("second page of one: %v", got)
	}
	// jellyfin-web is the app the user signed in with.
	webApp := `MediaBrowser Client="Jellyfin Web", Device="Firefox", DeviceId="web-id", Version="10.11.0"`
	status, body := s.call(http.MethodPost, "/Users/AuthenticateByName", webApp, map[string]string{"Username": "member", "Pw": "correct horse"})
	var signedIn struct{ AccessToken string }
	if status != http.StatusOK || json.Unmarshal(body, &signedIn) != nil || signedIn.AccessToken == "" {
		t.Fatalf("jellyfin-web's sign-in: %d %s", status, body)
	}
	web := webApp + `, Token="` + signedIn.AccessToken + `"`
	if got := itemNames(listed(web, "&SortBy=SortName&EnableTotalRecordCount=false")); !slices.Equal(got, []string{"Picks"}) {
		t.Errorf("jellyfin-web's Add to collection dialog: %v", got)
	}

	hidden, _ := accounts.ParseID(ids["Groups"])
	member, _ := accounts.ParseID(me.Id)
	if _, err := s.store.UpdateUser(t.Context(), member, accounts.UserChanges{HiddenLibraries: &[]accounts.ID{hidden}}, nil); err != nil {
		t.Fatal(err)
	}
	if got := itemNames(listed(app("phone", token), "&SortBy=SortName")); !slices.Equal(got, []string{"Picks"}) {
		t.Errorf("with the collection library hidden: %v", got)
	}
}
