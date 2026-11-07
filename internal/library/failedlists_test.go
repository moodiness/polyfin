package library

import (
	"testing"
	"time"
)

// oneStream is an addon's answer listing one stream.
const oneStream = `{"streams":[{"name":"Source 1","url":"https://cdn.example/movie/1.mkv"}]}`

// explained explains the movie's versions, as the title's versions page
// reads them.
func (e logEnv) explained() VersionsExplained {
	e.t.Helper()
	explained, err := e.service.ExplainVersions(e.t.Context(), e.member, e.movie)
	if err != nil {
		e.t.Fatal(err)
	}
	return explained
}

// fails has the addon fail to list the movie's streams, asked in the
// background as the versions page asks it, and waits for the failure.
func (e logEnv) fails() {
	e.t.Helper()
	if explained := e.explained(); explained.Pending != 1 || explained.Sources[0].State != ListAsked {
		e.t.Fatalf("on opening: %d pending, %s, want the addon asked", explained.Pending, explained.Sources[0].State)
	}
	e.bodies <- ""
	e.eventually("the addon's failure", func() bool {
		n, err := e.service.Pending(e.t.Context(), e.member, e.movie)
		return err == nil && n == 0
	})
}

// The versions page reads a title's versions every few seconds while an
// addon is asked. An addon that failed is not asked again by those reads,
// nor by item details, for failedFor: the page says it failed, nothing is
// pending, and its failure is logged once. Then it is asked again.
func TestAFailedListIsNotAskedAgainForAWhile(t *testing.T) {
	t.Parallel()
	e := newLogEnv(t)
	e.fails()
	for read := range 5 {
		explained := e.explained()
		if explained.Pending != 0 || explained.Sources[0].State != ListFailed {
			t.Fatalf("read %d after the failure: %d pending, %s, want none pending and %s", read+1,
				explained.Pending, explained.Sources[0].State, ListFailed)
		}
	}
	if _, complete, err := e.service.VersionsNow(t.Context(), e.member, e.movie); err != nil || !complete {
		t.Fatalf("item details after the failure: complete %t, %v; want complete, the addon not asked", complete, err)
	}
	if got := e.asked("stream"); got != 1 {
		t.Errorf("the addon was asked %d times for streams, want once", got)
	}
	if got := len(e.lines("An addon could not list streams", 0)); got != 1 {
		t.Errorf("the failure was logged %d times, want once:\n%s", got, e.log)
	}

	e.wait(failedFor + time.Second)
	if explained := e.explained(); explained.Pending != 1 || explained.Sources[0].State != ListAsked {
		t.Fatalf("after %s: %d pending, %s, want the addon asked again", failedFor, explained.Pending, explained.Sources[0].State)
	}
	e.bodies <- oneStream
	e.eventually("the addon's answer", func() bool {
		explained := e.explained()
		return explained.Pending == 0 && explained.Sources[0].State == ListKnown && len(explained.Versions) == 1
	})
	if got := e.asked("stream"); got != 2 {
		t.Errorf("the addon was asked %d times for streams, want twice", got)
	}
}

// What a user asks for still asks an addon that failed lately, at once: a
// play, and Ask addons again, which the web player's Try again and a
// refresh of the title's streams share (see AskAgain).
func TestAFailedListIsAskedAtOnceForAPlayOrWhenAskedAgain(t *testing.T) {
	t.Parallel()
	e := newLogEnv(t)
	e.fails()
	e.bodies <- oneStream
	versions, _, err := e.service.VersionsToPlay(t.Context(), e.member, e.movie)
	if err != nil || len(versions) != 1 {
		t.Fatalf("a play after the failure: %d versions, %v; want the addon asked, 1", len(versions), err)
	}
	if got := e.asked("stream"); got != 2 {
		t.Errorf("after the play, the addon was asked %d times for streams, want twice", got)
	}

	if err := e.service.AskAgain(t.Context(), e.member, e.movie); err != nil {
		t.Fatal(err)
	}
	e.bodies <- ""
	e.eventually("the addon's failure", func() bool {
		return e.explained().Sources[0].State == ListFailed
	})
	if err := e.service.AskAgain(t.Context(), e.member, e.movie); err != nil {
		t.Fatal(err)
	}
	if explained := e.explained(); explained.Pending != 1 || explained.Sources[0].State != ListAsked {
		t.Fatalf("asked again after the failure: %d pending, %s, want the addon asked", explained.Pending, explained.Sources[0].State)
	}
	e.bodies <- oneStream
	e.eventually("the addon's answer", func() bool {
		explained := e.explained()
		return explained.Pending == 0 && explained.Sources[0].State == ListKnown && len(explained.Versions) == 1
	})
	if got := e.asked("stream"); got != 4 {
		t.Errorf("the addon was asked %d times for streams, want 4: opening, the play and each Ask addons again", got)
	}
}
