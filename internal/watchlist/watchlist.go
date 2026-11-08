// Package watchlist keeps each user's watchlist: a playlist of theirs,
// flagged as their watchlist, one per user, which Jellyfin apps list with
// their playlists. Its entries are movies and episodes, each once, in the
// order the user keeps, as any playlist's: a series stands for its aired
// episodes of regular seasons, in order (see Entries).
//
// The lists the user keeps elsewhere fill it: Trakt's watchlist and
// Simkl's Plan to Watch with the imports of their history (see package
// trackers), the library of a Stremio or Nuvio account with each import
// of it (see package appimport), and the one-off imports (see Import).
// The watchlist table, the ledger, remembers which lists hold each title,
// a movie or a series, and the playlist follows it:
//
//   - A title a list newly holds, one that list did not hold before or
//     held again after dropping it, waits in watchlist_pending until its
//     entries are made: a movie at once, a series once its episodes are
//     known and one has aired, in the background (see Expand). They go at
//     the top of the playlist, the latest title first, a series' episodes
//     in order, but those the playlist holds already: the episodes the
//     user took off come back. Episodes that air later are not added:
//     Next Up shows them.
//   - A title a list still holds changes nothing: an entry the user took
//     off stays off, one they moved stays where they put it.
//   - A list read whole that no longer holds a title (see Save), or a list
//     no longer read (see Drop), takes the title's entries off once no
//     list holds it, but those the user started or watched, and those
//     they added or moved, which become their own.
//
// The user changes the playlist from any app, as any playlist, and from
// Polyfin's own routes. What they add goes on their own list (see Own), at
// the top unless they say where; an entry they move becomes theirs too.
// What they take off leaves their own list; the other lists still holding
// it keep it off, until one newly holds it. Deleting the playlist forgets
// their own list; the playlist comes back, empty, only when an entry is
// added.
//
// The titles the user started stay on the playlist. The checks for
// versions and the Now available and Still waiting rows, which list the
// ledger's titles the playlist shows, leave them out (see available.go).
package watchlist

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Import is the list of the one-off imports, which have no next import:
// they only add.
const Import = "import"

// Tracking is the list of a tracking service's watchlist, such as Trakt's.
func Tracking(service string) string { return "tracking:" + service }

// Connection is the list of a user's connection to an app, such as
// Stremio: the library of the account.
func Connection(app string) string { return "app:" + app }

// Own is the list of the titles users put on their watchlist themselves,
// or moved in it, or started once no other list held them: what it holds
// stays until they take it off, whatever the other lists do.
const Own = "own"

// Title is a title a list holds, by its item, and when the list added it,
// nil when the list does not tell.
type Title struct {
	ID      accounts.ID
	AddedAt *time.Time
}

// Made is what a title of the ledger makes in a watchlist: Items, each an
// entry, in order, and Series, the series of Items when they are its
// episodes. Known is false while the title's entries cannot be told yet:
// a series whose episodes are not known, or none of which aired.
type Made struct {
	Items  []accounts.ID
	Series accounts.ID
	Known  bool
}

// Entries tells what title makes in user's watchlist: a movie itself, a
// series its aired episodes of regular seasons, in order, as any playlist
// takes a series. A title that makes nothing, as no playlist holds it, is
// Known without Items.
type Entries func(ctx context.Context, user, title accounts.ID) (Made, error)

// Store keeps the watchlists in PostgreSQL.
type Store struct {
	db      *pgxpool.Pool
	entries Entries
	now     func() time.Time
}

// New returns a store backed by db, which makes no entry of the titles
// lists newly hold until WithEntries tells how.
func New(db *pgxpool.Pool) *Store {
	return &Store{db: db, now: time.Now}
}

// WithEntries has the store make the entries of the titles lists newly
// hold with entries, right after each save (see Expand), and returns it.
func (s *Store) WithEntries(entries Entries) *Store {
	s.entries = entries
	return s
}

// WithClock has the store read the time with now when it tells which
// titles that wait are tried again (see retried), and returns it.
func (s *Store) WithClock(now func() time.Time) *Store {
	s.now = now
	return s
}

