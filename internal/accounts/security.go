package accounts

import (
	"context"
	"log/slog"
	"time"
)

// LoginBlock is how long an account stays blocked after
// Settings.LoginAttempts wrong passwords in a row.
const LoginBlock = 15 * time.Minute

// DeviceSweepInterval is how often devices left unused are signed out.
const DeviceSweepInterval = time.Hour

// Blocked reports whether the account refuses every sign-in at now, for
// wrong passwords (see Store.Authenticate).
func (user User) Blocked(now time.Time) bool {
	return user.BlockedUntil != nil && now.Before(*user.BlockedUntil)
}

// BlockError is the wrong password that blocked an account: Authenticate
// returns it, as ErrInvalidCredentials, for the wrong password that made
// Settings.LoginAttempts in a row, once per block. User is the account,
// blocked until its BlockedUntil.
type BlockError struct {
	User User
}

func (e *BlockError) Error() string { return "account blocked: " + ErrInvalidCredentials.Error() }

func (e *BlockError) Unwrap() error { return ErrInvalidCredentials }

// countWrongPassword counts one more wrong password for user, and blocks
// the account until now plus LoginBlock when it makes limit in a row,
// which it reports with the account. The first wrong password after a
// block ended counts as the first again. The count is read and written in
// one statement, so that wrong passwords sent at once all count, and only
// one of them blocks.
func (s *Store) countWrongPassword(ctx context.Context, user ID, limit int, now time.Time) (User, bool, error) {
	defer s.forgetSignIns()
	until := now.Add(LoginBlock)
	var updated User
	var blocked bool
	err := s.db.QueryRow(ctx, `UPDATE users SET
			invalid_login_attempts = CASE WHEN blocked_until <= $2 THEN 1 ELSE invalid_login_attempts + 1 END,
			blocked_until = CASE
				WHEN blocked_until > $2 THEN blocked_until
				WHEN (CASE WHEN blocked_until <= $2 THEN 1 ELSE invalid_login_attempts + 1 END) >= $3 THEN $4
			END
		WHERE id = $1 RETURNING blocked_until IS NOT DISTINCT FROM $4, `+userColumns, user, now, limit, until).
		Scan(append([]any{&blocked}, updated.fields()...)...)
	return updated, blocked, err
}

// Unblock ends the block of an account for wrong passwords and starts their
// count again.
func (s *Store) Unblock(ctx context.Context, id ID) (User, error) {
	defer s.forgetSignIns()
	return scanUser(s.db.QueryRow(ctx,
		"UPDATE users SET invalid_login_attempts = 0, blocked_until = NULL WHERE id = $1 RETURNING "+userColumns, id))
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
// SignOutInactiveDevices) and logs how many; the task scheduler runs it at
// start, then every DeviceSweepInterval.
func (s *Store) SweepInactiveDevices(ctx context.Context, logger *slog.Logger) error {
	signedOut, err := s.SignOutInactiveDevices(ctx)
	if signedOut > 0 {
		logger.Info("Signed out devices left unused", "devices", signedOut, "days", s.Settings().InactiveDeviceDays)
	}
	return err
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
