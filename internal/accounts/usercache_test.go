package accounts

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// tableReads counts the statements that read the users table and the
// devices table.
type tableReads struct{ users, devices atomic.Int64 }

var (
	readsUsers   = regexp.MustCompile(`(?i)\b(from|join)\s+users\b`)
	readsDevices = regexp.MustCompile(`(?i)\b(from|join)\s+devices\b`)
)

func (c *tableReads) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if readsUsers.MatchString(data.SQL) {
		c.users.Add(1)
	}
	if readsDevices.MatchString(data.SQL) {
		c.devices.Add(1)
	}
	return ctx
}

func (c *tableReads) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// tracedStore is a store whose statements tracer sees.
func tracedStore(t *testing.T, tracer pgx.QueryTracer) *Store {
	t.Helper()
	config := migrated.New(t).Config()
	config.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	store, err := Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// countedStore is a store whose reads of users and devices are counted.
func countedStore(t *testing.T) (*Store, *tableReads) {
	t.Helper()
	reads := &tableReads{}
	return tracedStore(t, reads), reads
}

// heldRead holds the next read of a user by identifier, once it has the
// row, until release is closed; read is closed once it has the row.
type heldRead struct {
	armed         atomic.Bool
	read, release chan struct{}
}

type heldKey struct{}

func (h *heldRead) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.HasSuffix(data.SQL, " FROM users WHERE id = $1") && h.armed.CompareAndSwap(true, false) {
		return context.WithValue(ctx, heldKey{}, true)
	}
	return ctx
}

func (h *heldRead) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if ctx.Value(heldKey{}) != nil {
		close(h.read)
		<-h.release
	}
}

// A user read before a change and kept after it is not remembered: the
// next request reads the change.
func TestAUserReadBeforeAChangeIsNotKept(t *testing.T) {
	t.Parallel()
	held := &heldRead{read: make(chan struct{}), release: make(chan struct{})}
	store := tracedStore(t, held)
	ctx := t.Context()
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	held.armed.Store(true)
	before := make(chan User, 1)
	go func() {
		user, err := store.User(ctx, alice.ID)
		if err != nil {
			t.Error(err)
		}
		before <- user
	}()
	select {
	case <-held.read:
	case <-time.After(10 * time.Second):
		t.Fatal("the user was not read")
	}
	_, err := store.UpdateUser(ctx, alice.ID, UserChanges{IsDisabled: new(true)}, nil)
	close(held.release)
	if err != nil {
		t.Fatal(err)
	}
	if user := <-before; user.IsDisabled {
		t.Fatal("the first read saw the change")
	}
	if user, err := store.User(ctx, alice.ID); err != nil || !user.IsDisabled {
		t.Errorf("read after the change: %+v %v", user, err)
	}
}

// Once a user, their device and their admin session were read, the
// requests that look them up again read no users row and no devices row:
// streams, HLS segments and admin requests run these on every request.
func TestWarmUsersAndDevicesAreNotReadAgain(t *testing.T) {
	t.Parallel()
	store, reads := countedStore(t)
	ctx := t.Context()
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	token, signedIn, err := store.SignInDevice(ctx, alice.ID, device("tv"))
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := store.CreateAdminSession(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.DeviceByToken(ctx, token, "192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AdminSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	reads.users.Store(0)
	reads.devices.Store(0)
	for range 3 {
		if user, err := store.User(ctx, alice.ID); err != nil || user.Name != "alice" {
			t.Fatalf("user: %+v %v", user, err)
		}
		if _, _, err := store.DeviceByToken(ctx, token, "192.0.2.1"); err != nil {
			t.Fatal(err)
		}
		if got, err := store.AdminSession(ctx, session); err != nil || got.User.ID != alice.ID {
			t.Fatalf("admin session: %+v %v", got, err)
		}
		if got, err := store.Device(ctx, signedIn.ID); err != nil || got.ID != signedIn.ID || got.DeviceName != "TV" {
			t.Fatalf("device: %+v %v", got, err)
		}
	}
	if users, devices := reads.users.Load(), reads.devices.Load(); users != 0 || devices != 0 {
		t.Errorf("warm lookups read users %d times and devices %d times", users, devices)
	}
}

// signedInEverywhere is alice signed in on two devices and the admin
// interface, each looked up once, so that what the store remembers of her
// is what a change must replace.
type signedInEverywhere struct {
	store        *Store
	alice        User
	tv, phone    string
	tvDevice     Device
	adminSession string
}

func signInEverywhere(t *testing.T) signedInEverywhere {
	t.Helper()
	store := newStore(t)
	ctx := t.Context()
	s := signedInEverywhere{store: store, alice: mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})}
	var err error
	if s.tv, s.tvDevice, err = store.SignInDevice(ctx, s.alice.ID, device("tv")); err != nil {
		t.Fatal(err)
	}
	if s.phone, _, err = store.SignInDevice(ctx, s.alice.ID, device("phone")); err != nil {
		t.Fatal(err)
	}
	if s.adminSession, _, err = store.CreateAdminSession(ctx, s.alice.ID); err != nil {
		t.Fatal(err)
	}
	s.lookUp(t)
	return s
}

