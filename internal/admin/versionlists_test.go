package admin

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Replacing an addon's address, once it was reconfigured, forgets the
// version lists it gave, in memory and saved, and those alone: the title
// asks it again, and lists what its new configuration lists, while the
// other addon's list stays, not asked again.
func TestReplacingAnAddressForgetsThatAddonsVersionLists(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	asked := map[string]int{}
	// stream answers a stream request with one stream named name.
	stream := func(w http.ResponseWriter, name string) {
		mu.Lock()
		asked[name]++
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{{Name: name, URL: "https://cdn.example/" + strings.ReplaceAll(name, " ", "-") + ".mkv"}}})
	}
	movie := stremio.Meta{ID: "tt0001", Type: "movie", Name: "First"}
	// The reconfigured addon answers at its new address with other
	// streams.
	reels := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		name := "Reels before"
		if rest, ok := strings.CutPrefix(path, "/configured"); ok {
			path, name = rest, "Reels after"
		}
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "reels", Name: "Reels", Version: "1", Types: []string{"movie"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta", IDPrefixes: []string{"tt"}}, {Name: "stream", IDPrefixes: []string{"tt"}}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case path == "/meta/movie/tt0001.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case path == "/stream/movie/tt0001.json":
			stream(w, name)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(reels.Close)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "other", Name: "Other", Version: "1", Types: []string{"movie"},
				Resources: []stremio.Resource{{Name: "stream", IDPrefixes: []string{"tt"}}}})
		case "/stream/movie/tt0001.json":
			stream(w, "Other")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(other.Close)
	var lib *library.Service
	var pool *pgxpool.Pool
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		lib, pool = o.AddonCaches.(*library.Service), deps.pool
		// No follow-ups: the addons are asked only as the title is listed.
		lib.SetFollowUps()
	})
	admin := api.signedIn("root", true)
	ids := map[string]string{}
	for name, url := range map[string]string{"reels": reels.URL, "other": other.URL} {
		status, installed, _ := admin.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": url + "/manifest.json"})
		if status != http.StatusCreated {
			t.Fatalf("installing %s: %d %v", name, status, installed)
		}
		ids[name] = installed["id"].(string)
	}
	root, err := api.store.Authenticate(t.Context(), "root", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	libraries := admin.libraries("shared")
	i := slices.IndexFunc(libraries, func(l map[string]any) bool { return l["catalogType"] == "movie" })
	if i < 0 {
		t.Fatalf("no movie library in %v", libraries)
	}
	libraryID, _ := accounts.ParseID(libraries[i]["itemId"].(string))
	page, err := lib.Children(t.Context(), root, libraryID, 0, 10, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("the library: %v %v", page.Items, err)
	}
	title := page.Items[0].ID
	// names lists the names of the versions, known only or asked for.
	names := func(list func() ([]library.Version, error)) []string {
		t.Helper()
		versions, err := list()
		if err != nil {
			t.Fatal(err)
		}
		var result []string
		for _, version := range versions {
			result = append(result, version.Name)
		}
		slices.Sort(result)
		return result
	}
	known := func() ([]library.Version, error) { return lib.KnownVersions(t.Context(), root, title) }
	listed := func() ([]library.Version, error) { return lib.Versions(t.Context(), root, title) }
	// saved lists the addons whose stream list is saved.
	saved := func() []string {
		t.Helper()
		rows, _ := pool.Query(t.Context(), "SELECT addon_id FROM stream_lists ORDER BY addon_id")
		addons, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
		if err != nil {
			t.Fatal(err)
		}
		var result []string
		for _, id := range addons {
			result = append(result, id.String())
		}
		slices.Sort(result)
		return result
	}
	both := []string{ids["other"], ids["reels"]}
	slices.Sort(both)
	if got := names(listed); !slices.Equal(got, []string{"Other", "Reels before"}) {
		t.Fatalf("versions before: %q", got)
	}
	if got := saved(); !slices.Equal(got, both) {
		t.Fatalf("lists saved before: %v, want %v", got, both)
	}

	status, body, _ := admin.call(http.MethodPatch, "/scopes/shared/addons/"+ids["reels"], map[string]string{"manifestUrl": reels.URL + "/configured/manifest.json"})
	if status != http.StatusOK {
		t.Fatalf("replacing the address: %d %v", status, body)
	}
	if got := names(known); !slices.Equal(got, []string{"Other"}) {
		t.Errorf("versions known once the address is replaced: %q, want the other addon's", got)
	}
	if got := saved(); !slices.Equal(got, []string{ids["other"]}) {
		t.Errorf("lists saved once the address is replaced: %v, want the other addon's", got)
	}
	// Listed again, the title asks the reconfigured addon alone.
	if got := names(listed); !slices.Equal(got, []string{"Other", "Reels after"}) {
		t.Errorf("versions listed again: %q", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if want := map[string]int{"Reels before": 1, "Reels after": 1, "Other": 1}; !maps.Equal(asked, want) {
		t.Errorf("addons asked %v, want %v", asked, want)
	}
}
