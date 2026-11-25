package jellyfin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/moonfin"
)

// Moonfin's companion.
//
// Moonfin's apps call routes of their own on a server, under /Moonfin, for
// their extra features, and find them with GET /Moonfin/Ping (see package
// moonfin). Polyfin answers those it can serve from what it keeps: the
// settings each user's apps sync across their devices, with the
// administrator's defaults, the ratings MDBList gives and TMDB's extras
// (see moonfinextras.go), and the logs apps upload. The Ping answer tells
// which features are served; an app hides the others, as it does on a
// server without the companion.
//
// The routes answer a user the companion is served to (see
// accounts.Settings.MoonfinFor), signed in as for any route; to another,
// they answer 404, as a server without the companion does, and the app
// goes on without it. Silo's apps reach the same routes through Silo's
// plugin routes (see package silo).

// moonfinBodyBytes bounds the body of a request that saves settings.
const moonfinBodyBytes = moonfin.MaxEnvelopeBytes

// moonfinHeartbeat is how often the stream of a user's changes writes
// something while nothing changes: well within the 75 seconds Moonfin's
// apps wait before they connect again.
const moonfinHeartbeat = 30 * time.Second

// moonfinRoutes adds the routes of Moonfin's companion.
func (h *Handler) moonfinRoutes(rt *router) {
	route := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(h.moonfinServed(handler)))
	}
	route(http.MethodGet, "/Moonfin/Ping", h.moonfinPing)
	route(http.MethodGet, "/Moonfin/Defaults", h.moonfinDefaults)
	route(http.MethodGet, "/Moonfin/Settings", h.moonfinSettings)
	route(http.MethodPost, "/Moonfin/Settings", h.saveMoonfinSettings)
	route(http.MethodDelete, "/Moonfin/Settings", h.deleteMoonfinSettings)
	route(http.MethodGet, "/Moonfin/Settings/Resolved/{profile}", h.resolvedMoonfinSettings)
	route(http.MethodPost, "/Moonfin/Settings/Profile/{profile}", h.saveMoonfinProfile)
	route(http.MethodDelete, "/Moonfin/Settings/Profile/{profile}", h.deleteMoonfinProfile)
	route(http.MethodGet, "/Moonfin/Settings/Stream", h.moonfinStream)
	route(http.MethodGet, "/Moonfin/Themes", h.moonfinThemes)
	route(http.MethodPost, "/Moonfin/ClientLog/Document", h.clientLog)
	admin := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(h.moonfinAdministrator(handler)))
	}
	admin(http.MethodGet, "/Moonfin/Settings/{userId}", h.userMoonfinSettings)
	admin(http.MethodPost, "/Moonfin/Settings/{userId}", h.saveUserMoonfinSettings)
	h.moonfinExtraRoutes(route)
}

// moonfinServed serves next to a user the companion is served to, and
// answers 404 to another, as a server without it does. An API key, which
// is no user, gets 400: the companion keeps a user's settings.
func (h *Handler) moonfinServed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := callerFrom(r.Context())
		switch {
		case h.Moonfin == nil || c.APIKey == nil && !h.Accounts.Settings().MoonfinFor(c.User):
			w.WriteHeader(http.StatusNotFound)
		case c.APIKey != nil:
			moonfinError(w, http.StatusBadRequest, "An API key has no user.")
		default:
			next(w, r)
		}
	}
}

// moonfinAdministrator serves next to an administrator, an API key
// included, while Polyfin serves the companion, and answers others 403.
func (h *Handler) moonfinAdministrator(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case h.Moonfin == nil:
			w.WriteHeader(http.StatusNotFound)
		case !callerFrom(r.Context()).User.IsAdministrator:
			moonfinError(w, http.StatusForbidden, "Only administrators may do this.")
		default:
			next(w, r)
		}
	}
}

// moonfinError answers a failed request of the companion with a message,
// as Moonfin's apps log it.
func moonfinError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}

// moonfinStoreError answers a request that failed on the store's error.
func (h *Handler) moonfinStoreError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, moonfin.ErrNotFound):
		moonfinError(w, http.StatusNotFound, "No settings found for user.")
	case errors.Is(err, moonfin.ErrTooLarge):
		moonfinError(w, http.StatusRequestEntityTooLarge, "The settings are too large.")
	case errors.Is(err, moonfin.ErrInvalidProfile):
		moonfinError(w, http.StatusBadRequest, "Invalid profile. Valid profiles: global, desktop, mobile, tv.")
	default:
		h.internalError(w, r, err)
	}
}