// retried selects, at the time the parameter at gives, the rows of
// watchlist_pending tried again: those never tried, those tried over an
// hour ago during the first day they wait, and the others once a day.
// A series not known yet usually is within hours; one the library cannot
// find may never be, and costs a try a day.
func retried(at string) string {
	at += "::timestamptz"
	return `(tried_at IS NULL OR tried_at <= ` + at + ` - interval '1 day'
		OR since > ` + at + ` - interval '1 day' AND tried_at <= ` + at + ` - interval '1 hour')`
}

// started is true for the title item, a column or a parameter, that user
// started: a movie or an episode with a resume point or played, a series
// with such an episode or marked played itself.
func started(user, item string) string {
	return `EXISTS (SELECT 1 FROM user_data d WHERE d.user_id = ` + user + `
		AND (d.item_id = ` + item + ` OR d.series_id = ` + item + `) AND (d.played OR d.position_ticks > 0))`
}

// playlistOf selects the watchlist playlist of user $1.
const playlistOf = "SELECT id FROM playlists WHERE owner_id = $1 AND watchlist"

// Playlist returns the identifier of user's watchlist playlist, and false
// when they deleted it and nothing was added since.
func (s *Store) Playlist(ctx context.Context, user accounts.ID) (accounts.ID, bool, error) {
	var id accounts.ID
	err := s.db.QueryRow(ctx, playlistOf, user).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return id, false, nil
	}
	return id, err == nil, err
}

