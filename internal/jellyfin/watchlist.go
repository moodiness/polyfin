package jellyfin

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playlists"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// The watchlist.
//
// Each user's watchlist is a playlist of theirs (see package watchlist),
// listed with their playlists, which every app changes as any playlist:
// its entries are movies and episodes, a series or a season added being
// its episodes as playlistTitles tells, each once. What its owner, or a
// user it is shared with to edit, adds goes on the owner's own list (see
// watchlistGroups). While it keeps its default name, it is named in the
// language of whoever sees it, with its owner's name for anyone else (see
// playlistName).

// Polyfin's own routes put titles on the user's watchlist and take them
// off, for Silo's apps (see package silo), the web client script and any
// app:
//
//	POST /Polyfin/UserItems/{itemId}/Watchlist puts a movie or an episode
//	  at the top of the user's watchlist, a series or a season its
//	  episodes, as the playlist routes add them, on the user's own list,
//	  even when another list holds them: a series none of whose episodes
//	  aired gets them once one airs, and waits as pending until then.
//	  DELETE takes a movie or an episode off, a series all its entries,
//	  a season its own entries only, a pending title its wait.
//	GET /Polyfin/UserItems/Watchlist gives the identifier of the user's
//	  watchlist, empty while they have none, and lists its entries,
//	  started or not, in its order: each item, an episode's SeriesId and
//	  SeasonId, and the latest date a list holding it gives; then the
//	  pending titles, latest first, each with Pending true.

func (h *Handler) watchlistRoutes(signedIn func(method, pattern string, handler http.HandlerFunc)) {
	signedIn(http.MethodPost, "/Polyfin/UserItems/{itemId}/Watchlist", h.watchlistTitle(true))
	signedIn(http.MethodDelete, "/Polyfin/UserItems/{itemId}/Watchlist", h.watchlistTitle(false))
	signedIn(http.MethodGet, "/Polyfin/UserItems/Watchlist", h.watchlistHeld)
}

// watchlistTitle answers POST and DELETE /Polyfin/UserItems/{itemId}/Watchlist
// with 204, and 400 for an item that is neither a movie, a series, a
// season nor an episode.
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
		switch item.Kind {
		case library.KindMovie, library.KindEpisode, library.KindSeries, library.KindSeason:
		default:
			badRequestProblem(w)
			return
		}
		var change library.WatchlistChange
		var err error
		if add {
			var groups []watchlist.Group
			if groups, err = h.watchlistGroups(r.Context(), user, []library.Item{item}); err == nil {
				change, err = h.Library.AddToWatchlist(r.Context(), user.ID, groups, 0)
			}
		} else {
			change, err = h.removeFromWatchlist(r.Context(), user, item)
		}
		h.watchlistEdited(user, user.ID, change)
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		h.userDataChanged(user, []library.Item{item})
		w.WriteHeader(http.StatusNoContent)
	}
}

// removeFromWatchlist takes item off user's watchlist: a movie or an
// episode itself, a series all its episodes, a season its own.
func (h *Handler) removeFromWatchlist(ctx context.Context, user accounts.User, item library.Item) (library.WatchlistChange, error) {
	switch item.Kind {
	case library.KindSeries:
		return h.Library.RemoveFromWatchlist(ctx, user.ID, nil, []accounts.ID{item.ID})
	case library.KindSeason:
		episodes, err := h.Library.Episodes(ctx, user, item.SeriesID, &item.ID)
		if err != nil {
			return library.WatchlistChange{}, err
		}
		ids := make([]accounts.ID, len(episodes))
		for i, e := range episodes {
			ids[i] = e.ID
		}
		return h.Library.RemoveFromWatchlist(ctx, user.ID, ids, nil)
	}
	return h.Library.RemoveFromWatchlist(ctx, user.ID, []accounts.ID{item.ID}, nil)
}

// WatchlistTitleDto is an entry of the user's watchlist: its item, a
// movie or an episode, an episode's SeriesId and SeasonId, and when it
// was added. A title whose entries are not made yet, such as a series
// none of whose episodes aired, is listed with Pending, after the
// entries.
type WatchlistTitleDto struct {
	Id       string
	SeriesId string `json:",omitempty"`
	SeasonId string `json:",omitempty"`
	Pending  bool   `json:",omitempty"`
	AddedAt  time.Time
}

// WatchlistDto is the user's watchlist: its PlaylistId, empty while they
// have none, its entries, then its pending titles.
type WatchlistDto struct {
	PlaylistId string
	Items      []WatchlistTitleDto
}

