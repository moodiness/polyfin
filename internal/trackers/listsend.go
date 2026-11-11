package trackers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The watchlist changes a user makes themselves go to Trakt's watchlist,
// MDBList's and Simkl's Plan to Watch as PublicMetaDB's (see listsync.go),
// all the titles of one change in one request: a movie as itself, a series
// as itself. Those the service does not know are skipped. Trakt refuses an
// add to a full watchlist with 420: the change is dropped, and the
// connection tells it until a change goes through (see watchlistFull).
//
// Simkl keeps each title in one of its lists only: adding one to Plan to
// Watch moves it there, and removing one deletes it from the user's
// library, with its history and rating. So their statuses are asked
// first: a title Simkl says plainly the user does not have is added, one
// only in Plan to Watch and never watched is removed, and the others,
// whatever Simkl answers of them, are left as they are.

// statusWatchlistFull is Trakt's answer to an add to a full watchlist,
// "Account Limit Exceeded".
const statusWatchlistFull = 420

// watchlistItem is a movie or show of a watchlist change for Trakt, Simkl
// and MDBList: by its identifiers, and, for Simkl, the list it goes to.
type watchlistItem struct {
	IDs ids    `json:"ids"`
	To  string `json:"to,omitempty"`
}

// watchlistBody is a watchlist change for Trakt, Simkl and MDBList.
type watchlistBody struct {
	Movies []watchlistItem `json:"movies,omitempty"`
	Shows  []watchlistItem `json:"shows,omitempty"`
}

// add adds title to b, going to list to.
func (b *watchlistBody) add(title Title, to string) {
	item := watchlistItem{IDs: title.ids(), To: to}
	if title.Series {
		b.Shows = append(b.Shows, item)
	} else {
		b.Movies = append(b.Movies, item)
	}
}

// listPaths are where Trakt and MDBList take additions to the watchlist
// and removals from it.
var listPaths = map[string][2]string{
	Trakt:   {"/sync/watchlist", "/sync/watchlist/remove"},
	MDBList: {"/watchlist/items/add", "/watchlist/items/remove"},
}

// sendList sends a watchlist change of kind, the titles of ev, to Trakt,
// Simkl or MDBList for the connection holding token.
func (s *Service) sendList(ctx context.Context, service, token string, settings accounts.Settings, kind string, ev event) result {
	if service == Simkl {
		return s.sendSimklList(ctx, token, settings, kind, ev)
	}
	var body watchlistBody
	for _, title := range ev.Titles {
		body.add(title, "")
	}
	path := listPaths[service][0]
	if kind == kindListRemove {
		path = listPaths[service][1]
	}
	r, err := s.api(ctx, service, token, settings, http.MethodPost, path, nil, body)
	if err != nil {
		return result{outcome: retry}
	}
	if service == Trakt && r.status == statusWatchlistFull {
		return result{outcome: refused, status: r.status, full: true}
	}
	return classify(service, r)
}

// simklStatus is what Simkl tells of a title of the user, each field as it
// came: result, whether the user has it, or "not_found" when Simkl does
// not know it; list, the list holding it; last_watched_at, when the user
// last watched it.
type simklStatus map[string]json.RawMessage

// null reports whether field came as null, or is absent when absent may
// be.
func (st simklStatus) null(field string, absent bool) bool {
	value, ok := st[field]
	return ok && string(value) == "null" || !ok && absent
}

// unlisted reports whether Simkl tells plainly that the user does not have
// the title: not theirs, in no list and never watched, or not known, with
// nothing else told. Any other answer leaves the title as it is.
func (st simklStatus) unlisted() bool {
	switch string(st["result"]) {
	case "false":
		return st.null("list", false) && st.null("last_watched_at", false)
	case `"not_found"`:
		// Simkl tells nothing more of a title it does not know.
		return st.null("list", true) && st.null("last_watched_at", true)
	}
	return false
}

// planned reports whether Simkl tells that the title is only in the
// user's Plan to Watch, never watched.
func (st simklStatus) planned() bool {
	var list string
	result := string(st["result"])
	return (result == "true" || result == "false") && json.Unmarshal(st["list"], &list) == nil && list == "plantowatch" &&
		st.null("last_watched_at", true)
}

// sendSimklList adds the titles of ev that Simkl does not list to Plan to
// Watch, or takes off those only in Plan to Watch and never watched.
func (s *Service) sendSimklList(ctx context.Context, token string, settings accounts.Settings, kind string, ev event) result {
	type lookup struct {
		IDs  ids    `json:"ids"`
		Type string `json:"type"`
	}
	asked := make([]lookup, len(ev.Titles))
	for i, title := range ev.Titles {
		asked[i] = lookup{IDs: title.ids(), Type: "movie"}
		if title.Series {
			asked[i].Type = "show"
		}
	}
	r, err := s.api(ctx, Simkl, token, settings, http.MethodPost, "/sync/watched", nil, asked)
	if err != nil {
		return result{outcome: retry}
	}
	if res := classify(Simkl, r); res.outcome != delivered {
		return res
	}
	var statuses []simklStatus
	// No titles is null.
	if json.Unmarshal(r.body, &statuses) != nil || len(statuses) != len(ev.Titles) {
		return result{outcome: retry, status: r.status}
	}
	var body watchlistBody
	path := "/sync/add-to-list"
	if kind == kindListRemove {
		path = "/sync/history/remove"
	}
	for i, title := range ev.Titles {
		switch {
		case kind == kindListAdd && statuses[i].unlisted():
			body.add(title, "plantowatch")
		case kind == kindListRemove && statuses[i].planned():
			body.add(title, "")
		}
	}
	if len(body.Movies)+len(body.Shows) == 0 {
		return result{outcome: delivered, status: r.status}
	}
	// The write waits its turn after the lookup (see writePace).
	r, err = s.api(ctx, Simkl, token, settings, http.MethodPost, path, nil, body)
	if err != nil {
		return result{outcome: retry}
	}
	return classify(Simkl, r)
}

// setWatchlistFull records whether the service of key refused the last
// title sent to its watchlist as the list is full.
func (s *Service) setWatchlistFull(ctx context.Context, key laneKey, full bool) {
	if _, err := s.db.Exec(ctx, "UPDATE tracking_imports SET watchlist_full = $3 WHERE user_id = $1 AND service = $2 AND watchlist_full <> $3",
		key.user, key.service, full); err != nil && ctx.Err() == nil {
		s.logger.Warn("Whether a watchlist is full could not be recorded", "service", key.service, "error", err)
	}
}