// lock locks user's watchlist playlist against other changes until tx
// ends, creating it when create is set and they have none, and returns
// its identifier: the zero one when they have none.
func lock(ctx context.Context, tx pgx.Tx, user accounts.ID, create bool) (accounts.ID, error) {
	var id accounts.ID
	err := tx.QueryRow(ctx, playlistOf+" FOR UPDATE", user).Scan(&id)
	if !errors.Is(err, pgx.ErrNoRows) || !create {
		if errors.Is(err, pgx.ErrNoRows) {
			err = nil
		}
		return id, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO playlists (owner_id, watchlist) VALUES ($1, true)
		ON CONFLICT (owner_id) WHERE watchlist DO NOTHING`, user); err != nil {
		return id, err
	}
	err = tx.QueryRow(ctx, playlistOf+" FOR UPDATE", user).Scan(&id)
	return id, err
}

// Save records that list holds titles in user's watchlist. A title gets
// the date the list gives; one without keeps the date it has, or, new to
// the list, gets now, the first of titles the latest. The titles new to
// the list wait for their entries (see the package's comment), made at
// once when the store knows how (see WithEntries). With whole, titles is
// all the list holds: the titles it held before and no longer does are
// taken off it. Without, the list was read in part, or only adds (see
// Import). Save returns how many titles new to the list the playlist did
// not show, leaving out those that made no entry and wait for none, as
// they had nothing left to watch.
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
	var unshown []accounts.ID
	saved, waiting := false, false
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if held != nil {
			if ok, err := held(ctx, tx); err != nil || !ok {
				return err
			}
		}
		saved = true
		playlist, err := lock(ctx, tx, user, false)
		if err != nil {
			return err
		}
		rows, _ := tx.Query(ctx, `SELECT c.item_id FROM unnest($3::uuid[]) AS c (item_id)
			WHERE NOT EXISTS (SELECT 1 FROM watchlist w WHERE w.user_id = $1 AND w.list = $2 AND w.item_id = c.item_id)`, user, list, ids)
		fresh, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
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
			rows, _ := tx.Query(ctx, "DELETE FROM watchlist WHERE user_id = $1 AND list = $2 AND item_id <> ALL($3) RETURNING item_id, added_at",
				user, list, ids)
			if err := leave(ctx, tx, user, playlist, rows); err != nil {
				return err
			}
		}
		// The titles new to the list wait for their entries, as if added
		// anew: those the playlist shows already bring back the items it
		// no longer holds, such as the episodes the user took off.
		rows, _ = tx.Query(ctx, `SELECT c.item_id FROM unnest($2::uuid[]) AS c (item_id)
			WHERE NOT EXISTS (SELECT 1 FROM playlist_entries e WHERE e.playlist_id = $1 AND (e.title_id = c.item_id OR e.item_id = c.item_id))`,
			playlist, fresh)
		if unshown, err = pgx.CollectRows(rows, pgx.RowTo[accounts.ID]); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `INSERT INTO watchlist_pending (user_id, item_id, added_at, since)
			SELECT $1, w.item_id, w.added_at, $4 FROM watchlist w
			WHERE w.user_id = $1 AND w.list = $2 AND w.item_id = ANY($3)
			ON CONFLICT (user_id, item_id) DO UPDATE SET added_at = greatest(watchlist_pending.added_at, excluded.added_at),
				since = excluded.since, tried_at = NULL`,
			user, list, fresh, s.now())
		waiting = tag.RowsAffected() > 0
		return err
	})
	if err != nil || !saved || !waiting || s.entries == nil {
		return len(unshown), err
	}
	if _, err := s.Expand(ctx, user, false); err != nil {
		return 0, err
	}
	// A title with nothing left to watch added nothing: it counts not.
	var added int
	err = s.db.QueryRow(ctx, `SELECT count(*) FROM unnest($2::uuid[]) AS c (item_id)
		WHERE EXISTS (SELECT 1 FROM watchlist_pending p WHERE p.user_id = $1 AND p.item_id = c.item_id)
			OR EXISTS (SELECT 1 FROM playlists l JOIN playlist_entries e ON e.playlist_id = l.id
				WHERE l.owner_id = $1 AND l.watchlist AND (e.title_id = c.item_id OR e.item_id = c.item_id))`, user, unshown).Scan(&added)
	return added, err
}

// Drop takes off user's watchlist what list holds, as the list is no
// longer read: its import turned off, its service or app disconnected.
// The titles another list holds stay, and so do the entries the user
// started or made theirs.
func (s *Store) Drop(ctx context.Context, user accounts.ID, list string) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		playlist, err := lock(ctx, tx, user, false)
		if err != nil {
			return err
		}
		rows, _ := tx.Query(ctx, "DELETE FROM watchlist WHERE user_id = $1 AND list = $2 RETURNING item_id, added_at", user, list)
		return leave(ctx, tx, user, playlist, rows)
	})
}

// leave takes off playlist, user's watchlist (the zero identifier for
// none), the entries of the titles of rows, those a list no longer holds,
// with the date it gave them, once no list holds them. The entries the
// user started or watched, and those they added or moved, which their own
// list holds, stay as titles of their own, the started ones from that
// date.
func leave(ctx context.Context, tx pgx.Tx, user, playlist accounts.ID, rows pgx.Rows) error {
	var titles []accounts.ID
	var dates []time.Time
	var title accounts.ID
	var date time.Time
	if _, err := pgx.ForEachRow(rows, []any{&title, &date}, func() error {
		titles, dates = append(titles, title), append(dates, date)
		return nil
	}); err != nil || len(titles) == 0 {
		return err
	}
	const gone = "NOT EXISTS (SELECT 1 FROM watchlist w WHERE w.user_id = $1 AND w.item_id = g.title_id)"
	if _, err := tx.Exec(ctx, `DELETE FROM watchlist_pending p USING unnest($2::uuid[]) AS g (title_id)
		WHERE p.user_id = $1 AND p.item_id = g.title_id AND `+gone, user, titles); err != nil {
		return err
	}
	if playlist == (accounts.ID{}) {
		return nil
	}
	if _, err := tx.Exec(ctx, `INSERT INTO watchlist (user_id, item_id, list, added_at)
		SELECT $1, e.item_id, $3, g.added_at
		FROM unnest($4::uuid[], $5::timestamptz[]) AS g (title_id, added_at)
		JOIN playlist_entries e ON e.playlist_id = $2 AND e.title_id = g.title_id
		WHERE `+gone+` AND `+started("$1", "e.item_id")+`
		ON CONFLICT (user_id, item_id, list) DO NOTHING`, user, playlist, Own, titles, dates); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM playlist_entries e USING unnest($3::uuid[]) AS g (title_id)
		WHERE e.playlist_id = $2 AND e.title_id = g.title_id AND `+gone+`
			AND NOT EXISTS (SELECT 1 FROM watchlist o WHERE o.user_id = $1 AND o.item_id = e.item_id)`, user, playlist, titles)
	if err != nil {
		return err
	}
	// What stays is its own title now.
	if _, err := tx.Exec(ctx, `UPDATE playlist_entries e SET title_id = e.item_id FROM unnest($3::uuid[]) AS g (title_id)
		WHERE e.playlist_id = $2 AND e.title_id = g.title_id AND `+gone, user, playlist, titles); err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	return compact(ctx, tx, playlist)
}

