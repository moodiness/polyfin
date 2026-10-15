package accounts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/totp"
)

var (
	// ErrTwoStepOn reports setting up two-step sign-in for a user who has it.
	ErrTwoStepOn = errors.New("two-step sign-in is already on")
	// ErrTwoStepOff reports a second step asked of a user without it.
	ErrTwoStepOff = errors.New("two-step sign-in is off")
	// ErrTwoStepNotStarted reports a confirmation without a setup started,
	// or one whose secret the key can no longer read.
	ErrTwoStepNotStarted = errors.New("two-step sign-in is not being set up")
	// ErrInvalidCode reports a code that is neither the authenticator
	// app's, within a step of now and not used yet, nor a recovery code
	// left.
	ErrInvalidCode = errors.New("invalid code")
)

// RecoveryCodes is how many recovery codes a user is given at a time.
const RecoveryCodes = 10

// TwoStepIssuer names the server in authenticator apps.
const TwoStepIssuer = "Polyfin"

// TwoStep is how a user's two-step sign-in stands.
type TwoStep struct {
	On bool
	// RecoveryCodes counts the recovery codes left.
	RecoveryCodes int
	// Unreadable tells that POLYFIN_SECRET_KEY cannot open the secret, as
	// after a restore under another key: only recovery codes sign in.
	Unreadable bool
}

// TwoStepSetup is what an authenticator app reads: the secret in base32,
// and the otpauth:// address that holds it.
type TwoStepSetup struct {
	Secret string
	URI    string
}

// NeedsTwoStep tells whether u's password alone does not sign them in: they
// use two-step sign-in, or are an administrator while settings require it
// of administrators, set up or not.
func (u User) NeedsTwoStep(settings Settings) bool {
	return u.TwoStep || (u.IsAdministrator && settings.TwoStepAdministrators)
}

// TwoStep reads how user's two-step sign-in stands.
func (s *Store) TwoStep(ctx context.Context, user ID) (TwoStep, error) {
	var result TwoStep
	var secret string
	err := s.db.QueryRow(ctx, "SELECT two_step, two_step_secret, cardinality(two_step_recovery) FROM users WHERE id = $1", user).
		Scan(&result.On, &secret, &result.RecoveryCodes)
	if errors.Is(err, pgx.ErrNoRows) {
		return TwoStep{}, ErrNotFound
	}
	if err != nil {
		return TwoStep{}, err
	}
	if !result.On {
		return TwoStep{}, nil
	}
	_, err = s.box.Open(secret)
	result.Unreadable = err != nil
	return result, nil
}

// StartTwoStep gives a user without two-step sign-in a new secret, which
// replaces the one being set up if any; it stays off until ConfirmTwoStep.
func (s *Store) StartTwoStep(ctx context.Context, user User) (TwoStepSetup, error) {
	secret := totp.NewSecret()
	tag, err := s.db.Exec(ctx, "UPDATE users SET two_step_secret = $2, two_step_step = 0, two_step_recovery = '{}' WHERE id = $1 AND NOT two_step",
		user.ID, s.box.Seal(secret))
	if err != nil {
		return TwoStepSetup{}, err
	}
	if tag.RowsAffected() == 0 {
		if _, err := s.User(ctx, user.ID); err != nil {
			return TwoStepSetup{}, err
		}
		return TwoStepSetup{}, ErrTwoStepOn
	}
	return TwoStepSetup{Secret: secret, URI: totp.URI(TwoStepIssuer, user.Name, secret)}, nil
}

// ConfirmTwoStep turns two-step sign-in on once code, from the app given
// the secret StartTwoStep made, matches it, and returns the user's
// recovery codes, which only their hashes are kept of.
func (s *Store) ConfirmTwoStep(ctx context.Context, user ID, code string) ([]string, error) {
	defer s.forgetSignIns()
	var codes []string
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var on bool
		var stored string
		err := tx.QueryRow(ctx, "SELECT two_step, two_step_secret FROM users WHERE id = $1 FOR UPDATE", user).Scan(&on, &stored)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if on {
			return ErrTwoStepOn
		}
		secret, err := s.box.Open(stored)
		if err != nil || secret == "" {
			return ErrTwoStepNotStarted
		}
		step, ok := totp.Match(secret, code, s.now(), 0)
		if !ok {
			return ErrInvalidCode
		}
		var hashes [][]byte
		codes, hashes = newRecoveryCodes()
		_, err = tx.Exec(ctx, "UPDATE users SET two_step = true, two_step_step = $2, two_step_recovery = $3 WHERE id = $1",
			user, step, hashes)
		return err
	})
	return codes, err
}

