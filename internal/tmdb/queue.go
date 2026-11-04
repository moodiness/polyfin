package tmdb

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// Background lookups.
//
// What Polyfin learns from TMDB ahead of requests, such as the collections
// of movies (see the boxset package), the credits of titles (see the
// library package's titlecredits.go) and the artwork of titles whose
// addon gives none (see its tmdbartwork.go), waits its turn in one Queue,
// so that together they respect TMDB's rate. Two kinds of lookups wait.
// Passes (see Queue.Pass) queue those of the first titles of the server's
// libraries, once a key is saved, then each passEvery, which finds the
// answers grown old. A lookup a request asks for, a title's page, a search
// or an image, goes first, ahead of the passes'.
//
// The lookups are made one at a time, at most one every interval, each one
// or two requests to TMDB, which the client bounds with the server's other
// requests to it (see Client.Hold). A lookup TMDB refuses for its rate
// pauses them all as long as its Retry-After asks, else rateWait, and is
// tried again first. At most maxPending lookups wait their turn; others
// are queued again when next asked for. Without a key, nothing is looked
// up, and the lookups waiting are dropped.

const (
	// interval is the least time between the starts of two lookups: at
	// most four lookups a second, well below TMDB's rate, leaving room for
	// the server's other requests to it.
	interval = 250 * time.Millisecond
	// rateWait is how long the lookups pause after TMDB refused one for
	// its rate without saying how long; rateWaitMost bounds every pause.
	rateWait     = 10 * time.Second
	rateWaitMost = 5 * time.Minute
	// maxPending bounds the lookups waiting their turn.
	maxPending = 20000
	// lookupTime bounds one lookup.
	lookupTime = time.Minute
	// passEvery is how often a pass runs while a key is saved, and
	// passCheck how often the passes look whether one is due.
	passEvery = 24 * time.Hour
	passCheck = time.Minute
)

// Lookup is a lookup waiting in a Queue: Key tells it apart, so that it
// waits once however often it is asked for; Run makes it, and fails with
// RateLimited when TMDB refuses it for its rate.
type Lookup struct {
	Key string
	Run func(context.Context) error
}

// QueueOptions configure a Queue; their zero values are the defaults.
type QueueOptions struct {
	// Interval is the least time between the starts of two lookups, and
	// PassCheck how often the passes look whether one is due.
	Interval  time.Duration
	PassCheck time.Duration
	Logger    *slog.Logger
}

// Queue makes background lookups on TMDB, in turn. Its zero value is not
// ready: use NewQueue.
type Queue struct {
	client    *Client
	logger    *slog.Logger
	interval  time.Duration
	passCheck time.Duration

	mu sync.Mutex
	// asked are the lookups requests wait for, before those of passes,
	// ahead; queued holds both, and the one under way.
	asked, ahead []string
	queued       map[string]func(context.Context) error
	busy         bool
	// changed is closed, and replaced, whenever the queue changes.
	changed chan struct{}
	wake    chan struct{}
	ctx     context.Context
	stop    context.CancelFunc
	done    sync.WaitGroup
}

// NewQueue returns a queue of lookups asking client, and starts making
// them; Close stops it.
func NewQueue(client *Client, options QueueOptions) *Queue {
	if options.Interval <= 0 {
		options.Interval = interval
	}
	if options.PassCheck <= 0 {
		options.PassCheck = passCheck
	}
	if options.Logger == nil {
		options.Logger = slog.New(slog.DiscardHandler)
	}
	ctx, stop := context.WithCancel(context.Background())
	q := &Queue{client: client, logger: options.Logger, interval: options.Interval, passCheck: options.PassCheck,
		queued: map[string]func(context.Context) error{}, changed: make(chan struct{}), wake: make(chan struct{}, 1), ctx: ctx, stop: stop}
	q.done.Go(q.run)
	return q
}

// Close stops the lookups and passes, and waits for those under way.
func (q *Queue) Close() {
	if q == nil {
		return
	}
	q.stop()
	q.done.Wait()
}

// Enabled reports whether lookups are made: while the server's TMDB key is
// saved.
func (q *Queue) Enabled() bool { return q != nil && q.client.Enabled() }

// Add queues lookups, each once, up to maxPending: a request's (asked)
// ahead of those of passes, a lookup a pass queued moving ahead when a
// request asks for it.
func (q *Queue) Add(lookups []Lookup, asked bool) {
	if len(lookups) == 0 || !q.Enabled() {
		return
	}
	q.mu.Lock()
	added := false
	for _, l := range lookups {
		if _, ok := q.queued[l.Key]; ok {
			if asked {
				if i := index(q.ahead, l.Key); i >= 0 {
					q.ahead = append(q.ahead[:i], q.ahead[i+1:]...)
					q.asked = append(q.asked, l.Key)
					added = true
				}
			}
			continue
		}
		if len(q.asked)+len(q.ahead) >= maxPending {
			continue
		}
		q.queued[l.Key] = l.Run
		if asked {
			q.asked = append(q.asked, l.Key)
		} else {
			q.ahead = append(q.ahead, l.Key)
		}
		added = true
	}
	if added {
		q.changedLocked()
	}
	q.mu.Unlock()
	if added {
		select {
		case q.wake <- struct{}{}:
		default:
		}
	}
}

