package watchlist

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The titles of a watchlist are checked for versions in the background:
// every CheckInterval, up to PerPass titles of each user's watchlist that
// they have not started, among those not checked for the hours the
// settings set (accounts.Settings.WatchlistCheckHours, 6 by default, 0
// for never), those never checked first. A title is available when an
// addon lists a version the user may play (see Found): the check records
// it, with when versions were first found, and the Now available and Still
// waiting rows list the titles by it (see Store.Available and
// Store.Waiting). The first time versions are found for a title the
// previous checks found none for, the user is told, once: a title
// available at its first check was so when it was added, and is not news.
// A title that loses its versions goes back to Still waiting, and is not
// told of again when it finds some anew.

const (
	// CheckInterval is how often the checks run: each takes the titles
	// due then.
	CheckInterval = time.Hour
	// PerPass bounds the titles of one user checked at each run, each of
	// which asks the user's stream addons.
	PerPass = 20
)

// Found is what a check of a title found for a user.
type Found int

const (
	// Unseen is a title the user may not see, by parental control,
	// blocked genres or their hidden titles, or one gone: no addon is
	// asked, and what earlier checks found stays.
	Unseen Found = iota
	// Unreleased is a title not released yet, a series none of whose
	// episodes is: no addon is asked, and the title waits.
	Unreleased
	// Unknown is a title no version was found for while an addon was not
	// asked, as it asked to wait, or failed: the title is checked again
	// at the next run.
	Unknown
	// Missing is a title every addon answered for, none with a version
	// the user may play.
	Missing
	// Available is a title an addon lists a version of that the user may
	// play.
	Available
)

// CheckerOptions are what a Checker needs.
type CheckerOptions struct {
	Store *Store
	// Users lists the users whose watchlists are checked; the refused
	// ones are left out.
	Users func(ctx context.Context) ([]accounts.User, error)
	// Hours is how many hours after its last check a title is checked
	// again, 0 for never, read at each run.
	Hours func() int
	// Find checks one title for a user.
	Find func(ctx context.Context, user accounts.User, title accounts.ID) (Found, error)
	// Tell tells the user that versions were found for title; nil tells
	// nobody.
	Tell   func(ctx context.Context, user accounts.User, title accounts.ID)
	Logger *slog.Logger
	// Now is the clock, time.Now when nil.
	Now func() time.Time
}

// Checker checks the titles of the watchlists for versions.
type Checker struct {
	CheckerOptions
}

// NewChecker returns a checker.
func NewChecker(options CheckerOptions) *Checker {
	if options.Now == nil {
		options.Now = time.Now
	}
	return &Checker{options}
}

// Check checks the titles due of every user's watchlist, one after the
// other, until ctx ends.
func (c *Checker) Check(ctx context.Context) error {
	hours := c.Hours()
	if hours <= 0 {
		return nil
	}
	users, err := c.Users(ctx)
	if err != nil {
		return err
	}
	for _, user := range users {
		now := c.Now()
		if user.Refused(now) {
			continue
		}
		due, err := c.Store.Due(ctx, user.ID, now.Add(-time.Duration(hours)*time.Hour), PerPass)
		if err != nil {
			return err
		}
		for _, title := range due {
			if err := c.check(ctx, user, title); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				c.Logger.Warn("A watchlist title could not be checked for versions", "user_id", user.ID.String(),
					"item_id", title.String(), "error", err)
			}
		}
	}
	return nil
}

// check checks one title of user's watchlist, and records what it found.
func (c *Checker) check(ctx context.Context, user accounts.User, title accounts.ID) error {
	found, err := c.Find(ctx, user, title)
	if err != nil {
		return err
	}
	now := c.Now()
	switch found {
	case Unknown:
		return nil
	case Unseen:
		return c.Store.Checked(ctx, user.ID, title, now)
	}
	told, err := c.Store.Record(ctx, user.ID, title, found == Available, now)
	if err == nil && told && c.Tell != nil {
		c.Tell(ctx, user, title)
	}
	return err
}

