package jellyfin

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
)

// storeLibrary is a server with the library of a cloud store's downloads
// (see storeAddon), a member signed in from the device tv, and the
// library's folder and its title without videos.
type storeLibrary struct {
	testServer
	store          *storeAddon
	token          string
	folder, single string
}

func newStoreLibrary(t *testing.T) storeLibrary {
	t.Helper()
	// The addon's streams are never read here: the file only has to exist.
	clip := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(clip, []byte("media bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := newTestServer(t, 10)
	s.user("member", nil)
	store := newStoreAddon(t, clip)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), store.url, false)
	if err != nil {
		t.Fatal(err)
	}
	catalog := addon.Manifest.Catalogs[0]
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(),
		[]addons.LibraryChoice{{AddonID: addon.ID, CatalogType: catalog.Type, CatalogID: catalog.ID}}); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	if len(views.Items) != 1 {
		t.Fatalf("the views: %+v", views.Items)
	}
	var titles QueryResult
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &titles)
	if len(titles.Items) != 2 || titles.Items[0].Type != "Folder" || titles.Items[1].Type != "Video" {
		t.Fatalf("the library: %+v", titles.Items)
	}
	return storeLibrary{testServer: s, store: store, token: token, folder: titles.Items[0].Id, single: titles.Items[1].Id}
}

// A folder of videos' children narrowed by type count only those kept,
// as Jellyfin's query counts a folder's children: a listing that keeps
// none answers none, out of none, not out of all its videos.
func TestFolderChildrenCountOnlyThoseKept(t *testing.T) {
	t.Parallel()
	l := newStoreLibrary(t)
	for _, tc := range []struct {
		parent, query string
		items, total  int
	}{
		{l.folder, "", storeVideos, storeVideos},
		{l.folder, "&IncludeItemTypes=Video", storeVideos, storeVideos},
		{l.folder, "&IncludeItemTypes=Video&StartIndex=30&Limit=10", storeVideos - 30, storeVideos},
		// jellyfin-web plays a folder so.
		{l.folder, "&Filters=IsNotFolder&Recursive=true&MediaTypes=Audio,Video&SortBy=IsFolder,SortName", storeVideos, storeVideos},
		{l.folder, "&IncludeItemTypes=Episode&Recursive=true", 0, 0},
		{l.folder, "&IncludeItemTypes=Movie,Series", 0, 0},
		{l.folder, "&ExcludeItemTypes=Video", 0, 0},
		{l.folder, "&Filters=IsFolder", 0, 0},
		{l.folder, "&MediaTypes=Audio", 0, 0},
		// A title without videos lists itself, the video it is.
		{l.single, "", 1, 1},
		{l.single, "&IncludeItemTypes=Folder", 0, 0},
	} {
		var children QueryResult
		if status := l.get(t, "/Items?ParentId="+tc.parent+tc.query, l.token, &children); status != http.StatusOK ||
			len(children.Items) != tc.items || children.TotalRecordCount != tc.total {
			t.Errorf("children%s: %d, %d items of %d, want %d of %d", tc.query, status, len(children.Items), children.TotalRecordCount, tc.items, tc.total)
		}
	}
}
