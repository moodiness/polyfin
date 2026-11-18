package library

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/stremio/reachtest"
)

// reachEnv is an env with three addons of the same catalogs: the server's
// on a public address, the server's on the local network, and the
// member's own on the local network, installed while they were an
// administrator. Only the server's local one may reach the local network.
type reachEnv struct {
	env
	network                 *reachtest.Network
	public, local, members  *fakeAddon
	publicID, localID, mine addons.Addon
}

// reachAddon is an addon named name with a catalog of movies, one of
// other videos and one of channels, each of one title.
func reachAddon(name string) *fakeAddon {
	id := strings.ToLower(name)
	return &fakeAddon{
		manifest: stremio.Manifest{ID: id, Name: name, Version: "1", Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: name + " Top"}, {Type: "other", ID: "clips", Name: name + " Clips"},
				{Type: "tv", ID: "live", Name: name + " Live"}}},
		catalogs: map[string][]stremio.Meta{
			"movie/top":   {{ID: "tt" + id, Type: "movie", Name: name + " movie", Poster: "http://192.168.1.50/" + id + ".jpg"}},
			"other/clips": {{ID: id + ":clip", Type: "other", Name: name + " clip", Poster: "http://192.168.1.50/" + id + "-clip.jpg"}},
			"tv/live":     {{ID: id + ":channel", Type: "tv", Name: name + " channel"}},
		},
	}
}

func newReachEnv(t *testing.T) reachEnv {
	t.Helper()
	e := reachEnv{env: newEnv(t), public: reachAddon("Public"), local: reachAddon("Local"), members: reachAddon("Members")}
	e.network = reachtest.New(e.service.client)
	e.setting(func(s *accounts.Settings) { s.PersonalAddons = true })
	serve := func(addon *fakeAddon) string {
		server := httptest.NewServer(addon)
		t.Cleanup(server.Close)
		return server.URL
	}
	install := func(scope addons.Scope, address string) addons.Addon {
		installed, err := e.addons.Install(t.Context(), scope, address+"/manifest.json", false)
		if err != nil {
			t.Fatal(err)
		}
		return installed
	}
	e.publicID = install(addons.Shared(), e.network.Public(t, serve(e.public)))
	e.localID = install(addons.Shared(), serve(e.local))
	e.mine = install(addons.Personal(e.member.ID), serve(e.members))
	if e.publicID.Network != addons.NetworkPublic || e.localID.Network != addons.NetworkLocal || e.mine.Network != addons.NetworkLocal {
		t.Fatalf("installed on %q, %q and %q", e.publicID.Network, e.localID.Network, e.mine.Network)
	}
	return e
}

// asked counts the requests the addon received for a resource since the
// last call for it.
func (a *fakeAddon) asked(resource string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	count := 0
	kept := a.requests[:0]
	for _, request := range a.requests {
		if request == resource || strings.HasPrefix(request, resource+"/") {
			count++
		} else {
			kept = append(kept, request)
		}
	}
	a.requests = kept
	return count
}

// reached checks which of the three addons a reader asked for resource:
// the server's two, each through the client of its network, and never the
// member's, which is on the local network.
func (e reachEnv) reached(t *testing.T, reader, resource string) {
	t.Helper()
	if public, local, members := e.public.asked(resource), e.local.asked(resource), e.members.asked(resource); public == 0 || local == 0 || members != 0 {
		t.Errorf("%s asked the server's public addon %d times, its local one %d times, the member's local one %d times", reader, public, local, members)
	}
}

