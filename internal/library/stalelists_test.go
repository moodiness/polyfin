package library

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

// staleEnv is a followEnv whose addon answers with the streams or
// subtitles numbered as the next list sent on numbers (see answerNumbered),
// holding each request until it comes.
type staleEnv struct {
	followEnv
	numbers chan []int
}

func newStaleEnv(t *testing.T, delays ...time.Duration) staleEnv {
	t.Helper()
	numbers := make(chan []int, 10)
	reply := func(w http.ResponseWriter, r *http.Request, resource string) {
		select {
		case listed := <-numbers:
			answerNumbered(w, resource, listed)
		case <-r.Context().Done():
		}
	}
	return staleEnv{followEnv: repliedEnv(t, reply, delays...), numbers: numbers}
}

// expired lists the movie's three streams, which the follow-up confirms,
// then moves the clock past ten minutes, how long version lists are kept
// by default.
func (e staleEnv) expired() {
	e.t.Helper()
	e.numbers <- []int{1, 2, 3}
	e.numbers <- []int{1, 2, 3}
	if versions, err := e.service.Versions(e.t.Context(), e.member, e.movie); err != nil || len(versions) != 3 {
		e.t.Fatalf("first answer: %+v %v", versions, err)
	}
	e.settles("stream", 2)
	e.wait(11 * time.Minute)
}

// listed names the versions known, asking no addon.
func (e staleEnv) listed() []string {
	e.t.Helper()
	versions, err := e.service.KnownVersions(e.t.Context(), e.member, e.movie)
	if err != nil {
		e.t.Fatal(err)
	}
	names := make([]string, len(versions))
	for i, version := range versions {
		names[i] = version.Name
	}
	return names
}

func (e staleEnv) pending() int {
	e.t.Helper()
	n, err := e.service.Pending(e.t.Context(), e.member, e.movie)
	if err != nil {
		e.t.Fatal(err)
	}
	return n
}

// reopened opens the title again, as item details do: the versions known
// are listed at once, while the addon is asked again in the background.
func (e staleEnv) reopened() {
	e.t.Helper()
	versions, complete, err := e.service.VersionsNow(e.t.Context(), e.member, e.movie)
	if err != nil || complete || len(versions) != 3 {
		e.t.Fatalf("opened again: %d versions, complete %v, %v", len(versions), complete, err)
	}
	e.eventually("the addon to be asked again", func() bool { return e.asked("stream") == 3 })
}

func TestAnExpiredListIsListedWhileTheAddonIsAskedAgain(t *testing.T) {
	e := newStaleEnv(t, 20*time.Millisecond, 20*time.Millisecond)
	e.expired()
	e.reopened()
	if got := e.pending(); got != 1 {
		t.Errorf("%d addons pending while asked again, want 1", got)
	}
	if got := e.listed(); !slices.Equal(got, []string{"Source 1", "Source 2", "Source 3"}) {
		t.Errorf("listed while asked again: %q", got)
	}
	// It answers with part of the streams: the others stay listed after
	// them while it is followed up.
	e.numbers <- []int{3}
	e.eventually("the follow-up", func() bool { return e.asked("stream") == 4 })
	if got := e.listed(); !slices.Equal(got, []string{"Source 3", "Source 1", "Source 2"}) {
		t.Errorf("listed while followed up: %q", got)
	}
	if got := e.pending(); got != 1 {
		t.Errorf("%d addons pending while followed up, want 1", got)
	}
	// The follow-up lists no more: the schedule ends, and with it the
	// streams the addon no longer lists.
	e.numbers <- []int{3}
	e.settles("stream", 4)
	if got := e.listed(); !slices.Equal(got, []string{"Source 3"}) {
		t.Errorf("listed once the follow-ups ended: %q", got)
	}
	if got := e.pending(); got != 0 {
		t.Errorf("%d addons pending once the follow-ups ended", got)
	}
}

func TestAFollowUpListingEveryStreamEndsWithTheFullList(t *testing.T) {
	e := newStaleEnv(t, 20*time.Millisecond, 20*time.Millisecond)
	e.expired()
	e.reopened()
	e.numbers <- []int{2}
	e.eventually("the follow-up", func() bool { return e.asked("stream") == 4 })
	if got := e.listed(); !slices.Equal(got, []string{"Source 2", "Source 1", "Source 3"}) {
		t.Errorf("listed while followed up: %q", got)
	}
	// The follow-up lists every stream: more than the answer before it,
	// whatever was kept listed after that answer, so the addon is asked
	// once more.
	e.numbers <- []int{1, 2, 3}
	e.eventually("the second follow-up", func() bool { return e.asked("stream") == 5 })
	if got := e.listed(); !slices.Equal(got, []string{"Source 1", "Source 2", "Source 3"}) {
		t.Errorf("listed after the first follow-up: %q", got)
	}
	e.numbers <- []int{1, 2, 3}
	e.settles("stream", 5)
	if got := e.listed(); !slices.Equal(got, []string{"Source 1", "Source 2", "Source 3"}) {
		t.Errorf("listed once the follow-ups ended: %q", got)
	}
	if got := e.pending(); got != 0 {
		t.Errorf("%d addons pending once the follow-ups ended", got)
	}
}

