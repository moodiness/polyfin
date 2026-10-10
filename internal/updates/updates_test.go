package updates

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/testdb"
)

// fakeGitHub answers the latest release of Polyfin's repository as a test
// sets it, and records the requests it receives.
type fakeGitHub struct {
	mu       sync.Mutex
	status   int
	body     string
	requests []*http.Request
	url      string
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	t.Helper()
	f := &fakeGitHub{status: http.StatusNotFound}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, r.Clone(context.Background()))
		status, body := f.status, f.body
		f.mu.Unlock()
		if r.URL.Path != "/repos/moodiness/polyfin/releases/latest" {
			status, body = http.StatusNotFound, `{"message":"Not Found"}`
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	f.url = server.URL
	return f
}

// release has the fake answer a release of tag, a pre-release when told.
func (f *fakeGitHub) release(tag string, prerelease bool) {
	pre := "false"
	if prerelease {
		pre = "true"
	}
	f.answer(http.StatusOK, `{"tag_name":"`+tag+`","html_url":"https://github.com/moodiness/polyfin/releases/tag/`+tag+
		`","draft":false,"prerelease":`+pre+`,"name":"Polyfin `+tag+`"}`)
}

// answer has the fake answer status and body.
func (f *fakeGitHub) answer(status int, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.body = status, body
}

// asked counts the requests the fake received.
func (f *fakeGitHub) asked() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// harness is a checker of a running version, on a database, asking the
// fake, with the setting a test turns on or off.
type harness struct {
	*Checker
	github   *fakeGitHub
	pool     *pgxpool.Pool
	enabled  *atomic.Bool
	mu       *sync.Mutex
	notified *[]Release
}

func newHarness(t *testing.T, version string) harness {
	t.Helper()
	pool := testdb.New(t)
	if err := database.Migrate(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	h := harness{github: newFakeGitHub(t), pool: pool, enabled: &atomic.Bool{}, mu: &sync.Mutex{}, notified: &[]Release{}}
	h.enabled.Store(true)
	h.Checker = h.restart(t, version)
	return h
}

// restart is a new checker on the harness's database, as after a restart
// of Polyfin running version.
func (h harness) restart(t *testing.T, version string) *Checker {
	t.Helper()
	return New(Options{DB: h.pool, Version: version, API: h.github.url, Enabled: h.enabled.Load,
		Notify: func(release Release) {
			h.mu.Lock()
			defer h.mu.Unlock()
			*h.notified = append(*h.notified, release)
		},
		Logger: slog.New(slog.DiscardHandler)})
}

// told lists the releases administrators were told about.
func (h harness) told() []Release {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]Release(nil), *h.notified...)
}

