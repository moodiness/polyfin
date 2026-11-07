package library

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/smartlists"
	"github.com/moodiness/polyfin/internal/stremio"
)

// What a removed addon, IPTV source or local folder, or a catalog an addon
// no longer offers, leaves behind.
//
// The foreign keys delete an addon's own tables with it. The rest names
// it, its catalogs or its items without one: the items of its libraries,
// collections and channels, with their artwork and edits; the titles it
// listed first; smart lists, home rows, hidden libraries, the views of
// Jellyfin apps and the limits of Live TV channels; timers on its
// channels; and its health records. cleanLeftovers deletes or prunes them
// all, in the transaction that removes the addon (see CleanRemoved), and
// for every addon or catalog found gone since, as a user deleted with
// their own addons, a catalog an addon dropped or a row deleted in the
// database (see CleanUpRemoved).
//
// Recordings stay: they are users' own. So do the titles, which users'
// data names, but released, for the next catalog that lists them to take
// them (see savedData). Own channels airing a source gone, and smart lists
// left with no catalog, stay too: Health tells of them.

// LeftoversInterval is how often CleanUpRemoved runs.
const LeftoversInterval = time.Hour

// leftovers names what cleanLeftovers removes: the addons gone, the
// catalogs addons still installed dropped, and the items of their
// libraries, collections and channels, the channels' Stremio identifiers
// along.
type leftovers struct {
	addons      []accounts.ID
	catalogs    []droppedCatalog
	libraries   []accounts.ID
	collections []accounts.ID
	channels    []accounts.ID
	channelIDs  []string
}

// droppedCatalog is a catalog its addon, still installed, no longer offers.
type droppedCatalog struct {
	addon       accounts.ID
	catalogType string
	catalogID   string
}

// Cleaned counts what a cleanup removed: items deleted, titles released
// and other rows deleted or pruned.
type Cleaned struct {
	Items    int64
	Released int64
	Rows     int64
}

// Any reports whether the cleanup removed anything.
func (c Cleaned) Any() bool { return c.Items+c.Released+c.Rows > 0 }

// healthKinds are the kinds of health problems kept for an addon, whose
// keys are the kind and the addon's identifier (see admin's
// healthproblems.go); a guide's goes on with its catalog.
var healthKinds = []string{"addon", "folder", "folder_unwatched", "folder_search", "folder_no_search", "tuner", "iptv", "guide"}

// releasedKinds are the kinds of the titles a catalog lists, which a
// catalog gone releases (see savedData).
var releasedKinds = []string{string(KindMovie), string(KindSeries), string(KindVideo)}

// iptvKinds are the kinds of the items an IPTV source's identifiers make,
// deleted with it; its programmes and replays have their own sweep (see
// livetv.go).
var iptvKinds = []string{string(KindChannel), string(KindMovie), string(KindSeries), string(KindSeason), string(KindEpisode), string(KindVideo)}

// CleanRemoved deletes, in tx, the transaction removing addon, what the
// addon left besides its own tables, and returns what to forget once tx
// is stored. It is the addons store's Cleanup (see addons.SetCleanup).
func (s *Service) CleanRemoved(ctx context.Context, tx pgx.Tx, addon addons.Addon) (func(), error) {
	l, err := addonLeftovers(ctx, tx, addon)
	if err != nil {
		return nil, err
	}
	if _, err := cleanLeftovers(ctx, tx, l); err != nil {
		return nil, err
	}
	return func() { s.forgetLeftovers(context.WithoutCancel(ctx), l, addon.ID) }, nil
}

// addonLeftovers finds what addon, removed in tx, left: the items of the
// libraries of all its catalogs, recorded or not, of its collections and
// of its channels.
func addonLeftovers(ctx context.Context, tx pgx.Tx, addon addons.Addon) (leftovers, error) {
	l := leftovers{addons: []accounts.ID{addon.ID}}
	for _, catalog := range addon.Manifest.Catalogs {
		l.libraries = append(l.libraries, itemID(liveLibraryKey(addon, catalog)))
	}
	recorded, err := keyedItems(ctx, tx, "library|"+addon.ID.String()+"|")
	if err != nil {
		return l, err
	}
	for _, item := range recorded {
		if !slices.Contains(l.libraries, item.id) {
			l.libraries = append(l.libraries, item.id)
		}
	}
	collections, err := keyedItems(ctx, tx, "collection|"+addon.ID.String()+"|")
	if err != nil {
		return l, err
	}
	for _, item := range collections {
		l.collections = append(l.collections, item.id)
	}
	channels, err := recordedChannels(ctx, tx)
	if err != nil {
		return l, err
	}
	for _, c := range channels {
		if c.addon == addon.ID {
			l.channels, l.channelIDs = append(l.channels, c.id), append(l.channelIDs, c.stremioID)
		}
	}
	return l, nil
}

