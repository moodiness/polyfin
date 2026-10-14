// Package smartlists keeps the smart lists: lists of titles drawn from
// catalogs of addons, which keep the titles that match their rules over
// what the titles are (genre, year, IMDb rating, language, movie or series,
// place in their catalog) and over what the user who opens them did with
// them (played, unplayed, started, favorite). Apps see a list as a library
// or as a collection. The server's lists, made by administrators, are seen
// by every user; a user's own lists only by them.
//
// This package keeps the lists and judges a title by their rules (see
// Rule.Judge); the library reads their catalogs and lists them.
package smartlists

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
)

var (
	// ErrNotFound reports a list that does not exist, or not in the scope
	// asked.
	ErrNotFound = errors.New("smart list not found")
	// ErrInvalidName reports an empty name, or one longer than
	// MaxNameLength.
	ErrInvalidName = errors.New("a smart list's name has 1 to 64 characters")
	// ErrInvalidCatalogs reports a list without catalogs, with more than
	// MaxCatalogs, or naming one twice.
	ErrInvalidCatalogs = errors.New("a smart list draws from 1 to 50 catalogs, each once")
	// ErrInvalidRules reports rules that are not well formed (see
	// Rule.Validate).
	ErrInvalidRules = errors.New("a smart list's rules are not valid")
	// ErrInvalidSort reports an unknown sort.
	ErrInvalidSort = errors.New("a smart list's sort is catalog, premiere, rating, name or random")
	// ErrInvalidMaxItems reports a maximum outside 1 to MaxItems.
	ErrInvalidMaxItems = errors.New("a smart list's maximum is a whole number from 1 to 20,000")
	// ErrInvalidShownAs reports a list shown as neither a library nor a
	// collection.
	ErrInvalidShownAs = errors.New("a smart list is shown as a library or as a collection")
	// ErrTooMany reports a scope that holds MaxLists lists already.
	ErrTooMany = errors.New("a scope holds at most 100 smart lists")
)

const (
	// MaxNameLength bounds a list's name, in characters, as a library's.
	MaxNameLength = 64
	// MaxCatalogs bounds the catalogs a list draws from.
	MaxCatalogs = 50
	// MaxItems bounds a list's maximum, as a library's.
	MaxItems = 20000
	// MaxLists bounds the lists of a scope: every request a user makes
	// reads theirs.
	MaxLists = 100
)

// How apps see a list.
const (
	ShownAsLibrary    = "library"
	ShownAsCollection = "collection"
)

// How a list orders its titles: one of each catalog in turn, as an addon's
// collection lists them; by premiere date, the latest first; by IMDb
// rating, the highest first; by name; or at random, in an order drawn
// again each day.
const (
	SortCatalog  = "catalog"
	SortPremiere = "premiere"
	SortRating   = "rating"
	SortName     = "name"
	SortRandom   = "random"
)

// Catalog names a catalog of an addon a list draws from.
type Catalog struct {
	AddonID accounts.ID
	Type    string
	ID      string
}

// catalogJSON is a catalog as lists keep it.
type catalogJSON struct {
	AddonID string `json:"addonId"`
	Type    string `json:"type"`
	ID      string `json:"id"`
}

func (c Catalog) MarshalJSON() ([]byte, error) {
	return json.Marshal(catalogJSON{c.AddonID.String(), c.Type, c.ID})
}

func (c *Catalog) UnmarshalJSON(data []byte) error {
	var kept catalogJSON
	if err := json.Unmarshal(data, &kept); err != nil {
		return err
	}
	id, err := accounts.ParseID(kept.AddonID)
	if err != nil {
		return err
	}
	*c = Catalog{id, kept.Type, kept.ID}
	return nil
}

// List is a smart list.
type List struct {
	ID accounts.ID
	// Owner is the user whose own list it is, nil for the server's.
	Owner    *accounts.ID
	Name     string
	ShownAs  string
	Catalogs []Catalog
	// Rules is the group of rules a title must match (see Rule).
	Rules Rule
	Sort  string
	// MaxItems is the most titles the list lists, 0 for no maximum.
	MaxItems  int
	UpdatedAt time.Time
}

// Validate checks a list before it is saved.
func (l List) Validate() error {
	name := strings.TrimSpace(l.Name)
	if name == "" || utf8.RuneCountInString(name) > MaxNameLength {
		return ErrInvalidName
	}
	if l.ShownAs != ShownAsLibrary && l.ShownAs != ShownAsCollection {
		return ErrInvalidShownAs
	}
	if len(l.Catalogs) == 0 || len(l.Catalogs) > MaxCatalogs {
		return ErrInvalidCatalogs
	}
	seen := map[Catalog]bool{}
	for _, c := range l.Catalogs {
		if c.AddonID == (accounts.ID{}) || c.Type == "" || c.ID == "" || seen[c] {
			return ErrInvalidCatalogs
		}
		seen[c] = true
	}
	if err := l.Rules.Validate(); err != nil {
		return err
	}
	switch l.Sort {
	case SortCatalog, SortPremiere, SortRating, SortName, SortRandom:
	default:
		return ErrInvalidSort
	}
	if l.MaxItems < 0 || l.MaxItems > MaxItems {
		return ErrInvalidMaxItems
	}
	return nil
}

