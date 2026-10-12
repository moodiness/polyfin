package accounts

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// EndCheckInterval is how often the accounts that reached their end date
// are looked for, to record them (see Store.EndedAccounts). The end itself
// applies on its date, without waiting for it.
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

// Ended reports whether the account's end date has come at now, in the
// server's time zone.
func (u User) Ended(now time.Time) bool {
	return u.EndsOn != nil && !u.EndsOn.After(Day(now))
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

// EndedAccounts returns the accounts that reached their end date at now and
// were not returned yet, once each: an account given a new end date is
// returned again when it reaches it.
func (s *Store) EndedAccounts(ctx context.Context) ([]User, error) {
	rows, err := s.db.Query(ctx, "UPDATE users SET end_recorded = true WHERE ends_on <= $1 AND NOT end_recorded RETURNING "+userColumns,
		Day(s.now()))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (User, error) { return scanUser(row) })
}
