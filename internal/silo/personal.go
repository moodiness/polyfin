package silo

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/library"
)

// Favorites, the watchlist and the watch history.
//
// A user's favorites are their Jellyfin favorites, which all their apps
// share. Their watchlist is Polyfin's (see package watchlist): Silo's apps
// list its Watchlist view, as Jellyfin apps show it, and put titles on it
// and take them off through Polyfin's own routes of the Jellyfin API. Their
// history is what they played. Each is read from the Jellyfin API as the
// user, under the same limits.

func (s *Server) personalRoutes() {
	s.mux.HandleFunc("GET /api/v2/favorites", s.profiled(s.favorites))
	s.mux.HandleFunc("GET /api/v2/favorites/{id}", s.profiled(s.membership(favoriteEntry, false)))
	s.mux.HandleFunc("GET /api/v2/favorites/{id}/membership", s.profiled(s.membership(favoriteEntry, true)))
	s.mux.HandleFunc("PUT /api/v2/favorites/{id}", s.profiled(s.markFavorite(true)))
	s.mux.HandleFunc("DELETE /api/v2/favorites/{id}", s.profiled(s.markFavorite(false)))
	s.mux.HandleFunc("GET /api/v2/watchlist", s.profiled(s.watchlist))
	s.mux.HandleFunc("GET /api/v2/watchlist/{id}", s.profiled(s.membership(watchlistEntry, false)))
	s.mux.HandleFunc("GET /api/v2/watchlist/{id}/membership", s.profiled(s.membership(watchlistEntry, true)))
	s.mux.HandleFunc("PUT /api/v2/watchlist/{id}", s.profiled(s.markWatchlist(true)))
	s.mux.HandleFunc("DELETE /api/v2/watchlist/{id}", s.profiled(s.markWatchlist(false)))
	s.mux.HandleFunc("GET /api/v2/history", s.profiled(s.history))
	// Polyfin keeps what was played as played marks, which the apps take
	// off by marking titles unplayed.
	s.mux.HandleFunc("POST /api/v2/history/remove", s.profiled(unsupported("This server does not hide watches from the history.")))
}

// The user's watchlist, read once for a request when it first needs it:
// the cards it describes tell whether the watchlist holds them.

type watchlistKey struct{}

type heldTitles struct {
	once sync.Once
	read func() map[string]time.Time
	held map[string]time.Time
}

// withWatchlist is r, whose cards read v's watchlist.
func (s *Server) withWatchlist(r *http.Request, v viewer) *http.Request {
	h := &heldTitles{read: func() map[string]time.Time { return s.readWatchlist(r, v) }}
	return r.WithContext(context.WithValue(r.Context(), watchlistKey{}, h))
}

// readWatchlist reads the titles the user's watchlist holds, started or
// not, and when each was added. One that cannot be read holds nothing,
// for the cards: the watchlist's own operations read it again.
func (s *Server) readWatchlist(r *http.Request, v viewer) map[string]time.Time {
	held, err := s.heldWatchlist(r, v)
	if err != nil && r.Context().Err() == nil {
		s.options.Logger.Warn("The watchlist of a Silo app's user could not be read", "error", err)
	}
	return held
}

func (s *Server) heldWatchlist(r *http.Request, v viewer) (map[string]time.Time, error) {
	var result struct {
		Items []struct {
			Id      string
			AddedAt time.Time
		}
	}
	a, err := s.call(r, v, http.MethodGet, "/Polyfin/UserItems/Watchlist", nil, &result)
	if err == nil && !a.ok() {
		err = fmt.Errorf("the Jellyfin API answered %d", a.status)
	}
	if err != nil {
		return nil, err
	}
	held := make(map[string]time.Time, len(result.Items))
	for _, title := range result.Items {
		held[title.Id] = title.AddedAt
	}
	return held, nil
}

// watchlisted is the watchlist of r's user, by title.
func watchlisted(r *http.Request) map[string]time.Time {
	h, ok := r.Context().Value(watchlistKey{}).(*heldTitles)
	if !ok {
		return nil
	}
	h.once.Do(func() { h.held = h.read() })
	return h.held
}

// watchlistTitle is the title the watchlist holds for an item: a movie or
// a series itself, the series of a season or an episode; none for others.
func watchlistTitle(i item) string {
	switch i.Type {
	case "Movie", "Series":
		return i.Id
	case "Season", "Episode":
		return i.SeriesId
	}
	return ""
}