// liveLibraryKey is the key of the library item of an installed addon's
// catalog (see LibraryID).
func liveLibraryKey(addon addons.Addon, catalog stremio.Catalog) string {
	return libraryKey(addon.ID, cmp.Or(addon.LibraryKeyTypes[catalog.ID], catalog.Type), catalog.ID)
}

// CleanUpRemoved cleans what addons and catalogs found gone left, as
// CleanRemoved does at a removal: a user deleted with their own addons, a
// catalog an addon dropped, a row deleted in the database, a removal whose
// requests under way recorded items after it, or what earlier versions
// left. It reads the addons installed and what names them in one snapshot,
// holding the addons as they are, so that it never takes for gone an addon
// added, or a catalog an addon gained again, meanwhile: their items are
// recorded once the change is stored, after the snapshot, and a change
// waits for the cleanup. A row changed since the snapshot fails it, for
// the next run.
func (s *Service) CleanUpRemoved(ctx context.Context) (Cleaned, error) {
	var l leftovers
	var cleaned Cleaned
	var live []addons.Addon
	err := pgx.BeginTxFunc(ctx, s.db, pgx.TxOptions{IsoLevel: pgx.RepeatableRead}, func(tx pgx.Tx) error {
		var err error
		if live, err = addons.Installed(ctx, tx); err != nil {
			return err
		}
		if l, err = goneLeftovers(ctx, tx, live); err != nil {
			return err
		}
		cleaned, err = cleanLeftovers(ctx, tx, l)
		return err
	})
	if err != nil {
		return Cleaned{}, err
	}
	if cleaned.Any() {
		s.forgetLeftovers(ctx, l)
	}
	return cleaned, nil
}

// installedCatalogs knows the addons installed and their catalogs.
type installedCatalogs struct {
	addons    map[accounts.ID]addons.Addon
	libraries map[string]bool
}

func newInstalledCatalogs(installed []addons.Addon) installedCatalogs {
	c := installedCatalogs{addons: map[accounts.ID]addons.Addon{}, libraries: map[string]bool{}}
	for _, addon := range installed {
		c.addons[addon.ID] = addon
		for _, catalog := range addon.Manifest.Catalogs {
			c.libraries[liveLibraryKey(addon, catalog)] = true
		}
	}
	return c
}

// installed reports whether the addon of a hexadecimal identifier is.
func (c installedCatalogs) installed(hexID string) (accounts.ID, bool) {
	id, err := accounts.ParseID(hexID)
	if err != nil {
		return id, true
	}
	_, ok := c.addons[id]
	return id, ok
}

// offers reports whether an installed addon offers a catalog.
func (c installedCatalogs) offers(addon accounts.ID, catalogType, catalogID string) bool {
	_, ok := c.addons[addon].Manifest.Catalog(catalogType, catalogID)
	return ok
}

