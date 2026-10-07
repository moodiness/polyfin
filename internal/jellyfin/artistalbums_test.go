package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
)

// soloistAddon is an Eclipse addon whose artist page lists top tracks,
// which name their albums, and no albums; its album pages list their
// tracks.
func soloistAddon(t *testing.T) string {
	t.Helper()
	track := func(id, title, album, albumID string) map[string]any {
		return map[string]any{"id": id, "title": title, "artist": "Soloist", "album": album, "albumId": albumID, "duration": 200}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		answer := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		switch r.URL.Path {
		case "/manifest.json":
			answer(map[string]any{"id": "com.example.soloist", "name": "Soloist", "version": "1.0.0",
				"resources": []string{"search", "stream", "catalog"}, "types": []string{"track", "album", "artist"}})
		case "/search":
			answer(map[string]any{"artists": []any{map[string]string{"id": "solo", "name": "Soloist"}}})
		case "/artist/solo":
			answer(map[string]any{"id": "solo", "name": "Soloist", "topTracks": []any{
				track("t1", "Opening", "First Album", "a1"), track("t2", "Closing", "Second Album", "a2"), track("t3", "Middle", "First Album", "a1")}})
		case "/album/a1":
			answer(map[string]any{"id": "a1", "title": "First Album", "artist": "Soloist",
				"tracks": []any{track("t1", "Opening", "First Album", "a1"), track("t3", "Middle", "First Album", "a1")}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// An artist whose page lists no albums shows those its top tracks name,
// each once, in Jellyfin apps; each opens through the addon's album page.
func TestAnArtistWithoutAlbumsShowsThoseItsTopTracksName(t *testing.T) {
	s := newTestServer(t, 10)
	s.user("listener", nil)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), soloistAddon(t), false); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("listener", "web")
	var artists QueryResult
	s.get(t, "/Artists?searchTerm=soloist", token, &artists)
	if len(artists.Items) != 1 {
		t.Fatalf("artists: %+v", artists.Items)
	}
	var albums QueryResult
	s.get(t, "/Items?ArtistIds="+artists.Items[0].Id+"&IncludeItemTypes=MusicAlbum&Recursive=true", token, &albums)
	var names []string
	for _, album := range albums.Items {
		names = append(names, album.Name+":"+album.Type)
	}
	if want := []string{"First Album:MusicAlbum", "Second Album:MusicAlbum"}; !slices.Equal(names, want) {
		t.Fatalf("the artist's albums: %v, want %v", names, want)
	}
	var tracks QueryResult
	s.get(t, "/Items?ParentId="+albums.Items[0].Id, token, &tracks)
	if got := itemNames(tracks.Items); strings.Join(got, ",") != "Opening,Middle" {
		t.Errorf("First Album's tracks: %v", got)
	}
}
