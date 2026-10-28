package localfiles

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/stremio"
)

// searchAddon is a metadata addon named name, whose movie search catalog
// declares extra and finds metaTitles by name, as the fake metadata addon
// does, unless answer answers a catalog request itself: it is given the
// request's number, from 1, and its extra properties, and reports whether
// it answered.
type searchAddon struct {
	name   string
	extra  []stremio.Extra
	answer func(w http.ResponseWriter, n int, extra map[string]string) bool

	mu       sync.Mutex
	requests []map[string]string
	times    []time.Time
}

// start serves the addon, and returns its manifest's address.
func (a *searchAddon) start(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "meta." + strings.ToLower(a.name), Name: a.name, Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top", Extra: a.extra}}})
		case strings.HasPrefix(path, "/catalog/movie/top/"):
			extra := map[string]string{}
			for _, pair := range strings.Split(strings.TrimSuffix(strings.TrimPrefix(path, "/catalog/movie/top/"), ".json"), "&") {
				name, value, _ := strings.Cut(pair, "=")
				extra[name], _ = url.PathUnescape(value)
			}
			a.mu.Lock()
			a.requests = append(a.requests, extra)
			a.times = append(a.times, time.Now())
			n := len(a.requests)
			a.mu.Unlock()
			if a.answer != nil && a.answer(w, n, extra) {
				return
			}
			metas := []stremio.Meta{}
			for _, meta := range metaTitles {
				if meta.Type == "movie" && extra["search"] != "" && strings.Contains(strings.ToLower(meta.Name), strings.ToLower(extra["search"])) {
					metas = append(metas, meta)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		case strings.HasPrefix(path, "/meta/"):
			id := strings.TrimSuffix(path[strings.LastIndex(path, "/")+1:], ".json")
			for _, meta := range metaTitles {
				if meta.ID == id {
					_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
					return
				}
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// asked returns the extra properties of the catalog requests made.
func (a *searchAddon) asked() []map[string]string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]map[string]string(nil), a.requests...)
}

// failing answers every catalog request with status.
func failing(status int) func(http.ResponseWriter, int, map[string]string) bool {
	return func(w http.ResponseWriter, _ int, _ map[string]string) bool {
		w.WriteHeader(status)
		return true
	}
}

// A scan searches a file's title, as its name gives it and without its
// year, on the first page of the search catalog only, as Stremio apps
// search: an addon that fails the pages past the first fails nothing.
func TestScanSearchesTheFirstPageOnly(t *testing.T) {
	addon := &searchAddon{name: "Meta", extra: []stremio.Extra{{Name: "search"}, {Name: "skip"}},
		answer: func(w http.ResponseWriter, _ int, extra map[string]string) bool {
			if extra["skip"] != "" {
				w.WriteHeader(http.StatusInternalServerError)
				return true
			}
			return false
		}}
	e := newEnvWith(t, addon.start(t))
	folder, _ := e.folder(KindMovies, map[string]int{"The General (1926).mkv": 1, "Nosferatu.1922.1080p.mkv": 1})
	files := e.files(folder.ID)
	if files["The General (1926).mkv"] != "tt0017925" || files["Nosferatu.1922.1080p.mkv"] != "tt0013442" {
		t.Errorf("files: %v", files)
	}
	asked := addon.asked()
	terms := map[string]bool{}
	for _, extra := range asked {
		terms[extra["search"]] = true
		if extra["skip"] != "" {
			t.Errorf("a page past the first was asked for: %v", extra)
		}
	}
	if len(terms) != 2 || !terms["The General"] || !terms["Nosferatu"] {
		t.Errorf("searched: %v", asked)
	}
}

// A scan whose searches an addon answers "too many requests" waits as long
// as it asks, or a moment when it does not say, and searches again: every
// file is matched in that scan.
func TestScanWaitsOutRateLimits(t *testing.T) {
	for _, after := range []string{"1", ""} {
		t.Run("Retry-After "+after, func(t *testing.T) {
			addon := &searchAddon{name: "Meta", extra: []stremio.Extra{{Name: "search", IsRequired: true}},
				answer: func(w http.ResponseWriter, n int, _ map[string]string) bool {
					if n > 2 {
						return false
					}
					if after != "" {
						w.Header().Set("Retry-After", after)
					}
					w.WriteHeader(http.StatusTooManyRequests)
					return true
				}}
			e := newEnvWith(t, addon.start(t))
			folder, _ := e.folder(KindMovies, map[string]int{"The General (1926).mkv": 1, "Nosferatu (1922).mkv": 1,
				"The Gold Rush (1925).mkv": 1})
			files := e.files(folder.ID)
			if files["The General (1926).mkv"] != "tt0017925" || files["Nosferatu (1922).mkv"] != "tt0013442" ||
				files["The Gold Rush (1925).mkv"] != "tt0015864" {
				t.Errorf("files: %v", files)
			}
			addon.mu.Lock()
			defer addon.mu.Unlock()
			if len(addon.times) != 5 {
				t.Fatalf("%d requests", len(addon.times))
			}
			// The searches asked again come once the addon's wait is over.
			if after == "1" {
				for _, at := range addon.times[3:] {
					if waited := at.Sub(addon.times[0]); waited < time.Second {
						t.Errorf("asked again after %v", waited)
					}
				}
			}
		})
	}
}

// One addon whose search fails fails no other: a title the other finds is
// matched, and one it does not find is left unmatched as a failed search,
// which the failing addon might have found, not as a title not found.
func TestOneFailingSearchAddonFailsNoOther(t *testing.T) {
	broken := &searchAddon{name: "Broken", extra: []stremio.Extra{{Name: "search", IsRequired: true}}, answer: failing(http.StatusInternalServerError)}
	meta := &searchAddon{name: "Meta", extra: []stremio.Extra{{Name: "search", IsRequired: true}}}
	e := newEnvWith(t, broken.start(t), meta.start(t))
	folder, _ := e.folder(KindMovies, map[string]int{"The General (1926).mkv": 1, "Lost Reel (1931).mkv": 1})
	files := e.files(folder.ID)
	if files["The General (1926).mkv"] != "tt0017925" || files["Lost Reel (1931).mkv"] != "unmatched: search_failed" {
		t.Errorf("files: %v", files)
	}
}

// warnings records the warnings a logger writes.
type warnings struct {
	mu   sync.Mutex
	logs bytes.Buffer
}

func (w *warnings) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.logs.Write(p)
}

