package library

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// logEnv is a followEnv whose addon answers each stream request with the
// next body sent on bodies, holding it until it comes, an empty one a
// failure, and whose service logs into log.
type logEnv struct {
	followEnv
	bodies chan string
	log    *lockedBuffer
}

func newLogEnv(t *testing.T) logEnv {
	t.Helper()
	bodies := make(chan string, 10)
	e := repliedEnv(t, func(w http.ResponseWriter, r *http.Request, resource string) {
		if resource != "stream" {
			answerNumbered(w, resource, nil)
			return
		}
		select {
		case body := <-bodies:
			if body == "" {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(body))
		case <-r.Context().Done():
		}
	})
	log := &lockedBuffer{}
	e.service.logger = slog.New(slog.NewTextHandler(log, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return logEnv{followEnv: e, bodies: bodies, log: log}
}

// lines are the lines logged with message, those logged before skipped.
func (e logEnv) lines(message string, skipped int) []string {
	var lines []string
	for line := range strings.Lines(e.log.String()) {
		if strings.Contains(line, `msg="`+message) {
			lines = append(lines, line)
		}
	}
	return lines[min(skipped, len(lines)):]
}

// listed lists the movie's versions on ctx, waiting for the addon.
func (e logEnv) listed(ctx context.Context) {
	e.t.Helper()
	if _, err := e.service.Versions(ctx, e.member, e.movie); err != nil {
		e.t.Fatal(err)
	}
}

// An answer of notices alone that refuses nothing is information, logged
// once in six hours for the same addon, title and text, whatever its
// digits, at Debug in between; logged again, it counts the answers that
// repeated it, at Debug for background work alone. A refusal for the rate
// is a warning. The line counts the entries, and those of each shape
// Polyfin cannot play.
func TestNoticesAreLoggedOnceInSixHoursWithTheirRepeats(t *testing.T) {
	t.Parallel()
	e := newLogEnv(t)
	summary := func(filtered int) string {
		return fmt.Sprintf(`{"streams":[
		{"name":"Summary","description":"%d results filtered out"},
		{"name":"Usenet","nzbUrl":"https://downloads.example/a.nzb","servers":["nntps://news.example:563"]},
		{"name":"Rar","rarUrls":[{"url":"https://downloads.example/a.rar","bytes":10}]},
		{"name":"Seven","7zipUrls":[{"url":"https://downloads.example/a.7z"}]},
		{"name":"Ftp","url":"ftp://downloads.example/a.mkv"},
		{"name":"Hash","infoHash":"abc"},
		{"name":"Clip","ytId":"abc123"}]}`, filtered)
	}
	const message = "An addon answered only notices, nothing to play"
	level := func(what string, skipped int, want string) string {
		t.Helper()
		lines := e.lines(message, skipped)
		if len(lines) != 1 || !strings.Contains(lines[0], "level="+want) {
			t.Fatalf("%s: %q, want one %s line", what, lines, want)
		}
		return lines[0]
	}
	e.bodies <- summary(12)
	e.listed(t.Context())
	line := level("a summary", 0, "INFO")
	for _, field := range []string{"type=movie", "id=ttm000", "entries=7", "no_link=2", "usenet=1", "archives=2", "other_schemes=1",
		"invalid_info_hash=1", `notice="Summary: 12 results filtered out | Usenet | Rar`} {
		if !strings.Contains(line, field) {
			t.Errorf("no %s: %s", field, line)
		}
	}
	if strings.Contains(line, "minute") || strings.Contains(line, "downloads.example") || strings.Contains(line, "repeats=") {
		t.Errorf("line: %s", line)
	}
	// Past the answer's freshness, a user's request asks again.
	e.wait(11 * time.Minute)
	e.bodies <- summary(13)
	e.listed(t.Context())
	level("the same summary within six hours", 1, "DEBUG")
	e.wait(noticeLogEvery)
	e.bodies <- summary(14)
	e.listed(t.Context())
	if line := level("the same summary six hours later", 2, "INFO"); !strings.Contains(line, "repeats=1 ") {
		t.Errorf("no repeats=1: %s", line)
	}
	e.wait(noticeLogEvery)
	e.bodies <- summary(15)
	e.listed(Background(t.Context()))
	if line := level("background work's six hours later", 3, "DEBUG"); !strings.Contains(line, "repeats=0 ") {
		t.Errorf("no repeats=0: %s", line)
	}
	e.wait(11 * time.Minute)
	e.bodies <- `{"streams":[{"name":"Notice","description":"Rate limit exceeded"}]}`
	e.listed(t.Context())
	level("a refusal", 4, "WARN")
}

// A failed request for an addon's streams is logged once, however many
// callers share it, with the title: information when only background work
// waited for it, a warning when a user did too.
func TestAFailedStreamRequestIsLoggedOnceAtItsLevel(t *testing.T) {
	t.Parallel()
	e := newLogEnv(t)
	var mu sync.Mutex
	joins := 0
	e.service.joined = func(key string) {
		if strings.HasPrefix(key, "streams ") {
			mu.Lock()
			joins++
			mu.Unlock()
		}
	}
	joined := func() int {
		mu.Lock()
		defer mu.Unlock()
		return joins
	}
	const message = "An addon could not list streams"
	shared := func(what string, skipped int, want string, contexts ...context.Context) {
		t.Helper()
		before := joined()
		var wg sync.WaitGroup
		for _, ctx := range contexts {
			wg.Go(func() { _, _ = e.service.Versions(ctx, e.member, e.movie) })
		}
		e.eventually(what+": every caller joined", func() bool { return joined() == before+len(contexts) })
		e.bodies <- ""
		wg.Wait()
		lines := e.lines(message, skipped)
		if len(lines) != 1 || !strings.Contains(lines[0], "level="+want) || !strings.Contains(lines[0], "type=movie") ||
			!strings.Contains(lines[0], "id=ttm000") || !strings.Contains(lines[0], "error=") {
			t.Errorf("%s: %q, want one %s line", what, lines, want)
		}
	}
	shared("background work", 0, "INFO", Background(t.Context()), Background(t.Context()))
	shared("background work and a user", 1, "WARN", Background(t.Context()), t.Context())
	shared("a user", 2, "WARN", t.Context())
}
