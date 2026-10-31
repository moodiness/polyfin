package admin

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// EpisodeOrders shows and sets the order series show their episodes in,
// as the library keeps it (see library's episodeorder.go), and finds the
// series to set it for.
type EpisodeOrders interface {
	EpisodeOrders(ctx context.Context, user accounts.User) ([]library.OrderedSeries, error)
	EpisodeOrderOf(ctx context.Context, user accounts.User, id accounts.ID) (library.Item, library.EpisodeOrder, error)
	SetEpisodeOrder(ctx context.Context, user accounts.User, id accounts.ID, group, specials string) error
	Search(ctx context.Context, user accounts.User, term string, kinds []library.Kind, limit int) ([]library.Item, error)
}

// maxOrderedSeriesResults bounds the series a search for one to order
// lists.
const maxOrderedSeriesResults = 20

// orderedSeriesJSON is a series as Content › Libraries lists it under
// Episode orders.
type orderedSeriesJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Year int    `json:"year"`
}

// episodeGroupJSON is one of a series' TMDB episode groups: its type is
// Jellyfin's name of it (library.DisplayOrderOf).
type episodeGroupJSON struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Episodes int    `json:"episodes"`
	Seasons  int    `json:"seasons"`
}

// episodeOrderJSON is a series' order of episodes. Order is Jellyfin's
// DisplayOrder; Group the group an administrator chose, empty for the
// first of Order's type; Lookup why Groups is empty when TMDB could not
// tell (see library.EpisodeOrder).
type episodeOrderJSON struct {
	Series   orderedSeriesJSON  `json:"series"`
	Order    string             `json:"order"`
	Group    string             `json:"group"`
	Specials string             `json:"specials"`
	Groups   []episodeGroupJSON `json:"groups"`
	Lookup   string             `json:"lookup"`
}

func newOrderedSeriesJSON(item library.Item) orderedSeriesJSON {
	return orderedSeriesJSON{ID: item.ID.String(), Name: item.Name, Year: item.ProductionYear}
}

func newEpisodeOrderJSON(series library.Item, order library.EpisodeOrder) episodeOrderJSON {
	result := episodeOrderJSON{Series: newOrderedSeriesJSON(series), Order: order.DisplayOrder, Group: order.Group,
		Specials: order.Specials, Groups: []episodeGroupJSON{}, Lookup: order.Lookup}
	for _, g := range order.Groups {
		result.Groups = append(result.Groups, episodeGroupJSON{ID: g.ID, Name: g.Name, Type: library.DisplayOrderOf(g.Type),
			Episodes: g.Episodes, Seasons: g.Groups})
	}
	return result
}

// episodeOrderError answers the errors of choosing an order; it reports
// whether it did.
func episodeOrderError(w http.ResponseWriter, err error) bool {
	for _, known := range []struct {
		err    error
		status int
		code   string
	}{
		{library.ErrNotFound, http.StatusNotFound, "not_found"},
		{library.ErrUnknownGroup, http.StatusBadRequest, "unknown_episode_group"},
		{library.ErrInvalidSpecials, http.StatusBadRequest, "invalid_request"},
		{tmdb.ErrNoKey, http.StatusConflict, "no_tmdb_key"},
		{library.ErrGroupLookup, http.StatusBadGateway, "episode_groups_unreachable"},
	} {
		if errors.Is(err, known.err) {
			writeError(w, known.status, known.code)
			return true
		}
	}
	return false
}

// episodeOrders lists the series shown in an order of their own, or with
// their specials elsewhere than first.
func (h *handler) episodeOrders(w http.ResponseWriter, r *http.Request) {
	result := []episodeOrderJSON{}
	if h.EpisodeOrders != nil {
		ordered, err := h.EpisodeOrders.EpisodeOrders(r.Context(), sessionFrom(r.Context()).User)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		for _, o := range ordered {
			result = append(result, newEpisodeOrderJSON(o.Series, o.Order))
		}
	}
	writeJSON(w, http.StatusOK, result)
}

// searchOrderedSeries searches the series the administrator sees, to
// choose one's order.
func (h *handler) searchOrderedSeries(w http.ResponseWriter, r *http.Request) {
	result := []orderedSeriesJSON{}
	term := strings.TrimSpace(r.URL.Query().Get("search"))
	if term == "" || h.EpisodeOrders == nil {
		writeJSON(w, http.StatusOK, result)
		return
	}
	items, err := h.EpisodeOrders.Search(r.Context(), sessionFrom(r.Context()).User, term, []library.Kind{library.KindSeries},
		maxOrderedSeriesResults)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	for _, item := range items[:min(len(items), maxOrderedSeriesResults)] {
		result = append(result, newOrderedSeriesJSON(item))
	}
	writeJSON(w, http.StatusOK, result)
}

// episodeOrder describes a series' order, with the TMDB episode groups it
// may show in.
func (h *handler) episodeOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if h.EpisodeOrders == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	series, order, err := h.EpisodeOrders.EpisodeOrderOf(r.Context(), sessionFrom(r.Context()).User, id)
	if episodeOrderError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newEpisodeOrderJSON(series, order))
}

// setEpisodeOrder chooses a series' TMDB episode group, none for the
// aired order, and where its specials show, and answers its order.
func (h *handler) setEpisodeOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Group    string `json:"group"`
		Specials string `json:"specials"`
	}
	if !decode(w, r, &body) {
		return
	}
	if h.EpisodeOrders == nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	user := sessionFrom(r.Context()).User
	err := h.EpisodeOrders.SetEpisodeOrder(r.Context(), user, id, body.Group, body.Specials)
	if err == nil {
		var series library.Item
		var order library.EpisodeOrder
		if series, order, err = h.EpisodeOrders.EpisodeOrderOf(r.Context(), user, id); err == nil {
			writeJSON(w, http.StatusOK, newEpisodeOrderJSON(series, order))
			return
		}
	}
	if !episodeOrderError(w, err) {
		h.internalError(w, r, err)
	}
}
