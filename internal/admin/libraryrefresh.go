package admin

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
)

// LibraryRefreshes reads the descriptions of the server's libraries again,
// paced (see library.Service.RefreshLibrary).
type LibraryRefreshes interface {
	RefreshLibrary(ctx context.Context, id accounts.ID) (library.LibraryRefresh, error)
	StopLibraryRefresh(id accounts.ID) bool
	LibraryRefreshes() map[accounts.ID]library.LibraryRefresh
}

// libraryRefreshJSON is how the refresh of a library's descriptions
// stands: State is "queued", "running", "done", "stopped" or "failed";
// Titles counts the titles it describes again, Done those described,
// Failed those no addon described; StartedAt and EndedAt are null until
// it starts and ends.
type libraryRefreshJSON struct {
	State     string     `json:"state"`
	Titles    int        `json:"titles"`
	Done      int        `json:"done"`
	Failed    int        `json:"failed"`
	StartedAt *time.Time `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt"`
}

func newLibraryRefreshJSON(r library.LibraryRefresh) *libraryRefreshJSON {
	result := &libraryRefreshJSON{State: r.State, Titles: r.Titles, Done: r.Done, Failed: r.Failed}
	if !r.Started.IsZero() {
		result.StartedAt = new(r.Started)
	}
	if !r.Ended.IsZero() {
		result.EndedAt = new(r.Ended)
	}
	return result
}

// refreshedLibrary finds the server's enabled library a request names by
// its addon and catalog, answering an error when there is none.
func (h *handler) refreshedLibrary(w http.ResponseWriter, r *http.Request) (accounts.ID, bool) {
	if h.LibraryRefreshes == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return accounts.ID{}, false
	}
	var body struct {
		AddonID     string `json:"addonId"`
		CatalogType string `json:"catalogType"`
		CatalogID   string `json:"catalogId"`
	}
	if !decode(w, r, &body) {
		return accounts.ID{}, false
	}
	addonID, err := accounts.ParseID(body.AddonID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_library")
		return accounts.ID{}, false
	}
	catalogs, err := h.Addons.Libraries(r.Context(), addons.Shared())
	if err != nil {
		h.internalError(w, r, err)
		return accounts.ID{}, false
	}
	index := slices.IndexFunc(catalogs, func(l addons.Library) bool {
		return l.Enabled && l.AddonID == addonID && l.Catalog.Type == body.CatalogType && l.Catalog.ID == body.CatalogID
	})
	if index < 0 {
		writeError(w, http.StatusBadRequest, "invalid_library")
		return accounts.ID{}, false
	}
	return library.LibraryID(catalogs[index]), true
}

// refreshLibrary has the descriptions of one of the server's libraries read
// again in the background, and answers how its refresh stands.
func (h *handler) refreshLibrary(w http.ResponseWriter, r *http.Request) {
	id, ok := h.refreshedLibrary(w, r)
	if !ok {
		return
	}
	state, err := h.LibraryRefreshes.RefreshLibrary(r.Context(), id)
	switch {
	case errors.Is(err, library.ErrNotRefreshable):
		writeError(w, http.StatusBadRequest, "library_not_refreshable")
	case errors.Is(err, library.ErrNotFound):
		writeError(w, http.StatusBadRequest, "invalid_library")
	case err != nil:
		h.internalError(w, r, err)
	default:
		writeJSON(w, http.StatusAccepted, newLibraryRefreshJSON(state))
	}
}

// stopLibraryRefresh stops the refresh of one of the server's libraries,
// running or waiting.
func (h *handler) stopLibraryRefresh(w http.ResponseWriter, r *http.Request) {
	id, ok := h.refreshedLibrary(w, r)
	if !ok {
		return
	}
	if !h.LibraryRefreshes.StopLibraryRefresh(id) {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
