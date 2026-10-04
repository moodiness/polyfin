package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/stremio"
)

const (
	metaTTL     = 6 * time.Hour
	pageFetches = 4 // catalog pages of one catalog fetched at once
	// catalogFetches bounds the catalogs merged reads at once.
	catalogFetches = 8
)

// ErrNotFound reports an item that does not exist or that the user cannot
// reach through their libraries.
var ErrNotFound = errors.New("item not found")

// ErrSeasonNotFound reports a season that is not one of the series' seasons.
var ErrSeasonNotFound = errors.New("season not found")

// Service browses the libraries of users.
type Service struct {
	db       *pgxpool.Pool
	addons   *addons.Store
	client   *stremio.Client
	logger   *slog.Logger
	now      func() time.Time
	settings func() accounts.Settings

	pages  *cache.Cache[pageKey, []stremio.Meta]
	metas  *cache.Cache[metaKey, stremio.Meta]
	flight singleflight.Group

	streamLists   *cache.Cache[streamKey, []stremio.Stream]
	subtitleLists *cache.Cache[streamKey, []stremio.Subtitle]
	versions      *cache.Cache[accounts.ID, Version]
	// ratingLookups holds a place for each rating looked up (see visible);
	// ratingWait bounds how long a request waits for them.
	ratingLookups chan struct{}
	ratingWait    time.Duration
}

type pageKey struct {
	addon       accounts.ID
	catalogType string
	catalogID   string
	genre       string
	search      string
	date        string
	skip        int
}

type metaKey struct {
	addon    accounts.ID
	metaType string
	id       string
}

// New returns a library service. settings returns the server settings, of
// which it uses the language, the catalog limits and how long catalog
// pages and version lists are kept; they are read for every request, so a
// change applies at once.
func New(db *pgxpool.Pool, store *addons.Store, client *stremio.Client, logger *slog.Logger, settings func() accounts.Settings) *Service {
	s := &Service{
		db:            db,
		addons:        store,
		client:        client,
		logger:        logger,
		now:           time.Now,
		settings:      settings,
		metas:         cache.New[metaKey, stremio.Meta](4000, metaTTL),
		versions:      cache.New[accounts.ID, Version](20000, versionsTTL),
		ratingLookups: make(chan struct{}, ratingFetches),
		ratingWait:    ratingWait,
	}
	clock := func() time.Time { return s.now() }
	s.pages = cache.NewLasting[pageKey, []stremio.Meta](4000, s.catalogLife, clock)
	s.streamLists = cache.NewLasting[streamKey, []stremio.Stream](2000, s.listLife, clock)
	s.subtitleLists = cache.NewLasting[streamKey, []stremio.Subtitle](2000, s.listLife, clock)
	return s
}

// catalogLife is how long catalog pages are kept, and listLife how long a
// title's version and subtitle lists from the addons are: the settings'
// CatalogRefreshMinutes and VersionListMinutes.
func (s *Service) catalogLife() time.Duration {
	return time.Duration(s.settings().CatalogRefreshMinutes) * time.Minute
}

func (s *Service) listLife() time.Duration {
	return time.Duration(s.settings().VersionListMinutes) * time.Minute
}

// words returns the generated names in the current server language.
func (s *Service) words() words {
	return vocabularyOf(s.settings().Language)
}

// installed is an addon a user can use, with how to reach it. shared is
// true for the server's addons, installed by an administrator.
type installed struct {
	addon    addons.Addon
	confined bool
	shared   bool
}

// library is a library of a user with its catalog.
type library struct {
	item    Item
	addon   installed
	catalog stremio.Catalog
}

// view is what a user can browse: their enabled addons and libraries, the
// server's first unless they turned them off. Enabled live TV catalogs are
// not libraries: they list the user's channels, in the same order.
type view struct {
	addons    []installed
	libraries []library
	channels  []source
	// parental is the user's parental control, which hides titles (see
	// visible). The ratings of the titles a request lists are looked up
	// until deadline, lookups counting how many it started; held is set
	// once a listing of the request stopped short for parental control.
	parental accounts.ParentalControl
	deadline time.Time
	lookups  *atomic.Int32
	held     *atomic.Bool
	// catalogLimit and channelLimit are the settings' limits when the
	// request came (see limit).
	catalogLimit int
	channelLimit int
}

// limit is how many items one read of src's catalog fetches at most: the
// channel limit for a live TV catalog, the catalog limit for any other.
// Some catalogs are nearly endless.
func (v view) limit(src source) int {
	if LiveCatalog(src.catalog.Type) {
		return v.channelLimit
	}
	return v.catalogLimit
}

