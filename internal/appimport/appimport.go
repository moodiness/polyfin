// Package appimport imports the watch data of a user's account on a media
// app built on Stremio addons into their Polyfin account: the movies and
// episodes they played, their resume points, their library, which goes on
// their watchlist (see package watchlist), and, when they choose them,
// the app's addons, which become their own addons.
//
// Each app is read by a source of its own, which signs in to the account
// the app's way and reads it whole (see packages nuvio and stremio): it
// hands the account over as an Account, normalized, with its profiles.
// The service holds it in memory for a few minutes, while the user chooses
// the profile to import and its addons (see Service.Hold), then imports
// that profile in the background (see Service.Start). Nothing the source
// signed in with is kept, unless the user chose to stay connected: then
// the import keeps what reads the account again, sealed, and imports the
// same profile as often as the settings say (see connections.go).
//
// Titles are found by the identifiers the source names them by (see
// TitleRef), as the watch histories of tracking services and other servers
// are, and merge the same way: Polyfin's data is only added to, never
// taken back, and a newer resume point of Polyfin's stays. Running an
// import again adds nothing twice. The library is the watchlist of the
// app's connection, which each of its imports replaces, or, for a one-off
// import, adds to the titles earlier ones brought. Favorites stay the
// user's own: an import sets none.
//
// One import into a user's account runs at a time. It and the last one's
// result are kept in memory: an import cut by a restart is lost and is
// started again.
package appimport

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/activity"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/userdata"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// The ways asking for an import fails before it starts.
var (
	// ErrRunning reports an import into an account where one runs.
	ErrRunning = errors.New("an import into this account is running")
	// ErrExpired reports an account read that is no longer held: it was
	// dropped after holdFor, imported already, replaced by another read,
	// or read by another user.
	ErrExpired = errors.New("the account read is no longer held: sign in again")
	// ErrUnknownProfile reports a profile the account does not have.
	ErrUnknownProfile = errors.New("the account has no such profile")
	// ErrUnknownAddon reports an addon the profile does not have.
	ErrUnknownAddon = errors.New("the profile has no such addon")
	// ErrAddonsNotAllowed reports addons chosen for a user who may not add
	// their own (see accounts.Settings.PersonalAddonsAllowed).
	ErrAddonsNotAllowed = errors.New("the user may not add their own addons")
)

// holdFor is how long an account read is held while the user chooses what
// to import, at most.
const holdFor = 5 * time.Minute

// batch is how many items an import resolves and writes at a time.
const batch = 500

// maxUnmatched bounds the unmatched titles an import lists; the count goes
// on.
const maxUnmatched = 500

// Account is an app account a source signed in to and read.
type Account struct {
	// App names the app: "nuvio" or "stremio".
	App string
	// Profiles are the account's, in the app's order; an app without
	// profiles has one.
	Profiles []Profile
	// Name is the account's email or name, as the app gives it, empty
	// when it gives none.
	Name string
	// SignIn reads the account again, for a connection: set when the
	// user asked to stay connected, nil otherwise. It is never answered
	// nor logged.
	SignIn *SignIn
}

// Profile is one of an account's profiles, and what it holds.
type Profile struct {
	// ID identifies the profile in the account; Name is how the app
	// names it to the user.
	ID, Name string
	Entries  []Entry
	Addons   []Addon
}

// Entry is a title of a profile: a movie, a series, or an episode, and
// what the profile holds of it. A source gives one entry per title.
type Entry struct {
	// Title names the title by the identifiers Polyfin finds titles by.
	// A Title without any (see Identified) counts as unmatched, with
	// ReasonOtherIdentifier.
	Title library.TitleRef
	// Label describes the entry in the report, when it is not matched.
	Label Label
	// Played marks the title played: at PlayedAt when the app tells when,
	// Plays times when it counts them (zero when it does not).
	Played   bool
	PlayedAt *time.Time
	Plays    int
	// Resume is the title's resume point, nil when it has none.
	Resume *Resume
	// Watchlist is set for a title of the app's library, which goes on
	// the user's watchlist; AddedAt is when it was added to the library,
	// nil when the app does not tell.
	Watchlist bool
	AddedAt   *time.Time
	// Problem names why the source could not read part of what the
	// profile holds of the title, empty when it read it all: the entry
	// is listed among the unmatched with this reason, once its title is
	// found, and what was read of it is imported still.
	Problem string
}

