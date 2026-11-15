package trackers

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/importlinks"
	"github.com/moodiness/polyfin/internal/library"
)

// An import of the history also reads the shows the user dropped on the
// service, whole, once its activities tell they may have changed since
// the last read: Trakt's dropped shows, MDBList's, Simkl's Dropped shows
// and anime, and the Dropped entries of AniList's and MyAnimeList's
// lists, which are read whole with each import anyway. Their series are
// kept per user and service in tracker_dropped, with the date the
// service gives for the drop, which Simkl, AniList and MyAnimeList do
// not; package userdata leaves them out of Next Up, Upcoming and Continue
// Watching. A series no longer listed leaves at the next read. A read
// that fails changes nothing of them and is made again at the next
// import, without stopping the history's (see droppedErr). On hold is
// not dropped. Titles are found as the watchlist's are, a series only;
// those not found are left out without counting among the titles not
// found, as the history already counts the episodes it names. The series
// go when the history import is turned off or the service disconnected.

// droppedShow is a show the user dropped, and when, nil when the service
// does not tell.
type droppedShow struct {
	ref library.TitleRef
	at  *time.Time
}

// droppedRead is what an import read of the shows dropped: whole once it
// read them all, else only adding to those kept. cursor is where the
// service's activities stood for them: the next import reads them again
// only once that changed.
type droppedRead struct {
	shows  []droppedShow
	whole  bool
	cursor string
}

// add adds the show ref names as a series, dropped at at, unless it has no
// identifier Polyfin finds titles by.
func (d *droppedRead) add(ref library.TitleRef, at *time.Time) {
	if usable(ref) || ref.Anime != (library.AnimeRef{}) {
		d.shows = append(d.shows, droppedShow{ref: ref, at: at})
	}
}

// droppedDue reports whether the shows dropped are read: the service's
// activities for them, gate, moved since the last read, or the last
// import read none. An empty gate tells the user never dropped anything
// there: then nothing is read, and none is kept.
func (r *reader) droppedDue(gate string) (*droppedRead, bool) {
	if gate == "" {
		return &droppedRead{whole: true}, false
	}
	if r.droppedCursor != "" && gate == r.droppedCursor {
		return nil, false
	}
	return &droppedRead{whole: true, cursor: gate}, true
}

// droppedErr is what err, from reading the shows dropped, does to the
// import. A token refused, a service asking to wait too long, or the
// import stopping stop it, as anywhere in its read. Any other failure
// stops that read only, MDBList's dropped shows being in beta: the shows
// kept stay, to be read again at the next import, and the history is
// imported as read, without a problem.
func (r *reader) droppedErr(ctx context.Context, err error) error {
	if err == nil || ctx.Err() != nil || errors.Is(err, errTokenRefused) || errors.Is(err, errRateLimited) {
		return err
	}
	r.s.logger.Info("The shows dropped on a tracking service could not be read: those kept stay", "user_id", r.key.user.String(),
		"service", r.key.service, "problem", importProblem(err))
	return nil
}

// readTraktDropped reads Trakt's dropped shows, a page at a time, unless
// gate, when its last activities saw a show dropped or taken back, did
// not change since the last read. Trakt dates each drop.
func (r *reader) readTraktDropped(ctx context.Context, gate string) (*droppedRead, error) {
	read, due := r.droppedDue(gate)
	for page := 1; due; page++ {
		answer, err := r.get(ctx, "/users/hidden/dropped", url.Values{"type": {"show"}, "page": {strconv.Itoa(page)}, "limit": {"250"}})
		if err != nil {
			return nil, err
		}
		var hidden []struct {
			HiddenAt string     `json:"hidden_at"`
			Type     string     `json:"type"`
			Show     *titleJSON `json:"show"`
		}
		if json.Unmarshal(answer.body, &hidden) != nil {
			return nil, ErrUnreachable
		}
		for _, h := range hidden {
			if h.Type == "show" && h.Show != nil {
				read.add(h.Show.seriesRef(), date(h.HiddenAt))
			}
		}
		pages, _ := strconv.Atoi(answer.header.Get("X-Pagination-Page-Count"))
		if len(hidden) == 0 || page >= pages {
			break
		}
	}
	return read, nil
}

