package jellyfin

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// Jellyfin 12.4 has no way to take a series off Next Up but its watch
// state: marked played, or every episode played or started marked
// unplayed. Polyfin's own routes take it off while keeping what the user
// watched, for its web player's script (see internal/server/webclient.js),
// its admin app's My account and any app:
//
//	DELETE /Polyfin/UserItems/{itemId}/NextUp takes the series of an
//	  episode or a season, or the series itself, off the user's Next Up;
//	POST /Polyfin/UserItems/{itemId}/NextUp puts it back.
//
// A series taken off stays off until one of its episodes is played after
// the removal, one of them is released after it, or the user puts it back
// (see userdata.NextUpRemoval). Next Up asked for one series, as a series'
// page and playing a series ask, still answers its next episode. Both
// routes answer 204, and tell the user's sockets of a change of the user
// data of the item and its series, so that their apps list Next Up again.

// ErrNoNextUp reports an item that is neither a series nor in one.
var ErrNoNextUp = errors.New("only series and their episodes are in Next Up")

func (h *Handler) nextUpRemovalRoutes(signedIn func(method, pattern string, handler http.HandlerFunc)) {
	signedIn(http.MethodDelete, "/Polyfin/UserItems/{itemId}/NextUp", h.changeNextUp(true))
	signedIn(http.MethodPost, "/Polyfin/UserItems/{itemId}/NextUp", h.changeNextUp(false))
}

