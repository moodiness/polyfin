package admin

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/activity"
)

// clientAddress is the address a request came from, for the activity log.
func clientAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// apiKeyJSON is an API key, without the key itself, which Polyfin does not
// keep: Key is set only in the answer that creates it.
type apiKeyJSON struct {
	ID         string     `json:"id"`
	App        string     `json:"app"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	Key        string     `json:"key,omitempty"`
}

func newAPIKeyJSON(key accounts.APIKey) apiKeyJSON {
	return apiKeyJSON{ID: key.ID.String(), App: key.App, CreatedAt: key.CreatedAt, LastUsedAt: key.LastUsedAt}
}

func (h *handler) apiKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := h.Accounts.APIKeys(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := make([]apiKeyJSON, 0, len(keys))
	for _, key := range keys {
		result = append(result, newAPIKeyJSON(key))
	}
	writeJSON(w, http.StatusOK, result)
}

// createAPIKey makes a key and answers it, the only time it is shown.
func (h *handler) createAPIKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		App string `json:"app"`
	}
	if !decode(w, r, &body) {
		return
	}
	key, token, err := h.Accounts.CreateAPIKey(r.Context(), body.App)
	if errors.Is(err, accounts.ErrInvalidApp) {
		writeError(w, http.StatusBadRequest, "invalid_app")
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.Logger.Info("An API key was made", "app", key.App, "by", sessionFrom(r.Context()).User.Name)
	result := newAPIKeyJSON(key)
	result.Key = token
	writeJSON(w, http.StatusCreated, result)
}

func (h *handler) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	err := h.Accounts.RevokeAPIKey(r.Context(), id)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// activityEntryJSON is an entry of the activity log.
type activityEntryJSON struct {
	ID            int64     `json:"id"`
	Date          time.Time `json:"date"`
	Name          string    `json:"name"`
	Type          string    `json:"type"`
	Overview      *string   `json:"overview"`
	ShortOverview *string   `json:"shortOverview"`
	Severity      string    `json:"severity"`
	UserID        *string   `json:"userId"`
}

// The number of activity entries one request lists, by default and at
// most.
const (
	defaultActivityLimit = 20
	maxActivityLimit     = 100
)

// recentActivity lists the latest entries of the activity log: from start
// on, of the types and severities listed, comma-separated, when given.
func (h *handler) recentActivity(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit := defaultActivityLimit
	if raw := query.Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > maxActivityLimit {
			writeError(w, http.StatusBadRequest, "invalid_limit")
			return
		}
		limit = value
	}
	start := 0
	if raw := query.Get("start"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			writeError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		start = value
	}
	page, err := h.Activity.Entries(r.Context(), activity.Query{Start: start, Limit: limit, Types: commaList(query.Get("type")),
		Severities: commaList(query.Get("severity"))})
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	items := make([]activityEntryJSON, 0, len(page.Entries))
	for _, e := range page.Entries {
		entry := activityEntryJSON{ID: e.ID, Date: e.Date, Name: e.Name, Type: e.Type, Overview: e.Overview,
			ShortOverview: e.ShortOverview, Severity: e.Severity}
		if e.UserID != nil {
			entry.UserID = new(e.UserID.String())
		}
		items = append(items, entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": page.Total})
}

// commaList splits a comma-separated parameter, nil when empty.
func commaList(raw string) []string {
	var values []string
	for value := range strings.SplitSeq(raw, ",") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
}
