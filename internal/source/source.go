// Package source reads remote files the way FFmpeg needs them: in blocks
// fetched with range requests, over one connection per file or a few when
// its host allows, kept in a bounded disk cache, and through a fresh link
// when the old one expires.
package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

const (
	blockSize = 1 << 20
	// skipLimit is how many blocks a connection reads through to reach a
	// block ahead of it, rather than opening a new connection.
	skipLimit = 8
	// attempts bounds the tries to reach a source before readers get an
	// error, and slowTries those to reach one asking to slow down (429,
	// 503): a host's burst passes within seconds.
	attempts  = 4
	slowTries = 5
	// cooldown is how long a failure is reported to new readers before the
	// source is tried again, and slowCooldown how long one asking to slow
	// down past every try is.
	cooldown     = 30 * time.Second
	slowCooldown = 5 * time.Second
	// maxWait bounds how long a source asking to come back later
	// (Retry-After) is waited for, and maxBackoff the growing delay
	// between tries otherwise: 0.5 s, 1 s, then 2 s.
	maxWait    = 5 * time.Second
	maxBackoff = 2 * time.Second
	// sizeTolerance is how far, as a share of it, the size an addon
	// announces may be from the file's: some addons' are a little off.
	sizeTolerance = 0.01
	// fetchInterval spaces the requests of Fetch and FetchRanges to a
	// source: hosts answer 429 to bursts of them, then refuse every file
	// of the account for minutes.
	fetchInterval = time.Second / 8
	// An attempt of Fetch or FetchRanges may take attemptTime, plus the
	// time the bytes asked take at attemptRate bytes a second: the client
	// sending them waits for headers without a deadline.
	attemptTime = 30 * time.Second
	attemptRate = 256 << 10
)

// ErrUnavailable reports a source that did not answer with its content.
var ErrUnavailable = errors.New("source unavailable")

// ErrSlowDown reports a source that asked to slow down (429, or 503), past
// the tries made, or, read once, was overloaded (502, 504 too). It tells
// the host's state, not the file's. It is an ErrUnavailable.
var ErrSlowDown = fmt.Errorf("%w: the source asked to slow down", ErrUnavailable)

// ErrExpired reports a link a source refused (401, 403, 404 or 410) on a
// request that renews nothing: Once.Renew asks for a fresh one. It is an
// ErrUnavailable.
var ErrExpired = fmt.Errorf("%w: the link expired", ErrUnavailable)

// StatusError is a source's answer with a status Polyfin does not read:
// Kind is ErrUnavailable, ErrSlowDown or ErrExpired.
type StatusError struct {
	Status int
	Kind   error
}

func (e *StatusError) Error() string { return fmt.Sprintf("%v: HTTP %d", e.Kind, e.Status) }

func (e *StatusError) Unwrap() error { return e.Kind }

// errWrongRange reports an answer for another range than the one asked.
var errWrongRange = errors.New("unexpected range")

// errOtherFile reports an answer whose size is not the file's, known from
// an earlier answer or from the size the addon announced: a short error
// page some hosts send with HTTP 200, or a link now naming another file.
// Its body is not read as the file's.
var errOtherFile = errors.New("an answer of another size than the file")

// errNoAnswer reports a source that sent no headers in time.
var errNoAnswer = fmt.Errorf("no headers in time: %w", context.DeadlineExceeded)

// errStalled reports a connection that sent no bytes for a while.
var errStalled = errors.New("the connection stalled")

// Answer describes what a source answered to make a request fail, for
// logs: its status, the range ignored, an answer cut short, or none. Never
// the source's URL.
func Answer(err error) string {
	var status *StatusError
	switch {
	case errors.As(err, &status):
		return "HTTP " + strconv.Itoa(status.Status)
	case errors.Is(err, ErrRangesIgnored):
		return "HTTP 200, the range asked ignored"
	case errors.Is(err, errWrongRange):
		return "HTTP 206, another range than the one asked"
	case errors.Is(err, errBroken):
		return "an answer cut short"
	case errors.Is(err, errStalled):
		return "an answer that stopped"
	case errors.Is(err, errOtherFile):
		return "an answer of another size than the file"
	case errors.Is(err, context.DeadlineExceeded):
		return "no answer in time"
	case errors.Is(err, ErrUnavailable):
		return "no answer"
	}
	return ""
}

