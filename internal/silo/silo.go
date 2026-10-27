// Package silo serves Silo's API, under /api/v2, to Silo's apps for
// iPhone, iPad, Apple TV, Mac and Android, while the settings allow them.
//
// Silo's apps speak Silo's REST API alone, published as an OpenAPI
// contract by Silo-Server/silo-server; Polyfin answers the operations its
// apps need to sign in, browse and play, written from the contract's facts
// (see docs/silo-apps.md for the commit followed). Each answer is made of
// the Jellyfin API's answer for the same user and device, asked of the
// Jellyfin API's handler in process: what Silo's apps see and play is what
// Jellyfin apps get, under the same limits (parental control, blocked
// genres, visible libraries, hidden titles, allowed hours, disabled and
// ended accounts, version rules and quality groups, conversion
// permissions, sign-in throttling, two-step sign-in), and their sessions,
// played state and resume points are the Jellyfin apps' too.
package silo

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/forwarded"
	"github.com/moodiness/polyfin/internal/quickconnect"
)

// Accounts is what the API reads and does of the accounts.
type Accounts interface {
	Settings() accounts.Settings
	// DeviceByToken is the signed-in device of an access token, and its
	// user; accounts.ErrNotFound for none, or one of a disabled or ended
	// account.
	DeviceByToken(ctx context.Context, token, remoteAddress string) (accounts.Device, accounts.User, error)
	// SetupRequired reports whether no administrator exists yet.
	SetupRequired(ctx context.Context) (bool, error)
	// SignOutDevice signs out the device of an access token.
	SignOutDevice(ctx context.Context, token string) error
}

// Options configure the API.
type Options struct {
	ServerID string
	Version  string
	Accounts Accounts
	// Jellyfin is the Jellyfin API's handler: it answers every operation
	// for the apps' sessions, and every request while the settings do not
	// allow Silo's apps, as it did before.
	Jellyfin http.Handler
	DB       *pgxpool.Pool
	// QuickConnect holds the requests TVs sign in with a code through
	// (see deviceLoginRoutes).
	QuickConnect *quickconnect.Store
	Logger       *slog.Logger

	// now is the clock; time.Now by default.
	now func() time.Time
}

// Server serves Silo's API.
type Server struct {
	options  Options
	mux      *http.ServeMux
	numbers  *numbers
	sessions *sessions
	shuffles *shuffles
	unknown  *unknownOperations
}

// New returns the API's handler.
func New(options Options) *Server {
	if options.now == nil {
		options.now = time.Now
	}
	s := &Server{options: options, mux: http.NewServeMux(), numbers: newNumbers(options.DB), sessions: newSessions(),
		shuffles: newShuffles(), unknown: newUnknownOperations(options.Logger, options.now)}
	s.accountRoutes()
	s.browseRoutes()
	s.filterRoutes()
	s.libraryRoutes()
	s.playbackRoutes()
	s.collectionRoutes()
	s.personalRoutes()
	s.peopleRoutes()
	s.discoverRoutes()
	s.probeRoutes()
	s.shuffleRoutes()
	s.deviceLoginRoutes()
	s.mux.HandleFunc("/api/v2/", func(w http.ResponseWriter, r *http.Request) {
		s.unknown.report(r)
		problem(w, http.StatusNotFound, "not_found", "This operation does not exist on this server.")
	})
	return s
}

// ServeHTTP serves Silo's API while the settings allow Silo's apps. Off,
// the Jellyfin API's handler answers, which knows nothing under /api/v2:
// the API answers nothing.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.options.Accounts.Settings().SiloApps {
		s.options.Jellyfin.ServeHTTP(w, r)
		return
	}
	s.mux.ServeHTTP(w, r)
}

// viewer is the signed-in session of a request: its user, its device and
// the token both the apps and the Jellyfin API know it by.
type viewer struct {
	user   accounts.User
	device accounts.Device
	token  string
}

// bearer is the token of a request's Authorization header.
func bearer(r *http.Request) string {
	scheme, token, ok := strings.Cut(strings.TrimSpace(r.Header.Get("Authorization")), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

// signedIn serves next for the session of the request's bearer token, a
// session of an enabled account: one disabled or ended since is signed
// out, and a user outside their allowed hours is refused, except
// administrators, as Jellyfin apps are.
func (s *Server) signedIn(next func(http.ResponseWriter, *http.Request, viewer)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, ok := s.session(w, r, true)
		if ok {
			next(w, s.withWatchlist(r, v), v)
		}
	}
}

// signedInAnyHour is signedIn whatever the user's allowed hours: signing
// out is never refused.
func (s *Server) signedInAnyHour(next func(http.ResponseWriter, *http.Request, viewer)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, ok := s.session(w, r, false)
		if ok {
			next(w, r, v)
		}
	}
}

// session finds the session of the request's bearer token, answering the
// request when there is none, or when hours holds and the user is outside
// their allowed hours.
func (s *Server) session(w http.ResponseWriter, r *http.Request, hours bool) (viewer, bool) {
	token := bearer(r)
	if token == "" {
		problem(w, http.StatusUnauthorized, "authentication_required", "Sign in first.")
		return viewer{}, false
	}
	device, user, err := s.options.Accounts.DeviceByToken(r.Context(), token, forwarded.Client(r))
	if errors.Is(err, accounts.ErrNotFound) {
		problem(w, http.StatusUnauthorized, "invalid_token", "The session ended.")
		return viewer{}, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return viewer{}, false
	}
	if hours && !user.IsAdministrator && !user.AllowedAt(s.options.now()) {
		problem(w, http.StatusForbidden, "permission_denied", "This account may not be used at this hour.")
		return viewer{}, false
	}
	return viewer{user: user, device: device, token: token}, true
}

