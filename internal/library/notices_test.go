package library

import (
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"
)

// noticeEnv is a staleEnv whose service logs into log.
type noticeEnv struct {
	staleEnv
	log *lockedBuffer
}

func newNoticeEnv(t *testing.T, delays ...time.Duration) noticeEnv {
	t.Helper()
	e := newStaleEnv(t, delays...)
	log := &lockedBuffer{}
	e.service.logger = slog.New(slog.NewTextHandler(log, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return noticeEnv{staleEnv: e, log: log}
}

// warnings are the warnings logged.
func (e noticeEnv) warnings() []string {
	var warnings []string
	for line := range strings.Lines(e.log.String()) {
		if strings.Contains(line, "level=WARN") {
			warnings = append(warnings, line)
		}
	}
	return warnings
}

// versions counts the versions of the movie, waiting for the addon.
func (e noticeEnv) versions() int {
	e.t.Helper()
	versions, err := e.service.Versions(e.t.Context(), e.member, e.movie)
	if err != nil {
		e.t.Fatal(err)
	}
	return len(versions)
}

// An addon limiting requests answers every request at once with a notice.
// Its answer is logged once, with the title and the notice but not its
// link, and the callers that come within the minute list the versions kept
// without asking the addon again; after the minute, it is asked again.
func TestANoticesAnswerIsLoggedOnceAndNotAskedAgainAtOnce(t *testing.T) {
	t.Parallel()
	e := newNoticeEnv(t, 20*time.Millisecond)
	e.expired()
	for range 5 {
		e.numbers <- []int{0}
	}
	for range 3 {
		if n := e.versions(); n != 3 {
			t.Errorf("%d versions during the notices, want the 3 kept", n)
		}
	}
	if _, _, err := e.service.VersionsNow(t.Context(), e.member, e.movie); err != nil {
		t.Fatal(err)
	}
	e.eventually("the background request", func() bool { return e.pending() == 0 })
	if got := e.asked("stream"); got != 3 {
		t.Errorf("the addon was asked %d times, want once after the first answer", got-2)
	}
	warnings := e.warnings()
	if len(warnings) != 1 {
		t.Fatalf("%d warnings, want 1:\n%s", len(warnings), e.log)
	}
	if w := warnings[0]; !strings.Contains(w, "type=movie") || !strings.Contains(w, "id=ttm000") ||
		!strings.Contains(w, `notice="Notice: Rate limit exceeded"`) || strings.Contains(w, "addon.example") {
		t.Errorf("warning: %s", w)
	}
	for len(e.numbers) > 0 {
		<-e.numbers
	}
	e.wait(noticeLife + time.Second)
	e.numbers <- []int{1, 2}
	e.numbers <- []int{1, 2}
	e.versions()
	e.settles("stream", 5)
	if got := e.listed(); !slices.Equal(got, []string{"Source 1", "Source 2"}) {
		t.Errorf("listed after the minute: %q", got)
	}
}

// A notices answer for a title the addon listed nothing for is not kept as
// its list: the title is asked again after a minute, not after the list's
// life, and lists its versions then.
func TestANewTitleIsAskedAgainAMinuteAfterANotice(t *testing.T) {
	t.Parallel()
	e := newNoticeEnv(t)
	e.numbers <- []int{0}
	if n := e.versions(); n != 0 {
		t.Fatalf("%d versions from a notice", n)
	}
	if n := e.versions(); n != 0 || e.asked("stream") != 1 {
		t.Errorf("within the minute: %d versions, %d requests", n, e.asked("stream"))
	}
	e.wait(noticeLife + time.Second)
	e.numbers <- []int{1, 2}
	if n := e.versions(); n != 2 || e.asked("stream") != 2 {
		t.Errorf("after the minute: %d versions, %d requests", n, e.asked("stream"))
	}
	if warnings := e.warnings(); len(warnings) != 1 {
		t.Errorf("%d warnings, want 1:\n%s", len(warnings), e.log)
	}
}
