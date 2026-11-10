package silo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/boxset"
	"github.com/moodiness/polyfin/internal/silo/silotest"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// shelfMovie is a movie of the test library: its TMDB and IMDb
// identifiers, and whether the user played it.
type shelfMovie struct {
	id, name   string
	year       int
	tmdb, imdb string
	played     bool
}

func (m shelfMovie) item() map[string]any {
	added := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	i := map[string]any{"Id": m.id, "Name": m.name, "Type": "Movie", "ProviderIds": map[string]string{},
		"ImageTags": map[string]string{"Primary": "poster-" + m.id}, "DateCreated": added,
		"UserData": map[string]any{"Played": m.played}}
	if m.year > 0 {
		i["ProductionYear"] = m.year
		i["PremiereDate"] = time.Date(m.year, 6, 1, 0, 0, 0, 0, time.UTC)
	}
	if m.tmdb != "" {
		i["ProviderIds"].(map[string]string)["Tmdb"] = m.tmdb
	}
	if m.imdb != "" {
		i["ProviderIds"].(map[string]string)["Imdb"] = m.imdb
	}
	return i
}

func shelfID(n int) string { return fmt.Sprintf("%032x", 0x5000+n) }

// shelf is the test library, in its order: three movies of the Alpha
// collection, listed out of their release order, one of them played;
// Gamma, alone of its collection, known by its IMDb identifier only; two
// movies of a collection whose name does not say so; and movies in none.
var shelf = []shelfMovie{
	{id: shelfID(1), name: "Alpha Two", year: 2004, tmdb: "103"},
	{id: shelfID(2), name: "Beta", year: 2003, tmdb: "102"},
	{id: shelfID(3), name: "Alpha One", year: 2001, tmdb: "101"},
	{id: shelfID(4), name: "Gamma", year: 2005, imdb: "tt0000104"},
	{id: shelfID(5), name: "Alpha Three", year: 2008, tmdb: "105", played: true},
	{id: shelfID(6), name: "Delta", year: 2010, tmdb: "106"},
	{id: shelfID(7), name: "Epsilon Rises", year: 2012, tmdb: "107"},
	{id: shelfID(8), name: "Epsilon Falls", year: 2014, tmdb: "108"},
}

// shelfCollections are the movies' collections, as TMDB names them, by
// their TMDB identifier: 106 is in none, 102 is unknown to TMDB.
var shelfCollections = map[string]map[string]any{
	"101": {"id": 10, "name": "Alpha Collection", "poster_path": "/alpha.jpg", "backdrop_path": "/alpha-wide.jpg"},
	"103": {"id": 10, "name": "Alpha Collection", "poster_path": "/alpha.jpg", "backdrop_path": "/alpha-wide.jpg"},
	"105": {"id": 10, "name": "Alpha Collection", "poster_path": "/alpha.jpg", "backdrop_path": "/alpha-wide.jpg"},
	"104": {"id": 20, "name": "Gamma Collection", "poster_path": "/gamma.jpg"},
	"106": nil,
	"107": {"id": 30, "name": "The Saga"},
	"108": {"id": 30, "name": "The Saga"},
}

const shelfShows = "7777777777777777777777777777777a"

// shelfLibrary answers the Jellyfin API's calls for a library of movies
// (testLibrary) holding movies, a page at a time, and a library of series.
func shelfLibrary(movies []shelfMovie) func(w http.ResponseWriter, c call) {
	return shelfLibraryOf(func() []shelfMovie { return movies })
}

