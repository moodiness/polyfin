package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Refresh metadata on a library, as jellyfin-web sends it, queues the
// paced refresh of its titles' descriptions that Content › Libraries
// starts, and answers at once; asked again while it runs, it starts no
// other. Without a mode that asks for metadata, and on a library whose
// descriptions are not refreshed, it answers the same and starts nothing.
// Members may not refresh.
func TestJellyfinRefreshOfALibraryQueuesItsDescriptions(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	asked := make(chan struct{}, 10)
	var metas atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "reels", Name: "Reels", Version: "1", Types: []string{"movie", "collection"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}, {Type: "collection", ID: "sets", Name: "Sets"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{{ID: "tt0001", Type: "movie", Name: "First"}}})
		case strings.HasPrefix(path, "/catalog/collection/sets"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{}})
		case path == "/meta/movie/tt0001.json":
			metas.Add(1)
			// The refresh's request is held until the test releases it.
			asked <- struct{}{}
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": stremio.Meta{ID: "tt0001", Type: "movie", Name: "First", Description: "New."}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	s := newTestServer(t, 10)
	installed, err := s.addons.Install(t.Context(), addons.Shared(), server.URL+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: installed.ID, CatalogType: "movie", CatalogID: "top"},
		{AddonID: installed.ID, CatalogType: "collection", CatalogID: "sets"}}); err != nil {
		t.Fatal(err)
	}
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	s.user("member", nil)
	admin, member := s.signIn("admin", "web"), s.signIn("member", "web")
	views := map[string]string{}
	var page QueryResult
	s.get(t, "/UserViews", admin, &page)
	for _, view := range page.Items {
		views[view.Name] = view.Id
	}
	// An app read the library's first page, which the refresh describes.
	var titles QueryResult
	if status := s.get(t, "/Items?ParentId="+views["Top"], admin, &titles); status != http.StatusOK || len(titles.Items) != 1 {
		t.Fatalf("the library: %d %+v", status, titles.Items)
	}
	top, _ := accounts.ParseID(views["Top"])
	sets, _ := accounts.ParseID(views["Sets"])
	refresh := func(token, id, query string) int {
		t.Helper()
		status, _ := s.call(http.MethodPost, "/Items/"+id+"/Refresh"+query, app("web", token), nil)
		return status
	}
	const asWeb = "?Recursive=true&ImageRefreshMode=Default&MetadataRefreshMode=Default&ReplaceAllImages=false&RegenerateTrickplay=false&ReplaceAllMetadata=false"

	if status := refresh(member, views["Top"], asWeb); status != http.StatusForbidden {
		t.Errorf("a member: %d", status)
	}
	// No mode asking for metadata, as Jellyfin's default: nothing to read.
	if status := refresh(admin, views["Top"], ""); status != http.StatusNoContent {
		t.Errorf("without modes: %d", status)
	}
	if status := refresh(admin, views["Top"], "?metadataRefreshMode=ValidationOnly&imageRefreshMode=None"); status != http.StatusNoContent {
		t.Errorf("validation only: %d", status)
	}
	if _, ok := s.library.LibraryRefreshes()[top]; ok {
		t.Fatal("a refresh without metadata started the library's")
	}
	// A collection library's descriptions are not refreshed.
	if status := refresh(admin, views["Sets"], asWeb); status != http.StatusNoContent {
		t.Errorf("a collection library: %d", status)
	}
	if _, ok := s.library.LibraryRefreshes()[sets]; ok {
		t.Error("a collection library's refresh started")
	}

	if status := refresh(admin, views["Top"], asWeb); status != http.StatusNoContent {
		t.Fatalf("refreshing the library: %d", status)
	}
	select {
	case <-asked:
	case <-time.After(holdLimit):
		t.Fatalf("the refresh asked for no description: %+v", s.library.LibraryRefreshes()[top])
	}
	first := s.library.LibraryRefreshes()[top]
	if first.State != library.RefreshRunning || first.Titles != 1 || first.Started.IsZero() {
		t.Fatalf("the refresh: %+v", first)
	}
	// Asked again while it runs, as with a full refresh, it starts no other.
	if status := refresh(admin, views["Top"], "?metadataRefreshMode=FullRefresh&imageRefreshMode=FullRefresh&replaceAllMetadata=true&replaceAllImages=true"); status != http.StatusNoContent {
		t.Errorf("refreshing again: %d", status)
	}
	if again := s.library.LibraryRefreshes()[top]; again.State != library.RefreshRunning || !again.Started.Equal(first.Started) {
		t.Errorf("asked again while it runs: %+v, was %+v", again, first)
	}
	close(release)
	eventually(t, "the refresh's end", func() bool { return s.library.LibraryRefreshes()[top].State == library.RefreshDone })
	if done := s.library.LibraryRefreshes()[top]; done.Done != 1 || metas.Load() != 1 {
		t.Errorf("the refresh ended: %+v, %d descriptions asked", done, metas.Load())
	}
}
