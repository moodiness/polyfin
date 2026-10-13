package appimport

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/userdata"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// fakeSource reads again the account of a fake app, as a source does.
type fakeSource struct {
	mu sync.Mutex
	// account is what a read answers; accepted are the secrets the app
	// takes, and rotate renews the one read with each read; fail, when
	// set, is the error every read fails with.
	account  Account
	accepted map[string]bool
	rotate   bool
	fail     error
	// reads are the secrets read with, signedOut those signed out.
	reads, signedOut []string
	renewals         int
	// signOutFails fails the sign-outs; hang has them wait for their
	// context to end, and fail.
	signOutFails, hang bool
}

func (f *fakeSource) Reread(_ context.Context, signIn SignIn) (Account, SignIn, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = append(f.reads, signIn.Secret)
	if f.fail != nil {
		return Account{}, signIn, f.fail
	}
	if !f.accepted[signIn.Secret] {
		return Account{}, signIn, ErrSignInRefused
	}
	if f.rotate {
		f.renewals++
		delete(f.accepted, signIn.Secret)
		signIn.Secret = "renewed-secret-" + string(rune('a'+f.renewals))
		f.accepted[signIn.Secret] = true
	}
	return f.account, signIn, nil
}

func (f *fakeSource) SignOut(ctx context.Context, signIn SignIn) error {
	f.mu.Lock()
	hang, fails := f.hang, f.signOutFails
	f.mu.Unlock()
	if hang {
		<-ctx.Done()
		return errors.New("the app does not answer")
	}
	if fails {
		return errors.New("the app answered 500")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.signedOut = append(f.signedOut, signIn.Secret)
	return nil
}

func (f *fakeSource) set(change func(*fakeSource)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	change(f)
}

func (f *fakeSource) readsWith(secret string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.reads {
		if r == secret {
			n++
		}
	}
	return n
}

func (f *fakeSource) signedOutSecrets() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.signedOut)
}

// lockedBuffer is a log the service writes to from its goroutines.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// connectionHarness is a harness keeping connections, sealed, to the fake
// app "stremio", logging to log, with the setting that lets users import
// their history in imports, and the stops told in stops.
type connectionHarness struct {
	harness
	source  *fakeSource
	log     *lockedBuffer
	imports *atomic.Bool
	stops   *stops
}

// stops records the connections told stopped, as "user app".
type stops struct {
	mu   sync.Mutex
	told []string
}

func (s *stops) add(user accounts.ID, app string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.told = append(s.told, user.String()+" "+app)
}

func (s *stops) list() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.told)
}

