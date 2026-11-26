package moonfin

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Sign-in PINs.
//
// Moonfin's apps let a user lock their profile on the app's user picker
// with a PIN of PINLength digits. The store keeps it for every device of
// the user: a salted hash, as a password's (see accounts.HashSecret),
// never the PIN, and the wrong guesses since the last right one. Ten
// thousand PINs are soon tried, so past FreePINGuesses wrong ones each
// makes the next guess wait PINWaitStep longer, up to MaxPINWait, and
// while it waits no guess is checked, the right one included. A right
// PIN, or a new one set, starts the count again. The count is the
// user's, whatever the device or address the guesses come from: the
// caller is signed in as the user already, and the PIN guards their
// profile on a shared device, which a count per address would let any
// other device of the household reset.

const (
	// PINLength is the number of digits of a PIN, as Moonfin's apps ask
	// for it.
	PINLength int = 4
	// FreePINGuesses is how many wrong guesses in a row cost no wait.
	FreePINGuesses int = 5
	// PINWaitStep is how much longer each wrong guess past FreePINGuesses
	// makes the next one wait.
	PINWaitStep = 30 * time.Second
	// MaxPINWait is the longest wait, that of a password's block.
	MaxPINWait = accounts.LoginBlock
)

var (
	// ErrNoPIN reports a user without a PIN.
	ErrNoPIN = errors.New("no Moonfin PIN")
	// ErrWrongPIN reports a wrong guess, counted.
	ErrWrongPIN = errors.New("wrong Moonfin PIN")
	// ErrInvalidPIN reports a new PIN that is not PINLength digits.
	ErrInvalidPIN = errors.New("a Moonfin PIN is four digits")
)

// PINWaitError reports a guess refused, or the wrong one that started a
// wait: the next one is checked after Wait.
type PINWaitError struct {
	Wait time.Duration
}

func (e *PINWaitError) Error() string { return "too many wrong Moonfin PINs" }

// Seconds is Wait in whole seconds, rounded up, as Retry-After tells it.
func (e *PINWaitError) Seconds() int {
	return int((e.Wait + time.Second - 1) / time.Second)
}

// ValidPIN reports whether pin is PINLength digits.
func ValidPIN(pin string) bool {
	if len(pin) != PINLength {
		return false
	}
	for _, c := range []byte(pin) {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// pinWait is how long the next guess waits after wrong wrong guesses in a
// row.
func pinWait(wrong int) time.Duration {
	if wrong <= FreePINGuesses {
		return 0
	}
	return min(MaxPINWait, time.Duration(wrong-FreePINGuesses)*PINWaitStep)
}

// PINState tells whether a user has a PIN, the wrong guesses given since
// the last right one, and, while the next guess waits, until when.
type PINState struct {
	Set          bool
	WrongGuesses int
	WaitUntil    *time.Time
}

// PIN tells the state of the PIN of user.
func (s *Store) PIN(ctx context.Context, user accounts.ID) (PINState, error) {
	var state PINState
	err := s.db.QueryRow(ctx, "SELECT wrong_guesses, wait_until FROM moonfin_pins WHERE user_id = $1", user).
		Scan(&state.WrongGuesses, &state.WaitUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return PINState{}, nil
	}
	if err != nil {
		return PINState{}, err
	}
	state.Set = true
	if state.WaitUntil != nil && !s.now().Before(*state.WaitUntil) {
		state.WaitUntil = nil
	}
	return state, nil
}

// CheckPIN checks guess against the PIN of user: nil when right,
// ErrWrongPIN, counted, a *PINWaitError while guesses wait, or for the
// wrong one that starts a wait, and ErrNoPIN for a user without one.
func (s *Store) CheckPIN(ctx context.Context, user accounts.ID, guess string) error {
	var refusal error
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var err error
		refusal, err = s.guess(ctx, tx, user, guess)
		return err
	})
	if err != nil {
		return err
	}
	return refusal
}

