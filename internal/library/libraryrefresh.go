package library

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Refreshing a library's descriptions.
//
// An administrator may have the descriptions of one of the server's movie,
// series or collection libraries read again. For a movie or series
// library, those of the titles of its catalog pages Polyfin keeps, the
// pages apps opened and the first pages the hourly warming reads (see
// Warm), up to the library's maximum; for a local folder, of every title
// it lists. For a collection library, those of its collections, then of
// the titles of the catalogs they group, from the pages Polyfin keeps (see
// refreshedCollections). Titles are described again one after the other,
// LibraryRefreshPace apart, so that a library of thousands of titles does
// not flood the metadata addons: Polyfin never reads a whole remote
// catalog to find more. An addon that answers "too many requests" is
// waited for as long as it asks, refreshRateWait doubled at each try when
// it does not say, at most maxRefreshWait, and asked again, refreshTries
// times at most. Libraries asked to be refreshed meanwhile wait their
// turn: one refresh runs at a time on the server, so the pace bounds them
// all. A title read again since its library's refresh was asked, by the
// refresh of a library before it that lists it too, is not asked again.
// Each title is described as Refresh describes it (see describeAgain), a
// collection by its addon: an addon that fails leaves its description as
// it was.

// LibraryRefreshPace is the time between two titles a library refresh
// describes again: 30 a minute.
const LibraryRefreshPace = 2 * time.Second

const (
	refreshTries    = 3
	refreshRateWait = 30 * time.Second
	maxRefreshWait  = 15 * time.Minute
)

// ErrNotRefreshable is returned for a library whose descriptions are not
// refreshed: a live TV, music or IPTV source's library.
var ErrNotRefreshable = errors.New("this library's descriptions are not refreshed")

// The states of a library refresh.
const (
	RefreshQueued  = "queued"
	RefreshRunning = "running"
	RefreshDone    = "done"
	RefreshStopped = "stopped"
	RefreshFailed  = "failed"
)

// LibraryRefresh is how the refresh of a library's descriptions stands:
// queued, running, or ended done, stopped, or failed when its titles
// could not be read. Titles counts the titles to describe again, a
// collection library's collections included, Done those described, or
// read again since it was asked, Failed those no addon described.
// Started and Ended are zero until it starts and ends.
type LibraryRefresh struct {
	Library        accounts.ID
	State          string
	Titles         int
	Done, Failed   int
	Started, Ended time.Time
	// asked is when it was asked for.
	asked time.Time
}

// libraryRefreshes holds the refreshes asked for since the server
// started, the last one of each library, the queue of those waiting, and
// how to stop the one running.
type libraryRefreshes struct {
	mu      sync.Mutex
	states  map[accounts.ID]*LibraryRefresh
	queue   []accounts.ID
	running bool
	stop    context.CancelFunc
	current accounts.ID
	// pace is LibraryRefreshPace, wait waits for d or ctx (tests replace
	// them).
	pace time.Duration
	wait func(ctx context.Context, d time.Duration) error
}