// ErrRangesIgnored reports a source that answered without honoring the
// range asked.
var ErrRangesIgnored = errors.New("the source ignores ranges")

// Location is where a source's bytes are fetched.
type Location struct {
	URL string
	// Headers must be sent with every request.
	Headers map[string]string
	// Confined restricts requests to public addresses.
	Confined bool
	// Size is the file's size its addon announced, 0 when unknown, and
	// Announcer that addon: an answer of another size is another file,
	// refused at once, once the addon's sizes proved right (see
	// Cache.sizeTrusted).
	Size      int64
	Announcer string
}

// Opener sends requests to sources; *stremio.Client is one.
type Opener interface {
	Open(ctx context.Context, method, target string, header http.Header, confined bool) (*http.Response, error)
}

// Renewer returns a fresh location for a source whose link expired.
type Renewer func(ctx context.Context) (Location, error)

// Cache keeps the sources being read and their blocks on disk, in files of
// a chunk's blocks, within a size limit. The least recently read chunks
// are evicted first, of any source, which bounds the disk a long file read
// to its end takes; chunks read lately are kept, as readers are on them.
type Cache struct {
	dir string
	// limit gives the bytes the cache may keep, read at each use, so that
	// a lower limit applies at the next write.
	limit  func() int64
	opener Opener
	logger *slog.Logger
	// chunkBlocks is how many blocks one file on disk holds; readahead how
	// many blocks past the last one read a reader starting to stream has
	// read ahead, and aheadBudget how many one that has streamed long has
	// (see aheadBudget), so that a sequential reader rarely waits, and a
	// connection stays busy while FFmpeg pauses; now the clock chunks are
	// dated by,
	// fetchInterval the time between two requests of Fetch and FetchRanges
	// to a source, and attemptTime the time one of them may take before
	// the bytes asked.
	chunkBlocks, readahead int64
	aheadBudget            func() int64
	now                    func() time.Time
	fetchInterval          time.Duration
	attemptTime            time.Duration
	// headerTimeout bounds the wait for a source's headers, stallTimeout
	// the time a connection read may go without a byte, recoveryTimeout
	// the wait for the headers of the connection replacing a stalled one,
	// starveAfter the wait of an older reader's block behind the newest
	// reader's window, and linger and attachedLinger how long an idle
	// connection stays open for more, longer while a reader is attached;
	// connections bounds the connections to a file, and hostConnections
	// those to a host (see slots.go).
	headerTimeout, stallTimeout, recoveryTimeout, starveAfter time.Duration
	linger, attachedLinger                                    time.Duration
	connections, hostConnections                              int

	mu      sync.Mutex
	sources map[accounts.ID]*Source
	chunks  map[chunkKey]*chunkUse
	used    int64

	// hosts and sizes are what the cache learned of hosts and of the sizes
	// addons announce, under their own lock, taken with a source's or
	// slotsMu.
	learned sync.Mutex
	hosts   map[string]*hostState
	sizes   map[string]*sizeRecord

	// slots are the connections held of each host, under slotsMu, which
	// may take a source's lock, never the other way round.
	slotsMu sync.Mutex
	slots   map[string]*hostSlots
}

// protectedFor spares the chunks read lately from eviction.
const protectedFor = 30 * time.Second

type chunkKey struct {
	source accounts.ID
	index  int64
}

type chunkUse struct {
	size    int64
	touched time.Time
}

// New returns a cache keeping blocks in dir, up to the bytes limit gives,
// read whenever the limit is used. Blocks a previous run left there are
// removed: they are not trusted.
func New(dir string, limit func() int64, opener Opener, logger *slog.Logger) (*Cache, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	stale, err := filepath.Glob(filepath.Join(dir, "*.blocks"))
	if err != nil {
		return nil, err
	}
	for _, path := range stale {
		if err := os.RemoveAll(path); err != nil {
			return nil, err
		}
	}
	return &Cache{dir: dir, limit: limit, opener: opener, logger: logger, chunkBlocks: 64, readahead: 32,
		aheadBudget: func() int64 { return aheadBudget(limit()) }, now: time.Now, fetchInterval: fetchInterval, attemptTime: attemptTime,
		headerTimeout: headerTimeout, stallTimeout: stallTimeout, recoveryTimeout: recoveryTimeout, starveAfter: starveAfter,
		linger: lingerDelay, attachedLinger: attachedLinger, connections: maxConnections, hostConnections: hostConnections,
		sources: map[accounts.ID]*Source{}, chunks: map[chunkKey]*chunkUse{},
		hosts: map[string]*hostState{}, sizes: map[string]*sizeRecord{}, slots: map[string]*hostSlots{}}, nil
}

