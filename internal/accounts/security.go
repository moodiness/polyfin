package accounts

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"
)

// LoginBlock is how long a client stays blocked for an account after
// Settings.LoginAttempts wrong passwords in a row.
const LoginBlock = 15 * time.Minute

// blockClients is how many clients' wrong passwords are kept for one
// account. Past it, the client whose last wrong password is the oldest
// gives way: its count starts again, and its block ends.
const blockClients = 64

// DeviceSweepInterval is how often devices left unused are signed out.
const DeviceSweepInterval = time.Hour

// Blocked reports whether the account refuses the password sign-ins of
// one of its clients at now, for that client's wrong passwords (see
// Store.AuthenticateFrom): the latest of its blocks is still in force.
func (user User) Blocked(now time.Time) bool {
	return user.BlockedUntil != nil && now.Before(*user.BlockedUntil)
}

// BlockError is the wrong password that blocked a client for an account:
// AuthenticateFrom and the password changes return it, as the error of any
// wrong password of theirs, for the one that made Settings.LoginAttempts
// in a row from that client, once per block. User is the account, with
// that client's count as its InvalidLoginAttempts and the end of its block
// as its BlockedUntil.
type BlockError struct {
	User User
	// refusal is the error of a wrong password where this one was sent.
	refusal error
}

func (e *BlockError) Error() string { return "account blocked: " + e.refusal.Error() }

func (e *BlockError) Unwrap() error { return e.refusal }

// blockClient is the client whose wrong passwords those sent from address
// count with: the address, an IPv6 one by its /64, which one client holds
// whole. An address that does not read as one counts as it is written.
func blockClient(address string) string {
	parsed, err := netip.ParseAddr(address)
	if err != nil {
		return address
	}
	parsed = parsed.Unmap().WithZone("")
	if parsed.Is4() {
		return parsed.String()
	}
	return netip.PrefixFrom(parsed, 64).Masked().String()
}

// passwordChecked applies the block to a password of user's account that
// was just verified, right or not, sent by from: its RemoteAddress is the
// client, and its DeviceID, when it names one, the device. It returns the
// account as the check leaves it, or refusal, the error of a wrong
// password there: for a wrong password, which it counts, as a *BlockError
// when it blocks the client, and for any password of a blocked client,
// unless it comes from a device the account signed in from before. A right
// password starts its client's count again, except one that only its
// device let through: the client's block then lasts its time.
func (s *Store) passwordChecked(ctx context.Context, user User, right bool, from DeviceInfo, refusal error) (User, error) {
	limit, now := s.Settings().LoginAttempts, s.now()
	client := blockClient(from.RemoteAddress)
	// The account's row tells whether one of its clients is blocked: only
	// then is this client's own block read.
	passed := false
	if limit > 0 && user.Blocked(now) {
		var blocked, seen bool
		err := s.db.QueryRow(ctx, `SELECT
				EXISTS (SELECT FROM wrong_passwords WHERE user_id = $1 AND client = $2 AND blocked_until > $3),
				$4 <> '' AND EXISTS (SELECT FROM seen_devices WHERE user_id = $1 AND device_id = $4)`,
			user.ID, client, now, from.DeviceID).Scan(&blocked, &seen)
		if err != nil {
			return User{}, err
		}
		if blocked && !seen {
			return User{}, refusal
		}
		passed = blocked
	}
	if !right {
		if limit > 0 {
			counted, blocked, err := s.countWrongPassword(ctx, user.ID, client, limit, now)
			if err != nil {
				return User{}, err
			}
			if blocked {
				return User{}, &BlockError{User: counted, refusal: refusal}
			}
		}
		return User{}, refusal
	}
	if !passed && (user.InvalidLoginAttempts > 0 || user.BlockedUntil != nil) {
		return s.changeWrongPasswords(ctx, user.ID, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "DELETE FROM wrong_passwords WHERE user_id = $1 AND (client = $2 OR blocked_until <= $3)", user.ID, client, now)
			return err
		})
	}
	return user, nil
}

// countWrongPassword counts one more wrong password of client for user's
// account, and blocks that client until now plus LoginBlock when it makes
// limit in a row, which it reports with the account, as a BlockError
// holds it. The first wrong password after a block ended counts as the
// first again. The blocks that ended are forgotten, and so is the client
// whose last wrong password is the oldest once the account keeps more than
// blockClients.
func (s *Store) countWrongPassword(ctx context.Context, user ID, client string, limit int, now time.Time) (User, bool, error) {
	var attempts int
	var until *time.Time
	blocked := false
	counted, err := s.changeWrongPasswords(ctx, user, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "DELETE FROM wrong_passwords WHERE user_id = $1 AND blocked_until <= $2", user, now); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `INSERT INTO wrong_passwords AS w (user_id, client, attempts, last_at) VALUES ($1, $2, 1, $3)
			ON CONFLICT (user_id, client) DO UPDATE SET attempts = w.attempts + 1, last_at = excluded.last_at
			RETURNING attempts, blocked_until`, user, client, now).Scan(&attempts, &until)
		if err != nil {
			return err
		}
		if until == nil && attempts >= limit {
			end := now.Add(LoginBlock)
			until, blocked = &end, true
			if _, err := tx.Exec(ctx, "UPDATE wrong_passwords SET blocked_until = $3 WHERE user_id = $1 AND client = $2", user, client, end); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `DELETE FROM wrong_passwords WHERE user_id = $1 AND client IN (
			SELECT client FROM wrong_passwords WHERE user_id = $1 ORDER BY last_at DESC, client OFFSET $2)`, user, blockClients)
		return err
	})
	if err != nil {
		return User{}, false, err
	}
	counted.InvalidLoginAttempts, counted.BlockedUntil = attempts, until
	return counted, blocked, nil
}

