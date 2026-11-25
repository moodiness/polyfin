package jellyfin

import (
	"net/http"
	"strings"
	"testing"
)

// listedByIdentifier lists ids with /Items?ids=, more added to the query,
// for the allowed user of a, and returns the items by identifier.
func (a parityArea) listedByIdentifier(t *testing.T, ids []string, more string) map[string]BaseItemDto {
	t.Helper()
	var page QueryResult
	user := a.users["allowed"]
	if status := a.get(t, "/Items?userId="+user.id+"&ids="+strings.Join(ids, ",")+more, user.token, &page); status != http.StatusOK {
		t.Fatalf("listing by identifier: %d", status)
	}
	byID := map[string]BaseItemDto{}
	for _, item := range page.Items {
		byID[item.Id] = item
	}
	if page.TotalRecordCount != len(page.Items) {
		t.Errorf("listing by identifier: a total of %d for %d items", page.TotalRecordCount, len(page.Items))
	}
	return byID
}

// The listing by identifier lists what Polyfin makes itself, as its
// details describe it and as Jellyfin lists its own: playlists,
// collections, recordings, the root folder and Polyfin's views. Type
// filters apply to them as to the library's items.
func TestItemsByIdentifierListPolyfinsOwnItems(t *testing.T) {
	t.Parallel()
	for _, area := range []parityArea{videoParity(t), liveParity(t)} {
		var ids []string
		types := map[string]string{}
		for kind, id := range area.items {
			ids = append(ids, id)
			types[id] = kind
		}
		if _, isVideo := area.items["movie"]; isVideo {
			ids = append(ids, rootFolderID.String())
			types[rootFolderID.String()] = "root"
		}
		listed := area.listedByIdentifier(t, ids, "&Fields=ChildCount")
		for _, id := range ids {
			if _, ok := listed[id]; !ok {
				t.Errorf("%s is not listed by its identifier", types[id])
			}
		}
		for id, item := range area.listedByIdentifier(t, ids, "&IncludeItemTypes=Playlist,BoxSet") {
			if item.Type != "Playlist" && item.Type != "BoxSet" {
				t.Errorf("%s listed for playlists and collections: %s", types[id], item.Type)
			}
		}
	}
}

// The Collections, Playlists and recordings views count what they list,
// among the views, in their details and listed by identifier, as
// Jellyfin's views do.
func TestOwnViewsCountWhatTheyList(t *testing.T) {
	t.Parallel()
	for _, area := range []parityArea{videoParity(t), liveParity(t)} {
		user := area.users["allowed"]
		for _, kind := range []string{"view-boxsets", "view-playlists", "view-recordings"} {
			id, ok := area.items[kind]
			if !ok {
				continue
			}
			var children QueryResult
			area.get(t, "/Items?userId="+user.id+"&ParentId="+id, user.token, &children)
			if children.TotalRecordCount == 0 {
				t.Fatalf("%s lists nothing", kind)
			}
			var details BaseItemDto
			area.get(t, "/Users/"+user.id+"/Items/"+id, user.token, &details)
			listed := area.listedByIdentifier(t, []string{id}, "&Fields=ChildCount")[id]
			for where, dto := range map[string]BaseItemDto{"details": details, "listing": listed} {
				if dto.ChildCount == nil || *dto.ChildCount != children.TotalRecordCount {
					t.Errorf("%s %s: ChildCount %v, it lists %d", kind, where, dto.ChildCount, children.TotalRecordCount)
				}
			}
		}
		var views QueryResult
		area.get(t, "/UserViews", user.token, &views)
		for _, view := range views.Items {
			if view.CollectionType == "boxsets" || view.CollectionType == "playlists" {
				if view.ChildCount == nil || *view.ChildCount == 0 {
					t.Errorf("%s among the views: ChildCount %v", view.Name, view.ChildCount)
				}
			}
		}
	}
}

// Libraries are children of the root folder /Items/Root describes, as
// Jellyfin's are, but for the Live TV and Playlists views, UserViews that
// have no parent in Jellyfin either.
func TestLibrariesHaveTheRootFolderAsParent(t *testing.T) {
	t.Parallel()
	for _, area := range []parityArea{videoParity(t), liveParity(t)} {
		user := area.users["allowed"]
		var root BaseItemDto
		area.get(t, "/Items/Root", user.token, &root)
		var views QueryResult
		area.get(t, "/UserViews", user.token, &views)
		recordings := area.items["view-recordings"]
		var folders QueryResult
		if recordings != "" {
			area.get(t, "/LiveTv/Recordings/Folders", user.token, &folders)
		}
		for _, view := range append(views.Items, folders.Items...) {
			var details BaseItemDto
			area.get(t, "/Users/"+user.id+"/Items/"+view.Id, user.token, &details)
			want := root.Id
			if view.Type == "UserView" {
				want = ""
			}
			if view.ParentId != want || details.ParentId != want {
				t.Errorf("%s: parent %q listed, %q in its details, want %q", view.Name, view.ParentId, details.ParentId, want)
			}
		}
	}
}
