package jellyfin

import (
	"context"
	"maps"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// queueWatch is an answer's writer that records how many titles the home
// rows' queue held when the answer's body was first written, -1 until it
// is.
type queueWatch struct {
	http.ResponseWriter
	queued  func() int
	atWrite *atomic.Int64
}

func (w queueWatch) Write(p []byte) (int, error) {
	w.atWrite.CompareAndSwap(-1, int64(w.queued()))
	return w.ResponseWriter.Write(p)
}

// Continue Watching and Next Up queue their titles once their answer is
// written, not while they describe it, and the titles a row queued share
// one context, its request's views.
func TestHomeRowsQueueTheirTitlesOnceAnswered(t *testing.T) {
	t.Parallel()
	rs := rowsOn(t, false)
	rs.setting(t, func(s *accounts.Settings) { s.PrepareAhead = true })
	// The titles wait until the rows are answered: what is queued stays
	// queued, whatever the pace of the workers.
	release := make(chan struct{})
	var mu sync.Mutex
	rows := map[context.Context]int{}
	listings := newListings(func(row context.Context, _ accounts.User, _ accounts.ID, _ bool) {
		<-release
		mu.Lock()
		rows[row]++
		mu.Unlock()
	})
	rs.handler.listings = listings
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	queued := func() int {
		listings.mu.Lock()
		defer listings.mu.Unlock()
		return len(listings.waiting)
	}
	var atWrite atomic.Int64
	watched := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rs.handler.ServeHTTP(queueWatch{ResponseWriter: w, queued: queued, atWrite: &atWrite}, r)
	}))
	t.Cleanup(watched.Close)
	answered := rs.testServer
	answered.url = watched.URL

	before := 0
	for _, path := range []string{"/Shows/NextUp", "/UserItems/Resume"} {
		atWrite.Store(-1)
		var page QueryResult
		if status := answered.get(t, path, rs.token, &page); status != http.StatusOK || len(page.Items) != rowTitles {
			t.Fatalf("%s: %d, %d titles", path, status, len(page.Items))
		}
		if n := atWrite.Load(); n != int64(before) {
			t.Errorf("%s: %d titles queued when the answer was written, want %d", path, n, before)
		}
		before += homeRowTitles
		if n := queued(); n != before {
			t.Errorf("%s: %d titles queued once answered, want %d", path, n, before)
		}
	}
	close(release)
	rs.listedAhead(t)
	mu.Lock()
	defer mu.Unlock()
	if len(rows) != 2 {
		t.Errorf("the rows' titles listed under %d contexts, want one per row", len(rows))
	}
	for _, n := range rows {
		if n != homeRowTitles {
			t.Errorf("a row's context listed %d titles, want %d", n, homeRowTitles)
		}
	}
}

// buildsViews matches the statements a user's view is built with: their
// hidden titles and their smart lists.
var buildsViews = regexp.MustCompile(`FROM hidden_titles WHERE user_id|FROM smart_lists`)

// After a restart, the titles Next Up and Continue Watching queue are
// listed under the view their row's request built: none builds its own,
// as each did, its hidden titles and smart lists read once per title.
func TestHomeRowsQueueBuildsNoViewOfItsOwn(t *testing.T) {
	t.Parallel()
	rs := rowsOn(t, false)
	rs.setting(t, func(s *accounts.Settings) { s.PrepareAhead = true })
	counter := &statementCounter{}
	restarted := rs
	restarted.testServer = rs.restartedCounting(t, counter)
	restarted.library.SetPacing(1000, time.Minute)
	// Listed, not prepared: preparing reads what analyses need, which
	// TestHomeRowsPrepareTheirFirstTitles proves.
	handler := restarted.handler
	handler.listings = newListings(func(row context.Context, user accounts.User, title accounts.ID, _ bool) {
		handler.listVersions(row, user, title, false)
	})
	t.Cleanup(func() { restarted.listedAhead(t) })

	asked := map[string]int{}
	for _, path := range []string{"/Shows/NextUp", "/UserItems/Resume"} {
		counter.reset()
		titles := restarted.row(t, path)
		restarted.listedAhead(t)
		for _, id := range titles[:homeRowTitles] {
			asked[id]++
		}
		t.Logf("%s and its queue: %d statements\n%s", path, counter.statements.Load(), counter.mostSent(12))
		if views := counter.sentMatching(buildsViews); views > 2 {
			t.Errorf("%s and its queue: %d statements building views, want 2 at most, the row's", path, views)
		}
	}
	if got := rs.addon.requests(); !maps.Equal(got, asked) {
		t.Errorf("the queues asked %v, want %v", got, asked)
	}
}
