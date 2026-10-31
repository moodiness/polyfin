package accounts

import (
	"context"
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
	// ErrInvalidInviteAccountEnd refuses an account end out of bounds.
	ErrInvalidInviteAccountEnd = errors.New("invalid invite account end")
	// ErrInviteUnknown reports a token that is no invite's; the other
	// errors, an invite that may no longer create accounts.
	ErrInviteUnknown = errors.New("unknown invite")
	ErrInviteUsedUp  = errors.New("invite used up")
	ErrInviteExpired = errors.New("invite expired")
	ErrInviteRevoked = errors.New("invite revoked")
)

// Invite is an invite link, which lets guests create a member account for
// themselves: at most MaxUses of them, until ExpiresAt (never when nil),
// unless it is revoked. Each account it creates ends AccountDays after it
// is created, never when 0.
type Invite struct {
	ID        ID
	MaxUses   int
	Uses      int
	ExpiresAt *time.Time
	CreatedAt time.Time
	RevokedAt *time.Time
	// Model is the user whose settings the new accounts copy, nil for the
	// defaults of a new user; Group, the group they join and follow, nil
	// for none; CreatedBy, the administrator who created the invite, nil
	// once deleted.
	Model       *InviteUser
	Group       *InviteUser
	CreatedBy   *InviteUser
	AccountDays int
}

// InviteUser names a user, or a group, an invite refers to.
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
// 0 for no expiry; the user whose settings the accounts copy, if any; the
// group they join, if any; the administrator creating it; and how many days
// after its creation each account ends, from 1 to MaxInviteDays, 0 for
// never.
type NewInvite struct {
	MaxUses     int
	Days        int
	Model       *ID
	Group       *ID
	CreatedBy   ID
	AccountDays int
}

const (
	// modelColumns are the settings a new account copies from an invite's
	// model: everything an administrator sets on a user's page but the
	// name, the password, administrator status, the disabled switch and the
	// group.
	modelColumns  = "is_hidden, metadata_language, web_theme, web_theme_locked, " + groupColumns
	inviteColumns = "i.id, i.max_uses, i.uses, i.expires_at, i.created_at, i.revoked_at, " +
		"m.id, m.name, g.id, g.name, c.id, c.name, coalesce(i.account_days, 0)"
	inviteJoins = " FROM invites i LEFT JOIN users m ON m.id = i.model_user_id LEFT JOIN user_groups g ON g.id = i.group_id" +
		" LEFT JOIN users c ON c.id = i.created_by"
)

func scanInvite(row pgx.Row, more ...any) (Invite, error) {
	var invite Invite
	var modelID, groupID, creatorID *ID
	var modelName, groupName, creatorName *string
	err := row.Scan(append([]any{&invite.ID, &invite.MaxUses, &invite.Uses, &invite.ExpiresAt, &invite.CreatedAt, &invite.RevokedAt,
		&modelID, &modelName, &groupID, &groupName, &creatorID, &creatorName, &invite.AccountDays}, more...)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return invite, ErrNotFound
	}
	if modelID != nil {
		invite.Model = &InviteUser{ID: *modelID, Name: *modelName}
	}
	if groupID != nil {
		invite.Group = &InviteUser{ID: *groupID, Name: *groupName}
	}
	if creatorID != nil {
		invite.CreatedBy = &InviteUser{ID: *creatorID, Name: *creatorName}
	}
	return invite, err
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
	if invite.AccountDays < 0 || invite.AccountDays > MaxInviteDays {
		return Invite{}, "", ErrInvalidInviteAccountEnd
	}
	var expires *time.Time
	if invite.Days > 0 {
		expires = new(s.now().Add(time.Duration(invite.Days) * 24 * time.Hour))
	}
	secret, hash := newLinkSecret()
	var created Invite
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		// Taken before the model and the group, as DeleteGroup takes it
		// before locking the group and its members.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", administratorsLock); err != nil {
			return err
		}
		if invite.Model != nil {
			// The model stays until the invite is saved.
			if _, err := scanUser(tx.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE id = $1 FOR SHARE", *invite.Model)); errors.Is(err, ErrNotFound) {
				return ErrInvalidInviteModel
			} else if err != nil {
				return err
			}
		}
		if invite.Group != nil {
			// The group stays until the invite is saved.
			var found bool
			if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM user_groups WHERE id = $1 FOR KEY SHARE)", *invite.Group).Scan(&found); err != nil {
				return err
			} else if !found {
				return ErrInvalidGroup
			}
		}
		var id ID
		if err := tx.QueryRow(ctx, `INSERT INTO invites (secret_hash, max_uses, expires_at, model_user_id, group_id, created_by, account_days)
			VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, 0)) RETURNING id`, hash[:], invite.MaxUses, expires, invite.Model, invite.Group,
			invite.CreatedBy, invite.AccountDays).Scan(&id); err != nil {
			return err
		}
		var err error
		created, err = scanInvite(tx.QueryRow(ctx, "SELECT "+inviteColumns+inviteJoins+" WHERE i.id = $1", id))
		return err
	})
	if err != nil {
		return Invite{}, "", err
	}
	return created, linkToken(created.ID, secret), nil
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
	id, secret, ok := parseLinkToken(token)
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
	if !secretMatches(secret, stored) {
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
// It joins and follows the invite's group, if it names one. It ends the
// invite's AccountDays after today, in the server's time zone. The invite
// is locked meanwhile: two guests on its last use create one account, and
// the other gets ErrInviteUsedUp. It returns the invite as it was before
// this use, with InviteByToken's errors.
func (s *Store) AcceptInvite(ctx context.Context, token, name, password string) (User, Invite, error) {
	var user User
	var invite Invite
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		// Taken before the invite, as changes to groups and users take it
		// before the invites they change.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", administratorsLock); err != nil {
			return err
		}
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
		if invite.Group != nil {
			if user, _, err = applyUserChanges(ctx, tx, user.ID, UserChanges{Group: &invite.Group.ID}, userUpdate{}, signIn{}); err != nil {
				return err
			}
		}
		if invite.AccountDays > 0 {
			user, err = scanUser(tx.QueryRow(ctx, "UPDATE users SET ends_on = $2 WHERE id = $1 RETURNING "+userColumns,
				user.ID, Day(s.now()).AddDate(0, 0, invite.AccountDays)))
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
