// Package homerows keeps the rows the administrator adds to the web
// player's home page (Settings › Web player › Home rows), when each shows
// (Row.ShowsAt), and the rows each user turned off (My account › Home
// rows). What a row lists, for whom, is told by the Jellyfin API (see
// jellyfin's customrows.go).
package homerows

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
)

// ErrNotFound reports a row that does not exist.
var ErrNotFound = errors.New("home row not found")

// ErrTooMany reports a row added past MaxRows.
var ErrTooMany = errors.New("too many home rows")

// ErrInvalid reports a row whose fields do not go together (see
// Row.Valid).
var ErrInvalid = errors.New("invalid home row")

const (
	// MaxRows bounds the rows of the server.
	MaxRows = 20
	// MaxNameLength bounds a row's name, in characters.
	MaxNameLength = 100
	// MaxCount bounds the rows a Because you watched row makes.
	MaxCount = 10
	// DefaultCount is how many rows a Because you watched row makes when
	// none is chosen.
	DefaultCount = 3
)

// Kind is what a row lists.
type Kind string

const (
	// KindCatalog lists the first page of a catalog of one of the
	// server's addons, a library or not.
	KindCatalog Kind = "catalog"
	// KindCollection lists a collection: one made by users, a smart list
	// seen as a collection, or an addon's.
	KindCollection Kind = "collection"
	// KindBecause makes a row for each of the movies and series the user
	// played last, of the titles similar to it.
	KindBecause Kind = "because"
	// KindNew lists the titles new in the user's libraries.
	KindNew Kind = "new"
	// KindAvailable lists the titles of the user's watchlist that versions
	// were found for, and KindWaiting those still without (see package
	// watchlist).
	KindAvailable Kind = "available"
	KindWaiting   Kind = "waiting"
)

// Kinds are the kinds of rows, in the order the admin app offers them.
var Kinds = []Kind{KindCatalog, KindCollection, KindBecause, KindNew, KindAvailable, KindWaiting}

// Place is where the web player shows a row on its home page.
type Place string

const (
	PlaceTop    Place = "top"
	PlaceResume Place = "resume"
	PlaceNextUp Place = "nextup"
	PlaceBottom Place = "bottom"
)

// Places are the places of rows, from the top of the page.
var Places = []Place{PlaceTop, PlaceResume, PlaceNextUp, PlaceBottom}

// Row is a row of the home page.
type Row struct {
	ID   accounts.ID
	Name string
	Kind Kind
	// Place is where the web player shows it.
	Place Place
	// Addon, CatalogType and CatalogID name the catalog of a catalog row.
	Addon       accounts.ID
	CatalogType string
	CatalogID   string
	// Collection names the collection of a collection row.
	Collection accounts.ID
	// Count is how many rows a Because you watched row makes.
	Count int
	// Everyone shows the row to every user; otherwise only Users see it.
	Everyone bool
	Users    []accounts.ID
	// Library makes the row a view of its own too, which other apps list
	// among the user's libraries.
	Library bool
	// Schedule is when the row shows.
	Schedule Schedule
}

// Schedule is when a row shows, in the server's time zone: from its Start
// date to its End date, on its Days, in its Hours. What it leaves out
// does not bound the row: the zero Schedule shows it all the time.
type Schedule struct {
	// Start and End are the first and the last dates the row shows on, as
	// accounts.Day gives dates: midnight UTC of the date. Zero leaves that
	// side open.
	Start, End time.Time
	// Days are the days of the week the row shows on, from Sunday, without
	// repeats; none is every day.
	Days []time.Weekday
	// Hours are the hours of the day it shows in; nil is all day.
	Hours *Hours
}

// Hours are the hours of the day a row shows in: from the start of hour
// From, 0 to 23, to the start of hour To, 1 to 24 (midnight). A To before
// From passes midnight, and that night belongs to the day it starts on:
// from 22 to 2 on Fridays shows the row from Friday 22:00 to Saturday
// 2:00.
type Hours struct {
	From, To int
}

