package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// browseFailure lists the library named name of the user token signs in,
// which must fail, and returns the attributes "Browsing failed" was logged
// with.
func (s testServer) browseFailure(t *testing.T, token, name, query string) map[string]string {
	t.Helper()
	views := s.viewIDs(t, token)
	id, ok := views[name]
	if !ok {
		t.Fatalf("no library %s among %v", name, views)
	}
	if status := s.get(t, "/Items?ParentId="+id+query, token, nil); status != http.StatusBadGateway {
		t.Fatalf("a library whose addon refuses it: %d", status)
	}
	logged := s.warnings.attrsOf("Browsing failed")
	if len(logged) == 0 {
		t.Fatalf("nothing logged: %v", s.warnings.logged())
	}
	return logged[len(logged)-1]
}

// A music library whose addon refuses its requests fails, and the log
// names the addon and the catalog, and tells the refusal with its status
// rather than calling the addon unreachable.
func TestABrowseOfARefusingMusicAddonNamesIt(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	addon := newFakeEclipse(t, "")
	if _, err := s.addons.Install(t.Context(), addons.Shared(), addon.url, false); err != nil {
		t.Fatal(err)
	}
	addon.refusing.Store(true)
	s.user("listener", nil)
	token := s.signIn("listener", "web")
	attrs := s.browseFailure(t, token, "New Releases", "&IncludeItemTypes=MusicAlbum&Recursive=true")
	if attrs["addon"] != "Tones" || attrs["catalog"] != "new" || !strings.Contains(attrs["error"], "refused the request: HTTP 403") ||
		strings.Contains(attrs["error"], "unreachable") {
		t.Errorf("logged %v", attrs)
	}
}

// A movie library whose addon refuses its catalog fails the same way, its
// addon and catalog named.
func TestABrowseOfARefusingCatalogNamesIt(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/manifest.json" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "refusing", Name: "Refusing", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}}, Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
	}))
	t.Cleanup(server.Close)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), server.URL+"/manifest.json", false); err != nil {
		t.Fatal(err)
	}
	s.user("viewer", nil)
	token := s.signIn("viewer", "web")
	attrs := s.browseFailure(t, token, "Top", "")
	if attrs["addon"] != "Refusing" || attrs["catalog"] != "top" || !strings.Contains(attrs["error"], "refused the request: HTTP 403") {
		t.Errorf("logged %v", attrs)
	}
}
