package library

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// aheadAddon is the addon of the tests of rowAhead.
var aheadAddon = accounts.ID{7}

// aheadRecorder is a rowAhead whose requests the test records, on the
// clock of synctest: the titles asked, in order, when, and the most asked
// at once. A title in fresh needs no request; while paused, the addon's
// server is asked nothing ahead; refuseAt pauses it at that request, as a
// refusal for the rate does; hold, when set, holds each request until it
// receives.
type aheadRecorder struct {
	r *rowAhead

	mu       sync.Mutex
	asked    []string
	at       []time.Time
	running  int
	most     int
	fresh    map[string]bool
	paused   bool
	refuseAt int
	hold     chan struct{}
}

func newAheadRecorder() *aheadRecorder {
	rec := &aheadRecorder{fresh: map[string]bool{}}
	rec.r = &rowAhead{
		now: time.Now,
		paused: func(string) bool {
			rec.mu.Lock()
			defer rec.mu.Unlock()
			return rec.paused
		},
		needless: func(source streamSource) bool {
			rec.mu.Lock()
			defer rec.mu.Unlock()
			return rec.fresh[source.id]
		},
		ask:    rec.ask,
		logger: slog.New(slog.DiscardHandler),
		addons: map[accounts.ID]*addonAhead{},
	}
	return rec
}

func (rec *aheadRecorder) ask(source streamSource) {
	rec.mu.Lock()
	rec.asked, rec.at = append(rec.asked, source.id), append(rec.at, time.Now())
	rec.running++
	rec.most = max(rec.most, rec.running)
	hold := rec.hold
	if len(rec.asked) == rec.refuseAt {
		rec.paused = true
	}
	rec.mu.Unlock()
	if hold != nil {
		<-hold
	}
	rec.mu.Lock()
	rec.running--
	rec.mu.Unlock()
}

// add queues titles named after ids, at place.
func (rec *aheadRecorder) add(place int, ids ...string) {
	for _, id := range ids {
		rec.r.add(streamSource{installed: installed{addon: addons.Addon{ID: aheadAddon, Kind: addons.KindStremio,
			ManifestURL: "https://streams.example/manifest.json"}}, contentType: "movie", id: id}, place)
	}
}

// atOnce returns the most requests that were under way at once.
func (rec *aheadRecorder) atOnce() int {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return rec.most
}

func (rec *aheadRecorder) requests() []string {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return slices.Clone(rec.asked)
}

// waiting counts the titles waiting for the addon.
func (rec *aheadRecorder) waiting() int {
	rec.r.mu.Lock()
	defer rec.r.mu.Unlock()
	if a, ok := rec.r.addons[aheadAddon]; ok {
		return len(a.queue)
	}
	return 0
}

// mostWithin returns the most requests sent within any window, from the
// times recorded.
func (rec *aheadRecorder) mostWithin(window time.Duration) int {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	most := 0
	for i, from := range rec.at {
		n := 0
		for _, at := range rec.at[i:] {
			if at.Sub(from) < window {
				n++
			}
		}
		most = max(most, n)
	}
	return most
}

func ids(prefix string, n int) []string {
	names := make([]string, n)
	for i := range names {
		names[i] = prefix + string(rune('a'+i/26)) + string(rune('a'+i%26))
	}
	return names
}

