package library

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
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
	"github.com/moodiness/polyfin/internal/anime"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/eclipse"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/ratings"
	"github.com/moodiness/polyfin/internal/smartlists"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
	"github.com/moodiness/polyfin/internal/userdata"
	"github.com/moodiness/polyfin/internal/watchlist"
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

// AddonError reports a failure of the addon, IPTV source or local folder a
// library asked: its name, the catalog asked, empty for another resource,
// and the error, whose text it keeps. Browsing logs who failed with it.
type AddonError struct {
	Addon   string
	Catalog string
	Err     error
}

func (e *AddonError) Error() string { return e.Err.Error() }

func (e *AddonError) Unwrap() error { return e.Err }

// failedAt names the addon of entry, and catalog, in err, a failure of a
// request to it (see AddonError): nil stays nil, and an error naming an
// addon already stays as it is.
func failedAt(entry installed, catalog string, err error) error {
	if err == nil {
		return nil
	}
	if _, named := errors.AsType[*AddonError](err); named {
		return err
	}
	return &AddonError{Addon: entry.addon.Manifest.Name, Catalog: catalog, Err: err}
}

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
	// joined, when tests set it, is called with the key of each call to
	// shared once it waits on the key's fetch: the one it started, or the
	// one under way it shares.
	joined func(key string)
	// libraryImages are the libraries' automatic images, by catalog (see
	// automaticImage).
	libraryImages *cache.Cache[pageKey, fetched[foundImage]]
	sizes         pageSizes
	// generated remembers what the generated images of collections and
	// libraries show (see generated.go).
	generated generatedImages
	// prefetches holds a place for each listing read ahead (see
	// readAhead); readAhead turns reading ahead on.
	prefetches chan struct{}
	readAhead  bool
	// translations holds a place for each listing whose descriptions in a
	// user's language are read in the background (see translate).
	translations chan struct{}
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
	// wholeWait bounds how long a whole listing waits for addons, and
	// keptWait how long one does when what was kept fills it (see Whole).
	wholeWait time.Duration
	keptWait  time.Duration
	// searchGrace and searchBound bound how long a search waits for its
	// title searches, and lateSearches are the search catalogs it leaves
	// out, as they were late (see searchdeadline.go).
	searchGrace  time.Duration
	searchBound  time.Duration
	lateSearches lateSearches
	// catalogWaits are the waits hosts asked for, catalogSlots bound the
	// catalog pages asked of each host at once, and failedRefreshes are
	// the pages whose refresh failed lately (see catalogwaits.go).
	catalogWaits    catalogWaits
	catalogSlots    *addonSlots[string]
	failedRefreshes *cache.Cache[pageKey, time.Time]
	// guideDir is where XMLTV guides in ZIP archives are spooled while they
	// are read (see SpoolGuidesIn); empty, they are not read.
	guideDir string
	// guides is what the refreshes of the guides remember between runs
	// (see guideState).
	guides guideState
	// iptv answers for Polyfin's own IPTV sources (see UseIPTV).
	iptv IPTV
	// local answers for Polyfin's own local folders (see UseLocal).
	local Local
	// anime maps anime between identifiers and numberings, for the stream
	// addons that accept other identifiers than a title's (see
	// streamids.go); nil, it maps nothing.
	anime *anime.Service
	// refreshes are the refreshes of libraries' descriptions (see
	// libraryrefresh.go).
	refreshes libraryRefreshes
	// watchlist holds the users' watchlists, which the Watchlist view
	// lists (see watchlist.go).
	watchlist *watchlist.Store
	// ratings looks movies' and series' ratings up on MDBList (see
	// ratings.go); nil, they show their addons'.
	ratings *ratings.Service
	// smart holds the smart lists; smartCache keeps the titles read for
	// each list, by the catalogs that read them and within smartBytes,
	// smartFetches holds a place for each description read for them, and
	// smartWait bounds how long a listing waits for their catalogs (see
	// smartlists.go).
	smart        *smartlists.Store
	smartCache   *cache.Cache[smartKey, []smartTitle]
	smartFetches chan struct{}
	smartWait    time.Duration
	// userData tells the users' state of titles, which smart lists judge
	// by, and holds the titles each user hid, which their listings leave
	// out (see hidden.go).
	userData *userdata.Store

	// music asks Eclipse addons for their resources, which musicCache
	// keeps (see musicCaches).
	music      *eclipse.Client
	musicCache musicCaches

	// overrides are administrators' edits of items, loaded on first use
	// (see overridesNow); overridesMu orders their loading and changes.
	overrides   atomic.Pointer[overrideSet]
	overridesMu sync.Mutex

	// warmed tells when Warm read the libraries' first pages, and opened
	// the users' own libraries opened lately (see warmedposters.go).
	warmed warmedPosters

	// tmdb looks people up, and personLookups are the people being
	// looked up (see persondetails.go).
	tmdb          *tmdb.Client
	personLookups personLookups
}

// IPTV answers, for Polyfin's own IPTV sources, the catalog, meta and
// stream requests addons answer over HTTP (see package iptv), and which
// channels have an archive of past programmes (see replay.go).
type IPTV interface {
	Channels(ctx context.Context, source accounts.ID) ([]stremio.Meta, error)
	Catalog(ctx context.Context, source accounts.ID, catalogType, catalogID string, skip int, genre, search string) ([]stremio.Meta, error)
	CatalogWindow(ctx context.Context, source accounts.ID, catalogType, catalogID, genre, search string, start, count int) ([]stremio.Meta, int, error)
	Enrichment(ctx context.Context, source accounts.ID) bool
	Meta(ctx context.Context, source accounts.ID, id string) (stremio.Meta, error)
	Streams(ctx context.Context, source accounts.ID, id string) ([]stremio.Stream, error)
	MappingChannels(ctx context.Context, source accounts.ID) ([]stremio.Meta, error)
	Logo(ctx context.Context, item accounts.ID) (string, bool, error)
	Archives(ctx context.Context, source accounts.ID) (map[string]int, error)
	GuideArchives(ctx context.Context, guide accounts.ID) (map[string]int, error)
}

// UseIPTV sets what answers for IPTV sources; it is called before the
// service is used. Without it, IPTV sources list nothing.
func (s *Service) UseIPTV(iptv IPTV) { s.iptv = iptv }

