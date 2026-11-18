package iptv

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/stremio/reachtest"
)

// reachPlaylist is a list of one channel whose logo and stream are on the
// local network.
const reachPlaylist = "#EXTM3U\n#EXTINF:-1 tvg-logo=\"http://192.168.1.50/zeb.png\" group-title=\"News\",Zeb One\nhttp://192.168.1.50/zeb.ts\n"

// networkOf reads where the source id is told to be.
func (e env) networkOf(id accounts.ID) addons.Network {
	e.t.Helper()
	found, err := e.addons.Find(e.t.Context(), id)
	if err != nil {
		e.t.Fatal(err)
	}
	return found.Network
}

// zeb is the item of the one channel of a source added from reachPlaylist.
func (e env) zeb(scope addons.Scope, source accounts.ID) Channel {
	e.t.Helper()
	_, channels, err := e.service.ListChannels(e.t.Context(), scope, source, ChannelFilter{Limit: 10})
	if err != nil || len(channels) != 1 {
		e.t.Fatalf("channels %v: %v", channels, err)
	}
	return channels[0]
}

// An IPTV source is where its list was read from: only one on the local
// network, the server's or an administrator's own, may name addresses of
// the local network, as its channels' logos. A member's list is read from
// public addresses only, and a list that is a file is on the server.
func TestAnIPTVSourceIsWhereItsListWasReadFrom(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	network := reachtest.New(e.service.client)
	ctx := t.Context()
	bob, err := e.users.CreateUser(ctx, accounts.NewUser{Name: "bob", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "list.m3u")
	if err := os.WriteFile(file, []byte(reachPlaylist), 0o600); err != nil {
		t.Fatal(err)
	}
	named, device := newPlaylistServer(t, reachPlaylist), newPlaylistServer(t, "")
	device.set("", 404)
	network.Serve(t, "203.0.113.80:80", named.url)
	network.Serve(t, "192.168.1.60:80", device.url)
	network.Name("lists.test", []string{"192.168.1.60", "203.0.113.80"})
	for _, c := range []struct {
		name     string
		scope    addons.Scope
		member   bool
		address  func(list *playlistServer) string
		network  addons.Network
		confined bool
		refused  error
	}{
		{"the server's, on the loopback", addons.Shared(), false, func(l *playlistServer) string { return l.url }, addons.NetworkLocal, false, nil},
		{"the server's, on a public address", addons.Shared(), false, func(l *playlistServer) string { return network.Public(t, l.url) + "/list.m3u" },
			addons.NetworkPublic, true, nil},
		{"the server's, a file", addons.Shared(), false, func(*playlistServer) string { return file }, addons.NetworkLocal, false, nil},
		{"the server's, a name that answers a local device and the list on the internet", addons.Shared(), false,
			func(*playlistServer) string { return "http://lists.test/list.m3u" }, addons.NetworkPublic, true, nil},
		{"a member's, on a public address", addons.Personal(bob.ID), true, func(l *playlistServer) string { return network.Public(t, l.url) + "/list.m3u" },
			addons.NetworkPublic, true, nil},
		{"a member's, on the loopback", addons.Personal(bob.ID), true, func(l *playlistServer) string { return l.url }, "", true,
			stremio.ErrPrivateNetwork},
	} {
		list := newPlaylistServer(t, reachPlaylist)
		addon, err := e.service.Add(ctx, c.scope, NewSource{Name: "TV", Account: Account{Kind: addons.KindM3U, URL: c.address(list)}}, c.member)
		if c.refused != nil {
			if !errors.Is(err, c.refused) || list.downloads.Load() != 0 {
				t.Errorf("%s: %v, %d downloads", c.name, err, list.downloads.Load())
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if addon.Network != c.network || e.networkOf(addon.ID) != c.network {
			t.Errorf("%s: on %q, kept on %q", c.name, addon.Network, e.networkOf(addon.ID))
		}
		// What its list names is reached as a source there may.
		logo, confined, err := e.service.Logo(ctx, e.zeb(c.scope, addon.ID).ID)
		if err != nil || logo != "http://192.168.1.50/zeb.png" || confined != c.confined {
			t.Errorf("%s: the logo %q is confined %v: %v", c.name, logo, confined, err)
		}
		if member, confined, err := e.service.reach(ctx, addon.ID); err != nil || member != c.member || confined != c.confined {
			t.Errorf("%s: member %v, confined %v: %v", c.name, member, confined, err)
		}
	}
	if device.downloads.Load() != 1 || named.downloads.Load() != 1 {
		t.Errorf("the name: %d requests to the local device, %d to the list on the internet", device.downloads.Load(), named.downloads.Load())
	}
}

// A source on the local network whose list later answers only from the
// internet is not read from there as a local source: its refresh tells
// where it is again, and what it names is confined from then on. A source
// that does not tell where it is yet is due at once, without waiting for
// its next refresh, and counts as on the internet until its list is read.
func TestAnIPTVSourceIsLocatedAgainByItsRefresh(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	network := reachtest.New(e.service.client)
	ctx, shared := t.Context(), addons.Shared()
	var reconfined int
	e.addons.SetReconfine(func(context.Context, pgx.Tx, *accounts.ID) error { reconfined++; return nil })
	device, internet := newPlaylistServer(t, reachPlaylist), newPlaylistServer(t, reachPlaylist)
	network.Serve(t, "192.168.1.60:80", device.url)
	network.Serve(t, "203.0.113.80:80", internet.url)
	network.Name("lists.test", []string{"192.168.1.60"})
	addon := must(e.service.Add(ctx, shared, NewSource{Name: "TV", Account: Account{Kind: addons.KindM3U, URL: "http://lists.test/list.m3u"}}, false))
	channel := e.zeb(shared, addon.ID).ID
	if _, confined, err := e.service.Logo(ctx, channel); err != nil || confined || e.networkOf(addon.ID) != addons.NetworkLocal {
		t.Fatalf("added on %q, its logo confined %v: %v", e.networkOf(addon.ID), confined, err)
	}

	// The local device no longer has the list; the name answers a public
	// address, where one is.
	device.set("", 404)
	network.Name("lists.test", []string{"203.0.113.80"})
	if err := e.service.Refresh(ctx, shared, addon.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, confined, err := e.service.Logo(ctx, channel); err != nil || !confined || e.networkOf(addon.ID) != addons.NetworkPublic {
		t.Errorf("refreshed on %q, its logo confined %v: %v", e.networkOf(addon.ID), confined, err)
	}
	if source := must(e.service.Source(ctx, shared, addon.ID)); source.Error != "" {
		t.Errorf("the refresh failed: %s", source.Error)
	}

	// As after the upgrade: the source does not tell where it is.
	if _, err := e.service.db.Exec(ctx, "UPDATE addons SET network = NULL WHERE id = $1", addon.ID); err != nil {
		t.Fatal(err)
	}
	if _, confined, err := e.service.Logo(ctx, channel); err != nil || !confined {
		t.Errorf("before it is located, its logo is confined %v: %v", confined, err)
	}
	before := internet.downloads.Load()
	for range 2 {
		if err := e.service.RefreshDue(ctx, false); err != nil {
			t.Fatal(err)
		}
	}
	if n := internet.downloads.Load() - before; n != 1 || e.networkOf(addon.ID) != addons.NetworkPublic {
		t.Errorf("%d downloads by the refreshes due, on %q", n, e.networkOf(addon.ID))
	}
	// Told where it was already, nothing kept is rewritten for it.
	if reconfined != 3 {
		t.Errorf("reconfined %d times", reconfined)
	}
}

// What the owner of a source gave it themselves is reached as they may,
// not as what the source names: an administrator's logo and custom stream
// for a channel of a source on the internet may be on the local network;
// a member's stay on public addresses.
func TestWhatAnOwnerGaveAnIPTVSourceIsReachedAsTheyMay(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	network := reachtest.New(e.service.client)
	ctx := t.Context()
	bob, err := e.users.CreateUser(ctx, accounts.NewUser{Name: "bob", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name   string
		scope  addons.Scope
		member bool
	}{{"the server's", addons.Shared(), false}, {"a member's", addons.Personal(bob.ID), true}} {
		list := newPlaylistServer(t, reachPlaylist)
		addon, err := e.service.Add(ctx, c.scope, NewSource{Name: "TV", Account: Account{Kind: addons.KindM3U, URL: network.Public(t, list.url) + "/list.m3u"}},
			c.member)
		if err != nil {
			t.Fatal(err)
		}
		channel := e.zeb(c.scope, addon.ID)
		streams, err := e.service.Streams(ctx, addon.ID, prefix(addon.ID)+channel.key)
		if err != nil || len(streams) != 1 || streams[0].Confined != nil {
			t.Fatalf("%s source's streams: %+v %v", c.name, streams, err)
		}
		if _, err := e.service.AddStream(ctx, c.scope, addon.ID, channel.ID, "http://203.0.113.9/zeb.ts", "Mine", c.member); err != nil {
			t.Fatal(err)
		}
		streams, err = e.service.Streams(ctx, addon.ID, prefix(addon.ID)+channel.key)
		if err != nil || len(streams) != 2 {
			t.Fatalf("%s source's streams: %+v %v", c.name, streams, err)
		}
		for _, stream := range streams {
			switch {
			case stream.URL == "http://192.168.1.50/zeb.ts" && stream.Confined == nil:
			case stream.URL == "http://203.0.113.9/zeb.ts" && stream.Confined != nil && *stream.Confined == c.member:
			default:
				t.Errorf("%s source's stream %s: %v", c.name, stream.URL, stream.Confined)
			}
		}
		if _, confined, err := e.service.Logo(ctx, channel.ID); err != nil || !confined {
			t.Errorf("%s source's own logo is confined %v: %v", c.name, confined, err)
		}
		if _, err := e.service.UpdateChannel(ctx, c.scope, addon.ID, channel.ID, ChannelChanges{
			Logo: &Nullable[string]{Value: new("http://203.0.113.9/mine.png")}}); err != nil {
			t.Fatal(err)
		}
		if logo, confined, err := e.service.Logo(ctx, channel.ID); err != nil || logo != "http://203.0.113.9/mine.png" || confined != c.member {
			t.Errorf("the logo chosen for %s source's channel, %q, is confined %v: %v", c.name, logo, confined, err)
		}
	}
}

// Everything that asks an Xtream account asks it as the one rule tells:
// the refreshes due, a title's details and a preview of what is not
// imported reach the server's account on a public address confined, its
// account on the local network from there, and never a member's on the
// local network.
func TestEveryReaderOfAnIPTVSourceFollowsWhereItIs(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	network := reachtest.New(e.service.client)
	ctx := t.Context()
	bob, err := e.users.CreateUser(ctx, accounts.NewUser{Name: "bob", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	type account struct {
		name    string
		scope   addons.Scope
		server  *syntheticServer
		source  accounts.ID
		reached bool
	}
	movies := true
	add := func(name string, scope addons.Scope, public bool) *account {
		server, credentials := syntheticVOD(t, 3, 1)
		if public {
			credentials.Server = network.Public(t, server.URL)
		}
		addon, err := e.service.Add(ctx, scope, NewSource{Name: name, Account: credentials, Options: &OptionsPatch{Movies: &movies}}, false)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return &account{name: name, scope: scope, server: server, source: addon.ID, reached: scope.Owner == nil}
	}
	// The member's was added while they could reach the local network.
	list := []*account{add("the server's public account", addons.Shared(), true), add("the server's local account", addons.Shared(), false),
		add("a member's local account", addons.Personal(bob.ID), false)}
	if e.networkOf(list[0].source) != addons.NetworkPublic || e.networkOf(list[1].source) != addons.NetworkLocal {
		t.Fatalf("added on %q and %q", e.networkOf(list[0].source), e.networkOf(list[1].source))
	}
	check := func(reader, action string, asked func(a *account)) {
		t.Helper()
		for _, a := range list {
			before := a.server.count(action)
			asked(a)
			if n := a.server.count(action) - before; a.reached != (n > 0) {
				t.Errorf("%s asked %s %d times", reader, a.name, n)
			}
		}
	}

	check("a title's details", "get_vod_info", func(a *account) {
		page, err := e.service.Catalog(ctx, a.source, "movie", "movies", 0, "", "")
		if err != nil || len(page) == 0 {
			t.Fatalf("%s: %v %v", a.name, page, err)
		}
		_, _ = e.service.Meta(Opening(ctx), a.source, page[0].ID)
	})
	check("a preview of the series not imported", "get_series", func(a *account) {
		_, _, _ = e.service.Preview(ctx, a.scope, a.source, PreviewSeries, "")
	})
	e.later(e.service.interval() + 1)
	before := map[*account]int{}
	for _, a := range list {
		before[a] = a.server.count("get_live_streams")
	}
	if err := e.service.RefreshDue(ctx, false); err != nil {
		t.Fatal(err)
	}
	for _, a := range list {
		if n := a.server.count("get_live_streams") - before[a]; a.reached != (n > 0) {
			t.Errorf("the refreshes due asked %s %d times", a.name, n)
		}
	}
}
