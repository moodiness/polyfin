// Package playlists keeps the playlists users make of movies and episodes:
// their entries in order, the users they are shared with, and whether every
// user may see them.
package playlists

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
)

// ErrNotFound reports a playlist that does not exist.
var ErrNotFound = errors.New("playlist not found")

// Playlist is a user's ordered list of titles.
type Playlist struct {
	ID    accounts.ID
	Owner accounts.ID
	Name  string
	// OpenAccess lets every user see the playlist; only its owner and the
	// users shared with to edit change it.
	OpenAccess bool
	Shares     []Share
	Entries    []Entry
	Created    time.Time
	// LastAdded is when titles were last added, nil before any.
	LastAdded *time.Time
}

// Share gives a user access to a playlist; CanEdit lets them change it.
type Share struct {
	User    accounts.ID
	CanEdit bool
}

// Entry is one place in a playlist. Its identifier is its own, so that the
// same title can appear more than once and be moved or removed alone.
type Entry struct {
	ID   accounts.ID
	Item accounts.ID
}

// Visible reports whether user may see the playlist: its owner, a user it
// is shared with, or anyone when it is open.
func (p Playlist) Visible(user accounts.ID) bool {
	_, shared := p.Share(user)
	return p.OpenAccess || p.Owner == user || shared
}

// Editable reports whether user may change the playlist: its owner, or a
// user it is shared with to edit.
func (p Playlist) Editable(user accounts.ID) bool {
	share, shared := p.Share(user)
	return p.Owner == user || shared && share.CanEdit
}

// Share returns the share of user, if the playlist is shared with them.
func (p Playlist) Share(user accounts.ID) (Share, bool) {
	i := slices.IndexFunc(p.Shares, func(s Share) bool { return s.User == user })
	if i < 0 {
		return Share{}, false
	}
	return p.Shares[i], true
}

// Store keeps playlists in PostgreSQL.
type Store struct {
	db *pgxpool.Pool
}

// New returns a store backed by db.
func New(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

// Draft is a playlist about to be created.
type Draft struct {
	Owner      accounts.ID
	Name       string
	OpenAccess bool
	Shares     []Share
	Items      []accounts.ID
}

// Create stores a new playlist and returns its identifier.
func (s *Store) Create(ctx context.Context, d Draft) (accounts.ID, error) {
	var id accounts.ID
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "INSERT INTO playlists (owner_id, name, open_access) VALUES ($1, $2, $3) RETURNING id",
			d.Owner, d.Name, d.OpenAccess).Scan(&id); err != nil {
			return err
		}
		if err := setShares(ctx, tx, id, d.Owner, d.Shares); err != nil {
			return err
		}
		return insertEntries(ctx, tx, id, d.Items, 0)
	})
	return id, err
}

// Get returns a playlist.
func (s *Store) Get(ctx context.Context, id accounts.ID) (Playlist, error) {
	found, err := s.load(ctx, "WHERE id = $1", id)
	if err != nil {
		return Playlist{}, err
	}
	if len(found) == 0 {
		return Playlist{}, ErrNotFound
	}
	return found[0], nil
}

// visibleTo selects, on the playlists table, the playlists user $1 may see.
const visibleTo = `owner_id = $1 OR open_access
	OR EXISTS (SELECT 1 FROM playlist_shares WHERE playlist_id = playlists.id AND user_id = $1)`

// Visible lists the playlists user may see, by name.
func (s *Store) Visible(ctx context.Context, user accounts.ID) ([]Playlist, error) {
	return s.load(ctx, "WHERE "+visibleTo, user)
}

// AnyVisible reports whether user may see at least one playlist.
func (s *Store) AnyVisible(ctx context.Context, user accounts.ID) (bool, error) {
	var found bool
	err := s.db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM playlists WHERE "+visibleTo+")", user).Scan(&found)
	return found, err
}

