package accounts

import (
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The activity times a request writes, the user's and the device's, do
// not wait for the disk: the request then never waits for it. A sign-in,
// which writes them too, still waits, as do the accounts and devices
// created.
func TestActivityTimesAreWrittenWithoutWaitingForTheDisk(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()
	modes := testdb.WatchCommitModes(t, store.db, "users", "devices")
	// noted returns the commit modes noted of table since the last call.
	seen := map[string]int{}
	noted := func(table string) []string {
		t.Helper()
		all := modes(table)
		got := all[seen[table]:]
		seen[table] = len(all)
		return got
	}
	only := func(what, table, mode string) {
		t.Helper()
		if got := noted(table); len(got) == 0 || slices.ContainsFunc(got, func(m string) bool { return m != mode }) {
			t.Errorf("%s: %s written with synchronous_commit %v, want %s", what, table, got, mode)
		}
	}

	alice := mustCreate(t, store, NewUser{Name: "alice", Password: "correct horse"})
	only("the account created", "users", "on")
	token, _, err := store.SignInDevice(ctx, alice.ID, device("tv"))
	if err != nil {
		t.Fatal(err)
	}
	only("the device's sign-in", "devices", "on")
	only("the device's sign-in", "users", "on")
	adminToken, _, err := store.CreateAdminSession(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	only("the admin sign-in", "users", "on")

	later := time.Now().Add(2 * time.Minute)
	store.now = func() time.Time { return later }
	if _, _, err := store.DeviceByToken(ctx, token, "192.0.2.2"); err != nil {
		t.Fatal(err)
	}
	only("the device's activity", "devices", "off")
	only("the device's activity", "users", "off")
	later = later.Add(2 * time.Minute)
	if _, err := store.AdminSession(ctx, adminToken); err != nil {
		t.Fatal(err)
	}
	only("the admin's activity", "users", "off")
}
