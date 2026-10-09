// Package trackers sends what each Polyfin user watches and listens to to
// the tracking services they connect to their account: Trakt, Simkl and
// MDBList, which follow playback as it happens (scrobbling) and keep a
// watch history, and PublicMetaDB, which keeps resume points and a watch
// history. Only movies and episodes known by an IMDb, TMDB or TVDB
// identifier are sent to them, episodes by their series' identifiers and
// their numbers; anime that addons name by their Kitsu, MyAnimeList or
// AniDB identifier are sent by those the anime mapping gives (see
// anime.go). The songs of music addons go to the music services only,
// Last.fm and ListenBrainz (see music.go), by their artist and title.
//
// Nothing is sent while a request is answered: playback reports and played
// marks are queued and sent in the background, one user and service at a
// time, at the pace each service allows. Changes to the history and resume
// points, and scrobbled songs, are kept in the database until the service
// accepts them or they are too old to matter; the start and pause of
// playback, and the song playing now, are sent once, as they mean nothing
// late.
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
	"github.com/moodiness/polyfin/internal/anime"
	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/userdata"
)

// The services, by the names the admin API gives them.
const (
	Trakt        = "trakt"
	Simkl        = "simkl"
	MDBList      = "mdblist"
	PublicMetaDB = "publicmetadb"
	LastFM       = "lastfm"
	ListenBrainz = "listenbrainz"
)

// Services lists the services in the order the admin app shows them.
var Services = []string{Trakt, Simkl, MDBList, PublicMetaDB, LastFM, ListenBrainz}

// publicURLs are the base URLs of the services' APIs.
var publicURLs = map[string]string{
	Trakt:        "https://api.trakt.tv",
	Simkl:        "https://api.simkl.com",
	MDBList:      "https://api.mdblist.com",
	PublicMetaDB: "https://publicmetadb.com",
	LastFM:       "https://ws.audioscrobbler.com/2.0/",
	ListenBrainz: "https://api.listenbrainz.org",
}

// ByCode reports whether users connect service by entering a code on its
// site (Trakt, Simkl) rather than by pasting their API key.
func ByCode(service string) bool {
	return service == Trakt || service == Simkl
}

// BySignIn reports whether users connect service by signing in on its site
// and allowing Polyfin there (Last.fm).
func BySignIn(service string) bool {
	return service == LastFM
}

// ByKey reports whether users connect service by pasting their API key or
// user token (MDBList, PublicMetaDB, ListenBrainz), which they may read
// again.
func ByKey(service string) bool {
	return known(service) && !ByCode(service) && !BySignIn(service)
}

// Music reports whether service is told the songs users play, rather than
// the movies and episodes they watch.
func Music(service string) bool {
	return service == LastFM || service == ListenBrainz
}

// scrobbles reports whether service follows the playback of movies and
// episodes as it happens.
func scrobbles(service string) bool {
	return service != PublicMetaDB && !Music(service)
}

// imports reports whether Polyfin reads the watch history of service.
func imports(service string) bool {
	return known(service) && !Music(service)
}

// known reports whether service is one of Services.
func known(service string) bool {
	_, ok := publicURLs[service]
	return ok
}

