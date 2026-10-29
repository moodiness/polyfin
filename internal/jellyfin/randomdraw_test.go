package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// deepAddon serves one movie catalog of 150 titles, Movie 001 to Movie 150,
// in pages of 25 read with skip. Movie 030 is rated R, Movie 040 is a crime
// title; the others are PG dramas.
func deepAddon(t *testing.T) string {
	t.Helper()
	var movies []stremio.Meta
	for i := 1; i <= 150; i++ {
		m := stremio.Meta{ID: fmt.Sprintf("tt7%04d", i), Type: "movie", Name: fmt.Sprintf("Movie %03d", i),
			Genres: []string{"Drama"}, Extras: &stremio.Extras{Certification: "PG"}}
		switch i {
		case 30:
			m.Extras.Certification = "R"
		case 40:
			m.Genres = []string{"Crime"}
		}
		movies = append(movies, m)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.EscapedPath(), ".json")
		switch {
		case path == "/manifest":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "deep", Name: "Deep", Version: "1", Types: []string{"movie"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "deep", Name: "Deep", Extra: []stremio.Extra{{Name: "skip"}}}}})
		case strings.HasPrefix(path, "/catalog/movie/deep"):
			skip := 0
			if _, value, ok := strings.Cut(path, "skip="); ok {
				skip, _ = strconv.Atoi(value)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": movies[min(skip, len(movies)):min(skip+25, len(movies))]})
		case strings.HasPrefix(path, "/meta/movie/"):
			id := path[strings.LastIndex(path, "/")+1:]
			for _, meta := range movies {
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

// A listing of a library's titles at random, as the LumaaGlaass carousel
// and jellyfin-web's shuffle ask for it, draws among the library's first
// 100 titles, not its first page, and says so in its total. Its pages
// follow one order while the draw lasts, each title once; a listing two
// minutes later draws anew. A draw never holds what the user may not see:
// titles above their parental control, of a blocked genre, or that they
// hid, even after the draw.
func TestRandomListingsDrawAmongALibrarysFirstTitles(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	var clock atomic.Pointer[time.Time]
	now := time.Now()
	clock.Store(&now)
	s.handler.now = func() time.Time { return *clock.Load() }
	s.user("member", nil)
	s.user("child", func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{MaxRating: new(13)} })
	s.user("calm", func(c *accounts.UserChanges) { c.BlockedGenres = &[]string{"crime"} })
	if _, err := s.addons.Install(t.Context(), addons.Shared(), deepAddon(t), false); err != nil {
		t.Fatal(err)
	}
	tokens := map[string]string{}
	for _, name := range []string{"member", "child", "calm"} {
		tokens[name] = s.signIn(name, "tv")
	}
	library := s.viewIDs(t, tokens["member"])["Deep"]
	listing := func(name string, query string) QueryResult {
		t.Helper()
		var page QueryResult
		if status := s.get(t, "/Items?ParentId="+library+"&IncludeItemTypes=Movie,Series&Recursive=true"+query, tokens[name], &page); status != http.StatusOK {
			t.Fatalf("%s %s: %d", name, query, status)
		}
		return page
	}
	// Each user's listing reads the library's first 100 titles, as a
	// listing of 100 asks: the draws then find them kept.
	for name := range tokens {
		if page := listing(name, "&Limit=100"); len(page.Items) < 99 {
			t.Fatalf("%s's first 100 titles: %d", name, len(page.Items))
		}
	}
	// drawn pages through the draw of a user ten titles at a time.
	drawn := func(name string) (names []string, total int) {
		t.Helper()
		for start := 0; start < 120; start += 10 {
			page := listing(name, "&SortBy=Random,SortName&StartIndex="+strconv.Itoa(start)+"&Limit=10")
			names = append(names, itemNames(page.Items)...)
			total = page.TotalRecordCount
		}
		return names, total
	}
	var first100 []string
	for i := 1; i <= 100; i++ {
		first100 = append(first100, fmt.Sprintf("Movie %03d", i))
	}

	order, total := drawn("member")
	if sorted := slices.Sorted(slices.Values(order)); total != 100 || !slices.Equal(sorted, first100) {
		t.Fatalf("the member's draw: total %d, %d titles %v", total, len(order), sorted)
	}
	if slices.Equal(order[:25], first100[:25]) {
		t.Errorf("the draw keeps the catalog's order: %v", order[:25])
	}
	if beyond := slices.IndexFunc(order[:30], func(name string) bool { return name > "Movie 025" }); beyond < 0 {
		t.Errorf("the first 30 titles drawn are all on the catalog's first page: %v", order[:30])
	}
	again, _ := drawn("member")
	if !slices.Equal(again, order) {
		t.Errorf("the same draw paged again:\n%v\nwant\n%v", again, order)
	}

	later := now.Add(drawLife + time.Second)
	clock.Store(&later)
	if redrawn, _ := drawn("member"); slices.Equal(redrawn, order) {
		t.Errorf("a listing after %v drew the same order again", drawLife)
	}

	for name, gone := range map[string]string{"child": "Movie 030", "calm": "Movie 040"} {
		titles, total := drawn(name)
		if total != 100 || len(titles) != 100 || slices.Contains(titles, gone) || slices.Contains(titles, "Movie 101") == false {
			t.Errorf("%s's draw: total %d, %d titles, with %s: %v, with Movie 101: %v", name, total, len(titles), gone,
				slices.Contains(titles, gone), slices.Contains(titles, "Movie 101"))
		}
	}

	// A title the member hides leaves the draw they are paging through.
	ids := map[string]string{}
	for _, item := range listing("member", "&Limit=100").Items {
		ids[item.Name] = item.Id
	}
	if status, _ := s.call(http.MethodPost, "/Polyfin/UserItems/"+ids["Movie 050"]+"/Hidden", app("tv", tokens["member"]), nil); status != http.StatusNoContent {
		t.Fatalf("hiding: %d", status)
	}
	if titles, _ := drawn("member"); slices.Contains(titles, "Movie 050") || len(titles) != 99 {
		t.Errorf("after hiding Movie 050: %d titles, with it: %v", len(titles), slices.Contains(titles, "Movie 050"))
	}
}

// A collection library listed at random gives its collections' titles, as
// its other listings of titles do, never its collections.
func TestRandomListingsOfACollectionLibraryDrawItsTitles(t *testing.T) {
	t.Parallel()
	s, token, views := browsing(t)
	var titles, random QueryResult
	s.get(t, "/Items?ParentId="+views["Groups"]+"&IncludeItemTypes=Movie,Series&Recursive=true&Limit=50", token, &titles)
	s.get(t, "/Items?ParentId="+views["Groups"]+"&IncludeItemTypes=Movie,Series&Recursive=true&SortBy=Random&Limit=50", token, &random)
	if len(titles.Items) == 0 || !slices.Equal(slices.Sorted(slices.Values(itemNames(random.Items))), slices.Sorted(slices.Values(itemNames(titles.Items)))) {
		t.Errorf("titles at random %v, want those of %v", itemNames(random.Items), itemNames(titles.Items))
	}
	for _, item := range random.Items {
		if item.Type == "BoxSet" {
			t.Errorf("a collection among the titles at random: %s", item.Name)
		}
	}
}
