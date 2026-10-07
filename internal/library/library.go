package library

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/eclipse"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/stremio"
)

const (
	// metaTTL is the age past which a title's description is asked for
	// again (see meta).
	metaTTL = 6 * time.Hour
	// pageFetches bounds the pages of one catalog a read asks for at once,
	// those already kept not counting.
	pageFetches = 8
	// catalogFetches bounds the catalogs merged reads at once: a
	// collection's sources each read a page or two, together.
	catalogFetches = 16
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

	// pages, metas and libraryImages keep what addons answered, with when
	// (see kept.go); searchPages, the pages of searches, briefly; sizes,
	// how long each catalog's pages are.
	pages       *cache.Cache[pageKey, fetched[[]stremio.Meta]]
	searchPages *cache.Cache[pageKey, fetched[[]stremio.Meta]]
	metas       *cache.Cache[metaKey, fetched[stremio.Meta]]
	flight      singleflight.Group
	// libraryImages are the libraries' automatic images, by catalog (see
	// automaticImage).
	libraryImages *cache.Cache[pageKey, fetched[string]]
	sizes         pageSizes
	// prefetches holds a place for each listing read ahead (see
	// readAhead); readAhead turns reading ahead on.
	prefetches chan struct{}
	readAhead  bool
	// iptvHosts tells the hosts of IPTV sources and of their logos (see
	// IPTVHost).
	iptvHosts iptvHosts

	// streamLists and subtitleLists are the addons' lists for each title,
	// kept stale once expired (see staleLists).
	streamLists   *cache.Cache[streamKey, list[stremio.Stream]]
	subtitleLists *cache.Cache[streamKey, list[stremio.Subtitle]]
	versions      *cache.Cache[accounts.ID, Version]
	// asked counts, for each user's title, the addons still asked for its
	// streams in the background (see VersionsNow).
	asked *cache.Cache[askedKey, *asked]
	// followUps are the addons asked again for the lists they just gave,
	// after each of followUpDelays (see followUpList).
	followUps      followUps
	followUpDelays []time.Duration
	// tracking follows titles' versions for the pages that show them, and
	// saves stream lists (see versionTracking).
	tracking versionTracking
	// ratingLookups holds a place for each rating looked up (see visible);
	// ratingWait bounds how long a request waits for them.
	ratingLookups chan struct{}
	ratingWait    time.Duration
	// wholeWait bounds how long a whole listing waits for addons (see
	// Whole).
	wholeWait time.Duration
	// guideDir is where XMLTV guides in ZIP archives are spooled while they
	// are read (see SpoolGuidesIn); empty, they are not read.
	guideDir string
	// iptv answers for Polyfin's own IPTV sources (see UseIPTV).
	iptv IPTV

	// music asks Eclipse addons for their resources, which musicCache
	// keeps (see musicCaches).
	music      *eclipse.Client
	musicCache musicCaches

	// overrides are administrators' edits of items, loaded on first use
	// (see overridesNow); overridesMu orders their loading and changes.
	overrides   atomic.Pointer[overrideSet]
	overridesMu sync.Mutex
}

// IPTV answers, for Polyfin's own IPTV sources, the catalog, meta and
// stream requests addons answer over HTTP (see package iptv).
type IPTV interface {
	Channels(ctx context.Context, source accounts.ID) ([]stremio.Meta, error)
	Catalog(ctx context.Context, source accounts.ID, catalogType, catalogID string, skip int, genre, search string) ([]stremio.Meta, error)
	CatalogWindow(ctx context.Context, source accounts.ID, catalogType, catalogID, genre, search string, start, count int) ([]stremio.Meta, int, error)
	Enrichment(ctx context.Context, source accounts.ID) bool
	Meta(ctx context.Context, source accounts.ID, id string) (stremio.Meta, error)
	Streams(ctx context.Context, source accounts.ID, id string) ([]stremio.Stream, error)
	MappingChannels(ctx context.Context, source accounts.ID) ([]stremio.Meta, error)
	Logo(ctx context.Context, item accounts.ID) (string, bool, error)
}

// UseIPTV sets what answers for IPTV sources; it is called before the
// service is used. Without it, IPTV sources list nothing.
func (s *Service) UseIPTV(iptv IPTV) { s.iptv = iptv }

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
		db:       db,
		addons:   store,
		client:   client,
		logger:   logger,
		now:      time.Now,
		settings: settings,
		metas: cache.NewLasting[metaKey, fetched[stremio.Meta]](4000, func() time.Duration { return metaKept }, time.Now).
			Sized(64<<20, fetchedSize[stremio.Meta]),
		versions:       cache.New[accounts.ID, Version](20000, versionsTTL),
		asked:          cache.New[askedKey, *asked](maxAsked, askedFor),
		followUps:      followUps{running: map[followKey]*followUp{}},
		followUpDelays: followUpDelays,
		ratingLookups:  make(chan struct{}, ratingFetches),
		ratingWait:     ratingWait,
		wholeWait:      wholeListingWait,
		music:          eclipse.NewClient(client),
	}
	clock := func() time.Time { return s.now() }
	s.pages = cache.NewLasting[pageKey, fetched[[]stremio.Meta]](4000, s.keptCatalogLife, clock).Sized(64<<20, fetchedSize[[]stremio.Meta])
	s.searchPages = cache.NewLasting[pageKey, fetched[[]stremio.Meta]](500, func() time.Duration { return searchTTL }, clock).
		Sized(16<<20, fetchedSize[[]stremio.Meta])
	s.libraryImages = cache.NewLasting[pageKey, fetched[string]](1000, s.keptCatalogLife, clock)
	s.prefetches = make(chan struct{}, prefetchLimit)
	s.readAhead = true
	s.streamLists = cache.NewLasting[streamKey, list[stremio.Stream]](2000, s.keptListLife, clock)
	s.subtitleLists = cache.NewLasting[streamKey, list[stremio.Subtitle]](2000, s.keptListLife, clock)
	s.musicCache = newMusicCaches(s)
	return s
}

