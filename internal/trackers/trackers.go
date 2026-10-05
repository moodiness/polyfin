// Package trackers sends what each Polyfin user watches to the tracking
// services they connect to their account: Trakt, Simkl and MDBList, which
// follow playback as it happens (scrobbling) and keep a watch history, and
// PublicMetaDB, which keeps resume points and a watch history. Only movies
// and episodes known by an IMDb, TMDB or TVDB identifier are sent, episodes
// by their series' identifiers and their numbers.
//
// Nothing is sent while a request is answered: playback reports and played
// marks are queued and sent in the background, one user and service at a
// time, at the pace each service allows. Changes to the history and resume
// points are kept in the database until the service accepts them or they
// are too old to matter; the start and pause of playback are sent once, as
// they mean nothing late.
package trackers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The services, by the names the admin API gives them.
const (
	Trakt        = "trakt"
	Simkl        = "simkl"
	MDBList      = "mdblist"
	PublicMetaDB = "publicmetadb"
)

// Services lists the services in the order the admin app shows them.
var Services = []string{Trakt, Simkl, MDBList, PublicMetaDB}

// publicURLs are the base URLs of the services' APIs.
var publicURLs = map[string]string{
	Trakt:        "https://api.trakt.tv",
	Simkl:        "https://api.simkl.com",
	MDBList:      "https://api.mdblist.com",
	PublicMetaDB: "https://publicmetadb.com",
}

// ByCode reports whether users connect service by entering a code on its
// site (Trakt, Simkl) rather than by pasting their API key.
func ByCode(service string) bool {
	return service == Trakt || service == Simkl
}

// scrobbles reports whether service follows playback as it happens.
func scrobbles(service string) bool {
	return service != PublicMetaDB
}

// known reports whether service is one of Services.
func known(service string) bool {
	_, ok := publicURLs[service]
	return ok
}

// Available reports whether users can connect service with settings: a
// code service needs the app an administrator registered with it.
func Available(service string, settings accounts.Settings) bool {
	switch service {
	case Trakt:
		return settings.TraktAvailable()
	case Simkl:
		return settings.SimklAvailable()
	}
	return true
}

// The problems a connection can have.
const (
	// ProblemReconnect: the service refused the token or key, and nothing
	// is sent until the user connects again.
	ProblemReconnect = "reconnect"
	// ProblemUnreachable: recent sends failed and are being retried.
	ProblemUnreachable = "unreachable"
)

var (
	// ErrUnknownService reports a service that is not one of Services, or
	// that is not connected the way asked.
	ErrUnknownService = errors.New("unknown tracking service")
	// ErrNotAvailable reports a code service whose app the settings lack.
	ErrNotAvailable = errors.New("tracking service not available")
	// ErrInvalidKey reports an API key the service refused, or a malformed
	// one.
	ErrInvalidKey = errors.New("invalid API key")
	// ErrUnreachable reports a service that could not be asked.
	ErrUnreachable = errors.New("tracking service unreachable")
)

// Options are the dependencies of the service.
type Options struct {
	DB *pgxpool.Pool
	// Settings returns the server's settings, which hold the apps
	// registered with Trakt and Simkl.
	Settings func() accounts.Settings
	// Version is Polyfin's, which the services are told.
	Version string
	Logger  *slog.Logger
	// URLs replaces the base URLs of the services' APIs, by service name.
	// Tests point them at fakes; the public APIs are used otherwise.
	URLs map[string]string
}

// timing is how long the service waits for what. Tests shorten it.
type timing struct {
	// gaps are the least time between two requests for one user to a
	// service: Trakt and Simkl take one write a second.
	gaps map[string]time.Duration
	// publicMetaDBGap spaces every request to PublicMetaDB, which limits
	// the server's address to 300 requests every 10 seconds.
	publicMetaDBGap time.Duration
	// retryFirst is the first wait after a failed send, doubling up to
	// retryMax; giveUp is how old a change gets before it is dropped.
	retryFirst, retryMax, giveUp time.Duration
	// unreachableAfter is how many sends in a row fail before the
	// connection shows the service as unreachable.
	unreachableAfter int
	// stale is how old a start or pause gets before it is not worth
	// sending.
	stale time.Duration
	// refreshAhead is how long before it expires a token is refreshed;
	// tokens are checked every refreshEvery.
	refreshAhead, refreshEvery time.Duration
	// watchedWindow is how near the time a service counted a title
	// watched a played mark of it counts as the same viewing.
	watchedWindow time.Duration
	// pollUnit is the unit of the intervals the services poll codes at,
	// a second.
	pollUnit time.Duration
	// request bounds each request.
	request time.Duration
}

var defaultTiming = timing{
	gaps:             map[string]time.Duration{Trakt: time.Second, Simkl: time.Second, MDBList: 250 * time.Millisecond},
	publicMetaDBGap:  40 * time.Millisecond,
	retryFirst:       30 * time.Second,
	retryMax:         30 * time.Minute,
	giveUp:           48 * time.Hour,
	unreachableAfter: 3,
	stale:            2 * time.Minute,
	refreshAhead:     24 * time.Hour,
	refreshEvery:     time.Hour,
	watchedWindow:    6 * time.Hour,
	pollUnit:         time.Second,
	request:          15 * time.Second,
}