func (w *warnings) lines() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.FieldsFunc(w.logs.String(), func(r rune) bool { return r == '\n' })
}

// A failed search tells, on each file it leaves unmatched, the addon that
// failed and why, and the log tells it once per scan, as a warning.
func TestSearchFailureTold(t *testing.T) {
	addon := &searchAddon{name: "Broken", extra: []stremio.Extra{{Name: "search", IsRequired: true}}, answer: failing(http.StatusServiceUnavailable)}
	e := newEnvWith(t, addon.start(t))
	logs := &warnings{}
	e.service.logger = slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelWarn}))
	folder, _ := e.folder(KindMovies, map[string]int{"The General (1926).mkv": 1, "Nosferatu (1922).mkv": 1, "Lost Reel (1931).mkv": 1})
	lines := logs.lines()
	if len(lines) != 1 || !strings.Contains(lines[0], "level=WARN") || !strings.Contains(lines[0], `addon=Broken`) ||
		!strings.Contains(lines[0], "files=3") || !strings.Contains(lines[0], "HTTP 503") {
		t.Errorf("logs: %q", lines)
	}
	rows, err := e.pool.Query(t.Context(), "SELECT path, reason, search_addon, search_failure FROM local_files WHERE addon_id = $1", folder.ID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for rows.Next() {
		var path, reason, failedAddon, failure string
		if err := rows.Scan(&path, &reason, &failedAddon, &failure); err != nil {
			t.Fatal(err)
		}
		count++
		if reason != "search_failed" || failedAddon != "Broken" || failure != "server_error" {
			t.Errorf("%s: %q %q %q", path, reason, failedAddon, failure)
		}
	}
	if err := rows.Err(); err != nil || count != 3 {
		t.Fatalf("%d files: %v", count, err)
	}
}

// A unit whose search failed is searched again soon, by the scans due,
// later and later while it fails, and is matched once the addon answers,
// without anyone asking.
func TestFailedSearchesAreTriedAgain(t *testing.T) {
	var down sync.Mutex
	failingNow := true
	addon := &searchAddon{name: "Meta", extra: []stremio.Extra{{Name: "search", IsRequired: true}},
		answer: func(w http.ResponseWriter, _ int, _ map[string]string) bool {
			down.Lock()
			defer down.Unlock()
			if failingNow {
				w.WriteHeader(http.StatusBadGateway)
			}
			return failingNow
		}}
	e := newEnvWith(t, addon.start(t))
	now := time.Now().Truncate(time.Microsecond)
	var clock sync.Mutex
	e.service.now = func() time.Time {
		clock.Lock()
		defer clock.Unlock()
		return now
	}
	advance := func(d time.Duration) {
		clock.Lock()
		defer clock.Unlock()
		now = now.Add(d)
	}
	folder, _ := e.folder(KindMovies, map[string]int{"The General (1926).mkv": 1})
	retry := func() (at *time.Time, retries int) {
		t.Helper()
		if err := e.pool.QueryRow(t.Context(), "SELECT search_retry_at, search_retries FROM local_folders WHERE addon_id = $1",
			folder.ID).Scan(&at, &retries); err != nil {
			t.Fatal(err)
		}
		return at, retries
	}
	scanDue := func() {
		t.Helper()
		if err := e.service.ScanDue(t.Context(), false); err != nil {
			t.Fatal(err)
		}
	}
	searched := func() int { return len(addon.asked()) }

	// Not due yet: nothing is searched.
	before := searched()
	scanDue()
	if searched() != before {
		t.Error("searched again before the retry was due")
	}
	// Due: searched again, failing again, then tried later. Each scan
	// tries a failed search twice more (see serverWaits).
	advance(searchRetry(0))
	scanDue()
	if want := before + 1 + len(serverWaits); searched() != want {
		t.Errorf("%d searches, want %d", searched(), want)
	}
	at, retries := retry()
	if at == nil || retries != 2 || at.Sub(now) != searchRetry(1) || searchRetry(1) <= searchRetry(0) {
		t.Fatalf("after the second failure: %v %d", at, retries)
	}
	down.Lock()
	failingNow = false
	down.Unlock()
	advance(searchRetry(1))
	scanDue()
	if got := e.files(folder.ID)["The General (1926).mkv"]; got != "tt0017925" {
		t.Errorf("after the addon answered again: %s", got)
	}
	if at, retries = retry(); at != nil || retries != 0 {
		t.Errorf("once matched: %v %d", at, retries)
	}
}
