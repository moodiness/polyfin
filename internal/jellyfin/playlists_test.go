package jellyfin

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// playlistTitles are the identifiers of the catalog addon's titles a
// playlist test uses.
type playlistTitles struct {
	movies              []string
	series, season, ep1 string
}

func titlesForPlaylists(t *testing.T, s testServer, token string, views map[string]string) playlistTitles {
	t.Helper()
	var movies, seasons, episodes QueryResult
	s.get(t, "/Items?ParentId="+views["Top"], token, &movies)
	var shows QueryResult
	s.get(t, "/Items?ParentId="+views["Shows"], token, &shows)
	titles := playlistTitles{series: shows.Items[0].Id}
	for _, movie := range movies.Items {
		titles.movies = append(titles.movies, movie.Id)
	}
	s.get(t, "/Shows/"+titles.series+"/Seasons", token, &seasons)
	for _, season := range seasons.Items {
		if *season.IndexNumber == 2 {
			titles.season = season.Id
		}
	}
	s.get(t, "/Shows/"+titles.series+"/Episodes", token, &episodes)
	titles.ep1 = episodes.Items[0].Id
	return titles
}

// createPlaylist creates a playlist as token's user and returns its
// identifier.
func (s testServer) createPlaylist(t *testing.T, token string, body any) string {
	t.Helper()
	status, response := s.call(http.MethodPost, "/Playlists", app("tv", token), body)
	var created PlaylistCreationResult
	if status != http.StatusOK || json.Unmarshal(response, &created) != nil || created.Id == "" {
		t.Fatalf("playlist creation: %d %s", status, response)
	}
	return created.Id
}

func (s testServer) playlistEntries(t *testing.T, token, id string) []BaseItemDto {
	t.Helper()
	var page QueryResult
	if status := s.get(t, "/Playlists/"+id+"/Items", token, &page); status != http.StatusOK {
		t.Fatalf("playlist items: %d", status)
	}
	if page.TotalRecordCount != len(page.Items) {
		t.Errorf("playlist items: total %d for %d items", page.TotalRecordCount, len(page.Items))
	}
	return page.Items
}

func (s testServer) expectStatus(t *testing.T, want int, method, path, token string, body any) {
	t.Helper()
	if status, response := s.call(method, path, app("tv", token), body); status != want {
		t.Errorf("%s %s: %d %s, want %d", method, path, status, response, want)
	}
}

