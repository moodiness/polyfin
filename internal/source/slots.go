package source

import (
	"context"
	"slices"
	"time"
)

// The connections the server opens to a host, over every file it reads
// there, are bounded: a host, such as a debrid proxy, refuses a burst of
// them (429), which would fail the playback that needs one. Background
// connections wait for one of their host's slots: those of a source no
// playback urges (see Source.Urge), as warms, reads ahead of a play to
// come and analyses of versions not chosen, a file's connections past its
// first, which are not opened when no slot is free, and the requests read
// once (see Source.Once). A playback's connection, the first of a source
// urged, never waits: it takes a slot at once, and background connections
// give theirs up for it, as a lingering one does for any connection
// waiting; so do the short requests of Fetch and FetchRanges, which are
// never given up.

const (
	// hostConnections bounds the connections to a host, and
	// crowdedConnections those to a host that answered 429 or 503 lately,
	// for crowdedFor.
	hostConnections    = 4
	crowdedConnections = 2
	crowdedFor         = 10 * time.Minute
	// yieldWait bounds how long a playback's connection waits for the
	// background ones giving their slots up for it to close.
	yieldWait = time.Second
)

// slot is a connection's place among those its host may have.
type slot struct {
	host string
	// ctx is the connection's while it holds the slot, canceled once it is
	// to give the slot up.
	ctx    context.Context
	cancel context.CancelFunc
	claim  claim
	// idle and yielding, that the connection lingers and that it is to
	// give the slot up, are guarded by the cache's slotsMu.
	idle, yielding bool
}

// claim is what asks for a slot: state tells, each time it is asked,
// whether it reads for a playback, and whether it is to read nothing more;
// nudge and done wake it, waiting, to ask again. extra marks a connection
// more of a file, firm a request too short to be given up.
type claim struct {
	state       func() (playback, gone bool)
	nudge, done <-chan struct{}
	extra, firm bool
}

// yields reports whether the slot is given up for a playback's: not a
// playback's itself, nor firm. c.slotsMu is held.
func (sl *slot) yields() bool {
	if sl.claim.firm {
		return false
	}
	playback, _ := sl.claim.state()
	return !playback
}

// hostSlots are the slots of a host held.
type hostSlots struct {
	held []*slot
	// urgent counts the playbacks' connections waiting for background ones
	// to give their slots up: those are theirs, not background ones'.
	urgent int
	// changed is closed, and replaced, when a slot is given back or lingers.
	changed chan struct{}
}

// slotCap is how many connections host may have now.
func (c *Cache) slotCap(host string) int {
	c.learned.Lock()
	defer c.learned.Unlock()
	if state := c.hosts[host]; state != nil && time.Now().Before(state.crowded) {
		return max(1, min(c.hostConnections, crowdedConnections))
	}
	return max(1, c.hostConnections)
}

// crowd gives a host that answered 429 or 503 fewer connections for a
// while.
func (c *Cache) crowd(host string) {
	c.learned.Lock()
	defer c.learned.Unlock()
	state := c.host(host)
	if !time.Now().Before(state.crowded) {
		c.logger.Debug("A host is read over fewer connections", "host", host, "connections", crowdedConnections)
	}
	state.crowded = time.Now().Add(crowdedFor)
}

// roomFor reports whether host has a slot free, as last known: a
// connection more of a file is not opened otherwise.
func (c *Cache) roomFor(host string) bool {
	c.learned.Lock()
	defer c.learned.Unlock()
	state := c.hosts[host]
	return state == nil || !state.full
}

// acquire returns a slot of host for what cl claims it, reading s, nil
// once it is to read nothing more. A playback's claim takes one at once,
// the background ones over the bound giving theirs up, which it waits for
// a moment at most. A background one waits for a slot free, having a
// lingering one given up, unless it is a connection more of its file:
// then nil when none is free.
func (c *Cache) acquire(s *Source, host string, cl claim) *slot {
	var deadline time.Time
	// counted is the slots cl counts among the urgent ones, while it
	// waits as a playback's. c.slotsMu is held to change it.
	var counted *hostSlots
	uncount := func() {
		if counted != nil {
			counted.urgent--
			counted = nil
		}
	}
	defer func() {
		c.slotsMu.Lock()
		uncount()
		c.slotsMu.Unlock()
	}()
	for {
		playback, gone := cl.state()
		if gone || s.ctx.Err() != nil {
			return nil
		}
		c.slotsMu.Lock()
		q := c.queue(host)
		limit := c.slotCap(host)
		if !playback {
			uncount()
		} else if len(q.held) >= limit {
			if deadline.IsZero() {
				deadline = time.Now().Add(yieldWait)
			}
			if counted == nil {
				counted = q
				q.urgent++
			}
			c.yieldSlots(q, len(q.held)+q.urgent-limit, nil)
		}
		free := len(q.held) < limit
		if !playback {
			free = len(q.held)+q.urgent < limit
		}
		if free || playback && (!q.yielding() || !time.Now().Before(deadline)) {
			uncount()
			sl := &slot{host: host, claim: cl}
			sl.ctx, sl.cancel = context.WithCancel(s.ctx)
			q.held = append(q.held, sl)
			c.noteRoom(host, q)
			c.slotsMu.Unlock()
			return sl
		}
		if !playback && cl.extra {
			c.slotsMu.Unlock()
			return nil
		}
		if !playback && !q.yielding() {
			c.yieldIdle(q)
		}
		changed := q.changed
		c.slotsMu.Unlock()
		var timer *time.Timer
		var timeout <-chan time.Time
		if playback {
			timer = time.NewTimer(time.Until(deadline))
			timeout = timer.C
		} else {
			s.waitingSlot(1)
		}
		select {
		case <-changed:
		case <-cl.nudge:
		case <-cl.done:
		case <-timeout:
		case <-s.ctx.Done():
		}
		if timer != nil {
			timer.Stop()
		} else {
			s.waitingSlot(-1)
		}
	}
}