// Due lists up to limit titles of user's watchlist they have not started
// whose last check is before before: those never checked first, then the
// longest unchecked, the latest added first among them.
func (s *Store) Due(ctx context.Context, user accounts.ID, before time.Time, limit int) ([]accounts.ID, error) {
	rows, err := s.db.Query(ctx, `SELECT w.item_id FROM watchlist w
		LEFT JOIN watchlist_availability a ON a.user_id = w.user_id AND a.item_id = w.item_id
		WHERE w.user_id = $1 AND (a.checked_at IS NULL OR a.checked_at < $2) AND `+unstarted+`
		GROUP BY w.item_id, a.checked_at ORDER BY a.checked_at NULLS FIRST, max(w.added_at) DESC, w.item_id LIMIT $3`, user, before, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
}

// Checked records a check of title for user at now that found nothing to
// record: what earlier checks found stays, and a title never checked
// waits.
func (s *Store) Checked(ctx context.Context, user, title accounts.ID, now time.Time) error {
	_, err := s.db.Exec(ctx, `INSERT INTO watchlist_availability (user_id, item_id, checked_at, available) VALUES ($1, $2, $3, false)
		ON CONFLICT (user_id, item_id) DO UPDATE SET checked_at = excluded.checked_at`, user, title, now)
	return err
}

// Record records a check of title for user at now that found versions, or
// none. It reports whether the user is to be told: versions found for the
// first time, for a title the earlier checks found none for.
func (s *Store) Record(ctx context.Context, user, title accounts.ID, available bool, now time.Time) (told bool, err error) {
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var first *time.Time
		err := tx.QueryRow(ctx, "SELECT first_available_at FROM watchlist_availability WHERE user_id = $1 AND item_id = $2 FOR UPDATE",
			user, title).Scan(&first)
		checked := err == nil
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		told = available && checked && first == nil
		_, err = tx.Exec(ctx, `INSERT INTO watchlist_availability (user_id, item_id, checked_at, available, available_since, first_available_at)
			VALUES ($1, $2, $3, $4, CASE WHEN $4 THEN $3::timestamptz END, CASE WHEN $4 THEN $3::timestamptz END)
			ON CONFLICT (user_id, item_id) DO UPDATE SET checked_at = excluded.checked_at, available = excluded.available,
				available_since = CASE WHEN excluded.available THEN coalesce(watchlist_availability.available_since, excluded.checked_at) END,
				first_available_at = coalesce(watchlist_availability.first_available_at, excluded.first_available_at)`,
			user, title, now, available)
		return err
	})
	return told && err == nil, err
}

// Available lists the titles of user's watchlist they have not started
// that their last check found versions for, those found the latest first.
func (s *Store) Available(ctx context.Context, user accounts.ID) ([]accounts.ID, error) {
	rows, err := s.db.Query(ctx, `SELECT w.item_id FROM watchlist w
		JOIN watchlist_availability a ON a.user_id = w.user_id AND a.item_id = w.item_id
		WHERE w.user_id = $1 AND a.available AND `+unstarted+`
		GROUP BY w.item_id, a.available_since ORDER BY a.available_since DESC, w.item_id`, user)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
}

// Waiting lists the titles of user's watchlist they have not started that
// were checked and are not available, the latest added first, as Titles
// orders them.
func (s *Store) Waiting(ctx context.Context, user accounts.ID) ([]accounts.ID, error) {
	rows, err := s.db.Query(ctx, `SELECT w.item_id FROM watchlist w
		JOIN watchlist_availability a ON a.user_id = w.user_id AND a.item_id = w.item_id
		WHERE w.user_id = $1 AND NOT a.available AND `+unstarted+`
		GROUP BY w.item_id ORDER BY max(w.added_at) DESC, w.item_id`, user)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
}
