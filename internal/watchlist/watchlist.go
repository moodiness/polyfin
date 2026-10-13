// Package watchlist keeps each user's watchlist: the movies and series
// they mean to watch, as the lists they keep elsewhere hold them. Trakt's
// watchlist and Simkl's Plan to Watch come with the imports of their
// history (see package trackers), and the library of a Stremio or Nuvio
// account with each import of it (see package appimport).
//
// Each title remembers the lists that hold it. A list's import replaces
// what that list holds: a title it no longer holds leaves the watchlist,
// unless another list still holds it; so does what a list held once it is
// no longer read (see Drop). A one-off app import, which has no next
// import, only adds (see Import). Jellyfin apps have no watchlist button:
// the watchlist is edited where the lists live.
//
// The titles the user started, a movie with a resume point or played, a
// series with an episode started or played, are left out when the
// watchlist is read: they show in Continue Watching and Next Up instead.
package watchlist

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Import is the list of the one-off app imports: what they add stays
// until the user starts it.
const Import = "import"

// Tracking is the list of a tracking service's watchlist, such as Trakt's.
func Tracking(service string) string { return "tracking:" + service }

// Connection is the list of a user's connection to an app, such as
// Stremio: the library of the account.
func Connection(app string) string { return "app:" + app }

// Title is a movie or series a list holds, by its item, and when the list
// added it, nil when the list does not tell.
type Title struct {
	ID      accounts.ID
	AddedAt *time.Time
}

// Store keeps the watchlists in PostgreSQL.
type Store struct {
	db *pgxpool.Pool
}

// New returns a store backed by db.
func New(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

// unstarted is true for a watchlist row w whose title the user has not
// started: a movie with neither a resume point nor a played mark, a series
// with no such episode and not marked played itself.
const unstarted = `NOT EXISTS (SELECT 1 FROM user_data d WHERE d.user_id = w.user_id
	AND (d.item_id = w.item_id OR d.series_id = w.item_id) AND (d.played OR d.position_ticks > 0))`

// Save records that list holds titles in user's watchlist. A title gets
// the date the list gives; one without keeps the date it has, or, new to
// the list, gets now, the first of titles the latest. With whole, titles
// is all the list holds: the titles it held before and no longer does are
// taken off it, and leave the watchlist unless another list holds them.
// Without, the list was read in part, or only adds (see Import). Save
// returns how many of titles the watchlist did not hold before and the
// user has not started: the titles it adds to the Watchlist view.
func (s *Store) Save(ctx context.Context, user accounts.ID, list string, titles []Title, whole bool) (int, error) {
	return s.SaveWhile(ctx, user, list, titles, whole, nil)
}

// SaveWhile is Save, done only while held, when not nil, reports true,
// run first in the transaction that saves: it locks what it reads, such
// as the connection whose list it is, so that what deletes that, then
// drops the list, waits for the titles saved, or keeps them from being
// saved. Not held, nothing is saved, and SaveWhile returns 0.
func (s *Store) SaveWhile(ctx context.Context, user accounts.ID, list string, titles []Title, whole bool,
	held func(context.Context, pgx.Tx) (bool, error)) (int, error) {
	// A title listed twice is saved once, with its latest date.
	ids := make([]accounts.ID, 0, len(titles))
	dates := make([]pgtype.Timestamptz, 0, len(titles))
	index := make(map[accounts.ID]int, len(titles))
	for _, t := range titles {
		i, seen := index[t.ID]
		if !seen {
			i = len(ids)
			index[t.ID] = i
			ids, dates = append(ids, t.ID), append(dates, pgtype.Timestamptz{})
		}
		if t.AddedAt != nil && (!dates[i].Valid || t.AddedAt.After(dates[i].Time)) {
			dates[i] = pgtype.Timestamptz{Time: *t.AddedAt, Valid: true}
		}
	}
	var added int
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if held != nil {
			if ok, err := held(ctx, tx); err != nil || !ok {
				return err
			}
		}
		err := tx.QueryRow(ctx, `SELECT count(*) FROM (SELECT $1::uuid AS user_id, item_id FROM unnest($2::uuid[]) AS c (item_id)) w
			WHERE NOT EXISTS (SELECT 1 FROM watchlist o WHERE o.user_id = w.user_id AND o.item_id = w.item_id) AND `+unstarted,
			user, ids).Scan(&added)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO watchlist (user_id, item_id, list, added_at)
			SELECT $1, c.item_id, $2, c.added_at FROM unnest($3::uuid[], $4::timestamptz[]) AS c (item_id, added_at)
			WHERE c.added_at IS NOT NULL
			ON CONFLICT (user_id, item_id, list) DO UPDATE SET added_at = excluded.added_at`, user, list, ids, dates); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO watchlist (user_id, item_id, list, added_at)
			SELECT $1, c.item_id, $2, now() - c.place * interval '1 microsecond'
			FROM unnest($3::uuid[], $4::timestamptz[]) WITH ORDINALITY AS c (item_id, added_at, place)
			WHERE c.added_at IS NULL
			ON CONFLICT (user_id, item_id, list) DO NOTHING`, user, list, ids, dates); err != nil {
			return err
		}
		if whole {
			_, err = tx.Exec(ctx, "DELETE FROM watchlist WHERE user_id = $1 AND list = $2 AND item_id <> ALL($3)", user, list, ids)
		}
		return err
	})
	return added, err
}

// Drop takes off user's watchlist what list holds, as the list is no
// longer read: its import turned off, its service or app disconnected.
// The titles another list holds stay.
func (s *Store) Drop(ctx context.Context, user accounts.ID, list string) error {
	_, err := s.db.Exec(ctx, "DELETE FROM watchlist WHERE user_id = $1 AND list = $2", user, list)
	return err
}

// Titles lists the titles of user's watchlist they have not started, the
// latest added first: by the latest date a list holding it gives.
func (s *Store) Titles(ctx context.Context, user accounts.ID) ([]accounts.ID, error) {
	rows, err := s.db.Query(ctx, `SELECT w.item_id FROM watchlist w WHERE w.user_id = $1 AND `+unstarted+`
		GROUP BY w.item_id ORDER BY max(w.added_at) DESC, w.item_id`, user)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
}
