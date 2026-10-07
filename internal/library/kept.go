package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/stremio"
)

// What addons answered for browsing.
//
// Catalog pages, titles' descriptions and libraries' automatic images are
// kept with the time the addon gave them. Past their refresh age (the
// settings' CatalogRefreshMinutes for pages and images, metaTTL for
// descriptions), they are still shown, at once, while the addon is asked
// again in the background; an addon that fails then leaves them as they
// were. They are forgotten a while after their refresh age: staleCatalogs
// for pages and images, metaKept for descriptions.
//
// Pages and descriptions are also kept in the database (catalog_pages and
// metas), read when memory lacks them, so that a home screen, a library or
// a title page answers at once after a restart. Searches and guide pages
// stay in memory.
//
// A fetch is shared by every request that needs it at once, and runs on a
// context of its own: a request that gives up stops waiting, not the
// fetch, whose answer is kept for the next request.

const (
	// staleCatalogs is how long catalog pages and automatic images are kept
	// past their refresh age.
	staleCatalogs = 24 * time.Hour
	// metaKept is how long a description is kept, stale ones included.
	metaKept = 7 * 24 * time.Hour
)

// fetched is what an addon gave, at the time it gave it.
type fetched[T any] struct {
	value T
	at    time.Time
}

// fresh reports whether f is at most life old at now.
func (f fetched[T]) fresh(now time.Time, life time.Duration) bool { return now.Sub(f.at) <= life }

// fetchedSize is the size of what an addon gave, as the JSON of its value
// measures it (see cache.Sized).
func fetchedSize[T any](f fetched[T]) int { return cache.JSONSize(f.value) }

// keptCatalogLife is how long catalog pages and automatic images are kept,
// stale ones included.
func (s *Service) keptCatalogLife() time.Duration { return s.catalogLife() + staleCatalogs }

