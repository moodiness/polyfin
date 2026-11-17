package jellyfin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"
)

// nameOrderShelf is a member's IPTV library of five movies, with the
// path of its order by name.
func nameOrderShelf(t *testing.T) (shelf, string) {
	t.Helper()
	x := newStoredShelf(t, []string{"Delta", "Alpha", "Echo", "Bravo", "Charlie"}, 0)
	return x, "/Polyfin/NameOrder?" + url.Values{"ParentId": {x.viewsByType(t, x.account)["movies"]}, "Recursive": {"true"}}.Encode()
}

// listByName reads a page of the order by name narrowed by more, and
// returns its total.
func (x shelf) listByName(t *testing.T, path, more string) int {
	t.Helper()
	status, data := x.call(http.MethodGet, path+more, app("tv", x.account), nil)
	var page QueryResult
	if status != http.StatusOK || json.Unmarshal(data, &page) != nil {
		t.Fatalf("%s%s: %d %s", path, more, status, data)
	}
	return page.TotalRecordCount
}

// askByName sends a listing by name with ctx, in the background, and
// returns when it ends.
func (x shelf) askByName(ctx context.Context, path string) <-chan int {
	ended := make(chan int, 1)
	go func() {
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, x.url+path, nil)
		request.Header.Set("Authorization", app("tv", x.account))
		response, err := testClient.Do(request)
		if err != nil {
			ended <- 0
			return
		}
		_ = response.Body.Close()
		ended <- response.StatusCode
	}()
	return ended
}

// Listings that ask for kinds no title has, or name a kind twice, read the
// order every other listing of the library reads.
func TestNameOrderOneOrderForEveryKind(t *testing.T) {
	t.Parallel()
	x, path := nameOrderShelf(t)
	for _, kinds := range []string{"Movie,a1", "Movie,a2", "Movie,Movie", "Series,Movie"} {
		if total := x.listByName(t, path, "&Limit=0&IncludeItemTypes="+kinds); total != 5 {
			t.Errorf("%s: %d titles", kinds, total)
		}
	}
	if kept := x.handler.nameOrders.Len(); kept != 1 {
		t.Errorf("%d orders kept", kept)
	}
}

// Each listing keeps the kinds its filters ask for, whichever listing the
// order was sorted for.
func TestNameOrderFollowsEachListingsFilters(t *testing.T) {
	t.Parallel()
	x, path := nameOrderShelf(t)
	for _, tc := range []struct {
		more  string
		total int
	}{{"&IncludeItemTypes=Movie&ExcludeItemTypes=Movie", 0}, {"&IncludeItemTypes=Movie", 5}, {"&IncludeItemTypes=Series", 0},
		{"&IncludeItemTypes=Movie&MediaTypes=Audio", 0}, {"&IncludeItemTypes=Movie,Series", 5}, {"&IncludeItemTypes=Movie&Filters=IsFolder", 0}} {
		if total := x.listByName(t, path, tc.more); total != tc.total {
			t.Errorf("%s: %d titles, want %d", tc.more, total, tc.total)
		}
	}
}

// building is how many orders are being sorted, or waiting to be.
func (x shelf) building() int {
	x.handler.nameBuilds.mu.Lock()
	defer x.handler.nameBuilds.mu.Unlock()
	return len(x.handler.nameBuilds.builds)
}

// until waits up to within for done, and reports whether it came.
func until(within time.Duration, done func() bool) bool {
	for deadline := time.Now().Add(within); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if done() {
			return true
		}
	}
	return false
}

// A sort waits while as many sorts as the bound run, and runs once one
// ends; one no listing waits for any more stops, and keeps no order.
func TestNameOrderSortsWithinTheBoundAndStopWithTheirListings(t *testing.T) {
	t.Parallel()
	x, path := nameOrderShelf(t)
	path += "&IncludeItemTypes=Movie"
	for range nameSortsAtOnce {
		x.handler.nameBuilds.sorts <- struct{}{}
	}
	gaveUp, giveUp := context.WithCancel(t.Context())
	ended := x.askByName(gaveUp, path)
	if !until(10*time.Second, func() bool { return x.building() == 1 }) {
		t.Fatal("the listing asked for no sort")
	}
	giveUp()
	<-ended
	if !until(10*time.Second, func() bool { return x.building() == 0 }) {
		t.Fatal("the sort no listing waits for still runs")
	}
	if until(2*time.Second, func() bool { return x.handler.nameOrders.Len() > 0 }) {
		t.Fatal("the sort no listing waited for kept an order")
	}
	waiting := x.askByName(t.Context(), path)
	if !until(10*time.Second, func() bool { return x.building() == 1 }) {
		t.Fatal("the listing asked for no sort")
	}
	if until(2*time.Second, func() bool { return x.handler.nameOrders.Len() > 0 }) {
		t.Fatal("a sort beyond the bound ran")
	}
	<-x.handler.nameBuilds.sorts
	if status := <-waiting; status != http.StatusOK {
		t.Errorf("the sort once one ended: %d", status)
	}
	<-x.handler.nameBuilds.sorts
	if kept := x.handler.nameOrders.Len(); kept != 1 {
		t.Errorf("%d orders kept", kept)
	}
}