// compact numbers the entries of playlist from 0 again, in order, once
// some left.
func compact(ctx context.Context, tx pgx.Tx, playlist accounts.ID) error {
	_, err := tx.Exec(ctx, `UPDATE playlist_entries e SET position = o.position
		FROM (SELECT id, row_number() OVER (ORDER BY position) - 1 AS position FROM playlist_entries WHERE playlist_id = $1) o
		WHERE e.id = o.id AND e.position <> o.position`, playlist)
	return err
}

// entry is an entry about to go in a watchlist: its item, the title of
// the ledger it stands for, and its series, the zero identifier for a
// movie.
type entry struct {
	item, title, series accounts.ID
}

// insertAt inserts entries, but those whose item playlist holds already,
// at position, or after the last entry when position is past it; a
// negative position counts as the first. It reports how many it inserted.
func insertAt(ctx context.Context, tx pgx.Tx, playlist accounts.ID, entries []entry, position int) (int, error) {
	if len(entries) == 0 {
		return 0, nil
	}
	rows, _ := tx.Query(ctx, "SELECT item_id FROM playlist_entries WHERE playlist_id = $1", playlist)
	held, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
	if err != nil {
		return 0, err
	}
	holds := make(map[accounts.ID]bool, len(held)+len(entries))
	for _, item := range held {
		holds[item] = true
	}
	var items, titles, series []accounts.ID
	for _, e := range entries {
		if !holds[e.item] {
			holds[e.item] = true
			items, titles, series = append(items, e.item), append(titles, e.title), append(series, e.series)
		}
	}
	if len(items) == 0 {
		return 0, nil
	}
	position = min(max(position, 0), len(held))
	if _, err := tx.Exec(ctx, "UPDATE playlist_entries SET position = position + $3 WHERE playlist_id = $1 AND position >= $2",
		playlist, position, len(items)); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO playlist_entries (playlist_id, item_id, title_id, series_id, position)
		SELECT $1, c.item_id, c.title_id, nullif(c.series_id, '00000000-0000-0000-0000-000000000000'), $2 + c.place - 1
		FROM unnest($3::uuid[], $4::uuid[], $5::uuid[]) WITH ORDINALITY AS c (item_id, title_id, series_id, place)`,
		playlist, position, items, titles, series); err != nil {
		return 0, err
	}
	_, err = tx.Exec(ctx, "UPDATE playlists SET last_added_at = now() WHERE id = $1", playlist)
	return len(items), err
}

// own puts titles on user's own list, from now, those it holds keeping
// their date.
func own(ctx context.Context, tx pgx.Tx, user accounts.ID, titles []accounts.ID) error {
	_, err := tx.Exec(ctx, `INSERT INTO watchlist (user_id, item_id, list, added_at)
		SELECT $1, c.item_id, $2, now() FROM unnest($3::uuid[]) AS c (item_id)
		ON CONFLICT (user_id, item_id, list) DO NOTHING`, user, Own, titles)
	return err
}

// Expand makes the entries of the titles of user's watchlist that wait for
// them, the latest title first, at the top of the playlist, a series'
// episodes in order (see Entries), but the items the playlist holds. A
// title whose entries cannot be told yet waits; with later, only those
// due to be tried again are tried (see retried). It reports how many
// titles it made entries of.
func (s *Store) Expand(ctx context.Context, user accounts.ID, later bool) (int, error) {
	if s.entries == nil {
		return 0, nil
	}
	condition := "$2::timestamptz IS NOT NULL"
	if later {
		condition = retried("$2")
	}
	rows, err := s.db.Query(ctx, "SELECT item_id FROM watchlist_pending WHERE user_id = $1 AND "+condition+" ORDER BY added_at DESC, item_id",
		user, s.now())
	if err != nil {
		return 0, err
	}
	waiting, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
	if err != nil || len(waiting) == 0 {
		return 0, err
	}
	made := make(map[accounts.ID]Made, len(waiting))
	var known, unknown []accounts.ID
	for _, title := range waiting {
		m, err := s.entries(ctx, user, title)
		if err != nil {
			if ctx.Err() != nil {
				return 0, ctx.Err()
			}
			m = Made{}
		}
		if m.Known {
			made[title] = m
			known = append(known, title)
		} else {
			unknown = append(unknown, title)
		}
	}
	var done int
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		playlist, err := lock(ctx, tx, user, false)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE watchlist_pending SET tried_at = $3 WHERE user_id = $1 AND item_id = ANY($2)", user, unknown, s.now()); err != nil {
			return err
		}
		if len(known) == 0 {
			return nil
		}
		// Only what still waits: a list may have dropped a title meanwhile.
		rows, _ := tx.Query(ctx, "DELETE FROM watchlist_pending WHERE user_id = $1 AND item_id = ANY($2) RETURNING item_id", user, known)
		still, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
		if err != nil {
			return err
		}
		done = len(still)
		var entries []entry
		for _, title := range known {
			if !slices.Contains(still, title) {
				continue
			}
			for _, item := range made[title].Items {
				entries = append(entries, entry{item: item, title: title, series: made[title].Series})
			}
		}
		if len(entries) == 0 {
			return nil
		}
		if playlist == (accounts.ID{}) {
			if playlist, err = lock(ctx, tx, user, true); err != nil {
				return err
			}
		}
		_, err = insertAt(ctx, tx, playlist, entries, 0)
		return err
	})
	return done, err
}

// ExpandAll makes, user after user, the entries of the titles that wait
// for them and are due to be tried again (see Expand and retried): the
// background pass, which finds the episodes of the series learnt or aired
// since.
func (s *Store) ExpandAll(ctx context.Context) error {
	if s.entries == nil {
		return nil
	}
	rows, err := s.db.Query(ctx, "SELECT DISTINCT user_id FROM watchlist_pending WHERE "+retried("$1"), s.now())
	if err != nil {
		return err
	}
	users, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
	if err != nil {
		return err
	}
	for _, user := range users {
		if _, err := s.Expand(ctx, user, true); err != nil {
			return err
		}
	}
	return nil
}

// Group is what the user adds to their watchlist for one title of theirs:
// Title, which their own list holds, a movie, an episode or a series, and
// Items, its entries in order, the episodes of Series for a series or an
// episode. A series none of whose episodes aired yet has no Items: they
// are added once one airs (see Expand).
type Group struct {
	Title, Series accounts.ID
	Items         []accounts.ID
}

// Add puts groups on user's watchlist, their titles on their own list (see
// Own): the entries the playlist does not hold go in at position, the
// first being 0, or last when position is past the last entry, in order;
// those it holds keep their place. A playlist the user deleted comes back
// once an entry goes in, not for a series that only waits for its
// episodes.
func (s *Store) Add(ctx context.Context, user accounts.ID, groups []Group, position int) error {
	if len(groups) == 0 {
		return nil
	}
	titles := make([]accounts.ID, 0, len(groups))
	var waiting []accounts.ID
	var entries []entry
	for _, g := range groups {
		titles = append(titles, g.Title)
		if len(g.Items) == 0 {
			waiting = append(waiting, g.Title)
		}
		for _, item := range g.Items {
			entries = append(entries, entry{item: item, title: g.Title, series: g.Series})
		}
	}
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		playlist, err := lock(ctx, tx, user, len(entries) > 0)
		if err != nil {
			return err
		}
		if err := own(ctx, tx, user, titles); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO watchlist_pending (user_id, item_id, added_at, since)
			SELECT $1, c.item_id, now(), $3 FROM unnest($2::uuid[]) AS c (item_id)
			ON CONFLICT (user_id, item_id) DO NOTHING`, user, waiting, s.now()); err != nil {
			return err
		}
		_, err = insertAt(ctx, tx, playlist, entries, position)
		return err
	})
}

