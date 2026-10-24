package admin

import (
	"net/http"
	"net/http/httptest"
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