// Valid reports whether s's fields go together: dates at midnight UTC, the
// start not after the end, days of the week in order without repeats, and
// hours that are a span.
func (s Schedule) Valid() bool {
	date := func(t time.Time) bool {
		year, month, day := t.UTC().Date()
		return t.IsZero() || t.Equal(time.Date(year, month, day, 0, 0, 0, 0, time.UTC))
	}
	if !date(s.Start) || !date(s.End) || !s.Start.IsZero() && !s.End.IsZero() && s.Start.After(s.End) {
		return false
	}
	for i, day := range s.Days {
		if day < time.Sunday || day > time.Saturday || i > 0 && day <= s.Days[i-1] {
			return false
		}
	}
	return s.Hours == nil || s.Hours.From >= 0 && s.Hours.From <= 23 && s.Hours.To >= 1 && s.Hours.To <= 24 && s.Hours.From != s.Hours.To
}

// ShowsAt reports whether the row shows at t, by its schedule, in the
// server's time zone. A row that is also a library follows its dates only,
// so that the libraries of other apps do not come and go by the hour.
func (r Row) ShowsAt(t time.Time) bool {
	s := r.Schedule
	day := t.In(time.Local)
	if h := s.Hours; h != nil && !r.Library {
		switch hour := day.Hour(); {
		case h.From < h.To && (hour < h.From || hour >= h.To), h.From > h.To && hour < h.From && hour >= h.To:
			return false
		case h.From > h.To && hour < h.To:
			// After midnight, in the night of the day before.
			day = day.AddDate(0, 0, -1)
		}
	}
	date := accounts.Day(day)
	if !s.Start.IsZero() && date.Before(s.Start) || !s.End.IsZero() && date.After(s.End) {
		return false
	}
	return r.Library || len(s.Days) == 0 || slices.Contains(s.Days, day.Weekday())
}

// Valid reports whether r's fields go together: a known kind and place, a
// name, a catalog for a catalog row and a collection for a collection row,
// and nothing of either for the other kinds, and a valid schedule.
func (r Row) Valid() bool {
	name := []rune(r.Name)
	catalog := r.Addon != (accounts.ID{}) && r.CatalogType != "" && r.CatalogID != ""
	return slices.Contains(Kinds, r.Kind) && slices.Contains(Places, r.Place) && len(name) > 0 && len(name) <= MaxNameLength &&
		r.Count >= 1 && r.Count <= MaxCount &&
		catalog == (r.Kind == KindCatalog) && (r.Kind == KindCatalog || r.Addon == (accounts.ID{}) && r.CatalogType == "" && r.CatalogID == "") &&
		(r.Collection != (accounts.ID{})) == (r.Kind == KindCollection) && r.Schedule.Valid()
}

// ShownTo reports whether the row is shown to user, before they turn it
// off.
func (r Row) ShownTo(user accounts.ID) bool {
	return r.Everyone || slices.Contains(r.Users, user)
}

// Store keeps the rows in PostgreSQL.
type Store struct {
	db *pgxpool.Pool
}