func waitFor(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Refreshable reports whether the descriptions of a library of addon's
// catalog are refreshed: those of a movie or series catalog of a Stremio
// addon or a local folder, and of a collection catalog of a Stremio
// addon.
func Refreshable(addon addons.Addon, catalog stremio.Catalog) bool {
	switch catalog.Type {
	case "movie", "series":
		return !addon.IPTV() && !addon.HasMusic()
	case "collection":
		return addon.Stremio() && !addon.HasMusic()
	}
	return false
}

// RefreshLibrary has the descriptions of one of the server's libraries,
// by its item, read again in the background, after the refreshes asked
// for before it. A library being refreshed, or waiting to be, is left as
// it is. It returns ErrNotFound for an item that is not one of the
// server's libraries, ErrNotRefreshable for one not refreshed.
func (s *Service) RefreshLibrary(ctx context.Context, id accounts.ID) (LibraryRefresh, error) {
	v, err := s.view(ctx, accounts.User{})
	if err != nil {
		return LibraryRefresh{}, err
	}
	l, ok := v.library(id)
	if !ok {
		return LibraryRefresh{}, ErrNotFound
	}
	if !Refreshable(l.addon.addon, l.catalog) {
		return LibraryRefresh{}, ErrNotRefreshable
	}
	r := &s.refreshes
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.states == nil {
		r.states = map[accounts.ID]*LibraryRefresh{}
	}
	if state, ok := r.states[id]; ok && (state.State == RefreshQueued || state.State == RefreshRunning) {
		return *state, nil
	}
	state := &LibraryRefresh{Library: id, State: RefreshQueued, asked: s.now()}
	r.states[id] = state
	r.queue = append(r.queue, id)
	if !r.running {
		r.running = true
		// The request ending does not stop the refreshes, the server
		// stopping does (see detached).
		detached, done := s.detached(ctx)
		go func() {
			defer done()
			s.runRefreshes(detached)
		}()
	}
	return *state, nil
}

// StopLibraryRefresh stops a library's refresh, running or waiting, and
// reports whether there was one. The titles described stay so.
func (s *Service) StopLibraryRefresh(id accounts.ID) bool {
	r := &s.refreshes
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.states[id]
	if !ok {
		return false
	}
	switch state.State {
	case RefreshQueued:
		r.queue = slices.DeleteFunc(r.queue, func(queued accounts.ID) bool { return queued == id })
		state.State, state.Ended = RefreshStopped, s.now()
		return true
	case RefreshRunning:
		if r.current == id && r.stop != nil {
			r.stop()
		}
		return true
	}
	return false
}

// LibraryRefreshes tells how the refreshes asked for since the server
// started stand, the last one of each library.
func (s *Service) LibraryRefreshes() map[accounts.ID]LibraryRefresh {
	r := &s.refreshes
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make(map[accounts.ID]LibraryRefresh, len(r.states))
	for id, state := range r.states {
		result[id] = *state
	}
	return result
}

// runRefreshes refreshes the libraries queued, one after the other, until
// none is left.
func (s *Service) runRefreshes(ctx context.Context) {
	r := &s.refreshes
	for {
		r.mu.Lock()
		if len(r.queue) == 0 {
			r.running, r.stop, r.current = false, nil, accounts.ID{}
			r.mu.Unlock()
			return
		}
		id := r.queue[0]
		r.queue = r.queue[1:]
		state := r.states[id]
		run, stop := context.WithCancel(ctx)
		r.current, r.stop = id, stop
		state.State, state.Started = RefreshRunning, s.now()
		r.mu.Unlock()
		err := s.refreshLibrary(run, id, state)
		stopped := run.Err() != nil
		stop()
		r.mu.Lock()
		switch {
		case stopped:
			state.State = RefreshStopped
		case err != nil:
			state.State = RefreshFailed
			s.logger.Warn("A library's descriptions could not be refreshed", "library", id, "error", err)
		default:
			state.State = RefreshDone
		}
		state.Ended = s.now()
		s.logger.Info("The refresh of a library's descriptions ended", "library", id, "state", state.State, "titles", state.Titles,
			"described", state.Done, "failed", state.Failed, "took", state.Ended.Sub(state.Started).Round(time.Second))
		r.mu.Unlock()
	}
}

// refreshLibrary describes again the titles of a library, counting them
// in state.
func (s *Service) refreshLibrary(ctx context.Context, id accounts.ID, state *LibraryRefresh) error {
	v, err := s.view(ctx, accounts.User{})
	if err != nil {
		return err
	}
	l, ok := v.library(id)
	if !ok {
		return ErrNotFound
	}
	titles, err := s.refreshedTitles(ctx, v, l)
	if err != nil {
		return err
	}
	r := &s.refreshes
	r.mu.Lock()
	state.Titles = len(titles)
	pace, wait, since := r.pace, r.wait, state.asked
	r.mu.Unlock()
	paced := false
	for _, title := range titles {
		if at, ok := s.describedAt(ctx, v, l, title); ok && at.After(since) {
			r.mu.Lock()
			state.Done++
			r.mu.Unlock()
			continue
		}
		if paced {
			if err := wait(ctx, pace); err != nil {
				return nil
			}
		}
		paced = true
		described := false
		for try := 0; ; try++ {
			ok, keptBy, err := s.redescribeRefreshed(ctx, v, l, title)
			if ctx.Err() != nil {
				return nil
			}
			if ok || try+1 == refreshTries || stremio.StatusOf(err) != 429 {
				described = ok
				if !ok {
					s.keptBefore(title, keptBy, err)
				}
				break
			}
			delay, asked := stremio.RetryAfter(err)
			if !asked {
				delay = refreshRateWait << try
			}
			delay = min(delay, maxRefreshWait)
			// Waited out, the title may well be described: only a title
			// still failing after its last try is a warning.
			s.logger.Info("An addon asked to wait: the title is asked again after the wait", "item", title.ID, "wait", delay,
				"try", try+1)
			if err := wait(ctx, delay); err != nil {
				return nil
			}
		}
		r.mu.Lock()
		if described {
			state.Done++
		} else {
			state.Failed++
		}
		r.mu.Unlock()
	}
	return nil
}

// redescribeRefreshed asks for the description of a title of library l
// again, as describeAgain does, and for a collection's, of its addon, as
// the library lists it (see groupedSources).
func (s *Service) redescribeRefreshed(ctx context.Context, v view, l library, r record) (described bool, keptBy string, err error) {
	if r.Kind != KindCollection {
		return s.describeAgain(ctx, v, r)
	}
	key := metaKey{l.addon.addon.ID, r.Meta.Type, r.Meta.ID}
	if _, err = s.fetchKeptMeta(ctx, l.addon, key); err == nil {
		return true, "", nil
	}
	if _, ok := s.keptDescription(ctx, l.addon, key, inDatabase); ok {
		keptBy = l.addon.addon.Manifest.Name
	}
	return false, keptBy, err
}

// describedAt tells when the description a refresh of library l reads
// again was given, as Polyfin keeps it, asking no addon: a collection's
// by its addon, a title's by the first of its describers that keeps one
// (see describedMeta). ok is false when none is kept.
func (s *Service) describedAt(ctx context.Context, v view, l library, r record) (at time.Time, ok bool) {
	candidates := []installed{l.addon}
	if r.Kind != KindCollection {
		candidates = describers(v, r, false)
	}
	for _, candidate := range candidates {
		if own(candidate.addon) {
			continue
		}
		key := metaKey{candidate.addon.ID, r.Meta.Type, r.Meta.ID}
		kept, ok := s.metas.Get(key)
		if !ok {
			kept, ok = s.loadMeta(ctx, candidate, key)
		}
		if ok {
			return kept.at, true
		}
	}
	return time.Time{}, false
}

// refreshedTitles lists the titles of a library a refresh describes again,
// in their order: those of its catalog pages Polyfin keeps, or of every
// page of a local folder's catalog, at most its maximum; for a collection
// library, what refreshedCollections lists.
func (s *Service) refreshedTitles(ctx context.Context, v view, l library) ([]record, error) {
	most := addons.MaxLibraryItems
	if l.maxItems > 0 {
		most = l.maxItems
	}
	if l.catalog.Type == "collection" {
		return s.refreshedCollections(ctx, v, l, most)
	}
	var pages [][]stremio.Meta
	if l.addon.addon.Local() {
		if s.local == nil {
			return nil, nil
		}
		listed := 0
		for listed < most {
			metas, err := s.local.Catalog(ctx, l.addon.addon.ID, l.catalog.Type, l.catalog.ID, listed, "")
			if err != nil {
				return nil, err
			}
			if len(metas) == 0 {
				break
			}
			pages = append(pages, metas)
			listed += len(metas)
		}
	} else {
		var err error
		if pages, err = s.keptPages(ctx, l.catalogSource()); err != nil {
			return nil, err
		}
	}
	var titles []record
	seen := map[accounts.ID]bool{}
	for _, metas := range pages {
		for _, meta := range metas {
			if len(titles) == most {
				return titles, nil
			}
			// Only the record is kept, which holds no rating: no country
			// needed.
			_, r, err := s.titleItem(l.addon, l.catalog, meta, l.item.ID, "")
			if err != nil || seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			titles = append(titles, r)
		}
	}
	return titles, nil
}

// refreshedCollections lists what the refresh of collection library l
// describes again, from what Polyfin keeps alone, asking no addon: the
// collections of the catalog pages it keeps, up to most, then those
// within them, followed as groupedSources follows them; then the titles
// of the catalogs they group, one of each catalog in turn, as the library
// lists them, among its first most. Only those whose description is kept
// are: a collection without one groups catalogs unknown, and a title never
// described has nothing to read again.
func (s *Service) refreshedCollections(ctx context.Context, v view, l library, most int) ([]record, error) {
	pages, err := s.keptPages(ctx, l.catalogSource())
	if err != nil {
		return nil, err
	}
	type listed struct {
		meta   stremio.Meta
		parent accounts.ID
	}
	var level []listed
	for _, metas := range pages {
		for _, meta := range metas {
			if len(level) < most {
				level = append(level, listed{meta, l.item.ID})
			}
		}
	}
	var refreshed []record
	var sources []source
	described := map[string]bool{}
	grouped := map[catalogKey]bool{}
	for depth := 0; len(level) > 0 && depth < maxNesting; depth++ {
		var within []listed
		for _, entry := range level {
			if entry.meta.ID == "" || described[entry.meta.ID] {
				continue
			}
			described[entry.meta.ID] = true
			meta, ok := s.keptDescription(ctx, l.addon, metaKey{l.addon.addon.ID, "collection", entry.meta.ID}, inDatabase)
			if !ok {
				continue
			}
			preview := entry.meta
			preview.Type = "collection"
			_, r := s.collectionItem(l.addon.addon.ID, preview, entry.parent, l.addon.confined, "")
			refreshed = append(refreshed, r)
			if meta.Collection == nil {
				continue
			}
			for _, ref := range meta.Collection.Sources {
				key := catalogKey{l.addon.addon.ID, ref.Type, ref.CatalogID, ref.Genre}
				if catalog, ok := l.addon.addon.Manifest.Catalog(ref.Type, ref.CatalogID); ok && !grouped[key] {
					grouped[key] = true
					sources = append(sources, source{addon: l.addon, catalog: catalog, genre: ref.Genre, max: l.maxItems})
				}
			}
			for _, sub := range meta.Collection.Items {
				within = append(within, listed{sub, r.ID})
			}
		}
		level = within
	}
	lists := make([][]stremio.Meta, len(sources))
	for i, src := range sources {
		pages, err := s.keptPages(ctx, src)
		if err != nil {
			return nil, err
		}
		lists[i] = slices.Concat(pages...)
	}
	seen := map[accounts.ID]bool{}
	titles := 0
	for i := 0; titles < most; i++ {
		more := false
		for j, src := range sources {
			if i >= len(lists[j]) || titles == most {
				continue
			}
			more = true
			_, r, err := s.titleItem(src.addon, src.catalog, lists[j][i], l.item.ID, "")
			if err != nil || seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			titles++
			if _, ok := s.describedAt(ctx, v, l, r); ok {
				refreshed = append(refreshed, r)
			}
		}
		if !more {
			break
		}
	}
	return refreshed, nil
}

// keptPages reads the pages of src's catalog Polyfin keeps in the
// database, stale ones included, in their order.
func (s *Service) keptPages(ctx context.Context, src source) ([][]stremio.Meta, error) {
	rows, err := s.db.Query(ctx, `SELECT metas FROM catalog_pages
		WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3 AND genre = $4 AND config = $5 AND fetched_at > $6
		ORDER BY skip`, src.addon.addon.ID, src.catalog.Type, src.catalog.ID, src.genre, configOf(src.addon.addon),
		s.now().Add(-s.keptCatalogLife()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pages [][]stremio.Meta
	for rows.Next() {
		var data []byte
		var metas []stremio.Meta
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &metas); err != nil {
			return nil, err
		}
		pages = append(pages, metas)
	}
	return pages, rows.Err()
}