// release gives a slot back.
func (c *Cache) release(sl *slot) {
	if sl == nil {
		return
	}
	sl.cancel()
	c.slotsMu.Lock()
	defer c.slotsMu.Unlock()
	c.leave(sl)
}

// leave takes sl out of its host's slots. c.slotsMu is held.
func (c *Cache) leave(sl *slot) {
	q := c.slots[sl.host]
	if q == nil {
		return
	}
	q.held = slices.DeleteFunc(q.held, func(other *slot) bool { return other == sl })
	q.signal()
	c.noteRoom(sl.host, q)
	if len(q.held) == 0 && q.urgent == 0 {
		delete(c.slots, sl.host)
	}
}

// rehome moves a slot to the host that answered its connection, as a
// resolver redirects to the host of the file: the slots of that host count
// it, and other background connections over their bound give theirs up.
// The connection itself, just opened, is spared: given up, it would ask
// again where its slot was, then be redirected again.
func (c *Cache) rehome(sl *slot, host string) {
	if sl == nil || sl.host == host || host == "" {
		return
	}
	c.slotsMu.Lock()
	defer c.slotsMu.Unlock()
	c.leave(sl)
	sl.host = host
	q := c.queue(host)
	q.held = append(q.held, sl)
	c.noteRoom(host, q)
	c.yieldSlots(q, len(q.held)-c.slotCap(host), sl)
}

// press has the background connections over the bound of sl's host give
// their slots up: once it answered 429 or 503, its bound shrank.
func (c *Cache) press(sl *slot) {
	if sl == nil {
		return
	}
	c.slotsMu.Lock()
	defer c.slotsMu.Unlock()
	if q := c.slots[sl.host]; q != nil {
		c.yieldSlots(q, len(q.held)-c.slotCap(sl.host), nil)
	}
}

// lingering records whether a slot's connection lingers: a background one
// then gives its slot up to a connection waiting.
func (c *Cache) lingering(sl *slot, idle bool) {
	if sl == nil {
		return
	}
	c.slotsMu.Lock()
	defer c.slotsMu.Unlock()
	sl.idle = idle
	if q := c.slots[sl.host]; q != nil && idle {
		q.signal()
	}
}

// yieldSlots has n slots given up, those given up already counted, by
// background connections other than spare's: lingering ones first, then
// connections more of a file, then the newest. c.slotsMu is held.
func (c *Cache) yieldSlots(q *hostSlots, n int, spare *slot) {
	for _, sl := range q.held {
		if sl.yielding {
			n--
		}
	}
	for ; n > 0; n-- {
		var victim *slot
		for _, sl := range q.held {
			if sl != spare && !sl.yielding && (victim == nil || sl.rank() >= victim.rank()) && sl.yields() {
				victim = sl
			}
		}
		if victim == nil {
			return
		}
		victim.yielding = true
		victim.cancel()
	}
}

// yieldIdle has a lingering background connection give its slot up, for a
// connection waiting. c.slotsMu is held.
func (c *Cache) yieldIdle(q *hostSlots) {
	for _, sl := range q.held {
		if sl.idle && sl.yields() {
			sl.yielding = true
			sl.cancel()
			return
		}
	}
}

// rank is how readily a slot is given up, from 0.
func (sl *slot) rank() int {
	rank := 0
	if sl.idle {
		rank += 2
	}
	if sl.claim.extra {
		rank++
	}
	return rank
}

// queue is host's slots, made on first use. c.slotsMu is held.
func (c *Cache) queue(host string) *hostSlots {
	q := c.slots[host]
	if q == nil {
		q = &hostSlots{changed: make(chan struct{})}
		c.slots[host] = q
	}
	return q
}

// yielding reports whether a slot is being given up. c.slotsMu is held.
func (q *hostSlots) yielding() bool {
	return slices.ContainsFunc(q.held, func(sl *slot) bool { return sl.yielding })
}

// signal wakes the connections waiting for a slot. c.slotsMu is held.
func (q *hostSlots) signal() {
	close(q.changed)
	q.changed = make(chan struct{})
}

// noteRoom records whether host has a slot free, for roomFor. c.slotsMu
// is held.
func (c *Cache) noteRoom(host string, q *hostSlots) {
	full := len(q.held) >= c.slotCap(host)
	c.learned.Lock()
	defer c.learned.Unlock()
	if state := c.hosts[host]; state != nil || full {
		c.host(host).full = full
	}
}