// Resume is a resume point: at Position of Runtime, set at At. Runtime is
// zero when the app does not tell it: the title's, as its addons tell it,
// is used.
type Resume struct {
	Position, Runtime time.Duration
	At                time.Time
}

// Label describes an entry to the user: its ID as the app names it, its
// Name, empty when the app gives none, its Kind ("movie", "series" or
// "episode"), and for an episode its Season and Episode numbers.
type Label struct {
	ID, Name, Kind  string
	Season, Episode int
}

// Addon is an addon of a profile: its Name, and the address of its
// manifest, which may hold the user's settings and keys, and is never
// logged nor shown in full.
type Addon struct {
	Name, ManifestURL string
}

// Titles finds the items of titles named by other services' identifiers
// (see library.Service.Resolve).
type Titles interface {
	Resolve(ctx context.Context, refs []library.TitleRef) ([][]library.TitleTarget, error)
}

// Installer installs addons (see addons.Store.Install).
type Installer interface {
	Install(ctx context.Context, scope addons.Scope, rawURL string, confined bool) (addons.Addon, error)
}

// Options are the dependencies of the service.
type Options struct {
	// Settings give the thresholds resume points are kept and titles
	// played at, and whether users may add their own addons.
	Settings func() accounts.Settings
	Titles   Titles
	UserData *userdata.Store
	Addons   Installer
	// Activity records the addons installed; nil records nothing.
	Activity *activity.Store
	Logger   *slog.Logger
	// DB keeps the connections of the users who stay connected; nil keeps
	// none: the sign-ins kept are signed out once imported.
	DB *pgxpool.Pool
	// Secrets seals what the connections sign in with; nil stores it as
	// it is.
	Secrets *secrets.Box
	// Sources read the accounts of the connections again, and sign them
	// out, by app.
	Sources map[string]Source
	// Stopped is told once each time a connection stops, as its app
	// refuses the sign-in kept; nil tells no one.
	Stopped func(user accounts.ID, app string)
	// Watchlist keeps the titles of the apps' libraries, on the users'
	// watchlists; nil keeps none.
	Watchlist *watchlist.Store
}

// Service holds the accounts read until the user chooses what to import,
// and imports them, one import into an account at a time.
type Service struct {
	settings func() accounts.Settings
	titles   Titles
	userData *userdata.Store
	addons   Installer
	activity *activity.Store
	logger   *slog.Logger
	db       *pgxpool.Pool
	box      *secrets.Box
	sources  map[string]Source
	stopped  func(user accounts.ID, app string)
	now      func() time.Time
	holdFor  time.Duration
	timing   timing
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	// signOuts bounds the sign-outs in the background, which Close gives
	// closeWait to end before cancelling them.
	signOuts       context.Context
	cancelSignOuts context.CancelFunc
	closeWait      time.Duration

	mu     sync.Mutex
	closed bool
	// held are the accounts read, by the user who read them: one each.
	held map[accounts.ID]*held
	// imports are the import running into each account, else its last.
	imports map[accounts.ID]*Status
	// connecting are the imports of connections running.
	connecting map[connectionKey]*connectionRun
	// watchlist keeps the titles of the apps' libraries.
	watchlist *watchlist.Store
}

// held is an account read, and the key that names it to its reader.
type held struct {
	key     string
	account Account
	timer   *time.Timer
}

// New returns a service importing with options.
func New(options Options) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	signOuts, cancelSignOuts := context.WithCancel(context.Background())
	return &Service{settings: options.Settings, titles: options.Titles, userData: options.UserData, addons: options.Addons,
		activity: options.Activity, logger: options.Logger, db: options.DB, box: options.Secrets, sources: options.Sources, now: time.Now,
		stopped: options.Stopped,
		holdFor: holdFor, timing: defaultTiming, ctx: ctx, cancel: cancel, signOuts: signOuts, cancelSignOuts: cancelSignOuts,
		closeWait: closeWait, held: map[accounts.ID]*held{}, imports: map[accounts.ID]*Status{}, connecting: map[connectionKey]*connectionRun{},
		watchlist: options.Watchlist}
}

// closeWait is how long Close waits, at most, for the sessions being
// signed out.
const closeWait = 5 * time.Second

