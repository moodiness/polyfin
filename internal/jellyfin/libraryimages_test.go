package jellyfin

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A library shows no image unless its scope chooses one: an automatic
// image, the first backdrop its catalog lists, or one uploaded for it,
// which wins. Apps see it as the library's Primary image, in /UserViews
// and VirtualFolders, and download it; a user under parental control is
// not shown the automatic one.
func TestLibrariesShowTheImageChosen(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "films", Name: "Films", Version: "1", Types: []string{"movie"},
				Resources: []stremio.Resource{{Name: "catalog"}}, Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{
				{ID: "tt0001", Type: "movie", Name: "First", Poster: server.URL + "/first.jpg"},
				{ID: "tt0002", Type: "movie", Name: "Second", Poster: server.URL + "/second.jpg", Background: server.URL + "/backdrop.jpg"},
			}})
		case strings.HasSuffix(path, ".jpg"):
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpeg " + strings.TrimSuffix(strings.TrimPrefix(path, "/"), ".jpg")))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	s := newTestServer(t, 10)
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	s.user("kid", func(c *accounts.UserChanges) { c.BlockedGenres = &[]string{"Horror"} })
	addon, err := s.addons.Install(t.Context(), addons.Shared(), server.URL+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "movie", CatalogID: "top"}}); err != nil {
		t.Fatal(err)
	}
	key := addons.LibraryKey{AddonID: addon.ID, CatalogType: "movie", CatalogID: "top"}
	token, kid := s.signIn("admin", "tv"), s.signIn("kid", "tv")

	// view answers the library's Primary image tag in /UserViews and
	// whether VirtualFolders names it as holding its image.
	view := func(token string) (id, tag string) {
		t.Helper()
		var views QueryResult
		s.get(t, "/UserViews", token, &views)
		if len(views.Items) != 1 {
			t.Fatalf("views: %+v", views.Items)
		}
		return views.Items[0].Id, views.Items[0].ImageTags["Primary"]
	}
	folderImage := func() string {
		t.Helper()
		var folders []VirtualFolderInfo
		s.get(t, "/Library/VirtualFolders", token, &folders)
		return folders[0].PrimaryImageItemId
	}
	download := func(id string) (int, string) {
		t.Helper()
		status, body := s.call(http.MethodGet, "/Items/"+id+"/Images/Primary", "", nil)
		return status, string(body)
	}

	id, tag := view(token)
	if tag != "" || folderImage() != "" {
		t.Errorf("a library without an image: tag %q, folder image %q", tag, folderImage())
	}

	if err := s.addons.SetLibraryImage(t.Context(), addons.Shared(), key, addons.LibraryImageAutomatic); err != nil {
		t.Fatal(err)
	}
	// Nothing of the catalog was read yet: the first listing does not wait
	// for it, and shows the image once found.
	want := library.ImageTag(server.URL + "/backdrop.jpg")
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if _, tag = view(token); tag == want || time.Now().After(deadline) {
			break
		}
	}
	if tag != want || folderImage() != id {
		t.Errorf("an automatic image: tag %q, folder image %q", tag, folderImage())
	}
	if status, body := download(id); status != http.StatusOK || body != "jpeg backdrop" {
		t.Errorf("downloading the automatic image: %d %q", status, body)
	}
	if _, tag := view(kid); tag != "" {
		t.Errorf("a user blocking genres sees the automatic image: %q", tag)
	}
	var item BaseItemDto
	s.get(t, "/Items/"+id, token, &item)
	if item.ImageTags["Primary"] != tag {
		t.Errorf("the library as an item: %v", item.ImageTags)
	}

	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 32, 18))); err != nil {
		t.Fatal(err)
	}
	libraryID, _ := accounts.ParseID(id)
	if err := s.library.UploadImage(t.Context(), libraryID, "Primary", picture.Bytes()); err != nil {
		t.Fatal(err)
	}
	uploaded, _ := s.library.UploadedArtwork(libraryID, "Primary")
	if _, tag := view(token); tag != library.ImageTag(uploaded) {
		t.Errorf("an uploaded image: tag %q", tag)
	}
	if _, tag := view(kid); tag != library.ImageTag(uploaded) {
		t.Errorf("a user blocking genres does not see the uploaded image: %q", tag)
	}
	if status, body := download(id); status != http.StatusOK || !strings.HasPrefix(body, "\x89PNG") {
		t.Errorf("downloading the uploaded image: %d %q", status, body[:min(len(body), 16)])
	}

	if err := s.library.DeleteUploadedImage(t.Context(), libraryID, "Primary"); err != nil {
		t.Fatal(err)
	}
	if err := s.addons.SetLibraryImage(t.Context(), addons.Shared(), key, addons.LibraryImageNone); err != nil {
		t.Fatal(err)
	}
	if _, tag := view(token); tag != "" || folderImage() != "" {
		t.Errorf("a library whose image was removed: tag %q", tag)
	}
	if status, _ := download(id); status != http.StatusNotFound {
		t.Errorf("downloading a removed image: %d", status)
	}
}
