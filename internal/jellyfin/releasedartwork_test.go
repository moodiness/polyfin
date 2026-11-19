package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// No test server is on a public address. As in
// TestConfinedAddonsImagesStayConfined, a user's own addon is read while
// they are an administrator, whose own addons are not confined; they then
// become a member, and their addon, with what it lists and describes, is
// confined as a member's addon on a public host is.

// libraryAddon serves a Stremio addon named id with one catalog, "mine",
// of catalogType, listing metas; it describes the titles of described, by
// the identifier asked, when there are any.
func libraryAddon(t *testing.T, id, catalogType string, metas []stremio.Meta, described map[string]stremio.Meta) string {
	t.Helper()
	resources := []stremio.Resource{{Name: "catalog"}}
	if len(described) > 0 {
		resources = append(resources, stremio.Resource{Name: "meta"})
	}
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.Path; {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: id, Name: id, Version: "1", Types: []string{catalogType}, Resources: resources,
				Catalogs: []stremio.Catalog{{Type: catalogType, ID: "mine", Name: id + " library"}}})
		case strings.HasPrefix(path, "/catalog/"+catalogType+"/mine"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		case strings.HasPrefix(path, "/meta/"+catalogType+"/"):
			meta, ok := described[strings.TrimSuffix(strings.TrimPrefix(path, "/meta/"+catalogType+"/"), ".json")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(addon.Close)
	return addon.URL + "/manifest.json"
}

// withLibrary installs the addon at manifest in scope, with its catalog
// "mine" of catalogType as a library.
func (s testServer) withLibrary(t *testing.T, scope addons.Scope, manifest, catalogType string) addons.Addon {
	t.Helper()
	installed, err := s.addons.Install(t.Context(), scope, manifest, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), scope, []addons.LibraryChoice{
		{AddonID: installed.ID, CatalogType: catalogType, CatalogID: "mine"}}); err != nil {
		t.Fatal(err)
	}
	return installed
}

// libraryItems lists the items of the library named library, for the
// user of token, by name.
func (s testServer) libraryItems(t *testing.T, token, library string) map[string]string {
	t.Helper()
	var page QueryResult
	if status := s.get(t, "/Items?ParentId="+s.viewIDs(t, token)[library], token, &page); status != http.StatusOK || len(page.Items) == 0 {
		t.Fatalf("the library %s: %d %+v", library, status, page.Items)
	}
	ids := map[string]string{}
	for _, item := range page.Items {
		ids[item.Name] = item.Id
	}
	return ids
}

// becomesMember takes the administrator's role from user.
func (s testServer) becomesMember(t *testing.T, user accounts.User) {
	t.Helper()
	if _, err := s.store.UpdateUser(t.Context(), user.ID, accounts.UserChanges{IsAdministrator: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
}

// A member's own addon gave a title artwork on a local address, which is
// never downloaded. The member removes the addon: the title stays, with
// that artwork, and it is still never downloaded, for a request without
// credentials either.
func TestAReleasedTitlesArtworkStaysConfined(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	// As the server wires it.
	s.addons.SetCleanup(s.library.CleanRemoved)
	local := newCountingArt(t)
	meta := stremio.Meta{ID: "own:1", Type: "movie", Name: "Mine", Poster: local.URL + "/poster.png", Background: local.URL + "/backdrop.png"}
	// Another administrator stays: the last one cannot become a member.
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	owner := s.user("owner", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	installed := s.withLibrary(t, addons.Personal(owner.ID), libraryAddon(t, "own", "movie", []stremio.Meta{meta}, nil), "movie")
	s.libraryItems(t, s.signIn("owner", "tv"), "own library")
	s.becomesMember(t, owner)
	// The member lists their library: the title is their confined addon's.
	id := s.libraryItems(t, s.signIn("owner", "tv"), "own library")["Mine"]
	// Apps ask by the tag the title gave, without credentials.
	if response, _ := s.fetch(t, "/Items/"+id+"/Images/Backdrop?tag="+library.ImageTag(meta.Background)); response.StatusCode != http.StatusForbidden {
		t.Fatalf("the confined addon's backdrop: %d, want 403", response.StatusCode)
	}

	if _, err := s.addons.Remove(t.Context(), addons.Personal(owner.ID), installed.ID); err != nil {
		t.Fatal(err)
	}

	for _, imageType := range []string{"Primary", "Backdrop"} {
		if response, _ := s.fetch(t, "/Items/"+id+"/Images/"+imageType); response.StatusCode != http.StatusForbidden {
			t.Errorf("%s of the released title: %d, want 403", imageType, response.StatusCode)
		}
	}
	if poster, backdrop := local.requests("/poster.png"), local.requests("/backdrop.png"); poster+backdrop != 0 {
		t.Errorf("the local server was asked the poster %d times and the backdrop %d times", poster, backdrop)
	}
}
