package silo

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Not interested.
//
// A user marks a movie or a series Not interested from Silo's apps: it
// then leaves their For You and similar titles in those apps, the
// recommendations Polyfin answers them, at once. Their library, searches,
// a title's page, playback, favorites and the watchlist still show it,
// and Jellyfin apps know nothing of the mark. Polyfin keeps the marks of
// each user, the one profile Silo's apps know them by, in its database.
// Silo's server also ranks the titles similar to a marked one lower;
// Polyfin's recommendations are ranked by the Jellyfin API, which does not
// know the marks, so it does not.

func (s *Server) notInterestedRoutes() {
	s.mux.HandleFunc("GET /api/v2/capabilities/not-interested", s.profiled(s.notInterestedCapability))
	s.mux.HandleFunc("GET /api/v2/not-interested", s.profiled(s.notInterestedList))
	s.mux.HandleFunc("GET /api/v2/not-interested/{id}", s.profiled(s.notInterestedEntry))
	s.mux.HandleFunc("PUT /api/v2/not-interested/{id}", s.profiled(s.markNotInterested(true)))
	s.mux.HandleFunc("DELETE /api/v2/not-interested/{id}", s.profiled(s.markNotInterested(false)))
}

// notInterestedCapability tells the apps users may mark titles.
func (s *Server) notInterestedCapability(w http.ResponseWriter, _ *http.Request, _ viewer) {
	writeJSON(w, http.StatusOK, map[string]any{"state": "available", "allowed": true, "revision": revision})
}

// markable tells whether an item takes the mark: a movie, a video the
// apps show as one, or a series. A series' mark covers its episodes,
// which take none of their own.
func markable(i item) bool {
	return i.Type == "Movie" || i.Type == "Video" || i.Type == "Series"
}

