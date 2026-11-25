package accounts

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
)

// The values of User.Moonfin: the server's Moonfin, or the user set
// apart, served Moonfin's companion or not.
const (
	MoonfinServer = ""
	MoonfinOn     = "on"
	MoonfinOff    = "off"
)

// MoonfinChoices are the values of User.Moonfin, the server's first.
var MoonfinChoices = []string{MoonfinServer, MoonfinOn, MoonfinOff}

var (
	// ErrInvalidMoonfin reports a User.Moonfin not in MoonfinChoices.
	ErrInvalidMoonfin = errors.New("invalid Moonfin choice")
	// ErrMoonfinLocked reports a user changing their own Moonfin while
	// the administrator keeps them from it (User.MoonfinLocked).
	ErrMoonfinLocked = errors.New("locked Moonfin choice")
)

// SetOwnMoonfin changes the Moonfin of the user id as the user themselves
// does: choice is one of MoonfinChoices. It is refused with
// ErrMoonfinLocked while the administrator keeps them from it. As
// UpdateUser, it applies from the user's next request.
func (s *Store) SetOwnMoonfin(ctx context.Context, id ID, choice string) (User, error) {
	if !slices.Contains(MoonfinChoices, choice) {
		return User{}, ErrInvalidMoonfin
	}
	var updated User
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		current, err := scanUser(tx.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE id = $1 FOR UPDATE", id))
		if err != nil {
			return err
		}
		if current.MoonfinLocked {
			return ErrMoonfinLocked
		}
		updated, err = scanUser(tx.QueryRow(ctx, "UPDATE users SET moonfin = $2 WHERE id = $1 RETURNING "+userColumns, id, choice))
		return err
	})
	if err != nil {
		return User{}, err
	}
	s.forgetSignIns()
	return updated, nil
}