// inWatchlist reports whether r's user's watchlist holds an item's title.
func inWatchlist(r *http.Request, i item) bool {
	title := watchlistTitle(i)
	if title == "" {
		return false
	}
	_, ok := watchlisted(r)[title]
	return ok
}

// listEntry tells whether a list of the user's holds an item, and since
// when.
type listEntry func(r *http.Request, i item) (time.Time, bool)

// favoriteEntry tells whether an item is one of the user's favorites.
// Polyfin does not record when a title became one: its date is the day
// the title joined the server.
func favoriteEntry(_ *http.Request, i item) (time.Time, bool) {
	if i.UserData == nil || !i.UserData.IsFavorite {
		return time.Time{}, false
	}
	if i.DateCreated != nil && !i.DateCreated.IsZero() {
		return *i.DateCreated, true
	}
	return time.Now(), true
}

// watchlistEntry tells whether the user's watchlist holds an item's title.
func watchlistEntry(r *http.Request, i item) (time.Time, bool) {
	title := watchlistTitle(i)
	if title == "" {
		return time.Time{}, false
	}
	at, ok := watchlisted(r)[title]
	return at, ok
}

// membership answers whether a list holds an item, for an item the user
// may see: with always, as is_member; else the entry, or not_found.
func (s *Server) membership(entry listEntry, always bool) func(http.ResponseWriter, *http.Request, viewer) {
	return func(w http.ResponseWriter, r *http.Request, v viewer) {
		i, ok := s.itemOf(w, r, v, r.PathValue("id"))
		if !ok {
			return
		}
		at, member := entry(r, i)
		type answer struct {
			ItemID   string `json:"item_id"`
			IsMember *bool  `json:"is_member,omitempty"`
			AddedAt  string `json:"added_at,omitempty"`
		}
		result := answer{ItemID: i.Id}
		if member {
			result.AddedAt = instant(at)
		}
		switch {
		case always:
			result.IsMember = &member
		case !member:
			problem(w, http.StatusNotFound, "not_found", "The item is not on this list.")
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// markFavorite makes an item one of the user's favorites, or not, as
// Jellyfin apps do.
func (s *Server) markFavorite(favorite bool) func(http.ResponseWriter, *http.Request, viewer) {
	return func(w http.ResponseWriter, r *http.Request, v viewer) {
		i, ok := s.itemOf(w, r, v, r.PathValue("id"))
		if !ok {
			return
		}
		method := http.MethodPost
		if !favorite {
			method = http.MethodDelete
		}
		s.change(w, r, v, method, "/UserFavoriteItems/"+i.Id)
	}
}

// markWatchlist puts an item's title on the user's watchlist, or takes it
// off every list holding it. Only movies and series, or the seasons and
// episodes of a series, go on it.
func (s *Server) markWatchlist(add bool) func(http.ResponseWriter, *http.Request, viewer) {
	return func(w http.ResponseWriter, r *http.Request, v viewer) {
		i, ok := s.itemOf(w, r, v, r.PathValue("id"))
		if !ok {
			return
		}
		switch {
		case watchlistTitle(i) != "" && add:
			s.change(w, r, v, http.MethodPost, "/Polyfin/UserItems/"+i.Id+"/Watchlist")
		case watchlistTitle(i) != "":
			s.change(w, r, v, http.MethodDelete, "/Polyfin/UserItems/"+i.Id+"/Watchlist")
		case add:
			validationProblem(w, "path.item_id", "unsupported", "Only movies and series go on the watchlist.")
		default:
			// No list holds it: taking it off changes nothing.
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

// change asks the Jellyfin API for a change of the user's data, answering
// 204 once made.
func (s *Server) change(w http.ResponseWriter, r *http.Request, v viewer, method, target string) {
	a, err := s.call(r, v, method, target, nil, nil)
	switch {
	case err != nil:
		s.internalError(w, r, err)
	case !a.ok():
		s.jellyfinRefusal(w, r, a)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// personalPage is a page of a personal list: its cards, and whether more
// follow.
type personalPage struct {
	Items []catalogItem `json:"items"`
	Page  pageInfo      `json:"page"`
}

// personalList answers a page of a listing of the Jellyfin API.
func (s *Server) personalList(w http.ResponseWriter, r *http.Request, v viewer, query url.Values) {
	start, limit, ok := pageAsked(w, r.URL.Query())
	if !ok {
		return
	}
	query.Set("Fields", listFields)
	query.Set("StartIndex", strconv.Itoa(start))
	query.Set("Limit", strconv.Itoa(limit))
	query.Set("EnableTotalRecordCount", "true")
	page, a, err := s.listPage(r, v, query, start)
	switch {
	case err != nil:
		s.internalError(w, r, err)
	case !a.ok():
		s.jellyfinRefusal(w, r, a)
	default:
		writeJSON(w, http.StatusOK, personalPage{page.Items, page.Page})
	}
}

// favorites lists the user's favorite movies, series and episodes, by
// name, as Jellyfin apps list them: Polyfin does not record when each
// became a favorite.
func (s *Server) favorites(w http.ResponseWriter, r *http.Request, v viewer) {
	s.personalList(w, r, v, url.Values{"Filters": {"IsFavorite"}, "Recursive": {"true"}, "IncludeItemTypes": {"Movie,Series,Episode"},
		"SortBy": {"SortName"}, "SortOrder": {"Ascending"}})
}

// watchlistView identifies the Watchlist view, the same on every server.
var watchlistView = library.WatchlistViewID.String()

// watchlist lists the Watchlist view: the movies and series of the user's
// watchlist they have not started, the latest added first.
func (s *Server) watchlist(w http.ResponseWriter, r *http.Request, v viewer) {
	s.personalList(w, r, v, url.Values{"ParentId": {watchlistView}})
}

// historyQuery is the Jellyfin API's listing of what the user played, of
// the types asked, the latest played first.
func historyQuery(types string) url.Values {
	return url.Values{"Filters": {"IsPlayed"}, "Recursive": {"true"}, "IncludeItemTypes": {types}, "SortBy": {"DatePlayed"},
		"SortOrder": {"Descending"}}
}

// historyTypes are the Jellyfin API's types of a history's media scope:
// movies and episodes, which are what is played.
var historyTypes = map[string]string{"": "Movie,Episode", "video": "Movie,Episode", "video_with_episodes": "Movie,Episode",
	"movie": "Movie", "episode": "Episode", "series": "Episode"}

// historyWatch is when a title of the history was played.
type historyWatch struct {
	MediaItemID     string  `json:"media_item_id"`
	WatchedAt       string  `json:"watched_at"`
	DurationSeconds float64 `json:"duration_seconds"`
	Completed       bool    `json:"completed"`
}

// history lists what the user played, the latest first, each card with
// when it was played.
func (s *Server) history(w http.ResponseWriter, r *http.Request, v viewer) {
	start, limit, ok := pageAsked(w, r.URL.Query())
	if !ok {
		return
	}
	query := historyQuery(historyTypes[""])
	query.Set("Fields", listFields)
	query.Set("StartIndex", strconv.Itoa(start))
	query.Set("Limit", strconv.Itoa(limit))
	query.Set("EnableTotalRecordCount", "true")
	var result queryResult
	a, err := s.call(r, v, http.MethodGet, "/Items?"+query.Encode(), nil, &result)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !a.ok() {
		s.jellyfinRefusal(w, r, a)
		return
	}
	type historyCard struct {
		catalogItem
		Watch historyWatch `json:"watch"`
	}
	cards := []historyCard{}
	for _, i := range result.Items {
		if i.Type != "Movie" && i.Type != "Episode" && i.Type != "Video" {
			continue
		}
		watched := time.Now()
		if i.UserData != nil && i.UserData.LastPlayedDate != nil {
			watched = *i.UserData.LastPlayedDate
		}
		cards = append(cards, historyCard{catalogItemOf(r, i), historyWatch{MediaItemID: i.Id, WatchedAt: instant(watched),
			DurationSeconds: seconds(i.RunTimeTicks), Completed: true}})
	}
	next := pageInfo{}
	if total := max(result.TotalRecordCount, start+len(result.Items)); len(result.Items) > 0 && start+len(result.Items) < total {
		next = pageInfo{HasMore: true, NextCursor: cursor(start + len(result.Items))}
	}
	writeJSON(w, http.StatusOK, struct {
		Items []historyCard `json:"items"`
		Page  pageInfo      `json:"page"`
	}{cards, next})
}