// shelfLibraryOf is shelfLibrary of the movies holds lists at each call.
func shelfLibraryOf(holds func() []shelfMovie) func(w http.ResponseWriter, c call) {
	return func(w http.ResponseWriter, c call) {
		movies := holds()
		switch {
		case c.path == "/UserViews":
			jsonAnswer(w, map[string]any{"Items": []any{map[string]any{"Id": testLibrary, "Name": "Films", "CollectionType": "movies"},
				map[string]any{"Id": shelfShows, "Name": "Shows", "CollectionType": "tvshows"}}})
		case c.path == "/Items" && c.query.Get("ParentId") == testLibrary:
			types := strings.Split(c.query.Get("IncludeItemTypes"), ",")
			var listed []any
			for _, m := range movies {
				if c.query.Get("IncludeItemTypes") == "" || slices.Contains(types, "Movie") {
					listed = append(listed, m.item())
				}
			}
			start, _ := strconv.Atoi(c.query.Get("StartIndex"))
			limit, err := strconv.Atoi(c.query.Get("Limit"))
			if err != nil {
				limit = len(listed)
			}
			start = min(start, len(listed))
			jsonAnswer(w, map[string]any{"Items": listed[start:min(start+limit, len(listed))], "TotalRecordCount": len(listed)})
		case c.path == "/Items":
			jsonAnswer(w, map[string]any{"Items": []any{}, "TotalRecordCount": 0})
		case strings.HasPrefix(c.path, "/Items/"):
			for _, m := range movies {
				if c.path == "/Items/"+m.id {
					jsonAnswer(w, m.item())
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

// fakeCollections answers TMDB's details of the movies of collections,
// and the find of 104 by its IMDb identifier.
type fakeCollections struct {
	*httptest.Server
	mu    sync.Mutex
	asked int
}

func newFakeCollections(t *testing.T) *fakeCollections {
	t.Helper()
	f := &fakeCollections{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.asked++
		f.mu.Unlock()
		if r.URL.Path == "/find/tt0000104" {
			_ = json.NewEncoder(w).Encode(map[string]any{"movie_results": []any{map[string]any{"id": 104}}})
			return
		}
		collection, ok := shelfCollections[strings.TrimPrefix(r.URL.Path, "/movie/")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"belongs_to_collection": collection})
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeCollections) requests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.asked
}

// boxSetHarness is a harness whose API knows box sets from a fake TMDB.
// The lookups read its clock in the background.
type boxSetHarness struct {
	*harness
	store *boxset.Store
	mu    sync.Mutex
}

// newBoxSetHarness is newHarness with box sets known from tmdbFake, with
// the server's TMDB key or without.
func newBoxSetHarness(t *testing.T, answer func(w http.ResponseWriter, c call), tmdbFake *fakeCollections, key string) *boxSetHarness {
	t.Helper()
	user := accounts.User{ID: accounts.ID{1}, Name: "member", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), RemoteAccess: true}
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.Local)
	h := &boxSetHarness{harness: &harness{t: t, accounts: &fakeAccounts{user: user, tokens: map[string]bool{testToken: true},
		device: accounts.Device{ID: accounts.ID{2}, UserID: user.ID, DeviceInfo: accounts.DeviceInfo{DeviceID: "pixel", DeviceName: "Pixel",
			Client: "Silo", ClientVersion: "1"}}},
		jellyfin: &fakeJellyfin{answer: answer}, profile: user.ID.String(), log: &bytes.Buffer{}, clock: &now}}
	db := migrated.New(t)
	client := tmdb.New(tmdbFake.URL, tmdbFake.URL+"/img/", "test", func() accounts.Settings { return accounts.Settings{TMDBKey: key} })
	h.store = boxset.New(db, client, boxset.Options{Interval: time.Millisecond, Now: func() time.Time {
		h.mu.Lock()
		defer h.mu.Unlock()
		return *h.clock
	}})
	t.Cleanup(h.store.Close)
	h.server = New(Options{ServerID: "server", Version: "test", Accounts: h.accounts, Jellyfin: h.jellyfin, DB: db, BoxSets: h.store,
		Logger: slog.New(slog.NewTextHandler(h.log, nil)), now: func() time.Time {
			h.mu.Lock()
			defer h.mu.Unlock()
			return *h.clock
		}})
	return h
}

// settled waits for the box sets' lookups.
func (h *boxSetHarness) settled() {
	h.t.Helper()
	if err := h.store.Settled(h.t.Context()); err != nil {
		h.t.Fatal(err)
	}
}

// filmsNumber is the number the apps know the library of movies by.
func (h *harness) filmsNumber() string {
	h.t.Helper()
	number, err := h.server.numbers.number(h.t.Context(), accounts.ID(mustID(h.t, testLibrary)))
	if err != nil {
		h.t.Fatal(err)
	}
	return number
}

func mustID(t *testing.T, text string) accounts.ID {
	t.Helper()
	id, err := accounts.ParseID(text)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// catalogCard is a card of a catalog page, as the tests read it.
type catalogCard struct {
	ContentID string `json:"content_id"`
	Title     string `json:"title"`
}

type readPage struct {
	Items []catalogCard `json:"items"`
	Page  struct {
		HasMore    bool   `json:"has_more"`
		NextCursor string `json:"next_cursor"`
	} `json:"page"`
	Total int `json:"total"`
}

// page reads a catalog page, as the apps check it.
func (h *harness) page(method, path string, body any) readPage {
	h.t.Helper()
	w := h.do(method, path, body)
	var p readPage
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &p) != nil {
		h.t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body)
	}
	if err := silotest.CheckCatalogPage(w.Body.Bytes()); err != nil {
		h.t.Errorf("%s %s: %v", method, path, err)
	}
	return p
}

func (p readPage) ids() []string {
	var ids []string
	for _, c := range p.Items {
		ids = append(ids, c.ContentID)
	}
	return ids
}

// A library of movies has box sets while the server's TMDB key is saved,
// and the search capabilities say so. A box set search asks TMDB for the
// collections of the libraries' movies, in the background: once they are
// known, it finds the box sets, each of the movies of a collection the
// library shows the user two or more of, titled after it, "Collection"
// read "Box Set", with its TMDB artwork, its movies, how many the user
// played, and the years they span.
func TestLibrariesOfMoviesHaveBoxSets(t *testing.T) {
	t.Parallel()
	tmdbFake := newFakeCollections(t)
	h := newBoxSetHarness(t, shelfLibrary(shelf), tmdbFake, "k3y")
	w := h.do(http.MethodGet, "/api/v2/user/libraries", nil)
	var libraries struct {
		Items []map[string]any `json:"items"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &libraries) != nil || len(libraries.Items) != 2 ||
		libraries.Items[0]["box_sets_enabled"] != true || libraries.Items[1]["box_sets_enabled"] != false {
		t.Fatalf("libraries: %d %s", w.Code, w.Body)
	}
	if err := silotest.CheckLibraries(w.Body.Bytes()); err != nil {
		t.Error(err)
	}
	var capabilities map[string]any
	if w := h.do(http.MethodGet, "/api/v2/catalog/search/capabilities", nil); json.Unmarshal(w.Body.Bytes(), &capabilities) != nil ||
		capabilities["box_sets"] != true {
		t.Errorf("search capabilities: %s", w.Body)
	}
	films := h.filmsNumber()
	search := func(q string) []map[string]any {
		t.Helper()
		w := h.do(http.MethodGet, "/api/v2/catalog/box-sets?poster_width=300&q="+url.QueryEscape(q), nil)
		var found struct {
			Items []map[string]any `json:"items"`
		}
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &found) != nil {
			t.Fatalf("search %q: %d %s", q, w.Code, w.Body)
		}
		if err := silotest.CheckBoxSetSearch(w.Body.Bytes(), 5); err != nil {
			t.Errorf("search %q: %v", q, err)
		}
		return found.Items
	}
	if found := search("alpha"); len(found) != 0 {
		t.Fatalf("before the collections are known: %v", found)
	}
	h.settled()
	found := search("alpha")
	want := map[string]any{"id": "10", "library_id": films, "title": "Alpha Box Set", "movie_count": 3.0, "played_count": 1.0,
		"first_year": 2001.0, "last_year": 2008.0, "poster_url": "/api/v2/artwork/box-sets/10/Primary?tag=alpha.jpg&width=300",
		"backdrop_url": "/api/v2/artwork/box-sets/10/Backdrop?tag=alpha-wide.jpg"}
	if len(found) != 1 || !mapsEqual(found[0], want) {
		t.Errorf("the Alpha box set: %v", found)
	}
	saga := search("saga")
	if len(saga) != 1 || saga[0]["title"] != "The Saga" || saga[0]["movie_count"] != 2.0 ||
		saga[0]["poster_url"] != "/api/v2/artwork/"+shelfID(7)+"/Primary?tag=poster-"+shelfID(7)+"&width=300" {
		t.Errorf("the Saga box set, without artwork of its own: %v", saga)
	}
	// The collection's artwork is the Jellyfin API's, which relays TMDB's:
	// the apps never reach TMDB.
	h.do(http.MethodGet, "/api/v2/artwork/box-sets/10/Primary?tag=alpha.jpg&width=300", nil, "Authorization", "")
	if calls := h.jellyfin.called(http.MethodGet, "/Polyfin/BoxSets/10/Images/Primary"); len(calls) != 1 || calls[0].query.Get("maxWidth") != "300" {
		t.Errorf("the poster's relay: %+v", calls)
	}
}

// A movie's first page waits for the lookup of its own collection, so
// that it shows its box set when the box set's other movies are known.
func TestAMoviesFirstPageShowsItsBoxSet(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	movies := slices.DeleteFunc(slices.Clone(shelf), func(m shelfMovie) bool { return m.id == shelfID(1) })
	tmdbFake := newFakeCollections(t)
	h := newBoxSetHarness(t, shelfLibraryOf(func() []shelfMovie {
		mu.Lock()
		defer mu.Unlock()
		return movies
	}), tmdbFake, "k3y")
	h.do(http.MethodGet, "/api/v2/catalog/box-sets?q=alpha", nil)
	h.settled()
	// Alpha Two joins the library, not looked up yet.
	mu.Lock()
	movies = slices.Clone(shelf)
	mu.Unlock()
	w := h.do(http.MethodGet, "/api/v2/catalog/items/"+shelfID(1)+"/box-set", nil)
	known, err := silotest.CheckItemBoxSet(w.Body.Bytes(), shelfID(1))
	if w.Code != http.StatusOK || !known || err != nil {
		t.Errorf("its first page: %d %s %v", w.Code, w.Body, err)
	}
}

// An app older than silo-apple 9723df00 and silo-android 6f3e3682 asks a
// library with box sets for its grid grouped by box set, as Silo's apps
// did before Silo stopped grouping
// them: the Apple apps in the query, the Android apps in the body. Polyfin
// answers as without the group, the movies one by one, page after page,
// once their collections are known too.
func TestOlderAppsAskingGridsByBoxSetGetTheirMovies(t *testing.T) {
	t.Parallel()
	tmdbFake := newFakeCollections(t)
	h := newBoxSetHarness(t, shelfLibrary(shelf), tmdbFake, "k3y")
	films := h.filmsNumber()
	h.do(http.MethodGet, "/api/v2/catalog/box-sets?q=alpha", nil)
	h.settled()
	browse := "/api/v2/catalog?source=query&library_id=" + films + "&limit=3"
	for _, page := range []string{"", "&cursor=" + url.QueryEscape(cursor(3))} {
		g, p := h.do(http.MethodGet, browse+"&group=box_set"+page, nil), h.do(http.MethodGet, browse+page, nil)
		if g.Code != http.StatusOK || p.Code != http.StatusOK {
			t.Fatalf("the page from %q: %d %s, %d %s", page, g.Code, g.Body, p.Code, p.Body)
		}
		if err := silotest.CheckBrowseUngrouped(g.Body.Bytes(), p.Body.Bytes()); err != nil {
			t.Errorf("the page from %q: %v", page, err)
		}
	}
	if got := h.page(http.MethodGet, browse+"&group=box_set", nil).ids(); !slices.Equal(got, []string{shelfID(1), shelfID(2), shelfID(3)}) {
		t.Errorf("the first page asked by box set: %v", got)
	}
	body := map[string]any{"source": "query", "library_id": films, "limit": 3,
		"groups": []any{map[string]any{"match": "all", "rules": []any{map[string]any{"field": "year", "op": "between", "value": []int{2000, 2005}}}}}}
	grouped := maps.Clone(body)
	grouped["group"] = "box_set"
	g, p := h.do(http.MethodPost, "/api/v2/catalog/query", grouped), h.do(http.MethodPost, "/api/v2/catalog/query", body)
	if g.Code != http.StatusOK || p.Code != http.StatusOK {
		t.Fatalf("the Android way: %d %s, %d %s", g.Code, g.Body, p.Code, p.Body)
	}
	if err := silotest.CheckBrowseUngrouped(g.Body.Bytes(), p.Body.Bytes()); err != nil {
		t.Errorf("the Android way: %v", err)
	}
}

func mapsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

// A box set opens on its movies in release order, in the library asked;
// one a library shows a single movie of is none. A movie's page reads its
// box set, and a movie in none reads none. A search finds the box sets
// whose name holds its words, "Box Set" for "Collection" included, then
// those two of whose movies' titles do.
func TestBoxSetsOpenAndAreFound(t *testing.T) {
	t.Parallel()
	tmdbFake := newFakeCollections(t)
	h := newBoxSetHarness(t, shelfLibrary(shelf), tmdbFake, "k3y")
	films := h.filmsNumber()
	h.do(http.MethodGet, "/api/v2/catalog/box-sets?q=alpha", nil)
	h.settled()

	w := h.do(http.MethodGet, "/api/v2/catalog/box-sets/10?library_id="+films, nil)
	var detail struct {
		Title string        `json:"title"`
		Items []catalogCard `json:"items"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &detail) != nil {
		t.Fatalf("box set: %d %s", w.Code, w.Body)
	}
	if err := silotest.CheckBoxSet(w.Body.Bytes(), "10", films); err != nil {
		t.Error(err)
	}
	if got := (readPage{Items: detail.Items}).ids(); detail.Title != "Alpha Box Set" || !slices.Equal(got, []string{shelfID(3), shelfID(1), shelfID(5)}) {
		t.Errorf("box set: %s %v", detail.Title, got)
	}
	for path, status := range map[string]int{
		"/api/v2/catalog/box-sets/20?library_id=" + films: http.StatusNotFound,
		"/api/v2/catalog/box-sets/10?library_id=999":      http.StatusNotFound,
		"/api/v2/catalog/box-sets/10":                     http.StatusUnprocessableEntity,
		"/api/v2/catalog/box-sets?q=":                     http.StatusUnprocessableEntity,
		"/api/v2/catalog/box-sets?q=alpha&limit=21":       http.StatusUnprocessableEntity,
	} {
		if w := h.do(http.MethodGet, path, nil); w.Code != status {
			t.Errorf("%s: %d %s", path, w.Code, w.Body)
		}
	}

	w = h.do(http.MethodGet, "/api/v2/catalog/items/"+shelfID(1)+"/box-set", nil)
	if known, err := silotest.CheckItemBoxSet(w.Body.Bytes(), shelfID(1)); w.Code != http.StatusOK || !known || err != nil {
		t.Errorf("a movie's box set: %d %s %v", w.Code, w.Body, err)
	}
	for _, id := range []string{shelfID(2), shelfID(4)} {
		w := h.do(http.MethodGet, "/api/v2/catalog/items/"+id+"/box-set?library_id="+films, nil)
		if known, err := silotest.CheckItemBoxSet(w.Body.Bytes(), id); w.Code != http.StatusOK || known || err != nil {
			t.Errorf("the box set of %s: %d %s %v", id, w.Code, w.Body, err)
		}
	}

	for q, want := range map[string][]string{
		"alpha":                {"10"},
		"ALPHA box set":        {"10"},
		"alpha collection":     {"10"},
		"box":                  {},
		"epsilon":              {"30"},
		"saga":                 {"30"},
		"gamma":                {},
		"epsilon rises":        {},
		"the":                  {"30"},
		strings.Repeat("a", 3): {},
	} {
		w := h.do(http.MethodGet, "/api/v2/catalog/box-sets?q="+url.QueryEscape(q), nil)
		var found struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &found) != nil {
			t.Fatalf("search %q: %d %s", q, w.Code, w.Body)
		}
		if err := silotest.CheckBoxSetSearch(w.Body.Bytes(), 5); err != nil {
			t.Errorf("search %q: %v", q, err)
		}
		var got []string
		for _, f := range found.Items {
			got = append(got, f.ID)
		}
		if !slices.Equal(got, want) && !(len(got) == 0 && len(want) == 0) {
			t.Errorf("search %q: %v, want %v", q, got, want)
		}
	}
}