// aheadBudget is how many blocks a reader streaming long has read ahead:
// 256 MiB, a minute or two of a 4K remux, within an eighth of the cache.
func aheadBudget(limit int64) int64 {
	return max(32, min(256<<20, limit/8)/blockSize)
}

// Open returns the source identified by id, read from location, and
// renewed by renew (which may be nil) when its link expires. A source is
// shared by all its readers; each must Release it when done. A location
// naming another URL replaces the one known, as addons hand out new links;
// the same one keeps where it redirected to (see Source.target).
func (c *Cache) Open(id accounts.ID, location Location, renew Renewer) *Source {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.sources[id]
	if s == nil {
		ctx, cancel := context.WithCancel(context.Background())
		s = &Source{cache: c, id: id, dir: filepath.Join(c.dir, id.String()+".blocks"), ctx: ctx, cancel: cancel,
			size: -1, wanted: map[int64]*want{}, wake: make(chan struct{}, 1), progress: make(chan struct{}),
			failed: make(chan struct{})}
		c.sources[id] = s
	}
	s.mu.Lock()
	if location.URL != s.origin.URL {
		s.pinned = nil
	}
	s.origin, s.renew = location, renew
	s.mu.Unlock()
	s.refs++
	return s
}

// Answered reports when the source of id last answered with content at
// the URL given, its origin: zero when it did not, or is not open.
func (c *Cache) Answered(id accounts.ID, origin string) time.Time {
	c.mu.Lock()
	s := c.sources[id]
	c.mu.Unlock()
	if s == nil {
		return time.Time{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.origin.URL != origin {
		return time.Time{}
	}
	return s.answeredAt
}

// Streamed reports whether a streaming reader reads the source of id now,
// as FFmpeg does while it remuxes it.
func (c *Cache) Streamed(id accounts.ID) bool {
	c.mu.Lock()
	s := c.sources[id]
	c.mu.Unlock()
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.attached > 0
}

// Usage tells the bytes the cache keeps on disk, the sources open, and its
// limit (the settings' CacheSizeGB).
func (c *Cache) Usage() (used int64, sources int, limit int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.used, len(c.sources), c.limit()
}

// Close stops reading every source and removes their blocks.
func (c *Cache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var errs []error
	for id, s := range c.sources {
		errs = append(errs, s.discard())
		delete(c.sources, id)
	}
	clear(c.chunks)
	c.used = 0
	return errors.Join(errs...)
}

// touch records that a chunk was read.
func (c *Cache) touch(key chunkKey) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if use := c.chunks[key]; use != nil {
		use.touched = c.now()
	}
}

// stored accounts for bytes written to a chunk, and evicts the least
// recently read chunks until the cache fits its limit.
func (c *Cache) stored(key chunkKey, size int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	use := c.chunks[key]
	if use == nil {
		use = &chunkUse{}
		c.chunks[key] = use
	}
	use.size += size
	use.touched = c.now()
	c.used += size
	protected := c.now().Add(-protectedFor)
	for limit := c.limit(); c.used > limit; {
		var oldest chunkKey
		var found *chunkUse
		for key, use := range c.chunks {
			if use.touched.Before(protected) && (found == nil || use.touched.Before(found.touched)) {
				oldest, found = key, use
			}
		}
		if found == nil {
			return
		}
		c.evict(oldest, found)
	}
}

// evict removes a chunk from disk, and its source once nobody reads it and
// it has nothing cached left.
func (c *Cache) evict(key chunkKey, use *chunkUse) {
	delete(c.chunks, key)
	c.used -= use.size
	s := c.sources[key.source]
	if s == nil {
		return
	}
	s.forget(key.index)
	if err := os.Remove(s.chunkPath(key.index)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		c.logger.Warn("A cached chunk could not be removed", "error", err)
	}
	for other := range c.chunks {
		if other.source == key.source {
			return
		}
	}
	if s.refs == 0 {
		if err := s.discard(); err != nil {
			c.logger.Warn("A cached source could not be removed", "error", err)
		}
		delete(c.sources, key.source)
	}
}

// Source is a remote file read through the cache.
type Source struct {
	cache  *Cache
	id     accounts.ID
	dir    string
	ctx    context.Context
	cancel context.CancelFunc
	// refs is guarded by the cache's lock.
	refs int

	mu sync.Mutex
	// origin is where the addon said the file is, and pinned where the
	// origin redirected the server's reads, nil when it did not or the
	// address stopped working.
	origin      Location
	pinned      *pin
	renew       Renewer
	size        int64
	contentType string
	present     []uint64
	// sizeChecked tells that the first answer was checked against the
	// size the addon announced, and sizeNoted that it counted in the
	// addon's record.
	sizeChecked, sizeNoted bool
	// readers are the readers open, oldest first; window is the
	// read-ahead window of the newest streaming one; warms are the spans
	// Warm keeps read; wanted are the blocks readers wait for.
	readers []*Reader
	window  window
	warms   []*warmSpans
	wanted  map[int64]*want
	// conns are the connections reading the source, or about to.
	conns []*conn
	// wake tells a lingering connection that a reader wants more;
	// progress is closed, and replaced, when a block is stored or the
	// source fails.
	wake     chan struct{}
	progress chan struct{}
	// rangeless marks a source that ignores ranges: it is read from its
	// start, whatever block is wanted. ranged marks one that answered a
	// range with it, which a whole file answered once does not make
	// rangeless.
	rangeless, ranged bool
	// singleRanges marks a source that does not serve several ranges with
	// one request: FetchRanges no longer asks it to.
	singleRanges bool
	// multiRanges marks a source that served several ranges with one
	// request.
	multiRanges bool
	// nextFetch is when Fetch and FetchRanges may send their next request.
	nextFetch time.Time
	// failure is why the source failed, at failedAt; failed is closed
	// while it is reported, and replaced once it is tried again.
	failure  error
	failedAt time.Time
	failed   chan struct{}
	// answeredAt is when the source last answered with content.
	answeredAt time.Time
	// attached counts the streaming readers open.
	attached int
	// urged counts the playbacks reading the source (see Urge);
	// heldBack is when its host last asked it to slow down, or a
	// connection of it stopped waiting for a slot, and slotWaits counts
	// those waiting now.
	urged     int
	heldBack  time.Time
	slotWaits int
}

// Urge has the source read for a playback until done is called, as a
// remux, a relay, or the analysis of the version a play chose: its first
// connection never waits for its host's other connections, and those of
// background reads give way to it (see slots.go). Without, the source is
// read in the background.
func (s *Source) Urge() (done func()) {
	s.mu.Lock()
	s.urged++
	s.nudgeConns()
	s.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			s.urged--
			s.mu.Unlock()
		})
	}
}

