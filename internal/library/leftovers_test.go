package library

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/smartlists"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/testdb"
)

// leftoverAddon is an addon of movies, shows and live TV channels, whose
// catalogs and titles are named after prefix; its first movies are those
// of every leftoverAddon.
func leftoverAddon(prefix string) *fakeAddon {
	movies := titles("movie", 3)
	shows := titles("series", 2)
	for i := range shows {
		shows[i].ID = prefix + "-" + shows[i].ID
	}
	return &fakeAddon{
		manifest: stremio.Manifest{ID: prefix, Name: prefix, Version: "1", Types: []string{"movie", "series", "tv"},
			Resources: []stremio.Resource{{Name: "catalog"}},
			Catalogs: []stremio.Catalog{
				{Type: "movie", ID: prefix + "-top", Name: prefix + " top"},
				{Type: "series", ID: prefix + "-shows", Name: prefix + " shows"},
				{Type: "tv", ID: "channels", Name: prefix + " channels"},
			}},
		catalogs: map[string][]stremio.Meta{
			"movie/" + prefix + "-top":    movies,
			"series/" + prefix + "-shows": shows,
			"tv/channels":                 {{ID: prefix + "-channel", Type: "tv", Name: prefix + " channel"}},
		},
	}
}

// installedID returns the identifier of the addon of scope whose manifest
// has id.
func (e env) installedID(scope addons.Scope, id string) accounts.ID {
	e.t.Helper()
	list, err := e.addons.Addons(e.t.Context(), scope)
	if err != nil {
		e.t.Fatal(err)
	}
	for _, addon := range list {
		if addon.Manifest.ID == id {
			return addon.ID
		}
	}
	e.t.Fatalf("no addon %q", id)
	return accounts.ID{}
}

// exec runs statements, each with its arguments.
func (e env) exec(statements ...[]any) {
	e.t.Helper()
	for _, s := range statements {
		if _, err := e.service.db.Exec(e.t.Context(), s[0].(string), s[1:]...); err != nil {
			e.t.Fatalf("%s: %v", s[0], err)
		}
	}
}

// count runs a query of one count.
func (e env) count(query string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.service.db.QueryRow(e.t.Context(), query, args...).Scan(&n); err != nil {
		e.t.Fatalf("%s: %v", query, err)
	}
	return n
}

// spellings are the ways the database writes ids: as a uuid, as the
// hexadecimal of keys and Jellyfin apps, and as the array of bytes of
// items' data.
func spellings(ids ...accounts.ID) []string {
	var result []string
	for _, id := range ids {
		hex := id.String()
		bytes := make([]string, len(id))
		for i, b := range id {
			bytes[i] = fmt.Sprint(b)
		}
		result = append(result, hex, hex[:8]+"-"+hex[8:12]+"-"+hex[12:16]+"-"+hex[16:20]+"-"+hex[20:], "["+strings.Join(bytes, ", ")+"]")
	}
	return result
}

// references counts, table by table, the rows whose text holds one of
// needles, but in the tables skipped.
func references(t *testing.T, db *pgxpool.Pool, needles []string, skipped ...string) map[string]int {
	t.Helper()
	rows, err := db.Query(t.Context(), `SELECT table_name FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_type = 'BASE TABLE' ORDER BY table_name`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	var table string
	for rows.Next() {
		if err := rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, table)
	}
	patterns := make([]string, len(needles))
	for i, needle := range needles {
		patterns[i] = "%" + needle + "%"
	}
	found := map[string]int{}
	for _, table := range tables {
		if slices.Contains(skipped, table) {
			continue
		}
		var n int
		if err := db.QueryRow(t.Context(), `SELECT count(*) FROM "`+table+`" r WHERE r::text LIKE ANY($1)`, patterns).Scan(&n); err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if n > 0 {
			found[table] = n
		}
	}
	return found
}

// titleAddon returns the addon a title's item is recorded under, nil for
// none.
func (e env) titleAddon(kind Kind, stremioID string) *accounts.ID {
	e.t.Helper()
	var raw []byte
	if err := e.service.db.QueryRow(e.t.Context(), "SELECT coalesce(data->'addon', 'null') FROM items WHERE key = $1", titleKey(kind, stremioID)).
		Scan(&raw); err != nil {
		e.t.Fatalf("%s: %v", stremioID, err)
	}
	var addon *accounts.ID
	if err := json.Unmarshal(raw, &addon); err != nil {
		e.t.Fatal(err)
	}
	return addon
}

