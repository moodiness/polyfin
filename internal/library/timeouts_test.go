package library

import (
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// advance moves the clock d ahead, ending no wait.
func (c *paceClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// A server that leaves timeoutsInARow requests for streams in a row
// unanswered has its background requests paused, for timeoutPause, then
// twice as long at each further run, timeoutPauseMost at most. Timeouts
// during a pause count for nothing, and an answer lifts the pause, the
// next run pausing timeoutPause again. Only the addons that did not answer
// are told as such.
func TestTimeoutsInARowPauseAServersBackgroundRequests(t *testing.T) {
	t.Parallel()
	clock := newPaceClock()
	p := newPacing(clock.Now)
	p.after = clock.after
	const host = "addon.example"
	slow, other := accounts.ID{1}, accounts.ID{2}
	run := func(want time.Duration) {
		t.Helper()
		for i := range timeoutsInARow {
			pause, started := p.streamsTimedOut(host, slow)
			if last := i == timeoutsInARow-1; started != last || last && pause != want {
				t.Fatalf("timeout %d: paused %t for %s, want a pause of %s at the last", i+1, started, pause, want)
			}
		}
		if until, ok := p.timingOut(host, slow); !ok || !until.Equal(clock.Now().Add(want)) || !p.paused(host) {
			t.Fatalf("the pause ends at %v (%t), want %v", until, ok, clock.Now().Add(want))
		}
	}

	run(timeoutPause)
	if _, ok := p.timingOut(host, other); ok {
		t.Error("an addon that answered is told as not answering")
	}
	// A request sent before the pause times out during it.
	if _, started := p.streamsTimedOut(host, slow); started {
		t.Fatal("a timeout during the pause started another")
	}
	last := timeoutPause
	for _, want := range []time.Duration{10 * time.Minute, 20 * time.Minute, timeoutPauseMost, timeoutPauseMost} {
		clock.advance(last)
		if p.paused(host) {
			t.Fatal("the pause did not end")
		}
		run(want)
		last = want
	}

	p.streamsAnswered(host)
	if _, ok := p.timingOut(host, slow); ok || p.paused(host) {
		t.Fatal("an answer did not lift the pause")
	}
	run(timeoutPause)

	// Timeouts not in a row pause nothing.
	p.streamsAnswered(host)
	for range 2 {
		for range timeoutsInARow - 1 {
			if _, started := p.streamsTimedOut(host, slow); started {
				t.Fatal("timeouts not in a row started a pause")
			}
		}
		p.streamsAnswered(host)
	}
}

// deadAddon answers no stream request, holding each until the client gives
// up, until it answers again.
type deadAddon struct {
	answering atomic.Bool
	asked     atomic.Int32
}

func (d *deadAddon) reply(w http.ResponseWriter, r *http.Request, resource string) {
	if resource != "stream" {
		answerNumbered(w, resource, nil)
		return
	}
	d.asked.Add(1)
	if d.answering.Load() {
		answerNumbered(w, resource, []int{1})
		return
	}
	<-r.Context().Done()
}

// newDeadEnv is a paceEnv whose one addon is dead, its stream requests
// given 100 ms, its log kept in log.
func newDeadEnv(t *testing.T, titleCount int) (paceEnv, *deadAddon, *lockedBuffer) {
	t.Helper()
	e := newEnv(t)
	e.service.client.SetTimeouts(time.Second, 100*time.Millisecond)
	log := &lockedBuffer{}
	e.service.logger = slog.New(slog.NewTextHandler(log, nil))
	clock := newPaceClock()
	e.service.now = clock.Now
	e.service.pace.after = clock.after
	dead := &deadAddon{}
	movies := titles("movie", titleCount)
	metas := map[string]stremio.Meta{}
	for _, m := range movies {
		metas["movie/"+m.ID] = m
	}
	e.install(addons.Shared(), &fakeAddon{
		manifest: stremio.Manifest{ID: "dead", Name: "Dead", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{"movie/top": movies},
		metas:    metas,
		reply:    dead.reply,
	})
	page, err := e.service.Children(t.Context(), e.member, e.library(e.member, "Top").ID, 0, titleCount, "")
	if err != nil || len(page.Items) != titleCount {
		t.Fatalf("listing: %d titles, %v", len(page.Items), err)
	}
	ids := make([]accounts.ID, len(page.Items))
	for i, item := range page.Items {
		ids[i] = item.ID
	}
	return paceEnv{env: e, clock: clock, movies: ids}, dead, log
}

// Background work listing titles, as Continue Watching and Next Up list
// theirs ahead, asks an addon that never answers 4 times, the third
// timeout pausing the server's background requests for 5 minutes and the
// fourth, sent before, counting for nothing; then nothing more, where it
// asked as fast as the pace let it, 8 times a minute. A user's request
// still goes during the pause, and an answer lifts it: the titles waiting
// are listed at once.
func TestAnAddonThatKeepsTimingOutPausesBackgroundWork(t *testing.T) {
	t.Parallel()
	e, dead, log := newDeadEnv(t, 8)
	pausedAt := e.clock.Now()
	listed := e.movies[:6]
	done, _ := e.background(listed)
	if e.quiet(done, len(listed)) {
		t.Fatal("every title was listed")
	}
	if got := dead.asked.Load(); got != 4 {
		t.Fatalf("the addon was asked %d times before the pause, want 4", got)
	}
	if at, ok := e.clock.first(); !ok || !at.Equal(pausedAt.Add(timeoutPause)) {
		t.Errorf("the background requests wait until %v, want %v", at, pausedAt.Add(timeoutPause))
	}
	addon := e.installedAddon("dead")
	if until, ok := e.service.StreamsTimingOut(addon); !ok || !until.Equal(pausedAt.Add(timeoutPause)) {
		t.Errorf("Health is told the pause ends at %v (%t)", until, ok)
	}

	// The clock does not move: whatever waits for it stays waiting.
	if _, _, err := e.service.VersionsToPlay(t.Context(), e.member, e.movies[6]); err != nil {
		t.Fatal(err)
	}
	if got := dead.asked.Load(); got != 5 {
		t.Fatalf("the user's request during the pause: the addon was asked %d times, want 5", got)
	}
	dead.answering.Store(true)
	versions, _, err := e.service.VersionsToPlay(t.Context(), e.member, e.movies[7])
	if err != nil || len(versions) != 1 {
		t.Fatalf("the user's request once the addon answers: %d versions, %v", len(versions), err)
	}
	if !e.quiet(done, len(listed)) {
		t.Fatal("the answer did not lift the pause")
	}
	if _, ok := e.service.StreamsTimingOut(addon); ok {
		t.Error("Health is still told of the pause")
	}
	if got := dead.asked.Load(); got != 8 {
		t.Errorf("the addon was asked %d times, want 8", got)
	}
	logged := 0
	for line := range strings.Lines(log.String()) {
		if strings.Contains(line, "did not answer requests for streams in time") {
			logged++
			if !strings.Contains(line, "level=INFO") || !strings.Contains(line, "addon=Dead") || !strings.Contains(line, "wait=5m0s") {
				t.Errorf("line: %s", line)
			}
		}
	}
	if logged != 1 {
		t.Errorf("the pause was logged %d times, want once:\n%s", logged, log)
	}
}

// installedAddon returns the installed addon of the server whose manifest
// has the identifier id.
func (e env) installedAddon(id string) addons.Addon {
	e.t.Helper()
	list, err := e.addons.Addons(e.t.Context(), addons.Shared())
	if err != nil {
		e.t.Fatal(err)
	}
	for _, addon := range list {
		if addon.Manifest.ID == id {
			return addon
		}
	}
	e.t.Fatalf("no addon %q", id)
	return addons.Addon{}
}
