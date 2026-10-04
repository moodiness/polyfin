// Package accounts stores users, their signed-in devices, admin interface
// sessions and server settings.
package accounts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidName        = errors.New("invalid user name")
	ErrInvalidPassword    = errors.New("invalid password")
	ErrNameTaken          = errors.New("user name already taken")
	ErrNotFound           = errors.New("not found")
	ErrLastAdministrator  = errors.New("the last enabled administrator must stay an enabled administrator")
	ErrInvalidCredentials = errors.New("invalid user name or password")
	ErrDisabled           = errors.New("account disabled")
	ErrWrongPassword      = errors.New("wrong current password")
	ErrSetupComplete      = errors.New("an administrator already exists")
)

const (
	maxNameLength     = 64
	minPasswordLength = 8
	// Bounds the cost of hashing attacker-supplied input.
	maxPasswordLength = 256
	// Serializes changes that could leave the server without an enabled
	// administrator.
	administratorsLock int64 = 0x706f6c79_61646d6e // "polyadmn"
)

// User is an account usable from Jellyfin apps and the admin interface.
type User struct {
	ID              ID
	Name            string
	IsAdministrator bool
	// IsHidden keeps the user off the sign-in screen of Jellyfin apps.
	IsHidden       bool
	IsDisabled     bool
	CreatedAt      time.Time
	LastLoginAt    *time.Time
	LastActivityAt *time.Time
	// Parental limits the titles the user reaches by their rating.
	Parental ParentalControl
}

// NewUser describes an account to create.
type NewUser struct {
	Name            string
	Password        string
	IsAdministrator bool
	IsHidden        bool
}

// UserChanges lists the fields to update; nil fields are kept.
type UserChanges struct {
	Name            *string
	Password        *string
	IsAdministrator *bool
	IsHidden        *bool
	IsDisabled      *bool
	Parental        *ParentalControl
}

// Store is the accounts repository.
type Store struct {
	db       *pgxpool.Pool
	settings atomic.Pointer[Settings]
	// signedOut is told of the devices signed out.
	signedOut atomic.Pointer[func(devices []ID)]
}

// OnSignOut has f told of the devices signed out, once their tokens no
// longer work: signed out by their app, revoked by an administrator, or
// signed out with the rest of their account by a password change, or by
// the account being disabled or deleted.
func (s *Store) OnSignOut(f func(devices []ID)) {
	s.signedOut.Store(&f)
}

func (s *Store) notifySignOut(devices []ID) {
	if f := s.signedOut.Load(); f != nil && len(devices) > 0 {
		(*f)(devices)
	}
}

// qualifiedUserColumns prefixes userColumns with a table alias for joins.
func qualifiedUserColumns(alias string) string {
	return alias + "." + strings.ReplaceAll(userColumns, ", ", ", "+alias+".")
}

// Open returns a store backed by db and loads the server settings.
func Open(ctx context.Context, db *pgxpool.Pool) (*Store, error) {
	store := &Store{db: db}
	settings, err := store.loadSettings(ctx)
	if err != nil {
		return nil, err
	}
	store.settings.Store(&settings)
	return store, nil
}

const userColumns = "id, name, is_administrator, is_hidden, is_disabled, created_at, last_login_at, last_activity_at, " +
	"max_parental_rating, max_parental_sub_rating, block_unrated_items"

// fields lists where the userColumns of a row go.
func (user *User) fields() []any {
	return []any{&user.ID, &user.Name, &user.IsAdministrator, &user.IsHidden, &user.IsDisabled,
		&user.CreatedAt, &user.LastLoginAt, &user.LastActivityAt,
		&user.Parental.MaxRating, &user.Parental.MaxSubRating, &user.Parental.BlockUnrated}
}

func scanUser(row pgx.Row) (User, error) {
	var user User
	err := row.Scan(user.fields()...)
	if errors.Is(err, pgx.ErrNoRows) {
		return user, ErrNotFound
	}
	return user, err
}

// normalizeName trims a user name and checks the characters Jellyfin apps
// display and type reliably.
func normalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxNameLength {
		return "", ErrInvalidName
	}
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune(" -_'.@+", r) {
			return "", ErrInvalidName
		}
	}
	return name, nil
}

func checkPassword(password string) error {
	length := utf8.RuneCountInString(password)
	if length < minPasswordLength || len(password) > maxPasswordLength {
		return ErrInvalidPassword
	}
	return nil
}

func uniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// SetupRequired reports whether no administrator exists yet.
func (s *Store) SetupRequired(ctx context.Context) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE is_administrator)").Scan(&exists)
	return !exists, err
}

