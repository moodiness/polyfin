package appimport

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// A user who stays connected to an app has the account imported again
// every 6 hours, and when they ask, the way it was imported first: the
// profile chosen then, with the same merge, and never its addons. Polyfin
// only reads the account: nothing is written to the app.
//
// A connection keeps, in app_connections, what reads the account again
// (see SignIn), sealed with the server's secret key: never answered nor
// logged. One connection per app per user; a new one replaces it. When the
// app refuses what is kept, the connection stops until the user signs in
// again. Disconnecting forgets it, signing out first the session Polyfin
// opened.
//
// A member's connections are imported while the setting that lets users
// import their watch history allows it; an administrator's always.

// The problems of a connection's import: the app did not answer, asked to
// wait, answered what Polyfin does not read, no longer has the profile
// chosen, or refused the sign-in kept, which stops the connection.
// ProblemInternal is Polyfin's database failing.
const (
	ProblemUnreachable    = "unreachable"
	ProblemRateLimited    = "rate_limited"
	ProblemUnexpected     = "unexpected"
	ProblemProfileMissing = "profile_missing"
	ProblemSignInRefused  = "sign_in_refused"
)

// The ways a source reads a connection's account again fails (see
// Source.Reread); any other error counts as ProblemUnexpected.
var (
	// ErrSignInRefused reports a sign-in kept the app refuses: a key
	// revoked, a session ended.
	ErrSignInRefused = errors.New("the app refuses the sign-in kept")
	// ErrAppUnreachable reports an app that does not answer, or fails.
	ErrAppUnreachable = errors.New("the app does not answer")
	// ErrAppRateLimited reports an app asking to wait longer than a read
	// waits.
	ErrAppRateLimited = errors.New("the app asks to wait")
)

// The ways asking about a connection fails.
var (
	// ErrNotConnected reports an app the user is not connected to, or
	// whose sign-in the server's key cannot open.
	ErrNotConnected = errors.New("not connected to this app")
	// ErrStopped reports a connection the app refused the sign-in of: the
	// user signs in again.
	ErrStopped = errors.New("the app refused the sign-in kept: sign in again")
)

// SignIn is what reads an account again: never answered nor logged.
type SignIn struct {
	// Secret signs in: Stremio's auth key, Nuvio's refresh token.
	Secret string `json:"secret"`
	// Backend is the address of the Nuvio backend, empty for Stremio.
	Backend string `json:"backend,omitempty"`
	// Profile is the ID of the profile imported, which Start sets.
	Profile string `json:"profile"`
	// Owned tells that Polyfin opened the session, by a link code or a
	// password, rather than the user pasting a key their own app holds:
	// Polyfin signs it out when it no longer keeps it.
	Owned bool `json:"-"`
}

// Source reads again the accounts of an app, for connections.
type Source interface {
	// Reread reads the account signIn signs in to, and returns it with
	// the sign-in to keep from now on, which the app may have renewed: it
	// is returned even when reading fails. It fails with
	// ErrSignInRefused, ErrAppUnreachable, ErrAppRateLimited, or another
	// error for an answer Polyfin does not read; errors never hold the
	// sign-in.
	Reread(ctx context.Context, signIn SignIn) (Account, SignIn, error)
	// SignOut ends the session signIn signs in to, when Polyfin opened it,
	// as best it can.
	SignOut(ctx context.Context, signIn SignIn)
}

// timing is how often connections are imported. Tests shorten it.
type timing struct {
	// every is how long after an import of a connection the next one
	// runs, connections being looked for every check.
	every, check time.Duration
}

var defaultTiming = timing{every: 6 * time.Hour, check: 10 * time.Minute}

// connectionKey names a connection: its user, and its app.
type connectionKey struct {
	user accounts.ID
	app  string
}

// connectionRun is an import of a connection running: the first, Start's,
// or a later one, which cancel stops.
type connectionRun struct {
	cancel context.CancelFunc
}

// newConnection is a connection the import Start runs makes.
type newConnection struct {
	key     connectionKey
	signIn  SignIn
	account string
	by      accounts.ID
	// run is the import making it.
	run *connectionRun
}

