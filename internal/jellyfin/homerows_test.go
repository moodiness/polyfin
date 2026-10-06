package jellyfin

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// rowTitles is how many movies, and series, rowsAddon serves: more than a
// row's titles listed ahead.
const rowTitles = 12

// rowsAddon serves rowTitles movies of 1h30 and rowTitles series of two
// episodes, each named after its identifier, and one stream of each, and
// counts the stream requests of each title. Held, it answers them only
// once released.
type rowsAddon struct {
	url     string
	release chan struct{}
	once    sync.Once

	mu    sync.Mutex
	asked map[string]int
	// inFlight counts the stream requests under way, and most the most
	// that were under way at once.
	inFlight, most atomic.Int32
}

func newRowsAddon(t *testing.T, held bool) *rowsAddon {
	t.Helper()
	a := &rowsAddon{release: make(chan struct{}), asked: map[string]int{}}
	if !held {
		a.answer()
	}
	movies := func() []stremio.Meta {
		var list []stremio.Meta
		for i := range rowTitles {
			id := fmt.Sprintf("tt80000%02d", i)
			list = append(list, stremio.Meta{ID: id, Type: "movie", Name: id, Runtime: "1h 30min", Released: "2020-04-10T00:00:00.000Z"})
		}
		return list
	}
	series := func() []stremio.Meta {
		var list []stremio.Meta
		for i := range rowTitles {
			id := fmt.Sprintf("tt70000%02d", i)
			list = append(list, stremio.Meta{ID: id, Type: "series", Name: id, Videos: []stremio.Video{
				{ID: id + ":1:1", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00.000Z", Runtime: "45min"},
				{ID: id + ":1:2", Season: 1, Episode: 2, Released: "2020-01-08T00:00:00.000Z", Runtime: "45min"},
			}})
		}
		return list
	}
	find := func(list []stremio.Meta, id string) (stremio.Meta, bool) {
		i := slices.IndexFunc(list, func(m stremio.Meta) bool { return m.ID == id })
		if i < 0 {
			return stremio.Meta{}, false
		}
		return list[i], true
	}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "rows", Name: "Rows", Version: "1", Types: []string{"movie", "series"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "movies", Name: "Movies"}, {Type: "series", ID: "shows", Name: "Shows"}}})
		case strings.HasPrefix(path, "/catalog/movie/movies"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": movies()})
		case strings.HasPrefix(path, "/catalog/series/shows"):
			previews := series()
			for i := range previews {
				previews[i].Videos = nil
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": previews})
		case strings.HasPrefix(path, "/meta/"):
			kind, id, _ := strings.Cut(strings.TrimSuffix(strings.TrimPrefix(path, "/meta/"), ".json"), "/")
			list := movies()
			if kind == "series" {
				list = series()
			}
			meta, ok := find(list, id)
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
		case strings.HasPrefix(path, "/stream/"):
			_, id, _ := strings.Cut(strings.TrimSuffix(strings.TrimPrefix(path, "/stream/"), ".json"), "/")
			a.mu.Lock()
			a.asked[id]++
			a.mu.Unlock()
			now := a.inFlight.Add(1)
			for seen := a.most.Load(); now > seen && !a.most.CompareAndSwap(seen, now); seen = a.most.Load() {
			}
			defer a.inFlight.Add(-1)
			<-a.release
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{{Name: "1080p", URL: server.URL + "/files/" + id + ".mkv"}}})
		case strings.HasPrefix(path, "/files/"):
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader("\x1a\x45\xdf\xa3 media bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	// Registered after the server's, this runs first: closing the server
	// waits for the requests held.
	t.Cleanup(server.Close)
	t.Cleanup(a.answer)
	a.url = server.URL + "/manifest.json"
	return a
}

// answer releases the stream requests.
func (a *rowsAddon) answer() { a.once.Do(func() { close(a.release) }) }

// requests are the stream requests of each title so far.
func (a *rowsAddon) requests() map[string]int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return maps.Clone(a.asked)
}

// rowsSetup has a member with every movie of rowsAddon under way, in
// Continue Watching, and the first episode of every series played, its
// second in Next Up.
type rowsSetup struct {
	testServer
	addon  *rowsAddon
	token  string
	member accounts.User
}