// CreateFirstAdministrator creates the first administrator, once. language,
// the language the administrator set the server up in, becomes the server
// language when it is one of Languages; anything else is ignored.
func (s *Store) CreateFirstAdministrator(ctx context.Context, name, password, language string) (User, error) {
	var user User
	var settings *Settings
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", administratorsLock); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE is_administrator)").Scan(&exists); err != nil {
			return err
		}
		if exists {
			return ErrSetupComplete
		}
		var err error
		user, err = createUser(ctx, tx, NewUser{Name: name, Password: password, IsAdministrator: true, IsHidden: true})
		if err != nil || !ValidLanguage(language) {
			return err
		}
		settings = &Settings{}
		return tx.QueryRow(ctx, `UPDATE settings SET language = $1
			RETURNING server_name, quick_connect_enabled, legacy_authorization, language, chapters, prepare_ahead`, language).
			Scan(&settings.ServerName, &settings.QuickConnectEnabled, &settings.LegacyAuthorization, &settings.Language, &settings.Chapters, &settings.PrepareAhead)
	})
	if err == nil && settings != nil {
		s.settings.Store(settings)
	}
	return user, err
}

// CreateUser adds an account.
func (s *Store) CreateUser(ctx context.Context, user NewUser) (User, error) {
	return createUser(ctx, s.db, user)
}

func createUser(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, user NewUser) (User, error) {
	name, err := normalizeName(user.Name)
	if err != nil {
		return User{}, err
	}
	if err := checkPassword(user.Password); err != nil {
		return User{}, err
	}
	created, err := scanUser(db.QueryRow(ctx,
		"INSERT INTO users (name, password_hash, is_administrator, is_hidden) VALUES ($1, $2, $3, $4) RETURNING "+userColumns,
		name, hashPassword(user.Password), user.IsAdministrator, user.IsHidden))
	if uniqueViolation(err) {
		return User{}, ErrNameTaken
	}
	return created, err
}

// User returns one account.
func (s *Store) User(ctx context.Context, id ID) (User, error) {
	return scanUser(s.db.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE id = $1", id))
}

// Users returns every account, sorted by name.
func (s *Store) Users(ctx context.Context) ([]User, error) {
	rows, err := s.db.Query(ctx, "SELECT "+userColumns+" FROM users ORDER BY lower(name), id")
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (User, error) { return scanUser(row) })
}

// Authenticate checks a name and password. The name is matched without
// regard to case. A disabled account is reported only after its password is
// verified, so the response does not reveal which names exist.
func (s *Store) Authenticate(ctx context.Context, name, password string) (User, error) {
	var hash string
	var user User
	err := s.db.QueryRow(ctx,
		"SELECT password_hash, "+userColumns+" FROM users WHERE lower(name) = lower($1)",
		strings.TrimSpace(name)).Scan(append([]any{&hash}, user.fields()...)...)
	if errors.Is(err, pgx.ErrNoRows) {
		_, _ = verifyPassword(password, unknownUserHash)
		return User{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, err
	}
	if len(password) > maxPasswordLength {
		return User{}, ErrInvalidCredentials
	}
	ok, err := verifyPassword(password, hash)
	if err != nil {
		return User{}, fmt.Errorf("user %s: %w", user.ID, err)
	}
	if !ok {
		return User{}, ErrInvalidCredentials
	}
	if user.IsDisabled {
		return User{}, ErrDisabled
	}
	return user, nil
}

// UpdateUser applies changes to an account. A new password or disabling the
// account signs out all of its devices and admin sessions, except the admin
// session whose token hash is keepSession (the caller changing their own
// password stays signed in).
func (s *Store) UpdateUser(ctx context.Context, id ID, changes UserChanges, keepSession []byte) (User, error) {
	return s.updateUser(ctx, id, changes, signIn{adminSession: keepSession})
}

// UpdateUserFromDevice applies changes an administrator made from a
// Jellyfin app: when they sign the account out, the app's device, which
// may be the account's own, stays signed in.
func (s *Store) UpdateUserFromDevice(ctx context.Context, id ID, changes UserChanges, device ID) (User, error) {
	return s.updateUser(ctx, id, changes, signIn{device: &device})
}

// signIn names the admin session or Jellyfin device that made a change to
// an account, which stays signed in when the change signs the account out
// everywhere else. The zero value keeps nothing.
type signIn struct {
	adminSession []byte
	device       *ID
}

func (s *Store) updateUser(ctx context.Context, id ID, changes UserChanges, keep signIn) (User, error) {
	var name, hash *string
	var parental ParentalControl
	if changes.Parental != nil {
		normalized, err := changes.Parental.normalized()
		if err != nil {
			return User{}, err
		}
		parental = normalized
	}
	if changes.Name != nil {
		normalized, err := normalizeName(*changes.Name)
		if err != nil {
			return User{}, err
		}
		name = &normalized
	}
	if changes.Password != nil {
		if err := checkPassword(*changes.Password); err != nil {
			return User{}, err
		}
		hashed := hashPassword(*changes.Password)
		hash = &hashed
	}
	var updated User
	var signedOut []ID
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", administratorsLock); err != nil {
			return err
		}
		current, err := scanUser(tx.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE id = $1 FOR UPDATE", id))
		if err != nil {
			return err
		}
		staysAdministrator := current.IsAdministrator && (changes.IsAdministrator == nil || *changes.IsAdministrator)
		staysEnabled := !current.IsDisabled && (changes.IsDisabled == nil || !*changes.IsDisabled)
		if current.IsAdministrator && !current.IsDisabled && !(staysAdministrator && staysEnabled) {
			if err := requireAnotherAdministrator(ctx, tx, id); err != nil {
				return err
			}
		}
		updated, err = scanUser(tx.QueryRow(ctx, `UPDATE users SET
				name = coalesce($2, name),
				password_hash = coalesce($3, password_hash),
				is_administrator = coalesce($4, is_administrator),
				is_hidden = coalesce($5, is_hidden),
				is_disabled = coalesce($6, is_disabled),
				max_parental_rating = CASE WHEN $7 THEN $8 ELSE max_parental_rating END,
				max_parental_sub_rating = CASE WHEN $7 THEN $9 ELSE max_parental_sub_rating END,
				block_unrated_items = CASE WHEN $7 THEN $10 ELSE block_unrated_items END
			WHERE id = $1 RETURNING `+userColumns,
			id, name, hash, changes.IsAdministrator, changes.IsHidden, changes.IsDisabled,
			changes.Parental != nil, parental.MaxRating, parental.MaxSubRating, parental.BlockUnrated))
		if uniqueViolation(err) {
			return ErrNameTaken
		}
		if err != nil {
			return err
		}
		if hash != nil || (updated.IsDisabled && !current.IsDisabled) {
			signedOut, err = signOutEverywhere(ctx, tx, id, keep)
		}
		return err
	})
	if err == nil {
		s.notifySignOut(signedOut)
	}
	return updated, err
}

