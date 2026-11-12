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
// slow matches after delay, as a busy addon does, and counts the pages
// asked and those answered, by path.
type slowCatalogs struct {
	addon *fakeAddon
	slow  func(path string) bool
	delay time.Duration

	mu            sync.Mutex
	asked, served map[string]int
}

func (a *slowCatalogs) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.EscapedPath()
	if strings.HasPrefix(path, "/catalog/") {
		a.mu.Lock()
		a.asked[path]++
		a.mu.Unlock()
		if a.slow(path) {
			select {
			case <-time.After(a.delay):
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

// installServing serves handler, an addon, and installs it in scope.
func (e env) installServing(scope addons.Scope, handler http.Handler) {
	e.t.Helper()
	server := httptest.NewServer(handler)
	e.t.Cleanup(server.Close)
	if _, err := e.addons.Install(e.t.Context(), scope, server.URL+"/manifest.json", false); err != nil {
		e.t.Fatal(err)
	}
}

// A genre's page offered by catalogs of a busy addon answers within
// narrowedWait: its counts at once, from what is read already, and its
// listing with the titles of the catalogs read by then, then the titles
// kept of the genre, each user's limits applying, more to follow. The
// catalogs are read on, each page once, and the next listing is whole.
func TestAGenrePageAnswersWithinItsWait(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	const wait = 400 * time.Millisecond
	e.service.SetNarrowedWait(wait)
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
	// once: two answer at once, six in 1.5 s.
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
	busy := &slowCatalogs{addon: picks, delay: 1500 * time.Millisecond, asked: map[string]int{}, served: map[string]int{},
		slow: func(path string) bool {
			return !strings.Contains(path, "/pick0/") && !strings.Contains(path, "/pick1/")
		}}
	e.installServing(addons.Shared(), busy)
	if err := e.service.userData.Hide(t.Context(), e.member.ID, kept["Fern Hill"], time.Now()); err != nil {
		t.Fatal(err)
	}
	child := e.restrict("child", accounts.ParentalControl{MaxRating: new(13)})

	page := func(user accounts.User, atOnce bool, count int) (Page, time.Duration) {
		t.Helper()
		read := t.Context()
		if atOnce {
			read = AtOnce(read)
		}
		started := time.Now()
		page, err := e.service.GenreTitles(read, user, []string{"Drama"}, func(option string) bool { return option == "Drama" },
			[]Kind{KindMovie}, 0, count)
		if err != nil {
			t.Fatal(err)
		}
		return page, time.Since(started)
	}

	// The page's counts answer at once, long before the busy catalogs,
	// and start reading them.
	if counted, took := page(e.member, true, 100); took >= busy.delay || counted.Total == 0 || !counted.More {
		t.Errorf("the counts, cold: %d titles, more %v, in %v", counted.Total, counted.More, took)
	}
	// The listing waits for them, then lists the two that answered, then
	// the titles kept, but those the user hid.
	for deadline := time.Now().Add(10 * time.Second); ; {
		if pages, _ := busy.answered("/pick0/"); pages == 1 {
			if pages, _ := busy.answered("/pick1/"); pages == 1 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("the quick catalogs did not answer")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// A restricted user opens the page meanwhile.
	var restricted Page
	var restrictedErr error
	var both sync.WaitGroup
	both.Go(func() {
		restricted, restrictedErr = e.service.GenreTitles(t.Context(), child, []string{"Drama"}, func(option string) bool { return option == "Drama" },
			[]Kind{KindMovie}, 0, 12)
	})
	listed, took := page(e.member, false, 12)
	both.Wait()
	if restrictedErr != nil {
		t.Fatal(restrictedErr)
	}
	if took < wait || took > wait+time.Second {
		t.Errorf("the listing, cold, answered in %v", took)
	}
	got := names(listed.Items)
	if len(got) != 12 || !slices.Equal(slices.Sorted(slices.Values(got[:10])), quick) || !slices.Equal(got[10:], []string{"Ash Road", "Blue Hour"}) ||
		listed.Total != 15 || !listed.More {
		t.Errorf("the listing, cold: %v, total %d, more %v", got, listed.Total, listed.More)
	}
	// The restricted user's has their limits: no title rated R.
	if got := names(restricted.Items); len(got) != 12 || slices.Contains(got, "Pick 0-4") || slices.Contains(got, "Cold Night") ||
		!slices.Equal(got[9:], []string{"Ash Road", "Blue Hour", "Fern Hill"}) || !restricted.More {
		t.Errorf("for a restricted user, cold: %v, total %d", got, restricted.Total)
	}

	// The catalogs are read on, each page once whoever asked for it.
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
	whole, took := page(e.member, false, 100)
	if pages, most := busy.answered("genre=Drama"); pages != catalogHostFetches || most != 1 {
		t.Errorf("%d catalog pages asked, one %d times", pages, most)
	}
	got = names(whole.Items)
	if len(got) != 44 || !slices.Equal(slices.Sorted(slices.Values(got[:40])), all) ||
		!slices.Equal(got[40:], []string{"Ash Road", "Blue Hour", "Cold Night", "Gray Dock"}) || whole.Total != 44 || whole.More {
		t.Errorf("the listing, once read: %d titles in %v, total %d, more %v: %v", len(got), took, whole.Total, whole.More, got)
	}
	if counted, _ := page(e.member, true, 100); counted.Total != 44 || counted.More {
		t.Errorf("the counts, once read: %d titles, more %v", counted.Total, counted.More)
	}
	restrictedWhole, _ := page(child, false, 100)
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
// the read stops waiting, and no further once the server stops.
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
	paged := &fakeAddon{
		manifest: stremio.Manifest{ID: "paged", Name: "Paged", Version: "1", Types: []string{"movie"}, Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{{Type: "movie", ID: "paged", Name: "Paged",
				Extra: []stremio.Extra{{Name: "genre", Options: []string{"Drama", "Crime"}}, {Name: "skip"}}}}},
		catalogs: map[string][]stremio.Meta{"movie/paged": titles},
		pageSize: 5,
	}
	busy := &slowCatalogs{addon: paged, delay: 200 * time.Millisecond, asked: map[string]int{}, served: map[string]int{},
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

	// The request is gone; its four pages are read one after the other.
	open("Drama")
	for deadline := time.Now().Add(10 * time.Second); served("Drama") < 4; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%d pages read once the request was gone", served("Drama"))
		}
	}

	// The server stops after the first page of another read.
	open("Crime")
	for deadline := time.Now().Add(10 * time.Second); served("Crime") < 1; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the read did not start")
		}
	}
	stop()
	time.Sleep(time.Second)
	if pages := served("Crime"); pages >= 4 {
		t.Errorf("%d pages read after the server stopped", pages)
	}
	asked, _ := busy.answered("")
	time.Sleep(500 * time.Millisecond)
	if again, _ := busy.answered(""); again != asked {
		t.Errorf("pages still read after the server stopped: %d, then %d", asked, again)
	}
}
