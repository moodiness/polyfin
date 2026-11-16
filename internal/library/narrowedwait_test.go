package library

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// slowCatalogs serves addon, but answers the catalog pages whose path
// slow matches once gate is closed, or after delay when gate is nil, as a
// busy addon does, and counts the pages asked and those answered, by path.
type slowCatalogs struct {
	addon *fakeAddon
	slow  func(path string) bool
	gate  chan struct{}
	delay time.Duration

	mu            sync.Mutex
	asked, served map[string]int
	// arrived is when each catalog page asked arrived, by path, in order.
	arrived []arrival
}

// arrival is a catalog page asked of an addon, and when it arrived.
type arrival struct {
	path string
	at   time.Time
}

func (a *slowCatalogs) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.EscapedPath()
	if strings.HasPrefix(path, "/catalog/") {
		a.mu.Lock()
		a.asked[path]++
		a.arrived = append(a.arrived, arrival{path, time.Now()})
		a.mu.Unlock()
		if a.slow(path) {
			// Of the gate and the delay, the one not used is nil: it
			// never answers.
			var later <-chan time.Time
			if a.gate == nil {
				later = time.After(a.delay)
			}
			select {
			case <-a.gate:
			case <-later:
			case <-r.Context().Done():
				return
			}
		}
	}
	a.addon.ServeHTTP(w, r)
	if strings.HasPrefix(path, "/catalog/") {
		a.mu.Lock()
		a.served[path]++
		a.mu.Unlock()
	}
}

// answered counts the catalog pages whose path contains part: those
// answered, and the most any was asked.
func (a *slowCatalogs) answered(part string) (pages, most int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for path := range a.served {
		if strings.Contains(path, part) {
			pages++
		}
	}
	for path, n := range a.asked {
		if strings.Contains(path, part) {
			most = max(most, n)
		}
	}
	return pages, most
}

// askedSince returns the catalog pages whose path contains part that
// arrived after since.
func (a *slowCatalogs) askedSince(part string, since time.Time) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var paths []string
	for _, arrival := range a.arrived {
		if strings.Contains(arrival.path, part) && arrival.at.After(since) {
			paths = append(paths, arrival.path)
		}
	}
	return paths
}

// installServing serves handler, an addon, and installs it in scope.
func (e env) installServing(scope addons.Scope, handler http.Handler) {
	e.t.Helper()
	server := httptest.NewServer(handler)
	e.t.Cleanup(server.Close)
	if _, err := e.addons.Install(e.t.Context(), scope, server.URL+"/manifest.json", false); err != nil {
		e.t.Fatal(err)
	}
}

