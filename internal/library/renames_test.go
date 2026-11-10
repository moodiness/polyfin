package library

import (
	"fmt"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/smartlists"
	"github.com/moodiness/polyfin/internal/stremio"
)

// renamed returns addon's manifest with the catalogs of from given the
// identifiers of to, their names kept, and its catalogs listing the same.
func renamed(addon *fakeAddon, from map[string]string) {
	addon.mu.Lock()
	defer addon.mu.Unlock()
	for i, c := range addon.manifest.Catalogs {
		if to, ok := from[c.Type+"/"+c.ID]; ok {
			addon.catalogs[c.Type+"/"+to] = addon.catalogs[c.Type+"/"+c.ID]
			addon.manifest.Catalogs[i].ID = to
		}
	}
}

// libraryOf returns the library of the scope's catalog, enabled or not.
func (e env) libraryOf(catalogType, catalogID string) addons.Library {
	e.t.Helper()
	libraries, err := e.addons.Libraries(e.t.Context(), addons.Shared())
	if err != nil {
		e.t.Fatal(err)
	}
	for _, l := range libraries {
		if l.Catalog.Type == catalogType && l.Catalog.ID == catalogID {
			return l
		}
	}
	e.t.Fatalf("no library of %s/%s", catalogType, catalogID)
	return addons.Library{}
}

