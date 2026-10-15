package accounts

import (
	"bytes"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
	"github.com/moodiness/polyfin/internal/totp"
)

// twoStepOn sets two-step sign-in up for user with a code at now, and
// returns the secret and the recovery codes.
func twoStepOn(t *testing.T, store *Store, user User, now time.Time) (string, []string) {
	t.Helper()
	setup, err := store.StartTwoStep(t.Context(), user)
	if err != nil {
		t.Fatal(err)
	}
	code, _ := totp.Code(setup.Secret, now)
	codes, err := store.ConfirmTwoStep(t.Context(), user.ID, code)
	if err != nil {
		t.Fatal(err)
	}
	return setup.Secret, codes
}

func TestTwoStepTurnsOnOnlyWithAConfirmedCode(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	now := time.Now()
	store.now = func() time.Time { return now }
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})

	if _, err := store.ConfirmTwoStep(ctx, alice.ID, "123456"); !errors.Is(err, ErrTwoStepNotStarted) {
		t.Errorf("confirmed without a setup: %v", err)
	}
	setup, err := store.StartTwoStep(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(setup.URI, "otpauth://totp/Polyfin:alice?") || !strings.Contains(setup.URI, "secret="+setup.Secret) {
		t.Errorf("address: %s", setup.URI)
	}
	if state, _ := store.TwoStep(ctx, alice.ID); state.On {
		t.Error("on before a code confirmed it")
	}
	if _, err := store.VerifyTwoStep(ctx, alice.ID, "123456"); !errors.Is(err, ErrTwoStepOff) {
		t.Errorf("a code asked before it is on: %v", err)
	}
	wrong, _ := totp.Code(setup.Secret, now.Add(-2*totp.Step))
	if _, err := store.ConfirmTwoStep(ctx, alice.ID, wrong); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("a code two steps old: %v", err)
	}
	if user, _ := store.User(ctx, alice.ID); user.TwoStep {
		t.Error("on after a wrong code")
	}
	right, _ := totp.Code(setup.Secret, now)
	codes, err := store.ConfirmTwoStep(ctx, alice.ID, right)
	if err != nil || len(codes) != RecoveryCodes {
		t.Fatalf("confirmed: %v %v", codes, err)
	}
	if user, _ := store.User(ctx, alice.ID); !user.TwoStep {
		t.Error("off after the code confirmed it")
	}
	if state, _ := store.TwoStep(ctx, alice.ID); !state.On || state.RecoveryCodes != RecoveryCodes || state.Unreadable {
		t.Errorf("state: %+v", state)
	}
	if _, err := store.StartTwoStep(ctx, alice); !errors.Is(err, ErrTwoStepOn) {
		t.Errorf("set up again while on: %v", err)
	}
	// The code that confirmed it is used.
	if _, err := store.VerifyTwoStep(ctx, alice.ID, right); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("the confirming code accepted again: %v", err)
	}
}

// Codes are accepted a step either way of now, not two, and each once.
func TestTwoStepCodesWithinDriftAndOnce(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	start := time.Unix(1_800_000_000, 0)
	now := start
	store.now = func() time.Time { return now }
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	secret, _ := twoStepOn(t, store, alice, start)

	now = start.Add(10 * totp.Step)
	code := func(steps int) string {
		c, _ := totp.Code(secret, now.Add(time.Duration(steps)*totp.Step))
		return c
	}
	for _, tc := range []struct {
		name  string
		steps int
		want  error
	}{
		{"two steps behind", -2, ErrInvalidCode},
		{"two steps ahead", 2, ErrInvalidCode},
		{"a step behind", -1, nil},
		{"the same again", -1, ErrInvalidCode},
		{"now", 0, nil},
		{"now again", 0, ErrInvalidCode},
		{"a step behind, after now", -1, ErrInvalidCode},
		{"a step ahead", 1, nil},
		{"a step ahead again", 1, ErrInvalidCode},
	} {
		if recovery, err := store.VerifyTwoStep(ctx, alice.ID, code(tc.steps)); !errors.Is(err, tc.want) || recovery {
			t.Errorf("%s: %v %v, want %v", tc.name, recovery, err, tc.want)
		}
	}
}