// An addon is asked ahead rowAheadPerMinute titles a minute and
// rowAheadPerHour an hour at most: the titles beyond a minute's wait for
// the next, those beyond the hour's are dropped, and the rows asked for
// once the hour passed are listed again.
func TestTitlesListedAheadKeepWithinTheAddonsBudget(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		rec := newAheadRecorder()
		rec.add(0, ids("first", rowAheadQueued)...)
		synctest.Wait()
		if n := len(rec.requests()); n != rowAheadPerMinute {
			t.Errorf("%d requests at once, want %d", n, rowAheadPerMinute)
		}
		time.Sleep(10 * time.Minute)
		if n := len(rec.requests()); n != rowAheadQueued {
			t.Errorf("%d requests within 10 minutes, want the %d queued", n, rowAheadQueued)
		}
		rec.add(0, ids("second", rowAheadQueued)...)
		time.Sleep(20 * time.Minute)
		if n := len(rec.requests()); n != rowAheadPerHour {
			t.Errorf("%d requests within half an hour, want %d", n, rowAheadPerHour)
		}
		if n := rec.waiting(); n != 0 {
			t.Errorf("%d titles still wait once the hour's requests were sent, want them dropped", n)
		}
		if n := rec.mostWithin(time.Minute); n > rowAheadPerMinute {
			t.Errorf("%d requests within a minute, want %d at most", n, rowAheadPerMinute)
		}
		if most := rec.atOnce(); most != 1 {
			t.Errorf("%d requests at once, want 1", most)
		}
		time.Sleep(31 * time.Minute)
		rec.add(0, "later")
		rec.r.workers.Wait()
		if got := rec.requests(); len(got) != rowAheadPerHour+1 || got[len(got)-1] != "later" {
			t.Errorf("a row asked for once the hour passed: %d requests, the last %q; want %d, later", len(got), got[len(got)-1],
				rowAheadPerHour+1)
		}
	})
}

// A refusal for the rate, which pauses the server's background requests,
// drops at once the titles waiting for the addon, and the rows asked for
// while the pause runs have none asked; once it ends, they are again.
func TestTitlesListedAheadStopAtARefusal(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		rec := newAheadRecorder()
		rec.refuseAt = 3
		start := time.Now()
		rec.add(0, ids("row", 10)...)
		rec.r.workers.Wait()
		if n := len(rec.requests()); n != 3 {
			t.Errorf("%d requests, want 3: none after the refusal", n)
		}
		if n := rec.waiting(); n != 0 || !time.Now().Equal(start) {
			t.Errorf("%d titles wait after %v, want none at once", n, time.Since(start))
		}
		rec.add(0, ids("paused", 5)...)
		rec.r.workers.Wait()
		if n := len(rec.requests()); n != 3 {
			t.Errorf("%d requests while the pause runs, want 3", n)
		}
		rec.mu.Lock()
		rec.paused = false
		rec.mu.Unlock()
		rec.add(0, "after")
		rec.r.workers.Wait()
		if got := rec.requests(); len(got) != 4 || got[3] != "after" {
			t.Errorf("requests %v once the pause ended, want after asked", got)
		}
	})
}

// A title whose list needs no request, fresh or asked already, is not
// asked and costs nothing of the budget, even when it became so while it
// waited for its turn.
func TestTitlesListedAheadSkipTheFreshOnes(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		rec := newAheadRecorder()
		rec.fresh["kept"] = true
		rec.hold = make(chan struct{})
		rec.add(0, "first", "kept", "opened", "last")
		synctest.Wait()
		// The title is opened while the first is asked.
		rec.mu.Lock()
		rec.fresh["opened"] = true
		rec.mu.Unlock()
		close(rec.hold)
		rec.r.workers.Wait()
		if got := rec.requests(); !slices.Equal(got, []string{"first", "last"}) {
			t.Errorf("asked %v, want first and last", got)
		}
		if a := rec.r.addons[aheadAddon]; len(a.sent) != 2 {
			t.Errorf("%d requests counted, want 2", len(a.sent))
		}
	})
}

// The titles first in their rows are asked first, whatever the row, one
// at a time, in the order they came for the same place.
func TestTitlesListedAheadGoInTheirPlacesOrder(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		rec := newAheadRecorder()
		rec.hold = make(chan struct{})
		rec.add(0, "under way")
		synctest.Wait()
		rec.add(2, "row1-third")
		rec.add(1, "row1-second")
		rec.add(0, "row2-first")
		rec.add(1, "row2-second")
		rec.add(0, "row3-first")
		for range 6 {
			rec.hold <- struct{}{}
		}
		rec.r.workers.Wait()
		want := []string{"under way", "row2-first", "row3-first", "row1-second", "row2-second", "row1-third"}
		if got := rec.requests(); !slices.Equal(got, want) {
			t.Errorf("asked %v, want %v", got, want)
		}
		if most := rec.atOnce(); most != 1 {
			t.Errorf("%d requests at once, want 1", most)
		}
	})
}

