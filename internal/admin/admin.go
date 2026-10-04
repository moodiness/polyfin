// Package admin serves the JSON API of the admin interface.
package admin

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/quickconnect"
	"github.com/moodiness/polyfin/internal/throttle"
)

const (
	cookieName = "polyfin_session"
	cookiePath = "/admin"
	maxBody    = 64 << 10
	readyWait  = 2 * time.Second
)

// Pinger reports whether the database answers.
type Pinger interface {
	Ping(context.Context) error
}

// Options are the dependencies of the admin API.
type Options struct {
	Version      string
	ServerID     string
	Database     Pinger
	Accounts     *accounts.Store
	Addons       *addons.Store
	QuickConnect *quickconnect.Store
	SignIns      *throttle.Failures
	// SetupCode authorizes creating the first administrator. It is printed
	// in the server log at startup while no administrator exists.
	SetupCode string
	Logger    *slog.Logger
	// Now tells the time users' allowed hours are checked against; nil
	// means the system clock.
	Now func() time.Time
}

type handler struct {
	Options
	now func() time.Time
}

// New returns the handler of every /admin/api/ route.
func New(options Options) http.Handler {
	h := &handler{Options: options, now: time.Now}
	if options.Now != nil {
		h.now = options.Now
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/api/status", h.status)
	mux.HandleFunc("POST /admin/api/setup", h.setup)
	mux.HandleFunc("POST /admin/api/session", h.signIn)

	mux.Handle("GET /admin/api/session", h.signedInAnyHour(h.session))
	mux.Handle("DELETE /admin/api/session", h.signedInAnyHour(h.signOut))
	mux.Handle("PUT /admin/api/account/password", h.signedIn(h.changePassword))
	mux.Handle("GET /admin/api/account/devices", h.signedIn(h.ownDevices))
	mux.Handle("DELETE /admin/api/account/devices/{id}", h.signedIn(h.revokeOwnDevice))
	mux.Handle("GET /admin/api/quick-connect/{code}", h.signedIn(h.quickConnectRequest))
	mux.Handle("POST /admin/api/quick-connect", h.signedIn(h.quickConnectApprove))

	mux.Handle("GET /admin/api/users", h.administrator(h.users))
	mux.Handle("POST /admin/api/users", h.administrator(h.createUser))
	mux.Handle("PATCH /admin/api/users/{id}", h.administrator(h.updateUser))
	mux.Handle("DELETE /admin/api/users/{id}", h.administrator(h.deleteUser))
	mux.Handle("GET /admin/api/users/{id}/devices", h.administrator(h.userDevices))
	mux.Handle("DELETE /admin/api/users/{id}/devices/{deviceId}", h.administrator(h.revokeUserDevice))
	mux.Handle("GET /admin/api/parental-ratings", h.administrator(h.parentalRatings))
	mux.Handle("GET /admin/api/settings", h.administrator(h.settings))
	mux.Handle("PUT /admin/api/settings", h.administrator(h.updateSettings))
	mux.Handle("POST /admin/api/users/{id}/unblock", h.administrator(h.unblockUser))
	mux.Handle("GET /admin/api/user-content-choices", h.administrator(h.userContentChoices))

	mux.Handle("GET /admin/api/scopes/{scope}/addons", h.signedIn(h.listAddons))
	mux.Handle("POST /admin/api/scopes/{scope}/addons", h.signedIn(h.installAddon))
	mux.Handle("PUT /admin/api/scopes/{scope}/addons/order", h.signedIn(h.reorderAddons))
	mux.Handle("PATCH /admin/api/scopes/{scope}/addons/{id}", h.signedIn(h.updateAddon))
	mux.Handle("POST /admin/api/scopes/{scope}/addons/{id}/refresh", h.signedIn(h.refreshAddon))
	mux.Handle("DELETE /admin/api/scopes/{scope}/addons/{id}", h.signedIn(h.removeAddon))
	mux.Handle("GET /admin/api/scopes/{scope}/libraries", h.signedIn(h.listLibraries))
	mux.Handle("PUT /admin/api/scopes/{scope}/libraries", h.signedIn(h.saveLibraries))
	mux.Handle("GET /admin/api/account/addon-preferences", h.signedIn(h.addonPreferences))
	mux.Handle("PUT /admin/api/account/addon-preferences", h.signedIn(h.saveAddonPreferences))

	mux.HandleFunc("/admin/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found")
	})
	return sameOrigin(mux)
}

// NewSetupCode returns a random code formatted XXXX-XXXX, without letters
// and digits that are easy to confuse.
func NewSetupCode() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	code := make([]byte, 0, 9)
	for i := range 8 {
		if i == 4 {
			code = append(code, '-')
		}
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		code = append(code, alphabet[n.Int64()])
	}
	return string(code)
}

