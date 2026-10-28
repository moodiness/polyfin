package localfiles

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// downAddon is a metadata addon of movies and series whose search catalogs
// and descriptions answer 503, or, with none set, whose searches find
// nothing.
func downAddon(t *testing.T, name string, none bool) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			search := []stremio.Extra{{Name: "search", IsRequired: true}}
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "meta." + strings.ToLower(name), Name: name, Version: "1",
				Types: []string{"movie", "series"}, IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top", Extra: search}, {Type: "series", ID: "top", Name: "Top", Extra: search}}})
		case none && strings.HasPrefix(path, "/catalog/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{}})
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// titlesTMDB answers TMDB's movie and series searches and external
// identifiers from what it is given, and counts the requests.
type titlesTMDB struct {
	*httptest.Server
	mu       sync.Mutex
	requests int
}

func newTitlesTMDB(t *testing.T) *titlesTMDB {
	t.Helper()
	movie := func(id int, title, date string) map[string]any {
		return map[string]any{"id": id, "title": title, "original_title": title, "release_date": date, "poster_path": "/" + title + ".jpg"}
	}
	searches := map[string][]map[string]any{
		"movie|Harbor Lights|2011": {movie(7001, "Harbor Lights", "2011-05-06")},
		"movie|Common Name|":       {movie(7101, "Common Name", "2001-01-01"), movie(7102, "Common Name", "2015-01-01")},
		"tv|Vicarious|2010": {{"id": 9001, "name": "Vicarious", "original_name": "Vicarious", "first_air_date": "2010-03-27",
			"poster_path": "/vicarious.jpg"}},
	}
	imdb := map[string]string{"movie/7001": "tt0100001", "tv/9001": "tt1604099", "movie/7101": "tt0100101", "movie/7102": "tt0100102"}
	f := &titlesTMDB{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests++
		f.mu.Unlock()
		q := r.URL.Query()
		if q.Get("api_key") != "k3y" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/3/")
		if kind, ok := strings.CutPrefix(path, "search/"); ok {
			year := q.Get("year") + q.Get("first_air_date_year")
			_ = json.NewEncoder(w).Encode(map[string]any{"results": searches[kind+"|"+q.Get("query")+"|"+year]})
			return
		}
		if id, ok := strings.CutSuffix(path, "/external_ids"); ok {
			if found, ok := imdb[id]; ok {
				_ = json.NewEncoder(w).Encode(map[string]any{"imdb_id": found})
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *titlesTMDB) asked() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests
}

// tmdbEnv is an environment of the addons of manifests that searches TMDB
// with key, empty for none.
func tmdbEnv(t *testing.T, key string, manifests ...string) (env, *titlesTMDB) {
	t.Helper()
	e := newEnvWith(t, manifests...)
	fake := newTitlesTMDB(t)
	settings := e.users.Settings()
	settings.TMDBKey = key
	if _, err := e.users.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	e.service.UseTMDB(tmdb.New(fake.URL+"/3", fake.URL+"/img/", "test", e.users.Settings))
	return e, fake
}

// A folder's files whose title every metadata addon fails to search are
// matched through TMDB's search, as Jellyfin's TMDB providers search a
// title, by the exact rule: a movie and a show of one title each are
// matched to their IMDb identifiers, named, dated and pictured by TMDB
// while the addons cannot describe them, and told as matched by TMDB; a
// name TMDB knows twice, or not at all, stays unmatched, its search
// failed.
func TestFilesAreMatchedThroughTMDBWhenEveryAddonFails(t *testing.T) {
	t.Parallel()
	e, fake := tmdbEnv(t, "k3y", downAddon(t, "Down", false), downAddon(t, "Other", false))
	movies, _ := e.folder(KindMovies, map[string]int{"Harbor Lights (2011).mkv": 1, "Common Name.mkv": 1, "Unknown Reel (1931).mkv": 1})
	want := map[string]string{"Harbor Lights (2011).mkv": "tt0100001", "Common Name.mkv": "unmatched: search_failed",
		"Unknown Reel (1931).mkv": "unmatched: search_failed"}
	if got := e.files(movies.ID); !maps.Equal(got, want) {
		t.Errorf("movies: %v, want %v", got, want)
	}
	shows, _ := e.folder(KindShows, map[string]int{"Vicarious (2010)/Season 01/Vicarious S01E01 Pilot.mkv": 1})
	if got := e.files(shows.ID); got["Vicarious (2010)/Season 01/Vicarious S01E01 Pilot.mkv"] != "tt1604099" {
		t.Errorf("shows: %v", got)
	}

	var name, poster, by string
	var year int
	if err := e.pool.QueryRow(t.Context(), `SELECT name, poster, title_year, matched_by FROM local_files WHERE addon_id = $1 AND path = $2`,
		movies.ID, "Harbor Lights (2011).mkv").Scan(&name, &poster, &year, &by); err != nil {
		t.Fatal(err)
	}
	if name != "Harbor Lights" || year != 2011 || by != "tmdb" || poster != fake.URL+"/img/original/Harbor Lights.jpg" {
		t.Errorf("Harbor Lights: %q %d %q %q", name, year, by, poster)
	}
	for _, id := range []accounts.ID{movies.ID, shows.ID} {
		folder, err := e.service.Folder(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if folder.MatchedBy.TMDB != 1 || folder.MatchedBy.Search != 0 {
			t.Errorf("folder %s matched by %+v", folder.Kind, folder.MatchedBy)
		}
	}
}

// TMDB is searched only while the server has a TMDB key, and only when
// every addon failed: a search an addon answered, finding nothing, leaves
// the file unmatched as before, without asking TMDB, even when another
// addon failed.
func TestTMDBIsSearchedOnlyWhenEveryAddonFailsWithAKey(t *testing.T) {
	t.Parallel()
	files := map[string]int{"Harbor Lights (2011).mkv": 1}
	for _, c := range []struct {
		name, key string
		addons    []bool // each addon answers nothing, or fails
		want      string
	}{
		{"no key", "", []bool{false}, "unmatched: search_failed"},
		{"an addon finding nothing", "k3y", []bool{true}, "unmatched: not_found"},
		{"one addon failing, another finding nothing", "k3y", []bool{false, true}, "unmatched: search_failed"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			var manifests []string
			for i, none := range c.addons {
				manifests = append(manifests, downAddon(t, "Addon"+string(rune('A'+i)), none))
			}
			e, fake := tmdbEnv(t, c.key, manifests...)
			folder, _ := e.folder(KindMovies, files)
			if got := e.files(folder.ID)["Harbor Lights (2011).mkv"]; got != c.want {
				t.Errorf("Harbor Lights: %q, want %q", got, c.want)
			}
			if n := fake.asked(); n != 0 {
				t.Errorf("TMDB asked %d times", n)
			}
		})
	}
}