func index(keys []string, key string) int {
	for i, k := range keys {
		if k == key {
			return i
		}
	}
	return -1
}

// Wait waits, at most for d, until the lookups of keys, those queued, are
// made.
func (q *Queue) Wait(ctx context.Context, keys []string, d time.Duration) {
	if !q.Enabled() {
		return
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	for {
		q.mu.Lock()
		waiting := false
		for _, key := range keys {
			if _, ok := q.queued[key]; ok {
				waiting = true
			}
		}
		changed := q.changed
		q.mu.Unlock()
		if !waiting {
			return
		}
		select {
		case <-changed:
		case <-timer.C:
			return
		case <-ctx.Done():
			return
		}
	}
}

// changedLocked tells those waiting for the queue that it changed.
func (q *Queue) changedLocked() {
	close(q.changed)
	q.changed = make(chan struct{})
}

// Settled waits until no lookup is waiting or under way.
func (q *Queue) Settled(ctx context.Context) error {
	for {
		q.mu.Lock()
		idle, changed := len(q.asked)+len(q.ahead) == 0 && !q.busy, q.changed
		q.mu.Unlock()
		if idle {
			return nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// next takes the first lookup waiting, a request's first, and marks the
// queue busy.
func (q *Queue) next() (string, func(context.Context) error, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	var key string
	switch {
	case len(q.asked) > 0:
		key, q.asked = q.asked[0], q.asked[1:]
	case len(q.ahead) > 0:
		key, q.ahead = q.ahead[0], q.ahead[1:]
	default:
		return "", nil, false
	}
	q.busy = true
	q.changedLocked()
	return key, q.queued[key], true
}

// finished ends a lookup of key: again puts it back first in line, among
// a request's.
func (q *Queue) finished(key string, again bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if again {
		q.asked = append([]string{key}, q.asked...)
	} else {
		delete(q.queued, key)
	}
	q.busy = false
	q.changedLocked()
}

// drop forgets every lookup waiting: without a key, none is made.
func (q *Queue) drop() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.asked, q.ahead, q.queued = nil, nil, map[string]func(context.Context) error{}
	q.changedLocked()
}

// run makes the lookups waiting, one at a time, at most one every
// interval, until the queue is closed.
func (q *Queue) run() {
	ctx := q.ctx
	var last time.Time
	for {
		key, lookUp, ok := q.next()
		if !ok {
			select {
			case <-q.wake:
				continue
			case <-ctx.Done():
				return
			}
		}
		if !q.Enabled() {
			q.finished(key, false)
			q.drop()
			continue
		}
		if wait := q.interval - time.Since(last); wait > 0 {
			if !sleep(ctx, wait) {
				q.finished(key, false)
				return
			}
		}
		last = time.Now()
		lookup, cancel := context.WithTimeout(ctx, lookupTime)
		err := lookUp(lookup)
		cancel()
		var limited *RateLimited
		switch {
		case errors.As(err, &limited):
			pause := min(max(limited.RetryAfter, 0), rateWaitMost)
			if pause == 0 {
				pause = rateWait
			}
			q.logger.Info("TMDB asked to slow down: its background lookups wait", "wait", pause)
			q.finished(key, true)
			if !sleep(ctx, pause) {
				return
			}
			continue
		case err != nil && ctx.Err() == nil:
			q.logger.Debug("A background lookup on TMDB failed", "lookup", key, "error", err)
		}
		q.finished(key, false)
	}
}

// Pass runs pass when one is due, until the queue is closed: while a key
// is saved, the first time, and passEvery after the last, a failed one an
// hour after. pass queues its lookups (see Add), after a request's; name
// tells it in the log.
func (q *Queue) Pass(name string, pass func(context.Context) error) {
	q.done.Go(func() {
		ctx := q.ctx
		var last time.Time
		for {
			// The first check waits passCheck too: the server is starting.
			if !sleep(ctx, q.passCheck) {
				return
			}
			if q.Enabled() && (last.IsZero() || time.Since(last) >= passEvery) {
				if err := pass(ctx); err != nil {
					if ctx.Err() != nil {
						return
					}
					q.logger.Warn("A pass of TMDB lookups failed", "pass", name, "error", err)
					last = time.Now().Add(time.Hour - passEvery)
				} else {
					last = time.Now()
				}
			}
			if !q.Enabled() {
				last = time.Time{}
			}
		}
	})
}

// sleep waits for d, and reports false when ctx ended first.
func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
