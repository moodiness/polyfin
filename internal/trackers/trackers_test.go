package trackers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/testdb"
	"github.com/moodiness/polyfin/internal/userdata"
)

// request is a request one of the fake services received.
type request struct {
	service, method, path string
	query                 url.Values
	header                http.Header
	// body is the JSON body decoded, or the form's values.
	body any
	form url.Values
	// at is when it came.
	at time.Time
}

// answer is what a fake service answers.
type answer struct {
	status int
	body   string
	header http.Header
}

// fakes stands in for every service, each under a path named after it:
// /trakt, /simkl, /mdblist, /publicmetadb. Unless told otherwise, it
// accepts every request with an empty object.
type fakes struct {
	mu       sync.Mutex
	requests []request
	handlers map[string]func(request) answer
	url      string
}

func newFakes(t *testing.T) *fakes {
	t.Helper()
	f := &fakes{handlers: map[string]func(request) answer{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		service, path, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
		raw, _ := io.ReadAll(r.Body)
		req := request{service: service, method: r.Method, path: "/" + path, query: r.URL.Query(), header: r.Header.Clone(), at: time.Now()}
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
			req.form, _ = url.ParseQuery(string(raw))
		} else if len(raw) > 0 {
			_ = json.Unmarshal(raw, &req.body)
		}
		f.mu.Lock()
		f.requests = append(f.requests, req)
		handler := f.handlers[r.Method+" /"+service+"/"+path]
		f.mu.Unlock()
		reply := answer{status: http.StatusCreated, body: "{}"}
		if r.Method == http.MethodGet {
			reply.status = http.StatusOK
		}
		if handler != nil {
			reply = handler(req)
		}
		for name, values := range reply.header {
			w.Header()[name] = values
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(reply.status)
		_, _ = io.WriteString(w, reply.body)
	}))
	t.Cleanup(server.Close)
	f.url = server.URL
	return f
}

// on sets how the fake answers method and path, which starts with the
// service's name.
func (f *fakes) on(method, path string, handler func(request) answer) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers[method+" "+path] = handler
}

// reply answers method and path with status and body, always.
func (f *fakes) reply(method, path string, status int, body string) {
	f.on(method, path, func(request) answer { return answer{status: status, body: body} })
}

// sent lists the requests of service to path so far.
func (f *fakes) sent(service, path string) []request {
	f.mu.Lock()
	defer f.mu.Unlock()
	var found []request
	for _, r := range f.requests {
		if r.service == service && r.path == path {
			found = append(found, r)
		}
	}
	return found
}

