package library

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// failing is a scripted answer that fails.
const failing = -1

// scriptedReplies answers each stream or subtitle request with the next
// count sent on replies, holding it until it comes: that many streams or
// subtitles, or a failure for failing.
func scriptedReplies(replies <-chan int) func(http.ResponseWriter, *http.Request, string) {
	return func(w http.ResponseWriter, r *http.Request, resource string) {
		var n int
		select {
		case n = <-replies:
		case <-r.Context().Done():
			return
		}
		if n == failing {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		numbers := make([]int, n)
		for i := range numbers {
			numbers[i] = i + 1
		}
		answerNumbered(w, resource, numbers)
	}
}

// answerNumbered answers a request for a resource with the streams or
// subtitles numbered as numbers lists: stream 2 is the second stream of an
// answer of scriptedReplies, whatever the answer. Stream 0 is a notice,
// with nothing to play, such as addons send when they limit requests.
func answerNumbered(w http.ResponseWriter, resource string, numbers []int) {
	if resource == "stream" {
		streams := make([]stremio.Stream, len(numbers))
		for i, n := range numbers {
			streams[i] = stremio.Stream{Name: fmt.Sprintf("Source %d", n), URL: fmt.Sprintf("https://cdn.example/movie/%d.mkv", n)}
			if n == 0 {
				streams[i] = stremio.Stream{Name: "Notice", Description: "Rate limit exceeded", ExternalURL: "https://addon.example/"}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"streams": streams})
		return
	}
	subtitles := make([]stremio.Subtitle, len(numbers))
	for i, n := range numbers {
		subtitles[i] = stremio.Subtitle{ID: stremio.Text(fmt.Sprint(n)), URL: fmt.Sprintf("https://cdn.example/movie/%d.srt", n), Lang: "eng"}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"subtitles": subtitles})
}

// followEnv serves one movie from an addon whose streams and subtitles
// the test scripts, followed up after short delays, on a clock the test
// moves.
type followEnv struct {
	env
	addon   *fakeAddon
	replies chan int
	movie   accounts.ID
	elapsed *atomic.Int64
}

func newFollowEnv(t *testing.T, delays ...time.Duration) followEnv {
	t.Helper()
	replies := make(chan int, 10)
	e := repliedEnv(t, scriptedReplies(replies), delays...)
	e.replies = replies
	return e
}

// repliedEnv is a followEnv whose addon answers its stream and subtitle
// requests with reply.
func repliedEnv(t *testing.T, reply func(http.ResponseWriter, *http.Request, string), delays ...time.Duration) followEnv {
	t.Helper()
	e := newEnv(t)
	e.service.followUpDelays = delays
	elapsed := new(atomic.Int64)
	e.service.now = func() time.Time { return time.Now().Add(time.Duration(elapsed.Load())) }
	movies := titles("movie", 1)
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "gathered", Name: "Gathered", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}, {Name: "subtitles"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{"movie/top": movies},
		metas:    map[string]stremio.Meta{"movie/" + movies[0].ID: movies[0]},
		reply:    reply,
	}
	e.install(addons.Shared(), addon)
	page, err := e.service.Children(t.Context(), e.member, e.library(e.member, "Top").ID, 0, 10, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("listing: %+v %v", page, err)
	}
	return followEnv{env: e, addon: addon, movie: page.Items[0].ID, elapsed: elapsed}
}

// wait moves the clock d ahead.
func (e followEnv) wait(d time.Duration) { e.elapsed.Add(int64(d)) }

// asked counts the addon's requests for a resource.
func (e followEnv) asked(resource string) int {
	e.addon.mu.Lock()
	defer e.addon.mu.Unlock()
	n := 0
	for _, request := range e.addon.requests {
		if strings.HasPrefix(request, resource+"/") {
			n++
		}
	}
	return n
}

// scheduled counts the schedules running.
func (e followEnv) scheduled() int {
	e.service.followUps.mu.Lock()
	defer e.service.followUps.mu.Unlock()
	return len(e.service.followUps.running)
}

// hurry has every schedule running ask its next follow-up at once.
func (e followEnv) hurry() {
	e.service.followUps.mu.Lock()
	defer e.service.followUps.mu.Unlock()
	for _, f := range e.service.followUps.running {
		select {
		case f.hurry <- struct{}{}:
		default:
		}
	}
}

// schedule waits for the one schedule running, and returns it.
func (e followEnv) schedule() *followUp {
	e.t.Helper()
	var f *followUp
	e.eventually("a schedule", func() bool {
		e.service.followUps.mu.Lock()
		defer e.service.followUps.mu.Unlock()
		for _, running := range e.service.followUps.running {
			f = running
		}
		return len(e.service.followUps.running) == 1
	})
	return f
}

// known counts the versions known, asking no addon.
func (e followEnv) known() int {
	e.t.Helper()
	versions, err := e.service.KnownVersions(e.t.Context(), e.member, e.movie)
	if err != nil {
		e.t.Fatal(err)
	}
	return len(versions)
}