// readMDBListDropped reads MDBList's dropped shows, a page at a time,
// unless gate, when its last activities saw a show dropped or taken back,
// did not change since the last read. MDBList dates each drop; a show
// taken back may stay listed with no date.
func (r *reader) readMDBListDropped(ctx context.Context, gate string) (*droppedRead, error) {
	read, due := r.droppedDue(gate)
	const limit = 1000
	for offset := 0; due; offset += limit {
		answer, err := r.get(ctx, "/sync/dropped", url.Values{"offset": {strconv.Itoa(offset)}, "limit": {strconv.Itoa(limit)}})
		if err != nil {
			return nil, err
		}
		var page struct {
			Shows []struct {
				DroppedAt string    `json:"dropped_at"`
				Show      titleJSON `json:"show"`
			} `json:"shows"`
			Pagination struct {
				HasMore bool `json:"has_more"`
			} `json:"pagination"`
		}
		if json.Unmarshal(answer.body, &page) != nil {
			return nil, ErrUnreachable
		}
		for _, show := range page.Shows {
			if at := date(show.DroppedAt); at != nil {
				read.add(show.Show.seriesRef(), at)
			}
		}
		if !page.Pagination.HasMore || len(page.Shows) == 0 {
			break
		}
	}
	return read, nil
}

// readSimklDropped reads Simkl's Dropped shows and anime, by their
// identifiers only, unless the activities of its shows and anime, shows
// and animeAt, did not change since the last read. Anime is found through
// the anime mapping, the series an entry is part of dropped with it:
// until the mapping is read, the shows only are read, which add to those
// kept, and the next import reads them again. Simkl does not date the
// drop.
func (r *reader) readSimklDropped(ctx context.Context, shows, animeAt string) (*droppedRead, error) {
	mapped := r.s.anime.Ready()
	if !mapped {
		animeAt = ""
	}
	gate := ""
	if shows != "" || animeAt != "" {
		gate = shows + "|" + animeAt
	}
	read, due := r.droppedDue(gate)
	if !mapped {
		if !due {
			return nil, nil
		}
		read.whole, read.cursor = false, r.droppedCursor
	}
	if !due {
		return read, nil
	}
	mapping := animeEntry{mapping: r.s.anime, unknown: map[string]bool{}}
	for _, list := range []struct {
		kind string
		at   string
	}{{"shows", shows}, {"anime", animeAt}} {
		if list.at == "" {
			continue
		}
		answer, err := r.get(ctx, "/sync/all-items/"+list.kind+"/dropped", url.Values{"extended": {"ids_only"}})
		if err != nil {
			return nil, err
		}
		var items struct {
			Shows []struct {
				Show titleJSON `json:"show"`
			} `json:"shows"`
			Anime []simklAnime `json:"anime"`
		}
		// An empty list is {} or null.
		if json.Unmarshal(answer.body, &items) != nil && strings.TrimSpace(string(answer.body)) != "null" {
			return nil, ErrUnreachable
		}
		for _, show := range items.Shows {
			read.add(show.Show.seriesRef(), nil)
		}
		for _, item := range items.Anime {
			a := item.Anime
			if a == nil {
				a = item.Show
			}
			if a != nil && !strings.EqualFold(cmpOr(a.AnimeType, item.AnimeType), "movie") {
				read.addAnime(mapping, *a, nil)
			}
		}
	}
	return read, nil
}

