package library

import (
	"context"
	"slices"
	"sync/atomic"
	"time"

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
	// narrowedWait is how long a page of narrowed titles waits for its
	// catalogs before it answers with those read by then (see Narrowed).
	narrowedWait = 1500 * time.Millisecond
)

// atOnceKey marks the context of a read of narrowed titles that answers at
// once with what is ready (see AtOnce).
type atOnceKey struct{}

// AtOnce marks ctx's reads of narrowed titles, such as the counts of a
// genre's page, as answering at once with what is read already, rather
// than waiting for the catalogs (see Narrowed). They still start the
// reads, for what follows.
func AtOnce(ctx context.Context) context.Context {
	return context.WithValue(ctx, atOnceKey{}, true)
}

func atOnce(ctx context.Context) bool {
	marked, _ := ctx.Value(atOnceKey{}).(bool)
	return marked
}

// SetNarrowedWait replaces, before the service is used, how long a page
// of narrowed titles waits for its catalogs (see Narrowed). Tests shorten
// it; the server keeps narrowedWait.
func (s *Service) SetNarrowedWait(wait time.Duration) { s.narrowedWait = wait }

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
//
// A common genre may be offered by a hundred catalogs, of an addon asked
// for a few pages at once (see catalogHostFetches): the listing waits for
// them until narrowedWait after it started at most, then answers with the
// titles of the catalog pages read by then, and tells that more may
// follow. A read marked to answer at once (see AtOnce) does not wait. The
// catalogs are read on, to the end of the listing, for the next one (see
// mergedWithin).
func (s *Service) Narrowed(ctx context.Context, user accounts.User, matches func(option string) bool, kinds []Kind, start, count int) (Page, error) {
	page, _, err := s.narrowed(ctx, user, matches, kinds, start, count)
	return page, err
}

// narrowed is Narrowed, and tells whether catalogs were still read when it
// answered.
func (s *Service) narrowed(ctx context.Context, user accounts.User, matches func(option string) bool, kinds []Kind, start, count int) (
	_ Page, pending bool, _ error) {
	deadline := time.Now().Add(s.narrowedWait)
	v, err := s.view(ctx, user)
	if err != nil {
		return Page{}, false, err
	}
	catalogs, err := s.titleCatalogs(ctx, v)
	if err != nil {
		return Page{}, false, err
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
		return Page{}, false, nil
	}
	titles, total, pending, err := s.mergedWithin(ctx, v, sources, start, count, deadline)
	if err != nil {
		return Page{}, false, err
	}
	var items []Item
	var records []record
	for _, title := range titles {
		item, r, err := title.title(s, accounts.ID{}, v.country)
		if err != nil {
			continue
		}
		// Merged catalogs do not tell which library listed a title: like a
		// search result, it keeps the folder it has.
		r.Parent = nil
		items, records = append(items, item), append(records, r)
	}
	s.translate(ctx, v, items)
	return page(s.overridden(items), start, total), pending, s.save(ctx, records)
}

// merging is the outcome of a read of narrowed titles (see merged).
type merging struct {
	titles []listed
	total  int
	err    error
}

// mergedWithin merges sources as merged does, waiting until deadline at
// most. The read runs on its own, to the end, and a request that stops
// waiting does not cancel it: the server stopping does (see EndWith). Its
// catalog pages yield to those a request waits for (see withYielding),
// and a page another read asks for already is shared rather than asked
// again (see readPage), so that reads started together, by a page's counts
// and its listing, ask each page once. When it did not end in time, the
// titles are those of the pages kept in memory then (see mergedReady), and
// pending is set unless those pages held the whole listing. A read marked
// AtOnce takes those pages without waiting, and starts the read only when
// they do not hold the listing.
func (s *Service) mergedWithin(ctx context.Context, v view, sources []source, start, count int, deadline time.Time) (
	titles []listed, total int, pending bool, err error) {
	if atOnce(ctx) {
		titles, total, complete := s.mergedReady(ctx, v, sources, start, count)
		if !complete {
			s.readOn(ctx, v, sources, start, count)
		}
		return titles, total, !complete, nil
	}
	results := s.readOn(ctx, v, sources, start, count)
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case result := <-results:
		return result.titles, result.total, false, result.err
	case <-ctx.Done():
		return nil, 0, false, ctx.Err()
	case <-timer.C:
	}
	titles, total, complete := s.mergedReady(ctx, v, sources, start, count)
	return titles, total, !complete, nil
}

// readOn merges sources as merged does, on its own (see mergedWithin), and
// sends what it read once it ends.
func (s *Service) readOn(ctx context.Context, v view, sources []source, start, count int) <-chan merging {
	results := make(chan merging, 1)
	reading, done := s.detached(withYielding(ctx))
	// What the read leaves out for want of ratings is its own: it does
	// not cut the request's other listings short.
	v.held = new(atomic.Bool)
	go func() {
		defer done()
		titles, total, err := s.merged(reading, v, sources, start, count)
		results <- merging{titles, total, err}
	}()
	return results
}

