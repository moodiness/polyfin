package library

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// listedBy lists the movie's versions on behalf of a user, or of
// background work, and checks how many there are and how many times the
// addon was asked for streams in all.
func (e followEnv) listedBy(what string, background bool, versions, asked int) {
	e.t.Helper()
	ctx := e.t.Context()
	if background {
		ctx = Background(ctx)
	}
	found, err := e.service.Versions(ctx, e.member, e.movie)
	if err != nil || len(found) != versions || e.asked("stream") != asked {
		e.t.Fatalf("%s: %d versions, %d requests, %v; want %d, %d", what, len(found), e.asked("stream"), err, versions, asked)
	}
}

// An addon that answers only its summary, its filters having left out
// every stream, is asked again by background work an hour after, then
// twice as long after each further answer, a day at most. A user's
// request asks it once the answer is no longer fresh, and background work
// waits an hour again after it.
func TestBackgroundWorkAsksAnAddonWithNothingLessAndLessOften(t *testing.T) {
	t.Parallel()
	e := repliedEnv(t, func(w http.ResponseWriter, r *http.Request, resource string) {
		if resource != "stream" {
			answerNumbered(w, resource, nil)
			return
		}
		_, _ = w.Write([]byte(`{"streams":[{"name":"Summary","description":"Streams: 10, removed by season and episode: 10"}]}`))
	})
	asked := 1
	e.listedBy("the first answer", true, 0, asked)
	e.wait(15 * time.Minute)
	e.listedBy("a quarter of an hour later", true, 0, asked)
	for _, wait := range []time.Duration{time.Hour, 2 * time.Hour, 4 * time.Hour, 8 * time.Hour, 16 * time.Hour, 24 * time.Hour, 24 * time.Hour} {
		e.wait(wait - 30*time.Minute)
		e.listedBy(fmt.Sprintf("within %v", wait), true, 0, asked)
		e.wait(15*time.Minute + time.Second)
		asked++
		e.listedBy(fmt.Sprintf("after %v", wait), true, 0, asked)
		e.wait(15 * time.Minute)
	}
	asked++
	e.listedBy("a user's request past the answer's freshness", false, 0, asked)
	e.wait(5 * time.Minute)
	e.listedBy("a user's request while its answer is fresh", false, 0, asked)
	e.wait(54 * time.Minute)
	e.listedBy("background work within the hour after it", true, 0, asked)
	e.wait(time.Minute + time.Second)
	asked++
	e.listedBy("background work after the hour after it", true, 0, asked)
}

// An empty answer, kept as the addon's list, keeps background work
// waiting as notices alone do, past the list's freshness. An answer with
// streams to play and the version lists forgotten end the wait. The
// addon's manifest read again has it asked again, the waits growing on.
func TestAnEmptyAnswerHasBackgroundWorkWaitToo(t *testing.T) {
	t.Parallel()
	var streams atomic.Int32
	e := repliedEnv(t, func(w http.ResponseWriter, r *http.Request, resource string) {
		numbers := make([]int, 0, 1)
		if resource == "stream" && streams.Load() > 0 {
			numbers = append(numbers, 1)
		}
		answerNumbered(w, resource, numbers)
	})
	e.listedBy("an empty answer", true, 0, 1)
	e.wait(11 * time.Minute)
	e.listedBy("past the list's freshness", true, 0, 1)
	e.wait(50 * time.Minute)
	e.listedBy("past the hour", true, 0, 2)
	if _, err := e.service.ForgetVersionLists(t.Context()); err != nil {
		t.Fatal(err)
	}
	e.listedBy("the version lists forgotten", true, 0, 3)
	streams.Store(1)
	e.wait(11 * time.Minute)
	e.listedBy("a user's request with a stream", false, 1, 4)
	streams.Store(0)
	e.wait(11 * time.Minute)
	e.listedBy("background work after an answer with a stream", true, 0, 5)
	e.wait(11 * time.Minute)
	e.listedBy("background work after the empty answer", true, 0, 5)
	installed, err := e.addons.Addons(t.Context(), addons.Shared())
	if err != nil || len(installed) != 1 {
		t.Fatalf("addons: %v %v", installed, err)
	}
	if _, _, err := e.addons.Refresh(t.Context(), addons.Shared(), installed[0].ID, false); err != nil {
		t.Fatal(err)
	}
	e.listedBy("the addon's manifest read again", true, 0, 6)
	e.wait(time.Hour + time.Second)
	e.listedBy("an hour after, within twice the hour", true, 0, 6)
}

// Notices alone that refuse the request for its rate are remembered a
// minute only, background work's requests too, and pause the background
// requests to the addon's server.
func TestARefusalIsRememberedAMinuteOnly(t *testing.T) {
	t.Parallel()
	e := newStaleEnv(t)
	e.numbers <- []int{0}
	e.listedBy("a refusal", true, 0, 1)
	installed, err := e.addons.Addons(t.Context(), addons.Shared())
	if err != nil || len(installed) != 1 {
		t.Fatalf("addons: %v %v", installed, err)
	}
	if _, reason, ok := e.service.BackgroundPause(installed[0]); !ok || reason != stremio.FailureRateLimited {
		t.Errorf("the pause: %q %v, want a pause for the rate", reason, ok)
	}
	e.listedBy("within the minute", true, 0, 1)
	e.wait(noticeLife + time.Second)
	e.numbers <- []int{1}
	e.listedBy("after the minute", true, 1, 2)
}
