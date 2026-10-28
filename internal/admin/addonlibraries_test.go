package admin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Adding an addon tells how many of its catalogs became libraries: none
// for an addon whose catalogs are all of a type Polyfin enables none of,
// such as sport, which the admin app then says can be enabled under
// Content › Libraries; one for a movie catalog.
func TestAddingAnAddonTellsItsLibraries(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	manifests := map[string]string{
		"/sports/manifest.json": `{"id": "org.example.sports", "name": "Sports", "version": "1.0.0", "types": ["sport"],
			"resources": ["catalog", "meta", "stream"], "idPrefixes": ["live:", "recap:"],
			"catalogs": [{"type": "sport", "id": "live", "name": "Live Now"}, {"type": "sport", "id": "today", "name": "Today"},
				{"type": "sport", "id": "football", "name": "Football"}]}`,
		"/movies/manifest.json": `{"id": "org.example.movies", "name": "Movies", "version": "1.0.0", "resources": ["catalog"],
			"catalogs": [{"type": "movie", "id": "top", "name": "Top"}, {"type": "sport", "id": "live", "name": "Live Now"}]}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(manifests[r.URL.Path]))
	}))
	defer server.Close()
	administrator := api.signedIn("administrator", true)
	for path, want := range map[string]float64{"/sports/manifest.json": 0, "/movies/manifest.json": 1} {
		status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": server.URL + path})
		if enabled, ok := body["enabledLibraries"].(float64); status != http.StatusCreated || !ok || enabled != want {
			t.Errorf("%s: %d %v, want %v libraries", path, status, body["enabledLibraries"], want)
		}
	}
}

// Adding a music addon tells how many catalog rows of its own it has: none
// leaves My music, which starts empty, as its library. Once the scope has
// the libraries made automatically, an addon whose catalogs would have
// become libraries gets none, and the answer tells the limit; an addon
// none of whose catalogs would have become one is not told it.
func TestAddingAnAddonTellsMyMusicAndTheLibraryLimit(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	music := func(id string, rows bool) string {
		catalogs := `[]`
		if rows {
			catalogs = `[{"id": "top", "type": "track", "name": "Top songs"}, {"id": "new", "type": "album", "name": "New albums"}]`
		}
		return fmt.Sprintf(`{"id": "org.example.%s", "name": "%s", "version": "1.0.0", "resources": ["search", "stream", "catalog"],
			"catalogs": %s}`, id, id, catalogs)
	}
	movies := make([]string, 20)
	for i := range movies {
		movies[i] = fmt.Sprintf(`{"type": "movie", "id": "c%d", "name": "Movies %d"}`, i, i)
	}
	manifests := map[string]string{
		"/searching/manifest.json": music("searching", false),
		"/rows/manifest.json":      music("rows", true),
		"/movies/manifest.json": `{"id": "org.example.movies", "name": "Movies", "version": "1.0.0", "resources": ["catalog"],
			"catalogs": [` + strings.Join(movies, ", ") + `]}`,
		"/late/manifest.json": music("late", false),
		"/sports/manifest.json": `{"id": "org.example.sports", "name": "Sports", "version": "1.0.0", "resources": ["catalog"],
			"catalogs": [{"type": "sport", "id": "live", "name": "Live Now"}]}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(manifests[r.URL.Path]))
	}))
	defer server.Close()
	administrator := api.signedIn("administrator", true)
	for _, want := range []struct {
		path          string
		enabled, rows float64
		limit         float64
	}{
		// My music alone.
		{"/searching/manifest.json", 1, 0, 0},
		{"/rows/manifest.json", 2, 2, 0},
		// 17 of its 20 catalogs reach the 20 libraries.
		{"/movies/manifest.json", 17, -1, 0},
		{"/late/manifest.json", 0, 0, 20},
		{"/sports/manifest.json", 0, -1, 0},
	} {
		status, body, _ := administrator.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": server.URL + want.path})
		rows := float64(-1)
		if m, ok := body["music"].(map[string]any); ok {
			rows, _ = m["rows"].(float64)
		}
		limit, _ := body["libraryLimit"].(float64)
		if status != http.StatusCreated || body["enabledLibraries"] != want.enabled || rows != want.rows || limit != want.limit {
			t.Errorf("%s: %d, %v libraries, %v rows, limit %v; want %v, %v, %v", want.path, status, body["enabledLibraries"], rows, limit,
				want.enabled, want.rows, want.limit)
		}
	}
}
