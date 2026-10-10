// Package jellyfinimport moves users and what they watched from a Jellyfin
// server, or a server that speaks Jellyfin's API, to Polyfin. An
// administrator connects to the server with its address and a key, or a
// user's name and password, and lists its users, then chooses the Polyfin
// account each one's watch data goes to, creating those that do not exist
// yet. The import then reads, user by user, the movies and episodes played
// with their dates and play counts, the resume points, and the favorite
// movies, series and episodes, and adds them to the accounts.
//
// Titles are found by their IMDb, TMDB or TVDB identifiers, episodes by
// their series' and their numbers, as the watch histories of tracking
// services are (see trackers), and merge the same way: Polyfin's data is
// only added to, never taken back, and a newer resume point of Polyfin's
// stays. Running an import again adds nothing twice. The Jellyfin server is
// only read, but for the sessions signing in opens and the import ends.
//
// An API key of the server's dashboard reads every user's watch data. A
// user's own key or access token, or a user signed in, reads that user's
// only: Jellyfin refuses it the others', and some servers that speak
// Jellyfin's API answer it with its owner's data whatever user is asked,
// which would land in the wrong accounts. Another user's watch data is
// then read signed in as them, with their password, as Jellyfin's apps
// sign in: every such server lets its apps do so.
//
// One import runs at a time, in the background. It and the last one's
// result are kept in memory: an import cut by a restart is lost and is
// started again.
package jellyfinimport

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
)

// The ways asking for an import fails before it starts.
var (
	// ErrRunning reports an import asked while one runs.
	ErrRunning = errors.New("a Jellyfin import is running")
	// ErrNotKeyOwner reports watch data asked, with a user's key, of
	// another user than the key's owner, without signing in as them (see
	// Server.KeyOwner).
	ErrNotKeyOwner = errors.New("a user's key reads only that user's watch data")
)

// Titles finds the items of titles named by other services' identifiers
// (see library.Service.Resolve).
type Titles interface {
	Resolve(ctx context.Context, refs []library.TitleRef) ([][]library.TitleTarget, error)
}

// Options are the dependencies of the service.
type Options struct {
	// Settings give the thresholds resume points are kept and titles
	// played at.
	Settings func() accounts.Settings
	Titles   Titles
	UserData *userdata.Store
	// Version is Polyfin's, which the Jellyfin server is told.
	Version string
	Logger  *slog.Logger
}

// timing is how the service paces its requests. Tests shorten it.
type timing struct {
	// gap is the least time between two requests: the server is the
	// administrator's own, and keeps serving its users meanwhile.
	gap time.Duration
	// request bounds each request; a page of a large library takes the
	// server a while.
	request time.Duration
	// retryFirst is the first wait after a failed request of an import,
	// doubling for each of retries more tries; maxWait bounds how long a
	// server asking to wait is waited for.
	retryFirst, maxWait time.Duration
	retries             int
	// pageSize is how many items a request reads.
	pageSize int
}

var defaultTiming = timing{
	gap:        100 * time.Millisecond,
	request:    time.Minute,
	retryFirst: 5 * time.Second,
	maxWait:    time.Minute,
	retries:    3,
	pageSize:   200,
}

// Service lists the users of Jellyfin servers and imports their watch
// data, one import at a time.
type Service struct {
	settings func() accounts.Settings
	titles   Titles
	userData *userdata.Store
	version  string
	logger   *slog.Logger
	client   *http.Client
	timing   timing
	now      func() time.Time
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	mu sync.Mutex
	// busy is set from the moment an import is asked until it ends.
	busy bool
	// current is the import running, else the last one; stop ends the
	// one running.
	current *Status
	stop    context.CancelFunc
}

// New returns a service importing with options.
func New(options Options) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		settings: options.Settings,
		titles:   options.Titles,
		userData: options.UserData,
		version:  options.Version,
		logger:   options.Logger,
		client:   &http.Client{},
		timing:   defaultTiming,
		now:      time.Now,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Close stops the import running and waits for it to end.
func (s *Service) Close() {
	s.cancel()
	s.wg.Wait()
}

