package accounts

import (
	"context"
	"log/slog"
	"time"
)

// LoginBlock is how long an account stays blocked after
// Settings.LoginAttempts wrong passwords in a row.
const LoginBlock = 15 * time.Minute

// deviceSweepInterval is how often devices left unused are signed out.
const deviceSweepInterval = time.Hour

// Blocked reports whether the account refuses every sign-in at now, for
// wrong passwords (see Store.Authenticate).
func (user User) Blocked(now time.Time) bool {
	return user.BlockedUntil != nil && now.Before(*user.BlockedUntil)
}

// countWrongPassword counts one more wrong password for user, and blocks
// the account until now plus LoginBlock when it makes limit in a row. The
// first wrong password after a block ended counts as the first again. The
// count is read and written in one statement, so that wrong passwords sent
// at once all count.
func (s *Store) countWrongPassword(ctx context.Context, user ID, limit int, now time.Time) error {
	_, err := s.db.Exec(ctx, `UPDATE users SET
			invalid_login_attempts = CASE WHEN blocked_until <= $2 THEN 1 ELSE invalid_login_attempts + 1 END,
			blocked_until = CASE
				WHEN blocked_until > $2 THEN blocked_until
				WHEN (CASE WHEN blocked_until <= $2 THEN 1 ELSE invalid_login_attempts + 1 END) >= $3 THEN $4
			END
		WHERE id = $1`, user, now, limit, now.Add(LoginBlock))
	return err
}

// Unblock ends the block of an account for wrong passwords and starts their
// count again.
func (s *Store) Unblock(ctx context.Context, id ID) (User, error) {
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
	if err != nil {
		return 0, err
	}
	s.notifySignOut(devices)
	return len(devices), nil
}

// SweepInactiveDevices signs out the devices left unused (see
// SignOutInactiveDevices) at once, then every hour until ctx ends.
func (s *Store) SweepInactiveDevices(ctx context.Context, logger *slog.Logger) {
	ticker := time.NewTicker(deviceSweepInterval)
	defer ticker.Stop()
	for {
		switch signedOut, err := s.SignOutInactiveDevices(ctx); {
		case err != nil && ctx.Err() == nil:
			logger.Warn("Could not sign out the devices left unused", "error", err)
		case signedOut > 0:
			logger.Info("Signed out devices left unused", "devices", signedOut, "days", s.Settings().InactiveDeviceDays)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
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