// maxIntake bounds the reports and marks waiting to be looked at for one
// user: past it, new ones are dropped.
const maxIntake = 256

// Service connects users' accounts and sends what they watch.
type Service struct {
	db        *pgxpool.Pool
	settings  func() accounts.Settings
	version   string
	logger    *slog.Logger
	urls      map[string]string
	client    *http.Client
	now       func() time.Time
	timing    timing
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	refreshes refreshLocks

	mu     sync.Mutex
	closed bool
	// pending are the code connections waiting for users.
	pending map[laneKey]*pending
	// intakes are the reports and marks waiting to be looked at, by user.
	intakes map[accounts.ID]*intake
	// sessions are the playbacks under way, by user and device.
	sessions map[sessionKey]*session
	// lanes are what waits to be sent, by user and service.
	lanes map[laneKey]*lane
	seq   int64
	// pace spaces every request to PublicMetaDB.
	pace pace
}

// New returns a service keeping connections and queued changes in the
// database of options. Run sends what an earlier run left.
func New(options Options) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	urls := make(map[string]string, len(publicURLs))
	for service, base := range publicURLs {
		urls[service] = base
		if replaced, ok := options.URLs[service]; ok {
			urls[service] = replaced
		}
	}
	return &Service{
		db:       options.DB,
		settings: options.Settings,
		version:  options.Version,
		logger:   options.Logger,
		urls:     urls,
		client:   &http.Client{},
		now:      time.Now,
		timing:   defaultTiming,
		ctx:      ctx,
		cancel:   cancel,
		pending:  map[laneKey]*pending{},
		intakes:  map[accounts.ID]*intake{},
		sessions: map[sessionKey]*session{},
		lanes:    map[laneKey]*lane{},
	}
}

// Run sends the changes an earlier run left queued and keeps tokens
// fresh until ctx ends, then stops sending and waits for the sends under
// way.
func (s *Service) Run(ctx context.Context) {
	s.resumeQueued()
	ticker := time.NewTicker(s.timing.refreshEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.Close()
			return
		case <-ticker.C:
			s.refreshDue()
		}
	}
}

// Close stops sending and waits for the sends under way. What was not sent
// stays queued for the next run.
func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.cancel()
	s.wg.Wait()
}

// spawn runs f on a goroutine Close waits for, unless the service is
// closed. It is called with s.mu held.
func (s *Service) spawn(f func()) bool {
	if s.closed {
		return false
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		f()
	}()
	return true
}

// sleep waits for d, and reports false if the service stopped first.
func (s *Service) sleep(d time.Duration) bool {
	if d <= 0 {
		return s.ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-s.ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// Title identifies a movie or an episode to the services.
type Title struct {
	// Item is the movie's or episode's identifier in Polyfin.
	Item accounts.ID `json:"item"`
	// IMDb ("tt…"), TMDB and TVDB identify the movie, or the series of an
	// episode; any may be missing, but not all. Movies have no TVDB
	// identifier.
	IMDb string `json:"imdb,omitempty"`
	TMDB int    `json:"tmdb,omitempty"`
	TVDB int    `json:"tvdb,omitempty"`
	// Episode is set for an episode, numbered Number in season Season.
	Episode bool `json:"episode,omitempty"`
	Season  int  `json:"season,omitempty"`
	Number  int  `json:"number,omitempty"`
}

var imdbPattern = regexp.MustCompile(`^tt\d+$`)

// providerIDs reads the identifiers of providers, named as Jellyfin names
// them, and reports whether any is usable.
func (t *Title) providerIDs(providers map[string]string, tvdb bool) bool {
	if imdbPattern.MatchString(providers["Imdb"]) {
		t.IMDb = providers["Imdb"]
	}
	if id, err := strconv.Atoi(providers["Tmdb"]); err == nil && id > 0 {
		t.TMDB = id
	}
	if id, err := strconv.Atoi(providers["Tvdb"]); err == nil && id > 0 && tvdb {
		t.TVDB = id
	}
	return t.IMDb != "" || t.TMDB != 0 || t.TVDB != 0
}

// Movie identifies the movie item by its providers' identifiers (Imdb,
// Tmdb), and reports false when it has none.
func Movie(item accounts.ID, providers map[string]string) (Title, bool) {
	title := Title{Item: item}
	return title, title.providerIDs(providers, false)
}

// Episode identifies the episode item by its series' providers'
// identifiers (Imdb, Tmdb, Tvdb) and its numbers, and reports false when
// the series has none or the episode is not numbered.
func Episode(item accounts.ID, series map[string]string, season, number int) (Title, bool) {
	title := Title{Item: item, Episode: true, Season: season, Number: number}
	return title, title.providerIDs(series, true) && season >= 0 && number > 0
}
