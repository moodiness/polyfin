package jellyfin

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// addWatchlistView adds the Watchlist view to views while user's watchlist
// holds titles they have not started (see library.WatchlistViewID): a
// library without type, which apps give a home row of latest items, its
// titles the latest added first. Jellyfin apps have no watchlist button:
// the view is filled and emptied from the lists it gathers, and from
// Silo's apps through Polyfin's own routes below.
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

// Polyfin's own routes put titles on the user's watchlist and take them
// off, for Silo's apps (see package silo) and any app:
//
//	POST /Polyfin/UserItems/{itemId}/Watchlist puts a movie or a series on
//	  the user's watchlist, unless a list already holds it; a season or an
//	  episode stands for its series. DELETE takes it off every list.
//	GET /Polyfin/UserItems/Watchlist lists the titles the user's lists
//	  hold, started or not, by identifier, with the date each was added,
//	  the latest first.

// errNotWatchlisted reports an item that is neither a movie nor a series,
// nor in one.
var errNotWatchlisted = errors.New("only movies and series go on the watchlist")

func (h *Handler) watchlistRoutes(signedIn func(method, pattern string, handler http.HandlerFunc)) {
	signedIn(http.MethodPost, "/Polyfin/UserItems/{itemId}/Watchlist", h.watchlistTitle(true))
	signedIn(http.MethodDelete, "/Polyfin/UserItems/{itemId}/Watchlist", h.watchlistTitle(false))
	signedIn(http.MethodGet, "/Polyfin/UserItems/Watchlist", h.watchlistHeld)
}

// watchlistTarget returns the title the watchlist holds for item: the
// movie or series itself, the series of a season or an episode.
func (h *Handler) watchlistTarget(ctx context.Context, user accounts.User, item library.Item) (library.Item, error) {
	switch {
	case item.Kind == library.KindMovie || item.Kind == library.KindSeries:
		return item, nil
	case (item.Kind == library.KindSeason || item.Kind == library.KindEpisode) && item.SeriesID != accounts.ID{}:
		return h.Library.Item(ctx, user, item.SeriesID)
	}
	return library.Item{}, errNotWatchlisted
}

// watchlistTitle answers POST and DELETE /Polyfin/UserItems/{itemId}/Watchlist
// with 204.
func (h *Handler) watchlistTitle(add bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, item, ok := h.userDataItem(w, r, bindErrors{})
		if !ok {
			return
		}
		if user.ID == (accounts.ID{}) {
			// A request made with an API key names no user.
			notFoundProblem(w)
			return
		}
		title, err := h.watchlistTarget(r.Context(), user, item)
		if errors.Is(err, errNotWatchlisted) {
			badRequestProblem(w)
			return
		}
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		if add {
			err = h.Library.AddToWatchlist(r.Context(), user.ID, title.ID)
		} else {
			err = h.Library.RemoveFromWatchlist(r.Context(), user.ID, title.ID)
		}
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		h.userDataChanged(user, []library.Item{title})
		w.WriteHeader(http.StatusNoContent)
	}
}

// WatchlistTitleDto is a title the user's watchlist holds.
type WatchlistTitleDto struct {
	Id      string
	AddedAt time.Time
}

// watchlistHeld answers GET /Polyfin/UserItems/Watchlist.
func (h *Handler) watchlistHeld(w http.ResponseWriter, r *http.Request) {
	user, ok := h.viewer(w, r, bindErrors{}, notFoundProblem)
	if !ok {
		return
	}
	items := []WatchlistTitleDto{}
	if user.ID != (accounts.ID{}) {
		held, err := h.Library.WatchlistHeld(r.Context(), user.ID)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		for _, title := range held {
			items = append(items, WatchlistTitleDto{Id: title.ID.String(), AddedAt: title.AddedAt.UTC()})
		}
	}
	writeJSON(w, http.StatusOK, struct{ Items []WatchlistTitleDto }{items})
}
