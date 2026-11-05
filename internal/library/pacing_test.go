package library

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// paceClock is a clock that moves only when a test moves it, to the time
// the first of the pacing's waits ends: requests are then sent at the
// times the pacing chose, however slowly the machine runs.
type paceClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*paceTimer
}

type paceTimer struct {
	at time.Time
	c  chan time.Time
}

func newPaceClock() *paceClock {
	return &paceClock{now: time.Date(2026, 11, 2, 14, 49, 51, 0, time.UTC)}
}

func (c *paceClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *paceClock) after(d time.Duration) (<-chan time.Time, func() bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &paceTimer{at: c.now.Add(d), c: make(chan time.Time, 1)}
	c.timers = append(c.timers, t)
	return t.c, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.timers = slices.DeleteFunc(c.timers, func(other *paceTimer) bool { return other == t })
		return true
	}
}

// first is when the first wait ends; false when none runs.
func (c *paceClock) first() (time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.timers) == 0 {
		return time.Time{}, false
	}
	return slices.MinFunc(c.timers, func(a, b *paceTimer) int { return a.at.Compare(b.at) }).at, true
}

// next moves the clock to when the first wait ends, and ends the waits
// due then.
func (c *paceClock) next(t *testing.T) {
	t.Helper()
	at, ok := c.first()
	if !ok {
		t.Fatal("nothing waits for the clock")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = at
	c.timers = slices.DeleteFunc(c.timers, func(timer *paceTimer) bool {
		if timer.at.After(at) {
			return false
		}
		timer.c <- at
		return true
	})
}

// limitedAddon answers stream requests as an addon limiting the requests
// of each address does: of the requests within window, refused ones
// included, the first most are answered, the others refused with refuse.
type limitedAddon struct {
	clock  *paceClock
	most   int
	window time.Duration
	refuse func(w http.ResponseWriter)
	// refusing, while above 0, refuses that many requests whatever the
	// limit.
	refusing int

	mu       sync.Mutex
	sent     []time.Time
	answered int
	refused  int
}

func (l *limitedAddon) reply(w http.ResponseWriter, r *http.Request, resource string) {
	now := l.clock.Now()
	l.mu.Lock()
	l.sent = slices.DeleteFunc(l.sent, func(at time.Time) bool { return now.Sub(at) >= l.window })
	l.sent = append(l.sent, now)
	refused := len(l.sent) > l.most || l.refusing > 0
	if refused {
		l.refused++
		l.refusing = max(l.refusing-1, 0)
	} else {
		l.answered++
	}
	l.mu.Unlock()
	if refused {
		l.refuse(w)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{{Name: "Source", URL: "https://cdn.example" + r.URL.Path + ".mkv"}}})
}

// counts are the requests the addon answered and refused.
func (l *limitedAddon) counts() (answered, refused int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.answered, l.refused
}

// refuseWithNotice refuses as an addon that answers its refusals with a
// notice, HTTP 200.
func refuseWithNotice(w http.ResponseWriter) {
	_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{{Name: "[!] Addon", Description: "Rate Limit Exceeded",
		ExternalURL: "https://addon.example/"}}})
}

// refuseWith429 refuses with HTTP 429, asking to wait two minutes.
func refuseWith429(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "120")
	http.Error(w, "too many requests", http.StatusTooManyRequests)
}

// paceEnv is a server whose one addon limits its stream requests, with
// titles movies and the pacing on clock.
type paceEnv struct {
	env
	clock   *paceClock
	limited *limitedAddon
	movies  []accounts.ID
}