// Local answers, for Polyfin's own local folders, the catalog, meta and
// stream requests addons answer over HTTP (see package localfiles); for
// its music folders, those Eclipse addons answer: the pages of their rows,
// album, artist and playlist pages, searches and streams, and also their
// album artists, their playlists and the lyrics of their tracks. Artwork
// is the artwork its movies and shows folders hold for titles, seasons and
// episodes, by item (see TitleItemID, SeasonItemID and EpisodeItemID); it
// is read from memory, for every item listed, and never changed by its
// callers. Open opens one of those images by its key (see
// LocalArtworkKey).
type Local interface {
	Catalog(ctx context.Context, source accounts.ID, catalogType, catalogID string, skip int, search string) ([]stremio.Meta, error)
	Meta(ctx context.Context, source accounts.ID, id string) (stremio.Meta, error)
	Streams(ctx context.Context, source accounts.ID, id string) ([]stremio.Stream, error)
	MusicCatalog(ctx context.Context, source accounts.ID, catalog eclipse.Catalog, skip int) ([]eclipse.Item, int, error)
	MusicAlbum(ctx context.Context, source accounts.ID, id string) (eclipse.Album, error)
	MusicArtist(ctx context.Context, source accounts.ID, id string) (eclipse.Artist, error)
	MusicArtists(ctx context.Context, source accounts.ID) ([]eclipse.Artist, error)
	MusicPlaylist(ctx context.Context, source accounts.ID, id string) (eclipse.Playlist, error)
	MusicPlaylists(ctx context.Context, source accounts.ID) ([]eclipse.Playlist, error)
	MusicSearch(ctx context.Context, source accounts.ID, term string) (eclipse.Results, error)
	MusicStream(ctx context.Context, source accounts.ID, id string) (eclipse.Stream, error)
	MusicLyrics(ctx context.Context, source accounts.ID, id string) (string, error)
	Artwork() map[accounts.ID]Images
	Open(ctx context.Context, key string) (io.ReadSeekCloser, time.Time, error)
	// HeldVideos reports which of the home videos and plain videos of the
	// Stremio identifiers ids a folder holds now.
	HeldVideos(ctx context.Context, ids []string) (map[string]bool, error)
	// Extras lists the extras of folders that belong to owner, and Extra
	// describes one by its item, ErrNotFound when no folder holds it (see
	// extras.go).
	Extras(ctx context.Context, folders []accounts.ID, owner ExtraOwner) ([]Extra, error)
	Extra(ctx context.Context, id accounts.ID) (Extra, error)
	// LinkPlain links the unit of a plain video still listed, by its
	// Stremio identifier, to an IMDb identifier, as the folder's page
	// links it; ErrNotFound when no folder lists it. PlainMatch tells the
	// folder and the Stremio identifier of the title a plain video's unit
	// was matched to since, empty while it is not.
	LinkPlain(ctx context.Context, plain, imdb string) error
	PlainMatch(ctx context.Context, plain string) (accounts.ID, string, error)
	// Episodes lists the episodes the files of folders hold for the
	// series of a Stremio identifier, each named after its file, and
	// NoteUnlisted records those of them a metadata addon's description
	// of the series does not list, by season and number, which the
	// folders' pages count (see fileepisodes.go).
	Episodes(ctx context.Context, folders []accounts.ID, stremioID string) ([]stremio.Video, error)
	NoteUnlisted(ctx context.Context, stremioID string, unlisted [][2]int) error
}

// UseLocal sets what answers for local folders; it is called before the
// service is used. Without it, local folders list nothing.
func (s *Service) UseLocal(local Local) { s.local = local }

// own reports whether an addon is one of Polyfin's own sources, an IPTV
// source or a local folder, whose answers are read from the database:
// they ask no addon, and are not kept.
func own(addon addons.Addon) bool { return addon.IPTV() || addon.Local() }

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
		keptWait:       keptListingWait,
		searchGrace:    searchGrace,
		searchBound:    searchBound,
		music:          eclipse.NewClient(client),
		watchlist:      watchlist.New(db),
		smart:          smartlists.New(db),
		userData:       userdata.New(db),
		smartCache:     cache.New[smartKey, []smartTitle](200, smartLife).Sized(smartBytes, smartTitlesSize),
		smartFetches:   make(chan struct{}, smartFetches),
		smartWait:      smartWait,
		personLookups:  personLookups{slots: make(chan struct{}, personLookupsAtOnce)},
	}
	clock := func() time.Time { return s.now() }
	s.pages = cache.NewLasting[pageKey, fetched[[]stremio.Meta]](4000, s.keptCatalogLife, clock).Sized(64<<20, fetchedSize[[]stremio.Meta])
	s.searchPages = cache.NewLasting[pageKey, fetched[[]stremio.Meta]](500, func() time.Duration { return searchTTL }, clock).
		Sized(16<<20, fetchedSize[[]stremio.Meta])
	s.libraryImages = cache.NewLasting[pageKey, fetched[foundImage]](1000, s.keptCatalogLife, clock)
	s.catalogSlots = &addonSlots[string]{size: catalogHostFetches, slots: map[string]chan struct{}{}}
	s.failedRefreshes = cache.NewLasting[pageKey, time.Time](4000, func() time.Duration { return failedRefreshWait }, clock)
	s.prefetches = make(chan struct{}, prefetchLimit)
	s.translations = make(chan struct{}, translationReaders)
	s.readAhead = true
	s.streamLists = cache.NewLasting[streamKey, list[stremio.Stream]](2000, s.keptListLife, clock)
	s.subtitleLists = cache.NewLasting[streamKey, list[stremio.Subtitle]](2000, s.keptListLife, clock)
	s.musicCache = newMusicCaches(s)
	s.warmed.opened = cache.New[accounts.ID, struct{}](openedLibraries, openedFor)
	s.refreshes.pace, s.refreshes.wait = LibraryRefreshPace, waitFor
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

// installed is an addon a user can use, with how to reach it. shared is
// true for the server's addons, installed by an administrator.
type installed struct {
	addon    addons.Addon
	confined bool
	shared   bool
}

// library is a library of a user with its catalog, the genre it is
// narrowed to and the most items it lists (see addons.Library), how it
// finds its image (see addons.Library.Image), and whether the web player's
// menus leave it out (see addons.Library.HideInMenus). serverName is its
// name in the server's language, which its generated image shows whatever
// the user's language.
type library struct {
	item        Item
	serverName  string
	addon       installed
	catalog     stremio.Catalog
	genre       string
	maxItems    int
	image       string
	hideInMenus bool
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
	// user is the user browsing, zero for a request made with an API key:
	// whose music My music lists (see myMusicRecords).
	user      accounts.ID
	addons    []installed
	libraries []library
	channels  []source
	// smart are the smart lists the user sees, in order (see
	// smartlists.go).
	smart []smartList
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
	// hidden are the titles the user hid, which listings leave out (see
	// hidden.go).
	hidden HiddenTitles
	// catalogLimit and channelLimit are the settings' limits when the
	// request came (see limit).
	catalogLimit int
	channelLimit int
	// language is the user's metadata language when one of the server's
	// addons of the view has it, and translators those addons, which
	// describe the user's titles in it (see languages.go); empty for the
	// server's descriptions. words are the names Polyfin generates for the
	// user.
	language    string
	translators []installed
	words       words
	// peopleLanguage is the language people's biographies are asked of
	// TMDB in (see persondetails.go).
	peopleLanguage string
}

