package accounts

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Resetting limits.
//
// An administrator puts the settings a group holds (groupColumns) of a
// user, or of a group, back to a new user's: the values CreateUser gives.
// The rest of the account stays as it is: the name and password,
// administrator status, the sign-in screen, the disabled switch, the end
// date, the group and whether the user follows it, and everything kept for
// the user elsewhere.

// administratorPermissions are the settings a group holds that CreateUser
// gives an administrator and not a user: an administrator may control
// other users' apps and manage collections, subtitles and Live TV
// recordings unless that is taken away, a user may not unless given it.
// The other settings start at their columns' defaults.
var administratorPermissions = []string{"remote_control", "collection_management", "subtitle_management", "live_tv_management"}

// newUserValues is the SET list that gives a row of users or user_groups a
// new user's values for the groupColumns: each column's default, and
// administrator, an SQL boolean, for administratorPermissions.
func newUserValues(administrator string) string {
	columns := strings.Split(groupColumns, ", ")
	set := make([]string, 0, len(columns))
	for _, column := range columns {
		value := "DEFAULT"
		if slices.Contains(administratorPermissions, column) {
			value = administrator
		}
		set = append(set, column+" = "+value)
	}
	return strings.Join(set, ", ")
}

// errPreview rolls back a change made only to read what it gives.
var errPreview = errors.New("preview")

// preview runs change in a transaction rolled back once it returns.
func (s *Store) preview(ctx context.Context, change func(tx pgx.Tx) error) error {
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := change(tx); err != nil {
			return err
		}
		return errPreview
	})
	if errors.Is(err, errPreview) {
		return nil
	}
	return err
}

// ResetLimits gives the user id a new user's values for the settings a
// group holds, those CreateUser gives an account with their administrator
// status, and returns the user as they are now. A member who follows their
// group has the group's values, and is refused with ErrFollowsGroup; one
// with their own settings stays in the group, not following it. As
// UpdateUser, it applies from the user's next request, and a user who may
// no longer share loses their share links.
func (s *Store) ResetLimits(ctx context.Context, id ID) (User, error) {
	var updated User
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var err error
		if updated, err = resetLimits(ctx, tx, id); err != nil {
			return err
		}
		if !updated.CanShare {
			return revokeShareLinks(ctx, tx, id)
		}
		return nil
	})
	s.forgetSignIns()
	return updated, err
}

// LimitsAfterReset returns the settings ResetLimits would give the user id,
// changing nothing.
func (s *Store) LimitsAfterReset(ctx context.Context, id ID) (Limits, error) {
	var limits Limits
	err := s.preview(ctx, func(tx pgx.Tx) error {
		user, err := resetLimits(ctx, tx, id)
		limits = user.Limits()
		return err
	})
	return limits, err
}

// resetLimits gives the user id a new user's limits in tx. It takes the
// administrators' lock first, which serializes it with the changes to the
// user's group and administrator status.
func resetLimits(ctx context.Context, tx pgx.Tx, id ID) (User, error) {
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", administratorsLock); err != nil {
		return User{}, err
	}
	current, err := scanUser(tx.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE id = $1 FOR UPDATE", id))
	if err != nil {
		return User{}, err
	}
	if current.GroupID != nil && current.FollowGroup {
		return User{}, ErrFollowsGroup
	}
	return scanUser(tx.QueryRow(ctx, "UPDATE users SET "+newUserValues("is_administrator")+" WHERE id = $1 RETURNING "+userColumns, id))
}

// ResetGroup gives the group id a new user's values for the settings it
// holds, those of a group CreateGroup makes without a user to copy, and
// returns it as it is now. Its name stays; the members who follow it take
// the new values at once, as with UpdateGroup, and the others keep their
// own.
func (s *Store) ResetGroup(ctx context.Context, id ID) (Group, error) {
	var updated Group
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", administratorsLock); err != nil {
			return err
		}
		var err error
		if updated, err = resetGroup(ctx, tx, id); err != nil {
			return err
		}
		return spreadGroup(ctx, tx, updated)
	})
	s.forgetSignIns()
	return updated, err
}

// GroupLimitsAfterReset returns the settings ResetGroup would give the
// group id, changing nothing.
func (s *Store) GroupLimitsAfterReset(ctx context.Context, id ID) (Limits, error) {
	var limits Limits
	err := s.preview(ctx, func(tx pgx.Tx) error {
		group, err := resetGroup(ctx, tx, id)
		limits = group.Limits
		return err
	})
	return limits, err
}

func resetGroup(ctx context.Context, tx pgx.Tx, id ID) (Group, error) {
	tag, err := tx.Exec(ctx, "UPDATE user_groups SET "+newUserValues("DEFAULT")+" WHERE id = $1", id)
	if err != nil {
		return Group{}, err
	}
	if tag.RowsAffected() == 0 {
		return Group{}, ErrNotFound
	}
	return scanGroup(tx.QueryRow(ctx, groupSelect+" WHERE id = $1", id))
}
