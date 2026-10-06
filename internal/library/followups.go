package library

import (
	"context"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Some addons gather other addons' streams: they answer the first request
// for a title with the streams that came in within their own time limit,
// and keep the others, which come moments later, for the next request. So
// once Polyfin stores an addon's answer for a title, it asks the addon
// again a little later, and keeps the longer list.

// followUpDelays are how long after an addon's answer for a title Polyfin
// asks it again: once, then once more while its answers grow.
var followUpDelays = []time.Duration{10 * time.Second, 30 * time.Second}

// The lists followed up.
const (
	listStreams   = "streams"
	listSubtitles = "subtitles"
)

// followKey is a list an addon gave for a title.
type followKey struct {
	list string
	streamKey
}

// followUp is the schedule running for a list, told apart from the others
// by its address: it is not empty, as pointers to empty values may be
// equal.
type followUp struct{ _ byte }

// followUps holds the schedule running for each list, at most one.
type followUps struct {
	mu      sync.Mutex
	running map[followKey]*followUp
}

// SetFollowUps replaces the delays of the follow-ups, 10 and 30 seconds,
// before the service is used. Tests shorten them, or give none to ask no
// addon again; the server keeps them.
func (s *Service) SetFollowUps(delays ...time.Duration) { s.followUpDelays = delays }

// followStreams follows up the streams an addon just listed for a title,
// stored under key (see followUpList).
func (s *Service) followStreams(ctx context.Context, entry installed, key streamKey) {
	followUpList(ctx, s, s.streamLists, followKey{listStreams, key}, entry.addon.Manifest.Name,
		func(ctx context.Context) ([]stremio.Stream, error) {
			return s.fetchStreams(ctx, entry, key.contentType, key.id)
		},
		func(streams []stremio.Stream) int {
			n := 0
			for _, stream := range streams {
				if stream.Playable() {
					n++
				}
			}
			return n
		})
}

// followSubtitles follows up the subtitles an addon just listed for a
// title, stored under key (see followUpList).
func (s *Service) followSubtitles(ctx context.Context, entry installed, key streamKey) {
	followUpList(ctx, s, s.subtitleLists, followKey{listSubtitles, key}, entry.addon.Manifest.Name,
		func(ctx context.Context) ([]stremio.Subtitle, error) {
			return s.fetchSubtitles(ctx, entry, key.contentType, key.id)
		},
		func(subtitles []stremio.Subtitle) int {
			n := 0
			for _, subtitle := range subtitles {
				if subtitle.Valid() {
					n++
				}
			}
			return n
		})
}

// followUpList asks an addon again for a list it just gave, which lists
// keeps under key, after each of the service's followUpDelays in turn,
// detached from ctx's end, as shared does. An answer that counts more than
// the list kept replaces it; the schedule stops at the first answer that
// does not, at an error, after the last delay, or once the list is no
// longer kept: a refresh or its expiry forgot it, and a late answer never
// brings it back. The list just stored is a new first answer: its schedule
// replaces the one running for key, which stops.
func followUpList[T any](ctx context.Context, s *Service, lists *cache.Cache[streamKey, []T], key followKey, addon string,
	fetch func(context.Context) ([]T, error), count func([]T) int) {
	delays := s.followUpDelays
	if len(delays) == 0 {
		return
	}
	f := &followUp{}
	s.followUps.mu.Lock()
	s.followUps.running[key] = f
	s.followUps.mu.Unlock()
	detached := context.WithoutCancel(ctx)
	flightKey := "follow-up " + key.list + " " + key.addon.String() + " " + key.contentType + " " + key.id
	go func() {
		defer s.endFollowUp(key, f)
		for _, delay := range delays {
			time.Sleep(delay)
			if _, kept := lists.Get(key.streamKey); !kept || !s.followingUp(key, f) {
				return
			}
			answer, err := shared(detached, &s.flight, flightKey, fetch)
			if err != nil {
				s.logger.Debug("An addon asked again for a list failed", "addon", addon, "list", key.list, "error", err)
				return
			}
			before, after := 0, count(answer)
			// Checked and replaced together: a refresh that stops the
			// schedule first forgets the list after, or finds it stopped.
			s.followUps.mu.Lock()
			grew := s.followUps.running[key] == f && lists.Improve(key.streamKey, answer, func(kept []T) bool {
				before = count(kept)
				return after > before
			})
			s.followUps.mu.Unlock()
			if !grew {
				return
			}
			s.logger.Debug("An addon asked again listed more", "addon", addon, "list", key.list, "before", before, "after", after)
		}
	}()
}

// followingUp reports whether f is still the schedule of key.
func (s *Service) followingUp(key followKey, f *followUp) bool {
	s.followUps.mu.Lock()
	defer s.followUps.mu.Unlock()
	return s.followUps.running[key] == f
}

// endFollowUp forgets f, the schedule of key, unless another replaced it.
func (s *Service) endFollowUp(key followKey, f *followUp) {
	s.followUps.mu.Lock()
	defer s.followUps.mu.Unlock()
	if s.followUps.running[key] == f {
		delete(s.followUps.running, key)
	}
}

// stopFollowUps stops the schedules of an addon's lists for a title, which
// a refresh forgets.
func (s *Service) stopFollowUps(key streamKey) {
	s.followUps.mu.Lock()
	defer s.followUps.mu.Unlock()
	delete(s.followUps.running, followKey{listStreams, key})
	delete(s.followUps.running, followKey{listSubtitles, key})
}

// streamFollowUps counts the addons among serving whose streams for a
// title have a schedule running.
func (s *Service) streamFollowUps(serving []installed, t target) int {
	s.followUps.mu.Lock()
	defer s.followUps.mu.Unlock()
	n := 0
	for _, entry := range serving {
		if _, ok := s.followUps.running[followKey{listStreams, streamKey{entry.addon.ID, t.metaType, t.id}}]; ok {
			n++
		}
	}
	return n
}