// Store keeps the smart lists in PostgreSQL.
type Store struct {
	db *pgxpool.Pool
}

// New returns a store backed by db.
func New(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

const columns = "id, owner_id, name, shown_as, catalogs, rules, sort, coalesce(max_items, 0), updated_at"

// Lists returns the lists of a scope, the server's for a nil owner, in the
// order they were made.
func (s *Store) Lists(ctx context.Context, owner *accounts.ID) ([]List, error) {
	if owner == nil {
		return s.load(ctx, "WHERE owner_id IS NULL")
	}
	return s.load(ctx, "WHERE owner_id = $1", *owner)
}

// Seen returns the lists a user sees, in order: the server's when shared
// is set, then the user's own when own is set.
func (s *Store) Seen(ctx context.Context, user accounts.ID, shared, own bool) ([]List, error) {
	if !shared && !own {
		return nil, nil
	}
	return s.load(ctx, `WHERE ($1 AND owner_id IS NULL) OR ($2 AND owner_id = $3)
		ORDER BY owner_id IS NOT NULL, created_at, id`, shared, own, user)
}

// Get returns one list of a scope.
func (s *Store) Get(ctx context.Context, owner *accounts.ID, id accounts.ID) (List, error) {
	found, err := s.load(ctx, "WHERE id = $1 AND owner_id IS NOT DISTINCT FROM $2", id, owner)
	if err != nil {
		return List{}, err
	}
	if len(found) == 0 {
		return List{}, ErrNotFound
	}
	return found[0], nil
}

func (s *Store) load(ctx context.Context, where string, args ...any) ([]List, error) {
	if !strings.Contains(where, "ORDER BY") {
		where += " ORDER BY created_at, id"
	}
	rows, _ := s.db.Query(ctx, "SELECT "+columns+" FROM smart_lists "+where, args...)
	var result []List
	var l List
	var catalogs, rules []byte
	_, err := pgx.ForEachRow(rows, []any{&l.ID, &l.Owner, &l.Name, &l.ShownAs, &catalogs, &rules, &l.Sort, &l.MaxItems, &l.UpdatedAt}, func() error {
		l.Catalogs, l.Rules = nil, Rule{}
		if err := json.Unmarshal(catalogs, &l.Catalogs); err != nil {
			return err
		}
		if err := json.Unmarshal(rules, &l.Rules); err != nil {
			return err
		}
		result = append(result, l)
		return nil
	})
	return result, err
}

// Create stores a new list in the scope of l.Owner and returns it.
func (s *Store) Create(ctx context.Context, l List) (List, error) {
	if err := l.Validate(); err != nil {
		return List{}, err
	}
	catalogs, rules, err := encoded(l)
	if err != nil {
		return List{}, err
	}
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		// Lists of one scope are counted one at a time.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('smart_lists'))"); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM smart_lists WHERE owner_id IS NOT DISTINCT FROM $1", l.Owner).Scan(&count); err != nil {
			return err
		}
		if count >= MaxLists {
			return ErrTooMany
		}
		return tx.QueryRow(ctx, `INSERT INTO smart_lists (owner_id, name, shown_as, catalogs, rules, sort, max_items)
			VALUES ($1, $2, $3, $4, $5, $6, nullif($7, 0)) RETURNING id, updated_at`,
			l.Owner, strings.TrimSpace(l.Name), l.ShownAs, catalogs, rules, l.Sort, l.MaxItems).Scan(&l.ID, &l.UpdatedAt)
	})
	l.Name = strings.TrimSpace(l.Name)
	return l, err
}

// Update replaces a list of the scope of l.Owner and returns it.
func (s *Store) Update(ctx context.Context, l List) (List, error) {
	if err := l.Validate(); err != nil {
		return List{}, err
	}
	catalogs, rules, err := encoded(l)
	if err != nil {
		return List{}, err
	}
	l.Name = strings.TrimSpace(l.Name)
	err = s.db.QueryRow(ctx, `UPDATE smart_lists SET name = $3, shown_as = $4, catalogs = $5, rules = $6, sort = $7,
		max_items = nullif($8, 0), updated_at = now()
		WHERE id = $1 AND owner_id IS NOT DISTINCT FROM $2 RETURNING updated_at`,
		l.ID, l.Owner, l.Name, l.ShownAs, catalogs, rules, l.Sort, l.MaxItems).Scan(&l.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return List{}, ErrNotFound
	}
	return l, err
}

// Delete deletes a list of a scope.
func (s *Store) Delete(ctx context.Context, owner *accounts.ID, id accounts.ID) error {
	tag, err := s.db.Exec(ctx, "DELETE FROM smart_lists WHERE id = $1 AND owner_id IS NOT DISTINCT FROM $2", id, owner)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func encoded(l List) (catalogs, rules []byte, err error) {
	if catalogs, err = json.Marshal(l.Catalogs); err != nil {
		return nil, nil, err
	}
	rules, err = json.Marshal(l.Rules)
	return catalogs, rules, err
}