// lookUp reads alice as each request does.
func (s signedInEverywhere) lookUp(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	if _, err := s.store.User(ctx, s.alice.ID); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{s.tv, s.phone} {
		if !signedIn(ctx, s.store, token) {
			t.Fatal("a device is not signed in")
		}
	}
	if _, err := s.store.AdminSession(ctx, s.adminSession); err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.Device(ctx, s.tvDevice.ID); err != nil {
		t.Fatal(err)
	}
}

// Every change to a user applies from their next request, whatever was
// read of them before: disabling or deleting them, a new password, a new
// permission, their group's values, a wrong password.
func TestChangesToAUserApplyAtTheNextRequest(t *testing.T) {
	t.Parallel()
	t.Run("disabled", func(t *testing.T) {
		t.Parallel()
		s := signInEverywhere(t)
		ctx := t.Context()
		if _, err := s.store.UpdateUser(ctx, s.alice.ID, UserChanges{IsDisabled: new(true)}, nil); err != nil {
			t.Fatal(err)
		}
		if user, err := s.store.User(ctx, s.alice.ID); err != nil || !user.IsDisabled {
			t.Errorf("user: %+v %v", user, err)
		}
		if signedIn(ctx, s.store, s.tv) || signedIn(ctx, s.store, s.phone) {
			t.Error("a device of a disabled user is signed in")
		}
		if _, err := s.store.AdminSession(ctx, s.adminSession); !errors.Is(err, ErrNotFound) {
			t.Errorf("admin session of a disabled user: %v", err)
		}
	})
	t.Run("deleted", func(t *testing.T) {
		t.Parallel()
		s := signInEverywhere(t)
		ctx := t.Context()
		if err := s.store.DeleteUser(ctx, s.alice.ID); err != nil {
			t.Fatal(err)
		}
		if user, err := s.store.User(ctx, s.alice.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("user: %+v %v", user, err)
		}
		if signedIn(ctx, s.store, s.tv) || signedIn(ctx, s.store, s.phone) {
			t.Error("a device of a deleted user is signed in")
		}
		if _, err := s.store.Device(ctx, s.tvDevice.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("device of a deleted user: %v", err)
		}
		if _, err := s.store.AdminSession(ctx, s.adminSession); !errors.Is(err, ErrNotFound) {
			t.Errorf("admin session of a deleted user: %v", err)
		}
	})
	t.Run("password", func(t *testing.T) {
		t.Parallel()
		s := signInEverywhere(t)
		ctx := t.Context()
		if err := s.store.ChangePasswordFromDevice(ctx, s.alice.ID, "correct horse", "battery staple", s.tvDevice.ID); err != nil {
			t.Fatal(err)
		}
		if !signedIn(ctx, s.store, s.tv) {
			t.Error("the device that changed the password was signed out")
		}
		if signedIn(ctx, s.store, s.phone) {
			t.Error("another device is still signed in")
		}
		if _, err := s.store.AdminSession(ctx, s.adminSession); !errors.Is(err, ErrNotFound) {
			t.Errorf("admin session after a new password: %v", err)
		}
		if _, err := s.store.Authenticate(ctx, "alice", "correct horse"); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("the former password: %v", err)
		}
	})
	t.Run("permission", func(t *testing.T) {
		t.Parallel()
		s := signInEverywhere(t)
		ctx := t.Context()
		if _, err := s.store.UpdateUser(ctx, s.alice.ID, UserChanges{LiveTv: new(false), MaxBitrate: new(8_000_000)}, nil); err != nil {
			t.Fatal(err)
		}
		if user, err := s.store.User(ctx, s.alice.ID); err != nil || user.LiveTv || user.MaxBitrate != 8_000_000 {
			t.Errorf("user: %+v %v", user, err)
		}
		if _, user, err := s.store.DeviceByToken(ctx, s.tv, "192.0.2.1"); err != nil || user.LiveTv || user.MaxBitrate != 8_000_000 {
			t.Errorf("device's user: %+v %v", user, err)
		}
		if session, err := s.store.AdminSession(ctx, s.adminSession); err != nil || session.User.LiveTv || session.User.MaxBitrate != 8_000_000 {
			t.Errorf("admin session's user: %+v %v", session.User, err)
		}
	})
	t.Run("group", func(t *testing.T) {
		t.Parallel()
		s := signInEverywhere(t)
		ctx := t.Context()
		group := kidsGroup(t, s.store)
		if _, err := s.store.AddGroupMembers(ctx, group.ID, []ID{s.alice.ID}); err != nil {
			t.Fatal(err)
		}
		if user, err := s.store.User(ctx, s.alice.ID); err != nil || user.GroupID == nil || *user.GroupID != group.ID {
			t.Errorf("member: %+v %v", user, err)
		}
		s.lookUp(t)
		if _, err := s.store.UpdateGroup(ctx, group.ID, GroupChanges{Limits: UserChanges{MaxBitrate: new(4_000_000)}}); err != nil {
			t.Fatal(err)
		}
		if user, err := s.store.User(ctx, s.alice.ID); err != nil || user.MaxBitrate != 4_000_000 {
			t.Errorf("after the group's change: %+v %v", user, err)
		}
		if session, err := s.store.AdminSession(ctx, s.adminSession); err != nil || session.User.MaxBitrate != 4_000_000 {
			t.Errorf("admin session's user after the group's change: %+v %v", session.User, err)
		}
		s.lookUp(t)
		if _, err := s.store.DeleteGroup(ctx, group.ID); err != nil {
			t.Fatal(err)
		}
		if user, err := s.store.User(ctx, s.alice.ID); err != nil || user.GroupID != nil {
			t.Errorf("after the group went: %+v %v", user, err)
		}
	})
	t.Run("wrong password", func(t *testing.T) {
		t.Parallel()
		s := signInEverywhere(t)
		ctx := t.Context()
		settings := s.store.Settings()
		settings.LoginAttempts = MinLoginAttempts
		if _, err := s.store.UpdateSettings(ctx, settings); err != nil {
			t.Fatal(err)
		}
		s.lookUp(t)
		if _, err := s.store.Authenticate(ctx, "alice", "wrong password"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatal(err)
		}
		if user, err := s.store.User(ctx, s.alice.ID); err != nil || user.InvalidLoginAttempts != 1 {
			t.Errorf("user: %+v %v", user, err)
		}
	})
}

