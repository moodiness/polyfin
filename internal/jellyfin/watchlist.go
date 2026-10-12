package jellyfin

import (
	"errors"
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// addWatchlistView adds the Watchlist view to views while user's watchlist
// holds titles they have not started (see library.WatchlistViewID): a
// library without type, which apps give a home row of latest items, its
// titles the latest added first. Jellyfin apps have no watchlist button:
// the view is filled and emptied from the lists it gathers.
func (h *Handler) addWatchlistView(r *http.Request, user accounts.User, views []BaseItemDto) ([]BaseItemDto, error) {
	view, err := h.Library.Item(r.Context(), user, library.WatchlistViewID)
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