// Connection is how a user's connection to an app stands.
type Connection struct {
	App string
	// Account is the account's email or name, as the app gave it, empty
	// when it gave none.
	Account     string
	ConnectedAt time.Time
	// ConnectedBy is who connected it, the user or an administrator, and
	// ConnectedByName their name; zero when their account was deleted.
	ConnectedBy     accounts.ID
	ConnectedByName string
	// Problem is ProblemSignInRefused once the app refused the sign-in
	// kept: nothing is imported until the user signs in again.
	Problem string
	// Paused tells a member's connection while the setting that lets
	// users import their watch history is off.
	Paused bool
	// Importing tells that an import of it runs now.
	Importing bool
	// LastImport is how the last import went, nil before the first ends.
	LastImport *ConnectionImport
	// NextImport is when the next import runs, nil while the connection
	// is stopped or paused.
	NextImport *time.Time
}

// ConnectionImport is how an import of a connection went.
type ConnectionImport struct {
	// At is when it ended.
	At time.Time
	// Read counts the entries of the profile, Played the titles marked
	// played that were not, Resumed the resume points set, Favorites the
	// titles made favorites, and Unmatched the titles not found.
	Read, Played, Resumed, Favorites, Unmatched int
	// Problem is empty, or what kept the import from reading the account
	// (see ProblemUnreachable), or ProblemInternal.
	Problem string
}

// seal returns what to store of signIn.
func (s *Service) seal(signIn SignIn) (string, error) {
	data, err := json.Marshal(signIn)
	if err != nil {
		return "", err
	}
	return s.box.Seal(string(data)), nil
}

// open reads a sign-in stored, owned or not: secrets.ErrUnreadable when the
// key cannot open it.
func (s *Service) open(stored string, owned bool) (SignIn, error) {
	opened, err := s.box.Open(stored)
	if err != nil {
		return SignIn{}, err
	}
	var signIn SignIn
	if err := json.Unmarshal([]byte(opened), &signIn); err != nil {
		return SignIn{}, err
	}
	signIn.Owned = owned
	return signIn, nil
}

// claim records an import of key running, stopping the later import of
// the connection it replaces, if one runs. It is called with s.mu held.
func (s *Service) claim(key connectionKey, cancel context.CancelFunc) *connectionRun {
	if previous := s.connecting[key]; previous != nil && previous.cancel != nil {
		previous.cancel()
	}
	run := &connectionRun{cancel: cancel}
	s.connecting[key] = run
	return run
}

// unclaim records the import run of key ended.
func (s *Service) unclaim(key connectionKey, run *connectionRun) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.connecting[key] == run {
		delete(s.connecting, key)
	}
}

// signOut signs out, in the background, the session signIn of app keeps
// open when Polyfin opened it.
func (s *Service) signOut(signIn SignIn, app string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.release(&signIn, app)
}

// connect saves the connection c, in place of the one there was, whose
// session is signed out when Polyfin opened it, and returns when it was
// made. When saving fails, the session c keeps open is signed out.
func (s *Service) connect(ctx context.Context, c *newConnection) (time.Time, error) {
	connectedAt := s.now().UTC().Truncate(time.Microsecond)
	sealed, err := s.seal(c.signIn)
	var previous struct {
		stored string
		owned  bool
		found  bool
	}
	if err == nil {
		err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
			err := tx.QueryRow(ctx, "DELETE FROM app_connections WHERE user_id = $1 AND app = $2 RETURNING sign_in, owned", c.key.user,
				c.key.app).Scan(&previous.stored, &previous.owned)
			switch {
			case err == nil:
				previous.found = true
			case !errors.Is(err, pgx.ErrNoRows):
				return err
			}
			_, err = tx.Exec(ctx, `INSERT INTO app_connections (user_id, app, sign_in, owned, account, connected_at, connected_by)
				VALUES ($1, $2, $3, $4, $5, $6, $7)`, c.key.user, c.key.app, sealed, c.signIn.Owned, c.account, connectedAt, c.by)
			return err
		})
	}
	if err != nil {
		s.signOut(c.signIn, c.key.app)
		return time.Time{}, err
	}
	if previous.found && previous.owned {
		// Signing in again may give the same session: it stays.
		if old, err := s.open(previous.stored, true); err == nil && old.Secret != c.signIn.Secret {
			s.signOut(old, c.key.app)
		}
	}
	s.logger.Info("A user stays connected to an app", "user_id", c.key.user.String(), "app", c.key.app)
	return connectedAt, nil
}

