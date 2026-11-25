package moonfin

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
)

const (
	// MaxEnvelopeBytes bounds a user's settings, as JSON: an app's own
	// come to some tens of kilobytes.
	MaxEnvelopeBytes = 1 << 20
	// MaxDefaultsBytes bounds the administrator's defaults for one
	// profile, as JSON.
	MaxDefaultsBytes = 256 << 10
)

var (
	// ErrNotFound reports a user, or a profile of theirs, with no settings
	// kept.
	ErrNotFound = errors.New("no Moonfin settings")
	// ErrTooLarge reports settings past MaxEnvelopeBytes, or defaults past
	// MaxDefaultsBytes.
	ErrTooLarge = errors.New("settings too large for Moonfin")
	// ErrInvalidProfile reports a profile not in Profiles.
	ErrInvalidProfile = errors.New("invalid Moonfin profile")
)

// Store keeps the users' Moonfin settings and the administrator's
// defaults, and tells those who watch a user's settings when a save
// changed them (see Watch).
type Store struct {
	db  *pgxpool.Pool
	now func() time.Time

	mu       sync.Mutex
	watchers map[accounts.ID]map[chan struct{}]struct{}
}

// New returns the store of the database db.
func New(db *pgxpool.Pool) *Store {
	return &Store{db: db, now: time.Now, watchers: map[accounts.ID]map[chan struct{}]struct{}{}}
}

// Sync tells what a user's settings hold: whether they have any, how many
// settings over which profiles, and when and by which app they were last
// saved.
type Sync struct {
	Synced    bool
	Settings  int
	Profiles  []string
	UpdatedAt *time.Time
	UpdatedBy string
}

func syncOf(env Envelope, updated time.Time) Sync {
	count, profiles := Count(env)
	by, _ := env["lastUpdatedBy"].(string)
	return Sync{Synced: true, Settings: count, Profiles: profiles, UpdatedAt: &updated, UpdatedBy: by}
}

// Envelope returns the settings of user, ErrNotFound when they have none.
func (s *Store) Envelope(ctx context.Context, user accounts.ID) (Envelope, error) {
	var env Envelope
	err := s.db.QueryRow(ctx, "SELECT envelope FROM moonfin_settings WHERE user_id = $1", user).Scan(&env)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return env, err
}

// SyncOf tells what the settings of user hold.
func (s *Store) SyncOf(ctx context.Context, user accounts.ID) (Sync, error) {
	var env Envelope
	var updated time.Time
	err := s.db.QueryRow(ctx, "SELECT envelope, updated_at FROM moonfin_settings WHERE user_id = $1", user).Scan(&env, &updated)
	if errors.Is(err, pgx.ErrNoRows) {
		return Sync{Profiles: []string{}}, nil
	}
	if err != nil {
		return Sync{}, err
	}
	return syncOf(env, updated), nil
}

// Synced lists what the settings of each user who has some hold.
func (s *Store) Synced(ctx context.Context) (map[accounts.ID]Sync, error) {
	rows, _ := s.db.Query(ctx, "SELECT user_id, envelope, updated_at FROM moonfin_settings")
	out := map[accounts.ID]Sync{}
	var user accounts.ID
	var env Envelope
	var updated time.Time
	_, err := pgx.ForEachRow(rows, []any{&user, &env, &updated}, func() error {
		out[user] = syncOf(env, updated)
		env = nil
		return nil
	})
	return out, err
}