// HeldBack reports whether the source's reads were held back since a
// time, or are now: its host asked to slow down (429, 503), or they
// waited for the host's other connections. What fails meanwhile, such as
// an analysis out of time, tells nothing of the file.
func (s *Source) HeldBack(since time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.slotWaits > 0 || !s.heldBack.Before(since)
}

// slowedDown records that the source's host asked to slow down: it is
// given fewer connections for a while.
func (s *Source) slowedDown(host string) {
	s.mu.Lock()
	s.heldBack = time.Now()
	s.mu.Unlock()
	s.cache.crowd(host)
}

// waitingSlot counts a connection of the source starting (1) or ending
// (-1) a wait for a slot.
func (s *Source) waitingSlot(delta int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.slotWaits += delta
	s.heldBack = time.Now()
}

// slotState tells whether connection c reads for a playback, as the
// first of a source urged, and whether it is to read nothing more.
func (s *Source) slotState(c *conn) (playback, gone bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.dropped || s.ctx.Err() != nil {
		return false, true
	}
	return s.urged > 0 && len(s.conns) > 0 && s.conns[0] == c, false
}

// nudgeConns has the connections waiting for a slot look again whether
// they read for a playback. The source's lock is held.
func (s *Source) nudgeConns() {
	for _, c := range s.conns {
		select {
		case c.nudge <- struct{}{}:
		default:
		}
	}
}

type want struct {
	done chan struct{}
	err  error
	// waiters counts the readers waiting, since when the first did.
	waiters int
	since   time.Time
}

// Release tells the cache a reader is done with the source.
func (s *Source) Release() {
	s.cache.mu.Lock()
	s.refs--
	s.cache.mu.Unlock()
}

