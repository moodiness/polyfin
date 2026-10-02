package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

const (
	pageTTL     = 10 * time.Minute
	metaTTL     = 6 * time.Hour
	maxCrawl    = 2000 // items fetched from one catalog to answer a request
	pageFetches = 4    // catalog pages of one catalog fetched at once
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
	language func() string

	pages  *cache[pageKey, []stremio.Meta]
	metas  *cache[metaKey, stremio.Meta]
	flight singleflight.Group
}

type pageKey struct {
	addon       accounts.ID
	catalogType string
	catalogID   string
	genre       string
	search      string
	skip        int
}

type metaKey struct {
	addon    accounts.ID
	metaType string
	id       string
}

// New returns a library service. language returns the server language, one
// of accounts.Languages; it is read for every request, so a change applies
// at once.
func New(db *pgxpool.Pool, store *addons.Store, client *stremio.Client, logger *slog.Logger, language func() string) *Service {
	return &Service{
		db:       db,
		addons:   store,
		client:   client,
		logger:   logger,
		now:      time.Now,
		language: language,
		pages:    newCache[pageKey, []stremio.Meta](4000, pageTTL),
		metas:    newCache[metaKey, stremio.Meta](4000, metaTTL),
	}
}

// words returns the generated names in the current server language.
func (s *Service) words() words {
	return vocabularyOf(s.language())
}

// installed is an addon a user can use, with how to reach it.
type installed struct {
	addon    addons.Addon
	confined bool
}

// library is a library of a user with its catalog.
type library struct {
	item    Item
	addon   installed
	catalog stremio.Catalog
}

// view is what a user can browse: their enabled addons and libraries, the
// server's first unless they turned them off.
type view struct {
	addons    []installed
	libraries []library
}

func (s *Service) view(ctx context.Context, user accounts.User) (view, error) {
	scopes := []addons.Scope{addons.Personal(user.ID)}
	if shared, err := s.addons.UsesSharedAddons(ctx, user.ID); err != nil {
		return view{}, err
	} else if shared {
		scopes = append([]addons.Scope{addons.Shared()}, scopes...)
	}
	var v view
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
				entry := installed{addon: addon, confined: confined}
				v.addons = append(v.addons, entry)
				byID[addon.ID] = entry
			}
		}
		libraries, err := s.addons.Libraries(ctx, scope)
		if err != nil {
			return view{}, err
		}
		for _, l := range libraries {
			if entry, active := byID[l.AddonID]; l.Enabled && active {
				visible = append(visible, l)
				entries = append(entries, entry)
			}
		}
	}
	for i, name := range LibraryNames(visible, s.language()) {
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
	key := pageKey{src.addon.addon.ID, src.catalog.Type, src.catalog.ID, src.genre, src.search, skip}
	if metas, ok := s.pages.get(key); ok {
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
		s.pages.put(key, metas)
		return metas, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]stremio.Meta), nil
}

