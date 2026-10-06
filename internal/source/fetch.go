package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"
	"strconv"
	"sync/atomic"
	"time"
)

// How a source's blocks are fetched.
//
// A connection reads a stretch of blocks in order, a request each time it
// starts one. It serves, in this order: the block the newest reader waits
// for, the blocks other readers have waited for too long, its own stretch
// while a reader needs it, the window read ahead of the newest streaming
// reader, the blocks other readers wait for, then the spans warmed ahead of
// a playback. A host allowing it reads a file over up to three
// connections: one keeps serving the reader, the others take the next
// stretches it will need, shorter near it and longer further, as long as
// they read more than one connection alone did. A host asking to slow
// down, refusing a connection more, or not serving several faster, is
// read over one for a while.

const (
	// maxConnections bounds the connections reading a file.
	maxConnections = 3
	// minStretch is the fewest blocks a connection more is opened for.
	minStretch = 4
	// headerTimeout bounds the wait for a source's headers: a host that
	// accepts a connection and says nothing fails the source rather than
	// holding its readers.
	headerTimeout = 15 * time.Second
	// stallTimeout is how long a connection read may go without a byte
	// before it is closed and opened again, as one more try.
	stallTimeout = 10 * time.Second
	// recoveryTimeout bounds the headers of the connection replacing a
	// stalled one: a host that stopped answering fails the source within
	// seconds.
	recoveryTimeout = 4 * time.Second
	// lingerDelay is how long an idle connection stays open for the next
	// read, and attachedLinger how long while a streaming reader is
	// attached: FFmpeg pausing ahead of the player.
	lingerDelay    = 5 * time.Second
	attachedLinger = 30 * time.Second
	// starveAfter is how long an older reader's block may wait behind the
	// newest reader's window.
	starveAfter = 3 * time.Second
	// otherFileWait is the wait before an answer of another size than the
	// file is asked again, once.
	otherFileWait = 250 * time.Millisecond
	// measureAfter is how long a connection reads before its rate counts.
	measureAfter = time.Second
)

// noEnd is the end of a stretch without one.
const noEnd = math.MaxInt64

// conn is a connection reading a source's blocks, or about to.
type conn struct {
	// Guarded by the source's lock: next is the block it reads next, or is
	// to connect at, -1 when it has none; end the first block of its
	// stretch it is not to read. connected tells it has a body, idle that
	// it lingers, dropped that it is to read nothing more. runStart is when
	// it last started reading without a pause, runBytes and runBlocks what
	// it read since, and rate how fast.
	next, end           int64
	connected, idle     bool
	dropped             bool
	cancel              context.CancelFunc
	runStart            time.Time
	runBytes, runBlocks int64
	rate                float64
	host                string
	// via is the address pinned the connection reads, nil for the origin.
	via *pin
	// The fetch's own: the body read, the failures since the last block,
	// and whether the last read stalled.
	body     io.ReadCloser
	failures int
	stalled  bool
}

// drop stops the connection: its reads fail at once. The source's lock is
// held.
func (c *conn) drop() {
	c.dropped = true
	if c.cancel != nil {
		c.cancel()
	}
}

// closeBody ends the connection's request.
func (c *conn) closeBody() {
	if c.body != nil {
		c.body.Close()
		c.body = nil
		c.cancel()
	}
}

