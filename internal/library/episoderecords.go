package library

import (
	"context"
	"encoding/json"
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
// one series at a time for the whole server, from the records the
// request listed, asking no addon, and does a series' again only once
// its list changed, as a fingerprint of its encoded records tells: a
// list that stays the same costs its encoding, not a save's look-ups.
// A failed save is logged and tried again at the next listing of the
// series. The server stopping ends it, forgetting what waits.
type episodeRecording struct {
	mu sync.Mutex
	// waiting holds the records of each series to record, the latest
	// listing's, and queue the series in the order they came.
	waiting map[accounts.ID][]record
	queue   []accounts.ID
	// running tells whether a worker records the queue.
	running bool
	// recorded holds the fingerprint of the list last recorded of each
	// series, made on first use; generation counts the times it was
	// forgotten (see forgetRecordedEpisodes).
	recorded   *cache.Cache[accounts.ID, uint64]
	generation uint64
	// idle is closed once the queue is empty and no worker runs, for
	// tests; nil until a series waits.
	idle chan struct{}
}

// recordedSeed seeds the fingerprints of the lists episodeRecording
// remembers, which live in memory only.
var recordedSeed = maphash.MakeSeed()

// recordEpisodesLater queues the records of each series listed, to be
// recorded once ctx, the listing's request, ends: the request never waits
// for them, nor shares the database with them.
func (s *Service) recordEpisodesLater(ctx context.Context, listed map[accounts.ID][]record) {
	if len(listed) == 0 {
		return
	}
	m := &s.episodeRecords
	m.mu.Lock()
	if m.waiting == nil {
		m.waiting = map[accounts.ID][]record{}
	}
	for id, records := range listed {
		if _, queued := m.waiting[id]; !queued {
			m.queue = append(m.queue, id)
		}
		m.waiting[id] = records
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
		records := m.waiting[id]
		delete(m.waiting, id)
		if m.recorded == nil {
			m.recorded = cache.New[accounts.ID, uint64](recordedSeries, math.MaxInt64)
		}
		recorded, known := m.recorded.Get(id)
		generation := m.generation
		m.mu.Unlock()

		fingerprint, err := listFingerprint(records)
		if err == nil && known && fingerprint == recorded {
			continue
		}
		if err == nil {
			err = s.save(ctx, records)
		}
		if err != nil {
			if ctx.Err() == nil {
				s.logger.Debug("The episodes of a series could not be recorded", "series", id, "error", err)
			}
			continue
		}
		m.mu.Lock()
		if m.generation == generation {
			m.recorded.Put(id, fingerprint)
		}
		m.mu.Unlock()
	}
}

// listFingerprint is the fingerprint of the records of a series' list,
// in their order, as a save stores them: encoded straight into the hash,
// as it runs at every Next Up.
func listFingerprint(records []record) (uint64, error) {
	var h maphash.Hash
	h.SetSeed(recordedSeed)
	encoder := json.NewEncoder(&h)
	for i := range records {
		r := records[i].stored()
		_, _ = h.Write(r.ID[:])
		_, _ = h.WriteString(r.Key)
		// Each record ends with a new line.
		if err := encoder.Encode(r); err != nil {
			return 0, err
		}
	}
	return h.Sum64(), nil
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
