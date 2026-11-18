package silo

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// genreLibrary answers a library of movies whose every listing holds
// 5,000 titles, and counts the listings asked by genre.
type genreLibrary struct {
	// own gives each genre titles of its own, named after it; otherwise
	// every listing holds the same titles.
	own bool
	// overview is each title's.
	overview string
	// chunk is how many titles a listing answers at most, when not what
	// it is asked.
	chunk int
	// asked, when set, is told each listing by genre as it arrives.
	asked func()

	mu      sync.Mutex
	pages   map[string][]byte
	byGenre map[string]int
}

func (g *genreLibrary) answer(w http.ResponseWriter, c call) {
	switch c.path {
	case "/UserViews":
		jsonAnswer(w, map[string]any{"Items": []any{map[string]any{"Id": testLibrary, "Name": "Films", "CollectionType": "movies"}}})
	case "/Items":
		start, _ := strconv.Atoi(c.query.Get("StartIndex"))
		limit, _ := strconv.Atoi(c.query.Get("Limit"))
		if g.chunk > 0 {
			limit = min(limit, g.chunk)
		}
		genre := strings.ToLower(c.query.Get("Genres"))
		if genre != "" && g.asked != nil {
			g.asked()
		}
		g.mu.Lock()
		if g.byGenre == nil {
			g.byGenre, g.pages = map[string]int{}, map[string][]byte{}
		}
		if genre != "" {
			g.byGenre[genre]++
		}
		shelf := ""
		if g.own {
			shelf = genre
		}
		key := fmt.Sprintf("%s/%d/%d", shelf, start, limit)
		page, ok := g.pages[key]
		if !ok {
			hash := fnv.New32a()
			_, _ = hash.Write([]byte(shelf))
			items := []any{}
			for n := start; n < min(start+limit, 5000); n++ {
				items = append(items, map[string]any{"Id": fmt.Sprintf("%08x%024x", hash.Sum32(), n+1), "Name": strings.TrimSpace(shelf + " " + strconv.Itoa(n+1)),
					"Type": "Movie", "Overview": g.overview})
			}
			page, _ = json.Marshal(map[string]any{"Items": items, "TotalRecordCount": 5000})
			g.pages[key] = page
		}
		g.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(page)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// listings are the listings by genre asked since the last call, in all.
func (g *genreLibrary) listings() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	asked := 0
	for _, count := range g.byGenre {
		asked += count
	}
	g.byGenre = map[string]int{}
	return asked
}

// offering makes a harness's library offer genres.
func (h *harness) offering(genres ...string) {
	h.server.options.Genres = func(context.Context, accounts.User, accounts.ID) ([]string, error) { return genres, nil }
}

// anyGenre is a filter any of whose genres a title may have.
func anyGenre(genres ...string) []any {
	rules := make([]any, len(genres))
	for n, genre := range genres {
		rules[n] = map[string]any{"field": "genre", "op": "is", "value": genre}
	}
	return []any{map[string]any{"match": "any", "rules": rules}}
}

// numbered are count names made of a word and a number.
func numbered(word string, count int) []string {
	names := make([]string, count)
	for n := range names {
		names[n] = word + " " + strconv.Itoa(n)
	}
	return names
}

// request is a request of the API as an app sends it, under ctx.
func (h *harness) request(ctx context.Context, method, path string, body any) *http.Request {
	data, _ := json.Marshal(body)
	r := httptest.NewRequestWithContext(ctx, method, path, strings.NewReader(string(data)))
	r.Header.Set("Authorization", "Bearer "+testToken)
	r.Header.Set("X-Profile-Id", h.profile)
	r.Header.Set("Content-Type", "application/json")
	return r
}

// A genre a filter names again and again, in any letter case, is read as
// one listing: one for a first page, and for a page sought far those that
// maxScanned titles take, whether the filter comes in a body or in the
// address.
func TestAGenreNamedAgainIsOneListing(t *testing.T) {
	t.Parallel()
	library := &genreLibrary{}
	h := newHarness(t, library.answer)
	h.offering("Action")
	names := make([]string, maxRules)
	for n := range names {
		names[n] = []string{"Action", "action", " ACTION "}[n%3]
	}
	groups := anyGenre(names...)
	body := map[string]any{"source": "query", "library_id": h.filmsNumber(), "limit": 1, "groups": groups}
	if p := h.page(http.MethodPost, "/api/v2/catalog/query", body); len(p.Items) != 1 {
		t.Errorf("the first page: %+v", p)
	}
	if asked := library.listings(); asked != 1 {
		t.Errorf("%d listings by genre for the first page, want 1", asked)
	}
	body["seek"] = maxSeek
	if p := h.page(http.MethodPost, "/api/v2/catalog/query", body); len(p.Items) != 0 || p.Total != maxScanned {
		t.Errorf("a page sought far: %d titles of %d", len(p.Items), p.Total)
	}
	if asked := library.listings(); asked != maxScanned/scanChunk {
		t.Errorf("%d listings by genre for a page sought far, want %d", asked, maxScanned/scanChunk)
	}
	encoded, _ := json.Marshal(groups)
	q := url.Values{"source": {"query"}, "library_id": {h.filmsNumber()}, "limit": {"1"}, "groups": {string(encoded)}}
	if len(encoded) > maxGroupsLength {
		t.Fatalf("the filter is %d characters", len(encoded))
	}
	h.page(http.MethodGet, "/api/v2/catalog?"+q.Encode(), nil)
	if asked := library.listings(); asked != 1 {
		t.Errorf("%d listings by genre for the filter in the address, want 1", asked)
	}
}

// The genres a filter names are still read together, one title of each
// in turn, each genre as one listing however often it is named.
func TestGenresNamedTogetherAreReadInTurn(t *testing.T) {
	t.Parallel()
	library := &genreLibrary{own: true}
	h := newHarness(t, library.answer)
	h.offering("Action", "Drama")
	titles := func(genres ...string) []string {
		t.Helper()
		p := h.page(http.MethodPost, "/api/v2/catalog/query", map[string]any{"source": "query", "library_id": h.filmsNumber(), "limit": 5,
			"groups": anyGenre(genres...)})
		var titles []string
		for _, card := range p.Items {
			titles = append(titles, card.Title)
		}
		return titles
	}
	want := []string{"action 1", "drama 1", "action 2", "drama 2", "action 3"}
	if got := titles("Action", "Drama"); !slices.Equal(got, want) {
		t.Errorf("two genres: %q, want %q", got, want)
	}
	library.listings()
	if got := titles("Action", "Drama", "action", "DRAMA", "Western", "Action"); !slices.Equal(got, want) {
		t.Errorf("two genres named again: %q, want %q", got, want)
	}
	if asked := library.listings(); asked != 2 {
		t.Errorf("%d listings by genre for two genres named again, want 2", asked)
	}
}

// A page asks the Jellyfin API at most maxListings listings, however many
// genres a catalog offers and a filter names, and however far it is
// sought: a filter of every genre of a catalog offering 512, all of the
// same titles, is cut there, and says its total is not exact.
func TestAPageAsksABoundedNumberOfListings(t *testing.T) {
	t.Parallel()
	library := &genreLibrary{}
	h := newHarness(t, library.answer)
	genres := numbered("Genre", maxRules)
	h.offering(genres...)
	body := map[string]any{"source": "query", "library_id": h.filmsNumber(), "limit": 1, "groups": anyGenre(genres...)}
	for _, tc := range []struct {
		name string
		seek any
	}{{"the first page", nil}, {"a page sought far", maxSeek}} {
		if tc.seek != nil {
			body["seek"] = tc.seek
		}
		w := h.do(http.MethodPost, "/api/v2/catalog/query", body)
		var page struct {
			Total      int   `json:"total"`
			TotalExact *bool `json:"total_exact"`
		}
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &page) != nil || page.TotalExact == nil || *page.TotalExact || page.Total == 0 {
			t.Errorf("%s: %d %.200s", tc.name, w.Code, w.Body)
		}
		if asked := library.listings(); asked != maxListings {
			t.Errorf("%s: %d listings by genre, want %d", tc.name, asked, maxListings)
		}
	}
}

