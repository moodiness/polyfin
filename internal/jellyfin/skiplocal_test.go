package jellyfin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/stremio"
)

// shelfShows serves a series catalog Top of five shows, tt5001 to tt5005
// named "Show 1" to "Show 5", two to a page.
func shelfShows(t *testing.T) string {
	t.Helper()
	show := func(n int) stremio.Meta {
		return stremio.Meta{ID: fmt.Sprintf("tt500%d", n), Type: "series", Name: fmt.Sprintf("Show %d", n), Year: "2011"}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "test.shelf.shows", Name: "Shelf Shows", Version: "1", Types: []string{"series"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "series", ID: "top", Name: "Top", Extra: []stremio.Extra{{Name: "skip"}}}}})
		case strings.HasPrefix(path, "/catalog/series/top"):
			skip := 0
			_, _ = fmt.Sscanf(strings.TrimPrefix(path, "/catalog/series/top/"), "skip=%d.json", &skip)
			metas := []stremio.Meta{}
			for n := skip + 1; n <= min(skip+2, 5); n++ {
				metas = append(metas, show(n))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		case strings.HasPrefix(path, "/meta/series/tt500"):
			var n int
			_, _ = fmt.Sscanf(path, "/meta/series/tt500%d.json", &n)
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": show(n)})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// skipLocalSetup is a server with the catalog of shelfShows and the
// shows folder Shelf, which holds Show 2 linked to tt5002, both libraries
// of the server; member sees both.
type skipLocalSetup struct {
	testServer
	catalog, shelf addons.Addon
	token          string
	views          map[string]string
}

func skipLocalOn(t *testing.T) skipLocalSetup {
	t.Helper()
	var folders *localfiles.Service
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, slog.New(slog.DiscardHandler), o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
		folders.UseFiles(o.Playback.LocalFiles(folders.Open), o.Playback.LocalInputs(folders))
	})
	p := skipLocalSetup{testServer: s}
	var err error
	if p.catalog, err = s.addons.Install(t.Context(), addons.Shared(), shelfShows(t), false); err != nil {
		t.Fatal(err)
	}
	file, err := os.ReadFile(filepath.Join("..", "container", "testdata", "forced.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	season := filepath.Join(dir, "Show 2", "Season 01")
	if err := os.MkdirAll(season, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(season, "Show 2 S01E01.mkv"), file, 0o644); err != nil {
		t.Fatal(err)
	}
	if p.shelf, err = folders.Add(t.Context(), localfiles.NewFolder{Name: "Shelf", Path: dir, Kind: localfiles.KindShows}); err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), p.shelf.ID); err != nil {
		t.Fatal(err)
	}
	if err := folders.Link(t.Context(), p.shelf.ID, "Show 2", "tt5002"); err != nil {
		t.Fatal(err)
	}
	s.user("member", nil)
	p.token = s.signIn("member", "tv")
	p.views = map[string]string{}
	var views QueryResult
	s.get(t, "/UserViews", p.token, &views)
	for _, view := range views.Items {
		p.views[view.Name] = view.Id
	}
	if p.views["Top"] == "" || p.views["Shelf"] == "" {
		t.Fatalf("views: %v", p.views)
	}
	return p
}

// setTop saves the server's libraries as they are, Top leaving out the
// titles local folders hold when skip is set, with a maximum when max is
// above 0.
func (p skipLocalSetup) setTop(t *testing.T, skip bool, max int) {
	t.Helper()
	libraries, err := p.addons.Libraries(t.Context(), addons.Shared())
	if err != nil {
		t.Fatal(err)
	}
	var choices []addons.LibraryChoice
	for _, l := range libraries {
		if !l.Enabled {
			continue
		}
		choice := addons.LibraryChoice{AddonID: l.AddonID, CatalogType: l.Catalog.Type, CatalogID: l.Catalog.ID}
		if l.AddonID == p.catalog.ID {
			choice.SkipLocal = skip
			if max > 0 {
				choice.MaxItems = &max
			}
		}
		choices = append(choices, choice)
	}
	if _, err := p.addons.SetLibraries(t.Context(), addons.Shared(), choices); err != nil {
		t.Fatal(err)
	}
}

