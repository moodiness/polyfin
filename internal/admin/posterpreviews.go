package admin

import (
	"context"
	"net/http"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/jellyfin"
)

// PosterPreviews finds the titles whose posters Settings › Content ›
// Posters previews the badges on (see jellyfin.Handler.PosterPreviews).
type PosterPreviews interface {
	PosterPreviews(ctx context.Context, user accounts.User, size string, count int) ([]jellyfin.PosterPreview, error)
}

// posterPreviewCount is how many titles a preview offers.
const posterPreviewCount = 12

// posterPreviewJSON is a title to preview the badges on: the tags of its
// poster without and with them, for /Items/{id}/Images/Primary.
type posterPreviewJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Tag       string `json:"tag"`
	BadgedTag string `json:"badgedTag"`
}

// posterPreviews answers GET /admin/api/settings/poster-previews?size=…:
// real titles of the administrator's libraries, as kept, with their
// posters' tags without badges and with badges at the size asked for,
// one of accounts.PosterBadgeSizes, whether badges are on or not.
func (h *handler) posterPreviews(w http.ResponseWriter, r *http.Request) {
	size := r.URL.Query().Get("size")
	if !slices.Contains(accounts.PosterBadgeSizes, size) {
		writeError(w, http.StatusBadRequest, "invalid_poster_badge_size")
		return
	}
	titles := []posterPreviewJSON{}
	if h.PosterPreviews != nil {
		found, err := h.PosterPreviews.PosterPreviews(r.Context(), sessionFrom(r.Context()).User, size, posterPreviewCount)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		for _, title := range found {
			titles = append(titles, posterPreviewJSON{ID: title.ID.String(), Name: title.Name, Tag: title.Tag, BadgedTag: title.BadgedTag})
		}
	}
	writeJSON(w, http.StatusOK, struct {
		Titles []posterPreviewJSON `json:"titles"`
	}{titles})
}