// discard stops reading the source and removes its blocks.
func (s *Source) discard() error {
	s.cancel()
	s.mu.Lock()
	s.present = nil
	s.mu.Unlock()
	return os.RemoveAll(s.dir)
}

func (s *Source) chunkPath(index int64) string {
	return filepath.Join(s.dir, strconv.FormatInt(index, 10))
}

// forget marks the blocks of an evicted chunk as no longer cached.
func (s *Source) forget(index int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for block := index * s.cache.chunkBlocks; block < (index+1)*s.cache.chunkBlocks; block++ {
		if word := block / 64; word < int64(len(s.present)) {
			s.present[word] &^= 1 << (block % 64)
		}
	}
}

// Size returns the size of the source, reaching it if needed. A source that
// does not announce it is read to its end.
func (s *Source) Size(ctx context.Context) (int64, error) {
	r := s.reader(false)
	defer r.Close()
	return r.Size(ctx)
}

// ContentType is the media type the source announced, once reached.
func (s *Source) ContentType() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.contentType
}

func (s *Source) knownSize() (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.size, s.size >= 0
}

// ReadAt reads len(p) bytes at off, waiting for the blocks not cached yet
// at most until ctx is done. It reads like a reader of its own, without
// reading ahead: for the scattered reads of a container's index, which
// must not move the window a stream reads ahead of.
func (s *Source) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	r := s.reader(false)
	defer r.Close()
	return r.ReadAt(ctx, p, off)
}

// readChunk reads part of a chunk file at off.
func (s *Source) readChunk(index int64, part []byte, off int64) (int, error) {
	file, err := os.Open(s.chunkPath(index))
	if err != nil {
		return 0, err
	}
	defer file.Close()
	s.cache.touch(chunkKey{s.id, index})
	return file.ReadAt(part, off)
}

// wait waits until block is cached, asking for it to be fetched on behalf
// of r. A streaming reader's read moves the window read ahead of it,
// cached blocks included, when it is the newest one.
func (s *Source) wait(ctx context.Context, r *Reader, block int64) error {
	s.mu.Lock()
	if r.streaming {
		s.moveWindow(r, block)
	}
	cooling := s.coolingLocked()
	if s.has(block) {
		if r.streaming && !cooling && s.missingAhead() {
			s.kick()
		}
		s.mu.Unlock()
		return nil
	}
	if s.pastEnd(block) {
		s.mu.Unlock()
		return io.EOF
	}
	if cooling {
		err := s.failure
		s.mu.Unlock()
		return err
	}
	w := s.wanted[block]
	if w == nil {
		w = &want{done: make(chan struct{}), since: time.Now()}
		s.wanted[block] = w
	}
	w.waiters++
	r.waiting = block
	s.kick()
	s.mu.Unlock()
	select {
	case <-w.done:
		s.mu.Lock()
		r.waiting = -1
		s.mu.Unlock()
		return w.err
	case <-ctx.Done():
		// The reader gave up: nobody else waiting, the block is no longer
		// wanted.
		s.mu.Lock()
		r.waiting = -1
		if w.waiters--; w.waiters == 0 && s.wanted[block] == w {
			delete(s.wanted, block)
		}
		s.mu.Unlock()
		return ctx.Err()
	}
}

// coolingLocked reports whether a failure is reported to readers still;
// one older is forgotten, and the source tried again, sooner when its
// host asked to slow down. The source's lock is held.
func (s *Source) coolingLocked() bool {
	if s.failure == nil {
		return false
	}
	wait := cooldown
	if errors.Is(s.failure, ErrSlowDown) {
		wait = slowCooldown
	}
	if time.Since(s.failedAt) < wait {
		return true
	}
	s.failure, s.failed = nil, make(chan struct{})
	return false
}

func (s *Source) has(block int64) bool {
	word := block / 64
	return word < int64(len(s.present)) && s.present[word]&(1<<(block%64)) != 0
}

// pastEnd reports whether block starts beyond the end of the source.
func (s *Source) pastEnd(block int64) bool {
	return s.size >= 0 && block*blockSize >= s.size
}

// missing reports whether block is to be fetched: in the file, not cached.
func (s *Source) missing(block int64) bool {
	return !s.has(block) && !s.pastEnd(block)
}

// blockLength is how many bytes block holds: a whole block, or what is
// left of the source.
func (s *Source) blockLength(block int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.size >= 0 {
		return int(min(blockSize, s.size-block*blockSize))
	}
	return blockSize
}

