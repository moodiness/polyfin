package jellyfin

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/process"
)

// turnProbe is an ffprobe whose nth run fails when the file fail<n> is
// there, and waits while the file hold<n> is, before answering the
// analysis of probe. A run takes its number, the first free, by making
// the folder run<n>: in one step, so that two runs that overlap never
// take the same, and a run the test counts already knows its number.
type turnProbe struct{ path string }

func newTurnProbe(t *testing.T) turnProbe {
	t.Helper()
	probe := newFakeProbe(t, true)
	p := turnProbe{path: probe.path + "-turns"}
	script := "#!/bin/sh\nn=1\nwhile ! mkdir \"$0.run$n\" 2> /dev/null; do n=$((n + 1)); done\n" +
		"if [ -e \"$0.fail$n\" ]; then exit 1; fi\n" +
		"while [ -e \"$0.hold$n\" ]; do sleep 0.01; done\n" +
		"exec cat \"" + probe.path + ".json\"\n"
	if err := process.WriteExecutable(p.path, strings.NewReader(script), 0o700); err != nil {
		t.Fatal(err)
	}
	// A run still held ends with the test.
	t.Cleanup(func() {
		holds, _ := filepath.Glob(p.path + ".hold*")
		for _, hold := range holds {
			_ = os.Remove(hold)
		}
	})
	return p
}

func (p turnProbe) mark(t *testing.T, what string, run int) {
	t.Helper()
	if err := os.WriteFile(p.path+"."+what+strconv.Itoa(run), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

func (p turnProbe) runs() int {
	n := 0
	for {
		if _, err := os.Stat(p.path + ".run" + strconv.Itoa(n+1)); err != nil {
			return n
		}
		n++
	}
}

// fresh are versions of the files the test addon serves, as copies no
// test analyzed yet.
func fresh(p playbackSetup, tag byte) []library.Version {
	versions := []library.Version{p.versions[0], p.versions[1]}
	for i := range versions {
		versions[i].ID = accounts.ID{0xA7, tag, byte(i)}
	}
	return versions
}

// A title's preparation analyzes the version its play reads first, and
// the next only when the first fails, or stays silent: each one read is
// up to tens of megabytes from a debrid host.
func TestPreparationsAnalyzeTheNextVersionOnlyWhenTheFirstFails(t *testing.T) {
	t.Parallel()
	probe := newTurnProbe(t)
	p := playingOn(t, newProbingServer(t, 10, probe.path))
	t.Cleanup(func() { p.settle(t) })
	silences := make(chan chan time.Time, 3)
	p.handler.preparations.after = func(time.Duration) <-chan time.Time {
		silence := make(chan time.Time, 1)
		silences <- silence
		return silence
	}
	analyzed := func(v library.Version) bool {
		_, ok := p.handler.Playback.Analyzed(t.Context(), v.ID)
		return ok
	}
	key := func(versions []library.Version) preparationKey {
		return preparationKey{user: p.user.ID, title: versions[0].ID, version: versions[0].ID}
	}

	// The first answers: the second is never read.
	versions := fresh(p, 1)
	p.handler.prepareVersions(t.Context(), key(versions), versions)
	p.settle(t)
	if probe.runs() != 1 || !analyzed(versions[0]) || analyzed(versions[1]) {
		t.Fatalf("the first analyzed: %d runs, the second analyzed: %v", probe.runs(), analyzed(versions[1]))
	}
	<-silences

	// The first fails: the second is analyzed after it.
	probe.mark(t, "fail", 2)
	versions = fresh(p, 2)
	p.handler.prepareVersions(t.Context(), key(versions), versions)
	p.settle(t)
	if probe.runs() != 3 || analyzed(versions[0]) || !analyzed(versions[1]) {
		t.Fatalf("the first failed: %d runs, analyzed %v %v", probe.runs(), analyzed(versions[0]), analyzed(versions[1]))
	}
	<-silences

	// The first stays silent: the second is analyzed meanwhile, and the
	// first goes on.
	probe.mark(t, "hold", 4)
	versions = fresh(p, 3)
	p.handler.prepareVersions(t.Context(), key(versions), versions)
	eventually(t, "the first to be read", func() bool { return probe.runs() == 4 })
	(<-silences) <- time.Time{}
	eventually(t, "the second to be analyzed", func() bool { return analyzed(versions[1]) })
	if analyzed(versions[0]) {
		t.Fatal("the first was analyzed while held")
	}
	if err := os.Remove(probe.path + ".hold4"); err != nil {
		t.Fatal(err)
	}
	p.settle(t)
	if probe.runs() != 5 || !analyzed(versions[0]) {
		t.Errorf("released: %d runs, the first analyzed: %v", probe.runs(), analyzed(versions[0]))
	}
}

// What the titles of a user's home rows read ahead of their first
// segments stays within aheadBytes an hour; another user has their own.
func TestFirstSegmentsReadAheadStayWithinABudget(t *testing.T) {
	t.Parallel()
	p := newPreparations(nil)
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	p.now = func() time.Time { return now }
	alice, bob := accounts.ID{1}, accounts.ID{2}
	if !p.spend(alice, aheadBytes/2) || !p.spend(alice, aheadBytes/2) {
		t.Fatal("within the budget: declined")
	}
	if p.spend(alice, 1) {
		t.Error("past the budget: allowed")
	}
	if !p.spend(alice, 0) {
		t.Error("what is cached already: declined")
	}
	if !p.spend(bob, aheadBytes) {
		t.Error("another user: declined")
	}
	now = now.Add(aheadPeriod - time.Second)
	if p.spend(alice, 1) {
		t.Error("within the hour: allowed")
	}
	now = now.Add(time.Second)
	if !p.spend(alice, aheadBytes) {
		t.Error("an hour later: declined")
	}
}
