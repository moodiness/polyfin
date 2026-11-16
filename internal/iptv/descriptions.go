package iptv

import (
	"context"
	"errors"
	"maps"
	"math"
	"slices"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Descriptions kept in memory.
//
// Describing a movie or a series a source shows reads its list data, its
// details and, for an M3U series, its episodes, then decodes them: a
// series' details hold every episode, up to a few thousand. The home rows
// describe the same series at every load (Next Up, Upcoming, the versions
// listed ahead), so each description is kept decoded, by source, type and
// key, with the version of the source it was read at (see
// CatalogVersion): every write of a source's titles, episodes or options
// changes it, and the details of titles gone are deleted in the same
// transactions. A description kept serves while its source's version is
// still the one read with it, which takes one statement at each use; a
// request marked by CheckOnce reads the version of each source once. The
// details stored when a title is opened or played (see details) make its
// description be read again. A request opening a title (see Opening)
// serves the one kept too, unless the provider is to be asked for the
// title's details: then it reads the title whole. What is kept never
// changes an answer.
//
// The bound: a series' description takes about 190 bytes an episode in
// memory with the synthetic library's short texts (see descriptionSize),
// and a title without details about 1 KB. The bench's library (8,267
// series, 139 episodes on average, 2,023 at most) keeps 27 KB for an
// average series and 380 KB for its longest; its five users have the
// details of 259 series, 42,128 episodes, about 8 MB, which
// descriptionsBytes holds with room, as it does with a real provider's
// longer texts (plots and thumbnails, about 600 bytes an episode, some
// 25 MB). A provider of 40,000 series takes no more: only the titles
// described are kept, the least recently used evicted first, at most
// descriptionsMost of them, about 4 MB for titles without details.
const (
	descriptionsMost  = 4096
	descriptionsBytes = 32 << 20
)

// descriptions are the descriptions kept (see above).
type descriptions struct {
	once sync.Once
	kept *cache.Cache[descriptionKey, *description]
	// stored counts the details stored: a description read before the
	// details of a title changed is not kept once they did.
	stored atomic.Uint64
}

func (x *descriptions) cache() *cache.Cache[descriptionKey, *description] {
	x.once.Do(func() { x.kept = newDescriptionCache(descriptionsMost, descriptionsBytes) })
	return x.kept
}

func newDescriptionCache(most, bytes int) *cache.Cache[descriptionKey, *description] {
	return cache.New[descriptionKey, *description](most, math.MaxInt64).Sized(bytes, descriptionSize)
}

// forget forgets a title's description, as its details were stored.
func (x *descriptions) forget(source accounts.ID, typ, key string) {
	x.stored.Add(1)
	x.cache().Delete(descriptionKey{source, typ, key})
}

type descriptionKey struct {
	source   accounts.ID
	typ, key string
}

// description is what describes a title a source shows: its list data,
// its source's kind, its details when known with their version and when
// they were fetched, an M3U series' episodes, and the version of the
// source they were read at. It is shared: never changed once kept.
type description struct {
	version        string
	title          Title
	kind           string
	details        *details
	detailsVersion string
	detailsAt      *time.Time
	videos         []stremio.Video
}

// meta describes the title, as titleMeta does.
func (d *description) meta(source accounts.ID) stremio.Meta {
	return describedTitle(source, d.title, d.kind, d.details, d.videos)
}

// fetchDue reports whether opening the title at now asks its provider for
// its details, as details does: an Xtream account's title whose details
// are not known, are of another version of the title, or are old.
func (d *description) fetchDue(now time.Time) bool {
	fresh := d.details != nil && d.detailsVersion == d.title.Version && d.detailsAt != nil && now.Sub(*d.detailsAt) < detailTTL
	return !fresh && d.kind == addons.KindXtream
}

// describedTitle describes a title from its list data, completed by its
// details when known, and an M3U series by its episodes. What it shares
// with t, d and videos is copied: the caller may change the description.
func describedTitle(source accounts.ID, t Title, kind string, d *details, videos []stremio.Video) stremio.Meta {
	meta := listingMeta(source, t)
	if d != nil {
		d.apply(source, t, &meta)
	}
	if t.Type == typeSeries && kind != addons.KindXtream {
		meta.Videos = slices.Clone(videos)
	}
	meta.Cast, meta.Director = slices.Clone(meta.Cast), slices.Clone(meta.Director)
	if meta.Extras != nil {
		meta.Extras = &stremio.Extras{SeasonPosterByNumber: maps.Clone(meta.Extras.SeasonPosterByNumber)}
	}
	return meta
}

// description returns the description of a title a source shows, the one
// kept while its source's version stays the same, else read and kept.
func (s *Service) description(ctx context.Context, source accounts.ID, typ, key string) (*description, error) {
	state, err := s.sourceState(ctx, source)
	if err != nil {
		return nil, err
	}
	k, kept := descriptionKey{source, typ, key}, s.descriptions.cache()
	if d, ok := kept.Get(k); ok && d.version == state.version {
		return d, nil
	}
	stored := s.descriptions.stored.Load()
	d, err := s.readDescription(ctx, source, typ, key)
	if err != nil {
		return nil, err
	}
	kept.Update(k, func(*description, bool) (*description, bool) { return d, s.descriptions.stored.Load() == stored })
	return d, nil
}

// readDescription reads a title's description, with the version of its
// source in the same statement.
func (s *Service) readDescription(ctx context.Context, source accounts.ID, typ, key string) (*description, error) {
	var t storedTitle
	d := &description{}
	err := scanTitle(s.db.QueryRow(ctx, `SELECT `+titleColumns+`, a.kind, i.xmin::text || ' ' || coalesce(i.fetched_at::text, ''), x.details,
		coalesce(x.version, ''), x.fetched_at
		FROM iptv_titles t JOIN iptv_sources i ON i.addon_id = t.addon_id JOIN addons a ON a.id = t.addon_id
		LEFT JOIN iptv_details x ON x.addon_id = t.addon_id AND x.type = t.type AND x.key = t.key
		WHERE t.addon_id = $1 AND t.type = $2 AND t.key = $3 AND `+shownTitle, source, typ, key), &t, &d.kind, &d.version, &d.details,
		&d.detailsVersion, &d.detailsAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, stremio.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	d.title = t.Title
	if typ == typeSeries && d.kind != addons.KindXtream {
		if d.videos, err = s.m3uEpisodes(ctx, source, key); err != nil {
			return nil, err
		}
	}
	return d, nil
}

// sourceState is what a description reads of its source: its version
// (see CatalogVersion) and whether it is enriched (see Enrichment).
type sourceState struct {
	version    string
	enrichment bool
}

// sourceState reads a source's state, once for the request when ctx is
// marked by CheckOnce. stremio.ErrNotFound reports a source of no row.
func (s *Service) sourceState(ctx context.Context, source accounts.ID) (sourceState, error) {
	if checks, ok := ctx.Value(checkOnceKey{}).(*sourceChecks); ok {
		return checks.state(ctx, s, source)
	}
	return s.readSourceState(ctx, source)
}

func (s *Service) readSourceState(ctx context.Context, source accounts.ID) (sourceState, error) {
	var state sourceState
	err := s.db.QueryRow(ctx, `SELECT xmin::text || ' ' || coalesce(fetched_at::text, ''), enrichment FROM iptv_sources WHERE addon_id = $1`,
		source).Scan(&state.version, &state.enrichment)
	if errors.Is(err, pgx.ErrNoRows) {
		return sourceState{}, stremio.ErrNotFound
	}
	return state, err
}

type checkOnceKey struct{}

// CheckOnce marks ctx as one request's, which describes many titles of
// the same sources, as Next Up does: the state of each source, its
// version and enrichment, is read once, at its first use, for all of
// them; a read that failed is tried again at the next use. A source
// changed during the request may then be described as it was when the
// request first read it; a request that starts once the change is made
// reads the change.
func CheckOnce(ctx context.Context) context.Context {
	if _, ok := ctx.Value(checkOnceKey{}).(*sourceChecks); ok {
		return ctx
	}
	return context.WithValue(ctx, checkOnceKey{}, &sourceChecks{sources: map[accounts.ID]*sourceCheck{}})
}

// sourceChecks are the states of the sources a request read (see
// CheckOnce).
type sourceChecks struct {
	mu      sync.Mutex
	sources map[accounts.ID]*sourceCheck
}

// sourceCheck is a source's state, once read; mu is held while it is.
type sourceCheck struct {
	mu    sync.Mutex
	read  bool
	state sourceState
}

func (c *sourceChecks) state(ctx context.Context, s *Service, source accounts.ID) (sourceState, error) {
	c.mu.Lock()
	check, ok := c.sources[source]
	if !ok {
		check = &sourceCheck{}
		c.sources[source] = check
	}
	c.mu.Unlock()
	check.mu.Lock()
	defer check.mu.Unlock()
	if !check.read {
		state, err := s.readSourceState(ctx, source)
		if err != nil {
			return sourceState{}, err
		}
		check.state, check.read = state, true
	}
	return check.state, nil
}

// descriptionSize tells about how many bytes a description takes in
// memory: its structures and its texts.
func descriptionSize(d *description) int {
	t := d.title
	n := int(unsafe.Sizeof(description{})) + len(d.version) + len(d.kind) +
		texts(t.Type, t.Key, t.Name, t.Category, t.Poster, t.Rating, t.Extension, t.Quality, t.TMDB, t.IMDb, t.URL, t.Version) +
		texts(t.Listing.Plot, t.Listing.Released, t.Listing.Background, t.Listing.Trailer) +
		texts(t.Listing.Cast...) + texts(t.Listing.Director...) + texts(t.Listing.Genres...)
	for name, value := range t.Headers {
		n += texts(name, value) + mapEntry
	}
	for _, v := range d.videos {
		n += int(unsafe.Sizeof(v)) + len(v.ID) + len(v.Title)
	}
	if x := d.details; x != nil {
		n += int(unsafe.Sizeof(*x)) + texts(x.Plot, x.Released, x.Poster, x.Background, x.Trailer, x.TMDB, x.IMDb, x.Rating, x.Extension) +
			texts(x.Cast...) + texts(x.Director...) + texts(x.Genres...)
		for season, poster := range x.Seasons {
			n += texts(season, poster) + mapEntry
		}
		for _, e := range x.Episodes {
			n += int(unsafe.Sizeof(e)) + len(e.Key) + len(e.Title) + len(e.Plot) + len(e.Thumbnail) + len(e.Released) + len(e.Extension) + len(e.Quality)
		}
	}
	return n
}

// mapEntry is about what a map's entry takes besides its key and value.
const mapEntry = 16

// texts tells the bytes strings take: their headers and their bytes.
func texts(values ...string) int {
	n := len(values) * int(unsafe.Sizeof(""))
	for _, value := range values {
		n += len(value)
	}
	return n
}