// addAnime adds the series the anime entry a is part of, as the anime
// mapping places its first episode, dropped at at. An entry the mapping
// takes for a movie is left out.
func (d *droppedRead) addAnime(mapping animeEntry, a animeJSON, at *time.Time) {
	if ref, ok := mapping.ref(a, false, 1, 1); ok && ref.Episode {
		d.add(asSeries(ref), at)
	}
}

// animeDropped reads the Dropped entries of a list of AniList or
// MyAnimeList, read whole, as the series they are part of. The list does
// not date the drop.
func (r *reader) animeDropped(items []listItem) *droppedRead {
	read := &droppedRead{whole: true}
	mapping := animeEntry{mapping: r.s.anime, unknown: map[string]bool{}}
	for _, item := range items {
		if item.status == statusDropped && !item.movie {
			read.addAnime(mapping, r.animeOf(item), nil)
		}
	}
	return read
}

// saveDropped finds the series of the shows read dropped and keeps them
// as the connection's: new ones seen now, those kept with the date read,
// and, once read whole, those no longer listed gone, with the cursor the
// read tells. The titles linked by hand are named by their links. Nothing
// is saved once the history import was turned off or the service
// disconnected meanwhile: what turns it off waits for what is saved, then
// deletes it.
func (s *Service) saveDropped(ctx context.Context, r *reader, read droppedRead, linked importlinks.Linked) error {
	refs := make([]library.TitleRef, len(read.shows))
	for i, d := range read.shows {
		refs[i], _ = keptTitle(linked, d.ref, nil)
	}
	targets, err := s.resolve(ctx, r, refs)
	if err != nil {
		return err
	}
	// A series named twice, as by two anime entries, is kept once, with
	// its latest date.
	ids := []accounts.ID{}
	dates := []pgtype.Timestamptz{}
	index := map[accounts.ID]int{}
	for i, d := range read.shows {
		for _, t := range targets[i] {
			id := t.ID
			if t.Series != (accounts.ID{}) {
				id = t.Series
			}
			at, seen := index[id]
			if !seen {
				at = len(ids)
				index[id] = at
				ids, dates = append(ids, id), append(dates, pgtype.Timestamptz{})
			}
			if d.at != nil && (!dates[at].Valid || d.at.After(dates[at].Time)) {
				dates[at] = pgtype.Timestamptz{Time: *d.at, Valid: true}
			}
		}
	}
	key := r.key
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var on bool
		err := tx.QueryRow(ctx, "SELECT enabled FROM tracking_imports WHERE user_id = $1 AND service = $2 FOR UPDATE",
			key.user, key.service).Scan(&on)
		if errors.Is(err, pgx.ErrNoRows) || err == nil && !on {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO tracker_dropped (user_id, service, series_id, dropped_at, seen_at)
			SELECT $1, $2, d.series_id, d.dropped_at, $5 FROM unnest($3::uuid[], $4::timestamptz[]) AS d (series_id, dropped_at)
			ON CONFLICT (user_id, service, series_id) DO UPDATE SET dropped_at = excluded.dropped_at`,
			key.user, key.service, ids, dates, s.now().UTC()); err != nil {
			return err
		}
		if read.whole {
			if _, err := tx.Exec(ctx, "DELETE FROM tracker_dropped WHERE user_id = $1 AND service = $2 AND series_id <> ALL($3)",
				key.user, key.service, ids); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, "UPDATE tracking_imports SET dropped_cursor = $3 WHERE user_id = $1 AND service = $2",
			key.user, key.service, read.cursor)
		return err
	})
}

// forgetDropped forgets the series the user dropped on the service, and
// where its activities stood for them, so that turned on again, the
// import reads them anew.
func forgetDropped(ctx context.Context, tx pgx.Tx, key laneKey) error {
	if _, err := tx.Exec(ctx, "DELETE FROM tracker_dropped WHERE user_id = $1 AND service = $2", key.user, key.service); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "UPDATE tracking_imports SET dropped_cursor = '' WHERE user_id = $1 AND service = $2", key.user, key.service)
	return err
}
