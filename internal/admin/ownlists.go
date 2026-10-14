package admin

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/jellyfin"
	"github.com/moodiness/polyfin/internal/library"
)

// Watching is what My account shows of the user's lists and changes in
// them, as the Jellyfin API's Polyfin routes do: Continue Watching, whose
// titles can be taken off it, and the titles hidden from every list, which
// can be shown again.
type Watching interface {
	ResumeItems(ctx context.Context, user accounts.User) ([]jellyfin.ResumeItem, error)
	RemoveResume(ctx context.Context, user accounts.User, id accounts.ID) error
	HiddenTitles(ctx context.Context, user accounts.User) ([]jellyfin.HiddenTitle, error)
	ShowTitle(ctx context.Context, user accounts.User, id accounts.ID) error
}

// resumeJSON is an item of Continue Watching: its resume point and the
// runtime it is measured against, in seconds, the runtime 0 when unknown.
type resumeJSON struct {
	Item         *liveItemJSON `json:"item"`
	Position     int64         `json:"position"`
	Runtime      int64         `json:"runtime"`
	Played       bool          `json:"played"`
	LastPlayedAt *time.Time    `json:"lastPlayedAt"`
}

// hiddenJSON is a title hidden from the user's lists, and when it was.
type hiddenJSON struct {
	Item     *liveItemJSON `json:"item"`
	HiddenAt time.Time     `json:"hiddenAt"`
}

func (h *handler) ownResume(w http.ResponseWriter, r *http.Request) {
	if h.Watching == nil {
		writeJSON(w, http.StatusOK, []resumeJSON{})
		return
	}
	items, err := h.Watching.ResumeItems(r.Context(), sessionFrom(r.Context()).User)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := make([]resumeJSON, 0, len(items))
	for _, entry := range items {
		runtime := entry.Data.Runtime
		if runtime <= 0 {
			runtime = entry.Item.Runtime
		}
		result = append(result, resumeJSON{Item: newLiveItemJSON(entry.Item), Position: int64(entry.Data.Position.Seconds()),
			Runtime: int64(runtime.Seconds()), Played: entry.Data.Played, LastPlayedAt: entry.Data.LastPlayed})
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handler) removeOwnResume(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if h.Watching == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	err := h.Watching.RemoveResume(r.Context(), sessionFrom(r.Context()).User, id)
	switch {
	case errors.Is(err, library.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found")
	case err != nil:
		h.internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *handler) ownHidden(w http.ResponseWriter, r *http.Request) {
	if h.Watching == nil {
		writeJSON(w, http.StatusOK, []hiddenJSON{})
		return
	}
	titles, err := h.Watching.HiddenTitles(r.Context(), sessionFrom(r.Context()).User)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result := make([]hiddenJSON, 0, len(titles))
	for _, title := range titles {
		result = append(result, hiddenJSON{Item: newLiveItemJSON(title.Item), HiddenAt: title.At})
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handler) showOwnHidden(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if h.Watching == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	if err := h.Watching.ShowTitle(r.Context(), sessionFrom(r.Context()).User, id); err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
