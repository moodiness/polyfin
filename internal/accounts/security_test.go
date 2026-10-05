package accounts

import (
	"bytes"
	"errors"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestSecuritySettingsRoundTripAndStayInRange(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	if got := store.Settings(); !got.PersonalAddons || got.LoginAttempts != 0 || got.InactiveDeviceDays != 0 || got.DetailedLog {
		t.Errorf("defaults: %+v", got)
	}
	if DefaultLoginAttempts != 0 || DefaultInactiveDeviceDays != 0 || LoginBlock != 15*time.Minute {
		t.Errorf("defaults: %d attempts, %d days, block of %v", DefaultLoginAttempts, DefaultInactiveDeviceDays, LoginBlock)
	}
	for _, tc := range []struct {
		attempts, days int
		err            error
	}{
		{1, 0, ErrInvalidLoginAttempts},
		{MinLoginAttempts - 1, 0, ErrInvalidLoginAttempts},
		{MaxLoginAttempts + 1, 0, ErrInvalidLoginAttempts},
		{-1, 0, ErrInvalidLoginAttempts},
		{0, -1, ErrInvalidInactiveDeviceDays},
		{0, MaxInactiveDeviceDays + 1, ErrInvalidInactiveDeviceDays},
	} {
		changed := store.Settings()
		changed.LoginAttempts, changed.InactiveDeviceDays = tc.attempts, tc.days
		if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, tc.err) {
			t.Errorf("%d attempts, %d days: got %v, want %v", tc.attempts, tc.days, err, tc.err)
		}
	}
	if got := store.Settings(); got.LoginAttempts != 0 || got.InactiveDeviceDays != 0 {
		t.Errorf("a refused update changed the settings: %+v", got)
	}
	for _, tc := range []struct {
		attempts, days int
		personal, log  bool
	}{
		{MinLoginAttempts, 1, false, true},
		{MaxLoginAttempts, MaxInactiveDeviceDays, true, false},
		{0, 0, false, true},
	} {
		changed := store.Settings()
		changed.LoginAttempts, changed.InactiveDeviceDays, changed.PersonalAddons, changed.DetailedLog = tc.attempts, tc.days, tc.personal, tc.log
		if _, err := store.UpdateSettings(ctx, changed); err != nil {
			t.Fatalf("%+v: %v", tc, err)
		}
		reopened, err := Open(ctx, store.db)
		if err != nil {
			t.Fatal(err)
		}
		if got := reopened.Settings(); !reflect.DeepEqual(got, changed) {
			t.Errorf("after reopening: %+v, want %+v", got, changed)
		}
	}
}

// clockAt sets the store's clock, which blocks and unused devices are
// measured by, and returns a function moving it on.
func clockAt(store *Store, now time.Time) func(time.Duration) {
	store.now = func() time.Time { return now }
	return func(d time.Duration) {
		now = now.Add(d)
		store.now = func() time.Time { return now }
	}
}

func setLoginAttempts(t *testing.T, store *Store, attempts int) {
	t.Helper()
	settings := store.Settings()
	settings.LoginAttempts = attempts
	if _, err := store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
}