// shared runs fetch once for all the callers of key at once, on a context
// that the callers giving up does not cancel: each caller waits on its own
// ctx. Addon requests bound themselves (see stremio.Client).
func (s *Service) shared(ctx context.Context, key string, fetch func(context.Context) (any, error)) (any, error) {
	detached := context.WithoutCancel(ctx)
	results := s.flight.DoChan(key, func() (any, error) { return fetch(detached) })
	select {
	case result := <-results:
		return result.Val, result.Err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// background runs fetch for key on its own, unless it runs already, and
// does not wait for it. due, checked once it runs, tells whether it is
// still to be done: a refresh that ended between the caller's read and
// this one already did it.
func (s *Service) background(ctx context.Context, key string, due func() bool, fetch func(context.Context) error) {
	detached := context.WithoutCancel(ctx)
	s.flight.DoChan(key, func() (any, error) {
		if !due() {
			return nil, nil
		}
		return nil, fetch(detached)
	})
}

// replacedSince reports whether c keeps for key a value newer than seen.
func replacedSince[K comparable, T any](c *cache.Cache[K, fetched[T]], key K, seen time.Time) bool {
	kept, ok := c.Get(key)
	return ok && kept.at.After(seen)
}

// configOf names the address an addon is asked at, its configuration
// included, without keeping the address, which may carry credentials.
func configOf(addon addons.Addon) string {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(addon.ManifestURL))
	return strconv.FormatUint(hash.Sum64(), 16)
}

// key is the page of src's catalog starting at skip.
func (src source) key(skip int) pageKey {
	return pageKey{src.addon.addon.ID, src.catalog.Type, src.catalog.ID, src.genre, src.search, src.date, skip}
}

// kept reports whether pages of key's kind are kept in the database: those
// of catalogs, not of searches or guides.
func (key pageKey) kept() bool { return key.search == "" && key.date == "" }

// keptOnly marks a context whose reads take only the pages already kept,
// asking no addon (see KnownPages).
type keptOnlyKey struct{}

func withKeptOnly(ctx context.Context) context.Context {
	return context.WithValue(ctx, keptOnlyKey{}, true)
}

func keptOnly(ctx context.Context) bool {
	only, _ := ctx.Value(keptOnlyKey{}).(bool)
	return only
}

// errNotKept reports a page a read that asks no addon does not know.
var errNotKept = errors.New("catalog page not kept")

// refreshingKey marks the context of a read that asks the addon at once
// for the pages past their refresh age, and waits for them, instead of
// taking them and asking again in the background (see ReadCollections).
type refreshingKey struct{}

func withRefreshing(ctx context.Context) context.Context {
	return context.WithValue(ctx, refreshingKey{}, true)
}

func refreshing(ctx context.Context) bool {
	asked, _ := ctx.Value(refreshingKey{}).(bool)
	return asked
}

// page returns the catalog page starting at skip: the one kept in memory,
// else the one kept in the database, either asked for again in the
// background once past its refresh age, else the addon's answer. A read
// that asks no addon (see keptOnly) gets errNotKept instead of the addon's
// answer; a read that refreshes (see withRefreshing) waits for the addon's
// answer to a page past its refresh age, and takes the kept one if the
// addon fails. A search's page is kept apart, for searchTTL, and never
// refreshed: every term is a new page, seldom read again.
func (s *Service) page(ctx context.Context, src source, skip int) ([]stremio.Meta, error) {
	// An IPTV source's live TV catalog is one page, which it remembers
	// itself until its list changes; its movie and series catalogs read
	// pages from its stored lists.
	if src.addon.addon.IPTV() {
		switch {
		case s.iptv == nil || src.date != "":
			return nil, nil
		case !LiveCatalog(src.catalog.Type):
			return s.iptv.Catalog(ctx, src.addon.addon.ID, src.catalog.Type, src.catalog.ID, skip, src.genre, src.search)
		case skip > 0 || src.genre != "" || src.search != "":
			return nil, nil
		}
		return s.iptv.Channels(ctx, src.addon.addon.ID)
	}
	key := src.key(skip)
	if kept, ok := s.pagesOf(key).Get(key); ok {
		if key.search == "" && !kept.fresh(s.now(), s.catalogLife()) {
			if refreshing(ctx) {
				return s.refetchPage(ctx, src, key, kept.value)
			}
			s.refreshPage(ctx, src, key, kept.at)
		}
		return kept.value, nil
	}
	if _, ok := src.extras(skip); !ok {
		return nil, nil
	}
	if keptOnly(ctx) {
		kept, ok := s.loadPage(ctx, src, key)
		if !ok {
			return nil, errNotKept
		}
		if !kept.fresh(s.now(), s.catalogLife()) {
			s.refreshPage(ctx, src, key, kept.at)
		}
		return kept.value, nil
	}
	result, err := s.shared(ctx, fmt.Sprintf("page %v", key), func(ctx context.Context) (any, error) {
		if kept, ok := s.loadPage(ctx, src, key); ok {
			if !kept.fresh(s.now(), s.catalogLife()) {
				if refreshing(ctx) {
					return s.refetchPage(ctx, src, key, kept.value)
				}
				s.refreshPage(ctx, src, key, kept.at)
			}
			return kept.value, nil
		}
		return s.fetchPage(ctx, src, key)
	})
	if err != nil {
		return nil, err
	}
	return result.([]stremio.Meta), nil
}

// pagesOf returns the cache that keeps the page of key.
func (s *Service) pagesOf(key pageKey) *cache.Cache[pageKey, fetched[[]stremio.Meta]] {
	if key.search != "" {
		return s.searchPages
	}
	return s.pages
}

// cachedPage returns the page of key if it is kept in memory.
func (s *Service) cachedPage(key pageKey) ([]stremio.Meta, bool) {
	kept, ok := s.pagesOf(key).Get(key)
	return kept.value, ok
}

// readyPage returns the page of key if it is kept in memory and a read on
// ctx takes it without asking the addon: any kept page, but only a fresh
// one for a read that refreshes.
func (s *Service) readyPage(ctx context.Context, key pageKey) ([]stremio.Meta, bool) {
	kept, ok := s.pagesOf(key).Get(key)
	if !ok || refreshing(ctx) && key.search == "" && !kept.fresh(s.now(), s.catalogLife()) {
		return nil, false
	}
	return kept.value, true
}

// refetchPage asks the addon for a page past its refresh age and waits for
// its answer, for a read that refreshes: the stale page when the addon
// fails.
func (s *Service) refetchPage(ctx context.Context, src source, key pageKey, stale []stremio.Meta) ([]stremio.Meta, error) {
	result, err := s.shared(ctx, fmt.Sprintf("refetch page %v", key), func(ctx context.Context) (any, error) {
		return s.fetchPage(ctx, src, key)
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return stale, nil
	}
	return result.([]stremio.Meta), nil
}

// fetchPage asks the addon for a catalog page, and keeps its answer, as old
// as the request: a slow answer is not taken for a newer one.
func (s *Service) fetchPage(ctx context.Context, src source, key pageKey) ([]stremio.Meta, error) {
	extra, ok := src.extras(key.skip)
	if !ok {
		return nil, nil
	}
	asked := s.now()
	metas, err := s.client.Catalog(ctx, src.addon.addon.ManifestURL, src.catalog.Type, src.catalog.ID, extra, src.addon.confined)
	if err != nil {
		return nil, err
	}
	kept := fetched[[]stremio.Meta]{metas, asked}
	s.pagesOf(key).Put(key, kept)
	if key.kept() {
		if err := s.storePage(ctx, src, key, kept); err != nil {
			s.logger.Debug("A catalog page could not be kept", "catalog", src.catalog.ID, "error", err)
		}
	}
	return metas, nil
}

// refreshPage asks the addon for a page again in the background; the page
// kept, given at seen, stays until it answers.
func (s *Service) refreshPage(ctx context.Context, src source, key pageKey, seen time.Time) {
	due := func() bool { return !replacedSince(s.pagesOf(key), key, seen) }
	s.background(ctx, fmt.Sprintf("refresh page %v", key), due, func(ctx context.Context) error {
		_, err := s.fetchPage(ctx, src, key)
		if err != nil {
			s.logger.Debug("A catalog page could not be refreshed", "catalog", src.catalog.ID, "error", err)
		}
		return err
	})
}

// loadPage reads a page kept in the database, and keeps it in memory.
func (s *Service) loadPage(ctx context.Context, src source, key pageKey) (fetched[[]stremio.Meta], bool) {
	if !key.kept() {
		return fetched[[]stremio.Meta]{}, false
	}
	var data []byte
	var kept fetched[[]stremio.Meta]
	err := s.db.QueryRow(ctx, `SELECT metas, fetched_at FROM catalog_pages
		WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3 AND genre = $4 AND skip = $5 AND config = $6 AND fetched_at > $7`,
		key.addon, key.catalogType, key.catalogID, key.genre, key.skip, configOf(src.addon.addon), s.now().Add(-s.keptCatalogLife())).
		Scan(&data, &kept.at)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) && ctx.Err() == nil {
			s.logger.Debug("A kept catalog page could not be read", "catalog", key.catalogID, "error", err)
		}
		return kept, false
	}
	if err := json.Unmarshal(data, &kept.value); err != nil {
		return kept, false
	}
	s.pages.Put(key, kept)
	return kept, true
}

