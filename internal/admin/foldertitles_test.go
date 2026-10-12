package admin

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/stremio"
)

// newSearchAddon serves a metadata addon whose movie and series search
// catalogs find the titles whose names hold the term: twelve movies of the
// Harbor Lights saga, one more known only by the addon's own identifier,
// and a show.
func newSearchAddon(t *testing.T) *httptest.Server {
	t.Helper()
	titles := []stremio.Meta{{ID: "kitsu:7", Type: "movie", Name: "Harbor Lights Special", Year: "1951", Poster: "https://images.example/special.jpg"}}
	for i := range 12 {
		meta := stremio.Meta{ID: fmt.Sprintf("tt09000%02d", i), Type: "movie", Name: fmt.Sprintf("Harbor Lights %d", i+1),
			Year: stremio.Text(fmt.Sprint(1940 + i))}
		if i%2 == 0 {
			meta.Poster = fmt.Sprintf("https://images.example/%d.jpg", i)
		}
		titles = append(titles, meta)
	}
	titles = append(titles, stremio.Meta{ID: "tt0950000", Type: "series", Name: "Harbor Lights", Year: "1958–1960"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			search := []stremio.Extra{{Name: "search", IsRequired: true}}
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "meta", Name: "Meta", Version: "1", Types: []string{"movie", "series"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "search", Name: "Search", Extra: search},
					{Type: "series", ID: "search", Name: "Search", Extra: search}}})
		case strings.HasPrefix(path, "/catalog/"):
			typ, query, _ := strings.Cut(strings.TrimSuffix(strings.TrimPrefix(path, "/catalog/"), ".json"), "/search/search=")
			term, _ := url.PathUnescape(query)
			metas := []stremio.Meta{}
			for _, meta := range titles {
				if meta.Type == typ && strings.Contains(strings.ToLower(meta.Name), strings.ToLower(term)) {
					metas = append(metas, meta)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// The title search of a local folder finds titles of the folder's kind
// with an IMDb identifier, those of the year asked first, ten at most; an
// unknown folder is not found, and only administrators may search.
func TestFolderTitleSearch(t *testing.T) {
	t.Parallel()
	var folders *localfiles.Service
	var store *addons.Store
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		lib := library.New(deps.pool, deps.addons, deps.client, logger, o.Accounts.Settings)
		folders = localfiles.New(deps.pool, deps.addons, lib, logger, o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Folders, store = folders, deps.addons
	})
	addon := newSearchAddon(t)
	if _, err := store.Install(t.Context(), addons.Shared(), addon.URL+"/manifest.json", false); err != nil {
		t.Fatal(err)
	}
	movies, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Movies", Path: t.TempDir(), Kind: localfiles.KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	shows, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Shows", Path: t.TempDir(), Kind: localfiles.KindShows})
	if err != nil {
		t.Fatal(err)
	}
	admin := api.signedIn("root", true)
	member := api.signedIn("sam", false)
	search := func(b browser, folder, query string) (int, []titleJSON) {
		t.Helper()
		status, answer, _ := b.call(http.MethodGet, "/scopes/shared/folders/"+folder+"/titles?"+query, nil)
		var body struct{ Titles []titleJSON }
		encoded, _ := json.Marshal(answer)
		_ = json.Unmarshal(encoded, &body)
		return status, body.Titles
	}

	status, titles := search(admin, movies.ID.String(), "query=harbor+lights&year=1945")
	if status != http.StatusOK || len(titles) != 10 {
		t.Fatalf("movies: %d %+v", status, titles)
	}
	if first := titles[0]; first.ImdbID != "tt0900005" || first.Name != "Harbor Lights 6" || first.Year == nil || *first.Year != 1945 ||
		first.Kind != "movie" || first.PosterID != nil {
		t.Errorf("the year's title first: %+v", first)
	}
	posters := 0
	for _, title := range titles {
		if !strings.HasPrefix(title.ImdbID, "tt09000") || title.Kind != "movie" {
			t.Errorf("not a movie with an IMDb identifier: %+v", title)
		}
		if title.PosterID != nil {
			posters++
		}
	}
	if posters == 0 {
		t.Error("no poster")
	}
	if status, titles := search(admin, shows.ID.String(), "query=harbor"); status != http.StatusOK || len(titles) != 1 ||
		titles[0].ImdbID != "tt0950000" || titles[0].Kind != "show" || titles[0].Year == nil || *titles[0].Year != 1958 {
		t.Errorf("shows: %d %+v", status, titles)
	}
	if status, titles := search(admin, movies.ID.String(), "query="); status != http.StatusOK || len(titles) != 0 {
		t.Errorf("no query: %d %+v", status, titles)
	}
	if status, _ := search(admin, movies.ID.String(), "query=harbor&year=soon"); status != http.StatusBadRequest {
		t.Errorf("a wrong year: %d", status)
	}
	if status, _ := search(admin, "0123456789abcdef0123456789abcdef", "query=harbor"); status != http.StatusNotFound {
		t.Errorf("an unknown folder: %d", status)
	}
	if status, _ := search(member, movies.ID.String(), "query=harbor"); status != http.StatusForbidden {
		t.Errorf("a member: %d", status)
	}
	// An addon that cannot be reached is told, as other addon failures are.
	addon.Close()
	if status, answer, _ := admin.call(http.MethodGet, "/scopes/shared/folders/"+movies.ID.String()+"/titles?query=lights", nil); status != http.StatusBadGateway ||
		answer["error"] != "addon_unreachable" {
		t.Errorf("an unreachable addon: %d %v", status, answer)
	}
}
