package jellyfin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/stremio"
)

// sortedMovie is a movie of an Xtream account's list: when it was added,
// as Unix seconds, its year and its rating, empty when the list gives
// none.
type sortedMovie struct {
	name, added, year, rating string
}

// sortingProvider is an Xtream account (user / secret) of movies.
func sortingProvider(t *testing.T, movies []sortedMovie) string {
	t.Helper()
	address, _ := changingProvider(t, movies)
	return address
}

// changingProvider is sortingProvider, whose movies set changes.
func changingProvider(t *testing.T, movies []sortedMovie) (address string, set func([]sortedMovie)) {
	t.Helper()
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if r.URL.Path != "/player_api.php" || query.Get("username") != "user" || query.Get("password") != "secret" {
			http.NotFound(w, r)
			return
		}
		var answer any
		switch query.Get("action") {
		case "":
			answer = map[string]any{"user_info": map[string]any{"auth": 1, "allowed_output_formats": []string{"ts"}}}
		case "get_vod_categories":
			answer = []any{map[string]any{"category_id": "1", "category_name": "Action"}}
		case "get_vod_streams":
			mu.Lock()
			listed := movies
			mu.Unlock()
			list := make([]any, len(listed))
			for i, m := range listed {
				list[i] = map[string]any{"name": m.name, "stream_id": 100 + i, "category_id": "1", "container_extension": "mkv",
					"added": m.added, "year": m.year, "rating": m.rating}
			}
			answer = list
		default:
			answer = []any{}
		}
		_ = json.NewEncoder(w).Encode(answer)
	}))
	t.Cleanup(server.Close)
	return server.URL, func(changed []sortedMovie) {
		mu.Lock()
		defer mu.Unlock()
		movies = changed
	}
}

// pageNames lists the names of a page of a listing, and its total.
func (s testServer) pageNames(t *testing.T, path, token string) ([]string, int) {
	t.Helper()
	var page QueryResult
	if status := s.get(t, path, token, &page); status != http.StatusOK {
		t.Fatalf("%s: %d", path, status)
	}
	return itemNames(page.Items), page.TotalRecordCount
}