// check runs a check that must succeed.
func check(t *testing.T, c *Checker) {
	t.Helper()
	if err := c.Check(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// A newer release shows and is told, with its release notes; the same
// version, an older one or no release at all neither shows nor is told.
// The request names Polyfin and its version.
func TestNewerEqualAndOlderReleases(t *testing.T) {
	h := newHarness(t, "1.4.0")
	check(t, h.Checker)
	if _, ok := h.Available(); ok || len(h.told()) != 0 {
		t.Fatalf("without any release: %v", h.told())
	}
	for _, tag := range []string{"v1.4.0", "v1.3.9", "1.4.0"} {
		h.github.release(tag, false)
		check(t, h.Checker)
		if release, ok := h.Available(); ok || len(h.told()) != 0 {
			t.Errorf("%s while running 1.4.0: %v %v", tag, release, h.told())
		}
	}

	h.github.release("v1.5.0", false)
	check(t, h.Checker)
	want := Release{Version: "1.5.0", URL: "https://github.com/moodiness/polyfin/releases/tag/v1.5.0"}
	if release, ok := h.Available(); !ok || release != want {
		t.Errorf("1.5.0 while running 1.4.0: %v %v", release, ok)
	}
	if told := h.told(); len(told) != 1 || told[0] != want {
		t.Errorf("told: %v", told)
	}
	h.github.mu.Lock()
	request := h.github.requests[len(h.github.requests)-1]
	h.github.mu.Unlock()
	if request.Header.Get("User-Agent") != "Polyfin/1.4.0" || request.Header.Get("Accept") != "application/vnd.github+json" {
		t.Errorf("request headers: %v", request.Header)
	}

	// Asked again, the same release is not told twice.
	check(t, h.Checker)
	if told := h.told(); len(told) != 1 {
		t.Errorf("told again: %v", told)
	}
}

// A release GitHub marks as a pre-release, or whose tag is one, is left
// out: what was found before stays.
func TestPreReleasesAreIgnored(t *testing.T) {
	h := newHarness(t, "1.4.0")
	h.github.release("v1.5.0", false)
	check(t, h.Checker)
	for _, tc := range []struct {
		tag        string
		prerelease bool
	}{{"v1.6.0-rc.1", true}, {"v1.6.0-beta.2", false}, {"v1.6.0", true}} {
		h.github.release(tc.tag, tc.prerelease)
		check(t, h.Checker)
		if release, ok := h.Available(); !ok || release.Version != "1.5.0" {
			t.Errorf("after %s (pre-release: %v): %v", tc.tag, tc.prerelease, release)
		}
	}
	h.github.answer(http.StatusOK, `{"tag_name":"v1.7.0","html_url":"https://github.com/moodiness/polyfin/releases/tag/v1.7.0","draft":true}`)
	check(t, h.Checker)
	if release, _ := h.Available(); release.Version != "1.5.0" || len(h.told()) != 1 {
		t.Errorf("after a draft: %v, told %v", release, h.told())
	}
}

// A development build asks nothing and never tells a new version.
func TestDevelopmentBuildsAreNeverNotified(t *testing.T) {
	for _, version := range []string{"dev", ""} {
		h := newHarness(t, version)
		h.github.release("v99.0.0", false)
		check(t, h.Checker)
		if _, ok := h.Available(); ok || len(h.told()) != 0 || h.github.asked() != 0 {
			t.Errorf("version %q: told %v, asked %d times", version, h.told(), h.github.asked())
		}
	}
}

// A failed request, such as GitHub refusing too many requests with 403 or
// 429, or an answer that cannot be read, changes nothing: the release
// found before still shows, and nothing is told.
func TestFailedRequestsChangeNothing(t *testing.T) {
	h := newHarness(t, "1.4.0")
	h.github.release("v1.5.0", false)
	check(t, h.Checker)
	for _, failure := range []struct {
		status int
		body   string
	}{
		{http.StatusForbidden, `{"message":"API rate limit exceeded"}`},
		{http.StatusTooManyRequests, `{"message":"API rate limit exceeded"}`},
		{http.StatusInternalServerError, ``},
		{http.StatusOK, `{"tag_name":`},
		{http.StatusOK, `{"tag_name":"nightly","html_url":"https://github.com/moodiness/polyfin/releases/tag/nightly"}`},
	} {
		h.github.answer(failure.status, failure.body)
		if err := h.Check(t.Context()); err == nil {
			t.Errorf("%d %s: no error", failure.status, failure.body)
		}
		if release, ok := h.Available(); !ok || release.Version != "1.5.0" || len(h.told()) != 1 {
			t.Errorf("after %d %s: %v, told %v", failure.status, failure.body, release, h.told())
		}
	}
	var latest string
	if err := h.pool.QueryRow(t.Context(), "SELECT latest_version FROM update_check").Scan(&latest); err != nil || latest != "1.5.0" {
		t.Errorf("kept: %q %v", latest, err)
	}

	// GitHub unreachable fails too.
	unreachable := New(Options{DB: h.pool, Version: "1.4.0", API: "http://127.0.0.1:1", Enabled: h.enabled.Load,
		Logger: slog.New(slog.DiscardHandler)})
	if err := unreachable.Check(t.Context()); err == nil {
		t.Error("GitHub unreachable: no error")
	}
	if release, ok := unreachable.Available(); !ok || release.Version != "1.5.0" {
		t.Errorf("GitHub unreachable: %v", release)
	}
}

// A version is told once, even across a restart, which shows the release
// found before at once; a later version is told in turn, and none once
// Polyfin runs it.
func TestOneNotificationPerVersionAcrossRestarts(t *testing.T) {
	h := newHarness(t, "1.4.0")
	h.github.release("v1.5.0", false)
	check(t, h.Checker)

	restarted := h.restart(t, "1.4.0")
	restarted.load(t.Context())
	if release, ok := restarted.Available(); !ok || release.Version != "1.5.0" {
		t.Errorf("after a restart, before a check: %v", release)
	}
	check(t, restarted)
	if told := h.told(); len(told) != 1 {
		t.Errorf("told after a restart: %v", told)
	}

	h.github.release("v1.6.0", false)
	check(t, restarted)
	check(t, h.restart(t, "1.4.0"))
	if told := h.told(); len(told) != 2 || told[1].Version != "1.6.0" {
		t.Errorf("told: %v", told)
	}

	upgraded := h.restart(t, "1.6.0")
	check(t, upgraded)
	if release, ok := upgraded.Available(); ok || len(h.told()) != 2 {
		t.Errorf("running the latest version: %v, told %v", release, h.told())
	}
}

// With Check for new versions off, nothing is asked, nothing is told, and
// a release found before no longer shows; turned on again, it shows.
func TestSettingOffAsksNothing(t *testing.T) {
	h := newHarness(t, "1.4.0")
	h.github.release("v1.5.0", false)
	h.enabled.Store(false)
	check(t, h.Checker)
	if _, ok := h.Available(); ok || h.github.asked() != 0 || len(h.told()) != 0 {
		t.Fatalf("off: told %v, asked %d times", h.told(), h.github.asked())
	}

	h.enabled.Store(true)
	check(t, h.Checker)
	h.enabled.Store(false)
	if release, ok := h.Available(); ok {
		t.Errorf("found, then turned off: %v", release)
	}
	h.github.release("v1.6.0", false)
	check(t, h.Checker)
	if h.github.asked() != 1 || len(h.told()) != 1 {
		t.Errorf("off again: told %v, asked %d times", h.told(), h.github.asked())
	}
	h.enabled.Store(true)
	if release, ok := h.Available(); !ok || release.Version != "1.5.0" {
		t.Errorf("turned on again: %v", release)
	}
}

// Run checks shortly after it starts, and again once a day, while the
// setting is on; off, it asks nothing, and looks again sooner.
func TestRunChecksAfterStartAndDaily(t *testing.T) {
	h := newHarness(t, "1.4.0")
	h.github.release("v1.5.0", false)
	h.enabled.Store(false)
	h.timing = timing{first: 10 * time.Millisecond, every: time.Hour, retry: 20 * time.Millisecond}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.Run(ctx)
	}()
	time.Sleep(100 * time.Millisecond)
	if h.github.asked() != 0 {
		t.Errorf("off: asked %d times", h.github.asked())
	}
	h.enabled.Store(true)
	deadline := time.Now().Add(5 * time.Second)
	for len(h.told()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done
	if h.github.asked() != 1 || len(h.told()) != 1 {
		t.Errorf("on: told %v, asked %d times", h.told(), h.github.asked())
	}
}