// limit is how many items one read of src's catalog fetches at most: the
// channel limit for a live TV catalog, else the maximum of the library it
// lists for, lower or higher than the catalog limit, else the catalog
// limit. Some catalogs are nearly endless; an IPTV source's movies and
// series, and a local folder's titles, are a stored list, read whole unless
// the library has a maximum.
func (v view) limit(src source) int {
	switch {
	case LiveCatalog(src.catalog.Type):
		return v.channelLimit
	case src.max > 0:
		return src.max
	case own(src.addon.addon):
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
	v := view{user: user.ID, parental: user.Parental, genres: user.BlockedGenres, deadline: time.Now().Add(s.ratingWait), lookups: new(atomic.Int32), held: new(atomic.Bool),
		catalogLimit: settings.CatalogLimit, channelLimit: settings.ChannelLimit}
	if user.ID != (accounts.ID{}) {
		hidden, err := s.hiddenTitles(ctx, user.ID)
		if err != nil {
			return view{}, err
		}
		v.hidden = hidden
	}
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
	v.useLanguage(user.MetadataLanguage, settings.Language)
	v.peopleLanguage = peopleLanguage(user, settings)
	serverNames := LibraryNames(visible, settings.Language)
	for i, name := range LibraryNames(visible, v.words.language) {
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
				LocalFolder:    entries[i].addon.Local(),
			},
			serverName:  serverNames[i],
			addon:       entries[i],
			catalog:     l.Catalog,
			genre:       l.Genre,
			maxItems:    l.MaxItems,
			image:       l.Image,
			hideInMenus: l.HideInMenus,
		})
	}
	if err := s.smartLists(ctx, &v, user, scopes); err != nil {
		return view{}, err
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
// libraryimages.go), then the smart lists they see as libraries.
func (s *Service) Libraries(ctx context.Context, user accounts.User) ([]Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	images := s.automaticImages(ctx, v.libraries)
	items := make([]Item, 0, len(v.libraries)+len(v.smart))
	records := make([]record, 0, len(v.libraries))
	for i, l := range v.libraries {
		item := l.item
		image, shown := images[i], !v.restricted()
		if l.image == addons.LibraryImageGenerated {
			// A library's generated image is wide.
			image.wide = true
			image.url, shown = s.libraryImage(ctx, v, l)
		}
		if shown && image.url != "" {
			item.Images.Primary, item.WidePrimary = image.url, image.wide
		}
		items = append(items, item)
		records = append(records, libraryRecord(l, image.url))
	}
	for _, l := range v.smart {
		if l.item.Kind == KindLibrary {
			items = append(items, l.item)
		}
	}
	return s.overridden(items), s.save(ctx, records)
}

// HiddenInMenus lists those of a user's libraries the web player leaves
// out of its menus (see addons.Library.HideInMenus), in order. It reads
// the view the request built for Libraries, under PerRequest.
func (s *Service) HiddenInMenus(ctx context.Context, user accounts.User) ([]accounts.ID, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	var hidden []accounts.ID
	for _, l := range v.libraries {
		if l.hideInMenus {
			hidden = append(hidden, l.item.ID)
		}
	}
	return hidden, nil
}

// LibraryID is the item of a library in Jellyfin apps, which keeps its
// identifier when its catalog's type changes (see addons.Library.KeyType).
func LibraryID(l addons.Library) accounts.ID {
	return itemID(libraryKey(l.AddonID, cmp.Or(l.KeyType, l.Catalog.Type), l.Catalog.ID))
}

// ServerLibrary is one of the server's libraries, with the genres its
// catalog can be narrowed to, which name its genre pages: only its own
// when it is narrowed to one. CollectionType is the content its catalog
// holds, as collectionType tells it: "movies", "tvshows", "boxsets" for
// collections, "music", or another.
type ServerLibrary struct {
	ID             accounts.ID
	Name           string
	Genres         []string
	CollectionType string
}

// ServerLibraries lists the server's libraries, in order and named in
// language as apps show them: the enabled catalogs of its enabled addons,
// but live TV catalogs, which make no library, then the server's smart
// lists shown as libraries, which offer no genre. A user's apps show those
// the user does not hide, when the user uses the server's addons.
func ServerLibraries(ctx context.Context, store *addons.Store, lists *smartlists.Store, language string) ([]ServerLibrary, error) {
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
		library := ServerLibrary{ID: LibraryID(shown[i]), Name: name, Genres: []string{}, CollectionType: collectionType(shown[i].Catalog.Type)}
		if genre := shown[i].Genre; genre != "" {
			library.Genres = append(library.Genres, genre)
		} else {
			library.Genres = append(library.Genres, addons.Genres(shown[i].Catalog)...)
		}
		result = append(result, library)
	}
	if lists == nil {
		return result, nil
	}
	smart, err := lists.Lists(ctx, nil)
	if err != nil {
		return nil, err
	}
	for _, l := range smart {
		if item := smartItem(l); item.Kind == KindLibrary {
			result = append(result, ServerLibrary{ID: item.ID, Name: item.Name, Genres: []string{}, CollectionType: item.CollectionType})
		}
	}
	return result, nil
}

