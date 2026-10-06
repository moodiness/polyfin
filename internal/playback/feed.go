package playback

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

const (
	// feedBuffer is how much of a live stream a feed keeps for its
	// readers: about 8 seconds at 6 Mb/s. A reader that falls further
	// behind is dropped rather than holding the others back.
	feedBuffer = 6 << 20
	// feedJoin is how far back in what it keeps a reader joining a running
	// feed starts: a few seconds, so that FFmpeg finds a keyframe at once.
	feedJoin = 3 << 20
	// A source must send sniffSize bytes; fewer than sniffLeast is no
	// stream.
	sniffSize  = 4 << 10
	sniffLeast = 1 << 10
	// tsPacket is the size of an MPEG-TS packet.
	tsPacket = 188
)

// liveTimes are the times of live streams.
type liveTimes struct {
	// grace is how long a feed stays open after its last reader left, so
	// that a player switching back, or FFmpeg started after the analysis,
	// finds it running.
	grace time.Duration
	// stall ends a feed that received nothing for that long: its readers
	// see the stream end, and FFmpeg reads it again.
	stall time.Duration
	// A source must answer within answer, and send its first sniffSize
	// bytes within body.
	answer, body time.Duration
	// slotWait is how long an open refused right after Polyfin closed
	// another stream of the same source is tried again: providers count a
	// connection a few seconds after it closed.
	slotWait time.Duration
}

// defaultLiveTimes are the times of live streams; tests shorten them.
var defaultLiveTimes = liveTimes{grace: 20 * time.Second, stall: 10 * time.Second, answer: 5 * time.Second, body: 3 * time.Second,
	slotWait: 5 * time.Second}

// timingLocked returns the times of the service's live streams. The
// caller holds st.mu.
func (st *liveState) timingLocked() liveTimes {
	if st.times == (liveTimes{}) {
		return defaultLiveTimes
	}
	return st.times
}

// Codes of the ways a live stream failed, as iptv.ReportStream records
// them.
const (
	// LiveDead is an answer that is no live stream: an error status, a
	// web page, an empty or short body, bytes of no known container.
	LiveDead = "dead"
	// LiveRefused is a source that would not serve the stream now: 401,
	// 403, 429, 458, 503, or its connections all in use.
	LiveRefused = "refused"
	// LiveTimeout is a source from which nothing came in time.
	LiveTimeout = "timeout"
)

var (
	// ErrLiveDead, ErrLiveRefused and ErrLiveTimeout are the failures of a
	// live stream (see LiveFailure).
	ErrLiveDead    = errors.New("the source answered no live stream")
	ErrLiveRefused = errors.New("the source refused to serve the stream now")
	ErrLiveTimeout = errors.New("nothing came from the source in time")
	// ErrSlotsInUse reports a source whose connections Polyfin uses all,
	// for channels others watch.
	ErrSlotsInUse = fmt.Errorf("%w: all the provider's connections are in use", ErrLiveRefused)
	// ErrSlowReader reports a reader that fell behind what a feed keeps.
	ErrSlowReader = errors.New("a reader fell behind the live stream")
	// errManifest reports a stream that is an HLS playlist, which has no
	// feed: its files are relayed one by one.
	errManifest = errors.New("the stream is an HLS playlist")
)

// LiveFailure classifies an error of a live stream: LiveDead, LiveRefused,
// LiveTimeout, or "" for none of these.
func LiveFailure(err error) string {
	switch {
	case errors.Is(err, ErrLiveRefused):
		return LiveRefused
	case errors.Is(err, ErrLiveTimeout), errors.Is(err, context.DeadlineExceeded):
		return LiveTimeout
	case errors.Is(err, ErrLiveDead):
		return LiveDead
	}
	return ""
}