func TestWaitingForVersionsWaitsForTheAnswerReplacingAStaleList(t *testing.T) {
	e := newStaleEnv(t, 20*time.Millisecond)
	e.expired()
	e.reopened()
	waited := make(chan []string, 1)
	go func() {
		versions, err := e.service.Versions(t.Context(), e.member, e.movie)
		if err != nil {
			t.Error(err)
		}
		names := make([]string, len(versions))
		for i, version := range versions {
			names[i] = version.Name
		}
		waited <- names
	}()
	select {
	case got := <-waited:
		t.Fatalf("versions listed from the stale list: %q", got)
	case <-time.After(100 * time.Millisecond):
	}
	// The request it waits for is the one details started: the addon is
	// asked once, and its answer listed with the streams kept after it.
	e.numbers <- []int{3}
	select {
	case got := <-waited:
		if !slices.Equal(got, []string{"Source 3", "Source 1", "Source 2"}) {
			t.Errorf("versions once the addon answered: %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("versions still waited once the addon answered")
	}
	e.eventually("the follow-up", func() bool { return e.asked("stream") == 4 })
	e.numbers <- []int{3}
	e.settles("stream", 4)
}

// An addon limiting requests answers with a notice that has nothing to
// play: the versions it listed before stay listed, the addon is not asked
// again meanwhile, and it is asked when the title opens again.
func TestANoticeKeepsTheVersionsOfAnExpiredList(t *testing.T) {
	e := newStaleEnv(t, 20*time.Millisecond)
	e.expired()
	e.reopened()
	// The addon limits requests for a while: whatever asks gets a notice.
	for range 3 {
		e.numbers <- []int{0}
	}
	e.eventually("the answer", func() bool { return e.pending() == 0 && e.scheduled() == 0 })
	if got := e.listed(); !slices.Equal(got, []string{"Source 1", "Source 2", "Source 3"}) {
		t.Errorf("listed after the notice: %q", got)
	}
	e.settles("stream", 3)
	for len(e.numbers) > 0 {
		<-e.numbers
	}
	versions, complete, err := e.service.VersionsNow(t.Context(), e.member, e.movie)
	if err != nil || complete || len(versions) != 3 {
		t.Fatalf("opened again: %d versions, complete %v, %v", len(versions), complete, err)
	}
	e.eventually("the addon to be asked again", func() bool { return e.asked("stream") == 4 })
	e.numbers <- []int{2}
	e.eventually("the follow-up", func() bool { return e.asked("stream") == 5 })
	e.numbers <- []int{2}
	e.settles("stream", 5)
	if got := e.listed(); !slices.Equal(got, []string{"Source 2"}) {
		t.Errorf("listed once the addon answered again: %q", got)
	}
}

func TestARefreshDropsStaleLists(t *testing.T) {
	e := newStaleEnv(t, 20*time.Millisecond)
	e.expired()
	if got := len(e.listed()); got != 3 {
		t.Fatalf("%d versions listed from the stale list, want 3", got)
	}
	if err := e.service.Refresh(t.Context(), e.member, e.movie); err != nil {
		t.Fatal(err)
	}
	if got := e.listed(); len(got) != 0 {
		t.Errorf("listed after a refresh: %q", got)
	}
	if versions, complete := e.service.CachedVersions(t.Context(), e.member, e.movie); complete || len(versions) != 0 {
		t.Errorf("cached after a refresh: %d versions, complete %v", len(versions), complete)
	}
}

func TestStaleListsAreGoneAfterADay(t *testing.T) {
	e := newStaleEnv(t, 20*time.Millisecond)
	e.expired()
	// Ten minutes fresh, then a day stale: a minute before its end, the
	// list is still listed, by listings too.
	e.wait(24*time.Hour - 2*time.Minute)
	if versions, complete := e.service.CachedVersions(t.Context(), e.member, e.movie); !complete || len(versions) != 3 {
		t.Fatalf("a minute before the day ends: %d versions, complete %v", len(versions), complete)
	}
	e.wait(2 * time.Minute)
	if got := e.listed(); len(got) != 0 {
		t.Errorf("listed a minute after the day ended: %q", got)
	}
	if versions, complete := e.service.CachedVersions(t.Context(), e.member, e.movie); complete || len(versions) != 0 {
		t.Errorf("cached a minute after the day ended: %d versions, complete %v", len(versions), complete)
	}
}

func TestStaleSubtitleListsAreListedWhileTheAddonIsAskedAgain(t *testing.T) {
	e := newStaleEnv(t, 20*time.Millisecond, 20*time.Millisecond)
	subtitles := func() ([]string, bool) {
		t.Helper()
		subtitles, complete, err := e.service.SubtitlesNow(t.Context(), e.member, e.movie)
		if err != nil {
			t.Fatal(err)
		}
		files := make([]string, len(subtitles))
		for i, subtitle := range subtitles {
			files[i] = subtitle.URL[strings.LastIndex(subtitle.URL, "/")+1:]
		}
		return files, complete
	}
	e.numbers <- []int{1, 2, 3}
	e.numbers <- []int{1, 2, 3}
	if listed, err := e.service.Subtitles(t.Context(), e.member, e.movie); err != nil || len(listed) != 3 {
		t.Fatalf("first answer: %+v %v", listed, err)
	}
	e.settles("subtitles", 2)
	e.wait(11 * time.Minute)
	if got, complete := subtitles(); complete || !slices.Equal(got, []string{"1.srt", "2.srt", "3.srt"}) {
		t.Fatalf("opened again: %q, complete %v", got, complete)
	}
	e.eventually("the addon to be asked again", func() bool { return e.asked("subtitles") == 3 })
	e.numbers <- []int{3}
	e.eventually("the follow-up", func() bool { return e.asked("subtitles") == 4 })
	if got, complete := subtitles(); !complete || !slices.Equal(got, []string{"3.srt", "1.srt", "2.srt"}) {
		t.Errorf("while followed up: %q, complete %v", got, complete)
	}
	e.numbers <- []int{3}
	e.settles("subtitles", 4)
	if got, _ := subtitles(); !slices.Equal(got, []string{"3.srt"}) {
		t.Errorf("once the follow-ups ended: %q", got)
	}
}
