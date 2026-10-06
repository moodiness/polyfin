package library

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Some addons gather other addons' streams: they answer the first request
// for a title with the streams that came in within their own time limit,
// and keep the others, which come moments later, for the next request. So
// once Polyfin stores an addon's answer for a title, it asks the addon
// again a little later, and keeps the longer list. An answer that replaces
// an expired list keeps that list's streams listed meanwhile (see
// keepAnswer): the title opened again does not lose the versions it had.

// followUpDelays are how long after an addon's answer for a title Polyfin
// asks it again: once, then once more while its answers grow.
var followUpDelays = []time.Duration{10 * time.Second, 30 * time.Second}

// staleLists is how long an addon's list for a title is kept once expired,
// as a stale list: item details list it while the addon is asked again
// (see VersionsNow), while requests that wait for every version wait for
// the answer instead.
const staleLists = 24 * time.Hour

// The lists followed up.
const (
	listStreams   = "streams"
	listSubtitles = "subtitles"
)

// list is an addon's list for a title as Polyfin keeps it: the addon's
// last answer, given at, then, while that answer is followed up, the items
// of the list it replaced that it lacks (see keepAnswer). A list restored
// from the database after a restart is stale however recent: its follow-ups
// did not run to their end, and its links may have expired.
type list[T any] struct {
	items []T
	// answer is how many of items the last answer gave.
	answer   int
	at       time.Time
	restored bool
}

// state tells whether l is fresh at now, lists being fresh for life, and
// whether it is still kept: expired, a list is kept staleLists more.
func (l list[T]) state(now time.Time, life time.Duration) (fresh, kept bool) {
	age := now.Sub(l.at)
	return age <= life && !l.restored, age <= life+staleLists
}

// listKind names the lists of T, and tells how to merge them: usable picks
// the items that count, playable streams or subtitles that can be
// downloaded, and identity tells the same item in two answers.
type listKind[T any] struct {
	name     string
	usable   func(T) bool
	identity func(T) string
}

var (
	streamKind   = listKind[stremio.Stream]{listStreams, stremio.Stream.Playable, streamIdentity}
	subtitleKind = listKind[stremio.Subtitle]{listSubtitles, stremio.Subtitle.Valid, subtitleIdentity}
)

// count is how many of items count.
func (k listKind[T]) count(items []T) int {
	n := 0
	for _, item := range items {
		if k.usable(item) {
			n++
		}
	}
	return n
}

// merge lists answer, given at, followed by the usable items of extras it
// lacks.
func (k listKind[T]) merge(answer, extras []T, at time.Time) list[T] {
	merged := list[T]{items: answer, answer: len(answer), at: at}
	if len(extras) == 0 {
		return merged
	}
	listed := make(map[string]bool, len(answer)+len(extras))
	for _, item := range answer {
		listed[k.identity(item)] = true
	}
	var lacking []T
	for _, item := range extras {
		if id := k.identity(item); k.usable(item) && !listed[id] {
			listed[id] = true
			lacking = append(lacking, item)
		}
	}
	if len(lacking) > 0 {
		merged.items = append(slices.Clip(answer), lacking...)
	}
	return merged
}

// listOf returns the list lists keeps under key, if it is still kept, and
// whether it is fresh: no older than the settings' VersionListMinutes.
func listOf[T any](s *Service, lists *cache.Cache[streamKey, list[T]], key streamKey) (l list[T], fresh, ok bool) {
	if l, ok = lists.Get(key); ok {
		if fresh, ok = l.state(s.now(), s.listLife()); ok {
			return l, fresh, true
		}
	}
	return list[T]{}, false, false
}

// keepAnswer stores answer, an addon's answer for a title, under key in
// lists, and returns the items listed: the answer, then the items of the
// list it replaces that it lacks. Those stay listed while the addon may
// still be gathering streams: an addon that gathers other addons' streams
// often answers first with part of them. For an answer followed up from
// now on (followed), they are every item of the list replaced, still kept,
// fresh as when a renewal asks again (see Renew), or stale; otherwise the
// items a fresh list kept that way, its follow-ups still running. The
// follow-ups' end drops them (see endFollowUp).
func keepAnswer[T any](s *Service, lists *cache.Cache[streamKey, list[T]], kind listKind[T], key streamKey, answer []T, followed bool) []T {
	now, life := s.now(), s.listLife()
	var stored list[T]
	lists.Update(key, func(kept list[T], ok bool) (list[T], bool) {
		var extras []T
		if ok {
			switch fresh, stillKept := kept.state(now, life); {
			case stillKept && followed:
				extras = kept.items
			case fresh:
				extras = kept.items[kept.answer:]
			}
		}
		stored = kind.merge(answer, extras, now)
		return stored, true
	})
	return stored.items
}