// mergedReady merges sources as merged does, from the catalog pages kept in
// memory alone, each catalog read once, as far as start+count, so that it
// answers at once (see readyOnly). The user's limits apply as on any
// listing, but without waiting for the ratings and release dates not
// known yet: those titles are left out. complete reports that what was
// read holds every title merged would have read, and the user's limits
// left nothing out for want of what is not known yet: the titles and the
// total are then merged's. Otherwise they are those of every page read,
// plus one for the total, as more may follow.
func (s *Service) mergedReady(ctx context.Context, v view, sources []source, start, count int) (_ []listed, total int, complete bool) {
	v = v.withoutWait()
	// What this read leaves out does not stop the read going on.
	v.held = new(atomic.Bool)
	ready := withReadyOnly(ctx)
	need := start + count
	lists := make([][]stremio.Meta, len(sources))
	mores := make([]bool, len(sources))
	limit := 0
	var group errgroup.Group
	group.SetLimit(catalogFetches)
	for i, src := range sources {
		limit = max(limit, v.limit(src))
		group.Go(func() error {
			metas, total, _ := s.window(ready, v, src, 0, need)
			lists[i], mores[i] = metas, total > len(metas)
			return nil
		})
	}
	_ = group.Wait()
	// merged's rounds, on what was read: an even share of each catalog,
	// then more of each while the listing is short; a single catalog is
	// read as far as the listing goes. A catalog that has more than was
	// read, as memory lacked its next page or the read stopped at
	// start+count, cannot give a larger share.
	per := need/len(sources) + 1
	if len(sources) == 1 {
		per = need
	}
	complete = !v.held.Load()
	for complete {
		shares := make([][]stremio.Meta, len(lists))
		anyMore := false
		for i, list := range lists {
			shares[i] = list[:min(per, len(list))]
			anyMore = anyMore || len(list) > per || mores[i]
			complete = complete && (per <= len(list) || !mores[i])
		}
		if !complete {
			break
		}
		result := interleave(sources, shares)
		if len(result) >= need || !anyMore || per >= limit {
			total = len(result)
			if anyMore {
				total++
			}
			return result[min(start, len(result)):min(need, len(result))], total, true
		}
		per += need - len(result)
	}
	result := interleave(sources, lists)
	return result[min(start, len(result)):min(need, len(result))], len(result) + 1, false
}

// GenreOption returns the first genre option of the catalogs of titles the
// user reaches that matches, as Narrowed reads them; ok is false for none.
func (s *Service) GenreOption(ctx context.Context, user accounts.User, matches func(option string) bool) (option string, ok bool, err error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return "", false, err
	}
	catalogs, err := s.titleCatalogs(ctx, v)
	if err != nil {
		return "", false, err
	}
	for _, c := range catalogs {
		options := c.options()
		if i := slices.IndexFunc(options, matches); i >= 0 {
			return options[i], true, nil
		}
	}
	return "", false, nil
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
		grouped, _, err := s.groupedSources(ctx, v, l, l.catalogSource())
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			s.logger.Warn("A collection library could not be listed", "catalog", l.catalog.ID, "error", err)
			continue
		}
		for _, src := range grouped {
			add(l.addon, src.catalog, "")
		}
	}
	return result, nil
}

// groupedSources lists, each once, the catalogs the collections of a
// collection library group, as each narrows them, following the
// collections within them: the library's collections are those its catalog
// lists as src. A collection groups catalogs of its own addon, listed up to
// the library's maximum. Descriptions are cached, so this mostly costs
// requests the first time; a collection that cannot be described is left
// out. unknown is set when a read that asks no addon does not know every
// collection of the library (see withKeptOnly).
func (s *Service) groupedSources(ctx context.Context, v view, l library, src source) (sources []source, unknown bool, err error) {
	entries, total, err := s.window(ctx, v, src, 0, v.limit(src))
	if err != nil {
		return nil, false, err
	}
	unknown = keptOnly(ctx) && total > len(entries)
	described := map[string]bool{}
	seen := map[catalogKey]bool{}
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
			return nil, false, ctx.Err()
		}
		entries = nil
		for _, meta := range metas {
			if meta.Collection == nil {
				continue
			}
			for _, ref := range meta.Collection.Sources {
				key := catalogKey{l.addon.addon.ID, ref.Type, ref.CatalogID, ref.Genre}
				if catalog, ok := l.addon.addon.Manifest.Catalog(ref.Type, ref.CatalogID); ok && !seen[key] {
					seen[key] = true
					sources = append(sources, source{addon: l.addon, catalog: catalog, genre: ref.Genre, first: src.first, max: l.maxItems})
				}
			}
			entries = append(entries, meta.Collection.Items...)
		}
	}
	return sources, unknown, nil
}
