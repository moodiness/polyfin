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

	// A user never seen gets their first activity: the change, made in the
	// database, is forgotten as one the server makes.
	if _, err := store.db.Exec(ctx, "UPDATE users SET last_activity_at = NULL WHERE id = $1", alice.ID); err != nil {
		t.Fatal(err)
	}
	store.forgetSignIns()
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

// lastActivityApp returns the app of the user's last activity as stored.
func lastActivityApp(t *testing.T, store *Store, user ID) string {
	t.Helper()
	var app string
	if err := store.db.QueryRow(t.Context(), "SELECT last_activity_app FROM users WHERE id = $1", user).Scan(&app); err != nil {
		t.Fatal(err)
	}
	return app
}

func TestActivityIsWrittenWithItsApp(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	now := time.Now()
	store.now = func() time.Time { return now }
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	if got := lastActivityApp(t, store, alice.ID); got != "" {
		t.Errorf("app of a user never active: %q", got)
	}

	// A device's sign-in, then its use a minute later, answered from the
	// token remembered.
	tv := device("tv")
	tv.Client = "Jellyfin Android TV"
	tvToken, _, err := store.SignInDevice(ctx, alice.ID, tv)
	if err != nil {
		t.Fatal(err)
	}
	if got := lastActivityApp(t, store, alice.ID); got != "Jellyfin Android TV" {
		t.Errorf("app after the device's sign-in: %q", got)
	}
	if _, err := store.db.Exec(ctx, "UPDATE users SET last_activity_app = 'stale', last_activity_at = now() - interval '1 hour' WHERE id = $1", alice.ID); err != nil {
		t.Fatal(err)
	}
	now = time.Now().Add(activityResolution + time.Second)
	_, user, err := store.DeviceByToken(ctx, tvToken, "192.0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	if got := lastActivityApp(t, store, alice.ID); got != "Jellyfin Android TV" {
		t.Errorf("app after the device's use: %q", got)
	}
	if user.LastActivityApp != "Jellyfin Android TV" || user.LastActivityAt == nil || !user.LastActivityAt.Equal(now) {
		t.Errorf("activity answered: %v from %q", user.LastActivityAt, user.LastActivityApp)
	}
	// The token remembered answers the same, and so after a write it made.
	now = now.Add(time.Second)
	if _, user, err = store.DeviceByToken(ctx, tvToken, "198.51.100.7"); err != nil {
		t.Fatal(err)
	}
	if user.LastActivityApp != "Jellyfin Android TV" || user.LastActivityAt == nil || !user.LastActivityAt.Equal(now) {
		t.Errorf("remembered activity: %v from %q", user.LastActivityAt, user.LastActivityApp)
	}
	if _, user, err = store.DeviceByToken(ctx, tvToken, "198.51.100.7"); err != nil {
		t.Fatal(err)
	}
	if user.LastActivityApp != "Jellyfin Android TV" || user.LastActivityAt == nil || !user.LastActivityAt.Equal(now) {
		t.Errorf("remembered activity again: %v from %q", user.LastActivityAt, user.LastActivityApp)
	}
}

func TestAdminAppActivityIsTheAdminApps(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	now := time.Now()
	store.now = func() time.Time { return now }
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	token, _, err := store.CreateAdminSession(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := lastActivityApp(t, store, alice.ID); got != AdminApp {
		t.Errorf("app after the admin app's sign-in: %q", got)
	}
	if _, err := store.db.Exec(ctx, "UPDATE users SET last_activity_app = 'Jellyfin Web', last_activity_at = now() - interval '1 hour' WHERE id = $1", alice.ID); err != nil {
		t.Fatal(err)
	}
	session, err := store.AdminSession(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if got := lastActivityApp(t, store, alice.ID); got != AdminApp {
		t.Errorf("app after the admin app's use: %q", got)
	}
	if session.User.LastActivityApp != AdminApp {
		t.Errorf("app answered: %q", session.User.LastActivityApp)
	}
	if AdminApp != "Polyfin admin" {
		t.Errorf("the admin app is named %q", AdminApp)
	}
}

func TestActivityAppNeverMovesWithoutTheTime(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	store.now = time.Now
	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	ahead := func() time.Time {
		t.Helper()
		var at time.Time
		if err := store.db.QueryRow(ctx, `UPDATE users SET last_activity_at = date_trunc('second', now()) + interval '1 hour',
			last_activity_app = 'Jellyfin Web' WHERE id = $1 RETURNING last_activity_at`, alice.ID).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at
	}
	stays := func(what string, future time.Time) {
		t.Helper()
		if got, app := lastActivity(t, store, alice.ID), lastActivityApp(t, store, alice.ID); got == nil || !got.Equal(future) || app != "Jellyfin Web" {
			t.Errorf("%s moved the activity from %v in Jellyfin Web to %v in %q", what, future, got, app)
		}
	}

	future := ahead()
	sessionToken, _, err := store.CreateAdminSession(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	stays("the admin app's sign-in", future)
	future = ahead()
	tv := device("tv")
	tv.Client = "Jellyfin Android TV"
	deviceToken, _, err := store.SignInDevice(ctx, alice.ID, tv)
	if err != nil {
		t.Fatal(err)
	}
	stays("the device's sign-in", future)

	// Requests on a clock behind the database's, the device's with a new
	// address that writes the device.
	store.now = func() time.Time { return time.Now().Add(-time.Hour) }
	_, user, err := store.DeviceByToken(ctx, deviceToken, "198.51.100.7")
	if err != nil {
		t.Fatal(err)
	}
	stays("the device's request", future)
	if user.LastActivityApp != "Jellyfin Web" || user.LastActivityAt == nil || !user.LastActivityAt.Equal(future) {
		t.Errorf("activity answered: %v from %q", user.LastActivityAt, user.LastActivityApp)
	}
	session, err := store.AdminSession(ctx, sessionToken)
	if err != nil {
		t.Fatal(err)
	}
	stays("the admin app's request", future)
	if session.User.LastActivityApp != "Jellyfin Web" {
		t.Errorf("admin app answered %q", session.User.LastActivityApp)
	}
}