// ChangePassword replaces the caller's own password after checking the
// current one.
func (s *Store) ChangePassword(ctx context.Context, id ID, current, next string, keepSession []byte) error {
	return s.changePassword(ctx, id, current, next, signIn{adminSession: keepSession})
}

// ChangePasswordFromDevice replaces the password of the user signed in on a
// Jellyfin app after checking the current one. The app's device stays
// signed in; the account's other devices and admin sessions are signed out.
func (s *Store) ChangePasswordFromDevice(ctx context.Context, id ID, current, next string, device ID) error {
	return s.changePassword(ctx, id, current, next, signIn{device: &device})
}

func (s *Store) changePassword(ctx context.Context, id ID, current, next string, keep signIn) error {
	var hash string
	if err := s.db.QueryRow(ctx, "SELECT password_hash FROM users WHERE id = $1", id).Scan(&hash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	ok, err := verifyPassword(current, hash)
	if err != nil {
		return err
	}
	if !ok {
		return ErrWrongPassword
	}
	_, err = s.updateUser(ctx, id, UserChanges{Password: &next}, keep)
	return err
}

// DeleteUser removes an account with its devices and sessions.
func (s *Store) DeleteUser(ctx context.Context, id ID) error {
	var signedOut []ID
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", administratorsLock); err != nil {
			return err
		}
		current, err := scanUser(tx.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE id = $1 FOR UPDATE", id))
		if err != nil {
			return err
		}
		if current.IsAdministrator && !current.IsDisabled {
			if err := requireAnotherAdministrator(ctx, tx, id); err != nil {
				return err
			}
		}
		// The devices would go with the account; they are listed first, to
		// be told signed out.
		if signedOut, err = deletedDevices(tx.Query(ctx, "DELETE FROM devices WHERE user_id = $1 RETURNING id", id)); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "DELETE FROM users WHERE id = $1", id)
		return err
	})
	if err == nil {
		s.notifySignOut(signedOut)
	}
	return err
}

func requireAnotherAdministrator(ctx context.Context, tx pgx.Tx, except ID) error {
	var others bool
	err := tx.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM users WHERE is_administrator AND NOT is_disabled AND id <> $1)",
		except).Scan(&others)
	if err != nil {
		return err
	}
	if !others {
		return ErrLastAdministrator
	}
	return nil
}

func signOutEverywhere(ctx context.Context, tx pgx.Tx, user ID, keep signIn) ([]ID, error) {
	devices, err := deletedDevices(tx.Query(ctx, "DELETE FROM devices WHERE user_id = $1 AND id IS DISTINCT FROM $2 RETURNING id", user, keep.device))
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx,
		"DELETE FROM admin_sessions WHERE user_id = $1 AND token_hash IS DISTINCT FROM $2", user, keep.adminSession)
	return devices, err
}
