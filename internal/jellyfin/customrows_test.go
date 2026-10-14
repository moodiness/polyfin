package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/homerows"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// customRowsOf answers GET /Polyfin/HomeRows for token.
func (s testServer) customRowsOf(t *testing.T, token string) []CustomRowDto {
	t.Helper()
	var answer struct{ Items []CustomRowDto }
	if status := s.get(t, "/Polyfin/HomeRows", token, &answer); status != http.StatusOK || answer.Items == nil {
		t.Fatalf("home rows: %d %+v", status, answer)
	}
	return answer.Items
}

// addRow adds a home row.
func (s testServer) addRow(t *testing.T, row homerows.Row) homerows.Row {
	t.Helper()
	if row.Count == 0 {
		row.Count = homerows.DefaultCount
	}
	created, err := homerows.New(s.pool).Create(t.Context(), row)
	if err != nil {
		t.Fatal(err)
	}
	return created
}

// rowSummary tells a row's name, place and titles.
func rowSummary(rows []CustomRowDto) []string {
	result := []string{}
	for _, row := range rows {
		result = append(result, row.Name+" @"+row.Place+": "+strings.Join(itemNames(row.Items), ", "))
	}
	return result
}

// homeRowsServer installs the catalog addon with its movie catalog alone
// as a library, signs a member and an administrator in, and returns the
// addon's identifier and the movies of the library.
func homeRowsServer(t *testing.T) (s testServer, member, admin string, addon accounts.ID, movies []BaseItemDto) {
	t.Helper()
	s = newTestServer(t, 10)
	s.user("member", nil)
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator, c.CollectionManagement = new(true), new(true) })
	installed, err := s.addons.Install(t.Context(), addons.Shared(), catalogAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: installed.ID, CatalogType: "movie", CatalogID: "top"},
		{AddonID: installed.ID, CatalogType: "collection", CatalogID: "groups"},
	}); err != nil {
		t.Fatal(err)
	}
	member, admin = s.signIn("member", "tv"), s.signIn("admin", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", member, &views)
	for _, view := range views.Items {
		if view.Name == "Top" {
			s.get(t, "/Items?ParentId="+view.Id, member, &page)
		}
	}
	if len(page.Items) != 3 {
		t.Fatalf("movies: %v", itemNames(page.Items))
	}
	return s, member, admin, installed.ID, page.Items
}