// update changes the settings of user as change says, from their current
// ones, nil when they have none, and their baselines, which change may
// change in place. created tells that they had none.
func (s *Store) update(ctx context.Context, user accounts.ID, change func(Envelope, Baselines) Envelope) (created bool, err error) {
	changed := false
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var env Envelope
		var baselines Baselines
		err := tx.QueryRow(ctx, "SELECT envelope, baselines FROM moonfin_settings WHERE user_id = $1 FOR UPDATE", user).Scan(&env, &baselines)
		if errors.Is(err, pgx.ErrNoRows) {
			created, env, baselines = true, nil, Baselines{}
		} else if err != nil {
			return err
		}
		before, _ := json.Marshal(env)
		var previous Envelope
		_ = json.Unmarshal(before, &previous)
		updated := change(env, baselines)
		text, err := json.Marshal(updated)
		if err != nil {
			return err
		}
		if len(text) > MaxEnvelopeBytes {
			return ErrTooLarge
		}
		changed = created || !sameSettings(previous, updated)
		_, err = tx.Exec(ctx, `INSERT INTO moonfin_settings (user_id, envelope, baselines, updated_at) VALUES ($1, $2, $3, $4)
			ON CONFLICT (user_id) DO UPDATE SET envelope = $2, baselines = $3, updated_at = $4`, user, text, baselines, s.now())
		return err
	})
	if err == nil && changed {
		s.tell(user)
	}
	return created, err
}

// SaveProfile merges profile, a profile Clean made, into the settings of
// user (see SaveProfile). created tells that they had none.
func (s *Store) SaveProfile(ctx context.Context, user accounts.ID, name string, profile Settings, clientID string) (bool, error) {
	if !slices.Contains(Profiles, name) {
		return false, ErrInvalidProfile
	}
	return s.update(ctx, user, func(env Envelope, baselines Baselines) Envelope {
		return SaveProfile(env, baselines, name, profile, clientID, s.now())
	})
}

// SaveEnvelope keeps a whole envelope an app sent for user, merged into
// theirs or replacing it (see SaveEnvelope).
func (s *Store) SaveEnvelope(ctx context.Context, user accounts.ID, env Envelope, replace bool, clientID string) (bool, error) {
	return s.update(ctx, user, func(current Envelope, baselines Baselines) Envelope {
		return SaveEnvelope(current, env, baselines, replace, clientID, s.now())
	})
}

// Delete forgets the settings of user.
func (s *Store) Delete(ctx context.Context, user accounts.ID) error {
	tag, err := s.db.Exec(ctx, "DELETE FROM moonfin_settings WHERE user_id = $1", user)
	if err == nil && tag.RowsAffected() > 0 {
		s.tell(user)
	}
	return err
}

// DeleteProfile forgets one device profile of the settings of user: the
// device then takes the global one.
func (s *Store) DeleteProfile(ctx context.Context, user accounts.ID, name string) error {
	if name == Global || !slices.Contains(Profiles, name) {
		return ErrInvalidProfile
	}
	tag, err := s.db.Exec(ctx, `UPDATE moonfin_settings SET envelope = jsonb_set(envelope - $2::text, '{lastUpdated}', to_jsonb($3::bigint)),
		baselines = baselines - $2::text, updated_at = $4
		WHERE user_id = $1 AND envelope ? $2`, user, name, s.now().UnixMilli(), s.now())
	if err == nil && tag.RowsAffected() > 0 {
		s.tell(user)
	}
	return err
}

// Resolved is the profile name of the settings of user as a device of its
// kind takes them (see Resolve), the administrator's defaults included.
// It records the hidden titles it gives the device (see the baselines).
func (s *Store) Resolved(ctx context.Context, user accounts.ID, name string) (Settings, error) {
	if !slices.Contains(Profiles, name) {
		return nil, ErrInvalidProfile
	}
	defaults, err := s.Defaults(ctx)
	if err != nil {
		return nil, err
	}
	var resolved Settings
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var env Envelope
		var baselines Baselines
		err := tx.QueryRow(ctx, "SELECT envelope, baselines FROM moonfin_settings WHERE user_id = $1 FOR UPDATE", user).Scan(&env, &baselines)
		if errors.Is(err, pgx.ErrNoRows) {
			resolved = Resolve(nil, name, LayerDefaults(defaults, name))
			return nil
		}
		if err != nil {
			return err
		}
		resolved = Resolve(env, name, LayerDefaults(defaults, name))
		if baselines == nil {
			baselines = Baselines{}
		}
		if !told(baselines, name, resolved) {
			return nil
		}
		_, err = tx.Exec(ctx, "UPDATE moonfin_settings SET baselines = $2 WHERE user_id = $1", user, baselines)
		return err
	})
	return resolved, err
}

