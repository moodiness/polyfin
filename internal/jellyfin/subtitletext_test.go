package jellyfin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/speech"
	"github.com/moodiness/polyfin/internal/subtitles"
)

// An ASS subtitle synced across a cut keeps its script, cut as its cues
// are: the events of the scene the version lacks are left out, and those
// after it are shifted by the cut's offset.
func TestASyncedScriptIsCutAsItsCues(t *testing.T) {
	t.Parallel()
	text, err := readSubtitleText([]byte("[Script Info]\nScriptType: v4.00+\n\n[Events]\n" +
		"Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n" +
		"Dialogue: 0,0:00:10.00,0:00:12.00,Default,,0,0,0,,Before\n" +
		"Dialogue: 0,0:01:05.00,0:01:07.00,Default,,0,0,0,,Lacking\n" +
		"Dialogue: 0,0:02:10.00,0:02:12.00,Default,,0,0,0,,After\n"))
	if err != nil || text.script == nil {
		t.Fatalf("read %+v: %v", text, err)
	}
	timing := speech.Timing{Ratio: 1, Offset: time.Second, Cuts: []speech.Cut{{Start: time.Minute, End: 2 * time.Minute, Offset: -time.Minute}}}
	retimed := text.retimed(timing)
	want := []struct {
		start time.Duration
		text  string
	}{{11 * time.Second, "Before"}, {70 * time.Second, "After"}}
	if len(retimed.script.Events) != len(want) || len(retimed.cues) != len(want) {
		t.Fatalf("events %+v, cues %+v", retimed.script.Events, retimed.cues)
	}
	for i, w := range want {
		event, cue := retimed.script.Events[i], retimed.cues[i]
		if event.Start != w.start || !strings.HasSuffix(event.Rest, w.text) || cue.Start != w.start {
			t.Errorf("event %d starts at %s (%s), its cue at %s, want %s (%s)", i, event.Start, event.Rest, cue.Start, w.start, w.text)
		}
	}
}

// subtitleSource serves subtitle files to a test and counts the requests
// for each of them.
type subtitleSource struct {
	url   string
	mu    sync.Mutex
	asked map[string]int
}