// goneLeftovers finds, in tx, what names an addon not installed, or a
// catalog its addon no longer offers: the items of libraries, collections
// and channels, smart lists, Live TV channel limits and health records.
// Each is found by an index (see keyedItems) or in a small table: the
// titles alone are looked for among every item, by cleanLeftovers, and
// only when something is gone.
func goneLeftovers(ctx context.Context, tx pgx.Tx, installed []addons.Addon) (leftovers, error) {
	var l leftovers
	live := newInstalledCatalogs(installed)
	gone := func(id accounts.ID) {
		if !slices.Contains(l.addons, id) {
			l.addons = append(l.addons, id)
		}
	}
	dropped := func(c droppedCatalog) {
		if !slices.Contains(l.catalogs, c) {
			l.catalogs = append(l.catalogs, c)
		}
	}
	libraries, err := keyedItems(ctx, tx, "library|")
	if err != nil {
		return l, err
	}
	for _, item := range libraries {
		// library|<addon>|<key type>|<catalog>
		parts := strings.SplitN(item.key, "|", 4)
		if len(parts) != 4 || live.libraries[item.key] {
			continue
		}
		addon, installed := live.installed(parts[1])
		if !installed {
			gone(addon)
		} else {
			dropped(droppedCatalog{addon: addon, catalogType: cmp.Or(item.catalogType, parts[2]), catalogID: parts[3]})
		}
		l.libraries = append(l.libraries, item.id)
	}
	collections, err := keyedItems(ctx, tx, "collection|")
	if err != nil {
		return l, err
	}
	for _, item := range collections {
		parts := strings.SplitN(item.key, "|", 3)
		if len(parts) != 3 {
			continue
		}
		if addon, installed := live.installed(parts[1]); !installed {
			gone(addon)
			l.collections = append(l.collections, item.id)
		}
	}
	channels, err := recordedChannels(ctx, tx)
	if err != nil {
		return l, err
	}
	for _, c := range channels {
		if _, installed := live.addons[c.addon]; c.addon != (accounts.ID{}) && !installed {
			gone(c.addon)
			l.channels, l.channelIDs = append(l.channels, c.id), append(l.channelIDs, c.stremioID)
		}
	}
	// Smart lists name catalogs by addon, type and identifier.
	rows, err := tx.Query(ctx, `SELECT DISTINCT coalesce(c->>'addonId', ''), coalesce(c->>'type', ''), coalesce(c->>'id', '')
		FROM smart_lists, jsonb_array_elements(catalogs) c`)
	if err != nil {
		return l, err
	}
	var addonHex, catalogType, catalogID string
	if _, err := pgx.ForEachRow(rows, []any{&addonHex, &catalogType, &catalogID}, func() error {
		addon, installed := live.installed(addonHex)
		switch {
		case !installed:
			gone(addon)
		case !live.offers(addon, catalogType, catalogID):
			dropped(droppedCatalog{addon: addon, catalogType: catalogType, catalogID: catalogID})
		}
		return nil
	}); err != nil {
		return l, err
	}
	// Live TV channel limits name channel groups by addon, and IPTV
	// sources' channels by their identifiers.
	rows, err = tx.Query(ctx, `SELECT DISTINCT m[1] FROM (
			SELECT live_tv_channels FROM users UNION ALL SELECT live_tv_channels FROM user_groups) r(rule),
			regexp_matches(r.rule::text, '"(?:polyfin-iptv:)?([0-9a-f]{32})[/:]', 'g') m
		WHERE r.rule->>'mode' <> 'all'`)
	if err != nil {
		return l, err
	}
	if _, err := pgx.ForEachRow(rows, []any{&addonHex}, func() error {
		if addon, installed := live.installed(addonHex); !installed {
			gone(addon)
		}
		return nil
	}); err != nil {
		return l, err
	}
	// Health records: kind:<addon>, and guide:<addon>:<type>:<catalog>.
	rows, err = tx.Query(ctx, `SELECT key FROM notification_health WHERE split_part(key, ':', 1) = ANY($1)
		UNION SELECT key FROM notification_health_found WHERE split_part(key, ':', 1) = ANY($1)`, healthKinds)
	if err != nil {
		return l, err
	}
	var key string
	if _, err := pgx.ForEachRow(rows, []any{&key}, func() error {
		parts := strings.SplitN(key, ":", 4)
		if len(parts) < 2 {
			return nil
		}
		addon, installed := live.installed(parts[1])
		switch {
		case !installed:
			gone(addon)
		case parts[0] == "guide" && len(parts) == 4 && !live.offers(addon, parts[2], parts[3]):
			dropped(droppedCatalog{addon: addon, catalogType: parts[2], catalogID: parts[3]})
		}
		return nil
	}); err != nil {
		return l, err
	}
	return l, nil
}

// keyedItem is an item found by its key.
type keyedItem struct {
	id          accounts.ID
	key         string
	catalogType string
}

