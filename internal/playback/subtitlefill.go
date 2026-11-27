package playback

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/source"
)

// Subtitle tracks read through the source cache.
//
// A host that serves one range per request would take a request for nearly
// every block of a movie's dialogue, hundreds, past a track's share (see
// trackRequests). A track read takes the blocks the source cache holds
// with no request, as a relayed playback brought them in; a track that
// would still take more awaits a fill. While nothing plays, a fill reads
// into the cache the span of the file from the first block of the tracks
// awaiting to their last, as Source.Preload reads: in pieces, at the
// preloading speed, above its free-space reserve, a quarter of the cache
// a pass at most, the pieces cached skipped, so that a fill a playback
// held, or a pass ended, resumes where it stopped. The tracks are then
// read whole from the cache, with no request, kept, and offered as files;
// a fill that does not end offers none.
//
// Only the versions an app played, or asked such a track of, are filled,
// one at a time, the one asked last first. The queue, and what bounds the
// fills (the day's bytes, the bytes of each version, the hosts held), are
// not kept across restarts: the next play queues its version again.

const (
	// fillQuiet is how long nothing must have played from a host, and
	// since a version was last asked, before its fill starts: an app that
	// asked for a playback starts reading it a moment later, and the next
	// episode starts a moment after one ends.
	fillQuiet = 2 * time.Minute
	// fillPoll is how often a fill under way looks whether a playback
	// started, and fillWait how often the fills waiting look whether
	// their host is quiet.
	fillPoll = time.Second
	fillWait = 30 * time.Second
	// fillPause is how long a fill waits once the disk reached the
	// reserve, or once it read its share of the cache for a pass.
	fillPause = 15 * time.Minute
	// fillRetry is how long a version's fill waits after it failed,
	// doubled after each failure in a row; it is given up after
	// maxFillFailures.
	fillRetry       = time.Hour
	maxFillFailures = 4
	// quotaWait is how long the fills of a host that answered that its
	// bandwidth quota is spent wait, and slowWait those of one that asked
	// to slow down or was overloaded.
	quotaWait = 24 * time.Hour
	slowWait  = time.Hour
	// unfilledFor is how long a version whose fill was given up is not
	// queued again.
	unfilledFor = 24 * time.Hour
	// fillAsked is how long a version stays queued after it was last
	// asked, and maxFills how many are queued at most.
	fillAsked = 3 * 24 * time.Hour
	maxFills  = 100
	// A fill reads a span of at most 1/fillShare of the source cache's
	// size, which stays cached while its tracks are read, a pass at most
	// 1/fillPass of it, as next-episode preloading does, so that the
	// parts played stay, and its fills fetch at most fillRounds times its
	// span: a cache evicting what they read faster than they read is not
	// filled. Over fillDay, the fills fetch at most the cache's size;
	// fillPiece, what Preload reads at a time, must be left for a pass.
	fillShare  = 2
	fillPass   = 4
	fillRounds = 2
	fillDay    = 24 * time.Hour
	fillPiece  = 8 << 20
	// statusQuotaSpent is the status of a host whose bandwidth quota is
	// spent, until it resets: 509 Bandwidth Limit Exceeded.
	statusQuotaSpent = 509
)

var (
	// errFillHeld reports a fill stopped for a playback of its host.
	errFillHeld = errors.New("a playback reads from the host")
	// errFillDay reports a fill past what the fills may fetch today.
	errFillDay = errors.New("the fills read what they may today")
	// errFillTooLarge reports a span too large for the source cache, and
	// errFillSpent a version whose fills fetched too much already.
	errFillTooLarge = errors.New("the subtitle blocks span too much of the file for the source cache")
	errFillSpent    = errors.New("the source cache evicted what the fills read before it was used")
)