// changeNextUp answers DELETE (removed) and POST /Polyfin/UserItems/
// {itemId}/NextUp with 204.
func (h *Handler) changeNextUp(removed bool) http.HandlerFunc {
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
		var err error
		if removed {
			err = h.removeFromNextUp(r.Context(), user, item)
		} else {
			err = h.putBackOnNextUp(r.Context(), user, item)
		}
		switch {
		case errors.Is(err, ErrNoNextUp):
			badRequestProblem(w)
		case err != nil:
			h.browseError(w, r, err)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

// nextUpSeries returns the series item stands for in Next Up: itself, or
// the series of a season or an episode.
func (h *Handler) nextUpSeries(ctx context.Context, user accounts.User, item library.Item) (library.Item, error) {
	switch {
	case item.Kind == library.KindSeries:
		return item, nil
	case (item.Kind == library.KindSeason || item.Kind == library.KindEpisode) && item.SeriesID != accounts.ID{}:
		return h.Library.Item(ctx, user, item.SeriesID)
	}
	return library.Item{}, ErrNoNextUp
}

// removeFromNextUp takes the series item stands for off user's Next Up.
func (h *Handler) removeFromNextUp(ctx context.Context, user accounts.User, item library.Item) error {
	series, err := h.nextUpSeries(ctx, user, item)
	if err != nil {
		return err
	}
	if err := h.UserData.RemoveFromNextUp(ctx, user.ID, series.ID, time.Now()); err != nil {
		return err
	}
	h.userDataChanged(user, nextUpChanged(item, series))
	return nil
}

// putBackOnNextUp puts the series item stands for back on user's Next Up.
func (h *Handler) putBackOnNextUp(ctx context.Context, user accounts.User, item library.Item) error {
	series, err := h.nextUpSeries(ctx, user, item)
	if err != nil {
		return err
	}
	removed, err := h.UserData.PutBackOnNextUp(ctx, user.ID, series.ID)
	if err == nil && removed {
		h.userDataChanged(user, nextUpChanged(item, series))
	}
	return err
}

// nextUpChanged lists the items whose user data a change of Next Up
// changes: the item asked for and its series.
func nextUpChanged(item, series library.Item) []library.Item {
	if item.ID == series.ID {
		return []library.Item{series}
	}
	return []library.Item{item, series}
}

// RemoveFromNextUp takes the series user opens by id, or that of the
// season or episode they open by it, off their Next Up (see the routes
// above). library.ErrNotFound reports an item the user cannot open.
func (h *Handler) RemoveFromNextUp(ctx context.Context, user accounts.User, id accounts.ID) error {
	item, err := h.Library.Item(ctx, user, id)
	if err != nil {
		return err
	}
	return h.removeFromNextUp(ctx, user, item)
}

// PutBackOnNextUp puts a series user took off Next Up back, by its
// identifier or that of one of its seasons or episodes. A series no
// longer known is put back all the same.
func (h *Handler) PutBackOnNextUp(ctx context.Context, user accounts.User, id accounts.ID) error {
	if item, err := h.Library.Item(ctx, user, id); err == nil {
		if series, err := h.nextUpSeries(ctx, user, item); err == nil {
			return h.putBackOnNextUp(ctx, user, series)
		}
	}
	_, err := h.UserData.PutBackOnNextUp(ctx, user.ID, id)
	return err
}

// NextUpItems lists user's Next Up as an app asking for it without a
// library or a date does, the episodes under way included.
func (h *Handler) NextUpItems(ctx context.Context, user accounts.User) ([]library.Item, error) {
	return h.nextUpItems(ctx, user, nil, nil, nil, true, false)
}

// NextUpRemoval is a series a user took off Next Up, and when.
type NextUpRemoval struct {
	Item library.Item
	At   time.Time
}

// NextUpRemovals lists the series user took off Next Up that are still
// off it and that Polyfin can still describe, the latest removed first.
func (h *Handler) NextUpRemovals(ctx context.Context, user accounts.User) ([]NextUpRemoval, error) {
	removals, err := h.UserData.NextUpRemovals(ctx, user.ID)
	if err != nil || len(removals) == 0 {
		return nil, err
	}
	latest, err := h.latestPlayed(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	ids := make([]accounts.ID, 0, len(removals))
	for _, removal := range removals {
		ids = append(ids, removal.Series)
	}
	items, err := h.Library.Items(ctx, user, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[accounts.ID]library.Item, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	found := make([]*NextUpRemoval, len(removals))
	var wg sync.WaitGroup
	limiter := make(chan struct{}, 6)
	for i, removal := range removals {
		item, ok := byID[removal.Series]
		if !ok || item.Kind != library.KindSeries || removalLifted(removal.At, latest[removal.Series]) {
			continue
		}
		wg.Go(func() {
			limiter <- struct{}{}
			defer func() { <-limiter }()
			episodes, err := h.Library.Episodes(ctx, user, item.ID, nil)
			if err != nil || !releasedSince(episodes, removal.At) {
				found[i] = &NextUpRemoval{Item: item, At: removal.At}
			}
		})
	}
	wg.Wait()
	result := make([]NextUpRemoval, 0, len(removals))
	for _, removal := range found {
		if removal != nil {
			result = append(result, *removal)
		}
	}
	return result, nil
}

// nextUpRemovals returns when the user took each series off Next Up.
func (h *Handler) nextUpRemovals(ctx context.Context, user accounts.ID) (map[accounts.ID]time.Time, error) {
	removals, err := h.UserData.NextUpRemovals(ctx, user)
	if err != nil {
		return nil, err
	}
	removed := make(map[accounts.ID]time.Time, len(removals))
	for _, removal := range removals {
		removed[removal.Series] = removal.At
	}
	return removed, nil
}

// latestPlayed returns, for each series, the latest date the user played
// or started one of its episodes.
func (h *Handler) latestPlayed(ctx context.Context, user accounts.ID) (map[accounts.ID]*time.Time, error) {
	entries, err := h.UserData.Episodes(ctx, user)
	if err != nil {
		return nil, err
	}
	latest := map[accounts.ID]*time.Time{}
	for _, e := range entries {
		// Entries come latest first.
		if _, seen := latest[e.Series]; !seen && e.LastPlayed != nil {
			latest[e.Series] = e.LastPlayed
		}
	}
	return latest, nil
}

// removalLifted reports whether the user played or started an episode of
// a series after taking it off Next Up at removed, latest being the latest
// date they did.
func removalLifted(removed time.Time, latest *time.Time) bool {
	return latest != nil && latest.After(removed)
}

// releasedSince reports whether one of a series' regular episodes was
// released after t: an episode with no date, or not out yet, never was.
func releasedSince(episodes []library.Item, t time.Time) bool {
	for _, episode := range regularEpisodes(episodes) {
		if episode.Available && episode.PremiereDate != nil && episode.PremiereDate.After(t) {
			return true
		}
	}
	return false
}