// window returns the catalog's items [start, start+count) and whether more
// follow. A Stremio catalog is read in order: each page is requested with
// skip set to the number of items before it, and an empty page ends it.
// Pages may be shorter than the first when the addon filters them, so the
// pages fetched ahead together, on the guess that they are as long as the
// first, only count while the guess holds; reading then goes on from the
// actual position.
func (s *Service) window(ctx context.Context, src source, start, count int) ([]stremio.Meta, int, error) {
	var collected []stremio.Meta
	seen := map[string]bool{}
	received, size := 0, 0
	more := true
read:
	for len(collected) < start+count {
		if received >= maxCrawl {
			more = false
			break
		}
		offsets := []int{received}
		needed := start + count - len(collected)
		for next := received + size; size > 0 && next < min(received+needed, maxCrawl) && len(offsets) < pageFetches; next += size {
			offsets = append(offsets, next)
		}
		pages, err := s.pagesAt(ctx, src, offsets)
		if err != nil {
			return nil, 0, err
		}
		for i, metas := range pages {
			if offsets[i] != received {
				break
			}
			fresh := 0
			for _, meta := range metas {
				if !seen[meta.ID] {
					seen[meta.ID] = true
					collected = append(collected, meta)
					fresh++
				}
			}
			// An addon that ignores skip sends the same items again.
			if fresh == 0 {
				more = false
				break read
			}
			received += len(metas)
			size = max(size, len(metas))
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

// merged interleaves several catalogs, one item of each in turn, without
// duplicates, and returns the items [start, start+count) and the number of
// items, plus one when more may follow.
func (s *Service) merged(ctx context.Context, sources []source, start, count int) ([]stremio.Meta, int, error) {
	if len(sources) == 1 {
		return s.window(ctx, sources[0], start, count)
	}
	need := start + count
	per := need/len(sources) + 1
	for {
		lists := make([][]stremio.Meta, len(sources))
		mores := make([]bool, len(sources))
		var wg sync.WaitGroup
		for i, src := range sources {
			wg.Go(func() {
				metas, total, err := s.window(ctx, src, 0, per)
				if err != nil {
					s.logger.Warn("A catalog of a collection could not be listed", "catalog", src.catalog.ID, "error", err)
				}
				lists[i], mores[i] = metas, total > len(metas)
			})
		}
		wg.Wait()
		var result []stremio.Meta
		seen := map[string]bool{}
		for rank := 0; ; rank++ {
			added := false
			for _, list := range lists {
				if rank < len(list) {
					added = true
					if !seen[list[rank].ID] {
						seen[list[rank].ID] = true
						result = append(result, list[rank])
					}
				}
			}
			if !added {
				break
			}
		}
		anyMore := slices.Contains(mores, true)
		if len(result) >= need || !anyMore || per >= maxCrawl {
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
		return s.libraryChildren(ctx, l, start, count, genre)
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

func (s *Service) libraryChildren(ctx context.Context, l library, start, count int, genre string) (Page, error) {
	metas, total, err := s.window(ctx, source{addon: l.addon, catalog: l.catalog, genre: genre}, start, count)
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
		metas, titles, err := s.merged(ctx, sources, max(start-len(nested), 0), count-len(items))
		if err != nil {
			return Page{}, err
		}
		total += titles
		for _, meta := range metas {
			catalog := sources[0].catalog
			for _, src := range sources {
				if src.catalog.Type == meta.Type {
					catalog = src.catalog
					break
				}
			}
			item, rec, err := titleItem(addon.addon.ID, catalog, meta, r.ID, addon.confined)
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
	if meta, ok := s.metas.get(key); ok {
		return meta, nil
	}
	result, err, _ := s.flight.Do(fmt.Sprintf("meta %v", key), func() (any, error) {
		meta, err := s.client.Meta(ctx, addon.addon.ManifestURL, metaType, id, addon.confined)
		if err != nil {
			return nil, err
		}
		s.metas.put(key, meta)
		return meta, nil
	})
	if err != nil {
		return stremio.Meta{}, err
	}
	return result.(stremio.Meta), nil
}

// servesMeta reports whether an addon describes titles of this type and ID.
func servesMeta(manifest stremio.Manifest, metaType, id string) bool {
	for _, resource := range manifest.Resources {
		if resource.Name != "meta" {
			continue
		}
		types, prefixes := resource.Types, resource.IDPrefixes
		if len(types) == 0 {
			types = manifest.Types
		}
		if len(prefixes) == 0 {
			prefixes = manifest.IDPrefixes
		}
		if !slices.Contains(types, metaType) {
			continue
		}
		if len(prefixes) == 0 || slices.ContainsFunc(prefixes, func(prefix string) bool { return strings.HasPrefix(id, prefix) }) {
			return true
		}
	}
	return false
}

// titleMeta finds the complete description of a title among the user's
// addons, starting with the one that listed it.
func (s *Service) titleMeta(ctx context.Context, v view, r record) (stremio.Meta, bool) {
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
		if !servesMeta(candidate.addon.Manifest, r.Meta.Type, r.Meta.ID) {
			continue
		}
		meta, err := s.meta(ctx, candidate, r.Meta.Type, r.Meta.ID)
		if err == nil {
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
		if full, ok := s.titleMeta(ctx, v, r); ok {
			meta = full
		}
		item := Item{ID: id, Kind: r.Kind, ParentID: deref(r.Parent), Available: true}
		fromMeta(&item, meta)
		if r.Kind == KindSeries && len(meta.Videos) > 0 {
			item.Contents = contents(meta, nil, s.now())
		}
		// Apps show the credited people and open them by identifier.
		credits := make([]record, 0, len(item.People))
		for _, person := range item.People {
			credits = append(credits, record{ID: person.ID, Key: personKey(person.Name), Kind: KindPerson,
				Person: &Person{Name: person.Name, Image: person.Image}, Confined: r.Confined})
		}
		return item, s.save(ctx, credits)
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
		return Item{ID: id, Kind: KindPerson, Name: r.Person.Name, Images: Images{Primary: r.Person.Image}}, nil
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
	if !ok {
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
			kind, ok := titleKind(catalog.Type)
			if !ok || !slices.Contains(kinds, kind) {
				continue
			}
			searchable := false
			for _, extra := range catalog.Extra {
				if extra.Name == "search" {
					searchable = true
				} else if extra.IsRequired && len(extra.Options) == 0 {
					searchable = false
					break
				}
			}
			if searchable {
				sources = append(sources, source{addon: entry, catalog: catalog, search: term})
			}
		}
	}
	if len(sources) == 0 {
		return nil, nil
	}
	metas, _, err := s.merged(ctx, sources, 0, limit)
	if err != nil {
		return nil, err
	}
	var items []Item
	var records []record
	for _, meta := range metas {
		src := sources[0]
		for _, candidate := range sources {
			if candidate.catalog.Type == meta.Type {
				src = candidate
				break
			}
		}
		item, r, err := titleItem(src.addon.addon.ID, src.catalog, meta, accounts.ID{}, src.addon.confined)
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
	return s.metas.get(metaKey{*r.Addon, r.Meta.Type, r.Meta.ID})
}
