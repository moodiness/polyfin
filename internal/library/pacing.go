package library

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Pacing the requests background work makes of addons.
//
// Background work asks addons for streams and descriptions in bursts: the
// titles of Continue Watching and Next Up listed ahead, the next episodes
// prepared or preloaded, the watchlists checked, descriptions refreshed or
// read in the users' languages, links renewed for trickplay images, and
// the follow-ups of every answer (see followUpList). Some addons limit the
// requests of each address, and refuse those beyond with a notice. So a
// request of background work (see Background) to an addon waits for its
// turn, by the server the addon is reached at, whose limits every addon
// it serves shares (see stremio.HostOf), and by resource: while fewer than
// paceAtOnce of the server's background requests for it are under way,
// and fewer than its pace's most were sent to the server within its
// window, the requests users wait for included. Those, opening a title,
// PlaybackInfo, a play, are never paced: they are sent at once, and count
// for the background requests that follow. A background request joins a
// request for the same list under way rather than waiting, and one that
// waited uses the answer another request brought meanwhile.
//
// A server that refuses a request for its rate, by HTTP 429, or 503 with
// Retry-After, or by notices alone saying so (see stremio.SaysRateLimited),
// has its background requests paused: as long as Retry-After asks, at most
// paceWaitMost, else paceWait. The pause is logged once. Requests users
// wait for are still sent meanwhile.

// paceKind is a resource whose background requests are paced.
type paceKind int

const (
	paceStreams paceKind = iota
	paceMetas
	paceKinds
)

// paceRate is how many requests of a kind a server is sent within window
// before background requests wait.
type paceRate struct {
	most   int
	window time.Duration
}

// paceRates leave room below a common limit, 10 stream requests per 15
// seconds and 15 descriptions per 5 seconds for each address, for the
// requests users make meanwhile.
var paceRates = [paceKinds]paceRate{
	paceStreams: {most: 8, window: 15 * time.Second},
	paceMetas:   {most: 12, window: 5 * time.Second},
}

const (
	// paceAtOnce is how many background requests of a kind are under way
	// to a server at once.
	paceAtOnce = 2
	// paceWait is how long a server's background requests pause after a
	// refusal that says no wait: a refused title is asked again after as
	// long (see noticeLife). paceWaitMost bounds every pause.
	paceWait     = time.Minute
	paceWaitMost = 15 * time.Minute
)

// pacing holds each server's pace.
type pacing struct {
	now func() time.Time
	// after returns what receives once d passed, and what stops it; tests
	// replace it with their clock's.
	after func(d time.Duration) (<-chan time.Time, func() bool)
	// rates are paceRates but in tests (see Service.SetPacing).
	rates [paceKinds]paceRate

	mu    sync.Mutex
	hosts map[string]*hostPace
}

// hostPace is a server's pace: until when its background requests pause,
// and each kind's requests.
type hostPace struct {
	until time.Time
	kinds [paceKinds]kindPace
	// changed is closed, and replaced, when a waiting request may go.
	changed chan struct{}
}

// kindPace holds the requests of a kind sent to a server within its
// window, the oldest first, the background ones under way, and those
// waiting, in their order.
type kindPace struct {
	sent    []time.Time
	running int
	queue   []*paceTurn
}

// paceTurn is a background request waiting for its turn; a later one
// lets the others go first (see laterBackground).
type paceTurn struct{ later bool }

func newPacing(now func() time.Time) *pacing {
	return &pacing{now: now, after: realAfter, rates: paceRates, hosts: map[string]*hostPace{}}
}

// SetPacing replaces, before the service is used, how many background
// requests of each kind a server is sent within window (see pacing).
// Tests of what background work asks, rather than of its pace, set a
// pace their requests never reach; the server keeps paceRates.
func (s *Service) SetPacing(most int, window time.Duration) {
	for kind := range s.pace.rates {
		s.pace.rates[kind] = paceRate{most: most, window: window}
	}
}