func (s *Service) storePage(ctx context.Context, src source, key pageKey, kept fetched[[]stremio.Meta]) error {
	metas := kept.value
	if metas == nil {
		metas = []stremio.Meta{}
	}
	data, err := json.Marshal(metas)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO catalog_pages (addon_id, catalog_type, catalog_id, genre, skip, config, metas, fetched_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (addon_id, catalog_type, catalog_id, genre, skip)
		DO UPDATE SET config = excluded.config, metas = excluded.metas, fetched_at = excluded.fetched_at`,
		key.addon, key.catalogType, key.catalogID, key.genre, key.skip, configOf(src.addon.addon), data, kept.at)
	return err
}

// meta returns an addon's complete description of a title (see keptMeta).
func (s *Service) meta(ctx context.Context, addon installed, metaType, id string) (stremio.Meta, error) {
	kept, err := s.keptMeta(ctx, addon, metaType, id)
	return kept.value, err
}

// keptMeta returns an addon's complete description of a title, with when
// the addon gave it: the one kept, refreshed in the background once past
// metaTTL, or past the catalog refresh age when an app opens the title
// (see iptv.Opening), else the one kept in the database, else the addon's
// answer.
func (s *Service) keptMeta(ctx context.Context, addon installed, metaType, id string) (fetched[stremio.Meta], error) {
	// An IPTV source describes its titles from its database, with details
	// once a title was opened: it is not cached here, lest a listing's
	// description hide the details.
	if addon.addon.IPTV() {
		meta, err := s.fetchMeta(ctx, addon, metaType, id)
		return fetched[stremio.Meta]{meta, s.now()}, err
	}
	key := metaKey{addon.addon.ID, metaType, id}
	if kept, ok := s.metas.Get(key); ok {
		if s.metaStale(ctx, kept) {
			s.refreshMeta(ctx, addon, key, kept.at)
		}
		return kept, nil
	}
	result, err := s.shared(ctx, fmt.Sprintf("meta %v", key), func(ctx context.Context) (any, error) {
		if kept, ok := s.loadMeta(ctx, addon, key); ok {
			if s.metaStale(ctx, kept) {
				s.refreshMeta(ctx, addon, key, kept.at)
			}
			return kept, nil
		}
		return s.fetchKeptMeta(ctx, addon, key)
	})
	if err != nil {
		return fetched[stremio.Meta]{}, err
	}
	return result.(fetched[stremio.Meta]), nil
}

// metaStale reports whether a description is to be asked for again.
func (s *Service) metaStale(ctx context.Context, kept fetched[stremio.Meta]) bool {
	return !kept.fresh(s.now(), metaTTL) || iptv.IsOpening(ctx) && !kept.fresh(s.now(), s.catalogLife())
}

// fetchKeptMeta asks the addon for a description and keeps it. A movie's
// or series' credited people are recorded then, so that apps can open
// them (see saveCredits): reading a kept description writes nothing.
func (s *Service) fetchKeptMeta(ctx context.Context, addon installed, key metaKey) (fetched[stremio.Meta], error) {
	meta, err := s.fetchMeta(ctx, addon, key.metaType, key.id)
	if err != nil {
		return fetched[stremio.Meta]{}, err
	}
	kept := fetched[stremio.Meta]{meta, s.now()}
	s.metas.Put(key, kept)
	if err := s.storeMeta(ctx, addon, key, kept); err != nil {
		s.logger.Debug("A description could not be kept", "addon", addon.addon.Manifest.Name, "error", err)
	}
	if kind, ok := titleKind(key.metaType); ok && (kind == KindMovie || kind == KindSeries) {
		if err := s.saveCredits(ctx, itemID(titleKey(kind, key.id)), people(meta), addon.confined); err != nil {
			s.logger.Debug("The people of a title could not be recorded", "addon", addon.addon.Manifest.Name, "error", err)
		}
	}
	return kept, nil
}

// refreshMeta asks the addon for a description again in the background;
// the one kept, given at seen, stays until it answers.
func (s *Service) refreshMeta(ctx context.Context, addon installed, key metaKey, seen time.Time) {
	due := func() bool { return !replacedSince(s.metas, key, seen) }
	s.background(ctx, fmt.Sprintf("refresh meta %v", key), due, func(ctx context.Context) error {
		_, err := s.fetchKeptMeta(ctx, addon, key)
		if err != nil {
			s.logger.Debug("A description could not be refreshed", "addon", addon.addon.Manifest.Name, "error", err)
		}
		return err
	})
}

// loadMeta reads a description kept in the database, and keeps it in
// memory.
func (s *Service) loadMeta(ctx context.Context, addon installed, key metaKey) (fetched[stremio.Meta], bool) {
	var data []byte
	var kept fetched[stremio.Meta]
	err := s.db.QueryRow(ctx, `SELECT meta, fetched_at FROM metas
		WHERE addon_id = $1 AND meta_type = $2 AND meta_id = $3 AND config = $4 AND fetched_at > $5`,
		key.addon, key.metaType, key.id, configOf(addon.addon), s.now().Add(-metaKept)).Scan(&data, &kept.at)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) && ctx.Err() == nil {
			s.logger.Debug("A kept description could not be read", "error", err)
		}
		return kept, false
	}
	if err := json.Unmarshal(data, &kept.value); err != nil {
		return kept, false
	}
	s.metas.Put(key, kept)
	return kept, true
}

func (s *Service) storeMeta(ctx context.Context, addon installed, key metaKey, kept fetched[stremio.Meta]) error {
	data, err := json.Marshal(kept.value)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO metas (addon_id, meta_type, meta_id, config, meta, fetched_at) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (addon_id, meta_type, meta_id) DO UPDATE SET config = excluded.config, meta = excluded.meta, fetched_at = excluded.fetched_at`,
		key.addon, key.metaType, key.id, configOf(addon.addon), data, kept.at)
	return err
}