// The states of an import.
const (
	StateRunning = "running"
	StateDone    = "done"
	// StateStopped is an import an administrator stopped, StateFailed one
	// the server stopped answering, or refused the key of, midway.
	StateStopped = "stopped"
	StateFailed  = "failed"
)

// The states of a user's import. A user still waiting once the import
// ended was not reached.
const (
	UserWaiting = "waiting"
	UserReading = "reading"
	UserSaving  = "saving"
	UserDone    = "done"
	UserFailed  = "failed"
)

// The problems that end an import: ProblemUnreachable, ProblemKeyRefused
// and ProblemNotJellyfin come from the server, ProblemInternal from
// Polyfin's database. ProblemForbidden, a user whose data the server does
// not let the key read, fails that user only: the import goes on.
const (
	ProblemUnreachable = "jellyfin_unreachable"
	ProblemKeyRefused  = "jellyfin_key_refused"
	ProblemNotJellyfin = "not_jellyfin"
	ProblemInternal    = "internal"
	ProblemForbidden   = "jellyfin_user_forbidden"
)

// The reasons a title is not matched: it has no identifier at all, as a
// home video, or an episode not numbered, or no Polyfin title has its
// identifiers.
const (
	ReasonNoIdentifier = "no_identifier"
	ReasonNotFound     = "not_found"
)

// maxUnmatched bounds the unmatched titles listed for each user; the
// count goes on.
const maxUnmatched = 500

// Status is how an import goes, or went.
type Status struct {
	ID         string
	ServerName string
	Address    string
	State      string
	// Problem is what ended an import that failed.
	Problem   string
	StartedAt time.Time
	EndedAt   *time.Time
	Users     []UserStatus
}

// UserStatus is how the import of one Jellyfin user's watch data goes.
type UserStatus struct {
	JellyfinID, JellyfinName string
	User                     accounts.ID
	UserName                 string
	State                    string
	// Read counts the items read from Jellyfin. Played counts the titles
	// marked played that were not, Resumed the resume points set, and
	// Favorites the titles made favorites.
	Read, Played, Resumed, Favorites int
	// Unmatched lists the first titles not found, UnmatchedCount counts
	// them all.
	Unmatched      []Unmatched
	UnmatchedCount int
	Problem        string
}

// Unmatched is a title of Jellyfin's that no Polyfin title matched.
type Unmatched struct {
	// Name is the title's; Kind is "movie", "series" or "episode"; Year is
	// zero when Jellyfin does not tell it.
	Name, Kind string
	Year       int
	// Series, Season and Episode place an episode; Season and Episode are
	// nil when Jellyfin does not number it.
	Series          string
	Season, Episode *int
	Reason          string
}

// clone copies s, so that what the import changes later does not show.
func (s *Status) clone() *Status {
	c := *s
	c.Users = slices.Clone(s.Users)
	for i := range c.Users {
		c.Users[i].Unmatched = slices.Clone(c.Users[i].Unmatched)
	}
	return &c
}

// Current returns the import running, else the last one, nil when none
// ran since Polyfin started.
func (s *Service) Current() *Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == nil {
		return nil
	}
	return s.current.clone()
}

// Credentials are what Polyfin connects to a Jellyfin server with: a key,
// which is an API key of the server's dashboard or a user's own key or
// access token, or a user's name and password, which it signs in with as
// Jellyfin's apps do, then signs out once done. A password may be empty: a
// Jellyfin account may have none. A name, when given, is used over a key.
type Credentials struct {
	Key            string
	Name, Password string
}

// Users reads the server at address with credentials: which server it is,
// and its users. Signed in, it signs out before returning. It fails with
// ErrInvalidAddress, ErrUnreachable, ErrNotJellyfin, ErrKeyRefused,
// ErrForbidden, ErrSignInRefused or ErrSignInForbidden.
func (s *Service) Users(ctx context.Context, address string, credentials Credentials) (Server, []User, error) {
	address, err := ParseAddress(address)
	if err != nil {
		return Server{}, nil, err
	}
	// An administrator waits on the answer: a server that fails is told
	// at once.
	c := &client{s: s, address: address}
	defer c.signOut()
	return c.connect(ctx, credentials)
}

// Target is a Jellyfin user whose watch data an import adds to a Polyfin
// account.
type Target struct {
	JellyfinID string
	User       accounts.ID
	UserName   string
}

