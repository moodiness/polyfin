// Package streamyfin keeps the rows of Streamyfin's home screen that an
// administrator chose. Streamyfin, a Jellyfin app, asks the server for the
// settings of its server plugin; Polyfin answers them with these rows (see
// jellyfin's Streamyfin config).
package streamyfin

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Kind is what a row lists.
type Kind string

const (
	// KindResume lists the user's titles to resume.
	KindResume Kind = "resume"
	// KindNextUp lists the next episodes of the series the user watches.
	KindNextUp Kind = "next_up"
	// KindLibrary lists a library's items: a collection library's
	// collections, or a library's titles.
	KindLibrary Kind = "library"
	// KindCollection lists a collection's titles, those of the collections
	// within it included.
	KindCollection Kind = "collection"
)

// MaxRows bounds the rows of the home screen, and MaxTitle a row's title,
// in characters.
const (
	MaxRows  = 30
	MaxTitle = 64
)

var (
	// ErrInvalidRow reports a row of an unknown kind, a library or
	// collection row without its item or a row of the user's own lists
	// with one, or a title longer than MaxTitle.
	ErrInvalidRow = errors.New("invalid Streamyfin row")
	// ErrTooManyRows reports more than MaxRows rows.
	ErrTooManyRows = errors.New("too many Streamyfin rows")
)

// Row is a row of the home screen. Item is the library or the collection,
// zero for the user's own lists, and ItemName its name when the row was
// saved; Title is the administrator's, empty for the default name.
type Row struct {
	Kind     Kind
	Item     accounts.ID
	ItemName string
	Title    string
}

// defaultNames are the default names of the user's own rows, as
// jellyfin-web names them in the server's language.
var defaultNames = map[string]map[Kind]string{
	"en": {KindResume: "Continue Watching", KindNextUp: "Next Up"},
	"fr": {KindResume: "Continuer de regarder", KindNextUp: "À suivre"},
}

// DefaultName is the name of a row of the user's own lists in language,
// English for a language Polyfin does not speak; "" for another kind.
func DefaultName(kind Kind, language string) string {
	names, ok := defaultNames[language]
	if !ok {
		names = defaultNames["en"]
	}
	return names[kind]
}

// Store keeps the rows in PostgreSQL.
type Store struct {
	db *pgxpool.Pool
}

// New returns a store backed by db.
func New(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

// Rows lists the rows, in order.
func (s *Store) Rows(ctx context.Context) ([]Row, error) {
	rows, err := s.db.Query(ctx, "SELECT kind, item_id, item_name, coalesce(title, '') FROM streamyfin_rows ORDER BY position")
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Row, error) {
		var r Row
		var item pgtype.UUID
		if err := row.Scan(&r.Kind, &item, &r.ItemName, &r.Title); err != nil {
			return Row{}, err
		}
		if item.Valid {
			r.Item = accounts.ID(item.Bytes)
		}
		return r, nil
	})
}

// Check validates rows' kinds, items and titles, and returns them with
// their titles trimmed; whether their items are libraries and collections
// is the caller's to check.
func Check(rows []Row) ([]Row, error) {
	if len(rows) > MaxRows {
		return nil, ErrTooManyRows
	}
	checked := make([]Row, 0, len(rows))
	for _, r := range rows {
		r.Title = strings.TrimSpace(r.Title)
		if utf8.RuneCountInString(r.Title) > MaxTitle || strings.ContainsRune(r.Title, 0) {
			return nil, ErrInvalidRow
		}
		switch r.Kind {
		case KindResume, KindNextUp:
			if r.Item != (accounts.ID{}) {
				return nil, ErrInvalidRow
			}
		case KindLibrary, KindCollection:
			if r.Item == (accounts.ID{}) {
				return nil, ErrInvalidRow
			}
		default:
			return nil, ErrInvalidRow
		}
		checked = append(checked, r)
	}
	return checked, nil
}

// SetRows replaces the rows with rows, in order (see Check).
func (s *Store) SetRows(ctx context.Context, rows []Row) error {
	rows, err := Check(rows)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "DELETE FROM streamyfin_rows"); err != nil {
			return err
		}
		for i, r := range rows {
			var item, title any
			if r.Item != (accounts.ID{}) {
				item = r.Item
			}
			if r.Title != "" {
				title = r.Title
			}
			if _, err := tx.Exec(ctx, "INSERT INTO streamyfin_rows (position, kind, item_id, item_name, title) VALUES ($1, $2, $3, $4, $5)",
				i+1, r.Kind, item, r.ItemName, title); err != nil {
				return err
			}
		}
		return nil
	})
}
