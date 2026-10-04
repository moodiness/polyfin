package jellyfin

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Server administration: API keys, devices, users, the activity log, the
// scheduled tasks, the server configuration, storage and logs, as
// Jellyfin's dashboard and the tools that sync from a server use them.
func (h *Handler) administrationRoutes(rt *router) {
	anonymous := func(method, pattern string, handler http.HandlerFunc) { rt.handle(method, pattern, handler) }
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(handler))
	}
	// Endpoints Jellyfin keeps to administrators (its RequiresElevation
	// policy) answer anyone else an empty 403, before reading the request.
	administrator := func(method, pattern string, handler http.HandlerFunc) {
		signedIn(method, pattern, func(w http.ResponseWriter, r *http.Request) {
			if !callerFrom(r.Context()).User.IsAdministrator {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			handler(w, r)
		})
	}

	administrator(http.MethodGet, "/Auth/Keys", h.apiKeys)
	administrator(http.MethodPost, "/Auth/Keys", h.createAPIKey)
	administrator(http.MethodDelete, "/Auth/Keys/{key}", h.revokeAPIKey)
	administrator(http.MethodGet, "/Auth/Providers", h.authenticationProviders)
	administrator(http.MethodGet, "/Auth/PasswordResetProviders", h.passwordResetProviders)

	administrator(http.MethodGet, "/Devices", h.devices)
	administrator(http.MethodDelete, "/Devices", h.deleteDevices)
	administrator(http.MethodGet, "/Devices/Info", h.deviceInfo)
	administrator(http.MethodGet, "/Devices/Options", h.deviceOptions)
	administrator(http.MethodPost, "/Devices/Options", h.updateDeviceOptions)

	administrator(http.MethodPost, "/Users/New", h.createUser)
	administrator(http.MethodDelete, "/Users/{userId}", h.deleteUser)
	signedIn(http.MethodPost, "/Users", h.updateUser)
	// Apps written for older Jellyfin versions name the user in the path.
	signedIn(http.MethodPost, "/Users/{userId}", h.updateUser)
	anonymous(http.MethodPost, "/Users/ForgotPassword", h.forgotPassword)
	anonymous(http.MethodPost, "/Users/ForgotPassword/Pin", h.redeemPasswordPin)

	administrator(http.MethodGet, "/System/ActivityLog/Entries", h.activityEntries)
	administrator(http.MethodGet, "/ScheduledTasks", h.scheduledTasks)
	administrator(http.MethodGet, "/ScheduledTasks/{taskId}", h.scheduledTask)
	administrator(http.MethodPost, "/ScheduledTasks/Running/{taskId}", h.startTask)
	administrator(http.MethodDelete, "/ScheduledTasks/Running/{taskId}", h.stopTask)
	administrator(http.MethodPost, "/ScheduledTasks/{taskId}/Triggers", h.updateTaskTriggers)
	administrator(http.MethodGet, "/System/Logs", h.logFiles)
	administrator(http.MethodGet, "/System/Logs/Log", h.logFile)
	signedIn(http.MethodPost, "/ClientLog/Document", h.clientLog)

	signedIn(http.MethodGet, "/System/Configuration", h.serverConfiguration)
	administrator(http.MethodPost, "/System/Configuration", h.updateServerConfiguration)
	administrator(http.MethodGet, "/System/Info/Storage", h.storage)
	administrator(http.MethodGet, "/Library/MediaFolders", h.mediaFolders)
}

// listResult is Jellyfin's QueryResult of any item type.
type listResult[T any] struct {
	Items            []T
	TotalRecordCount int
	StartIndex       int
}

// badRequestProblem answers like ASP.NET's BadRequest().
func badRequestProblem(w http.ResponseWriter) {
	writeJSON(w, http.StatusBadRequest, problemDetails{
		Type:    "https://tools.ietf.org/html/rfc9110#section-15.5.1",
		Title:   "Bad Request",
		Status:  http.StatusBadRequest,
		TraceID: traceID(),
	})
}

// actor names who made a change for the activity log: the user, or the
// app of the API key.
func actor(c caller) string {
	if c.APIKey != nil {
		return c.APIKey.App
	}
	return c.User.Name
}

// The authentication and password reset providers Jellyfin names, the
// only ones Polyfin has.
const (
	defaultAuthenticationProvider = "Jellyfin.Server.Implementations.Users.DefaultAuthenticationProvider"
	defaultPasswordResetProvider  = "Jellyfin.Server.Implementations.Users.DefaultPasswordResetProvider"
)

