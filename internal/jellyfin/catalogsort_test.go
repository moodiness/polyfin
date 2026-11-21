package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// ratedAddon serves one movie catalog of 100 titles in pages of 25 read
// with skip: the catalog lists them by rank, its names run backwards from
// Movie 100, its ratings and release dates follow no order. Pages past the
// first wait for held to close.
func ratedAddon(t *testing.T, held chan struct{}) (string, []stremio.Meta) {
	t.Helper()
	var movies []stremio.Meta
	for i := range 100 {
		year := 1950 + (i*37)%70
		released := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
		movies = append(movies, stremio.Meta{ID: fmt.Sprintf("tt8%04d", i), Type: "movie", Name: fmt.Sprintf("Movie %03d", 100-i),
			ImdbRating: stremio.Text(strconv.FormatFloat(1+float64((i*53)%90)/10, 'f', 1, 64)), Released: released,
			Year: stremio.Text(strconv.Itoa(year))})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.EscapedPath(), ".json")
		switch {
		case path == "/manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "rated", Name: "Rated", Version: "1", Types: []string{"movie"},
				Resources: []stremio.Resource{{Name: "catalog"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "rated", Name: "Rated", Extra: []stremio.Extra{{Name: "skip"}}}}})
		case strings.HasPrefix(path, "/catalog/movie/rated"):
			skip := 0
			if _, value, ok := strings.Cut(path, "skip="); ok {
				skip, _ = strconv.Atoi(value)
			}
			if skip > 0 {
				select {
				case <-held:
				case <-r.Context().Done():
					return
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": movies[min(skip, len(movies)):min(skip+25, len(movies))]})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json", movies
}

// An addon's library listed by name ascending, as every app asks by
// default, keeps its catalog's order. Sorted another way, it sorts the
// titles read of its catalog while the rest is read in the background:
// those known first, counted as its total and said to be partial, then
// all of them, with when each was first seen as its date added. An
// administrator's edit sorts them again at once.
func TestAddonLibrariesSortTheTitlesRead(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("member", nil)
	s.user("boss", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	held := make(chan struct{})
	manifest, movies := ratedAddon(t, held)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), manifest, false); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	library := s.viewIDs(t, token)["Rated"]
	listing := func(query string) (QueryResult, bool) {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/Items?ParentId="+library+"&IncludeItemTypes=Movie&Recursive=true"+query, nil)
		request.Header.Set("Authorization", `MediaBrowser Token="`+token+`"`)
		recorder := httptest.NewRecorder()
		s.handler.ServeHTTP(recorder, request)
		var page QueryResult
		if recorder.Code != http.StatusOK || json.Unmarshal(recorder.Body.Bytes(), &page) != nil {
			t.Fatalf("%s: %d", query, recorder.Code)
		}
		return page, recorder.Header().Get(partialTotalHeader) == "true"
	}
	names := func(metas []stremio.Meta) []string {
		result := make([]string, len(metas))
		for i, meta := range metas {
			result[i] = meta.Name
		}
		return result
	}

	byName, _ := listing("&SortBy=SortName,ProductionYear&SortOrder=Ascending&Limit=10")
	if got := itemNames(byName.Items); !slices.Equal(got, names(movies[:10])) {
		t.Errorf("by name ascending, the catalog's order: %v", got)
	}
	// The first page alone is known.
	known, partial := listing("&SortBy=SortName&SortOrder=Descending&Limit=10")
	want := names(movies[:25])
	slices.Sort(want)
	slices.Reverse(want)
	if got := itemNames(known.Items); !slices.Equal(got, want[:10]) || known.TotalRecordCount != 25 || !partial {
		t.Errorf("by name descending, the first page read: %v of %d, partial %t", got, known.TotalRecordCount, partial)
	}

	close(held)
	var whole QueryResult
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if whole, partial = listing("&SortBy=SortName&SortOrder=Descending&StartIndex=90&Limit=20"); !partial {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the catalog was never read through")
		}
	}
	if got := itemNames(whole.Items); !slices.Equal(got, []string{"Movie 010", "Movie 009", "Movie 008", "Movie 007", "Movie 006",
		"Movie 005", "Movie 004", "Movie 003", "Movie 002", "Movie 001"}) || whole.TotalRecordCount != 100 {
		t.Errorf("by name descending, the last page: %v of %d", got, whole.TotalRecordCount)
	}

	rated, _ := listing("&SortBy=CommunityRating,SortName&SortOrder=Descending&StartIndex=20&Limit=30")
	if len(rated.Items) != 30 || !slices.IsSortedFunc(rated.Items, func(a, b BaseItemDto) int {
		return -strings.Compare(fmt.Sprintf("%04.1f", *a.CommunityRating), fmt.Sprintf("%04.1f", *b.CommunityRating))
	}) {
		t.Errorf("by rating descending: %d titles, not in order", len(rated.Items))
	}
	years, _ := listing("&SortBy=ProductionYear,SortName&SortOrder=Ascending&Limit=100")
	if !slices.IsSortedFunc(years.Items, func(a, b BaseItemDto) int { return *a.ProductionYear - *b.ProductionYear }) {
		t.Errorf("by year: not in order")
	}

	added, _ := listing("&SortBy=DateCreated,SortName&SortOrder=Descending&Limit=100&Fields=DateCreated")
	if len(added.Items) != 100 {
		t.Fatalf("by date added: %d titles", len(added.Items))
	}
	for i, item := range added.Items {
		if item.DateCreated == nil || time.Time(*item.DateCreated).Year() < 2000 {
			t.Fatalf("%s added %v", item.Name, item.DateCreated)
		}
		if i > 0 && time.Time(*item.DateCreated).After(time.Time(*added.Items[i-1].DateCreated)) {
			t.Errorf("by date added descending: %s after %s", item.Name, added.Items[i-1].Name)
		}
	}

	// Movie 001, last by name, renamed to come first.
	last := whole.Items[len(whole.Items)-1]
	if status, body := s.call(http.MethodPost, "/Items/"+last.Id, app("dashboard", s.signIn("boss", "dashboard")),
		map[string]any{"Name": "Zulu"}); status != http.StatusNoContent {
		t.Fatalf("renaming %s: %d %s", last.Name, status, body)
	}
	renamed, _ := listing("&SortBy=SortName&SortOrder=Descending&Limit=2")
	if got := itemNames(renamed.Items); !slices.Equal(got, []string{"Zulu", "Movie 100"}) {
		t.Errorf("by name descending once %s is renamed: %v", last.Name, got)
	}
}
