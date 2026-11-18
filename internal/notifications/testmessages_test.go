package notifications

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// stopClock stops the clock of h at a time that moves only when the test
// moves it, with the function returned.
func (h harness) stopClock() (advance func(time.Duration)) {
	var mu sync.Mutex
	now := time.Date(2030, 3, 15, 12, 0, 0, 0, time.UTC)
	h.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	return func(d time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		now = now.Add(d)
	}
}

// refusedFor fails the test unless err is a test refused for reason, and
// returns how long it tells to wait.
func refusedFor(t *testing.T, err, reason error) time.Duration {
	t.Helper()
	var refused *TestRefused
	if !errors.As(err, &refused) || !errors.Is(err, reason) {
		t.Fatalf("the test: %v, want it refused: %v", err, reason)
	}
	return refused.Wait
}

// "Send a test" is spaced: a target is sent a test once in half a minute,
// and the targets of an owner ten tests in any hour, whoever they write
// to. A test that is refused sends nothing, tells how long to wait and
// counts for nothing, and the tests of one owner leave the others theirs.
func TestTestsAreSpacedByTargetAndCountedByOwner(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	advance := h.stopClock()
	sink := newSMTPSink(t, false, "PLAIN LOGIN")
	h.useSMTP(t, sink, sink.password)
	inbox := func(owner *accounts.User, address string) Target {
		t.Helper()
		return h.add(t, owner, Draft{Kind: Email, Name: new(address), Address: new(address)})
	}
	deliver := func(owner *accounts.User, target Target) {
		t.Helper()
		if result, err := h.Test(t.Context(), owner, target.ID); err != nil || !result.Delivered {
			t.Fatalf("a test of %s: %+v, %v", target.Name, result, err)
		}
	}
	sunk := func(want int) {
		t.Helper()
		sink.wait(want)
		if mails, _, _ := sink.received(); len(mails) != want {
			t.Fatalf("the SMTP server received %d messages, want %d", len(mails), want)
		}
	}
	member := h.member

	// One target, asked again and again: one message.
	first := inbox(&member, "someone@elsewhere.example")
	deliver(&member, first)
	for range 24 {
		_, err := h.Test(t.Context(), &member, first.ID)
		if wait := refusedFor(t, err, ErrTestTooSoon); wait != 30*time.Second {
			t.Fatalf("a test asked at once after another tells to wait %s", wait)
		}
	}
	sunk(1)
	advance(29 * time.Second)
	_, err := h.Test(t.Context(), &member, first.ID)
	if wait := refusedFor(t, err, ErrTestTooSoon); wait != time.Second {
		t.Errorf("a test asked 29 seconds after another tells to wait %s", wait)
	}
	advance(time.Second)
	deliver(&member, first)
	sunk(2)

	// Other targets are not held by the first, until the owner's tests of
	// the hour are ten.
	for i := range MaxTestsPerHour - 2 {
		deliver(&member, inbox(&member, fmt.Sprintf("other%d@elsewhere.example", i)))
	}
	sunk(MaxTestsPerHour)
	eleventh := inbox(&member, "eleventh@elsewhere.example")
	_, err = h.Test(t.Context(), &member, eleventh.ID)
	if wait := refusedFor(t, err, ErrTooManyTests); wait != time.Hour-30*time.Second {
		t.Errorf("an eleventh test tells to wait %s", wait)
	}
	advance(30 * time.Second)
	_, err = h.Test(t.Context(), &member, first.ID)
	refusedFor(t, err, ErrTooManyTests)
	sunk(MaxTestsPerHour)

	// The administrator's own targets and the server's have their own
	// counts, and the same spacing.
	own, server := inbox(&h.admin, "admin@example.org"), inbox(nil, "server@example.org")
	deliver(&h.admin, own)
	deliver(nil, server)
	_, err = h.Test(t.Context(), nil, server.ID)
	refusedFor(t, err, ErrTestTooSoon)
	sunk(MaxTestsPerHour + 2)

	// An hour after the member's first test, one more is allowed, and no
	// more until an hour after their second.
	advance(time.Hour - 61*time.Second)
	_, err = h.Test(t.Context(), &member, eleventh.ID)
	if wait := refusedFor(t, err, ErrTooManyTests); wait != time.Second {
		t.Errorf("a second before the hour of the first test, a test tells to wait %s", wait)
	}
	advance(time.Second)
	deliver(&member, eleventh)
	_, err = h.Test(t.Context(), &member, first.ID)
	if wait := refusedFor(t, err, ErrTooManyTests); wait != 30*time.Second {
		t.Errorf("an hour after the first test, a second one tells to wait %s", wait)
	}
	sunk(MaxTestsPerHour + 3)
	for _, mail := range sink.wait(MaxTestsPerHour + 3) {
		if mail.from != "polyfin@example.org" || mail.user != "polyfin" {
			t.Errorf("a message from %q, signed in as %q", mail.from, mail.user)
		}
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.tests[member.ID]) != MaxTestsPerHour || len(h.tests) != 3 {
		t.Errorf("%d tests kept for the member, of %d owners", len(h.tests[member.ID]), len(h.tests))
	}
}