// forgetMeta forgets the description of key, kept in memory and in the
// database, so that the addon is asked for it again.
func (s *Service) forgetMeta(ctx context.Context, key metaKey) error {
	s.metas.Delete(key)
	_, err := s.db.Exec(ctx, "DELETE FROM metas WHERE addon_id = $1 AND meta_type = $2 AND meta_id = $3", key.addon, key.metaType, key.id)
	return err
}

// sizeKey is a catalog, whose pages are as long whatever the genre.
type sizeKey struct {
	addon       accounts.ID
	catalogType string
	catalogID   string
}

// pageSizes remembers how many titles a page of each catalog holds: the
// longest page seen. A listing then asks for all the pages it needs at
// once, its first page included (see window).
type pageSizes struct {
	mu        sync.Mutex
	loaded    bool
	byCatalog map[sizeKey]int
}

// pageSize returns the page size remembered for src's catalog, 0 when none
// is.
func (s *Service) pageSize(ctx context.Context, src source) int {
	s.sizes.mu.Lock()
	defer s.sizes.mu.Unlock()
	if !s.sizes.loaded {
		if err := s.loadPageSizes(ctx); err != nil {
			if ctx.Err() == nil {
				s.logger.Debug("Catalog page sizes could not be read", "error", err)
			}
			return 0
		}
		s.sizes.loaded = true
	}
	return s.sizes.byCatalog[sizeKey{src.addon.addon.ID, src.catalog.Type, src.catalog.ID}]
}

