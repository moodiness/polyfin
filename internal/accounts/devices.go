package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// activityResolution limits how often a device's last activity is written.
const activityResolution = time.Minute

// DeviceInfo describes the app and device signing in, as sent by Jellyfin
// apps in their Authorization header.
type DeviceInfo struct {
	DeviceID      string
	DeviceName    string
	Client        string
	ClientVersion string
	RemoteAddress string
}

// Capabilities are what a signed-in app reports it can play and control.
type Capabilities struct {
	PlayableMediaTypes           []string `json:"playableMediaTypes"`
	SupportedCommands            []string `json:"supportedCommands"`
	SupportsMediaControl         bool     `json:"supportsMediaControl"`
	SupportsPersistentIdentifier bool     `json:"supportsPersistentIdentifier"`
}

// DefaultCapabilities are those of an app that has not reported any yet.
func DefaultCapabilities() Capabilities {
	return Capabilities{PlayableMediaTypes: []string{}, SupportedCommands: []string{}, SupportsPersistentIdentifier: true}
}

// Device is a Jellyfin app signed in as a user. Each holds one access token.
type Device struct {
	ID     ID
	UserID ID
	DeviceInfo
	Capabilities   Capabilities
	CreatedAt      time.Time
	LastActivityAt time.Time
}

const deviceColumns = "d.id, d.user_id, d.device_id, d.device_name, d.client, d.client_version, d.remote_address, d.capabilities, d.created_at, d.last_activity_at"

func scanDevice(row pgx.Row, extra ...any) (Device, error) {
	var device Device
	var capabilities []byte
	err := row.Scan(append([]any{&device.ID, &device.UserID, &device.DeviceID, &device.DeviceName, &device.Client,
		&device.ClientVersion, &device.RemoteAddress, &capabilities, &device.CreatedAt, &device.LastActivityAt}, extra...)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return device, ErrNotFound
	}
	if err != nil {
		return device, err
	}
	device.Capabilities = DefaultCapabilities()
	if err := json.Unmarshal(capabilities, &device.Capabilities); err != nil {
		return device, err
	}
	return device, nil
}

// SignInDevice issues an access token for a user on a device. Signing in
// again from the same device replaces the previous token, as Jellyfin does,
// and resets the capabilities the app reported.
func (s *Store) SignInDevice(ctx context.Context, user ID, info DeviceInfo) (string, Device, error) {
	token, hash := newToken()
	var device Device
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var err error
		device, err = scanDevice(tx.QueryRow(ctx, `INSERT INTO devices AS d
				(user_id, token_hash, device_id, device_name, client, client_version, remote_address)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (user_id, device_id) DO UPDATE SET
				token_hash = excluded.token_hash,
				device_name = excluded.device_name,
				client = excluded.client,
				client_version = excluded.client_version,
				remote_address = excluded.remote_address,
				capabilities = '{}',
				last_activity_at = now()
			RETURNING `+deviceColumns,
			user, hash, info.DeviceID, info.DeviceName, info.Client, info.ClientVersion, info.RemoteAddress))
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "UPDATE users SET last_login_at = now(), last_activity_at = now() WHERE id = $1", user)
		return err
	})
	if err != nil {
		return "", Device{}, err
	}
	return token, device, nil
}

// DeviceByToken resolves an access token to its device and enabled user, and
// records the activity.
func (s *Store) DeviceByToken(ctx context.Context, token, remoteAddress string) (Device, User, error) {
	var user User
	device, err := scanDevice(s.db.QueryRow(ctx, "SELECT "+deviceColumns+", "+qualifiedUserColumns("u")+`
		FROM devices d JOIN users u ON u.id = d.user_id
		WHERE d.token_hash = $1 AND NOT u.is_disabled`, hashToken(token)),
		&user.ID, &user.Name, &user.IsAdministrator, &user.IsHidden, &user.IsDisabled,
		&user.CreatedAt, &user.LastLoginAt, &user.LastActivityAt)
	if err != nil {
		return Device{}, User{}, err
	}
	if time.Since(device.LastActivityAt) >= activityResolution || device.RemoteAddress != remoteAddress {
		_, err = s.db.Exec(ctx, `WITH touched AS (
				UPDATE devices SET last_activity_at = now(), remote_address = $2 WHERE id = $1 RETURNING user_id
			) UPDATE users SET last_activity_at = now() WHERE id = (SELECT user_id FROM touched)`,
			device.ID, remoteAddress)
		if err != nil {
			return Device{}, User{}, err
		}
		device.RemoteAddress = remoteAddress
	}
	return device, user, nil
}

// SignOutDevice revokes an access token.
func (s *Store) SignOutDevice(ctx context.Context, token string) error {
	_, err := s.db.Exec(ctx, "DELETE FROM devices WHERE token_hash = $1", hashToken(token))
	return err
}

// RevokeDevice signs a user's device out.
func (s *Store) RevokeDevice(ctx context.Context, user, device ID) error {
	tag, err := s.db.Exec(ctx, "DELETE FROM devices WHERE id = $1 AND user_id = $2", device, user)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// Devices lists a user's signed-in devices, most recently active first.
func (s *Store) Devices(ctx context.Context, user ID) ([]Device, error) {
	rows, err := s.db.Query(ctx, "SELECT "+deviceColumns+" FROM devices d WHERE d.user_id = $1 ORDER BY d.last_activity_at DESC, d.id", user)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Device, error) { return scanDevice(row) })
}

// Device returns a signed-in device by its identifier.
func (s *Store) Device(ctx context.Context, id ID) (Device, error) {
	return scanDevice(s.db.QueryRow(ctx, "SELECT "+deviceColumns+" FROM devices d WHERE d.id = $1", id))
}

// SetCapabilities records what a signed-in app reported it supports.
func (s *Store) SetCapabilities(ctx context.Context, device ID, capabilities Capabilities) error {
	if capabilities.PlayableMediaTypes == nil {
		capabilities.PlayableMediaTypes = []string{}
	}
	if capabilities.SupportedCommands == nil {
		capabilities.SupportedCommands = []string{}
	}
	encoded, err := json.Marshal(capabilities)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, "UPDATE devices SET capabilities = $2 WHERE id = $1", device, encoded)
	return err
}