// liveState is what a Service shares of live streams: one feed per
// channel stream, the limits of the sources' connections, and who is told
// how streams answer. Its zero value is ready.
type liveState struct {
	mu    sync.Mutex
	feeds map[accounts.ID]*feed
	// manifests are the streams found to be HLS playlists.
	manifests map[accounts.ID]bool
	// connections and health are set by LiveSources.
	connections func(ctx context.Context, source accounts.ID) (int, error)
	health      func(ctx context.Context, version library.Version, failure string)
	// times are the times of live streams, defaultLiveTimes when unset.
	times liveTimes
}

// LiveSources sets how many streams a source may play at once
// (connections, 0 for no limit), and what is told how a channel's stream
// answered when opened (health; failure is "" when it played). It is
// called before the service is used.
func (s *Service) LiveSources(connections func(ctx context.Context, source accounts.ID) (int, error),
	health func(ctx context.Context, version library.Version, failure string)) {
	s.feeds.mu.Lock()
	defer s.feeds.mu.Unlock()
	s.feeds.connections, s.feeds.health = connections, health
}

// report tells LiveSources' health how a stream answered.
func (s *Service) report(ctx context.Context, version library.Version, err error) {
	s.feeds.mu.Lock()
	health := s.feeds.health
	s.feeds.mu.Unlock()
	if health == nil {
		return
	}
	failure := LiveFailure(err)
	if err != nil && failure == "" {
		failure = LiveDead
	}
	health(context.WithoutCancel(ctx), version, failure)
}

// manifest reports whether a stream was found to be an HLS playlist.
func (s *Service) manifest(version accounts.ID) bool {
	if analysis, ok := s.analyses.Get(version); ok {
		return Manifest(analysis)
	}
	s.feeds.mu.Lock()
	defer s.feeds.mu.Unlock()
	return s.feeds.manifests[version]
}

// feed is the one connection to a live stream that every reader of it
// shares: the analysis, FFmpeg and players relayed. It keeps the last
// feedBuffer bytes in a ring, and stays open liveTimes.grace after its last
// reader left.
type feed struct {
	s       *Service
	version library.Version
	source  accounts.ID
	opened  time.Time
	times   liveTimes
	// closed is closed once the connection to the source is.
	closed chan struct{}
	// ready is closed once the source answered, err set before.
	ready  chan struct{}
	err    error
	cancel context.CancelFunc

	mu       sync.Mutex
	buf      []byte
	received int64
	// changed is closed, then replaced, when bytes arrive or the feed ends.
	changed chan struct{}
	done    bool
	end     error
	readers map[*feedReader]bool
	grace   *time.Timer
	evicted bool
}

// feedReader reads a feed from where it joined.
type feedReader struct {
	f    *feed
	user accounts.ID
	ctx  context.Context
	pos  int64
}

type userKey struct{}

// ForUser marks ctx as a request of user, whose oldest stream of a source
// may be replaced when the source's connections are all in use.
func ForUser(ctx context.Context, user accounts.ID) context.Context {
	return context.WithValue(ctx, userKey{}, user)
}

func userOf(ctx context.Context) accounts.ID {
	user, _ := ctx.Value(userKey{}).(accounts.ID)
	return user
}

// openFeed returns a reader of a version's live stream, sharing its feed
// when one runs: else it opens one, checking that the source answers with
// a live stream (see sniff). An HLS playlist is errManifest.
func (s *Service) openFeed(ctx context.Context, version library.Version) (*feedReader, error) {
	user := userOf(ctx)
	for {
		st := &s.feeds
		st.mu.Lock()
		if st.feeds == nil {
			st.feeds, st.manifests = map[accounts.ID]*feed{}, map[accounts.ID]bool{}
		}
		f := st.feeds[version.ID]
		if f != nil && f.finished() {
			delete(st.feeds, version.ID)
			f = nil
		}
		if f == nil {
			f = &feed{s: s, version: version, source: version.Origin.Addon, opened: time.Now(), times: st.timingLocked(),
				ready: make(chan struct{}), closed: make(chan struct{}), buf: make([]byte, feedBuffer), changed: make(chan struct{}),
				readers: map[*feedReader]bool{}}
			st.feeds[version.ID] = f
			st.mu.Unlock()
			go f.open(user)
		} else {
			st.mu.Unlock()
		}
		r := f.join(ctx, user)
		select {
		case <-f.ready:
		case <-ctx.Done():
			r.Close()
			return nil, ctx.Err()
		}
		if f.err != nil {
			r.Close()
			return nil, f.err
		}
		// A feed evicted between its opening and this join is opened again.
		if r == nil {
			continue
		}
		return r, nil
	}
}

