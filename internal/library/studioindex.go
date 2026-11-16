package library

import (
	"context"
	"math"
	"sync"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/stremio"
)

// The titles whose studios Studios lists.
//
// Studios reads the first CreditsPassMost titles of each library of movies
// or series the user reaches, as the library's pages list them, 100 at a
// time (see Children). A library of one catalog of movies or series (see
// studioPlain) has its pages read here, as libraryChildren reads them, but
// for what only an app's listing needs: the titles are not saved, nor
// translated, nor read ahead. Its catalog is read at once up to the bound,
// then paged from what was read, which lists the same titles: a catalog
// lists the same titles however far it is read, and an IPTV source's
// stored lists are then counted once rather than once a page. A user's
// parental control or blocked genres, or a request's rating filter, read a
// catalog as far as each page asks (see window): their pages are read one
// by one, as before. Other libraries are listed through Children.
//
// The studio index: the catalog of an IPTV source's movies or series read
// from its stored lists, for a user who hid no title and whose listings
// are not narrowed by rating (see window), is the same for every such
// user. What Studios reads of it is kept in memory, by source, catalog,
// genre and bound: each title's identifier and the identifiers its
// listing gives, a few hundred kilobytes for CreditsPassMost titles. It
// holds while the source's version stays the same (see
// iptv.Service.CatalogVersion), which changes whenever its stored lists or
// options do; one statement reads it at each listing. A server restarted
// reads it again. What a title shows is not kept: its item is described
// again at each listing from what was kept, so that the corrections of
// TMDB identifiers (see tmdbidcheck.go) and an administrator's edits apply
// at once, and so do the studios of its kept details. The catalogs of
// addons have no such version: they are read again at each listing, from
// the pages Polyfin keeps (see window).

// studioCatalogsMost bounds the catalogs the studio index holds.
const studioCatalogsMost = 64

// iptvCatalogVersions tells the version of what an IPTV source's movie and
// series catalogs list (see iptv.Service.CatalogVersion). Without it, an
// IPTV source's catalog is read at each listing.
type iptvCatalogVersions interface {
	CatalogVersion(ctx context.Context, source accounts.ID) (string, error)
}

// studioIndex is the studio index (see above), made on first use.
type studioIndex struct {
	once     sync.Once
	catalogs *cache.Cache[studioCatalogKey, studioCatalog]
}

func (x *studioIndex) kept() *cache.Cache[studioCatalogKey, studioCatalog] {
	x.once.Do(func() { x.catalogs = cache.New[studioCatalogKey, studioCatalog](studioCatalogsMost, math.MaxInt64) })
	return x.catalogs
}

// studioCatalogKey identifies a read of an IPTV source's catalog: its
// first count titles, narrowed to genre.
type studioCatalogKey struct {
	source                        accounts.ID
	catalogType, catalogID, genre string
	count                         int
}

// studioCatalog is a read of an IPTV source's catalog kept by the studio
// index: its titles, how many the catalog lists, and the version of the
// source read.
type studioCatalog struct {
	version string
	titles  []studioMeta
	total   int
}

// studioMeta is what titleItem reads of a title to tell its item, its kind
// and its identifiers (see fromMeta and checkedMeta).
type studioMeta struct {
	id, metaType, imdb, tmdb, tvdb, releaseInfo, year string
	fromMoviedb                                       bool
}

func studioMetaOf(m stremio.Meta) studioMeta {
	return studioMeta{id: m.ID, metaType: m.Type, imdb: m.ImdbID, tmdb: string(m.TmdbID), tvdb: string(m.TvdbID), releaseInfo: string(m.ReleaseInfo),
		year: string(m.Year), fromMoviedb: m.TmdbFromMoviedb}
}

func (m studioMeta) meta() stremio.Meta {
	return stremio.Meta{ID: m.id, Type: m.metaType, ImdbID: m.imdb, TmdbID: stremio.Text(m.tmdb), TvdbID: stremio.Text(m.tvdb),
		ReleaseInfo: stremio.Text(m.releaseInfo), Year: stremio.Text(m.year), TmdbFromMoviedb: m.fromMoviedb}
}

// catalogWindow reads a window of a catalog: its titles [start,
// start+count), how many it lists, and why it stopped short, as window
// does.
type catalogWindow func(start, count int) ([]stremio.Meta, int, error)

// studioPlain reports whether library l lists the titles of its catalog as
// the catalog gives them (see libraryChildren): a catalog of movies or of
// series.
func studioPlain(l library) bool {
	_, ok := listedKind(l.catalog.Type)
	return ok && !l.mixed && !l.addon.addon.HasMusic()
}

