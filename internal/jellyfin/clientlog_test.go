package jellyfin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/logs"
)

// standardError stands for the server's standard error: a buffer several
// requests write to at once.
type standardError struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (w *standardError) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (w *standardError) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

// loggingServer is a test server that logs as Polyfin does: a text handler
// at the Info level writing to standard error, here a buffer, and to the
// ring /System/Logs serves. It has an administrator, root, and two members,
// alice and bob.
func loggingServer(t *testing.T) (s testServer, stderr *standardError, ring *logs.Ring) {
	t.Helper()
	stderr, ring = &standardError{}, logs.NewRing(logs.Capacity)
	logger := slog.New(slog.NewTextHandler(io.MultiWriter(stderr, ring), &slog.HandlerOptions{Level: slog.LevelInfo}))
	s = newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, _ *pgxpool.Pool) {
		o.Logger, o.Logs = logger, ring
	})
	s.user("root", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	s.user("alice", nil)
	s.user("bob", nil)
	return s, stderr, ring
}

// uploadedName is the file name an uploaded log is answered with.
var uploadedName = regexp.MustCompile(`^upload_Test App_1\.0\.0_\d{14}_[0-9a-f]{32}\.log$`)

func TestAnUploadedAppLogStaysOutOfTheServerLog(t *testing.T) {
	t.Parallel()
	s, stderr, ring := loggingServer(t)
	admin, member := app("dashboard", s.signIn("root", "dashboard")), app("tv", s.signIn("alice", "tv"))

	// A line the server logs by itself, which an administrator reads.
	if status, body := s.call(http.MethodPost, "/Users/ForgotPassword", "", map[string]string{"EnteredUsername": "root"}); status != http.StatusOK {
		t.Fatalf("forgot password: %d %s", status, body)
	}
	const earlier = "A password reset PIN was made"
	if _, body := s.call(http.MethodGet, "/System/Logs/Log?name=polyfin.log", admin, nil); !strings.Contains(string(body), earlier) {
		t.Fatalf("the earlier line is not in the log: %s", body)
	}

	// Each upload may add this much to standard error and to the ring: a
	// line that names the file, the user and the device.
	const line = 512
	errBefore := len(stderr.String())
	_, _, ringBefore := ring.Stat()
	// Quotes and control characters are what the text log writes longest.
	for i, unit := range []string{`"`, "\x01", "pw=1 "} {
		status, body := s.postRaw("/ClientLog/Document", member, strings.Repeat(unit, maxClientLog/len(unit)))
		var answer ClientLogDocumentResponseDto
		if err := json.Unmarshal(body, &answer); status != http.StatusOK || err != nil || !uploadedName.MatchString(answer.FileName) {
			t.Fatalf("upload %d: %d %s", i, status, body)
		}
		if !strings.Contains(stderr.String(), "file=\""+answer.FileName+"\"") {
			t.Errorf("upload %d: the log does not name %s", i, answer.FileName)
		}
	}
	if grew := len(stderr.String()) - errBefore; grew <= 0 || grew > 3*line {
		t.Errorf("standard error grew by %d bytes", grew)
	}
	if _, _, size := ring.Stat(); size-ringBefore <= 0 || size-ringBefore > 3*line {
		t.Errorf("the ring grew by %d bytes", size-ringBefore)
	}
	status, body := s.call(http.MethodGet, "/System/Logs/Log?name=polyfin.log", admin, nil)
	if status != http.StatusOK || !strings.Contains(string(body), earlier) {
		t.Errorf("the earlier line is no longer served: %d, %d bytes", status, len(body))
	}
	if got := strings.Count(string(body), `msg="An app sent its log, which is not kept"`); got != 3 {
		t.Errorf("%d lines tell of an upload: %s", got, body)
	}
	if !strings.Contains(string(body), fmt.Sprintf("user=alice device=tv bytes=%d\n", maxClientLog)) {
		t.Errorf("no line tells who sent how much: %s", body)
	}
}