// A page past the first of an IPTV source's movies comes in the order of
// jellyfin-web's Sort menu, ascending and descending: by name, date added,
// release date, year and rating, then by name, the way the listing asks
// it. A movie without a value comes first ascending and last descending,
// as in Jellyfin. The listing reads the same through /Users/{id}/Items. An
// addon's catalog keeps its order whatever the sort.
func TestStoredLibrariesPageInTheSortAsked(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	token := s.signIn("member", "tv")
	on, off := true, false
	movies := []sortedMovie{
		{"Delta", "1700000600", "1999", "7.1"}, {"Alpha", "1700000100", "2005", "8"}, {"Echo", "1700000700", "", ""},
		{"Bravo", "1700000300", "2005", "6.5"}, {"Charlie", "1700000200", "1987", "9.2"}, {"Foxtrot", "", "2010", "5"},
		{"Golf", "1700000500", "1999", "6.5"},
	}
	if _, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Box", Account: iptv.Account{Kind: addons.KindXtream,
		Server: sortingProvider(t, movies), Username: "user", Password: "secret"}, Options: &iptv.OptionsPatch{Movies: &on, LiveTv: &off}},
		false); err != nil {
		t.Fatal(err)
	}
	library := s.viewsByType(t, token)["movies"]
	if _, err := s.addons.Install(t.Context(), addons.Shared(), deepAddon(t), false); err != nil {
		t.Fatal(err)
	}
	deep := s.viewIDs(t, token)["Deep"]
	if library == "" || deep == "" || library == deep {
		t.Fatalf("libraries: %v", s.viewIDs(t, token))
	}
	listing := func(parent, sortBy, sortOrder string) string {
		return "/Items?" + url.Values{"ParentId": {parent}, "IncludeItemTypes": {"Movie"}, "Recursive": {"true"},
			"SortBy": {sortBy}, "SortOrder": {sortOrder}, "StartIndex": {"2"}, "Limit": {"3"}}.Encode()
	}
	for _, tc := range []struct {
		sortBy    string
		ascending []string
	}{
		{"SortName", []string{"Alpha", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot", "Golf"}},
		// Foxtrot has no date added: the provider's list puts it last.
		{"DateCreated,SortName", []string{"Foxtrot", "Alpha", "Charlie", "Bravo", "Golf", "Delta", "Echo"}},
		{"PremiereDate,ProductionYear,SortName", []string{"Echo", "Charlie", "Delta", "Golf", "Alpha", "Bravo", "Foxtrot"}},
		{"ProductionYear,PremiereDate,SortName", []string{"Echo", "Charlie", "Delta", "Golf", "Alpha", "Bravo", "Foxtrot"}},
		{"CommunityRating,SortName", []string{"Echo", "Foxtrot", "Bravo", "Golf", "Delta", "Alpha", "Charlie"}},
	} {
		descending := slices.Clone(tc.ascending)
		slices.Reverse(descending)
		for order, want := range map[string][]string{"Ascending": tc.ascending, "Descending": descending} {
			names, total := s.pageNames(t, listing(library, tc.sortBy, order), token)
			if total != len(movies) || !slices.Equal(names, want[2:5]) {
				t.Errorf("%s %s: %v of %d, want %v of %d", tc.sortBy, order, names, total, want[2:5], len(movies))
			}
		}
	}
	// Movies of one year by name ascending, the years descending.
	names, _ := s.pageNames(t, listing(library, "ProductionYear,SortName", "Descending,Ascending"), token)
	if want := []string{"Bravo", "Delta", "Golf"}; !slices.Equal(names, want) {
		t.Errorf("years descending, names ascending: %v, want %v", names, want)
	}
	names, _ = s.pageNames(t, "/Users/"+member.ID.String()+listing(library, "CommunityRating,SortName", "Descending"), token)
	if want := []string{"Delta", "Golf", "Bravo"}; !slices.Equal(names, want) {
		t.Errorf("through /Users/{id}/Items: %v, want %v", names, want)
	}
	for _, sortBy := range []string{"SortName", "DateCreated,SortName", "CommunityRating,SortName"} {
		names, _ := s.pageNames(t, listing(deep, sortBy, "Descending"), token)
		if want := []string{"Movie 003", "Movie 004", "Movie 005"}; !slices.Equal(names, want) {
			t.Errorf("an addon's catalog by %s: %v, want %v", sortBy, names, want)
		}
	}
}

// An order kept for a stored library holds while what the library lists
// stays as it was: once its IPTV source's list is refreshed with a movie
// renamed, or an administrator renames one, the next listing by name sorts
// them again, at once. A listing without a sort keeps the provider's
// order, the movies added last first.
func TestStoredLibraryOrdersFollowItsChanges(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("member", nil)
	s.user("boss", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	token, admin := s.signIn("member", "tv"), s.signIn("boss", "dashboard")
	on, off := true, false
	movies := []sortedMovie{{"Alpha", "1700000300", "2001", "7"}, {"Bravo", "1700000100", "2002", "7"}, {"Charlie", "1700000200", "2003", "7"}}
	address, set := changingProvider(t, movies)
	source, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Box", Account: iptv.Account{Kind: addons.KindXtream,
		Server: address, Username: "user", Password: "secret"}, Options: &iptv.OptionsPatch{Movies: &on, LiveTv: &off}}, false)
	if err != nil {
		t.Fatal(err)
	}
	library := s.viewsByType(t, token)["movies"]
	listing := func(sorted bool) QueryResult {
		t.Helper()
		query := url.Values{"ParentId": {library}, "IncludeItemTypes": {"Movie"}, "Recursive": {"true"}}
		if sorted {
			query.Set("SortBy", "SortName")
			query.Set("SortOrder", "Ascending")
		}
		var page QueryResult
		if status := s.get(t, "/Items?"+query.Encode(), token, &page); status != http.StatusOK {
			t.Fatalf("listing: %d", status)
		}
		return page
	}
	if got := itemNames(listing(true).Items); !slices.Equal(got, []string{"Alpha", "Bravo", "Charlie"}) {
		t.Fatalf("by name: %v", got)
	}

	// The provider renames Alpha.
	set([]sortedMovie{{"Zulu", "1700000300", "2001", "7"}, movies[1], movies[2]})
	if err := s.iptv.Refresh(t.Context(), addons.Shared(), source.ID, false); err != nil {
		t.Fatal(err)
	}
	byName := listing(true)
	if got := itemNames(byName.Items); !slices.Equal(got, []string{"Bravo", "Charlie", "Zulu"}) {
		t.Errorf("by name once the list is refreshed: %v", got)
	}

	// An administrator renames Charlie.
	charlie := slices.IndexFunc(byName.Items, func(item BaseItemDto) bool { return item.Name == "Charlie" })
	if charlie < 0 {
		t.Fatalf("no Charlie in %v", itemNames(byName.Items))
	}
	if status, body := s.call(http.MethodPost, "/Items/"+byName.Items[charlie].Id, app("dashboard", admin),
		map[string]any{"Name": "Able"}); status != http.StatusNoContent {
		t.Fatalf("renaming Charlie: %d %s", status, body)
	}
	if got := itemNames(listing(true).Items); !slices.Equal(got, []string{"Able", "Bravo", "Zulu"}) {
		t.Errorf("by name once Charlie is renamed: %v", got)
	}
	if got := itemNames(listing(false).Items); !slices.Equal(got, []string{"Zulu", "Able", "Bravo"}) {
		t.Errorf("without a sort: %v", got)
	}
}

