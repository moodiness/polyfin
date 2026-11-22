package library

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Listing home rows' titles ahead.
//
// With playback prepared ahead, the first titles of the home rows an app
// asks for have their stream lists asked of the addons before anyone opens
// them (see ListRowAhead): a first open then lists its versions at once
// rather than waiting for the addons, a second and a half or more for an
// addon gathering other addons' streams. Only the lists are asked: no
// version is analyzed, no byte of a file read. Each addon is asked within
// bounds of its own, which the listings of every user share, a user's own
// addon having its own:
//   - one request at a time, rowAheadPerMinute within a minute and
//     rowAheadPerHour within an hour, the follow-ups of its answers
//     included (see followUpList), past which the titles waiting are
//     dropped;
//   - the titles first in their rows first, whatever the row;
//   - as background work that lets the other background work go first
//     (see laterBackground), the requests users wait for, opening or
//     playing a title, going at once and counting in the pace of the
//     server (see pacing);
//   - not for a title whose list is kept, in memory or in the database,
//     younger than the version list life, nor one the addon failed to
//     list lately, had nothing for (see nothingAnswer), or is asked for
//     already;
//   - not while its server's background requests pause (see pacing), as
//     after a refusal for its rate, which drops at once the titles
//     waiting for the addon, nor while its server asked to wait (see
//     catalogWaits).

const (
	// rowAheadPerMinute and rowAheadPerHour bound the requests an addon is
	// sent within a minute and an hour to list home rows' titles ahead:
	// well below the pace of the server's background requests (see
	// paceRates), and a few dozen titles an hour, their follow-ups
	// included.
	rowAheadPerMinute = 12
	rowAheadPerHour   = 120
	// rowAheadQueued bounds the titles waiting for an addon: the first
	// three of a dozen rows, with room for a second user.
	rowAheadQueued = 72
	// rowAheadTimeout bounds a title's request, its wait for its turn
	// included.
	rowAheadTimeout = 2 * time.Minute
)

// RowTitle is a movie or an episode of a home row, and its place in the
// row, 0 for the first.
type RowTitle struct {
	ID    accounts.ID
	Place int
}

// ListRowAhead asks, in the background, the user's addons for the streams
// of titles, the first of a home row, as far as the bounds of each addon
// allow (see rowAhead). Titles other than movies and episodes, and movies
// waiting for their digital release, are left out. It returns once the
// titles are queued, reading what Polyfin keeps of them, never waiting for
// an addon.
func (s *Service) ListRowAhead(ctx context.Context, user accounts.User, titles []RowTitle) {
	ctx = Background(ctx)
	for _, title := range titles {
		sources, err := s.rowAheadSources(ctx, user, title.ID)
		if err != nil {
			s.logger.Debug("A home row's title could not be listed ahead", "error", err)
			continue
		}
		for _, source := range sources {
			s.rowAhead.add(source, title.Place)
		}
	}
}

// rowAheadSources lists the addons a title's streams are to be asked of
// ahead: the Stremio addons that list streams for it, but those whose
// lists need no request now (see rowAheadNeedless).
func (s *Service) rowAheadSources(ctx context.Context, user accounts.User, id accounts.ID) ([]streamSource, error) {
	t, v, err := s.target(ctx, user, id)
	if err != nil || t.kind != KindMovie && t.kind != KindEpisode {
		return nil, err
	}
	if t.kind == KindMovie {
		if _, held, err := s.upcoming(ctx, v, id); err != nil || held {
			return nil, err
		}
	}
	// What the title's description embeds is read as it is kept: a list
	// ahead asks for no description.
	s.embed(ctx, v, &t, false)
	serving := s.streamServing(ctx, v, t)
	return slices.DeleteFunc(serving, func(source streamSource) bool {
		return !source.addon.Stremio() || len(t.embedded) > 0 && source.addon.ID == t.embeddedBy.addon.ID || s.rowAheadNeedless(ctx, source)
	}), nil
}

// rowAheadNeedless reports whether a title's list from an addon is not to
// be asked ahead now: its server's background requests pause or its server
// asked to wait, the addon failed lately or had nothing for the title,
// it is asked for the title already, or its list, in memory or in the
// database, is younger than the version list life. A list restored from
// the database after a restart counts: the title's page lists it at once.
func (s *Service) rowAheadNeedless(ctx context.Context, source streamSource) bool {
	key := source.key()
	if s.rowAheadPaused(stremio.HostOf(source.addon.ManifestURL)) || s.failed(key) || s.askingFor(followKey{listStreams, key}) {
		return true
	}
	if _, refused := s.noticed.Get(key); refused {
		return true
	}
	if _, nothing := s.nothingFor(ctx, source.installed, key); nothing {
		return true
	}
	l, _, ok := s.streamListOf(ctx, key)
	return ok && s.now().Sub(l.at) <= s.listLife()
}