func TestRecoveryCodesAreSingleUseAndVoidedByNewOnes(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	_, codes := twoStepOn(t, store, alice, time.Now())
	// Four groups of four Crockford base32 characters: 80 random bits.
	format := regexp.MustCompile(`^[0-9a-hjkmnp-tv-z]{4}(-[0-9a-hjkmnp-tv-z]{4}){3}$`)
	seen := map[string]bool{}
	for _, code := range codes {
		if !format.MatchString(code) || seen[code] {
			t.Errorf("recovery code %q among %v", code, codes)
		}
		seen[code] = true
	}
	// Typed in capitals, with spaces for dashes, it is the same code.
	if recovery, err := store.VerifyTwoStep(ctx, alice.ID, " "+strings.ToUpper(strings.ReplaceAll(codes[0], "-", " "))+" "); err != nil || !recovery {
		t.Fatalf("recovery code: %v %v", recovery, err)
	}
	// And without them.
	if recovery, err := store.VerifyTwoStep(ctx, alice.ID, strings.ReplaceAll(codes[2], "-", "")); err != nil || !recovery {
		t.Fatalf("recovery code without dashes: %v %v", recovery, err)
	}
	if _, err := store.VerifyTwoStep(ctx, alice.ID, codes[0]); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("a recovery code used twice: %v", err)
	}
	if state, _ := store.TwoStep(ctx, alice.ID); state.RecoveryCodes != RecoveryCodes-2 {
		t.Errorf("codes left: %d", state.RecoveryCodes)
	}
	fresh, err := store.NewRecoveryCodes(ctx, alice.ID)
	if err != nil || len(fresh) != RecoveryCodes {
		t.Fatalf("new codes: %v %v", fresh, err)
	}
	if _, err := store.VerifyTwoStep(ctx, alice.ID, codes[1]); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("a former recovery code after new ones: %v", err)
	}
	if recovery, err := store.VerifyTwoStep(ctx, alice.ID, fresh[9]); err != nil || !recovery {
		t.Errorf("a new recovery code: %v %v", recovery, err)
	}
	bob := mustCreate(t, store, NewUser{Name: "bob", Password: "correct horse"})
	if _, err := store.NewRecoveryCodes(ctx, bob.ID); !errors.Is(err, ErrTwoStepOff) {
		t.Errorf("recovery codes without two-step sign-in: %v", err)
	}
}