// listed lists a view's titles from start, limit of them (all for 0), as
// token's user sees them, by name, with the identifier of each.
func (p skipLocalSetup) listed(t *testing.T, token, view string, start, limit int) ([]string, map[string]string) {
	t.Helper()
	path := fmt.Sprintf("/Items?ParentId=%s&StartIndex=%d", p.views[view], start)
	if limit > 0 {
		path += fmt.Sprintf("&Limit=%d", limit)
	}
	var page QueryResult
	if status := p.get(t, path, token, &page); status != http.StatusOK {
		t.Fatalf("%s: %d", path, status)
	}
	ids := map[string]string{}
	for _, item := range page.Items {
		ids[item.Name] = item.Id
	}
	return itemNames(page.Items), ids
}

// A library set to leave out the titles local folders hold does not list
// the show a shows folder holds, linked to the identifier the catalog
// lists it under: its pages stay full, the titles after it moving up, and
// its maximum counts the titles it lists. The folder's library still lists
// the show, under the same item. Without the option, the catalog's
// library lists it; and a user who does not see the folder's library
// still finds it in the catalog's.
func TestLibrarySkipsWhatLocalFoldersHold(t *testing.T) {
	t.Parallel()
	p := skipLocalOn(t)
	all := []string{"Show 1", "Show 2", "Show 3", "Show 4", "Show 5"}
	got, before := p.listed(t, p.token, "Top", 0, 0)
	if !slices.Equal(got, all) {
		t.Fatalf("without the option: %v", got)
	}
	_, shelf := p.listed(t, p.token, "Shelf", 0, 0)
	if len(shelf) != 1 || shelf["Show 2"] != before["Show 2"] {
		t.Fatalf("the folder's library: %v, the catalog's show %s", shelf, before["Show 2"])
	}

	p.setTop(t, true, 0)
	if got, _ := p.listed(t, p.token, "Top", 0, 0); !slices.Equal(got, []string{"Show 1", "Show 3", "Show 4", "Show 5"}) {
		t.Errorf("with the option: %v", got)
	}
	for start, want := range map[int][]string{0: {"Show 1", "Show 3"}, 2: {"Show 4", "Show 5"}} {
		if got, _ := p.listed(t, p.token, "Top", start, 2); !slices.Equal(got, want) {
			t.Errorf("with the option, the page from %d: %v, want %v", start, got, want)
		}
	}
	var latest []BaseItemDto
	p.get(t, "/Items/Latest?parentId="+p.views["Top"]+"&limit=10", p.token, &latest)
	if slices.Contains(itemNames(latest), "Show 2") || len(latest) == 0 {
		t.Errorf("the library's home row: %v", itemNames(latest))
	}
	if _, after := p.listed(t, p.token, "Shelf", 0, 0); len(after) != 1 || after["Show 2"] != before["Show 2"] {
		t.Errorf("the folder's library with the option: %v", after)
	}

	// The maximum counts the titles listed, not those left out.
	p.setTop(t, true, 3)
	if got, _ := p.listed(t, p.token, "Top", 0, 0); !slices.Equal(got, []string{"Show 1", "Show 3", "Show 4"}) {
		t.Errorf("with the option and a maximum of 3: %v", got)
	}
	p.setTop(t, true, 0)

	// A user whose visible libraries leave out the folder's still finds
	// the show in the catalog's library.
	shelfLibrary, err := accounts.ParseID(p.views["Shelf"])
	if err != nil {
		t.Fatal(err)
	}
	p.user("outsider", func(c *accounts.UserChanges) { c.HiddenLibraries = &[]accounts.ID{shelfLibrary} })
	outsider := p.signIn("outsider", "tv")
	if got, ids := p.listed(t, outsider, "Top", 0, 0); !slices.Equal(got, all) || ids["Show 2"] != before["Show 2"] {
		t.Errorf("a user who does not see the folder's library: %v", got)
	}

	p.setTop(t, false, 0)
	if got, _ := p.listed(t, p.token, "Top", 0, 0); !slices.Equal(got, all) {
		t.Errorf("the option turned off: %v", got)
	}
}
