// Package preferences stores the display preferences Jellyfin apps save for
// each user, preference id and client app, and the configuration each user
// saves.
package preferences

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Store keeps display preferences in PostgreSQL. Values are JSON objects
// whose content belongs to the caller.
type Store struct {
	db *pgxpool.Pool
}

// New returns a store backed by db.
func New(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

// Get returns the preferences user saved under id for client, and whether
// any were saved. Ids and clients match exactly, letter case included.
func (s *Store) Get(ctx context.Context, user accounts.ID, id, client string) (json.RawMessage, bool, error) {
	var value json.RawMessage
	err := s.db.QueryRow(ctx,
		"SELECT value FROM display_preferences WHERE user_id = $1 AND preference_id = $2 AND client = $3",
		user, id, client).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

// Put replaces the preferences user saved under id for client. value must
// be a JSON object.
func (s *Store) Put(ctx context.Context, user accounts.ID, id, client string, value json.RawMessage) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO display_preferences (user_id, preference_id, client, value) VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, preference_id, client) DO UPDATE SET value = excluded.value`,
		user, id, client, string(value))
	return err
}

// Configuration returns the configuration user saved, and whether one was
// saved.
func (s *Store) Configuration(ctx context.Context, user accounts.ID) (json.RawMessage, bool, error) {
	var value json.RawMessage
	err := s.db.QueryRow(ctx, "SELECT value FROM user_configurations WHERE user_id = $1", user).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

// PutConfiguration replaces the configuration user saved. value must be a
// JSON object.
func (s *Store) PutConfiguration(ctx context.Context, user accounts.ID, value json.RawMessage) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO user_configurations (user_id, value) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET value = excluded.value`,
		user, string(value))
	return err
}

// Reset forgets the display preferences and the configuration user saved,
// from every app: they have a new user's again.
func (s *Store) Reset(ctx context.Context, user accounts.ID) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "DELETE FROM display_preferences WHERE user_id = $1", user); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "DELETE FROM user_configurations WHERE user_id = $1", user)
		return err
	})
}

// ownOrder selects the configurations that keep an order of libraries of
// their own: an OrderedViews that is not empty.
const ownOrder = `jsonb_typeof(value->'OrderedViews') = 'array' AND value->'OrderedViews' <> '[]'::jsonb`

// LibraryOrders counts the users whose configuration keeps an order of
// libraries of their own.
func (s *Store) LibraryOrders(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, "SELECT count(*) FROM user_configurations WHERE "+ownOrder).Scan(&count)
	return count, err
}

// ClearLibraryOrders empties the order of libraries every user's
// configuration keeps, leaving the rest of it, and returns the users whose
// order it cleared. Rows are locked in the order of their keys, as every
// statement writing several rows takes them.
func (s *Store) ClearLibraryOrders(ctx context.Context) ([]accounts.ID, error) {
	rows, err := s.db.Query(ctx, `
		WITH ordered AS (
			SELECT user_id FROM user_configurations WHERE `+ownOrder+` ORDER BY user_id FOR UPDATE
		)
		UPDATE user_configurations c SET value = jsonb_set(c.value, '{OrderedViews}', '[]'::jsonb)
		FROM ordered WHERE c.user_id = ordered.user_id
		RETURNING c.user_id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
}