func rowsOn(t *testing.T, held bool) rowsSetup {
	t.Helper()
	return rowsOnServer(t, newTestServer(t, 10), held)
}

// rowsOnServer is rowsOn on a given test server.
func rowsOnServer(t *testing.T, s testServer, held bool) rowsSetup {
	t.Helper()
	member := s.user("member", nil)
	a := newRowsAddon(t, held)
	// Registered after the server's and the addon's, this runs first: the
	// listings under way end before the addon and the database go.
	t.Cleanup(func() { a.answer(); s.listedAhead(t) })
	addon, err := s.addons.Install(t.Context(), addons.Shared(), a.url, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "movie", CatalogID: "movies"},
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "shows"},
	}); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	ids := map[string]string{}
	for _, view := range views.Items {
		ids[view.Name] = view.Id
	}
	send := func(method, path string, body map[string]any, want int) {
		t.Helper()
		if status, data := s.call(method, path, app("tv", token), body); status != want {
			t.Fatalf("%s %s: %d %s", method, path, status, data)
		}
	}
	var page QueryResult
	s.get(t, "/Items?ParentId="+ids["Movies"]+"&Limit=100", token, &page)
	if len(page.Items) != rowTitles {
		t.Fatalf("%d movies listed", len(page.Items))
	}
	for _, movie := range page.Items {
		// Ten minutes of 1h30 leave a resume point.
		send(http.MethodPost, "/Sessions/Playing/Stopped", map[string]any{"ItemId": movie.Id, "PositionTicks": 6_000_000_000}, http.StatusNoContent)
	}
	s.get(t, "/Items?ParentId="+ids["Shows"]+"&Limit=100", token, &page)
	if len(page.Items) != rowTitles {
		t.Fatalf("%d series listed", len(page.Items))
	}
	for _, series := range page.Items {
		var episodes QueryResult
		s.get(t, "/Shows/"+series.Id+"/Episodes", token, &episodes)
		send(http.MethodPost, "/UserPlayedItems/"+episodes.Items[0].Id, nil, http.StatusOK)
	}
	if asked := a.requests(); len(asked) != 0 {
		t.Fatalf("streams asked while setting up: %v", asked)
	}
	return rowsSetup{testServer: s, addon: a, token: token, member: member}
}

// row asks for a row and gives the stream identifiers of its titles, in
// order.
func (rs rowsSetup) row(t *testing.T, path string) []string {
	t.Helper()
	var page QueryResult
	if status := rs.get(t, path, rs.token, &page); status != http.StatusOK {
		t.Fatalf("%s: %d", path, status)
	}
	if len(page.Items) != rowTitles {
		t.Fatalf("%s: %d titles", path, len(page.Items))
	}
	var ids []string
	for _, item := range page.Items {
		if item.Type == "Episode" {
			ids = append(ids, item.SeriesName+":1:2")
		} else {
			ids = append(ids, item.Name)
		}
	}
	return ids
}

// listedAhead waits for the titles queued from home rows to be listed.
func (s testServer) listedAhead(t *testing.T) {
	t.Helper()
	eventually(t, "the titles of home rows to be listed", func() bool {
		l := s.handler.listings
		l.mu.Lock()
		defer l.mu.Unlock()
		return l.running == 0 && len(l.waiting) == 0
	})
}

// onceEach is the requests of asking each of ids once.
func onceEach(ids []string) map[string]int {
	want := map[string]int{}
	for _, id := range ids {
		want[id]++
	}
	return want
}

