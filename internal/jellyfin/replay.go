package jellyfin

import (
	"errors"
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// addReplayView adds the Replay view to views when user has channels whose
// provider keeps past programmes (see library.ReplayViewID): a library of
// folders, one per channel, which jellyfin-web lists as it lists a
// folder's items.
func (h *Handler) addReplayView(r *http.Request, user accounts.User, views []BaseItemDto) ([]BaseItemDto, error) {
	view, err := h.Library.Item(r.Context(), user, library.ReplayViewID)
	if errors.Is(err, library.ErrNotFound) {
		return views, nil
	}
	if err != nil {
		return nil, err
	}
	dtos, err := h.folderDtos(r, user, []library.Item{view})
	if err != nil {
		return nil, err
	}
	return append(views, dtos[0]), nil
}
