package accounts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// The bounds of an invite: how many accounts it may create, and how many
// days it may last before it expires.
const (
	MaxInviteUses = 100
	MaxInviteDays = 365
)

// The states of an invite.
const (
	InviteActive  = "active"
	InviteUsedUp  = "used_up"
	InviteExpired = "expired"
	InviteRevoked = "revoked"
)

var (
	ErrInvalidInviteUses   = errors.New("invalid invite uses")
	ErrInvalidInviteExpiry = errors.New("invalid invite expiry")
	ErrInvalidInviteModel  = errors.New("invalid invite model user")
	// ErrInviteUnknown reports a token that is no invite's; the other
	// errors, an invite that may no longer create accounts.
	ErrInviteUnknown = errors.New("unknown invite")
	ErrInviteUsedUp  = errors.New("invite used up")
	ErrInviteExpired = errors.New("invite expired")
	ErrInviteRevoked = errors.New("invite revoked")
)

// Invite is an invite link, which lets guests create a member account for
// themselves: at most MaxUses of them, until ExpiresAt (never when nil),
// unless it is revoked.
type Invite struct {
	ID        ID
	MaxUses   int
	Uses      int
	ExpiresAt *time.Time
	CreatedAt time.Time
	RevokedAt *time.Time
	// Model is the user whose settings the new accounts copy, nil for the
	// defaults of a new user; CreatedBy, the administrator who created the
	// invite, nil once deleted.
	Model     *InviteUser
	CreatedBy *InviteUser
}

// InviteUser names a user an invite refers to.
type InviteUser struct {
	ID   ID
	Name string
}

// State tells whether the invite may still create accounts at now: active,
// or else revoked, used up or expired, in that order.
func (i Invite) State(now time.Time) string {
	switch {
	case i.RevokedAt != nil:
		return InviteRevoked
	case i.Uses >= i.MaxUses:
		return InviteUsedUp
	case i.ExpiresAt != nil && !now.Before(*i.ExpiresAt):
		return InviteExpired
	}
	return InviteActive
}

// stateError is the error of an invite that may no longer create
// accounts, nil for an active one.
func (i Invite) stateError(now time.Time) error {
	switch i.State(now) {
	case InviteRevoked:
		return ErrInviteRevoked
	case InviteUsedUp:
		return ErrInviteUsedUp
	case InviteExpired:
		return ErrInviteExpired
	}
	return nil
}

// NewInvite describes an invite to create: how many accounts it may create,
// from 1 to MaxInviteUses; how many days it lasts, from 1 to MaxInviteDays,
// 0 for no expiry; the user whose settings the accounts copy, if any; and
// the administrator creating it.
type NewInvite struct {
	MaxUses   int
	Days      int
	Model     *ID
	CreatedBy ID
}

const (
	// inviteSecretSize is the size of the random part of an invite's
	// token, as large as an admin session token's.
	inviteSecretSize = 32
	// modelColumns are the settings a new account copies from an invite's
	// model: everything an administrator sets on a user's page but the
	// name, the password, administrator status and the disabled switch.
	modelColumns = "is_hidden, max_parental_rating, max_parental_sub_rating, block_unrated_items, " +
		"video_transcoding, audio_transcoding, content_downloading, personal_addons, " +
		"max_playbacks, max_bitrate, live_tv, sync_play, remote_control, " +
		"hidden_libraries, blocked_genres, access_schedules, collection_management, " +
		"subtitle_management, live_tv_management, quality_group"
	inviteColumns = "i.id, i.max_uses, i.uses, i.expires_at, i.created_at, i.revoked_at, " +
		"m.id, m.name, c.id, c.name"
	inviteJoins = " FROM invites i LEFT JOIN users m ON m.id = i.model_user_id LEFT JOIN users c ON c.id = i.created_by"
)

func scanInvite(row pgx.Row, more ...any) (Invite, error) {
	var invite Invite
	var modelID, creatorID *ID
	var modelName, creatorName *string
	err := row.Scan(append([]any{&invite.ID, &invite.MaxUses, &invite.Uses, &invite.ExpiresAt, &invite.CreatedAt, &invite.RevokedAt,
		&modelID, &modelName, &creatorID, &creatorName}, more...)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return invite, ErrNotFound
	}
	if modelID != nil {
		invite.Model = &InviteUser{ID: *modelID, Name: *modelName}
	}
	if creatorID != nil {
		invite.CreatedBy = &InviteUser{ID: *creatorID, Name: *creatorName}
	}
	return invite, err
}

// inviteToken is the token of an invite's link: its ID then its secret,
// in base64 for URLs.
func inviteToken(id ID, secret []byte) string {
	return base64.RawURLEncoding.EncodeToString(append(id[:], secret...))
}

// parseInviteToken splits a token into the invite's ID and its secret.
func parseInviteToken(token string) (ID, []byte, bool) {
	var id ID
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != len(id)+inviteSecretSize {
		return id, nil, false
	}
	copy(id[:], raw)
	return id, raw[len(id):], true
}