// Defaults returns the administrator's defaults, by profile, each present.
func (s *Store) Defaults(ctx context.Context) (map[string]Settings, error) {
	out := map[string]Settings{}
	for _, name := range Profiles {
		out[name] = Settings{}
	}
	rows, _ := s.db.Query(ctx, "SELECT profile, settings FROM moonfin_defaults")
	var name string
	var settings Settings
	_, err := pgx.ForEachRow(rows, []any{&name, &settings}, func() error {
		out[name] = settings
		settings = nil
		return nil
	})
	return out, err
}

// SetDefaults sets the administrator's defaults for profile name, without
// a user's PersonalKeys: empty, there are none.
func (s *Store) SetDefaults(ctx context.Context, name string, settings Settings) error {
	if !slices.Contains(Profiles, name) {
		return ErrInvalidProfile
	}
	settings = withoutPersonal(Clean(settings))
	text, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	if len(text) > MaxDefaultsBytes {
		return ErrTooLarge
	}
	if len(settings) == 0 {
		_, err = s.db.Exec(ctx, "DELETE FROM moonfin_defaults WHERE profile = $1", name)
		return err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO moonfin_defaults (profile, settings) VALUES ($1, $2)
		ON CONFLICT (profile) DO UPDATE SET settings = $2`, name, text)
	return err
}

// CopyToDefaults makes the profile from of the settings of user the
// administrator's defaults for profile to, without their PersonalKeys.
// ErrNotFound tells that the user has no such profile.
func (s *Store) CopyToDefaults(ctx context.Context, user accounts.ID, from, to string) error {
	if !slices.Contains(Profiles, from) || !slices.Contains(Profiles, to) {
		return ErrInvalidProfile
	}
	env, err := s.Envelope(ctx, user)
	if err != nil {
		return err
	}
	profile := profileOf(env, from)
	if len(withoutPersonal(profile)) == 0 {
		return ErrNotFound
	}
	return s.SetDefaults(ctx, to, maps.Clone(profile))
}

// Apply writes the administrator's defaults into the settings of every
// user who has some (see ApplyDefaults), and returns how many there were.
func (s *Store) Apply(ctx context.Context, reset bool) (int, error) {
	defaults, err := s.Defaults(ctx)
	if err != nil {
		return 0, err
	}
	var users []accounts.ID
	rows, _ := s.db.Query(ctx, "SELECT user_id FROM moonfin_settings ORDER BY user_id")
	var user accounts.ID
	if _, err := pgx.ForEachRow(rows, []any{&user}, func() error {
		users = append(users, user)
		return nil
	}); err != nil {
		return 0, err
	}
	applied := 0
	for _, user := range users {
		_, err := s.update(ctx, user, func(env Envelope, _ Baselines) Envelope {
			return ApplyDefaults(env, defaults, reset, s.now())
		})
		if errors.Is(err, ErrTooLarge) {
			continue
		}
		if err != nil {
			return applied, err
		}
		applied++
	}
	return applied, nil
}

// Watch returns a channel told each time a save changes the settings of
// user, and the function that stops it. A change told while the last
// is still unread is one with it.
func (s *Store) Watch(user accounts.ID) (<-chan struct{}, func()) {
	changed := make(chan struct{}, 1)
	s.mu.Lock()
	if s.watchers[user] == nil {
		s.watchers[user] = map[chan struct{}]struct{}{}
	}
	s.watchers[user][changed] = struct{}{}
	s.mu.Unlock()
	return changed, func() {
		s.mu.Lock()
		delete(s.watchers[user], changed)
		if len(s.watchers[user]) == 0 {
			delete(s.watchers, user)
		}
		s.mu.Unlock()
	}
}

func (s *Store) tell(user accounts.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for changed := range s.watchers[user] {
		select {
		case changed <- struct{}{}:
		default:
		}
	}
}