// guess checks guess against the PIN of user in tx, its row held until tx
// ends so that guesses at once count one after the other, and records
// what it found. It returns the refusal CheckPIN tells, kept apart from
// the error that ends tx: a wrong guess is counted once tx commits.
func (s *Store) guess(ctx context.Context, tx pgx.Tx, user accounts.ID, guess string) (refusal, err error) {
	var hash string
	var wrong int
	var until *time.Time
	err = tx.QueryRow(ctx, "SELECT hash, wrong_guesses, wait_until FROM moonfin_pins WHERE user_id = $1 FOR UPDATE", user).
		Scan(&hash, &wrong, &until)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoPIN, nil
	}
	if err != nil {
		return nil, err
	}
	now := s.now()
	if until != nil && now.Before(*until) {
		return &PINWaitError{Wait: until.Sub(now)}, nil
	}
	right := false
	if ValidPIN(guess) {
		if right, err = accounts.VerifySecret(guess, hash); err != nil {
			return nil, err
		}
	}
	if right {
		if wrong > 0 || until != nil {
			_, err = tx.Exec(ctx, "UPDATE moonfin_pins SET wrong_guesses = 0, wait_until = NULL WHERE user_id = $1", user)
		}
		return nil, err
	}
	wrong++
	wait := pinWait(wrong)
	var next *time.Time
	if wait > 0 {
		at := now.Add(wait)
		next = &at
	}
	if _, err = tx.Exec(ctx, "UPDATE moonfin_pins SET wrong_guesses = $2, wait_until = $3 WHERE user_id = $1", user, wrong, next); err != nil {
		return nil, err
	}
	if wait > 0 {
		return &PINWaitError{Wait: wait}, nil
	}
	return ErrWrongPIN, nil
}

// SetPIN gives user the PIN pin, in place of any, and starts their count
// of wrong guesses again: for an administrator, who needs no current PIN,
// which is how a forgotten one is reset. ErrInvalidPIN reports a pin that
// is not PINLength digits.
func (s *Store) SetPIN(ctx context.Context, user accounts.ID, pin string) error {
	if !ValidPIN(pin) {
		return ErrInvalidPIN
	}
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		return s.savePIN(ctx, tx, user, accounts.HashSecret(pin))
	})
}

func (s *Store) savePIN(ctx context.Context, tx pgx.Tx, user accounts.ID, hash string) error {
	_, err := tx.Exec(ctx, `INSERT INTO moonfin_pins (user_id, hash, updated_at) VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET hash = $2, wrong_guesses = 0, wait_until = NULL, updated_at = $3`, user, hash, s.now())
	return err
}

// ChangePIN gives user the PIN pin, as the user asks: when they have one,
// current must be it, and a wrong one is a guess, refused as CheckPIN
// refuses it. ErrInvalidPIN reports a pin that is not PINLength digits,
// before any guess.
func (s *Store) ChangePIN(ctx context.Context, user accounts.ID, pin, current string) error {
	if !ValidPIN(pin) {
		return ErrInvalidPIN
	}
	hash := accounts.HashSecret(pin)
	return s.ownPIN(ctx, user, current, func(tx pgx.Tx) error { return s.savePIN(ctx, tx, user, hash) })
}

// RemovePIN takes the PIN of user away, for an administrator.
func (s *Store) RemovePIN(ctx context.Context, user accounts.ID) error {
	_, err := s.db.Exec(ctx, "DELETE FROM moonfin_pins WHERE user_id = $1", user)
	return err
}

// RemoveOwnPIN takes the PIN of user away, as the user asks: current must
// be it, as ChangePIN wants it.
func (s *Store) RemoveOwnPIN(ctx context.Context, user accounts.ID, current string) error {
	return s.ownPIN(ctx, user, current, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "DELETE FROM moonfin_pins WHERE user_id = $1", user)
		return err
	})
}

// ownPIN has change change the PIN of user once current proved to be it,
// or at once when they have none, and returns the refusal of a wrong
// current PIN, counted.
func (s *Store) ownPIN(ctx context.Context, user accounts.ID, current string, change func(pgx.Tx) error) error {
	var refusal error
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var err error
		if refusal, err = s.guess(ctx, tx, user, current); err != nil {
			return err
		}
		if refusal != nil && !errors.Is(refusal, ErrNoPIN) {
			return nil
		}
		refusal = nil
		return change(tx)
	})
	if err != nil {
		return err
	}
	return refusal
}