// A catalog whose identifier changes, its type, name and extra properties
// kept, keeps its library: its identifier, its settings, the users and
// groups who hide it, its home row, its place in smart lists and channel
// limits, its guides and channel choices, the titles it listed first and
// what is kept of its pages. Refresh tells it followed, and tells the
// library of a catalog gone removed. The hourly cleanup then leaves it be.
func TestARenamedCatalogKeepsItsLibrary(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.addons.SetCleanup(e.service.CleanRemoved)
	e.addons.SetCarry(e.service.CarryRenamed)
	ctx := t.Context()
	addon := leftoverAddon("ren")
	e.install(addons.Shared(), addon)
	id := e.installedID(addons.Shared(), "ren")
	e.listed(e.member, "ren top")
	if _, err := e.service.Channels(ctx, e.member); err != nil {
		t.Fatal(err)
	}
	top := e.library(e.member, "ren top").ID
	if got := LibraryID(e.libraryOf("movie", "ren-top")); got != top {
		t.Fatalf("the library's item %v, apps see %v", got, top)
	}
	channels := LibraryID(e.libraryOf("tv", "channels"))
	group, err := e.users.CreateGroup(ctx, "family", nil)
	if err != nil {
		t.Fatal(err)
	}
	list := e.smartList("mine", smartlists.Catalog{AddonID: id, Type: "movie", ID: "ren-top"})
	guide := accounts.ID{1}
	channel := itemID(channelKey("ren-channel"))
	e.exec(
		[]any{"UPDATE libraries SET name = 'My top', hide_in_menus = true, max_items = 50 WHERE addon_id = $1 AND catalog_id = 'ren-top'", id},
		[]any{"UPDATE users SET hidden_libraries = $2 WHERE id = $1", e.member.ID, []accounts.ID{top}},
		[]any{"UPDATE user_groups SET hidden_libraries = $2 WHERE id = $1", group.ID, []accounts.ID{top, channels}},
		[]any{`INSERT INTO home_rows (position, name, kind, place, addon_id, catalog_type, catalog_id) VALUES (1, 'Top', 'catalog', 'top', $1, 'movie', 'ren-top')`, id},
		[]any{`UPDATE users SET live_tv_channels = $2 WHERE id = $1`, e.member.ID, fmt.Sprintf(
			`{"mode": "except", "groups": [%q], "knownGroups": [%q], "channels": ["ren-channel"]}`, id.String()+"/channels/News", id.String()+"/channels/")},
		[]any{`INSERT INTO live_guides (id, addon_id, catalog_type, catalog_id, position, url) VALUES ($1, $2, 'tv', 'channels', 1, 'https://guide.example/a.xml')`,
			guide, id},
		[]any{`INSERT INTO live_guide_maps (addon_id, catalog_type, catalog_id, channel_id, guide_id, xmltv_id, manual) VALUES ($1, 'tv', 'channels', $2, $3, 'one', true)`,
			id, channel, guide},
		[]any{`INSERT INTO notification_health (key, severity, text, since) VALUES ($1, 'warning', 'Guide', now())`, "guide:" + id.String() + ":tv:channels"},
		[]any{`INSERT INTO catalog_pages (addon_id, catalog_type, catalog_id, genre, skip, config, metas, fetched_at)
			VALUES ($1, 'movie', 'ren-top', 'kept', 0, '', '[]', now())`, id},
		[]any{`INSERT INTO catalog_sizes (addon_id, catalog_type, catalog_id, page_size) VALUES ($1, 'movie', 'ren-top', 7)
			ON CONFLICT (addon_id, catalog_type, catalog_id) DO UPDATE SET page_size = 7`, id},
		[]any{`INSERT INTO catalog_reach (addon_id, catalog_type, catalog_id, genre, reach, read_at) VALUES ($1, 'movie', 'ren-top', 'kept', 3, now())`, id},
		[]any{`INSERT INTO catalog_arrivals (addon_id, catalog_type, catalog_id, genre, item_id, first_seen, last_seen, arrived)
			VALUES ($1, 'movie', 'ren-top', 'kept', $2, now(), now(), true)`, id, itemID(titleKey(KindMovie, "ttm000"))},
	)
	if n := e.count("SELECT count(*) FROM items WHERE key = $1 AND data->>'catalogId' = 'ren-top'", titleKey(KindMovie, "ttm000")); n != 1 {
		t.Fatal("the catalog did not list the movie first")
	}

	// The addon renames its movies and its channels, and drops its shows.
	renamed(addon, map[string]string{"movie/ren-top": "ren-top2", "tv/channels": "channels2"})
	addon.mu.Lock()
	addon.manifest.Catalogs = slices.DeleteFunc(addon.manifest.Catalogs, func(c stremio.Catalog) bool { return c.Type == "series" })
	addon.mu.Unlock()
	_, changes, err := e.addons.Refresh(ctx, addons.Shared(), id, false)
	if err != nil {
		t.Fatal(err)
	}
	wantFollowed := []addons.ChangedLibrary{{Name: "My top", CatalogType: "movie", CatalogID: "ren-top", NewID: "ren-top2"},
		{Name: "ren channels", CatalogType: "tv", CatalogID: "channels", NewID: "channels2"}}
	wantRemoved := []addons.ChangedLibrary{{Name: "ren shows", CatalogType: "series", CatalogID: "ren-shows"}}
	if !slices.Equal(changes.Followed, wantFollowed) || !slices.Equal(changes.Removed, wantRemoved) {
		t.Errorf("followed %+v, removed %+v", changes.Followed, changes.Removed)
	}

	check := func(when string) {
		t.Helper()
		l := e.libraryOf("movie", "ren-top2")
		if !l.Enabled || l.Name == nil || *l.Name != "My top" || !l.HideInMenus || l.MaxItems != 50 || LibraryID(l) != top {
			t.Errorf("%s: the library %+v, item %v", when, l, LibraryID(l))
		}
		if tv := e.libraryOf("tv", "channels2"); !tv.Enabled || LibraryID(tv) != channels || len(tv.Guides) != 1 || tv.Guides[0].ID != guide {
			t.Errorf("%s: the channels' library %+v", when, tv)
		}
		if n := e.count("SELECT count(*) FROM live_guide_maps WHERE addon_id = $1 AND catalog_id = 'channels2' AND manual", id); n != 1 {
			t.Errorf("%s: %d channel choices", when, n)
		}
		var hidden []accounts.ID
		if err := e.service.db.QueryRow(ctx, "SELECT hidden_libraries FROM users WHERE id = $1", e.member.ID).Scan(&hidden); err != nil ||
			!slices.Equal(hidden, []accounts.ID{top}) {
			t.Errorf("%s: the member hides %v: %v", when, hidden, err)
		}
		if err := e.service.db.QueryRow(ctx, "SELECT hidden_libraries FROM user_groups WHERE id = $1", group.ID).Scan(&hidden); err != nil ||
			!slices.Equal(hidden, []accounts.ID{top, channels}) {
			t.Errorf("%s: the group hides %v: %v", when, hidden, err)
		}
		if n := e.count("SELECT count(*) FROM items WHERE id = $1 AND data->>'catalogId' = 'ren-top2'", top); n != 1 {
			t.Errorf("%s: the library's item is gone or names the old catalog", when)
		}
		if n := e.count("SELECT count(*) FROM home_rows WHERE addon_id = $1 AND catalog_id = 'ren-top2'", id); n != 1 {
			t.Errorf("%s: %d home rows on the new identifier", when, n)
		}
		saved, err := e.service.smart.Get(ctx, nil, list.ID)
		if err != nil || !slices.Equal(saved.Catalogs, []smartlists.Catalog{{AddonID: id, Type: "movie", ID: "ren-top2"}}) {
			t.Errorf("%s: the smart list draws from %v: %v", when, saved.Catalogs, err)
		}
		want := fmt.Sprintf(`{"mode": "except", "groups": [%q], "channels": ["ren-channel"], "knownGroups": [%q]}`,
			id.String()+"/channels2/News", id.String()+"/channels2/")
		if n := e.count("SELECT count(*) FROM users WHERE id = $1 AND live_tv_channels @> $2::jsonb AND jsonb_array_length(live_tv_channels->'groups') = 1",
			e.member.ID, want); n != 1 {
			t.Errorf("%s: the member's channel limit does not follow", when)
		}
		if n := e.count("SELECT count(*) FROM items WHERE key = $1 AND data->>'catalogId' = 'ren-top2'", titleKey(KindMovie, "ttm000")); n != 1 {
			t.Errorf("%s: the movie is not recorded under the new identifier", when)
		}
		// The pages listed before the rename, and those set up, follow.
		for _, table := range []string{"catalog_pages", "catalog_sizes", "catalog_reach", "catalog_arrivals"} {
			if n := e.count("SELECT count(*) FROM "+table+" WHERE addon_id = $1 AND catalog_id = 'ren-top2'", id); n == 0 {
				t.Errorf("%s: no rows of %s on the new identifier", when, table)
			}
		}
		if n := e.count("SELECT count(*) FROM catalog_arrivals WHERE addon_id = $1 AND catalog_id = 'ren-top2' AND genre = 'kept'", id); n != 1 {
			t.Errorf("%s: %d arrivals set up on the new identifier", when, n)
		}
		// The addon's keys keep the library's first identifier, and so
		// does its item's key; nothing else names the old one.
		if left := references(t, e.service.db, []string{"ren-top\"", ":tv:channels", "/channels/"}, "addons", "items"); len(left) != 0 {
			t.Errorf("%s: rows naming the old identifiers: %v", when, left)
		}
		if n := e.count(`SELECT count(*) FROM items WHERE data->>'catalogId' IN ('ren-top', 'channels') OR data->>'channelGroup' LIKE '%/channels/%'`); n != 0 {
			t.Errorf("%s: %d items naming the old identifiers", when, n)
		}
	}
	check("after the refresh")
	if n := e.count("SELECT count(*) FROM notification_health WHERE key LIKE 'guide:%'"); n != 0 {
		t.Errorf("%d guide health records", n)
	}
	if _, err := e.service.CleanUpRemoved(ctx); err != nil {
		t.Fatal(err)
	}
	check("after the cleanup")
	// The library hidden from the member stays hidden once listed.
	member, err := e.users.User(ctx, e.member.ID)
	if err != nil {
		t.Fatal(err)
	}
	libraries, err := e.service.Libraries(ctx, member)
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(libraries, func(l Item) bool { return l.ID == top || l.Name == "My top" }) {
		t.Errorf("the member sees the library they hid: %v", names(libraries))
	}
}