// moonfinBody reads the JSON object of a request's body, its names in
// lower case, as the companion's model binding matches them. ok is false
// once it answered the request.
func moonfinBody(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, moonfinBodyBytes))
	if maxBytes := new(http.MaxBytesError); errors.As(err, &maxBytes) {
		moonfinError(w, http.StatusRequestEntityTooLarge, "The settings are too large.")
		return nil, false
	}
	var body map[string]any
	if err != nil || json.Unmarshal(data, &body) != nil || body == nil {
		moonfinError(w, http.StatusBadRequest, "The body must be a JSON object.")
		return nil, false
	}
	out := make(map[string]any, len(body))
	for name, value := range body {
		out[strings.ToLower(name)] = value
	}
	return out, true
}

// moonfinPing tells Moonfin's apps that the companion is there, which of
// its features Polyfin serves, and the administrator's defaults, for the
// kind of device profile names, else for every device.
func (h *Handler) moonfinPing(w http.ResponseWriter, r *http.Request) {
	defaults, err := h.Moonfin.Defaults(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	profile, ok := moonfin.ProfileName(query(r, "profile"))
	if !ok {
		profile = moonfin.Global
	}
	settings := h.Accounts.Settings()
	tmdb := h.TMDB.Enabled()
	writeJSON(w, http.StatusOK, map[string]any{
		"installed":           true,
		"version":             h.PolyfinVersion,
		"serverName":          settings.ServerName,
		"settingsSyncEnabled": true,
		"defaultSettings":     moonfin.LayerDefaults(defaults, profile),
		"mdblistAvailable":    settings.MDBListRatingsKey != "",
		// TMDB's home rows are custom rows, which Polyfin does not serve;
		// its extras for a title or a person it does, with the server's
		// key, which no user is given.
		"tmdbAvailable":            false,
		"tmdbThumbsSupported":      tmdb,
		"tmdbDetailsSupported":     tmdb,
		"tmdbPersonLinksSupported": tmdb,
		"tmdbReviewsSupported":     tmdb,
		"clientLogSupported":       true,
		// The features Polyfin does not serve: the app hides them.
		"seerrEnabled":                      false,
		"messagesSupported":                 false,
		"recommendationsSupported":          false,
		"pinSupported":                      false,
		"discoverChartsSupported":           false,
		"smartSearchEnabled":                false,
		"calendarAvailable":                 false,
		"calendarReminders":                 false,
		"arrQueueAvailable":                 false,
		"pushSupported":                     false,
		"incompleteRecordingsSupported":     false,
		"extendedCompleteShowingsSupported": false,
	})
}

// moonfinDefaults answers the administrator's defaults for every device.
func (h *Handler) moonfinDefaults(w http.ResponseWriter, r *http.Request) {
	defaults, err := h.Moonfin.Defaults(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, defaults[moonfin.Global])
}

// moonfinSettings answers the caller's settings, 404 when they have none:
// HEAD tells which.
func (h *Handler) moonfinSettings(w http.ResponseWriter, r *http.Request) {
	h.writeMoonfinSettings(w, r, callerFrom(r.Context()).User.ID)
}

func (h *Handler) writeMoonfinSettings(w http.ResponseWriter, r *http.Request, user accounts.ID) {
	env, err := h.Moonfin.Envelope(r.Context(), user)
	if err != nil {
		h.moonfinStoreError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, env)
}

// saveMoonfinSettings keeps the whole settings an app sends for the
// caller, in settings, merged into theirs, or replacing them when
// mergeMode is anything but "merge".
func (h *Handler) saveMoonfinSettings(w http.ResponseWriter, r *http.Request) {
	h.saveMoonfinEnvelope(w, r, callerFrom(r.Context()).User.ID)
}

func (h *Handler) saveMoonfinEnvelope(w http.ResponseWriter, r *http.Request, user accounts.ID) {
	body, ok := moonfinBody(w, r)
	if !ok {
		return
	}
	env, isObject := body["settings"].(map[string]any)
	if !isObject {
		moonfinError(w, http.StatusBadRequest, "Settings are required.")
		return
	}
	mode, _ := body["mergemode"].(string)
	clientID, _ := body["clientid"].(string)
	created, err := h.Moonfin.SaveEnvelope(r.Context(), user, env, mode != "" && mode != "merge", clientID)
	if err != nil {
		h.moonfinStoreError(w, r, err)
		return
	}
	writeMoonfinSaved(w, user, created)
}

func writeMoonfinSaved(w http.ResponseWriter, user accounts.ID, created bool) {
	writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Created": created, "UserId": user.String()})
}

// deleteMoonfinSettings forgets the caller's settings.
func (h *Handler) deleteMoonfinSettings(w http.ResponseWriter, r *http.Request) {
	if err := h.Moonfin.Delete(r.Context(), callerFrom(r.Context()).User.ID); err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "Settings deleted"})
}

