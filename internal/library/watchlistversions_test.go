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