// kick has the fetch run for what readers want: a lingering connection is
// woken, one more opened when it helps, a first one otherwise. The
// source's lock is held.
func (s *Source) kick() {
	if len(s.conns) == 0 {
		c := &conn{next: -1}
		s.conns = append(s.conns, c)
		go s.work(c)
		return
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
	s.spawn()
}

// spawn opens one connection more when the host allows it and something a
// reader needs is not on the way: once the size is known, one connection
// opening at a time, and none while one lingers. The source's lock is
// held.
func (s *Source) spawn() {
	if s.failure != nil || s.size < 0 || s.rangeless || s.ctx.Err() != nil {
		return
	}
	if len(s.conns) >= s.cache.connectionsFor(s.host()) {
		return
	}
	for _, o := range s.conns {
		if !o.connected || o.idle {
			return
		}
	}
	c := &conn{next: -1}
	target, end, split, ok := s.pick(c, true)
	if !ok {
		return
	}
	if split != nil {
		split.end = target
	}
	c.next, c.end = target, end
	s.conns = append(s.conns, c)
	go s.work(c)
}

// work reads blocks over one connection until there is nothing left to
// read, the source fails, or the connection is not needed.
func (s *Source) work(c *conn) {
	defer c.closeBody()
	buffer := make([]byte, blockSize)
	for {
		target, ok, reconnect, done := s.assign(c)
		if done {
			return
		}
		if !ok {
			if s.idle(c) {
				continue
			}
			return
		}
		if reconnect {
			c.closeBody()
		}
		if c.body == nil {
			err := s.connect(c, target)
			switch {
			case s.dropped(c):
				return
			case errors.Is(err, io.EOF):
				continue
			case err != nil && s.extraFailed(c, err, true):
				return
			case err != nil:
				s.fail(err)
				return
			case !s.needed(target):
				// The reader gave up while the connection opened: it reads
				// what is needed next from there, or lingers.
				continue
			}
		}
		block := s.nextOf(c)
		length := s.blockLength(block)
		n, err := s.readBlock(c, buffer[:length])
		if s.dropped(c) {
			return
		}
		switch {
		case n == length:
			if err := s.store(block, buffer[:n], false); err != nil {
				s.fail(err)
				return
			}
			s.advance(c, n)
			continue
		case n > 0 && errors.Is(err, io.ErrUnexpectedEOF) && s.sizeUnknown():
			// The source ends inside this block.
			if err := s.store(block, buffer[:n], true); err != nil {
				s.fail(err)
				return
			}
		case n == 0 && errors.Is(err, io.EOF) && s.sizeUnknown():
			s.end(block * blockSize)
		default:
			// The connection broke or stalled: reconnect, within limits. A
			// stall right after one fails at once: the host stopped
			// answering. A pinned address breaking again sends the next
			// connection to the origin.
			stalledAgain := errors.Is(err, errStalled) && c.stalled
			c.stalled = c.stalled || errors.Is(err, errStalled)
			if c.failures++; c.failures >= attempts || stalledAgain {
				err := fmt.Errorf("%w: %w", ErrUnavailable, err)
				if s.extraFailed(c, err, false) {
					return
				}
				s.fail(err)
				return
			}
			if c.failures >= 2 {
				s.unpinFrom(c.via)
			}
		}
		c.closeBody()
		s.disconnected(c)
	}
}

// assign chooses the next block the connection reads, and tells whether it
// reconnects to reach it, or is done.
func (s *Source) assign(c *conn) (target int64, ok, reconnect, done bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.dropped {
		return 0, false, false, true
	}
	if len(s.conns) > 1 && len(s.conns) > s.cache.connectionsFor(c.host) && !s.nearest(c) {
		// The host was found to want one connection per file: the
		// connection nearest the readers stays.
		s.removeConn(c)
		return 0, false, false, true
	}
	target, end, split, ok := s.pick(c, false)
	if !ok {
		return 0, false, false, false
	}
	if split != nil {
		split.end = target
	}
	c.end = end
	if c.idle {
		c.idle = false
		c.runStart, c.runBytes, c.runBlocks = time.Now(), 0, 0
	}
	if c.connected && (target < c.next || target-c.next > skipLimit && !s.rangeless) {
		c.connected, reconnect = false, true
	}
	if !c.connected {
		c.next = target
	}
	return target, true, reconnect, false
}

// pick chooses what connection c reads next: the block, the end of the
// stretch it reads from there, and the connection whose stretch it takes
// the rest of, if any. fresh is for a connection to open, which a stretch
// of the windows must be worth. The source's lock is held.
func (s *Source) pick(c *conn, fresh bool) (target, end int64, split *conn, ok bool) {
	// The block the newest reader waits for.
	if r := s.newest(); r != nil && r.waiting >= 0 {
		if target, end, split, ok := s.pickBlock(c, r.waiting); ok {
			return target, end, split, true
		}
	}
	// The blocks older readers have waited for too long.
	now := time.Now()
	var starving []int64
	for block, w := range s.wanted {
		if now.Sub(w.since) >= starveAfter {
			starving = append(starving, block)
		}
	}
	slices.SortFunc(starving, func(a, b int64) int { return s.wanted[a].since.Compare(s.wanted[b].since) })
	for _, block := range starving {
		if target, end, split, ok := s.pickBlock(c, block); ok {
			return target, end, split, true
		}
	}
	// Its own stretch, while a reader needs it.
	own := c.next >= 0 && c.next < c.end && s.missing(c.next)
	if own && (s.inWindow(c.next) || s.wanted[c.next] != nil) {
		return c.next, c.end, nil, true
	}
	// The window read ahead of the newest streaming reader.
	if s.window.owner != nil {
		if target, end, split, ok := s.pickSpan(c, s.window.cursor, s.window.horizon, fresh); ok {
			return target, end, split, true
		}
	}
	if own && s.inWarm(c.next) {
		return c.next, c.end, nil, true
	}
	// The blocks other readers wait for, the newest reader's first.
	for i := len(s.readers) - 1; i >= 0; i-- {
		if block := s.readers[i].waiting; block >= 0 {
			if target, end, split, ok := s.pickBlock(c, block); ok {
				return target, end, split, true
			}
		}
	}
	// The spans warmed ahead of a playback.
	for _, warm := range s.warms {
		for _, span := range warm.spans {
			if target, end, split, ok := s.pickSpan(c, span[0], span[1], fresh); ok {
				return target, end, split, true
			}
		}
	}
	return 0, 0, nil, false
}

// pickBlock reports whether connection c is to fetch block, which a reader
// waits for: not when another connection reaches it shortly. The source's
// lock is held.
func (s *Source) pickBlock(c *conn, block int64) (target, end int64, split *conn, ok bool) {
	if !s.missing(block) {
		return 0, 0, nil, false
	}
	if c.connected && c.next <= block && (block-c.next <= skipLimit || s.rangeless) {
		return block, max(c.end, block+1), nil, true
	}
	for _, o := range s.conns {
		if o != c && !o.dropped && o.next >= 0 && o.next <= block && (block-o.next <= skipLimit || s.rangeless) {
			return 0, 0, nil, false
		}
	}
	end = noEnd
	if owner := s.owner(c, block); owner != nil {
		end, split = owner.end, owner
	}
	return block, end, split, true
}

// pickSpan finds the first block from first to last, excluded, that is not
// cached and that no other connection reads shortly: the part of its
// stretch a connection keeps grows with its distance from first, so that
// the connections near a reader read short stretches, and those further
// long ones. For a connection to open, the stretch must hold minStretch
// blocks. The source's lock is held.
func (s *Source) pickSpan(c *conn, first, last int64, fresh bool) (target, end int64, split *conn, ok bool) {
	for block := first; block < last && !s.pastEnd(block); {
		if s.has(block) {
			block++
			continue
		}
		if !fresh && c.next >= 0 && block >= c.next && block < c.end {
			return block, c.end, nil, true
		}
		if o := s.covering(c, block, first); o != nil {
			block = min(o.end, o.next+keep(o, first))
			continue
		}
		if fresh && !s.missingRun(block, min(last, block+minStretch)) {
			return 0, 0, nil, false
		}
		end = noEnd
		if owner := s.owner(c, block); owner != nil {
			end, split = owner.end, owner
		}
		return block, end, split, true
	}
	return 0, 0, nil, false
}

// keep is how much of its stretch a connection at o.next keeps for itself,
// counted from first.
func keep(o *conn, first int64) int64 {
	return max(minStretch, (o.next-first)/2)
}

// covering is a connection other than c that reads block shortly, within
// the part of its stretch it keeps. The source's lock is held.
func (s *Source) covering(c *conn, block, first int64) *conn {
	for _, o := range s.conns {
		if o != c && !o.dropped && o.next >= 0 && o.next <= block && block < min(o.end, o.next+keep(o, first)) {
			return o
		}
	}
	return nil
}

// owner is a connection other than c whose stretch holds block. The
// source's lock is held.
func (s *Source) owner(c *conn, block int64) *conn {
	for _, o := range s.conns {
		if o != c && !o.dropped && o.next >= 0 && o.next <= block && block < o.end {
			return o
		}
	}
	return nil
}

// missingRun reports whether every block from first to last, excluded, is
// to be fetched, and they are minStretch at least. The source's lock is
// held.
func (s *Source) missingRun(first, last int64) bool {
	for block := first; block < last; block++ {
		if !s.missing(block) {
			return false
		}
	}
	return last-first >= minStretch
}

// inWindow reports whether block is in the read-ahead window. The source's
// lock is held.
func (s *Source) inWindow(block int64) bool {
	return s.window.owner != nil && block >= s.window.cursor && block < s.window.horizon
}

// inWarm reports whether block is in a span warmed. The source's lock is
// held.
func (s *Source) inWarm(block int64) bool {
	for _, warm := range s.warms {
		for _, span := range warm.spans {
			if block >= span[0] && block < span[1] {
				return true
			}
		}
	}
	return false
}

// needed reports whether block is to be fetched for someone: a reader
// waiting for it, the window, or a warm.
func (s *Source) needed(block int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.missing(block) && (s.wanted[block] != nil || s.inWindow(block) || s.inWarm(block))
}

// idle waits a little, with the connection open, for a reader to want
// more, and tells whether one did. Only the last connection lingers; the
// others end at once.
func (s *Source) idle(c *conn) bool {
	s.mu.Lock()
	if c.dropped || !c.connected || len(s.conns) > 1 {
		s.removeConn(c)
		s.mu.Unlock()
		return false
	}
	c.idle = true
	delay := s.cache.linger
	if s.attached > 0 {
		delay = s.cache.attachedLinger
	}
	s.mu.Unlock()
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-s.wake:
		return true
	case <-timer.C:
	case <-s.ctx.Done():
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, _, _, ok := s.pick(c, false); ok && !c.dropped && s.ctx.Err() == nil {
		return true
	}
	s.removeConn(c)
	return false
}

// removeConn ends connection c. The source's lock is held.
func (s *Source) removeConn(c *conn) {
	c.dropped = true
	s.conns = slices.DeleteFunc(s.conns, func(o *conn) bool { return o == c })
}

// nearest reports whether no other connection reads a block before c's.
// The source's lock is held.
func (s *Source) nearest(c *conn) bool {
	for _, o := range s.conns {
		if o != c && !o.dropped && o.next >= 0 && o.next < c.next {
			return false
		}
	}
	return true
}

// dropped reports whether connection c is to read nothing more.
func (s *Source) dropped(c *conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.dropped || s.ctx.Err() != nil {
		s.removeConn(c)
		return true
	}
	return false
}

func (s *Source) nextOf(c *conn) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return c.next
}

