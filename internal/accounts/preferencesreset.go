package accounts

import (
	"context"
	"time"
)

// MarkPreferencesReset records that the user id reset their own
// preferences now, as User.PreferencesResetAt, and answers the user. The
// settings jellyfin-web keeps in each browser are beyond the server's
// reach: Polyfin's script there compares this time with the one the
// browser last applied, and forgets them when it is newer. As UpdateUser,
// it applies from the user's next request.
func (s *Store) MarkPreferencesReset(ctx context.Context, id ID) (User, error) {
	user, err := scanUser(s.db.QueryRow(ctx,
		"UPDATE users SET preferences_reset_at = $2 WHERE id = $1 RETURNING "+userColumns,
		id, s.now().UTC().Truncate(time.Microsecond)))
	if err != nil {
		return User{}, err
	}
	s.forgetSignIns()
	return user, nil
}
