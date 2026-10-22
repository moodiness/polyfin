package localfiles

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// Without a server addon that describes and searches the folder's kind of
// titles, its files are unmatched as no addon can search them, not as
// titles no addon knows, and the folder counts them. A user's own
// metadata addon does not count: folders search the server's addons. Once
// a server addon searches, the next scan matches them.
func TestFoldersTellWhenNoAddonSearchesTheirTitles(t *testing.T) {
	t.Parallel()
	e := newEnvWith(t)
	ctx := t.Context()
	user, err := e.users.CreateUser(ctx, accounts.NewUser{Name: "member", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.addons.Install(ctx, addons.Personal(user.ID), newMetaAddon(t), false); err != nil {
		t.Fatal(err)
	}
	addon, _ := e.folder(KindMovies, map[string]int{"Nosferatu (1922).mkv": 10})
	unmatched, _, err := e.service.Unmatched(ctx, addon.ID)
	if err != nil || len(unmatched) != 1 || unmatched[0].Reason != reasonNoSearch {
		t.Fatalf("unmatched: %+v %v", unmatched, err)
	}
	folder, err := e.service.Folder(ctx, addon.ID)
	if err != nil || folder.NoSearch != 1 || folder.SearchFailed != 0 {
		t.Errorf("the folder: no search %d, search failed %d, %v", folder.NoSearch, folder.SearchFailed, err)
	}

	if _, err := e.addons.Install(ctx, addons.Shared(), newMetaAddon(t), false); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(ctx, addon.ID); err != nil {
		t.Fatal(err)
	}
	if got := e.files(addon.ID)["Nosferatu (1922).mkv"]; got != "tt0013442" {
		t.Errorf("once a server addon searches: %s", got)
	}
	if folder, err := e.service.Folder(ctx, addon.ID); err != nil || folder.NoSearch != 0 {
		t.Errorf("the folder once matched: no search %d, %v", folder.NoSearch, err)
	}
}

// describerLog keeps the addons named by the log records written at Debug
// and above.
type describerLog struct {
	mu     *sync.Mutex
	addons *[]string
}

func (h describerLog) Enabled(context.Context, slog.Level) bool { return true }
func (h describerLog) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h describerLog) WithGroup(string) slog.Handler            { return h }
func (h describerLog) Handle(_ context.Context, r slog.Record) error {
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "addon" {
			h.mu.Lock()
			*h.addons = append(*h.addons, a.Value.String())
			h.mu.Unlock()
		}
		return true
	})
	return nil
}

// A title no addon describes is not one of a local folder's either: the
// folder, asked last, does not log that it could not describe it each time
// the title opens, as titles of the catalogs that are not its own are many.
func TestFoldersDoNotLogTitlesThatAreNotTheirs(t *testing.T) {
	t.Parallel()
	pool := migrated.New(t)
	users, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := users.CreateUser(t.Context(), accounts.NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var named []string
	logger := slog.New(describerLog{mu: &mu, addons: &named})
	client := stremio.NewClient("test")
	store := addons.New(pool, client)
	lib := library.New(pool, store, client, logger, users.Settings)
	service := New(pool, store, lib, logger, users.Settings, nil)
	t.Cleanup(service.Close)
	service.UseFiles("http://files.example/", "http://inputs.example/")
	lib.UseLocal(service)

	// A catalog of a title no addon describes.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "ratings", Name: "Ratings", Version: "1", Types: []string{"movie"},
				Resources: []stremio.Resource{{Name: "catalog"}}, Catalogs: []stremio.Catalog{{Type: "movie", ID: "rated", Name: "Rated"}}})
		case strings.HasPrefix(path, "/catalog/movie/rated"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{{ID: "tt0017925", Type: "movie", Name: "The General"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	if _, err := store.Install(t.Context(), addons.Shared(), server.URL+"/manifest.json", false); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Nosferatu (1922).mkv"), 10)
	folder, err := service.Add(t.Context(), NewFolder{Name: "Shelf", Path: dir, Kind: KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	(env{t: t, service: service}).idle(folder.ID)

	libraries, err := lib.Libraries(t.Context(), admin)
	if err != nil {
		t.Fatal(err)
	}
	var rated accounts.ID
	for _, l := range libraries {
		if l.Name == "Rated" {
			rated = l.ID
		}
	}
	page, err := lib.Children(t.Context(), admin, rated, 0, 10, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("the catalog: %v %v", page.Items, err)
	}
	mu.Lock()
	named = nil
	mu.Unlock()
	for range 3 {
		if _, err := lib.Item(t.Context(), admin, page.Items[0].ID); err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if slices.Contains(named, "Shelf") {
		t.Errorf("the folder logged a title that is not its own: %v", named)
	}
}