// keyedItems returns the items whose key starts with prefix, found by the
// index of the keys: they lie from prefix to the key keyRangeEnd gives.
func keyedItems(ctx context.Context, db pgx.Tx, prefix string) ([]keyedItem, error) {
	rows, err := db.Query(ctx, `SELECT id, key, coalesce(data->>'catalogType', '') FROM items
		WHERE key >= $1 AND key < $2 AND starts_with(key, $1)`, prefix, keyRangeEnd(prefix))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (keyedItem, error) {
		var item keyedItem
		err := row.Scan(&item.id, &item.key, &item.catalogType)
		return item, err
	})
}

// keyRangeEnd returns a key past every key starting with prefix, in the
// database's collation, whatever it is: prefix up to its last letter or
// digit, that one counted up. Collations other than C may weigh or skip
// punctuation, but compare letters and digits in their order: a key
// starting with prefix differs from the end at that character, and comes
// first. The keys in between that do not start with prefix are left out by
// starts_with, which compares bytes. prefix's last letter or digit is
// neither z nor Z.
func keyRangeEnd(prefix string) string {
	end := strings.TrimRightFunc(prefix, func(r rune) bool { return !('0' <= r && r <= '9' || 'a' <= r && r <= 'z') })
	last := end[len(end)-1]
	if last == '9' {
		last = 'a'
	} else {
		last++
	}
	return end[:len(end)-1] + string(last)
}

// recordedChannel is a channel's item: its Stremio identifier, and its
// addon, zero for an own channel's (see virtualtv).
type recordedChannel struct {
	id        accounts.ID
	stremioID string
	addon     accounts.ID
}

// recordedChannels returns every channel's item: an IPTV source's channels
// name it in their identifiers, an addon's are recorded with it.
func recordedChannels(ctx context.Context, tx pgx.Tx) ([]recordedChannel, error) {
	const prefix = "channel|"
	rows, err := tx.Query(ctx, `SELECT id, key, coalesce(data->'addon', 'null') FROM items
		WHERE key >= $1 AND key < $2 AND starts_with(key, $1)`, prefix, keyRangeEnd(prefix))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (recordedChannel, error) {
		var c recordedChannel
		var key string
		var addon []byte
		if err := row.Scan(&c.id, &key, &addon); err != nil {
			return c, err
		}
		c.stremioID = strings.TrimPrefix(key, prefix)
		if rest, ok := strings.CutPrefix(c.stremioID, iptv.IDPrefix); ok {
			if source, _, ok := strings.Cut(rest, ":"); ok {
				if id, err := accounts.ParseID(source); err == nil {
					c.addon = id
					return c, nil
				}
			}
		}
		var id *accounts.ID
		if err := json.Unmarshal(addon, &id); err == nil && id != nil {
			c.addon = *id
		}
		return c, nil
	})
}

