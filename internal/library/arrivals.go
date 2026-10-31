package library

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Titles new in the catalogs, and catalogs listed for themselves.
//
// Polyfin keeps when it first saw each title in each catalog it reads
// from an addon, by genre (catalog_arrivals), for the web player's home
// row of the titles new in a user's libraries (see Arrivals). A catalog's
// titles are seen as its pages come from the addon: as apps list its
// libraries, and as Warm reads their first pages every hour. The titles of
// a catalog's first read, and those of a page further than the catalog was
// ever read, were there before Polyfin looked: they are kept as known, not
// as new. A catalog no read reached for catalogForgotten is forgotten with
// its titles, and so is a title no read saw for arrivalForgotten: a title
// that comes back after that is new again. IPTV sources and local folders
// read their own lists, and searches are not catalogs: none of them is
// followed.

const (
	// ArrivalWindow is how long a title stays new in a catalog.
	ArrivalWindow = 7 * 24 * time.Hour
	// catalogForgotten is how long a catalog not read keeps its titles.
	catalogForgotten = 30 * 24 * time.Hour
	// arrivalForgotten is how long a title no read saw stays known.
	arrivalForgotten = 180 * 24 * time.Hour
	// maxArrivals bounds the new titles a listing describes.
	maxArrivals = 500
)

// arrivalKinds are what the home row of new titles lists.
var arrivalKinds = []Kind{KindMovie, KindSeries, KindVideo}

