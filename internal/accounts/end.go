package accounts

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// EndCheckInterval is how often the accounts that reached their end are
// looked for, to record them (see Store.EndedAccounts). The end itself
// applies on its date, or at its time, without waiting for it.
const EndCheckInterval = time.Hour

// ErrAdministratorEnds refuses an end date for an administrator: an
// administrator's account never ends, so that the last one is never locked
// out.
var ErrAdministratorEnds = errors.New("an administrator's account cannot end")

// Day is the date of t in the server's time zone, as end dates are kept:
// midnight UTC of that date.
func Day(t time.Time) time.Time {
	year, month, day := t.In(time.Local).Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// Ended reports whether the account's end has come at now: its end date,
// in the server's time zone, or its exact end.
func (u User) Ended(now time.Time) bool {
	return u.EndsOn != nil && !u.EndsOn.After(Day(now)) || u.EndsAt != nil && !now.Before(*u.EndsAt)
}

// Refused reports whether the account is refused at now, as Jellyfin apps
// see a disabled one: disabled, or ended.
func (u User) Refused(now time.Time) bool {
	return u.IsDisabled || u.Ended(now)
}

// endDate is the date an end change gives an account, nil for never.
func endDate(change time.Time) *time.Time {
	if change.IsZero() {
		return nil
	}
	year, month, day := change.Date()
	return new(time.Date(year, month, day, 0, 0, 0, 0, time.UTC))
}

// EndedAccounts returns the accounts that reached their end at now, date
// or exact time, and were not returned yet, once each: an account given a
// new end is returned again when it reaches it. Their share links are revoked, which
// their end already refuses.
func (s *Store) EndedAccounts(ctx context.Context) ([]User, error) {
	var ended []User
	now := s.now()
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "UPDATE users SET end_recorded = true WHERE (ends_on <= $1 OR ends_at <= $2) AND NOT end_recorded RETURNING "+userColumns,
			Day(now), now)
		if err != nil {
			return err
		}
		if ended, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (User, error) { return scanUser(row) }); err != nil {
			return err
		}
		ids := make([]ID, len(ended))
		for i, user := range ended {
			ids[i] = user.ID
		}
		return revokeShareLinks(ctx, tx, ids...)
	})
	return ended, err
}