// cleanLeftovers deletes, in tx, the leftovers of l: in this order, the
// timers on its channels, which name them; the references to its items;
// its smart list entries, channel limits and health records; its IPTV
// items; then the titles its catalogs listed first are released and its
// items deleted. The rows each statement changes are locked in the order
// of their identifiers, as saves lock items (see inLockOrder), so that
// none deadlocks with a listing.
func cleanLeftovers(ctx context.Context, tx pgx.Tx, l leftovers) (Cleaned, error) {
	var cleaned Cleaned
	if len(l.addons) == 0 && len(l.catalogs) == 0 {
		return cleaned, nil
	}
	addonHexes := make([]string, 0, len(l.addons))
	addonJSON := make([]string, 0, len(l.addons))
	for _, id := range l.addons {
		addonHexes = append(addonHexes, id.String())
		encoded, err := json.Marshal(id)
		if err != nil {
			return cleaned, err
		}
		addonJSON = append(addonJSON, string(encoded))
	}
	// Every list is sent, empty rather than NULL: x <> ALL(NULL) is no
	// more true than x = ANY(NULL).
	l.libraries, l.collections, l.channels = nonNil(l.libraries), nonNil(l.collections), nonNil(l.channels)
	droppedAddons, droppedJSON, droppedTypes, droppedIDs := []string{}, []string{}, []string{}, []string{}
	guideKeys, droppedGroups := []string{}, []string{}
	for _, c := range l.catalogs {
		encoded, err := json.Marshal(c.addon)
		if err != nil {
			return cleaned, err
		}
		droppedAddons, droppedJSON = append(droppedAddons, c.addon.String()), append(droppedJSON, string(encoded))
		droppedTypes, droppedIDs = append(droppedTypes, c.catalogType), append(droppedIDs, c.catalogID)
		guideKeys = append(guideKeys, "guide:"+c.addon.String()+":"+c.catalogType+":"+c.catalogID)
		droppedGroups = append(droppedGroups, c.addon.String()+"/"+c.catalogID+"/")
	}
	views := nonNil(slices.Concat(l.libraries, l.collections))
	viewHexes := make([]string, 0, len(views))
	for _, id := range views {
		viewHexes = append(viewHexes, id.String())
	}

	exec := func(counted *int64, query string, args ...any) error {
		tag, err := tx.Exec(ctx, query, args...)
		if err == nil {
			*counted += tag.RowsAffected()
		}
		return err
	}
	statements := []struct {
		counted *int64
		query   string
		args    []any
	}{
		// Timers yet to record, series recordings and series reminders on
		// its channels; recordings stay. A series recording on any channel
		// goes on.
		{&cleaned.Rows, `DELETE FROM live_timers WHERE id IN (SELECT id FROM live_timers
			WHERE channel_id = ANY($1) AND status = 'New' ORDER BY id FOR UPDATE)`, []any{l.channels}},
		{&cleaned.Rows, `DELETE FROM live_series_timers WHERE id IN (SELECT id FROM live_series_timers
			WHERE channel_id = ANY($1) AND NOT record_any_channel ORDER BY id FOR UPDATE)`, []any{l.channels}},
		{&cleaned.Rows, `DELETE FROM programme_series_reminders WHERE id IN (SELECT id FROM programme_series_reminders
			WHERE channel_id = ANY($1) ORDER BY id FOR UPDATE)`, []any{l.channels}},
		// What names its libraries and collections.
		{&cleaned.Rows, `DELETE FROM home_rows WHERE id IN (SELECT id FROM home_rows
			WHERE kind = 'collection' AND collection_id = ANY($1) ORDER BY id FOR UPDATE)`, []any{l.collections}},
		{&cleaned.Rows, `UPDATE users SET hidden_libraries = ARRAY(SELECT x FROM unnest(hidden_libraries) x WHERE x <> ALL($1))
			WHERE id IN (SELECT id FROM users WHERE hidden_libraries && $1 ORDER BY id FOR UPDATE)`, []any{views}},
		{&cleaned.Rows, `UPDATE user_groups SET hidden_libraries = ARRAY(SELECT x FROM unnest(hidden_libraries) x WHERE x <> ALL($1))
			WHERE id IN (SELECT id FROM user_groups WHERE hidden_libraries && $1 ORDER BY id FOR UPDATE)`, []any{views}},
		{&cleaned.Rows, `UPDATE user_configurations c SET value = c.value || (
				SELECT jsonb_object_agg(f.k, (SELECT coalesce(jsonb_agg(e ORDER BY n), '[]') FROM jsonb_array_elements(c.value->f.k) WITH ORDINALITY v(e, n)
					WHERE lower(replace(e#>>'{}', '-', '')) <> ALL($1)))
				FROM unnest($2::text[]) f(k) WHERE jsonb_typeof(c.value->f.k) = 'array')
			WHERE c.user_id IN (SELECT u.user_id FROM user_configurations u
				WHERE EXISTS (SELECT FROM unnest($2::text[]) f(k), jsonb_array_elements_text(CASE WHEN jsonb_typeof(u.value->f.k) = 'array' THEN u.value->f.k ELSE '[]' END) e
					WHERE lower(replace(e, '-', '')) = ANY($1))
				ORDER BY u.user_id FOR UPDATE)`, []any{viewHexes, viewLists}},
		{&cleaned.Rows, `DELETE FROM display_preferences WHERE lower(replace(preference_id, '-', '')) = ANY($1)`, []any{viewHexes}},
		{&cleaned.Rows, `DELETE FROM generated_images WHERE item_id = ANY($1)`, []any{views}},
		{&cleaned.Rows, `DELETE FROM item_images WHERE item_id = ANY($1)`, []any{views}},
		{&cleaned.Rows, `DELETE FROM item_overrides WHERE item_id = ANY($1)`, []any{views}},
		// A dropped catalog's pages, reach and page size; a removed
		// addon's go with it.
		{&cleaned.Rows, `DELETE FROM catalog_pages WHERE (addon_id, catalog_type, catalog_id) IN (SELECT a::uuid, t, c FROM unnest($1::text[], $2::text[], $3::text[]) d(a, t, c))`,
			[]any{droppedAddons, droppedTypes, droppedIDs}},
		{&cleaned.Rows, `DELETE FROM catalog_reach WHERE (addon_id, catalog_type, catalog_id) IN (SELECT a::uuid, t, c FROM unnest($1::text[], $2::text[], $3::text[]) d(a, t, c))`,
			[]any{droppedAddons, droppedTypes, droppedIDs}},
		{&cleaned.Rows, `DELETE FROM catalog_sizes WHERE (addon_id, catalog_type, catalog_id) IN (SELECT a::uuid, t, c FROM unnest($1::text[], $2::text[], $3::text[]) d(a, t, c))`,
			[]any{droppedAddons, droppedTypes, droppedIDs}},
		// Smart lists keep their other catalogs; one left with none is
		// kept, and Health tells of it.
		{&cleaned.Rows, `UPDATE smart_lists SET catalogs = (SELECT coalesce(jsonb_agg(c ORDER BY n), '[]') FROM jsonb_array_elements(catalogs) WITH ORDINALITY e(c, n)
				WHERE NOT (c->>'addonId' = ANY($1) OR (c->>'addonId', c->>'type', c->>'id') IN (SELECT * FROM unnest($2::text[], $3::text[], $4::text[]))))
			WHERE id IN (SELECT s.id FROM smart_lists s WHERE EXISTS (SELECT FROM jsonb_array_elements(s.catalogs) c
				WHERE c->>'addonId' = ANY($1) OR (c->>'addonId', c->>'type', c->>'id') IN (SELECT * FROM unnest($2::text[], $3::text[], $4::text[])))
				ORDER BY s.id FOR UPDATE)`, []any{addonHexes, droppedAddons, droppedTypes, droppedIDs}},
		// Its health records go without a word: a problem told is not
		// told solved (see notifications.CheckHealth).
		{&cleaned.Rows, `DELETE FROM notification_health WHERE split_part(key, ':', 1) = ANY($1) AND split_part(key, ':', 2) = ANY($2) OR key = ANY($3)`,
			[]any{healthKinds, addonHexes, guideKeys}},
		{&cleaned.Rows, `DELETE FROM notification_health_found WHERE split_part(key, ':', 1) = ANY($1) AND split_part(key, ':', 2) = ANY($2) OR key = ANY($3)`,
			[]any{healthKinds, addonHexes, guideKeys}},
	}
	for _, s := range statements {
		if err := exec(s.counted, s.query, s.args...); err != nil {
			return cleaned, err
		}
	}
	rules, err := pruneChannelRules(ctx, tx, addonHexes, droppedGroups, l.channelIDs)
	if err != nil {
		return cleaned, err
	}
	cleaned.Rows += rules

	// An IPTV source's items: its titles, their seasons and episodes, and
	// its channels, as its identifiers name them.
	iptvItems := []accounts.ID{}
	for _, hexID := range addonHexes {
		for _, kind := range iptvKinds {
			found, err := keyedItems(ctx, tx, kind+"|"+iptv.IDPrefix+hexID+":")
			if err != nil {
				return cleaned, err
			}
			for _, item := range found {
				iptvItems = append(iptvItems, item.id)
			}
		}
	}
	// The titles its catalogs listed first, all items scanned once: the
	// array of their identifiers has them updated by the primary key.
	if err := exec(&cleaned.Released, `UPDATE items SET data = items.data - '{addon,catalogType,catalogId,confined,parent}'::text[], updated_at = now()
		WHERE items.id = ANY(ARRAY(SELECT id FROM items WHERE kind = ANY($1) AND items.id <> ALL($2) AND (data->'addon' = ANY($3::jsonb[])
			OR (data->'addon', data->>'catalogType', data->>'catalogId') IN (SELECT a::jsonb, t, c FROM unnest($4::text[], $5::text[], $6::text[]) d(a, t, c)))
			ORDER BY id FOR UPDATE))`,
		releasedKinds, iptvItems, addonJSON, droppedJSON, droppedTypes, droppedIDs); err != nil {
		return cleaned, err
	}
	if err := exec(&cleaned.Items, `DELETE FROM items WHERE id = ANY(ARRAY(SELECT id FROM items WHERE id = ANY($1) ORDER BY id FOR UPDATE))`,
		slices.Concat(views, l.channels, iptvItems)); err != nil {
		return cleaned, err
	}
	return cleaned, nil
}

