package accounts

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// ErrInvalidApp reports an API key app name that is empty, longer than
// maxNameLength or unprintable.
var ErrInvalidApp = errors.New("invalid API key app name")

// APIKey is a key tools and apps use the Jellyfin API with: administrator
// rights, and no user. Only a hash of the key is kept; the key itself is
// returned once, when it is created.
type APIKey struct {
	ID        ID
	App       string
	CreatedAt time.Time
	// LastUsedAt is when the key last authenticated a request, to the
	// minute; nil when it never did.
	LastUsedAt *time.Time
}

const apiKeyColumns = "id, app, created_at, last_used_at"

func scanAPIKey(row pgx.Row) (APIKey, error) {
	var key APIKey
	err := row.Scan(&key.ID, &key.App, &key.CreatedAt, &key.LastUsedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return key, ErrNotFound
	}
	return key, err
}

// CreateAPIKey issues a key for app and returns it with its record.
func (s *Store) CreateAPIKey(ctx context.Context, app string) (APIKey, string, error) {
	app = strings.TrimSpace(app)
	if app == "" || utf8.RuneCountInString(app) > maxNameLength || strings.IndexFunc(app, func(r rune) bool { return !unicode.IsPrint(r) }) >= 0 {
		return APIKey{}, "", ErrInvalidApp
	}
	token, hash := newToken()
	key, err := scanAPIKey(s.db.QueryRow(ctx,
		"INSERT INTO api_keys (app, token_hash) VALUES ($1, $2) RETURNING "+apiKeyColumns, app, hash))
	if err != nil {
		return APIKey{}, "", err
	}
	return key, token, nil
}

// APIKeys lists the keys, newest first.
func (s *Store) APIKeys(ctx context.Context) ([]APIKey, error) {
	rows, err := s.db.Query(ctx, "SELECT "+apiKeyColumns+" FROM api_keys ORDER BY created_at DESC, id")
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (APIKey, error) { return scanAPIKey(row) })
}

// APIKeyByToken resolves a key and records its use, at most once a minute.
// A key resolved less than signInLife ago is not looked up again.
func (s *Store) APIKeyByToken(ctx context.Context, token string) (APIKey, error) {
	hash := hashToken(token)
	now := s.now()
	key, ok := s.signIns.key(string(hash), now)
	if !ok {
		generation := s.signIns.current()
		var err error
		if key, err = scanAPIKey(s.db.QueryRow(ctx, "SELECT "+apiKeyColumns+" FROM api_keys WHERE token_hash = $1", hash)); err != nil {
			return APIKey{}, err
		}
		s.signIns.keepKey(string(hash), key, now, generation)
	}
	if key.LastUsedAt == nil || now.Sub(*key.LastUsedAt) >= activityResolution {
		if _, err := s.db.Exec(ctx, "UPDATE api_keys SET last_used_at = $2 WHERE id = $1", key.ID, now); err != nil {
			return APIKey{}, err
		}
		key.LastUsedAt = &now
		s.signIns.used(string(hash), now)
	}
	return key, nil
}

// RevokeAPIKey deletes a key by its identifier.
func (s *Store) RevokeAPIKey(ctx context.Context, id ID) error {
	tag, err := s.db.Exec(ctx, "DELETE FROM api_keys WHERE id = $1", id)
	s.forgetSignIns()
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// RevokeAPIKeyByToken deletes the key token is, if any.
func (s *Store) RevokeAPIKeyByToken(ctx context.Context, token string) error {
	_, err := s.db.Exec(ctx, "DELETE FROM api_keys WHERE token_hash = $1", hashToken(token))
	s.forgetSignIns()
	return err
}