// markNotInterested marks a movie or a series, or clears its mark. A title
// marked already keeps the time it was first marked; clearing a title not
// marked, or an episode or a season, which none are, changes nothing.
func (s *Server) markNotInterested(mark bool) func(http.ResponseWriter, *http.Request, viewer) {
	return func(w http.ResponseWriter, r *http.Request, v viewer) {
		i, ok := s.itemOf(w, r, v, r.PathValue("id"))
		if !ok {
			return
		}
		id, err := accounts.ParseID(i.Id)
		switch {
		case err != nil:
			s.internalError(w, r, err)
			return
		case !markable(i) && mark:
			validationProblem(w, "path.item_id", "unsupported", "Only movies and series are marked Not interested; mark the series.")
			return
		case !markable(i):
		case mark:
			_, err = s.options.DB.Exec(r.Context(), `INSERT INTO silo_not_interested (user_id, item_id, marked_at) VALUES ($1, $2, $3)
				ON CONFLICT (user_id, item_id) DO NOTHING`, v.user.ID, id, s.options.now())
		default:
			_, err = s.options.DB.Exec(r.Context(), "DELETE FROM silo_not_interested WHERE user_id = $1 AND item_id = $2", v.user.ID, id)
		}
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// notInterestedEntry answers the mark of a title the user may see, or
// not_found when it is not marked.
func (s *Server) notInterestedEntry(w http.ResponseWriter, r *http.Request, v viewer) {
	i, ok := s.itemOf(w, r, v, r.PathValue("id"))
	if !ok {
		return
	}
	got := marksOf(r)
	if got.err != nil {
		s.internalError(w, r, got.err)
		return
	}
	at, marked := got.at[markKey(i.Id)]
	if !marked {
		problem(w, http.StatusNotFound, "not_found", "The item is not marked Not interested.")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		ItemID   string `json:"item_id"`
		MarkedAt string `json:"marked_at"`
	}{i.Id, instant(at)})
}

// notInterestedList lists the titles the user marked as cards, the latest
// marked first, a page of marks at a time: those the user may no longer
// see are left out of their page.
func (s *Server) notInterestedList(w http.ResponseWriter, r *http.Request, v viewer) {
	start, limit, ok := pageAsked(w, r.URL.Query())
	if !ok {
		return
	}
	got := marksOf(r)
	if got.err != nil {
		s.internalError(w, r, got.err)
		return
	}
	result := personalPage{Items: []catalogItem{}}
	if start >= len(got.order) {
		writeJSON(w, http.StatusOK, result)
		return
	}
	ids := got.order[start:min(start+limit, len(got.order))]
	var found queryResult
	a, err := s.call(r, v, http.MethodGet, "/Items?"+url.Values{"Ids": {strings.Join(ids, ",")}, "Fields": {listFields}}.Encode(), nil, &found)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !a.ok() {
		s.jellyfinRefusal(w, r, a)
		return
	}
	byID := make(map[string]item, len(found.Items))
	for _, i := range found.Items {
		if markable(i) {
			byID[markKey(i.Id)] = i
		}
	}
	for _, id := range ids {
		if i, seen := byID[id]; seen {
			result.Items = append(result.Items, catalogItemOf(r, i))
		}
	}
	if next := start + len(ids); next < len(got.order) {
		result.Page = pageInfo{HasMore: true, NextCursor: cursor(next)}
	}
	writeJSON(w, http.StatusOK, result)
}

// The user's marks, read once for a request when it first needs it: the
// cards tell whether a title is marked, and the recommendations leave the
// marked ones out.

type marksKey struct{}

type heldMarks struct {
	once sync.Once
	read func() marksRead
	got  marksRead
}

// marksRead are the titles the user marked, by the Jellyfin API's
// identifier, with when, and in the order they were marked, the latest
// first. Marks that cannot be read are none, and err tells why.
type marksRead struct {
	at    map[string]time.Time
	order []string
	err   error
}

// withMarks is r, whose cards and recommendations read v's marks.
func (s *Server) withMarks(r *http.Request, v viewer) *http.Request {
	h := &heldMarks{read: func() marksRead { return s.readMarks(r.Context(), v) }}
	return r.WithContext(context.WithValue(r.Context(), marksKey{}, h))
}

func (s *Server) readMarks(ctx context.Context, v viewer) marksRead {
	rows, err := s.options.DB.Query(ctx, `SELECT item_id, marked_at FROM silo_not_interested WHERE user_id = $1
		ORDER BY marked_at DESC, item_id`, v.user.ID)
	type mark struct {
		ID accounts.ID
		At time.Time
	}
	var marks []mark
	if err == nil {
		marks, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (mark, error) {
			var m mark
			err := row.Scan(&m.ID, &m.At)
			return m, err
		})
	}
	if err != nil {
		if ctx.Err() == nil {
			s.options.Logger.Warn("The Not interested marks of a Silo app's user could not be read", "error", err)
		}
		return marksRead{err: fmt.Errorf("reading the Not interested marks: %w", err)}
	}
	got := marksRead{at: make(map[string]time.Time, len(marks)), order: make([]string, 0, len(marks))}
	for _, m := range marks {
		id := hyphenless(m.ID)
		got.at[id] = m.At.UTC()
		got.order = append(got.order, id)
	}
	return got
}

// hyphenless is an identifier as the Jellyfin API writes an item's.
func hyphenless(id accounts.ID) string {
	return strings.ReplaceAll(id.String(), "-", "")
}

// markKey is the key of the marks of an item, by the Jellyfin API's
// identifier, in any of the forms accounts.ParseID reads.
func markKey(id string) string {
	parsed, err := accounts.ParseID(id)
	if err != nil {
		return id
	}
	return hyphenless(parsed)
}

// marksOf are the marks of r's user.
func marksOf(r *http.Request) marksRead {
	h, ok := r.Context().Value(marksKey{}).(*heldMarks)
	if !ok {
		return marksRead{}
	}
	h.once.Do(func() { h.got = h.read() })
	return h.got
}

// notInterested reports whether r's user marked an item.
func notInterested(r *http.Request, i item) bool {
	if !markable(i) {
		return false
	}
	_, ok := marksOf(r).at[markKey(i.Id)]
	return ok
}

// unmarked are the cards of the titles r's user did not mark, a
// recommendation's. Marks that cannot be read leave out none.
func unmarked(r *http.Request, cards []catalogItem) []catalogItem {
	got := marksOf(r)
	if len(got.at) == 0 {
		return cards
	}
	return slices.DeleteFunc(cards, func(c catalogItem) bool {
		_, marked := got.at[markKey(c.ContentID)]
		return marked && (c.Type == "movie" || c.Type == "series")
	})
}

// headroom is how many more titles a recommendation of limit asks for, so
// that one with marked titles left out still fills: as many as the user
// marked, up to limit more.
func headroom(r *http.Request, limit int) int {
	return min(len(marksOf(r).at), limit)
}