// Without the server's TMDB key, Polyfin answers as it did without box
// sets: no library has them, the search capabilities say none, a browse
// by box set lists the movies, and the box set reads are not found. TMDB
// is never asked.
func TestWithoutATMDBKeyThereAreNoBoxSets(t *testing.T) {
	t.Parallel()
	tmdbFake := newFakeCollections(t)
	h := newBoxSetHarness(t, shelfLibrary(shelf), tmdbFake, "")
	w := h.do(http.MethodGet, "/api/v2/user/libraries", nil)
	var libraries struct {
		Items []map[string]any `json:"items"`
	}
	if json.Unmarshal(w.Body.Bytes(), &libraries) != nil || len(libraries.Items) != 2 || libraries.Items[0]["box_sets_enabled"] != false {
		t.Errorf("libraries: %s", w.Body)
	}
	var capabilities map[string]any
	if w := h.do(http.MethodGet, "/api/v2/catalog/search/capabilities", nil); json.Unmarshal(w.Body.Bytes(), &capabilities) != nil ||
		capabilities["box_sets"] != false {
		t.Errorf("search capabilities: %s", w.Body)
	}
	films := h.filmsNumber()
	p := h.page(http.MethodGet, "/api/v2/catalog?source=query&library_id="+films+"&group=box_set&limit=3", nil)
	if len(p.Items) != 3 || p.Total != len(shelf) || p.Page.NextCursor != cursor(3) {
		t.Errorf("browse by box set: %+v", p)
	}
	h.settled()
	for _, path := range []string{"/api/v2/catalog/box-sets?q=alpha", "/api/v2/catalog/box-sets/10?library_id=" + films,
		"/api/v2/catalog/items/" + shelfID(1) + "/box-set"} {
		if w := h.do(http.MethodGet, path, nil); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d %s", path, w.Code, w.Body)
		}
	}
	if n := tmdbFake.requests(); n != 0 {
		t.Errorf("TMDB was asked %d times", n)
	}
}