// A catalog row, of a catalog that is a library or not, and a collection
// row show their titles at their place, in the order of the list; a row
// without titles is left out. A row's folder describes the row and lists
// its titles, which its name opens in the web player; it is not among the
// user's libraries.
func TestCatalogAndCollectionRowsShowTheirTitlesAtTheirPlace(t *testing.T) {
	t.Parallel()
	s, member, admin, addon, movies := homeRowsServer(t)
	picks, _ := accounts.ParseID(s.createCollection(t, admin, "name=Picks&ids="+movies[1].Id))
	s.addRow(t, homerows.Row{Name: "New", Kind: homerows.KindNew, Place: homerows.PlaceTop, Everyone: true})
	shows := s.addRow(t, homerows.Row{Name: "Series to try", Kind: homerows.KindCatalog, Place: homerows.PlaceNextUp, Addon: addon,
		CatalogType: "series", CatalogID: "shows", Everyone: true})
	s.addRow(t, homerows.Row{Name: "Picks", Kind: homerows.KindCollection, Place: homerows.PlaceResume, Collection: picks, Everyone: true})
	s.addRow(t, homerows.Row{Name: "Top movies", Kind: homerows.KindCatalog, Place: homerows.PlaceBottom, Addon: addon,
		CatalogType: "movie", CatalogID: "top", Everyone: true})
	rows := s.customRowsOf(t, member)
	if got := rowSummary(rows); !slices.Equal(got, []string{
		"Series to try @AfterNextUp: Show",
		"Picks @AfterResume: Movie 1",
		"Top movies @Bottom: Movie 0, Movie 1, Movie 2",
	}) {
		t.Fatalf("rows: %q", got)
	}
	if rows[0].Id != customRowID(shows.ID, accounts.ID{}).String() || rows[0].Items[0].Type != "Series" || rows[0].Items[0].ImageTags["Primary"] == "" {
		t.Errorf("the catalog row: %+v", rows[0])
	}
	var folder BaseItemDto
	if status := s.get(t, "/Items/"+rows[0].Id, member, &folder); status != http.StatusOK || folder.Name != "Series to try" || folder.Type != "CollectionFolder" {
		t.Errorf("the row's folder: %d %+v", status, folder)
	}
	var listed QueryResult
	if status := s.get(t, "/Items?ParentId="+rows[1].Id+"&SortBy=SortName", member, &listed); status != http.StatusOK ||
		!slices.Equal(itemNames(listed.Items), []string{"Movie 1"}) || listed.TotalRecordCount != 1 {
		t.Errorf("the collection row's list: %d %v of %d", status, itemNames(listed.Items), listed.TotalRecordCount)
	}
	var ancestors []BaseItemDto
	if status := s.get(t, "/Items/"+rows[2].Id+"/Ancestors", member, &ancestors); status != http.StatusOK || len(ancestors) != 0 {
		t.Errorf("the row's ancestors: %d %v", status, ancestors)
	}
	var views QueryResult
	s.get(t, "/UserViews", member, &views)
	if got := itemNames(views.Items); slices.Contains(got, "Series to try") || slices.Contains(got, "Picks") {
		t.Errorf("rows among the libraries: %v", got)
	}
	// Another user sees them too; an API key, whose requests have no user,
	// sees none.
	if got := rowSummary(s.customRowsOf(t, admin)); len(got) != 3 {
		t.Errorf("the administrator's rows: %q", got)
	}
}

// A row also a library in other apps is a view of /UserViews, a library
// without type whose latest items are its first titles, and the web
// player's menus list it as they list the others.
func TestARowAlsoALibraryIsAView(t *testing.T) {
	t.Parallel()
	s, member, _, addon, _ := homeRowsServer(t)
	row := s.addRow(t, homerows.Row{Name: "Series to try", Kind: homerows.KindCatalog, Place: homerows.PlaceTop, Addon: addon,
		CatalogType: "series", CatalogID: "shows", Everyone: true, Library: true})
	var views QueryResult
	s.get(t, "/UserViews", member, &views)
	i := slices.IndexFunc(views.Items, func(view BaseItemDto) bool { return view.Name == "Series to try" })
	if i < 0 {
		t.Fatalf("views: %v", itemNames(views.Items))
	}
	view := views.Items[i]
	if view.Id != customRowID(row.ID, accounts.ID{}).String() || view.Type != "CollectionFolder" || view.CollectionType != "" {
		t.Errorf("the row's view: %+v", view)
	}
	var latest []BaseItemDto
	if status := s.get(t, "/Items/Latest?ParentId="+view.Id+"&Limit=16", member, &latest); status != http.StatusOK ||
		!slices.Equal(itemNames(latest), []string{"Show"}) {
		t.Errorf("its latest items: %d %v", status, itemNames(latest))
	}
	var menus struct{ Items []MenuView }
	s.get(t, "/Polyfin/UserViews", member, &menus)
	if !slices.ContainsFunc(menus.Items, func(m MenuView) bool { return m.Id == view.Id && !m.HideInMenus }) {
		t.Errorf("the web player's menus: %+v", menus.Items)
	}
}