// fillState is the fills queued and under way.
type fillState struct {
	mu   sync.Mutex
	jobs map[accounts.ID]*fillJob
	// busy holds, by host, when a playback was last seen reading from it,
	// and held when its fills may start again, once its quota reset or
	// its asking to slow down passed; both drop what no longer holds a
	// fill back.
	busy, held map[string]time.Time
	// day is when the day of fills began, spent what they fetched since,
	// and full whether one found too little left.
	day   time.Time
	spent int64
	full  bool
	// running tells that the loop runs, loops counts it, and wake has it
	// look again.
	running bool
	loops   sync.WaitGroup
	wake    chan struct{}
	// ctx ends once the service closes.
	ctx    context.Context
	cancel context.CancelFunc
	// unfilled are the versions whose fill was given up lately.
	unfilled *cache.Cache[accounts.ID, error]
	// idle reports whether no app plays anything (see IdleWhen).
	idle atomic.Pointer[func() bool]
	// quiet, poll and wait are fillQuiet, fillPoll and fillWait, which
	// tests shorten; speed, when set, gives the fills' speed and reserve
	// rather than the settings, which tests raise; limit, when set, the
	// source cache's size, which tests lower.
	quiet, poll, wait time.Duration
	speed             func() (rate, reserve int64)
	limit             func() int64
}

// fillJob is a version queued for a fill.
type fillJob struct {
	version  library.Version
	analysis media.Analysis
	// host is the host its source reads from, once a fill opened it: the
	// version's link names the server's own for the files of storage the
	// server reads itself, as SFTP or S3.
	host string
	// asked is when an app last played the version or asked for a track
	// awaiting, and retry when the fill may start again after a failure,
	// failures counting those in a row.
	asked, retry time.Time
	failures     int
	// fetched counts the bytes its fills fetched.
	fetched int64
}

// hostLocked is the host a job's fills read from, as far as known. The
// fills' lock is held.
func (job *fillJob) hostLocked() string {
	if job.host != "" {
		return job.host
	}
	return hostOf(job.version.URL)
}

func newFillState() fillState {
	ctx, cancel := context.WithCancel(context.Background())
	return fillState{jobs: map[accounts.ID]*fillJob{}, busy: map[string]time.Time{}, held: map[string]time.Time{},
		wake: make(chan struct{}, 1), ctx: ctx, cancel: cancel, unfilled: cache.New[accounts.ID, error](2000, unfilledFor),
		quiet: fillQuiet, poll: fillPoll, wait: fillWait}
}

// IdleWhen has the fills of subtitle tracks wait while idle reports that
// an app plays something: a playback sent to the source itself reads
// nothing through the server, which sees only what apps report.
func (s *Service) IdleWhen(idle func() bool) {
	s.fills.idle.Store(&idle)
}

// FillSubtitles has the subtitle tracks of a version that await a fill
// read into the source cache in the background: an app is about to play
// the version, or asked for one of them. Nothing happens when none awaits.
func (s *Service) FillSubtitles(version library.Version, analysis media.Analysis) {
	if index, ok := s.locations.Get(version.ID); ok && len(index.awaiting) > 0 {
		s.queueFill(version, analysis)
	}
}

