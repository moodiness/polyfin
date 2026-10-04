package accounts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
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
// a sign-in of the account.
func (s *Store) CreateAdminSession(ctx context.Context, user ID) (string, time.Time, error) {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	token := base64.RawURLEncoding.EncodeToString(raw)
	expires := time.Now().Add(AdminSessionLifetime)
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "DELETE FROM admin_sessions WHERE expires_at < now()"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO admin_sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)",
			HashAdminToken(token), user, expires); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "UPDATE users SET last_login_at = now(), last_activity_at = now() WHERE id = $1", user)
		return err
	})
	return token, expires, err
}

// AdminSession resolves a session token of an enabled user and extends the
// session when it is more than a day old.
func (s *Store) AdminSession(ctx context.Context, token string) (AdminSession, error) {
	session := AdminSession{TokenHash: HashAdminToken(token)}
	user := &session.User
	err := s.db.QueryRow(ctx, "SELECT s.expires_at, "+qualifiedUserColumns("u")+`
		FROM admin_sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now() AND NOT u.is_disabled`, session.TokenHash).Scan(
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