// Remove takes off user's watchlist the entries of items and the episodes
// of series, and what waits for them: their titles leave their own list.
// The other lists holding them keep them off until one newly holds them.
func (s *Store) Remove(ctx context.Context, user accounts.ID, items, series []accounts.ID) error {
	return s.remove(ctx, user, append(slices.Clone(items), series...), "item_id = ANY($2) OR series_id = ANY($3)", items, series)
}

// RemoveEntries takes off user's watchlist its entries, as Remove does;
// the entries it does not have are ignored.
func (s *Store) RemoveEntries(ctx context.Context, user accounts.ID, entries []accounts.ID) error {
	return s.remove(ctx, user, nil, "id = ANY($2)", entries)
}

// remove takes off user's watchlist the entries a condition on
// playlist_entries, which, selects with args, from $2 on, and titles and
// what waits for them.
func (s *Store) remove(ctx context.Context, user accounts.ID, titles []accounts.ID, which string, args ...any) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		playlist, err := lock(ctx, tx, user, false)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM watchlist_pending WHERE user_id = $1 AND item_id = ANY($2)", user, titles); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM watchlist WHERE user_id = $1 AND list = $2 AND item_id = ANY($3)", user, Own, titles); err != nil {
			return err
		}
		if playlist == (accounts.ID{}) {
			return nil
		}
		rows, _ := tx.Query(ctx, "SELECT id FROM playlist_entries WHERE playlist_id = $1 AND ("+which+")", append([]any{playlist}, args...)...)
		entries, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
		if err != nil || len(entries) == 0 {
			return err
		}
		if err := s.removeIn(ctx, tx, user, playlist, entries); err != nil {
			return err
		}
		return compact(ctx, tx, playlist)
	})
}

