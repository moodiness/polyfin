package dlna

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Device is a DLNA device seen on the local network, by its address.
type Device struct {
	ID      accounts.ID
	Address string
	// Name, Manufacturer and Model are what its description or its
	// requests announce.
	Name, Manufacturer, Model string
	// UserAgent and ClientInfo are the User-Agent and X-AV-Client-Info
	// headers of its latest request.
	UserAgent, ClientInfo string
	// User is the user whose libraries it shows; nil, no one's.
	User    *accounts.ID
	Blocked bool
	// Profile forces a profile: BuiltinPrefix and a built-in profile's name,
	// or an added profile's ID; empty, it is chosen from what the device
	// sends.
	Profile   string
	FirstSeen time.Time
	LastSeen  time.Time
}

// BuiltinPrefix starts Device.Profile for a built-in profile.
const BuiltinPrefix = "builtin:"

// Sighting is what a device announced, at an address.
type Sighting struct {
	Address                   string
	Name, Manufacturer, Model string
	UserAgent, ClientInfo     string
}

// StoredProfile is a profile an administrator added.
type StoredProfile struct {
	ID        accounts.ID
	Name      string
	XML       string
	CreatedAt time.Time
	Profile   Profile
}

var (
	// ErrNotFound reports a device or a profile that does not exist.
	ErrNotFound = errors.New("not found")
	// ErrUnknownProfile reports a device forced to a profile that does not
	// exist.
	ErrUnknownProfile = errors.New("unknown DLNA profile")
	// ErrUnknownUser reports a device given a user that does not exist.
	ErrUnknownUser = errors.New("unknown user")
)

// lastSeenEvery is how often a device's last sighting is written while it
// keeps sending requests.
const lastSeenEvery = time.Minute

// store keeps the devices and the added profiles.
type store struct {
	db *pgxpool.Pool
	mu sync.Mutex
	// devices are the devices read or written lately, by address.
	devices map[string]Device
	// written is when each address's sighting was last written.
	written map[string]time.Time
}

func newStore(db *pgxpool.Pool) *store {
	return &store{db: db, devices: map[string]Device{}, written: map[string]time.Time{}}
}

const deviceColumns = "id, address, name, manufacturer, model, user_agent, client_info, user_id, blocked, profile, first_seen, last_seen"

func scanDevice(row pgx.Row) (Device, error) {
	var d Device
	err := row.Scan(&d.ID, &d.Address, &d.Name, &d.Manufacturer, &d.Model, &d.UserAgent, &d.ClientInfo, &d.User, &d.Blocked, &d.Profile,
		&d.FirstSeen, &d.LastSeen)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, err
}

// clip cuts text to max bytes, on a character boundary, without control
// characters.
func clip(text string, max int) string {
	text = strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return -1
		}
		return r
	}, strings.TrimSpace(text))
	for len(text) > max {
		_, size := utf8.DecodeLastRuneInString(text)
		text = text[:len(text)-size]
	}
	return text
}

// seen records a sighting: a device seen for the first time shows the
// libraries of newUser; a known one keeps its user, and takes the names it
// announces now when it announces any. The sighting is written at most
// every lastSeenEvery when nothing changed.
func (s *store) seen(ctx context.Context, sighting Sighting, newUser *accounts.ID, now time.Time) (Device, error) {
	sighting.Name, sighting.Manufacturer, sighting.Model = clip(sighting.Name, 256), clip(sighting.Manufacturer, 256), clip(sighting.Model, 256)
	sighting.UserAgent, sighting.ClientInfo = clip(sighting.UserAgent, 512), clip(sighting.ClientInfo, 512)
	s.mu.Lock()
	known, ok := s.devices[sighting.Address]
	fresh := ok && now.Sub(s.written[sighting.Address]) < lastSeenEvery &&
		(sighting.Name == "" || sighting.Name == known.Name) && (sighting.Model == "" || sighting.Model == known.Model) &&
		(sighting.Manufacturer == "" || sighting.Manufacturer == known.Manufacturer) &&
		(sighting.UserAgent == "" || sighting.UserAgent == known.UserAgent) && (sighting.ClientInfo == "" || sighting.ClientInfo == known.ClientInfo)
	s.mu.Unlock()
	if fresh {
		return known, nil
	}
	device, err := scanDevice(s.db.QueryRow(ctx, `
		INSERT INTO dlna_devices (address, name, manufacturer, model, user_agent, client_info, user_id, first_seen, last_seen)
		VALUES ($1, $2, $3, $4, $5, $6, (SELECT id FROM users WHERE id = $7), $8, $8)
		ON CONFLICT (address) DO UPDATE SET
			name = COALESCE(NULLIF(EXCLUDED.name, ''), dlna_devices.name),
			manufacturer = COALESCE(NULLIF(EXCLUDED.manufacturer, ''), dlna_devices.manufacturer),
			model = COALESCE(NULLIF(EXCLUDED.model, ''), dlna_devices.model),
			user_agent = COALESCE(NULLIF(EXCLUDED.user_agent, ''), dlna_devices.user_agent),
			client_info = COALESCE(NULLIF(EXCLUDED.client_info, ''), dlna_devices.client_info),
			last_seen = EXCLUDED.last_seen
		RETURNING `+deviceColumns, sighting.Address, sighting.Name, sighting.Manufacturer, sighting.Model, sighting.UserAgent,
		sighting.ClientInfo, newUser, now))
	if err != nil {
		return Device{}, err
	}
	s.remember(device, now)
	return device, nil
}

