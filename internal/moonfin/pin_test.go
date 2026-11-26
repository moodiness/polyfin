package moonfin

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The tests hash PINs at a small cost (see
// accounts.LowerPasswordCostForTests).
func TestMain(m *testing.M) {
	accounts.LowerPasswordCostForTests()
	m.Run()
}

// waitOf returns the wait err tells, or fails.
func waitOf(t *testing.T, err error) time.Duration {
	t.Helper()
	var wait *PINWaitError
	if !errors.As(err, &wait) {
		t.Fatalf("not a wait: %v", err)
	}
	return wait.Wait
}

// A user sets a PIN, changes and removes it with the current one, a wrong
// current one counting as a guess; the store keeps a hash of it, never
// the PIN, each user's apart.
func TestPINsSetChangedAndRemoved(t *testing.T) {
	t.Parallel()
	s, ids := newStore(t, "alice", "bob")
	ctx := t.Context()
	alice, bob := ids[0], ids[1]
	if state, err := s.PIN(ctx, alice); err != nil || state.Set {
		t.Fatalf("before any PIN: %+v %v", state, err)
	}
	if err := s.CheckPIN(ctx, alice, "1234"); !errors.Is(err, ErrNoPIN) {
		t.Errorf("a check without a PIN: %v", err)
	}
	for _, pin := range []string{"", "123", "12345", "12a4", "١٢٣٤"} {
		if err := s.ChangePIN(ctx, alice, pin, ""); !errors.Is(err, ErrInvalidPIN) {
			t.Errorf("PIN %q: %v", pin, err)
		}
	}
	if err := s.ChangePIN(ctx, alice, "4821", ""); err != nil {
		t.Fatal(err)
	}
	var hash string
	if err := s.db.QueryRow(ctx, "SELECT hash FROM moonfin_pins WHERE user_id = $1", alice).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(hash, "4821") || !strings.HasPrefix(hash, "$argon2id$") {
		t.Errorf("kept as %q", hash)
	}
	if state, err := s.PIN(ctx, alice); err != nil || !state.Set || state.WrongGuesses != 0 {
		t.Errorf("with a PIN: %+v %v", state, err)
	}
	if err := s.CheckPIN(ctx, alice, "4821"); err != nil {
		t.Errorf("the right PIN: %v", err)
	}
	if err := s.CheckPIN(ctx, bob, "4821"); !errors.Is(err, ErrNoPIN) {
		t.Errorf("bob's: %v", err)
	}
	if err := s.ChangePIN(ctx, alice, "7777", "1111"); !errors.Is(err, ErrWrongPIN) {
		t.Errorf("a change with a wrong current PIN: %v", err)
	}
	if err := s.ChangePIN(ctx, alice, "7777", ""); !errors.Is(err, ErrWrongPIN) {
		t.Errorf("a change without the current PIN: %v", err)
	}
	if state, _ := s.PIN(ctx, alice); state.WrongGuesses != 2 {
		t.Errorf("wrong current PINs counted: %+v", state)
	}
	if err := s.CheckPIN(ctx, alice, "7777"); !errors.Is(err, ErrWrongPIN) {
		t.Errorf("the PIN a refused change gave: %v", err)
	}
	if err := s.ChangePIN(ctx, alice, "7777", "4821"); err != nil {
		t.Fatal(err)
	}
	if state, _ := s.PIN(ctx, alice); state.WrongGuesses != 0 {
		t.Errorf("after a change: %+v", state)
	}
	if err := s.CheckPIN(ctx, alice, "4821"); !errors.Is(err, ErrWrongPIN) {
		t.Errorf("the old PIN: %v", err)
	}
	if err := s.RemoveOwnPIN(ctx, alice, "4821"); !errors.Is(err, ErrWrongPIN) {
		t.Errorf("a removal with a wrong current PIN: %v", err)
	}
	if err := s.RemoveOwnPIN(ctx, alice, "7777"); err != nil {
		t.Fatal(err)
	}
	if state, err := s.PIN(ctx, alice); err != nil || state.Set {
		t.Errorf("after the removal: %+v %v", state, err)
	}
	if err := s.RemoveOwnPIN(ctx, alice, ""); err != nil {
		t.Errorf("a removal without a PIN: %v", err)
	}
}

// Past the free wrong guesses each makes the next wait longer, up to
// MaxPINWait; while it waits even the right PIN is refused. A right PIN
// once the wait is over, or a PIN an administrator sets, starts the count
// again, which a store opened again keeps.
func TestWrongPINGuessesWaitLongerEachTime(t *testing.T) {
	t.Parallel()
	s, ids := newStore(t, "alice")
	ctx := t.Context()
	alice := ids[0]
	clock := time.Date(2026, 11, 26, 20, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return clock }
	if err := s.SetPIN(ctx, alice, "4821"); err != nil {
		t.Fatal(err)
	}
	for i := range FreePINGuesses {
		if err := s.CheckPIN(ctx, alice, "0000"); !errors.Is(err, ErrWrongPIN) {
			t.Fatalf("free guess %d: %v", i+1, err)
		}
	}
	if wait := waitOf(t, s.CheckPIN(ctx, alice, "0000")); wait != PINWaitStep {
		t.Errorf("first wait: %v", wait)
	}
	clock = clock.Add(10 * time.Second)
	if wait := waitOf(t, s.CheckPIN(ctx, alice, "4821")); wait != 20*time.Second {
		t.Errorf("the right PIN while waiting: %v", wait)
	}
	if state, _ := s.PIN(ctx, alice); state.WrongGuesses != FreePINGuesses+1 || state.WaitUntil == nil || !state.WaitUntil.Equal(clock.Add(20*time.Second)) {
		t.Errorf("waiting: %+v", state)
	}
	// Each wrong guess once the wait is over makes the next longer.
	var waits []time.Duration
	for range 40 {
		state, _ := s.PIN(ctx, alice)
		if state.WaitUntil != nil {
			clock = *state.WaitUntil
		}
		waits = append(waits, waitOf(t, s.CheckPIN(ctx, alice, "0000")))
	}
	if waits[0] != 2*PINWaitStep || waits[1] != 3*PINWaitStep || waits[len(waits)-1] != MaxPINWait {
		t.Errorf("waits: %v", waits)
	}
	// A store opened again, as after a restart, keeps the wait.
	again := New(s.db)
	again.now = s.now
	if err := again.CheckPIN(ctx, alice, "4821"); waitOf(t, err) != MaxPINWait {
		t.Errorf("after a restart: %v", err)
	}
	clock = clock.Add(MaxPINWait)
	if err := again.CheckPIN(ctx, alice, "4821"); err != nil {
		t.Fatalf("the right PIN after the wait: %v", err)
	}
	if state, _ := again.PIN(ctx, alice); state.WrongGuesses != 0 || state.WaitUntil != nil {
		t.Errorf("after the right PIN: %+v", state)
	}
	for range FreePINGuesses + 1 {
		_ = s.CheckPIN(ctx, alice, "0000")
	}
	if err := s.SetPIN(ctx, alice, "1357"); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckPIN(ctx, alice, "1357"); err != nil {
		t.Errorf("the PIN an administrator set: %v", err)
	}
	if err := s.SetPIN(ctx, alice, "13579"); !errors.Is(err, ErrInvalidPIN) {
		t.Errorf("an administrator's PIN of five digits: %v", err)
	}
	if err := s.RemovePIN(ctx, alice); err != nil {
		t.Fatal(err)
	}
	if state, _ := s.PIN(ctx, alice); state.Set {
		t.Errorf("removed by an administrator: %+v", state)
	}
}