func (s *Service) view(ctx context.Context, user accounts.User) (view, error) {
	settings := s.settings()
	// A user under parental control browses the server's addons only: their
	// own addons could describe titles without the ratings that hide them.
	// So does a user whose own addons the server or their own permission
	// turned off: their addons are kept, but not used.
	scopes := []addons.Scope{addons.Shared()}
	if !user.Parental.Restricted() && settings.PersonalAddonsAllowed(user) {
		scopes = []addons.Scope{addons.Personal(user.ID)}
		if shared, err := s.addons.UsesSharedAddons(ctx, user.ID); err != nil {
			return view{}, err
		} else if shared {
			scopes = append([]addons.Scope{addons.Shared()}, scopes...)
		}
	}
	v := view{parental: user.Parental, deadline: time.Now().Add(s.ratingWait), lookups: new(atomic.Int32), held: new(atomic.Bool),
		catalogLimit: settings.CatalogLimit, channelLimit: settings.ChannelLimit}
	var visible []addons.Library
	var entries []installed
	for _, scope := range scopes {
		// The server's addons were installed by an administrator; a user's own
		// addons may only reach the local network if that user is one.
		confined := scope.Owner != nil && !user.IsAdministrator
		list, err := s.addons.Addons(ctx, scope)
		if err != nil {
			return view{}, err
		}
		byID := map[accounts.ID]installed{}
		for _, addon := range list {
			if addon.Enabled {
				entry := installed{addon: addon, confined: confined, shared: scope.Owner == nil}
				v.addons = append(v.addons, entry)
				byID[addon.ID] = entry
			}
		}
		libraries, err := s.addons.Libraries(ctx, scope)
		if err != nil {
			return view{}, err
		}
		for _, l := range libraries {
			entry, active := byID[l.AddonID]
			switch {
			case !l.Enabled || !active:
			case LiveCatalog(l.Catalog.Type):
				v.channels = append(v.channels, source{addon: entry, catalog: l.Catalog})
			default:
				visible = append(visible, l)
				entries = append(entries, entry)
			}
		}
	}
	for i, name := range LibraryNames(visible, settings.Language) {
		l := visible[i]
		v.libraries = append(v.libraries, library{
			item: Item{
				ID:             itemID(libraryKey(l.AddonID, l.Catalog.Type, l.Catalog.ID)),
				Kind:           KindLibrary,
				Name:           name,
				CollectionType: collectionType(l.Catalog.Type),
			},
			addon:   entries[i],
			catalog: l.Catalog,
		})
	}
	return v, nil
}

func (v view) addon(id accounts.ID) (installed, bool) {
	for _, entry := range v.addons {
		if entry.addon.ID == id {
			return entry, true
		}
	}
	return installed{}, false
}

func (v view) library(id accounts.ID) (library, bool) {
	for _, l := range v.libraries {
		if l.item.ID == id {
			return l, true
		}
	}
	return library{}, false
}

// Libraries lists a user's libraries in order.
func (s *Service) Libraries(ctx context.Context, user accounts.User) ([]Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(v.libraries))
	records := make([]record, 0, len(v.libraries))
	for _, l := range v.libraries {
		items = append(items, l.item)
		addon := l.addon.addon.ID
		records = append(records, record{ID: l.item.ID, Key: libraryKey(addon, l.catalog.Type, l.catalog.ID), Kind: KindLibrary,
			Addon: &addon, CatalogType: l.catalog.Type, CatalogID: l.catalog.ID, Confined: l.addon.confined})
	}
	return items, s.save(ctx, records)
}

// source is a catalog to list, possibly narrowed to a genre.
type source struct {
	addon   installed
	catalog stremio.Catalog
	genre   string
	search  string
	// date asks a guide catalog for the programmes of a UTC day,
	// YYYY-MM-DD.
	date string
}

// paged reports whether the catalog can be read past its first page.
func (src source) paged() bool {
	return slices.ContainsFunc(src.catalog.Extra, func(extra stremio.Extra) bool { return extra.Name == "skip" })
}

func (src source) extras(skip int) ([]stremio.ExtraValue, bool) {
	var values []stremio.ExtraValue
	for _, extra := range src.catalog.Extra {
		switch {
		case extra.Name == "search" && src.search != "":
			values = append(values, stremio.ExtraValue{Name: "search", Value: src.search})
		case extra.Name == "genre" && src.genre != "":
			values = append(values, stremio.ExtraValue{Name: "genre", Value: src.genre})
		case extra.Name == "date" && src.date != "":
			values = append(values, stremio.ExtraValue{Name: "date", Value: src.date})
		case extra.Name != "skip" && extra.IsRequired && len(extra.Options) > 0:
			values = append(values, stremio.ExtraValue{Name: extra.Name, Value: extra.Options[0]})
		}
	}
	if skip > 0 {
		if !src.paged() {
			return nil, false
		}
		values = append(values, stremio.ExtraValue{Name: "skip", Value: strconv.Itoa(skip)})
	}
	return values, true
}

