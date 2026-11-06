package accounts

import (
	"errors"
	"testing"
	"time"
)

// lastActivity returns the user's last activity as stored.
func lastActivity(t *testing.T, store *Store, user ID) *time.Time {
	t.Helper()
	var at *time.Time
	if err := store.db.QueryRow(t.Context(), "SELECT last_activity_at FROM users WHERE id = $1", user).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at
}

func TestAdminAppUseIsActivityOnceAMinute(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	now := time.Now()
	store.now = func() time.Time { return now }
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	token, _, err := store.CreateAdminSession(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	signedInAt := lastActivity(t, store, alice.ID)
	if signedInAt == nil {
		t.Fatal("no activity at sign-in")
	}
	start := *signedInAt

	// Within a minute of the sign-in, nothing is written.
	now = start.Add(20 * time.Second)
	if _, err := store.AdminSession(ctx, token); err != nil {
		t.Fatal(err)
	}
	if got := lastActivity(t, store, alice.ID); !got.Equal(start) {
		t.Errorf("activity written within a minute: %v, then %v", start, got)
	}

	// A minute later, the use is written and answered.
	now = start.Add(activityResolution + time.Second)
	session, err := store.AdminSession(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if got := lastActivity(t, store, alice.ID); got == nil || !got.Equal(now.Truncate(time.Microsecond)) {
		t.Errorf("activity after a minute: %v, want %v", got, now)
	}
	if got := session.User.LastActivityAt; got == nil || !got.Equal(now) {
		t.Errorf("activity answered: %v, want %v", got, now)
	}

	// Not again within the next minute.
	written := now
	now = now.Add(30 * time.Second)
	if _, err := store.AdminSession(ctx, token); err != nil {
		t.Fatal(err)
	}
	if got := lastActivity(t, store, alice.ID); got == nil || !got.Equal(written.Truncate(time.Microsecond)) {
		t.Errorf("activity written again within a minute: %v", got)
	}

	// A user never seen gets their first activity.
	if _, err := store.db.Exec(ctx, "UPDATE users SET last_activity_at = NULL WHERE id = $1", alice.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AdminSession(ctx, token); err != nil {
		t.Fatal(err)
	}
	if got := lastActivity(t, store, alice.ID); got == nil || !got.Equal(now.Truncate(time.Microsecond)) {
		t.Errorf("first activity: %v, want %v", got, now)
	}
}

func TestActivityNeverMovesBackwards(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	ahead := func() time.Time {
		t.Helper()
		var at time.Time
		if err := store.db.QueryRow(ctx, `UPDATE users SET last_activity_at = date_trunc('second', now()) + interval '1 hour'
			WHERE id = $1 RETURNING last_activity_at`, alice.ID).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at
	}

	// Sign-ins, of the admin app and of a device.
	future := ahead()
	sessionToken, _, err := store.CreateAdminSession(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := lastActivity(t, store, alice.ID); got == nil || !got.Equal(future) {
		t.Errorf("admin sign-in moved activity from %v to %v", future, got)
	}
	future = ahead()
	deviceToken, signedIn, err := store.SignInDevice(ctx, alice.ID, device("tv"))
	if err != nil {
		t.Fatal(err)
	}
	if got := lastActivity(t, store, alice.ID); got == nil || !got.Equal(future) {
		t.Errorf("device sign-in moved activity from %v to %v", future, got)
	}

	// A device's request on a clock behind the database's: the new address
	// is written, the activity times stay.
	store.now = func() time.Time { return time.Now().Add(-time.Hour) }
	var deviceAt time.Time
	if err := store.db.QueryRow(ctx, "SELECT last_activity_at FROM devices WHERE id = $1", signedIn.ID).Scan(&deviceAt); err != nil {
		t.Fatal(err)
	}
	got, user, err := store.DeviceByToken(ctx, deviceToken, "198.51.100.7")
	if err != nil {
		t.Fatal(err)
	}
	var storedDevice time.Time
	var address string
	if err := store.db.QueryRow(ctx, "SELECT last_activity_at, remote_address FROM devices WHERE id = $1", signedIn.ID).Scan(&storedDevice, &address); err != nil {
		t.Fatal(err)
	}
	if !storedDevice.Equal(deviceAt) || address != "198.51.100.7" {
		t.Errorf("device stored at %v from %q, want %v from the new address", storedDevice, address, deviceAt)
	}
	if !got.LastActivityAt.Equal(deviceAt) {
		t.Errorf("device answered at %v, want %v", got.LastActivityAt, deviceAt)
	}
	if stored := lastActivity(t, store, alice.ID); stored == nil || !stored.Equal(future) {
		t.Errorf("device request moved activity from %v to %v", future, stored)
	}
	if user.LastActivityAt == nil || !user.LastActivityAt.Equal(future) {
		t.Errorf("user answered at %v, want %v", user.LastActivityAt, future)
	}

	// The admin app on that clock writes nothing.
	if _, err := store.AdminSession(ctx, sessionToken); err != nil {
		t.Fatal(err)
	}
	if stored := lastActivity(t, store, alice.ID); stored == nil || !stored.Equal(future) {
		t.Errorf("admin app moved activity from %v to %v", future, stored)
	}
}

func TestRefusedAdminSessionsRecordNothing(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	store.now = time.Now
	for _, refuse := range []string{
		"UPDATE users SET is_disabled = true WHERE id = $1",
		"UPDATE users SET ends_on = current_date - 1 WHERE id = $1",
	} {
		alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
		token, _, err := store.CreateAdminSession(ctx, alice.ID)
		if err != nil {
			t.Fatal(err)
		}
		var before time.Time
		if err := store.db.QueryRow(ctx, `UPDATE users SET last_activity_at = date_trunc('second', now()) - interval '2 days'
			WHERE id = $1 RETURNING last_activity_at`, alice.ID).Scan(&before); err != nil {
			t.Fatal(err)
		}
		if _, err := store.db.Exec(ctx, refuse, alice.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := store.AdminSession(ctx, token); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: session answered %v", refuse, err)
		}
		if got := lastActivity(t, store, alice.ID); got == nil || !got.Equal(before) {
			t.Errorf("%s: activity moved from %v to %v", refuse, before, got)
		}
		if _, err := store.db.Exec(ctx, "DELETE FROM users WHERE id = $1", alice.ID); err != nil {
			t.Fatal(err)
		}
	}
}
