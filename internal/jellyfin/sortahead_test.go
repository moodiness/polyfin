package jellyfin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/library"
)

// restartedCounting is the test server as after a restart: a new handler
// over a new library and new IPTV sources, which remember nothing in
// memory and count their statements with counter.
func (s testServer) restartedCounting(t *testing.T, counter *statementCounter) testServer {
	t.Helper()
	config := s.pool.Config()
	config.ConnConfig.Tracer = counter
	counted, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(counted.Close)
	options := s.handler.Options
	options.Library = library.New(counted, s.addons, options.Stremio, options.Logger, s.store.Settings)
	options.Library.SetFollowUps()
	options.IPTV = iptv.New(counted, s.addons, options.Stremio, options.Logger, s.store.Settings)
	options.Library.UseIPTV(options.IPTV)
	options.IPTV.OnChange(options.Library.LineupChanged)
	handler := New(options)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	s.handler, s.url, s.library, s.iptv = handler, server.URL, options.Library, options.IPTV
	return s
}

// sentMatching counts the statements sent since the last reset that match
// pattern.
func (c *statementCounter) sentMatching(pattern *regexp.Regexp) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for sql, times := range c.sent {
		if pattern.MatchString(sql) {
			n += times
		}
	}
	return n
}

// readsLibraries matches the statements that read a library's titles
// whole: an IPTV source's stored list, an addon's catalog pages kept, when
// its titles were first seen, and the records of what they list.
var readsLibraries = regexp.MustCompile(`FROM iptv_titles|FROM catalog_pages|FROM catalog_arrivals|JOIN item_records`)

// Once Polyfin started and read its libraries' first pages, it sorts the
// libraries its users see ahead: the first sorted listing after a restart,
// of an IPTV source's movies by name or of an addon's library by date
// added, reads none of their titles, as the next listings do not, and
// answers the same page as a listing that sorts them itself. Users who see
// a library alike share its order; one who hid a title gets their own,
// without it.
func TestFirstSortedListingsAfterARestartFindTheirOrder(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("member", nil)
	s.user("other", nil)
	s.user("hider", nil)
	token, other, hider := s.signIn("member", "tv"), s.signIn("other", "tv"), s.signIn("hider", "tv")
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
	stored := s.viewsByType(t, token)["movies"]
	held := make(chan struct{})
	close(held)
	manifest, _ := ratedAddon(t, held)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), manifest, false); err != nil {
		t.Fatal(err)
	}
	rated := s.viewIDs(t, token)["Rated"]
	if stored == "" || rated == "" || stored == rated {
		t.Fatalf("libraries: %v", s.viewIDs(t, token))
	}
	byName := "/Items?" + url.Values{"ParentId": {stored}, "IncludeItemTypes": {"Movie"}, "Recursive": {"true"},
		"SortBy": {"SortName"}, "SortOrder": {"Ascending"}, "StartIndex": {"2"}, "Limit": {"3"}}.Encode()
	byDate := "/Items?" + url.Values{"ParentId": {rated}, "IncludeItemTypes": {"Movie"}, "Recursive": {"true"},
		"SortBy": {"DateCreated,SortName"}, "SortOrder": {"Descending"}, "StartIndex": {"40"}, "Limit": {"10"}}.Encode()
	// The addon's catalog is read through and kept.
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		var page QueryResult
		if s.get(t, byDate, token, &page) == http.StatusOK && page.TotalRecordCount == 100 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the catalog was never read through")
		}
	}
	var hidden QueryResult
	s.get(t, "/Items?ParentId="+stored+"&IncludeItemTypes=Movie&Recursive=true&SortBy=SortName&Limit=1", hider, &hidden)
	if len(hidden.Items) != 1 || hidden.Items[0].Name != "Alpha" {
		t.Fatalf("the first movie: %v", itemNames(hidden.Items))
	}
	if status, body := s.call(http.MethodPost, "/Polyfin/UserItems/"+hidden.Items[0].Id+"/Hidden", app("tv", hider), nil); status != http.StatusNoContent {
		t.Fatalf("hiding Alpha: %d %s", status, body)
	}

	type listing struct {
		names []string
		total int
		reads int
		took  time.Duration
	}
	list := func(s testServer, counter *statementCounter, path, token string) listing {
		t.Helper()
		counter.reset()
		started := time.Now()
		names, total := s.pageNames(t, path, token)
		return listing{names, total, counter.sentMatching(readsLibraries), time.Since(started)}
	}

	// After a restart that read no first page yet, the first listings
	// sort the libraries themselves.
	counter := &statementCounter{}
	cold := s.restartedCounting(t, counter)
	coldName, coldDate := list(cold, counter, byName, token), list(cold, counter, byDate, token)
	if want := []string{"Charlie", "Delta", "Echo"}; !slices.Equal(coldName.names, want) || coldName.total != len(movies) || coldName.reads == 0 {
		t.Errorf("by name, sorted by the listing: %v of %d, %d reads; want %v of %d", coldName.names, coldName.total, coldName.reads, want, len(movies))
	}
	if len(coldDate.names) != 10 || coldDate.total != 100 || coldDate.reads == 0 {
		t.Errorf("by date added, sorted by the listing: %v of %d, %d reads", coldDate.names, coldDate.total, coldDate.reads)
	}

	// After a restart that read them, every order is found.
	counter = &statementCounter{}
	ahead := s.restartedCounting(t, counter)
	if err := ahead.library.Warm(t.Context()); err != nil {
		t.Fatal(err)
	}
	kept := ahead.handler.nameOrders.Len()
	for _, c := range []struct {
		name, path, token string
		want              listing
	}{
		{"by name", byName, token, coldName},
		{"by date added", byDate, token, coldDate},
		{"another user by name", byName, other, coldName},
		{"another user by date added", byDate, other, coldDate},
	} {
		first := list(ahead, counter, c.path, c.token)
		next := list(ahead, counter, c.path, c.token)
		if !slices.Equal(first.names, c.want.names) || first.total != c.want.total || first.reads != 0 || next.reads != 0 {
			t.Errorf("%s, sorted ahead: %v of %d, %d reads then %d; want %v of %d, none", c.name, first.names, first.total, first.reads,
				next.reads, c.want.names, c.want.total)
		}
		t.Logf("%s: the first listing after a restart took %v sorted ahead, %v sorting itself; the next one %v", c.name, first.took,
			c.want.took, next.took)
	}
	// Alpha hidden, Bravo comes first, sorted ahead too.
	if hid := list(ahead, counter, byName, hider); !slices.Equal(hid.names, []string{"Delta", "Echo", "Foxtrot"}) ||
		hid.total != len(movies)-1 || hid.reads != 0 {
		t.Errorf("by name, Alpha hidden: %v of %d, %d reads", hid.names, hid.total, hid.reads)
	}
	// The two users share the order by date added sorted from the one by
	// name.
	if n := ahead.handler.nameOrders.Len(); n != kept+1 {
		t.Errorf("%d orders kept, %d sorted ahead", n, kept)
	}
}