func TestPlaylistsExpandSeriesAndKeepTheirOrder(t *testing.T) {
	s, token, views := browsing(t)
	titles := titlesForPlaylists(t, s, token, views)

	// A series adds its episodes in order after the movie.
	id := s.createPlaylist(t, token, map[string]any{"Name": "Mix", "Ids": []string{titles.movies[1], titles.series}, "MediaType": "Video"})
	entries := s.playlistEntries(t, token, id)
	if got := itemNames(entries); !slices.Equal(got, []string{"Movie 1", "Pilot", "Second", "Return"}) {
		t.Fatalf("created playlist: %v", got)
	}
	entryIDs := map[string]bool{}
	for _, entry := range entries {
		entryIDs[entry.PlaylistItemId] = true
	}
	if len(entryIDs) != 4 || entryIDs[""] {
		t.Errorf("entries do not have identifiers of their own: %v", entryIDs)
	}

	// Older apps create with query parameters; a season adds its own
	// episodes only.
	member, _ := s.store.Authenticate(t.Context(), "member", "correct horse")
	status, response := s.call(http.MethodPost, "/Playlists?name=Season&ids="+titles.season+"&userId="+member.ID.String(), app("tv", token), nil)
	var created PlaylistCreationResult
	if status != http.StatusOK || json.Unmarshal(response, &created) != nil {
		t.Fatalf("creation from query parameters: %d %s", status, response)
	}
	if got := itemNames(s.playlistEntries(t, token, created.Id)); !slices.Equal(got, []string{"Return"}) {
		t.Errorf("season playlist: %v", got)
	}

	// The same movie twice makes two entries; one goes with its entry.
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+id+"/Items?ids="+titles.movies[0]+","+titles.movies[1]+"&position=1", token, nil)
	entries = s.playlistEntries(t, token, id)
	if got := itemNames(entries); !slices.Equal(got, []string{"Movie 1", "Movie 0", "Movie 1", "Pilot", "Second", "Return"}) {
		t.Fatalf("after adding at a position: %v", got)
	}
	s.expectStatus(t, http.StatusNoContent, http.MethodDelete, "/Playlists/"+id+"/Items?entryIds="+entries[0].PlaylistItemId+","+entries[4].PlaylistItemId, token, nil)
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+id+"/Items/"+entries[5].PlaylistItemId+"/Move/0", token, nil)
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+id+"/Items/"+entries[1].PlaylistItemId+"/Move/9", token, nil)
	after := s.playlistEntries(t, token, id)
	if got := itemNames(after); !slices.Equal(got, []string{"Return", "Movie 1", "Pilot", "Movie 0"}) {
		t.Fatalf("after removing and moving: %v", got)
	}
	if after[1].PlaylistItemId != entries[2].PlaylistItemId || after[0].PlaylistItemId != entries[5].PlaylistItemId {
		t.Errorf("entries changed identifiers: %v then %v", entries, after)
	}

	var page QueryResult
	s.get(t, "/Playlists/"+id+"/Items?startIndex=1&limit=2", token, &page)
	if got := itemNames(page.Items); !slices.Equal(got, []string{"Movie 1", "Pilot"}) || page.TotalRecordCount != 4 || page.StartIndex != 1 {
		t.Errorf("a page of the playlist: %v total %d", got, page.TotalRecordCount)
	}

	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+id, token, map[string]any{"Name": "Renamed", "Ids": nil})
	var playlist BaseItemDto
	s.get(t, "/Users/"+member.ID.String()+"/Items/"+id, token, &playlist)
	if playlist.Name != "Renamed" || playlist.Type != "Playlist" || playlist.MediaType != "Video" || playlist.ChildCount == nil || *playlist.ChildCount != 4 ||
		playlist.RunTimeTicks == nil || *playlist.RunTimeTicks != (46+45+90+90)*60*10_000_000 || playlist.CanDelete == nil || !*playlist.CanDelete {
		t.Errorf("renamed playlist: %+v", playlist)
	}
	var dto PlaylistDto
	s.get(t, "/Playlists/"+id, token, &dto)
	want := []string{after[0].Id, after[1].Id, after[2].Id, after[3].Id}
	if !slices.Equal(dto.ItemIds, want) || dto.OpenAccess || len(dto.Shares) != 0 {
		t.Errorf("playlist: %+v, want items %v", dto, want)
	}

	// Replacing the titles expands them too.
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+id, token, map[string]any{"Ids": []string{titles.season, titles.ep1}})
	if got := itemNames(s.playlistEntries(t, token, id)); !slices.Equal(got, []string{"Return", "Pilot"}) {
		t.Errorf("replaced titles: %v", got)
	}

	s.expectStatus(t, http.StatusBadRequest, http.MethodPost, "/Playlists", token, map[string]any{"Ids": []string{titles.ep1}})
	s.expectStatus(t, http.StatusBadRequest, http.MethodPost, "/Playlists/"+id, token, nil)
}

