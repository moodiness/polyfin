package trackers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/secrets"
)

// maxKey is the longest API key accepted, in bytes.
const maxKey = 256

// simklScope is the access Polyfin asks Simkl for: writing what users
// watch.
const simklScope = "media:read media:write"

// connection is a user's connection to a service, its token and refresh
// token opened.
type connection struct {
	token, refresh string
	// stored is the token as the database holds it, sealed or not.
	stored      string
	expires     *time.Time
	account     *string
	connectedAt time.Time
	lastSent    *time.Time
	problem     string
}

const connectionColumns = "token, refresh_token, expires_at, account, connected_at, last_sent_at, coalesce(problem, '')"

// scanConnection reads a connection and opens its tokens:
// secrets.ErrUnreadable when the key cannot.
func (s *Service) scanConnection(row pgx.Row) (connection, error) {
	var c connection
	if err := row.Scan(&c.stored, &c.refresh, &c.expires, &c.account, &c.connectedAt, &c.lastSent, &c.problem); err != nil {
		return c, err
	}
	var err error
	if c.token, err = s.box.Open(c.stored); err == nil {
		c.refresh, err = s.box.Open(c.refresh)
	}
	return c, err
}

// connection loads the connection of user to service, and reports whether
// there is one. A connection whose tokens the key cannot open counts as
// none: nothing is sent with it until the user connects again or the key
// is corrected.
func (s *Service) connection(ctx context.Context, user accounts.ID, service string) (connection, bool, error) {
	c, err := s.scanConnection(s.db.QueryRow(ctx, "SELECT "+connectionColumns+" FROM tracking_connections WHERE user_id = $1 AND service = $2",
		user, service))
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, secrets.ErrUnreadable) {
		return connection{}, false, nil
	}
	return c, err == nil, err
}

// connect saves a new connection of user to service, its tokens sealed, in
// place of the one there was and what it left to send.
func (s *Service) connect(ctx context.Context, user accounts.ID, service string, c connection) error {
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "DELETE FROM tracking_connections WHERE user_id = $1 AND service = $2", user, service); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO tracking_connections (user_id, service, token, refresh_token, expires_at, account, connected_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, user, service, s.box.Seal(c.token), s.box.Seal(c.refresh), c.expires, c.account, c.connectedAt)
		return err
	})
	if err != nil {
		return err
	}
	s.clearLane(laneKey{user, service})
	s.logger.Info("A user connected a tracking service", "user_id", user.String(), "service", service)
	return nil
}

// ErrNotConnected reports a service the user has no connection to, or
// none the key can open.
var ErrNotConnected = errors.New("tracking service not connected")

// Key returns the API key or user token user connected a key service with,
// for them to read it again. The tokens and session keys of code and
// sign-in services are never handed out: ErrUnknownService.
func (s *Service) Key(ctx context.Context, user accounts.ID, service string) (string, error) {
	if !ByKey(service) {
		return "", ErrUnknownService
	}
	c, ok, err := s.connection(ctx, user, service)
	switch {
	case err != nil:
		return "", err
	case !ok:
		return "", ErrNotConnected
	}
	return c.token, nil
}

// Status is how a user's connection to a service stands.
type Status struct {
	Service string
	// ByCode tells that the user connects by entering a code on the
	// service's site, and BySignIn by signing in there and allowing
	// Polyfin, rather than by pasting an API key.
	ByCode, BySignIn bool
	// Music tells that the service is told the songs the user plays rather
	// than the movies and episodes they watch; it has no history to import.
	Music bool
	// Available is false for a code or sign-in service whose app the
	// settings lack.
	Available bool
	Connected bool
	// Account is the account name the service gave, empty when unknown.
	Account     string
	ConnectedAt *time.Time
	// LastSentAt is the last time the service accepted a change.
	LastSentAt *time.Time
	// Problem is empty, ProblemReconnect or ProblemUnreachable.
	Problem string
	// Code is the code the user is asked to enter, or the page they are
	// asked to allow Polyfin on, while one waits.
	Code *Code
	// ImportHistory tells that the service's watch history is imported
	// into Polyfin; Importing, that an import runs now; LastImport, how
	// the last one went, nil before the first.
	ImportHistory bool
	Importing     bool
	LastImport    *ImportResult
}

// Code is a code waiting for a user to enter it on a service's site, or,
// for a sign-in service, the page of its site waiting for them to allow
// Polyfin, UserCode empty.
type Code struct {
	UserCode, VerificationURL string
	ExpiresAt                 time.Time
}

