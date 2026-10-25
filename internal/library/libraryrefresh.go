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
// An administrator may have the descriptions of one of the server's movie
// or series libraries read again: those of the titles of its catalog
// pages Polyfin keeps, the pages apps opened and the first pages the
// hourly warming reads (see Warm), up to the library's maximum; for a
// local folder, of every title it lists. Titles are described again one
// after the other, LibraryRefreshPace apart, so that a library of
// thousands of titles does not flood the metadata addons: Polyfin never
// reads a whole remote catalog to find more. An addon that answers "too
// many requests" is waited for as long as it asks, refreshRateWait
// doubled at each try when it does not say, at most maxRefreshWait, and
// asked again, refreshTries times at most. Libraries asked to be
// refreshed meanwhile wait their turn: one refresh runs at a time on the
// server, so the pace bounds them all. Each title is described as
// Refresh describes it (see describeAgain): an addon that fails leaves its
// description as it was.

// LibraryRefreshPace is the time between two titles a library refresh
// describes again: 30 a minute.
const LibraryRefreshPace = 2 * time.Second

const (
	refreshTries    = 3
	refreshRateWait = 30 * time.Second
	maxRefreshWait  = 15 * time.Minute
)

// ErrNotRefreshable is returned for a library whose descriptions are not
// refreshed: a collection, live TV, music or IPTV source's library.
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
// could not be read. Titles counts the titles to describe again, Done
// those described, Failed those no addon described. Started and Ended
// are zero until it starts and ends.
type LibraryRefresh struct {
	Library        accounts.ID
	State          string
	Titles         int
	Done, Failed   int
	Started, Ended time.Time
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
// addon or a local folder.
func Refreshable(addon addons.Addon, catalog stremio.Catalog) bool {
	return (catalog.Type == "movie" || catalog.Type == "series") && !addon.IPTV() && !addon.HasMusic()
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
	state := &LibraryRefresh{Library: id, State: RefreshQueued}
	r.states[id] = state
	r.queue = append(r.queue, id)
	if !r.running {
		r.running = true
		go s.runRefreshes(context.WithoutCancel(ctx))
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
	titles, err := s.refreshedTitles(ctx, l)
	if err != nil {
		return err
	}
	r := &s.refreshes
	r.mu.Lock()
	state.Titles = len(titles)
	pace, wait := r.pace, r.wait
	r.mu.Unlock()
	for i, title := range titles {
		if i > 0 {
			if err := wait(ctx, pace); err != nil {
				return nil
			}
		}
		described := false
		for try := 0; ; try++ {
			ok, keptBy, err := s.describeAgain(ctx, v, title)
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

// refreshedTitles lists the titles of a library a refresh describes again,
// in their order: those of its catalog pages Polyfin keeps, or of every
// page of a local folder's catalog, at most its maximum.
func (s *Service) refreshedTitles(ctx context.Context, l library) ([]record, error) {
	most := addons.MaxLibraryItems
	if l.maxItems > 0 {
		most = l.maxItems
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
		src := l.catalogSource()
		rows, err := s.db.Query(ctx, `SELECT metas FROM catalog_pages
			WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3 AND genre = $4 AND config = $5 AND fetched_at > $6
			ORDER BY skip`, l.addon.addon.ID, l.catalog.Type, l.catalog.ID, src.genre, configOf(l.addon.addon),
			s.now().Add(-s.keptCatalogLife()))
		if err != nil {
			return nil, err
		}
		defer rows.Close()
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
		if err := rows.Err(); err != nil {
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
			_, r, err := titleItem(l.addon.addon.ID, l.catalog, meta, l.item.ID, l.addon.confined)
			if err != nil || seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			titles = append(titles, r)
		}
	}
	return titles, nil
}