// load reads the playlists a condition on the playlists table selects, by
// name, with their shares and entries.
func (s *Store) load(ctx context.Context, where string, args ...any) ([]Playlist, error) {
	rows, _ := s.db.Query(ctx, "SELECT id, owner_id, name, open_access, created_at, last_added_at FROM playlists "+
		where+" ORDER BY lower(name), id", args...)
	var result []Playlist
	var p Playlist
	if _, err := pgx.ForEachRow(rows, []any{&p.ID, &p.Owner, &p.Name, &p.OpenAccess, &p.Created, &p.LastAdded}, func() error {
		result = append(result, p)
		return nil
	}); err != nil || len(result) == 0 {
		return result, err
	}
	index := make(map[accounts.ID]int, len(result))
	ids := make([]accounts.ID, len(result))
	for i, p := range result {
		index[p.ID], ids[i] = i, p.ID
	}

	var playlist accounts.ID
	var share Share
	rows, _ = s.db.Query(ctx, "SELECT playlist_id, user_id, can_edit FROM playlist_shares WHERE playlist_id = ANY($1) ORDER BY ordinal", ids)
	if _, err := pgx.ForEachRow(rows, []any{&playlist, &share.User, &share.CanEdit}, func() error {
		result[index[playlist]].Shares = append(result[index[playlist]].Shares, share)
		return nil
	}); err != nil {
		return nil, err
	}
	var entry Entry
	rows, _ = s.db.Query(ctx, "SELECT playlist_id, id, item_id FROM playlist_entries WHERE playlist_id = ANY($1) ORDER BY position", ids)
	if _, err := pgx.ForEachRow(rows, []any{&playlist, &entry.ID, &entry.Item}, func() error {
		result[index[playlist]].Entries = append(result[index[playlist]].Entries, entry)
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

// Changes are the parts of a playlist an update replaces; nil parts are
// kept.
type Changes struct {
	Name       *string
	OpenAccess *bool
	Shares     *[]Share
	// Items replaces every entry.
	Items *[]accounts.ID
}

// Update changes a playlist.
func (s *Store) Update(ctx context.Context, id accounts.ID, c Changes) error {
	return s.change(ctx, id, func(tx pgx.Tx, owner accounts.ID) error {
		if c.Name != nil {
			if _, err := tx.Exec(ctx, "UPDATE playlists SET name = $2 WHERE id = $1", id, *c.Name); err != nil {
				return err
			}
		}
		if c.OpenAccess != nil {
			if _, err := tx.Exec(ctx, "UPDATE playlists SET open_access = $2 WHERE id = $1", id, *c.OpenAccess); err != nil {
				return err
			}
		}
		if c.Shares != nil {
			if _, err := tx.Exec(ctx, "DELETE FROM playlist_shares WHERE playlist_id = $1", id); err != nil {
				return err
			}
			if err := setShares(ctx, tx, id, owner, *c.Shares); err != nil {
				return err
			}
		}
		if c.Items != nil {
			if _, err := tx.Exec(ctx, "DELETE FROM playlist_entries WHERE playlist_id = $1", id); err != nil {
				return err
			}
			return insertEntries(ctx, tx, id, *c.Items, 0)
		}
		return nil
	})
}

// Add inserts items at position, or after the last entry when position is
// past it; a negative position counts as the first.
func (s *Store) Add(ctx context.Context, id accounts.ID, items []accounts.ID, position int) error {
	if len(items) == 0 {
		return nil
	}
	return s.change(ctx, id, func(tx pgx.Tx, _ accounts.ID) error {
		var count int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM playlist_entries WHERE playlist_id = $1", id).Scan(&count); err != nil {
			return err
		}
		position = min(max(position, 0), count)
		if _, err := tx.Exec(ctx, "UPDATE playlist_entries SET position = position + $3 WHERE playlist_id = $1 AND position >= $2",
			id, position, len(items)); err != nil {
			return err
		}
		return insertEntries(ctx, tx, id, items, position)
	})
}

// Remove deletes entries; those the playlist does not have are ignored.
func (s *Store) Remove(ctx context.Context, id accounts.ID, entries []accounts.ID) error {
	return s.change(ctx, id, func(tx pgx.Tx, _ accounts.ID) error {
		if _, err := tx.Exec(ctx, "DELETE FROM playlist_entries WHERE playlist_id = $1 AND id = ANY($2)", id, entries); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE playlist_entries e SET position = o.position
			FROM (SELECT id, row_number() OVER (ORDER BY position) - 1 AS position FROM playlist_entries WHERE playlist_id = $1) o
			WHERE e.id = o.id AND e.position <> o.position`, id)
		return err
	})
}

// Move places entry just before the entry before, or last when before is
// the zero identifier or no longer in the playlist. An entry the playlist
// does not have is ignored.
func (s *Store) Move(ctx context.Context, id, entry, before accounts.ID) error {
	return s.change(ctx, id, func(tx pgx.Tx, _ accounts.ID) error {
		rows, _ := tx.Query(ctx, "SELECT id FROM playlist_entries WHERE playlist_id = $1 ORDER BY position", id)
		order, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
		if err != nil {
			return err
		}
		from := slices.Index(order, entry)
		if from < 0 || entry == before {
			return nil
		}
		order = slices.Delete(order, from, from+1)
		to := slices.Index(order, before)
		if to < 0 {
			to = len(order)
		}
		order = slices.Insert(order, to, entry)
		_, err = tx.Exec(ctx, `UPDATE playlist_entries e SET position = o.position - 1
			FROM unnest($2::uuid[]) WITH ORDINALITY AS o(id, position)
			WHERE e.playlist_id = $1 AND e.id = o.id AND e.position <> o.position - 1`, id, order)
		return err
	})
}

// SetShare shares a playlist with a user, or changes what they may do. Like
// Jellyfin, it puts them last among the playlist's users.
func (s *Store) SetShare(ctx context.Context, id accounts.ID, share Share) error {
	return s.change(ctx, id, func(tx pgx.Tx, owner accounts.ID) error {
		if _, err := tx.Exec(ctx, "DELETE FROM playlist_shares WHERE playlist_id = $1 AND user_id = $2", id, share.User); err != nil {
			return err
		}
		return setShares(ctx, tx, id, owner, []Share{share})
	})
}

// RemoveShare stops sharing a playlist with a user.
func (s *Store) RemoveShare(ctx context.Context, id, user accounts.ID) error {
	_, err := s.db.Exec(ctx, "DELETE FROM playlist_shares WHERE playlist_id = $1 AND user_id = $2", id, user)
	return err
}

// Delete deletes a playlist.
func (s *Store) Delete(ctx context.Context, id accounts.ID) error {
	_, err := s.db.Exec(ctx, "DELETE FROM playlists WHERE id = $1", id)
	return err
}

// change runs apply on a playlist, locked against concurrent changes, with
// its owner.
func (s *Store) change(ctx context.Context, id accounts.ID, apply func(pgx.Tx, accounts.ID) error) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var owner accounts.ID
		err := tx.QueryRow(ctx, "SELECT owner_id FROM playlists WHERE id = $1 FOR UPDATE", id).Scan(&owner)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return apply(tx, owner)
	})
}

// setShares adds shares after the existing ones. The owner needs none, and
// users that no longer exist are left out; a user named twice keeps the
// last share.
func setShares(ctx context.Context, tx pgx.Tx, id, owner accounts.ID, shares []Share) error {
	users := make([]accounts.ID, 0, len(shares))
	edits := make([]bool, 0, len(shares))
	for i, share := range shares {
		later := slices.ContainsFunc(shares[i+1:], func(s Share) bool { return s.User == share.User })
		if share.User == owner || later {
			continue
		}
		users, edits = append(users, share.User), append(edits, share.CanEdit)
	}
	if len(users) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `INSERT INTO playlist_shares (playlist_id, user_id, can_edit)
		SELECT $1, s.user_id, s.can_edit FROM unnest($2::uuid[], $3::boolean[]) WITH ORDINALITY AS s(user_id, can_edit, n)
		JOIN users ON users.id = s.user_id ORDER BY s.n`, id, users, edits)
	return err
}

// insertEntries adds items as entries from position on, which the caller
// left free.
func insertEntries(ctx context.Context, tx pgx.Tx, id accounts.ID, items []accounts.ID, position int) error {
	if len(items) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `INSERT INTO playlist_entries (playlist_id, item_id, position)
		SELECT $1, item_id, $3 + n - 1 FROM unnest($2::uuid[]) WITH ORDINALITY AS i(item_id, n)`, id, items, position); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "UPDATE playlists SET last_added_at = now() WHERE id = $1", id)
	return err
}