func (s *Source) sizeUnknown() bool {
	_, known := s.knownSize()
	return !known
}

// store keeps a fetched block and wakes the readers waiting for it. last
// marks the final, short block of a source whose size was unknown.
func (s *Source) store(block int64, data []byte, last bool) error {
	index := block / s.cache.chunkBlocks
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(s.chunkPath(index), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	_, err = file.WriteAt(data, (block%s.cache.chunkBlocks)*blockSize)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	s.mu.Lock()
	added := 0
	if !s.has(block) {
		word := block / 64
		if grow := word + 1 - int64(len(s.present)); grow > 0 {
			s.present = append(s.present, make([]uint64, grow)...)
		}
		s.present[word] |= 1 << (block % 64)
		added = len(data)
	}
	if s.failure != nil {
		s.failure, s.failed = nil, make(chan struct{})
	}
	if last {
		s.size = block*blockSize + int64(len(data))
	}
	if w := s.wanted[block]; w != nil {
		delete(s.wanted, block)
		close(w.done)
	}
	if last {
		s.wakePastEnd()
	}
	s.signal()
	s.mu.Unlock()
	if added > 0 {
		s.cache.stored(chunkKey{s.id, index}, int64(added))
	}
	return nil
}

// signal tells whoever waits for progress that there is some. The source's
// lock is held.
func (s *Source) signal() {
	close(s.progress)
	s.progress = make(chan struct{})
}

// end records that the source ends at size, learned from a connection
// that ended.
func (s *Source) end(size int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.size = size
	s.wakePastEnd()
	s.signal()
}

// wakePastEnd answers the readers waiting for blocks past the end.
func (s *Source) wakePastEnd() {
	for block, w := range s.wanted {
		if s.pastEnd(block) {
			w.err = io.EOF
			delete(s.wanted, block)
			close(w.done)
		}
	}
}

// fail answers every waiting reader with err, stops every connection, and
// reports err to readers for a while: Failed is closed first, so that
// what watches it stops what reads the source before the readers see the
// error.
func (s *Source) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure == nil {
		close(s.failed)
	}
	s.failure, s.failedAt = err, time.Now()
	for block, w := range s.wanted {
		w.err = err
		delete(s.wanted, block)
		close(w.done)
	}
	for _, c := range s.conns {
		c.drop()
	}
	s.conns = nil
	s.signal()
	s.cache.logger.Debug("A source could not be read", "source", s.id, "error", err)
}

// Failed returns a channel closed once the source failed: its host did
// not answer, refused it, or answers another file, past the retries the
// cache makes. It stays closed while the failure is reported to readers
// (Failure); a source tried again afterwards has a new one. Whatever reads
// the source, such as ffprobe, is stopped when it closes rather than left
// waiting.
func (s *Source) Failed() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.coolingLocked()
	return s.failed
}

// Failure is why the source failed, while that is reported to readers;
// nil otherwise. It is an ErrUnavailable, which Answer describes.
func (s *Source) Failure() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.coolingLocked() {
		return s.failure
	}
	return nil
}

// learn records what a response tells about the source: its total size,
// when known, and its media type. errOtherFile when the size is not the
// one an earlier response told, or the first one tells a size more than
// 1% from the one its addon announced while that addon's sizes are
// trusted: the response is not the file's, and nothing is recorded.
func (s *Source) learn(total int64, contentType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if total >= 0 && s.size >= 0 && total != s.size {
		return fmt.Errorf("%w: %d bytes, not %d", errOtherFile, total, s.size)
	}
	if total >= 0 && s.size < 0 && s.origin.Size > 0 && s.origin.Announcer != "" && !s.sizeChecked {
		if err := s.checkAnnounced(total); err != nil {
			return err
		}
	}
	if total >= 0 {
		s.size = total
	}
	if s.contentType == "" {
		s.contentType = contentType
	}
	return nil
}

// checkAnnounced compares the first size the source tells with the one
// its addon announced, and refuses one more than 1% off while the
// addon's sizes are trusted. Within 1%, the size told is the file's. Each
// file counts once in the addon's record. The source's lock is held.
func (s *Source) checkAnnounced(total int64) error {
	agrees := nearSize(total, s.origin.Size)
	trusted := s.cache.sizeTrusted(s.origin.Announcer)
	if !s.sizeNoted {
		s.sizeNoted = true
		s.cache.noteSize(s.origin.Announcer, agrees)
	}
	if !agrees && trusted {
		return fmt.Errorf("%w: %d bytes, not the %d its addon announced", errOtherFile, total, s.origin.Size)
	}
	s.sizeChecked = true
	return nil
}