// catalogLife is how long catalog pages are kept, and listLife how long a
// title's version and subtitle lists from the addons are fresh: the
// settings' CatalogRefreshMinutes and VersionListMinutes. keptListLife is
// how long those lists are kept, stale ones included (see staleLists).
func (s *Service) catalogLife() time.Duration {
	return time.Duration(s.settings().CatalogRefreshMinutes) * time.Minute
}

func (s *Service) listLife() time.Duration {
	return time.Duration(s.settings().VersionListMinutes) * time.Minute
}

func (s *Service) keptListLife() time.Duration {
	return s.listLife() + staleLists
}

// SetClock replaces the clock the service measures time on, before the
// service is used. Tests move it to let lists expire; the server keeps
// time.Now.
func (s *Service) SetClock(now func() time.Time) { s.now = now }

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

// library is a library of a user with its catalog, the genre it is
// narrowed to and the most items it lists (see addons.Library), and how it
// finds its image (see addons.Library.Image).
type library struct {
	item     Item
	addon    installed
	catalog  stremio.Catalog
	genre    string
	maxItems int
	image    string
}

// catalogSource is the library's catalog as it lists it: narrowed to its
// genre and limited to its maximum.
func (l library) catalogSource() source {
	return source{addon: l.addon, catalog: l.catalog, genre: l.genre, max: l.maxItems}
}

// view is what a user can browse: their enabled addons and libraries, the
// server's first unless they turned them off. Enabled live TV catalogs are
// not libraries: they list the user's channels, in the same order.
type view struct {
	addons    []installed
	libraries []library
	channels  []source
	// parental is the user's parental control and genres the genres they
	// block, which hide titles (see visible). The ratings and genres of the
	// titles a request lists are looked up until deadline, lookups counting
	// how many it started; held is set once a listing of the request
	// stopped short for them.
	parental accounts.ParentalControl
	genres   []string
	deadline time.Time
	lookups  *atomic.Int32
	held     *atomic.Bool
	// catalogLimit and channelLimit are the settings' limits when the
	// request came (see limit).
	catalogLimit int
	channelLimit int
}

// limit is how many items one read of src's catalog fetches at most: the
// channel limit for a live TV catalog, else the maximum of the library it
// lists for, lower or higher than the catalog limit, else the catalog
// limit. Some catalogs are nearly endless; an IPTV source's movies and
// series are a stored list, read whole unless the library has a maximum.
func (v view) limit(src source) int {
	switch {
	case LiveCatalog(src.catalog.Type):
		return v.channelLimit
	case src.max > 0:
		return src.max
	case src.addon.addon.IPTV():
		return math.MaxInt32
	}
	return v.catalogLimit
}

// view returns what user can browse, built once per request served under
// PerRequest.
func (s *Service) view(ctx context.Context, user accounts.User) (view, error) {
	views, _ := ctx.Value(requestViewsKey{}).(*requestViews)
	if views == nil || time.Since(views.started) > viewReuse {
		return s.buildView(ctx, user)
	}
	views.mu.Lock()
	defer views.mu.Unlock()
	if v, ok := views.views[user.ID]; ok {
		return v, nil
	}
	v, err := s.buildView(ctx, user)
	if err == nil {
		views.views[user.ID] = v
	}
	return v, err
}

