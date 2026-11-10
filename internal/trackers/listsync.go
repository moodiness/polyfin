package trackers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// A user who syncs their PublicMetaDB watchlist has it imported (see
// watchlist.go), and the titles they put on their Polyfin watchlist or
// take off it themselves sent to it: a movie as itself, an episode,
// season or series as its series, which PublicMetaDB lists alone. A
// series leaves only once none of its entries is left and it waits for
// none. Moving entries, watching a title or deleting the playlist sends
// nothing, nor do the imports, and turning the sync on sends nothing of
// what the watchlist holds. Titles PublicMetaDB cannot name by TMDB, such
// as an anime known only to an anime catalog, are not sent.

// The kinds of change sent to a watchlist.
const (
	kindListAdd    = "list_add"
	kindListRemove = "list_remove"
)

// WatchlistChange is what a user's own change of their watchlist made of
// its titles: the movies and series it newly lists, and those it no
// longer lists.
type WatchlistChange struct {
	// Titles identifies them (see Movie and Series), leaving out those the
	// services cannot know. It runs in the background.
	Titles func(context.Context) (added, removed []Title)
}

// Watchlist sends a change user made to their watchlist to the services
// they sync their watchlist with, in the background.
func (s *Service) Watchlist(user accounts.ID, change WatchlistChange) {
	if s == nil {
		return
	}
	s.later(user, func(ctx context.Context) { s.sendWatchlist(ctx, user, change) })
}

// sendWatchlist queues change for each service user syncs their watchlist
// with.
func (s *Service) sendWatchlist(ctx context.Context, user accounts.ID, c WatchlistChange) {
	services, err := s.watchlistSynced(ctx, user)
	if err != nil {
		s.logger.Warn("A watchlist change could not be sent to tracking services", "error", err)
		return
	}
	if len(services) == 0 {
		return
	}
	added, removed := c.Titles(ctx)
	for _, service := range services {
		for i, titles := range [][]Title{added, removed} {
			kind := kindListAdd
			if i == 1 {
				kind = kindListRemove
			}
			for _, t := range titles {
				// One known only to an anime catalog has no TMDB identifier.
				if t.Identified() {
					s.queue(ctx, user, service, change{kind: kind, durable: true, event: event{Titles: []Title{t}}})
				}
			}
		}
	}
}

// syncsWatchlist reports whether Polyfin sends the watchlist changes of
// user to service.
func syncsWatchlist(service string) bool {
	return service == PublicMetaDB
}

// watchlistSynced lists the services user syncs their watchlist with.
func (s *Service) watchlistSynced(ctx context.Context, user accounts.ID) ([]string, error) {
	rows, err := s.db.Query(ctx, `SELECT c.service FROM tracking_connections c
		JOIN tracking_imports i ON i.user_id = c.user_id AND i.service = c.service
		WHERE c.user_id = $1 AND i.watchlist AND c.problem IS DISTINCT FROM 'reconnect' ORDER BY c.service`, user)
	if err != nil {
		return nil, err
	}
	connected, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	settings := s.settings()
	var services []string
	for _, service := range connected {
		if syncsWatchlist(service) && Available(service, settings) {
			services = append(services, service)
		}
	}
	return services, nil
}

// stillSynced reports whether user still syncs their watchlist with
// service, for a change waiting to be sent.
func (s *Service) stillSynced(ctx context.Context, key laneKey) (bool, error) {
	var on bool
	err := s.db.QueryRow(ctx, "SELECT watchlist FROM tracking_imports WHERE user_id = $1 AND service = $2", key.user, key.service).Scan(&on)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return on, err
}

// publicMetaDBListItem is an entry of a PublicMetaDB list.
type publicMetaDBListItem struct {
	ID        string  `json:"id"`
	TMDB      flexInt `json:"tmdb_id"`
	MediaType string  `json:"media_type"`
}

// sendPublicMetaDBList adds the movie or series of ev to the PublicMetaDB
// watchlist of the user of key, creating the list on the first add when
// they have none, or takes it off.
func (s *Service) sendPublicMetaDBList(ctx context.Context, key string, settings accounts.Settings, kind string, ev event) result {
	title := ev.Titles[0]
	tmdb, res := s.publicMetaDBTMDB(ctx, key, settings, title)
	if tmdb == 0 {
		return res
	}
	add := kind == kindListAdd
	list, res := s.publicMetaDBWatchlist(ctx, key, settings, add)
	if list == "" {
		return res
	}
	path := "/api/external/lists/" + url.PathEscape(list) + "/items"
	if add {
		res = s.publicMetaDB(ctx, key, settings, http.MethodPost, path, nil, map[string]any{"tmdb_id": tmdb, "media_type": title.mediaType()})
	} else {
		var item string
		item, res = s.publicMetaDBListed(ctx, key, settings, path, tmdb, title.mediaType())
		if item == "" {
			return res
		}
		res = s.publicMetaDB(ctx, key, settings, http.MethodDelete, path+"/"+url.PathEscape(item), nil, nil)
		if res.status == http.StatusNotFound {
			// Taken off already.
			res.outcome = delivered
		}
		return res
	}
	if res.status == http.StatusNotFound {
		// The list is gone: it is looked for again on the next try.
		s.mu.Lock()
		delete(s.publicMetaDBLists, key)
		s.mu.Unlock()
		res.outcome = retry
	}
	return res
}