// nearSize reports whether a file's size is within sizeTolerance of the
// one its addon announced.
func nearSize(size, announced int64) bool {
	return math.Abs(float64(size-announced)) <= sizeTolerance*float64(announced)
}

// backoff is how long to wait before the next attempt: what the source
// asks for, up to maxWait, or a growing delay, up to maxBackoff.
func backoff(attempt int, retryAfter string) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && seconds >= 0 {
		return min(time.Duration(seconds)*time.Second, maxWait)
	}
	return min(time.Duration(1<<min(attempt, 8))*250*time.Millisecond, maxBackoff)
}

// withoutURL is err without the request's URL, which may hold credentials.
func withoutURL(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

// contentRange reads "bytes start-end/total"; end is -1 for "*", total -1
// when unknown.
func contentRange(value string) (start, end, total int64, ok bool) {
	spec, found := strings.CutPrefix(strings.TrimSpace(value), "bytes ")
	if !found {
		return 0, 0, 0, false
	}
	span, size, found := strings.Cut(spec, "/")
	if !found {
		return 0, 0, 0, false
	}
	total = -1
	if size != "*" {
		parsed, err := strconv.ParseInt(size, 10, 64)
		if err != nil {
			return 0, 0, 0, false
		}
		total = parsed
	}
	if span == "*" {
		return 0, -1, total, true
	}
	first, last, found := strings.Cut(span, "-")
	if !found {
		return 0, 0, 0, false
	}
	start, err := strconv.ParseInt(first, 10, 64)
	if err != nil {
		return 0, 0, 0, false
	}
	end, err = strconv.ParseInt(last, 10, 64)
	return start, end, total, err == nil
}

// ServeHTTP serves the source with byte ranges, for FFmpeg and ffprobe,
// which read it from a loopback address, each request as a reader of its
// own. A failed source is answered 502 at once while it is reported. One
// failing while a body is sent is not cut at once: what reads it is
// stopped on Failed, and a body cut short reads as the end of the file.
func (s *Source) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if sending, err := s.serve(w, r); err != nil && sending && r.Context().Err() == nil {
		timer := time.NewTimer(failedHold)
		defer timer.Stop()
		select {
		case <-r.Context().Done():
		case <-timer.C:
		}
	}
}

// failedHold is how long the loopback holds a request whose source failed
// before cutting it short, for what watches Failed to stop the reader.
const failedHold = time.Second

// Relay serves the source to a player with byte ranges, as ServeHTTP does:
// from the cache, and through the address its origin redirected to. A
// content type set on w is kept. The error, for logs, is what made the
// answer fail or end early.
func (s *Source) Relay(w http.ResponseWriter, r *http.Request) error {
	_, err := s.serve(w, r)
	return err
}

// serve answers a request for the source's bytes, and tells whether the
// error, if any, came once the answer was being sent.
func (s *Source) serve(w http.ResponseWriter, r *http.Request) (sending bool, err error) {
	if err := s.Failure(); err != nil {
		http.Error(w, "source unavailable", http.StatusBadGateway)
		return false, err
	}
	reader := s.NewReader()
	defer reader.Close()
	size, err := reader.Size(r.Context())
	if err != nil {
		http.Error(w, "source unavailable", http.StatusBadGateway)
		return false, err
	}
	if w.Header().Get("Content-Type") == "" {
		contentType := s.ContentType()
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		w.Header().Set("Content-Type", contentType)
	}
	served := &servedReader{reader: reader, ctx: r.Context()}
	http.ServeContent(w, r, "", time.Time{}, io.NewSectionReader(served, 0, size))
	return true, served.err
}

// servedReader reads a source on behalf of one request, and keeps the
// first error.
type servedReader struct {
	reader *Reader
	ctx    context.Context
	err    error
}

func (r *servedReader) ReadAt(p []byte, off int64) (int, error) {
	n, err := r.reader.ReadAt(r.ctx, p, off)
	if err != nil && !errors.Is(err, io.EOF) && r.err == nil {
		r.err = err
	}
	return n, err
}