func newPaceEnv(t *testing.T, titleCount int, refuse func(http.ResponseWriter)) paceEnv {
	t.Helper()
	e := newEnv(t)
	clock := newPaceClock()
	e.service.now = clock.Now
	e.service.pace.after = clock.after
	// The addon's defaults: 10 stream requests per 15 seconds.
	limited := &limitedAddon{clock: clock, most: 10, window: 15 * time.Second, refuse: refuse}
	movies := titles("movie", titleCount)
	metas := map[string]stremio.Meta{}
	for _, m := range movies {
		metas["movie/"+m.ID] = m
	}
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "limited", Name: "Limited", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{"movie/top": movies},
		metas:    metas,
		reply:    limited.reply,
	}
	e.install(addons.Shared(), addon)
	page, err := e.service.Children(t.Context(), e.member, e.library(e.member, "Top").ID, 0, titleCount, "")
	if err != nil || len(page.Items) != titleCount {
		t.Fatalf("listing: %d titles, %v", len(page.Items), err)
	}
	ids := make([]accounts.ID, len(page.Items))
	for i, item := range page.Items {
		ids[i] = item.ID
	}
	return paceEnv{env: e, clock: clock, limited: limited, movies: ids}
}

// paceState counts the background requests waiting for their turn, and
// those under way.
func (e paceEnv) paceState() (queued, running int) {
	p := e.service.pace
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, h := range p.hosts {
		for _, k := range h.kinds {
			queued += len(k.queue)
			running += k.running
		}
	}
	return queued, running
}

// background lists the versions of titles at once as background work,
// as home rows list theirs ahead, and returns how many titles are done,
// with each one's version count.
func (e paceEnv) background(titles []accounts.ID) (*atomic.Int64, []atomic.Int64) {
	done := new(atomic.Int64)
	counts := make([]atomic.Int64, len(titles))
	for i, id := range titles {
		go func() {
			defer done.Add(1)
			versions, err := e.service.Versions(Background(context.WithoutCancel(e.t.Context())), e.member, id)
			if err != nil {
				e.t.Errorf("title %d: %v", i, err)
			}
			counts[i].Store(int64(len(versions)))
		}()
	}
	return done, counts
}

// quiet waits until each of titles is done or waits for its turn, none
// under way, the first waiting for the clock, and reports whether all
// are done.
func (e paceEnv) quiet(done *atomic.Int64, titles int) bool {
	e.t.Helper()
	eventually(e.t, "the titles to be done or wait for their turn", func() bool {
		queued, running := e.paceState()
		_, timed := e.clock.first()
		return int(done.Load())+queued == titles && running == 0 && (queued == 0 || timed)
	})
	return int(done.Load()) == titles
}

// drain moves the clock until every one of titles is done.
func (e paceEnv) drain(done *atomic.Int64, titles int) {
	e.t.Helper()
	for !e.quiet(done, titles) {
		e.clock.next(e.t)
	}
}

// A burst of titles listed in the background, as Continue Watching and
// Next Up list theirs ahead, never exceeds an addon's limit of 10 stream
// requests per 15 seconds: every title lists its version, and the addon
// refuses none.
func TestABurstOfBackgroundTitlesStaysWithinAnAddonsLimit(t *testing.T) {
	t.Parallel()
	e := newPaceEnv(t, 20, refuseWithNotice)
	done, counts := e.background(e.movies)
	e.drain(done, len(e.movies))
	if answered, refused := e.limited.counts(); refused != 0 || answered != 20 {
		t.Errorf("the addon answered %d requests and refused %d, want 20 and none", answered, refused)
	}
	for i := range counts {
		if got := counts[i].Load(); got != 1 {
			t.Errorf("title %d listed %d versions, want 1", i, got)
		}
	}
}

// A user's request, as PlaybackInfo's, is sent at once while background
// work waits for its turn, a title the background work waits to list
// included, and the addon answers it: the background requests left room
// for it. The background request for that title then uses its answer.
func TestAUsersRequestIsNotQueuedBehindBackgroundWork(t *testing.T) {
	t.Parallel()
	e := newPaceEnv(t, 21, refuseWithNotice)
	listed := e.movies[:20]
	done, _ := e.background(listed)
	if e.quiet(done, len(listed)) {
		t.Error("every title was listed at once")
	}
	waiting := slices.IndexFunc(listed, func(id accounts.ID) bool {
		_, known := e.service.CachedVersions(t.Context(), e.member, id)
		return !known
	})
	// The clock does not move: whatever waits for it stays waiting.
	for _, id := range []accounts.ID{e.movies[20], listed[max(waiting, 0)]} {
		versions, _, err := e.service.VersionsToPlay(t.Context(), e.member, id)
		if err != nil || len(versions) != 1 {
			t.Errorf("the user's title: %d versions, %v; want 1", len(versions), err)
		}
	}
	e.drain(done, len(listed))
	if answered, refused := e.limited.counts(); refused != 0 || answered != 21 {
		t.Errorf("the addon answered %d requests and refused %d, want 21 and none", answered, refused)
	}
}