func realAfter(d time.Duration) (<-chan time.Time, func() bool) {
	timer := time.NewTimer(d)
	return timer.C, timer.Stop
}

// host returns host's pace, under p.mu.
func (p *pacing) host(host string) *hostPace {
	h, ok := p.hosts[host]
	if !ok {
		h = &hostPace{changed: make(chan struct{})}
		p.hosts[host] = h
	}
	return h
}

// change wakes the requests waiting for h, under p.mu.
func (h *hostPace) change() {
	close(h.changed)
	h.changed = make(chan struct{})
}

// trim forgets the requests sent before window.
func (k *kindPace) trim(now time.Time, window time.Duration) {
	i := 0
	for i < len(k.sent) && !now.Before(k.sent[i].Add(window)) {
		i++
	}
	k.sent = slices.Delete(k.sent, 0, i)
}

// wait waits for the turn of a background request of kind to host, after
// those waiting before it, and returns what ends it once answered. A
// later request waits behind every other, and one not later goes before
// the later ones waiting.
func (p *pacing) wait(ctx context.Context, host string, kind paceKind, later bool) (release func(), err error) {
	turn := &paceTurn{later: later}
	p.mu.Lock()
	h := p.host(host)
	k := &h.kinds[kind]
	at := len(k.queue)
	if i := slices.IndexFunc(k.queue, func(t *paceTurn) bool { return t.later }); i >= 0 && !later {
		at = i
	}
	k.queue = slices.Insert(k.queue, at, turn)
	for {
		now := p.now()
		k.trim(now, p.rates[kind].window)
		var left time.Duration
		ready := false
		switch {
		case k.queue[0] != turn:
			// The head goes first, and wakes the next once gone.
		case now.Before(h.until):
			left = h.until.Sub(now)
		case k.running >= paceAtOnce:
		case len(k.sent) >= p.rates[kind].most:
			left = k.sent[0].Add(p.rates[kind].window).Sub(now)
		default:
			ready = true
		}
		if ready {
			k.queue = k.queue[1:]
			k.sent = append(k.sent, now)
			k.running++
			h.change()
			p.mu.Unlock()
			var once sync.Once
			return func() {
				once.Do(func() {
					p.mu.Lock()
					k.running--
					h.change()
					p.mu.Unlock()
				})
			}, nil
		}
		changed := h.changed
		p.mu.Unlock()
		var timeout <-chan time.Time
		stop := func() bool { return false }
		if left > 0 {
			timeout, stop = p.after(left)
		}
		select {
		case <-changed:
		case <-timeout:
		case <-ctx.Done():
			stop()
			p.mu.Lock()
			k.queue = slices.DeleteFunc(k.queue, func(t *paceTurn) bool { return t == turn })
			h.change()
			p.mu.Unlock()
			return nil, ctx.Err()
		}
		stop()
		p.mu.Lock()
	}
}

// sent counts a request of kind sent to host at once, a user's.
func (p *pacing) sent(host string, kind paceKind) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	k := &p.host(host).kinds[kind]
	k.trim(now, p.rates[kind].window)
	k.sent = append(k.sent, now)
}

// refused pauses host's background requests for wait, unless a longer
// pause runs; started reports a pause that started.
func (p *pacing) refused(host string, wait time.Duration) (started bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	h := p.host(host)
	running := now.Before(h.until)
	if until := now.Add(min(wait, paceWaitMost)); until.After(h.until) {
		h.until = until
	}
	return !running
}

// paused reports whether host's background requests pause.
func (p *pacing) paused(host string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	h, ok := p.hosts[host]
	return ok && p.now().Before(h.until)
}

// paceMark tells how a context's requests are paced: those of background
// work wait for their turn, unless it came already (paced), after the
// others when later.
type paceMark struct {
	background, paced, later bool
}

type paceKey struct{}

// Background marks ctx as background work's, whose requests to addons are
// paced (see pacing): the work no user waits for, such as listing ahead
// the titles of home rows, preparing, preloading or checking watchlists.
func Background(ctx context.Context) context.Context {
	return context.WithValue(ctx, paceKey{}, paceMark{background: true})
}