// A row shown to chosen users is not shown to the others, and a row a
// user turned off is not shown to them: neither its titles nor its
// folder, which the other users keep.
func TestRowsFollowWhoTheyAreShownTo(t *testing.T) {
	t.Parallel()
	s, member, admin, addon, _ := homeRowsServer(t)
	other := s.user("other", nil)
	otherToken := s.signIn("other", "phone")
	store := homerows.New(s.pool)
	everyone := s.addRow(t, homerows.Row{Name: "For all", Kind: homerows.KindCatalog, Place: homerows.PlaceTop, Addon: addon,
		CatalogType: "series", CatalogID: "shows", Everyone: true, Library: true})
	s.addRow(t, homerows.Row{Name: "For one", Kind: homerows.KindCatalog, Place: homerows.PlaceBottom, Addon: addon,
		CatalogType: "movie", CatalogID: "top", Users: []accounts.ID{other.ID}})
	var me UserDto
	s.get(t, "/Users/Me", member, &me)
	memberID, _ := accounts.ParseID(me.Id)
	if err := store.SetOff(t.Context(), memberID, everyone.ID, true); err != nil {
		t.Fatal(err)
	}
	if got := rowSummary(s.customRowsOf(t, member)); len(got) != 0 {
		t.Errorf("the member's rows: %q", got)
	}
	if got := rowSummary(s.customRowsOf(t, otherToken)); !slices.Equal(got, []string{"For all @Top: Show", "For one @Bottom: Movie 0, Movie 1, Movie 2"}) {
		t.Errorf("the other user's rows: %q", got)
	}
	if got := rowSummary(s.customRowsOf(t, admin)); !slices.Equal(got, []string{"For all @Top: Show"}) {
		t.Errorf("the administrator's rows: %q", got)
	}
	folder := customRowID(everyone.ID, accounts.ID{}).String()
	if status := s.get(t, "/Items?ParentId="+folder, member, nil); status != http.StatusBadRequest {
		t.Errorf("the folder of a row turned off: %d", status)
	}
	var views QueryResult
	s.get(t, "/UserViews", member, &views)
	if slices.Contains(itemNames(views.Items), "For all") {
		t.Errorf("the member's views: %v", itemNames(views.Items))
	}
	// A row not shown to a user cannot be turned off by them.
	if err := store.SetOff(t.Context(), memberID, store2ID(t, store, "For one"), true); err == nil {
		t.Error("a row not shown was turned off")
	}
	if err := store.SetOff(t.Context(), memberID, everyone.ID, false); err != nil {
		t.Fatal(err)
	}
	if got := rowSummary(s.customRowsOf(t, member)); !slices.Equal(got, []string{"For all @Top: Show"}) {
		t.Errorf("turned on again: %q", got)
	}
}

// store2ID returns the identifier of the row named name.
func store2ID(t *testing.T, store *homerows.Store, name string) accounts.ID {
	t.Helper()
	rows, err := store.Rows(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Name == name {
			return row.ID
		}
	}
	t.Fatalf("no row %q", name)
	return accounts.ID{}
}

// A Because you watched row follows the user's last movies and series: a
// row for each, of the titles similar to it but those played. None while
// Similar titles is off.
func TestBecauseYouWatchedFollowsTheLastMovie(t *testing.T) {
	t.Parallel()
	s, member, _, _, movies := homeRowsServer(t)
	row := s.addRow(t, homerows.Row{Name: "Because you watched", Kind: homerows.KindBecause, Place: homerows.PlaceResume, Count: 1,
		Everyone: true, Library: true})
	if got := rowSummary(s.customRowsOf(t, member)); len(got) != 0 {
		t.Fatalf("before anything is played: %q", got)
	}
	if status, _ := s.call(http.MethodPost, "/UserPlayedItems/"+movies[0].Id, app("tv", member), nil); status != http.StatusOK {
		t.Fatalf("played: %d", status)
	}
	rows := s.customRowsOf(t, member)
	if got := rowSummary(rows); !slices.Equal(got, []string{"Because you watched Movie 0 @AfterResume: Movie 2"}) {
		t.Fatalf("after Movie 0: %q", got)
	}
	baseline, _ := accounts.ParseID(movies[0].Id)
	if rows[0].Id != customRowID(row.ID, baseline).String() {
		t.Errorf("the row's folder: %s", rows[0].Id)
	}
	var listed QueryResult
	if status := s.get(t, "/Items?ParentId="+rows[0].Id, member, &listed); status != http.StatusOK || !slices.Equal(itemNames(listed.Items), []string{"Movie 2"}) {
		t.Errorf("its list: %d %v", status, itemNames(listed.Items))
	}
	// Movie 1 is played last: the row follows it, as its view does; its
	// similar titles are played or of another genre.
	if status, _ := s.call(http.MethodPost, "/UserPlayedItems/"+movies[1].Id, app("tv", member), nil); status != http.StatusOK {
		t.Fatalf("played: %d", status)
	}
	var views QueryResult
	s.get(t, "/UserViews", member, &views)
	if got := itemNames(views.Items); !slices.Contains(got, "Because you watched Movie 1") || slices.Contains(got, "Because you watched Movie 0") {
		t.Errorf("views after Movie 1: %v", got)
	}
	for _, row := range s.customRowsOf(t, member) {
		if strings.Contains(row.Name, "Movie 0") {
			t.Errorf("a row still follows Movie 0: %+v", row)
		}
	}
	s.setting(t, func(settings *accounts.Settings) { settings.SimilarTitles = false })
	s.get(t, "/UserViews", member, &views)
	if got := rowSummary(s.customRowsOf(t, member)); len(got) != 0 || slices.ContainsFunc(itemNames(views.Items), func(name string) bool {
		return strings.HasPrefix(name, "Because")
	}) {
		t.Errorf("with similar titles off: %q, views %v", got, itemNames(views.Items))
	}
}