// Move places entry of user's watchlist just before the entry before, or
// last when before is the zero identifier or no longer in it, and makes it
// the user's own. An entry the playlist does not have is ignored.
func (s *Store) Move(ctx context.Context, user, entry, before accounts.ID) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		playlist, err := lock(ctx, tx, user, false)
		if err != nil || playlist == (accounts.ID{}) {
			return err
		}
		rows, _ := tx.Query(ctx, "SELECT id FROM playlist_entries WHERE playlist_id = $1 ORDER BY position", playlist)
		order, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
		if err != nil {
			return err
		}
		from := slices.Index(order, entry)
		if from < 0 {
			return nil
		}
		if _, err := tx.Exec(ctx, `INSERT INTO watchlist (user_id, item_id, list, added_at)
			SELECT $1, e.item_id, $3, coalesce((SELECT max(w.added_at) FROM watchlist w WHERE w.user_id = $1 AND w.item_id = e.title_id), now())
			FROM playlist_entries e WHERE e.id = $2
			ON CONFLICT (user_id, item_id, list) DO NOTHING`, user, entry, Own); err != nil {
			return err
		}
		if entry == before {
			return nil
		}
		order = slices.Delete(order, from, from+1)
		to := slices.Index(order, before)
		if to < 0 {
			to = len(order)
		}
		order = slices.Insert(order, to, entry)
		_, err = tx.Exec(ctx, `UPDATE playlist_entries e SET position = o.position - 1
			FROM unnest($2::uuid[]) WITH ORDINALITY AS o (id, position)
			WHERE e.playlist_id = $1 AND e.id = o.id AND e.position <> o.position - 1`, playlist, order)
		return err
	})
}