// A refusal for the rate, a 429 with Retry-After or a notice saying so,
// pauses the background requests to the addon for as long as it asks, or
// a minute: they wait, while a user's request is still sent, and resume
// once the pause ends.
func TestARefusalPausesBackgroundRequestsUntilTheyResume(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		refuse func(http.ResponseWriter)
		pause  time.Duration
	}{
		{"429 with Retry-After", refuseWith429, 2 * time.Minute},
		{"a rate-limit notice", refuseWithNotice, time.Minute},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			e := newPaceEnv(t, 5, c.refuse)
			e.limited.refusing = 1
			refusedAt := e.clock.Now()
			first, _ := e.background(e.movies[:1])
			e.drain(first, 1)
			done, counts := e.background(e.movies[1:4])
			if e.quiet(done, 3) {
				t.Fatal("the background requests went on after the refusal")
			}
			if answered, refused := e.limited.counts(); answered != 0 || refused != 1 {
				t.Fatalf("during the pause the addon answered %d requests and refused %d, want none and 1", answered, refused)
			}
			if at, ok := e.clock.first(); !ok || !at.Equal(refusedAt.Add(c.pause)) {
				t.Errorf("the background requests wait until %v, want %v", at, refusedAt.Add(c.pause))
			}
			versions, _, err := e.service.VersionsToPlay(t.Context(), e.member, e.movies[4])
			if err != nil || len(versions) != 1 {
				t.Fatalf("the user's title during the pause: %d versions, %v; want 1", len(versions), err)
			}
			e.drain(done, 3)
			if answered, refused := e.limited.counts(); answered != 4 || refused != 1 {
				t.Errorf("the addon answered %d requests and refused %d, want 4 and 1", answered, refused)
			}
			for i := range counts {
				if got := counts[i].Load(); got != 1 {
					t.Errorf("title %s listed %d versions after the pause, want 1", strconv.Itoa(i+1), got)
				}
			}
		})
	}
}

// Follow-ups wait behind the other background requests: once the pace
// lets one more go, a title's first request goes before a follow-up that
// waited longer.
func TestFollowUpsWaitBehindOtherBackgroundRequests(t *testing.T) {
	t.Parallel()
	clock := newPaceClock()
	p := newPacing(clock.Now)
	p.after = clock.after
	const host = "addon.example"
	// The window fills, and paceAtOnce-1 of its requests stay under way:
	// once the window ends, the pace lets one more go, and the next only
	// once that one is done. Were both let go at once, which of them
	// reached order first would be up to their goroutines, not the pace.
	for i := range paceRates[paceStreams].most {
		release, err := p.wait(t.Context(), host, paceStreams, false)
		if err != nil {
			t.Fatal(err)
		}
		if i >= paceAtOnce-1 {
			release()
		}
	}
	queued := func() int {
		p.mu.Lock()
		defer p.mu.Unlock()
		return len(p.hosts[host].kinds[paceStreams].queue)
	}
	order := make(chan string, 2)
	turn := func(name string, later bool) {
		release, err := p.wait(t.Context(), host, paceStreams, later)
		if err != nil {
			t.Error(err)
			return
		}
		order <- name
		release()
	}
	timers := func() int {
		clock.mu.Lock()
		defer clock.mu.Unlock()
		return len(clock.timers)
	}
	go turn("follow-up", true)
	eventually(t, "the follow-up to wait", func() bool { return queued() == 1 && timers() == 1 })
	go turn("first request", false)
	eventually(t, "the first request to wait", func() bool { return queued() == 2 && timers() == 2 })
	clock.next(t)
	if first, second := <-order, <-order; first != "first request" || second != "follow-up" {
		t.Errorf("went in the order %s, %s", first, second)
	}
}
