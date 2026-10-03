// Package source reads remote files the way FFmpeg needs them: in blocks
// fetched with range requests over a single connection per file, kept in a
// bounded disk cache, and through a fresh link when the old one expires.
package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/container"
)

const (
	blockSize = 1 << 20
	// skipLimit is how many blocks the connection reads through to reach a
	// block ahead of it, rather than opening a new connection.
	skipLimit = 8
	// attempts bounds the tries to reach a source before readers get an
	// error.
	attempts = 4
	// cooldown is how long a failure is reported to new readers before the
	// source is tried again.
	cooldown = 30 * time.Second
	// maxWait bounds how long a source asking to come back later is
	// waited for.
	maxWait = 30 * time.Second
	// lingerDelay is how long an idle connection stays open for the
	// reader's next request.
	lingerDelay = 5 * time.Second
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
	dir    string
	limit  int64
	opener Opener
	logger *slog.Logger
	// chunkBlocks is how many blocks one file on disk holds, readahead how
	// many blocks past the last one asked a file keeps being read, so that
	// a sequential reader rarely waits, now the clock chunks are dated by,
	// fetchInterval the time between two requests of Fetch and FetchRanges
	// to a source, and attemptTime the time one of them may take before
	// the bytes asked.
	chunkBlocks, readahead int64
	now                    func() time.Time
	fetchInterval          time.Duration
	attemptTime            time.Duration

	mu      sync.Mutex
	sources map[accounts.ID]*Source
	chunks  map[chunkKey]*chunkUse
	used    int64
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

// New returns a cache keeping blocks in dir, up to limit bytes. Blocks a
// previous run left there are removed: they are not trusted.
func New(dir string, limit int64, opener Opener, logger *slog.Logger) (*Cache, error) {
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
	return &Cache{dir: dir, limit: limit, opener: opener, logger: logger, chunkBlocks: 64, readahead: 32, now: time.Now,
		fetchInterval: fetchInterval, attemptTime: attemptTime, sources: map[accounts.ID]*Source{}, chunks: map[chunkKey]*chunkUse{}}, nil
}

// Open returns the source identified by id, read from location, and
// renewed by renew (which may be nil) when its link expires. A source is
// shared by all its readers; each must Release it when done. A location
// given again replaces the one known, as addons hand out new links.
func (c *Cache) Open(id accounts.ID, location Location, renew Renewer) *Source {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.sources[id]
	if s == nil {
		ctx, cancel := context.WithCancel(context.Background())
		s = &Source{cache: c, id: id, dir: filepath.Join(c.dir, id.String()+".blocks"), ctx: ctx, cancel: cancel,
			size: -1, wanted: map[int64]*want{}, wake: make(chan struct{}, 1)}
		c.sources[id] = s
	}
	s.mu.Lock()
	s.location, s.renew = location, renew
	s.mu.Unlock()
	s.refs++
	return s
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
	for c.used > c.limit {
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

	mu          sync.Mutex
	location    Location
	renew       Renewer
	size        int64
	contentType string
	present     []uint64
	// wanted are the blocks readers wait for, fetched in the order asked.
	wanted map[int64]*want
	order  []int64
	// cursor is the block last read, and horizon the block reading ahead
	// stops at.
	cursor, horizon int64
	// rangeless marks a source that ignores ranges: it is read from its
	// start, whatever block is wanted.
	rangeless bool
	// singleRanges marks a source that does not serve several ranges with
	// one request: FetchRanges no longer asks it to.
	singleRanges bool
	// nextFetch is when Fetch and FetchRanges may send their next request.
	nextFetch time.Time
	running   bool
	// wake tells a lingering fetch that a reader wants more.
	wake     chan struct{}
	failure  error
	failedAt time.Time
}

type want struct {
	done chan struct{}
	err  error
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
	for block := int64(0); ; block++ {
		if size, known := s.knownSize(); known {
			return size, nil
		}
		if err := s.wait(ctx, block, false); err != nil && !errors.Is(err, io.EOF) {
			return 0, err
		}
	}
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
// at most until ctx is done.
func (s *Source) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	n := 0
	for refetched := false; n < len(p); {
		position := off + int64(n)
		if size, known := s.knownSize(); known && position >= size {
			return n, io.EOF
		}
		block := position / blockSize
		if err := s.wait(ctx, block, true); err != nil {
			return n, err
		}
		end := (block + 1) * blockSize
		if size, known := s.knownSize(); known {
			end = min(end, size)
		}
		part := p[n:min(len(p), n+int(end-position))]
		index := block / s.cache.chunkBlocks
		read, err := s.readChunk(index, part, position-index*s.cache.chunkBlocks*blockSize)
		if errors.Is(err, fs.ErrNotExist) && !refetched {
			// The chunk was evicted between the wait and the read: fetch it
			// again, once.
			s.forget(index)
			refetched = true
			continue
		}
		n += read
		if err != nil && (!errors.Is(err, io.EOF) || read < len(part)) {
			return n, err
		}
		refetched = false
	}
	return n, nil
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

// wait waits until block is cached, asking for it to be fetched. A read
// moves the window read ahead of the reader, cached blocks included, so
// that a sequential reader rarely waits.
func (s *Source) wait(ctx context.Context, block int64, read bool) error {
	s.mu.Lock()
	if read {
		s.cursor, s.horizon = block, block+1+s.cache.readahead
	}
	cooling := s.failure != nil && time.Since(s.failedAt) < cooldown
	if s.has(block) {
		if read && !cooling && s.missingAhead() {
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
		w = &want{done: make(chan struct{})}
		s.wanted[block] = w
		s.order = append(s.order, block)
	}
	s.kick()
	s.mu.Unlock()
	select {
	case <-w.done:
		return w.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// missingAhead reports whether a block of the read-ahead window is not
// cached yet.
func (s *Source) missingAhead() bool {
	_, missing := s.ahead()
	return missing
}

// ahead returns the first block of the read-ahead window not cached yet.
func (s *Source) ahead() (int64, bool) {
	for block := s.cursor; block < s.horizon && !s.pastEnd(block); block++ {
		if !s.has(block) {
			return block, true
		}
	}
	return 0, false
}

func (s *Source) has(block int64) bool {
	word := block / 64
	return word < int64(len(s.present)) && s.present[word]&(1<<(block%64)) != 0
}

// pastEnd reports whether block starts beyond the end of the source.
func (s *Source) pastEnd(block int64) bool {
	return s.size >= 0 && block*blockSize >= s.size
}

// connection is a response body being read, which yields block next.
type connection struct {
	body io.ReadCloser
	next int64
}

// fetch reads the blocks readers wait for, oldest request first, then the
// read-ahead window. It runs alone for a source, over one connection at a
// time.
func (s *Source) fetch() {
	var conn *connection
	defer func() {
		if conn != nil {
			conn.body.Close()
		}
	}()
	buffer := make([]byte, blockSize)
	failures := 0
	for {
		target, ok := s.next()
		if !ok {
			// A reader often comes back for the next blocks: the connection
			// lingers a little for it.
			if conn != nil && s.linger() {
				continue
			}
			if s.stop() {
				return
			}
			continue
		}
		// A block shortly ahead is reached by reading through; a source that
		// ignores ranges is always read through.
		if conn != nil && (target < conn.next || target-conn.next > skipLimit && !s.ignoresRanges()) {
			conn.body.Close()
			conn = nil
		}
		if conn == nil {
			opened, err := s.connect(target)
			if errors.Is(err, io.EOF) {
				continue
			}
			if err != nil {
				s.fail(err)
				return
			}
			conn = opened
		}
		block := conn.next
		length := s.blockLength(block)
		n, err := io.ReadFull(conn.body, buffer[:length])
		switch {
		case n == length:
			if err := s.store(block, buffer[:n], false); err != nil {
				s.fail(err)
				return
			}
			conn.next++
			failures = 0
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
			// The connection broke: reconnect, within limits.
			if failures++; failures >= attempts {
				s.fail(fmt.Errorf("%w: %v", ErrUnavailable, err))
				return
			}
		}
		conn.body.Close()
		conn = nil
	}
}

// next chooses the block to fetch: the oldest one a reader waits for, else
// the first one missing from the read-ahead window.
func (s *Source) next() (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextLocked()
}

func (s *Source) nextLocked() (int64, bool) {
	for len(s.order) > 0 {
		block := s.order[0]
		if !s.has(block) && !s.pastEnd(block) {
			return block, true
		}
		s.order = s.order[1:]
	}
	return s.ahead()
}

// kick has the fetch run: woken when it lingers, started otherwise. The
// source's lock is held.
func (s *Source) kick() {
	if !s.running {
		s.running = true
		go s.fetch()
		return
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// linger waits a little for a reader to want more, and tells whether one
// did.
func (s *Source) linger() bool {
	select {
	case <-s.wake:
		return true
	case <-time.After(lingerDelay):
		return false
	case <-s.ctx.Done():
		return false
	}
}

// stop ends the fetch unless a reader wanted something meanwhile.
func (s *Source) stop() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, more := s.nextLocked(); more {
		return false
	}
	s.running = false
	return true
}

func (s *Source) ignoresRanges() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rangeless
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
	s.failure = nil
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
	s.mu.Unlock()
	if added > 0 {
		s.cache.stored(chunkKey{s.id, index}, int64(added))
	}
	return nil
}

// end records that the source ends at size, learned from a connection
// that ended.
func (s *Source) end(size int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.size = size
	s.wakePastEnd()
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

// fail answers every waiting reader with err and remembers it for a while.
func (s *Source) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failure, s.failedAt = err, time.Now()
	for block, w := range s.wanted {
		w.err = err
		delete(s.wanted, block)
		close(w.done)
	}
	s.order = nil
	s.running = false
	s.cache.logger.Debug("A source could not be read", "source", s.id, "error", err)
}

// connect opens a connection yielding block, renewing the link once when it
// expired and waiting when the source asks to.
func (s *Source) connect(block int64) (*connection, error) {
	offset := block * blockSize
	renewed := false
	for attempt := 1; ; attempt++ {
		s.mu.Lock()
		location, renew := s.location, s.renew
		s.mu.Unlock()
		header := http.Header{}
		for name, value := range location.Headers {
			header.Set(name, value)
		}
		header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
		response, err := s.cache.opener.Open(s.ctx, http.MethodGet, location.URL, header, location.Confined)
		if err != nil {
			if s.ctx.Err() != nil || attempt >= attempts {
				return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
			}
			s.sleep(backoff(attempt, ""))
			continue
		}
		switch status := response.StatusCode; {
		case status == http.StatusPartialContent:
			start, _, total, ok := contentRange(response.Header.Get("Content-Range"))
			if !ok || start != offset {
				response.Body.Close()
				return nil, fmt.Errorf("%w: unexpected range %q", ErrUnavailable, response.Header.Get("Content-Range"))
			}
			s.learn(total, response.Header.Get("Content-Type"))
			return &connection{body: response.Body, next: block}, nil
		case status == http.StatusOK:
			// The body starts at the first byte whatever was asked: a source
			// answering so past its start ignores ranges, and the blocks
			// before the one asked are read through.
			s.learn(response.ContentLength, response.Header.Get("Content-Type"))
			if offset > 0 {
				s.mu.Lock()
				s.rangeless = true
				s.mu.Unlock()
			}
			return &connection{body: response.Body, next: 0}, nil
		case status == http.StatusRequestedRangeNotSatisfiable:
			response.Body.Close()
			if _, _, total, ok := contentRange(response.Header.Get("Content-Range")); ok {
				s.end(total)
			} else {
				s.end(offset)
			}
			return nil, io.EOF
		case (status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusNotFound || status == http.StatusGone) &&
			renew != nil && !renewed:
			response.Body.Close()
			renewed = true
			fresh, err := renew(s.ctx)
			if err != nil {
				return nil, fmt.Errorf("%w: HTTP %d, and the link could not be renewed: %v", ErrUnavailable, status, err)
			}
			s.mu.Lock()
			s.location = fresh
			s.mu.Unlock()
			s.cache.logger.Debug("A source link was renewed", "source", s.id)
			attempt = 0
		case (status == http.StatusTooManyRequests || status >= 500) && attempt < attempts:
			response.Body.Close()
			s.sleep(backoff(attempt, response.Header.Get("Retry-After")))
		default:
			response.Body.Close()
			return nil, fmt.Errorf("%w: HTTP %d", ErrUnavailable, status)
		}
	}
}

// Fetch reads the n bytes at off with one request for that range alone,
// past the block cache: for small reads scattered over a file, such as its
// subtitle blocks, which the cache's 1 MiB blocks would multiply. It uses
// the source's location and renews it when it expired, as reads do, spaces
// its requests with those of others to the source, and waits out a server
// asking it to slow down, failing (429, 502, 503 or 504) or too slow to
// answer, a bounded number of times. Fewer than n bytes only at the end of
// the file.
func (s *Source) Fetch(ctx context.Context, off int64, n int) ([]byte, error) {
	if off < 0 || n < 0 {
		return nil, fmt.Errorf("fetching %d bytes at %d: invalid range", n, off)
	}
	if size, known := s.knownSize(); known && off >= size {
		return nil, io.EOF
	}
	if n == 0 {
		return nil, nil
	}
	ranges := "bytes=" + strconv.FormatInt(off, 10) + "-" + strconv.FormatInt(off+int64(n)-1, 10)
	var data []byte
	err := s.exchange(ctx, ranges, int64(n), func(response *http.Response) error {
		var err error
		switch response.StatusCode {
		case http.StatusPartialContent:
			data, err = s.fetched(response, off, n)
			return err
		case http.StatusOK:
			// The body starts at the first byte, and may hold the whole
			// file: it is not read.
			return fmt.Errorf("fetching %d bytes at %d: %w", n, off, ErrRangesIgnored)
		default:
			s.unsatisfiable(response)
			return io.EOF
		}
	})
	if err != nil {
		return nil, err
	}
	return data, nil
}

// errBroken reports a connection that broke while a body was read: the
// request may be tried again.
var errBroken = errors.New("the connection broke")

// fetched reads the body of a response to Fetch's request for the n bytes
// at off: those of them the file holds.
func (s *Source) fetched(response *http.Response, off int64, n int) ([]byte, error) {
	start, _, total, ok := contentRange(response.Header.Get("Content-Range"))
	if !ok || start != off {
		return nil, fmt.Errorf("%w: unexpected range %q", ErrUnavailable, response.Header.Get("Content-Range"))
	}
	s.learn(total, response.Header.Get("Content-Type"))
	if total >= 0 && off+int64(n) > total {
		n = int(max(0, total-off))
	}
	data := make([]byte, n)
	if _, err := io.ReadFull(response.Body, data); err != nil {
		return nil, fmt.Errorf("%w: %w: %v", ErrUnavailable, errBroken, err)
	}
	return data, nil
}

// FetchRanges reads several ranges of the source with one request, past
// the block cache, as Fetch reads one: result[i] holds ranges[i], shorter
// only at the end of the file. Servers answer with a multipart/byteranges
// body, whose parts they may merge and reorder, or with one part covering
// every range. A source answering otherwise, as with the whole file, is
// not read, and is remembered: then and since, ErrMultiRangeUnsupported.
func (s *Source) FetchRanges(ctx context.Context, ranges []container.Range) ([][]byte, error) {
	result := make([][]byte, len(ranges))
	size, known := s.knownSize()
	var asked []int
	var length int64
	for i, r := range ranges {
		if r.Off < 0 || r.N < 0 {
			return nil, fmt.Errorf("fetching %d bytes at %d: invalid range", r.N, r.Off)
		}
		if r.N > 0 && (!known || r.Off < size) {
			asked = append(asked, i)
			length += int64(r.N)
		}
	}
	switch {
	case len(asked) == 0:
		return result, nil
	case len(asked) == 1:
		data, err := s.Fetch(ctx, ranges[asked[0]].Off, ranges[asked[0]].N)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		result[asked[0]] = data
		return result, nil
	case s.multiRangeUnsupported():
		return nil, container.ErrMultiRangeUnsupported
	}
	var header strings.Builder
	header.WriteString("bytes=")
	for j, i := range asked {
		if j > 0 {
			header.WriteByte(',')
		}
		r := ranges[i]
		header.WriteString(strconv.FormatInt(r.Off, 10) + "-" + strconv.FormatInt(r.Off+int64(r.N)-1, 10))
	}
	err := s.exchange(ctx, header.String(), length, func(response *http.Response) error {
		// Merged parts may hold the bytes between the ranges too: a part
		// much larger than the ranges asked is not read.
		clear(result)
		parts := &multiRange{source: s, ranges: ranges, asked: asked, result: result, budget: 2*length + 1<<20}
		return parts.read(response)
	})
	switch {
	case err == nil:
		return result, nil
	case errors.Is(err, container.ErrMultiRangeUnsupported):
		s.mu.Lock()
		s.singleRanges = true
		s.mu.Unlock()
		s.cache.logger.Debug("A source does not serve several ranges at once", "source", s.id)
	}
	return nil, err
}

func (s *Source) multiRangeUnsupported() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.singleRanges
}

// multiRange is the answer to a request for several ranges being read.
type multiRange struct {
	source *Source
	ranges []container.Range
	// asked are the indexes of the ranges asked, and result where each
	// range read goes.
	asked  []int
	result [][]byte
	// budget is the bytes parts may still take.
	budget int64
}

// read reads the response to a request for several ranges into the
// results, the body only when it is made of them.
func (m *multiRange) read(response *http.Response) error {
	switch response.StatusCode {
	case http.StatusOK:
		m.source.learn(response.ContentLength, response.Header.Get("Content-Type"))
		return fmt.Errorf("%w: HTTP 200", container.ErrMultiRangeUnsupported)
	case http.StatusRequestedRangeNotSatisfiable:
		m.source.unsatisfiable(response)
		return io.EOF
	}
	mediaType, params, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	switch {
	case err == nil && mediaType == "multipart/byteranges" && params["boundary"] != "":
		parts := multipart.NewReader(response.Body, params["boundary"])
		for {
			part, err := parts.NextRawPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return fmt.Errorf("%w: %w: %v", ErrUnavailable, errBroken, err)
			}
			if err := m.part(part.Header.Get("Content-Range"), part, false); err != nil {
				return err
			}
		}
	case err == nil && strings.HasPrefix(mediaType, "multipart/"):
		return fmt.Errorf("%w: a body of type %s", container.ErrMultiRangeUnsupported, mediaType)
	default:
		// One part: the server merged the ranges asked.
		if err := m.part(response.Header.Get("Content-Range"), response.Body, true); err != nil {
			return err
		}
	}
	size, known := m.source.knownSize()
	for _, i := range m.asked {
		switch {
		case m.result[i] != nil:
		case known && m.ranges[i].Off >= size:
			// The range starts past the end, which the source told.
			m.result[i] = []byte{}
		default:
			return fmt.Errorf("%w: the range %d-%d is missing", container.ErrMultiRangeUnsupported,
				m.ranges[i].Off, m.ranges[i].Off+int64(m.ranges[i].N)-1)
		}
	}
	return nil
}

// part reads a part of the answer, the bytes value, its Content-Range,
// tells, into the results of the ranges it covers whole. A part larger
// than the budget, or alone not covering them all, is not read; one of a
// multipart body covering none is skipped.
func (m *multiRange) part(value string, body io.Reader, alone bool) error {
	start, end, total, ok := contentRange(value)
	if !ok || end < start {
		return fmt.Errorf("%w: a part of range %q", container.ErrMultiRangeUnsupported, value)
	}
	m.source.learn(total, "")
	// Ranges past the end of the file, which the part tells, have no part.
	var covered []int
	past := 0
	for _, i := range m.asked {
		r := m.ranges[i]
		n := int64(r.N)
		if total >= 0 {
			n = min(n, total-r.Off)
		}
		switch {
		case n <= 0:
			past++
		case r.Off >= start && r.Off+n-1 <= end:
			covered = append(covered, i)
		}
	}
	length := end - start + 1
	if alone && len(covered)+past != len(m.asked) || length > m.budget {
		return fmt.Errorf("%w: a part of range %q", container.ErrMultiRangeUnsupported, value)
	}
	m.budget -= length
	if len(covered) == 0 {
		return nil
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(body, data); err != nil {
		return fmt.Errorf("%w: %w: %v", ErrUnavailable, errBroken, err)
	}
	for _, i := range covered {
		r := m.ranges[i]
		m.result[i] = data[r.Off-start:][:min(int64(r.N), end+1-r.Off)]
	}
	return nil
}

// exchange sends the request of Fetch or FetchRanges for ranges, the value
// of a Range header asking length bytes, and has read read its answer: a
// 200, a 206 or a 416, closed once read. The request goes through the
// source's location, renewed once when it expired, spaced from the others.
// It is sent again, a bounded number of times, when the source asks to
// slow down or fails, when it breaks, or when it takes longer than an
// attempt may: a host that stalls must not hold a read for minutes.
func (s *Source) exchange(ctx context.Context, ranges string, length int64, read func(*http.Response) error) error {
	timeout := s.cache.attemptTime + time.Duration(float64(length)/attemptRate*float64(time.Second))
	renewed := false
	for attempt := 1; ; attempt++ {
		if err := s.pace(ctx); err != nil {
			return err
		}
		s.mu.Lock()
		location, renew := s.location, s.renew
		s.mu.Unlock()
		header := http.Header{}
		for name, value := range location.Headers {
			header.Set(name, value)
		}
		header.Set("Range", ranges)
		attemptCtx, cancel := context.WithTimeout(ctx, timeout)
		response, err := s.cache.opener.Open(attemptCtx, http.MethodGet, location.URL, header, location.Confined)
		if err != nil {
			cancel()
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if attempt >= attempts {
				return fmt.Errorf("%w: %v", ErrUnavailable, err)
			}
			s.holdOff(backoff(attempt, ""))
			continue
		}
		retryAfter := ""
		switch status := response.StatusCode; {
		case status == http.StatusOK || status == http.StatusPartialContent || status == http.StatusRequestedRangeNotSatisfiable:
			err := read(response)
			response.Body.Close()
			cancel()
			switch {
			case !errors.Is(err, errBroken):
				return err
			case ctx.Err() != nil:
				return ctx.Err()
			case attempt >= attempts:
				return err
			}
		case (status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusNotFound || status == http.StatusGone) &&
			renew != nil && !renewed:
			response.Body.Close()
			cancel()
			renewed = true
			fresh, err := renew(ctx)
			if err != nil {
				return fmt.Errorf("%w: HTTP %d, and the link could not be renewed: %v", ErrUnavailable, status, err)
			}
			s.mu.Lock()
			s.location = fresh
			s.mu.Unlock()
			s.cache.logger.Debug("A source link was renewed", "source", s.id)
			attempt = 0
			continue
		case (status == http.StatusTooManyRequests || status == http.StatusBadGateway || status == http.StatusServiceUnavailable ||
			status == http.StatusGatewayTimeout) && attempt < attempts:
			response.Body.Close()
			cancel()
			retryAfter = response.Header.Get("Retry-After")
		default:
			response.Body.Close()
			cancel()
			return fmt.Errorf("%w: HTTP %d", ErrUnavailable, status)
		}
		s.holdOff(backoff(attempt, retryAfter))
	}
}

// unsatisfiable records the size a 416 answer tells, and closes it.
func (s *Source) unsatisfiable(response *http.Response) {
	response.Body.Close()
	if _, _, total, ok := contentRange(response.Header.Get("Content-Range")); ok && total >= 0 {
		s.end(total)
	}
}

// pace waits for the next time the source may be sent a request of Fetch
// or FetchRanges, and takes it.
func (s *Source) pace(ctx context.Context) error {
	s.mu.Lock()
	now := time.Now()
	at := s.nextFetch
	if at.Before(now) {
		at = now
	}
	s.nextFetch = at.Add(s.cache.fetchInterval)
	s.mu.Unlock()
	return pause(ctx, at.Sub(now))
}

// holdOff sends the source no request of Fetch or FetchRanges for d: a
// host asking to slow down is asked nothing meanwhile, by any of them.
func (s *Source) holdOff(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if at := time.Now().Add(d); at.After(s.nextFetch) {
		s.nextFetch = at
	}
}

// pause waits for d, or until ctx is done.
func pause(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// learn records what a response tells about the source: its total size,
// when known, and its media type.
func (s *Source) learn(total int64, contentType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if total >= 0 {
		s.size = total
	}
	if s.contentType == "" {
		s.contentType = contentType
	}
}

func (s *Source) sleep(d time.Duration) {
	select {
	case <-time.After(d):
	case <-s.ctx.Done():
	}
}

// backoff is how long to wait before the next attempt: what the source
// asks for, within reason, or a growing delay.
func backoff(attempt int, retryAfter string) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && seconds >= 0 {
		return min(time.Duration(seconds)*time.Second, maxWait)
	}
	return min(time.Duration(1<<attempt)*250*time.Millisecond, maxWait)
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
// which read it from a loopback address.
func (s *Source) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	size, err := s.Size(r.Context())
	if err != nil {
		http.Error(w, "source unavailable", http.StatusBadGateway)
		return
	}
	contentType := s.ContentType()
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, r, "", time.Time{}, io.NewSectionReader(contextReader{s, r.Context()}, 0, size))
}

// contextReader reads a source on behalf of one request.
type contextReader struct {
	source *Source
	ctx    context.Context
}

func (r contextReader) ReadAt(p []byte, off int64) (int, error) {
	return r.source.ReadAt(r.ctx, p, off)
}
