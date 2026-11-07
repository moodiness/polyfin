package importlinks

import (
	"cmp"
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/userdata"
)

// place names an entry in a source's list: its key, and an episode's
// numbers.
type place struct {
	key             string
	season, episode int
}

func (e Entry) place() place {
	if e.Kind != KindEpisode {
		return place{key: e.Key}
	}
	return place{e.Key, e.Season, e.Episode}
}

// merge adds to e what o, the same entry read again, brings: the played
// mark at the later date with the most plays, the newer resume point, the
// watchlist and the rating when e has none, and o's label and reason.
func (e Entry) merge(o Entry) Entry {
	merged := o
	merged.Name, merged.IDs = cmp.Or(o.Name, e.Name), cmp.Or(o.IDs, e.IDs)
	merged.Year = cmp.Or(o.Year, e.Year)
	merged.Played = e.Played || o.Played
	merged.PlayedAt = later(e.PlayedAt, o.PlayedAt)
	merged.Plays = max(e.Plays, o.Plays)
	if o.Resume == nil || e.Resume != nil && e.Resume.At.After(o.Resume.At) {
		merged.Resume = e.Resume
	}
	if o.Watchlist == "" {
		merged.Watchlist, merged.AddedAt = e.Watchlist, e.AddedAt
	}
	if e.Rating != nil {
		merged.Rating = e.Rating
	}
	return merged
}

func later(a, b *time.Time) *time.Time {
	if a == nil || b != nil && b.After(*a) {
		return b
	}
	return a
}

// Save keeps entries, the titles an import of source into user's account
// did not find. With whole, the import read the source whole: what was
// kept of the entries it no longer holds goes, but for the entries linked
// by hand, which stay listed with their links. Without, the import read
// what changed since the last one, and entries add to what was kept. An
// entry kept already, or given twice, is merged: played at the later date
// with the most plays, the newer resume point, the watchlist and the
// rating kept.
func (s *Store) Save(ctx context.Context, user accounts.ID, source string, entries []Entry, whole bool) error {
	index := make(map[place]int, len(entries))
	kept := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if e.Key == "" || e.Kind == KindEpisode && e.Episode <= 0 {
			continue
		}
		if e.Kind != KindEpisode {
			e.Season, e.Episode = 0, 0
		}
		if i, seen := index[e.place()]; seen {
			kept[i] = kept[i].merge(e)
			continue
		}
		index[e.place()] = len(kept)
		kept = append(kept, e)
	}
	now := s.now().UTC()
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if whole {
			if _, err := tx.Exec(ctx, `DELETE FROM import_unfound u WHERE user_id = $1 AND source = $2 AND NOT EXISTS (
					SELECT 1 FROM import_title_links l WHERE l.user_id = u.user_id AND l.source = u.source AND l.entry_key = u.entry_key)`,
				user, source); err != nil {
				return err
			}
		}
		batch := &pgx.Batch{}
		for _, e := range kept {
			var percent *float64
			var position, runtime *int64
			var resumeAt *time.Time
			if r := e.Resume; r != nil {
				percent, resumeAt = &r.Percent, &r.At
				position, runtime = new(r.Position.Milliseconds()), new(r.Runtime.Milliseconds())
			}
			var year *int
			if e.Year > 0 {
				year = &e.Year
			}
			var list *string
			if e.Watchlist != "" {
				list = &e.Watchlist
			}
			batch.Queue(`INSERT INTO import_unfound (user_id, source, entry_key, season, episode, kind, name, year, ids, reason, played,
					played_at, plays, resume_percent, resume_position_ms, resume_runtime_ms, resume_at, watchlist, added_at, rating, seen_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
				ON CONFLICT (user_id, source, entry_key, season, episode) DO UPDATE SET kind = excluded.kind,
					name = CASE WHEN excluded.name <> '' THEN excluded.name ELSE import_unfound.name END,
					year = coalesce(excluded.year, import_unfound.year),
					ids = CASE WHEN excluded.ids <> '' THEN excluded.ids ELSE import_unfound.ids END,
					reason = excluded.reason, played = import_unfound.played OR excluded.played,
					played_at = greatest(import_unfound.played_at, excluded.played_at), plays = greatest(import_unfound.plays, excluded.plays),
					resume_percent = CASE WHEN `+newerResume+` THEN excluded.resume_percent ELSE import_unfound.resume_percent END,
					resume_position_ms = CASE WHEN `+newerResume+` THEN excluded.resume_position_ms ELSE import_unfound.resume_position_ms END,
					resume_runtime_ms = CASE WHEN `+newerResume+` THEN excluded.resume_runtime_ms ELSE import_unfound.resume_runtime_ms END,
					resume_at = CASE WHEN `+newerResume+` THEN excluded.resume_at ELSE import_unfound.resume_at END,
					watchlist = coalesce(excluded.watchlist, import_unfound.watchlist),
					added_at = CASE WHEN excluded.watchlist IS NOT NULL THEN excluded.added_at ELSE import_unfound.added_at END,
					rating = coalesce(import_unfound.rating, excluded.rating), seen_at = excluded.seen_at`,
				user, source, e.Key, e.Season, e.Episode, e.Kind, e.Name, year, e.IDs, e.Reason, e.Played, e.PlayedAt, e.Plays, percent,
				position, runtime, resumeAt, list, e.AddedAt, e.Rating, now)
		}
		return tx.SendBatch(ctx, batch).Close()
	})
}