// Every row leaves out what the user may not see: titles parental control
// hides, those the user hid, and the catalogs and collections of the
// server's libraries hidden from them.
func TestRowsLeaveOutWhatTheUserMayNotSee(t *testing.T) {
	t.Parallel()
	s, member, admin, addon, movies := homeRowsServer(t)
	s.user("child", func(c *accounts.UserChanges) { c.Parental = &accounts.ParentalControl{MaxRating: new(13)} })
	child := s.signIn("child", "tablet")
	var views, groups QueryResult
	s.get(t, "/UserViews", member, &views)
	ids := map[string]string{}
	for _, view := range views.Items {
		ids[view.Name] = view.Id
	}
	s.get(t, "/Items?ParentId="+ids["Groups"], member, &groups)
	group, _ := accounts.ParseID(groups.Items[0].Id)
	picks, _ := accounts.ParseID(s.createCollection(t, admin, "name=Picks&ids="+movies[0].Id+","+movies[2].Id))
	s.addRow(t, homerows.Row{Name: "Series", Kind: homerows.KindCatalog, Place: homerows.PlaceTop, Addon: addon,
		CatalogType: "series", CatalogID: "shows", Everyone: true})
	s.addRow(t, homerows.Row{Name: "Movies", Kind: homerows.KindCatalog, Place: homerows.PlaceTop, Addon: addon,
		CatalogType: "movie", CatalogID: "top", Everyone: true})
	s.addRow(t, homerows.Row{Name: "Group", Kind: homerows.KindCollection, Place: homerows.PlaceBottom, Collection: group, Everyone: true})
	s.addRow(t, homerows.Row{Name: "Picks", Kind: homerows.KindCollection, Place: homerows.PlaceBottom, Collection: picks, Everyone: true})
	all := []string{"Series @Top: Show", "Movies @Top: Movie 0, Movie 1, Movie 2", "Group @Bottom: Movie 0, Movie 1, Movie 2",
		"Picks @Bottom: Movie 0, Movie 2"}
	if got := rowSummary(s.customRowsOf(t, member)); !slices.Equal(got, all) {
		t.Fatalf("the member's rows: %q", got)
	}
	// The series is rated TV-14, above the child's limit.
	if got := rowSummary(s.customRowsOf(t, child)); !slices.Equal(got, all[1:]) {
		t.Errorf("the child's rows: %q", got)
	}
	// The member hides Movie 0.
	if status, _ := s.call(http.MethodPost, "/Polyfin/UserItems/"+movies[0].Id+"/Hidden", app("tv", member), nil); status != http.StatusNoContent {
		t.Fatalf("hiding: %d", status)
	}
	if got := rowSummary(s.customRowsOf(t, member)); !slices.Equal(got, []string{"Series @Top: Show", "Movies @Top: Movie 1, Movie 2",
		"Group @Bottom: Movie 1, Movie 2", "Picks @Bottom: Movie 2"}) {
		t.Errorf("with Movie 0 hidden: %q", got)
	}
	// The Top and Groups libraries are hidden from the member: their
	// catalog and collection make no row.
	var me UserDto
	s.get(t, "/Users/Me", member, &me)
	memberID, _ := accounts.ParseID(me.Id)
	top, _ := accounts.ParseID(ids["Top"])
	groupsID, _ := accounts.ParseID(ids["Groups"])
	if _, err := s.store.UpdateUser(t.Context(), memberID, accounts.UserChanges{HiddenLibraries: &[]accounts.ID{top, groupsID}}, nil); err != nil {
		t.Fatal(err)
	}
	if got := rowSummary(s.customRowsOf(t, member)); !slices.Equal(got, []string{"Series @Top: Show", "Picks @Bottom: Movie 2"}) {
		t.Errorf("with the libraries hidden: %q", got)
	}
}