// A user read while a change is stored is never remembered as it was
// before the change: once the change returns, every read sees it.
func TestUsersReadDuringAChangeAreNotKeptStale(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	stop := make(chan struct{})
	var readers sync.WaitGroup
	for range 4 {
		readers.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := store.User(ctx, alice.ID); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	defer func() {
		close(stop)
		readers.Wait()
	}()
	for i := range 20 {
		bitrate := 1_000_000 * (i + 1)
		if _, err := store.UpdateUser(ctx, alice.ID, UserChanges{MaxBitrate: &bitrate}, nil); err != nil {
			t.Fatal(err)
		}
		if user, err := store.User(ctx, alice.ID); err != nil || user.MaxBitrate != bitrate {
			t.Fatalf("after change %d: %d %v", i, user.MaxBitrate, err)
		}
	}
}

// A change made behind the server's back, by hand in the database, is seen
// once the user read lately is past signInLife.
func TestUsersAreKeptBriefly(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	now := time.Now()
	store.now = func() time.Time { return now }
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	if user, err := store.User(ctx, alice.ID); err != nil || user.Name != "alice" {
		t.Fatalf("first read: %v %v", user.Name, err)
	}
	if _, err := store.db.Exec(ctx, "UPDATE users SET is_disabled = true WHERE id = $1", alice.ID); err != nil {
		t.Fatal(err)
	}
	now = now.Add(signInLife - time.Second)
	if user, err := store.User(ctx, alice.ID); err != nil || user.IsDisabled {
		t.Errorf("read within the life: %+v %v", user, err)
	}
	now = now.Add(time.Second)
	if user, err := store.User(ctx, alice.ID); err != nil || !user.IsDisabled {
		t.Errorf("read past the life: %+v %v", user, err)
	}
}
