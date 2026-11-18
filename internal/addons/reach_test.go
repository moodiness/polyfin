package addons

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/stremio/reachtest"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// host is a test server that counts its requests, and answers an addon's
// manifest and one catalog page while it is up, 404 otherwise.
type host struct {
	url  string
	hits atomic.Int32
	down atomic.Bool
	// stop closes the server and its connections: nothing answers at its
	// address any more.
	stop func()
}

func newHost(t *testing.T, id string) *host {
	t.Helper()
	h := &host{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.hits.Add(1)
		switch {
		case h.down.Load():
			http.NotFound(w, r)
		case r.URL.Path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: id, Name: id, Version: "1.0.0",
				Resources: []stremio.Resource{{Name: "catalog"}}, Catalogs: []stremio.Catalog{{Type: "movie", ID: "all"}}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{}})
		}
	}))
	t.Cleanup(server.Close)
	h.url, h.stop = server.URL, server.Close
	return h
}

// reaching is a store whose client reaches through a test network.
func reaching(t *testing.T) (*Store, *accounts.Store, *reachtest.Network, *stremio.Client) {
	t.Helper()
	pool := migrated.New(t)
	users, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	client := stremio.NewClient("test")
	return New(pool, client), users, reachtest.New(client), client
}

// confinedInSQL reads the rule in SQL for the addon id.
func confinedInSQL(t *testing.T, store *Store, id accounts.ID) bool {
	t.Helper()
	var confined bool
	if err := store.db.QueryRow(t.Context(), "SELECT "+ConfinedSQL("a")+" FROM addons a WHERE a.id = $1", id).Scan(&confined); err != nil {
		t.Fatal(err)
	}
	return confined
}