// VerifyTwoStep checks the second step of user's sign-in: a code of their
// authenticator app, within a step of now and later than the last one
// accepted, or one of their recovery codes, which is then used up. It
// tells whether a recovery code was used. When the key cannot open the
// secret, only recovery codes are accepted.
func (s *Store) VerifyTwoStep(ctx context.Context, user ID, code string) (recovery bool, err error) {
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var on bool
		var stored string
		var last int64
		var hashes [][]byte
		err := tx.QueryRow(ctx, "SELECT two_step, two_step_secret, two_step_step, two_step_recovery FROM users WHERE id = $1 FOR UPDATE", user).
			Scan(&on, &stored, &last, &hashes)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !on {
			return ErrTwoStepOff
		}
		if secret, err := s.box.Open(stored); err == nil {
			if step, ok := totp.Match(secret, code, s.now(), last); ok {
				_, err := tx.Exec(ctx, "UPDATE users SET two_step_step = $2 WHERE id = $1", user, step)
				return err
			}
		}
		hash := recoveryHash(code)
		left := slices.DeleteFunc(hashes, func(h []byte) bool { return string(h) == string(hash) })
		if len(left) == len(hashes) {
			return ErrInvalidCode
		}
		recovery = true
		_, err = tx.Exec(ctx, "UPDATE users SET two_step_recovery = $2 WHERE id = $1", user, left)
		return err
	})
	return recovery, err
}

// NewRecoveryCodes gives a user with two-step sign-in new recovery codes,
// which void the former ones.
func (s *Store) NewRecoveryCodes(ctx context.Context, user ID) ([]string, error) {
	codes, hashes := newRecoveryCodes()
	tag, err := s.db.Exec(ctx, "UPDATE users SET two_step_recovery = $2 WHERE id = $1 AND two_step", user, hashes)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		if _, err := s.User(ctx, user); err != nil {
			return nil, err
		}
		return nil, ErrTwoStepOff
	}
	return codes, nil
}

// TurnOffTwoStep turns a user's two-step sign-in off, or stops its setup,
// forgetting their secret and recovery codes. The sessions open stay.
func (s *Store) TurnOffTwoStep(ctx context.Context, user ID) (User, error) {
	defer s.forgetSignIns()
	return scanUser(s.db.QueryRow(ctx, `UPDATE users SET two_step = false, two_step_secret = '', two_step_step = 0, two_step_recovery = '{}'
		WHERE id = $1 RETURNING `+userColumns, user))
}

// CheckPassword tells whether password is user's, for a choice they
// confirm with it. Wrong passwords count toward no block here: the caller
// throttles them.
func (s *Store) CheckPassword(ctx context.Context, user ID, password string) (bool, error) {
	var hash string
	if err := s.db.QueryRow(ctx, "SELECT password_hash FROM users WHERE id = $1", user).Scan(&hash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, ErrNotFound
		}
		return false, err
	}
	if len(password) > maxPasswordLength {
		return false, nil
	}
	return verifyPassword(password, hash)
}

// recoveryEncoding writes recovery codes in lowercase Crockford base32,
// whose digits and letters have no look-alikes: no i, l, o or u. It reads
// aloud and types without ambiguity of case.
var recoveryEncoding = base32.NewEncoding("0123456789abcdefghjkmnpqrstvwxyz").WithPadding(base32.NoPadding)

// newRecoveryCodes makes RecoveryCodes codes of 80 random bits, written
// xxxx-xxxx-xxxx-xxxx, and their hashes.
func newRecoveryCodes() ([]string, [][]byte) {
	codes := make([]string, RecoveryCodes)
	hashes := make([][]byte, RecoveryCodes)
	for i := range codes {
		var raw [10]byte
		_, _ = rand.Read(raw[:])
		text := recoveryEncoding.EncodeToString(raw[:])
		codes[i] = text[:4] + "-" + text[4:8] + "-" + text[8:12] + "-" + text[12:]
		hashes[i] = recoveryHash(codes[i])
	}
	return codes, hashes
}

// recoveryHash is the stored form of a recovery code, letter case, spaces
// and dashes aside. A fast hash is enough: each code holds 80 random bits,
// so that searching its hash among 2^80 codes stays out of reach, even
// with a copy of the database and many GPUs, where a slow hash would only
// make every attempt at sign-in cost more.
func recoveryHash(code string) []byte {
	normalized := strings.ToLower(strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(code)))
	sum := sha256.Sum256([]byte(normalized))
	return sum[:]
}
