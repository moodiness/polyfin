package silo

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// Favorites, the watchlist and the watch history.
//
// A user's favorites are their Jellyfin favorites, which all their apps
// share. Their watchlist is Polyfin's, a playlist of theirs (see package
// watchlist): Silo's apps list it as Jellyfin apps do, in its order, and
// put titles on it and take them off through Polyfin's own routes of the
// Jellyfin API. Their history is what they played. Each is read from the
// Jellyfin API as the user, under the same limits.

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
// the cards it describes tell whether the watchlist holds them, and its
// listings name its playlist.

type watchlistKey struct{}

type heldTitles struct {
	once sync.Once
	read func() watchlistRead
	got  watchlistRead
}

// watchlistRead is the user's watchlist: its playlist's identifier, empty
// while the user has none, its entries (movies and episodes), started or
// not, and the titles it holds pending (a series none of whose episodes
// aired), with when each was added, and the seasons and series of its
// episodes, with the latest of their dates. One that cannot be read holds
// nothing, and err tells why.
type watchlistRead struct {
	playlist string
	held     map[string]time.Time
	seasons  map[string]time.Time
	series   map[string]time.Time
	err      error
}

// withWatchlist is r, whose cards read v's watchlist.
func (s *Server) withWatchlist(r *http.Request, v viewer) *http.Request {
	h := &heldTitles{read: func() watchlistRead { return s.readWatchlist(r, v) }}
	return r.WithContext(context.WithValue(r.Context(), watchlistKey{}, h))
}

// readWatchlist reads the user's watchlist.
func (s *Server) readWatchlist(r *http.Request, v viewer) watchlistRead {
	var result struct {
		PlaylistId string
		Items      []struct {
			Id       string
			SeriesId string
			SeasonId string
			AddedAt  time.Time
		}
	}
	a, err := s.call(r, v, http.MethodGet, "/Polyfin/UserItems/Watchlist", nil, &result)
	if err == nil && !a.ok() {
		err = fmt.Errorf("the Jellyfin API answered %d", a.status)
	}
	if err != nil {
		if r.Context().Err() == nil {
			s.options.Logger.Warn("The watchlist of a Silo app's user could not be read", "error", err)
		}
		return watchlistRead{err: err}
	}
	got := watchlistRead{playlist: result.PlaylistId, held: make(map[string]time.Time, len(result.Items)),
		seasons: map[string]time.Time{}, series: map[string]time.Time{}}
	latest := func(dates map[string]time.Time, id string, at time.Time) {
		if id != "" && at.After(dates[id]) {
			dates[id] = at
		}
	}
	for _, entry := range result.Items {
		got.held[entry.Id] = entry.AddedAt
		latest(got.seasons, entry.SeasonId, entry.AddedAt)
		latest(got.series, entry.SeriesId, entry.AddedAt)
	}
	return got
}

// watchlistOf is the watchlist of r's user.
func watchlistOf(r *http.Request) watchlistRead {
	h, ok := r.Context().Value(watchlistKey{}).(*heldTitles)
	if !ok {
		return watchlistRead{}
	}
	h.once.Do(func() { h.got = h.read() })
	return h.got
}

// watchable tells whether the watchlist's button acts on an item: a
// movie, an episode, a season or a series.
func watchable(i item) bool {
	switch i.Type {
	case "Movie", "Episode", "Season", "Series":
		return true
	}
	return false
}

// inWatchlist reports whether r's user's watchlist holds an item (see
// watchlistEntry).
func inWatchlist(r *http.Request, i item) bool {
	_, ok := watchlistEntry(r, i)
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

// watchlistEntry tells whether the user's watchlist holds an item, and
// since when: a title when it is an entry or pending, a season or a series
// also when an episode of it is.
func watchlistEntry(r *http.Request, i item) (time.Time, bool) {
	got := watchlistOf(r)
	if at, ok := got.held[i.Id]; ok {
		return at, true
	}
	var at time.Time
	var ok bool
	switch i.Type {
	case "Season":
		at, ok = got.seasons[i.Id]
	case "Series":
		at, ok = got.series[i.Id]
	}
	return at, ok
}

// watchlistFollows tells whether the user's watchlist holds an episode,
// its season or its series, pending or by an episode of it, for the
// calendar.
func watchlistFollows(r *http.Request, i item) bool {
	got := watchlistOf(r)
	for _, id := range []string{i.Id, i.SeasonId, i.SeriesId} {
		if _, ok := got.held[id]; ok && id != "" {
			return true
		}
	}
	_, ok := got.series[i.SeriesId]
	return ok && i.SeriesId != ""
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

// markWatchlist puts an item on the user's watchlist, or takes it off, as
// the web player's menu does: a movie or an episode itself; a season or a
// series its aired episodes (of regular seasons, for a series), or all
// its entries off. Other items do not go on it.
func (s *Server) markWatchlist(add bool) func(http.ResponseWriter, *http.Request, viewer) {
	return func(w http.ResponseWriter, r *http.Request, v viewer) {
		i, ok := s.itemOf(w, r, v, r.PathValue("id"))
		if !ok {
			return
		}
		switch {
		case watchable(i) && add:
			s.change(w, r, v, http.MethodPost, "/Polyfin/UserItems/"+i.Id+"/Watchlist")
		case watchable(i):
			s.change(w, r, v, http.MethodDelete, "/Polyfin/UserItems/"+i.Id+"/Watchlist")
		case add:
			validationProblem(w, "path.item_id", "unsupported", "Only movies, episodes, seasons and series go on the watchlist.")
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

// watchlist lists the user's watchlist: its movies and episodes, started
// or not, in its order; nothing while the user deleted its playlist.
func (s *Server) watchlist(w http.ResponseWriter, r *http.Request, v viewer) {
	got := watchlistOf(r)
	switch {
	case got.err != nil:
		s.internalError(w, r, got.err)
	case got.playlist == "":
		if _, _, ok := pageAsked(w, r.URL.Query()); ok {
			writeJSON(w, http.StatusOK, personalPage{Items: []catalogItem{}})
		}
	default:
		s.personalList(w, r, v, url.Values{"ParentId": {got.playlist}})
	}
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