// remember keeps device as read or written at now.
func (s *store) remember(device Device, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devices[device.Address] = device
	s.written[device.Address] = now
}

// forget drops what is kept of the devices, which an update changed.
func (s *store) forget() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.devices)
	clear(s.written)
}

// list returns the devices, the latest seen first.
func (s *store) list(ctx context.Context) ([]Device, error) {
	rows, err := s.db.Query(ctx, "SELECT "+deviceColumns+" FROM dlna_devices ORDER BY last_seen DESC, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	devices := []Device{}
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

// device returns the device of id.
func (s *store) device(ctx context.Context, id accounts.ID) (Device, error) {
	return scanDevice(s.db.QueryRow(ctx, "SELECT "+deviceColumns+" FROM dlna_devices WHERE id = $1", id))
}

// DeviceChanges are what an administrator changes of a device; nil fields
// are left as they are.
type DeviceChanges struct {
	// User is the user whose libraries it shows; a pointer to nil shows no
	// one's.
	User    **accounts.ID
	Blocked *bool
	Profile *string
}

// update changes a device.
func (s *store) update(ctx context.Context, id accounts.ID, changes DeviceChanges) (Device, error) {
	if changes.Profile != nil && *changes.Profile != "" {
		if _, err := s.profileOf(ctx, *changes.Profile); err != nil {
			return Device{}, err
		}
	}
	device, err := s.device(ctx, id)
	if err != nil {
		return Device{}, err
	}
	if changes.User != nil {
		device.User = *changes.User
	}
	if changes.Blocked != nil {
		device.Blocked = *changes.Blocked
	}
	if changes.Profile != nil {
		device.Profile = *changes.Profile
	}
	if device.User != nil {
		var exists bool
		if err := s.db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)", *device.User).Scan(&exists); err != nil {
			return Device{}, err
		}
		if !exists {
			return Device{}, ErrUnknownUser
		}
	}
	device, err = scanDevice(s.db.QueryRow(ctx, "UPDATE dlna_devices SET user_id = $2, blocked = $3, profile = $4 WHERE id = $1 RETURNING "+
		deviceColumns, id, device.User, device.Blocked, device.Profile))
	s.forget()
	return device, err
}

// remove forgets a device: seen again, it is new.
func (s *store) remove(ctx context.Context, id accounts.ID) error {
	tag, err := s.db.Exec(ctx, "DELETE FROM dlna_devices WHERE id = $1", id)
	s.forget()
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// profiles returns the added profiles, by name.
func (s *store) profiles(ctx context.Context) ([]StoredProfile, error) {
	rows, err := s.db.Query(ctx, "SELECT id, name, xml, created_at FROM dlna_profiles ORDER BY lower(name), id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	profiles := []StoredProfile{}
	for rows.Next() {
		var p StoredProfile
		if err := rows.Scan(&p.ID, &p.Name, &p.XML, &p.CreatedAt); err != nil {
			return nil, err
		}
		// Saved profiles were read when they were saved.
		p.Profile, _ = ParseProfile([]byte(p.XML))
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

// profileOf returns the profile a device's Profile names.
func (s *store) profileOf(ctx context.Context, name string) (Profile, error) {
	if builtin, ok := strings.CutPrefix(name, BuiltinPrefix); ok {
		for _, p := range builtins {
			if p.Name == builtin {
				return p, nil
			}
		}
		return Profile{}, ErrUnknownProfile
	}
	id, err := accounts.ParseID(name)
	if err != nil {
		return Profile{}, ErrUnknownProfile
	}
	p, err := s.profile(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return Profile{}, ErrUnknownProfile
	}
	return p.Profile, err
}

// profile returns an added profile.
func (s *store) profile(ctx context.Context, id accounts.ID) (StoredProfile, error) {
	var p StoredProfile
	err := s.db.QueryRow(ctx, "SELECT id, name, xml, created_at FROM dlna_profiles WHERE id = $1", id).Scan(&p.ID, &p.Name, &p.XML, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, err
	}
	p.Profile, _ = ParseProfile([]byte(p.XML))
	return p, nil
}

// saveProfile adds a profile, or replaces the one of id when set.
func (s *store) saveProfile(ctx context.Context, id *accounts.ID, data []byte) (StoredProfile, error) {
	parsed, err := ParseProfile(data)
	if err != nil {
		return StoredProfile{}, err
	}
	p := StoredProfile{Name: parsed.Name, XML: string(data), Profile: parsed}
	if id == nil {
		err = s.db.QueryRow(ctx, "INSERT INTO dlna_profiles (name, xml) VALUES ($1, $2) RETURNING id, created_at", p.Name, p.XML).
			Scan(&p.ID, &p.CreatedAt)
	} else {
		err = s.db.QueryRow(ctx, "UPDATE dlna_profiles SET name = $2, xml = $3 WHERE id = $1 RETURNING id, created_at", *id, p.Name, p.XML).
			Scan(&p.ID, &p.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return StoredProfile{}, ErrNotFound
		}
	}
	return p, err
}

// removeProfile deletes an added profile; the devices forced to it choose
// theirs again.
func (s *store) removeProfile(ctx context.Context, id accounts.ID) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, "DELETE FROM dlna_profiles WHERE id = $1", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, "UPDATE dlna_devices SET profile = '' WHERE profile = $1", id.String()); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.forget()
	return nil
}
