package appimport

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/secrets"
)

// fakeSource reads again the account of a fake app, as a source does.
type fakeSource struct {
	mu sync.Mutex
	// account is what a read answers; accepted are the secrets the app
	// takes, and rotate renews the one read with each read.
	account  Account
	accepted map[string]bool
	rotate   bool
	// reads are the secrets read with, signedOut those signed out.
	reads, signedOut []string
	renewals         int
}

func (f *fakeSource) Reread(_ context.Context, signIn SignIn) (Account, SignIn, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = append(f.reads, signIn.Secret)
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

func (f *fakeSource) SignOut(_ context.Context, signIn SignIn) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.signedOut = append(f.signedOut, signIn.Secret)
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
// their history in imports.
type connectionHarness struct {
	harness
	source  *fakeSource
	log     *lockedBuffer
	imports *atomic.Bool
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
	h := newHarness(t, func(o *Options) {
		settings := o.Settings
		o.Settings = func() accounts.Settings {
			s := settings()
			s.ServerImports = imports.Load()
			return s
		}
		o.Logger = slog.New(slog.NewTextHandler(log, &slog.HandlerOptions{Level: slog.LevelDebug}))
		o.Secrets, o.Sources = box, map[string]Source{"stremio": source}
	})
	return connectionHarness{harness: h, source: source, log: log, imports: imports}
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
