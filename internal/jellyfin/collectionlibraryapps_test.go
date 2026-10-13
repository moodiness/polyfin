package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// An app an administrator chose, by the name it signs in with in any
// letter case, is told an addon's collection library holds collections,
// boxsets, wherever the library is described to it, and lists its
// collections, then a collection's titles, as it asks for them. Other apps
// are told it holds mixed content; Polyfin's Collections view is boxsets
// to all. Choosing an app or no longer choosing it takes effect at once.
func TestChosenAppsSeeCollectionLibrariesAsCollections(t *testing.T) {
	t.Parallel()
	s, _, ids := browsing(t)
	if _, err := s.store.CreateUser(t.Context(), accounts.NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true}); err != nil {
		t.Fatal(err)
	}
	admin := s.signIn("admin", "tv")
	var movies QueryResult
	s.get(t, "/Items?ParentId="+ids["Top"], admin, &movies)
	if len(movies.Items) == 0 {
		t.Fatal("no movie")
	}
	s.createCollection(t, admin, "name=Picks&ids="+movies.Items[0].Id)
	// The apps sign in as the administrator: Jellyfin keeps the library
	// settings' descriptions of libraries to administrators.
	signIn := func(client, device string) string {
		t.Helper()
		header := fmt.Sprintf(`MediaBrowser Client="%s", Device="%s", DeviceId="%s-id", Version="1.0.0"`, client, device, device)
		status, body := s.call(http.MethodPost, "/Users/AuthenticateByName", header, map[string]string{"Username": "admin", "Pw": "correct horse"})
		var signedIn AuthenticationResult
		if status != http.StatusOK || json.Unmarshal(body, &signedIn) != nil || signedIn.AccessToken == "" {
			t.Fatalf("sign-in of %s: %d %s", client, status, body)
		}
		return header + `, Token="` + signedIn.AccessToken + `"`
	}
	shelf, web := signIn("Shelf TV", "shelf"), signIn("Jellyfin Web", "browser")
	var me UserDto
	if status, body := s.call(http.MethodGet, "/Users/Me", shelf, nil); status != http.StatusOK || json.Unmarshal(body, &me) != nil {
		t.Fatalf("me: %d %s", status, body)
	}
	get := func(authorization, path string, into any) {
		t.Helper()
		status, body := s.call(http.MethodGet, path, authorization, nil)
		if status != http.StatusOK || json.Unmarshal(body, into) != nil {
			t.Fatalf("%s: %d %s", path, status, body)
		}
	}
	library := ids["Groups"]
	// typesOf collects the content types an app is told the collection
	// library and the Collections view hold, by where it is told them.
	typesOf := func(authorization string) map[string]string {
		t.Helper()
		types := map[string]string{}
		for _, path := range []string{"/UserViews", "/Users/" + me.Id + "/Views", "/Library/MediaFolders", "/Items?ParentId=" + rootFolderID.String()} {
			var views QueryResult
			get(authorization, path, &views)
			for _, view := range views.Items {
				if view.Id == library || view.Name == "Collections" {
					types[path+" "+view.Name] = view.CollectionType
				}
			}
		}
		for _, path := range []string{"/Items/" + library, "/Users/" + me.Id + "/Items/" + library} {
			var item BaseItemDto
			get(authorization, path, &item)
			types[path] = item.CollectionType
		}
		var byID QueryResult
		get(authorization, "/Items?Ids="+library, &byID)
		for _, item := range byID.Items {
			types["/Items?Ids "+item.Name] = item.CollectionType
		}
		var folders []VirtualFolderInfo
		get(authorization, "/Library/VirtualFolders", &folders)
		for _, folder := range folders {
			if folder.ItemId == library {
				types["/Library/VirtualFolders "+folder.Name] = folder.CollectionType
			}
		}
		return types
	}
	expect := func(who, authorization, want string) {
		t.Helper()
		types := typesOf(authorization)
		if len(types) != 11 {
			t.Errorf("%s was told %d descriptions: %v", who, len(types), types)
		}
		for where, got := range types {
			if strings.HasSuffix(where, " Collections") {
				if got != "boxsets" {
					t.Errorf("%s: the Collections view in %s is %q", who, where, got)
				}
			} else if got != want {
				t.Errorf("%s: the collection library in %s is %q, want %q", who, where, got, want)
			}
		}
	}
	expect("Shelf TV before it was chosen", shelf, "")

	s.setting(t, func(settings *accounts.Settings) { settings.CollectionLibraryApps = []string{"sHELF tv"} })
	expect("Shelf TV", shelf, "boxsets")
	expect("jellyfin-web", web, "")

	var collections QueryResult
	get(shelf, "/Items?ParentId="+library+"&IncludeItemTypes=BoxSet&Recursive=true&Limit=20&StartIndex=0&EnableTotalRecordCount=true", &collections)
	if got := itemNames(collections.Items); !slices.Equal(got, []string{"Group"}) || collections.Items[0].Type != "BoxSet" {
		t.Fatalf("the library's collections: %v", collections.Items)
	}
	var titles QueryResult
	get(shelf, "/Items?ParentId="+collections.Items[0].Id+"&IncludeItemTypes=Movie,Series&Recursive=true", &titles)
	if got := itemNames(titles.Items); !slices.Equal(got, []string{"Movie 0", "Movie 1", "Movie 2"}) {
		t.Errorf("the collection's titles: %v", got)
	}

	s.setting(t, func(settings *accounts.Settings) { settings.CollectionLibraryApps = nil })
	expect("Shelf TV no longer chosen", shelf, "")
}