// followKey is a list an addon gave for a title.
type followKey struct {
	list string
	streamKey
}

// followUp is the schedule running for a list, told apart from the others
// by its address. hurry asks its next follow-up at once rather than after
// its delay, and asked is closed once its first follow-up answered, failed
// or was not asked, the schedule having ended (see Renew).
type followUp struct {
	hurry chan struct{}
	asked chan struct{}
	once  sync.Once
}

func newFollowUp() *followUp {
	return &followUp{hurry: make(chan struct{}, 1), asked: make(chan struct{})}
}

// answered closes f.asked, once.
func (f *followUp) answered() { f.once.Do(func() { close(f.asked) }) }

// followUps holds the schedule running for each list, at most one.
type followUps struct {
	mu      sync.Mutex
	running map[followKey]*followUp
}

// SetFollowUps replaces the delays of the follow-ups, 10 and 30 seconds,
// before the service is used. Tests shorten them, or give none to ask no
// addon again; the server keeps them.
func (s *Service) SetFollowUps(delays ...time.Duration) { s.followUpDelays = delays }

// followStreams stores the streams an addon just listed for a title under
// key, follows them up (see followUpList), and returns those listed. Each
// change of the list is saved, and told to whoever follows the title's
// versions (see streamsChanged).
func (s *Service) followStreams(ctx context.Context, entry installed, key streamKey, streams []stremio.Stream) []stremio.Stream {
	return followUpList(ctx, s, s.streamLists, streamKind, key, entry.addon.Manifest.Name, streams,
		func(ctx context.Context) ([]stremio.Stream, error) {
			return s.fetchStreams(ctx, entry, key.contentType, key.id)
		}, func() { s.streamsChanged(key) })
}

// followSubtitles stores the subtitles an addon just listed for a title
// under key, follows them up (see followUpList), and returns those listed.
func (s *Service) followSubtitles(ctx context.Context, entry installed, key streamKey, subtitles []stremio.Subtitle) []stremio.Subtitle {
	return followUpList(ctx, s, s.subtitleLists, subtitleKind, key, entry.addon.Manifest.Name, subtitles,
		func(ctx context.Context) ([]stremio.Subtitle, error) {
			return s.fetchSubtitles(ctx, entry, key.contentType, key.id)
		}, nil)
}