// ServerLibraries lists the server's libraries (see ServerLibraries).
func (s *Service) ServerLibraries(ctx context.Context) ([]ServerLibrary, error) {
	return ServerLibraries(ctx, s.addons, s.smart, s.settings().Language)
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
// user's parental control hides, and those the user hid, are left out
// before positions are counted. A restricted listing reads at most
// hiddenReach times as far as an unrestricted one, and stops where titles
// wait for their rating: it reports that more may follow, which apps ask
// for later, once the ratings are known. A page the addon cannot give
// ends the window there: it returns the items before it, counted as the
// whole catalog, with errCutShort and why (see cutShort).
func (s *Service) window(ctx context.Context, v view, src source, start, count int) ([]stremio.Meta, int, error) {
	// An IPTV source's movies and series are paged and counted from its
	// stored lists, whole, but for a restricted user, or one who hid
	// titles, whose listing leaves those titles out as it reads.
	if src.addon.addon.IPTV() && !LiveCatalog(src.catalog.Type) && s.iptv != nil && !v.restricted() && len(v.hidden) == 0 && src.date == "" {
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
	var failed error
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
			// Pages a read takes without asking the addon do not count
			// toward the pages asked for at once (see readyPage).
			uncached := 0
			if _, ok := s.readyPage(ctx, src.key(received)); !ok {
				uncached++
			}
			for next := received + size; next < min(received+needed, limit); next += size {
				if kept, ok := s.cachedPage(src.key(offsets[len(offsets)-1])); ok && len(kept) == 0 {
					break
				}
				_, ready := s.readyPage(ctx, src.key(next))
				if !ready && uncached >= pageFetches {
					break
				}
				if !ready {
					uncached++
				}
				offsets = append(offsets, next)
			}
		}
		pages, missing, errs := s.pagesAt(ctx, src, offsets)
		if ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		var fresh []stremio.Meta
		repeated, unknown := false, false
		for i, metas := range pages {
			if offsets[i] != received {
				break
			}
			// A page the addon could not give ends what can be listed.
			if errs[i] != nil {
				failed = errs[i]
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
		if failed != nil {
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
	if failed != nil {
		err := fmt.Errorf("%w: %w", errCutShort, failed)
		if start >= len(collected) {
			return nil, len(collected), err
		}
		return collected[start:min(start+count, len(collected))], len(collected), err
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

// errCutShort reports a window that ends at a page its addon could not
// give (see window).
var errCutShort = errors.New("a catalog page could not be read")

// cutShort reports whether a window's err leaves its items to list: it
// stopped at a page its addon could not give, after items of the window
// were read. Apps then get those items as the whole listing, and the rest
// once the addon gives its pages again.
func cutShort(err error, items int) bool {
	return errors.Is(err, errCutShort) && items > 0
}

// pagesAt fetches the catalog pages starting at each offset together, and
// tells each page's error. The pages kept answer at once: reads bound the
// pages they fetch (see window). missing tells the pages a read that asks
// no addon does not know.
func (s *Service) pagesAt(ctx context.Context, src source, offsets []int) (pages [][]stremio.Meta, missing []bool, errs []error) {
	pages, missing, errs = make([][]stremio.Meta, len(offsets)), make([]bool, len(offsets)), make([]error, len(offsets))
	var wg sync.WaitGroup
	for i, offset := range offsets {
		wg.Go(func() {
			pages[i], errs[i] = s.page(ctx, src, offset)
			if errors.Is(errs[i], errNotKept) {
				missing[i], errs[i] = true, nil
			}
		})
	}
	wg.Wait()
	return pages, missing, errs
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
		if cutShort(err, len(metas)) {
			s.logger.Debug("A listing ends at a catalog page the addon could not give", "catalog", sources[0].catalog.ID, "error", err)
			err = nil
		}
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
		if l.addon.addon.HasMusic() {
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

// children lists a folder's children (see Children), its titles showing
// the user's language as far as it is kept (see translate). An app's
// listing of one of the user's own libraries opens it (see WithOpening).
func (s *Service) children(ctx context.Context, user accounts.User, parent accounts.ID, start, count int, genre string, first bool) (Page, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return Page{}, err
	}
	if l, ok := v.library(parent); ok && opening(ctx) {
		s.openedLibrary(l)
	}
	page, err := s.childrenIn(ctx, v, user, parent, start, count, genre, first)
	s.translate(ctx, v, page.Items)
	return page, err
}

func (s *Service) childrenIn(ctx context.Context, v view, user accounts.User, parent accounts.ID, start, count int, genre string, first bool) (Page, error) {
	if l, ok := v.library(parent); ok {
		return s.libraryChildren(ctx, v, l, start, count, genre, first)
	}
	if l, ok := v.smartList(parent); ok {
		return s.smartChildren(ctx, v, l, start, count)
	}
	if parent == ReplayViewID {
		if _, err := s.replayView(ctx, v); err != nil {
			return Page{}, err
		}
		folders, err := s.replayFolders(ctx, v)
		return slicePage(folders, start, count), err
	}
	if parent == WatchlistViewID {
		return s.watchlistChildren(ctx, v, start, count, first)
	}
	r, err := s.load(ctx, parent)
	if err != nil {
		return Page{}, err
	}
	switch r.Kind {
	case KindCollection:
		return s.collectionChildren(ctx, v, r, start, count, first)
	case KindFolder, KindVideo:
		if !r.otherTitle() {
			return Page{}, ErrNotFound
		}
		return s.folderChildren(ctx, v, parent, start, count)
	case KindSeries:
		seasons, err := s.Seasons(ctx, user, parent)
		return slicePage(seasons, start, count), err
	case KindSeason:
		episodes, err := s.Episodes(ctx, user, r.seriesItemID(), &parent)
		return slicePage(episodes, start, count), err
	case KindReplayFolder:
		programmes, err := s.replayProgrammes(ctx, v, r)
		return slicePage(programmes, start, count), err
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
	if l.addon.addon.HasMusic() {
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
	if l.catalog.Type == "collection" && titlesThrough(ctx) {
		return s.collectionLibraryTitles(ctx, v, l, src, start, count)
	}
	if otherType(l.catalog.Type) && titlesThrough(ctx) {
		return s.otherLibraryVideos(ctx, v, l, src, start, count)
	}
	// A listing that keeps none of what the library lists, its collections
	// or its titles, lists nothing, out of none.
	if kind, ok := listedKind(l.catalog.Type); l.catalog.Type == "collection" && !keepsKind(ctx, KindCollection) ||
		ok && !keepsKind(ctx, kind) {
		return Page{}, nil
	}
	metas, total, err := s.window(ctx, v, src, start, count)
	if cutShort(err, len(metas)) {
		s.logger.Debug("A listing ends at a catalog page the addon could not give", "catalog", src.catalog.ID, "error", err)
	} else if err != nil {
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
	s.shapeOthers(ctx, v, items, records)
	if l.catalog.Type == "collection" {
		s.addGenerated(ctx, v, items, records)
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
	if !s.readAhead || v.restricted() || keptOnly(ctx) || whole(ctx) || own(addon.addon) {
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

// titlesThroughKey marks the context of a listing of the titles under a
// folder, through the folders within it (see WithTitlesThrough).
type titlesThroughKey struct{}

// WithTitlesThrough marks a listing that asks for the titles under a
// folder at any depth, as Jellyfin lists the movies and series within a
// library's folders when asked recursively for them: a collection library
// then lists the titles of its collections rather than its collections
// (see collectionLibraryTitles). Other folders list as they do.
func WithTitlesThrough(ctx context.Context) context.Context {
	return context.WithValue(ctx, titlesThroughKey{}, true)
}

func titlesThrough(ctx context.Context) bool {
	marked, _ := ctx.Value(titlesThroughKey{}).(bool)
	return marked
}

// kindsKey marks the context of a listing that keeps only items of some
// kinds (see WithKinds).
type kindsKey struct{}

// WithKinds marks a listing that keeps only the items whose kind keeps
// reports, as a Jellyfin listing narrowed by item type: a library or a
// collection then reads only the catalogs whose titles it keeps, so that
// its pages and its total count those alone, and a collection library
// lists no collection when collections are not kept. The titles of a
// catalog are told by its type, movies or series (see listedKind); other
// catalogs are read as before.
func WithKinds(ctx context.Context, keeps func(Kind) bool) context.Context {
	return context.WithValue(ctx, kindsKey{}, keeps)
}

// keepsKind reports whether a listing keeps items of kind: always, unless
// it was marked by WithKinds.
func keepsKind(ctx context.Context, kind Kind) bool {
	keeps, ok := ctx.Value(kindsKey{}).(func(Kind) bool)
	return !ok || keeps(kind)
}

// listedKind is the kind of the titles a catalog of a type lists, when its
// type tells it: movies or series. Other catalogs, such as anime ones, may
// list both.
func listedKind(catalogType string) (Kind, bool) {
	switch catalogType {
	case "movie", "anime.movie":
		return KindMovie, true
	case "series", "anime.series":
		return KindSeries, true
	}
	return "", false
}

// keptSources returns the sources whose titles the listing of ctx keeps
// (see WithKinds).
func keptSources(ctx context.Context, sources []source) []source {
	kept := make([]source, 0, len(sources))
	for _, src := range sources {
		if kind, ok := listedKind(src.catalog.Type); !ok || keepsKind(ctx, kind) {
			kept = append(kept, src)
		}
	}
	return kept
}

// collectionLibraryTitles lists the titles of a collection library's
// collections, those its catalog lists as src: the titles of the catalogs
// they group, and of those the collections within them group, merged one
// of each in turn, as a collection lists them (see collectionChildren),
// up to the library's maximum. A read that asks no addon and does not know
// every collection tells that more may follow.
func (s *Service) collectionLibraryTitles(ctx context.Context, v view, l library, src source, start, count int) (Page, error) {
	sources, unknown, err := s.groupedSources(ctx, v, l, src)
	if err != nil {
		return Page{}, err
	}
	sources = keptSources(ctx, sources)
	if len(sources) == 0 {
		return Page{More: unknown}, nil
	}
	titles, total, err := s.merged(ctx, v, sources, start, count)
	if err != nil {
		return Page{}, err
	}
	if unknown && total <= start+len(titles) {
		total = start + len(titles) + 1
	}
	var items []Item
	var records []record
	for _, title := range titles {
		item, r, err := title.title(l.item.ID)
		if err != nil {
			continue
		}
		items, records = append(items, item), append(records, r)
	}
	s.shapeOthers(ctx, v, items, records)
	result := page(items, start, totalWithinMax(total, l.maxItems))
	if result.More && !src.first {
		s.prefetch(ctx, v, l.addon, func(ctx context.Context) { _, _, _ = s.merged(ctx, v, sources, start+count, count) })
	}
	return result, s.save(ctx, records)
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
	// Until its description tells (see shapeOthers), a title of the other
	// or channel type is a folder: one without videos lists itself.
	if otherType(metaType) {
		kind = KindFolder
	}
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
	sources = keptSources(ctx, sources)
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
	s.shapeOthers(ctx, v, items, records)
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
				// A generated image's titles take the descriptions kept
				// (see collectionCandidates).
				if descriptionsKept(ctx) {
					level[i], _ = s.keptDescription(ctx, addon, metaKey{addon.addon.ID, "collection", id}, inDatabase)
					return nil
				}
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
	// wholeListingWait bounds how long such a listing waits for addons,
	// and keptListingWait how long one does when what earlier listings
	// kept fills a listing of count (see Whole).
	wholeListingWait = 3 * time.Second
	keptListingWait  = time.Second
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
// lower (see listingLimit). Catalogs are read a page at a time, and such a
// listing may take many pages. What earlier listings kept answers at once
// when it holds the whole listing. Otherwise the listing waits for addons
// at most wholeListingWait, or keptListingWait when what was kept fills a
// listing of count, then lists what was read and kept by then; when
// nothing came in time, the first page of each catalog, waited for as a
// home row waits for it. What it did not wait for is read on in the
// background, to the end, for the next listing (see readRest).
func (s *Service) Whole(ctx context.Context, user accounts.User, parent accounts.ID, start, count int, genre string) (Page, error) {
	most, atOnce, err := s.listingLimit(ctx, user, parent)
	if err != nil {
		return Page{}, err
	}
	return s.upTo(ctx, user, parent, start, count, most, atOnce, genre)
}

// RandomPool is how many of a library's first titles a listing of them
// at random draws among, at the least (see DrawPool).
const RandomPool = 100

// DrawPool lists the titles a listing of the titles of library parent at
// random draws among: its first pool, at most WholeListing and the
// library's maximum, as a whole listing reads them (see Whole), and so
// with what the user may see only. A collection library gives the titles
// of its collections, as its listings of titles do (see WithTitlesThrough).
// ok is false when parent is not one of the user's libraries, such as a
// smart list, which keeps its own order.
func (s *Service) DrawPool(ctx context.Context, user accounts.User, parent accounts.ID, count, pool int, genre string) (page Page, ok bool, err error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return Page{}, false, err
	}
	l, ok := v.library(parent)
	if !ok {
		return Page{}, false, nil
	}
	most := min(max(pool, RandomPool), WholeListing)
	if l.maxItems > 0 {
		most = min(most, l.maxItems)
	}
	page, err = s.upTo(ctx, user, parent, 0, min(count, most), most, false, genre)
	return page, true, err
}

// upTo lists the children of parent from start for a listing of count as
// Whole does, reading up to most of them; atOnce lists them without
// waiting for addons, for parents that read no catalog.
func (s *Service) upTo(ctx context.Context, user accounts.User, parent accounts.ID, start, count, most int, atOnce bool, genre string) (Page, error) {
	// Other folders list a page, and a library's maximum bounds its own
	// listings (see withinMax).
	if most <= count {
		return s.Children(ctx, user, parent, start, count, genre)
	}
	// The Replay view and its folders, and the Watchlist view, read no
	// catalog: they are listed whole at once.
	if atOnce {
		return s.Children(ctx, user, parent, start, most, genre)
	}
	// From the start, a kept listing that wants no more is the whole
	// listing; from an index, it may only know less than the index.
	known, err := s.Children(withKeptOnly(ctx), user, parent, start, most, genre)
	if err == nil && !known.More && start == 0 {
		return known, nil
	}
	wait := s.wholeWait
	if err == nil && len(known.Items) >= count {
		wait = s.keptWait
	}
	waiting, cancel := context.WithTimeout(context.WithValue(ctx, wholeKey{}, true), wait)
	page, err := s.Children(waiting, user, parent, start, most, genre)
	late := waiting.Err() != nil
	cancel()
	// A late listing may have left out what it did not wait for: a
	// collection's catalogs that had not answered.
	if err == nil && !late {
		return page, nil
	}
	if ctx.Err() != nil {
		return Page{}, ctx.Err()
	}
	known, err = s.Children(withKeptOnly(ctx), user, parent, start, most, genre)
	if err != nil || len(known.Items) == 0 {
		known, err = s.children(ctx, user, parent, start, count, genre, true)
		if err != nil {
			return Page{}, err
		}
		known.Items = s.overridden(known.Items)
	}
	if known.More {
		s.readRest(ctx, user, parent, start, most, genre)
	}
	return known, nil
}

// readRest reads a whole listing of parent, up to most, in the
// background, as prefetch reads a listing's next window and on the same
// terms, so that the next whole listing finds all of it kept (see Whole).
func (s *Service) readRest(ctx context.Context, user accounts.User, parent accounts.ID, start, most int, genre string) {
	v, err := s.view(ctx, user)
	if err != nil {
		return
	}
	addon, ok := s.listedAddon(ctx, v, parent)
	if !ok {
		return
	}
	s.prefetch(ctx, v, addon, func(ctx context.Context) {
		_, _ = s.Children(context.WithValue(ctx, wholeKey{}, true), user, parent, start, most, genre)
	})
}

// listedAddon returns the addon whose catalogs list parent: a library's,
// or a collection's.
func (s *Service) listedAddon(ctx context.Context, v view, parent accounts.ID) (installed, bool) {
	if l, ok := v.library(parent); ok {
		return l.addon, true
	}
	r, err := s.load(ctx, parent)
	if err != nil || r.Addon == nil {
		return installed{}, false
	}
	return v.addon(*r.Addon)
}

// listingLimit is how many items a listing of parent that asks for no
// limit gets: WholeListing, or the maximum of the library the library or
// collection parent belongs to when lower; 0 for any other parent but the
// Replay view and its folders, the Watchlist view and the smart lists,
// which get WholeListing, a smart list's maximum when lower, atOnce set: a
// smart list bounds its own wait (see smartRead).
func (s *Service) listingLimit(ctx context.Context, user accounts.User, parent accounts.ID) (most int, atOnce bool, err error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return 0, false, err
	}
	if l, ok := v.library(parent); ok {
		most = l.maxItems
	} else if parent == ReplayViewID || parent == WatchlistViewID {
		return WholeListing, true, nil
	} else if l, ok := v.smartList(parent); ok {
		if l.list.MaxItems > 0 {
			return min(l.list.MaxItems, WholeListing), true, nil
		}
		return WholeListing, true, nil
	} else {
		r, err := s.load(ctx, parent)
		if errors.Is(err, ErrNotFound) {
			return 0, false, nil
		}
		if err != nil {
			return 0, false, err
		}
		switch r.Kind {
		case KindCollection:
			most = s.collectionMax(ctx, v, r)
		case KindReplayFolder:
			return WholeListing, true, nil
		default:
			return 0, false, nil
		}
	}
	if most > 0 && most < WholeListing {
		return most, false, nil
	}
	return WholeListing, false, nil
}

// fetchMeta asks an addon, an IPTV source or a local folder for its
// description of a title.
func (s *Service) fetchMeta(ctx context.Context, addon installed, metaType, id string) (stremio.Meta, error) {
	switch {
	case addon.addon.Stremio():
		meta, err := s.client.Meta(ctx, addon.addon.ManifestURL, metaType, id, addon.confined)
		withoutGenerated(&meta)
		return meta, failedAt(addon, "", err)
	case addon.addon.Local():
		if s.local == nil {
			return stremio.Meta{}, stremio.ErrNotFound
		}
		return s.local.Meta(ctx, addon.addon.ID, id)
	case s.iptv == nil:
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
// addons, the server's only when shared is set, in the order describers
// gives. A description from one of the server's addons gives the rating
// kept for the title.
func (s *Service) describe(ctx context.Context, v view, r record, shared bool) (stremio.Meta, bool) {
	meta, _, ok := s.describeBy(ctx, v, r, shared)
	return meta, ok
}

// describeBy is describe, which also returns the addon that described the
// title.
func (s *Service) describeBy(ctx context.Context, v view, r record, shared bool) (stremio.Meta, installed, bool) {
	for _, candidate := range describers(v, r, shared) {
		kept, err := s.keptMeta(ctx, candidate, r.Meta.Type, r.Meta.ID)
		if err == nil {
			return s.completed(ctx, v, r, candidate, kept, shared), candidate, true
		}
		if !notOwn(candidate, err) {
			s.logger.Debug("An addon could not describe a title", "addon", candidate.addon.Manifest.Name, "error", err)
		}
	}
	return stremio.Meta{}, installed{}, false
}

// notOwn reports whether err tells that a local folder or an IPTV source
// does not have a title: asked after every addon that may describe it, as
// they describe only their own titles, they do not know most titles, which
// is not worth a log record each time a title opens.
func notOwn(candidate installed, err error) bool {
	return own(candidate.addon) && errors.Is(err, stremio.ErrNotFound)
}

// completed is the description of r that candidate gave, completed as
// describeBy returns it, an IPTV source's with the details of the
// descriptions kept, a series' with the episodes local folders hold that
// it does not list (see withFileEpisodes); one from one of the server's
// addons gives the rating kept for the title.
func (s *Service) completed(ctx context.Context, v view, r record, candidate installed, kept fetched[stremio.Meta], shared bool) stremio.Meta {
	meta := kept.value
	if candidate.addon.IPTV() {
		how := inDatabase
		if iptv.IsOpening(ctx) {
			how = fromAddons
		}
		meta, _ = s.enrich(ctx, v, candidate, meta, shared, how)
	}
	if candidate.shared && !candidate.addon.Local() {
		s.learnTraits(ctx, r, meta, kept.at)
	}
	return s.withFileEpisodes(ctx, v, r, candidate, meta)
}

// describers lists the user's addons, the server's only when shared is
// set, that may describe a title, in the order they are asked: the one
// that listed it first. A local folder comes last, whoever listed the
// title: it describes its titles from what their files tell, for when no
// addon does. An addon given a language is none of them, even when it
// listed the title: it describes titles in its language only, for its
// language's users, unless no addon without a language describes the
// title; then those given a language are, the user's language's first
// (see languages.go).
func describers(v view, r record, shared bool) []installed {
	if r.Meta == nil {
		return nil
	}
	standIn := v.standIn(r.Meta.Type, r.Meta.ID, shared)
	var candidates []installed
	for _, candidate := range v.addons {
		if (!shared || candidate.shared) && (standIn || !translator(candidate)) && serves(candidate, "meta", r.Meta.Type, r.Meta.ID) {
			candidates = append(candidates, candidate)
		}
	}
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
	if standIn {
		v.inLanguageOrder(candidates)
	}
	slices.SortStableFunc(candidates, func(a, b installed) int {
		switch {
		case a.addon.Local() == b.addon.Local():
			return 0
		case b.addon.Local():
			return -1
		}
		return 1
	})
	return candidates
}

// describedMeta returns the complete description describe gives of a
// title, as far as it is known without asking any addon: that of the
// first of describers whose description is kept, in memory, or in the
// database too when how is inDatabase, wherever the title was listed. A
// local folder or an IPTV source describes its titles from its database,
// kept nowhere: the one that listed the title gives what its enrichment
// adds from the kept descriptions to its listing, if anything; another
// local folder gives nothing. It returns the addon that described the
// title. how is inMemory or inDatabase: describedMeta never asks an addon.
func (s *Service) describedMeta(ctx context.Context, v view, r record, shared bool, how reach) (stremio.Meta, installed, bool) {
	for _, candidate := range describers(v, r, shared) {
		switch {
		case !own(candidate.addon):
			if meta, ok := s.keptDescription(ctx, candidate, metaKey{candidate.addon.ID, r.Meta.Type, r.Meta.ID}, how); ok {
				return s.withFileEpisodes(ctx, v, r, candidate, meta), candidate, true
			}
		case r.Addon != nil && candidate.addon.ID == *r.Addon:
			if candidate.addon.IPTV() {
				if meta, ok := s.enrich(ctx, v, candidate, *r.Meta, shared, how); ok {
					return meta, candidate, true
				}
			}
			return stremio.Meta{}, installed{}, false
		}
	}
	return stremio.Meta{}, installed{}, false
}

// TitleMeta returns the description the server's addons give of a title by
// its Stremio type and identifier, from the first of them that describes
// it, Polyfin's own sources left out: local folders name the files linked
// to a title after it.
func (s *Service) TitleMeta(ctx context.Context, metaType, id string) (stremio.Meta, bool) {
	// A view without a user is the server's addons'.
	v, err := s.view(ctx, accounts.User{})
	if err != nil {
		return stremio.Meta{}, false
	}
	return s.serverMeta(ctx, v, metaType, id)
}

// serverMeta returns the description the addons of v, the server's view,
// give of a title, as TitleMeta does: those without a language, else
// those given one (see standIn).
func (s *Service) serverMeta(ctx context.Context, v view, metaType, id string) (stremio.Meta, bool) {
	for _, candidate := range v.addons {
		if own(candidate.addon) || !v.factual(candidate, metaType, id, false) || !serves(candidate, "meta", metaType, id) {
			continue
		}
		if kept, err := s.keptMeta(ctx, candidate, metaType, id); err == nil {
			return kept.value, true
		}
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

// Original describes one item as its addon and MDBList do, without what
// administrators changed of it: what their edits are made against (see
// SaveOverrides).
func (s *Service) Original(ctx context.Context, user accounts.User, id accounts.ID) (Item, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return Item{}, err
	}
	item, err := s.item(ctx, v, id)
	if err != nil {
		return Item{}, err
	}
	return s.rated(item), nil
}

// Items describes the items of ids the user can reach, in order. Items
// that no longer exist, or whose addon fails to describe them, are left
// out, and so are home videos no folder holds now (see goneVideos): their
// users' data stays, for when the video is found again.
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
	items, err := s.itemsOf(ctx, v, ids, kinds)
	return s.overridden(items), err
}

// itemsOf is ItemsOf in v, without administrators' edits.
func (s *Service) itemsOf(ctx context.Context, v view, ids []accounts.ID, kinds []Kind) ([]Item, error) {
	// The items show the user's language as far as it is kept, as
	// listings do (see shown).
	ctx = withListing(ctx)
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
	gone, err := s.goneVideos(ctx, records)
	if err != nil {
		return nil, err
	}
	found := make([]*Item, len(ids))
	var g errgroup.Group
	g.SetLimit(8)
	for i, id := range ids {
		r, recorded := byID[id]
		_, isLibrary := v.library(id)
		switch {
		case gone[id]:
			continue
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
	return items, nil
}

// goneVideos are the items of records that are home videos no folder
// holds now: deleted, or their folder holding something else. Within 30
// days, a video found again is the same title (see localfiles.recognize).
// So are the plain videos no folder lists now, matched or gone, with
// their seasons and episodes: what users did with them went to the title
// they were matched to (see localfiles.follow).
func (s *Service) goneVideos(ctx context.Context, records []record) (map[accounts.ID]bool, error) {
	var videos []string
	held := func(r record) (string, bool) {
		if r.homeVideo() {
			return r.Meta.ID, true
		}
		return r.plainVideo()
	}
	for _, r := range records {
		if id, ok := held(r); ok {
			videos = append(videos, id)
		}
	}
	if len(videos) == 0 {
		return nil, nil
	}
	holding := map[string]bool{}
	if s.local != nil {
		var err error
		if holding, err = s.local.HeldVideos(ctx, videos); err != nil {
			return nil, err
		}
	}
	gone := map[accounts.ID]bool{}
	for _, r := range records {
		if id, ok := held(r); ok && !holding[id] {
			gone[r.ID] = true
		}
	}
	return gone, nil
}

func (s *Service) item(ctx context.Context, v view, id accounts.ID) (Item, error) {
	if l, ok := v.library(id); ok {
		item := l.item
		switch {
		case l.image == addons.LibraryImageAutomatic && !v.restricted():
			image := s.automaticImage(ctx, l.catalogSource(), func() string {
				r, err := s.load(ctx, id)
				if err != nil {
					return ""
				}
				return r.Poster
			})
			if image.url != "" {
				item.Images.Primary, item.WidePrimary = image.url, image.wide
			}
		case l.image == addons.LibraryImageGenerated:
			if image, shown := s.libraryImage(ctx, v, l); shown && image != "" {
				item.Images.Primary, item.WidePrimary = image, true
			}
		}
		return item, nil
	}
	if l, ok := v.smartList(id); ok {
		items := []Item{l.item}
		s.smartCollectionImages(ctx, v, items)
		return items[0], nil
	}
	if id == ReplayViewID {
		return s.replayView(ctx, v)
	}
	if id == WatchlistViewID {
		return s.watchlistView(ctx, v)
	}
	r, err := s.load(ctx, id)
	if errors.Is(err, ErrNotFound) {
		if extra, err := s.extra(ctx, v, id); !errors.Is(err, ErrNotFound) {
			return extra, err
		}
		return s.unsavedProgram(ctx, v, id)
	}
	if err != nil {
		return Item{}, err
	}
	// A plain video matched or gone is no more: its title has its users'
	// data (see goneVideos).
	if _, plain := r.plainVideo(); plain {
		gone, err := s.goneVideos(ctx, []record{r})
		if err != nil {
			return Item{}, err
		}
		if gone[r.ID] {
			return Item{}, ErrNotFound
		}
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
		items := []Item{item}
		s.addGenerated(ctx, v, items, []record{r})
		return items[0], nil
	case KindMovie, KindSeries, KindVideo, KindFolder:
		if r.folderVideo() {
			return s.folderVideoItem(ctx, v, r)
		}
		if r.Meta == nil {
			return Item{}, ErrNotFound
		}
		meta := *r.Meta
		full, described := s.titleMeta(ctx, v, r)
		if described {
			meta = full
		}
		// A restricted user's addons are the server's (see view): the
		// description gives the rating and genres.
		t := describedTraits(meta)
		if !described {
			t, _ = s.knownTraits(ctx, v, r)
		}
		// Jellyfin answers a title the user may not see as one that does not
		// exist.
		kind := r.Kind
		if described && r.otherTitle() {
			kind = otherKind(meta)
		}
		if !v.allows(kind, s.overriddenTraits(id, t)) {
			return Item{}, ErrNotFound
		}
		item := Item{ID: id, Kind: kind, ParentID: deref(r.Parent), Available: true}
		// The user's language changes what the title shows, never the facts
		// judged above.
		fromMeta(&item, s.shown(ctx, v, r.Meta.Type, r.Meta.ID, meta))
		item.OfficialRating = t.rating
		switch {
		case kind == KindSeries && len(meta.Videos) > 0:
			item.Contents = contents(meta, nil, s.now())
		case kind == KindFolder && described:
			item.Contents = folderContents(meta, s.now())
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
		series, meta, _, err := s.series(ctx, v, r.seriesItemID())
		if err != nil {
			return Item{}, err
		}
		if r.Kind == KindSeason {
			for _, season := range seasons(series, meta, seasonPosters(meta), v.words, s.now()) {
				if season.ID == id {
					return season, nil
				}
			}
		} else {
			for _, episode := range episodes(series, meta, seasonPosters(meta), nil, v.words, s.now()) {
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
		return personItem(id, r, v.peopleLanguage), nil
	case KindChannel:
		return s.channel(ctx, v, r)
	case KindProgram:
		return s.program(ctx, v, r)
	case KindReplayFolder:
		return s.replayFolderItem(ctx, v, r)
	case KindReplay:
		return s.replayProgramme(ctx, v, r)
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

// series returns a series item and its complete metadata, with videos, as
// the user of v sees them (see shown), and its description itself, whose
// facts and episodes are those Polyfin records.
func (s *Service) series(ctx context.Context, v view, id accounts.ID) (Item, stremio.Meta, stremio.Meta, error) {
	// A series' seasons and episodes need its details: they open it.
	ctx = iptv.Opening(ctx)
	r, err := s.load(ctx, id)
	if err != nil || r.Kind != KindSeries {
		return Item{}, stremio.Meta{}, stremio.Meta{}, ErrNotFound
	}
	meta, ok := s.titleMeta(ctx, v, r)
	if !ok || !v.allows(KindSeries, s.overriddenTraits(id, describedTraits(meta))) {
		return Item{}, stremio.Meta{}, stremio.Meta{}, ErrNotFound
	}
	shown := s.shown(ctx, v, r.Meta.Type, r.Meta.ID, meta)
	item := Item{ID: id, Kind: KindSeries, ParentID: deref(r.Parent), Available: true}
	fromMeta(&item, shown)
	item.Contents = contents(meta, nil, s.now())
	return item, shown, meta, nil
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
	series, shown, meta, err := s.series(ctx, v, seriesID)
	if err != nil {
		return nil, err
	}
	posters := seasonPosters(meta)
	result := seasons(series, shown, posters, v.words, s.now())
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
	return s.episodes(ctx, v, seriesID, seasonID)
}

// episodes lists a series' episodes for v, of one season when seasonID is
// set, and records them and the series' seasons.
func (s *Service) episodes(ctx context.Context, v view, seriesID accounts.ID, seasonID *accounts.ID) ([]Item, error) {
	series, shown, meta, err := s.series(ctx, v, seriesID)
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
	// The episodes show the user's language; their records keep the
	// description's names and images.
	result := episodes(series, shown, posters, season, v.words, s.now())
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
// name: those of the user's addons, not already found nor hidden by the
// user, until there are limit items.
func (s *Service) renamed(ctx context.Context, v view, term string, kinds []Kind, found []Item, limit int) []Item {
	var result []Item
	for _, id := range s.renamedMatches(term) {
		if len(found)+len(result) >= limit {
			break
		}
		if v.hidden[id] || slices.ContainsFunc(found, func(item Item) bool { return item.ID == id }) {
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

// search looks a term up in the search catalogs of the user's addons: of
// those given a language (see languages.go), the user's translators only,
// whose results show the user's language, unless no addon without a
// language searches titles of a kind: then every translator searches
// them, for every user.
func (s *Service) search(ctx context.Context, v view, term string, kinds []Kind, limit int) ([]Item, error) {
	// The catalogs of folders are those of videos (see titleKind).
	catalogKinds := kinds
	if slices.Contains(kinds, KindFolder) && !slices.Contains(kinds, KindVideo) {
		catalogKinds = append(slices.Clone(kinds), KindVideo)
	}
	sources := standInCatalogs(v, catalogKinds, true, func(entry installed, catalog stremio.Catalog) (source, bool) {
		return source{addon: entry, catalog: catalog, search: term, first: true}, searchable(catalog)
	})
	if len(sources) == 0 {
		return nil, nil
	}
	titles := s.searched(ctx, v, sources, min(limit, searchLimit))
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	titles = titles[:min(len(titles), limit, searchLimit)]
	found := make([]Item, 0, len(titles))
	listed := make([]record, 0, len(titles))
	var translators []bool
	for _, title := range titles {
		item, r, err := title.title(accounts.ID{})
		if err != nil {
			continue
		}
		r.Parent = nil
		found, listed = append(found, item), append(listed, r)
		translators = append(translators, translator(title.src.addon) && !v.standIn(r.Meta.Type, r.Meta.ID, false))
	}
	s.shapeOthers(ctx, v, found, listed)
	var items []Item
	var records, translated []record
	for i, r := range listed {
		if r.otherTitle() && !slices.Contains(kinds, found[i].Kind) {
			continue
		}
		items = append(items, found[i])
		if translators[i] {
			// What a translator found is described in its language: it
			// stands for the title only until another addon lists it, and
			// never replaces what another addon listed.
			preview := *r.Meta
			preview.Videos = nil
			r.Meta = &preview
			translated = append(translated, r)
		} else {
			records = append(records, r)
		}
	}
	s.translate(ctx, v, items)
	return items, errors.Join(s.save(ctx, records), s.saveNew(ctx, translated))
}

// searched reads the first page of each search catalog, together, and
// interleaves their titles as merged does. It answers once the title
// searches did, or at its deadline: searchGrace after one of them found
// its page, searchBound at most. It then waits peopleGrace more for the
// people searches, and only for them, within searchBound, when there are
// only people searches. Searches still running then answer for the next
// request: their pages are kept (see page). The title searches that
// missed the deadline are left out of the searches for a while, and so
// are those already left out (see lateSearch).
func (s *Service) searched(ctx context.Context, v view, sources []source, need int) []listed {
	sources = slices.DeleteFunc(slices.Clone(sources), s.skipsSearch)
	lists := make([][]stremio.Meta, len(sources))
	asked, answered := make([]bool, len(sources)), make([]bool, len(sources))
	var mu sync.Mutex
	var titleSearches, all sync.WaitGroup
	fetches := make(chan struct{}, catalogFetches)
	found := make(chan struct{})
	var foundOnce sync.Once
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
			mu.Lock()
			asked[i] = true
			mu.Unlock()
			metas, _, err := s.window(ctx, v, src, 0, need)
			// An app that stops waiting cancels ctx: nothing failed.
			if err != nil && ctx.Err() == nil {
				s.logger.Warn("A search catalog could not be read", "addon", src.addon.addon.Manifest.Name, "catalog", src.catalog.ID, "error", err)
			}
			mu.Lock()
			lists[i], answered[i] = metas, true
			mu.Unlock()
			if err == nil && !byPeople {
				foundOnce.Do(func() { close(found) })
			}
		}()
	}
	everything := done(&all)
	titlesDone, firstFound := everything, (<-chan struct{})(nil)
	if titles > 0 {
		titlesDone, firstFound = done(&titleSearches), found
	}
	bound := time.NewTimer(s.searchBound)
	defer bound.Stop()
	var grace <-chan time.Time
	finished, late := false, false
	for !finished && !late && ctx.Err() == nil {
		select {
		case <-titlesDone:
			finished = true
		case <-firstFound:
			firstFound, grace = nil, time.After(s.searchGrace)
		case <-grace:
			late = true
		case <-bound.C:
			late = true
		case <-ctx.Done():
		}
	}
	if finished && people && titles > 0 {
		select {
		case <-everything:
		case <-time.After(peopleGrace):
		case <-ctx.Done():
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if late && ctx.Err() == nil {
		for i, src := range sources {
			if asked[i] && !answered[i] && !searchesPeople(src.catalog) && !own(src.addon.addon) {
				s.lateSearch(src)
			}
		}
	}
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

// Artwork is an item's image as Artwork finds it. URL stands for it (see
// ImageTag); downloading it is confined to public addresses when Confined.
// Generated, when set, is the image Polyfin draws for the item (see
// GeneratedImage): only Polyfin's own choice is one, never an artwork URL
// in the scheme of generated images.
type Artwork struct {
	URL       string
	Confined  bool
	Generated *GeneratedImage
}

// Artwork returns an item's image: the artwork an administrator uploaded,
// else a local folder's (see Local.Artwork), else the addon's (see
// Uploaded), else its generated image, if any. It needs no user: Jellyfin
// apps load images without credentials, and artwork URLs carry no secret.
// A title's image its listing lacks is the one of its description,
// whichever addon gave it (see artworkView); an extra's Primary and
// Backdrop images are its title's (see extraArtwork).
func (s *Service) Artwork(ctx context.Context, id accounts.ID, imageType string) (Artwork, error) {
	if url, ok := s.UploadedArtwork(id, imageType); ok {
		return Artwork{URL: url}, nil
	}
	local := s.localArtwork()
	if url := local[id].URL(imageType); url != "" {
		return Artwork{URL: url}, nil
	}
	r, err := s.load(ctx, id)
	if errors.Is(err, ErrNotFound) {
		// A smart list shown as a collection has no record: its generated
		// image, if any, is remembered apart.
		if g := s.generatedArtwork(ctx, id, imageType); g != nil {
			return g.artwork(), nil
		}
		// Nor has an extra: it shows its title's.
		if found, ok, err := s.extraArtwork(ctx, id, imageType); ok {
			return found, err
		}
	}
	// An IPTV channel shows its line-up's logo at once, listed or not: the
	// admin app shows those of hidden channels too.
	if s.iptv != nil && strings.EqualFold(imageType, "Primary") && (errors.Is(err, ErrNotFound) || err == nil && r.Kind == KindChannel &&
		r.Meta != nil && strings.HasPrefix(r.Meta.ID, iptv.IDPrefix)) {
		logo, confined, lineupErr := s.iptv.Logo(ctx, id)
		switch {
		case lineupErr == nil && logo == "":
			return Artwork{}, ErrNotFound
		case lineupErr == nil:
			s.iptvHosts.logo(logo)
			return Artwork{URL: logo, Confined: confined}, nil
		case !errors.Is(lineupErr, stremio.ErrNotFound):
			return Artwork{}, lineupErr
		}
	}
	if err != nil {
		return Artwork{}, err
	}
	// A Replay folder shows its channel's artwork, as does a programme of
	// it whose guide gives no image.
	if r.Kind == KindReplayFolder || r.Kind == KindReplay && r.Video != nil && r.Video.Thumbnail == "" {
		return s.Artwork(ctx, itemID(channelKey(r.Channel)), imageType)
	}
	var images Images
	confined := r.Confined
	switch {
	case r.Kind == KindChannel && r.Meta != nil:
		images = channelImages(*r.Meta)
	case (r.Kind == KindProgram || r.Kind == KindReplay) && r.Video != nil:
		// The guide that listed the programme may come from another addon
		// than its channel: either one confines its artwork.
		images.Primary = r.Video.Thumbnail
		channel, err := s.load(ctx, itemID(channelKey(r.Channel)))
		confined = r.Confined || err != nil || channel.Confined
	case r.Meta != nil:
		var item Item
		fromMeta(&item, *r.Meta)
		images = item.Images
		if images.URL(imageType) == "" {
			v, err := s.artworkView(ctx, r)
			if err != nil {
				return Artwork{}, err
			}
			if full, describer, ok := s.describedMeta(ctx, v, r, false, inDatabase); ok {
				fromMeta(&item, full)
				images = item.Images
				confined = confined || describer.confined
			}
		}
	case r.Kind == KindEpisode && r.Video != nil:
		images.Primary = r.Video.Thumbnail
		if series, err := s.load(ctx, r.seriesItemID()); err == nil {
			confined = series.Confined
		}
	case r.Kind == KindPerson && r.Person != nil:
		images.Primary = r.personPhoto()
	case r.Kind == KindLibrary:
		// A library's automatic or generated image, when it was last
		// looked up.
		if g := s.generatedLibraryArtwork(ctx, r); g != nil && strings.EqualFold(imageType, "Primary") {
			return g.artwork(), nil
		}
		images.Primary = r.Poster
	case r.Music != nil:
		images.Primary = r.Music.Artwork
	case r.Kind == KindSeason:
		series, err := s.load(ctx, r.seriesItemID())
		if err != nil || series.Meta == nil {
			return Artwork{}, ErrNotFound
		}
		// The season's own artwork, and its series', are read from the
		// series' complete metadata while it is kept, else as their last
		// listing recorded them: the images apps were given tags for,
		// without asking the addon again.
		v, err := s.artworkView(ctx, series)
		if err != nil {
			return Artwork{}, err
		}
		full, describer, described := s.describedMeta(ctx, v, series, false, inDatabase)
		images = withLocal(Images{Primary: cmp.Or(full.Poster, series.Meta.Poster), Backdrop: cmp.Or(full.Background, series.Meta.Background)},
			Images{Primary: local[series.ID].Primary, Backdrop: local[series.ID].Backdrop})
		// A season shows its series' artwork, local or uploaded, unless it
		// has its own.
		for _, imageType := range []string{"Primary", "Backdrop"} {
			if url, ok := s.UploadedArtwork(series.ID, imageType); ok {
				setImage(&images, imageType, url)
			}
		}
		poster := r.Poster
		if described {
			poster = seasonPosters(full)[r.Season]
		}
		if poster != "" {
			images.Primary = poster
		}
		confined = series.Confined || describer.confined
	}
	url := images.URL(imageType)
	if url == "" && r.Kind == KindEpisode && strings.EqualFold(imageType, "Primary") {
		// An episode without an image shows a frame of its file (see
		// applyLocal).
		url = local[id].Frame
	}
	if url == "" && r.Kind == KindCollection && images.Primary == "" {
		// An addon's collection without a poster shows its generated
		// images, if any.
		if g := s.generatedArtwork(ctx, id, imageType); g != nil {
			return g.artwork(), nil
		}
	}
	// An addon's artwork in the scheme of generated images, kept from
	// before such artwork was dropped as it came in, is no image.
	if url == "" || strings.HasPrefix(url, generatedScheme) {
		return Artwork{}, ErrNotFound
	}
	return Artwork{URL: url, Confined: confined}, nil
}

// artworkView is the view a title's artwork is found with (see
// describedMeta): image requests come without a user, so the server's
// addons describe it, after the addon that listed it when that one is a
// user's own.
func (s *Service) artworkView(ctx context.Context, r record) (view, error) {
	v, err := s.view(ctx, accounts.User{})
	if err != nil || r.Addon == nil {
		return v, err
	}
	if _, ok := v.addon(*r.Addon); ok {
		return v, nil
	}
	addon, err := s.addons.Find(ctx, *r.Addon)
	switch {
	case errors.Is(err, addons.ErrNotFound):
		return v, nil
	case err != nil:
		return view{}, err
	case addon.Enabled:
		v.addons = append([]installed{{addon: addon, confined: r.Confined}}, v.addons...)
	}
	return v, nil
}
