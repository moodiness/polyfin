package library

import (
	"context"
	"slices"

	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

const (
	// metaFetches bounds the collection descriptions fetched at once.
	metaFetches = 8
	// maxNesting bounds how deep collections within collections are
	// followed.
	maxNesting = 3
)

// Narrowed lists the titles of the given kinds that the user's catalogs
// list for a genre option: each catalog of titles the user reaches (see
// titleCatalogs) whose genre extra offers an option that matches is
// narrowed to that option, and the catalogs are merged, one title of each
// in turn. It returns the titles [start, start+count).
//
// The genre extra is the only filter Stremio catalogs accept, and addons
// put genres in it, but also years, studios or networks; so it is how a
// genre's, a year's or a studio's titles are listed without reading whole
// catalogs. A catalog that offers no matching option adds nothing, even if
// some of its titles would match.
func (s *Service) Narrowed(ctx context.Context, user accounts.User, matches func(option string) bool, kinds []Kind, start, count int) (Page, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return Page{}, err
	}
	catalogs, err := s.titleCatalogs(ctx, v)
	if err != nil {
		return Page{}, err
	}
	var sources []source
	for _, c := range catalogs {
		if kind, _ := titleKind(c.catalog.Type); !slices.Contains(kinds, kind) {
			continue
		}
		for _, extra := range c.catalog.Extra {
			if extra.Name != "genre" {
				continue
			}
			if i := slices.IndexFunc(extra.Options, matches); i >= 0 {
				sources = append(sources, source{addon: c.addon, catalog: c.catalog, genre: extra.Options[i]})
			}
			break
		}
	}
	if len(sources) == 0 {
		return Page{}, nil
	}
	titles, total, err := s.merged(ctx, sources, start, count)
	if err != nil {
		return Page{}, err
	}
	var items []Item
	var records []record
	for _, title := range titles {
		item, r, err := title.title(accounts.ID{})
		if err != nil {
			continue
		}
		// Merged catalogs do not tell which library listed a title: like a
		// search result, it keeps the folder it was last listed in.
		r.Parent = nil
		items, records = append(items, item), append(records, r)
	}
	return page(items, start, total), s.save(ctx, records)
}

// titleCatalog is a catalog of titles a user reaches, with its addon.
type titleCatalog struct {
	addon   installed
	catalog stremio.Catalog
}

// catalogKey identifies a catalog among those of all addons.
type catalogKey struct {
	addon           accounts.ID
	catalogType, id string
}

// titleCatalogs lists, each once, the catalogs of titles a user reaches:
// those of their movie and series libraries, then those their collection
// libraries group, nested collections included. Users who enabled an
// addon's collections often have no other library, so their titles are
// only reachable this way. A collection library that cannot be listed is
// left out.
func (s *Service) titleCatalogs(ctx context.Context, v view) ([]titleCatalog, error) {
	seen := map[catalogKey]bool{}
	var result []titleCatalog
	add := func(addon installed, catalog stremio.Catalog) {
		key := catalogKey{addon.addon.ID, catalog.Type, catalog.ID}
		if _, ok := titleKind(catalog.Type); ok && !seen[key] {
			seen[key] = true
			result = append(result, titleCatalog{addon: addon, catalog: catalog})
		}
	}
	var collections []library
	for _, l := range v.libraries {
		if l.catalog.Type == "collection" {
			collections = append(collections, l)
		} else {
			add(l.addon, l.catalog)
		}
	}
	for _, l := range collections {
		grouped, err := s.groupedCatalogs(ctx, l)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			s.logger.Warn("A collection library could not be listed", "catalog", l.catalog.ID, "error", err)
			continue
		}
		for _, catalog := range grouped {
			add(l.addon, catalog)
		}
	}
	return result, nil
}

// groupedCatalogs lists the catalogs the collections of a collection
// library group, following the collections within them. A collection
// groups catalogs of its own addon. Descriptions are cached, so this
// mostly costs requests the first time; a collection that cannot be
// described is left out.
func (s *Service) groupedCatalogs(ctx context.Context, l library) ([]stremio.Catalog, error) {
	entries, _, err := s.window(ctx, source{addon: l.addon, catalog: l.catalog}, 0, maxCrawl)
	if err != nil {
		return nil, err
	}
	described := map[string]bool{}
	var catalogs []stremio.Catalog
	for depth := 0; len(entries) > 0 && depth < maxNesting; depth++ {
		var ids []string
		for _, entry := range entries {
			if entry.ID != "" && !described[entry.ID] {
				described[entry.ID] = true
				ids = append(ids, entry.ID)
			}
		}
		metas := make([]stremio.Meta, len(ids))
		var group errgroup.Group
		group.SetLimit(metaFetches)
		for i, id := range ids {
			group.Go(func() error {
				meta, err := s.meta(ctx, l.addon, "collection", id)
				if err != nil && ctx.Err() == nil {
					s.logger.Debug("A collection could not be described", "collection", id, "error", err)
				}
				metas[i] = meta
				return nil
			})
		}
		_ = group.Wait()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		entries = nil
		for _, meta := range metas {
			if meta.Collection == nil {
				continue
			}
			for _, ref := range meta.Collection.Sources {
				if catalog, ok := l.addon.addon.Manifest.Catalog(ref.Type, ref.CatalogID); ok {
					catalogs = append(catalogs, catalog)
				}
			}
			entries = append(entries, meta.Collection.Items...)
		}
	}
	return catalogs, nil
}