func newConnectionHarness(t *testing.T) connectionHarness {
	t.Helper()
	box, err := secrets.New(bytes.Repeat([]byte{7}, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	source := &fakeSource{accepted: map[string]bool{}}
	log := &lockedBuffer{}
	imports := &atomic.Bool{}
	imports.Store(true)
	stopped := &stops{}
	h := newHarness(t, func(o *Options) {
		settings := o.Settings
		o.Settings = func() accounts.Settings {
			s := settings()
			s.ServerImports = imports.Load()
			return s
		}
		o.Logger = slog.New(slog.NewTextHandler(log, &slog.HandlerOptions{Level: slog.LevelDebug}))
		o.Secrets, o.Sources, o.Stopped = box, map[string]Source{"stremio": source}, stopped.add
	})
	return connectionHarness{harness: h, source: source, log: log, imports: imports, stops: stopped}
}

// connected holds account, read to stay connected with signIn, for by,
// imports choice of it into user's account, waits for the import to end,
// and returns user's connection.
func (h connectionHarness) connected(t *testing.T, by, user accounts.User, account Account, signIn SignIn, choice Choice) Connection {
	t.Helper()
	account.App, account.SignIn = "stremio", &signIn
	h.imported(t, by, user, account, choice)
	h.idle(t)
	return h.connectionOf(t, user)
}

// idle waits for the imports of connections running to end.
func (h connectionHarness) idle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		h.mu.Lock()
		running := len(h.connecting)
		h.mu.Unlock()
		if running == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the imports of connections never ended")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// connectionOf is user's connection to the fake app, failing the test when
// there is none.
func (h connectionHarness) connectionOf(t *testing.T, user accounts.User) Connection {
	t.Helper()
	connections, err := h.Connections(t.Context(), user)
	if err != nil || len(connections) != 1 || connections[0].App != "stremio" {
		t.Fatalf("%s's connections: %+v %v", user.Name, connections, err)
	}
	return connections[0]
}

// importedNow imports user's connection at once, and waits for the import
// to end.
func (h connectionHarness) importedNow(t *testing.T, user accounts.User) Connection {
	t.Helper()
	if _, err := h.ImportNow(t.Context(), user, "stremio"); err != nil {
		t.Fatal(err)
	}
	h.idle(t)
	return h.connectionOf(t, user)
}

// stored is what the database holds of user's connection to the fake app.
func (h connectionHarness) stored(t *testing.T, user accounts.ID) (signIn string, owned bool) {
	t.Helper()
	if err := h.pool.QueryRow(t.Context(), "SELECT sign_in, owned FROM app_connections WHERE user_id = $1 AND app = 'stremio'", user).
		Scan(&signIn, &owned); err != nil {
		t.Fatal(err)
	}
	return signIn, owned
}

// aged moves the imports of every connection back by d, as if they ran d
// earlier.
func (h connectionHarness) aged(t *testing.T, d time.Duration) {
	t.Helper()
	if _, err := h.pool.Exec(t.Context(), "UPDATE app_connections SET last_import_at = last_import_at - make_interval(secs => $1)",
		d.Seconds()); err != nil {
		t.Fatal(err)
	}
}

// setInterval sets how often connections are imported, in minutes.
func (h connectionHarness) setInterval(t *testing.T, minutes int) {
	t.Helper()
	settings := h.store.Settings()
	settings.ImportEveryMinutes = minutes
	if _, err := h.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
}

func TestStayingConnectedKeepsTheSignInAndImportsAgain(t *testing.T) {
	t.Parallel()
	h := newConnectionHarness(t)
	root, alice := h.user(t, "root", true), h.user(t, "alice", false)
	first, kids, later := ref(t, "tt0000001", true, 0, 0), ref(t, "tt0000009", true, 0, 0), ref(t, "tt0000010", true, 0, 0)
	addon := fakeAddon(t, "Kids addon")
	account := Account{Name: "alice@example.com", Profiles: []Profile{
		{ID: "1", Name: "Main", Entries: []Entry{{Title: first, Played: true}}},
		{ID: "2", Name: "Kids", Entries: []Entry{{Title: kids, Played: true}}, Addons: []Addon{{Name: "Kids addon", ManifestURL: addon}}},
	}}
	const secret = "kept-secret-0123456789abcdef"
	h.source.set(func(f *fakeSource) {
		f.account, f.accepted[secret], f.rotate = account, true, true
	})

	// An administrator imports Alice's Kids profile with its addon, on the
	// local network, which only they reach, and keeps her connected.
	c := h.connected(t, root, alice, account, SignIn{Secret: secret, Owned: true}, Choice{Profile: "2", Addons: []int{0}})
	if c.Account != "alice@example.com" || c.ConnectedBy != root.ID || c.Problem != "" || c.Paused || c.LastImport == nil ||
		c.LastImport.Read != 1 || c.LastImport.Played != 1 || c.NextImport == nil ||
		c.NextImport.Sub(c.LastImport.At) != 6*time.Hour {
		t.Fatalf("the connection: %+v %+v", c, c.LastImport)
	}
	stored, owned := h.stored(t, alice.ID)
	if !secrets.Sealed(stored) || strings.Contains(stored, secret) || strings.Contains(stored, "Kids") || !owned {
		t.Errorf("the sign-in is stored as %q", stored)
	}
	if signedOut := h.source.signedOutSecrets(); len(signedOut) != 0 {
		t.Errorf("signed out: %v", signedOut)
	}

	// Alice removes the addon; the account plays another movie in the
	// Kids profile. The next import reads it with the key kept, in the
	// profile kept, and installs no addon.
	installed, err := h.addons.Addons(t.Context(), addons.Personal(alice.ID))
	if err != nil || len(installed) != 1 {
		t.Fatalf("Alice's addons: %+v %v", installed, err)
	}
	if err := h.addons.Remove(t.Context(), addons.Personal(alice.ID), installed[0].ID); err != nil {
		t.Fatal(err)
	}
	h.source.set(func(f *fakeSource) {
		f.account.Profiles[1].Entries = append(f.account.Profiles[1].Entries, Entry{Title: later, Played: true})
	})
	c = h.importedNow(t, alice)
	if c.LastImport.Read != 2 || c.LastImport.Played != 1 || c.LastImport.Problem != "" || h.source.readsWith(secret) != 1 {
		t.Errorf("the second import: %+v", c.LastImport)
	}
	if !h.dataOf(t, alice.ID, later).Played || h.dataOf(t, alice.ID, first).Played {
		t.Error("the Kids profile is not the one imported")
	}
	if installed, err := h.addons.Addons(t.Context(), addons.Personal(alice.ID)); err != nil || len(installed) != 0 {
		t.Errorf("a later import installed %+v %v", installed, err)
	}
	// The app renewed the secret: the renewed one is kept, and read with.
	stored, _ = h.stored(t, alice.ID)
	if signIn, err := h.open(stored, true); err != nil || signIn.Secret != "renewed-secret-b" || signIn.Profile != "2" {
		t.Errorf("the renewed sign-in: %+v %v", signIn, err)
	}
	h.importedNow(t, alice)
	if h.source.readsWith("renewed-secret-b") != 1 {
		t.Errorf("reads: %v", h.source.reads)
	}

	// The app refuses the sign-in: the connection stops, with its reason,
	// and is not imported until Alice signs in again.
	h.source.set(func(f *fakeSource) { clear(f.accepted) })
	c = h.importedNow(t, alice)
	if c.Problem != ProblemSignInRefused || c.LastImport.Problem != ProblemSignInRefused || c.NextImport != nil {
		t.Errorf("refused: %+v %+v", c, c.LastImport)
	}
	if _, err := h.ImportNow(t.Context(), alice, "stremio"); !errors.Is(err, ErrStopped) {
		t.Errorf("importing a stopped connection: %v", err)
	}
	reads := len(h.source.reads)
	h.aged(t, 7*time.Hour)
	h.importDue()
	h.idle(t)
	if len(h.source.reads) != reads {
		t.Errorf("a stopped connection was imported: %v", h.source.reads)
	}

	// Alice signs in again: the new sign-in replaces the one refused,
	// which Polyfin signs out, and the connection runs again.
	h.source.set(func(f *fakeSource) { f.accepted["signed-in-again"] = true })
	c = h.connected(t, alice, alice, account, SignIn{Secret: "signed-in-again", Owned: true}, Choice{Profile: "2"})
	if c.Problem != "" || c.LastImport == nil || c.LastImport.Problem != "" {
		t.Errorf("signed in again: %+v", c)
	}
	if signedOut := h.source.signedOutSecrets(); !slices.Equal(signedOut, []string{"renewed-secret-c"}) {
		t.Errorf("signed out: %v", signedOut)
	}

	// Disconnecting signs out the session Polyfin opened, and forgets it.
	if err := h.Disconnect(t.Context(), alice.ID, "stremio"); err != nil {
		t.Fatal(err)
	}
	if signedOut := h.source.signedOutSecrets(); !slices.Equal(signedOut, []string{"renewed-secret-c", "signed-in-again"}) {
		t.Errorf("signed out: %v", signedOut)
	}
	if connections, err := h.Connections(t.Context(), alice); err != nil || len(connections) != 0 {
		t.Errorf("after disconnecting: %+v %v", connections, err)
	}
	if _, err := h.ImportNow(t.Context(), alice, "stremio"); !errors.Is(err, ErrNotConnected) {
		t.Errorf("importing after disconnecting: %v", err)
	}

	for _, kept := range []string{secret, "renewed-secret", "signed-in-again"} {
		if strings.Contains(h.log.String(), kept) {
			t.Errorf("the log holds %s: %s", kept, h.log.String())
		}
	}
}

func TestPastedKeysAreNeverSignedOut(t *testing.T) {
	t.Parallel()
	h := newConnectionHarness(t)
	bob := h.user(t, "bob", false)
	account := Account{Profiles: []Profile{{ID: "1"}}}
	h.source.set(func(f *fakeSource) { f.account, f.accepted["pasted"] = account, true })

	c := h.connected(t, bob, bob, account, SignIn{Secret: "pasted"}, Choice{Profile: "1"})
	if c.Account != "" || c.LastImport == nil {
		t.Errorf("the connection: %+v", c)
	}
	if _, owned := h.stored(t, bob.ID); owned {
		t.Error("a pasted key is stored as Polyfin's")
	}
	// A new pasted key replaces it, and is forgotten without signing out.
	h.connected(t, bob, bob, account, SignIn{Secret: "pasted again"}, Choice{Profile: "1"})
	if err := h.Disconnect(t.Context(), bob.ID, "stremio"); err != nil {
		t.Fatal(err)
	}
	if signedOut := h.source.signedOutSecrets(); len(signedOut) != 0 {
		t.Errorf("signed out: %v", signedOut)
	}

	// An account read to stay connected and never imported has the session
	// Polyfin opened signed out, a pasted key left alone.
	h.Service.holdFor = 20 * time.Millisecond
	h.Hold(bob.ID, Account{App: "stremio", SignIn: &SignIn{Secret: "pasted, never imported"}})
	h.Hold(bob.ID, Account{App: "stremio", SignIn: &SignIn{Secret: "replaced", Owned: true}})
	h.Hold(bob.ID, Account{App: "stremio", SignIn: &SignIn{Secret: "expired", Owned: true}})
	deadline := time.Now().Add(5 * time.Second)
	for len(h.source.signedOutSecrets()) < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if signedOut := h.source.signedOutSecrets(); !slices.Equal(signedOut, []string{"replaced", "expired"}) {
		t.Errorf("signed out: %v", signedOut)
	}
}

func TestTheSettingPausesMembersConnections(t *testing.T) {
	t.Parallel()
	h := newConnectionHarness(t)
	root, bob, carol := h.user(t, "root", true), h.user(t, "bob", false), h.user(t, "carol", false)
	account := Account{Profiles: []Profile{{ID: "1"}}}
	h.source.set(func(f *fakeSource) {
		f.account = account
		f.accepted["root's"], f.accepted["bob's"], f.accepted["carol's"] = true, true, true
	})
	h.connected(t, root, root, account, SignIn{Secret: "root's", Owned: true}, Choice{Profile: "1"})
	// An administrator connects Bob's account for him.
	c := h.connected(t, root, bob, account, SignIn{Secret: "bob's", Owned: true}, Choice{Profile: "1"})
	if c.ConnectedBy != root.ID || c.ConnectedByName != "root" {
		t.Errorf("Bob's connection: %+v", c)
	}
	h.connected(t, carol, carol, account, SignIn{Secret: "carol's", Owned: true}, Choice{Profile: "1"})
	if _, err := h.pool.Exec(t.Context(), "UPDATE users SET is_disabled = true WHERE id = $1", carol.ID); err != nil {
		t.Fatal(err)
	}

	// Nothing is due before 6 hours.
	h.importDue()
	h.idle(t)
	if len(h.source.reads) != 0 {
		t.Errorf("read early: %v", h.source.reads)
	}
	h.aged(t, 7*time.Hour)

	// The setting off pauses Bob's connection, not the administrator's;
	// a disabled user's never runs.
	h.imports.Store(false)
	if c := h.connectionOf(t, bob); !c.Paused || c.NextImport != nil {
		t.Errorf("Bob's paused connection: %+v", c)
	}
	if c := h.connectionOf(t, root); c.Paused || c.NextImport == nil {
		t.Errorf("the administrator's connection: %+v", c)
	}
	h.importDue()
	h.idle(t)
	if h.source.readsWith("root's") != 1 || h.source.readsWith("bob's") != 0 || h.source.readsWith("carol's") != 0 {
		t.Errorf("reads with the setting off: %v", h.source.reads)
	}
	h.imports.Store(true)
	h.importDue()
	h.idle(t)
	if h.source.readsWith("bob's") != 1 || h.source.readsWith("carol's") != 0 {
		t.Errorf("reads with the setting on: %v", h.source.reads)
	}
}

func TestTheNextImportFollowsTheSetting(t *testing.T) {
	t.Parallel()
	h := newConnectionHarness(t)
	alice := h.user(t, "alice", false)
	account := Account{Profiles: []Profile{{ID: "1"}}}
	h.source.set(func(f *fakeSource) { f.account, f.accepted["alice's"] = account, true })
	c := h.connected(t, alice, alice, account, SignIn{Secret: "alice's", Owned: true}, Choice{Profile: "1"})
	if c.NextImport == nil || c.NextImport.Sub(c.LastImport.At) != 6*time.Hour {
		t.Fatalf("by default: %+v", c)
	}

	// An hour later, nothing is due at 6 hours.
	h.aged(t, time.Hour)
	h.importDue()
	h.idle(t)
	if n := h.source.readsWith("alice's"); n != 0 {
		t.Errorf("read within 6 hours: %d", n)
	}

	// 30 minutes bring the import due forward: the next import shown
	// follows at once, and the import runs at the next check.
	h.setInterval(t, 30)
	c = h.connectionOf(t, alice)
	if c.NextImport == nil || c.NextImport.Sub(c.LastImport.At) != 30*time.Minute || !c.NextImport.Before(time.Now()) {
		t.Errorf("at 30 minutes: %+v", c)
	}
	h.importDue()
	h.idle(t)
	if n := h.source.readsWith("alice's"); n != 1 {
		t.Errorf("not brought forward: %d reads", n)
	}
	c = h.connectionOf(t, alice)
	if c.NextImport == nil || c.NextImport.Sub(c.LastImport.At) != 30*time.Minute || !c.NextImport.After(time.Now()) {
		t.Errorf("after the import: %+v", c)
	}
	h.importDue()
	h.idle(t)
	if n := h.source.readsWith("alice's"); n != 1 {
		t.Errorf("read again within 30 minutes: %d reads", n)
	}

	// A longer interval puts it off.
	h.aged(t, 2*time.Hour)
	h.setInterval(t, 3*60)
	h.importDue()
	h.idle(t)
	if n := h.source.readsWith("alice's"); n != 1 {
		t.Errorf("read within 3 hours: %d reads", n)
	}
}

func TestAStoppedConnectionIsToldOnce(t *testing.T) {
	t.Parallel()
	h := newConnectionHarness(t)
	alice := h.user(t, "alice", false)
	account := Account{Profiles: []Profile{{ID: "1"}}}
	h.source.set(func(f *fakeSource) { f.account, f.accepted["first"] = account, true })
	h.connected(t, alice, alice, account, SignIn{Secret: "first", Owned: true}, Choice{Profile: "1"})

	// An app that does not answer, or asks to wait, is tried again at the
	// next import: nothing is told.
	for _, failure := range []error{ErrAppUnreachable, ErrAppRateLimited} {
		h.source.set(func(f *fakeSource) { f.fail = failure })
		if c := h.importedNow(t, alice); c.Problem != "" || c.LastImport.Problem == "" {
			t.Errorf("%v: %+v %+v", failure, c, c.LastImport)
		}
	}
	if told := h.stops.list(); len(told) != 0 {
		t.Errorf("told %v", told)
	}

	// The app refuses the sign-in: the stop is told once, as it stops.
	h.source.set(func(f *fakeSource) {
		f.fail = nil
		clear(f.accepted)
	})
	c := h.importedNow(t, alice)
	want := []string{alice.ID.String() + " stremio"}
	if told := h.stops.list(); c.Problem != ProblemSignInRefused || !slices.Equal(told, want) {
		t.Errorf("stopped: %+v, told %v", c, told)
	}
	// A refusal recorded again, as by an import that ran meanwhile, tells
	// nothing more.
	h.record(connectionKey{alice.ID, "stremio"}, c.ConnectedAt, ConnectionImport{Problem: ProblemSignInRefused})
	if told := h.stops.list(); !slices.Equal(told, want) {
		t.Errorf("refused again: told %v", told)
	}

	// Signed in again, then refused again: told again.
	h.source.set(func(f *fakeSource) { f.accepted["second"] = true })
	if c := h.connected(t, alice, alice, account, SignIn{Secret: "second", Owned: true}, Choice{Profile: "1"}); c.Problem != "" {
		t.Fatalf("signed in again: %+v", c)
	}
	h.source.set(func(f *fakeSource) { clear(f.accepted) })
	h.importedNow(t, alice)
	if told := h.stops.list(); !slices.Equal(told, append(want, want...)) {
		t.Errorf("refused after signing in again: told %v", told)
	}
}

func TestTheLibraryLeavesTheWatchlistWhenNoListHoldsIt(t *testing.T) {
	t.Parallel()
	h := newConnectionHarness(t)
	alice := h.user(t, "alice", false)
	movie := func(n int) accounts.ID { return h.idOf(t, ref(t, fmt.Sprintf("tt%07d", n), true, 0, 0)).ID }
	// The library holds movies added to it on the day of their number.
	library := func(numbers ...int) []Profile {
		var entries []Entry
		for _, n := range numbers {
			entries = append(entries, Entry{Title: ref(t, fmt.Sprintf("tt%07d", n), true, 0, 0), Watchlist: true,
				AddedAt: new(time.Date(2026, 9, n, 0, 0, 0, 0, time.UTC))})
		}
		return []Profile{{ID: "1", Name: "Main", Entries: entries}}
	}
	// A one-off import brings 1 and 2; a connection 2, 3 and 4.
	h.imported(t, alice, alice, Account{App: "stremio", Profiles: library(1, 2)}, Choice{Profile: "1"})
	const secret = "kept-secret-0123456789abcdef"
	h.source.set(func(f *fakeSource) { f.accepted[secret] = true })
	c := h.connected(t, alice, alice, Account{Profiles: library(2, 3, 4)}, SignIn{Secret: secret}, Choice{Profile: "1"})
	if c.LastImport == nil || c.LastImport.Watchlist != 2 {
		t.Errorf("first import of the connection: %+v", c.LastImport)
	}
	if got := h.watchlistOf(t, alice.ID); !slices.Equal(got, []accounts.ID{movie(4), movie(3), movie(2), movie(1)}) {
		t.Fatalf("watchlist: %v", got)
	}

	// Taken off the app's library, 3 leaves at the next import; 2 stays,
	// as the one-off import brought it too.
	h.source.set(func(f *fakeSource) { f.account = Account{Profiles: library(4)} })
	if c := h.importedNow(t, alice); c.LastImport == nil || c.LastImport.Watchlist != 0 || c.LastImport.Problem != "" {
		t.Errorf("next import: %+v", c.LastImport)
	}
	if got := h.watchlistOf(t, alice.ID); !slices.Equal(got, []accounts.ID{movie(4), movie(2), movie(1)}) {
		t.Fatalf("after the next import: %v", got)
	}
	// A one-off import's title leaves once started.
	if _, err := h.data.Change(t.Context(), alice.ID, []userdata.Item{{ID: movie(1)}}, func(d *userdata.Data) { d.Played = true }); err != nil {
		t.Fatal(err)
	}
	if got := h.watchlistOf(t, alice.ID); !slices.Equal(got, []accounts.ID{movie(4), movie(2)}) {
		t.Errorf("after one started: %v", got)
	}
	// No import set a favorite.
	if favorites, err := h.data.Favorites(t.Context(), alice.ID); err != nil || len(favorites) != 0 {
		t.Errorf("favorites: %+v %v", favorites, err)
	}
	// Disconnected, what the connection alone held leaves; the one-off
	// import's title stays.
	if err := h.Disconnect(t.Context(), alice.ID, "stremio"); err != nil {
		t.Fatal(err)
	}
	if got := h.watchlistOf(t, alice.ID); !slices.Equal(got, []accounts.ID{movie(2)}) {
		t.Errorf("after disconnecting: %v", got)
	}
}

// blockingTitles finds titles as Titles does, once release is closed,
// closing entered as the first finding starts. It goes on when its
// context ends, as a finding under way may.
type blockingTitles struct {
	Titles
	once             sync.Once
	entered, release chan struct{}
}

func (b *blockingTitles) Resolve(ctx context.Context, refs []library.TitleRef) ([][]library.TitleTarget, error) {
	b.once.Do(func() { close(b.entered) })
	<-b.release
	return b.Titles.Resolve(context.WithoutCancel(ctx), refs)
}

// A connection disconnected, or its user deleted, while its first import
// still matches its titles leaves none of its library on the watchlist,
// and the import ends without failing.
func TestTheFirstImportOfAConnectionGoneSavesNoLibrary(t *testing.T) {
	t.Parallel()
	for _, leave := range []string{"disconnected", "deleted"} {
		t.Run(leave, func(t *testing.T) {
			t.Parallel()
			h := newConnectionHarness(t)
			h.store.OnDelete(h.Leaving)
			titles := &blockingTitles{Titles: h.lib, entered: make(chan struct{}), release: make(chan struct{})}
			h.titles = titles
			alice := h.user(t, "alice", false)
			const secret = "kept-secret-0123456789abcdef"
			h.source.set(func(f *fakeSource) { f.accepted[secret] = true })
			account := Account{App: "stremio", SignIn: &SignIn{Secret: secret, Owned: true}, Profiles: []Profile{{ID: "1", Entries: []Entry{
				{Title: ref(t, "tt0000001", true, 0, 0), Watchlist: true}, {Title: ref(t, "tt0000002", true, 0, 0), Watchlist: true}}}}}
			if _, err := h.Start(alice, alice, h.Hold(alice.ID, account), Choice{Profile: "1"}); err != nil {
				t.Fatal(err)
			}
			<-titles.entered
			if leave == "disconnected" {
				if err := h.Disconnect(t.Context(), alice.ID, "stremio"); err != nil {
					t.Fatal(err)
				}
			} else if err := h.store.DeleteUser(t.Context(), alice.ID); err != nil {
				t.Fatal(err)
			}
			close(titles.release)
			if status := h.ended(t, alice.ID); status.State != StateDone || status.Problem != "" || status.Watchlist != 0 {
				t.Errorf("the import: %+v", status)
			}
			h.idle(t)
			var rows int
			if err := h.pool.QueryRow(t.Context(), "SELECT count(*) FROM watchlist WHERE user_id = $1", alice.ID).Scan(&rows); err != nil || rows != 0 {
				t.Errorf("watchlist rows: %d %v", rows, err)
			}
			if strings.Contains(h.log.String(), "could not be imported") {
				t.Errorf("logged: %s", h.log.String())
			}
		})
	}
}

// An import that saves a connection's library once the connection is gone,
// as its stop came too late, saves nothing.
func TestALibraryIsNotSavedForAConnectionGone(t *testing.T) {
	t.Parallel()
	h := newConnectionHarness(t)
	alice := h.user(t, "alice", false)
	account := Account{Profiles: []Profile{{ID: "1"}}}
	h.source.set(func(f *fakeSource) { f.account, f.accepted["alice's"] = account, true })
	c := h.connected(t, alice, alice, account, SignIn{Secret: "alice's", Owned: true}, Choice{Profile: "1"})
	entries := []Entry{{Title: ref(t, "tt0000001", true, 0, 0), Watchlist: true}}
	merge := func() *Status {
		status := &Status{App: "stremio"}
		r := &run{s: h.Service, status: status, user: alice, entries: entries, list: watchlist.Connection("stremio"), connectedAt: c.ConnectedAt}
		if err := r.merge(t.Context()); err != nil {
			t.Fatal(err)
		}
		return status
	}
	if status := merge(); status.Watchlist != 1 || len(h.watchlistOf(t, alice.ID)) != 1 {
		t.Fatalf("connected: %+v %v", status, h.watchlistOf(t, alice.ID))
	}
	if err := h.Disconnect(t.Context(), alice.ID, "stremio"); err != nil {
		t.Fatal(err)
	}
	if status := merge(); status.Watchlist != 0 || len(h.watchlistOf(t, alice.ID)) != 0 {
		t.Errorf("disconnected: %+v %v", status, h.watchlistOf(t, alice.ID))
	}
}