// Statuses lists how user's connections stand, for every service in the
// order of Services.
func (s *Service) Statuses(ctx context.Context, user accounts.ID) ([]Status, error) {
	statuses := make([]Status, 0, len(Services))
	for _, service := range Services {
		status, err := s.status(ctx, user, service)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func (s *Service) status(ctx context.Context, user accounts.ID, service string) (Status, error) {
	status := Status{Service: service, ByCode: ByCode(service), BySignIn: BySignIn(service), Music: Music(service),
		Available: Available(service, s.settings())}
	c, ok, err := s.connection(ctx, user, service)
	if err != nil {
		return status, err
	}
	if ok {
		status.Connected, status.Problem = true, c.problem
		status.ConnectedAt, status.LastSentAt = &c.connectedAt, c.lastSent
		if c.account != nil {
			status.Account = *c.account
		}
	}
	s.mu.Lock()
	key := laneKey{user, service}
	if p := s.pending[key]; p != nil {
		status.Code = &Code{UserCode: p.userCode, VerificationURL: p.verificationURL, ExpiresAt: p.expiresAt}
	}
	if refused, ok := s.refusedApps[key]; ok {
		if refused == appCredentials(service, s.settings()) {
			status.Problem = ProblemAppRefused
		} else {
			// The app's settings changed since.
			delete(s.refusedApps, key)
		}
	}
	s.mu.Unlock()
	if ok && imports(service) {
		if err := s.importStatus(ctx, key, &status); err != nil {
			return status, err
		}
	}
	return status, nil
}

// validKey reports whether key may be an API key: printable ASCII without
// spaces, up to maxKey bytes.
func validKey(key string) bool {
	if key == "" || len(key) > maxKey {
		return false
	}
	for i := range len(key) {
		if key[i] <= ' ' || key[i] > '~' {
			return false
		}
	}
	return true
}

// ConnectKey connects user to a key service with their API key or user
// token, once the service accepted it, in place of the connection there
// was.
func (s *Service) ConnectKey(ctx context.Context, user accounts.ID, service, key string) (Status, error) {
	if !ByKey(service) {
		return Status{}, ErrUnknownService
	}
	key = strings.TrimSpace(key)
	if !validKey(key) {
		return Status{}, ErrInvalidKey
	}
	account, err := s.checkKey(ctx, service, key)
	if err != nil {
		return Status{}, err
	}
	c := connection{token: key, connectedAt: s.now().UTC().Truncate(time.Second)}
	if account != "" {
		c.account = &account
	}
	if err := s.connect(ctx, user, service, c); err != nil {
		return Status{}, err
	}
	return s.status(ctx, user, service)
}

// pending is a code connection waiting for the user.
type pending struct {
	user                      accounts.ID
	service                   string
	deviceCode                string
	userCode, verificationURL string
	expiresAt                 time.Time
	interval                  time.Duration
	done                      chan struct{}
	once                      sync.Once
}

func (p *pending) cancel() {
	p.once.Do(func() { close(p.done) })
}

// codeAnswer is the answer of Trakt and Simkl to a request for a code.
type codeAnswer struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURL         string `json:"verification_url"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// StartCode asks a code service for a code the user enters on its site, or
// a sign-in service for the page where the user allows Polyfin, in place of
// the one waiting, and waits for them in the background. Their connection,
// if any, stays until they enter it.
func (s *Service) StartCode(ctx context.Context, user accounts.ID, service string) (Status, error) {
	if !ByCode(service) && !BySignIn(service) {
		return Status{}, ErrUnknownService
	}
	settings := s.settings()
	if !Available(service, settings) {
		return Status{}, ErrNotAvailable
	}
	// A new attempt forgets that the app was refused.
	key := laneKey{user, service}
	s.mu.Lock()
	delete(s.refusedApps, key)
	s.mu.Unlock()
	if service == LastFM {
		p, err := s.lastFMToken(ctx, user, settings)
		if err != nil {
			return Status{}, err
		}
		return s.wait(ctx, key, p)
	}
	var (
		r   reply
		err error
	)
	if service == Trakt {
		r, err = s.api(ctx, Trakt, "", settings, http.MethodPost, "/oauth/device/code", nil, map[string]string{"client_id": settings.TraktClientID})
	} else {
		r, err = s.form(ctx, "/oauth2/device", url.Values{"client_id": {settings.SimklClientID}, "scope": {simklScope}})
	}
	switch {
	case err == nil && refusesApp(r):
		return Status{}, ErrAppRefused
	case err != nil || r.status != http.StatusOK:
		return Status{}, ErrUnreachable
	}
	var answer codeAnswer
	if json.Unmarshal(r.body, &answer) != nil || answer.DeviceCode == "" || answer.UserCode == "" {
		return Status{}, ErrUnreachable
	}
	p := &pending{
		user:            user,
		service:         service,
		deviceCode:      answer.DeviceCode,
		userCode:        answer.UserCode,
		verificationURL: answer.VerificationURL,
		expiresAt:       s.now().UTC().Add(time.Duration(answer.ExpiresIn) * time.Second).Truncate(time.Second),
		interval:        time.Duration(max(answer.Interval, 1)) * s.timing.pollUnit,
		done:            make(chan struct{}),
	}
	if service == Simkl {
		// The complete address fills the code in.
		p.verificationURL = answer.VerificationURIComplete
		if p.verificationURL == "" {
			p.verificationURL = answer.VerificationURI
		}
	}
	return s.wait(ctx, key, p)
}

// wait has p wait for its user in place of the code waiting for key, and
// returns the status of the user's connection with p: the first poll,
// which may end p at once, starts once the status shows p.
func (s *Service) wait(ctx context.Context, key laneKey, p *pending) (Status, error) {
	s.mu.Lock()
	if old := s.pending[key]; old != nil {
		old.cancel()
	}
	s.pending[key] = p
	s.mu.Unlock()
	status, err := s.status(ctx, p.user, p.service)
	s.mu.Lock()
	s.spawn(func() { s.poll(p) })
	s.mu.Unlock()
	return status, err
}

// pollState is what a poll of a code tells.
type pollState int

const (
	waiting pollState = iota
	slowDown
	approved
	ended
	// appRefused: the service refused the server's app.
	appRefused
)

// tokens are what Trakt and Simkl answer once a user lets Polyfin in, or
// to a refresh; for Last.fm, the session key and the account it is for.
type tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	CreatedAt    int64  `json:"created_at"`
	Scope        string `json:"scope"`
	account      string
}

// expiry is when the access token of t expires, nil when not said.
func (s *Service) expiry(t tokens) *time.Time {
	if t.ExpiresIn <= 0 {
		return nil
	}
	issued := s.now()
	if t.CreatedAt > 0 {
		issued = time.Unix(t.CreatedAt, 0)
	}
	expires := issued.Add(time.Duration(t.ExpiresIn) * time.Second).UTC()
	return &expires
}

// poll waits for the user to enter the code of p, at the pace the service
// asks, until they do, refuse, or the code expires.
func (s *Service) poll(p *pending) {
	defer func() {
		s.mu.Lock()
		if s.pending[laneKey{p.user, p.service}] == p {
			delete(s.pending, laneKey{p.user, p.service})
		}
		s.mu.Unlock()
	}()
	for {
		timer := time.NewTimer(p.interval)
		select {
		case <-s.ctx.Done():
			timer.Stop()
			return
		case <-p.done:
			timer.Stop()
			return
		case <-timer.C:
		}
		if !s.now().Before(p.expiresAt) {
			return
		}
		state, t := s.pollCode(p)
		switch state {
		case slowDown:
			p.interval += 5 * s.timing.pollUnit
		case ended:
			return
		case appRefused:
			key := laneKey{p.user, p.service}
			s.mu.Lock()
			s.refusedApps[key] = appCredentials(p.service, s.settings())
			// The code ends with the refusal, in the same step: a status in
			// between would show the code with the refused app.
			if s.pending[key] == p {
				delete(s.pending, key)
			}
			s.mu.Unlock()
			s.logger.Info("A tracking service refused this server's app: its credentials need checking under Settings › Tracking",
				"user_id", p.user.String(), "service", p.service)
			return
		case approved:
			select {
			case <-p.done:
				// Disconnected or started again meanwhile.
				return
			default:
			}
			c := connection{token: t.AccessToken, refresh: t.RefreshToken, expires: s.expiry(t), connectedAt: s.now().UTC().Truncate(time.Second)}
			account := t.account
			if ByCode(p.service) {
				account = s.account(p.service, t.AccessToken)
			}
			if account != "" {
				c.account = &account
			}
			if err := s.connect(s.ctx, p.user, p.service, c); err != nil {
				s.logger.Warn("A tracking connection could not be saved", "service", p.service, "error", err)
			}
			return
		}
	}
}

// pollCode asks the service of p whether its user entered the code.
func (s *Service) pollCode(p *pending) (pollState, tokens) {
	settings := s.settings()
	if p.service == LastFM {
		return s.pollLastFM(p, settings)
	}
	var (
		r   reply
		err error
	)
	if p.service == Trakt {
		r, err = s.api(s.ctx, Trakt, "", settings, http.MethodPost, "/oauth/device/token", nil, map[string]string{
			"code": p.deviceCode, "client_id": settings.TraktClientID, "client_secret": settings.TraktClientSecret})
	} else {
		r, err = s.form(s.ctx, "/oauth2/token", url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"},
			"client_id": {settings.SimklClientID}, "device_code": {p.deviceCode}})
	}
	if err != nil {
		return waiting, tokens{}
	}
	var t tokens
	switch {
	case r.status == http.StatusOK:
		if json.Unmarshal(r.body, &t) != nil || t.AccessToken == "" {
			return ended, t
		}
		if p.service == Simkl && !strings.Contains(t.Scope, "media:write") {
			// A token that cannot write is no use.
			s.logger.Info("Simkl gave read-only access: the user must connect again", "user_id", p.user.String())
			return ended, t
		}
		return approved, t
	case refusesApp(r):
		return appRefused, t
	case r.status == http.StatusTooManyRequests:
		return slowDown, t
	case p.service == Trakt && r.status == http.StatusBadRequest:
		// Trakt: pending.
		return waiting, t
	case p.service == Trakt && r.status >= 500:
		return waiting, t
	case p.service == Trakt:
		// Expired (410), denied (418), unknown (404) or already used (409).
		return ended, t
	}
	switch errorCode(r.body) {
	case "authorization_pending":
		return waiting, t
	case "slow_down":
		return slowDown, t
	}
	if r.status >= 500 {
		return waiting, t
	}
	// Simkl: expired, or the grant was refused.
	return ended, t
}

// refusesApp reports whether r refuses the app the settings name rather
// than the user or the code: Trakt and Simkl answer 401 invalid_client to
// an unknown client ID or a wrong secret.
func refusesApp(r reply) bool {
	return r.status == http.StatusUnauthorized || r.status == http.StatusForbidden || errorCode(r.body) == "invalid_client"
}

// appCredentials are the credentials of the app the settings name for
// service.
func appCredentials(service string, settings accounts.Settings) string {
	switch service {
	case Trakt:
		return settings.TraktClientID + "\x00" + settings.TraktClientSecret
	case LastFM:
		return settings.LastFMAPIKey + "\x00" + settings.LastFMSecret
	}
	return settings.SimklClientID
}

// account asks the service the name of the account token is for, empty
// when it does not answer.
func (s *Service) account(service, token string) string {
	r, err := s.api(s.ctx, service, token, s.settings(), http.MethodGet, "/users/settings", nil, nil)
	if err != nil || r.status != http.StatusOK {
		return ""
	}
	var answer struct {
		User struct {
			Username string `json:"username"`
			Name     string `json:"name"`
		} `json:"user"`
	}
	_ = json.Unmarshal(r.body, &answer)
	if service == Trakt {
		return answer.User.Username
	}
	return answer.User.Name
}

// Disconnect forgets user's connection to service and what it left to
// send, revoking the token where the service allows, and cancels a code
// waiting.
func (s *Service) Disconnect(ctx context.Context, user accounts.ID, service string) error {
	if !known(service) {
		return ErrUnknownService
	}
	key := laneKey{user, service}
	s.mu.Lock()
	if p := s.pending[key]; p != nil {
		p.cancel()
		delete(s.pending, key)
	}
	s.mu.Unlock()
	// Its imports stop; what they imported stays.
	if err := s.forgetImport(ctx, key); err != nil {
		return err
	}
	var token, refresh string
	err := s.db.QueryRow(ctx, "DELETE FROM tracking_connections WHERE user_id = $1 AND service = $2 RETURNING token, refresh_token",
		user, service).Scan(&token, &refresh)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	// Tokens the key cannot open are forgotten without being revoked.
	token, err = s.box.Open(token)
	if err == nil {
		refresh, err = s.box.Open(refresh)
	}
	revocable := err == nil
	s.clearLane(key)
	s.logger.Info("A user disconnected a tracking service", "user_id", user.String(), "service", service)
	if ByCode(service) && revocable {
		s.mu.Lock()
		s.spawn(func() { s.revoke(service, token, refresh) })
		s.mu.Unlock()
	}
	return nil
}

// revoke tells the service a token is no longer used.
func (s *Service) revoke(service, token, refresh string) {
	settings := s.settings()
	if !Available(service, settings) {
		return
	}
	if service == Trakt {
		_, _ = s.api(s.ctx, Trakt, "", settings, http.MethodPost, "/oauth/revoke", nil, map[string]string{
			"token": token, "client_id": settings.TraktClientID, "client_secret": settings.TraktClientSecret})
		return
	}
	// Revoking either token of Simkl revokes both.
	if refresh != "" {
		token = refresh
	}
	_, _ = s.form(s.ctx, "/oauth2/revoke", url.Values{"client_id": {settings.SimklClientID}, "token": {token}})
}

// errTokenRefused reports a refresh token the service refused.
var errTokenRefused = errors.New("refresh token refused")

// refreshLocks keeps one refresh of a connection at a time: Trakt's
// refresh tokens work once.
type refreshLocks struct {
	mu    sync.Mutex
	locks map[laneKey]*sync.Mutex
}

func (r *refreshLocks) lock(key laneKey) func() {
	r.mu.Lock()
	if r.locks == nil {
		r.locks = map[laneKey]*sync.Mutex{}
	}
	l := r.locks[key]
	if l == nil {
		l = &sync.Mutex{}
		r.locks[key] = l
	}
	r.mu.Unlock()
	l.Lock()
	return l.Unlock
}

// fresh returns the connection of key with a token that does not expire
// soon, refreshing it if needed, or if it is refused, the token the
// service refused.
func (s *Service) fresh(ctx context.Context, key laneKey, refusedToken string) (connection, error) {
	defer s.refreshes.lock(key)()
	c, ok, err := s.connection(ctx, key.user, key.service)
	switch {
	case err != nil:
		return c, err
	case !ok:
		// Disconnected meanwhile, or tokens the key cannot open, which
		// are not the service's refusal.
		return c, ErrNotConnected
	case c.problem == ProblemReconnect:
		return c, errTokenRefused
	}
	stale := refusedToken != "" && c.token == refusedToken
	if !stale && (c.expires == nil || c.expires.Sub(s.now()) > s.timing.refreshAhead) {
		return c, nil
	}
	if c.refresh == "" {
		return c, errTokenRefused
	}
	settings := s.settings()
	var r reply
	if key.service == Trakt {
		r, err = s.api(ctx, Trakt, "", settings, http.MethodPost, "/oauth/token", nil, map[string]string{
			"refresh_token": c.refresh, "client_id": settings.TraktClientID, "client_secret": settings.TraktClientSecret,
			"redirect_uri": traktRedirect, "grant_type": "refresh_token"})
	} else {
		r, err = s.form(ctx, "/oauth2/token", url.Values{"grant_type": {"refresh_token"}, "client_id": {settings.SimklClientID},
			"refresh_token": {c.refresh}})
	}
	switch {
	case err != nil:
		return c, err
	case r.status == http.StatusBadRequest || r.status == http.StatusUnauthorized:
		return c, errTokenRefused
	case r.status != http.StatusOK:
		return c, errors.New("refresh failed with status " + http.StatusText(r.status))
	}
	var t tokens
	if json.Unmarshal(r.body, &t) != nil || t.AccessToken == "" {
		return c, errors.New("malformed refresh answer")
	}
	if t.RefreshToken == "" {
		t.RefreshToken = c.refresh
	}
	if _, err := s.db.Exec(ctx, `UPDATE tracking_connections SET token = $4, refresh_token = $5, expires_at = $6
		WHERE user_id = $1 AND service = $2 AND token = $3`, key.user, key.service, c.stored,
		s.box.Seal(t.AccessToken), s.box.Seal(t.RefreshToken), s.expiry(t)); err != nil {
		return c, err
	}
	c.token, c.refresh, c.expires = t.AccessToken, t.RefreshToken, s.expiry(t)
	return c, nil
}

// refreshDue refreshes the tokens that expire soon, so that a user who
// watches nothing for a while stays connected.
func (s *Service) refreshDue() {
	rows, err := s.db.Query(s.ctx, `SELECT user_id, service FROM tracking_connections
		WHERE refresh_token <> '' AND problem IS DISTINCT FROM 'reconnect' AND expires_at < $1`, s.now().Add(s.timing.refreshAhead))
	if err != nil {
		return
	}
	due, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (laneKey, error) {
		var key laneKey
		return key, row.Scan(&key.user, &key.service)
	})
	if err != nil {
		return
	}
	for _, key := range due {
		if !Available(key.service, s.settings()) {
			continue
		}
		if _, err := s.fresh(s.ctx, key, ""); errors.Is(err, errTokenRefused) {
			s.refusedToken(s.ctx, key)
		}
	}
}