// page fetches the catalog page starting at skip, sharing concurrent and
// recent requests.
func (s *Service) page(ctx context.Context, src source, skip int) ([]stremio.Meta, error) {
	key := pageKey{src.addon.addon.ID, src.catalog.Type, src.catalog.ID, src.genre, src.search, src.date, skip}
	if metas, ok := s.pages.Get(key); ok {
		return metas, nil
	}
	extra, ok := src.extras(skip)
	if !ok {
		return nil, nil
	}
	result, err, _ := s.flight.Do(fmt.Sprintf("page %v", key), func() (any, error) {
		metas, err := s.client.Catalog(ctx, src.addon.addon.ManifestURL, src.catalog.Type, src.catalog.ID, extra, src.addon.confined)
		if err != nil {
			return nil, err
		}
		s.pages.Put(key, metas)
		return metas, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]stremio.Meta), nil
}

// window returns the catalog's items [start, start+count) and whether more
// follow, reading no further than the catalog's limit (see view.limit). A
// Stremio catalog is read in order: each page is requested with
// skip set to the number of items before it, and an empty page ends it.
// Pages may be shorter than the first when the addon filters them, so the
// pages fetched ahead together, on the guess that they are as long as the
// first, only count while the guess holds; reading then goes on from the
// actual position. Titles the user's parental control hides are left out
// before positions are counted. A restricted listing reads at most
// hiddenReach times as far as an unrestricted one, and stops where titles
// wait for their rating: it reports that more may follow, which apps ask
// for later, once the ratings are known.
func (s *Service) window(ctx context.Context, v view, src source, start, count int) ([]stremio.Meta, int, error) {
	var collected []stremio.Meta
	seen := map[string]bool{}
	received, size := 0, 0
	more := true
	limit := v.limit(src)
	reach := limit
	if v.parental.Restricted() {
		reach = min(limit, hiddenReach*(start+count))
	}
	for len(collected) < start+count {
		if received >= reach {
			more = received < limit
			if more {
				v.held.Store(true)
			}
			break
		}
		offsets := []int{received}
		needed := start + count - len(collected)
		for next := received + size; size > 0 && next < min(received+needed, limit) && len(offsets) < pageFetches; next += size {
			offsets = append(offsets, next)
		}
		pages, err := s.pagesAt(ctx, src, offsets)
		if err != nil {
			return nil, 0, err
		}
		var fresh []stremio.Meta
		repeated := false
		for i, metas := range pages {
			if offsets[i] != received {
				break
			}
			before := len(fresh)
			for _, meta := range metas {
				if !seen[meta.ID] {
					seen[meta.ID] = true
					fresh = append(fresh, meta)
				}
			}
			// An addon that ignores skip sends the same items again.
			if len(fresh) == before {
				repeated = true
				break
			}
			received += len(metas)
			size = max(size, len(metas))
		}
		visible, held := s.visibleMetas(ctx, v, src, fresh)
		collected = append(collected, visible...)
		if held {
			v.held.Store(true)
			break
		}
		if repeated {
			more = false
			break
		}
		if !src.paged() {
			more = false
			break
		}
	}
	total := len(collected)
	if more {
		total++
	}
	if start >= len(collected) {
		return nil, total, nil
	}
	return collected[start:min(start+count, len(collected))], total, nil
}

// pagesAt fetches the catalog pages starting at each offset together.
func (s *Service) pagesAt(ctx context.Context, src source, offsets []int) ([][]stremio.Meta, error) {
	pages := make([][]stremio.Meta, len(offsets))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(pageFetches)
	for i, offset := range offsets {
		group.Go(func() (err error) {
			pages[i], err = s.page(groupCtx, src, offset)
			return err
		})
	}
	return pages, group.Wait()
}

// listed is a title of a catalog, with the catalog that listed it.
type listed struct {
	meta stremio.Meta
	src  source
}

// title describes a listed title as an item under parent, with the record
// that finds it again through the catalog that listed it.
func (l listed) title(parent accounts.ID) (Item, record, error) {
	return titleItem(l.src.addon.addon.ID, l.src.catalog, l.meta, parent, l.src.addon.confined)
}

