package library

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
		if resource == "stream" {
			streams := make([]stremio.Stream, n)
			for i := range streams {
				streams[i] = stremio.Stream{Name: fmt.Sprintf("Source %d", i+1), URL: fmt.Sprintf("https://cdn.example/movie/%d.mkv", i+1)}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": streams})
			return
		}
		subtitles := make([]stremio.Subtitle, n)
		for i := range subtitles {
			subtitles[i] = stremio.Subtitle{ID: stremio.Text(fmt.Sprint(i + 1)), URL: fmt.Sprintf("https://cdn.example/movie/%d.srt", i+1), Lang: "eng"}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"subtitles": subtitles})
	}
}

// followEnv serves one movie from an addon whose streams and subtitles
// the test scripts, followed up after short delays.
type followEnv struct {
	env
	addon   *fakeAddon
	replies chan int
	movie   accounts.ID
}

func newFollowEnv(t *testing.T, delays ...time.Duration) followEnv {
	t.Helper()
	e := newEnv(t)
	e.service.followUpDelays = delays
	movies := titles("movie", 1)
	replies := make(chan int, 10)
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "gathered", Name: "Gathered", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}, {Name: "subtitles"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{"movie/top": movies},
		metas:    map[string]stremio.Meta{"movie/" + movies[0].ID: movies[0]},
		reply:    scriptedReplies(replies),
	}
	e.install(addons.Shared(), addon)
	page, err := e.service.Children(t.Context(), e.member, e.library(e.member, "Top").ID, 0, 10, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("listing: %+v %v", page, err)
	}
	return followEnv{env: e, addon: addon, replies: replies, movie: page.Items[0].ID}
}

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
	e := newFollowEnv(t, 100*time.Millisecond, 100*time.Millisecond)
	e.replies <- 1
	e.service.Versions(t.Context(), e.member, e.movie)
	// The follow-up is held while the title is refreshed; its answer, which
	// lists more, comes after.
	e.eventually("the follow-up", func() bool { return e.asked("stream") == 2 })
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
	e.settles("stream", 5)
	if got := e.known(); got != 3 {
		t.Errorf("%d versions after the new follow-ups, want 3", got)
	}

	// Refreshed while a follow-up is scheduled, the title asks nothing.
	e.service.Refresh(t.Context(), e.member, e.movie)
	e.replies <- 1
	e.service.Versions(t.Context(), e.member, e.movie)
	e.service.Refresh(t.Context(), e.member, e.movie)
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