// viewLists are the lists of view identifiers of a Jellyfin app's user
// configuration (see jellyfin's configuration.go).
var viewLists = []string{"OrderedViews", "MyMediaExcludes", "LatestItemsExcludes", "GroupedFolders"}

// pruneChannelRules takes out of the Live TV channel limits of users and
// groups the channel groups of the addons gone and of the catalogs
// dropped, the channels of the IPTV sources gone, and channels.
func pruneChannelRules(ctx context.Context, tx pgx.Tx, addonHexes, droppedGroups, channels []string) (int64, error) {
	stale := func(entry string) bool {
		group, _, _ := strings.Cut(entry, "/")
		source, _, _ := strings.Cut(strings.TrimPrefix(entry, iptv.IDPrefix), ":")
		return slices.Contains(addonHexes, group) || strings.HasPrefix(entry, iptv.IDPrefix) && slices.Contains(addonHexes, source) ||
			slices.ContainsFunc(droppedGroups, func(prefix string) bool { return strings.HasPrefix(entry, prefix) }) || slices.Contains(channels, entry)
	}
	var pruned int64
	for _, table := range []string{"users", "user_groups"} {
		rows, err := tx.Query(ctx, "SELECT id, live_tv_channels FROM "+table+" WHERE live_tv_channels->>'mode' <> 'all' ORDER BY id")
		if err != nil {
			return pruned, err
		}
		var ids []accounts.ID
		var rawRules []string
		var id accounts.ID
		var raw string
		if _, err := pgx.ForEachRow(rows, []any{&id, &raw}, func() error {
			ids, rawRules = append(ids, id), append(rawRules, raw)
			return nil
		}); err != nil {
			return pruned, err
		}
		for i := range ids {
			var rule accounts.ChannelRule
			if err := json.Unmarshal([]byte(rawRules[i]), &rule); err != nil {
				return pruned, err
			}
			kept := rule
			kept.Groups = slices.DeleteFunc(slices.Clone(rule.Groups), stale)
			kept.KnownGroups = slices.DeleteFunc(slices.Clone(rule.KnownGroups), stale)
			kept.Channels = slices.DeleteFunc(slices.Clone(rule.Channels), stale)
			if len(kept.Groups) == len(rule.Groups) && len(kept.KnownGroups) == len(rule.KnownGroups) && len(kept.Channels) == len(rule.Channels) {
				continue
			}
			encoded, err := json.Marshal(kept)
			if err != nil {
				return pruned, err
			}
			// A limit changed meanwhile is left as it was saved.
			tag, err := tx.Exec(ctx, "UPDATE "+table+" SET live_tv_channels = $2::jsonb WHERE id = $1 AND live_tv_channels = $3::jsonb", ids[i],
				string(encoded), rawRules[i])
			if err != nil {
				return pruned, err
			}
			pruned += tag.RowsAffected()
		}
	}
	return pruned, nil
}