// resolvedMoonfinSettings answers the caller's settings as a device of the
// profile the path names takes them, the administrator's defaults
// included: an empty object for a user without settings or defaults,
// whose app then starts syncing its own.
func (h *Handler) resolvedMoonfinSettings(w http.ResponseWriter, r *http.Request) {
	profile, ok := moonfin.ProfileName(r.PathValue("profile"))
	if !ok {
		h.moonfinStoreError(w, r, moonfin.ErrInvalidProfile)
		return
	}
	resolved, err := h.Moonfin.Resolved(r.Context(), callerFrom(r.Context()).User.ID, profile)
	if err != nil {
		h.moonfinStoreError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resolved)
}

// saveMoonfinProfile merges the settings of one profile an app sends, in
// profile, into the caller's.
func (h *Handler) saveMoonfinProfile(w http.ResponseWriter, r *http.Request) {
	profile, ok := moonfin.ProfileName(r.PathValue("profile"))
	if !ok {
		h.moonfinStoreError(w, r, moonfin.ErrInvalidProfile)
		return
	}
	body, ok := moonfinBody(w, r)
	if !ok {
		return
	}
	settings, isObject := body["profile"].(map[string]any)
	if !isObject {
		moonfinError(w, http.StatusBadRequest, "Profile settings are required.")
		return
	}
	clientID, _ := body["clientid"].(string)
	user := callerFrom(r.Context()).User.ID
	created, err := h.Moonfin.SaveProfile(r.Context(), user, profile, moonfin.Clean(settings), clientID)
	if err != nil {
		h.moonfinStoreError(w, r, err)
		return
	}
	writeMoonfinSaved(w, user, created)
}

// deleteMoonfinProfile forgets one device profile of the caller's
// settings. The global one goes only with all of them (DELETE
// /Moonfin/Settings).
func (h *Handler) deleteMoonfinProfile(w http.ResponseWriter, r *http.Request) {
	profile, ok := moonfin.ProfileName(r.PathValue("profile"))
	if !ok || profile == moonfin.Global {
		moonfinError(w, http.StatusBadRequest, "Only a device profile may be deleted: desktop, mobile or tv.")
		return
	}
	if err := h.Moonfin.DeleteProfile(r.Context(), callerFrom(r.Context()).User.ID, profile); err != nil {
		h.moonfinStoreError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"Success": true, "Message": "Profile '" + profile + "' deleted"})
}

// moonfinStream tells the caller's open apps, as server-sent events, that
// their settings changed, as "settingsUpdated": each then asks for them
// again. A comment every moonfinHeartbeat keeps the connection alive. The
// stream stays open until the app closes it, and is no request under way
// for the work that waits for the apps' requests to end (see
// appRequests).
func (h *Handler) moonfinStream(w http.ResponseWriter, r *http.Request) {
	h.requests.end()
	defer h.requests.begin()
	changed, stop := h.Moonfin.Watch(callerFrom(r.Context()).User.ID)
	defer stop()
	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flush := func(text string) bool {
		if _, err := io.WriteString(w, text); err != nil {
			return false
		}
		return http.NewResponseController(w).Flush() == nil
	}
	if !flush(": connected\n\n") {
		return
	}
	heartbeat := time.NewTicker(moonfinHeartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-changed:
			if !flush("data: {\"type\":\"settingsUpdated\"}\n\n") {
				return
			}
		case <-heartbeat.C:
			if !flush(": heartbeat\n\n") {
				return
			}
		}
	}
}

// moonfinThemes lists the custom themes the administrator uploaded for
// Moonfin's apps: Polyfin keeps none.
func (h *Handler) moonfinThemes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

// moonfinUser is the user the path names, for an administrator's routes.
// ok is false once it answered the request.
func (h *Handler) moonfinUser(w http.ResponseWriter, r *http.Request) (accounts.ID, bool) {
	id, err := accounts.ParseID(r.PathValue("userId"))
	if err != nil {
		moonfinError(w, http.StatusBadRequest, "Invalid user.")
		return accounts.ID{}, false
	}
	if _, err := h.Accounts.User(r.Context(), id); errors.Is(err, accounts.ErrNotFound) {
		moonfinError(w, http.StatusNotFound, "No such user.")
		return accounts.ID{}, false
	} else if err != nil {
		h.internalError(w, r, err)
		return accounts.ID{}, false
	}
	return id, true
}

// userMoonfinSettings answers the settings of the user the path names, for
// an administrator.
func (h *Handler) userMoonfinSettings(w http.ResponseWriter, r *http.Request) {
	if user, ok := h.moonfinUser(w, r); ok {
		h.writeMoonfinSettings(w, r, user)
	}
}

// saveUserMoonfinSettings keeps settings for the user the path names, as
// POST /Moonfin/Settings does for the caller, for an administrator.
func (h *Handler) saveUserMoonfinSettings(w http.ResponseWriter, r *http.Request) {
	if user, ok := h.moonfinUser(w, r); ok {
		h.saveMoonfinEnvelope(w, r, user)
	}
}