// Replace makes groups all user's watchlist holds, in order: the entries
// it held that groups do not leave as Remove takes them off, the others
// come as Add puts them.
func (s *Store) Replace(ctx context.Context, user accounts.ID, groups []Group) error {
	var items []accounts.ID
	for _, g := range groups {
		items = append(items, g.Items...)
	}
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		playlist, err := lock(ctx, tx, user, len(items) > 0)
		if err != nil {
			return err
		}
		rows, _ := tx.Query(ctx, "SELECT id FROM playlist_entries WHERE playlist_id = $1 AND item_id <> ALL($2)", playlist, items)
		gone, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
		if err != nil {
			return err
		}
		// Done in one transaction, as the store's own steps.
		if err := s.removeIn(ctx, tx, user, playlist, gone); err != nil {
			return err
		}
		var entries []entry
		titles := make([]accounts.ID, 0, len(groups))
		for _, g := range groups {
			titles = append(titles, g.Title)
			for _, item := range g.Items {
				entries = append(entries, entry{item: item, title: g.Title, series: g.Series})
			}
		}
		if err := own(ctx, tx, user, titles); err != nil {
			return err
		}
		if _, err := insertAt(ctx, tx, playlist, entries, len(items)); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE playlist_entries e SET position = o.position - 1
			FROM (SELECT DISTINCT ON (c.item_id) c.item_id, c.place AS position FROM unnest($2::uuid[]) WITH ORDINALITY AS c (item_id, place)
				ORDER BY c.item_id, c.place) o
			WHERE e.playlist_id = $1 AND e.item_id = o.item_id`, playlist, items)
		if err != nil {
			return err
		}
		return compact(ctx, tx, playlist)
	})
}

// removeIn takes entries off playlist, user's watchlist, within tx, as
// RemoveEntries does.
func (s *Store) removeIn(ctx context.Context, tx pgx.Tx, user, playlist accounts.ID, entries []accounts.ID) error {
	var items, owners []accounts.ID
	var item, owner accounts.ID
	rows, _ := tx.Query(ctx, "DELETE FROM playlist_entries WHERE playlist_id = $1 AND id = ANY($2) RETURNING item_id, title_id", playlist, entries)
	if _, err := pgx.ForEachRow(rows, []any{&item, &owner}, func() error {
		items, owners = append(items, item), append(owners, owner)
		return nil
	}); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `DELETE FROM watchlist w WHERE w.user_id = $1 AND w.list = $2
		AND (w.item_id = ANY($3) OR w.item_id = ANY($4) AND NOT EXISTS (SELECT 1 FROM playlist_entries e WHERE e.playlist_id = $5 AND e.title_id = w.item_id))`,
		user, Own, items, owners, playlist)
	return err
}

// Delete deletes user's watchlist playlist and forgets their own list.
// The other lists' titles stay off until a list newly holds them, as
// after Remove; the playlist comes back, empty, when an entry is added.
func (s *Store) Delete(ctx context.Context, user accounts.ID) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := lock(ctx, tx, user, false); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM watchlist WHERE user_id = $1 AND list = $2", user, Own); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM watchlist_pending WHERE user_id = $1", user); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "DELETE FROM playlists WHERE owner_id = $1 AND watchlist", user)
		return err
	})
}

// Titles lists the items of user's watchlist, movies and episodes, started
// or not, in its order.
func (s *Store) Titles(ctx context.Context, user accounts.ID) ([]accounts.ID, error) {
	rows, err := s.db.Query(ctx, `SELECT e.item_id FROM playlists p JOIN playlist_entries e ON e.playlist_id = p.id
		WHERE p.owner_id = $1 AND p.watchlist ORDER BY e.position`, user)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
}

// Held is an entry of a watchlist: its item, a movie or an episode, the
// series of an episode, and when the list that brought it, or the user,
// added it.
type Held struct {
	ID, Series accounts.ID
	AddedAt    time.Time
}

// Held lists the entries of user's watchlist, started or not, in its
// order.
func (s *Store) Held(ctx context.Context, user accounts.ID) ([]Held, error) {
	rows, err := s.db.Query(ctx, `SELECT e.item_id, coalesce(e.series_id, '00000000-0000-0000-0000-000000000000'),
			coalesce((SELECT max(w.added_at) FROM watchlist w WHERE w.user_id = p.owner_id AND w.item_id IN (e.item_id, e.title_id)),
				p.last_added_at, p.created_at)
		FROM playlists p JOIN playlist_entries e ON e.playlist_id = p.id
		WHERE p.owner_id = $1 AND p.watchlist ORDER BY e.position`, user)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Held, error) {
		var h Held
		err := row.Scan(&h.ID, &h.Series, &h.AddedAt)
		return h, err
	})
}

// Pending lists the titles of user's watchlist that wait for their
// entries (see Expand), each with the date its list gave, the latest
// first; Series is zero.
func (s *Store) Pending(ctx context.Context, user accounts.ID) ([]Held, error) {
	rows, err := s.db.Query(ctx, "SELECT item_id, added_at FROM watchlist_pending WHERE user_id = $1 ORDER BY added_at DESC, item_id", user)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Held, error) {
		var h Held
		err := row.Scan(&h.ID, &h.AddedAt)
		return h, err
	})
}