// loadPageSizes reads the page sizes kept in the database; s.sizes.mu is
// held.
func (s *Service) loadPageSizes(ctx context.Context) error {
	rows, err := s.db.Query(ctx, "SELECT addon_id, catalog_type, catalog_id, page_size FROM catalog_sizes")
	if err != nil {
		return err
	}
	sizes := map[sizeKey]int{}
	var key sizeKey
	var size int
	if _, err := pgx.ForEachRow(rows, []any{&key.addon, &key.catalogType, &key.catalogID, &size}, func() error {
		sizes[key] = size
		return nil
	}); err != nil {
		return err
	}
	s.sizes.byCatalog = sizes
	return nil
}

// learnPageSize remembers that a page of src's catalog held size titles,
// when it is more than remembered.
func (s *Service) learnPageSize(ctx context.Context, src source, size int) {
	key := sizeKey{src.addon.addon.ID, src.catalog.Type, src.catalog.ID}
	s.sizes.mu.Lock()
	if size <= s.sizes.byCatalog[key] || !s.sizes.loaded {
		s.sizes.mu.Unlock()
		return
	}
	s.sizes.byCatalog[key] = size
	s.sizes.mu.Unlock()
	if _, err := s.db.Exec(context.WithoutCancel(ctx), `INSERT INTO catalog_sizes (addon_id, catalog_type, catalog_id, page_size) VALUES ($1, $2, $3, $4)
		ON CONFLICT (addon_id, catalog_type, catalog_id) DO UPDATE SET page_size = greatest(catalog_sizes.page_size, excluded.page_size)`,
		key.addon, key.catalogType, key.catalogID, size); err != nil {
		s.logger.Debug("A catalog page size could not be kept", "catalog", key.catalogID, "error", err)
	}
}

// requestViewsKey holds the views a request built (see PerRequest).
type requestViewsKey struct{}

// requestViews are the views a request built, kept for viewReuse after
// it started: a request that lasts, as a WebSocket does, sees changes to
// addons and libraries.
type requestViews struct {
	mu      sync.Mutex
	started time.Time
	views   map[accounts.ID]view
}

const viewReuse = 10 * time.Second

// PerRequest returns a context under which the service builds each user's
// view once: what they can browse, read from their addons and libraries.
// The Jellyfin API serves each request under one, so that a listing whose
// items each need the view builds it once.
func PerRequest(ctx context.Context) context.Context {
	return context.WithValue(ctx, requestViewsKey{}, &requestViews{started: time.Now(), views: map[accounts.ID]view{}})
}

// WarmInterval is how often the first pages of libraries are read again
// (see Warm).
const WarmInterval = time.Hour

