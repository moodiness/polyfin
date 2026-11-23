// Package collections keeps the collections of titles users make, as
// Jellyfin's BoxSets: server-wide groups every user sees, which the users
// allowed to manage collections create, change and delete. A collection
// may hold other collections, as a box set holds box sets in Jellyfin.
package collections

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
)

// ErrNotFound reports a collection that does not exist.
var ErrNotFound = errors.New("collection not found")

// ErrCycle reports a collection put inside itself, or inside a collection
// it holds at any depth.
var ErrCycle = errors.New("a collection cannot hold itself")

// ErrTooDeep reports collections nested deeper than MaxDepth.
var ErrTooDeep = errors.New("collections nested too deep")

// MaxNameLength bounds a collection's name, in characters.
const MaxNameLength = 500

// MaxDepth bounds how deep collections nest, counting the outermost: a
// collection, the collections it holds, theirs, and theirs.
const MaxDepth = 4

// nestingLock is the advisory lock that changes nesting collections, and
// deletions, take first: holding it, a change sees every other nesting
// committed, so that two changes never make a cycle together, and no
// collection is deleted while another is put into it.
const nestingLock int64 = 0x706f6c79666e6573 // "polyfnes"

// DisplayOrder is the order apps list a collection in.
type DisplayOrder string

const (
	// OrderPremiere lists a collection by premiere date, as Jellyfin
	// orders a collection; the default.
	OrderPremiere DisplayOrder = "premiere"
	// OrderChosen lists it in the order of its entries, which an
	// administrator may choose in the admin app (see Reorder).
	OrderChosen DisplayOrder = "chosen"
)

// Valid reports whether o is one of the orders.
func (o DisplayOrder) Valid() bool { return o == OrderPremiere || o == OrderChosen }

// ErrEntriesChanged reports an order that does not name the entries a
// collection has, as when they changed since it was read.
var ErrEntriesChanged = errors.New("the collection's entries changed")

// Collection is a group of titles and other collections.
type Collection struct {
	ID   accounts.ID
	Name string
	// IsLocked is Jellyfin's IsLocked: no metadata is looked up for the
	// collection. Polyfin looks up none anyway; apps send and read it.
	IsLocked bool
	// DisplayOrder is the order apps list it in.
	DisplayOrder DisplayOrder
	// Items are the collection's titles, in its order: that they were
	// added in, unless an administrator chose another.
	Items []accounts.ID
	// Collections are the collections it holds, in its order.
	Collections []accounts.ID
	// Entries are its titles and collections together, in its order.
	Entries []accounts.ID
	Created time.Time
	// LastAdded is when titles or collections were last added, nil before
	// any.
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

// Create stores a new collection of items, titles and collections, and
// returns its identifier. It returns ErrTooDeep for collections nested
// too deep under it, and creates nothing.
func (s *Store) Create(ctx context.Context, name string, isLocked bool, items []accounts.ID) (accounts.ID, error) {
	var id accounts.ID
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := lockNesting(ctx, tx, items); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, "INSERT INTO collections (name, is_locked) VALUES ($1, $2) RETURNING id", name, isLocked).Scan(&id); err != nil {
			return err
		}
		if err := checkNesting(ctx, tx, id, items); err != nil {
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

// Some returns the collections among ids, by name; those that do not
// exist are left out.
func (s *Store) Some(ctx context.Context, ids []accounts.ID) ([]Collection, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return s.load(ctx, "WHERE id = ANY($1)", ids)
}

// Holders returns the collections that hold the collection id itself, by
// name.
func (s *Store) Holders(ctx context.Context, id accounts.ID) ([]Collection, error) {
	return s.load(ctx, "WHERE id IN (SELECT collection_id FROM collection_items WHERE item_id = $1)", id)
}

// Changes are what Change changes of a collection; nil leaves a value as
// it is.
type Changes struct {
	Name         *string
	DisplayOrder *DisplayOrder
}

// Change renames a collection, or sets its display order.
func (s *Store) Change(ctx context.Context, id accounts.ID, changes Changes) error {
	var order *string
	if changes.DisplayOrder != nil {
		order = new(string(*changes.DisplayOrder))
	}
	tag, err := s.db.Exec(ctx, "UPDATE collections SET name = coalesce($2, name), display_order = coalesce($3, display_order) WHERE id = $1",
		id, changes.Name, order)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// Reorder puts the entries of a collection in the order of entries,
// which must name each of them once: else it returns ErrEntriesChanged
// and changes nothing. Entries added later come after them.
func (s *Store) Reorder(ctx context.Context, id accounts.ID, entries []accounts.ID) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := lockCollection(ctx, tx, id); err != nil {
			return err
		}
		rows, _ := tx.Query(ctx, "SELECT item_id FROM collection_items WHERE collection_id = $1", id)
		held, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
		if err != nil {
			return err
		}
		named := make(map[accounts.ID]bool, len(entries))
		for _, entry := range entries {
			named[entry] = true
		}
		if len(named) != len(entries) || len(held) != len(entries) ||
			slices.ContainsFunc(held, func(entry accounts.ID) bool { return !named[entry] }) {
			return ErrEntriesChanged
		}
		// The ordinal is an identity: each entry takes the next value, in
		// the order asked.
		batch := &pgx.Batch{}
		for _, entry := range entries {
			batch.Queue("UPDATE collection_items SET ordinal = DEFAULT WHERE collection_id = $1 AND item_id = $2", id, entry)
		}
		return tx.SendBatch(ctx, batch).Close()
	})
}