// laterBackground marks ctx as background work's that lets other
// background work go first, as follow-ups do: a title's first answer
// matters more than a longer one.
func laterBackground(ctx context.Context) context.Context {
	return context.WithValue(ctx, paceKey{}, paceMark{background: true, later: true})
}

// foreground marks ctx as the work of a user waiting, its requests sent
// at once.
func foreground(ctx context.Context) context.Context {
	return context.WithValue(ctx, paceKey{}, paceMark{})
}

// withTurn marks ctx's background request as having had its turn.
func withTurn(ctx context.Context) context.Context {
	return context.WithValue(ctx, paceKey{}, paceMark{background: true, paced: true})
}

func markOf(ctx context.Context) paceMark {
	mark, _ := ctx.Value(paceKey{}).(paceMark)
	return mark
}

// paces reports whether a request to addon on ctx is to wait for its turn
// first: background work's to a Stremio addon, which did not have it yet.
func paces(ctx context.Context, addon addons.Addon) bool {
	mark := markOf(ctx)
	return mark.background && !mark.paced && addon.Stremio()
}

// turn waits, for a background request of kind to addon, for its turn
// (see pacing.wait), and returns ctx marked as having had it, with what
// ends it. Any other request goes at once.
func (s *Service) turn(ctx context.Context, addon addons.Addon, kind paceKind) (context.Context, func(), error) {
	if !paces(ctx, addon) {
		return ctx, func() {}, nil
	}
	release, err := s.pace.wait(ctx, stremio.HostOf(addon.ManifestURL), kind, markOf(ctx).later)
	if err != nil {
		return ctx, nil, err
	}
	return withTurn(ctx), release, nil
}

// request makes a request of kind to a Stremio addon: background work's
// waits for its turn unless it had it (see turn), any other is counted
// as sent at once. A refusal for the rate pauses the server's background
// requests (see refusedFor).
func request[T any](ctx context.Context, s *Service, addon addons.Addon, kind paceKind, ask func(context.Context) (T, error)) (T, error) {
	mark := markOf(ctx)
	switch {
	case mark.paced:
	case mark.background:
		var release func()
		var err error
		if ctx, release, err = s.turn(ctx, addon, kind); err != nil {
			var zero T
			return zero, err
		}
		defer release()
	default:
		s.pace.sent(stremio.HostOf(addon.ManifestURL), kind)
	}
	value, err := ask(ctx)
	if wait, refused := refusal(err); refused {
		s.refusedFor(addon, wait)
	}
	return value, err
}

// refusal tells whether err is an addon's refusal of a request for its
// rate, and how long it asked to wait.
func refusal(err error) (time.Duration, bool) {
	if err == nil || errors.Is(err, stremio.ErrAskedToWait) {
		return 0, false
	}
	if wait, said := stremio.RetryAfter(err); said {
		return wait, true
	}
	if stremio.StatusOf(err) == http.StatusTooManyRequests || errors.Is(err, stremio.ErrRateLimited) {
		return paceWait, true
	}
	return 0, false
}

// refusedNotices tells whether notices, an addon's answer of notices
// alone, refuse the request for its rate.
func refusedNotices(notices []stremio.Stream) bool {
	return slices.ContainsFunc(notices, func(n stremio.Stream) bool { return stremio.SaysRateLimited(n.Name, n.Title, n.Description) })
}

// refusedFor pauses the background requests to addon's server for wait
// (see pacing.refused), and logs a pause that starts.
func (s *Service) refusedFor(addon addons.Addon, wait time.Duration) {
	if s.pace.refused(stremio.HostOf(addon.ManifestURL), wait) {
		s.logger.Info("An addon refused a request for its rate: Polyfin's background requests to its server pause", "addon",
			addon.Manifest.Name, "wait", min(wait, paceWaitMost))
	}
}