// smartList saves a list of the server's drawing from catalogs.
func (e env) smartList(name string, catalogs ...smartlists.Catalog) smartlists.List {
	e.t.Helper()
	list, err := e.service.smart.Create(e.t.Context(), smartlists.List{Name: name, ShownAs: smartlists.ShownAsLibrary, Catalogs: catalogs,
		Rules: group("all"), Sort: "catalog"})
	if err != nil {
		e.t.Fatal(err)
	}
	return list
}

// Removing an addon deletes, with it, every row naming it, its libraries,
// collections or channels, in every table: but its recordings, kept as
// users' own. Its titles are released for the next catalog that lists
// them; a smart list left with no catalog is kept, for Health to tell.
func TestRemovingAnAddonLeavesNothingNamingIt(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.addons.SetCleanup(e.service.CleanRemoved)
	ctx := t.Context()
	e.install(addons.Shared(), leftoverAddon("gone"))
	e.install(addons.Shared(), leftoverAddon("kept"))
	gone, kept := e.installedID(addons.Shared(), "gone"), e.installedID(addons.Shared(), "kept")
	// The gone addon lists the shared movies first.
	e.listed(e.member, "gone top")
	e.listed(e.member, "gone shows")
	e.listed(e.member, "kept top")
	if _, err := e.service.Channels(ctx, e.member); err != nil {
		t.Fatal(err)
	}
	goneTop, goneShows, keptTop := e.library(e.member, "gone top").ID, e.library(e.member, "gone shows").ID, e.library(e.member, "kept top").ID
	// Its channels' library: live TV catalogs make none in apps.
	goneChannels := itemID(libraryKey(gone, "tv", "channels"))
	collection := itemID(collectionKey(gone, "collection"))
	channel := itemID(channelKey("gone-channel"))
	keptChannel := itemID(channelKey("kept-channel"))
	if e.titleAddon(KindMovie, "ttm000") == nil || *e.titleAddon(KindMovie, "ttm000") != gone {
		t.Fatal("the gone addon did not list the movies first")
	}

	group, err := e.users.CreateGroup(ctx, "family", nil)
	if err != nil {
		t.Fatal(err)
	}
	keptList := e.smartList("both", smartlists.Catalog{AddonID: gone, Type: "movie", ID: "gone-top"},
		smartlists.Catalog{AddonID: kept, Type: "movie", ID: "kept-top"})
	emptied := e.smartList("gone only", smartlists.Catalog{AddonID: gone, Type: "movie", ID: "gone-top"})
	now := time.Now()
	e.exec(
		[]any{"INSERT INTO items (id, key, kind, data) VALUES ($1, $2, 'collection', $3)", collection, collectionKey(gone, "collection"),
			fmt.Sprintf(`{"kind": "collection", "addon": %s}`, spellings(gone)[2])},
		[]any{"UPDATE user_groups SET hidden_libraries = $2 WHERE id = $1", group.ID, []accounts.ID{goneTop, keptTop}},
		[]any{"UPDATE users SET hidden_libraries = $2 WHERE id = $1", e.member.ID, []accounts.ID{goneShows}},
		[]any{"INSERT INTO user_configurations (user_id, value) VALUES ($1, $2)", e.member.ID, fmt.Sprintf(
			`{"OrderedViews": [%q, %q], "MyMediaExcludes": [%q], "LatestItemsExcludes": [], "PlayDefaultAudioTrack": true}`,
			goneTop.String(), keptTop.String(), spellings(goneShows)[1])},
		[]any{"INSERT INTO display_preferences (user_id, preference_id, client, value) VALUES ($1, $2, 'emby', '{}'), ($1, $3, 'emby', '{}')",
			e.member.ID, spellings(goneTop)[1], spellings(keptTop)[1]},
		[]any{"INSERT INTO generated_images (item_id, name, titles, posters) VALUES ($1, 'gone top', '{}', '{}') ON CONFLICT DO NOTHING", goneTop},
		[]any{"INSERT INTO item_images (item_id, image_type, image, content_type, tag) VALUES ($1, 'Primary', '\\x89', 'image/png', 'a'), ($2, 'Primary', '\\x89', 'image/png', 'b')",
			goneShows, collection},
		[]any{"INSERT INTO item_overrides (item_id, fields) VALUES ($1, '{\"name\": \"Mine\"}')", goneChannels},
		[]any{`INSERT INTO home_rows (position, name, kind, place, addon_id, catalog_type, catalog_id) VALUES (1, 'Top', 'catalog', 'top', $1, 'movie', 'gone-top')`, gone},
		[]any{`INSERT INTO home_rows (position, name, kind, place, collection_id, library) VALUES (2, 'Collection', 'collection', 'top', $1, true)`, collection},
		[]any{`INSERT INTO live_timers (user_id, channel_id, name, start_at, end_at) VALUES ($1, $2, 'Next', $3, $4), ($1, $5, 'Kept', $3, $4)`,
			e.admin.ID, channel, now.Add(time.Hour), now.Add(2 * time.Hour), keptChannel},
		[]any{`INSERT INTO live_series_timers (user_id, channel_id, name, start_at, end_at) VALUES ($1, $2, 'Series', $3, $4)`,
			e.admin.ID, channel, now.Add(time.Hour), now.Add(2 * time.Hour)},
		[]any{`INSERT INTO programme_series_reminders (user_id, channel_id, name, start_at) VALUES ($1, $2, 'Series', $3)`, e.member.ID, channel, now},
		[]any{`INSERT INTO live_recordings (user_id, channel_id, name, start_at, end_at, status, ended_at) VALUES ($1, $2, 'Recorded', $3, $3, 'Completed', $3)`,
			e.admin.ID, channel, now.Add(-time.Hour)},
		[]any{`UPDATE users SET live_tv_channels = $2 WHERE id = $1`, e.member.ID, fmt.Sprintf(
			`{"mode": "except", "groups": [%q, %q], "channels": ["gone-channel", "kept-channel"]}`, gone.String()+"/channels/", kept.String()+"/channels/")},
		[]any{`INSERT INTO notification_health (key, severity, text, since) VALUES ($1, 'warning', 'Gone', now()), ($2, 'warning', 'Guide', now())`,
			"addon:" + gone.String(), "guide:" + gone.String() + ":tv:channels"},
		[]any{`INSERT INTO notification_health_found (key, severity, text, found_at) VALUES ($1, 'warning', 'Gone', now())`, "addon:" + gone.String()},
	)
	needles := slices.Concat(spellings(gone, goneTop, goneShows, goneChannels, collection, channel), []string{"gone-channel", "gone-top", "gone-shows"})
	if before := references(t, e.service.db, needles); len(before) < 15 {
		t.Fatalf("the setup names the addon in %d tables: %v", len(before), before)
	}

	removed, err := e.addons.Remove(ctx, addons.Shared(), gone)
	if err != nil || removed.ID != gone {
		t.Fatalf("removed %v: %v", removed.ID, err)
	}
	// Recordings stay: they are users' own.
	if left := references(t, e.service.db, needles, "live_recordings"); len(left) != 0 {
		t.Errorf("rows still naming the addon: %v", left)
	}
	if n := e.count("SELECT count(*) FROM live_recordings"); n != 1 {
		t.Errorf("%d recordings", n)
	}
	if n := e.count("SELECT count(*) FROM live_timers WHERE name = 'Kept'"); n != 1 {
		t.Errorf("%d timers of the kept addon", n)
	}
	lists, err := e.service.smart.Lists(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lists {
		switch l.ID {
		case keptList.ID:
			if !slices.Equal(l.Catalogs, []smartlists.Catalog{{AddonID: kept, Type: "movie", ID: "kept-top"}}) {
				t.Errorf("the list of both: %v", l.Catalogs)
			}
		case emptied.ID:
			if len(l.Catalogs) != 0 {
				t.Errorf("the list of the gone addon only: %v", l.Catalogs)
			}
		}
	}
	if found, err := e.service.smart.Emptied(ctx); err != nil || len(found) != 1 || found[0].ID != emptied.ID {
		t.Errorf("lists emptied: %v %v", found, err)
	}
	// What the kept addon brought stays.
	var hidden []accounts.ID
	if err := e.service.db.QueryRow(ctx, "SELECT hidden_libraries FROM user_groups WHERE id = $1", group.ID).Scan(&hidden); err != nil ||
		!slices.Equal(hidden, []accounts.ID{keptTop}) {
		t.Errorf("the group hides %v: %v", hidden, err)
	}
	if n := e.count("SELECT count(*) FROM users WHERE live_tv_channels->'channels' = '[\"kept-channel\"]' AND jsonb_array_length(live_tv_channels->'groups') = 1"); n != 1 {
		t.Errorf("the member's channel limit lost the kept addon's")
	}
	if n := e.count("SELECT count(*) FROM display_preferences"); n != 1 {
		t.Errorf("%d display preferences", n)
	}
	// The released titles go to the next catalog that lists them.
	if addon := e.titleAddon(KindMovie, "ttm000"); addon != nil {
		t.Errorf("a title is still recorded under %v", addon)
	}
	e.listed(e.member, "kept top")
	if addon := e.titleAddon(KindMovie, "ttm000"); addon == nil || *addon != kept {
		t.Errorf("listed again, a title is recorded under %v", addon)
	}
}

// The hourly cleanup deletes what a user's own addons left once the user
// was deleted with them, and leaves the rest be.
func TestTheCleanupClearsWhatADeletedUserLeft(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()
	// An administrator's own addons may be on the local network.
	alice, err := e.users.CreateUser(ctx, accounts.NewUser{Name: "alice", Password: "correct horse", IsAdministrator: true})
	if err != nil {
		t.Fatal(err)
	}
	e.install(addons.Personal(alice.ID), leftoverAddon("own"))
	e.install(addons.Shared(), leftoverAddon("kept"))
	own, kept := e.installedID(addons.Personal(alice.ID), "own"), e.installedID(addons.Shared(), "kept")
	e.listed(alice, "own top")
	e.listed(alice, "kept top")
	e.listed(alice, "kept shows")
	if _, err := e.service.Channels(ctx, alice); err != nil {
		t.Fatal(err)
	}
	ownTop := e.library(alice, "own top").ID
	e.exec([]any{`INSERT INTO notification_health (key, severity, text, since) VALUES ($1, 'warning', 'Own', now())`, "addon:" + own.String()})

	if cleaned, err := e.service.CleanUpRemoved(ctx); err != nil || cleaned.Any() {
		t.Fatalf("a cleanup with nothing gone: %+v %v", cleaned, err)
	}
	if err := e.users.DeleteUser(ctx, alice.ID); err != nil {
		t.Fatal(err)
	}
	needles := slices.Concat(spellings(own, ownTop, itemID(channelKey("own-channel"))), []string{"own-channel", "own-top"})
	if before := references(t, e.service.db, needles); len(before) == 0 {
		t.Fatal("the deletion left nothing to clean")
	}
	cleaned, err := e.service.CleanUpRemoved(ctx)
	if err != nil || !cleaned.Any() {
		t.Fatalf("cleaned %+v: %v", cleaned, err)
	}
	if left := references(t, e.service.db, needles); len(left) != 0 {
		t.Errorf("rows still naming the addon: %v", left)
	}
	// The kept addon keeps its library, channel and titles.
	if n := e.count("SELECT count(*) FROM items WHERE key = ANY($1)", []string{
		libraryKey(kept, "movie", "kept-top"), channelKey("kept-channel"), titleKey(KindSeries, "kept-tts000")}); n != 3 {
		t.Errorf("%d items of the kept addon", n)
	}
	if cleaned, err := e.service.CleanUpRemoved(ctx); err != nil || cleaned.Any() {
		t.Errorf("a second cleanup: %+v %v", cleaned, err)
	}
}

// The hourly cleanup deletes what a catalog its addon dropped left: its
// library, its place in smart lists and hidden libraries, and its titles'
// record of it; the catalogs kept keep theirs.
func TestTheCleanupClearsWhatADroppedCatalogLeft(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()
	e.install(addons.Shared(), leftoverAddon("both"))
	id := e.installedID(addons.Shared(), "both")
	e.listed(e.member, "both top")
	e.listed(e.member, "both shows")
	top, shows := e.library(e.member, "both top").ID, e.library(e.member, "both shows").ID
	list := e.smartList("both", smartlists.Catalog{AddonID: id, Type: "movie", ID: "both-top"},
		smartlists.Catalog{AddonID: id, Type: "series", ID: "both-shows"})
	e.exec([]any{"UPDATE users SET hidden_libraries = $2 WHERE id = $1", e.member.ID, []accounts.ID{top, shows}})

	// The addon's new address no longer offers its shows.
	dropped := leftoverAddon("both")
	dropped.manifest.Catalogs = slices.DeleteFunc(dropped.manifest.Catalogs, func(c stremio.Catalog) bool { return c.Type == "series" })
	server := httptest.NewServer(dropped)
	t.Cleanup(server.Close)
	if _, _, err := e.addons.Replace(ctx, addons.Shared(), id, server.URL+"/manifest.json", false); err != nil {
		t.Fatal(err)
	}
	cleaned, err := e.service.CleanUpRemoved(ctx)
	if err != nil || !cleaned.Any() {
		t.Fatalf("cleaned %+v: %v", cleaned, err)
	}
	if left := references(t, e.service.db, append(spellings(shows), "both-shows\"", "both-shows,")); len(left) != 0 {
		t.Errorf("rows still naming the catalog: %v", left)
	}
	if addon := e.titleAddon(KindSeries, "both-tts000"); addon != nil {
		t.Errorf("a show is still recorded under %v", addon)
	}
	if addon := e.titleAddon(KindMovie, "ttm000"); addon == nil || *addon != id {
		t.Errorf("a movie of the catalog kept is recorded under %v", addon)
	}
	var hidden []accounts.ID
	if err := e.service.db.QueryRow(ctx, "SELECT hidden_libraries FROM users WHERE id = $1", e.member.ID).Scan(&hidden); err != nil ||
		!slices.Equal(hidden, []accounts.ID{top}) {
		t.Errorf("the member hides %v: %v", hidden, err)
	}
	saved, err := e.service.smart.Get(ctx, nil, list.ID)
	if err != nil || !slices.Equal(saved.Catalogs, []smartlists.Catalog{{AddonID: id, Type: "movie", ID: "both-top"}}) {
		t.Errorf("the smart list draws from %v: %v", saved.Catalogs, err)
	}
	if n := e.count("SELECT count(*) FROM items WHERE id = $1", top); n != 1 {
		t.Error("the library of the catalog kept was deleted")
	}
}

// The cleanup never deletes what an addon's change under way brings back:
// a catalog offered again while it runs, its library recorded already,
// waits for it, and fails it rather than lose the library.
func TestTheCleanupWaitsForAChangeOfTheAddons(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()
	e.install(addons.Shared(), leftoverAddon("race"))
	id := e.installedID(addons.Shared(), "race")
	e.listed(e.member, "race shows")
	shows := e.library(e.member, "race shows").ID
	var whole []byte
	if err := e.service.db.QueryRow(ctx, "SELECT manifest FROM addons WHERE id = $1", id).Scan(&whole); err != nil {
		t.Fatal(err)
	}
	dropped := leftoverAddon("race")
	dropped.manifest.Catalogs = slices.DeleteFunc(dropped.manifest.Catalogs, func(c stremio.Catalog) bool { return c.Type == "series" })
	server := httptest.NewServer(dropped)
	t.Cleanup(server.Close)
	if _, _, err := e.addons.Replace(ctx, addons.Shared(), id, server.URL+"/manifest.json", false); err != nil {
		t.Fatal(err)
	}

	// A change offers the shows again, from a connection of its own.
	conn, err := pgx.Connect(ctx, testdb.CopyURL(t, e.service.db))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	change, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := change.Exec(ctx, "UPDATE addons SET manifest = $2 WHERE id = $1", id, whole); err != nil {
		t.Fatal(err)
	}
	swept := make(chan error, 1)
	go func() {
		_, err := e.service.CleanUpRemoved(ctx)
		swept <- err
	}()
	for e.count("SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'") == 0 {
		select {
		case err := <-swept:
			t.Fatalf("the cleanup did not wait for the change: %v", err)
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := change.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-swept; err == nil {
		t.Error("the cleanup went on after the change")
	}
	if n := e.count("SELECT count(*) FROM items WHERE id = $1", shows); n != 1 {
		t.Fatal("the library offered again was deleted")
	}
	if cleaned, err := e.service.CleanUpRemoved(ctx); err != nil || cleaned.Any() {
		t.Errorf("the next cleanup: %+v %v", cleaned, err)
	}
}

// keyRangeEnd ends a range of keys past every key starting with the
// prefix, in any collation.
func TestKeyRangeEnd(t *testing.T) {
	t.Parallel()
	for prefix, want := range map[string]string{
		"library|":                            "librarz",
		"channel|":                            "channem",
		"library|0123abcf|":                   "library|0123abcg",
		"movie|polyfin-iptv:" + "0123456789:": "movie|polyfin-iptv:012345678a",
	} {
		if got := keyRangeEnd(prefix); got != want {
			t.Errorf("%s: %s, want %s", prefix, got, want)
		}
	}
}