// followUpList stores answer, a list an addon just gave for a title, under
// key in lists (see keepAnswer), and returns the items listed. It then asks
// the addon again after each of the service's followUpDelays in turn,
// detached from ctx's end, as shared does. An answer that counts more than
// the previous answer replaces it, followed by the items still kept from
// the list the first answer replaced; the schedule stops at the first
// answer that does not, at an error, after the last delay, or once the
// list is no longer fresh: a refresh or its expiry forgot it, and a late
// answer never brings it back. The list just stored is a new first answer:
// its schedule replaces the one running for key, which stops. changed,
// unless nil, runs after the list is stored, after each answer that
// replaces it, and once the schedule ends.
func followUpList[T any](ctx context.Context, s *Service, lists *cache.Cache[streamKey, list[T]], kind listKind[T], key streamKey, addon string,
	answer []T, fetch func(context.Context) ([]T, error), changed func()) []T {
	notify := func() {
		if changed != nil {
			changed()
		}
	}
	delays := s.followUpDelays
	if len(delays) == 0 {
		items := keepAnswer(s, lists, kind, key, answer, false)
		notify()
		return items
	}
	// The schedule replaces the one running before the answer is stored:
	// the end of the one it replaces leaves the new list as it is.
	followed := followKey{kind.name, key}
	f := newFollowUp()
	s.followUps.mu.Lock()
	s.followUps.running[followed] = f
	s.followUps.mu.Unlock()
	items := keepAnswer(s, lists, kind, key, answer, true)
	notify()
	detached := context.WithoutCancel(ctx)
	flightKey := "follow-up " + kind.name + " " + key.addon.String() + " " + key.contentType + " " + key.id
	go func() {
		defer func() {
			endFollowUp(s, lists, followed, f)
			f.answered()
			notify()
		}()
		for _, delay := range delays {
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			case <-f.hurry:
				timer.Stop()
			}
			if _, fresh, _ := listOf(s, lists, key); !fresh || !s.followingUp(followed, f) {
				return
			}
			answer, err := shared(detached, &s.flight, flightKey, fetch)
			if err != nil {
				s.logger.Debug("An addon asked again for a list failed", "addon", addon, "list", kind.name, "error", err)
				return
			}
			now, life := s.now(), s.listLife()
			before, after := 0, kind.count(answer)
			// Checked and replaced together: a refresh that stops the
			// schedule first forgets the list after, or finds it stopped.
			s.followUps.mu.Lock()
			grew := s.followUps.running[followed] == f && lists.Update(key, func(kept list[T], ok bool) (list[T], bool) {
				if fresh, _ := kept.state(now, life); !ok || !fresh {
					return kept, false
				}
				// Compared with the previous answer: the items kept from
				// the list replaced are not the addon's.
				before = kind.count(kept.items[:kept.answer])
				if after <= before {
					return kept, false
				}
				return kind.merge(answer, kept.items[kept.answer:], now), true
			})
			s.followUps.mu.Unlock()
			if !grew {
				return
			}
			s.logger.Debug("An addon asked again listed more", "addon", addon, "list", kind.name, "before", before, "after", after)
			notify()
			f.answered()
		}
	}()
	return items
}

// hurryFollowUp has the next follow-up of an addon's streams for a title
// asked at once, and returns what is closed once the schedule's first
// follow-up is done; nil when no schedule runs for them.
func (s *Service) hurryFollowUp(key streamKey) <-chan struct{} {
	s.followUps.mu.Lock()
	f := s.followUps.running[followKey{listStreams, key}]
	s.followUps.mu.Unlock()
	if f == nil {
		return nil
	}
	select {
	case f.hurry <- struct{}{}:
	default:
	}
	return f.asked
}

// followingUp reports whether f is still the schedule of key.
func (s *Service) followingUp(key followKey, f *followUp) bool {
	s.followUps.mu.Lock()
	defer s.followUps.mu.Unlock()
	return s.followUps.running[key] == f
}

// endFollowUp ends f, the schedule of key, unless another replaced it. The
// list in lists is then its last answer alone: of the items kept from the
// list the first answer replaced, those the addon still lists are in it,
// and the others are no longer offered. A list dropped meanwhile stays
// dropped.
func endFollowUp[T any](s *Service, lists *cache.Cache[streamKey, list[T]], key followKey, f *followUp) {
	s.followUps.mu.Lock()
	defer s.followUps.mu.Unlock()
	if s.followUps.running[key] != f {
		return
	}
	delete(s.followUps.running, key)
	lists.Update(key.streamKey, func(kept list[T], ok bool) (list[T], bool) {
		if !ok || kept.answer == len(kept.items) {
			return kept, false
		}
		return list[T]{items: slices.Clip(kept.items[:kept.answer]), answer: kept.answer, at: kept.at}, true
	})
}

// stopFollowUps stops the schedules of an addon's lists for a title, which
// a refresh forgets.
func (s *Service) stopFollowUps(key streamKey) {
	s.followUps.mu.Lock()
	defer s.followUps.mu.Unlock()
	delete(s.followUps.running, followKey{listStreams, key})
	delete(s.followUps.running, followKey{listSubtitles, key})
}

// forgetLists forgets an addon's lists for a title, which a refresh asks
// for: their follow-ups stop, and the stream list saved is deleted too, so
// that a restart does not bring it back (see savedStreams).
func (s *Service) forgetLists(ctx context.Context, key streamKey) {
	s.stopFollowUps(key)
	s.streamLists.Delete(key)
	s.subtitleLists.Delete(key)
	s.deleteSavedStreams(ctx, key)
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
