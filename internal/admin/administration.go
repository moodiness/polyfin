package admin

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/activity"
	"github.com/moodiness/polyfin/internal/library"
)

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
	// Title is the title the name holds, which the admin app links to the
	// web player; null when it names none, one the administrator no longer
	// reaches, or when Polyfin serves no web player.
	Title *activityTitleJSON `json:"title"`
}

// activityTitleJSON is where an entry's name holds the title of an item:
// from Start to End, in UTF-16 code units, as JavaScript counts them.
type activityTitleJSON struct {
	ID    string `json:"id"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// ActivityTitles describes the items of ids a user reaches, and leaves
// out the others: removed, or out of their view.
type ActivityTitles interface {
	Items(ctx context.Context, user accounts.User, ids []accounts.ID) ([]library.Item, error)
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
	reached := h.reachedTitles(r.Context(), sessionFrom(r.Context()).User, page.Entries)
	items := make([]activityEntryJSON, 0, len(page.Entries))
	for _, e := range page.Entries {
		entry := activityEntryJSON{ID: e.ID, Date: e.Date, Name: e.Name, Type: e.Type, Overview: e.Overview,
			ShortOverview: e.ShortOverview, Severity: e.Severity}
		if e.UserID != nil {
			entry.UserID = new(e.UserID.String())
		}
		if id, ok := entryTitle(e); ok && reached[id] {
			entry.Title = &activityTitleJSON{ID: id.String(), Start: utf16Index(e.Name, *e.TitleStart), End: utf16Index(e.Name, *e.TitleEnd)}
		}
		items = append(items, entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": page.Total})
}

// reachedTitles tells which of the titles the entries' names hold user
// still reaches, while the web player they open in is served. When they
// cannot be told, none is linked: the entries are still listed.
func (h *handler) reachedTitles(ctx context.Context, user accounts.User, entries []activity.Entry) map[accounts.ID]bool {
	reached := map[accounts.ID]bool{}
	if h.ActivityTitles == nil || !h.WebClient {
		return reached
	}
	var ids []accounts.ID
	for _, e := range entries {
		if id, ok := entryTitle(e); ok && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return reached
	}
	items, err := h.ActivityTitles.Items(ctx, user, ids)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			h.Logger.Warn("The titles of the activity log could not be found", "error", err)
		}
		return reached
	}
	for _, item := range items {
		reached[item.ID] = true
	}
	return reached
}

// entryTitle is the item whose title e's name holds, when it holds one.
func entryTitle(e activity.Entry) (accounts.ID, bool) {
	if e.ItemID == nil || e.TitleStart == nil || e.TitleEnd == nil {
		return accounts.ID{}, false
	}
	id, err := accounts.ParseID(*e.ItemID)
	return id, err == nil
}

// utf16Index is how many UTF-16 code units the first n characters of s
// take.
func utf16Index(s string, n int) int {
	units := 0
	for _, r := range s {
		if n == 0 {
			break
		}
		units += utf16.RuneLen(r)
		n--
	}
	return units
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