// Warm reads the first page of each enabled library, as home screens show
// it, with the automatic images of those that find their own, so that no
// home screen waits for an addon: those of the server's addons, and of
// users' own addons when the settings allow them. Pages already fresh are
// not asked again; stale ones are refreshed. It also forgets the pages
// and descriptions kept in the database that no one read for long.
func (s *Service) Warm(ctx context.Context) error {
	scopes := []struct {
		scope    addons.Scope
		confined bool
	}{{addons.Shared(), false}}
	if s.settings().PersonalAddons {
		rows, err := s.db.Query(ctx, `SELECT DISTINCT a.owner_id, u.is_administrator FROM addons a JOIN users u ON u.id = a.owner_id`)
		if err != nil {
			return err
		}
		var owner accounts.ID
		var administrator bool
		if _, err := pgx.ForEachRow(rows, []any{&owner, &administrator}, func() error {
			// A user's own addons may reach the local network only if that
			// user is an administrator (see view).
			scopes = append(scopes, struct {
				scope    addons.Scope
				confined bool
			}{addons.Personal(owner), !administrator})
			return nil
		}); err != nil {
			return err
		}
	}
	var group errgroup.Group
	group.SetLimit(catalogFetches)
	for _, scope := range scopes {
		list, err := s.addons.Addons(ctx, scope.scope)
		if err != nil {
			return err
		}
		byID := map[accounts.ID]addons.Addon{}
		for _, addon := range list {
			if addon.Enabled && addon.Stremio() {
				byID[addon.ID] = addon
			}
		}
		libraries, err := s.addons.Libraries(ctx, scope.scope)
		if err != nil {
			return err
		}
		for _, l := range libraries {
			addon, ok := byID[l.AddonID]
			if !ok || !l.Enabled || LiveCatalog(l.Catalog.Type) {
				continue
			}
			entry := installed{addon: addon, confined: scope.confined, shared: scope.scope.Owner == nil}
			// The page apps read first: of the library's genre, when it is
			// narrowed to one.
			src := source{addon: entry, catalog: l.Catalog, genre: l.Genre}
			group.Go(func() error {
				if _, err := s.page(ctx, src, 0); err != nil && ctx.Err() == nil {
					s.logger.Debug("A library's first page could not be read", "addon", addon.Manifest.Name, "catalog", l.Catalog.ID, "error", err)
				}
				if l.Image == addons.LibraryImageAutomatic {
					s.lookUpImage(ctx, src)
				}
				return nil
			})
		}
	}
	_ = group.Wait()
	if _, err := s.db.Exec(ctx, "DELETE FROM catalog_pages WHERE fetched_at < $1", s.now().Add(-s.keptCatalogLife())); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, "DELETE FROM metas WHERE fetched_at < $1", s.now().Add(-metaKept))
	return err
}

// ReadCollections reads every collection of the server's collection
// libraries whole, as a whole listing lists it (see Whole), one collection
// after the other, so that apps then open each at once with all its
// titles. It asks the addons again for the pages past their refresh age
// and waits for their answers (see withRefreshing), and reads the pages
// never read. It reads the libraries a request without a user sees: those
// of the server's addons.
func (s *Service) ReadCollections(ctx context.Context) error {
	started := time.Now()
	ctx = withRefreshing(context.WithValue(ctx, wholeKey{}, true))
	var server accounts.User
	v, err := s.view(ctx, server)
	if err != nil {
		return err
	}
	read, failed := 0, 0
	whole := func(parent accounts.ID) (Page, error) {
		most, err := s.listingLimit(ctx, server, parent)
		if err != nil {
			return Page{}, err
		}
		return s.Children(ctx, server, parent, 0, most, "")
	}
	for _, l := range v.libraries {
		if l.catalog.Type != "collection" {
			continue
		}
		listed, err := whole(l.item.ID)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			s.logger.Debug("A collection library could not be read", "library", l.item.Name, "error", err)
			failed++
			continue
		}
		for _, collection := range listed.Items {
			if collection.Kind != KindCollection {
				continue
			}
			if _, err := whole(collection.ID); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				s.logger.Debug("A collection could not be read", "collection", collection.Name, "error", err)
				failed++
				continue
			}
			read++
		}
	}
	s.logger.Info("Read the collections", "collections", read, "failed", failed, "took", time.Since(started).Round(time.Second))
	return nil
}