// SignIn signs in, while an import starts, as the Jellyfin user jellyfinID
// names, with their password on the server: the import reads their watch
// data as them. It fails as Users does, or with ErrOtherUser.
type SignIn func(jellyfinID, password string) error

// Start reads the server at address with credentials again, hands it and
// its users to choose, which maps them to Polyfin accounts, creating them
// as needed, and imports in the background the watch data of the users it
// returns. With a user's key (see Server.KeyOwner), choose must sign in as
// each other user it returns, with signIn, before creating any account:
// Start fails with ErrNotKeyOwner, reading no one, when it did not. The
// import reads the users signed in as themselves, ends each session once
// that user is read, and its own at its end. Start returns the import,
// nil when choose returned no one. While an import runs, it fails with
// ErrRunning before reading anything; it fails as Users does, or with
// choose's error.
func (s *Service) Start(ctx context.Context, address string, credentials Credentials, choose func(Server, []User, SignIn) ([]Target, error)) (*Status, error) {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return nil, ErrRunning
	}
	s.busy = true
	s.mu.Unlock()
	// own reads with the credentials; sessions read as the users signed in
	// as themselves, by their Jellyfin identifier. Until the import takes
	// them over, failing ends them.
	address, err := ParseAddress(address)
	own := &client{s: s, address: address}
	sessions := map[string]*client{}
	started := false
	defer func() {
		if !started {
			own.signOut()
			for _, session := range sessions {
				session.signOut()
			}
			s.mu.Lock()
			s.busy = false
			s.mu.Unlock()
		}
	}()
	if err != nil {
		return nil, err
	}

	server, users, err := own.connect(ctx, credentials)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(users))
	for _, u := range users {
		names[u.ID] = u.Name
	}
	signIn := func(jellyfinID, password string) error {
		name, ok := names[jellyfinID]
		if !ok {
			return errors.New("not a user of the Jellyfin server")
		}
		if sessions[jellyfinID] != nil {
			return nil
		}
		session := &client{s: s, address: server.Address}
		signed, err := session.signIn(ctx, name, password)
		if err != nil {
			return err
		}
		if signed != jellyfinID {
			session.signOut()
			return ErrOtherUser
		}
		sessions[jellyfinID] = session
		return nil
	}
	targets, err := choose(server, users, signIn)
	if err != nil || len(targets) == 0 {
		return nil, err
	}
	for _, t := range targets {
		if server.KeyOwner != "" && t.JellyfinID != server.KeyOwner && sessions[t.JellyfinID] == nil {
			return nil, ErrNotKeyOwner
		}
	}
	// The sessions of users not imported end now.
	used := make(map[string]*client, len(sessions))
	for _, t := range targets {
		if session := sessions[t.JellyfinID]; session != nil {
			session.retries = s.timing.retries
			used[t.JellyfinID] = session
		}
	}
	for id, session := range sessions {
		if used[id] == nil {
			session.signOut()
		}
	}
	sessions = used
	status := &Status{ID: newID(), ServerName: server.Name, Address: server.Address, State: StateRunning, StartedAt: s.now().UTC()}
	for _, t := range targets {
		status.Users = append(status.Users, UserStatus{JellyfinID: t.JellyfinID, JellyfinName: names[t.JellyfinID], User: t.User,
			UserName: t.UserName, State: UserWaiting})
	}
	runCtx, stop := context.WithCancel(s.ctx)
	s.mu.Lock()
	if s.ctx.Err() != nil {
		s.mu.Unlock()
		stop()
		return nil, s.ctx.Err()
	}
	s.current, s.stop = status, stop
	s.wg.Add(1)
	started = true
	snapshot := status.clone()
	s.mu.Unlock()

	own.retries = s.timing.retries
	r := &run{s: s, status: status, client: own, sessions: sessions, series: map[string]map[string]string{}}
	go func() {
		defer s.wg.Done()
		defer stop()
		r.run(runCtx)
	}()
	s.logger.Info("A Jellyfin import started", "server", server.Name, "users", len(targets))
	return snapshot, nil
}

// Stop stops the import running, if any: what it imported stays.
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stop != nil {
		s.stop()
	}
}
