package jellyfin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// artClock is a clock the tests move.
type artClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *artClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *artClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// clockedImageHandler is imageHandler on clock.
func clockedImageHandler(t *testing.T, clock *artClock) *Handler {
	h := &Handler{Options: Options{Stremio: stremio.NewClient("test"), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}}
	h.images.now = clock.Now
	h.images.open(t.TempDir(), h.Logger)
	return h
}

// countingArt is an artwork server that answers each path as answers
// tells, "ok" by default, and counts the requests of each.
type countingArt struct {
	*httptest.Server
	mu      sync.Mutex
	answers map[string]func(http.ResponseWriter)
	asked   map[string]int
}

func newCountingArt(t *testing.T) *countingArt {
	a := &countingArt{answers: map[string]func(http.ResponseWriter){}, asked: map[string]int{}}
	a.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		a.asked[r.URL.Path]++
		answer := a.answers[r.URL.Path]
		a.mu.Unlock()
		if answer != nil {
			answer(w)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("jpeg bytes"))
	}))
	t.Cleanup(a.Close)
	return a
}

func (a *countingArt) answer(path string, answer func(http.ResponseWriter)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.answers[path] = answer
}

func (a *countingArt) requests(path string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.asked[path]
}

func status(code int, retryAfter string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		if retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		w.WriteHeader(code)
	}
}

// A host that asks to wait with a 429 is asked nothing until then, 15
// minutes at most: its other images answer 503 at once, telling how long.
func TestAnArtworkHostThatAsksToWaitIsAskedNothing(t *testing.T) {
	t.Parallel()
	clock := &artClock{now: time.Now()}
	h := clockedImageHandler(t, clock)
	art := newCountingArt(t)
	art.answer("/busy.jpg", status(http.StatusTooManyRequests, "30"))

	if _, err := h.fetchArtwork(t.Context(), art.URL+"/busy.jpg", false, false); err == nil {
		t.Fatal("a busy host answered")
	}
	_, err := h.fetchArtwork(t.Context(), art.URL+"/other.jpg", false, false)
	failed, ok := errors.AsType[downloadError](err)
	if wait, waits := stremio.RetryAfter(err); !ok || failed.status != http.StatusServiceUnavailable || !waits || wait != 30*time.Second {
		t.Errorf("another image of the waiting host: %v", err)
	}
	if got := art.requests("/other.jpg"); got != 0 {
		t.Errorf("the waiting host was asked %d times", got)
	}
	clock.advance(31 * time.Second)
	if _, err := h.fetchArtwork(t.Context(), art.URL+"/other.jpg", false, false); err != nil || art.requests("/other.jpg") != 1 {
		t.Errorf("once the wait ended: %v, %d requests", err, art.requests("/other.jpg"))
	}

	// A wait longer than 15 minutes is cut to 15 minutes.
	long := newCountingArt(t)
	long.answer("/busy.jpg", status(http.StatusServiceUnavailable, "7200"))
	_, _ = h.fetchArtwork(t.Context(), long.URL+"/busy.jpg", false, false)
	clock.advance(imageHostWaitMost - time.Second)
	if _, err := h.fetchArtwork(t.Context(), long.URL+"/other.jpg", false, false); err == nil || long.requests("/other.jpg") != 0 {
		t.Errorf("before 15 minutes: %v, %d requests", err, long.requests("/other.jpg"))
	}
	clock.advance(2 * time.Second)
	if _, err := h.fetchArtwork(t.Context(), long.URL+"/other.jpg", false, false); err != nil {
		t.Errorf("after 15 minutes: %v", err)
	}
}