// forgetLeftovers forgets, once their cleanup is stored, what memory keeps
// of the leftovers of l: the generated images and uploaded artwork of its
// items, the page sizes of its catalogs, the version lists of its addons,
// and the health of the addresses no addon installed has any more; but
// for those of addons, still installed in memory, that the cleanup removed
// (removed).
func (s *Service) forgetLeftovers(ctx context.Context, l leftovers, removed ...accounts.ID) {
	views := slices.Concat(l.libraries, l.collections)
	s.generated.mu.Lock()
	for _, id := range views {
		delete(s.generated.entries, id)
	}
	s.generated.mu.Unlock()
	// Read again as next needed.
	s.overridesMu.Lock()
	s.overrides.Store(nil)
	s.overridesMu.Unlock()
	// Catalog arrivals were deleted with the catalogs, which saves read
	// (see savedRecords).
	s.forgetAllSaved()
	s.sizes.mu.Lock()
	for key := range s.sizes.byCatalog {
		if slices.Contains(l.addons, key.addon) || slices.Contains(l.catalogs, droppedCatalog(key)) {
			delete(s.sizes.byCatalog, key)
		}
	}
	s.sizes.mu.Unlock()
	for _, addon := range l.addons {
		if _, err := s.ForgetAddonVersionLists(ctx, addon); err != nil && ctx.Err() == nil {
			s.logger.Debug("The version lists of a removed addon could not be forgotten", "error", err)
		}
	}
	installed, err := s.addons.All(ctx)
	if err != nil {
		return
	}
	addresses := map[string]bool{}
	for _, addon := range installed {
		if !slices.Contains(removed, addon.ID) {
			addresses[stremio.BaseURL(addon.ManifestURL)] = true
		}
	}
	s.client.KeepHealth(func(base string) bool { return addresses[base] })
}