func TestHomeRowsListTheirTitlesAhead(t *testing.T) {
	// The first titles of each row are analyzed too (see
	// TestHomeRowsPrepareTheirFirstTitles).
	rs := rowsOnServer(t, newProbingServer(t, 10, newFakeProbe(t, true).path), false)
	t.Cleanup(func() { rs.listedAhead(t); rs.settle(t) })
	resume := "/UserItems/Resume"
	nextUp := "/Shows/NextUp"

	// Switched off, the rows ask the addons nothing.
	rs.row(t, resume)
	rs.row(t, nextUp)
	rs.listedAhead(t)
	if asked := rs.addon.requests(); len(asked) != 0 {
		t.Fatalf("switched off: %v", asked)
	}

	rs.setting(t, func(s *accounts.Settings) { s.PrepareAhead = true })
	movies := rs.row(t, resume)
	rs.listedAhead(t)
	if asked, want := rs.addon.requests(), onceEach(movies[:homeRowTitles]); !maps.Equal(asked, want) {
		t.Fatalf("Continue Watching: %v, want %v", asked, want)
	}
	episodes := rs.row(t, nextUp)
	rs.listedAhead(t)
	if asked, want := rs.addon.requests(), onceEach(append(movies[:homeRowTitles:homeRowTitles], episodes[:homeRowTitles]...)); !maps.Equal(asked, want) {
		t.Fatalf("Next Up: %v, want %v", asked, want)
	}
	// Opening one of them lists its version at once, with no addon left to
	// ask, and asks nothing more.
	first := rs.firstOf(t, nextUp)
	rs.listedAhead(t)
	var item BaseItemDto
	rs.get(t, "/Users/"+rs.member.ID.String()+"/Items/"+first, rs.token, &item)
	if item.MediaSources == nil || len(*item.MediaSources) != 1 || (*item.MediaSources)[0].Name != "1080p" {
		t.Errorf("the episode opened with %+v", item.MediaSources)
	}
	var progress VersionProgress
	if rs.get(t, "/Polyfin/Items/"+first+"/Versions", rs.token, &progress); progress != (VersionProgress{Pending: 0, Count: 1}) {
		t.Errorf("the episode's versions: %+v", progress)
	}
	if asked, want := rs.addon.requests(), onceEach(append(movies[:homeRowTitles:homeRowTitles], episodes[:homeRowTitles]...)); !maps.Equal(asked, want) {
		t.Errorf("after opening: %v, want %v", asked, want)
	}

	// Only movies and episodes are listed: channels and music are not.
	// The first two of them are prepared too.
	var mu sync.Mutex
	var listed, prepared []accounts.ID
	rs.handler.listings = newListings(func(_ accounts.User, title accounts.ID, prepare bool) {
		mu.Lock()
		defer mu.Unlock()
		listed = append(listed, title)
		if prepare {
			prepared = append(prepared, title)
		}
	})
	movies3 := []accounts.ID{{3}, {4}, {5}}
	rs.handler.listAhead(rs.member, []library.Item{{ID: accounts.ID{1}, Kind: library.KindChannel},
		{ID: accounts.ID{2}, Kind: library.KindTrack}, {ID: movies3[0], Kind: library.KindMovie},
		{ID: movies3[1], Kind: library.KindEpisode}, {ID: movies3[2], Kind: library.KindMovie}})
	rs.listedAhead(t)
	mu.Lock()
	defer mu.Unlock()
	slices.SortFunc(listed, func(a, b accounts.ID) int { return int(a[0]) - int(b[0]) })
	slices.SortFunc(prepared, func(a, b accounts.ID) int { return int(a[0]) - int(b[0]) })
	if !slices.Equal(listed, movies3) || !slices.Equal(prepared, movies3[:2]) {
		t.Errorf("listed %v, prepared %v", listed, prepared)
	}
}