// publicMetaDBWatchlist returns the identifier of the PublicMetaDB
// watchlist of the user of key, remembered once found. When they have
// none, it creates it if create, else it returns none with the change
// delivered: there is nothing to take off.
func (s *Service) publicMetaDBWatchlist(ctx context.Context, key string, settings accounts.Settings, create bool) (string, result) {
	s.mu.Lock()
	list, ok := s.publicMetaDBLists[key]
	s.mu.Unlock()
	if ok {
		return list, result{}
	}
	for page := 1; list == ""; page++ {
		r, err := s.api(ctx, PublicMetaDB, key, settings, http.MethodGet, "/api/external/lists",
			url.Values{"page": {strconv.Itoa(page)}, "perPage": {"500"}}, nil)
		if err != nil {
			return "", result{outcome: retry}
		}
		if res := classify(PublicMetaDB, r); res.outcome != delivered {
			return "", res
		}
		var answer struct {
			Items []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			} `json:"items"`
			TotalPages int `json:"totalPages"`
		}
		if json.Unmarshal(r.body, &answer) != nil {
			return "", result{outcome: retry, status: r.status}
		}
		for _, l := range answer.Items {
			if l.Type == "watchlist" && l.ID != "" {
				list = l.ID
				break
			}
		}
		if len(answer.Items) == 0 || page >= answer.TotalPages {
			break
		}
	}
	if list == "" && !create {
		return "", result{outcome: delivered}
	}
	if list == "" {
		r, err := s.api(ctx, PublicMetaDB, key, settings, http.MethodPost, "/api/external/lists", nil,
			map[string]any{"name": "Watchlist", "type": "watchlist"})
		if err != nil {
			return "", result{outcome: retry}
		}
		if res := classify(PublicMetaDB, r); res.outcome != delivered {
			return "", res
		}
		// The list comes alone, or under list or item.
		var made struct {
			ID   string `json:"id"`
			List struct {
				ID string `json:"id"`
			} `json:"list"`
			Item struct {
				ID string `json:"id"`
			} `json:"item"`
		}
		_ = json.Unmarshal(r.body, &made)
		list = cmpOr(made.ID, cmpOr(made.List.ID, made.Item.ID))
		if list == "" {
			return "", result{outcome: retry, status: r.status}
		}
	}
	s.mu.Lock()
	s.publicMetaDBLists[key] = list
	s.mu.Unlock()
	return list, result{}
}

// publicMetaDBListed finds the entry of the list at path holding the
// title tmdb of mediaType, a page at a time. None comes with the change
// delivered: there is nothing to take off.
func (s *Service) publicMetaDBListed(ctx context.Context, key string, settings accounts.Settings, path string, tmdb int, mediaType string) (string, result) {
	for page := 1; ; page++ {
		r, err := s.api(ctx, PublicMetaDB, key, settings, http.MethodGet, path, url.Values{"page": {strconv.Itoa(page)}, "perPage": {"500"}}, nil)
		if err != nil {
			return "", result{outcome: retry}
		}
		if r.status == http.StatusNotFound {
			// The list is gone, and the title with it.
			s.mu.Lock()
			delete(s.publicMetaDBLists, key)
			s.mu.Unlock()
			return "", result{outcome: delivered, status: r.status}
		}
		if res := classify(PublicMetaDB, r); res.outcome != delivered {
			return "", res
		}
		var answer struct {
			Items      []publicMetaDBListItem `json:"items"`
			TotalPages int                    `json:"totalPages"`
		}
		if json.Unmarshal(r.body, &answer) != nil {
			return "", result{outcome: retry, status: r.status}
		}
		for _, item := range answer.Items {
			if int(item.TMDB) == tmdb && item.MediaType == mediaType && item.ID != "" {
				return item.ID, result{}
			}
		}
		if len(answer.Items) == 0 || page >= answer.TotalPages {
			return "", result{outcome: delivered, status: r.status}
		}
	}
}