// New returns a store backed by db.
func New(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

// Rows lists the rows in order.
func (s *Store) Rows(ctx context.Context) ([]Row, error) {
	return s.load(ctx, "")
}

// Row returns a row.
func (s *Store) Row(ctx context.Context, id accounts.ID) (Row, error) {
	rows, err := s.load(ctx, "WHERE id = $1", id)
	if err != nil {
		return Row{}, err
	}
	if len(rows) == 0 {
		return Row{}, ErrNotFound
	}
	return rows[0], nil
}

func (s *Store) load(ctx context.Context, where string, args ...any) ([]Row, error) {
	rows, _ := s.db.Query(ctx, `SELECT id, name, kind, place, addon_id, catalog_type, catalog_id, collection_id, count, everyone, library,
		start_date, end_date, days, start_hour, end_hour
		FROM home_rows `+where+` ORDER BY position, created_at, id`, args...)
	var result []Row
	var r Row
	var addon, collection *accounts.ID
	var start, end *time.Time
	var days []int16
	var from, to *int16
	if _, err := pgx.ForEachRow(rows, []any{&r.ID, &r.Name, &r.Kind, &r.Place, &addon, &r.CatalogType, &r.CatalogID, &collection, &r.Count,
		&r.Everyone, &r.Library, &start, &end, &days, &from, &to}, func() error {
		row := r
		if addon != nil {
			row.Addon = *addon
		}
		if collection != nil {
			row.Collection = *collection
		}
		if start != nil {
			row.Schedule.Start = *start
		}
		if end != nil {
			row.Schedule.End = *end
		}
		for _, day := range days {
			row.Schedule.Days = append(row.Schedule.Days, time.Weekday(day))
		}
		if from != nil && to != nil {
			row.Schedule.Hours = &Hours{From: int(*from), To: int(*to)}
		}
		result = append(result, row)
		return nil
	}); err != nil || len(result) == 0 {
		return result, err
	}
	index := make(map[accounts.ID]int, len(result))
	ids := make([]accounts.ID, len(result))
	for i, row := range result {
		index[row.ID], ids[i] = i, row.ID
	}
	var row, user accounts.ID
	users, _ := s.db.Query(ctx, "SELECT row_id, user_id FROM home_row_users WHERE row_id = ANY($1) ORDER BY user_id", ids)
	if _, err := pgx.ForEachRow(users, []any{&row, &user}, func() error {
		result[index[row]].Users = append(result[index[row]].Users, user)
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

// Create adds a row after the others.
func (s *Store) Create(ctx context.Context, r Row) (Row, error) {
	if !r.Valid() {
		return Row{}, ErrInvalid
	}
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		// Rows added at once count one after the other.
		if _, err := tx.Exec(ctx, "LOCK TABLE home_rows IN SHARE ROW EXCLUSIVE MODE"); err != nil {
			return err
		}
		var count, last int
		if err := tx.QueryRow(ctx, "SELECT count(*), coalesce(max(position), 0) FROM home_rows").Scan(&count, &last); err != nil {
			return err
		}
		if count >= MaxRows {
			return ErrTooMany
		}
		start, end, days, from, to := scheduleColumns(r.Schedule)
		if err := tx.QueryRow(ctx, `INSERT INTO home_rows (position, name, kind, place, addon_id, catalog_type, catalog_id, collection_id, count,
			everyone, library, start_date, end_date, days, start_hour, end_hour)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16) RETURNING id`,
			last+1, r.Name, r.Kind, r.Place, nullable(r.Addon), r.CatalogType, r.CatalogID, nullable(r.Collection), r.Count, r.Everyone,
			r.Library, start, end, days, from, to).Scan(&r.ID); err != nil {
			return err
		}
		return setUsers(ctx, tx, r)
	})
	if err != nil {
		return Row{}, err
	}
	return s.Row(ctx, r.ID)
}

// Update changes a row, its place in the list kept.
func (s *Store) Update(ctx context.Context, r Row) (Row, error) {
	if !r.Valid() {
		return Row{}, ErrInvalid
	}
	start, end, days, from, to := scheduleColumns(r.Schedule)
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE home_rows SET name = $2, kind = $3, place = $4, addon_id = $5, catalog_type = $6, catalog_id = $7,
			collection_id = $8, count = $9, everyone = $10, library = $11, start_date = $12, end_date = $13, days = $14, start_hour = $15,
			end_hour = $16 WHERE id = $1`,
			r.ID, r.Name, r.Kind, r.Place, nullable(r.Addon), r.CatalogType, r.CatalogID, nullable(r.Collection), r.Count, r.Everyone, r.Library,
			start, end, days, from, to)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if _, err := tx.Exec(ctx, "DELETE FROM home_row_users WHERE row_id = $1", r.ID); err != nil {
			return err
		}
		return setUsers(ctx, tx, r)
	})
	if err != nil {
		return Row{}, err
	}
	return s.Row(ctx, r.ID)
}

// setUsers keeps the users a row not shown to everyone is shown to; those
// that no longer exist are left out.
func setUsers(ctx context.Context, tx pgx.Tx, r Row) error {
	if r.Everyone || len(r.Users) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `INSERT INTO home_row_users (row_id, user_id)
		SELECT $1, id FROM users WHERE id = ANY($2) ORDER BY id ON CONFLICT DO NOTHING`, r.ID, r.Users)
	return err
}

// Delete deletes a row.
func (s *Store) Delete(ctx context.Context, id accounts.ID) error {
	tag, err := s.db.Exec(ctx, "DELETE FROM home_rows WHERE id = $1", id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// Order puts the rows in the order of ids; rows ids leaves out keep their
// order after them.
func (s *Store) Order(ctx context.Context, ids []accounts.ID) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE home_rows h SET position = o.position
			FROM (SELECT id, row_number() OVER (ORDER BY coalesce(array_position($1::uuid[], id), $2 + position), created_at, id) AS position
				FROM home_rows) o
			WHERE h.id = o.id`, ids, len(ids))
		return err
	})
}