// newerResume is true when the resume point an import kept again is newer
// than the one kept.
const newerResume = `excluded.resume_at IS NOT NULL AND (import_unfound.resume_at IS NULL OR excluded.resume_at > import_unfound.resume_at)`

// Row is an entry of a source's list, and its link, nil when it has none.
type Row struct {
	Entry
	Link *Link
}

// entryColumns are the columns of an entry u, scanned by scanEntry.
const entryColumns = `u.entry_key, u.season, u.episode, u.kind, u.name, u.year, u.ids, u.reason, u.played, u.played_at, u.plays,
	u.resume_percent, u.resume_position_ms, u.resume_runtime_ms, u.resume_at, u.watchlist, u.added_at, u.rating`

// scanEntry reads an entry, then what extra scans.
func scanEntry(row pgx.Row, extra ...any) (Entry, error) {
	var e Entry
	var year *int
	var percent *float64
	var position, runtime *int64
	var resumeAt *time.Time
	var list *string
	if err := row.Scan(append([]any{&e.Key, &e.Season, &e.Episode, &e.Kind, &e.Name, &year, &e.IDs, &e.Reason, &e.Played, &e.PlayedAt,
		&e.Plays, &percent, &position, &runtime, &resumeAt, &list, &e.AddedAt, &e.Rating}, extra...)...); err != nil {
		return e, err
	}
	if year != nil {
		e.Year = *year
	}
	if resumeAt != nil {
		e.Resume = &userdata.ImportedResume{At: *resumeAt}
		if percent != nil {
			e.Resume.Percent = *percent
		}
		if position != nil {
			e.Resume.Position = time.Duration(*position) * time.Millisecond
		}
		if runtime != nil {
			e.Resume.Runtime = time.Duration(*runtime) * time.Millisecond
		}
	}
	if list != nil {
		e.Watchlist = *list
	}
	return e, nil
}

// List returns what user's imports of source kept of the titles they did
// not find, by name, those without one last, and an episode after its
// series' earlier ones, with their links.
func (s *Store) List(ctx context.Context, user accounts.ID, source string) ([]Row, error) {
	rows, err := s.db.Query(ctx, `SELECT `+entryColumns+`, l.entry_key IS NOT NULL, `+linkColumns+` FROM import_unfound u
		LEFT JOIN import_title_links l ON l.user_id = u.user_id AND l.source = u.source AND l.entry_key = u.entry_key
		LEFT JOIN users b ON b.id = l.linked_by
		WHERE u.user_id = $1 AND u.source = $2
		ORDER BY u.name = '', lower(u.name), u.ids, u.entry_key, u.season, u.episode`, user, source)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Row{}
	for rows.Next() {
		var linked bool
		var l scannedLink
		// The link's columns are null without one.
		var kind, name, byName *string
		var at *time.Time
		e, err := scanEntry(rows, &linked, &kind, &l.imdb, &l.tmdb, &name, &l.year, &l.by, &byName, &at)
		if err != nil {
			return nil, err
		}
		row := Row{Entry: e}
		if linked {
			l.kind, l.name, l.byName, l.at = *kind, *name, *byName, *at
			row.Link = new(l.link())
		}
		list = append(list, row)
	}
	return list, rows.Err()
}

// Count is how many entries of a source's list are not linked, and how
// many are.
type Count struct {
	Source           string
	NotFound, Linked int
}

// Counts counts the lists of user's imports, by source, in the order of
// their names.
func (s *Store) Counts(ctx context.Context, user accounts.ID) ([]Count, error) {
	rows, err := s.db.Query(ctx, `SELECT u.source, count(*) FILTER (WHERE l.entry_key IS NULL), count(l.entry_key)
		FROM import_unfound u
		LEFT JOIN import_title_links l ON l.user_id = u.user_id AND l.source = u.source AND l.entry_key = u.entry_key
		WHERE u.user_id = $1 GROUP BY u.source ORDER BY u.source`, user)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Count, error) {
		var c Count
		return c, row.Scan(&c.Source, &c.NotFound, &c.Linked)
	})
}
