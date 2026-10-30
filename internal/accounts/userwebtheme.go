package accounts

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
)

// ErrWebThemeLocked reports a user changing their own web player theme
// while the administrator keeps them from it (User.WebThemeLocked).
var ErrWebThemeLocked = errors.New("web player theme locked")

// SetOwnWebTheme changes the web player theme of the user id as the user
// themselves does: theme is one of WebThemes, or empty for the server's.
// It is refused with ErrWebThemeLocked while the administrator keeps them
// from it. Whether the Custom theme is offered is the caller's to check.
// As UpdateUser, it applies from the user's next request.
func (s *Store) SetOwnWebTheme(ctx context.Context, id ID, theme string) (User, error) {
	if theme != "" && !slices.Contains(WebThemes, theme) {
		return User{}, ErrInvalidWebTheme
	}
	var updated User
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		current, err := scanUser(tx.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE id = $1 FOR UPDATE", id))
		if err != nil {
			return err
		}
		if current.WebThemeLocked {
			return ErrWebThemeLocked
		}
		updated, err = scanUser(tx.QueryRow(ctx, "UPDATE users SET web_theme = $2 WHERE id = $1 RETURNING "+userColumns, id, theme))
		return err
	})
	if err != nil {
		return User{}, err
	}
	s.forgetSignIns()
	return updated, nil
}
