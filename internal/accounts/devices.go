package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"unicode/utf8"

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
	// The device's previous token, if any, stops working.
	defer s.forgetSignIns()
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
// records the activity, at most once a minute or when the device's address
// changes. A token resolved less than signInLife ago is not looked up again.
func (s *Store) DeviceByToken(ctx context.Context, token, remoteAddress string) (Device, User, error) {
	hash := string(hashToken(token))
	now := s.now()
	entry, ok := s.signIns.device(hash, now)
	if !ok {
		generation := s.signIns.current()
		device, err := scanDevice(s.db.QueryRow(ctx, "SELECT "+deviceColumns+", "+qualifiedUserColumns("u")+`
			FROM devices d JOIN users u ON u.id = d.user_id
			WHERE d.token_hash = $1 AND NOT u.is_disabled`, []byte(hash)), entry.user.fields()...)
		if err != nil {
			return Device{}, User{}, err
		}
		entry.device, entry.at = device, now
		s.signIns.keepDevice(hash, entry, generation)
	}
	if now.Sub(entry.device.LastActivityAt) >= activityResolution || entry.device.RemoteAddress != remoteAddress {
		_, err := s.db.Exec(ctx, `WITH touched AS (
				UPDATE devices SET last_activity_at = $3, remote_address = $2 WHERE id = $1 RETURNING user_id
			) UPDATE users SET last_activity_at = $3 WHERE id = (SELECT user_id FROM touched)`,
			entry.device.ID, remoteAddress, now)
		if err != nil {
			return Device{}, User{}, err
		}
		s.signIns.touched(hash, now, remoteAddress)
		entry.device.LastActivityAt, entry.device.RemoteAddress = now, remoteAddress
		entry.user.LastActivityAt = &now
	}
	return entry.device, entry.user, nil
}

// SignOutDevice revokes an access token.
func (s *Store) SignOutDevice(ctx context.Context, token string) error {
	devices, err := deletedDevices(s.db.Query(ctx, "DELETE FROM devices WHERE token_hash = $1 RETURNING id", hashToken(token)))
	s.forgetSignIns()
	if err == nil {
		s.notifySignOut(devices)
	}
	return err
}

// RevokeDevice signs a user's device out.
func (s *Store) RevokeDevice(ctx context.Context, user, device ID) error {
	devices, err := deletedDevices(s.db.Query(ctx, "DELETE FROM devices WHERE id = $1 AND user_id = $2 RETURNING id", device, user))
	s.forgetSignIns()
	switch {
	case err != nil:
		return err
	case len(devices) == 0:
		return ErrNotFound
	}
	s.notifySignOut(devices)
	return nil
}

// deletedDevices collects the identifiers of the devices a deletion
// returns.
func deletedDevices(rows pgx.Rows, err error) ([]ID, error) {
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[ID])
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
	// Requests carry the device, capabilities included.
	defer s.forgetSignIns()
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

// ListedDevice is a signed-in device as administrators see it: with the
// name of its user and the name an administrator gave its device
// identifier, nil when none.
type ListedDevice struct {
	Device
	UserName   string
	CustomName *string
}

// DeviceOptions are what an administrator set for a device identifier, as
// Jellyfin's DeviceOptions; ID numbers them in the order they were made.
type DeviceOptions struct {
	ID         int
	DeviceID   string
	CustomName *string
}

// maxDeviceNameLength bounds the names administrators give devices.
const maxDeviceNameLength = 256

// ErrInvalidDeviceName reports a device name longer than
// maxDeviceNameLength, or a device identifier that is empty or as long.
var ErrInvalidDeviceName = errors.New("invalid device name")

// AllDevices lists every signed-in device of every user, or those of
// deviceID when it is not empty, most recently active first.
func (s *Store) AllDevices(ctx context.Context, deviceID string) ([]ListedDevice, error) {
	rows, err := s.db.Query(ctx, "SELECT "+deviceColumns+`, u.name, o.custom_name
		FROM devices d JOIN users u ON u.id = d.user_id LEFT JOIN device_options o ON o.device_id = d.device_id
		WHERE $1 = '' OR d.device_id = $1
		ORDER BY d.last_activity_at DESC, d.device_id, d.id`, deviceID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ListedDevice, error) {
		var listed ListedDevice
		device, err := scanDevice(row, &listed.UserName, &listed.CustomName)
		listed.Device = device
		return listed, err
	})
}

// SignOutDeviceID signs out every user signed in on a device identifier,
// through the same path as any other sign-out, and returns how many.
func (s *Store) SignOutDeviceID(ctx context.Context, deviceID string) (int, error) {
	devices, err := deletedDevices(s.db.Query(ctx, "DELETE FROM devices WHERE device_id = $1 RETURNING id", deviceID))
	s.forgetSignIns()
	if err != nil {
		return 0, err
	}
	s.notifySignOut(devices)
	return len(devices), nil
}

// DeviceOptions returns what an administrator set for a device identifier;
// ErrNotFound when nothing was ever set.
func (s *Store) DeviceOptions(ctx context.Context, deviceID string) (DeviceOptions, error) {
	options := DeviceOptions{DeviceID: deviceID}
	err := s.db.QueryRow(ctx, "SELECT id, custom_name FROM device_options WHERE device_id = $1", deviceID).
		Scan(&options.ID, &options.CustomName)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeviceOptions{}, ErrNotFound
	}
	return options, err
}

// SetDeviceName names a device identifier for administrators; nil or an
// empty name removes the name.
func (s *Store) SetDeviceName(ctx context.Context, deviceID string, name *string) error {
	if name != nil && *name == "" {
		name = nil
	}
	if deviceID == "" || len(deviceID) > maxDeviceNameLength || name != nil && utf8.RuneCountInString(*name) > maxDeviceNameLength {
		return ErrInvalidDeviceName
	}
	_, err := s.db.Exec(ctx, `INSERT INTO device_options (device_id, custom_name) VALUES ($1, $2)
		ON CONFLICT (device_id) DO UPDATE SET custom_name = excluded.custom_name`, deviceID, name)
	return err
}