// Close drops the accounts held, signing out the sessions they kept open,
// stops the imports running and waits for them to end, and for the
// sign-outs under way, for closeWait at most.
func (s *Service) Close() {
	s.mu.Lock()
	for by, h := range s.held {
		h.timer.Stop()
		delete(s.held, by)
		s.release(h.account.SignIn, h.account.App)
	}
	s.closed = true
	s.mu.Unlock()
	s.cancel()
	stop := time.AfterFunc(s.closeWait, s.cancelSignOuts)
	defer stop.Stop()
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

// release signs out, in the background, the session signIn of app keeps
// open when Polyfin opened it, as no connection keeps it. It is called
// with s.mu held.
func (s *Service) release(signIn *SignIn, app string) {
	source := s.sources[app]
	if signIn == nil || !signIn.Owned || source == nil {
		return
	}
	kept := *signIn
	s.spawn(func() { s.signOutNow(s.signOuts, source, kept, app) })
}

// signOutNow signs out the session signIn of app keeps open with source,
// logging a failure, without the sign-in.
func (s *Service) signOutNow(ctx context.Context, source Source, signIn SignIn, app string) {
	if err := source.SignOut(ctx, signIn); err != nil {
		s.logger.Warn("A session Polyfin opened at an app could not be signed out", "app", app, "error", err)
	}
}

// Hold keeps account, which by read, while they choose what to import,
// for a few minutes at most, and returns the key that names it. It drops
// the account by held before. An account dropped without being imported
// has the session it kept open for a connection signed out.
func (s *Service) Hold(by accounts.ID, account Account) string {
	var key [24]byte
	_, _ = rand.Read(key[:])
	h := &held{key: base64.RawURLEncoding.EncodeToString(key[:]), account: account}
	s.mu.Lock()
	defer s.mu.Unlock()
	if previous := s.held[by]; previous != nil {
		previous.timer.Stop()
		s.release(previous.account.SignIn, previous.account.App)
	}
	s.held[by] = h
	h.timer = time.AfterFunc(s.holdFor, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.held[by] == h {
			delete(s.held, by)
			s.release(h.account.SignIn, h.account.App)
		}
	})
	return h.key
}

// The states of an import.
const (
	StateRunning = "running"
	StateDone    = "done"
	// StateFailed is an import Polyfin's database failed.
	StateFailed = "failed"
)

// ProblemInternal is what fails an import: Polyfin's database.
const ProblemInternal = "internal"

// The reasons a title is not matched: the app names it by an identifier
// Polyfin does not find titles by, or no Polyfin title has its
// identifiers. ReasonEpisodeList is a series found, whose episodes played
// the app tells by their place in its list of the series' episodes,
// which the list Polyfin's addons give does not match: none of them is
// marked played (see Entry.Problem).
const (
	ReasonOtherIdentifier = "other_identifier"
	ReasonNotFound        = "not_found"
	ReasonEpisodeList     = "episode_list"
)

// The reasons a chosen addon is not installed: the user may no longer add
// their own addons; it is installed already; its address is not one of a
// manifest, or is on a local network, which only administrators reach; it
// did not answer, or not with a manifest; or Polyfin's database failed.
const (
	AddonNotAllowed     = "not_allowed"
	AddonExists         = "exists"
	AddonInvalidAddress = "invalid_address"
	AddonPrivateNetwork = "private_network"
	AddonUnreachable    = "unreachable"
	AddonInvalid        = "invalid_manifest"
	AddonInternal       = "internal"
)

// Status is how an import goes, or went.
type Status struct {
	ID string
	// App and Profile name the app and the profile imported.
	App, Profile string
	// User is the account imported into, UserName its name then.
	User     accounts.ID
	UserName string
	// StartedBy is the user who started the import, StartedByName their
	// name then: the account's user, or an administrator.
	StartedBy     accounts.ID
	StartedByName string
	State         string
	// Problem is what ended an import that failed.
	Problem   string
	StartedAt time.Time
	EndedAt   *time.Time
	// Read counts the entries of the profile. Played counts the titles
	// marked played that were not, Resumed the resume points set, and
	// Watchlist the titles the import added to the user's watchlist that
	// were not on it, nor started.
	Read, Played, Resumed, Watchlist int
	// Unmatched lists the first titles not found, UnmatchedCount counts
	// them all.
	Unmatched      []Unmatched
	UnmatchedCount int
	// Addons are the addons chosen, in order, and how their installation
	// went.
	Addons []AddonStatus
}