// merged interleaves several catalogs, one item of each in turn, without
// duplicates, and returns the items [start, start+count), each with the
// catalog that listed it first, and the number of items, plus one when
// more may follow. Titles the user's parental control hides are left out
// (see window).
func (s *Service) merged(ctx context.Context, v view, sources []source, start, count int) ([]listed, int, error) {
	if len(sources) == 1 {
		metas, total, err := s.window(ctx, v, sources[0], start, count)
		result := make([]listed, 0, len(metas))
		for _, meta := range metas {
			result = append(result, listed{meta, sources[0]})
		}
		return result, total, err
	}
	need := start + count
	per := need/len(sources) + 1
	limit := 0
	for _, src := range sources {
		limit = max(limit, v.limit(src))
	}
	for {
		lists := make([][]stremio.Meta, len(sources))
		mores := make([]bool, len(sources))
		// A genre's titles may come from dozens of catalogs of one addon:
		// they are read a few at a time.
		var group errgroup.Group
		group.SetLimit(catalogFetches)
		for i, src := range sources {
			group.Go(func() error {
				metas, total, err := s.window(ctx, v, src, 0, per)
				// An app that stops waiting cancels ctx: nothing failed.
				if err != nil && ctx.Err() == nil {
					s.logger.Warn("A catalog of a collection could not be listed", "catalog", src.catalog.ID, "error", err)
				}
				lists[i], mores[i] = metas, total > len(metas)
				return nil
			})
		}
		_ = group.Wait()
		var result []listed
		seen := map[string]bool{}
		for rank := 0; ; rank++ {
			added := false
			for i, list := range lists {
				if rank < len(list) {
					added = true
					if !seen[list[rank].ID] {
						seen[list[rank].ID] = true
						result = append(result, listed{list[rank], sources[i]})
					}
				}
			}
			if !added {
				break
			}
		}
		anyMore := slices.Contains(mores, true)
		// Reading further would not show the titles a restricted listing
		// stopped at (see window).
		if len(result) >= need || !anyMore || per >= limit || v.held.Load() {
			total := len(result)
			if anyMore {
				total++
			}
			if start >= len(result) {
				return nil, total, nil
			}
			return result[start:min(need, len(result))], total, nil
		}
		per *= 2
	}
}

// Page is a window of a folder's children.
type Page struct {
	Items []Item
	// Total is the number of children, or a lower bound when More is true.
	Total int
	More  bool
}

// Children lists a folder's children: a library's titles or collections, a
// collection's titles, a series' seasons or a season's episodes. genre
// narrows a library to one of the genres its catalog offers (see Genres).
func (s *Service) Children(ctx context.Context, user accounts.User, parent accounts.ID, start, count int, genre string) (Page, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return Page{}, err
	}
	if l, ok := v.library(parent); ok {
		return s.libraryChildren(ctx, v, l, start, count, genre)
	}
	r, err := s.load(ctx, parent)
	if err != nil {
		return Page{}, err
	}
	switch r.Kind {
	case KindCollection:
		return s.collectionChildren(ctx, v, r, start, count)
	case KindSeries:
		seasons, err := s.Seasons(ctx, user, parent)
		return slicePage(seasons, start, count), err
	case KindSeason:
		episodes, err := s.Episodes(ctx, user, r.seriesItemID(), &parent)
		return slicePage(episodes, start, count), err
	default:
		return Page{}, ErrNotFound
	}
}

func slicePage(items []Item, start, count int) Page {
	total := len(items)
	if start >= total {
		return Page{Total: total}
	}
	return Page{Items: items[start:min(start+count, total)], Total: total}
}

func (r record) seriesItemID() accounts.ID { return itemID(titleKey(KindSeries, r.SeriesID)) }

func (s *Service) libraryChildren(ctx context.Context, v view, l library, start, count int, genre string) (Page, error) {
	metas, total, err := s.window(ctx, v, source{addon: l.addon, catalog: l.catalog, genre: genre}, start, count)
	if err != nil {
		return Page{}, err
	}
	var items []Item
	var records []record
	addon := l.addon.addon.ID
	for _, meta := range metas {
		var item Item
		var r record
		if l.catalog.Type == "collection" {
			item, r = collectionItem(addon, meta, l.item.ID, l.addon.confined)
		} else if item, r, err = titleItem(addon, l.catalog, meta, l.item.ID, l.addon.confined); err != nil {
			continue
		}
		items, records = append(items, item), append(records, r)
	}
	return page(items, start, total), s.save(ctx, records)
}

// page makes a Page of a window of children, out of total.
func page(items []Item, start, total int) Page {
	return Page{Items: items, Total: total, More: start+len(items) < total}
}

func collectionItem(addon accounts.ID, meta stremio.Meta, parent accounts.ID, confined bool) (Item, record) {
	key := collectionKey(addon, meta.ID)
	item := Item{ID: itemID(key), Kind: KindCollection, ParentID: parent}
	fromMeta(&item, meta)
	preview := meta
	return item, record{ID: item.ID, Key: key, Kind: KindCollection, Addon: &addon, Parent: &parent, Meta: &preview, Confined: confined}
}

var errUnsupported = errors.New("unsupported title type")

func titleItem(addon accounts.ID, catalog stremio.Catalog, meta stremio.Meta, parent accounts.ID, confined bool) (Item, record, error) {
	metaType := meta.Type
	if metaType == "" {
		metaType = catalog.Type
	}
	kind, ok := titleKind(metaType)
	if !ok || meta.ID == "" {
		return Item{}, record{}, errUnsupported
	}
	meta.Type = metaType
	key := titleKey(kind, meta.ID)
	item := Item{ID: itemID(key), Kind: kind, ParentID: parent, Available: true}
	fromMeta(&item, meta)
	preview := meta
	return item, record{ID: item.ID, Key: key, Kind: kind, Addon: &addon, CatalogType: catalog.Type, CatalogID: catalog.ID,
		Parent: &parent, Meta: &preview, Confined: confined}, nil
}