// changeWrongPasswords has change write the wrong passwords kept for
// user's account, then writes on the account what shows them, its
// InvalidLoginAttempts and BlockedUntil: the highest count and the latest
// block among its clients. It returns the account. The account's row is
// held meanwhile, so that wrong passwords sent at once count one after
// the other: all count, and only one of them blocks its client.
func (s *Store) changeWrongPasswords(ctx context.Context, user ID, change func(pgx.Tx) error) (User, error) {
	defer s.forgetSignIns()
	var updated User
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT FROM users WHERE id = $1 FOR UPDATE", user); err != nil {
			return err
		}
		if err := change(tx); err != nil {
			return err
		}
		var err error
		updated, err = scanUser(tx.QueryRow(ctx, `UPDATE users SET
				invalid_login_attempts = coalesce((SELECT max(w.attempts) FROM wrong_passwords w WHERE w.user_id = $1), 0),
				blocked_until = (SELECT max(w.blocked_until) FROM wrong_passwords w WHERE w.user_id = $1)
			WHERE id = $1 RETURNING `+userColumns, user))
		return err
	})
	return updated, err
}

// Unblock ends every block of an account for wrong passwords and starts
// the count of each of its clients again.
func (s *Store) Unblock(ctx context.Context, id ID) (User, error) {
	return s.changeWrongPasswords(ctx, id, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "DELETE FROM wrong_passwords WHERE user_id = $1", id)
		return err
	})
}

// forgetWrongPasswords ends every block of every account and starts every
// count again, and reports whether an account showed any. The accounts'
// rows are held before their clients are forgotten, as
// changeWrongPasswords holds one.
func (s *Store) forgetWrongPasswords(ctx context.Context) (bool, error) {
	var shown bool
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		unblocked, err := tx.Exec(ctx, "UPDATE users SET invalid_login_attempts = 0, blocked_until = NULL "+
			"WHERE invalid_login_attempts <> 0 OR blocked_until IS NOT NULL")
		if err != nil {
			return err
		}
		shown = unblocked.RowsAffected() > 0
		_, err = tx.Exec(ctx, "DELETE FROM wrong_passwords")
		return err
	})
	return shown, err
}

// SignOutInactiveDevices signs out the Jellyfin apps unused for
// Settings.InactiveDeviceDays, when set, through the same path as any other
// sign-out, and returns how many. Admin interface sessions have their own
// lifetime and are left alone.
func (s *Store) SignOutInactiveDevices(ctx context.Context) (int, error) {
	days := s.Settings().InactiveDeviceDays
	if days == 0 {
		return 0, nil
	}
	devices, err := deletedDevices(s.db.Query(ctx, "DELETE FROM devices WHERE last_activity_at < $1 RETURNING id",
		s.now().AddDate(0, 0, -days)))
	s.forgetSignIns()
	if err != nil {
		return 0, err
	}
	s.notifySignOut(devices)
	return len(devices), nil
}

// SweepInactiveDevices signs out the devices left unused (see
// SignOutInactiveDevices) and forgets how those still signed in but unused
// for UnusedDevicePlay play (see ForgetUnusedDevicePlays), and logs how
// many; the task scheduler runs it at start, then every
// DeviceSweepInterval.
func (s *Store) SweepInactiveDevices(ctx context.Context, logger *slog.Logger) error {
	signedOut, err := s.SignOutInactiveDevices(ctx)
	if signedOut > 0 {
		logger.Info("Signed out devices left unused", "devices", signedOut, "days", s.Settings().InactiveDeviceDays)
	}
	forgotten, forgetErr := s.ForgetUnusedDevicePlays(ctx)
	if forgotten > 0 {
		logger.Debug("Forgot the device profiles of devices left unused", "devices", forgotten)
	}
	return errors.Join(err, forgetErr)
}

// followedLevel is a log level that follows the settings' DetailedLog.
type followedLevel struct {
	level *slog.LevelVar
	// configured is the level while DetailedLog is off.
	configured slog.Level
}

// FollowLogLevel sets level to debug while the settings' DetailedLog is on,
// and to configured while it is off, from now on and at every change of the
// settings, so that a logger using level follows at once.
func (s *Store) FollowLogLevel(level *slog.LevelVar, configured slog.Level) {
	s.logLevel.Store(&followedLevel{level: level, configured: configured})
	s.applyLogLevel()
}

// storeSettings makes settings the current settings, which the log level
// follows, and tells those waiting on SettingsChanged.
func (s *Store) storeSettings(settings *Settings) {
	s.settings.Store(settings)
	s.applyLogLevel()
	s.changedMu.Lock()
	defer s.changedMu.Unlock()
	if s.changed != nil {
		close(s.changed)
		s.changed = nil
	}
}

// SettingsChanged returns a channel closed at the next change of the
// settings, for what follows them without a restart: read the settings
// after taking it, and take it again once it is closed.
func (s *Store) SettingsChanged() <-chan struct{} {
	s.changedMu.Lock()
	defer s.changedMu.Unlock()
	if s.changed == nil {
		s.changed = make(chan struct{})
	}
	return s.changed
}

// applyLogLevel sets the followed level from the current settings.
func (s *Store) applyLogLevel() {
	followed := s.logLevel.Load()
	if followed == nil {
		return
	}
	if s.Settings().DetailedLog {
		followed.level.Set(slog.LevelDebug)
	} else {
		followed.level.Set(followed.configured)
	}
}