// connectionColumns are what a connection's status reads.
const connectionColumns = `c.app, c.sign_in, c.owned, c.account, c.connected_at, c.connected_by, coalesce(b.name, ''),
	coalesce(c.problem, ''), c.last_import_at, c.read, c.played, c.resumed, c.favorites, c.unmatched, coalesce(c.last_problem, '')`

// storedConnection is a connection as the database holds it, its sign-in
// opened.
type storedConnection struct {
	Connection
	signIn SignIn
}

// scanConnection reads a connection: secrets.ErrUnreadable when the key
// cannot open its sign-in.
func (s *Service) scanConnection(row pgx.Row) (storedConnection, error) {
	var c storedConnection
	var stored string
	var owned bool
	var by *accounts.ID
	var last *time.Time
	var result ConnectionImport
	if err := row.Scan(&c.App, &stored, &owned, &c.Account, &c.ConnectedAt, &by, &c.ConnectedByName, &c.Problem, &last, &result.Read,
		&result.Played, &result.Resumed, &result.Favorites, &result.Unmatched, &result.Problem); err != nil {
		return c, err
	}
	if by != nil {
		c.ConnectedBy = *by
	}
	if last != nil {
		result.At = *last
		c.LastImport = &result
	}
	var err error
	if c.signIn, err = s.open(stored, owned); err != nil {
		return c, errUnopened
	}
	return c, nil
}

// errUnopened reports a sign-in stored the server's key cannot open, or
// damaged: the connection counts as none.
var errUnopened = errors.New("the sign-in of an app connection cannot be opened")

// describe completes c, a connection of user, with whether an import of
// it runs, importing, and what is due.
func (s *Service) describe(c *Connection, user accounts.User, importing bool) {
	c.Paused = !user.IsAdministrator && !s.settings().ServerImports
	c.Importing = importing
	if c.Problem != "" || c.Paused {
		return
	}
	next := c.ConnectedAt
	if c.LastImport != nil {
		next = c.LastImport.At
	}
	next = next.Add(s.timing.every)
	c.NextImport = &next
}

// importing lists the apps of user's connections an import of runs now.
// It is read before the connections: an import records how it went
// before it stops running, so a connection read as not importing holds
// its last import.
func (s *Service) importing(user accounts.ID) map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	apps := map[string]bool{}
	for key := range s.connecting {
		if key.user == user {
			apps[key.app] = true
		}
	}
	return apps
}

// Connections lists user's connections, by app. A connection whose
// sign-in the server's key cannot open counts as none: the user signs in
// again.
func (s *Service) Connections(ctx context.Context, user accounts.User) ([]Connection, error) {
	connections := []Connection{}
	if s.db == nil {
		return connections, nil
	}
	importing := s.importing(user.ID)
	rows, err := s.db.Query(ctx, "SELECT "+connectionColumns+` FROM app_connections c LEFT JOIN users b ON b.id = c.connected_by
		WHERE c.user_id = $1 ORDER BY c.app`, user.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		c, err := s.scanConnection(rows)
		if errors.Is(err, errUnopened) {
			continue
		}
		if err != nil {
			return nil, err
		}
		s.describe(&c.Connection, user, importing[c.App])
		connections = append(connections, c.Connection)
	}
	return connections, rows.Err()
}

// connection loads user's connection to app: ErrNotConnected when there
// is none, or none the key can open.
func (s *Service) connection(ctx context.Context, user accounts.ID, app string) (storedConnection, error) {
	if s.db == nil {
		return storedConnection{}, ErrNotConnected
	}
	c, err := s.scanConnection(s.db.QueryRow(ctx, "SELECT "+connectionColumns+` FROM app_connections c
		LEFT JOIN users b ON b.id = c.connected_by WHERE c.user_id = $1 AND c.app = $2`, user, app))
	switch {
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, errUnopened):
		return storedConnection{}, ErrNotConnected
	case err != nil:
		return storedConnection{}, err
	}
	return c, nil
}