func (s *Service) collectionChildren(ctx context.Context, v view, r record, start, count int) (Page, error) {
	if r.Addon == nil || r.Meta == nil {
		return Page{}, ErrNotFound
	}
	addon, ok := v.addon(*r.Addon)
	if !ok {
		return Page{}, ErrNotFound
	}
	meta, err := s.meta(ctx, addon, "collection", r.Meta.ID)
	if err != nil {
		return Page{}, err
	}
	// A collection groups either other collections or catalogs; should it
	// have both, its collections come first.
	var nested []stremio.Meta
	var sources []source
	if meta.Collection != nil {
		for _, sub := range meta.Collection.Items {
			if sub.ID != "" {
				sub.Type = "collection"
				nested = append(nested, sub)
			}
		}
		for _, ref := range meta.Collection.Sources {
			if catalog, ok := addon.addon.Manifest.Catalog(ref.Type, ref.CatalogID); ok {
				sources = append(sources, source{addon: addon, catalog: catalog, genre: ref.Genre})
			}
		}
	}
	var items []Item
	var records []record
	for _, sub := range nested[min(start, len(nested)):min(start+count, len(nested))] {
		item, rec := collectionItem(addon.addon.ID, sub, r.ID, addon.confined)
		items, records = append(items, item), append(records, rec)
	}
	total := len(nested)
	if len(sources) > 0 {
		titles, listedTotal, err := s.merged(ctx, v, sources, max(start-len(nested), 0), count-len(items))
		if err != nil {
			return Page{}, err
		}
		total += listedTotal
		for _, title := range titles {
			item, rec, err := title.title(r.ID)
			if err != nil {
				continue
			}
			items, records = append(items, item), append(records, rec)
		}
	}
	return page(items, start, total), s.save(ctx, records)
}

// meta returns an addon's complete description of a title, cached.
func (s *Service) meta(ctx context.Context, addon installed, metaType, id string) (stremio.Meta, error) {
	key := metaKey{addon.addon.ID, metaType, id}
	if meta, ok := s.metas.Get(key); ok {
		return meta, nil
	}
	result, err, _ := s.flight.Do(fmt.Sprintf("meta %v", key), func() (any, error) {
		meta, err := s.client.Meta(ctx, addon.addon.ManifestURL, metaType, id, addon.confined)
		if err != nil {
			return nil, err
		}
		s.metas.Put(key, meta)
		return meta, nil
	})
	if err != nil {
		return stremio.Meta{}, err
	}
	return result.(stremio.Meta), nil
}

// titleMeta finds the complete description of a title among the user's
// addons, starting with the one that listed it (see describe).
func (s *Service) titleMeta(ctx context.Context, v view, r record) (stremio.Meta, bool) {
	return s.describe(ctx, v, r, false)
}

// describe finds the complete description of a title among the user's
// addons, the server's only when shared is set, starting with the one that
// listed it. A description from one of the server's addons gives the
// rating kept for the title.
func (s *Service) describe(ctx context.Context, v view, r record, shared bool) (stremio.Meta, bool) {
	if r.Meta == nil {
		return stremio.Meta{}, false
	}
	candidates := slices.Clone(v.addons)
	if r.Addon != nil {
		slices.SortStableFunc(candidates, func(a, b installed) int {
			switch {
			case a.addon.ID == *r.Addon:
				return -1
			case b.addon.ID == *r.Addon:
				return 1
			}
			return 0
		})
	}
	for _, candidate := range candidates {
		if shared && !candidate.shared || !candidate.addon.Manifest.Serves("meta", r.Meta.Type, r.Meta.ID) {
			continue
		}
		meta, err := s.meta(ctx, candidate, r.Meta.Type, r.Meta.ID)
		if err == nil {
			if candidate.shared {
				s.learnRating(ctx, r, meta)
			}
			return meta, true
		}
		s.logger.Debug("An addon could not describe a title", "addon", candidate.addon.Manifest.Name, "error", err)
	}
	return stremio.Meta{}, false
}

// Item describes one item. Titles are completed with their addon's full
// metadata when available.
func (s *Service) Item(ctx context.Context, user accounts.User, id accounts.ID) (Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return Item{}, err
	}
	return s.item(ctx, v, id)
}