// A catalog back under an identifier a library followed away from makes
// a library of its own, with an identifier of its own: the library that
// followed keeps its own. A catalog renamed back to the identifier it had
// keeps its library and its identifier too.
func TestACatalogBackUnderAnOldIdentifierMakesALibraryOfItsOwn(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.addons.SetCleanup(e.service.CleanRemoved)
	e.addons.SetCarry(e.service.CarryRenamed)
	ctx := t.Context()
	addon := leftoverAddon("back")
	e.install(addons.Shared(), addon)
	id := e.installedID(addons.Shared(), "back")
	e.listed(e.member, "back top")
	top := e.library(e.member, "back top").ID
	e.exec([]any{"UPDATE users SET hidden_libraries = $2 WHERE id = $1", e.member.ID, []accounts.ID{top}})

	renamed(addon, map[string]string{"movie/back-top": "back-top2"})
	if _, changes, err := e.addons.Refresh(ctx, addons.Shared(), id, false); err != nil || len(changes.Followed) != 1 {
		t.Fatalf("followed %+v: %v", changes.Followed, err)
	}
	// The old identifier comes back, as another catalog.
	addon.mu.Lock()
	addon.manifest.Catalogs = append(addon.manifest.Catalogs, stremio.Catalog{Type: "movie", ID: "back-top", Name: "Comeback"})
	addon.catalogs["movie/back-top"] = titles("movie", 1)
	addon.mu.Unlock()
	_, changes, err := e.addons.Refresh(ctx, addons.Shared(), id, false)
	if err != nil || len(changes.Followed) != 0 || len(changes.Removed) != 0 {
		t.Fatalf("followed %+v, removed %+v: %v", changes.Followed, changes.Removed, err)
	}
	followed, comeback := e.libraryOf("movie", "back-top2"), e.libraryOf("movie", "back-top")
	if LibraryID(followed) != top || !followed.Enabled {
		t.Errorf("the library that followed: %+v, item %v", followed, LibraryID(followed))
	}
	if LibraryID(comeback) == top || comeback.Enabled {
		t.Errorf("the catalog back: %+v, item %v", comeback, LibraryID(comeback))
	}
	if _, err := e.addons.SetLibraries(ctx, addons.Shared(), []addons.LibraryChoice{{AddonID: id, CatalogType: "movie", CatalogID: "back-top2"},
		{AddonID: id, CatalogType: "movie", CatalogID: "back-top"}}); err != nil {
		t.Fatal(err)
	}
	e.listed(e.admin, "Comeback")
	if comebackID := e.library(e.admin, "Comeback").ID; comebackID == top || comebackID != LibraryID(comeback) {
		t.Errorf("the catalog back is listed as %v", comebackID)
	}
	if _, err := e.service.CleanUpRemoved(ctx); err != nil {
		t.Fatal(err)
	}
	if n := e.count("SELECT count(*) FROM items WHERE id = ANY($1)", []accounts.ID{top, LibraryID(comeback)}); n != 2 {
		t.Errorf("%d of the two libraries' items kept", n)
	}

	// The catalog back goes, and the one that followed takes its old
	// identifier again: it keeps its library and its identifier.
	addon.mu.Lock()
	addon.manifest.Catalogs = slices.DeleteFunc(addon.manifest.Catalogs, func(c stremio.Catalog) bool { return c.Name == "Comeback" })
	addon.mu.Unlock()
	if _, _, err := e.addons.Refresh(ctx, addons.Shared(), id, false); err != nil {
		t.Fatal(err)
	}
	renamed(addon, map[string]string{"movie/back-top2": "back-top"})
	if _, changes, err := e.addons.Refresh(ctx, addons.Shared(), id, false); err != nil || len(changes.Followed) != 1 {
		t.Fatalf("followed %+v: %v", changes.Followed, err)
	}
	if back := e.libraryOf("movie", "back-top"); LibraryID(back) != top || !back.Enabled || back.Key != (addons.CatalogKey{Type: "movie", ID: "back-top"}) {
		t.Errorf("renamed back: %+v, item %v", back, LibraryID(back))
	}
	installed, err := e.addons.Find(ctx, id)
	if err != nil || len(installed.LibraryKeys) != 0 {
		t.Errorf("the keys kept: %+v %v", installed.LibraryKeys, err)
	}
	var hidden []accounts.ID
	if err := e.service.db.QueryRow(ctx, "SELECT hidden_libraries FROM users WHERE id = $1", e.member.ID).Scan(&hidden); err != nil ||
		!slices.Equal(hidden, []accounts.ID{top}) {
		t.Errorf("the member hides %v: %v", hidden, err)
	}
}