// profiled serves next for a session whose request names its profile, the
// one Polyfin gives each user: its ID is the user's.
func (s *Server) profiled(next func(http.ResponseWriter, *http.Request, viewer)) http.HandlerFunc {
	return s.signedIn(func(w http.ResponseWriter, r *http.Request, v viewer) {
		if s.profileRefused(w, r, v, true) {
			return
		}
		next(w, r, v)
	})
}

// profileRefused answers a request whose X-Profile-Id names another
// profile than the user's, or none when required, and reports whether it
// did.
func (s *Server) profileRefused(w http.ResponseWriter, r *http.Request, v viewer, required bool) bool {
	profile := strings.TrimSpace(r.Header.Get("X-Profile-Id"))
	if profile == "" {
		if required {
			validationProblem(w, "header.x-profile-id", "required", "The X-Profile-Id header is required.")
		}
		return required
	}
	// The ID is the user's, in any of the forms accounts.ParseID reads.
	if id, err := accounts.ParseID(profile); err != nil || id != v.user.ID {
		problem(w, http.StatusForbidden, "permission_denied", "This profile is not the account's.")
		return true
	}
	return false
}

// problemJSON is an error answer, as RFC 9457 describes it.
type problemJSON struct {
	Type     string         `json:"type"`
	Title    string         `json:"title"`
	Status   int            `json:"status"`
	Detail   string         `json:"detail"`
	Instance string         `json:"instance"`
	Errors   []problemError `json:"errors,omitempty"`
}

type problemError struct {
	Location string `json:"location"`
	Code     string `json:"code"`
	Detail   string `json:"detail"`
}

// problemTypes is where the contract's problem types are named: the apps
// read the code from the last segment of the type.
const problemTypes = "https://siloserver.org/docs/api/v2/problems/"

// problem answers an error. Its instance names the request, whose ID the
// X-Request-ID header gives too.
func problem(w http.ResponseWriter, status int, code, detail string, errs ...problemError) {
	random := make([]byte, 12)
	_, _ = rand.Read(random)
	id := hex.EncodeToString(random)
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Request-ID", id)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problemJSON{Type: problemTypes + code, Title: http.StatusText(status), Status: status,
		Detail: detail, Instance: "urn:silo:request:" + id, Errors: errs})
}

func validationProblem(w http.ResponseWriter, location, code, detail string) {
	problem(w, http.StatusUnprocessableEntity, "validation_failed", "The request did not pass validation; see errors.",
		problemError{Location: location, Code: code, Detail: detail})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// maxBody bounds the JSON bodies the apps send: a playback start, with
// the client's capabilities, is the largest.
const maxBody = 1 << 20

// decode reads a request's JSON body into into, answering the request
// when it cannot.
func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	data, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil {
		problem(w, http.StatusBadRequest, "validation_failed", "The body could not be read.")
		return false
	}
	if len(data) > maxBody {
		problem(w, http.StatusRequestEntityTooLarge, "payload_too_large", "The body is too large.")
		return false
	}
	if err := json.Unmarshal(data, into); err != nil {
		validationProblem(w, "body", "invalid", "The body is not the expected JSON.")
		return false
	}
	return true
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	if r.Context().Err() != nil {
		return
	}
	s.options.Logger.Error("A request of Silo's apps failed", "method", r.Method, "path", r.URL.Path, "error", err)
	problem(w, http.StatusInternalServerError, "internal_error", "The server could not answer.")
}

// jellyfinRefusal answers what the Jellyfin API refused, as Silo's API
// answers it: a session it no longer knows, a user outside their hours,
// an item they may not see.
func (s *Server) jellyfinRefusal(w http.ResponseWriter, r *http.Request, answer answer) {
	switch {
	case answer.status == http.StatusUnauthorized && len(answer.body) == 0:
		problem(w, http.StatusUnauthorized, "invalid_token", "The session ended.")
	case answer.status == http.StatusForbidden:
		problem(w, http.StatusForbidden, "permission_denied", "This account may not do this now.")
	case answer.status == http.StatusTooManyRequests:
		w.Header().Set("Retry-After", cmp.Or(answer.header.Get("Retry-After"), "60"))
		problem(w, http.StatusTooManyRequests, "rate_limited", "Too many attempts; try again later.")
	case answer.status == http.StatusBadGateway || answer.status == http.StatusServiceUnavailable || answer.status == http.StatusGatewayTimeout:
		// A source failed, or the server is busy: the app may try again.
		w.Header().Set("Retry-After", cmp.Or(answer.header.Get("Retry-After"), "30"))
		problem(w, http.StatusServiceUnavailable, "unavailable", "A source of this server does not answer; try again later.")
	case answer.status >= http.StatusInternalServerError:
		s.internalError(w, r, errors.New("the Jellyfin API answered "+http.StatusText(answer.status)))
	default:
		// Unknown items, items the user may not see, and libraries hidden
		// from them are all unknown to the apps.
		problem(w, http.StatusNotFound, "not_found", "Not found.")
	}
}