// finished reports whether a feed ended or failed: it takes no reader.
func (f *feed) finished() bool {
	select {
	case <-f.ready:
		if f.err != nil {
			return true
		}
	default:
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.done
}

// join adds a reader starting a few seconds back in what the feed keeps,
// on a packet boundary; nil when the feed ended.
func (f *feed) join(ctx context.Context, user accounts.ID) *feedReader {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.done && f.received > 0 || f.evicted {
		return nil
	}
	start := max(f.received-feedJoin, f.received-int64(len(f.buf)), 0)
	start = (start + tsPacket - 1) / tsPacket * tsPacket
	r := &feedReader{f: f, user: user, ctx: ctx, pos: start}
	f.readers[r] = true
	if f.grace != nil {
		f.grace.Stop()
		f.grace = nil
	}
	return r
}

// Read reads the feed, waiting for its next bytes.
func (r *feedReader) Read(p []byte) (int, error) {
	f := r.f
	for {
		f.mu.Lock()
		if !f.readers[r] {
			evicted := f.evicted
			f.mu.Unlock()
			if evicted {
				return 0, ErrSlotsInUse
			}
			return 0, ErrSlowReader
		}
		if oldest := f.received - int64(len(f.buf)); r.pos < oldest {
			f.mu.Unlock()
			r.Close()
			return 0, ErrSlowReader
		}
		if r.pos < f.received {
			n := int(min(int64(len(p)), f.received-r.pos))
			at := int(r.pos % int64(len(f.buf)))
			n = min(n, len(f.buf)-at)
			copy(p, f.buf[at:at+n])
			r.pos += int64(n)
			f.mu.Unlock()
			return n, nil
		}
		if f.done {
			end := f.end
			f.mu.Unlock()
			if end == nil {
				end = io.EOF
			}
			return 0, end
		}
		changed := f.changed
		f.mu.Unlock()
		select {
		case <-changed:
		case <-r.ctx.Done():
			return 0, r.ctx.Err()
		}
	}
}

// Close leaves the feed; the last reader leaving starts its grace.
func (r *feedReader) Close() error {
	if r == nil {
		return nil
	}
	f := r.f
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.readers[r] {
		return nil
	}
	delete(f.readers, r)
	if len(f.readers) == 0 && !f.done && f.grace == nil {
		f.grace = time.AfterFunc(f.times.grace, f.close)
	}
	return nil
}

// close ends the feed and its connection.
func (f *feed) close() {
	f.mu.Lock()
	if len(f.readers) > 0 && !f.evicted {
		f.mu.Unlock()
		return
	}
	f.mu.Unlock()
	f.s.feeds.mu.Lock()
	if f.s.feeds.feeds[f.version.ID] == f {
		delete(f.s.feeds.feeds, f.version.ID)
	}
	f.s.feeds.mu.Unlock()
	if f.cancel != nil {
		f.cancel()
	}
	f.finish(nil)
}

// evict closes a feed for another channel's stream: its readers end. It
// returns once the connection is closed, a second at most.
func (f *feed) evict() {
	f.mu.Lock()
	f.evicted = true
	for r := range f.readers {
		delete(f.readers, r)
	}
	f.mu.Unlock()
	f.close()
	select {
	case <-f.closed:
	case <-time.After(time.Second):
	}
}

// finish records the end of the feed and wakes its readers.
func (f *feed) finish(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.done {
		return
	}
	f.done, f.end = true, err
	if f.grace != nil {
		f.grace.Stop()
		f.grace = nil
	}
	close(f.changed)
	f.changed = make(chan struct{})
}

// open connects to the source, within the source's connections, checks
// that it answers with a live stream, then reads it until it ends, stalls
// or its last reader left for liveTimes.grace.
func (f *feed) open(user accounts.ID) {
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	defer close(f.closed)
	freed, err := f.s.makeRoom(ctx, f, user)
	var body io.ReadCloser
	var head []byte
	if err == nil {
		deadline := time.Now().Add(f.times.slotWait)
		for {
			body, head, err = f.s.sniff(ctx, f.version, f.times)
			if !freed || !errors.Is(err, ErrLiveRefused) || time.Now().After(deadline) {
				break
			}
			// The provider still counts the stream just closed.
			time.Sleep(time.Second)
		}
	}
	if errors.Is(err, errManifest) {
		f.s.feeds.mu.Lock()
		f.s.feeds.manifests[f.version.ID] = true
		f.s.feeds.mu.Unlock()
	}
	if err != nil {
		cancel()
		f.err = err
		f.finish(err)
		close(f.ready)
		f.close()
		return
	}
	f.write(head)
	close(f.ready)
	defer body.Close()
	stall := time.AfterFunc(f.times.stall, cancel)
	defer stall.Stop()
	chunk := make([]byte, 64<<10)
	for {
		n, err := body.Read(chunk)
		if n > 0 {
			stall.Reset(f.times.stall)
			f.write(chunk[:n])
		}
		if err != nil {
			switch {
			case ctx.Err() != nil && f.isEvicted():
				err = ErrSlotsInUse
			case ctx.Err() != nil:
				err = fmt.Errorf("%w: nothing for %s", ErrLiveTimeout, f.times.stall)
			case errors.Is(err, io.EOF):
				err = nil
			}
			f.finish(err)
			f.close()
			return
		}
	}
}

func (f *feed) isEvicted() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.evicted
}

