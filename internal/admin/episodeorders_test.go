package admin

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/collections"
	"github.com/moodiness/polyfin/internal/jellyfin"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Episode orders' search and list answer each series' poster as Content ›
// Versions does: the item whose Primary image Polyfin's own image route
// serves. A series without a poster answers it too, and the route answers
// 404, on which the admin app shows its placeholder.
func TestEpisodeOrderSeriesCarryTheirPoster(t *testing.T) {
	t.Parallel()
	picture := pngPicture(t)
	var addon *httptest.Server
	addon = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "shows", Name: "Shows", Version: "1", Types: []string{"series"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "series", ID: "search", Name: "Search",
					Extra: []stremio.Extra{{Name: "search", IsRequired: true}}}}})
		case strings.HasPrefix(path, "/catalog/series/search/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{
				{ID: "tt0010", Type: "series", Name: "Harbour", ReleaseInfo: "2021", Poster: addon.URL + "/poster.png"},
				{ID: "tt0011", Type: "series", Name: "Harbour", ReleaseInfo: "1987"}}})
		case path == "/meta/series/tt0010.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": stremio.Meta{ID: "tt0010", Type: "series", Name: "Harbour",
				ReleaseInfo: "2021", Poster: addon.URL + "/poster.png", Videos: []stremio.Video{
					{ID: "tt0010:1:1", Title: "Pilot", Season: 1, Episode: 1, Released: "2021-01-01T00:00:00Z"}}}})
		case path == "/poster.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(picture)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(addon.Close)
	var images *httptest.Server
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		lib := library.New(deps.pool, deps.addons, deps.client, logger, o.Accounts.Settings)
		if _, err := deps.addons.Install(t.Context(), addons.Shared(), addon.URL+"/manifest.json", false); err != nil {
			t.Fatal(err)
		}
		o.EpisodeOrders = lib
		// Polyfin's image route, which the admin app's posters load from.
		images = httptest.NewServer(jellyfin.New(jellyfin.Options{Accounts: o.Accounts, Library: lib, Stremio: deps.client,
			Collections: collections.New(deps.pool), Logger: logger, CacheDir: t.TempDir()}))
		t.Cleanup(images.Close)
	})
	admin := api.signedIn("root", true)

	var found []map[string]any
	if status := admin.raw(http.MethodGet, "/episode-orders/series?search=Harbour", "", &found); status != http.StatusOK || len(found) != 2 {
		t.Fatalf("search: %d %v", status, found)
	}
	poster := func(series map[string]any) (int, []byte) {
		t.Helper()
		id, _ := series["posterId"].(string)
		if id == "" || id != series["id"] {
			t.Fatalf("a series' posterId is not its own: %v", series)
		}
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet,
			images.URL+"/Items/"+id+"/Images/Primary?maxHeight=96&quality=85", nil)
		response, err := (&http.Client{Transport: testTransport}).Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		return response.StatusCode, body
	}
	byYear := map[float64]map[string]any{}
	for _, series := range found {
		year, _ := series["year"].(float64)
		byYear[year] = series
	}
	if status, body := poster(byYear[2021]); status != http.StatusOK || !bytes.Equal(body, picture) {
		t.Errorf("the poster of the series that has one: %d, %d bytes", status, len(body))
	}
	if status, _ := poster(byYear[1987]); status != http.StatusNotFound {
		t.Errorf("the poster of the series without one: %d, want 404", status)
	}

	// The series given an order list theirs too.
	id, _ := byYear[2021]["id"].(string)
	if status := admin.raw(http.MethodPut, "/titles/"+id+"/episode-order", `{"group": "", "specials": "last"}`, nil); status != http.StatusOK {
		t.Fatalf("specials last: %d", status)
	}
	var orders []struct {
		Series map[string]any `json:"series"`
	}
	if status := admin.raw(http.MethodGet, "/episode-orders", "", &orders); status != http.StatusOK || len(orders) != 1 {
		t.Fatalf("orders: %d %v", status, orders)
	}
	if status, body := poster(orders[0].Series); status != http.StatusOK || !bytes.Equal(body, picture) {
		t.Errorf("the poster of the series listed: %d, %d bytes", status, len(body))
	}
}