// An addon is where its manifest was read from: on a local network, where
// the server's and an administrator's may name local addresses, or on the
// internet, where its requests and what it names stay on public
// addresses. A member's is read from public addresses only and stays
// confined, wherever it is.
func TestAnAddonIsWhereItsManifestWasReadFrom(t *testing.T) {
	t.Parallel()
	store, users, network, _ := reaching(t)
	ctx := t.Context()
	root, err := users.CreateUser(ctx, accounts.NewUser{Name: "root", Password: "correct horse", IsAdministrator: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := users.CreateUser(ctx, accounts.NewUser{Name: "other", Password: "correct horse", IsAdministrator: true}); err != nil {
		t.Fatal(err)
	}
	bob, err := users.CreateUser(ctx, accounts.NewUser{Name: "bob", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name     string
		scope    Scope
		member   bool
		address  func(local, public string) string
		network  Network
		confined bool
		refused  error
	}{
		{"the server's, on the loopback", Shared(), false, func(local, _ string) string { return local }, NetworkLocal, false, nil},
		{"the server's, on a private address", Shared(), false, func(local, _ string) string { return network.Serve(t, "10.0.0.7", local) },
			NetworkLocal, false, nil},
		{"the server's, on a carrier-grade NAT address", Shared(), false, func(local, _ string) string { return network.Serve(t, "100.64.1.1", local) },
			NetworkLocal, false, nil},
		{"the server's, on a public address", Shared(), false, func(_, public string) string { return public }, NetworkPublic, true, nil},
		{"an administrator's own, on the loopback", Personal(root.ID), false, func(local, _ string) string { return local }, NetworkLocal, false, nil},
		{"an administrator's own, on a public address", Personal(root.ID), false, func(_, public string) string { return public }, NetworkPublic, true, nil},
		{"a member's own, on a public address", Personal(bob.ID), true, func(_, public string) string { return public }, NetworkPublic, true, nil},
		{"a member's own, on the loopback", Personal(bob.ID), true, func(local, _ string) string { return local }, "", true, stremio.ErrPrivateNetwork},
	} {
		local, internet := newHost(t, "local"), newHost(t, "public")
		address := c.address(local.url, network.Public(t, internet.url))
		addon, err := store.Install(ctx, c.scope, address+"/manifest.json", c.member)
		if c.refused != nil {
			if !errors.Is(err, c.refused) || local.hits.Load()+internet.hits.Load() != 0 {
				t.Errorf("%s: %v, %d requests", c.name, err, local.hits.Load()+internet.hits.Load())
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		// One read told it: an address literal is read where it is.
		if addon.Network != c.network || local.hits.Load()+internet.hits.Load() != 1 {
			t.Errorf("%s: on %q after %d requests", c.name, addon.Network, local.hits.Load()+internet.hits.Load())
		}
		if got := addon.Confined(c.member); got != c.confined || confinedInSQL(t, store, addon.ID) != c.confined {
			t.Errorf("%s: confined %v, %v in SQL", c.name, got, confinedInSQL(t, store, addon.ID))
		}
		// It is read back with the addon, by every way of reading it.
		if found, err := store.Find(ctx, addon.ID); err != nil || found.Network != c.network {
			t.Errorf("%s: found on %q %v", c.name, found.Network, err)
		}
	}

	// An administrator's own addon on the local network is confined once
	// they are no longer one, and a member's stays so as an administrator
	// until it is read again: it was read from a public address.
	local := newHost(t, "kept")
	addon, err := store.Install(ctx, Personal(root.ID), local.url+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := users.UpdateUser(ctx, root.ID, accounts.UserChanges{IsAdministrator: new(false)}, nil); err != nil {
		t.Fatal(err)
	}
	if !confinedInSQL(t, store, addon.ID) || !addon.Confined(Member(Personal(root.ID), false)) {
		t.Error("a former administrator's local addon is not confined")
	}
}

// A name's answers are not where an addon is: what Polyfin connected to
// is. A name that answers a local address at one lookup and a public one
// at the next, or both at once, gives an addon on the internet nothing of
// a local addon's reach.
func TestANamedAddonIsWhereItWasConnectedTo(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	t.Run("a local address when looked up, a public one when read", func(t *testing.T) {
		t.Parallel()
		store, _, network, _ := reaching(t)
		device, internet := newHost(t, "device"), newHost(t, "public")
		// The device of the local network the name points at first is no
		// addon: it has no manifest.
		device.down.Store(true)
		network.Serve(t, "192.168.1.50:80", device.url)
		network.Serve(t, "203.0.113.80:80", internet.url)
		network.Name("turning.test", []string{"192.168.1.50"}, []string{"203.0.113.80"})
		addon, err := store.Install(ctx, Shared(), "http://turning.test/manifest.json", false)
		if err != nil {
			t.Fatal(err)
		}
		if addon.Network != NetworkPublic || !addon.Confined(false) || !confinedInSQL(t, store, addon.ID) {
			t.Errorf("on %q, confined %v", addon.Network, addon.Confined(false))
		}
		if network.Lookups("turning.test") != 2 || internet.hits.Load() != 1 {
			t.Errorf("%d lookups, %d requests to the public host", network.Lookups("turning.test"), internet.hits.Load())
		}
	})

	t.Run("a public address when looked up, a local one when read", func(t *testing.T) {
		t.Parallel()
		store, _, network, _ := reaching(t)
		device, internet := newHost(t, "device"), newHost(t, "public")
		network.Serve(t, "192.168.1.50:80", device.url)
		network.Serve(t, "203.0.113.80:80", internet.url)
		// The read from the local network is refused the public address;
		// the confined one, the local address: the name is no addon.
		network.Name("turning.test", []string{"203.0.113.80"}, []string{"192.168.1.50"})
		if _, err := store.Install(ctx, Shared(), "http://turning.test/manifest.json", false); !errors.Is(err, stremio.ErrPrivateNetwork) {
			t.Errorf("installed: %v", err)
		}
		if device.hits.Load()+internet.hits.Load() != 0 {
			t.Errorf("%d requests to the device, %d to the public host", device.hits.Load(), internet.hits.Load())
		}
	})

	t.Run("both, the addon on the public one", func(t *testing.T) {
		t.Parallel()
		store, _, network, _ := reaching(t)
		device, internet := newHost(t, "device"), newHost(t, "public")
		device.down.Store(true)
		network.Serve(t, "192.168.1.50:80", device.url)
		network.Serve(t, "203.0.113.80:80", internet.url)
		network.Name("both.test", []string{"203.0.113.80", "192.168.1.50"})
		addon, err := store.Install(ctx, Shared(), "http://both.test/manifest.json", false)
		if err != nil {
			t.Fatal(err)
		}
		if addon.Network != NetworkPublic || !addon.Confined(false) {
			t.Errorf("on %q, confined %v", addon.Network, addon.Confined(false))
		}
	})

	t.Run("both, an addon on each", func(t *testing.T) {
		t.Parallel()
		store, _, network, client := reaching(t)
		device, internet := newHost(t, "device"), newHost(t, "public")
		network.Serve(t, "192.168.1.50:80", device.url)
		network.Serve(t, "203.0.113.80:80", internet.url)
		network.Name("both.test", []string{"203.0.113.80", "192.168.1.50"})
		addon, err := store.Install(ctx, Shared(), "http://both.test/manifest.json", false)
		if err != nil {
			t.Fatal(err)
		}
		// The local one answered: the addon is the local one, and the
		// public host is never asked for anything of it.
		if addon.Network != NetworkLocal || addon.Manifest.ID != "device" {
			t.Errorf("on %q, the manifest of %q", addon.Network, addon.Manifest.ID)
		}
		if _, err := client.Catalog(ctx, addon.ManifestURL, "movie", "all", nil, addon.Confined(false)); err != nil {
			t.Error(err)
		}
		if internet.hits.Load() != 0 || device.hits.Load() != 2 {
			t.Errorf("%d requests to the public host, %d to the local one", internet.hits.Load(), device.hits.Load())
		}
	})
}

// An addon on the local network that later answers only from a public
// address is not read from there as a local addon: its requests fail
// until its manifest is read again, which tells it is on the internet,
// from where it is confined. A new address tells it anew.
func TestALocalAddonThatMovesToTheInternetIsLocatedAgainWhenRefreshed(t *testing.T) {
	t.Parallel()
	store, _, network, client := reaching(t)
	ctx := t.Context()
	var reconfined []accounts.ID
	store.SetReconfine(func(_ context.Context, _ pgx.Tx, source *accounts.ID) error {
		reconfined = append(reconfined, *source)
		return nil
	})
	device, elsewhere, internet := newHost(t, "addon"), newHost(t, "addon"), newHost(t, "addon")
	network.Serve(t, "192.168.1.50:80", device.url)
	network.Serve(t, "203.0.113.80:80", internet.url)
	network.Name("moving.test", []string{"192.168.1.50"})
	addon, err := store.Install(ctx, Shared(), "http://moving.test/manifest.json", false)
	if err != nil || addon.Network != NetworkLocal {
		t.Fatalf("installed on %q: %v", addon.Network, err)
	}

	// Nothing answers at its local address any more, and its name now
	// answers a public one.
	device.stop()
	network.Name("moving.test", []string{"203.0.113.80"})
	if _, err := client.Catalog(ctx, addon.ManifestURL, "movie", "all", nil, addon.Confined(false)); !errors.Is(err, stremio.ErrPublicNetwork) {
		t.Errorf("asked on the internet as a local addon: %v", err)
	}
	if internet.hits.Load() != 0 {
		t.Fatalf("the public host was asked %d times with a local addon's reach", internet.hits.Load())
	}
	// What is kept of an addon follows it only when where it is changes.
	if len(reconfined) != 0 {
		t.Errorf("reconfined %v before anything changed", reconfined)
	}

	addon, _, err = store.Refresh(ctx, Shared(), addon.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if addon.Network != NetworkPublic || !addon.Confined(false) || !confinedInSQL(t, store, addon.ID) {
		t.Errorf("after its refresh: on %q, confined %v", addon.Network, addon.Confined(false))
	}
	if len(reconfined) != 1 || reconfined[0] != addon.ID {
		t.Errorf("reconfined %v", reconfined)
	}
	if _, err := client.Catalog(ctx, addon.ManifestURL, "movie", "all", nil, addon.Confined(false)); err != nil {
		t.Error(err)
	}

	// A refresh that reaches it nowhere keeps what was told.
	internet.down.Store(true)
	if _, _, err := store.Refresh(ctx, Shared(), addon.ID, false); !errors.Is(err, stremio.ErrUnreachable) {
		t.Errorf("refreshed while down: %v", err)
	}
	if found, err := store.InScope(ctx, Shared(), addon.ID); err != nil || found.Network != NetworkPublic {
		t.Errorf("after a failed refresh: on %q %v", found.Network, err)
	}
	if len(reconfined) != 1 {
		t.Errorf("reconfined %v after a refresh that changed nothing", reconfined)
	}

	// Its local address installed in its place makes it a local addon.
	addon, _, err = store.Replace(ctx, Shared(), addon.ID, network.Serve(t, "192.168.1.51", elsewhere.url)+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	if addon.Network != NetworkLocal || addon.Confined(false) || len(reconfined) != 2 {
		t.Errorf("after its new address: on %q, confined %v, reconfined %v", addon.Network, addon.Confined(false), reconfined)
	}
}

// The sources installed before Polyfin kept where they are count as on
// the internet, and are located at start without anyone's action: by
// their address alone when it tells, a member's as public, the addons
// named by a host name by reading their manifest, again after a wait
// while they cannot be read.
func TestSourcesNotLocatedYetCountAsPublicAndAreLocatedAtStart(t *testing.T) {
	t.Parallel()
	store, users, network, _ := reaching(t)
	ctx := t.Context()
	bob, err := users.CreateUser(ctx, accounts.NewUser{Name: "bob", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	loopback, private, internet, named, late, bobs := newHost(t, "loopback"), newHost(t, "private"), newHost(t, "public"), newHost(t, "named"),
		newHost(t, "late"), newHost(t, "bobs")
	network.Serve(t, "192.168.1.60:80", named.url)
	network.Name("nas.test", []string{"192.168.1.60"})
	network.Serve(t, "192.168.1.61:80", late.url)
	network.Name("late.test", []string{"192.168.1.61"})
	installed := map[string]Addon{}
	for name, c := range map[string]struct {
		scope   Scope
		address string
		member  bool
	}{
		"loopback": {Shared(), loopback.url, false},
		"private":  {Shared(), network.Serve(t, "192.168.1.50", private.url), false},
		"public":   {Shared(), network.Public(t, internet.url), false},
		"named":    {Shared(), "http://nas.test", false},
		"late":     {Shared(), "http://late.test", false},
		"bobs":     {Personal(bob.ID), network.Public(t, bobs.url), true},
	} {
		addon, err := store.Install(ctx, c.scope, c.address+"/manifest.json", c.member)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		installed[name] = addon
	}
	folder, err := store.Create(ctx, Shared(), KindLocal, "/media/movies", func(accounts.ID) stremio.Manifest { return stremio.Manifest{} },
		func(pgx.Tx, Addon) error { return nil })
	if err != nil || folder.Network != NetworkLocal {
		t.Fatalf("a local folder on %q: %v", folder.Network, err)
	}
	list, err := store.Create(ctx, Shared(), KindM3U, "/srv/iptv/list.m3u", func(accounts.ID) stremio.Manifest { return stremio.Manifest{} },
		func(pgx.Tx, Addon) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	provider, err := store.Create(ctx, Shared(), KindXtream, "http://provider.test/player_api.php?username=u&password=p",
		func(accounts.ID) stremio.Manifest { return stremio.Manifest{} }, func(pgx.Tx, Addon) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	// As after the upgrade: nothing tells where the sources are.
	if _, err := store.db.Exec(ctx, "UPDATE addons SET network = NULL WHERE kind <> $1", KindLocal); err != nil {
		t.Fatal(err)
	}
	for name, addon := range installed {
		found, err := store.InScope(ctx, Scope{Owner: map[bool]*accounts.ID{true: &bob.ID}[name == "bobs"]}, addon.ID)
		if err != nil || found.Network != "" || !found.Confined(false) || !confinedInSQL(t, store, addon.ID) {
			t.Fatalf("%s before it is located: on %q, confined %v: %v", name, found.Network, found.Confined(false), err)
		}
	}

	var reconfined []*accounts.ID
	store.SetReconfine(func(_ context.Context, _ pgx.Tx, source *accounts.ID) error {
		reconfined = append(reconfined, source)
		return nil
	})
	// The addon that cannot be read at first can at the second try.
	late.down.Store(true)
	var waits []time.Duration
	store.locateWait = func(_ context.Context, d time.Duration) bool {
		waits = append(waits, d)
		late.down.Store(false)
		return true
	}
	before := loopback.hits.Load() + private.hits.Load() + internet.hits.Load() + bobs.hits.Load()
	store.LocateSources(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)))

	want := map[string]Network{"loopback": NetworkLocal, "private": NetworkLocal, "public": NetworkPublic, "named": NetworkLocal,
		"late": NetworkLocal, "bobs": NetworkPublic}
	for name, addon := range installed {
		found, err := store.Find(ctx, addon.ID)
		if err != nil || found.Network != want[name] {
			t.Errorf("%s: on %q %v", name, found.Network, err)
		}
	}
	// No request told where the address literals and the member's are.
	if n := loopback.hits.Load() + private.hits.Load() + internet.hits.Load() + bobs.hits.Load() - before; n != 0 {
		t.Errorf("%d requests to sources whose address tells where they are", n)
	}
	if len(waits) != 1 || waits[0] != locateDelays[0] {
		t.Errorf("waits %v", waits)
	}
	// A list that is a file is on the server; an IPTV source named by a
	// host name is left to its list's refresh.
	if found, _ := store.Find(ctx, list.ID); found.Network != NetworkLocal {
		t.Errorf("a list that is a file: on %q", found.Network)
	}
	if found, _ := store.Find(ctx, provider.ID); found.Network != "" {
		t.Errorf("an IPTV source named by a host name: on %q", found.Network)
	}
	// What is kept follows: of every source once the addresses told, then
	// of each addon read.
	if len(reconfined) != 3 || reconfined[0] != nil || reconfined[1] == nil || *reconfined[1] != installed["named"].ID ||
		reconfined[2] == nil || *reconfined[2] != installed["late"].ID {
		t.Errorf("reconfined %v", reconfined)
	}

	// At the next start, only the IPTV source is left untold, for its
	// list's refresh: no addon is read, and what is kept is checked again
	// while a source still counts as public for not being told.
	reconfined, waits = nil, nil
	store.LocateSources(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if len(waits) != 0 || len(reconfined) != 1 || reconfined[0] != nil {
		t.Errorf("at the next start: waits %v, reconfined %v", waits, reconfined)
	}
}

// The rule in SQL is the rule in Go, for every owner and every network.
func TestTheRuleInSQLIsTheRuleInGo(t *testing.T) {
	t.Parallel()
	pool := migrated.New(t)
	ctx := t.Context()
	var root, bob accounts.ID
	if err := pool.QueryRow(ctx, `INSERT INTO users (name, password_hash, is_administrator) VALUES ('root', 'x', true) RETURNING id`).Scan(&root); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (name, password_hash, is_administrator) VALUES ('bob', 'x', false) RETURNING id`).Scan(&bob); err != nil {
		t.Fatal(err)
	}
	position := 0
	for _, owner := range []struct {
		name          string
		id            *accounts.ID
		administrator bool
	}{{"the server", nil, false}, {"an administrator", &root, true}, {"a member", &bob, false}} {
		for _, network := range []Network{"", NetworkLocal, NetworkPublic} {
			position++
			var id accounts.ID
			if err := pool.QueryRow(ctx, `INSERT INTO addons (owner_id, kind, manifest_url, manifest, position, network)
				VALUES ($1, 'stremio', $2, '{}', $3, $4) RETURNING id`, owner.id, "http://addon.test/"+string(network)+owner.name, position,
				network.stored()).Scan(&id); err != nil {
				t.Fatal(err)
			}
			var member, confined bool
			if err := pool.QueryRow(ctx, "SELECT "+MemberSQL("a")+", "+ConfinedSQL("a")+" FROM addons a WHERE a.id = $1", id).
				Scan(&member, &confined); err != nil {
				t.Fatal(err)
			}
			scope := Scope{Owner: owner.id}
			wantMember := Member(scope, owner.administrator)
			want := Addon{Network: network}.Confined(wantMember)
			if member != wantMember || confined != want {
				t.Errorf("%s's addon on %q: member %v and confined %v in SQL, %v and %v in Go", owner.name, network, member, confined, wantMember, want)
			}
			// Only a source on a local network that is no member's may
			// reach it.
			if want != (owner.id == &bob || network != NetworkLocal) {
				t.Errorf("%s's addon on %q: confined %v", owner.name, network, want)
			}
		}
	}
}

// Locate reads a source where its address says it is, a host name from
// the network known for it first, and tells the failure of the network
// the source is on.
func TestLocateReadsASourceWhereItIs(t *testing.T) {
	t.Parallel()
	failed := errors.New("HTTP 500")
	for _, c := range []struct {
		name     string
		address  string
		confined bool
		known    Network
		// answers are what the reads confined to public addresses, and
		// from local ones, return.
		public, local error
		reads         []bool
		network       Network
		err           error
	}{
		{"a member's", "http://addon.test/manifest.json", true, "", nil, nil, []bool{true}, NetworkPublic, nil},
		{"a member's on a local address", "http://192.168.1.5/manifest.json", true, "", stremio.ErrPrivateNetwork, nil, []bool{true}, "",
			stremio.ErrPrivateNetwork},
		{"a local address", "http://192.168.1.5/manifest.json", false, NetworkPublic, nil, nil, []bool{false}, NetworkLocal, nil},
		{"a public address", "http://203.0.113.5/manifest.json", false, NetworkLocal, nil, nil, []bool{true}, NetworkPublic, nil},
		{"a public address that fails", "http://203.0.113.5/manifest.json", false, "", failed, nil, []bool{true}, "", failed},
		{"a file of the server", "/srv/iptv/list.m3u", false, "", nil, nil, []bool{false}, NetworkLocal, nil},
		{"a local name", "http://nas.test/manifest.json", false, "", stremio.ErrPrivateNetwork, nil, []bool{false}, NetworkLocal, nil},
		{"a public name", "http://addon.test/manifest.json", false, "", nil, stremio.ErrPublicNetwork, []bool{false, true}, NetworkPublic, nil},
		{"a public name known for one", "http://addon.test/manifest.json", false, NetworkPublic, nil, stremio.ErrPublicNetwork, []bool{true},
			NetworkPublic, nil},
		{"a name known for local that is public now", "http://addon.test/manifest.json", false, NetworkLocal, nil, stremio.ErrPublicNetwork,
			[]bool{false, true}, NetworkPublic, nil},
		{"a name known for public that is local now", "http://addon.test/manifest.json", false, NetworkPublic, stremio.ErrPrivateNetwork, nil,
			[]bool{true, false}, NetworkLocal, nil},
		{"a local name that fails", "http://nas.test/manifest.json", false, "", stremio.ErrPrivateNetwork, failed, []bool{false, true}, "", failed},
		{"a public name that fails", "http://addon.test/manifest.json", false, "", failed, stremio.ErrPublicNetwork, []bool{false, true}, "", failed},
		{"a public name known for one that fails", "http://addon.test/manifest.json", false, NetworkPublic, failed, stremio.ErrPublicNetwork,
			[]bool{true, false}, "", failed},
	} {
		var reads []bool
		value, network, err := Locate(t.Context(), c.address, c.confined, c.known, func(confined bool) (string, error) {
			reads = append(reads, confined)
			if confined {
				return "public", c.public
			}
			return "local", c.local
		})
		if network != c.network || !errors.Is(err, c.err) || c.err == nil && err != nil {
			t.Errorf("%s: on %q: %v", c.name, network, err)
		}
		if len(reads) != len(c.reads) || reads[0] != c.reads[0] || len(reads) == 2 && reads[1] != c.reads[1] {
			t.Errorf("%s: read %v", c.name, reads)
		}
		if err == nil && value != string(network) {
			t.Errorf("%s: the %s read told %q", c.name, value, network)
		}
	}
}