// An order read again stays past the 2 minutes it once lasted: a listing
// 10 minutes after the last, its titles read 11 minutes before, reads
// none of them and answers without waiting for a sort, the order being
// sorted again in the background, after which listings read none either.
// An order left unread for orderIdle is sorted again by the next listing.
func TestSortedOrdersStayWhileListingsReadThem(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("member", nil)
	token := s.signIn("member", "tv")
	on, off := true, false
	movies := []sortedMovie{{"Delta", "1700000600", "1999", "7.1"}, {"Alpha", "1700000100", "2005", "8"}, {"Echo", "1700000700", "", ""},
		{"Bravo", "1700000300", "2005", "6.5"}, {"Charlie", "1700000200", "1987", "9.2"}}
	if _, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Box", Account: iptv.Account{Kind: addons.KindXtream,
		Server: sortingProvider(t, movies), Username: "user", Password: "secret"}, Options: &iptv.OptionsPatch{Movies: &on, LiveTv: &off}},
		false); err != nil {
		t.Fatal(err)
	}
	path := "/Items?" + url.Values{"ParentId": {s.viewsByType(t, token)["movies"]}, "IncludeItemTypes": {"Movie"}, "Recursive": {"true"},
		"SortBy": {"SortName"}, "StartIndex": {"1"}, "Limit": {"3"}}.Encode()
	counter := &statementCounter{}
	r := s.restartedCounting(t, counter)
	var clock atomic.Pointer[time.Time]
	started := time.Now()
	clock.Store(&started)
	r.handler.now = func() time.Time { return *clock.Load() }
	after := func(d time.Duration) {
		next := clock.Load().Add(d)
		clock.Store(&next)
	}
	want := []string{"Bravo", "Charlie", "Delta"}
	list := func(when string, reads bool) {
		t.Helper()
		counter.reset()
		names, total := r.pageNames(t, path, token)
		if n := counter.sentMatching(readsLibraries); !slices.Equal(names, want) || total != len(movies) || n > 0 != reads {
			t.Errorf("%s: %v of %d, %d reads of the titles, want %v of %d, reading them %t", when, names, total, n, want, len(movies), reads)
		}
	}
	list("first", true)
	after(90 * time.Second)
	list("90 seconds later", false)
	// No place to sort: the listing answers from the order kept.
	after(10 * time.Minute)
	for range nameSortsAtOnce {
		r.handler.nameBuilds.sorts <- struct{}{}
	}
	list("10 minutes later", false)
	for range nameSortsAtOnce {
		<-r.handler.nameBuilds.sorts
	}
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		r.handler.nameBuilds.mu.Lock()
		sorting := len(r.handler.nameBuilds.builds)
		r.handler.nameBuilds.mu.Unlock()
		if counter.sentMatching(readsLibraries) > 0 && sorting == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the order was never sorted again in the background")
		}
	}
	list("once sorted again", false)
	after(orderIdle + time.Minute)
	list("unread for longer than orders stay", true)
}