// Items describes the items of ids the user can reach, in order. Items
// that no longer exist, or whose addon fails to describe them, are left
// out.
func (s *Service) Items(ctx context.Context, user accounts.User, ids []accounts.ID) ([]Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	found := make([]*Item, len(ids))
	var g errgroup.Group
	g.SetLimit(8)
	for i, id := range ids {
		g.Go(func() error {
			item, err := s.item(ctx, v, id)
			if err != nil {
				if !errors.Is(err, ErrNotFound) {
					s.logger.Debug("An item could not be described", "item", id, "error", err)
				}
				return nil
			}
			found[i] = &item
			return nil
		})
	}
	_ = g.Wait()
	items := make([]Item, 0, len(ids))
	for _, item := range found {
		if item != nil {
			items = append(items, *item)
		}
	}
	return items, nil
}

func (s *Service) item(ctx context.Context, v view, id accounts.ID) (Item, error) {
	if l, ok := v.library(id); ok {
		return l.item, nil
	}
	r, err := s.load(ctx, id)
	if err != nil {
		return Item{}, err
	}
	switch r.Kind {
	case KindCollection:
		if r.Meta == nil || r.Addon == nil {
			return Item{}, ErrNotFound
		}
		if _, ok := v.addon(*r.Addon); !ok {
			return Item{}, ErrNotFound
		}
		item := Item{ID: id, Kind: KindCollection, ParentID: deref(r.Parent)}
		fromMeta(&item, *r.Meta)
		return item, nil
	case KindMovie, KindSeries:
		meta := *r.Meta
		full, described := s.titleMeta(ctx, v, r)
		if described {
			meta = full
		}
		// A restricted user's addons are the server's (see view): the
		// description gives the rating.
		rating, known := certification(meta), described
		if !described {
			rating, known, _ = s.knownRating(v, r)
		}
		// Jellyfin answers a title the user may not see as one that does not
		// exist.
		if !v.allows(r.Kind, rating, known) {
			return Item{}, ErrNotFound
		}
		item := Item{ID: id, Kind: r.Kind, ParentID: deref(r.Parent), Available: true}
		fromMeta(&item, meta)
		item.OfficialRating = rating
		if r.Kind == KindSeries && len(meta.Videos) > 0 {
			item.Contents = contents(meta, nil, s.now())
		}
		// Apps show the credited people and open them by identifier.
		return item, s.saveCredits(ctx, id, item.People, r.Confined)
	case KindSeason, KindEpisode:
		series, meta, err := s.series(ctx, v, r.seriesItemID())
		if err != nil {
			return Item{}, err
		}
		if r.Kind == KindSeason {
			for _, season := range seasons(series, meta, s.words(), s.now()) {
				if season.ID == id {
					return season, nil
				}
			}
		} else {
			for _, episode := range episodes(series, meta, nil, s.words(), s.now()) {
				if episode.ID == id {
					return episode, nil
				}
			}
		}
		return Item{}, ErrNotFound
	case KindPerson:
		if _, err := s.credited(ctx, v, r); err != nil {
			return Item{}, err
		}
		return Item{ID: id, Kind: KindPerson, Name: r.Person.Name, Images: Images{Primary: r.Person.Image}}, nil
	case KindChannel:
		return s.channel(v, r)
	case KindProgram:
		return s.program(ctx, v, r)
	default:
		return Item{}, ErrNotFound
	}
}

func deref(id *accounts.ID) accounts.ID {
	if id == nil {
		return accounts.ID{}
	}
	return *id
}

// series returns a series item and its complete metadata, with videos.
func (s *Service) series(ctx context.Context, v view, id accounts.ID) (Item, stremio.Meta, error) {
	r, err := s.load(ctx, id)
	if err != nil || r.Kind != KindSeries {
		return Item{}, stremio.Meta{}, ErrNotFound
	}
	meta, ok := s.titleMeta(ctx, v, r)
	if !ok || !v.allows(KindSeries, certification(meta), true) {
		return Item{}, stremio.Meta{}, ErrNotFound
	}
	item := Item{ID: id, Kind: KindSeries, ParentID: deref(r.Parent), Available: true}
	fromMeta(&item, meta)
	item.Contents = contents(meta, nil, s.now())
	return item, meta, nil
}

func seasonNumbers(meta stremio.Meta) []int {
	var numbers []int
	for _, video := range meta.Videos {
		if n := int(video.Season); !slices.Contains(numbers, n) {
			numbers = append(numbers, n)
		}
	}
	slices.Sort(numbers)
	return numbers
}

// seasonPoster returns a season's own artwork, if the addon has one.
func seasonPoster(meta stremio.Meta, number int) string {
	if meta.Extras == nil {
		return ""
	}
	return meta.Extras.SeasonPosterByNumber[strconv.Itoa(number)]
}

// contents describes the episodes of a series, or of one season when
// season is set.
func contents(meta stremio.Meta, season *int, now time.Time) *Contents {
	c := &Contents{}
	if season == nil {
		c.Children = len(seasonNumbers(meta))
	}
	for _, video := range meta.Videos {
		if season != nil {
			if int(video.Season) != *season {
				continue
			}
			c.Children++
		}
		if !released(video, now) {
			continue
		}
		c.Released++
		c.Runtime += parseRuntime(string(video.Runtime))
		if date := parseDate(video.Released); date != nil && (c.LastReleased == nil || date.After(*c.LastReleased)) {
			c.LastReleased = date
		}
	}
	return c
}