// disconnected records that connection c's request ended.
func (s *Source) disconnected(c *conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.connected = false
}

// extraFailed takes a connection that failed out of the source when
// another one reads it: a connection more the host refused, which has it
// read over one connection per file, or one that broke after reading
// well. It reports whether it did; the source fails otherwise, and always
// when another file answered: that is the link's, not the connection's.
func (s *Source) extraFailed(c *conn, err error, connecting bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.alone(c) || errors.Is(err, errOtherFile) {
		return false
	}
	s.removeConn(c)
	if connecting || c.runBlocks == 0 {
		go s.cache.limitHost(c.host, "a connection more failed: "+Answer(err))
	}
	return true
}

// alone reports whether no other connection than c reads the source. The
// source's lock is held.
func (s *Source) alone(c *conn) bool {
	for _, o := range s.conns {
		if o != c && !o.dropped && o.connected {
			return false
		}
	}
	return true
}

// advance moves connection c past the block it stored, and measures how
// fast the source's connections read: once several read together no faster
// than one alone did, the host is given one connection per file.
func (s *Source) advance(c *conn, n int) {
	c.failures, c.stalled = 0, false
	s.mu.Lock()
	defer s.mu.Unlock()
	c.next++
	c.runBytes += int64(n)
	c.runBlocks++
	now := time.Now()
	elapsed := now.Sub(c.runStart)
	if elapsed < measureAfter {
		return
	}
	c.rate = float64(c.runBytes) / elapsed.Seconds()
	if len(s.conns) == 1 {
		s.cache.noteSingle(c.host, c.rate)
		return
	}
	total := 0.0
	for _, o := range s.conns {
		if !o.connected || o.idle || now.Sub(o.runStart) < measureAfter || o.rate == 0 {
			return
		}
		total += o.rate
	}
	if !s.cache.gains(c.host, total) {
		go s.cache.limitHost(c.host, "several connections read no faster than one")
	}
}