// The hourly cleanup deletes a catalog home row on a catalog its addon no
// longer offers, which would list nothing; the rows on catalogs offered
// stay.
func TestTheCleanupDeletesHomeRowsOnCatalogsGone(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()
	addon := leftoverAddon("rows")
	e.install(addons.Shared(), addon)
	id := e.installedID(addons.Shared(), "rows")
	e.exec([]any{`INSERT INTO home_rows (position, name, kind, place, addon_id, catalog_type, catalog_id)
		VALUES (1, 'Top', 'catalog', 'top', $1, 'movie', 'rows-top'), (2, 'Shows', 'catalog', 'top', $1, 'series', 'rows-shows')`, id})
	addon.mu.Lock()
	addon.manifest.Catalogs = slices.DeleteFunc(addon.manifest.Catalogs, func(c stremio.Catalog) bool { return c.Type == "series" })
	addon.mu.Unlock()
	if _, _, err := e.addons.Refresh(ctx, addons.Shared(), id, false); err != nil {
		t.Fatal(err)
	}
	if cleaned, err := e.service.CleanUpRemoved(ctx); err != nil || !cleaned.Any() {
		t.Fatalf("cleaned %+v: %v", cleaned, err)
	}
	if n := e.count("SELECT count(*) FROM home_rows WHERE catalog_id = 'rows-shows'"); n != 0 {
		t.Errorf("%d home rows on the catalog gone", n)
	}
	if n := e.count("SELECT count(*) FROM home_rows WHERE catalog_id = 'rows-top'"); n != 1 {
		t.Errorf("%d home rows on the catalog kept", n)
	}
	if cleaned, err := e.service.CleanUpRemoved(ctx); err != nil || cleaned.Any() {
		t.Errorf("a second cleanup: %+v %v", cleaned, err)
	}
}