// A password reset by PIN changes the password alone.
func TestPasswordResetPINKeepsTwoStep(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	_, codes := twoStepOn(t, store, alice, time.Now())
	_, pin, err := store.RequestPasswordReset(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	reset, err := store.RedeemPasswordResetPIN(ctx, pin.PIN)
	if err != nil || len(reset) != 1 || !reset[0].TwoStep {
		t.Fatalf("reset: %+v %v", reset, err)
	}
	user, err := store.Authenticate(ctx, "alice", pin.PIN)
	if err != nil || !user.NeedsTwoStep(store.Settings()) {
		t.Errorf("signed in with the PIN: %v %v", user.TwoStep, err)
	}
	if state, _ := store.TwoStep(ctx, alice.ID); !state.On || state.RecoveryCodes != RecoveryCodes {
		t.Errorf("two-step sign-in after the reset: %+v", state)
	}
	if recovery, err := store.VerifyTwoStep(ctx, alice.ID, codes[0]); err != nil || !recovery {
		t.Errorf("recovery code after the reset: %v", err)
	}
}

func TestNeedsTwoStep(t *testing.T) {
	required := Settings{TwoStepAdministrators: true}
	for _, tc := range []struct {
		user     User
		settings Settings
		want     bool
	}{
		{User{}, Settings{}, false},
		{User{}, required, false},
		{User{TwoStep: true}, Settings{}, true},
		{User{IsAdministrator: true}, Settings{}, false},
		{User{IsAdministrator: true}, required, true},
	} {
		if got := tc.user.NeedsTwoStep(tc.settings); got != tc.want {
			t.Errorf("%+v %v: %v", tc.user, tc.settings.TwoStepAdministrators, got)
		}
	}
}

// A secret sealed with another POLYFIN_SECRET_KEY, as after a restore,
// still lets its user in with a recovery code, and can be turned off. A
// secret stored without a key is sealed once one is set.
func TestTwoStepSecretsAreSealed(t *testing.T) {
	ctx := t.Context()
	pool := migrated.New(t)
	plain, err := Open(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	box, _ := secrets.New(bytes.Repeat([]byte{1}, secrets.KeySize))
	other, _ := secrets.New(bytes.Repeat([]byte{2}, secrets.KeySize))
	sealed, err := Open(ctx, pool, Sealing(box))
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Open(ctx, pool, Sealing(other))
	if err != nil {
		t.Fatal(err)
	}
	stored := func(user ID) string {
		var secret string
		if err := pool.QueryRow(ctx, "SELECT two_step_secret FROM users WHERE id = $1", user).Scan(&secret); err != nil {
			t.Fatal(err)
		}
		return secret
	}

	alice := mustCreate(t, sealed, NewUser{Name: "alice", Password: "correct horse"})
	secret, codes := twoStepOn(t, sealed, alice, time.Now())
	if value := stored(alice.ID); !secrets.Sealed(value) || strings.Contains(value, secret) {
		t.Errorf("stored: %q", value)
	}
	if state, _ := restored.TwoStep(ctx, alice.ID); !state.On || !state.Unreadable {
		t.Errorf("under another key: %+v", state)
	}
	code, _ := totp.Code(secret, time.Now().Add(totp.Step))
	if _, err := restored.VerifyTwoStep(ctx, alice.ID, code); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("a code under another key: %v", err)
	}
	if recovery, err := restored.VerifyTwoStep(ctx, alice.ID, codes[0]); err != nil || !recovery {
		t.Errorf("a recovery code under another key: %v %v", recovery, err)
	}
	report, err := other.Inspect(ctx, pool)
	if err != nil || len(report.Unreadable) != 1 || report.Unreadable[0] != (secrets.Unreadable{TwoStep: true, User: "alice"}) {
		t.Errorf("inspected: %+v %v", report, err)
	}
	if kind, count := other.UnreadableIn("users", map[string]string{"two_step": "t", "two_step_secret": stored(alice.ID)}); kind != secrets.KindTwoStep || count != 1 {
		t.Errorf("in a backup: %s %d", kind, count)
	}
	if kind, count := other.UnreadableIn("users", map[string]string{"two_step": "f", "two_step_secret": stored(alice.ID)}); kind != secrets.KindTwoStep || count != 0 {
		t.Errorf("a setup in a backup: %s %d", kind, count)
	}
	if user, err := restored.TurnOffTwoStep(ctx, alice.ID); err != nil || user.TwoStep || stored(alice.ID) != "" {
		t.Errorf("turned off: %v %v", user.TwoStep, err)
	}

	bob := mustCreate(t, plain, NewUser{Name: "bob", Password: "correct horse"})
	secret, _ = twoStepOn(t, plain, bob, time.Now())
	if stored(bob.ID) != secret {
		t.Errorf("stored without a key: %q", stored(bob.ID))
	}
	if count, err := box.SealStored(ctx, pool); err != nil || count != 1 || !secrets.Sealed(stored(bob.ID)) {
		t.Errorf("sealed %d: %v", count, err)
	}
	code, _ = totp.Code(secret, time.Now().Add(totp.Step))
	if recovery, err := sealed.VerifyTwoStep(ctx, bob.ID, code); err != nil || recovery {
		t.Errorf("a code once sealed: %v", err)
	}
}

func TestCheckPassword(t *testing.T) {
	store := newStore(t)
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	for password, want := range map[string]bool{"correct horse": true, "wrong horse": false, strings.Repeat("a", 300): false} {
		if got, err := store.CheckPassword(t.Context(), alice.ID, password); err != nil || got != want {
			t.Errorf("%.20s: %v %v", password, got, err)
		}
	}
	if _, err := store.CheckPassword(t.Context(), ID{1}, "correct horse"); !errors.Is(err, ErrNotFound) {
		t.Errorf("no such user: %v", err)
	}
}
