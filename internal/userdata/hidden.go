package userdata

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// HiddenTitle is a movie or series a user hid from their lists, and when
// they hid it. A hidden title leaves the user's listings, but still opens
// by its identifier (see package library).
type HiddenTitle struct {
	ID accounts.ID
	At time.Time
}

// Hide hides a title from the user's lists at at. A title already hidden
// keeps the time it was first hidden.
func (s *Store) Hide(ctx context.Context, user, title accounts.ID, at time.Time) error {
	_, err := s.db.Exec(ctx, `INSERT INTO hidden_titles (user_id, item_id, hidden_at) VALUES ($1, $2, $3)
		ON CONFLICT (user_id, item_id) DO NOTHING`, user, title, at)
	return err
}

// Show shows a hidden title in the user's lists again. It reports whether
// the title was hidden.
func (s *Store) Show(ctx context.Context, user, title accounts.ID) (bool, error) {
	tag, err := s.db.Exec(ctx, "DELETE FROM hidden_titles WHERE user_id = $1 AND item_id = $2", user, title)
	return tag.RowsAffected() > 0, err
}

// Hidden lists the titles the user hid, the latest hidden first.
func (s *Store) Hidden(ctx context.Context, user accounts.ID) ([]HiddenTitle, error) {
	rows, err := s.db.Query(ctx, `SELECT item_id, hidden_at FROM hidden_titles WHERE user_id = $1
		ORDER BY hidden_at DESC, item_id`, user)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (HiddenTitle, error) {
		var title HiddenTitle
		err := row.Scan(&title.ID, &title.At)
		title.At = title.At.UTC()
		return title, err
	})
}