// Available reports whether users can connect service with settings: a
// code or sign-in service needs the app an administrator registered with
// it.
func Available(service string, settings accounts.Settings) bool {
	switch service {
	case Trakt:
		return settings.TraktAvailable()
	case Simkl:
		return settings.SimklAvailable()
	case LastFM:
		return settings.LastFMAvailable()
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
	// ProblemAppRefused: the service refused the app an administrator
	// registered with it while the user was connecting, until the user
	// tries again or the app's settings change.
	ProblemAppRefused = "app_refused"
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
	// ErrAppRefused reports a code service that refused the app the
	// settings name: a wrong client ID or secret.
	ErrAppRefused = errors.New("tracking service refused the server's app")
)

// Options are the dependencies of the service.
type Options struct {
	DB *pgxpool.Pool
	// Settings returns the server's settings, which hold the apps
	// registered with Trakt and Simkl.
	Settings func() accounts.Settings
	// Secrets seals the tokens and API keys of the connections in the
	// database, and opens them; nil stores them as they are.
	Secrets *secrets.Box
	// Version is Polyfin's, which the services are told.
	Version string
	Logger  *slog.Logger
	// URLs replaces the base URLs of the services' APIs, by service name.
	// Tests point them at fakes; the public APIs are used otherwise.
	URLs map[string]string
	// Titles finds the items of the titles an imported watch history
	// names, and UserData keeps what imports add to the users' data;
	// without both, nothing is imported.
	Titles   Titles
	UserData *userdata.Store
	// Anime maps anime between identifiers and numberings: Simkl's anime
	// history, and the titles anime addons name, are found and sent by it.
	// Nil leaves them out.
	Anime *anime.Service
}

// timing is how long the service waits for what. Tests shorten it.
type timing struct {
	// gaps are the least time between two requests for one user to a
	// service: Trakt and Simkl take one write a second.
	gaps map[string]time.Duration
	// sharedGaps space every request to a service whose limit counts the
	// server's address rather than each user: PublicMetaDB takes 300
	// requests every 10 seconds, Last.fm about 5 a second.
	sharedGaps map[string]time.Duration
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
	// signInTime is how long Polyfin waits for a user to allow it on
	// Last.fm, asking every signInPoll pollUnits.
	signInTime time.Duration
	signInPoll int
	// request bounds each request.
	request time.Duration
	// importEvery is how long after an import of a watch history the
	// next one runs, imports being looked for every importCheck.
	importEvery, importCheck time.Duration
	// importGaps are the least time between two requests of an import to
	// a service, which reads at most for importMaxWait when the service
	// asks it to wait; it tries a failed request importRetries times more.
	importGaps    map[string]time.Duration
	importMaxWait time.Duration
	importRetries int
}

var defaultTiming = timing{
	gaps:             map[string]time.Duration{Trakt: time.Second, Simkl: time.Second, MDBList: 250 * time.Millisecond, ListenBrainz: 250 * time.Millisecond},
	sharedGaps:       map[string]time.Duration{PublicMetaDB: 40 * time.Millisecond, LastFM: 250 * time.Millisecond},
	retryFirst:       30 * time.Second,
	retryMax:         30 * time.Minute,
	giveUp:           48 * time.Hour,
	unreachableAfter: 3,
	stale:            2 * time.Minute,
	refreshAhead:     24 * time.Hour,
	refreshEvery:     time.Hour,
	watchedWindow:    6 * time.Hour,
	pollUnit:         time.Second,
	signInTime:       15 * time.Minute,
	signInPoll:       5,
	request:          15 * time.Second,
	importEvery:      6 * time.Hour,
	importCheck:      10 * time.Minute,
	// Trakt reads 1,000 pages (500 per its latest docs) every 5 minutes,
	// Simkl 10 a second, MDBList counts a daily allowance.
	importGaps:    map[string]time.Duration{Trakt: time.Second, Simkl: 500 * time.Millisecond, MDBList: 500 * time.Millisecond},
	importMaxWait: 15 * time.Minute,
	importRetries: 3,
}

// maxIntake bounds the reports and marks waiting to be looked at for one
// user: past it, new ones are dropped.
const maxIntake = 256

// Service connects users' accounts and sends what they watch.
type Service struct {
	db        *pgxpool.Pool
	settings  func() accounts.Settings
	box       *secrets.Box
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
	// refusedApps are the users whose code the service ended by refusing
	// the server's app, with the app's credentials it refused.
	refusedApps map[laneKey]string
	// intakes are the reports and marks waiting to be looked at, by user.
	intakes map[accounts.ID]*intake
	// sessions are the playbacks of movies and episodes under way, and
	// listens those of songs, by user and device.
	sessions map[sessionKey]*session
	listens  map[sessionKey]*listen
	// lanes are what waits to be sent, by user and service.
	lanes map[laneKey]*lane
	seq   int64
	// paces space every request to the services of sharedGaps.
	paces map[string]*pace

	titles   Titles
	userData *userdata.Store
	// importing are the imports running, by user and service.
	importing map[laneKey]*importRun
	// publicMetaDBIMDb caches the IMDb identifiers PublicMetaDB maps TMDB
	// identifiers to (see mapPublicMetaDB).
	publicMetaDBIMDb map[string]string
	// anime maps anime between identifiers and numberings.
	anime *anime.Service
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
		db:          options.DB,
		settings:    options.Settings,
		box:         options.Secrets,
		version:     options.Version,
		logger:      options.Logger,
		urls:        urls,
		client:      &http.Client{},
		now:         time.Now,
		timing:      defaultTiming,
		ctx:         ctx,
		cancel:      cancel,
		pending:     map[laneKey]*pending{},
		refusedApps: map[laneKey]string{},
		intakes:     map[accounts.ID]*intake{},
		sessions:    map[sessionKey]*session{},
		listens:     map[sessionKey]*listen{},
		paces:       map[string]*pace{PublicMetaDB: {}, LastFM: {}},
		lanes:       map[laneKey]*lane{},
		titles:      options.Titles,
		userData:    options.UserData,
		importing:   map[laneKey]*importRun{},

		publicMetaDBIMDb: map[string]string{},
		anime:            options.Anime,
	}
}

// Run sends the changes an earlier run left queued, keeps tokens fresh
// and imports watch histories when due until ctx ends, then stops sending
// and waits for the sends under way.
func (s *Service) Run(ctx context.Context) {
	s.resumeQueued()
	s.importDue()
	ticker := time.NewTicker(s.timing.refreshEvery)
	defer ticker.Stop()
	imports := time.NewTicker(s.timing.importCheck)
	defer imports.Stop()
	for {
		select {
		case <-ctx.Done():
			s.Close()
			return
		case <-ticker.C:
			s.refreshDue()
		case <-imports.C:
			s.importDue()
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
