package accounts

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// PasswordResetLifetime is how long a forgotten password PIN stays valid.
const PasswordResetLifetime = 30 * time.Minute

// PasswordResetPIN is a PIN a user asked for from a Jellyfin app's
// forgotten password screen. Redeeming it before ExpiresAt makes it the
// user's password.
type PasswordResetPIN struct {
	UserID    ID
	PIN       string
	ExpiresAt time.Time
}

// RequestPasswordReset gives the user named name, matched without regard
// to case, a new PIN valid PasswordResetLifetime, which replaces the one
// they had. ErrNotFound reports that no user has that name.
func (s *Store) RequestPasswordReset(ctx context.Context, name string) (User, PasswordResetPIN, error) {
	user, err := scanUser(s.db.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE lower(name) = lower($1)", strings.TrimSpace(name)))
	if err != nil {
		return User{}, PasswordResetPIN{}, err
	}
	// Four random bytes written as Jellyfin writes them: 6E-40-83-7F.
	var raw [4]byte
	_, _ = rand.Read(raw[:])
	pin := PasswordResetPIN{UserID: user.ID, PIN: fmt.Sprintf("%02X-%02X-%02X-%02X", raw[0], raw[1], raw[2], raw[3]),
		ExpiresAt: s.now().Add(PasswordResetLifetime)}
	_, err = s.db.Exec(ctx, `INSERT INTO password_reset_pins (user_id, pin, expires_at) VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET pin = excluded.pin, expires_at = excluded.expires_at`,
		pin.UserID, pin.PIN, pin.ExpiresAt)
	if err != nil {
		return User{}, PasswordResetPIN{}, err
	}
	return user, pin, nil
}

// PasswordResetPINs returns the PINs still valid, by user.
func (s *Store) PasswordResetPINs(ctx context.Context) (map[ID]PasswordResetPIN, error) {
	rows, err := s.db.Query(ctx, "SELECT user_id, pin, expires_at FROM password_reset_pins WHERE expires_at > $1", s.now())
	if err != nil {
		return nil, err
	}
	pins, err := pgx.CollectRows(rows, pgx.RowToStructByPos[PasswordResetPIN])
	if err != nil {
		return nil, err
	}
	byUser := make(map[ID]PasswordResetPIN, len(pins))
	for _, pin := range pins {
		byUser[pin.UserID] = pin
	}
	return byUser, nil
}

// RedeemPasswordResetPIN resets the password of every user whose valid PIN
// is entered, compared as Jellyfin does, letter case included and dashes
// left out: their password becomes entered, as typed, which signs them
// out everywhere, and their PIN is used up. Expired PINs are deleted on
// the way. ErrNotFound reports that no valid PIN matches.
func (s *Store) RedeemPasswordResetPIN(ctx context.Context, entered string) ([]User, error) {
	if _, err := s.db.Exec(ctx, "DELETE FROM password_reset_pins WHERE expires_at <= $1", s.now()); err != nil {
		return nil, err
	}
	stripped := strings.ReplaceAll(entered, "-", "")
	if stripped == "" {
		return nil, ErrNotFound
	}
	rows, err := s.db.Query(ctx, "DELETE FROM password_reset_pins WHERE replace(pin, '-', '') = $1 AND expires_at > $2 RETURNING user_id",
		stripped, s.now())
	if err != nil {
		return nil, err
	}
	matched, err := pgx.CollectRows(rows, pgx.RowTo[ID])
	if err != nil {
		return nil, err
	}
	if len(matched) == 0 {
		return nil, ErrNotFound
	}
	reset := make([]User, 0, len(matched))
	for _, id := range matched {
		user, err := s.updateUser(ctx, id, UserChanges{Password: &entered}, signIn{})
		if err != nil {
			return nil, err
		}
		reset = append(reset, user)
	}
	return reset, nil
}