// A test counts toward its target's cap of messages a minute: one asked
// while the cap is reached is refused until the minute of the message
// before has passed, and sends nothing.
func TestATestWaitsForTheCapOfItsTarget(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	advance := h.stopClock()
	draft := h.webhook("/capped", PlaybackStarted)
	draft.PerMinute = new(1)
	target := h.add(t, &h.member, draft)

	h.tell(PlaybackStarted, "play")
	h.targets.wait(t, "/capped", 1)
	h.idle(t)
	_, err := h.Test(t.Context(), &h.member, target.ID)
	if wait := refusedFor(t, err, ErrTestTooSoon); wait != time.Minute {
		t.Errorf("a test after a message tells to wait %s", wait)
	}
	advance(59 * time.Second)
	_, err = h.Test(t.Context(), &h.member, target.ID)
	if wait := refusedFor(t, err, ErrTestTooSoon); wait != time.Second {
		t.Errorf("a test 59 seconds after a message tells to wait %s", wait)
	}
	if got := len(h.targets.at("/capped")); got != 1 {
		t.Fatalf("the target received %d messages, want 1", got)
	}
	advance(time.Second)
	if result, err := h.Test(t.Context(), &h.member, target.ID); err != nil || !result.Delivered {
		t.Fatalf("a test a minute after a message: %+v, %v", result, err)
	}
	if got := len(h.targets.at("/capped")); got != 2 {
		t.Errorf("the target received %d messages, want 2", got)
	}
	// The test holds the minute in its turn, and nothing more is kept.
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.recent[target.ID]) != 1 || len(h.underWay) != 0 || len(h.tests[h.member.ID]) != 1 {
		t.Errorf("kept: %d messages of the minute, %d sends under way, %d tests", len(h.recent[target.ID]), len(h.underWay),
			len(h.tests[h.member.ID]))
	}
}

// A message queued after a test waits for the minute of the test: the
// target receives them a minute apart or more.
func TestAMessageWaitsForTheMinuteOfATest(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.timing.minute = time.Second
	draft := h.webhook("/capped", PlaybackStarted)
	draft.PerMinute = new(1)
	target := h.add(t, nil, draft)

	if result, err := h.Test(t.Context(), nil, target.ID); err != nil || !result.Delivered {
		t.Fatalf("a test: %+v, %v", result, err)
	}
	h.tell(PlaybackStarted, "play")
	got := h.targets.wait(t, "/capped", 2)
	h.idle(t)
	if got[0].body["type"] != Test || got[1].body["title"] != "play" {
		t.Fatalf("received %v, then %v", got[0].body["type"], got[1].body["title"])
	}
	if gap := got[1].at.Sub(got[0].at); gap < h.timing.minute {
		t.Errorf("the message came %s after the test, under the cap's minute", gap)
	}
}

