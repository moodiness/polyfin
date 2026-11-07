package library

import (
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// The watchlist checks keep within the addons' wait rules: an addon that
// answers its streams "too many requests" is not asked again until the
// wait it asks for ends, and its title counts as not settled meanwhile,
// as it does after a failure, which starts no wait.
func TestWatchlistChecksAskNoAddonThatAskedToWait(t *testing.T) {
	t.Parallel()
	// 0 answers one stream, 1 a refusal asking to wait a minute, 2 a
	// failure.
	var mode atomic.Int32
	e := repliedEnv(t, func(w http.ResponseWriter, r *http.Request, resource string) {
		switch mode.Load() {
		case 1:
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
		case 2:
			http.Error(w, "unavailable", http.StatusInternalServerError)
		default:
			answerNumbered(w, resource, []int{1})
		}
	})
	check := func(what string, versions, asked int, complete bool) {
		t.Helper()
		found, done, err := e.service.WatchlistVersions(t.Context(), e.member, e.movie)
		if err != nil || len(found) != versions || done != complete || e.asked("stream") != asked {
			t.Errorf("%s: %d versions, complete %v, %d requests, %v; want %d, %v, %d", what, len(found), done, e.asked("stream"), err,
				versions, complete, asked)
		}
	}
	mode.Store(2)
	check("a failure", 0, 1, false)
	check("after a failure", 0, 2, false)
	mode.Store(1)
	check("a refusal", 0, 3, false)
	mode.Store(0)
	check("during the wait", 0, 3, false)
	e.wait(61 * time.Second)
	check("after the wait", 1, 4, true)
}

// An addon's answer of notices alone that refuse nothing, such as its
// summary of the streams its own filters left out, settles a watchlist
// check: with nothing to play, or with the list it gave before. Notices
// refusing the request for its rate leave the title unsettled, as a
// failure does.
func TestWatchlistChecksSettleOnNoticesThatRefuseNothing(t *testing.T) {
	t.Parallel()
	// 0 answers a summary, 1 a refusal for the rate, 2 one stream.
	var mode atomic.Int32
	e := repliedEnv(t, func(w http.ResponseWriter, r *http.Request, resource string) {
		switch {
		case resource != "stream":
			answerNumbered(w, resource, nil)
		case mode.Load() == 1:
			answerNumbered(w, resource, []int{0})
		case mode.Load() == 2:
			answerNumbered(w, resource, []int{1})
		default:
			_, _ = w.Write([]byte(`{"streams":[{"name":"Summary","description":"12 results filtered out"}]}`))
		}
	})
	check := func(what string, versions, asked int, complete bool) {
		t.Helper()
		found, done, err := e.service.WatchlistVersions(t.Context(), e.member, e.movie)
		if err != nil || len(found) != versions || done != complete || e.asked("stream") != asked {
			t.Errorf("%s: %d versions, complete %v, %d requests, %v; want %d, %v, %d", what, len(found), done, e.asked("stream"), err,
				versions, complete, asked)
		}
	}
	check("a summary", 0, 1, true)
	e.wait(noticeLife + time.Second)
	mode.Store(1)
	check("a refusal", 0, 2, false)
	e.wait(paceWait + time.Second)
	mode.Store(2)
	check("a stream", 1, 3, true)
	e.wait(time.Hour)
	mode.Store(0)
	check("a summary after a list", 1, 4, true)
	e.wait(time.Hour)
	mode.Store(1)
	check("a refusal after a list", 0, 5, false)
}