func (e followEnv) eventually(what string, done func() bool) {
	e.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			e.t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// settles waits for the schedules to end, then checks that the addon is
// asked no more.
func (e followEnv) settles(resource string, requests int) {
	e.t.Helper()
	e.eventually("the schedule to end", func() bool { return e.scheduled() == 0 && e.asked(resource) == requests })
	time.Sleep(150 * time.Millisecond)
	if got := e.asked(resource); got != requests {
		e.t.Errorf("%d %s requests after the schedule ended, want %d", got, resource, requests)
	}
}

func TestFollowUpsKeepTheLongerStreamList(t *testing.T) {
	e := newFollowEnv(t, 20*time.Millisecond, 50*time.Millisecond)
	e.replies <- 1
	if versions, err := e.service.Versions(t.Context(), e.member, e.movie); err != nil || len(versions) != 1 {
		t.Fatalf("first answer: %+v %v", versions, err)
	}
	// Asked again, the addon lists more: they replace the list.
	e.replies <- 3
	e.eventually("the first follow-up's versions", func() bool { return e.known() == 3 })
	// Its answers grew: it is asked once more, and that answer, which does
	// not list more, ends the schedule.
	e.eventually("the second follow-up", func() bool { return e.asked("stream") == 3 })
	if e.scheduled() != 1 {
		t.Error("the second follow-up runs out of any schedule")
	}
	e.replies <- 3
	e.settles("stream", 3)
	if got := e.known(); got != 3 {
		t.Errorf("%d versions after the follow-ups, want 3", got)
	}
}

func TestFollowUpsNeverShortenAList(t *testing.T) {
	for _, tc := range []struct {
		name   string
		second int
		want   int
	}{
		{"shorter answer", 1, 3},
		{"failing answer", failing, 3},
		{"same answer", 3, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newFollowEnv(t, 20*time.Millisecond, 20*time.Millisecond)
			e.replies <- 3
			e.replies <- tc.second
			if versions, err := e.service.Versions(t.Context(), e.member, e.movie); err != nil || len(versions) != 3 {
				t.Fatalf("first answer: %+v %v", versions, err)
			}
			// The follow-up asks, and stops at its answer.
			e.settles("stream", 2)
			if got := e.known(); got != tc.want {
				t.Errorf("%d versions, want %d", got, tc.want)
			}
		})
	}
}

func TestAListServedFromTheCacheIsNotFollowedUp(t *testing.T) {
	e := newFollowEnv(t, 20*time.Millisecond, 20*time.Millisecond)
	e.replies <- 2
	e.replies <- 2
	e.service.Versions(t.Context(), e.member, e.movie)
	e.settles("stream", 2)
	// Listed again from what Polyfin keeps, the title asks nothing, now or
	// later.
	if versions, err := e.service.Versions(t.Context(), e.member, e.movie); err != nil || len(versions) != 2 {
		t.Fatalf("listed again: %+v %v", versions, err)
	}
	if e.scheduled() != 0 {
		t.Error("a list served from the cache is followed up")
	}
	e.settles("stream", 2)
}

func TestARefreshIsNotUndoneByAFollowUp(t *testing.T) {
	// Follow-ups wait an hour unless the test hurries them: on a slow
	// machine, a short delay could ask one before the refresh that should
	// cancel it.
	e := newFollowEnv(t, time.Hour, time.Hour)
	e.replies <- 1
	e.service.Versions(t.Context(), e.member, e.movie)
	// The follow-up is held while the title is refreshed; its answer, which
	// lists more, comes after.
	e.eventually("the follow-up", func() bool { e.hurry(); return e.asked("stream") == 2 })
	if err := e.service.Refresh(t.Context(), e.member, e.movie); err != nil {
		t.Fatal(err)
	}
	if e.scheduled() != 0 {
		t.Error("a refresh left the schedule running")
	}
	e.replies <- 3
	e.settles("stream", 2)
	if got := e.known(); got != 0 {
		t.Errorf("the follow-up brought back the list a refresh dropped: %d versions", got)
	}

	// Listed again, the title has a new first answer, followed up in turn.
	e.replies <- 2
	e.replies <- 3
	e.replies <- 3
	if versions, err := e.service.Versions(t.Context(), e.member, e.movie); err != nil || len(versions) != 2 {
		t.Fatalf("after the refresh: %+v %v", versions, err)
	}
	e.eventually("the new follow-ups", func() bool { e.hurry(); return e.asked("stream") == 5 })
	e.settles("stream", 5)
	if got := e.known(); got != 3 {
		t.Errorf("%d versions after the new follow-ups, want 3", got)
	}

	// Refreshed while a follow-up is scheduled, the title asks nothing,
	// even once the follow-up is due.
	e.service.Refresh(t.Context(), e.member, e.movie)
	e.replies <- 1
	e.service.Versions(t.Context(), e.member, e.movie)
	due := e.schedule()
	e.service.Refresh(t.Context(), e.member, e.movie)
	// What the addon would answer, were it asked.
	e.replies <- 1
	due.hurry <- struct{}{}
	select {
	case <-due.asked:
	case <-time.After(5 * time.Second):
		t.Fatal("the refreshed schedule did not end")
	}
	e.settles("stream", 6)
}

func TestSubtitleListsAreFollowedUp(t *testing.T) {
	e := newFollowEnv(t, 20*time.Millisecond, 50*time.Millisecond)
	e.replies <- 1
	subtitles := func() int {
		t.Helper()
		subtitles, err := e.service.Subtitles(t.Context(), e.member, e.movie)
		if err != nil {
			t.Fatal(err)
		}
		return len(subtitles)
	}
	if got := subtitles(); got != 1 {
		t.Fatalf("first answer: %d subtitles", got)
	}
	e.replies <- 3
	e.eventually("the first follow-up's subtitles", func() bool { return subtitles() == 3 })
	e.replies <- 2
	e.settles("subtitles", 3)
	if got := subtitles(); got != 3 {
		t.Errorf("%d subtitles after the follow-ups, want 3", got)
	}
}