func TestPlaylistsAreBrowsedInTheirView(t *testing.T) {
	s, token, views := browsing(t)
	titles := titlesForPlaylists(t, s, token, views)
	if _, ok := views["Playlists"]; ok {
		t.Fatal("the Playlists view shows before any playlist exists")
	}
	second := s.createPlaylist(t, token, map[string]any{"Name": "b list", "Ids": []string{titles.ep1}})
	first := s.createPlaylist(t, token, map[string]any{"Name": "A list", "Ids": []string{titles.movies[0], titles.movies[2]}})

	var result QueryResult
	s.get(t, "/UserViews", token, &result)
	var view BaseItemDto
	for _, item := range result.Items {
		if item.CollectionType == "playlists" {
			view = item
		}
	}
	if view.Id == "" || view.Name != "Playlists" || !view.IsFolder {
		t.Fatalf("no Playlists view: %v", itemNames(result.Items))
	}
	var detail BaseItemDto
	if status := s.get(t, "/Items/"+view.Id, token, &detail); status != http.StatusOK || detail.CollectionType != "playlists" {
		t.Errorf("Playlists view detail: %d %+v", status, detail)
	}

	for _, path := range []string{"/Items?parentId=" + view.Id, "/Items?includeItemTypes=Playlist&recursive=true"} {
		var page QueryResult
		s.get(t, path, token, &page)
		if got := itemNames(page.Items); !slices.Equal(got, []string{"A list", "b list"}) || page.TotalRecordCount != 2 {
			t.Errorf("%s: %v", path, got)
			continue
		}
		if page.Items[0].Id != first || page.Items[1].Id != second || page.Items[0].Type != "Playlist" || *page.Items[0].ChildCount != 2 {
			t.Errorf("%s: %+v", path, page.Items[0])
		}
	}
	var none QueryResult
	s.get(t, "/Items?parentId="+view.Id+"&includeItemTypes=Movie", token, &none)
	if len(none.Items) != 0 {
		t.Errorf("movies among playlists: %v", itemNames(none.Items))
	}

	// Apps that list a folder's children open a playlist by its identifier.
	var children QueryResult
	s.get(t, "/Items?parentId="+first, token, &children)
	if got := itemNames(children.Items); !slices.Equal(got, []string{"Movie 0", "Movie 2"}) || children.Items[0].PlaylistItemId == "" {
		t.Errorf("playlist children: %v", got)
	}

	// Played titles count toward the playlist's progress.
	s.expectStatus(t, http.StatusOK, http.MethodPost, "/UserPlayedItems/"+titles.movies[0], token, nil)
	var playlist BaseItemDto
	s.get(t, "/Items/"+first, token, &playlist)
	if playlist.UserData.UnplayedItemCount == nil || *playlist.UserData.UnplayedItemCount != 1 || *playlist.UserData.PlayedPercentage != 50 || playlist.UserData.Played {
		t.Errorf("playlist progress: %+v", playlist.UserData)
	}
}

func TestPlaylistSharing(t *testing.T) {
	s, token, views := browsing(t)
	titles := titlesForPlaylists(t, s, token, views)
	guest := s.user("guest", nil)
	guestToken := s.signIn("guest", "phone")
	s.user("third", nil)
	thirdToken := s.signIn("third", "laptop")
	id := s.createPlaylist(t, token, map[string]any{"Name": "Mix", "Ids": []string{titles.movies[0]}})
	users := "/Playlists/" + id + "/Users/"

	// To a user it is not shared with, the playlist does not exist.
	for _, path := range []string{"/Playlists/" + id, "/Playlists/" + id + "/Items", "/Playlists/" + id + "/Users"} {
		s.expectStatus(t, http.StatusNotFound, http.MethodGet, path, guestToken, nil)
	}
	s.expectStatus(t, http.StatusNotFound, http.MethodGet, "/Items/"+id, guestToken, nil)
	s.expectStatus(t, http.StatusNotFound, http.MethodPost, "/Playlists/"+id+"/Items?ids="+titles.movies[1], guestToken, nil)
	s.expectStatus(t, http.StatusNotFound, http.MethodDelete, "/Items/"+id, guestToken, nil)
	var guestViews QueryResult
	s.get(t, "/UserViews", guestToken, &guestViews)
	if slices.Contains(itemNames(guestViews.Items), "Playlists") {
		t.Errorf("the Playlists view shows to a user who sees no playlist")
	}

	// Shared read-only, the guest sees it but cannot change it.
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, users+guest.ID.String(), token, map[string]any{"CanEdit": false})
	if got := itemNames(s.playlistEntries(t, guestToken, id)); !slices.Equal(got, []string{"Movie 0"}) {
		t.Errorf("shared playlist for the guest: %v", got)
	}
	s.get(t, "/UserViews", guestToken, &guestViews)
	if !slices.Contains(itemNames(guestViews.Items), "Playlists") {
		t.Errorf("the Playlists view does not show to a user a playlist is shared with")
	}
	s.expectStatus(t, http.StatusForbidden, http.MethodPost, "/Playlists/"+id+"/Items?ids="+titles.movies[1], guestToken, nil)
	s.expectStatus(t, http.StatusForbidden, http.MethodPost, "/Playlists/"+id, guestToken, map[string]any{"Name": "Mine"})
	s.expectStatus(t, http.StatusForbidden, http.MethodGet, "/Playlists/"+id+"/Users", guestToken, nil)
	s.expectStatus(t, http.StatusForbidden, http.MethodPost, users+guest.ID.String(), guestToken, map[string]any{"CanEdit": true})
	s.expectStatus(t, http.StatusUnauthorized, http.MethodDelete, "/Items/"+id, guestToken, nil)
	var own PlaylistUserPermissions
	s.get(t, users+guest.ID.String(), guestToken, &own)
	if own.UserId != guest.ID.String() || own.CanEdit {
		t.Errorf("the guest's permissions: %+v", own)
	}
	var listed []PlaylistUserPermissions
	s.get(t, "/Playlists/"+id+"/Users", token, &listed)
	if !slices.Equal(listed, []PlaylistUserPermissions{{UserId: guest.ID.String()}}) {
		t.Errorf("playlist users: %+v", listed)
	}

	// Allowed to edit, the guest changes it.
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, users+guest.ID.String(), token, map[string]any{"CanEdit": true})
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+id+"/Items?ids="+titles.movies[1], guestToken, nil)
	if got := itemNames(s.playlistEntries(t, token, id)); !slices.Equal(got, []string{"Movie 0", "Movie 1"}) {
		t.Errorf("after the guest's addition: %v", got)
	}

	// Open access shows it to everyone, without letting them edit.
	s.expectStatus(t, http.StatusNotFound, http.MethodGet, "/Playlists/"+id+"/Items", thirdToken, nil)
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+id, token, map[string]any{"IsPublic": true})
	if got := itemNames(s.playlistEntries(t, thirdToken, id)); len(got) != 2 {
		t.Errorf("open playlist for another user: %v", got)
	}
	s.expectStatus(t, http.StatusForbidden, http.MethodDelete, "/Playlists/"+id+"/Items?entryIds="+id, thirdToken, nil)

	// No longer shared and closed again, the playlist is gone for the guest.
	s.expectStatus(t, http.StatusNoContent, http.MethodDelete, users+guest.ID.String(), token, nil)
	s.expectStatus(t, http.StatusNotFound, http.MethodDelete, users+guest.ID.String(), token, nil)
	s.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+id, token, map[string]any{"IsPublic": false})
	s.expectStatus(t, http.StatusNotFound, http.MethodGet, "/Playlists/"+id+"/Items", guestToken, nil)
}