// readBlock reads one block over connection c, closing it once it goes
// stallTimeout without a byte.
func (s *Source) readBlock(c *conn, p []byte) (int, error) {
	s.mu.Lock()
	cancel := c.cancel
	s.mu.Unlock()
	var stalled atomic.Bool
	timer := time.AfterFunc(s.cache.stallTimeout, func() {
		stalled.Store(true)
		cancel()
	})
	n, err := io.ReadFull(watched{c.body, timer, s.cache.stallTimeout}, p)
	timer.Stop()
	if stalled.Load() && n < len(p) {
		err = fmt.Errorf("%w: no byte for %s", errStalled, s.cache.stallTimeout)
	}
	return n, err
}

// watched is a body whose reads push back a watchdog.
type watched struct {
	body  io.Reader
	timer *time.Timer
	after time.Duration
}

func (w watched) Read(p []byte) (int, error) {
	n, err := w.body.Read(p)
	if n > 0 {
		w.timer.Reset(w.after)
	}
	return n, err
}

// pin is where a source's origin redirected the server's reads, and the
// headers sent there.
type pin struct {
	url    string
	header http.Header
}

// target is where a request of the source goes: through the pinned
// address when there is one, else the origin.
type target struct {
	url      string
	header   http.Header
	confined bool
	pinned   *pin
	renew    Renewer
	host     string
}