// studioTitles returns the movies and series of the first CreditsPassMost
// titles of library l, as its pages list them (see above), and why the
// listing stopped, with the titles listed before.
func (s *Service) studioTitles(ctx context.Context, v view, user accounts.User, l library) ([]Item, error) {
	if !studioPlain(l) {
		return s.studioChildren(ctx, user, l)
	}
	src := l.catalogSource()
	read := catalogWindow(func(start, count int) ([]stremio.Meta, int, error) { return s.window(ctx, v, src, start, count) })
	if !v.narrows() {
		read = s.studioPrefix(ctx, v, src, withinMax(0, CreditsPassMost, l.maxItems), read)
	}
	var items []Item
	for start := 0; start < CreditsPassMost; {
		metas, total, err := read(start, withinMax(start, min(movieIDsPage, CreditsPassMost-start), l.maxItems))
		if cutShort(err, len(metas)) {
			s.logger.Debug("A listing ends at a catalog page the addon could not give", "catalog", src.catalog.ID, "error", err)
		} else if err != nil {
			return items, err
		}
		listed := make([]Item, 0, len(metas))
		records := make([]record, 0, len(metas))
		for _, meta := range metas {
			if item, r, err := s.titleItem(l.addon, l.catalog, meta, l.item.ID, v.country); err == nil {
				listed, records = append(listed, item), append(records, r)
			}
		}
		s.shapeOthers(ctx, v, listed, records)
		result := page(listed, start, totalWithinMax(total, l.maxItems))
		for _, item := range listed {
			if item.Kind == KindMovie || item.Kind == KindSeries {
				items = append(items, item)
			}
		}
		start += len(listed)
		if len(listed) == 0 || !result.More && start >= result.Total {
			break
		}
	}
	return items, nil
}

// studioChildren returns the movies and series of the first
// CreditsPassMost titles of library l, listed by Children, and why the
// listing stopped, with the titles listed before.
func (s *Service) studioChildren(ctx context.Context, user accounts.User, l library) ([]Item, error) {
	var items []Item
	for start := 0; start < CreditsPassMost; {
		page, err := s.Children(ctx, user, l.item.ID, start, min(movieIDsPage, CreditsPassMost-start), "")
		if err != nil {
			return items, err
		}
		for _, item := range page.Items {
			if item.Kind == KindMovie || item.Kind == KindSeries {
				items = append(items, item)
			}
		}
		start += len(page.Items)
		if len(page.Items) == 0 || !page.More && start >= page.Total {
			break
		}
	}
	return items, nil
}

// studioPrefix reads the first n titles of src's catalog at once (see
// studioWindow) and returns the reads of its windows: those within what
// was read, or past the end of a catalog read whole or cut short, answer
// from it, as window would, and the others read answers. A window within
// what was read answers the whole count read: a listing's pages only tell
// whether more follow (see page), which it tells alike. A window answered
// from the studio index holds until the next read.
func (s *Service) studioPrefix(ctx context.Context, v view, src source, n int, read catalogWindow) catalogWindow {
	r := s.studioWindow(ctx, v, src, n)
	length := len(r.metas) + len(r.titles)
	whole := r.err != nil || r.total <= length
	var buffer []stremio.Meta
	titles := func(start, end int) []stremio.Meta {
		if r.titles == nil {
			return r.metas[start:end]
		}
		buffer = buffer[:0]
		for _, title := range r.titles[start:end] {
			buffer = append(buffer, title.meta())
		}
		return buffer
	}
	return func(start, count int) ([]stremio.Meta, int, error) {
		end := start + count
		switch {
		case end <= length:
			return titles(start, end), r.total, nil
		case !whole:
			return read(start, count)
		case start >= length:
			return nil, r.total, r.err
		}
		return titles(start, length), r.total, r.err
	}
}

// studioRead is what studioWindow read of a catalog: its first titles, as
// window gives them, or as the studio index keeps them, how many it
// lists, and why the read stopped short.
type studioRead struct {
	metas  []stremio.Meta
	titles []studioMeta
	total  int
	err    error
}

// studioWindow reads the first n titles of src's catalog, as window does:
// from the studio index for an IPTV source's catalog read from its stored
// lists (see above), else from window.
func (s *Service) studioWindow(ctx context.Context, v view, src source, n int) studioRead {
	// The condition of window's read of stored lists.
	versions, ok := s.iptv.(iptvCatalogVersions)
	if !ok || !src.addon.addon.IPTV() || LiveCatalog(src.catalog.Type) || v.narrows() || len(v.hidden) > 0 || src.date != "" {
		metas, total, err := s.window(ctx, v, src, 0, n)
		return studioRead{metas: metas, total: total, err: err}
	}
	key := studioCatalogKey{source: src.addon.addon.ID, catalogType: src.catalog.Type, catalogID: src.catalog.ID, genre: src.genre, count: n}
	// The version is read first: a change made while the catalog is read
	// then leaves a version the next listing does not find.
	version, err := versions.CatalogVersion(ctx, key.source)
	if err != nil {
		metas, total, err := s.window(ctx, v, src, 0, n)
		return studioRead{metas: metas, total: total, err: err}
	}
	index := s.studioIndex.kept()
	if c, ok := index.Get(key); ok && c.version == version {
		return studioRead{titles: c.titles, total: c.total}
	}
	metas, total, err := s.iptv.CatalogWindow(ctx, key.source, key.catalogType, key.catalogID, key.genre, "", 0, n)
	if err != nil {
		return studioRead{err: err}
	}
	titles := make([]studioMeta, len(metas))
	for i, meta := range metas {
		titles[i] = studioMetaOf(meta)
	}
	index.Put(key, studioCatalog{version: version, titles: titles, total: total})
	return studioRead{metas: metas, total: total}
}