// A test asked while a message is being sent to its target finds the
// place of that message taken in the cap, though the minute of the
// message has not begun.
func TestATestFindsTheCapHeldByAMessageBeingSent(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	draft := h.webhook("/slow", PlaybackStarted)
	draft.PerMinute = new(1)
	target := h.add(t, nil, draft)
	// The target answers the message only once the test was asked.
	asked := make(chan struct{})
	release := sync.OnceFunc(func() { close(asked) })
	t.Cleanup(release)
	h.targets.then("/slow", func(w http.ResponseWriter) {
		<-asked
		w.WriteHeader(http.StatusNoContent)
	})

	h.tell(PlaybackStarted, "play")
	h.targets.wait(t, "/slow", 1)
	_, err := h.Test(t.Context(), nil, target.ID)
	if wait := refusedFor(t, err, ErrTestTooSoon); wait != h.timing.minute {
		t.Errorf("a test asked while a message is being sent tells to wait %s", wait)
	}
	release()
	h.idle(t)
	// Sent, the message holds its minute.
	_, err = h.Test(t.Context(), nil, target.ID)
	refusedFor(t, err, ErrTestTooSoon)
	if got := len(h.targets.at("/slow")); got != 1 {
		t.Errorf("the target received %d messages, want 1", got)
	}
}

// The tests remembered are bounded: ten of the last hour for an owner, a
// thousand owners, and nothing of more than an hour ago. Past the bound of
// owners, another owner's test waits until one's hour has passed.
func TestTheTestsRememberedAreBounded(t *testing.T) {
	t.Parallel()
	s := New(Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	t.Cleanup(s.Close)
	now := time.Date(2030, 3, 15, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	owner := func(i int) *accounts.User { return &accounts.User{ID: accounts.ID{1, byte(i >> 8), byte(i)}} }
	// Each test is of another target.
	targets := 0
	ask := func(i int) error {
		targets++
		return s.allowTest(owner(i), accounts.ID{2, byte(targets >> 16), byte(targets >> 8), byte(targets)}, target{})
	}
	kept := func() (owners, most int) {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, tests := range s.tests {
			most = max(most, len(tests))
		}
		return len(s.tests), most
	}

	if err := ask(0); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	for i := 1; i < maxTestOwners; i++ {
		if err := ask(i); err != nil {
			t.Fatalf("owner %d: %v", i, err)
		}
	}
	// One owner asks for many tests, then many more owners for one each.
	for range 3 * MaxTestsPerHour {
		_ = ask(1)
	}
	for i := maxTestOwners; i < 2*maxTestOwners; i++ {
		if wait := refusedFor(t, ask(i), ErrTooManyTests); wait != 59*time.Minute {
			t.Fatalf("owner %d, past the bound, is told to wait %s", i, wait)
		}
	}
	if owners, most := kept(); owners != maxTestOwners || most != MaxTestsPerHour {
		t.Fatalf("%d owners kept, %d tests for one at most, want %d and %d", owners, most, maxTestOwners, MaxTestsPerHour)
	}
	// An owner who is remembered is not held by the bound of owners.
	if err := ask(2); err != nil {
		t.Errorf("a second test of a remembered owner: %v", err)
	}

	// The first owner's hour has passed: one more owner is remembered.
	now = now.Add(59 * time.Minute)
	if err := ask(maxTestOwners); err != nil {
		t.Errorf("an owner asking once another's hour has passed: %v", err)
	}
	refusedFor(t, ask(maxTestOwners+1), ErrTooManyTests)
	if owners, _ := kept(); owners != maxTestOwners {
		t.Errorf("%d owners kept, want %d", owners, maxTestOwners)
	}
	// Two hours later, everything is forgotten but the test just asked.
	now = now.Add(2 * time.Hour)
	if err := ask(maxTestOwners + 1); err != nil {
		t.Error(err)
	}
	if owners, most := kept(); owners != 1 || most != 1 || len(s.underWay) != 0 || len(s.recent) != 0 {
		t.Errorf("%d owners kept, %d tests for one at most, %d sends under way, %d targets' minutes", owners, most, len(s.underWay), len(s.recent))
	}
}