// CreateInvite creates an invite, and returns it with its token, which is
// given this once: only its hash is kept.
func (s *Store) CreateInvite(ctx context.Context, invite NewInvite) (Invite, string, error) {
	if invite.MaxUses < 1 || invite.MaxUses > MaxInviteUses {
		return Invite{}, "", ErrInvalidInviteUses
	}
	if invite.Days < 0 || invite.Days > MaxInviteDays {
		return Invite{}, "", ErrInvalidInviteExpiry
	}
	var expires *time.Time
	if invite.Days > 0 {
		expires = new(s.now().Add(time.Duration(invite.Days) * 24 * time.Hour))
	}
	secret := make([]byte, inviteSecretSize)
	_, _ = rand.Read(secret)
	hash := sha256.Sum256(secret)
	var created Invite
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if invite.Model != nil {
			// The model stays until the invite is saved.
			if _, err := scanUser(tx.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE id = $1 FOR SHARE", *invite.Model)); errors.Is(err, ErrNotFound) {
				return ErrInvalidInviteModel
			} else if err != nil {
				return err
			}
		}
		var id ID
		if err := tx.QueryRow(ctx, `INSERT INTO invites (secret_hash, max_uses, expires_at, model_user_id, created_by)
			VALUES ($1, $2, $3, $4, $5) RETURNING id`, hash[:], invite.MaxUses, expires, invite.Model, invite.CreatedBy).Scan(&id); err != nil {
			return err
		}
		var err error
		created, err = scanInvite(tx.QueryRow(ctx, "SELECT "+inviteColumns+inviteJoins+" WHERE i.id = $1", id))
		return err
	})
	if err != nil {
		return Invite{}, "", err
	}
	return created, inviteToken(created.ID, secret), nil
}

// Invites lists every invite, the newest first.
func (s *Store) Invites(ctx context.Context) ([]Invite, error) {
	rows, err := s.db.Query(ctx, "SELECT "+inviteColumns+inviteJoins+" ORDER BY i.created_at DESC, i.id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	invites := []Invite{}
	for rows.Next() {
		invite, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		invites = append(invites, invite)
	}
	return invites, rows.Err()
}

// RevokeInvite stops an invite from creating any more accounts. Revoking
// it again changes nothing.
func (s *Store) RevokeInvite(ctx context.Context, id ID) (Invite, error) {
	tag, err := s.db.Exec(ctx, "UPDATE invites SET revoked_at = coalesce(revoked_at, now()) WHERE id = $1", id)
	if err != nil {
		return Invite{}, err
	}
	if tag.RowsAffected() == 0 {
		return Invite{}, ErrNotFound
	}
	return scanInvite(s.db.QueryRow(ctx, "SELECT "+inviteColumns+inviteJoins+" WHERE i.id = $1", id))
}

// lookUpInvite finds the invite of token, its secret compared in constant
// time; ErrInviteUnknown reports a token that is no invite's. forUpdate
// locks the invite until tx ends.
func lookUpInvite(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, token string, forUpdate bool) (Invite, error) {
	id, secret, ok := parseInviteToken(token)
	if !ok {
		return Invite{}, ErrInviteUnknown
	}
	query := "SELECT " + inviteColumns + ", i.secret_hash" + inviteJoins + " WHERE i.id = $1"
	if forUpdate {
		query += " FOR UPDATE OF i"
	}
	var stored []byte
	invite, err := scanInvite(db.QueryRow(ctx, query, id), &stored)
	if errors.Is(err, ErrNotFound) {
		return Invite{}, ErrInviteUnknown
	}
	if err != nil {
		return Invite{}, err
	}
	hash := sha256.Sum256(secret)
	if subtle.ConstantTimeCompare(hash[:], stored) != 1 {
		return Invite{}, ErrInviteUnknown
	}
	return invite, nil
}

// InviteByToken returns the invite of token while it may create accounts.
// ErrInviteUnknown reports a token that is no invite's, ErrInviteRevoked,
// ErrInviteUsedUp and ErrInviteExpired one that may no longer.
func (s *Store) InviteByToken(ctx context.Context, token string) (Invite, error) {
	invite, err := lookUpInvite(ctx, s.db, token, false)
	if err != nil {
		return Invite{}, err
	}
	return invite, invite.stateError(s.now())
}

// AcceptInvite creates the member account a guest asked for through the
// invite of token, under the rules of CreateUser, and counts the use. The
// account copies the settings of the invite's model, or takes those of a
// new user, hidden from the sign-in screen; it is never an administrator.
// The invite is locked meanwhile: two guests on its last use create one
// account, and the other gets ErrInviteUsedUp. It returns the invite as it
// was before this use, with InviteByToken's errors.
func (s *Store) AcceptInvite(ctx context.Context, token, name, password string) (User, Invite, error) {
	var user User
	var invite Invite
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var err error
		if invite, err = lookUpInvite(ctx, tx, token, true); err != nil {
			return err
		}
		if err := invite.stateError(s.now()); err != nil {
			return err
		}
		if user, err = createUser(ctx, tx, NewUser{Name: name, Password: password, IsHidden: true}); err != nil {
			return err
		}
		if invite.Model != nil {
			user, err = scanUser(tx.QueryRow(ctx, "UPDATE users SET ("+modelColumns+") = (SELECT "+modelColumns+
				" FROM users WHERE id = $2) WHERE id = $1 RETURNING "+userColumns, user.ID, invite.Model.ID))
			if err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, "UPDATE invites SET uses = uses + 1 WHERE id = $1", invite.ID)
		return err
	})
	if err != nil {
		return User{}, Invite{}, err
	}
	return user, invite, nil
}
