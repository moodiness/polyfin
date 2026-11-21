package library

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Addon libraries sorted.
//
// A listing of an addon's library sorted by another key than its
// catalog's order sorts the titles Polyfin read of the catalog: those of
// the pages kept, in memory or in the database, up to the most titles the
// library lists (see view.limit), without asking the addon. Asking for
// them reads the rest of the catalog in the background (see readThrough),
// so that the next listings sort more of it, then all of it. A title's
// date added is when Polyfin first saw it in the catalog (see
// arrivals.go).

// catalogLibrary returns the library parent of v and its catalog, narrowed
// to genre, when parent is one of v's libraries of an addon's catalog of
// titles, which a sorted listing sorts the titles read of (see
// CatalogPool); ok is false otherwise.
func catalogLibrary(v view, parent accounts.ID, genre string) (l library, src source, ok bool) {
	l, ok = v.library(parent)
	if !ok || l.stored() || own(l.addon.addon) || l.addon.addon.HasMusic() || l.catalog.Type == "collection" || LiveCatalog(l.catalog.Type) {
		return library{}, source{}, false
	}
	src = l.catalogSource()
	if genre != "" {
		src.genre = genre
	}
	return l, src, true
}

// CatalogVersion returns the version of the titles a sorted listing of
// library parent, narrowed to genre, sorts (see catalogVersion): a listing
// sorted at one version sorts the same titles (see CatalogPool). ok is
// false for a parent CatalogPool does not list.
func (s *Service) CatalogVersion(ctx context.Context, user accounts.User, parent accounts.ID, genre string) (version string, ok bool,
	err error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return "", false, err
	}
	_, src, ok := catalogLibrary(v, parent, genre)
	if !ok {
		return "", false, nil
	}
	return s.catalogVersion(src), true, nil
}

// catalogVersion names the titles of src's catalog as a sorted listing
// reads them: the version of its pages (see catalogVersions), with the
// count of administrators' edits (see changeOverrides) and of the changes
// to the addons and their libraries (see addons.Store.Changes), which
// rename titles and bound how many a library lists.
func (s *Service) catalogVersion(src source) string {
	return strconv.FormatUint(s.catalogVersions.of(src), 10) + "." + strconv.FormatUint(s.editChanges.Load()+s.addons.Changes(), 10)
}

// CatalogPool lists the titles of library parent read so far of its
// addon's catalog, narrowed to genre, as a listing of them sorts them,
// with what the user may see only, and has the rest of the catalog read in
// the background (see readThrough). The first need titles of the catalog
// are read first as a listing of them in the catalog's order reads them,
// asking the addon for the pages not kept, so that a library sorted before
// it was ever listed sorts at least those. Each title holds when it was
// first seen in the catalog (see Item.Added). More tells that the catalog
// was read neither to its end nor up to the most titles the library
// lists. version is the catalog's (see CatalogVersion) as it was before
// its pages were read. ok is false when parent is not one of the user's
// libraries of an addon's catalog of titles: a library Polyfin stores,
// one of collections or of music, or another folder.
func (s *Service) CatalogPool(ctx context.Context, user accounts.User, parent accounts.ID, genre string, need int) (
	page Page, version string, ok bool, err error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return Page{}, "", false, err
	}
	l, src, ok := catalogLibrary(v, parent, genre)
	if !ok {
		return Page{}, "", false, nil
	}
	s.readThrough(WithOpening(ctx), v, src)
	version = s.catalogVersion(src)
	most := v.limit(src)
	if l.maxItems > 0 {
		most = min(most, l.maxItems)
	}
	if need > 0 {
		if _, err := s.Children(ctx, user, parent, 0, min(need, most), genre); err != nil {
			return Page{}, "", true, err
		}
	}
	page, err = s.Children(withKeptOnly(ctx), user, parent, 0, most, genre)
	if err != nil {
		return Page{}, "", true, err
	}
	// A pool that holds as many titles as the library lists is whole,
	// though the catalog may list more.
	page.More = page.More && len(page.Items) < most
	if err := s.addedTitles(ctx, src, page.Items); err != nil {
		return Page{}, "", true, err
	}
	return page, version, true, nil
}

// addedTitles gives items, titles of src's catalog, when they were first
// seen in it (see recordArrivals), in one statement.
func (s *Service) addedTitles(ctx context.Context, src source, items []Item) error {
	if len(items) == 0 {
		return nil
	}
	rows, _ := s.db.Query(ctx, `SELECT item_id, first_seen FROM catalog_arrivals
		WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3 AND genre = $4`,
		src.addon.addon.ID, src.catalog.Type, src.catalog.ID, src.genre)
	seen := make(map[accounts.ID]time.Time, len(items))
	var id accounts.ID
	var first time.Time
	if _, err := pgx.ForEachRow(rows, []any{&id, &first}, func() error {
		seen[id] = first
		return nil
	}); err != nil {
		return err
	}
	for i := range items {
		if first, ok := seen[items[i].ID]; ok {
			items[i].Added = &first
		}
	}
	return nil
}
