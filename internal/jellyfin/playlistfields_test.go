package jellyfin

import (
	"encoding/json"
	"net/http"
	"testing"
)

// A playlist listed with Fields=CanDownload,Chapters,RecursiveItemCount
// carries them, as Jellyfin's does: false, none and its entries; listed
// without them, it carries none of them.
func TestListedPlaylistsCarryTheFieldsAsked(t *testing.T) {
	t.Parallel()
	s, token, views := browsing(t)
	titles := titlesForPlaylists(t, s, token, views)
	id := s.createPlaylist(t, token, map[string]any{"Name": "Mix", "Ids": []string{titles.movies[0], titles.movies[1]}, "MediaType": "Video"})
	var playlists QueryResult
	s.get(t, "/UserViews", token, &playlists)
	listed := func(query string) map[string]json.RawMessage {
		t.Helper()
		for _, view := range playlists.Items {
			if view.CollectionType != "playlists" {
				continue
			}
			status, body := s.call(http.MethodGet, "/Items?ParentId="+view.Id+query, app("tv", token), nil)
			var page struct{ Items []map[string]json.RawMessage }
			if status != http.StatusOK || json.Unmarshal(body, &page) != nil {
				t.Fatalf("playlists: %d %s", status, body)
			}
			for _, item := range page.Items {
				if string(item["Id"]) == `"`+id+`"` {
					return item
				}
			}
		}
		t.Fatalf("the playlist is not listed")
		return nil
	}
	asked := listed("&Fields=CanDownload,Chapters,RecursiveItemCount")
	if string(asked["CanDownload"]) != "false" || string(asked["Chapters"]) != "[]" || string(asked["RecursiveItemCount"]) != "2" {
		t.Errorf("asked: CanDownload %s, Chapters %s, RecursiveItemCount %s", asked["CanDownload"], asked["Chapters"], asked["RecursiveItemCount"])
	}
	for _, field := range []string{"CanDownload", "Chapters", "RecursiveItemCount"} {
		if value, ok := listed("")[field]; ok {
			t.Errorf("%s sent unasked: %s", field, value)
		}
	}
}

// The Playlists view is a UserView, among the views and in its own
// details, as Jellyfin 12.2 describes it.
func TestPlaylistsViewIsAUserView(t *testing.T) {
	t.Parallel()
	s, token, views := browsing(t)
	titles := titlesForPlaylists(t, s, token, views)
	s.createPlaylist(t, token, map[string]any{"Name": "Mix", "Ids": []string{titles.movies[0]}, "MediaType": "Video"})
	var listed QueryResult
	s.get(t, "/UserViews", token, &listed)
	found := false
	for _, view := range listed.Items {
		if view.CollectionType != "playlists" {
			continue
		}
		found = true
		var details BaseItemDto
		s.get(t, "/Items/"+view.Id, token, &details)
		if view.Type != "UserView" || details.Type != "UserView" {
			t.Errorf("the Playlists view: %q among the views, %q in its details", view.Type, details.Type)
		}
	}
	if !found {
		t.Errorf("no Playlists view: %v", itemNames(listed.Items))
	}
}
