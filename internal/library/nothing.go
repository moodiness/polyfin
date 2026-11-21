package library

import (
	"context"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/stremio"
)

// Addons that have nothing for a title.
//
// An addon's answer with nothing to play for a title tells that it has
// nothing for it now: an empty answer, kept as its list, or notices alone
// that refuse nothing, such as its summary of the streams its filters left
// out, or "no streams found". Notices alone that tell a refusal for the
// rate of requests, as Health marks them (see limitedNotices), are no such
// answer: they are remembered noticeLife only, and the server's background
// requests pause (see pacing).
//
// Notices alone are not kept as a list, but remembered as long as an
// empty list is fresh, for the version list life: whoever asks meanwhile
// gets them again, or the list kept that has streams to play, without
// asking the addon (see rememberedAnswer). Background work (see
// Background) leaves the addon unasked for the title longer, after either
// answer: nothingFirst after the first, twice as long after each further
// one, nothingMost at most, so that the preload passes, every 15 minutes
// all night, the watchlist checks and the home rows listed ahead do not
// ask it again and again. They get the answer remembered meanwhile, or the
// list kept.
//
// A user's requests ask as before once the answer is no longer fresh, and
// a user opening or playing the title has the waits start again from
// nothingFirst (see userListing). An answer with something to play forgets
// what was remembered, and so do a refresh of the title, Try again, and the
// addon's version lists forgotten: Refresh everything, its address
// replaced, or the addon removed. Its manifest read again, by Refresh or
// the daily task Refresh the sources, has background work ask it again at
// once, its waits growing on: a daily reset would ask it 5 times a day.
// It is kept in memory only, nothingKept titles and nothingBytes of
// notices at most, nothingLife after the latest answer: a restart costs
// one request for each title.

const (
	// nothingFirst is how long background work leaves an addon unasked
	// for a title after its first answer with nothing to play, and
	// nothingMost the longest such wait.
	nothingFirst = time.Hour
	nothingMost  = 24 * time.Hour
	// nothingLife is how long an answer with nothing to play is
	// remembered: past the longest wait, so that the waits keep growing
	// to nothingMost, and stay there.
	nothingLife = 2 * nothingMost
	// nothingKept bounds the titles remembered, and nothingBytes the
	// memory their notices may take.
	nothingKept  = maxTitles
	nothingBytes = 8 << 20
)

// nothingAnswer is an addon's latest answer with nothing to play for a
// title: its notices, nil for an empty answer, which is kept as its list;
// when it came; how long background work waits after it; and the address
// and the manifest read of the addon then (see absence), with which it
// holds. A wait of 0 is the waits started again, a user having opened or
// played the title: background work waits nothingFirst, and after the
// next answer too.
type nothingAnswer struct {
	notices []stremio.Stream
	at      time.Time
	wait    time.Duration
	addon   absence
}

// nothingNow remembers an addon's answer with nothing to play for a
// title, notices alone or nil for an empty answer, with s.followUps.mu
// held. Background work waits nothingFirst after it, or twice as long as
// after the addon's answer before, nothingMost at most.
func (s *Service) nothingNow(entry installed, key streamKey, notices []stremio.Stream) {
	now, addon := s.now(), absenceOf(entry)
	s.tracked().nothing.Update(key, func(kept nothingAnswer, ok bool) (nothingAnswer, bool) {
		wait := nothingFirst
		if ok && kept.wait > 0 {
			wait = min(2*kept.wait, nothingMost)
		}
		return nothingAnswer{notices: notices, at: now, wait: wait, addon: addon}, true
	})
}

// streamsListed is told that request asked (see ask) for an addon's
// streams for a title stored its answer: an empty one is remembered (see
// nothingNow), and one with streams forgets what was. A request a refresh
// forgot meanwhile tells nothing.
func (s *Service) streamsListed(entry installed, key streamKey, asked uint64, empty bool) {
	s.followUps.mu.Lock()
	defer s.followUps.mu.Unlock()
	switch {
	case s.followUps.asking[followKey{listStreams, key}] != asked:
	case empty:
		s.nothingNow(entry, key, nil)
	default:
		s.tracked().nothing.Delete(key)
	}
}

// nothingFor returns an addon's latest answer with nothing to play for a
// title, while it holds for a request on ctx to the addon as it is now:
// as long as a list is fresh, and for background work until its wait
// ends.
func (s *Service) nothingFor(ctx context.Context, entry installed, key streamKey) (nothingAnswer, bool) {
	nothing, ok := s.tracked().nothing.Get(key)
	if !ok || nothing.addon != absenceOf(entry) {
		return nothing, false
	}
	age := s.now().Sub(nothing.at)
	return nothing, age <= s.listLife() || markOf(ctx).background && age < max(nothing.wait, nothingFirst)
}

// userListing is told that a user opens or plays a title the addons of
// serving list streams for: the waits of background work for them start
// again from nothingFirst.
func (s *Service) userListing(serving []streamSource) {
	nothing := s.tracked().nothing
	for _, source := range serving {
		nothing.Update(source.key(), func(kept nothingAnswer, ok bool) (nothingAnswer, bool) {
			waited := ok && kept.wait > 0
			kept.wait = 0
			return kept, waited
		})
	}
}

// limitedNotices tells whether notices, an addon's answer of notices
// alone, refuse the request for its rate as Health marks them Too many
// requests (see stremio.NoticeRateLimited), which includes what pauses
// the server's background requests (see refusedNotices).
func limitedNotices(notices []stremio.Stream) bool {
	return slices.ContainsFunc(notices, func(n stremio.Stream) bool { return stremio.NoticeRateLimited(n.NoticeText()) })
}