func (s *Service) buildView(ctx context.Context, user accounts.User) (view, error) {
	settings := s.settings()
	// A user under parental control or blocking genres browses the server's
	// addons only: their own addons could describe titles without the
	// ratings or genres that hide them. So does a user whose own addons the
	// server or their own permission turned off: their addons are kept, but
	// not used. A request without a user, made with an API key, browses the
	// server's addons too.
	scopes := []addons.Scope{addons.Shared()}
	if !user.Restricted() && user.ID != (accounts.ID{}) && settings.PersonalAddonsAllowed(user) {
		scopes = []addons.Scope{addons.Personal(user.ID)}
		if user.UseSharedAddons {
			scopes = append([]addons.Scope{addons.Shared()}, scopes...)
		}
	}
	v := view{parental: user.Parental, genres: user.BlockedGenres, deadline: time.Now().Add(s.ratingWait), lookups: new(atomic.Int32), held: new(atomic.Bool),
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
				// A user without Live TV has no channels, so none of them,
				// nor their programmes, can be reached.
				if user.LiveTv {
					v.channels = append(v.channels, source{addon: entry, catalog: l.Catalog, guide: len(l.Guides) > 0})
				}
			// The server's libraries the user does not see; their titles stay
			// reachable through the addon.
			case scope.Owner == nil && slices.Contains(user.HiddenLibraries, LibraryID(l)):
			default:
				visible = append(visible, l)
				entries = append(entries, entry)
			}
		}
	}
	for i, name := range LibraryNames(visible, settings.Language) {
		l := visible[i]
		collection := collectionType(l.Catalog.Type)
		if music := entries[i].addon.Music; music != nil && music.ContentType == eclipse.ContentAudiobook {
			// Jellyfin keeps audiobooks in books libraries.
			collection = "books"
		}
		v.libraries = append(v.libraries, library{
			item: Item{
				ID:             LibraryID(l),
				Kind:           KindLibrary,
				Name:           name,
				CollectionType: collection,
			},
			addon:    entries[i],
			catalog:  l.Catalog,
			genre:    l.Genre,
			maxItems: l.MaxItems,
			image:    l.Image,
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

// Libraries lists a user's libraries in order, with their images (see
// libraryimages.go).
func (s *Service) Libraries(ctx context.Context, user accounts.User) ([]Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	images := s.automaticImages(ctx, v.libraries)
	items := make([]Item, 0, len(v.libraries))
	records := make([]record, 0, len(v.libraries))
	for i, l := range v.libraries {
		item := l.item
		if !v.restricted() {
			item.Images.Primary = images[i]
		}
		items = append(items, item)
		records = append(records, libraryRecord(l, images[i]))
	}
	return s.overridden(items), s.save(ctx, records)
}

// LibraryID is the item of a library in Jellyfin apps.
func LibraryID(l addons.Library) accounts.ID {
	return itemID(libraryKey(l.AddonID, l.Catalog.Type, l.Catalog.ID))
}

// ServerLibrary is one of the server's libraries, with the genres its
// catalog can be narrowed to, which name its genre pages: only its own
// when it is narrowed to one.
type ServerLibrary struct {
	ID     accounts.ID
	Name   string
	Genres []string
}

// ServerLibraries lists the server's libraries, in order and named in
// language as apps show them: the enabled catalogs of its enabled addons,
// but live TV catalogs, which make no library. A user's apps show those
// the user does not hide, when the user uses the server's addons.
func ServerLibraries(ctx context.Context, store *addons.Store, language string) ([]ServerLibrary, error) {
	list, err := store.Addons(ctx, addons.Shared())
	if err != nil {
		return nil, err
	}
	enabled := map[accounts.ID]bool{}
	for _, addon := range list {
		enabled[addon.ID] = addon.Enabled
	}
	libraries, err := store.Libraries(ctx, addons.Shared())
	if err != nil {
		return nil, err
	}
	var shown []addons.Library
	for _, l := range libraries {
		if l.Enabled && enabled[l.AddonID] && !LiveCatalog(l.Catalog.Type) {
			shown = append(shown, l)
		}
	}
	result := make([]ServerLibrary, 0, len(shown))
	for i, name := range LibraryNames(shown, language) {
		library := ServerLibrary{ID: LibraryID(shown[i]), Name: name, Genres: []string{}}
		if genre := shown[i].Genre; genre != "" {
			library.Genres = append(library.Genres, genre)
		} else {
			library.Genres = append(library.Genres, addons.Genres(shown[i].Catalog)...)
		}
		result = append(result, library)
	}
	return result, nil
}

// ServerLibraries lists the server's libraries (see ServerLibraries).
func (s *Service) ServerLibraries(ctx context.Context) ([]ServerLibrary, error) {
	return ServerLibraries(ctx, s.addons, s.settings().Language)
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
	// guide is set for a live TV catalog with an XMLTV guide.
	guide bool
	// first reads the catalog's first page only, as Stremio apps show
	// searches and home rows.
	first bool
	// max is the most items of the catalog the library it lists for
	// shows, in place of the catalog limit; 0 for that limit (see
	// view.limit).
	max int
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

// window returns the catalog's items [start, start+count) and whether more
// follow, reading no further than the catalog's limit (see view.limit). A
// Stremio catalog is read in order: each page is requested with
// skip set to the number of items before it, and an empty page ends it.
// Pages may be shorter than the first when the addon filters them, so the
// pages fetched ahead together, on the guess that they are as long as the
// first, only count while the guess holds; reading then goes on from the
// actual position. Each read asks for up to pageFetches pages at once, the
// pages kept not counting, and none past a page kept empty, which ends the
// catalog. When the catalog's page size is remembered (see pageSize), the
// first read asks for every page it needs at once; a size learned from a
// short first page is only trusted once a second page confirms it, so
// that a small catalog is not asked for many empty pages. Titles the
// user's parental control hides are left out before positions are
// counted. A restricted listing reads at most hiddenReach times as far as
// an unrestricted one, and stops where titles wait for their rating: it
// reports that more may follow, which apps ask for later, once the
// ratings are known.
func (s *Service) window(ctx context.Context, v view, src source, start, count int) ([]stremio.Meta, int, error) {
	// An IPTV source's movies and series are paged and counted from its
	// stored lists, whole, but for a restricted user, whose listing leaves
	// hidden titles out as it reads.
	if src.addon.addon.IPTV() && !LiveCatalog(src.catalog.Type) && s.iptv != nil && !v.restricted() && src.date == "" {
		return s.iptv.CatalogWindow(ctx, src.addon.addon.ID, src.catalog.Type, src.catalog.ID, src.genre, src.search, start, count)
	}
	var collected []stremio.Meta
	seen := map[string]bool{}
	received := 0
	size := 0
	if src.paged() && !src.first {
		size = s.pageSize(ctx, src)
	}
	remembered, trusted, filled := size, size > 0, 0
	more := true
	limit := v.limit(src)
	reach := limit
	if v.restricted() {
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
		if !src.first && size > 0 && (trusted || size >= shortPage) {
			uncached := 0
			if _, ok := s.cachedPage(src.key(received)); !ok {
				uncached++
			}
			for next := received + size; next < min(received+needed, limit); next += size {
				if kept, ok := s.cachedPage(src.key(offsets[len(offsets)-1])); ok && len(kept) == 0 {
					break
				}
				_, cached := s.cachedPage(src.key(next))
				if !cached && uncached >= pageFetches {
					break
				}
				if !cached {
					uncached++
				}
				offsets = append(offsets, next)
			}
		}
		pages, missing, err := s.pagesAt(ctx, src, offsets)
		if err != nil {
			return nil, 0, err
		}
		var fresh []stremio.Meta
		repeated, unknown := false, false
		for i, metas := range pages {
			if offsets[i] != received {
				break
			}
			// A page not kept, read without asking the addon (see
			// KnownPages), ends what is known, not the catalog.
			if missing[i] {
				unknown = true
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
			if len(metas) > 0 {
				filled++
			}
		}
		trusted = trusted || filled >= 2
		visible, held := s.visibleMetas(ctx, v, src, fresh)
		collected = append(collected, visible...)
		if held {
			v.held.Store(true)
			break
		}
		if repeated || unknown {
			more = unknown
			break
		}
		if !src.paged() || src.first {
			more = more && src.paged()
			break
		}
	}
	if trusted && size > remembered {
		s.learnPageSize(ctx, src, size)
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

// shortPage is the length under which a first page does not tell the
// catalog's page size: the catalog may simply hold that few titles.
const shortPage = 10

// pagesAt fetches the catalog pages starting at each offset together. The
// pages kept answer at once: reads bound the pages they fetch (see
// window). missing tells the pages a read that asks no addon does not
// know.
func (s *Service) pagesAt(ctx context.Context, src source, offsets []int) (pages [][]stremio.Meta, missing []bool, err error) {
	pages, missing = make([][]stremio.Meta, len(offsets)), make([]bool, len(offsets))
	group, groupCtx := errgroup.WithContext(ctx)
	for i, offset := range offsets {
		group.Go(func() (err error) {
			pages[i], err = s.page(groupCtx, src, offset)
			if errors.Is(err, errNotKept) {
				missing[i], err = true, nil
			}
			return err
		})
	}
	return pages, missing, group.Wait()
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
	// A quarter more than an even share, as duplicates and short catalogs
	// leave the share short, saves most listings a second round.
	per := (need+need/4)/len(sources) + 1
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
		result := interleave(sources, lists)
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

// interleave merges the lists of sources, one item of each in turn,
// without duplicates.
func interleave(sources []source, lists [][]stremio.Meta) []listed {
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
			return result
		}
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
	page, err := s.children(ctx, user, parent, start, count, genre, false)
	page.Items = s.overridden(page.Items)
	return page, err
}

// Latest lists the first children of a library or collection as a home
// row shows them: from the first page of its catalogs only, as Stremio
// apps show their boards, so that a row never waits for more pages.
// Other folders list their children as Children does.
func (s *Service) Latest(ctx context.Context, user accounts.User, parent accounts.ID, count int) (Page, error) {
	page, err := s.children(ctx, user, parent, 0, count, "", true)
	page.Items = s.overridden(page.Items)
	return page, err
}

// KnownPages returns the first page of each of the user's libraries, but
// music ones, from the catalog pages Polyfin keeps, asking no addon: what
// apps' item counts are made of. A library none of whose first page is
// kept is empty.
func (s *Service) KnownPages(ctx context.Context, user accounts.User, count int) ([]Page, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	ctx = withKeptOnly(ctx)
	pages := make([]Page, 0, len(v.libraries))
	for _, l := range v.libraries {
		if l.addon.addon.Eclipse() {
			continue
		}
		page, err := s.libraryChildren(ctx, v, l, 0, count, "", false)
		if err != nil {
			return nil, err
		}
		pages = append(pages, page)
	}
	return pages, nil
}

func (s *Service) children(ctx context.Context, user accounts.User, parent accounts.ID, start, count int, genre string, first bool) (Page, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return Page{}, err
	}
	if l, ok := v.library(parent); ok {
		return s.libraryChildren(ctx, v, l, start, count, genre, first)
	}
	r, err := s.load(ctx, parent)
	if err != nil {
		return Page{}, err
	}
	switch r.Kind {
	case KindCollection:
		return s.collectionChildren(ctx, v, r, start, count, first)
	case KindSeries:
		seasons, err := s.Seasons(ctx, user, parent)
		return slicePage(seasons, start, count), err
	case KindSeason:
		episodes, err := s.Episodes(ctx, user, r.seriesItemID(), &parent)
		return slicePage(episodes, start, count), err
	case KindAlbum, KindArtist, KindMusicPlaylist:
		folder, entry, err := s.musicFolder(ctx, v, parent)
		if err != nil {
			return Page{}, err
		}
		records, err := s.folderMusic(ctx, v, entry, folder, nil)
		if err != nil {
			return Page{}, err
		}
		return s.musicChildren(ctx, v, records, entry, start, count)
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

func (s *Service) libraryChildren(ctx context.Context, v view, l library, start, count int, genre string, first bool) (Page, error) {
	if l.addon.addon.Eclipse() {
		records, err := s.musicLibraryRecords(ctx, v, l)
		if err != nil {
			return Page{}, err
		}
		return s.musicChildren(ctx, v, records, l.addon, start, count)
	}
	src := l.catalogSource()
	src.first = first
	if genre != "" {
		// A library narrowed to a genre lists that genre only.
		if l.genre != "" && genre != l.genre {
			return Page{}, nil
		}
		src.genre = genre
	}
	count = withinMax(start, count, l.maxItems)
	metas, total, err := s.window(ctx, v, src, start, count)
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
	result := page(items, start, totalWithinMax(total, l.maxItems))
	if result.More && !first {
		s.prefetch(ctx, v, l.addon, func(ctx context.Context) { _, _, _ = s.window(ctx, v, src, start+count, count) })
	}
	return result, s.save(ctx, records)
}

// withinMax bounds the count of a window starting at start to the first
// most items of a listing, 0 for no maximum.
func withinMax(start, count, most int) int {
	if most <= 0 {
		return count
	}
	return min(count, max(0, most-start))
}

// totalWithinMax bounds a listing's total to its first most items, 0 for no
// maximum.
func totalWithinMax(total, most int) int {
	if most <= 0 {
		return total
	}
	return min(total, most)
}

// page makes a Page of a window of children, out of total.
func page(items []Item, start, total int) Page {
	return Page{Items: items, Total: total, More: start+len(items) < total}
}

// prefetchLimit bounds the listings read ahead at once.
const prefetchLimit = 2

// prefetch reads the next window of a listing ahead, in the background, so
// that the app's next page answers at once: read fills the pages kept. It
// reads none when prefetchLimit listings are being read ahead already,
// for a user whose listings wait for ratings, nor from an addon whose last
// request failed, which would only be asked more.
func (s *Service) prefetch(ctx context.Context, v view, addon installed, read func(context.Context)) {
	if !s.readAhead || v.restricted() || keptOnly(ctx) || whole(ctx) || addon.addon.IPTV() {
		return
	}
	if health, ok := s.client.Health(addon.addon.ManifestURL); ok && health.Failure != "" {
		return
	}
	select {
	case s.prefetches <- struct{}{}:
	default:
		return
	}
	ctx = context.WithoutCancel(ctx)
	go func() {
		defer func() { <-s.prefetches }()
		read(ctx)
	}()
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

// collectionChildren lists an addon's collection: the titles of the
// catalogs it groups and of those the collections within it group, merged
// one of each in turn (see collectionSources), never those collections
// themselves, which apps would show as folders of folders. A collection
// that groups other collections, listed whole, is ordered by release date,
// as Jellyfin orders a collection: titles merged from several collections,
// a genre's movies and series or a franchise's sagas, have no other common
// order. A collection that only groups catalogs keeps their order, such as
// their titles' popularity.
func (s *Service) collectionChildren(ctx context.Context, v view, r record, start, count int, first bool) (Page, error) {
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
	// A collection lists at most the items of the library it belongs to.
	most := s.collectionMax(ctx, v, r)
	count = withinMax(start, count, most)
	sources, nested := s.collectionSources(ctx, addon, meta, most, first)
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
		item, rec, err := title.title(r.ID)
		if err != nil {
			continue
		}
		items, records = append(items, item), append(records, rec)
	}
	result := page(items, start, totalWithinMax(total, most))
	if nested && start == 0 && !result.More {
		slices.SortStableFunc(result.Items, func(a, b Item) int { return releaseOf(a).Compare(releaseOf(b)) })
	}
	if result.More && !first {
		s.prefetch(ctx, v, addon, func(ctx context.Context) { _, _, _ = s.merged(ctx, v, sources, start+count, count) })
	}
	return result, s.save(ctx, records)
}

// releaseOf is when a title came out, as Jellyfin orders a collection: its
// premiere date, else the start of its year; the earliest for neither.
func releaseOf(item Item) time.Time {
	switch {
	case item.PremiereDate != nil:
		return *item.PremiereDate
	case item.ProductionYear > 0:
		return time.Date(item.ProductionYear, time.January, 1, 0, 0, 0, 0, time.UTC)
	}
	return time.Time{}
}

// collectionSources lists, each once, the catalogs an addon's collection
// groups, then those the collections within it group, following them up to
// maxNesting deep, and tells whether it followed any. first reads each
// catalog's first page only. A collection within that cannot be described
// is left out; descriptions are cached.
func (s *Service) collectionSources(ctx context.Context, addon installed, meta stremio.Meta, most int, first bool) (sources []source, nested bool) {
	seen := map[catalogKey]bool{}
	described := map[string]bool{meta.ID: true}
	level := []stremio.Meta{meta}
	for depth := 0; len(level) > 0 && depth < maxNesting; depth++ {
		var within []string
		for _, m := range level {
			if m.Collection == nil {
				continue
			}
			for _, ref := range m.Collection.Sources {
				key := catalogKey{addon.addon.ID, ref.Type, ref.CatalogID, ref.Genre}
				if catalog, ok := addon.addon.Manifest.Catalog(ref.Type, ref.CatalogID); ok && !seen[key] {
					seen[key] = true
					sources = append(sources, source{addon: addon, catalog: catalog, genre: ref.Genre, first: first, max: most})
				}
			}
			for _, sub := range m.Collection.Items {
				if sub.ID != "" && !described[sub.ID] {
					described[sub.ID] = true
					within = append(within, sub.ID)
				}
			}
		}
		nested = nested || len(within) > 0
		level = make([]stremio.Meta, len(within))
		var group errgroup.Group
		group.SetLimit(metaFetches)
		for i, id := range within {
			group.Go(func() error {
				m, err := s.meta(ctx, addon, "collection", id)
				if err != nil && ctx.Err() == nil {
					s.logger.Debug("A collection could not be described", "collection", id, "error", err)
				}
				level[i] = m
				return nil
			})
		}
		_ = group.Wait()
	}
	return sources, nested
}

// collectionMax is the most items a collection lists: the maximum of the
// library it was listed in, through the collections it was listed in, 0
// for none.
func (s *Service) collectionMax(ctx context.Context, v view, r record) int {
	parent := r.Parent
	for depth := 0; parent != nil && depth <= maxNesting; depth++ {
		if l, ok := v.library(*parent); ok {
			return l.maxItems
		}
		up, err := s.load(ctx, *parent)
		if err != nil || up.Kind != KindCollection {
			return 0
		}
		parent = up.Parent
	}
	return 0
}

const (
	// WholeListing is how many items a listing of a library or a
	// collection that asks for no limit gets at most, as jellyfin-web's
	// collection pages ask for every title in one request.
	WholeListing = 500
	// wholeListingWait bounds how long such a listing waits for addons
	// (see Whole).
	wholeListingWait = 8 * time.Second
)

// wholeKey marks the context of a whole listing, which reads no page
// ahead: apps ask for no next page.
type wholeKey struct{}

func whole(ctx context.Context) bool {
	marked, _ := ctx.Value(wholeKey{}).(bool)
	return marked
}

// Whole lists the children of parent from start for a listing that asks
// for no limit, as a listing of count does but for a library or a
// collection: up to WholeListing items, or the library's maximum when
// lower (see listingLimit). Their first read may take long, catalogs
// being read a page at a time: it waits for addons at most wholeListingWait,
// then lists the items read and kept so far, while the reads go on and
// keep their pages for the next listing. It never lists fewer than a
// listing of count would.
func (s *Service) Whole(ctx context.Context, user accounts.User, parent accounts.ID, start, count int, genre string) (Page, error) {
	most, err := s.listingLimit(ctx, user, parent)
	if err != nil {
		return Page{}, err
	}
	// Other folders list a page, and a library's maximum bounds its own
	// listings (see withinMax).
	if most <= count {
		return s.Children(ctx, user, parent, start, count, genre)
	}
	wait, cancel := context.WithTimeout(context.WithValue(ctx, wholeKey{}, true), s.wholeWait)
	page, err := s.Children(wait, user, parent, start, most, genre)
	late := wait.Err() != nil
	cancel()
	// A late listing may have left out what it did not wait for: a
	// collection's catalogs that had not answered.
	if err == nil && !late {
		return page, nil
	}
	if ctx.Err() != nil {
		return Page{}, ctx.Err()
	}
	known, err := s.Children(withKeptOnly(ctx), user, parent, start, most, genre)
	if err == nil && (len(known.Items) >= count || !known.More) {
		return known, nil
	}
	return s.Children(ctx, user, parent, start, count, genre)
}

// listingLimit is how many items a listing of parent that asks for no
// limit gets: WholeListing, or the maximum of the library the library or
// collection parent belongs to when lower; 0 for any other parent.
func (s *Service) listingLimit(ctx context.Context, user accounts.User, parent accounts.ID) (int, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return 0, err
	}
	most := 0
	if l, ok := v.library(parent); ok {
		most = l.maxItems
	} else {
		r, err := s.load(ctx, parent)
		if errors.Is(err, ErrNotFound) {
			return 0, nil
		}
		if err != nil {
			return 0, err
		}
		if r.Kind != KindCollection {
			return 0, nil
		}
		most = s.collectionMax(ctx, v, r)
	}
	if most > 0 && most < WholeListing {
		return most, nil
	}
	return WholeListing, nil
}

// fetchMeta asks an addon, or an IPTV source, for its description of a
// title.
func (s *Service) fetchMeta(ctx context.Context, addon installed, metaType, id string) (stremio.Meta, error) {
	if addon.addon.Stremio() {
		return s.client.Meta(ctx, addon.addon.ManifestURL, metaType, id, addon.confined)
	}
	if s.iptv == nil {
		return stremio.Meta{}, stremio.ErrNotFound
	}
	return s.iptv.Meta(ctx, addon.addon.ID, id)
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
		kept, err := s.keptMeta(ctx, candidate, r.Meta.Type, r.Meta.ID)
		if err == nil {
			meta := kept.value
			if candidate.addon.IPTV() {
				meta = s.enrich(ctx, v, candidate, meta, shared)
			}
			if candidate.shared {
				s.learnTraits(ctx, r, meta, kept.at)
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
	item, err := s.item(ctx, v, id)
	if err != nil {
		return Item{}, err
	}
	return s.overriddenItem(item), nil
}

// Original describes one item as its addon does, without what
// administrators changed of it: what their edits are made against (see
// SaveOverrides).
func (s *Service) Original(ctx context.Context, user accounts.User, id accounts.ID) (Item, error) {
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
	return s.ItemsOf(ctx, user, ids, nil)
}

// ItemsOf is Items for the items of the given kinds only, any kind when
// kinds is nil: the others are left out before anything is described.
func (s *Service) ItemsOf(ctx context.Context, user accounts.User, ids []accounts.ID, kinds []Kind) ([]Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	// The records are read together; libraries and identifiers without
	// one are described one by one.
	records, err := s.loadAll(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[accounts.ID]record, len(records))
	for _, r := range records {
		byID[r.ID] = r
	}
	found := make([]*Item, len(ids))
	var g errgroup.Group
	g.SetLimit(8)
	for i, id := range ids {
		r, recorded := byID[id]
		_, isLibrary := v.library(id)
		switch {
		case kinds == nil:
		case isLibrary && !slices.Contains(kinds, KindLibrary), recorded && !isLibrary && !slices.Contains(kinds, r.Kind):
			continue
		}
		g.Go(func() error {
			var item Item
			var err error
			if recorded && !isLibrary {
				item, err = s.described(ctx, v, r)
			} else {
				item, err = s.item(ctx, v, id)
			}
			if err != nil {
				if !errors.Is(err, ErrNotFound) {
					s.logger.Debug("An item could not be described", "item", id, "error", err)
				}
				return nil
			}
			if kinds == nil || slices.Contains(kinds, item.Kind) {
				found[i] = &item
			}
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
	return s.overridden(items), nil
}

func (s *Service) item(ctx context.Context, v view, id accounts.ID) (Item, error) {
	if l, ok := v.library(id); ok {
		item := l.item
		if l.image == addons.LibraryImageAutomatic && !v.restricted() {
			item.Images.Primary = s.automaticImage(ctx, l.catalogSource(), func() string {
				r, err := s.load(ctx, id)
				if err != nil {
					return ""
				}
				return r.Poster
			})
		}
		return item, nil
	}
	r, err := s.load(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return s.unsavedProgram(ctx, v, id)
	}
	if err != nil {
		return Item{}, err
	}
	return s.described(ctx, v, r)
}

// described describes the item Polyfin recorded as r.
func (s *Service) described(ctx context.Context, v view, r record) (Item, error) {
	id := r.ID
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
		// description gives the rating and genres.
		t := describedTraits(meta)
		if !described {
			t, _ = s.knownTraits(v, r)
		}
		// Jellyfin answers a title the user may not see as one that does not
		// exist.
		if !v.allows(r.Kind, s.overriddenTraits(id, t)) {
			return Item{}, ErrNotFound
		}
		item := Item{ID: id, Kind: r.Kind, ParentID: deref(r.Parent), Available: true}
		fromMeta(&item, meta)
		item.OfficialRating = t.rating
		if r.Kind == KindSeries && len(meta.Videos) > 0 {
			item.Contents = contents(meta, nil, s.now())
		}
		// Apps show the credited people and open them by identifier: those
		// of an addon's description are recorded when it is fetched (see
		// fetchKeptMeta), those of an IPTV source's here, as it describes
		// its titles anew each time.
		if r.Addon != nil {
			if entry, ok := v.addon(*r.Addon); ok && entry.addon.IPTV() {
				return item, s.saveCredits(ctx, id, item.People, r.Confined)
			}
		}
		return item, nil
	case KindSeason, KindEpisode:
		series, meta, err := s.series(ctx, v, r.seriesItemID())
		if err != nil {
			return Item{}, err
		}
		if r.Kind == KindSeason {
			for _, season := range seasons(series, meta, seasonPosters(meta), s.words(), s.now()) {
				if season.ID == id {
					return season, nil
				}
			}
		} else {
			for _, episode := range episodes(series, meta, seasonPosters(meta), nil, s.words(), s.now()) {
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
		return s.channel(ctx, v, r)
	case KindProgram:
		return s.program(ctx, v, r)
	case KindArtist, KindAlbum, KindTrack, KindAudiobook, KindMusicPlaylist:
		return s.musicDetails(ctx, v, r)
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
	// A series' seasons and episodes need its details: they open it.
	ctx = iptv.Opening(ctx)
	r, err := s.load(ctx, id)
	if err != nil || r.Kind != KindSeries {
		return Item{}, stremio.Meta{}, ErrNotFound
	}
	meta, ok := s.titleMeta(ctx, v, r)
	if !ok || !v.allows(KindSeries, s.overriddenTraits(id, describedTraits(meta))) {
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

// seasonPosters maps a series' season numbers to the seasons' own artwork,
// for those the addon gives one: by number, or in its list of season
// posters, which names no season. Metadata addons build that list from
// their database's seasons in order, specials (season 0) included or not,
// one image or null each. It is read only when it matches the seasons the
// videos name: as many, mapped in order of number, or one fewer while
// there are specials, mapped to the seasons but those. Any other length
// tells seasons the videos do not have, or lack ones they have, and which
// would be which cannot be told: the list is then left unread rather than
// show a season another's poster.
func seasonPosters(meta stremio.Meta) map[int]string {
	if meta.Extras == nil {
		return nil
	}
	posters := map[int]string{}
	numbers := seasonNumbers(meta)
	listed := meta.Extras.SeasonPosters
	if specials := slices.Index(numbers, 0); len(listed) == len(numbers)-1 && specials >= 0 {
		numbers = slices.Delete(numbers, specials, specials+1)
	}
	if len(listed) == len(numbers) {
		for i, number := range numbers {
			if listed[i] != "" {
				posters[number] = listed[i]
			}
		}
	}
	for number, poster := range meta.Extras.SeasonPosterByNumber {
		if n, err := strconv.Atoi(number); err == nil && poster != "" {
			posters[n] = poster
		}
	}
	return posters
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

// seasons lists a series' seasons; posters are their own artwork (see
// seasonPosters).
func seasons(series Item, meta stremio.Meta, posters map[int]string, w words, now time.Time) []Item {
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
		if poster := posters[number]; poster != "" {
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

func episodes(series Item, meta stremio.Meta, posters map[int]string, season *int, w words, now time.Time) []Item {
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
			SeasonPoster:      posters[number],
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
	posters := seasonPosters(meta)
	result := seasons(series, meta, posters, s.words(), s.now())
	records := make([]record, 0, len(result))
	for _, season := range result {
		records = append(records, record{ID: season.ID, Key: seasonKey(meta.ID, season.IndexNumber), Kind: KindSeason,
			Parent: &series.ID, SeriesID: meta.ID, Season: season.IndexNumber, Poster: posters[season.IndexNumber]})
	}
	return s.overridden(result), s.save(ctx, records)
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
	posters := seasonPosters(meta)
	result := episodes(series, meta, posters, season, s.words(), s.now())
	records := make([]record, 0, len(result)+len(seasonNumbers(meta)))
	for _, number := range seasonNumbers(meta) {
		records = append(records, record{ID: itemID(seasonKey(meta.ID, number)), Key: seasonKey(meta.ID, number), Kind: KindSeason,
			Parent: &series.ID, SeriesID: meta.ID, Season: number, Poster: posters[number]})
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
	return s.overridden(result), s.save(ctx, records)
}

// Search looks a term up in the search catalogs of the user's addons, for
// titles of the given kinds, then among the titles administrators renamed.
func (s *Service) Search(ctx context.Context, user accounts.User, term string, kinds []Kind, limit int) ([]Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	items, err := s.search(ctx, v, term, kinds, limit)
	if err != nil {
		return nil, err
	}
	items = append(items, s.renamed(ctx, v, term, kinds, items, limit)...)
	return s.overridden(items), nil
}

// renamed returns the items of the given kinds an administrator renamed so
// that term matches their name, which the addons' searches know by another
// name: those of the user's addons, not already found, until there are
// limit items.
func (s *Service) renamed(ctx context.Context, v view, term string, kinds []Kind, found []Item, limit int) []Item {
	var result []Item
	for _, id := range s.renamedMatches(term) {
		if len(found)+len(result) >= limit {
			break
		}
		if slices.ContainsFunc(found, func(item Item) bool { return item.ID == id }) {
			continue
		}
		r, err := s.load(ctx, id)
		if err != nil || !slices.Contains(kinds, r.Kind) || r.Addon == nil {
			continue
		}
		if _, ok := v.addon(*r.Addon); !ok {
			continue
		}
		if item, err := s.item(ctx, v, id); err == nil {
			result = append(result, item)
		}
	}
	return result
}

// searchLimit bounds the titles a search lists, whatever the app asks:
// search catalogs are read for their first page only, as Stremio apps
// read them.
const searchLimit = 100

// peopleGrace is how long a search waits for its people-search catalogs
// (see searchesPeople) once its title searches answered: people searches
// are often much slower, and their titles mostly come up in the title
// searches too.
const peopleGrace = 300 * time.Millisecond

func (s *Service) search(ctx context.Context, v view, term string, kinds []Kind, limit int) ([]Item, error) {
	var sources []source
	for _, entry := range v.addons {
		for _, catalog := range entry.addon.Manifest.Catalogs {
			if kind, ok := titleKind(catalog.Type); ok && slices.Contains(kinds, kind) && searchable(catalog) {
				sources = append(sources, source{addon: entry, catalog: catalog, search: term, first: true})
			}
		}
	}
	if len(sources) == 0 {
		return nil, nil
	}
	titles := s.searched(ctx, v, sources, min(limit, searchLimit))
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	titles = titles[:min(len(titles), limit, searchLimit)]
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

// searched reads the first page of each search catalog, together, and
// interleaves their titles as merged does. It answers once the title
// searches did, waiting peopleGrace more for the people searches, and for
// all of them when there are only people searches. Searches still running
// then answer for the next request: their pages are kept (see page).
func (s *Service) searched(ctx context.Context, v view, sources []source, need int) []listed {
	lists := make([][]stremio.Meta, len(sources))
	var mu sync.Mutex
	var titleSearches, all sync.WaitGroup
	fetches := make(chan struct{}, catalogFetches)
	people, titles := false, 0
	for i, src := range sources {
		byPeople := searchesPeople(src.catalog)
		people = people || byPeople
		all.Add(1)
		if !byPeople {
			titleSearches.Add(1)
			titles++
		}
		go func() {
			defer all.Done()
			if !byPeople {
				defer titleSearches.Done()
			}
			fetches <- struct{}{}
			defer func() { <-fetches }()
			metas, _, err := s.window(ctx, v, src, 0, need)
			// An app that stops waiting cancels ctx: nothing failed.
			if err != nil && ctx.Err() == nil {
				s.logger.Warn("A search catalog could not be read", "catalog", src.catalog.ID, "error", err)
			}
			mu.Lock()
			defer mu.Unlock()
			lists[i] = metas
		}()
	}
	everything := done(&all)
	if people && titles > 0 {
		select {
		case <-done(&titleSearches):
			select {
			case <-everything:
			case <-time.After(peopleGrace):
			case <-ctx.Done():
			}
		case <-everything:
		case <-ctx.Done():
		}
	} else {
		select {
		case <-everything:
		case <-ctx.Done():
		}
	}
	mu.Lock()
	defer mu.Unlock()
	return interleave(sources, slices.Clone(lists))
}

// done is closed once group is done.
func done(group *sync.WaitGroup) <-chan struct{} {
	closed := make(chan struct{})
	go func() {
		group.Wait()
		close(closed)
	}()
	return closed
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

// Genres lists the genres a library's catalog can be narrowed to: only its
// own when the library is narrowed to one.
func (s *Service) Genres(ctx context.Context, user accounts.User, libraryID accounts.ID) ([]string, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	l, ok := v.library(libraryID)
	if !ok {
		return nil, ErrNotFound
	}
	if l.genre != "" {
		return []string{l.genre}, nil
	}
	return slices.Clone(addons.Genres(l.catalog)), nil
}

// Artwork returns the URL of an item's image and whether downloading it is
// confined to public addresses: the artwork an administrator uploaded, else
// the addon's (see Uploaded). It needs no user: Jellyfin apps load images
// without credentials, and artwork URLs carry no secret.
func (s *Service) Artwork(ctx context.Context, id accounts.ID, imageType string) (string, bool, error) {
	if url, ok := s.UploadedArtwork(id, imageType); ok {
		return url, false, nil
	}
	r, err := s.load(ctx, id)
	// An IPTV channel shows its line-up's logo at once, listed or not: the
	// admin app shows those of hidden channels too.
	if s.iptv != nil && strings.EqualFold(imageType, "Primary") && (errors.Is(err, ErrNotFound) || err == nil && r.Kind == KindChannel &&
		r.Meta != nil && strings.HasPrefix(r.Meta.ID, iptv.IDPrefix)) {
		logo, confined, lineupErr := s.iptv.Logo(ctx, id)
		switch {
		case lineupErr == nil && logo == "":
			return "", false, ErrNotFound
		case lineupErr == nil:
			s.iptvHosts.logo(logo)
			return logo, confined, nil
		case !errors.Is(lineupErr, stremio.ErrNotFound):
			return "", false, lineupErr
		}
	}
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
	case r.Kind == KindLibrary:
		// A library's automatic image, when it was last looked up.
		images.Primary = r.Poster
	case r.Music != nil:
		images.Primary = r.Music.Artwork
	case r.Kind == KindSeason:
		series, err := s.load(ctx, r.seriesItemID())
		if err != nil || series.Meta == nil {
			return "", false, ErrNotFound
		}
		images = Images{Primary: series.Meta.Poster, Backdrop: series.Meta.Background}
		// A season shows its series' artwork, uploaded or not, unless it
		// has its own.
		for _, imageType := range []string{"Primary", "Backdrop"} {
			if url, ok := s.UploadedArtwork(series.ID, imageType); ok {
				setImage(&images, imageType, url)
			}
		}
		// The season's own artwork is read from its series' complete
		// metadata while it is cached, else as its last listing recorded
		// it: the image apps were given its tag for, without asking the
		// addon again.
		poster := r.Poster
		if full, ok := s.cachedMeta(series); ok {
			poster = seasonPosters(full)[r.Season]
		}
		if poster != "" {
			images.Primary = poster
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
	kept, ok := s.metas.Get(metaKey{*r.Addon, r.Meta.Type, r.Meta.ID})
	return kept.value, ok
}