// arrivingAddon serves a movie catalog whose titles tests change.
type arrivingAddon struct {
	mu     sync.Mutex
	movies []stremio.Meta
}

func (a *arrivingAddon) add(meta stremio.Meta) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.movies = append([]stremio.Meta{meta}, a.movies...)
}

func (a *arrivingAddon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	movies := slices.Clone(a.movies)
	a.mu.Unlock()
	switch path := r.URL.EscapedPath(); {
	case path == "/manifest.json":
		_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "arriving", Name: "Arriving", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "fresh", Name: "Fresh"}}})
	case strings.HasPrefix(path, "/catalog/movie/fresh"):
		_ = json.NewEncoder(w).Encode(map[string]any{"metas": movies})
	case strings.HasPrefix(path, "/meta/movie/"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/meta/movie/"), ".json")
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
}

// A title new to a library's catalog shows in New in your libraries, and
// leaves it 7 days later.
func TestNewTitlesShowForAWeek(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("member", nil)
	var clock struct {
		sync.Mutex
		now time.Time
	}
	clock.now = time.Now()
	now := func() time.Time {
		clock.Lock()
		defer clock.Unlock()
		return clock.now
	}
	advance := func(d time.Duration) {
		clock.Lock()
		defer clock.Unlock()
		clock.now = clock.now.Add(d)
	}
	s.library.SetClock(now)
	fresh := &arrivingAddon{movies: []stremio.Meta{{ID: "tt9000", Type: "movie", Name: "Old"}}}
	server := httptest.NewServer(fresh)
	t.Cleanup(server.Close)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), server.URL+"/manifest.json", false); err != nil {
		t.Fatal(err)
	}
	member := s.signIn("member", "tv")
	s.addRow(t, homerows.Row{Name: "New in your libraries", Kind: homerows.KindNew, Place: homerows.PlaceTop, Everyone: true})
	var views, listed QueryResult
	s.get(t, "/UserViews", member, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, member, &listed)
	if got := rowSummary(s.customRowsOf(t, member)); len(got) != 0 {
		t.Fatalf("before a title came: %q", got)
	}
	// A title comes; once the catalog's page is old, a listing has it read
	// again, in the background.
	fresh.add(stremio.Meta{ID: "tt9001", Type: "movie", Name: "Arrived"})
	advance(2 * time.Hour)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, member, &listed)
	want := []string{"New in your libraries @Top: Arrived"}
	for deadline := time.Now().Add(10 * time.Second); !slices.Equal(rowSummary(s.customRowsOf(t, member)), want); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("the new title never showed: %q", rowSummary(s.customRowsOf(t, member)))
		}
	}
	advance(library.ArrivalWindow)
	if got := rowSummary(s.customRowsOf(t, member)); len(got) != 0 {
		t.Errorf("a week later: %q", got)
	}
}
