package appimport

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/moodiness/polyfin/internal/accounts"
)

// renewingSource is the fake app, whose first read renews the sign-in, then
// closes renewed and answers once release is closed: the app renewed it,
// and Polyfin has not saved it yet.
type renewingSource struct {
	*fakeSource
	once             sync.Once
	renewed, release chan struct{}
}

func (s *renewingSource) Reread(ctx context.Context, signIn SignIn) (Account, SignIn, error) {
	account, renewed, err := s.fakeSource.Reread(ctx, signIn)
	s.once.Do(func() {
		close(s.renewed)
		<-s.release
	})
	return account, renewed, err
}

// gate returns a channel and what closes it, once, and at the latest as
// the test ends, before the service stops: what the test holds with it
// goes on then.
func gate(t *testing.T) (chan struct{}, func()) {
	t.Helper()
	c := make(chan struct{})
	open := sync.OnceFunc(func() { close(c) })
	t.Cleanup(open)
	return c, open
}

// settled waits for the work the service runs in the background to end:
// the imports of connections, and the sign-outs.
func (h connectionHarness) settled() {
	h.wg.Wait()
}

// renewal is a harness where deleting a user signs out their connections,
// and alice is connected to the fake app with alice-first, Polyfin's when
// owned, else a key she pasted. Each read of the app renews the sign-in:
// the first renewal is renewed-secret-b.
type renewal struct {
	connectionHarness
	alice   accounts.User
	account Account
	owned   bool
}

func newRenewal(t *testing.T, owned bool, source func(*fakeSource) Source) renewal {
	t.Helper()
	h := newConnectionHarness(t)
	h.store.OnDelete(h.Leaving)
	if source != nil {
		h.sources["stremio"] = source(h.source)
	}
	r := renewal{connectionHarness: h, alice: h.user(t, "alice", false), owned: owned,
		account: Account{Profiles: []Profile{{ID: "1", Entries: []Entry{{Title: ref(t, "tt0000001", true, 0, 0), Played: true}}}}}}
	h.source.set(func(f *fakeSource) {
		f.account, f.rotate = r.account, true
		f.accepted["alice-first"], f.accepted["alice-second"] = true, true
	})
	h.connected(t, r.alice, r.alice, r.account, SignIn{Secret: "alice-first", Owned: owned}, Choice{Profile: "1"})
	return r
}

// leave replaces or removes alice's connection as she signs in again with
// alice-second, disconnects, or is deleted. Signing in again does not wait
// for its import when wait is false.
func (r renewal) leave(t *testing.T, how string, wait bool) {
	t.Helper()
	switch how {
	case "signed in again":
		signIn := SignIn{Secret: "alice-second", Owned: r.owned}
		if wait {
			r.connected(t, r.alice, r.alice, r.account, signIn, Choice{Profile: "1"})
			return
		}
		account := r.account
		account.App, account.SignIn, account.StayConnected = "stremio", &signIn, true
		if _, err := r.Start(r.alice, r.alice, r.Hold(r.alice.ID, account), Choice{Profile: "1"}); err != nil {
			t.Fatal(err)
		}
	case "disconnected":
		if err := r.Disconnect(t.Context(), r.alice.ID, "stremio"); err != nil {
			t.Fatal(err)
		}
	case "deleted":
		if err := r.store.DeleteUser(t.Context(), r.alice.ID); err != nil {
			t.Fatal(err)
		}
	}
}

// check checks, once the service settled, that the app signed out
// signedOut, sorted, in any order, as Polyfin signs out in the background;
// that alice's connection keeps alice-second when she signed in again; and
// that the log holds no sign-in.
func (r renewal) check(t *testing.T, how string, signedOut ...string) {
	t.Helper()
	r.settled()
	got := r.source.signedOutSecrets()
	slices.Sort(got)
	if !slices.Equal(got, signedOut) {
		t.Errorf("signed out: %v, want %v", got, signedOut)
	}
	if how == "signed in again" {
		stored, owned := r.stored(t, r.alice.ID)
		if signIn, err := r.open(stored, owned); err != nil || signIn.Secret != "alice-second" {
			t.Errorf("the connection kept: %+v %v", signIn, err)
		}
	}
	for _, secret := range []string{"alice-first", "alice-second", "renewed-secret"} {
		if strings.Contains(r.log.String(), secret) {
			t.Errorf("the log holds %s: %s", secret, r.log.String())
		}
	}
}