// A page reads listings while their answers weigh less than
// maxListedBytes in all: titles of a mebibyte each are read up to it.
func TestAPageReadsABoundedWeightOfListings(t *testing.T) {
	t.Parallel()
	library := &genreLibrary{overview: strings.Repeat("a", 1<<20), chunk: 1}
	h := newHarness(t, library.answer)
	genres := numbered("Genre", maxRules)
	h.offering(genres...)
	w := h.do(http.MethodPost, "/api/v2/catalog/query", map[string]any{"source": "query", "library_id": h.filmsNumber(), "limit": 1,
		"groups": anyGenre(genres...)})
	if w.Code != http.StatusOK {
		t.Fatalf("the query: %d %.200s", w.Code, w.Body)
	}
	library.mu.Lock()
	weight := len(library.pages["/0/1"])
	library.mu.Unlock()
	if weight < 1<<20 {
		t.Fatalf("a listing weighs %d bytes", weight)
	}
	if asked, want := library.listings(), (maxListedBytes+weight-1)/weight; asked != want {
		t.Errorf("%d listings of %d bytes each, want %d", asked, weight, want)
	}
}

// Once the request's context is done, as when the app closed its
// connection, a filtered page asks no further listing.
func TestAFilteredPageStopsWithItsRequest(t *testing.T) {
	t.Parallel()
	ctx, leave := context.WithCancel(t.Context())
	defer leave()
	library := &genreLibrary{own: true, asked: leave}
	h := newHarness(t, library.answer)
	h.offering("Action", "Drama", "Horror")
	h.server.ServeHTTP(httptest.NewRecorder(), h.request(ctx, http.MethodPost, "/api/v2/catalog/query", map[string]any{"source": "query",
		"library_id": h.filmsNumber(), "limit": 10, "groups": anyGenre("Action", "Drama", "Horror")}))
	if asked := library.listings(); asked != 1 {
		t.Errorf("%d listings by genre, want the one the request ended at", asked)
	}
	if strings.Contains(h.log.String(), "level=ERROR") {
		t.Errorf("logged: %s", h.log)
	}
}

// The titles box sets are made of are read while the request's context is
// not done.
func TestTheWindowOfALibraryStopsWithItsRequest(t *testing.T) {
	t.Parallel()
	ctx, leave := context.WithCancel(t.Context())
	defer leave()
	library := &genreLibrary{asked: leave}
	h := newHarness(t, library.answer)
	titles, _, err := h.server.window(h.request(ctx, http.MethodGet, "/api/v2/catalog/box-sets", nil), viewer{}, url.Values{"Genres": {"Action"}})
	if err == nil || len(titles) != 0 {
		t.Errorf("%d titles read, error %v", len(titles), err)
	}
	if asked := library.listings(); asked != 1 {
		t.Errorf("%d listings, want the one the request ended at", asked)
	}
}
