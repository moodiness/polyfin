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
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/stremio"
)

// checkAdded fails unless each of items, by name, has the DateCreated
// added gives it; a name added has no date for has none, never 1970.
func checkAdded(t *testing.T, where string, items []BaseItemDto, added map[string]time.Time) {
	t.Helper()
	if len(items) == 0 {
		t.Errorf("%s: no titles", where)
	}
	for _, item := range items {
		want, known := added[item.Name]
		switch {
		case !known && item.DateCreated != nil:
			t.Errorf("%s: %s, of no date added, says %v", where, item.Name, time.Time(*item.DateCreated))
		case known && item.DateCreated == nil:
			t.Errorf("%s: %s has no date added, want %v", where, item.Name, want)
		case known && !time.Time(*item.DateCreated).Equal(want):
			t.Errorf("%s: %s added %v, want %v", where, item.Name, time.Time(*item.DateCreated), want)
		}
	}
}

// Each movie of an IPTV source tells, as DateCreated, the date its
// provider gives, the one its library sorts by date added by: in the
// library's default listing, its listing sorted by date added, its latest
// and on its page. A movie the provider gives no date tells its premiere
// date, and sorts by it; one without a year either has no date.
func TestStoredTitlesTellTheDateTheySortBy(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	token := s.signIn("member", "tv")
	on, off := true, false
	movies := []sortedMovie{{"Alpha", "1700000300", "2001", "7"}, {"Bravo", "1700000100", "2002", "7"}, {"Charlie", "", "2030", "7"},
		{"Delta", "1700000200", "2004", "7"}, {"Echo", "", "", "7"}}
	added := map[string]time.Time{"Charlie": time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}
	for _, m := range movies {
		if seconds, err := strconv.ParseInt(m.added, 10, 64); err == nil {
			added[m.name] = time.Unix(seconds, 0)
		}
	}
	if _, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Box", Account: iptv.Account{Kind: addons.KindXtream,
		Server: sortingProvider(t, movies), Username: "user", Password: "secret"}, Options: &iptv.OptionsPatch{Movies: &on, LiveTv: &off}},
		false); err != nil {
		t.Fatal(err)
	}
	library := s.viewsByType(t, token)["movies"]
	if library == "" {
		t.Fatalf("libraries: %v", s.viewsByType(t, token))
	}
	listing := func(query url.Values) []BaseItemDto {
		t.Helper()
		query.Set("ParentId", library)
		query.Set("IncludeItemTypes", "Movie")
		query.Set("Recursive", "true")
		query.Set("Fields", "DateCreated")
		var page QueryResult
		if status := s.get(t, "/Items?"+query.Encode(), token, &page); status != http.StatusOK {
			t.Fatalf("listing %v: %d", query, status)
		}
		return page.Items
	}
	listed := listing(url.Values{})
	checkAdded(t, "default listing", listed, added)
	sorted := listing(url.Values{"SortBy": {"DateCreated,SortName"}, "SortOrder": {"Descending"}})
	checkAdded(t, "sorted by date added", sorted, added)
	if got, want := itemNames(sorted), []string{"Charlie", "Alpha", "Delta", "Bravo", "Echo"}; !slices.Equal(got, want) {
		t.Errorf("sorted by date added: %v, want %v", got, want)
	}
	var latest []BaseItemDto
	if status := s.get(t, "/Items/Latest?"+url.Values{"ParentId": {library}, "Fields": {"DateCreated"}}.Encode(), token, &latest); status != http.StatusOK {
		t.Fatalf("latest: %d", status)
	}
	checkAdded(t, "latest", latest, added)
	for _, item := range listed {
		var page BaseItemDto
		if status := s.get(t, "/Users/"+member.ID.String()+"/Items/"+item.Id, token, &page); status != http.StatusOK {
			t.Fatalf("%s's page: %d", item.Name, status)
		}
		checkAdded(t, "title page", []BaseItemDto{page}, added)
	}
}

// Each movie of a local folder tells, as DateCreated, the date of its
// file, the one its library sorts by date added by, in its listings and
// on its page.
func TestLocalTitlesTellTheDateTheySortBy(t *testing.T) {
	t.Parallel()
	var folders *localfiles.Service
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, slog.New(slog.NewTextHandler(io.Discard, nil)),
			o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
	})
	member := s.user("member", nil)
	token := s.signIn("member", "tv")
	dir := t.TempDir()
	added := map[string]time.Time{}
	for i, name := range []string{"Alpha", "Bravo", "Charlie"} {
		path := filepath.Join(dir, name+".mkv")
		if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		at := time.Date(2026, 3, 1+i, 12, 0, 0, 0, time.UTC)
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
		added[name] = at
	}
	folder, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Shelf", Path: dir, Kind: localfiles.KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), folder.ID); err != nil {
		t.Fatal(err)
	}
	library := s.viewIDs(t, token)["Shelf"]
	if library == "" {
		t.Fatalf("libraries: %v", s.viewIDs(t, token))
	}
	for _, sortBy := range []string{"", "DateCreated,SortName"} {
		query := url.Values{"ParentId": {library}, "IncludeItemTypes": {"Movie"}, "Recursive": {"true"}, "Fields": {"DateCreated"}}
		if sortBy != "" {
			query.Set("SortBy", sortBy)
		}
		var page QueryResult
		if status := s.get(t, "/Items?"+query.Encode(), token, &page); status != http.StatusOK {
			t.Fatalf("listing: %d", status)
		}
		checkAdded(t, "listing by "+sortBy, page.Items, added)
		if sortBy == "" {
			continue
		}
		if got, want := itemNames(page.Items), []string{"Alpha", "Bravo", "Charlie"}; !slices.Equal(got, want) {
			t.Errorf("sorted by date added: %v, want %v", got, want)
		}
		for _, item := range page.Items {
			var title BaseItemDto
			if status := s.get(t, "/Users/"+member.ID.String()+"/Items/"+item.Id, token, &title); status != http.StatusOK {
				t.Fatalf("%s's page: %d", item.Name, status)
			}
			checkAdded(t, "title page", []BaseItemDto{title}, added)
		}
	}
}