// Everything that reads an addon reads it as the one rule tells (see
// addons.Addon.Confined): a server addon on a public address is asked
// confined, one on the local network from there, and a member's own
// confined wherever it is, so one on the local network is never asked.
// Each reader is a route or a job that derives the rule on its own.
func TestEveryReaderOfAnAddonFollowsWhereItIs(t *testing.T) {
	t.Parallel()

	t.Run("a user's view", func(t *testing.T) {
		t.Parallel()
		e := newReachEnv(t)
		confined := map[string]bool{"Public": true, "Local": false}
		for name, want := range confined {
			page, err := e.service.Children(t.Context(), e.member, e.library(e.member, name+" Top").ID, 0, 10, "")
			if err != nil || len(page.Items) != 1 {
				t.Fatalf("%s: %v %v", name, page.Items, err)
			}
			// What the addon names is reached as the addon is.
			artwork, err := e.service.Artwork(t.Context(), page.Items[0].ID, "Primary")
			if err != nil || artwork.Confined != want {
				t.Errorf("the poster of %s: confined %v: %v", name, artwork.Confined, err)
			}
		}
		if page, err := e.service.Children(t.Context(), e.member, e.library(e.member, "Members Top").ID, 0, 10, ""); err == nil && len(page.Items) > 0 {
			t.Errorf("the member's local addon listed %v", page.Items)
		}
		e.reached(t, "a view", "catalog/movie/top")
	})

	t.Run("the first pages read ahead", func(t *testing.T) {
		t.Parallel()
		e := newReachEnv(t)
		if err := e.service.Warm(t.Context()); err != nil {
			t.Fatal(err)
		}
		e.reached(t, "Warm", "catalog/movie/top")
	})

	t.Run("how a library of other videos shows its titles", func(t *testing.T) {
		t.Parallel()
		e := newReachEnv(t)
		clips := func(addon addons.Addon) addons.LibraryChoice {
			return addons.LibraryChoice{AddonID: addon.ID, CatalogType: "other", CatalogID: "clips"}
		}
		if _, err := e.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{clips(e.publicID), clips(e.localID)}); err != nil {
			t.Fatal(err)
		}
		if _, err := e.addons.SetLibraries(t.Context(), addons.Personal(e.member.ID), []addons.LibraryChoice{clips(e.mine)}); err != nil {
			t.Fatal(err)
		}
		for _, scope := range []addons.Scope{addons.Shared(), addons.Personal(e.member.ID)} {
			if err := e.service.DecideShowAs(t.Context(), scope); err != nil {
				t.Fatal(err)
			}
		}
		e.reached(t, "DecideShowAs", "catalog/other/clips")
	})

	t.Run("the images of libraries", func(t *testing.T) {
		t.Parallel()
		e := newReachEnv(t)
		for scope, installed := range map[*accounts.ID][]addons.Addon{nil: {e.publicID, e.localID}, &e.member.ID: {e.mine}} {
			scope := addons.Scope{Owner: scope}
			for _, addon := range installed {
				key := addons.LibraryKey{AddonID: addon.ID, CatalogType: "movie", CatalogID: "top"}
				if err := e.addons.SetLibraryImage(t.Context(), scope, key, addons.LibraryImageAutomatic); err != nil {
					t.Fatal(err)
				}
			}
			libraries, err := e.addons.Libraries(t.Context(), scope)
			if err != nil {
				t.Fatal(err)
			}
			// The admin API gives who asks: a member for their own scope.
			if _, err := e.service.LibraryImages(t.Context(), scope, scope.Owner != nil, libraries); err != nil {
				t.Fatal(err)
			}
		}
		e.reached(t, "LibraryImages", "catalog/movie/top")
	})

	t.Run("the configure hints read at start", func(t *testing.T) {
		t.Parallel()
		e := newReachEnv(t)
		e.service.hintSpacing = 0
		if _, err := e.service.db.Exec(t.Context(), "UPDATE addons SET hints_read = false"); err != nil {
			t.Fatal(err)
		}
		e.public.asked("manifest")
		e.local.asked("manifest")
		e.members.asked("manifest")
		e.service.ReadConfigureHints(t.Context())
		e.reached(t, "ReadConfigureHints", "manifest")
	})

	t.Run("the channels of a live TV catalog", func(t *testing.T) {
		t.Parallel()
		e := newReachEnv(t)
		for scope, installed := range map[*accounts.ID][]addons.Addon{nil: {e.publicID, e.localID}, &e.member.ID: {e.mine}} {
			for _, addon := range installed {
				key := addons.LibraryKey{AddonID: addon.ID, CatalogType: "tv", CatalogID: "live"}
				c, err := e.addons.LiveCatalogOf(t.Context(), addons.Scope{Owner: scope}, key)
				if err != nil {
					t.Fatal(err)
				}
				// A guide's address is its owner's own: only a member's
				// stays on public addresses.
				if c.Member != (scope != nil) || c.Confined != (addon.ID != e.localID.ID) {
					t.Errorf("the live catalog of %s: member %v, confined %v", addon.Manifest.Name, c.Member, c.Confined)
				}
				_, _ = e.service.Automap(t.Context(), addons.Scope{Owner: scope}, key, AutomapUnmapped)
			}
		}
		e.reached(t, "Automap", "catalog/tv/live")
	})
}