// RemovalLoss counts what removing a source loses, that adding it again
// does not bring back: the home rows on its catalogs and collections, its
// libraries with their settings, the users who have one of them hidden,
// and the recordings scheduled on its channels.
type RemovalLoss struct {
	HomeRows        int
	Libraries       int
	HiddenLibraries int
	Recordings      int
}

// RemovalLoss counts what removing the scope's addon id would lose (see
// RemovalLoss); addons.ErrNotFound when the scope has no such addon.
func (s *Service) RemovalLoss(ctx context.Context, scope addons.Scope, id accounts.ID) (RemovalLoss, error) {
	var loss RemovalLoss
	list, err := s.addons.Addons(ctx, scope)
	if err != nil {
		return loss, err
	}
	i := slices.IndexFunc(list, func(a addons.Addon) bool { return a.ID == id })
	if i < 0 {
		return loss, addons.ErrNotFound
	}
	err = pgx.BeginTxFunc(ctx, s.db, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		l, err := addonLeftovers(ctx, tx, list[i])
		if err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT
				(SELECT count(*) FROM home_rows WHERE addon_id = $1 OR kind = 'collection' AND collection_id = ANY($2)),
				(SELECT count(*) FROM libraries WHERE addon_id = $1),
				(SELECT count(*) FROM users WHERE hidden_libraries && $3),
				(SELECT count(*) FROM live_timers WHERE channel_id = ANY($4) AND status = 'New')
					+ (SELECT count(*) FROM live_series_timers WHERE channel_id = ANY($4) AND NOT record_any_channel)`,
			id, l.collections, l.libraries, l.channels).Scan(&loss.HomeRows, &loss.Libraries, &loss.HiddenLibraries, &loss.Recordings)
	})
	return loss, err
}

// MissingSources returns those of ids, the items own channels air (see
// virtualtv.Source), that are gone: neither a title, series or collection
// recorded, nor a collection of users, nor one of the server's smart lists
// with a catalog.
func (s *Service) MissingSources(ctx context.Context, ids []accounts.ID) ([]accounts.ID, error) {
	rows, err := s.db.Query(ctx, `SELECT wanted.id FROM unnest($1::uuid[]) wanted(id)
		WHERE NOT EXISTS (SELECT FROM items i WHERE i.id = wanted.id) AND NOT EXISTS (SELECT FROM collections c WHERE c.id = wanted.id)`, nonNil(ids))
	if err != nil {
		return nil, err
	}
	unknown, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
	if err != nil || len(unknown) == 0 {
		return unknown, err
	}
	lists, err := s.smart.Lists(ctx, nil)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(unknown, func(id accounts.ID) bool {
		return slices.ContainsFunc(lists, func(l smartlists.List) bool { return len(l.Catalogs) > 0 && SmartListItemID(l.ID) == id })
	}), nil
}

// nonNil is s, or an empty list for nil.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