// watchlistHeld answers GET /Polyfin/UserItems/Watchlist.
func (h *Handler) watchlistHeld(w http.ResponseWriter, r *http.Request) {
	user, ok := h.viewer(w, r, bindErrors{}, notFoundProblem)
	if !ok {
		return
	}
	answer := WatchlistDto{Items: []WatchlistTitleDto{}}
	if user.ID != (accounts.ID{}) {
		playlist, exists, err := h.Library.Watchlist(r.Context(), user.ID)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		held, err := h.Library.WatchlistHeld(r.Context(), user.ID)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		if exists {
			answer.PlaylistId = playlist.String()
		}
		seasons, err := h.watchlistSeasons(r.Context(), user, held)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		shown := map[accounts.ID]bool{}
		for _, entry := range held {
			shown[entry.ID], shown[entry.Series] = true, true
			dto := WatchlistTitleDto{Id: entry.ID.String(), AddedAt: entry.AddedAt.UTC()}
			if entry.Series != (accounts.ID{}) {
				dto.SeriesId = entry.Series.String()
			}
			if season, ok := seasons[entry.ID]; ok {
				dto.SeasonId = season.String()
			}
			answer.Items = append(answer.Items, dto)
		}
		pending, err := h.Library.WatchlistPending(r.Context(), user.ID)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		// A title with entries already is listed by them.
		for _, title := range pending {
			if !shown[title.ID] {
				answer.Items = append(answer.Items, WatchlistTitleDto{Id: title.ID.String(), Pending: true, AddedAt: title.AddedAt.UTC()})
			}
		}
	}
	writeJSON(w, http.StatusOK, answer)
}

// watchlistSeasons maps the episodes among held that user can reach to
// their season.
func (h *Handler) watchlistSeasons(ctx context.Context, user accounts.User, held []watchlist.Held) (map[accounts.ID]accounts.ID, error) {
	var episodes []accounts.ID
	for _, entry := range held {
		if entry.Series != (accounts.ID{}) {
			episodes = append(episodes, entry.ID)
		}
	}
	seasons := map[accounts.ID]accounts.ID{}
	if len(episodes) == 0 {
		return seasons, nil
	}
	items, err := h.Library.Items(ctx, user, episodes)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.Kind == library.KindEpisode && item.SeasonID != (accounts.ID{}) {
			seasons[item.ID] = item.SeasonID
		}
	}
	return seasons, nil
}

// watchlistGroups lists what items put on user's watchlist, in order (see
// watchlist.Group): a movie or an episode itself, a series its episodes
// as playlistTitles tells, on the user's own list as a series, and a
// season each of its episodes as titles of their own. Other items are
// left out.
func (h *Handler) watchlistGroups(ctx context.Context, user accounts.User, items []library.Item) ([]watchlist.Group, error) {
	var groups []watchlist.Group
	for _, item := range items {
		switch item.Kind {
		case library.KindMovie:
			groups = append(groups, watchlist.Group{Title: item.ID, Items: []accounts.ID{item.ID}})
		case library.KindEpisode:
			groups = append(groups, watchlist.Group{Title: item.ID, Series: item.SeriesID, Items: []accounts.ID{item.ID}})
		case library.KindSeries, library.KindSeason:
			episodes, err := h.playlistTargets(ctx, user, item)
			if err != nil {
				return nil, err
			}
			if item.Kind == library.KindSeason {
				for _, e := range episodes {
					groups = append(groups, watchlist.Group{Title: e.ID, Series: e.SeriesID, Items: []accounts.ID{e.ID}})
				}
				continue
			}
			group := watchlist.Group{Title: item.ID, Series: item.ID}
			for _, e := range episodes {
				group.Items = append(group.Items, e.ID)
			}
			groups = append(groups, group)
		}
	}
	return groups, nil
}

// watchlistItems describes the items ids name that user can reach, in
// order, to put them on a watchlist (see watchlistGroups).
func (h *Handler) watchlistItems(ctx context.Context, user accounts.User, ids []accounts.ID) ([]watchlist.Group, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	items, err := h.Library.Items(ctx, user, ids)
	if err != nil {
		return nil, err
	}
	return h.watchlistGroups(ctx, user, items)
}

// playlistName is the name of p as viewer sees it. A watchlist that keeps
// its default name is named in viewer's language and, for anyone but its
// owner, followed by its owner's name, so that several users' watchlists
// never look alike.
func (h *Handler) playlistName(ctx context.Context, viewer accounts.User, p playlists.Playlist) (string, error) {
	if !p.Watchlist || p.Name != "" {
		return p.Name, nil
	}
	name, err := h.Library.WatchlistName(ctx, viewer)
	if err != nil || p.Owner == viewer.ID {
		return name, err
	}
	owner, err := h.Accounts.User(ctx, p.Owner)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s (%s)", name, owner.Name), nil
}