// write adds bytes to the ring and wakes the readers.
func (f *feed) write(data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for len(data) > 0 {
		at := int(f.received % int64(len(f.buf)))
		n := copy(f.buf[at:], data)
		data = data[n:]
		f.received += int64(n)
	}
	close(f.changed)
	f.changed = make(chan struct{})
}

// makeRoom keeps a source within its connections before f opens one:
// feeds of the source that no reader uses are closed first, then the
// oldest only user reads; else it fails with ErrSlotsInUse. It reports
// whether it closed one, which the provider may still count a moment.
func (s *Service) makeRoom(ctx context.Context, f *feed, user accounts.ID) (bool, error) {
	st := &s.feeds
	st.mu.Lock()
	limits := st.connections
	st.mu.Unlock()
	if limits == nil || f.source == (accounts.ID{}) {
		return false, nil
	}
	limit, err := limits(ctx, f.source)
	if err != nil || limit <= 0 {
		return false, err
	}
	st.mu.Lock()
	var open []*feed
	for _, other := range st.feeds {
		if other != f && other.source == f.source && !other.finished() {
			open = append(open, other)
		}
	}
	st.mu.Unlock()
	if len(open) < limit {
		return false, nil
	}
	slices.SortFunc(open, func(a, b *feed) int { return a.opened.Compare(b.opened) })
	idle := func(other *feed) bool {
		other.mu.Lock()
		defer other.mu.Unlock()
		return len(other.readers) == 0
	}
	mine := func(other *feed) bool {
		other.mu.Lock()
		defer other.mu.Unlock()
		for r := range other.readers {
			if r.user != user {
				return false
			}
		}
		return user != (accounts.ID{})
	}
	freed := false
	for _, choose := range []func(*feed) bool{idle, mine} {
		for _, other := range open {
			if len(open) < limit {
				return freed, nil
			}
			if choose(other) {
				other.evict()
				open = slices.DeleteFunc(open, func(o *feed) bool { return o == other })
				freed = true
			}
		}
	}
	if len(open) < limit {
		return freed, nil
	}
	return freed, ErrSlotsInUse
}