// A follow-up of an answer to a list ahead is one of the addon's requests
// ahead: it waits for the addon's turn and counts, ends once the hour's
// requests were sent, and goes at once, uncounted, when a play hurries it.
func TestFollowUpsOfTitlesListedAheadKeepWithinTheBudget(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		rec := newAheadRecorder()
		rec.add(0, ids("row", rowAheadPerMinute)...)
		rec.r.workers.Wait()
		a := rec.r.addons[aheadAddon]
		start := time.Now()
		release, ok, hurried := rec.r.followUp(t.Context(), aheadAddon, nil)
		if !ok || hurried || time.Since(start) != time.Minute || len(a.sent) != rowAheadPerMinute+1 {
			t.Errorf("a follow-up past the minute's requests: ok %v, hurried %v after %v, %d counted; want it a minute later, counted",
				ok, hurried, time.Since(start), len(a.sent))
		}
		release()
		hurry := make(chan struct{}, 1)
		a.sent = slices.Repeat([]time.Time{time.Now()}, rowAheadPerMinute)
		go func() {
			time.Sleep(time.Second)
			hurry <- struct{}{}
		}()
		start = time.Now()
		if _, ok, hurried := rec.r.followUp(t.Context(), aheadAddon, hurry); ok || !hurried || time.Since(start) != time.Second ||
			len(a.sent) != rowAheadPerMinute {
			t.Errorf("a hurried follow-up: ok %v, hurried %v after %v, %d counted; want it hurried at once, uncounted", ok, hurried,
				time.Since(start), len(a.sent))
		}
		a.sent = slices.Repeat([]time.Time{time.Now().Add(-30 * time.Minute)}, rowAheadPerHour)
		if _, ok, _ := rec.r.followUp(t.Context(), aheadAddon, nil); ok {
			t.Error("a follow-up past the hour's requests was asked")
		}
		if _, ok, _ := rec.r.followUp(t.Context(), accounts.ID{8}, nil); ok {
			t.Error("a follow-up of an addon listing nothing ahead counted as one")
		}
	})
}

// A list ahead's request is background work that lets the other go first,
// and its follow-ups know it.
func TestTitlesListedAheadAreAskedAfterOtherWork(t *testing.T) {
	t.Parallel()
	ctx, cancel := rowAheadContext(context.Background())
	defer cancel()
	if mark := markOf(ctx); !mark.background || !mark.later || mark.paced || !rowAheadOf(ctx) {
		t.Errorf("a list ahead's request is marked %+v, list ahead %v; want later background work's", mark, rowAheadOf(ctx))
	}
}

// A title listed ahead is asked of its addon once: not again while its
// list is fresh, in memory or, after a restart, in the database, nor
// while the addon's server pauses after a refusal for its rate.
func TestATitleListedAheadIsAskedOnceWhileItsListIsFresh(t *testing.T) {
	t.Parallel()
	refuse := false
	var mu sync.Mutex
	e := repliedEnv(t, func(w http.ResponseWriter, r *http.Request, resource string) {
		mu.Lock()
		refusing := refuse
		mu.Unlock()
		if refusing {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		answerNumbered(w, resource, []int{1})
	})
	listAhead := func(what string, asked int) {
		t.Helper()
		e.service.ListRowAhead(t.Context(), e.member, []RowTitle{{ID: e.movie}})
		e.service.rowAhead.workers.Wait()
		if n := e.asked("stream"); n != asked {
			t.Errorf("%s: %d stream requests, want %d", what, n, asked)
		}
	}
	listAhead("first listed", 1)
	listAhead("listed again", 1)
	e.env = e.restarted()
	listAhead("after a restart", 1)
	if versions := e.known(); versions != 1 {
		t.Errorf("%d versions known after a restart, want 1", versions)
	}
	e.wait(time.Duration(e.users.Settings().VersionListMinutes)*time.Minute + time.Second)
	mu.Lock()
	refuse = true
	mu.Unlock()
	listAhead("once expired, refused", 2)
	listAhead("while the pause runs", 2)
}