// NameIDPair is Jellyfin's NameIdPair.
type NameIDPair struct {
	Name string
	Id   string
}

func (h *Handler) authenticationProviders(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, []NameIDPair{{Name: "Default", Id: defaultAuthenticationProvider}})
}

func (h *Handler) passwordResetProviders(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, []NameIDPair{{Name: "Default Password Reset Provider", Id: defaultPasswordResetProvider}})
}

// AuthenticationInfo is Jellyfin's description of an API key.
type AuthenticationInfo struct {
	Id               int64
	AccessToken      string
	DeviceId         string
	AppName          string
	AppVersion       string
	DeviceName       string
	UserId           string
	IsActive         bool
	DateCreated      Time
	DateLastActivity Time
}

// revealWindow is how long a key made from a Jellyfin app is remembered,
// to be shown once by the next listing.
const revealWindow = 10 * time.Minute

// newKeys remembers the keys made from Jellyfin apps until a listing shows
// them. jellyfin-web makes a key, then lists the keys to show it: Jellyfin
// lists every key in full, Polyfin keeps only their hashes.
type newKeys struct {
	mu   sync.Mutex
	keys map[accounts.ID]newKey
}

type newKey struct {
	token   string
	created time.Time
}

func (n *newKeys) put(id accounts.ID, token string, now time.Time) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.keys == nil {
		n.keys = map[accounts.ID]newKey{}
	}
	n.keys[id] = newKey{token: token, created: now}
}

// take returns the key id names once, while it is new.
func (n *newKeys) take(id accounts.ID, now time.Time) (string, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for other, key := range n.keys {
		if now.Sub(key.created) > revealWindow {
			delete(n.keys, other)
		}
	}
	key, ok := n.keys[id]
	delete(n.keys, id)
	return key.token, ok
}

// apiKeys lists the API keys. Polyfin keeps only a hash of each key: the
// AccessToken of a key is its identifier, which revoking accepts as well
// as the key, except for a key just made from a Jellyfin app, which the
// first listing shows in full, once. DateLastActivity is when the key was
// last used, which Jellyfin leaves at its zero date.
func (h *Handler) apiKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := h.Accounts.APIKeys(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	now := h.now()
	result := listResult[AuthenticationInfo]{Items: make([]AuthenticationInfo, 0, len(keys)), TotalRecordCount: len(keys)}
	for _, key := range keys {
		token, ok := h.newKeys.take(key.ID, now)
		if !ok {
			token = key.ID.String()
		}
		info := AuthenticationInfo{AccessToken: token, AppName: key.App, UserId: accounts.ID{}.String(), DateCreated: Time(key.CreatedAt)}
		if key.LastUsedAt != nil {
			info.DateLastActivity = Time(*key.LastUsedAt)
		}
		result.Items = append(result.Items, info)
	}
	writeJSON(w, http.StatusOK, result)
}

// createAPIKey makes a key for the app named by app, and answers nothing,
// as Jellyfin does: the next listing shows it.
func (h *Handler) createAPIKey(w http.ResponseWriter, r *http.Request) {
	app := query(r, "app")
	if strings.TrimSpace(app) == "" {
		validationProblem(w, map[string][]string{"app": {"The app field is required."}})
		return
	}
	key, token, err := h.Accounts.CreateAPIKey(r.Context(), app)
	if errors.Is(err, accounts.ErrInvalidApp) {
		// Jellyfin takes any name; Polyfin's are those of its admin app.
		validationProblem(w, map[string][]string{"app": {"The app field must be at most 64 printable characters."}})
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.newKeys.put(key.ID, token, h.now())
	h.Logger.Info("An API key was made", "app", key.App, "by", actor(callerFrom(r.Context())))
	w.WriteHeader(http.StatusNoContent)
}

// revokeAPIKey deletes the key named by the key itself or, as the listing
// shows it, by its identifier. Like Jellyfin, a key that does not exist is
// no error.
func (h *Handler) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	raw := r.PathValue("key")
	err := h.Accounts.RevokeAPIKeyByToken(r.Context(), raw)
	if id, parseErr := accounts.ParseID(raw); err == nil && parseErr == nil {
		if err = h.Accounts.RevokeAPIKey(r.Context(), id); errors.Is(err, accounts.ErrNotFound) {
			err = nil
		}
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