// A failed download answers again without asking for 2 minutes, then 4, 8
// and so on as it keeps failing, an hour at most; an image its server does
// not have is asked again after an hour.
func TestArtworkFailuresBackOff(t *testing.T) {
	t.Parallel()
	clock := &artClock{now: time.Now()}
	h := clockedImageHandler(t, clock)
	art := newCountingArt(t)
	art.answer("/failing.jpg", status(http.StatusInternalServerError, ""))
	art.answer("/missing.jpg", status(http.StatusNotFound, ""))
	fetch := func(path string) {
		t.Helper()
		if _, err := h.fetchArtwork(t.Context(), art.URL+path, false, false); err == nil {
			t.Fatalf("%s answered", path)
		}
	}

	fetch("/failing.jpg")
	for i, life := range []time.Duration{2, 4, 8, 16, 32, 60, 60} {
		life *= time.Minute
		clock.advance(life - time.Second)
		fetch("/failing.jpg")
		if got := art.requests("/failing.jpg"); got != i+1 {
			t.Fatalf("within %v of failure %d: %d requests", life, i+1, got)
		}
		clock.advance(2 * time.Second)
		fetch("/failing.jpg")
		if got := art.requests("/failing.jpg"); got != i+2 {
			t.Fatalf("after %v of failure %d: %d requests", life, i+1, got)
		}
		clock.advance(-time.Second)
	}

	fetch("/missing.jpg")
	clock.advance(imageMissingLife - time.Second)
	fetch("/missing.jpg")
	if got := art.requests("/missing.jpg"); got != 1 {
		t.Errorf("a missing image within an hour: %d requests", got)
	}
	clock.advance(2 * time.Second)
	fetch("/missing.jpg")
	if got := art.requests("/missing.jpg"); got != 2 {
		t.Errorf("a missing image after an hour: %d requests", got)
	}
}

// Health counts the downloads that failed in the last hour by kind and by
// host, and tells the hosts that asked to wait; the summary line is logged
// while some failed.
func TestArtworkFailuresAreCounted(t *testing.T) {
	t.Parallel()
	clock := &artClock{now: time.Now()}
	h := clockedImageHandler(t, clock)
	logged := &summaryLog{}
	h.Logger = slog.New(logged)
	art := newCountingArt(t)
	art.answer("/missing.jpg", status(http.StatusNotFound, ""))
	art.answer("/failing.jpg", status(http.StatusBadGateway, ""))
	busy := newCountingArt(t)
	busy.answer("/busy.jpg", status(http.StatusTooManyRequests, "60"))
	for _, target := range []string{art.URL + "/missing.jpg", art.URL + "/failing.jpg", busy.URL + "/busy.jpg", busy.URL + "/other.jpg"} {
		_, _ = h.fetchArtwork(t.Context(), target, false, false)
	}
	host := func(server *countingArt) string {
		parsed, _ := url.Parse(server.URL)
		return parsed.Host
	}
	got := h.ArtworkStatus()
	if got.Failed != 3 || got.Kinds[ArtworkNotFound] != 1 || got.Kinds[ArtworkServerError] != 1 || got.Kinds[ArtworkRateLimited] != 1 {
		t.Errorf("counted: %+v", got)
	}
	if len(got.Hosts) != 2 || got.Hosts[0] != (ArtworkHostFailures{Host: host(art), Failed: 2}) || got.Hosts[1] != (ArtworkHostFailures{Host: host(busy), Failed: 1}) {
		t.Errorf("hosts: %+v", got.Hosts)
	}
	if len(got.Waiting) != 1 || got.Waiting[0].Host != host(busy) || !got.Waiting[0].Until.Equal(clock.Now().Add(time.Minute)) {
		t.Errorf("waiting: %+v", got.Waiting)
	}
	h.logArtworkSummary()
	if len(logged.lines) != 1 || !strings.Contains(logged.lines[0], "failed=3") || !strings.Contains(logged.lines[0], host(art)+" (2)") {
		t.Errorf("summary: %q", logged.lines)
	}

	clock.advance(imageFailureWindow + time.Second)
	if got := h.ArtworkStatus(); got.Failed != 0 || len(got.Hosts) != 0 || len(got.Waiting) != 0 {
		t.Errorf("an hour later: %+v", got)
	}
	h.logArtworkSummary()
	if len(logged.lines) != 1 {
		t.Errorf("a summary without failures: %q", logged.lines)
	}
}

// summaryLog keeps the INFO lines a logger handles, with their attributes.
type summaryLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *summaryLog) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelInfo
}