// Refusal tells why the collection into may not hold the collection item,
// as Add would refuse it, from all, every collection: ErrCycle when item
// is into or holds it at any depth, ErrTooDeep when they would nest
// deeper than MaxDepth; nil when it may.
func Refusal(all []Collection, into, item accounts.ID) error {
	byID := make(map[accounts.ID]Collection, len(all))
	parents := map[accounts.ID][]accounts.ID{}
	for _, c := range all {
		byID[c.ID] = c
		for _, inner := range c.Collections {
			parents[inner] = append(parents[inner], c.ID)
		}
	}
	// The levels from the outermost collection holding into down to into,
	// and whether item is among them. Nesting never goes past MaxDepth,
	// which bounds the walks.
	above, cycle := 0, false
	var up func(id accounts.ID, level int)
	up = func(id accounts.ID, level int) {
		above = max(above, level)
		cycle = cycle || id == item
		if level <= MaxDepth {
			for _, parent := range parents[id] {
				up(parent, level+1)
			}
		}
	}
	up(into, 1)
	if cycle {
		return ErrCycle
	}
	var down func(id accounts.ID, level int) int
	down = func(id accounts.ID, level int) int {
		deepest := level
		if level <= MaxDepth {
			for _, inner := range byID[id].Collections {
				deepest = max(deepest, down(inner, level+1))
			}
		}
		return deepest
	}
	if above+down(item, 1) > MaxDepth {
		return ErrTooDeep
	}
	return nil
}

