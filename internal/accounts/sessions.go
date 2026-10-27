package accounts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// AdminSessionLifetime is how long the admin interface stays signed in
// without activity.
const AdminSessionLifetime = 30 * 24 * time.Hour

// renewAfter is how old a session must be before activity extends it, so
// that every request does not rewrite the session row and the cookie.
const renewAfter = 24 * time.Hour

// AdminSession is a signed-in browser of the admin interface.
type AdminSession struct {
	User User
	// TokenHash identifies the session without exposing its token.
	TokenHash []byte
	ExpiresAt time.Time
	// Renewed is true when this request extended the session: the cookie
	// must be sent again with the new expiry.
	Renewed bool
}

// HashAdminToken returns the stored form of an admin session token.
func HashAdminToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// CreateAdminSession signs a user in to the admin interface, which counts as
// a sign-in of the account. The session lasts until it is signed out or
// expires.
func (s *Store) CreateAdminSession(ctx context.Context, user ID) (string, time.Time, error) {
	return s.createAdminSession(ctx, user, nil)
}

// CreateDeviceAdminSession signs a device's user in to the admin interface,
// for as long as the device stays signed in: the session ends with the
// device's sign-out, and when the device signs in again, as the same user or
// another. So the admin app, which takes jellyfin-web's sign-in in the same
// browser, never keeps a session the web client no longer has, as Jellyfin's
// dashboard, part of jellyfin-web, shares its sign-in. ErrNotFound when the
// device signed out meanwhile.
func (s *Store) CreateDeviceAdminSession(ctx context.Context, device Device) (string, time.Time, error) {
	token, expires, err := s.createAdminSession(ctx, device.UserID, &device.ID)
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23503" {
		return "", time.Time{}, ErrNotFound
	}
	return token, expires, err
}

func (s *Store) createAdminSession(ctx context.Context, user ID, device *ID) (string, time.Time, error) {
	// The user's last sign-in changes.
	defer s.forgetSignIns()
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	token := base64.RawURLEncoding.EncodeToString(raw)
	expires := time.Now().Add(AdminSessionLifetime)
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "DELETE FROM admin_sessions WHERE expires_at < now()"); err != nil {
			return err
		}
		// A device signed out meanwhile fails its reference.
		if _, err := tx.Exec(ctx, "INSERT INTO admin_sessions (token_hash, user_id, expires_at, device) VALUES ($1, $2, $3, $4)",
			HashAdminToken(token), user, expires, device); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "UPDATE users SET last_login_at = now(), last_activity_at = now() WHERE id = $1", user)
		return err
	})
	return token, expires, err
}

// AdminSession resolves a session token of an enabled user whose account
// has not ended, and extends the session when it is more than a day old.
func (s *Store) AdminSession(ctx context.Context, token string) (AdminSession, error) {
	session := AdminSession{TokenHash: HashAdminToken(token)}
	user := &session.User
	err := s.db.QueryRow(ctx, "SELECT s.expires_at, "+qualifiedUserColumns("u")+`
		FROM admin_sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now() AND NOT u.is_disabled AND (u.ends_on IS NULL OR u.ends_on > $2)`,
		session.TokenHash, Day(s.now())).Scan(
		append([]any{&session.ExpiresAt}, user.fields()...)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminSession{}, ErrNotFound
	}
	if err != nil {
		return AdminSession{}, err
	}
	if time.Until(session.ExpiresAt) < AdminSessionLifetime-renewAfter {
		session.ExpiresAt = time.Now().Add(AdminSessionLifetime)
		if _, err := s.db.Exec(ctx, "UPDATE admin_sessions SET expires_at = $2 WHERE token_hash = $1",
			session.TokenHash, session.ExpiresAt); err != nil {
			return AdminSession{}, err
		}
		session.Renewed = true
	}
	return session, nil
}

// DeleteAdminSession signs a browser out of the admin interface.
func (s *Store) DeleteAdminSession(ctx context.Context, tokenHash []byte) error {
	_, err := s.db.Exec(ctx, "DELETE FROM admin_sessions WHERE token_hash = $1", tokenHash)
	return err
}