// WaitRowAhead waits until no title queued by ListRowAhead waits or is
// asked, for tests to count what home rows asked.
func (s *Service) WaitRowAhead() {
	s.rowAhead.workers.Wait()
}

// rowAheadPaused reports whether host's server is asked nothing ahead
// now: its background requests pause (see pacing), or it asked to wait
// (see catalogWaits).
func (s *Service) rowAheadPaused(host string) bool {
	_, waits := s.catalogWaitLeft(host)
	return waits || s.pace.paused(host)
}

// askRowAhead asks an addon for a title's streams ahead (see
// rowAheadContext).
func (s *Service) askRowAhead(source streamSource) {
	ctx, cancel := rowAheadContext(s.lifetime)
	defer cancel()
	if _, err := s.streams(ctx, source.installed, source.contentType, source.id); err != nil {
		s.logger.Debug("An addon could not list a home row's title ahead", "addon", source.addon.LogName(), "error", err)
	}
}

// rowAheadContext returns the context of a list ahead's request, from
// parent, the server's: background work that lets the other background
// work go first, counted as home rows' work, marked as a list ahead's for
// its follow-ups to keep within the addon's bounds (see
// rowAhead.followUp), within rowAheadTimeout.
func rowAheadContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(withRowAhead(laterBackground(stremio.WithOrigin(parent, stremio.OriginCatalogs))), rowAheadTimeout)
}

// rowAheadKey marks a context as a list ahead's (see withRowAhead).
type rowAheadKey struct{}

// withRowAhead marks ctx as a list ahead's: the follow-ups of its answer
// are within the addon's bounds (see rowAhead.followUp).
func withRowAhead(ctx context.Context) context.Context {
	return context.WithValue(ctx, rowAheadKey{}, true)
}

func rowAheadOf(ctx context.Context) bool {
	marked, _ := ctx.Value(rowAheadKey{}).(bool)
	return marked
}

// rowAhead queues the titles listed ahead, by addon, and asks them within
// each addon's bounds (see the top of this file). Its functions are the
// service's, but in tests: now its clock, paused whether a server is asked
// nothing ahead (see Service.rowAheadPaused), needless whether a title's
// list is not to be asked (see Service.rowAheadNeedless), and ask asks it.
type rowAhead struct {
	now      func() time.Time
	paused   func(host string) bool
	needless func(source streamSource) bool
	ask      func(source streamSource)
	logger   *slog.Logger

	mu     sync.Mutex
	addons map[accounts.ID]*addonAhead
	// workers counts the addons' workers running, for tests to wait for.
	workers sync.WaitGroup
}

// addonAhead is an addon's listings ahead: its server's address, the
// titles waiting, in their order, the requests sent within the hour, the
// oldest first, and whether a worker runs. slot holds the request under
// way, one at most.
type addonAhead struct {
	host    string
	queue   []queuedAhead
	queued  map[streamKey]bool
	sent    []time.Time
	working bool
	slot    chan struct{}
}

// queuedAhead is a title's list from an addon waiting to be asked, and the
// title's place in its row.
type queuedAhead struct {
	source streamSource
	place  int
}

func newRowAhead(s *Service) *rowAhead {
	return &rowAhead{
		now:      func() time.Time { return s.now() },
		paused:   s.rowAheadPaused,
		needless: func(source streamSource) bool { return s.rowAheadNeedless(Background(context.Background()), source) },
		ask:      s.askRowAhead,
		logger:   s.logger,
		addons:   map[accounts.ID]*addonAhead{},
	}
}

// addon returns the listings ahead of an addon at host, under r.mu. The
// addons that sent nothing within the hour and have nothing to do are
// forgotten meanwhile, a removed addon's among them.
func (r *rowAhead) addon(id accounts.ID, host string) *addonAhead {
	a, ok := r.addons[id]
	if !ok {
		now := r.now()
		for other, idle := range r.addons {
			if idle.trim(now); !idle.working && len(idle.sent) == 0 {
				delete(r.addons, other)
			}
		}
		a = &addonAhead{queued: map[streamKey]bool{}, slot: make(chan struct{}, 1)}
		r.addons[id] = a
	}
	a.host = host
	return a
}

// trim forgets the requests sent more than an hour ago.
func (a *addonAhead) trim(now time.Time) {
	i := 0
	for i < len(a.sent) && now.Sub(a.sent[i]) >= time.Hour {
		i++
	}
	a.sent = slices.Delete(a.sent, 0, i)
}

// add queues a title's list from an addon, place its place in its row,
// unless it waits already, or rowAheadQueued wait, and starts the addon's
// worker if none runs.
func (r *rowAhead) add(source streamSource, place int) {
	key := source.key()
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.addon(source.addon.ID, stremio.HostOf(source.addon.ManifestURL))
	if a.queued[key] || len(a.queue) >= rowAheadQueued {
		return
	}
	a.queued[key] = true
	a.queue = append(a.queue, queuedAhead{source: source, place: place})
	if !a.working {
		a.working = true
		r.workers.Go(func() { r.work(a) })
	}
}