// load reads the collections a condition on the collections table selects,
// by name, with their titles and collections.
func (s *Store) load(ctx context.Context, where string, args ...any) ([]Collection, error) {
	rows, _ := s.db.Query(ctx, "SELECT id, name, is_locked, display_order, created_at, last_added_at FROM collections "+
		where+" ORDER BY lower(name), id", args...)
	var result []Collection
	var c Collection
	var order string
	if _, err := pgx.ForEachRow(rows, []any{&c.ID, &c.Name, &c.IsLocked, &order, &c.Created, &c.LastAdded}, func() error {
		c.DisplayOrder = DisplayOrder(order)
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
	var nested bool
	rows, _ = s.db.Query(ctx, `SELECT i.collection_id, i.item_id, c.id IS NOT NULL FROM collection_items i
		LEFT JOIN collections c ON c.id = i.item_id WHERE i.collection_id = ANY($1) ORDER BY i.ordinal`, ids)
	if _, err := pgx.ForEachRow(rows, []any{&collection, &item, &nested}, func() error {
		c := &result[index[collection]]
		c.Entries = append(c.Entries, item)
		if nested {
			c.Collections = append(c.Collections, item)
		} else {
			c.Items = append(c.Items, item)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

// Add adds items, titles and collections, after the collection's entries;
// those it already has keep their place. It returns ErrCycle for a
// collection that is this one or holds it at any depth, and ErrTooDeep
// for collections nested too deep, and adds nothing.
func (s *Store) Add(ctx context.Context, id accounts.ID, items []accounts.ID) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := lockNesting(ctx, tx, items); err != nil {
			return err
		}
		if err := lockCollection(ctx, tx, id); err != nil {
			return err
		}
		if err := checkNesting(ctx, tx, id, items); err != nil {
			return err
		}
		_, err := insertItems(ctx, tx, id, items)
		return err
	})
}

// Remove takes items, titles and collections, out of the collection;
// those it does not have are ignored.
func (s *Store) Remove(ctx context.Context, id accounts.ID, items []accounts.ID) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := lockCollection(ctx, tx, id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "DELETE FROM collection_items WHERE collection_id = $1 AND item_id = ANY($2)", id, items)
		return err
	})
}

// Delete deletes a collection, and takes it out of the collections that
// held it.
func (s *Store) Delete(ctx context.Context, id accounts.ID) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", nestingLock); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "DELETE FROM collections WHERE id = $1", id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		_, err = tx.Exec(ctx, "DELETE FROM collection_items WHERE item_id = $1", id)
		return err
	})
}

// lockCollection locks a collection against concurrent changes.
func lockCollection(ctx context.Context, tx pgx.Tx, id accounts.ID) error {
	err := tx.QueryRow(ctx, "SELECT id FROM collections WHERE id = $1 FOR UPDATE", id).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// lockNesting takes the nesting lock before a change adds items, which
// may be collections. It comes before any collection's lock, as in
// Delete, so that no two changes wait on each other.
func lockNesting(ctx context.Context, tx pgx.Tx, items []accounts.ID) error {
	if len(items) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", nestingLock)
	return err
}

// checkNesting checks that the collection id may hold the collections
// among items, under the nesting lock: none of them is id or holds it at
// any depth (ErrCycle), and the collections above id, id, and those
// nested in the deepest of them make at most MaxDepth levels
// (ErrTooDeep).
func checkNesting(ctx context.Context, tx pgx.Tx, id accounts.ID, items []accounts.ID) error {
	if len(items) == 0 {
		return nil
	}
	rows, _ := tx.Query(ctx, "SELECT id FROM collections WHERE id = ANY($1)", items)
	inner, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
	if err != nil || len(inner) == 0 {
		return err
	}
	// The collections that hold id, at any depth, id included at level 1.
	// Nesting never goes past MaxDepth, which bounds the walk.
	var cycle bool
	var above int
	if err := tx.QueryRow(ctx, `WITH RECURSIVE up (id, level) AS (
			SELECT $1::uuid, 1
			UNION
			SELECT i.collection_id, up.level + 1 FROM collection_items i JOIN up ON i.item_id = up.id WHERE up.level <= $3)
		SELECT coalesce(bool_or(id = ANY($2)), false), max(level) FROM up`, id, inner, MaxDepth).Scan(&cycle, &above); err != nil {
		return err
	}
	if cycle {
		return ErrCycle
	}
	// The levels of the deepest of the collections added, itself included.
	var below int
	if err := tx.QueryRow(ctx, `WITH RECURSIVE down (id, level) AS (
			SELECT unnest($1::uuid[]), 1
			UNION
			SELECT i.item_id, down.level + 1 FROM collection_items i JOIN down ON i.collection_id = down.id
			WHERE down.level <= $2 AND EXISTS (SELECT 1 FROM collections c WHERE c.id = i.item_id))
		SELECT max(level) FROM down`, inner, MaxDepth).Scan(&below); err != nil {
		return err
	}
	if above+below > MaxDepth {
		return ErrTooDeep
	}
	return nil
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
