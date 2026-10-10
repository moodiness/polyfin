// Package jellyfinimport moves users and what they watched from a Jellyfin
// server, a server that speaks Jellyfin's API, an Emby server or a Plex
// Media Server, to Polyfin. An administrator connects to the server with its
// address and a key, or a user's name and password, and lists its users,
// then chooses the Polyfin account each one's watch data goes to, creating
// those that do not exist yet. The import then reads, user by user, the
// movies and episodes played with their dates and play counts, the resume
// points, and the favorite movies, series and episodes, and adds them to the
// accounts. A user may also import their own watch data into their own
// account, signing in to a Jellyfin or Emby server as themselves (see
// Service.ImportOwn).
//
// Titles are found by their IMDb, TMDB or TVDB identifiers, episodes by
// their series' and their numbers, as the watch histories of tracking
// services are (see trackers), and merge the same way: Polyfin's data is
// only added to, never taken back, and a newer resume point of Polyfin's
// stays. Running an import again adds nothing twice. The server is only
// read, but for the sessions signing in opens and the import ends.
//
// An API key of the server's dashboard reads every user's watch data. A
// user's own key or access token, or a user signed in, reads that user's
// only: Jellyfin refuses it the others', and some servers that speak
// Jellyfin's API answer it with its owner's data whatever user is asked,
// which would land in the wrong accounts. Another user's watch data is
// then read signed in as them, with their password, as Jellyfin's apps
// sign in: every such server lets its apps do so. Emby does not tell whose
// a user's key is: only its API keys and users signed in are taken.
//
// Plex is read with its owner's token, which reads the owner's watch data
// from the library sections, and every other account's played history,
// but not their resume points. Plex has no favorites.
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
	ErrRunning = errors.New("an import is running")
	// ErrNotKeyOwner reports watch data asked, with a user's key, of
	// another user than the key's owner, without signing in as them (see
	// Server.KeyOwner).
	ErrNotKeyOwner = errors.New("a user's key reads only that user's watch data")
	// ErrNotSignedIn reports a user's own import asked of a Plex server, or
	// without a name to sign in with: it reads only the user who signs in.
	ErrNotSignedIn = errors.New("a user's own import signs in to a Jellyfin or Emby server")
)

// Kind is the kind of server an import reads.
type Kind string

const (
	// Jellyfin is a Jellyfin server, or a server that speaks its API.
	Jellyfin Kind = "jellyfin"
	// Emby is an Emby server, which takes its own token header, and serves
	// its API under /emby.
	Emby Kind = "emby"
	// Plex is a Plex Media Server, read with its owner's token.
	Plex Kind = "plex"
)

// Connection is a server an import reads, and what it connects with.
type Connection struct {
	Kind    Kind
	Address string
	Credentials
}

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
	// Version is Polyfin's, which the server is told.
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

// Service lists the users of Jellyfin, Emby and Plex servers and imports
// their watch data, one import at a time.
type Service struct {
	settings func() accounts.Settings
	titles   Titles
	userData *userdata.Store
	version  string
	logger   *slog.Logger
	client   *http.Client
	timing   timing
	now      func() time.Time
	// device names Polyfin to Plex, which lists the devices reading it:
	// one for as long as Polyfin runs.
	device string
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu sync.Mutex
	// busy is set from the moment an import is asked until it ends.
	busy bool
	// current is the import running, else the last one; stop ends the
	// one running. own are the last imports users made of their own watch
	// data, by the user.
	current *Status
	own     map[accounts.ID]*Status
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
		device:   newID(),
		ctx:      ctx,
		cancel:   cancel,
		own:      map[accounts.ID]*Status{},
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
	Kind       Kind
	ServerName string
	Address    string
	State      string
	// Problem is what ended an import that failed.
	Problem   string
	StartedAt time.Time
	EndedAt   *time.Time
	// StartedBy is the Polyfin user who started the import, StartedByName
	// their name then. Own tells an import of their own watch data.
	StartedBy     accounts.ID
	StartedByName string
	Own           bool
	Users         []UserStatus
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

// Own returns user's import of their own watch data running, else their
// last one, nil when they made none since Polyfin started.
func (s *Service) Own(user accounts.ID) *Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.own[user] == nil {
		return nil
	}
	return s.own[user].clone()
}

// Credentials are what Polyfin connects to a server with: a key, which is
// an API key of the server's dashboard, a user's own key or access token,
// or Plex's owner's token, or a user's name and password, which it signs in
// with as Jellyfin's apps do, then signs out once done. A password may be
// empty: an account may have none. A name, when given, is used over a key.
// Plex takes its owner's token only.
type Credentials struct {
	Key            string
	Name, Password string
}

// Users reads the server connection leads to: which server it is, and its
// users. Signed in, it signs out before returning. It fails with
// ErrInvalidAddress, ErrUnreachable, ErrNotJellyfin, ErrKeyRefused,
// ErrForbidden, ErrUserKey, ErrSignInRefused or ErrSignInForbidden.
func (s *Service) Users(ctx context.Context, connection Connection) (Server, []User, error) {
	address, err := ParseAddress(connection.Address)
	if err != nil {
		return Server{}, nil, err
	}
	// An administrator waits on the answer: a server that fails is told
	// at once.
	c := &client{s: s, kind: connection.Kind, address: address}
	defer c.signOut()
	return c.connect(ctx, connection.Credentials)
}

// Target is a Jellyfin user whose watch data an import adds to a Polyfin
// account.
type Target struct {
	JellyfinID string
	User       accounts.ID
	UserName   string
}

// SignIn signs in, while an import starts, as the user jellyfinID names,
// with their password on the server: the import reads their watch data as
// them. It fails as Users does, or with ErrOtherUser. Plex has no sign-in:
// it fails with ErrNotSignedIn.
type SignIn func(jellyfinID, password string) error

