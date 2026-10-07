package library

import (
	"context"
	"slices"

	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
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
// in turn. It returns the titles [start, start+count). A library narrowed
// to a genre adds its catalog to that genre's page only.
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
	read := map[catalogKey]bool{}
	for _, c := range catalogs {
		if kind, _ := titleKind(c.catalog.Type); !slices.Contains(kinds, kind) {
			continue
		}
		options := c.options()
		i := slices.IndexFunc(options, matches)
		if i < 0 {
			continue
		}
		// A catalog reached both through a library narrowed to the genre
		// and otherwise is read once.
		key := catalogKey{c.addon.addon.ID, c.catalog.Type, c.catalog.ID, options[i]}
		if !read[key] {
			read[key] = true
			sources = append(sources, source{addon: c.addon, catalog: c.catalog, genre: options[i]})
		}
	}
	if len(sources) == 0 {
		return Page{}, nil
	}
	titles, total, err := s.merged(ctx, v, sources, start, count)
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
	return page(s.overridden(items), start, total), s.save(ctx, records)
}

// titleCatalog is a catalog of titles a user reaches, with its addon, and
// the genre the library listing it is narrowed to, if any.
type titleCatalog struct {
	addon   installed
	catalog stremio.Catalog
	genre   string
}

// options are the genre options the catalog is listed under: its library's
// genre when it is narrowed to one, else every option of its genre filter.
func (c titleCatalog) options() []string {
	if c.genre != "" {
		return []string{c.genre}
	}
	return addons.Genres(c.catalog)
}

// catalogKey identifies a catalog among those of all addons, as narrowed
// to a genre by a library.
type catalogKey struct {
	addon                  accounts.ID
	catalogType, id, genre string
}

// titleCatalogs lists, each once, the catalogs of titles a user reaches:
// those of their movie and series libraries, then those their collection
// libraries group, nested collections included. Users who enabled an
// addon's collections often have no other library, so their titles are
// only reachable this way. A collection library that cannot be listed is
// left out. A catalog listed by a library narrowed to a genre is listed
// with that genre, and again without it when it is reached another way.
func (s *Service) titleCatalogs(ctx context.Context, v view) ([]titleCatalog, error) {
	seen := map[catalogKey]bool{}
	var result []titleCatalog
	add := func(addon installed, catalog stremio.Catalog, genre string) {
		key := catalogKey{addon.addon.ID, catalog.Type, catalog.ID, genre}
		if _, ok := titleKind(catalog.Type); ok && !seen[key] {
			seen[key] = true
			result = append(result, titleCatalog{addon: addon, catalog: catalog, genre: genre})
		}
	}
	var collections []library
	for _, l := range v.libraries {
		if l.catalog.Type == "collection" {
			collections = append(collections, l)
		} else {
			add(l.addon, l.catalog, l.genre)
		}
	}
	for _, l := range collections {
		grouped, err := s.groupedCatalogs(ctx, v, l)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			s.logger.Warn("A collection library could not be listed", "catalog", l.catalog.ID, "error", err)
			continue
		}
		for _, catalog := range grouped {
			add(l.addon, catalog, "")
		}
	}
	return result, nil
}

// groupedCatalogs lists the catalogs the collections of a collection
// library group, following the collections within them. A collection
// groups catalogs of its own addon. Descriptions are cached, so this
// mostly costs requests the first time; a collection that cannot be
// described is left out.
func (s *Service) groupedCatalogs(ctx context.Context, v view, l library) ([]stremio.Catalog, error) {
	src := l.catalogSource()
	entries, _, err := s.window(ctx, v, src, 0, v.limit(src))
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