func seasons(series Item, meta stremio.Meta, w words, now time.Time) []Item {
	var result []Item
	for _, number := range seasonNumbers(meta) {
		item := Item{
			ID:           itemID(seasonKey(meta.ID, number)),
			Kind:         KindSeason,
			Name:         w.seasonName(number),
			ParentID:     series.ID,
			SeriesID:     series.ID,
			SeriesName:   series.Name,
			SeriesPoster: series.Images.Primary,
			IndexNumber:  number,
			Available:    true,
			Images:       Images{Primary: series.Images.Primary, Backdrop: series.Images.Backdrop},
			Contents:     contents(meta, &number, now),
			StremioType:  meta.Type,
			StremioID:    meta.ID,
		}
		if poster := seasonPoster(meta, number); poster != "" {
			item.Images.Primary = poster
		}
		for _, video := range meta.Videos {
			if int(video.Season) != number {
				continue
			}
			if date := parseDate(video.Released); date != nil && (item.PremiereDate == nil || date.Before(*item.PremiereDate)) {
				item.PremiereDate = date
				item.ProductionYear = date.Year()
			}
		}
		result = append(result, item)
	}
	return result
}

func episodes(series Item, meta stremio.Meta, season *int, w words, now time.Time) []Item {
	var result []Item
	for _, video := range meta.Videos {
		number := int(video.Season)
		if season != nil && number != *season {
			continue
		}
		item := Item{
			ID:                itemID(episodeKey(video.ID)),
			Kind:              KindEpisode,
			Name:              video.DisplayName(),
			ParentID:          itemID(seasonKey(meta.ID, number)),
			Overview:          video.Overview,
			PremiereDate:      parseDate(video.Released),
			Runtime:           parseRuntime(string(video.Runtime)),
			Images:            Images{Primary: video.Thumbnail, Backdrop: series.Images.Backdrop},
			SeriesID:          series.ID,
			SeriesName:        series.Name,
			SeriesPoster:      series.Images.Primary,
			SeasonID:          itemID(seasonKey(meta.ID, number)),
			SeasonName:        w.seasonName(number),
			SeasonPoster:      seasonPoster(meta, number),
			IndexNumber:       video.EpisodeNumber(),
			ParentIndexNumber: number,
			Available:         released(video, now),
			StremioType:       meta.Type,
			StremioID:         video.ID,
		}
		if item.Name == "" {
			item.Name = w.episodeName(item.IndexNumber)
		}
		if item.PremiereDate != nil {
			item.ProductionYear = item.PremiereDate.Year()
		}
		result = append(result, item)
	}
	slices.SortStableFunc(result, func(a, b Item) int {
		if a.ParentIndexNumber != b.ParentIndexNumber {
			return a.ParentIndexNumber - b.ParentIndexNumber
		}
		return a.IndexNumber - b.IndexNumber
	})
	return result
}

// Seasons lists a series' seasons.
func (s *Service) Seasons(ctx context.Context, user accounts.User, seriesID accounts.ID) ([]Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	series, meta, err := s.series(ctx, v, seriesID)
	if err != nil {
		return nil, err
	}
	result := seasons(series, meta, s.words(), s.now())
	records := make([]record, 0, len(result))
	for _, season := range result {
		records = append(records, record{ID: season.ID, Key: seasonKey(meta.ID, season.IndexNumber), Kind: KindSeason,
			Parent: &series.ID, SeriesID: meta.ID, Season: season.IndexNumber})
	}
	return result, s.save(ctx, records)
}

// Episodes lists a series' episodes, of one season when seasonID is set.
func (s *Service) Episodes(ctx context.Context, user accounts.User, seriesID accounts.ID, seasonID *accounts.ID) ([]Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	series, meta, err := s.series(ctx, v, seriesID)
	if err != nil {
		return nil, err
	}
	var season *int
	if seasonID != nil {
		for _, number := range seasonNumbers(meta) {
			if itemID(seasonKey(meta.ID, number)) == *seasonID {
				season = &number
				break
			}
		}
		if season == nil {
			return nil, ErrSeasonNotFound
		}
	}
	result := episodes(series, meta, season, s.words(), s.now())
	records := make([]record, 0, len(result)+len(seasonNumbers(meta)))
	for _, number := range seasonNumbers(meta) {
		records = append(records, record{ID: itemID(seasonKey(meta.ID, number)), Key: seasonKey(meta.ID, number), Kind: KindSeason,
			Parent: &series.ID, SeriesID: meta.ID, Season: number})
	}
	videos := make(map[string]stremio.Video, len(meta.Videos))
	for _, video := range meta.Videos {
		videos[video.ID] = video
	}
	for i, episode := range result {
		video := videos[episode.StremioID]
		records = append(records, record{ID: episode.ID, Key: episodeKey(video.ID), Kind: KindEpisode,
			Parent: &result[i].SeasonID, SeriesID: meta.ID, Season: episode.ParentIndexNumber, Video: &video})
	}
	return result, s.save(ctx, records)
}