// Start reads the server connection leads to again, hands it and its users
// to choose, which maps them to Polyfin accounts, creating them as needed,
// and imports in the background the watch data of the users it returns.
// With a user's key (see Server.KeyOwner), choose must sign in as each
// other user it returns, with signIn, before creating any account: Start
// fails with ErrNotKeyOwner, reading no one, when it did not. The import
// reads the users signed in as themselves, ends each session once that
// user is read, and its own at its end. by is the Polyfin user who starts
// it. Start returns the import, nil when choose returned no one. While an
// import runs, it fails with ErrRunning before reading anything; it fails
// as Users does, or with choose's error.
func (s *Service) Start(ctx context.Context, connection Connection, by accounts.User,
	choose func(Server, []User, SignIn) ([]Target, error)) (*Status, error) {
	if !s.claim() {
		return nil, ErrRunning
	}
	// own reads with the credentials; sessions read as the users signed in
	// as themselves, by their identifier on the server. Until the import
	// takes them over, failing ends them.
	address, err := ParseAddress(connection.Address)
	own := &client{s: s, kind: connection.Kind, address: address}
	sessions := map[string]*client{}
	started := false
	defer func() {
		if !started {
			own.signOut()
			for _, session := range sessions {
				session.signOut()
			}
			s.release()
		}
	}()
	if err != nil {
		return nil, err
	}

	server, users, err := own.connect(ctx, connection.Credentials)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(users))
	for _, u := range users {
		names[u.ID] = u.Name
	}
	signIn := func(jellyfinID, password string) error {
		name, ok := names[jellyfinID]
		switch {
		case server.Kind == Plex:
			return ErrNotSignedIn
		case !ok:
			return errors.New("not a user of the server")
		case sessions[jellyfinID] != nil:
			return nil
		}
		session := &client{s: s, kind: server.Kind, address: server.Address}
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
			used[t.JellyfinID] = session
		}
	}
	for id, session := range sessions {
		if used[id] == nil {
			session.signOut()
		}
	}
	sessions = used
	status := s.newStatus(server, by, false)
	for _, t := range targets {
		status.Users = append(status.Users, UserStatus{JellyfinID: t.JellyfinID, JellyfinName: names[t.JellyfinID], User: t.User,
			UserName: t.UserName, State: UserWaiting})
	}
	snapshot, err := s.launch(status, own, sessions)
	started = err == nil
	return snapshot, err
}

// ImportOwn imports, in the background, the watch data of the user who
// signs in to the Jellyfin or Emby server connection leads to with its name
// and password, into the account of user, who starts it. It never lists
// the server's users, which a user's session may not, and reads the user
// signed in as themselves only: even a server that answers every user with
// the session's data gives them their own. The session ends with the
// import, and the password is not kept. It fails as Start does, or with
// ErrNotSignedIn for a Plex server or without a name.
func (s *Service) ImportOwn(ctx context.Context, connection Connection, user accounts.User) (*Status, error) {
	if connection.Kind == Plex || connection.Name == "" {
		return nil, ErrNotSignedIn
	}
	if !s.claim() {
		return nil, ErrRunning
	}
	address, err := ParseAddress(connection.Address)
	c := &client{s: s, kind: connection.Kind, address: address}
	started := false
	defer func() {
		if !started {
			c.signOut()
			s.release()
		}
	}()
	if err != nil {
		return nil, err
	}
	server, err := c.open(ctx, connection.Credentials)
	if err != nil {
		return nil, err
	}
	status := s.newStatus(server, user, true)
	status.Users = []UserStatus{{JellyfinID: server.KeyOwner, JellyfinName: connection.Name, User: user.ID, UserName: user.Name,
		State: UserWaiting}}
	snapshot, err := s.launch(status, c, nil)
	started = err == nil
	return snapshot, err
}

// claim marks the service busy for an import about to start, and reports
// false when one runs or starts already.
func (s *Service) claim() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		return false
	}
	s.busy = true
	return true
}

// release frees the service after an import that could not start.
func (s *Service) release() {
	s.mu.Lock()
	s.busy = false
	s.mu.Unlock()
}

// newStatus is a running import of server that by starts, without users
// yet; own tells an import of by's own watch data.
func (s *Service) newStatus(server Server, by accounts.User, own bool) *Status {
	return &Status{ID: newID(), Kind: server.Kind, ServerName: server.Name, Address: server.Address, State: StateRunning,
		StartedAt: s.now().UTC(), StartedBy: by.ID, StartedByName: by.Name, Own: own}
}

// launch runs the import of status in the background, reading with own and
// the sessions of the users signed in, and returns a copy of status. It
// fails only once the service is closed.
func (s *Service) launch(status *Status, own *client, sessions map[string]*client) (*Status, error) {
	runCtx, stop := context.WithCancel(s.ctx)
	s.mu.Lock()
	if s.ctx.Err() != nil {
		s.mu.Unlock()
		stop()
		return nil, s.ctx.Err()
	}
	s.current, s.stop = status, stop
	if status.Own {
		s.own[status.StartedBy] = status
	}
	s.wg.Add(1)
	snapshot := status.clone()
	s.mu.Unlock()

	own.retries = s.timing.retries
	for _, session := range sessions {
		session.retries = s.timing.retries
	}
	r := &run{s: s, status: status, client: own, sessions: sessions, series: map[string]map[string]string{}}
	go func() {
		defer s.wg.Done()
		defer stop()
		r.run(runCtx)
	}()
	s.logger.Info("An import started", "kind", status.Kind, "server", status.ServerName, "users", len(status.Users), "own", status.Own)
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