func TestWrongPasswordsBlockTheAccountForAWhile(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	start := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	advance := clockAt(store, start)
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	mustCreate(t, store, NewUser{Name: "bob", Password: "correct horse"})
	wrong := func(times int) {
		t.Helper()
		for range times {
			if _, err := store.Authenticate(ctx, "alice", "wrong password"); !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("wrong password: %v", err)
			}
		}
	}
	signsIn := func(password string) bool {
		t.Helper()
		_, err := store.Authenticate(ctx, "alice", password)
		if err != nil && !errors.Is(err, ErrInvalidCredentials) {
			t.Fatal(err)
		}
		return err == nil
	}
	state := func() User {
		t.Helper()
		user, err := store.User(ctx, alice.ID)
		if err != nil {
			t.Fatal(err)
		}
		return user
	}

	// Without a limit, as by default, nothing blocks nor counts.
	wrong(MaxLoginAttempts + 1)
	if user := state(); !signsIn("correct horse") || user.InvalidLoginAttempts != 0 || user.BlockedUntil != nil {
		t.Fatalf("without a limit: %d attempts, blocked until %v", user.InvalidLoginAttempts, user.BlockedUntil)
	}

	setLoginAttempts(t, store, 3)
	// A sign-in starts the count again.
	wrong(2)
	if user := state(); user.InvalidLoginAttempts != 2 || user.Blocked(start) {
		t.Fatalf("after 2 wrong passwords: %+v", user)
	}
	if !signsIn("correct horse") || state().InvalidLoginAttempts != 0 {
		t.Fatalf("a sign-in did not start the count again: %d", state().InvalidLoginAttempts)
	}
	wrong(2)
	if !signsIn("correct horse") {
		t.Fatal("refused before the limit")
	}

	// The third wrong password in a row blocks the account for 15 minutes,
	// the right password included, with the answer of a wrong one.
	wrong(3)
	if user := state(); user.InvalidLoginAttempts != 3 || user.BlockedUntil == nil || !user.BlockedUntil.Equal(start.Add(LoginBlock)) {
		t.Fatalf("after 3 wrong passwords: %d attempts, blocked until %v", user.InvalidLoginAttempts, user.BlockedUntil)
	}
	if signsIn("correct horse") {
		t.Fatal("a blocked account signed in")
	}
	if _, err := store.Authenticate(ctx, "bob", "correct horse"); err != nil {
		t.Errorf("another account: %v", err)
	}
	advance(LoginBlock - time.Second)
	if signsIn("correct horse") {
		t.Fatal("signed in before the block ended")
	}
	advance(time.Second)
	if !signsIn("correct horse") || state().InvalidLoginAttempts != 0 || state().BlockedUntil != nil {
		t.Fatalf("after the block: %+v", state())
	}
	// After a block, the count starts from the first wrong password.
	wrong(3)
	advance(LoginBlock)
	wrong(1)
	if user := state(); user.InvalidLoginAttempts != 1 || user.Blocked(start.Add(3*LoginBlock)) {
		t.Fatalf("first wrong password after a block: %+v", user)
	}
	if !signsIn("correct horse") {
		t.Fatal("refused after a block ended")
	}

	// An administrator's new password ends a block.
	wrong(3)
	if _, err := store.UpdateUser(ctx, alice.ID, UserChanges{Password: new("battery staple")}, nil); err != nil {
		t.Fatal(err)
	}
	if user := state(); user.InvalidLoginAttempts != 0 || user.BlockedUntil != nil || !signsIn("battery staple") {
		t.Fatalf("after a new password: %+v", user)
	}
	// So does unblocking.
	wrong(3)
	if signsIn("battery staple") {
		t.Fatal("not blocked again")
	}
	unblocked, err := store.Unblock(ctx, alice.ID)
	if err != nil || unblocked.InvalidLoginAttempts != 0 || unblocked.BlockedUntil != nil || !signsIn("battery staple") {
		t.Fatalf("after unblocking: %+v %v", unblocked, err)
	}
	if _, err := store.Unblock(ctx, ID{1}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unblocking an unknown user: %v", err)
	}
	// And turning the limit off.
	wrong(3)
	setLoginAttempts(t, store, 0)
	if user := state(); user.BlockedUntil != nil || user.InvalidLoginAttempts != 0 || !signsIn("battery staple") {
		t.Fatalf("after turning the limit off: %+v", user)
	}
}

// Wrong passwords sent at once all count.
func TestWrongPasswordsSentAtOnceAllCount(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	setLoginAttempts(t, store, 5)
	done := make(chan error)
	for range 5 {
		go func() {
			_, err := store.Authenticate(ctx, "alice", "wrong password")
			done <- err
		}()
	}
	for range 5 {
		if err := <-done; !errors.Is(err, ErrInvalidCredentials) {
			t.Fatal(err)
		}
	}
	if user, _ := store.User(ctx, alice.ID); user.InvalidLoginAttempts != 5 || !user.Blocked(time.Now()) {
		t.Errorf("after 5 wrong passwords at once: %+v", user)
	}
}