var leaving = []string{"signed in again", "disconnected", "deleted"}

// The app renews the sign-in while its user signs in again, disconnects
// or is deleted: the connection went, and the one it held was signed out,
// when Polyfin comes to save the renewed one, which it then signs out too,
// when it is Polyfin's. A key the user pasted is never signed out.
func TestASignInRenewedAsItsConnectionGoesIsSignedOut(t *testing.T) {
	t.Parallel()
	for _, owned := range []bool{true, false} {
		for _, how := range leaving {
			kind := "pasted "
			if owned {
				kind = "owned "
			}
			t.Run(kind+how, func(t *testing.T) {
				t.Parallel()
				var held *renewingSource
				r := newRenewal(t, owned, func(f *fakeSource) Source {
					held = &renewingSource{fakeSource: f, renewed: make(chan struct{})}
					return held
				})
				release, open := gate(t)
				held.release = release
				if _, err := r.ImportNow(t.Context(), r.alice, "stremio"); err != nil {
					t.Fatal(err)
				}
				<-held.renewed
				r.leave(t, how, true)
				open()
				if owned {
					r.check(t, how, "alice-first", "renewed-secret-b")
				} else {
					r.check(t, how)
				}
			})
		}
	}
}

// The app renews the sign-in, which Polyfin saves, and the user signs in
// again, disconnects or is deleted while the import still runs: the
// renewed sign-in is the one signed out.
func TestASignInRenewedBeforeItsConnectionGoesIsSignedOutWithIt(t *testing.T) {
	t.Parallel()
	for _, how := range leaving {
		t.Run(how, func(t *testing.T) {
			t.Parallel()
			r := newRenewal(t, true, nil)
			// The import waits as it starts to merge, the renewal saved.
			release, open := gate(t)
			titles := &blockingTitles{Titles: r.lib, entered: make(chan struct{}), release: release}
			r.titles = titles
			if _, err := r.ImportNow(t.Context(), r.alice, "stremio"); err != nil {
				t.Fatal(err)
			}
			<-titles.entered
			stored, owned := r.stored(t, r.alice.ID)
			if signIn, err := r.open(stored, owned); err != nil || signIn.Secret != "renewed-secret-b" {
				t.Fatalf("the renewal saved: %+v %v", signIn, err)
			}
			r.leave(t, how, false)
			open()
			r.check(t, how, "renewed-secret-b")
		})
	}
}

// Deleting a user locks the connections it reads the sign-ins of until
// the deletion ends: a renewal cannot save on them in between, to be
// deleted unseen. It waits, finds its connection gone, and is signed out.
func TestARenewalWaitsForTheDeletionOfItsUser(t *testing.T) {
	t.Parallel()
	var held *renewingSource
	r := newRenewal(t, true, func(f *fakeSource) Source {
		held = &renewingSource{fakeSource: f, renewed: make(chan struct{})}
		return held
	})
	release, openRelease := gate(t)
	held.release = release
	// The deletion waits once its user's sign-ins are read.
	reading := make(chan struct{})
	deleting, openDeleting := gate(t)
	r.store.OnDelete(func(context.Context, pgx.Tx, accounts.ID) (func(), error) {
		close(reading)
		<-deleting
		return nil, nil
	})
	if _, err := r.ImportNow(t.Context(), r.alice, "stremio"); err != nil {
		t.Fatal(err)
	}
	<-held.renewed
	deleted := make(chan error, 1)
	go func() { deleted <- r.store.DeleteUser(t.Context(), r.alice.ID) }()
	<-reading
	// A renewal's update would wait for the connection: it is locked.
	_, err := r.pool.Exec(t.Context(), "SELECT 1 FROM app_connections WHERE user_id = $1 FOR NO KEY UPDATE NOWAIT", r.alice.ID)
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); !ok || pgErr.Code != "55P03" {
		t.Errorf("a renewal saves on the connection being deleted: %v", err)
	}
	openRelease()
	openDeleting()
	if err := <-deleted; err != nil {
		t.Fatal(err)
	}
	r.check(t, "deleted", "alice-first", "renewed-secret-b")
}
