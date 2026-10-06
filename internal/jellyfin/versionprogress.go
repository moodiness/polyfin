package jellyfin

import (
	"errors"
	"net/http"

	"github.com/moodiness/polyfin/internal/library"
)

// VersionProgress tells how far the listing of a title's versions has
// come since its details opened: Pending is how many of the user's addons
// are still asked for its streams in the background, Count how many media
// sources its details would list now, the placeholder included. Polyfin's
// jellyfin-web script polls it to reload the title page as versions come.
type VersionProgress struct {
	Pending int
	Count   int
}

// versionProgress answers /Polyfin/Items/{itemId}/Versions for a movie or
// an episode, by its own identifier or one of its versions', with the
// checks of its details; other items answer zero for both.
func (h *Handler) versionProgress(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "itemId")
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	item, err := h.Library.Item(r.Context(), user, id)
	if errors.Is(err, library.ErrNotFound) {
		if owner, ok := h.Library.VersionOwner(id); ok {
			item, err = h.Library.Item(r.Context(), user, owner)
		}
	}
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	var progress VersionProgress
	if item.Kind == library.KindMovie || item.Kind == library.KindEpisode {
		// Pending first: an addon no longer asked has its streams kept, so
		// the count that follows has its versions.
		progress.Pending = h.Library.Pending(user, item.ID)
		versions, err := h.Library.KnownVersions(r.Context(), user, item.ID)
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		// As details count them (see addMediaSources).
		progress.Count = max(len(h.offered(r.Context(), user, versions)), 1)
	}
	writeJSON(w, http.StatusOK, progress)
}