func TestUnusedDevicesAreSignedOut(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	told := make(chan []ID, 10)
	store.OnSignOut(func(devices []ID) { told <- devices })
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	tvToken, _, _ := store.SignInDevice(ctx, alice.ID, device("tv"))
	phoneToken, phone, _ := store.SignInDevice(ctx, alice.ID, device("phone"))
	tabletToken, tablet, _ := store.SignInDevice(ctx, alice.ID, device("tablet"))
	adminToken, _, err := store.CreateAdminSession(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	age := func(device ID, days int) {
		t.Helper()
		if _, err := store.db.Exec(ctx, "UPDATE devices SET last_activity_at = now() - make_interval(days => $2) WHERE id = $1", device, days); err != nil {
			t.Fatal(err)
		}
	}
	age(phone.ID, 31)
	age(tablet.ID, 29)

	// By default, devices stay signed in however long unused.
	if signedOut, err := store.SignOutInactiveDevices(ctx); err != nil || signedOut != 0 || !signedIn(ctx, store, phoneToken) {
		t.Fatalf("by default: %d signed out, %v", signedOut, err)
	}
	// The DeviceByToken above used the phone: age it again.
	age(phone.ID, 31)

	settings := store.Settings()
	settings.InactiveDeviceDays = 30
	if _, err := store.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if err := store.SweepInactiveDevices(ctx, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	select {
	case devices := <-told:
		if !slices.Equal(devices, []ID{phone.ID}) {
			t.Errorf("signed out %v, want the phone %v", devices, phone.ID)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the sweep told of no device")
	}
	if signedIn(ctx, store, phoneToken) || !signedIn(ctx, store, tvToken) || !signedIn(ctx, store, tabletToken) {
		t.Error("the sweep signed out the wrong devices")
	}
	// Admin interface sessions are not devices.
	if _, err := store.AdminSession(ctx, adminToken); err != nil {
		t.Errorf("admin session: %v", err)
	}
	// Two days later, the tablet, last used 29 days ago, is unused for 31
	// days; the TV was just used.
	age(tablet.ID, 29)
	clockAt(store, time.Now().AddDate(0, 0, 2))
	if signedOut, err := store.SignOutInactiveDevices(ctx); err != nil || signedOut != 1 || signedIn(ctx, store, tabletToken) || !signedIn(ctx, store, tvToken) {
		t.Fatalf("two days later: %d signed out, %v", signedOut, err)
	}
	if devices := <-told; !slices.Equal(devices, []ID{tablet.ID}) {
		t.Errorf("told of %v, want the tablet", devices)
	}
}

func TestDetailedLogAppliesAtOnce(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	var out bytes.Buffer
	level := new(slog.LevelVar)
	logger := slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: level}))
	store.FollowLogLevel(level, slog.LevelWarn)
	logged := func(message string) bool {
		t.Helper()
		out.Reset()
		logger.Debug(message)
		return strings.Contains(out.String(), message)
	}
	if level.Level() != slog.LevelWarn || logged("off by default") {
		t.Fatalf("by default: level %v", level.Level())
	}
	settings := store.Settings()
	settings.DetailedLog = true
	if _, err := store.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if !logged("turned on") {
		t.Error("no debug record once the detailed log is on")
	}
	// A server started with the detailed log on logs in detail from the start.
	reopened, err := Open(ctx, store.db)
	if err != nil {
		t.Fatal(err)
	}
	restarted := new(slog.LevelVar)
	reopened.FollowLogLevel(restarted, slog.LevelInfo)
	if restarted.Level() != slog.LevelDebug {
		t.Errorf("reopened with the detailed log on: level %v", restarted.Level())
	}
	settings.DetailedLog = false
	if _, err := store.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if logged("turned off") || level.Level() != slog.LevelWarn {
		t.Errorf("turned off: level %v", level.Level())
	}
}