// ImportNow imports user's connection to app at once, unless an import of
// it runs already, and returns how it stands. It fails with
// ErrNotConnected, or ErrStopped once the app refused the sign-in kept.
func (s *Service) ImportNow(ctx context.Context, user accounts.User, app string) (Connection, error) {
	c, err := s.connection(ctx, user.ID, app)
	if err != nil {
		return Connection{}, err
	}
	if c.Problem != "" {
		return Connection{}, ErrStopped
	}
	s.startConnection(connectionKey{user.ID, app})
	s.describe(&c.Connection, user, s.importing(user.ID)[app])
	return c.Connection, nil
}

// Disconnect forgets user's connection to app, stopping its import, after
// signing out the session Polyfin opened. A key the user pasted is only
// forgotten. Disconnecting an app not connected does nothing.
func (s *Service) Disconnect(ctx context.Context, user accounts.ID, app string) error {
	if s.db == nil {
		return nil
	}
	key := connectionKey{user, app}
	s.mu.Lock()
	if run := s.connecting[key]; run != nil && run.cancel != nil {
		run.cancel()
		delete(s.connecting, key)
	}
	s.mu.Unlock()
	var stored string
	var owned bool
	err := s.db.QueryRow(ctx, "DELETE FROM app_connections WHERE user_id = $1 AND app = $2 RETURNING sign_in, owned", user, app).
		Scan(&stored, &owned)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if signIn, err := s.open(stored, owned); err == nil && owned {
		if source := s.sources[app]; source != nil {
			source.SignOut(ctx, signIn)
		}
	}
	s.logger.Info("A user disconnected an app", "user_id", user.String(), "app", app)
	return nil
}

// Run imports the connections due until ctx ends, then stops the imports
// running.
func (s *Service) Run(ctx context.Context) {
	s.importDue()
	ticker := time.NewTicker(s.timing.check)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.Close()
			return
		case <-ticker.C:
			s.importDue()
		}
	}
}

// importDue starts the imports due: of connections not stopped, not
// imported for timing.every, of users enabled, administrators or members
// while the setting lets users import their watch history.
func (s *Service) importDue() {
	if s.db == nil {
		return
	}
	rows, err := s.db.Query(s.ctx, `SELECT c.user_id, c.app FROM app_connections c JOIN users u ON u.id = c.user_id
		WHERE c.problem IS NULL AND NOT u.is_disabled AND (u.is_administrator OR $1)
			AND coalesce(c.last_import_at, c.connected_at) <= $2`, s.settings().ServerImports, s.now().Add(-s.timing.every))
	if err != nil {
		if s.ctx.Err() == nil {
			s.logger.Warn("The app connections due could not be read", "error", err)
		}
		return
	}
	due, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (connectionKey, error) {
		var key connectionKey
		return key, row.Scan(&key.user, &key.app)
	})
	if err != nil {
		if s.ctx.Err() == nil {
			s.logger.Warn("The app connections due could not be read", "error", err)
		}
		return
	}
	for _, key := range due {
		s.startConnection(key)
	}
}