// target is where the source's next request goes. The pinned address,
// where the origin redirected the server's reads, saves a resolver's round
// trip per connection and keeps the file from changing under a playback;
// it is the server's own, never handed to a player, as some hosts tie it
// to the address that resolved it.
func (s *Source) target() target {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.target0()
}

// target0 is target, the source's lock held. Its header is its own.
func (s *Source) target0() target {
	t := target{confined: s.origin.Confined, renew: s.renew}
	if s.pinned != nil {
		t.url, t.header, t.pinned = s.pinned.url, s.pinned.header.Clone(), s.pinned
	} else {
		t.url, t.header = s.origin.URL, http.Header{}
		for name, value := range s.origin.Headers {
			t.header.Set(name, value)
		}
	}
	t.host = hostOf(t.url)
	return t
}

// host is the host the source's next request goes to. The source's lock is
// held.
func (s *Source) host() string {
	if s.pinned != nil {
		return hostOf(s.pinned.url)
	}
	return hostOf(s.origin.URL)
}

// answered records that the source answered a request sent to t with
// content: when, and where the origin redirected it, pinned.
func (s *Source) answered(t target, response *http.Response) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.answeredAt = time.Now()
	if t.pinned != nil || response.Request == nil || response.Request.URL == nil || s.origin.URL != t.url {
		return
	}
	if final := response.Request.URL.String(); final != t.url {
		header := response.Request.Header.Clone()
		header.Del("Range")
		s.pinned = &pin{url: final, header: header}
	}
}

// unpin sends the source's next requests to its origin again, when t went
// to the address pinned.
func (s *Source) unpin(t target) {
	s.unpinFrom(t.pinned)
}

// unpinFrom sends the source's next requests to its origin again, when
// they go to via.
func (s *Source) unpinFrom(via *pin) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if via != nil && s.pinned == via {
		s.pinned = nil
	}
}