// wait waits until service received n requests to path, and returns them.
func (f *fakes) wait(t *testing.T, n int, service, path string) []request {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		found := f.sent(service, path)
		if len(found) >= n {
			return found
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s %s: %d requests, want %d", service, path, len(found), n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// eventually waits until check holds.
func eventually(t *testing.T, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !check() {
		if time.Now().After(deadline) {
			t.Fatalf("%s never happened", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// idle waits until nothing waits to be looked at or sent.
func (s *Service) idle(t *testing.T) {
	t.Helper()
	eventually(t, "the queues emptying", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return len(s.intakes) == 0 && len(s.lanes) == 0
	})
}

// lockedBuffer collects the log.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// clockStart is the time the tests' services run at.
var clockStart = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

type harness struct {
	*Service
	store *accounts.Store
	f     *fakes
	log   *lockedBuffer
}

// newHarness is a service on a fresh database, sending to fakes with
// Trakt's and Simkl's apps set, at a fixed time, without waiting between
// requests.
func newHarness(t *testing.T) harness {
	t.Helper()
	pool := testdb.New(t)
	if err := database.Migrate(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	store, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	settings := store.Settings()
	settings.TraktClientID, settings.TraktClientSecret, settings.SimklClientID = "trakt-client", "trakt-secret", "simkl-client"
	if _, err := store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	f := newFakes(t)
	log := &lockedBuffer{}
	s := newService(pool, store, f, log)
	t.Cleanup(s.Close)
	return harness{Service: s, store: store, f: f, log: log}
}

// newService is a service on pool sending to f, logging to log, at a fixed
// time, without waiting between requests and retrying soon.
func newService(pool *pgxpool.Pool, store *accounts.Store, f *fakes, log io.Writer) *Service {
	logger := slog.New(slog.NewTextHandler(log, nil))
	client := stremio.NewClient("test")
	s := New(Options{DB: pool, Settings: store.Settings, Version: "1.2.3", Logger: logger,
		URLs:     map[string]string{Trakt: f.url + "/trakt", Simkl: f.url + "/simkl", MDBList: f.url + "/mdblist", PublicMetaDB: f.url + "/publicmetadb"},
		Titles:   library.New(pool, addons.New(pool, client), client, logger, store.Settings),
		UserData: userdata.New(pool)})
	s.now = func() time.Time { return clockStart }
	s.timing.gaps = map[string]time.Duration{}
	s.timing.publicMetaDBGap = 0
	s.timing.retryFirst, s.timing.retryMax = 10*time.Millisecond, 40*time.Millisecond
	s.timing.pollUnit = 5 * time.Millisecond
	s.timing.importGaps = map[string]time.Duration{}
	s.timing.importMaxWait = 2 * time.Second
	return s
}

func (h harness) user(t *testing.T, name string) accounts.ID {
	t.Helper()
	user, err := h.store.CreateUser(t.Context(), accounts.NewUser{Name: name, Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	return user.ID
}

// connected connects user to service with token, as after a code was
// entered or a key checked.
func (h harness) connected(t *testing.T, user accounts.ID, service, token string) {
	t.Helper()
	c := connection{token: token, connectedAt: clockStart}
	if ByCode(service) {
		expires := clockStart.Add(7 * 24 * time.Hour)
		c.refresh, c.expires = token+"-refresh", &expires
	}
	if err := h.connect(t.Context(), user, service, c); err != nil {
		t.Fatal(err)
	}
}

func (h harness) status(t *testing.T, user accounts.ID, service string) Status {
	t.Helper()
	status, err := h.Service.status(t.Context(), user, service)
	if err != nil {
		t.Fatal(err)
	}
	return status
}

// asJSON decodes body as a JSON value, to compare with what a fake got.
func asJSON(t *testing.T, body string) any {
	t.Helper()
	var value any
	if err := json.Unmarshal([]byte(body), &value); err != nil {
		t.Fatalf("%s: %v", body, err)
	}
	return value
}

func sameJSON(t *testing.T, what string, got any, want string) {
	t.Helper()
	if !reflect.DeepEqual(got, asJSON(t, want)) {
		encoded, _ := json.Marshal(got)
		t.Errorf("%s:\n got %s\nwant %s", what, encoded, want)
	}
}

var (
	movie   = Title{Item: accounts.ID{1}, IMDb: "tt0000001", TMDB: 10}
	episode = Title{Item: accounts.ID{2}, IMDb: "tt0100", TMDB: 30, TVDB: 20, Episode: true, Season: 1, Number: 2}
	pilot   = Title{Item: accounts.ID{3}, IMDb: "tt0100", TMDB: 30, TVDB: 20, Episode: true, Season: 1, Number: 1}
	finale  = Title{Item: accounts.ID{4}, IMDb: "tt0100", TMDB: 30, TVDB: 20, Episode: true, Season: 2, Number: 1}
)

func titled(title Title) func(context.Context) (Title, bool) {
	return func(context.Context) (Title, bool) { return title, true }
}

func titles(list ...Title) func(context.Context) []Title {
	return func(context.Context) []Title { return list }
}

var device = accounts.ID{9}

// play reports a playback of title at position minutes of a 100-minute
// title.
func play(event Event, title Title, minutes float64, paused, played bool) Playback {
	return Playback{Event: event, Device: device, Item: title.Item, Position: time.Duration(minutes * float64(time.Minute)),
		PositionKnown: true, Runtime: 100 * time.Minute, Paused: paused, Played: played, Title: titled(title)}
}

func TestTitlesNeedAnIdentifier(t *testing.T) {
	if _, ok := Movie(accounts.ID{1}, map[string]string{}); ok {
		t.Error("a movie without identifiers is tracked")
	}
	if _, ok := Movie(accounts.ID{1}, map[string]string{"Imdb": "not-imdb", "Tmdb": "x"}); ok {
		t.Error("a movie with malformed identifiers is tracked")
	}
	if title, ok := Movie(accounts.ID{1}, map[string]string{"Imdb": "tt0000001", "Tmdb": "10", "Tvdb": "5"}); !ok ||
		title != (Title{Item: accounts.ID{1}, IMDb: "tt0000001", TMDB: 10}) {
		t.Errorf("movie: %+v %v", title, ok)
	}
	if _, ok := Episode(accounts.ID{2}, nil, 1, 2); ok {
		t.Error("an episode of a series without identifiers is tracked")
	}
	if _, ok := Episode(accounts.ID{2}, map[string]string{"Tvdb": "20"}, 1, 0); ok {
		t.Error("an unnumbered episode is tracked")
	}
	if title, ok := Episode(accounts.ID{2}, map[string]string{"Tvdb": "20"}, 0, 3); !ok || title.TVDB != 20 || title.Season != 0 || title.Number != 3 {
		t.Errorf("special: %+v %v", title, ok)
	}
}