// startConnection imports the connection key names in the background,
// unless an import of it runs already.
func (s *Service) startConnection(key connectionKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.connecting[key] != nil || s.sources[key.app] == nil {
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	run := s.claim(key, cancel)
	if !s.spawn(func() {
		defer cancel()
		defer s.unclaim(key, run)
		s.importConnection(ctx, key)
	}) {
		delete(s.connecting, key)
		cancel()
	}
}

// importConnection reads the account of the connection key names again,
// keeping the sign-in the app renewed, and merges the profile chosen into
// the user's data, as the first import did, without its addons.
func (s *Service) importConnection(ctx context.Context, key connectionKey) {
	c, err := s.connection(ctx, key.user, key.app)
	if err != nil {
		if !errors.Is(err, ErrNotConnected) && ctx.Err() == nil {
			s.logger.Warn("An app connection could not be read", "app", key.app, "error", err)
		}
		return
	}
	if c.Problem != "" {
		return
	}
	account, renewed, err := s.sources[key.app].Reread(ctx, c.signIn)
	if renewed.Secret == "" {
		renewed = c.signIn
	}
	renewed.Owned, renewed.Profile = c.signIn.Owned, c.signIn.Profile
	if renewed != c.signIn || account.Name != "" && account.Name != c.Account {
		if err := s.renew(key, c.ConnectedAt, renewed, account.Name); err != nil {
			s.logger.Warn("The renewed sign-in of an app connection could not be saved", "app", key.app, "error", err)
		}
	}
	if ctx.Err() != nil {
		// Disconnected, replaced, or the server stops.
		return
	}
	result := ConnectionImport{Problem: readProblem(err)}
	var profile *Profile
	if err == nil {
		for i := range account.Profiles {
			if account.Profiles[i].ID == c.signIn.Profile {
				profile = &account.Profiles[i]
			}
		}
		if profile == nil {
			result.Problem = ProblemProfileMissing
		}
	}
	if result.Problem != "" {
		s.logger.Info("An app connection could not be read: it is imported again later", "user_id", key.user.String(), "app", key.app,
			"problem", result.Problem)
	}
	if profile != nil {
		status := &Status{App: key.app, Profile: profile.Name, User: key.user, State: StateRunning, Read: len(profile.Entries)}
		r := &run{s: s, status: status, user: accounts.User{ID: key.user}, entries: profile.Entries}
		if err := r.merge(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			s.logger.Warn("An app connection could not be imported", "app", key.app, "user_id", key.user.String(), "error", err)
			result.Problem = ProblemInternal
		}
		s.mu.Lock()
		result.Read, result.Played, result.Resumed, result.Favorites, result.Unmatched = status.Read, status.Played, status.Resumed,
			status.Favorites, status.UnmatchedCount
		s.mu.Unlock()
	}
	s.record(key, c.ConnectedAt, result)
}

// readProblem names why a source could not read an account again.
func readProblem(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrSignInRefused):
		return ProblemSignInRefused
	case errors.Is(err, ErrAppUnreachable):
		return ProblemUnreachable
	case errors.Is(err, ErrAppRateLimited):
		return ProblemRateLimited
	}
	return ProblemUnexpected
}

// renew saves the sign-in the app renewed of the connection key names
// made at connectedAt, and the account's name when the app gave one.
// Saved at once: the app refuses the one it replaced.
func (s *Service) renew(key connectionKey, connectedAt time.Time, signIn SignIn, account string) error {
	sealed, err := s.seal(signIn)
	if err != nil {
		return err
	}
	// The server stopping does not lose it.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = s.db.Exec(ctx, `UPDATE app_connections SET sign_in = $4, account = CASE WHEN $5 = '' THEN account ELSE $5 END
		WHERE user_id = $1 AND app = $2 AND connected_at = $3`, key.user, key.app, connectedAt, sealed, account)
	return err
}

// record saves how an import of the connection key names, made at
// connectedAt, went: a sign-in refused stops the connection.
func (s *Service) record(key connectionKey, connectedAt time.Time, result ConnectionImport) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result.At = s.now().UTC()
	var problem *string
	if result.Problem != "" {
		problem = &result.Problem
	}
	if _, err := s.db.Exec(ctx, `UPDATE app_connections SET last_import_at = $4, read = $5, played = $6, resumed = $7, favorites = $8,
			unmatched = $9, last_problem = $10, problem = CASE WHEN $10 = 'sign_in_refused' THEN 'sign_in_refused' ELSE problem END
		WHERE user_id = $1 AND app = $2 AND connected_at = $3`, key.user, key.app, connectedAt, result.At, result.Read, result.Played,
		result.Resumed, result.Favorites, result.Unmatched, problem); err != nil {
		s.logger.Warn("An app connection's import could not be recorded", "app", key.app, "error", err)
	}
	if result.Problem == ProblemSignInRefused {
		s.logger.Info("An app refused the sign-in of a connection: it stops until the user signs in again", "user_id", key.user.String(),
			"app", key.app)
	}
}