// recordArrivals notes the titles of a page src's addon gave at skip, at
// seen: new to the catalog when the page lies within what was read of it
// before. The records of the titles new to Polyfin are saved, so that the
// row of new titles describes them before any listing did.
func (s *Service) recordArrivals(ctx context.Context, src source, skip int, metas []stremio.Meta, seen time.Time) error {
	if LiveCatalog(src.catalog.Type) || src.catalog.Type == "collection" || src.search != "" || src.date != "" {
		return nil
	}
	var records []record
	for _, meta := range metas {
		if _, r, err := titleItem(src.addon.addon.ID, src.catalog, meta, accounts.ID{}, src.addon.confined); err == nil {
			// The listing that reads the page saves its records with their
			// folder; these keep any folder they have.
			r.Parent = nil
			records = append(records, r)
		}
	}
	records = inLockOrderValues(records)
	ids := make([]accounts.ID, 0, len(records))
	for _, r := range records {
		ids = append(ids, r.ID)
	}
	key := []any{src.addon.addon.ID, src.catalog.Type, src.catalog.ID, src.genre}
	var fresh []accounts.ID
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO catalog_reach (addon_id, catalog_type, catalog_id, genre, reach, read_at)
			VALUES ($1, $2, $3, $4, 0, $5) ON CONFLICT DO NOTHING`, append(key, seen)...); err != nil {
			return err
		}
		var reach int
		if err := tx.QueryRow(ctx, `SELECT reach FROM catalog_reach
			WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3 AND genre = $4 FOR UPDATE`, key...).Scan(&reach); err != nil {
			return err
		}
		// Only a page that starts within the reach extends it: one beyond
		// it leaves unread titles before it, which were there all along.
		end := reach
		if skip <= reach {
			end = max(reach, skip+len(metas))
		}
		if _, err := tx.Exec(ctx, `UPDATE catalog_reach SET reach = $5, read_at = greatest(read_at, $6)
			WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3 AND genre = $4`, append(key, end, seen)...); err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		rows, _ := tx.Query(ctx, `INSERT INTO catalog_arrivals (addon_id, catalog_type, catalog_id, genre, item_id, first_seen, last_seen, arrived)
			SELECT $1, $2, $3, $4, id, $6, $6, $7 FROM unnest($5::uuid[]) AS i(id) ORDER BY id
			ON CONFLICT (addon_id, catalog_type, catalog_id, genre, item_id) DO UPDATE SET last_seen = greatest(catalog_arrivals.last_seen, excluded.last_seen)
			RETURNING item_id, xmax = 0`, append(key, ids, seen, skip < reach)...)
		var id accounts.ID
		var inserted bool
		_, err := pgx.ForEachRow(rows, []any{&id, &inserted}, func() error {
			if inserted {
				fresh = append(fresh, id)
			}
			return nil
		})
		return err
	})
	if err != nil || len(fresh) == 0 {
		return err
	}
	return s.saveNew(ctx, slices.DeleteFunc(records, func(r record) bool { return !slices.Contains(fresh, r.ID) }))
}

// inLockOrderValues is inLockOrder for records held by value.
func inLockOrderValues(records []record) []record {
	unique := inLockOrder(records)
	result := make([]record, 0, len(unique))
	for _, r := range unique {
		result = append(result, *r)
	}
	return result
}

// forgetArrivals forgets the catalogs no read reached for
// catalogForgotten, and the titles no read saw for arrivalForgotten. Both
// take their rows in key order, as recordArrivals does.
func (s *Service) forgetArrivals(ctx context.Context) error {
	if _, err := s.db.Exec(ctx, `DELETE FROM catalog_reach WHERE (addon_id, catalog_type, catalog_id, genre) IN (
		SELECT addon_id, catalog_type, catalog_id, genre FROM catalog_reach WHERE read_at < $1
		ORDER BY addon_id, catalog_type, catalog_id, genre FOR UPDATE)`, s.now().Add(-catalogForgotten)); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `DELETE FROM catalog_arrivals WHERE (addon_id, catalog_type, catalog_id, genre, item_id) IN (
		SELECT addon_id, catalog_type, catalog_id, genre, item_id FROM catalog_arrivals WHERE last_seen < $1
		ORDER BY addon_id, catalog_type, catalog_id, genre, item_id FOR UPDATE)`, s.now().Add(-arrivalForgotten))
	return err
}

// Arrivals lists from start the titles that came into the catalogs of the
// user's libraries within ArrivalWindow, the newest first: those none of
// these catalogs held before. A library narrowed to a genre counts the
// titles of that genre; smart lists, music, IPTV sources and local folders
// are left out (see the top of this file). Titles the user may not see,
// by parental control or blocked genres, and those they hid, are left
// out. Titles show the user's language as far as it is kept (see
// itemsOf). first describes only as many titles as the window shows, as
// a home row does; a listing describes them all to count them.
func (s *Service) Arrivals(ctx context.Context, user accounts.User, start, count int, first bool) (Page, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return Page{}, err
	}
	var addonIDs []accounts.ID
	var types, catalogs, genres []string
	for _, l := range v.libraries {
		if l.addon.addon.HasMusic() || own(l.addon.addon) || l.catalog.Type == "collection" || LiveCatalog(l.catalog.Type) {
			continue
		}
		addonIDs, types, catalogs, genres = append(addonIDs, l.addon.addon.ID), append(types, l.catalog.Type),
			append(catalogs, l.catalog.ID), append(genres, l.genre)
	}
	if len(addonIDs) == 0 {
		return Page{}, nil
	}
	// A title counts from the first time one of the catalogs held it, and
	// only when none held it before Polyfin looked.
	rows, _ := s.db.Query(ctx, `SELECT a.item_id FROM catalog_arrivals a
		JOIN unnest($1::uuid[], $2::text[], $3::text[], $4::text[]) AS l(addon_id, catalog_type, catalog_id, genre)
			USING (addon_id, catalog_type, catalog_id, genre)
		GROUP BY a.item_id HAVING bool_and(a.arrived) AND min(a.first_seen) > $5
		ORDER BY min(a.first_seen) DESC, a.item_id LIMIT $6`,
		addonIDs, types, catalogs, genres, s.now().Add(-ArrivalWindow), maxArrivals)
	var ids []accounts.ID
	var id accounts.ID
	if _, err := pgx.ForEachRow(rows, []any{&id}, func() error {
		if !v.hidden[id] {
			ids = append(ids, id)
		}
		return nil
	}); err != nil {
		return Page{}, err
	}
	return s.describedPage(ctx, v, ids, arrivalKinds, start, count, first)
}

// describedPage describes the titles ids from start, in order, those the
// user may not see left out. first describes only as many as the window
// shows; otherwise all are described to count them.
func (s *Service) describedPage(ctx context.Context, v view, ids []accounts.ID, kinds []Kind, start, count int, first bool) (Page, error) {
	if !first {
		items, err := s.itemsOf(ctx, v, ids, kinds)
		return slicePage(s.overridden(items), start, count), err
	}
	want := start + count
	var items []Item
	from := 0
	for from < len(ids) && len(items) < want {
		to := min(from+want-len(items), len(ids))
		described, err := s.itemsOf(ctx, v, ids[from:to], kinds)
		if err != nil {
			return Page{}, err
		}
		items, from = append(items, described...), to
	}
	page := slicePage(s.overridden(items), start, count)
	page.More = from < len(ids)
	return page, nil
}

// CatalogTitles lists from start the titles of a catalog of one of the
// server's addons, a library or not, as a library lists them: first reads
// its first page only, as a home row shows it. The user must see the
// server's addons: a catalog of an addon they do not use, turned off, or
// one of a library of the server hidden from them, is ErrNotFound. So is
// a catalog that cannot be browsed: a live TV catalog, a collection
// catalog, or one that needs a search. Its titles carry the library of
// the catalog the user sees, if any, and leave out what local folders
// hold when that library does (see skiplocal.go); the titles the user may
// not see, by parental control or blocked genres, and those they hid, are
// left out.
// Titles show the user's language as far as it is kept (see translate).
func (s *Service) CatalogTitles(ctx context.Context, user accounts.User, addon accounts.ID, catalogType, catalogID string, start, count int,
	first bool) (Page, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return Page{}, err
	}
	entry, ok := v.addon(addon)
	if !ok || !entry.shared || entry.addon.HasMusic() {
		return Page{}, ErrNotFound
	}
	i := slices.IndexFunc(entry.addon.Manifest.Catalogs, func(c stremio.Catalog) bool { return c.Type == catalogType && c.ID == catalogID })
	if i < 0 || !smartCatalog(entry.addon.Manifest.Catalogs[i]) {
		return Page{}, ErrNotFound
	}
	catalog := entry.addon.Manifest.Catalogs[i]
	hidden, err := s.hiddenCatalog(ctx, user, addon, catalogType, catalogID)
	if err != nil || hidden {
		return Page{}, cmp.Or(err, ErrNotFound)
	}
	src := source{addon: entry, catalog: catalog, first: first}
	var parent accounts.ID
	if j := slices.IndexFunc(v.libraries, func(l library) bool {
		return l.addon.addon.ID == addon && l.catalog.Type == catalogType && l.catalog.ID == catalogID && l.genre == ""
	}); j >= 0 {
		parent = v.libraries[j].item.ID
		src.max, src.skipLocal = v.libraries[j].maxItems, v.libraries[j].skipLocal
		count = withinMax(start, count, src.max)
	}
	metas, total, err := s.window(ctx, v, src, start, count)
	if err != nil {
		return Page{}, err
	}
	var items []Item
	var records []record
	for _, meta := range metas {
		item, r, err := titleItem(addon, catalog, meta, parent, entry.confined)
		if err != nil {
			continue
		}
		if parent == (accounts.ID{}) {
			// Outside a library, a title keeps the folder it has (see save).
			r.Parent = nil
		}
		items, records = append(items, item), append(records, r)
	}
	s.translate(ctx, v, items)
	return page(s.overridden(items), start, totalWithinMax(total, src.max)), s.save(ctx, records)
}

// hiddenCatalog reports whether one of the server's libraries of a
// catalog is hidden from user.
func (s *Service) hiddenCatalog(ctx context.Context, user accounts.User, addon accounts.ID, catalogType, catalogID string) (bool, error) {
	if len(user.HiddenLibraries) == 0 {
		return false, nil
	}
	libraries, err := s.addons.Libraries(ctx, addons.Shared())
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(libraries, func(l addons.Library) bool {
		return l.AddonID == addon && l.Catalog.Type == catalogType && l.Catalog.ID == catalogID && slices.Contains(user.HiddenLibraries, LibraryID(l))
	}), nil
}