// A page past the first of a local folder's movies comes by date added,
// the dates of their files, and by length, the length their files play,
// which the listing gives, a movie of unknown length first ascending.
func TestLocalFoldersSortByDateAddedAndLength(t *testing.T) {
	t.Parallel()
	var folders *localfiles.Service
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, slog.New(slog.NewTextHandler(io.Discard, nil)),
			o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
	})
	s.user("member", nil)
	token := s.signIn("member", "tv")
	dir := t.TempDir()
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	files := []struct {
		name    string
		minutes int
		seconds float64
	}{{"Alpha", 30, 5400}, {"Bravo", 10, 3600}, {"Charlie", 40, 0}, {"Delta", 20, 7200}, {"Echo", 50, 6000}}
	for _, f := range files {
		path := filepath.Join(dir, f.name+".mkv")
		if err := os.WriteFile(path, []byte(f.name), 0o644); err != nil {
			t.Fatal(err)
		}
		at := base.Add(time.Duration(f.minutes) * time.Minute)
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	folder, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Shelf", Path: dir, Kind: localfiles.KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), folder.ID); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.seconds == 0 {
			continue
		}
		if _, err := s.pool.Exec(t.Context(), "UPDATE local_files SET duration = $1 WHERE addon_id = $2 AND path = $3",
			f.seconds, folder.ID, f.name+".mkv"); err != nil {
			t.Fatal(err)
		}
	}
	library := s.viewIDs(t, token)["Shelf"]
	if library == "" {
		t.Fatalf("libraries: %v", s.viewIDs(t, token))
	}
	listing := func(sortBy, sortOrder string) string {
		return "/Items?" + url.Values{"ParentId": {library}, "IncludeItemTypes": {"Movie"}, "Recursive": {"true"}, "SortBy": {sortBy},
			"SortOrder": {sortOrder}, "StartIndex": {"1"}, "Limit": {"3"}}.Encode()
	}
	for _, tc := range []struct {
		sortBy    string
		ascending []string
	}{
		{"DateCreated,SortName", []string{"Bravo", "Delta", "Alpha", "Charlie", "Echo"}},
		{"Runtime,SortName", []string{"Charlie", "Bravo", "Alpha", "Echo", "Delta"}},
	} {
		descending := slices.Clone(tc.ascending)
		slices.Reverse(descending)
		for order, want := range map[string][]string{"Ascending": tc.ascending, "Descending": descending} {
			names, total := s.pageNames(t, listing(tc.sortBy, order), token)
			if total != len(files) || !slices.Equal(names, want[1:4]) {
				t.Errorf("%s %s: %v of %d, want %v of %d", tc.sortBy, order, names, total, want[1:4], len(files))
			}
		}
	}
	var page QueryResult
	s.get(t, listing("Runtime", "Descending"), token, &page)
	for i, item := range page.Items {
		if item.RunTimeTicks == nil || *item.RunTimeTicks != []int64{6000, 5400, 3600}[i]*10_000_000 {
			t.Errorf("%s's length: %v", item.Name, item.RunTimeTicks)
		}
	}
}