// sniff opens a live stream and checks it is one before anything reads
// it: the source must answer within times.answer with a success, and its
// first sniffSize bytes (times.body to come) must start an HLS playlist
// (errManifest, the connection closed), MPEG-TS packets or an MP4 box.
// It returns the body and the bytes already read of it.
func (s *Service) sniff(ctx context.Context, version library.Version, times liveTimes) (io.ReadCloser, []byte, error) {
	header := http.Header{}
	for name, value := range version.Headers {
		header.Set(name, value)
	}
	answerCtx, cancelAnswer := context.WithCancel(ctx)
	answered := time.AfterFunc(times.answer, cancelAnswer)
	response, err := s.opener.Open(answerCtx, http.MethodGet, version.URL, header, version.Confined)
	if !answered.Stop() {
		if err == nil {
			response.Body.Close()
		}
		cancelAnswer()
		return nil, nil, fmt.Errorf("%w: no answer in %s", ErrLiveTimeout, times.answer)
	}
	if err != nil {
		cancelAnswer()
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		return nil, nil, fmt.Errorf("%w: %v", ErrLiveTimeout, err)
	}
	// The connection lives on with ctx; answerCtx only bounded the wait.
	stopAfter := context.AfterFunc(ctx, cancelAnswer)
	var body io.ReadCloser = &closer{ReadCloser: response.Body, done: func() { stopAfter(); cancelAnswer() }}
	if err := statusFailure(response); err != nil {
		body.Close()
		return nil, nil, err
	}
	head, rest, err := readHead(body, times.body)
	if err != nil {
		body.Close()
		return nil, nil, err
	}
	body = rest
	trimmed := bytes.TrimPrefix(head, []byte("\xef\xbb\xbf"))
	if bytes.HasPrefix(trimmed, []byte("#EXTM3U")) {
		body.Close()
		return nil, nil, errManifest
	}
	mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	switch mediaType {
	case "text/html", "application/json", "text/plain", "application/xml", "text/xml":
		body.Close()
		return nil, nil, fmt.Errorf("%w: a %s answer", ErrLiveDead, mediaType)
	}
	if len(head) < sniffLeast {
		body.Close()
		return nil, nil, fmt.Errorf("%w: %d bytes", ErrLiveDead, len(head))
	}
	if offset, ok := tsStart(head); ok {
		return body, head[offset:], nil
	}
	if mp4Box(head) {
		return body, head, nil
	}
	body.Close()
	return nil, nil, fmt.Errorf("%w: bytes of no known container", ErrLiveDead)
}

// closer runs done once its body is closed.
type closer struct {
	io.ReadCloser
	done func()
	once sync.Once
}

func (c *closer) Close() error {
	err := c.ReadCloser.Close()
	c.once.Do(c.done)
	return err
}

// statusFailure classifies an answer that is not a success.
func statusFailure(response *http.Response) error {
	switch status := response.StatusCode; {
	case status >= 200 && status < 300:
		return nil
	case status == http.StatusUnauthorized, status == http.StatusForbidden, status == http.StatusTooManyRequests,
		status == 458, status == http.StatusServiceUnavailable:
		return fmt.Errorf("%w: HTTP %d", ErrLiveRefused, status)
	case status >= 500:
		return fmt.Errorf("%w: HTTP %d", ErrLiveTimeout, status)
	default:
		return fmt.Errorf("%w: HTTP %d", ErrLiveDead, status)
	}
}

// readHead reads the first sniffSize bytes of a body, waiting wait at
// most: fewer is a short body when the body ended, a timeout when fewer
// than sniffLeast came. It returns the body to read on, which first gives
// what a read under way when the time ran out brings.
func readHead(body io.ReadCloser, wait time.Duration) ([]byte, io.ReadCloser, error) {
	head := make([]byte, sniffSize)
	results := make(chan readResult, 1)
	got := 0
	timeout := time.NewTimer(wait)
	defer timeout.Stop()
	for got < sniffSize {
		go func(at int) {
			n, err := body.Read(head[at:])
			results <- readResult{n, err}
		}(got)
		select {
		case r := <-results:
			got += r.n
			if r.err != nil {
				if got >= sniffLeast || bytes.HasPrefix(head[:got], []byte("#EXTM3U")) {
					return head[:got:got], body, nil
				}
				return nil, nil, fmt.Errorf("%w: %d bytes before the end", ErrLiveDead, got)
			}
		case <-timeout.C:
			if got < sniffLeast {
				return nil, nil, fmt.Errorf("%w: %d bytes in %s", ErrLiveTimeout, got, wait)
			}
			return head[:got:got], &pendingReader{ReadCloser: body, pending: results, data: head[got:]}, nil
		}
	}
	return head, body, nil
}

