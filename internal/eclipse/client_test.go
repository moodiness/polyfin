package eclipse

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/stremio"
)

// queryAddon serves an addon that keeps the user's token in its manifest
// URL's query, as some do (…/manifest.json?token=abc), and refuses every
// request without it. It returns the queries its resources were asked
// with, by path.
func queryAddon(t *testing.T) (*httptest.Server, func(path string) url.Values) {
	t.Helper()
	var mu sync.Mutex
	asked := map[string]url.Values{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		mu.Lock()
		asked[r.URL.Path] = query
		mu.Unlock()
		if query.Get("token") != "abc" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		var answer any
		switch path := strings.TrimPrefix(r.URL.Path, "/addon"); {
		case path == "/search":
			answer = map[string]any{"tracks": []map[string]any{{"id": "t1", "title": "Found " + query.Get("q"), "artist": "A"}}}
		case path == "/stream/t 1":
			answer = map[string]any{"url": "https://cdn.example/a.mp3", "format": "mp3"}
		case path == "/catalog/top":
			answer = map[string]any{"items": []map[string]any{{"id": "t" + query.Get("skip"), "type": "track", "title": "Top", "artist": "A"}}}
		case path == "/album/a1":
			answer = map[string]any{"id": "a1", "title": "Album", "artist": "A", "tracks": []map[string]any{{"id": "t1", "title": "One", "artist": "A"}}}
		default:
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(answer)
	}))
	t.Cleanup(server.Close)
	return server, func(path string) url.Values {
		mu.Lock()
		defer mu.Unlock()
		return asked[path]
	}
}

// A manifest URL with a query asks for resources under its path, with its
// query merged into each request's: the settings replace its parameters of
// the same name, and the request's own (q, skip) replace both.
func TestAManifestURLsQueryTravelsWithEveryRequest(t *testing.T) {
	server, asked := queryAddon(t)
	manifest, err := ParseManifest([]byte(`{"id":"a","name":"A","version":"1.0.0","resources":["search","stream","catalog","settings"],
		"catalogs":[{"id":"top","type":"track","name":"Top"}],
		"settings":[{"key":"quality","type":"select","label":"Quality","default":"high",
			"options":[{"value":"high","label":"High"},{"value":"low","label":"Low"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	manifestURL, err := stremio.NormalizeManifestURL(server.URL + "/addon/manifest.json?token=abc&quality=url&skip=7&q=stale")
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(stremio.NewClient("test"))
	addon := Addon{ManifestURL: manifestURL, Manifest: manifest, Settings: map[string]string{"quality": "low"}}
	want := func(path string, params map[string]string) {
		t.Helper()
		query := asked(path)
		if query == nil {
			t.Errorf("%s was never asked", path)
			return
		}
		for key, value := range params {
			if got := query[key]; len(got) != 1 || got[0] != value {
				t.Errorf("%s: %s = %v, want %q (query %v)", path, key, got, value, query)
			}
		}
	}

	results, err := client.Search(t.Context(), addon, "daft punk é&x")
	if err != nil || len(results.Tracks) != 1 {
		t.Fatalf("search: %v, %+v", err, results)
	}
	want("/addon/search", map[string]string{"token": "abc", "quality": "low", "q": "daft punk é&x", "skip": "7"})

	if _, err := client.Stream(t.Context(), addon, "t 1"); err != nil {
		t.Fatalf("stream: %v", err)
	}
	want("/addon/stream/t 1", map[string]string{"token": "abc", "quality": "low"})

	catalog, _ := manifest.Catalog("top")
	for _, skip := range []int{0, CatalogPage} {
		items, _, err := client.Catalog(t.Context(), addon, catalog, skip)
		if err != nil || len(items) != 1 {
			t.Fatalf("catalog from %d: %v, %d items", skip, err, len(items))
		}
		want("/addon/catalog/top", map[string]string{"token": "abc", "quality": "low", "skip": strconv.Itoa(skip)})
	}

	if album, err := client.Album(t.Context(), addon, "a1"); err != nil || len(album.Tracks) != 1 {
		t.Fatalf("album: %v, %+v", err, album)
	}
	want("/addon/album/a1", map[string]string{"token": "abc", "quality": "low"})
}

// A token in the manifest URL's path stays the prefix of every request.
func TestATokenInTheManifestURLsPathPrefixesEveryRequest(t *testing.T) {
	asked := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked <- r.URL.Path + "?" + r.URL.RawQuery
		_, _ = w.Write([]byte(`{"tracks":[{"id":"t1","title":"T","artist":"A"}]}`))
	}))
	defer server.Close()
	manifest, err := ParseManifest([]byte(`{"id":"a","name":"A","version":"1.0.0","resources":["search","stream"]}`))
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(stremio.NewClient("test"))
	addon := Addon{ManifestURL: server.URL + "/tok123/manifest.json", Manifest: manifest}
	if _, err := client.Search(t.Context(), addon, "a b"); err != nil {
		t.Fatal(err)
	}
	if got := <-asked; got != "/tok123/search?q=a+b" {
		t.Errorf("asked %s", got)
	}
}