func normalizeSetupCode(code string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
}

// sameOrigin refuses state-changing requests sent by another site. The
// session cookie is SameSite=Strict as well; this also covers browsers
// that would send it.
func sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if origin := r.Header.Get("Origin"); origin != "" {
				parsed, err := url.Parse(origin)
				if err != nil || !strings.EqualFold(parsed.Host, r.Host) {
					writeError(w, http.StatusForbidden, "cross_origin")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(into); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return false
	}
	return true
}

func (h *handler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.Logger.Error("Admin API request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	writeError(w, http.StatusInternalServerError, "internal")
}

// accountError answers the client-facing account errors and reports
// whether err was one of them.
func accountError(w http.ResponseWriter, err error) bool {
	for _, known := range []struct {
		err    error
		status int
		code   string
	}{
		{accounts.ErrInvalidName, http.StatusBadRequest, "invalid_name"},
		{accounts.ErrInvalidPassword, http.StatusBadRequest, "invalid_password"},
		{accounts.ErrInvalidServerName, http.StatusBadRequest, "invalid_server_name"},
		{accounts.ErrInvalidLanguage, http.StatusBadRequest, "invalid_language"},
		{accounts.ErrInvalidCatalogLimit, http.StatusBadRequest, "invalid_catalog_limit"},
		{accounts.ErrInvalidChannelLimit, http.StatusBadRequest, "invalid_channel_limit"},
		{accounts.ErrInvalidPlayedPercent, http.StatusBadRequest, "invalid_played_percent"},
		{accounts.ErrInvalidResumePercent, http.StatusBadRequest, "invalid_resume_percent"},
		{accounts.ErrResumeNotBelowPlayed, http.StatusBadRequest, "resume_not_below_played"},
		{accounts.ErrInvalidVersionListMinutes, http.StatusBadRequest, "invalid_version_list_minutes"},
		{accounts.ErrInvalidCatalogRefreshMinutes, http.StatusBadRequest, "invalid_catalog_refresh_minutes"},
		{accounts.ErrInvalidLoginAttempts, http.StatusBadRequest, "invalid_login_attempts"},
		{accounts.ErrInvalidInactiveDeviceDays, http.StatusBadRequest, "invalid_inactive_device_days"},
		{accounts.ErrInvalidAnalysisTimeout, http.StatusBadRequest, "invalid_analysis_timeout"},
		{accounts.ErrInvalidVersionAttempts, http.StatusBadRequest, "invalid_version_attempts"},
		{accounts.ErrInvalidMaxConversions, http.StatusBadRequest, "invalid_max_conversions"},
		{accounts.ErrInvalidMaxConversionHeight, http.StatusBadRequest, "invalid_max_conversion_height"},
		{accounts.ErrInvalidParentalControl, http.StatusBadRequest, "invalid_parental_control"},
		{accounts.ErrInvalidMaxPlaybacks, http.StatusBadRequest, "invalid_max_playbacks"},
		{accounts.ErrInvalidMaxBitrate, http.StatusBadRequest, "invalid_max_bitrate"},
		{accounts.ErrInvalidSyncPlay, http.StatusBadRequest, "invalid_sync_play"},
		{accounts.ErrInvalidBlockedGenres, http.StatusBadRequest, "invalid_blocked_genres"},
		{accounts.ErrInvalidAccessSchedule, http.StatusBadRequest, "invalid_access_schedules"},
		{accounts.ErrNameTaken, http.StatusConflict, "name_taken"},
		{accounts.ErrLastAdministrator, http.StatusConflict, "last_administrator"},
		{accounts.ErrSetupComplete, http.StatusConflict, "setup_complete"},
		{accounts.ErrNotFound, http.StatusNotFound, "not_found"},
		{accounts.ErrWrongPassword, http.StatusForbidden, "wrong_password"},
		{accounts.ErrInvalidCredentials, http.StatusUnauthorized, "invalid_credentials"},
		{accounts.ErrDisabled, http.StatusForbidden, "account_disabled"},
	} {
		if errors.Is(err, known.err) {
			writeError(w, known.status, known.code)
			return true
		}
	}
	return false
}

// throttled answers 429 when the client exhausted its failed attempts.
func (h *handler) throttled(w http.ResponseWriter, key string) bool {
	allowed, wait := h.SignIns.Allowed(key)
	if allowed {
		return false
	}
	w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
	writeError(w, http.StatusTooManyRequests, "too_many_attempts")
	return true
}

func setupCodeMatches(expected, given string) bool {
	if expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(normalizeSetupCode(expected)), []byte(normalizeSetupCode(given))) == 1
}