// Continue Watching and Next Up have the first version of their first two
// titles analyzed once listed, so that resuming from them starts at once.
func TestHomeRowsPrepareTheirFirstTitles(t *testing.T) {
	probe := newFakeProbe(t, true)
	rs := rowsOnServer(t, newProbingServer(t, 10, probe.path), false)
	t.Cleanup(func() { rs.listedAhead(t); rs.settle(t) })
	rs.setting(t, func(s *accounts.Settings) { s.PrepareAhead = true })
	var page QueryResult
	rs.get(t, "/UserItems/Resume", rs.token, &page)
	rs.listedAhead(t)
	rs.settle(t)
	if probe.runs() != homeRowPrepared {
		t.Fatalf("%d analyses, want %d", probe.runs(), homeRowPrepared)
	}
	for i, item := range page.Items[:homeRowTitles] {
		id, _ := accounts.ParseID(item.Id)
		versions, err := rs.library.Versions(t.Context(), rs.member, id)
		if err != nil || len(versions) != 1 {
			t.Fatalf("%s: %v %v", item.Name, versions, err)
		}
		if _, analyzed := rs.handler.Playback.Analyzed(t.Context(), versions[0].ID); analyzed != (i < homeRowPrepared) {
			t.Errorf("title %d of the row analyzed: %v", i, analyzed)
		}
	}
	// Asked again, the row prepares nothing more.
	rs.get(t, "/UserItems/Resume", rs.token, nil)
	rs.listedAhead(t)
	rs.settle(t)
	if probe.runs() != homeRowPrepared {
		t.Errorf("asked again: %d analyses", probe.runs())
	}
}

func TestHomeRowsSkipTitlesListedAlready(t *testing.T) {
	rs := rowsOn(t, false)
	resume := "/Users/" + rs.member.ID.String() + "/Items/Resume"
	movies := rs.row(t, resume)
	// The first movie's lists are in, as an open page or a play leaves
	// them.
	rs.listed(t, rs.member, rs.firstOf(t, resume))
	if asked := rs.addon.requests(); !maps.Equal(asked, onceEach(movies[:1])) {
		t.Fatalf("listed: %v", asked)
	}

	rs.setting(t, func(s *accounts.Settings) { s.PrepareAhead = true })
	rs.row(t, resume)
	rs.listedAhead(t)
	if asked, want := rs.addon.requests(), onceEach(movies[:homeRowTitles]); !maps.Equal(asked, want) {
		t.Fatalf("Continue Watching: %v, want %v", asked, want)
	}
	// Asked again, the row asks nothing more.
	rs.row(t, resume)
	rs.listedAhead(t)
	if asked, want := rs.addon.requests(), onceEach(movies[:homeRowTitles]); !maps.Equal(asked, want) {
		t.Errorf("Continue Watching again: %v, want %v", asked, want)
	}
}

func TestHomeRowsListTwoTitlesAtOnce(t *testing.T) {
	rs := rowsOn(t, true)
	rs.setting(t, func(s *accounts.Settings) { s.PrepareAhead = true })
	nextUp := "/Shows/NextUp?limit=24&enableTotalRecordCount=false"
	queued := func() (running, queue, waiting int) {
		l := rs.handler.listings
		l.mu.Lock()
		defer l.mu.Unlock()
		return l.running, len(l.queue), len(l.waiting)
	}

	// The row answers while the addon holds every stream request.
	episodes := rs.row(t, nextUp)
	eventually(t, "titles to be listed", func() bool { return rs.addon.inFlight.Load() >= 2 })
	time.Sleep(100 * time.Millisecond)
	if running, queue, waiting := queued(); running != 2 || queue != homeRowTitles-2 || waiting != homeRowTitles || rs.addon.most.Load() != 2 {
		t.Fatalf("%d running, %d queued, %d waiting, at most %d at once", running, queue, waiting, rs.addon.most.Load())
	}
	// Asked again meanwhile, the row queues none of its titles twice.
	rs.row(t, nextUp)
	if running, queue, waiting := queued(); running != 2 || queue != homeRowTitles-2 || waiting != homeRowTitles {
		t.Fatalf("asked again: %d running, %d queued, %d waiting", running, queue, waiting)
	}

	rs.addon.answer()
	rs.listedAhead(t)
	if asked, want := rs.addon.requests(), onceEach(episodes[:homeRowTitles]); !maps.Equal(asked, want) {
		t.Errorf("Next Up: %v, want %v", asked, want)
	}
	if most := rs.addon.most.Load(); most != maxListing {
		t.Errorf("%d titles listed at once", most)
	}
}

// firstOf is the identifier of the first title of a row.
func (rs rowsSetup) firstOf(t *testing.T, path string) string {
	t.Helper()
	var page QueryResult
	rs.get(t, path, rs.token, &page)
	if len(page.Items) == 0 {
		t.Fatalf("%s is empty", path)
	}
	return page.Items[0].Id
}
