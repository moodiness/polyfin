// Package preferences stores the display preferences Jellyfin apps save for
// each user, preference id and client app.
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
