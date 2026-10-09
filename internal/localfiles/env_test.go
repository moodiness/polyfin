package localfiles

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/testdb"
)

type env struct {
	t       *testing.T
	pool    *pgxpool.Pool
	service *Service
	addons  *addons.Store
	library *library.Service
	titles  *countedTitles
}

// countedTitles counts the searches the service asks the library for.
type countedTitles struct {
	*library.Service
	searches atomic.Int32
}

func (c *countedTitles) RemoteSearch(ctx context.Context, kind library.Kind, name string, limit int) ([]library.RemoteResult, error) {
	c.searches.Add(1)
	return c.Service.RemoteSearch(ctx, kind, name, limit)
}

// metaTitles are the titles the fake metadata addon knows, public-domain
// ones.
var metaTitles = []stremio.Meta{
	{ID: "tt0063350", Type: "movie", Name: "Night of the Living Dead", Year: "1968", Poster: "https://images.example/notld.jpg"},
	{ID: "tt0100258", Type: "movie", Name: "Night of the Living Dead", Year: "1990"},
	{ID: "tt0013442", Type: "movie", Name: "Nosferatu", Year: "1922"},
	{ID: "tt0017925", Type: "movie", Name: "The General", Year: "1926"},
	{ID: "tt0015864", Type: "movie", Name: "The Gold Rush", Year: "1925", TmdbID: "962"},
	{ID: "tt0041038", Type: "series", Name: "The Lone Ranger", Year: "1949–1957"},
}

// metaAddon is a metadata addon: searchable movie and series catalogs and
// the descriptions of metaTitles.
func newMetaAddon(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			search := []stremio.Extra{{Name: "search"}}
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "meta", Name: "Meta", Version: "1", Types: []string{"movie", "series"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top", Extra: search}, {Type: "series", ID: "top", Name: "Top", Extra: search}}})
		case strings.HasPrefix(path, "/catalog/"):
			rest := strings.TrimSuffix(strings.TrimPrefix(path, "/catalog/"), ".json")
			typ, query, _ := strings.Cut(rest, "/top/search=")
			term, _ := url.PathUnescape(query)
			metas := []stremio.Meta{}
			for _, meta := range metaTitles {
				if meta.Type == typ && term != "" && strings.Contains(strings.ToLower(meta.Name), strings.ToLower(term)) {
					metas = append(metas, meta)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		case strings.HasPrefix(path, "/meta/"):
			id := strings.TrimSuffix(path[strings.LastIndex(path, "/")+1:], ".json")
			for _, meta := range metaTitles {
				if meta.ID == id {
					_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
					return
				}
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

func newEnv(t *testing.T) env {
	t.Helper()
	pool := testdb.New(t)
	if err := database.Migrate(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	users, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := stremio.NewClient("test")
	store := addons.New(pool, client)
	lib := library.New(pool, store, client, logger, users.Settings)
	if _, err := store.Install(t.Context(), addons.Shared(), newMetaAddon(t), false); err != nil {
		t.Fatal(err)
	}
	titles := &countedTitles{Service: lib}
	service := New(pool, store, titles, logger, users.Settings)
	t.Cleanup(service.Close)
	service.UseFiles("http://files.example/")
	lib.UseLocal(service)
	return env{t: t, pool: pool, service: service, addons: store, library: lib, titles: titles}
}

// folder adds a folder of kind at a new temporary path holding files, each
// as many bytes as its value, and waits for its first scan.
func (e env) folder(kind string, files map[string]int) (addons.Addon, string) {
	e.t.Helper()
	dir := e.t.TempDir()
	for rel, size := range files {
		write(e.t, filepath.Join(dir, rel), size)
	}
	addon, err := e.service.Add(e.t.Context(), NewFolder{Name: "Shelf", Path: dir, Kind: kind})
	if err != nil {
		e.t.Fatal(err)
	}
	e.idle(addon.ID)
	return addon, dir
}

// idle waits for the scan of a folder under way to end.
func (e env) idle(id accounts.ID) {
	e.t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		e.service.mu.Lock()
		_, busy := e.service.running[id]
		e.service.mu.Unlock()
		if !busy {
			return
		}
	}
	e.t.Fatal("the scan did not end")
}

// files reads a folder's files: by path, the title matched or the reason.
func (e env) files(id accounts.ID) map[string]string {
	e.t.Helper()
	rows, err := e.pool.Query(e.t.Context(), "SELECT path, coalesce(stremio_id, 'unmatched: ' || reason) FROM local_files WHERE addon_id = $1", id)
	if err != nil {
		e.t.Fatal(err)
	}
	result := map[string]string{}
	for rows.Next() {
		var rel, title string
		if err := rows.Scan(&rel, &title); err != nil {
			e.t.Fatal(err)
		}
		result[rel] = title
	}
	return result
}

// write makes a file of size bytes at path, its folders too.
func write(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}