// Off returns the rows user turned off.
func (s *Store) Off(ctx context.Context, user accounts.ID) (map[accounts.ID]bool, error) {
	rows, _ := s.db.Query(ctx, "SELECT row_id FROM home_rows_off WHERE user_id = $1", user)
	off := map[accounts.ID]bool{}
	var id accounts.ID
	_, err := pgx.ForEachRow(rows, []any{&id}, func() error {
		off[id] = true
		return nil
	})
	return off, err
}

// SetOff turns a row off for user, or on again. A row not shown to them
// is ErrNotFound.
func (s *Store) SetOff(ctx context.Context, user, id accounts.ID, off bool) error {
	r, err := s.Row(ctx, id)
	if err != nil {
		return err
	}
	if !r.ShownTo(user) {
		return ErrNotFound
	}
	if off {
		_, err = s.db.Exec(ctx, "INSERT INTO home_rows_off (user_id, row_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", user, id)
	} else {
		_, err = s.db.Exec(ctx, "DELETE FROM home_rows_off WHERE user_id = $1 AND row_id = $2", user, id)
	}
	return err
}

// TurnAllOn turns every row user turned off on again.
func (s *Store) TurnAllOn(ctx context.Context, user accounts.ID) error {
	_, err := s.db.Exec(ctx, "DELETE FROM home_rows_off WHERE user_id = $1", user)
	return err
}

// Shown lists, in order, the rows shown to user at now that they did not
// turn off.
func (s *Store) Shown(ctx context.Context, user accounts.ID, now time.Time) ([]Row, error) {
	rows, err := s.Rows(ctx)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	off, err := s.Off(ctx, user)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(rows, func(r Row) bool { return !r.ShownTo(user) || off[r.ID] || !r.ShowsAt(now) }), nil
}

// nullable is nil for the zero identifier.
func nullable(id accounts.ID) *accounts.ID {
	if id == (accounts.ID{}) {
		return nil
	}
	return &id
}

// scheduleColumns are the columns of s: its dates, null for none, its
// days, and its hours, null for all day.
func scheduleColumns(s Schedule) (start, end *time.Time, days []int16, from, to *int16) {
	if !s.Start.IsZero() {
		start = &s.Start
	}
	if !s.End.IsZero() {
		end = &s.End
	}
	days = make([]int16, 0, len(s.Days))
	for _, day := range s.Days {
		days = append(days, int16(day))
	}
	if s.Hours != nil {
		from, to = new(int16(s.Hours.From)), new(int16(s.Hours.To))
	}
	return start, end, days, from, to
}