// itemsConfined reads, for each of the titles named, whether its item is
// kept confined.
func (e reachEnv) itemsConfined(t *testing.T) map[string]bool {
	t.Helper()
	rows, err := e.service.db.Query(t.Context(), `SELECT data->'meta'->>'name', coalesce((data->>'confined')::boolean, false) FROM items
		WHERE kind = 'movie'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	kept := map[string]bool{}
	for rows.Next() {
		var name string
		var confined bool
		if err := rows.Scan(&name, &confined); err != nil {
			t.Fatal(err)
		}
		kept[name] = confined
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return kept
}

// What was kept of a source before Polyfin told where sources are follows
// the rule as soon as the sources are located at start, without being
// listed again: the titles of a server addon on a public address, kept
// free to reach the local network, are confined; those of an addon on the
// local network stay free; and a title follows its addon when a new
// address moves it to the other network.
func TestWhatWasKeptOfASourceFollowsWhereItIs(t *testing.T) {
	t.Parallel()
	e := newReachEnv(t)
	ctx := t.Context()
	e.addons.SetReconfine(e.service.Reconfine)
	var posters = map[string]accounts.ID{}
	for _, name := range []string{"Public", "Local"} {
		page, err := e.service.Children(ctx, e.admin, e.library(e.admin, name+" Top").ID, 0, 10, "")
		if err != nil || len(page.Items) != 1 {
			t.Fatalf("%s: %v %v", name, page.Items, err)
		}
		posters[name] = page.Items[0].ID
	}
	// As before the upgrade: nothing tells where the sources are, and the
	// titles of the server's addons were kept free to reach any address.
	// The member's own, never listed for being on the local network, has
	// a title kept from when it could be.
	mine, err := json.Marshal(e.mine.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.db.Exec(ctx, `INSERT INTO items (id, key, kind, data) VALUES (gen_random_uuid(), 'movie|ttmembers', 'movie',
		jsonb_build_object('kind', 'movie', 'addon', $1::jsonb, 'meta', '{"id": "ttmembers", "type": "movie", "name": "Members movie"}'::jsonb))`,
		string(mine)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.db.Exec(ctx, "UPDATE items SET data = data - 'confined'"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.db.Exec(ctx, "UPDATE addons SET network = NULL"); err != nil {
		t.Fatal(err)
	}
	if kept := e.itemsConfined(t); kept["Public movie"] || kept["Local movie"] || kept["Members movie"] {
		t.Fatalf("kept before the upgrade: %v", kept)
	}

	e.addons.LocateSources(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if kept := e.itemsConfined(t); !kept["Public movie"] || kept["Local movie"] || !kept["Members movie"] {
		t.Errorf("kept once the sources are located: %v", kept)
	}
	for name, want := range map[string]bool{"Public": true, "Local": false} {
		if artwork, err := e.service.Artwork(ctx, posters[name], "Primary"); err != nil || artwork.Confined != want {
			t.Errorf("the poster of %s: confined %v: %v", name, artwork.Confined, err)
		}
	}
	// The public addon installed by a local address is a local one: its
	// titles are free again, at once.
	server := httptest.NewServer(e.public)
	t.Cleanup(server.Close)
	moved, _, err := e.addons.Replace(ctx, addons.Shared(), e.publicID.ID, server.URL+"/manifest.json", false)
	if err != nil || moved.Network != addons.NetworkLocal {
		t.Fatalf("moved to %q: %v", moved.Network, err)
	}
	if kept := e.itemsConfined(t); kept["Public movie"] || kept["Local movie"] || !kept["Members movie"] {
		t.Errorf("kept once the addon is local: %v", kept)
	}
	// And back, with an address on the internet.
	away := e.network.Public(t, server.URL)
	moved, _, err = e.addons.Replace(ctx, addons.Shared(), e.publicID.ID, away+"/manifest.json", false)
	if err != nil || moved.Network != addons.NetworkPublic {
		t.Fatalf("moved to %q: %v", moved.Network, err)
	}
	if kept := e.itemsConfined(t); !kept["Public movie"] || kept["Local movie"] {
		t.Errorf("kept once the addon is public again: %v", kept)
	}
}

// What is kept without a source keeps its confinement, whatever the
// sources become: a title whose addon was removed, kept confined, stays
// so through the pass at start and through a source's move, and so does
// one kept free. A season's and an episode's own confinement, which may
// come from another addon than the one that listed their series, is set
// when their source is confined and never cleared.
func TestRecordsWithoutASourceAndEpisodesKeepTheirConfinement(t *testing.T) {
	t.Parallel()
	e := newReachEnv(t)
	ctx := t.Context()
	e.addons.SetReconfine(e.service.Reconfine)
	public, err := json.Marshal(e.publicID.ID)
	if err != nil {
		t.Fatal(err)
	}
	local, err := json.Marshal(e.localID.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.db.Exec(ctx, `INSERT INTO items (id, key, kind, data) VALUES
		(gen_random_uuid(), 'kept|released confined', 'movie', '{"kind": "movie", "confined": true}'),
		(gen_random_uuid(), 'kept|released free', 'movie', '{"kind": "movie"}'),
		(gen_random_uuid(), 'kept|person confined', 'person', '{"kind": "person", "person": {"name": "Someone"}, "confined": true}'),
		(gen_random_uuid(), 'kept|local season confined', 'season', jsonb_build_object('kind', 'season', 'addon', $2::jsonb, 'confined', true)),
		(gen_random_uuid(), 'kept|local episode confined', 'episode', jsonb_build_object('kind', 'episode', 'addon', $2::jsonb, 'confined', true)),
		(gen_random_uuid(), 'kept|local episode free', 'episode', jsonb_build_object('kind', 'episode', 'addon', $2::jsonb)),
		(gen_random_uuid(), 'kept|public episode free', 'episode', jsonb_build_object('kind', 'episode', 'addon', $1::jsonb)),
		(gen_random_uuid(), 'kept|public movie free', 'movie', jsonb_build_object('kind', 'movie', 'addon', $1::jsonb))`,
		string(public), string(local)); err != nil {
		t.Fatal(err)
	}
	kept := func() map[string]bool {
		t.Helper()
		rows, err := e.service.db.Query(ctx, `SELECT substr(key, 6), coalesce((data->>'confined')::boolean, false) FROM items WHERE starts_with(key, 'kept|')`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		result := map[string]bool{}
		for rows.Next() {
			var name string
			var confined bool
			if err := rows.Scan(&name, &confined); err != nil {
				t.Fatal(err)
			}
			result[name] = confined
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return result
	}
	check := func(when string, want map[string]bool) {
		t.Helper()
		got := kept()
		for name, confined := range want {
			if got[name] != confined {
				t.Errorf("%s, %s: confined %v", when, name, got[name])
			}
		}
	}

	// The pass at start, as after the upgrade.
	if _, err := e.service.db.Exec(ctx, "UPDATE addons SET network = NULL"); err != nil {
		t.Fatal(err)
	}
	e.addons.LocateSources(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)))
	check("after the pass at start", map[string]bool{"released confined": true, "released free": false, "person confined": true,
		"local season confined": true, "local episode confined": true, "local episode free": false, "public episode free": true,
		"public movie free": true})

	// The public addon becomes a local one: its movie is free again, its
	// episode keeps the confinement it got; nothing else changes.
	server := httptest.NewServer(e.public)
	t.Cleanup(server.Close)
	if moved, _, err := e.addons.Replace(ctx, addons.Shared(), e.publicID.ID, server.URL+"/manifest.json", false); err != nil ||
		moved.Network != addons.NetworkLocal {
		t.Fatalf("moved to %q: %v", moved.Network, err)
	}
	check("once the public addon is local", map[string]bool{"released confined": true, "released free": false, "person confined": true,
		"local season confined": true, "local episode confined": true, "local episode free": false, "public episode free": true,
		"public movie free": false})

	// The local addon becomes a public one: its episodes are confined.
	elsewhere := httptest.NewServer(e.local)
	t.Cleanup(elsewhere.Close)
	if moved, _, err := e.addons.Replace(ctx, addons.Shared(), e.localID.ID, e.network.Public(t, elsewhere.URL)+"/manifest.json", false); err != nil ||
		moved.Network != addons.NetworkPublic {
		t.Fatalf("moved to %q: %v", moved.Network, err)
	}
	check("once the local addon is public", map[string]bool{"released confined": true, "released free": false, "person confined": true,
		"local season confined": true, "local episode confined": true, "local episode free": true, "public episode free": true,
		"public movie free": false})
}

// A guide's address is its owner's own, not its source's: an
// administrator's guide on the local network is downloaded for a live TV
// catalog of a server addon on a public address, while a member's must be
// public, wherever their addon is.
func TestAGuidesAddressIsItsOwnersWhateverTheSource(t *testing.T) {
	t.Parallel()
	e := newReachEnv(t)
	ctx := t.Context()
	guide := newGuideServer(t, `<tv><channel id="1"><display-name>Public channel</display-name></channel></tv>`)
	live := func(addon addons.Addon) addons.LibraryKey {
		return addons.LibraryKey{AddonID: addon.ID, CatalogType: "tv", CatalogID: "live"}
	}

	guides, err := e.service.SetCatalogGuides(ctx, addons.Shared(), live(e.publicID), []addons.GuideAddress{{URL: guide.url}})
	if err != nil || len(guides.Guides) != 1 || guides.Guides[0].Error != "" || guide.downloads.Load() != 1 {
		t.Errorf("an administrator's local guide for a public addon's catalog: %+v %v, %d downloads", guides.Guides, err, guide.downloads.Load())
	}
	if _, err := e.service.SetCatalogGuides(ctx, addons.Personal(e.member.ID), live(e.mine), []addons.GuideAddress{{URL: guide.url}}); !errors.Is(err,
		stremio.ErrPrivateNetwork) || guide.downloads.Load() != 1 {
		t.Errorf("a member's local guide: %v, %d downloads", err, guide.downloads.Load())
	}
}
