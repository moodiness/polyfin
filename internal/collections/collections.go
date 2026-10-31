// Package collections keeps the collections of titles users make, as
// Jellyfin's BoxSets: server-wide groups every user sees, which the users
// allowed to manage collections create, change and delete.
package collections

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
)

// ErrNotFound reports a collection that does not exist.
var ErrNotFound = errors.New("collection not found")

// MaxNameLength bounds a collection's name, in characters.
const MaxNameLength = 500

// Collection is a group of titles.
type Collection struct {
	ID   accounts.ID
	Name string
	// IsLocked is Jellyfin's IsLocked: no metadata is looked up for the
	// collection. Polyfin looks up none anyway; apps send and read it.
	IsLocked bool
	// Items are the collection's titles, in the order they were added.
	Items   []accounts.ID
	Created time.Time
	// LastAdded is when titles were last added, nil before any.
	LastAdded *time.Time
}

// Store keeps collections in PostgreSQL.
type Store struct {
	db *pgxpool.Pool
}

// New returns a store backed by db.
func New(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

// Create stores a new collection of items and returns its identifier.
func (s *Store) Create(ctx context.Context, name string, isLocked bool, items []accounts.ID) (accounts.ID, error) {
	var id accounts.ID
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "INSERT INTO collections (name, is_locked) VALUES ($1, $2) RETURNING id", name, isLocked).Scan(&id); err != nil {
			return err
		}
		_, err := insertItems(ctx, tx, id, items)
		return err
	})
	return id, err
}

// Get returns a collection.
func (s *Store) Get(ctx context.Context, id accounts.ID) (Collection, error) {
	found, err := s.load(ctx, "WHERE id = $1", id)
	if err != nil {
		return Collection{}, err
	}
	if len(found) == 0 {
		return Collection{}, ErrNotFound
	}
	return found[0], nil
}

// All lists every collection, by name.
func (s *Store) All(ctx context.Context) ([]Collection, error) {
	return s.load(ctx, "")
}

// Any reports whether at least one collection exists.
func (s *Store) Any(ctx context.Context) (bool, error) {
	var found bool
	err := s.db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM collections)").Scan(&found)
	return found, err
}

// load reads the collections a condition on the collections table selects,
// by name, with their titles.
func (s *Store) load(ctx context.Context, where string, args ...any) ([]Collection, error) {
	rows, _ := s.db.Query(ctx, "SELECT id, name, is_locked, created_at, last_added_at FROM collections "+
		where+" ORDER BY lower(name), id", args...)
	var result []Collection
	var c Collection
	if _, err := pgx.ForEachRow(rows, []any{&c.ID, &c.Name, &c.IsLocked, &c.Created, &c.LastAdded}, func() error {
		result = append(result, c)
		return nil
	}); err != nil || len(result) == 0 {
		return result, err
	}
	index := make(map[accounts.ID]int, len(result))
	ids := make([]accounts.ID, len(result))
	for i, c := range result {
		index[c.ID], ids[i] = i, c.ID
	}
	var collection, item accounts.ID
	rows, _ = s.db.Query(ctx, "SELECT collection_id, item_id FROM collection_items WHERE collection_id = ANY($1) ORDER BY ordinal", ids)
	if _, err := pgx.ForEachRow(rows, []any{&collection, &item}, func() error {
		result[index[collection]].Items = append(result[index[collection]].Items, item)
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

// Add adds items after the collection's titles; those it already has keep
// their place.
func (s *Store) Add(ctx context.Context, id accounts.ID, items []accounts.ID) error {
	return s.change(ctx, id, func(tx pgx.Tx) error {
		_, err := insertItems(ctx, tx, id, items)
		return err
	})
}

// Remove takes items out of the collection; those it does not have are
// ignored.
func (s *Store) Remove(ctx context.Context, id accounts.ID, items []accounts.ID) error {
	return s.change(ctx, id, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "DELETE FROM collection_items WHERE collection_id = $1 AND item_id = ANY($2)", id, items)
		return err
	})
}

// Delete deletes a collection.
func (s *Store) Delete(ctx context.Context, id accounts.ID) error {
	tag, err := s.db.Exec(ctx, "DELETE FROM collections WHERE id = $1", id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// change runs apply on a collection, locked against concurrent changes.
func (s *Store) change(ctx context.Context, id accounts.ID, apply func(pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, "SELECT id FROM collections WHERE id = $1 FOR UPDATE", id).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return apply(tx)
	})
}

// insertItems adds the items the collection does not have yet, in order;
// an item named twice is added once. It counts the items added.
func insertItems(ctx context.Context, tx pgx.Tx, id accounts.ID, items []accounts.ID) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	tag, err := tx.Exec(ctx, `INSERT INTO collection_items (collection_id, item_id)
		SELECT $1, item_id FROM (SELECT item_id, min(n) AS n FROM unnest($2::uuid[]) WITH ORDINALITY AS i(item_id, n) GROUP BY item_id) i
		ORDER BY n ON CONFLICT DO NOTHING`, id, items)
	if err != nil || tag.RowsAffected() == 0 {
		return 0, err
	}
	_, err = tx.Exec(ctx, "UPDATE collections SET last_added_at = now() WHERE id = $1", id)
	return int(tag.RowsAffected()), err
}

// Import brings a collection of another server, which server names by
// its kind and its own identifier, and source names there: it adds items
// after the titles of the collection an earlier import brought from it,
// else creates one named name with them, which by's import brought. A
// collection deleted since is created again. Without items, and none
// brought before, it creates nothing, and returns the zero identifier. It
// reports whether it created the collection, else how many items it added
// to the one brought before.
func (s *Store) Import(ctx context.Context, server, source string, by accounts.ID, name string,
	items []accounts.ID) (id accounts.ID, created bool, added int, err error) {
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		// Locking the collection keeps it from being deleted meanwhile.
		err := tx.QueryRow(ctx, `SELECT c.id FROM imported_collections i JOIN collections c ON c.id = i.collection_id
			WHERE i.server = $1 AND i.source_id = $2 FOR UPDATE OF c`, server, source).Scan(&id)
		switch {
		case err == nil:
			added, err = insertItems(ctx, tx, id, items)
			return err
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		case len(items) == 0:
			return nil
		}
		if err := tx.QueryRow(ctx, "INSERT INTO collections (name) VALUES ($1) RETURNING id",
			truncate(name, MaxNameLength)).Scan(&id); err != nil {
			return err
		}
		created = true
		// A user deleted meanwhile leaves the collection without one.
		if _, err := tx.Exec(ctx, `INSERT INTO imported_collections (server, source_id, collection_id, imported_by)
			VALUES ($1, $2, $3, (SELECT id FROM users WHERE id = $4))
			ON CONFLICT (server, source_id) DO UPDATE SET collection_id = EXCLUDED.collection_id,
				imported_by = EXCLUDED.imported_by, imported_at = now()`, server, source, id, by); err != nil {
			return err
		}
		_, err = insertItems(ctx, tx, id, items)
		return err
	})
	if err != nil {
		return accounts.ID{}, false, 0, err
	}
	return id, created, added, nil
}

// truncate is text cut to its first max characters.
func truncate(text string, max int) string {
	for i := range text {
		if max == 0 {
			return text[:i]
		}
		max--
	}
	return text
}