// Unmatched is an entry no Polyfin title matched, or one the source could
// not read whole, and why.
type Unmatched struct {
	Label
	Reason string
}

// AddonStatus is an addon an import installs: its name, the address of its
// manifest, redacted, whether it was installed, and why not.
type AddonStatus struct {
	Name, Address string
	Installed     bool
	Problem       string
}

// clone copies s, so that what the import changes later does not show.
func (s *Status) clone() *Status {
	c := *s
	c.Unmatched = slices.Clone(s.Unmatched)
	c.Addons = slices.Clone(s.Addons)
	return &c
}

// Current returns the import running into user's account, else the last
// one, nil when none ran since Polyfin started.
func (s *Service) Current(user accounts.ID) *Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.imports[user] == nil {
		return nil
	}
	return s.imports[user].clone()
}

// Choice is what is imported of an account held: the ID of one of its
// profiles, and the indexes of the addons of that profile to install.
type Choice struct {
	Profile string
	Addons  []int
}

// Start imports, in the background, the profile choice names of the
// account by holds under key, into user's account: by themselves, or an
// administrator. The addons chosen are installed as user's own, which
// needs them to be allowed; only an administrator's import reaches
// addresses on local networks. An account read to stay connected becomes
// user's connection to its app, in place of the one there was, which
// imports the profile chosen again as often as the settings say, and never
// its addons.
// Start stops holding the account, and returns the import. It fails with
// ErrExpired, ErrUnknownProfile, ErrUnknownAddon, ErrAddonsNotAllowed, or
// ErrRunning while an import into user's account runs, holding the account
// still but for ErrExpired.
func (s *Service) Start(by, user accounts.User, key string, choice Choice) (*Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := s.held[by.ID]
	if h == nil || key == "" || h.key != key {
		return nil, ErrExpired
	}
	i := slices.IndexFunc(h.account.Profiles, func(p Profile) bool { return p.ID == choice.Profile })
	if i < 0 {
		return nil, ErrUnknownProfile
	}
	profile := h.account.Profiles[i]
	chosen := make([]Addon, 0, len(choice.Addons))
	for _, a := range choice.Addons {
		if a < 0 || a >= len(profile.Addons) || slices.Contains(choice.Addons[:len(chosen)], a) {
			return nil, ErrUnknownAddon
		}
		chosen = append(chosen, profile.Addons[a])
	}
	if len(chosen) > 0 && !s.settings().PersonalAddonsAllowed(user) {
		return nil, ErrAddonsNotAllowed
	}
	if current := s.imports[user.ID]; current != nil && current.State == StateRunning {
		return nil, ErrRunning
	}
	if s.ctx.Err() != nil {
		return nil, s.ctx.Err()
	}
	// The account is dropped: what is imported of it goes with the run.
	h.timer.Stop()
	delete(s.held, by.ID)
	status := &Status{ID: newID(), App: h.account.App, Profile: profile.Name, User: user.ID, UserName: user.Name, StartedBy: by.ID,
		StartedByName: by.Name, State: StateRunning, StartedAt: s.now().UTC(), Read: len(profile.Entries),
		Addons: make([]AddonStatus, 0, len(chosen))}
	for _, a := range chosen {
		status.Addons = append(status.Addons, AddonStatus{Name: a.Name, Address: stremio.RedactManifestURL(a.ManifestURL)})
	}
	s.imports[user.ID] = status
	r := &run{s: s, status: status, by: by, user: user, entries: profile.Entries, addons: chosen, list: watchlist.Import}
	if signIn := h.account.SignIn; signIn != nil {
		if s.db == nil {
			s.release(signIn, h.account.App)
		} else {
			kept := *signIn
			kept.Profile = profile.ID
			r.connection = &newConnection{key: connectionKey{user.ID, h.account.App}, signIn: kept, account: h.account.Name, by: by.ID}
			r.connection.run = s.claim(r.connection.key, nil)
			r.list = watchlist.Connection(h.account.App)
		}
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		r.run(s.ctx)
	}()
	return status.clone(), nil
}

// newID returns a random identifier for an import.
func newID() string {
	var id accounts.ID
	_, _ = rand.Read(id[:])
	return id.String()
}
