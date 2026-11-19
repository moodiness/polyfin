package playback

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/library"
)

// A panic of work shared by requests (an analysis, keyframes, a track,
// attached files, an index read ahead) is the error of every request
// waiting for it, logged once with its stack; the server goes on, and the
// next request runs the work again.
func TestAPanicInSharedWorkIsItsWaitersError(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	s := &Service{logger: slog.New(slog.NewTextHandler(&logs, nil))}
	release := make(chan struct{})
	work := func() (any, error) {
		<-release
		panic("a broken index")
	}
	// The waiters join the work before it runs on: DoChan returns at once.
	waiters := make([]<-chan singleflight.Result, 4)
	for i := range waiters {
		waiters[i] = s.doShared("track", work)
	}
	close(release)
	for i, results := range waiters {
		result := <-results
		if !errors.Is(result.Err, errSharedPanicked) || !strings.Contains(result.Err.Error(), "a broken index") {
			t.Errorf("waiter %d: %v, want the work's panic", i, result.Err)
		}
	}
	if got := strings.Count(logs.String(), "ended on a panic"); got != 1 || !strings.Contains(logs.String(), "stack=") {
		t.Fatalf("the panic was logged %d times, want once with its stack: %s", got, logs.String())
	}
	// shared, under which analyses, keyframes and tracks run.
	if _, err := s.shared(t.Context(), "analysis", work); !errors.Is(err, errSharedPanicked) {
		t.Errorf("shared: %v, want the work's panic", err)
	}
	if got, err := s.shared(t.Context(), "track", func() (any, error) { return 1, nil }); got != 1 || err != nil {
		t.Errorf("the work run again: %v, %v", got, err)
	}
}

// panickingBody is a feed's body whose reader panics.
type panickingBody struct{}

func (panickingBody) Read([]byte) (int, error) { panic("a broken stream") }
func (panickingBody) Close() error             { return nil }

// A panic reading a feed's first bytes fails the feed as a body that
// ended at once, logged with its stack.
func TestAPanicReadingAFeedsFirstBytesIsItsError(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	s := &Service{logger: slog.New(slog.NewTextHandler(&logs, nil))}
	if _, _, err := s.readHead(panickingBody{}, time.Hour); !errors.Is(err, ErrLiveDead) {
		t.Fatalf("readHead: %v, want ErrLiveDead", err)
	}
	if !strings.Contains(logs.String(), "a broken stream") || !strings.Contains(logs.String(), "stack=") {
		t.Fatalf("the panic was not logged with its stack: %s", logs.String())
	}
}

// A panic finding a version's speech fails that version for a while,
// logged with its stack; the queue goes on to the next one and ends. A
// service with no database panics as it reads the speech stored.
func TestAPanicFindingSpeechFailsThatVersionOnly(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	s := &Service{logger: slog.New(slog.NewTextHandler(&logs, nil)), speech: newSpeechState()}
	first, second := library.Version{ID: accounts.ID{1}}, library.Version{ID: accounts.ID{2}}
	s.speech.jobs, s.speech.running = []library.Version{first, second}, true
	s.findQueuedSpeech()
	for _, version := range []library.Version{first, second} {
		if err, failed := s.speech.failed.Get(version.ID); !failed || !errors.Is(err, errSharedPanicked) {
			t.Errorf("version %v: %v, %v, want the panic", version.ID, err, failed)
		}
	}
	if s.speech.running || len(s.speech.jobs) > 0 {
		t.Errorf("the queue did not end: %v, %d left", s.speech.running, len(s.speech.jobs))
	}
	if !strings.Contains(logs.String(), "stack=") {
		t.Fatalf("the panic was not logged with its stack: %s", logs.String())
	}
}

// A panic warming a version fails that warm, logged with its stack: the
// warm is forgotten, not remembered as done. A service with no database
// panics as it reads the version.
func TestAPanicWarmingAVersionFailsThatWarm(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	s := &Service{logger: slog.New(slog.NewTextHandler(&logs, nil)), warming: map[accounts.ID]*warmRun{},
		warmed: cache.New[accounts.ID, time.Duration](10, time.Hour), speech: newSpeechState()}
	version := library.Version{ID: accounts.ID{1}}
	run := &warmRun{}
	s.warming[version.ID] = run
	canceled := false
	s.runWarm(t.Context(), func() { canceled = true }, version, 0, run)
	if _, ok := s.warming[version.ID]; ok || !canceled {
		t.Errorf("the warm was not forgotten: %v, canceled %v", s.warming, canceled)
	}
	if _, ok := s.warmed.Get(version.ID); ok {
		t.Error("the warm that panicked is remembered as done")
	}
	if !strings.Contains(logs.String(), "ended on a panic") || !strings.Contains(logs.String(), "stack=") {
		t.Fatalf("the panic was not logged with its stack: %s", logs.String())
	}
}
