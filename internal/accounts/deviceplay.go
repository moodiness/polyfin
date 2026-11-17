package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// MaxDevicePlayProfile bounds the device profile kept for a device, in
// bytes of JSON. The profiles apps send weigh from a few hundred bytes to
// 11 KB: jellyfin-web 12.4 in Chrome 4.6 KB, Android TV 5.5 KB, Swiftfin
// 4.6 KB, Jellyfin for Android's integrated player 11 KB.
const MaxDevicePlayProfile = 64 << 10

// ErrDevicePlayTooLarge reports a device profile over MaxDevicePlayProfile.
var ErrDevicePlayTooLarge = errors.New("device profile too large to keep")

// UnusedDevicePlay is how long the profile of a device left unused is
// kept (see ForgetUnusedDevicePlays). Settings.InactiveDeviceDays signs
// such a device out, its profile with it, but never by default, and N
// sign-ins would keep N profiles: a profile goes after as long as Jellyfin
// handlers keep it in memory. The device, once back, names versions with
// how they play from its next PlaybackInfo.
const UnusedDevicePlay = 30 * 24 * time.Hour

// DevicePlay is how a device told it plays, kept across restarts: the
// device profile, as JSON, of its last PlaybackInfo and the bitrate it
// asked for, nil when none, else the profile its session's capabilities
// carried, which FromCapabilities tells.
type DevicePlay struct {
	Profile          json.RawMessage
	Bitrate          *int64
	FromCapabilities bool
}

// SetDevicePlay keeps how a signed-in device plays, in place of what was
// kept for it. A device signed out meanwhile keeps nothing: its profile
// goes with it.
func (s *Store) SetDevicePlay(ctx context.Context, device ID, play DevicePlay) error {
	if len(play.Profile) > MaxDevicePlayProfile {
		return ErrDevicePlayTooLarge
	}
	_, err := s.db.Exec(ctx, `INSERT INTO device_play_profiles (device_id, profile, bitrate, from_capabilities)
		SELECT id, $2, $3, $4 FROM devices WHERE id = $1
		ON CONFLICT (device_id) DO UPDATE SET profile = excluded.profile, bitrate = excluded.bitrate,
			from_capabilities = excluded.from_capabilities, updated_at = now()`,
		device, []byte(play.Profile), play.Bitrate, play.FromCapabilities)
	return err
}

// DevicePlay returns how a device told it plays, as SetDevicePlay kept
// it; false when nothing is kept.
func (s *Store) DevicePlay(ctx context.Context, device ID) (DevicePlay, bool, error) {
	var play DevicePlay
	var profile []byte
	err := s.db.QueryRow(ctx, "SELECT profile, bitrate, from_capabilities FROM device_play_profiles WHERE device_id = $1", device).
		Scan(&profile, &play.Bitrate, &play.FromCapabilities)
	if errors.Is(err, pgx.ErrNoRows) {
		return DevicePlay{}, false, nil
	}
	if err != nil {
		return DevicePlay{}, false, err
	}
	play.Profile = profile
	return play, true, nil
}

// ForgetDevicePlay forgets how a device plays, as when its profile is too
// large to keep: the profile kept before would no longer be the device's.
func (s *Store) ForgetDevicePlay(ctx context.Context, device ID) error {
	_, err := s.db.Exec(ctx, "DELETE FROM device_play_profiles WHERE device_id = $1", device)
	return err
}

// ForgetUnusedDevicePlays forgets how the devices unused for
// UnusedDevicePlay play, signed in as they stay, and returns how many.
func (s *Store) ForgetUnusedDevicePlays(ctx context.Context) (int, error) {
	tag, err := s.db.Exec(ctx, `DELETE FROM device_play_profiles p USING devices d
		WHERE d.id = p.device_id AND d.last_activity_at < $1`, s.now().Add(-UnusedDevicePlay))
	return int(tag.RowsAffected()), err
}