// Search looks a term up in the search catalogs of the user's addons, for
// titles of the given kinds.
func (s *Service) Search(ctx context.Context, user accounts.User, term string, kinds []Kind, limit int) ([]Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	var sources []source
	for _, entry := range v.addons {
		for _, catalog := range entry.addon.Manifest.Catalogs {
			if kind, ok := titleKind(catalog.Type); ok && slices.Contains(kinds, kind) && searchable(catalog) {
				sources = append(sources, source{addon: entry, catalog: catalog, search: term})
			}
		}
	}
	if len(sources) == 0 {
		return nil, nil
	}
	titles, _, err := s.merged(ctx, v, sources, 0, limit)
	if err != nil {
		return nil, err
	}
	var items []Item
	var records []record
	for _, title := range titles {
		item, r, err := title.title(accounts.ID{})
		if err != nil {
			continue
		}
		r.Parent = nil
		items, records = append(items, item), append(records, r)
	}
	return items, s.save(ctx, records)
}

// Ancestors lists the folders above an item, nearest first: a season and
// series for an episode, then the library or collection the title was listed
// in.
func (s *Service) Ancestors(ctx context.Context, user accounts.User, id accounts.ID) ([]Item, error) {
	item, err := s.Item(ctx, user, id)
	if err != nil {
		return nil, err
	}
	var result []Item
	for parent := item.ParentID; parent != (accounts.ID{}) && len(result) < 6; {
		folder, err := s.Item(ctx, user, parent)
		if err != nil {
			// A folder the user can no longer reach ends the chain.
			break
		}
		result = append(result, folder)
		parent = folder.ParentID
	}
	return result, nil
}

// Genres lists the genres a library's catalog can be narrowed to.
func (s *Service) Genres(ctx context.Context, user accounts.User, libraryID accounts.ID) ([]string, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	l, ok := v.library(libraryID)
	if !ok {
		return nil, ErrNotFound
	}
	for _, extra := range l.catalog.Extra {
		if extra.Name == "genre" {
			return slices.Clone(extra.Options), nil
		}
	}
	return nil, nil
}

// Artwork returns the URL of an item's image and whether downloading it is
// confined to public addresses. It needs no user: Jellyfin apps load images
// without credentials, and artwork URLs carry no secret.
func (s *Service) Artwork(ctx context.Context, id accounts.ID, imageType string) (string, bool, error) {
	r, err := s.load(ctx, id)
	if err != nil {
		return "", false, err
	}
	var images Images
	confined := r.Confined
	switch {
	case r.Kind == KindChannel && r.Meta != nil:
		images = channelImages(*r.Meta)
	case r.Kind == KindProgram && r.Video != nil:
		// The guide that listed the programme may come from another addon
		// than its channel: either one confines its artwork.
		images.Primary = r.Video.Thumbnail
		channel, err := s.load(ctx, itemID(channelKey(r.Channel)))
		confined = r.Confined || err != nil || channel.Confined
	case r.Meta != nil:
		var item Item
		fromMeta(&item, *r.Meta)
		images = item.Images
		if full, ok := s.cachedMeta(r); ok && images.URL(imageType) == "" {
			fromMeta(&item, full)
			images = item.Images
		}
	case r.Kind == KindEpisode && r.Video != nil:
		images.Primary = r.Video.Thumbnail
		if series, err := s.load(ctx, r.seriesItemID()); err == nil {
			confined = series.Confined
		}
	case r.Kind == KindPerson && r.Person != nil:
		images.Primary = r.Person.Image
	case r.Kind == KindSeason:
		series, err := s.load(ctx, r.seriesItemID())
		if err != nil || series.Meta == nil {
			return "", false, ErrNotFound
		}
		images = Images{Primary: series.Meta.Poster, Backdrop: series.Meta.Background}
		if full, ok := s.cachedMeta(series); ok && full.Extras != nil {
			if poster := full.Extras.SeasonPosterByNumber[strconv.Itoa(r.Season)]; poster != "" {
				images.Primary = poster
			}
		}
		confined = series.Confined
	}
	url := images.URL(imageType)
	if url == "" {
		return "", false, ErrNotFound
	}
	return url, confined, nil
}

// cachedMeta returns the complete metadata of a title if its addon already
// sent it.
func (s *Service) cachedMeta(r record) (stremio.Meta, bool) {
	if r.Addon == nil || r.Meta == nil {
		return stremio.Meta{}, false
	}
	return s.metas.Get(metaKey{*r.Addon, r.Meta.Type, r.Meta.ID})
}
