// Package letterboxd imports a Letterboxd member's export file into their
// Polyfin account: the movies they watched, with their dates and play
// counts, their watchlist, their ratings and, when they ask for it, the
// movies they liked, as favorites.
//
// Letterboxd has no API for this: every member downloads their data as a
// ZIP of CSV files, which names movies by their name, year and Letterboxd
// link only. The file is read in memory, never kept (see Parse), and each
// movie is searched by its name in the server's metadata addons, as a
// local file is: it matches when they find exactly one movie of that name
// and year (see localfiles.ChooseTitle). Searches run one at a time, each
// name once per import.
//
// What is found merges as the other imports' data does: played marks,
// ratings and favorites are only added, a title keeps Polyfin's date when
// it is later and the larger play count, a rating Polyfin has stays, and
// the watchlist adds to the user's (see watchlist.Import), where a title
// stays until started. Importing the same export again adds nothing twice.
//
// One import into a user's account runs at a time. It and the last one's
// result are kept in memory: an import cut by a restart is lost and is
// started again.
package letterboxd

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// ErrRunning reports an import into an account where one runs.
var ErrRunning = errors.New("an import into this account is running")

// Titles searches the server's metadata addons for titles, and finds the
// items of titles named by their identifiers (see library.Service).
type Titles interface {
	RemoteSearch(ctx context.Context, kind library.Kind, name string, limit int) ([]library.RemoteResult, error)
	RemoteSearches(ctx context.Context, kind library.Kind) (bool, error)
	Resolve(ctx context.Context, refs []library.TitleRef) ([][]library.TitleTarget, error)
}

// Options are the dependencies of the service.
type Options struct {
	Titles    Titles
	UserData  *userdata.Store
	Watchlist *watchlist.Store
	Logger    *slog.Logger
}

// Service imports exports, one import into an account at a time.
type Service struct {
	titles    Titles
	userData  *userdata.Store
	watchlist *watchlist.Store
	logger    *slog.Logger
	now       func() time.Time
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup

	mu sync.Mutex
	// imports are the import running into each account, else its last;
	// stops end those running.
	imports map[accounts.ID]*Status
	stops   map[accounts.ID]context.CancelFunc
}

// New returns a service importing with options.
func New(options Options) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{titles: options.Titles, userData: options.UserData, watchlist: options.Watchlist, logger: options.Logger,
		now: time.Now, ctx: ctx, cancel: cancel, imports: map[accounts.ID]*Status{}, stops: map[accounts.ID]context.CancelFunc{}}
}

// Close stops the imports running and waits for them to end.
func (s *Service) Close() {
	s.cancel()
	s.wg.Wait()
}

// Searchable reports whether the server's metadata addons search movies:
// without one, no movie of an export can be found.
func (s *Service) Searchable(ctx context.Context) (bool, error) {
	return s.titles.RemoteSearches(ctx, library.KindMovie)
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

// The reasons a movie is not matched: the metadata addons know no movie of
// its name and year, they know several, or the search failed.
const (
	ReasonNotFound     = "not_found"
	ReasonAmbiguous    = "ambiguous"
	ReasonSearchFailed = "search_failed"
)

// maxUnmatched bounds the movies not found an import lists; the count
// goes on.
const maxUnmatched = 500

// Choice is what is imported besides the movies watched: the watchlist,
// the ratings, and the movies liked, as favorites.
type Choice struct {
	Watchlist, Ratings, LikesAsFavorites bool
}

// Status is how an import goes, or went.
type Status struct {
	ID string
	// User is the account imported into, UserName its name then.
	User     accounts.ID
	UserName string
	// StartedBy is the user who started the import, StartedByName their
	// name then: the account's user, or an administrator.
	StartedBy     accounts.ID
	StartedByName string
	Choice        Choice
	State         string
	// Problem is what ended an import that failed.
	Problem   string
	StartedAt time.Time
	EndedAt   *time.Time
	// Movies counts the movies of the export to find, Searched those
	// looked up so far.
	Movies, Searched int
	// Played counts the movies marked played that were not, Watchlist
	// those the import added to the user's watchlist that were not on it,
	// nor started, Ratings the ratings set, and Favorites the movies made
	// favorites.
	Played, Watchlist, Ratings, Favorites int
	// Unmatched lists the first movies not found, UnmatchedCount counts
	// them all.
	Unmatched      []Unmatched
	UnmatchedCount int
}

// Unmatched is a movie of the export no title matched, and why.
type Unmatched struct {
	Name   string
	Year   int
	Link   string
	Reason string
}

// clone copies s, so that what the import changes later does not show.
func (s *Status) clone() *Status {
	c := *s
	c.Unmatched = slices.Clone(s.Unmatched)
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

// Running reports whether an import into user's account runs.
func (s *Service) Running(user accounts.ID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.imports[user] != nil && s.imports[user].State == StateRunning
}

// Start imports, in the background, what choice asks for of export into
// user's account: by themselves, or an administrator. Only the movies it
// imports are searched. It fails with ErrRunning while an import into
// user's account runs.
func (s *Service) Start(by, user accounts.User, export Export, choice Choice) (*Status, error) {
	movies := make([]Movie, 0, len(export.Movies))
	for _, m := range export.Movies {
		if m.Watched || choice.Watchlist && m.Watchlist || choice.Ratings && m.Rating != nil || choice.LikesAsFavorites && m.Liked {
			movies = append(movies, m)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current := s.imports[user.ID]; current != nil && current.State == StateRunning {
		return nil, ErrRunning
	}
	if s.ctx.Err() != nil {
		return nil, s.ctx.Err()
	}
	status := &Status{ID: newID(), User: user.ID, UserName: user.Name, StartedBy: by.ID, StartedByName: by.Name, Choice: choice,
		State: StateRunning, StartedAt: s.now().UTC(), Movies: len(movies)}
	s.imports[user.ID] = status
	ctx, stop := context.WithCancel(s.ctx)
	s.stops[user.ID] = stop
	r := &run{s: s, status: status, user: user.ID, movies: movies, choice: choice}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer stop()
		r.run(ctx)
	}()
	s.logger.Info("A Letterboxd import started", "user_id", user.ID.String(), "movies", len(movies))
	return status.clone(), nil
}

// Leaving stops the import into user's account, as user is being deleted,
// and forgets their last one, once the deletion is committed (see
// accounts.Store.OnDelete).
func (s *Service) Leaving(_ context.Context, _ pgx.Tx, user accounts.ID) (func(), error) {
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if stop := s.stops[user]; stop != nil {
			stop()
		}
		delete(s.stops, user)
		delete(s.imports, user)
	}, nil
}

// newID returns a random identifier for an import.
func newID() string {
	var id accounts.ID
	_, _ = rand.Read(id[:])
	return id.String()
}