func TestAppLogUploadsAreLimitedForEachUser(t *testing.T) {
	t.Parallel()
	s, _, _ := loggingServer(t)
	var clock atomic.Pointer[time.Time]
	at := time.Date(2026, time.November, 2, 10, 0, 0, 0, time.UTC)
	clock.Store(&at)
	s.handler.now = func() time.Time { return *clock.Load() }
	advance := func(by time.Duration) {
		next := clock.Load().Add(by)
		clock.Store(&next)
	}
	alice, bob := app("tv", s.signIn("alice", "tv")), app("phone", s.signIn("bob", "phone"))
	// upload answers the status and the Retry-After of an upload.
	upload := func(authorization string) (int, string) {
		t.Helper()
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, s.url+"/ClientLog/Document", strings.NewReader("app log"))
		request.Header.Set("Authorization", authorization)
		response, err := testClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, response.Body)
		return response.StatusCode, response.Header.Get("Retry-After")
	}
	for i := range clientLogsAMinute {
		if status, _ := upload(alice); status != http.StatusOK {
			t.Fatalf("upload %d: %d", i, status)
		}
		advance(time.Second)
	}
	// The first of the minute's uploads was 3 seconds ago.
	if status, wait := upload(alice); status != http.StatusTooManyRequests || wait != "57" {
		t.Fatalf("an upload past the minute's: %d, Retry-After %q", status, wait)
	}
	// Another device of the same user is the same user; another user is
	// not.
	if status, _ := upload(app("laptop", s.signIn("alice", "laptop"))); status != http.StatusTooManyRequests {
		t.Errorf("the user's other device: %d", status)
	}
	if status, _ := upload(bob); status != http.StatusOK {
		t.Errorf("another user: %d", status)
	}
	// A refused upload does not count: once the minute has passed, the
	// next is taken.
	advance(57 * time.Second)
	if status, _ := upload(alice); status != http.StatusOK {
		t.Fatalf("after the minute: %d", status)
	}
	// The hour's bound, a minute between uploads.
	for i := clientLogsAMinute + 1; i < clientLogsAnHour; i++ {
		advance(time.Minute)
		if status, _ := upload(alice); status != http.StatusOK {
			t.Fatalf("upload %d of the hour: %d", i, status)
		}
	}
	advance(time.Minute)
	// The hour's first upload was at 10:00:00, and it is 10:18:00.
	if status, wait := upload(alice); status != http.StatusTooManyRequests || wait != "2520" {
		t.Fatalf("an upload past the hour's: %d, Retry-After %q", status, wait)
	}
	advance(42 * time.Minute)
	if status, _ := upload(alice); status != http.StatusOK {
		t.Errorf("after the hour: %d", status)
	}
}

func TestAnAppsLongNameIsCutInItsLogsFileName(t *testing.T) {
	t.Parallel()
	s, stderr, _ := loggingServer(t)
	// The app's name, its version and the device's name are what the app
	// said when it signed in. A sign-in keeps none this long any more:
	// they are those of a device signed in before sign-ins were bounded.
	long := strings.Repeat("n", 100_000)
	status, body := s.call(http.MethodPost, "/Users/AuthenticateByName", `MediaBrowser Client="app", Device="tv", DeviceId="tv-id", Version="1"`,
		map[string]string{"Username": "alice", "Pw": "correct horse"})
	var signedIn AuthenticationResult
	if err := json.Unmarshal(body, &signedIn); status != http.StatusOK || err != nil || signedIn.AccessToken == "" {
		t.Fatalf("sign-in: %d %.200s", status, body)
	}
	if _, err := s.pool.Exec(t.Context(), "UPDATE devices SET client = $1, device_name = $1, client_version = $1 WHERE device_id = 'tv-id'", long); err != nil {
		t.Fatal(err)
	}
	before := len(stderr.String())
	status, body = s.postRaw("/ClientLog/Document", app("tv", signedIn.AccessToken), "app log")
	var answer ClientLogDocumentResponseDto
	if err := json.Unmarshal(body, &answer); status != http.StatusOK || err != nil {
		t.Fatalf("upload: %d %.200s", status, body)
	}
	if !strings.HasPrefix(answer.FileName, "upload_") || !strings.HasSuffix(answer.FileName, ".log") || len(answer.FileName) > 2*maxLogged+100 {
		t.Errorf("the file name is %d bytes: %.100s", len(answer.FileName), answer.FileName)
	}
	if grew := len(stderr.String()) - before; grew <= 0 || grew > 4*maxLogged+300 {
		t.Errorf("standard error grew by %d bytes", grew)
	}
}