// An IPTV source's series tells the date its provider gives as its
// DateCreated, in its library and on its page, and its seasons and
// episodes carry it, as a series added at once was added with them.
func TestStoredSeriesShareTheirDateAdded(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if r.URL.Path != "/player_api.php" || query.Get("username") != "user" || query.Get("password") != "secret" {
			http.NotFound(w, r)
			return
		}
		var answer any = []any{}
		switch query.Get("action") {
		case "":
			answer = map[string]any{"user_info": map[string]any{"auth": 1, "allowed_output_formats": []string{"ts"}}}
		case "get_series_categories":
			answer = []any{map[string]any{"category_id": "2", "category_name": "Drama"}}
		case "get_series":
			answer = []any{map[string]any{"name": "Quill Days", "series_id": 7, "category_id": "2", "added": "1700000000", "last_modified": "1"}}
		case "get_series_info":
			answer = map[string]any{"info": map[string]any{"plot": "A detailed show."}, "episodes": map[string]any{
				"1": []any{map[string]any{"id": "701", "episode_num": 1, "season": 1, "title": "Quill Days - S01E01 - Pilot", "container_extension": "mkv"}},
				"2": []any{map[string]any{"id": "711", "episode_num": 1, "season": 2, "title": "Quill Days - S02E01 - Back", "container_extension": "mkv"}},
			}}
		}
		_ = json.NewEncoder(w).Encode(answer)
	}))
	t.Cleanup(server.Close)
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	token := s.signIn("member", "tv")
	on, off := true, false
	if _, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Box", Account: iptv.Account{Kind: addons.KindXtream,
		Server: server.URL, Username: "user", Password: "secret"}, Options: &iptv.OptionsPatch{Series: &on, Movies: &off, LiveTv: &off}},
		false); err != nil {
		t.Fatal(err)
	}
	want := time.Unix(1700000000, 0)
	check := func(where string, items []BaseItemDto) {
		t.Helper()
		if len(items) == 0 {
			t.Errorf("%s: none", where)
		}
		for _, item := range items {
			if item.DateCreated == nil || !time.Time(*item.DateCreated).Equal(want) {
				t.Errorf("%s: %s added %v, want %v", where, item.Name, item.DateCreated, want)
			}
		}
	}
	var shows QueryResult
	s.get(t, "/Items?Fields=DateCreated&ParentId="+s.viewsByType(t, token)["tvshows"], token, &shows)
	check("library", shows.Items)
	if len(shows.Items) != 1 {
		t.Fatalf("series: %v", itemNames(shows.Items))
	}
	series := shows.Items[0].Id
	var page BaseItemDto
	s.get(t, "/Users/"+member.ID.String()+"/Items/"+series, token, &page)
	check("series page", []BaseItemDto{page})
	var seasons, episodes QueryResult
	s.get(t, "/Shows/"+series+"/Seasons?Fields=DateCreated&userId="+member.ID.String(), token, &seasons)
	check("seasons", seasons.Items)
	s.get(t, "/Shows/"+series+"/Episodes?Fields=DateCreated&userId="+member.ID.String(), token, &episodes)
	check("episodes", episodes.Items)
	for _, episode := range episodes.Items {
		s.get(t, "/Users/"+member.ID.String()+"/Items/"+episode.Id, token, &page)
		check("episode page", []BaseItemDto{page})
	}
}

// An addon's library sorted by date added shows, as each title's
// DateCreated, when Polyfin first saw it in the catalog, the date it
// sorts by, rather than its premiere date.
func TestAddonTitlesTellTheDateTheySortBy(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("member", nil)
	held := make(chan struct{})
	close(held)
	manifest, _ := ratedAddon(t, held)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), manifest, false); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	path := "/Items?" + url.Values{"ParentId": {s.viewIDs(t, token)["Rated"]}, "IncludeItemTypes": {"Movie"}, "Recursive": {"true"},
		"SortBy": {"DateCreated,SortName"}, "SortOrder": {"Descending"}, "Limit": {"100"}, "Fields": {"DateCreated"}}.Encode()
	var page QueryResult
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", `MediaBrowser Token="`+token+`"`)
		recorder := httptest.NewRecorder()
		s.handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || json.Unmarshal(recorder.Body.Bytes(), &page) != nil {
			t.Fatalf("listing: %d", recorder.Code)
		}
		if recorder.Header().Get(partialTotalHeader) != "true" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the catalog was never read through")
		}
	}
	rows, err := s.pool.Query(t.Context(), "SELECT item_id, first_seen FROM catalog_arrivals")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[accounts.ID]time.Time{}
	var id accounts.ID
	var first time.Time
	if _, err := pgx.ForEachRow(rows, []any{&id, &first}, func() error {
		seen[id] = first
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 100 {
		t.Fatalf("%d titles", len(page.Items))
	}
	for i, item := range page.Items {
		want, ok := seen[mustID(t, item.Id)]
		if !ok || item.DateCreated == nil || !time.Time(*item.DateCreated).Equal(want) {
			t.Errorf("%s added %v, first seen %v", item.Name, item.DateCreated, want)
			continue
		}
		if i > 0 && want.After(time.Time(*page.Items[i-1].DateCreated)) {
			t.Errorf("%s, first seen %v, after %s", item.Name, want, page.Items[i-1].Name)
		}
	}
}