// connect opens a connection yielding block for c. An expired link goes
// back to the origin, then is renewed, once. An answer of another size
// than the file is asked again once, at the origin, after a renewal when
// the file was known: a link answering another file does not fix itself.
// A source asking to wait is waited for. A connection more is not retried:
// its failure is the host's refusal.
func (s *Source) connect(c *conn, block int64) error {
	offset := block * blockSize
	renewed, retried := false, false
	timeout := s.cache.headerTimeout
	if c.stalled {
		timeout = s.cache.recoveryTimeout
	}
	for attempt := 1; ; attempt++ {
		s.mu.Lock()
		alone := s.alone(c)
		t := s.target0()
		c.host = t.host
		s.mu.Unlock()
		header := t.header
		header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
		ctx, cancel := context.WithCancel(s.ctx)
		timer := time.AfterFunc(timeout, cancel)
		response, err := s.cache.opener.Open(ctx, http.MethodGet, t.url, header, t.confined)
		if !timer.Stop() {
			if err == nil {
				response.Body.Close()
			}
			err = errNoAnswer
		}
		if err != nil {
			cancel()
			s.unpin(t)
			switch {
			case s.ctx.Err() != nil:
				return fmt.Errorf("%w: %v", ErrUnavailable, s.ctx.Err())
			case !alone || errors.Is(err, errNoAnswer) || attempt >= attempts:
				return fmt.Errorf("%w: %w", ErrUnavailable, withoutURL(err))
			}
			s.sleep(backoff(attempt, ""))
			continue
		}
		status := response.StatusCode
		var other error
		switch {
		case status == http.StatusPartialContent:
			start, _, total, ok := contentRange(response.Header.Get("Content-Range"))
			if !ok || start != offset {
				response.Body.Close()
				cancel()
				return fmt.Errorf("%w: %w %q", ErrUnavailable, errWrongRange, response.Header.Get("Content-Range"))
			}
			if other = s.learn(total, response.Header.Get("Content-Type")); other == nil {
				s.answered(t, response)
				s.connected(c, response.Body, cancel, block, t.pinned)
				return nil
			}
			response.Body.Close()
			cancel()
		case status == http.StatusOK:
			// The body starts at the first byte whatever was asked: a source
			// answering so past its start ignores ranges, and the blocks
			// before the one asked are read through. A body of another size
			// than the file is an error page, which some hosts send so when
			// they refuse a request: the size of a file is never taken from
			// it, which would cut the file short for every reader.
			if other = s.learn(response.ContentLength, response.Header.Get("Content-Type")); other == nil {
				if offset > 0 {
					s.mu.Lock()
					s.rangeless = true
					s.mu.Unlock()
				}
				s.answered(t, response)
				s.connected(c, response.Body, cancel, 0, t.pinned)
				return nil
			}
			response.Body.Close()
			cancel()
		case status == http.StatusRequestedRangeNotSatisfiable:
			cancel()
			// Learned like the other answers: the size of another file is
			// not taken for the file's.
			other = s.unsatisfiable(response, offset)
			if errors.Is(other, io.EOF) {
				if size, known := s.knownSize(); known {
					s.end(size)
				} else {
					s.end(offset)
				}
				return io.EOF
			}
		case expiredStatus(status):
			response.Body.Close()
			cancel()
			switch {
			case !alone:
				return &StatusError{Status: status, Kind: ErrUnavailable}
			case t.pinned != nil:
				// Where the origin redirected expired: the origin tells
				// where the file is now.
				s.unpin(t)
				continue
			case t.renew != nil && !renewed:
				renewed = true
				if err := s.renewLink(s.ctx); err != nil {
					return fmt.Errorf("%w: HTTP %d, and the link could not be renewed: %v", ErrUnavailable, status, err)
				}
				attempt = 0
				continue
			}
			return &StatusError{Status: status, Kind: ErrUnavailable}
		case status == http.StatusTooManyRequests || status >= 500:
			response.Body.Close()
			cancel()
			if status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable {
				go s.cache.limitHost(t.host, "HTTP "+strconv.Itoa(status))
			}
			if !alone || attempt >= attempts {
				return &StatusError{Status: status, Kind: ErrUnavailable}
			}
			if attempt >= 2 {
				s.unpin(t)
			}
			s.sleep(backoff(attempt, response.Header.Get("Retry-After")))
			continue
		default:
			response.Body.Close()
			cancel()
			return &StatusError{Status: status, Kind: ErrUnavailable}
		}
		// Another file answered.
		if !errors.Is(other, ErrUnavailable) {
			other = fmt.Errorf("%w: %w", ErrUnavailable, other)
		}
		if retried {
			s.unpin(t)
			return other
		}
		retried = true
		s.unpin(t)
		if _, known := s.knownSize(); known && t.renew != nil && !renewed {
			// The file was known: the link names another now.
			renewed = true
			if err := s.renewLink(s.ctx); err != nil {
				return fmt.Errorf("%w, and the link could not be renewed: %v", other, err)
			}
		}
		s.sleep(otherFileWait)
	}
}

// connected records connection c's body, which yields next, read from via.
func (s *Source) connected(c *conn, body io.ReadCloser, cancel context.CancelFunc, next int64, via *pin) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.body, c.cancel, c.next, c.connected, c.via = body, cancel, next, true, via
	c.runStart, c.runBytes, c.runBlocks = time.Now(), 0, 0
	if c.dropped {
		cancel()
	}
}

func (s *Source) ignoresRanges() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rangeless
}