// next takes the title an addon is asked for next, the first of those
// nearest the start of their rows, under r.mu.
func (a *addonAhead) next() queuedAhead {
	first := 0
	for i, q := range a.queue {
		if q.place < a.queue[first].place {
			first = i
		}
	}
	q := a.queue[first]
	a.queue = slices.Delete(a.queue, first, first+1)
	delete(a.queued, q.source.key())
	return q
}

// drop drops the titles waiting for an addon, under r.mu.
func (a *addonAhead) drop() {
	a.queue = nil
	clear(a.queued)
}

// work asks an addon for the titles queued, one at a time, until none is
// left, within its bounds (see turn). Once its server's background
// requests pause, as after a refusal for its rate, or its hour's requests
// were sent, the titles waiting are dropped: the rows asked for later
// queue theirs again.
func (r *rowAhead) work(a *addonAhead) {
	for {
		r.mu.Lock()
		if len(a.queue) == 0 {
			a.working = false
			r.mu.Unlock()
			return
		}
		q := a.next()
		r.mu.Unlock()
		if r.needless(q.source) {
			continue
		}
		release, ok, _ := r.turn(context.Background(), a, nil)
		if !ok {
			r.stop(a, q.source)
			continue
		}
		// The list may have come meanwhile, as when the title was opened.
		if r.needless(q.source) {
			release()
			continue
		}
		r.spend(a)
		r.ask(q.source)
		release()
		if r.paused(stremio.HostOf(q.source.addon.ManifestURL)) {
			r.stop(a, q.source)
		}
	}
}

// stop drops the titles waiting for an addon whose server is asked
// nothing ahead now, or whose hour's requests were sent.
func (r *rowAhead) stop(a *addonAhead, source streamSource) {
	r.mu.Lock()
	dropped := len(a.queue)
	a.drop()
	r.mu.Unlock()
	r.logger.Debug("An addon lists no more home row titles ahead for now", "addon", source.addon.LogName(), "dropped", dropped)
}

// turn waits for an addon's turn to be asked ahead: no other request ahead
// to it under way, and fewer than rowAheadPerMinute sent within a minute.
// It returns what ends the turn, or false, at once, when the addon is not
// to be asked ahead now: its server is asked nothing ahead (see paused),
// rowAheadPerHour requests were sent within the hour, or ctx ended; also,
// with hurried, once something comes on hurry meanwhile. The request asked
// in the turn is counted by spend.
func (r *rowAhead) turn(ctx context.Context, a *addonAhead, hurry <-chan struct{}) (release func(), ok, hurried bool) {
	select {
	case a.slot <- struct{}{}:
	case <-hurry:
		return nil, false, true
	case <-ctx.Done():
		return nil, false, false
	}
	release = func() { <-a.slot }
	for {
		r.mu.Lock()
		now := r.now()
		a.trim(now)
		if r.paused(a.host) || len(a.sent) >= rowAheadPerHour {
			r.mu.Unlock()
			release()
			return nil, false, false
		}
		minute := len(a.sent)
		for minute > 0 && now.Sub(a.sent[minute-1]) < time.Minute {
			minute--
		}
		if len(a.sent)-minute < rowAheadPerMinute {
			r.mu.Unlock()
			return release, true, false
		}
		wait := a.sent[minute].Add(time.Minute).Sub(now)
		r.mu.Unlock()
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
			continue
		case <-hurry:
			hurried = true
		case <-ctx.Done():
		}
		timer.Stop()
		release()
		return nil, false, hurried
	}
}

// spend counts a request sent to an addon in its turn.
func (r *rowAhead) spend(a *addonAhead) {
	r.mu.Lock()
	a.sent = append(a.sent, r.now())
	r.mu.Unlock()
}

// followUp waits for the turn of a follow-up of an addon's answer to a
// list ahead (see followUpList), which counts as a request ahead, and
// returns what ends it; false when the addon is not to be asked ahead now
// (see turn), which ends the follow-ups. A play renewing a link hurries
// the follow-up (see hurryFollowUp): something on hurry while it waits
// ends the wait, with hurried, and the follow-up is asked at once, out of
// the addon's bounds, as the play's.
func (r *rowAhead) followUp(ctx context.Context, addon accounts.ID, hurry <-chan struct{}) (release func(), ok, hurried bool) {
	r.mu.Lock()
	a, known := r.addons[addon]
	r.mu.Unlock()
	if !known {
		return nil, false, false
	}
	if release, ok, hurried = r.turn(ctx, a, hurry); ok {
		r.spend(a)
	}
	return release, ok, hurried
}