func TestOnlyPlaylistsCanBeDeleted(t *testing.T) {
	s, token, views := browsing(t)
	titles := titlesForPlaylists(t, s, token, views)
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	adminToken := s.signIn("admin", "laptop")
	id := s.createPlaylist(t, token, map[string]any{"Name": "Mix", "Ids": []string{titles.movies[0]}, "IsPublic": true})

	s.expectStatus(t, http.StatusUnauthorized, http.MethodDelete, "/Items/"+titles.movies[0], token, nil)
	s.expectStatus(t, http.StatusNotFound, http.MethodDelete, "/Items/"+"00000000000000000000000000000001", token, nil)
	s.expectStatus(t, http.StatusNoContent, http.MethodDelete, "/Items/"+id, token, nil)
	s.expectStatus(t, http.StatusNotFound, http.MethodGet, "/Playlists/"+id, token, nil)
	s.expectStatus(t, http.StatusNotFound, http.MethodGet, "/Items/"+id, token, nil)

	// Like Jellyfin, an administrator deletes any playlist they see.
	other := s.createPlaylist(t, token, map[string]any{"Name": "Open", "IsPublic": true})
	s.expectStatus(t, http.StatusNoContent, http.MethodDelete, "/Items/"+other, adminToken, nil)
}

// playlistShapes lists what Polyfin cannot match in the playlist fixtures.
var playlistShapes = shapeRules{
	dynamic: []string{"ImageTags", "ImageBlurHashes", "ProviderIds"},
	absent: map[string][]string{
		// Polyfin has no files; the test addon has no streams, so the
		// format of the titles is unknown.
		"Path": {"*"}, "Container": {"*"},
		// Metadata addons do not provide it.
		"OriginalLanguage": {"*"},
		// Jellyfin rates a playlist and draws its artwork from its titles;
		// Polyfin does neither.
		"OfficialRating": {"playlists", "playlist-item"}, "PrimaryImageAspectRatio": {"playlist-item"},
		// As in the views fixture: the size of a remote catalog is unknown,
		// and Polyfin's libraries have no root folder.
		"ChildCount": {"views-with-playlists"}, "ParentId": {"views-with-playlists"},
	},
	returned: map[string][]string{
		// Polyfin gives the aspect ratio of artwork in every listing,
		// without waiting for the field to be requested.
		"PrimaryImageAspectRatio": {"playlist-items"},
	},
}