// A genre's page offered by catalogs of a busy addon answers once its
// wait ends: its counts at once, from what is read already, and its
// listing with the titles of the catalogs read by then, then the titles
// kept of the genre, each user's limits applying, more to follow. The
// catalogs are read on, each page once, and the next listing is whole.
// The busy catalogs answer when the test lets them, and the test ends
// the listings' waits (see narrowedEnd): what a listing counts does not
// depend on how fast the machine runs.
func TestAGenrePageAnswersWithinItsWait(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	// Each listing's wait ends when the test ends it.
	waits := make(chan chan<- time.Time, 8)
	e.service.narrowedEnd = func(time.Duration) <-chan time.Time {
		end := make(chan time.Time, 1)
		waits <- end
		return end
	}
	title := func(id, name, rating string, genres ...string) stremio.Meta {
		return stremio.Meta{ID: id, Type: "movie", Name: name, ReleaseInfo: "2001", Released: "2001-05-05T00:00:00.000Z", Genres: genres,
			Extras: &stremio.Extras{Certification: rating}}
	}
	// The titles kept of the genre, as a library listed them.
	shelf := &fakeAddon{
		manifest: stremio.Manifest{ID: "shelf", Name: "Shelf", Version: "1", Types: []string{"movie"}, Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{{Type: "movie", ID: "shelf", Name: "Shelf"}}},
		catalogs: map[string][]stremio.Meta{"movie/shelf": {
			title("tt951", "Ash Road", "PG", "Drama"), title("tt952", "Blue Hour", "PG", "Drama"), title("tt953", "Cold Night", "R", "Drama"),
			title("tt954", "Dawn Light", "PG", "Comedy"), title("tt955", "Fern Hill", "PG", "Drama"), title("tt956", "Gray Dock", "PG", "Drama"),
		}},
	}
	e.install(addons.Shared(), shelf)
	kept := map[string]accounts.ID{}
	for _, item := range e.children(e.admin, "Shelf", 0, 20).Items {
		kept[item.Name] = item.ID
	}
	// Eight catalogs offer Drama, as many as the addon is asked pages at
	// once: two answer at once, six once the test lets them.
	picks := &fakeAddon{
		manifest: stremio.Manifest{ID: "picks", Name: "Picks", Version: "1", Types: []string{"movie"}, Resources: []stremio.Resource{{Name: "catalog"}}},
		catalogs: map[string][]stremio.Meta{},
	}
	var quick, all []string
	for c := range catalogHostFetches {
		id := fmt.Sprintf("pick%d", c)
		picks.manifest.Catalogs = append(picks.manifest.Catalogs, stremio.Catalog{Type: "movie", ID: id, Name: id,
			Extra: []stremio.Extra{{Name: "genre", Options: []string{"Drama"}}}})
		for i := range 5 {
			rating := "PG"
			if c == 0 && i == 4 {
				rating = "R"
			}
			name := fmt.Sprintf("Pick %d-%d", c, i)
			picks.catalogs["movie/"+id] = append(picks.catalogs["movie/"+id], title(fmt.Sprintf("pick:%d:%d", c, i), name, rating, "Drama"))
			all = append(all, name)
			if c < 2 {
				quick = append(quick, name)
			}
		}
	}
	busy := &slowCatalogs{addon: picks, gate: make(chan struct{}), asked: map[string]int{}, served: map[string]int{},
		slow: func(path string) bool {
			return !strings.Contains(path, "/pick0/") && !strings.Contains(path, "/pick1/")
		}}
	e.installServing(addons.Shared(), busy)
	if err := e.service.userData.Hide(t.Context(), e.member.ID, kept["Fern Hill"], time.Now()); err != nil {
		t.Fatal(err)
	}
	child := e.restrict("child", accounts.ParentalControl{MaxRating: new(13)})

	drama := func(option string) bool { return option == "Drama" }
	page := func(user accounts.User, atOnce bool, count int) Page {
		t.Helper()
		read := t.Context()
		if atOnce {
			read = AtOnce(read)
		}
		page, err := e.service.GenreTitles(read, user, []string{"Drama"}, drama, []Kind{KindMovie}, 0, count)
		if err != nil {
			t.Fatal(err)
		}
		return page
	}

	// The page's counts answer at once, while the busy catalogs have not
	// answered, and start reading them.
	if counted := page(e.member, true, 100); counted.Total == 0 || !counted.More {
		t.Errorf("the counts, cold: %d titles, more %v", counted.Total, counted.More)
	}
	// The two quick catalogs answer: the counts then count their titles,
	// and the titles kept, but the one the user hid.
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if counted := page(e.member, true, 100); counted.Total == 15 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the quick catalogs did not answer")
		}
	}
	// The listing, and a restricted user's opened meanwhile, wait for the
	// busy catalogs until their waits end.
	type listing struct {
		page Page
		err  error
	}
	open := func(user accounts.User) <-chan listing {
		listed := make(chan listing, 1)
		go func() {
			page, err := e.service.GenreTitles(t.Context(), user, []string{"Drama"}, drama, []Kind{KindMovie}, 0, 12)
			listed <- listing{page, err}
		}()
		return listed
	}
	member, restricted := open(e.member), open(child)
	var ends []chan<- time.Time
	for len(ends) < 2 {
		select {
		case end := <-waits:
			ends = append(ends, end)
		case <-time.After(10 * time.Second):
			t.Fatalf("%d listings waited for the busy catalogs, want 2", len(ends))
		}
	}
	select {
	case <-member:
		t.Fatal("the listing answered before its wait ended")
	case <-restricted:
		t.Fatal("the restricted user's listing answered before its wait ended")
	default:
	}
	for _, end := range ends {
		end <- time.Now()
	}
	// The listing then lists the two catalogs that answered, then the
	// titles kept, but those the user hid.
	listed := <-member
	if listed.err != nil {
		t.Fatal(listed.err)
	}
	got := names(listed.page.Items)
	if len(got) != 12 || !slices.Equal(slices.Sorted(slices.Values(got[:10])), quick) || !slices.Equal(got[10:], []string{"Ash Road", "Blue Hour"}) ||
		listed.page.Total != 15 || !listed.page.More {
		t.Errorf("the listing, cold: %v, total %d, more %v", got, listed.page.Total, listed.page.More)
	}
	// The restricted user's has their limits: no title rated R.
	listed = <-restricted
	if listed.err != nil {
		t.Fatal(listed.err)
	}
	if got := names(listed.page.Items); len(got) != 12 || slices.Contains(got, "Pick 0-4") || slices.Contains(got, "Cold Night") ||
		!slices.Equal(got[9:], []string{"Ash Road", "Blue Hour", "Fern Hill"}) || !listed.page.More {
		t.Errorf("for a restricted user, cold: %v, total %d", got, listed.page.Total)
	}

	// The busy catalogs answer; they are read on, each page once whoever
	// asked for it.
	close(busy.gate)
	deadline := time.Now().Add(10 * time.Second)
	for {
		if pages, _ := busy.answered("genre=Drama"); pages == catalogHostFetches {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the catalogs were not read on")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// The next listing, whose wait the test never ends, lists them all.
	whole := page(e.member, false, 100)
	if pages, most := busy.answered("genre=Drama"); pages != catalogHostFetches || most != 1 {
		t.Errorf("%d catalog pages asked, one %d times", pages, most)
	}
	got = names(whole.Items)
	if len(got) != 44 || !slices.Equal(slices.Sorted(slices.Values(got[:40])), all) ||
		!slices.Equal(got[40:], []string{"Ash Road", "Blue Hour", "Cold Night", "Gray Dock"}) || whole.Total != 44 || whole.More {
		t.Errorf("the listing, once read: %d titles, total %d, more %v: %v", len(got), whole.Total, whole.More, got)
	}
	if counted := page(e.member, true, 100); counted.Total != 44 || counted.More {
		t.Errorf("the counts, once read: %d titles, more %v", counted.Total, counted.More)
	}
	restrictedWhole := page(child, false, 100)
	if got := names(restrictedWhole.Items); len(got) != 43 || slices.Contains(got, "Pick 0-4") || slices.Contains(got, "Cold Night") {
		t.Errorf("for a restricted user, once read: %d titles: %v", len(got), got)
	}
}

// A studio's page waiting for a busy catalog lists the titles of the
// catalogs read by then, then those whose details name the studio, then
// TMDB's, last, as once the catalogs are read.
func TestAStudioPageKeepsTMDBLastWithinItsWait(t *testing.T) {
	t.Parallel()
	e := newCreditsEnv(t, "k3y", time.Hour)
	e.service.UseTitleCredits(e.queue)
	e.service.SetNarrowedWait(300 * time.Millisecond)
	north := studio(71, "North Studio", "/north-co.png")
	e.tmdb.studios("/movie/1001", []any{}, []any{north})
	e.tmdb.studios("/movie/1013", []any{}, []any{north})
	for _, name := range []string{"Harbor Lights", "Night Shift"} {
		e.open(e.admin, name)
	}
	e.settle()
	catalog := func(id string, titles ...stremio.Meta) *fakeAddon {
		return &fakeAddon{
			manifest: stremio.Manifest{ID: id, Name: id, Version: "1", Types: []string{"movie"}, Resources: []stremio.Resource{{Name: "catalog"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: id, Name: id, Extra: []stremio.Extra{{Name: "genre", Options: []string{"North Studio"}}}}}},
			catalogs: map[string][]stremio.Meta{"movie/" + id: titles},
		}
	}
	e.install(addons.Shared(), catalog("picks", stremio.Meta{ID: "pick:1", Type: "movie", Name: "Catalog Pick", Genres: stremio.Names{"North Studio"}}))
	gate := make(chan struct{})
	late := catalog("late", stremio.Meta{ID: "late:1", Type: "movie", Name: "Late Pick", Genres: stremio.Names{"North Studio"}})
	late.catalogGate = gate
	e.install(addons.Shared(), late)
	e.tmdb.mu.Lock()
	e.tmdb.answers["/discover/movie?page=1"] = map[string]any{"results": []any{discovered("movie", 1301, "Discovered One", "2015-01-01", 50)}}
	e.tmdb.answers["/discover/movie?page=2"] = map[string]any{"results": []any{}}
	e.tmdb.answers["/discover/tv?page=1"] = map[string]any{"results": []any{}}
	e.tmdb.answers["/discover/tv?page=2"] = map[string]any{"results": []any{}}
	e.tmdb.mu.Unlock()
	studioPage := func() Page {
		t.Helper()
		page, err := e.service.StudioTitles(t.Context(), e.admin, []string{"North Studio"}, func(option string) bool { return option == "North Studio" },
			[]Kind{KindMovie, KindSeries}, 0, 100)
		if err != nil {
			t.Fatal(err)
		}
		return page
	}
	// The page's counts start reading its catalogs; the quick one answers.
	for deadline := time.Now().Add(10 * time.Second); ; {
		page, err := e.service.StudioTitles(AtOnce(t.Context()), e.admin, []string{"North Studio"},
			func(option string) bool { return option == "North Studio" }, []Kind{KindMovie}, 0, 100)
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(names(page.Items), "Catalog Pick") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the quick catalog did not answer")
		}
		time.Sleep(10 * time.Millisecond)
	}
	want := []string{"Catalog Pick", "Harbor Lights", "Night Shift", "Discovered One"}
	if page := studioPage(); !slices.Equal(names(page.Items), want) || page.Total != len(want)+1 || !page.More {
		t.Errorf("the studio's page, its late catalog not read: %v, total %d, more %v", names(page.Items), page.Total, page.More)
	}
	close(gate)
	want = []string{"Catalog Pick", "Late Pick", "Harbor Lights", "Night Shift", "Discovered One"}
	deadline := time.Now().Add(10 * time.Second)
	for {
		page := studioPage()
		if slices.Equal(names(page.Items), want) && page.Total == len(want) && !page.More {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the studio's page, once read: %v, total %d, more %v", names(page.Items), page.Total, page.More)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A host's place goes to a request waiting for a page before the reads no
// request waits for, whenever they came; a request that stops waiting
// passes its place on.
func TestAHostsPlacesGoToWaitingRequestsFirst(t *testing.T) {
	t.Parallel()
	slots := newAddonSlots[string](1)
	release, err := slots.acquire(t.Context(), "host")
	if err != nil {
		t.Fatal(err)
	}
	queued := func(first, yielded int) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); ; {
			slots.mu.Lock()
			q := slots.slots["host"]
			done := len(q.first) == first && len(q.yielded) == yielded
			slots.mu.Unlock()
			if done {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("not %d and %d waiting", first, yielded)
			}
			time.Sleep(time.Millisecond)
		}
	}
	served := make(chan string, 3)
	take := func(ctx context.Context, name string) {
		if release, err := slots.acquire(ctx, "host"); err == nil {
			served <- name
			release()
		}
	}
	go take(withYielding(t.Context()), "read on")
	queued(0, 1)
	gone, leave := context.WithCancel(t.Context())
	go take(gone, "gone")
	queued(1, 1)
	go take(t.Context(), "waited for")
	queued(2, 1)
	leave()
	queued(1, 1)
	release()
	if first, second := <-served, <-served; first != "waited for" || second != "read on" {
		t.Errorf("served %s, then %s", first, second)
	}
	slots.mu.Lock()
	defer slots.mu.Unlock()
	if q := slots.slots["host"]; q.taken != 0 || len(q.first)+len(q.yielded) != 0 {
		t.Errorf("left %d taken, %d waiting", q.taken, len(q.first)+len(q.yielded))
	}
}

// A page's catalogs are read on to the end once the request that started
// the read stops waiting, and no read starts once the server stops.
func TestAPagesReadGoesOnUntilTheServerStops(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	server, stop := context.WithCancel(t.Context())
	defer stop()
	e.service.EndWith(server)
	var titles []stremio.Meta
	for i := range 20 {
		titles = append(titles, stremio.Meta{ID: fmt.Sprintf("tt%03d", i), Type: "movie", Name: fmt.Sprintf("Title %d", i),
			Genres: stremio.Names{"Drama", "Crime"}})
	}
	// A catalog of each genre, so that the second read does not know the
	// pages' size from the first: it asks its first two pages one after
	// the other, then the last two at once.
	catalog := func(id, genre string) stremio.Catalog {
		return stremio.Catalog{Type: "movie", ID: id, Name: id, Extra: []stremio.Extra{{Name: "genre", Options: []string{genre}}, {Name: "skip"}}}
	}
	paged := &fakeAddon{
		manifest: stremio.Manifest{ID: "paged", Name: "Paged", Version: "1", Types: []string{"movie"}, Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{catalog("dramas", "Drama"), catalog("crimes", "Crime")}},
		catalogs: map[string][]stremio.Meta{"movie/dramas": titles, "movie/crimes": titles},
		pageSize: 5,
	}
	const delay = 200 * time.Millisecond
	busy := &slowCatalogs{addon: paged, delay: delay, asked: map[string]int{}, served: map[string]int{},
		slow: func(string) bool { return true }}
	e.installServing(addons.Shared(), busy)
	open := func(genre string) {
		t.Helper()
		request, leave := context.WithCancel(t.Context())
		if _, err := e.service.GenreTitles(AtOnce(request), e.member, []string{genre}, func(option string) bool { return option == genre },
			[]Kind{KindMovie}, 0, 20); err != nil {
			t.Fatal(err)
		}
		leave()
	}
	served := func(genre string) int {
		pages, _ := busy.answered("genre=" + genre)
		return pages
	}

	// The request is gone; its four pages are read all the same.
	open("Drama")
	for deadline := time.Now().Add(10 * time.Second); served("Drama") < 4; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%d pages read once the request was gone", served("Drama"))
		}
	}

	// The server stops once the first page of another read answered: the
	// page then asked, if any, is cancelled, and the last two are never
	// asked. Pages asked before the stop may still be on their way to the
	// addon for a moment; a page the read would ask next comes a whole
	// answer later.
	open("Crime")
	for deadline := time.Now().Add(10 * time.Second); served("Crime") < 1; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the read did not start")
		}
	}
	stop()
	stopped := time.Now()
	time.Sleep(3 * delay)
	if late := busy.askedSince("genre=Crime", stopped.Add(delay/2)); len(late) > 0 {
		t.Errorf("pages asked after the server stopped: %v", late)
	}
}
