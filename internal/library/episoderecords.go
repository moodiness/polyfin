package library

import (
	"context"
	"hash/maphash"
	"math"
	"sync"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
)

// recordedSeries bounds the series episodeRecording remembers.
const recordedSeries = 1 << 12

// episodeRecording records, after the request, the episodes of the
// series PickedEpisodes listed that its caller did not keep, with the
// seasons, as the series' listing records them: Next Up shows one or two
// episodes of each series it lists, and the others are found by name
// (see SearchEpisodes) and opened by identifier as before. It records
// one series at a time for the whole server, from the list the request
// made, whose records it makes then, asking no addon, and does a
// series' again only once its list changed, as its stamp tells: a list
// that stays the same costs the hash of what it is made of, neither its
// records nor a save's look-ups. A failed save is logged and tried again
// at the next listing of the series. The server stopping ends it,
// forgetting what waits.
type episodeRecording struct {
	mu sync.Mutex
	// waiting holds the list of each series to record, the latest
	// listing's, and queue the series in the order they came.
	waiting map[accounts.ID]*outlinedEpisodes
	queue   []accounts.ID
	// running tells whether a worker records the queue.
	running bool
	// recorded holds the stamp of the list last recorded of each series
	// (see outlinedEpisodes.stamp), made on first use; generation counts
	// the times it was forgotten (see forgetRecordedEpisodes).
	recorded   *cache.Cache[accounts.ID, uint64]
	generation uint64
	// idle is closed once the queue is empty and no worker runs, for
	// tests; nil until a series waits.
	idle chan struct{}
}

// recordedSeed seeds the stamps of the lists episodeRecording remembers,
// which live in memory only.
var recordedSeed = maphash.MakeSeed()

// recordEpisodesLater queues the list of each series listed, to be
// recorded once ctx, the listing's request, ends: the request never waits
// for them, nor shares the database with them, nor makes their records.
func (s *Service) recordEpisodesLater(ctx context.Context, listed map[accounts.ID]*outlinedEpisodes) {
	if len(listed) == 0 {
		return
	}
	m := &s.episodeRecords
	m.mu.Lock()
	if m.waiting == nil {
		m.waiting = map[accounts.ID]*outlinedEpisodes{}
	}
	for id, list := range listed {
		if _, queued := m.waiting[id]; !queued {
			m.queue = append(m.queue, id)
		}
		m.waiting[id] = list
	}
	start := !m.running
	if start {
		m.running = true
		m.idle = make(chan struct{})
	}
	m.mu.Unlock()
	if !start {
		return
	}
	run := func() { go s.recordEpisodes() }
	if ctx.Done() == nil {
		run()
		return
	}
	context.AfterFunc(ctx, run)
}

// recordEpisodes records the queued series one at a time, until none
// waits or the server stops.
func (s *Service) recordEpisodes() {
	ctx, done := s.detached(context.Background())
	defer done()
	ctx = Background(ctx)
	m := &s.episodeRecords
	for {
		m.mu.Lock()
		if len(m.queue) == 0 || ctx.Err() != nil {
			m.queue, m.waiting = nil, nil
			m.running = false
			close(m.idle)
			m.mu.Unlock()
			return
		}
		id := m.queue[0]
		m.queue = m.queue[1:]
		list := m.waiting[id]
		delete(m.waiting, id)
		if m.recorded == nil {
			m.recorded = cache.New[accounts.ID, uint64](recordedSeries, math.MaxInt64)
		}
		recorded, known := m.recorded.Get(id)
		generation := m.generation
		m.mu.Unlock()

		stamp := list.stamp()
		if known && stamp == recorded {
			continue
		}
		if err := s.save(ctx, list.records(nil)); err != nil {
			if ctx.Err() == nil {
				s.logger.Debug("The episodes of a series could not be recorded", "series", id, "error", err)
			}
			continue
		}
		m.mu.Lock()
		if m.generation == generation {
			m.recorded.Put(id, stamp)
		}
		m.mu.Unlock()
	}
}

// forgetRecordedEpisodes forgets which lists were recorded: their items
// may have been deleted (see forgetAllSaved).
func (s *Service) forgetRecordedEpisodes() {
	m := &s.episodeRecords
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recorded = nil
	m.generation++
}

// EpisodesRecorded returns a channel closed once the episodes queued so
// far are recorded (see episodeRecording), or the server stopped: what
// tests wait for before they count statements or look the records up.
func (s *Service) EpisodesRecorded() <-chan struct{} {
	m := &s.episodeRecords
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return m.idle
}