type readResult struct {
	n   int
	err error
}

// pendingReader reads first what a read started before brings, into
// data, then its body.
type pendingReader struct {
	io.ReadCloser
	pending <-chan readResult
	data    []byte
	err     error
}

func (r *pendingReader) Read(p []byte) (int, error) {
	if r.pending != nil {
		result := <-r.pending
		r.pending, r.data, r.err = nil, r.data[:result.n], result.err
	}
	if len(r.data) > 0 {
		n := copy(p, r.data)
		r.data = r.data[n:]
		return n, nil
	}
	if r.err != nil {
		return 0, r.err
	}
	return r.ReadCloser.Read(p)
}

// tsStart finds where MPEG-TS packets start in head: a sync byte repeated
// three times a packet apart, within the first packet.
func tsStart(head []byte) (int, bool) {
	for offset := 0; offset < tsPacket && offset+2*tsPacket < len(head); offset++ {
		if head[offset] == 0x47 && head[offset+tsPacket] == 0x47 && head[offset+2*tsPacket] == 0x47 {
			return offset, true
		}
	}
	return 0, false
}

// mp4Box reports whether head starts with an MP4 box a fragmented stream
// starts with.
func mp4Box(head []byte) bool {
	if len(head) < 8 {
		return false
	}
	switch string(head[4:8]) {
	case "ftyp", "styp", "moof", "sidx", "free", "moov":
		return true
	}
	return false
}

// liveReaders counts the readers of a version's feed, 0 without one.
func (s *Service) liveReaders(version accounts.ID) int {
	s.feeds.mu.Lock()
	f := s.feeds.feeds[version]
	s.feeds.mu.Unlock()
	if f == nil {
		return 0
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.readers)
}

// closeIdleFeed closes a version's feed when no reader uses it, before a
// player is sent to the source itself.
func (s *Service) closeIdleFeed(version accounts.ID) {
	s.feeds.mu.Lock()
	f := s.feeds.feeds[version]
	s.feeds.mu.Unlock()
	if f == nil {
		return
	}
	f.mu.Lock()
	idle := len(f.readers) == 0
	f.mu.Unlock()
	if idle {
		f.evict()
	}
}

// ServeFeed relays a channel's MPEG-TS stream to a player from its feed,
// shared with whatever else reads it, until the player leaves or the
// stream ends. The request's context may carry the user (see ForUser). An
// HLS playlist is errManifest, nothing written.
func (s *Service) ServeFeed(w http.ResponseWriter, r *http.Request, version library.Version) error {
	reader, err := s.openFeed(r.Context(), version)
	if errors.Is(err, errManifest) {
		return err
	}
	if err != nil {
		http.Error(w, "source unavailable", http.StatusBadGateway)
		return err
	}
	defer reader.Close()
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return nil
	}
	buffer := copyBuffers.Get().(*[256 << 10]byte)
	defer copyBuffers.Put(buffer)
	_, err = io.CopyBuffer(flushing{w}, reader, buffer[:])
	if errors.Is(err, io.EOF) || r.Context().Err() != nil {
		return nil
	}
	return err
}

// flushing sends each write to the player at once: a live stream comes
// in small pieces.
type flushing struct{ w http.ResponseWriter }

func (f flushing) Write(p []byte) (int, error) {
	n, err := f.w.Write(p)
	if flusher, ok := f.w.(http.Flusher); ok {
		flusher.Flush()
	}
	return n, err
}