// newSubtitleSource starts a source that answers the nth request for a
// path, from 1, with the status and body answer gives.
func newSubtitleSource(t *testing.T, answer func(path string, nth int) (int, string)) *subtitleSource {
	t.Helper()
	source := &subtitleSource{asked: map[string]int{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		source.mu.Lock()
		source.asked[r.URL.Path]++
		nth := source.asked[r.URL.Path]
		source.mu.Unlock()
		status, body := answer(r.URL.Path, nth)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	source.url = server.URL
	return source
}

// file describes the source's file at path, known by id.
func (s *subtitleSource) file(id byte, path string) library.ExternalSubtitle {
	return library.ExternalSubtitle{ID: accounts.ID{id}, URL: s.url + path}
}

// requests counts the requests the source got for path.
func (s *subtitleSource) requests(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.asked[path]
}

// smallSubtitle is a SubRip file of one cue.
const smallSubtitle = "1\n00:00:01,000 --> 00:00:02,000\nHello\n"

// tooManyEvents is a script of one event more than subtitles reads, in
// less than the 8 MiB a download may take.
func tooManyEvents() string {
	return "[Script Info]\n[Events]\nFormat: Start, End, Text\n" + strings.Repeat("Dialogue:0:0:1,0:0:2,x\n", subtitles.MaxCues+1)
}

// tooLargeText is a SubRip file of one cue, in less than the 8 MiB a
// download may take, whose text counts for more than the 32 MiB of
// subtitle texts kept: each of its characters takes six in JSON.
func tooLargeText() string {
	return "1\n00:00:01,000 --> 00:00:02,000\n" + strings.Repeat("<", 6<<20) + "\n"
}

// A subtitle file refused for its size is refused from the first request,
// as a file that cannot be read is, and is not read again at the next: a
// script of more events than subtitles reads, and a file whose text is
// larger than the texts kept, which is never served. A read that failed
// for another reason is tried again.
func TestASubtitleFileRefusedForItsSizeIsNotReadAgain(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	events, large := tooManyEvents(), tooLargeText()
	source := newSubtitleSource(t, func(path string, nth int) (int, string) {
		switch {
		case path == "/events.ass":
			return http.StatusOK, events
		case path == "/large.srt":
			return http.StatusOK, large
		case path == "/page.srt":
			return http.StatusOK, "<!DOCTYPE html><html></html>"
		case path == "/late.srt" && nth > 1:
			return http.StatusOK, smallSubtitle
		}
		return http.StatusServiceUnavailable, ""
	})
	// The subtitle route answers as for a file that is not a subtitle,
	// and asks the source for the script once.
	user := s.user("member", nil)
	token := s.signIn("member", "tv")
	item := accounts.ID{9}
	s.handler.subtitleFiles.Put(item, listedSubtitles{User: user.ID, Files: []library.ExternalSubtitle{
		source.file(1, "/events.ass"), source.file(2, "/page.srt")}})
	for range 2 {
		for index, path := range []string{"/events.ass", "/page.srt"} {
			address := fmt.Sprintf("/Videos/%s/%s/Subtitles/%d/Stream.vtt", item, item, index)
			if status, body := s.call(http.MethodGet, address, app("tv", token), nil); status != http.StatusInternalServerError {
				t.Errorf("%s through its route: %d %s", path, status, body)
			}
		}
	}
	if _, err := s.handler.subtitleFile(t.Context(), source.file(1, "/events.ass")); !errors.Is(err, subtitles.ErrTooManyCues) {
		t.Errorf("a script of too many events: %v", err)
	}
	if got := source.requests("/events.ass"); got != 1 {
		t.Errorf("the script of too many events was asked for %d times in 3 requests", got)
	}

	for range 2 {
		text, err := s.handler.subtitleFile(t.Context(), source.file(3, "/large.srt"))
		if !errors.Is(err, errSubtitleTooLarge) || len(text.cues) != 0 {
			t.Errorf("a text too large to keep: %d cues, %v", len(text.cues), err)
		}
	}
	if got := source.requests("/large.srt"); got != 1 {
		t.Errorf("the file too large to keep was asked for %d times in 2 requests", got)
	}
	if _, kept := s.handler.subtitleCache.Get(accounts.ID{3}); kept {
		t.Error("the text too large to keep is kept")
	}

	late := source.file(4, "/late.srt")
	if _, err := s.handler.subtitleFile(t.Context(), late); err == nil || errors.Is(err, errSubtitleTooLarge) || errors.Is(err, subtitles.ErrTooManyCues) {
		t.Errorf("a file its source does not serve: %v", err)
	}
	for range 2 {
		if text, err := s.handler.subtitleFile(t.Context(), late); err != nil || len(text.cues) != 1 {
			t.Errorf("the file once its source serves it: %d cues, %v", len(text.cues), err)
		}
	}
	if got := source.requests("/late.srt"); got != 2 {
		t.Errorf("the file served late was asked for %d times, want one failure and one read", got)
	}
}

// The requests asking for a subtitle file at the same time share one
// read: ten at once reach the source once, for a file that is kept as for
// one that is refused.
func TestRequestsAtOnceShareOneReadOfASubtitleFile(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	events := tooManyEvents()
	held := make(chan struct{})
	source := newSubtitleSource(t, func(path string, _ int) (int, string) {
		<-held
		if path == "/events.ass" {
			return http.StatusOK, events
		}
		return http.StatusOK, smallSubtitle
	})
	release := sync.OnceFunc(func() { close(held) })
	t.Cleanup(release)
	const callers = 10
	type answer struct {
		text subtitleText
		err  error
	}
	paths := []string{"/events.ass", "/small.srt"}
	answers := map[string]chan answer{}
	var asking sync.WaitGroup
	for i, path := range paths {
		answered := make(chan answer, callers)
		answers[path] = answered
		file := source.file(byte(i+1), path)
		for range callers {
			asking.Add(1)
			go func() {
				asking.Done()
				text, err := s.handler.subtitleFile(t.Context(), file)
				answered <- answer{text, err}
			}()
		}
	}
	asking.Wait()
	release()
	for range callers {
		if a := <-answers["/events.ass"]; !errors.Is(a.err, subtitles.ErrTooManyCues) {
			t.Errorf("a script of too many events: %v", a.err)
		}
		if a := <-answers["/small.srt"]; a.err != nil || len(a.text.cues) != 1 {
			t.Errorf("a small file: %d cues, %v", len(a.text.cues), a.err)
		}
	}
	for _, path := range paths {
		if got := source.requests(path); got != 1 {
			t.Errorf("%s was asked for %d times by %d requests at once", path, got, callers)
		}
	}
}

// A caller that gives up does not end the read it shares: the one still
// waiting gets the file, and the next finds it kept.
func TestACallerLeavingDoesNotEndASharedRead(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	arrived, held := make(chan struct{}, 16), make(chan struct{})
	source := newSubtitleSource(t, func(string, int) (int, string) {
		arrived <- struct{}{}
		<-held
		return http.StatusOK, smallSubtitle
	})
	release := sync.OnceFunc(func() { close(held) })
	t.Cleanup(release)
	file := source.file(1, "/small.srt")
	leaving, leave := context.WithCancel(t.Context())
	defer leave()
	left := make(chan error, 1)
	go func() {
		_, err := s.handler.subtitleFile(leaving, file)
		left <- err
	}()
	<-arrived
	type answer struct {
		text subtitleText
		err  error
	}
	waiting := make(chan answer, 1)
	go func() {
		text, err := s.handler.subtitleFile(t.Context(), file)
		waiting <- answer{text, err}
	}()
	leave()
	if err := <-left; !errors.Is(err, context.Canceled) {
		t.Errorf("the caller that left: %v", err)
	}
	release()
	if a := <-waiting; a.err != nil || len(a.text.cues) != 1 {
		t.Errorf("the caller still waiting: %d cues, %v", len(a.text.cues), a.err)
	}
	if text, err := s.handler.subtitleFile(t.Context(), file); err != nil || len(text.cues) != 1 {
		t.Errorf("the next caller: %d cues, %v", len(text.cues), err)
	}
	if got := source.requests("/small.srt"); got != 1 {
		t.Errorf("the file was asked for %d times", got)
	}
}

// A panic of what reads a subtitle file fails the read it happens in, for
// the requests sharing it, and is logged with its stack: the server goes
// on. It is not remembered, so the next request reads the file.
func TestAPanicWhileReadingASubtitleFileFailsThatReadAlone(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	source := newSubtitleSource(t, func(string, int) (int, string) { return http.StatusOK, smallSubtitle })
	file := source.file(1, "/small.srt")
	var failing atomic.Bool
	failing.Store(true)
	var panicked atomic.Int64
	entered, held := make(chan struct{}, 16), make(chan struct{})
	reading := func(context.Context, accounts.ID) {
		if !failing.Load() {
			return
		}
		panicked.Add(1)
		entered <- struct{}{}
		<-held
		panic("the read went wrong")
	}
	s.handler.subtitleReads.reading.Store(&reading)
	release := sync.OnceFunc(func() { close(held) })
	t.Cleanup(release)
	answers := make(chan error, 2)
	ask := func() {
		_, err := s.handler.subtitleFile(t.Context(), file)
		answers <- err
	}
	go ask()
	<-entered
	go ask()
	release()
	for range 2 {
		if err := <-answers; !errors.Is(err, errSubtitleReadPanicked) || !strings.Contains(err.Error(), "the read went wrong") {
			t.Errorf("a request waiting for the read: %v", err)
		}
	}
	// The second request shared the first's read, or began its own once
	// that one had ended: each read that panicked is logged once.
	logged := s.warnings.attrsOf("Reading a subtitle file ended on a panic")
	if reads := int(panicked.Load()); len(logged) != reads || reads < 1 || reads > 2 {
		t.Fatalf("%d reads panicked, %d logged", reads, len(logged))
	}
	for _, attrs := range logged {
		if attrs["subtitle"] != file.ID.String() || attrs["panic"] != "the read went wrong" || !strings.Contains(attrs["stack"], "sharedSubtitleRead") {
			t.Errorf("logged %v", attrs)
		}
	}
	if err := s.handler.subtitleReads.refusal(file.ID, s.handler.now()); err != nil {
		t.Errorf("the panic is remembered as a refusal: %v", err)
	}
	if got := source.requests("/small.srt"); got != 0 {
		t.Errorf("the file was asked for %d times before a read reached it", got)
	}
	failing.Store(false)
	if text, err := s.handler.subtitleFile(t.Context(), file); err != nil || len(text.cues) != 1 {
		t.Errorf("the next request: %d cues, %v", len(text.cues), err)
	}
	if got := source.requests("/small.srt"); got != 1 {
		t.Errorf("the file was asked for %d times by the next request", got)
	}
}

// The read requests share bounds itself: its context ends
// subtitleReadTime after it began, whatever the context of the request
// that began it, for an addon's file, a file a user added and a file
// inside a torrent. Each kind of read ends with the context it is given.
func TestASharedSubtitleReadBoundsItself(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	source := newSubtitleSource(t, func(string, int) (int, string) { return http.StatusOK, smallSubtitle })
	files := map[string]library.ExternalSubtitle{
		"an addon's file":         source.file(1, "/small.srt"),
		"a file a user added":     {ID: accounts.ID{2}, Uploaded: true},
		"a file inside a torrent": {ID: accounts.ID{3}, Torrent: &library.TorrentSubtitle{Version: accounts.ID{4}}},
	}
	type start struct {
		deadline time.Time
		bounded  bool
		ended    error
	}
	var mu sync.Mutex
	starts := map[accounts.ID]start{}
	reading := func(ctx context.Context, file accounts.ID) {
		deadline, bounded := ctx.Deadline()
		mu.Lock()
		defer mu.Unlock()
		starts[file] = start{deadline, bounded, ctx.Err()}
	}
	s.handler.subtitleReads.reading.Store(&reading)
	// The request's own context ends an hour later: the read's does not
	// follow it.
	asking, cancel := context.WithTimeout(t.Context(), time.Hour)
	defer cancel()
	for kind, file := range files {
		before := time.Now()
		_, err := s.handler.subtitleFile(asking, file)
		after := time.Now()
		// The files of the two last kinds do not exist: their reads go as
		// far as looking for them.
		if file.URL == "" && !errors.Is(err, library.ErrNotFound) || file.URL != "" && err != nil {
			t.Errorf("%s: %v", kind, err)
		}
		mu.Lock()
		began := starts[file.ID]
		mu.Unlock()
		if !began.bounded || began.deadline.Before(before.Add(subtitleReadTime)) || began.deadline.After(after.Add(subtitleReadTime)) || began.ended != nil {
			t.Errorf("%s: its read began with a deadline (%v) in %s, ended %v; want one in %s",
				kind, began.bounded, began.deadline.Sub(before).Round(time.Second), began.ended, subtitleReadTime)
		}
	}
	asked := source.requests("/small.srt")
	past, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	for kind, file := range files {
		if _, err := s.handler.readSubtitleFile(past, file); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("%s read past its deadline: %v", kind, err)
		}
	}
	if got := source.requests("/small.srt"); got != asked {
		t.Errorf("the source was asked %d times past the deadline", got-asked)
	}
}

// A file refused for its size is remembered for as long as a text is
// kept, an hour, on the handler's clock: asked for meanwhile, it is not
// read; after that, it is read again, and remembered from then.
func TestARefusedSubtitleFileIsReadAgainAfterAnHour(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	var clock atomic.Pointer[time.Time]
	now := time.Now()
	clock.Store(&now)
	s.handler.now = func() time.Time { return *clock.Load() }
	advance := func(d time.Duration) {
		next := clock.Load().Add(d)
		clock.Store(&next)
	}
	events := tooManyEvents()
	source := newSubtitleSource(t, func(string, int) (int, string) { return http.StatusOK, events })
	refused := func(when string, reads int) {
		t.Helper()
		if _, err := s.handler.subtitleFile(t.Context(), source.file(1, "/events.ass")); !errors.Is(err, subtitles.ErrTooManyCues) {
			t.Errorf("%s: %v", when, err)
		}
		if got := source.requests("/events.ass"); got != reads {
			t.Errorf("%s: the file was asked for %d times, want %d", when, got, reads)
		}
	}
	refused("at first", 1)
	refused("at once after", 1)
	advance(time.Hour)
	refused("an hour later", 1)
	advance(time.Second)
	refused("past the hour", 2)
	advance(time.Hour)
	refused("an hour after it was read again", 2)
}

// The files remembered as refused are bounded: past
// maxRefusedSubtitles, the oldest gives way, a file refused again counts
// once, and those refused more than an hour ago are dropped.
func TestTheOldestRefusedSubtitleFilesGiveWay(t *testing.T) {
	t.Parallel()
	var reads subtitleReads
	id := func(n int) accounts.ID { return accounts.ID{byte(n), byte(n >> 8), 1} }
	start := time.Now()
	for n := range maxRefusedSubtitles + 1 {
		reads.refuse(id(n), errSubtitleTooLarge, start.Add(time.Duration(n)*time.Second))
	}
	now := start.Add(maxRefusedSubtitles * time.Second)
	if len(reads.refused) != maxRefusedSubtitles || reads.refusal(id(0), now) != nil {
		t.Errorf("%d files remembered, the oldest among them: %v", len(reads.refused), reads.refusal(id(0), now))
	}
	for n := 1; n <= maxRefusedSubtitles; n++ {
		if !errors.Is(reads.refusal(id(n), now), errSubtitleTooLarge) {
			t.Fatalf("file %d is forgotten", n)
		}
	}
	reads.refuse(id(1), subtitles.ErrTooManyCues, now)
	reads.refuse(id(maxRefusedSubtitles+1), errSubtitleTooLarge, now)
	if len(reads.refused) != maxRefusedSubtitles || !errors.Is(reads.refusal(id(1), now), subtitles.ErrTooManyCues) || reads.refusal(id(2), now) != nil {
		t.Errorf("%d files remembered; the one refused again: %v; the oldest: %v", len(reads.refused), reads.refusal(id(1), now), reads.refusal(id(2), now))
	}
	later := now.Add(time.Hour + time.Second)
	reads.refuse(id(0), errSubtitleTooLarge, later)
	if len(reads.refused) != 1 || reads.refusal(id(1), later) != nil || reads.refusal(id(0), later) == nil {
		t.Errorf("%d files remembered an hour later", len(reads.refused))
	}
	reads.forget(id(0))
	if len(reads.refused) != 0 || reads.refusal(id(0), later) != nil {
		t.Errorf("%d files remembered once the last is forgotten", len(reads.refused))
	}
}

// Refreshing a title forgets which of its subtitle files were refused for
// their size, as it forgets their texts: a file replaced at the address
// it had is read again.
func TestRefreshingATitleForgetsItsRefusedSubtitleFiles(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("member", nil)
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	token, admin := s.signIn("member", "tv"), s.signIn("admin", "tv")
	var generation atomic.Int64
	generation.Store(1)
	movie, _ := installCaptioned(t, s, &generation, token)
	item, err := accounts.ParseID(movie)
	if err != nil {
		t.Fatal(err)
	}
	events := tooManyEvents()
	var replaced atomic.Bool
	source := newSubtitleSource(t, func(string, int) (int, string) {
		if replaced.Load() {
			return http.StatusOK, smallSubtitle
		}
		return http.StatusOK, events
	})
	file := source.file(1, "/events.ass")
	s.handler.subtitleFiles.Put(item, listedSubtitles{Files: []library.ExternalSubtitle{file}})
	for _, replace := range []bool{false, true} {
		replaced.Store(replace)
		if _, err := s.handler.subtitleFile(t.Context(), file); !errors.Is(err, subtitles.ErrTooManyCues) {
			t.Errorf("before the refresh, replaced %v: %v", replace, err)
		}
	}
	if got := source.requests("/events.ass"); got != 1 {
		t.Errorf("before the refresh, the file was asked for %d times", got)
	}
	if status, body := s.call(http.MethodPost, "/Items/"+movie+"/Refresh", app("tv", admin), nil); status != http.StatusNoContent {
		t.Fatalf("refresh: %d %s", status, body)
	}
	if text, err := s.handler.subtitleFile(t.Context(), file); err != nil || len(text.cues) != 1 {
		t.Errorf("after the refresh: %d cues, %v", len(text.cues), err)
	}
	if got := source.requests("/events.ass"); got != 2 {
		t.Errorf("after the refresh, the file was asked for %d times", got)
	}
}