// TestPlaylistResponsesMatchJellyfin compares playlist responses and
// statuses with those recorded from Jellyfin 12.1 by
// scripts/jellyfin-fixtures.sh, with the same requests.
func TestPlaylistResponsesMatchJellyfin(t *testing.T) {
	s, token, views := browsing(t)
	titles := titlesForPlaylists(t, s, token, views)
	member, _ := s.store.Authenticate(t.Context(), "member", "correct horse")
	viewer := s.user("viewer", nil)
	user := member.ID.String()
	raw, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.1", "next-statuses.json"))
	if err != nil {
		t.Fatal(err)
	}
	var statuses map[string]int
	if err := json.Unmarshal(raw, &statuses); err != nil {
		t.Fatal(err)
	}
	bodies := map[string][]byte{}
	request := func(name, method, path string, body any) []byte {
		t.Helper()
		status, response := s.call(method, path, app("tv", token), body)
		if status != statuses[name] {
			t.Errorf("%s: %d %s, Jellyfin answered %d", name, status, response, statuses[name])
		}
		bodies[name] = response
		return response
	}

	var created PlaylistCreationResult
	_ = json.Unmarshal(request("PlaylistCreate", http.MethodPost, "/Playlists",
		map[string]any{"Name": "Fixture playlist", "Ids": []string{titles.movies[0], titles.ep1}, "UserId": user, "MediaType": "Video"}), &created)
	playlist := created.Id
	request("PlaylistGet", http.MethodGet, "/Playlists/"+playlist, nil)
	var items QueryResult
	_ = json.Unmarshal(request("PlaylistItems", http.MethodGet, "/Playlists/"+playlist+"/Items?userId="+user, nil), &items)
	entry := items.Items[0].PlaylistItemId
	request("PlaylistItem", http.MethodGet, "/Users/"+user+"/Items/"+playlist, nil)
	request("ViewsWithPlaylists", http.MethodGet, "/UserViews?userId="+user, nil)
	request("Playlists", http.MethodGet, "/Items?userId="+user+"&includeItemTypes=Playlist&recursive=true", nil)
	request("PlaylistAddItem", http.MethodPost, "/Playlists/"+playlist+"/Items?ids="+titles.movies[1]+"&userId="+user, nil)
	request("PlaylistMoveItem", http.MethodPost, "/Playlists/"+playlist+"/Items/"+entry+"/Move/1", nil)
	request("PlaylistRemoveItem", http.MethodDelete, "/Playlists/"+playlist+"/Items?entryIds="+entry, nil)
	request("PlaylistRename", http.MethodPost, "/Playlists/"+playlist, map[string]any{"Name": "Renamed"})
	request("PlaylistShare", http.MethodPost, "/Playlists/"+playlist+"/Users/"+viewer.ID.String(), map[string]any{"CanEdit": true})
	request("PlaylistUsers", http.MethodGet, "/Playlists/"+playlist+"/Users", nil)
	request("PlaylistUser", http.MethodGet, "/Playlists/"+playlist+"/Users/"+user, nil)

	for fixture, name := range map[string]string{
		"playlist-created":     "PlaylistCreate",
		"playlist":             "PlaylistGet",
		"playlist-items":       "PlaylistItems",
		"playlist-item":        "PlaylistItem",
		"views-with-playlists": "ViewsWithPlaylists",
		"playlists":            "Playlists",
		"playlist-users":       "PlaylistUsers",
		"playlist-user":        "PlaylistUser",
	} {
		raw, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.1", fixture+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var want, got any
		if err := json.Unmarshal(raw, &want); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(bodies[name], &got); err != nil {
			t.Fatalf("%s: %v in %s", fixture, err, bodies[name])
		}
		for _, difference := range compareShapes(fixture, want, got, playlistShapes) {
			t.Error(difference)
		}
		if fixture != "views-with-playlists" {
			continue
		}
		// The fixture keeps the first view only, a library; the Playlists
		// view has the same shape.
		var view any
		for _, item := range got.(map[string]any)["Items"].([]any) {
			if item.(map[string]any)["CollectionType"] == "playlists" {
				view = item
			}
		}
		if view == nil {
			t.Fatalf("no Playlists view in %s", bodies[name])
		}
		for _, difference := range compareShapes(fixture+".Items[playlists]", want.(map[string]any)["Items"].([]any)[0], view, playlistShapes) {
			t.Error(difference)
		}
	}
}