// queueFill queues the fill of a version, asked now, unless it was given
// up lately. A torrent's file is never filled: only playbacks join its
// swarm. A full queue drops the version asked the longest ago.
func (s *Service) queueFill(version library.Version, analysis media.Analysis) {
	f := &s.fills
	if version.Torrent != nil {
		return
	}
	if _, given := f.unfilled.Get(version.ID); given {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ctx.Err() != nil {
		return
	}
	job := f.jobs[version.ID]
	if job == nil {
		if len(f.jobs) >= maxFills {
			var oldest *fillJob
			for _, other := range f.jobs {
				if oldest == nil || other.asked.Before(oldest.asked) {
					oldest = other
				}
			}
			delete(f.jobs, oldest.version.ID)
		}
		job = &fillJob{}
		f.jobs[version.ID] = job
		s.logger.Debug("Subtitle tracks of a version are to be read into the source cache while nothing plays", "addon", version.AddonLog)
	}
	job.version, job.analysis, job.asked = version, analysis, time.Now()
	if f.running {
		select {
		case f.wake <- struct{}{}:
		default:
		}
		return
	}
	f.running = true
	f.loops.Add(1)
	go s.fillQueued()
}

// forgetFill drops the fill of a version whose file changed.
func (s *Service) forgetFill(version accounts.ID) {
	s.fills.mu.Lock()
	defer s.fills.mu.Unlock()
	delete(s.fills.jobs, version)
}

// stopFills ends the fills, and waits for the one under way.
func (s *Service) stopFills() {
	s.fills.mu.Lock()
	s.fills.cancel()
	s.fills.mu.Unlock()
	s.fills.loops.Wait()
}

// fillBusy reports whether something plays that a fill from host waits
// for: an app's playback, as the apps report, or a read of a file of host
// for a playback.
func (s *Service) fillBusy(host string) bool {
	if idle := s.fills.idle.Load(); idle != nil && !(*idle)() {
		return true
	}
	return s.sources.Playing(host)
}

// fillQueued runs the fills queued, one at a time, until none is left or
// the service closes.
func (s *Service) fillQueued() {
	f := &s.fills
	defer f.loops.Done()
	for {
		job, wait := s.nextFill(time.Now())
		switch {
		case job != nil:
			s.runFill(job)
		case wait == 0:
			return
		default:
			timer := time.NewTimer(wait)
			select {
			case <-timer.C:
			case <-f.wake:
			case <-f.ctx.Done():
			}
			timer.Stop()
		}
		if f.ctx.Err() != nil {
			f.mu.Lock()
			f.running = false
			f.mu.Unlock()
			return
		}
	}
}

// nextFill returns the fill to run at now, the one asked last among those
// ready, or else how long to wait before looking again; neither once none
// is queued, the loop then ending. A fill is ready once its host and its
// version were quiet for fillQuiet, its host's quota or slowing down
// passed, its retry came, and the fills may still fetch today.
func (s *Service) nextFill(now time.Time) (*fillJob, time.Duration) {
	f := &s.fills
	f.mu.Lock()
	defer f.mu.Unlock()
	if now.Sub(f.day) >= fillDay {
		f.day, f.spent, f.full = now, 0, false
	}
	for host, at := range f.busy {
		if now.Sub(at) >= f.quiet {
			delete(f.busy, host)
		}
	}
	for host, until := range f.held {
		if !until.After(now) {
			delete(f.held, host)
		}
	}
	wait := f.wait
	var next *fillJob
	for id, job := range f.jobs {
		if now.Sub(job.asked) > fillAsked {
			delete(f.jobs, id)
			continue
		}
		host := job.hostLocked()
		if s.fillBusy(host) {
			f.busy[host] = now
		}
		ready := job.asked.Add(f.quiet)
		for _, at := range []time.Time{f.busy[host].Add(f.quiet), job.retry, f.held[host]} {
			if at.After(ready) {
				ready = at
			}
		}
		if f.full && f.day.Add(fillDay).After(ready) {
			ready = f.day.Add(fillDay)
		}
		if ready.After(now) {
			wait = min(wait, ready.Sub(now))
			continue
		}
		if next == nil || job.asked.After(next.asked) {
			next = job
		}
	}
	if next != nil {
		return next, 0
	}
	if len(f.jobs) == 0 {
		f.running = false
		return nil, 0
	}
	return nil, wait
}

// runFill runs a fill, and has the queue keep or drop it as it went. A
// panic of it, of what reads the version's file, is logged with its stack,
// and the version is not filled again for a while; the queue goes on.
func (s *Service) runFill(job *fillJob) {
	f := &s.fills
	f.mu.Lock()
	version, analysis, fetched := job.version, job.analysis, job.fetched
	f.mu.Unlock()
	defer func() {
		if cause := recover(); cause != nil {
			s.logger.Error("Reading subtitle tracks into the source cache ended on a panic", "addon", version.AddonLog,
				"panic", fmt.Sprint(cause), "stack", string(debug.Stack()))
			s.giveUpFill(job, fmt.Errorf("%w: %v", errSharedPanicked, cause))
		}
	}()
	src := s.open(version)
	host := src.Host()
	src.Release()
	f.mu.Lock()
	job.host = host
	f.mu.Unlock()
	more, err := s.fill(f.ctx, version, analysis, fetched, host)
	s.fillDone(job, more, err, time.Now())
}

// fillDone has the queue keep or drop a fill that fetched more bytes and
// ended with err at now.
func (s *Service) fillDone(job *fillJob, more int64, err error, now time.Time) {
	f := &s.fills
	f.mu.Lock()
	defer f.mu.Unlock()
	job.fetched += more
	f.spent += more
	version, host := job.version, job.hostLocked()
	var status *source.StatusError
	switch {
	case err == nil:
		if f.jobs[version.ID] == job {
			delete(f.jobs, version.ID)
		}
	case f.ctx.Err() != nil:
	case errors.Is(err, errFillHeld):
		f.busy[host] = now
		s.logger.Debug("Reading subtitle tracks into the source cache waits for a playback from their host to end",
			"addon", version.AddonLog, "fetched", more)
	case errors.Is(err, errFillDay):
		f.full = true
		s.logger.Debug("Reading subtitle tracks into the source cache waits: the fills read what they may today", "addon", version.AddonLog)
	case errors.Is(err, source.ErrPreloadHeld):
		job.retry = now.Add(fillPause)
		s.logger.Debug("Reading subtitle tracks into the source cache waits: the disk reached its reserve, or the pass read its share",
			"addon", version.AddonLog, "fetched", more)
	case errors.Is(err, errFillTooLarge) || errors.Is(err, errFillSpent):
		s.giveUpFillLocked(job, err)
	case errors.As(err, &status) && status.Status == statusQuotaSpent:
		f.held[host] = now.Add(quotaWait)
		s.logger.Info("A host answered that its bandwidth quota is spent: no subtitle track is read from it into the source cache for a day",
			"addon", version.AddonLog)
	case errors.Is(err, source.ErrSlowDown) || errors.As(err, &status) && slowStatus(status.Status):
		f.held[host] = now.Add(slowWait)
		s.logger.Debug("A host asked to slow down: no subtitle track is read from it into the source cache for an hour", "addon", version.AddonLog)
	default:
		job.failures++
		if job.failures >= maxFillFailures {
			s.giveUpFillLocked(job, err)
			return
		}
		job.retry = now.Add(fillRetry << (job.failures - 1))
		s.logger.Info("Subtitle tracks could not be read into the source cache: they are tried again later", "addon", version.AddonLog,
			"error", err)
	}
}

// slowStatus reports a status of a host asking to slow down or overloaded:
// 429, 502, 503 or 504.
func slowStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// giveUpFill drops a fill, not queued again for unfilledFor.
func (s *Service) giveUpFill(job *fillJob, err error) {
	s.fills.mu.Lock()
	defer s.fills.mu.Unlock()
	s.giveUpFillLocked(job, err)
}

// giveUpFillLocked is giveUpFill, s.fills.mu held.
func (s *Service) giveUpFillLocked(job *fillJob, err error) {
	if s.fills.jobs[job.version.ID] == job {
		delete(s.fills.jobs, job.version.ID)
	}
	s.fills.unfilled.Put(job.version.ID, err)
	s.logger.Info("Subtitle tracks inside a version are not read into the source cache: they are offered once repackaging has read the whole version",
		"addon", job.version.AddonLog, "error", err)
}

// fill reads into the source cache the span of a version's file holding
// the blocks of its tracks awaiting, from host, unless a playback from it
// comes, then reads the tracks from the cache and offers them. It returns
// the bytes fetched. fetched is what its fills fetched before.
func (s *Service) fill(ctx context.Context, version library.Version, analysis media.Analysis, fetched int64, host string) (int64, error) {
	index, ok := s.subtitleIndex(ctx, version, analysis)
	if !ok || len(index.awaiting) == 0 {
		return 0, nil
	}
	started := time.Now()
	ctx = Background(ctx)
	src := s.open(version)
	defer src.Release()
	var m *container.Matroska
	err := s.readSized(ctx, version, analysis, src, func(size int64) error {
		var err error
		m, err = container.OpenMatroska(ctx, src, size)
		return err
	})
	if err != nil {
		return 0, err
	}
	tracks := streamTracks(analysis, m.Tracks())
	start, end := int64(math.MaxInt64), int64(0)
	var streams []int
	for _, stream := range index.awaiting {
		track, ok := tracks[stream]
		if !ok {
			s.place(ctx, version.ID, stream, false, false)
			continue
		}
		from, to, err := m.SubtitleSpan(ctx, track.Number)
		switch {
		case lasting(err):
			s.place(ctx, version.ID, stream, false, false)
			continue
		case err != nil:
			return 0, err
		}
		start, end = min(start, from), max(end, to)
		streams = append(streams, stream)
	}
	if end <= start {
		return 0, nil
	}
	limit, span := s.cacheLimit(), end-start
	if span > limit/fillShare {
		return 0, fmt.Errorf("%w: %d bytes, in a source cache of %d", errFillTooLarge, span, limit)
	}
	missing := src.Missing(source.Span{Off: start, End: end})
	if fetched+missing > fillRounds*span {
		return 0, fmt.Errorf("%w: %d bytes fetched for a span of %d", errFillSpent, fetched, span)
	}
	left := s.fillsLeft(limit, time.Now())
	if left < min(missing, fillPiece) {
		return 0, errFillDay
	}
	var more int64
	if missing > 0 {
		s.logger.Debug("Reading subtitle tracks into the source cache", "addon", version.AddonLog, "bytes", missing)
		// A pass reads a piece at least, as far as the day leaves.
		pass := min(left, max(limit/fillPass, fillPiece))
		more, err = s.preloadSpan(ctx, src, host, source.Span{Off: start, End: end}, pass)
		if err != nil {
			return more, err
		}
	}
	// The tracks read whole are offered; the others are read again at the
	// next fill, as the cache may have evicted a part of the span.
	offered := 0
	var failed error
	for _, stream := range streams {
		if _, err := s.sharedTrack(ctx, version, analysis, stream); err != nil {
			if !lasting(err) {
				failed = err
			}
			continue
		}
		s.place(ctx, version.ID, stream, true, false)
		offered++
	}
	if failed != nil {
		return more, failed
	}
	if offered > 0 {
		s.logger.Info("Subtitle tracks read into the source cache are offered as files", "addon", version.AddonLog,
			"tracks", offered, "fetched", more, "duration", time.Since(started).Round(time.Second))
	}
	return more, nil
}

// cacheLimit is the source cache's size.
func (s *Service) cacheLimit() int64 {
	if s.fills.limit != nil {
		return s.fills.limit()
	}
	return s.sources.Usage().Limit
}

// preloadSpan reads span into the cache within the preloading's speed and
// reserve and budget bytes, unless something comes that a fill from host
// waits for: errFillHeld then, as soon as it is seen.
func (s *Service) preloadSpan(ctx context.Context, src *source.Source, host string, span source.Span, budget int64) (int64, error) {
	rate, reserve := s.fillSpeed()
	if s.fills.speed != nil {
		rate, reserve = s.fills.speed()
	}
	limits := source.Preloading{Rate: rate, Reserve: reserve, Budget: budget, Idle: func() bool { return !s.fillBusy(host) }}
	watched, stop := context.WithCancel(ctx)
	defer stop()
	var held atomic.Bool
	go func() {
		ticker := time.NewTicker(s.fills.poll)
		defer ticker.Stop()
		for {
			select {
			case <-watched.Done():
				return
			case <-ticker.C:
				if s.fillBusy(host) {
					held.Store(true)
					stop()
					return
				}
			}
		}
	}()
	fetched, err := src.Preload(watched, limits, func(int64) []source.Span { return []source.Span{span} })
	stop()
	if err != nil && (held.Load() || errors.Is(err, source.ErrPreloadHeld) && s.fillBusy(host)) {
		return fetched, errFillHeld
	}
	return fetched, err
}

// fillSpeed returns the bytes a second the fills read at most, and the
// free space they leave on the disk of the source cache: those of
// next-episode preloading while it is on, its defaults otherwise.
func (s *Service) fillSpeed() (rate, reserve int64) {
	settings := s.settings()
	mbps, gigabytes := accounts.DefaultPreloadMbps, accounts.DefaultPreloadReserveGB
	if settings.PreloadNextEpisodes {
		mbps, gigabytes = settings.PreloadMbps, settings.PreloadReserveGB
	}
	return int64(mbps) * 1e6 / 8, int64(gigabytes) * 1e9
}

// fillsLeft is what the fills may still fetch at now, limit a day.
func (s *Service) fillsLeft(limit int64, now time.Time) int64 {
	f := &s.fills
	f.mu.Lock()
	defer f.mu.Unlock()
	if now.Sub(f.day) >= fillDay {
		f.day, f.spent, f.full = now, 0, false
	}
	return limit - f.spent
}