func (l *summaryLog) Handle(_ context.Context, record slog.Record) error {
	line := record.Message
	record.Attrs(func(attr slog.Attr) bool {
		line += " " + attr.String()
		return true
	})
	l.mu.Lock()
	l.lines = append(l.lines, line)
	l.mu.Unlock()
	return nil
}

func (l *summaryLog) WithAttrs([]slog.Attr) slog.Handler { return l }

func (l *summaryLog) WithGroup(string) slog.Handler { return l }

// gonePhotoAddon serves a movie whose cast has a photo its server does not
// have, a backdrop it does not have either, and a poster it has.
func gonePhotoAddon(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	movie := func() stremio.Meta {
		return stremio.Meta{ID: "tt0900", Type: "movie", Name: "Harbor", Poster: server.URL + "/poster.jpg",
			Background: server.URL + "/backdrop.jpg", ReleaseInfo: "2020",
			Extras: &stremio.Extras{Cast: []stremio.CastMember{{Name: "Gone Actor", Character: "Hero", Photo: server.URL + "/gone.jpg"}}}}
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "gone", Name: "Gone", Version: "1", Types: []string{"movie"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			preview := movie()
			preview.Extras = nil
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{preview}})
		case path == "/meta/movie/tt0900.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie()})
		case path == "/poster.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpeg bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// Artwork found dead stops being advertised, so that apps draw their
// placeholder rather than an empty card: a title's backdrop its server
// does not have, and a person's photo, the person showing none without a
// TMDB key. Refresh metadata brings them back, to be tried again.
func TestDeadArtworkIsNotAdvertised(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	addon, err := s.addons.Install(t.Context(), addons.Shared(), gonePhotoAddon(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{{AddonID: addon.ID, CatalogType: "movie", CatalogID: "top"}}); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("admin", "tv")
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	var page QueryResult
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	movieID := page.Items[0].Id
	details := func() (BaseItemDto, BaseItemPerson) {
		t.Helper()
		var movie BaseItemDto
		s.get(t, "/Items/"+movieID, token, &movie)
		if movie.People == nil || len(*movie.People) != 1 {
			t.Fatalf("people: %+v", movie.People)
		}
		return movie, (*movie.People)[0]
	}
	image := func(path string) int {
		t.Helper()
		response, err := http.Get(s.url + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		return response.StatusCode
	}

	movie, person := details()
	if len(movie.BackdropImageTags) != 1 || person.PrimaryImageTag == "" {
		t.Fatalf("before any download: backdrop %v, photo %q", movie.BackdropImageTags, person.PrimaryImageTag)
	}
	if got := image("/Items/" + movieID + "/Images/Backdrop/0?tag=" + movie.BackdropImageTags[0]); got != http.StatusNotFound {
		t.Errorf("backdrop: %d", got)
	}
	if got := image("/Items/" + person.Id + "/Images/Primary?tag=" + person.PrimaryImageTag); got != http.StatusNotFound {
		t.Errorf("photo: %d", got)
	}
	movie, person = details()
	if len(movie.BackdropImageTags) != 0 || movie.ImageTags["Primary"] == "" || person.PrimaryImageTag != "" {
		t.Errorf("once dead: backdrop %v, poster %q, photo %q", movie.BackdropImageTags, movie.ImageTags["Primary"], person.PrimaryImageTag)
	}
	var page2 BaseItemDto
	s.get(t, "/Items/"+person.Id, token, &page2)
	if page2.ImageTags["Primary"] != "" {
		t.Errorf("the person's page shows the dead photo: %v", page2.ImageTags)
	}

	if status, body := s.call(http.MethodPost, "/Items/"+movieID+"/Refresh?metadataRefreshMode=FullRefresh", app("tv", token), nil); status != http.StatusNoContent {
		t.Fatalf("refresh: %d %s", status, body)
	}
	movie, person = details()
	if len(movie.BackdropImageTags) != 1 || person.PrimaryImageTag == "" {
		t.Errorf("after Refresh metadata: backdrop %v, photo %q", movie.BackdropImageTags, person.PrimaryImageTag)
	}
}
