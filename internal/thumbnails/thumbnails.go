// Package thumbnails makes the scrubbing thumbnails (Jellyfin's trickplay)
// and chapter images of the versions played, and of the local folders'
// files, from their keyframes alone: during a play, those whose bytes the
// playback brought into the source cache; after it, the others, read
// through the container's index, a few dozen spans of a file rather than
// the whole of it, gently, once nobody plays from its host. A torrent's
// file gets only what its plays brought into the cache. FFmpeg decodes
// them; the database keeps the images and the result of each version.
package thumbnails

import (
	"context"
	"errors"
	"fmt"
	"image"
	"log/slog"
	"math"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/source"
)

const (
	// queueLength bounds the versions waiting for their images: one is
	// made at a time, and more would wait for long; a version dropped is
	// queued again by its next play.
	queueLength = 8
	// budget bounds the requests the images of a version take, its index
	// included: a long version's thumbnails show keyframes further apart.
	// A local folder's file takes none of these limits (see local).
	budget = 60
	// requestInterval spaces the requests to a host, at most perHour of
	// which go to it in an hour, over every version.
	requestInterval = 3 * time.Second
	perHour         = 120
	// slowDownPause is how long a host that asked to slow down (429) or
	// was overloaded (502, 503, 504) is sent no request for images.
	slowDownPause = 2 * time.Hour
	// pollInterval is how often waiting versions, and a host busy with a
	// playback, are checked again.
	pollInterval = 10 * time.Second
	// generationTime bounds the making of a version's images, waits for
	// playbacks and for its host's hour included.
	generationTime = 3 * time.Hour
	// failureTTL is how long a version whose images could not be made is
	// left alone, and refusalTTL one whose file cannot give them, or whose
	// host asked to slow down.
	failureTTL = 30 * time.Minute
	refusalTTL = 24 * time.Hour
	// chapterWidth is the width of chapter images, at most: apps show them
	// as cards of a few hundred pixels.
	chapterWidth = 640
	// touchEvery is how often the last use of a version's images is saved.
	touchEvery = time.Hour
)

// Playing is a playback under way: the device playing, and the URL of the
// version it plays, empty when it is not known. A playback of a version
// not known holds back no host.
type Playing struct {
	Device accounts.ID
	URL    string
}

// Reasons a version played gets no images, which Skip logs.
const (
	SkippedTurnedOff = "the settings turn thumbnails and chapter images off"
	SkippedDisc      = "a disc, not one file"
	SkippedRecording = "a recording or a Replay programme"
)

// ErrTurnedOff reports images asked for while the settings turn
// thumbnails and chapter images off.
var ErrTurnedOff = errors.New("the settings turn thumbnails and chapter images off")

// Options are what the service needs.
type Options struct {
	DB *pgxpool.Pool
	// FFmpeg is the FFmpeg executable; Hardware returns the GPU decoding
	// video of a codec in a bit depth, nil for none, nil itself for no GPU;
	// ToneMapping tells whether FFmpeg has the filters bringing HDR to SDR.
	FFmpeg      string
	Hardware    func(codec string, bitDepth int) *hls.Hardware
	ToneMapping bool
	// Settings returns the server settings, read as they apply.
	Settings func() accounts.Settings
	// Open opens a version's source, to be read with one attempt per
	// request; the caller releases it. A torrent's file is never opened.
	Open func(library.Version) Source
	// Analyzed returns a version's analysis, if it was analyzed; Analyze
	// analyzes a local folder's file that was not, as one queued before
	// any play is (see MakeLocal and MakeAgain), nil for none.
	Analyzed func(context.Context, accounts.ID) (media.Analysis, bool)
	Analyze  func(context.Context, library.Version) (media.Analysis, error)
	// Cached returns what the source cache holds of a version's file, false
	// when it holds nothing of it: thumbnails are made during a play when
	// it is set (see live). Converting reports whether a conversion runs
	// on the processor now, when they are not.
	Cached     func(library.Version) (Cached, bool)
	Converting func() bool
	// Pace spaces the requests to a host and how often waits are checked
	// again; zero for requestInterval and pollInterval. LiveEvery is how
	// often the thumbnails of the versions playing are extended, zero for
	// liveInterval. Tests shorten them.
	Pace, Poll, LiveEvery time.Duration
	Logger                *slog.Logger
}

// job is a version waiting for its images, after the playback of device
// stopped, zero for none, and the title it is of, as the log names it.
// done is closed once its images were made, or it left the queue.
type job struct {
	version library.Version
	title   string
	host    string
	after   accounts.ID
	done    chan struct{}
}

// newJob is a version to queue.
func newJob(version library.Version, title string, device accounts.ID) job {
	return job{version: version, title: title, host: hostOf(version.URL), after: device, done: make(chan struct{})}
}

// finish tells that the version left the queue.
func (j job) finish() {
	if j.done != nil {
		close(j.done)
	}
}

// local reports whether a version is a file of a local folder, a network
// share's included: read from the server's own storage rather than from a
// provider, its images take no budget, no pace and no hourly cap, and wait
// only for the playbacks of that same file.
func local(version library.Version) bool {
	return version.LocalFolder
}

// hostless reports whether making a version's images reads no provider's
// host: a local folder's file, or a torrent's, whose thumbnails come from
// what its plays brought into the source cache alone, never from its
// swarm, which only a playback joins.
func hostless(version library.Version) bool {
	return local(version) || version.Torrent != nil
}

// heldBy reports whether a playback holds the version back: the one that
// asked for it, one from its host, or for a local folder's or a torrent's
// file, one of that file.
func (j job) heldBy(p Playing) bool {
	if j.after != (accounts.ID{}) && p.Device == j.after {
		return true
	}
	if hostless(j.version) {
		return p.URL == j.version.URL
	}
	return busyHost(p, j.host)
}

// Service makes and keeps the images of versions.
type Service struct {
	Options
	cancel context.CancelFunc
	done   chan struct{}
	wake   chan struct{}
	gate   *gate

	mu   sync.Mutex
	jobs []job
	// working is set while the images of a version are made after its
	// play, generating naming it; extending names the version whose
	// thumbnails a play extends: neither works on the other's version.
	working               bool
	generating, extending accounts.ID
	// playing lists the playbacks under way: images wait for them.
	playing func() []Playing
	// decoding runs one FFmpeg decoding keyframes at a time.
	decoding sync.Mutex

	failed  *cache.Cache[accounts.ID, error]
	refused *cache.Cache[accounts.ID, error]
	touched *cache.Cache[accounts.ID, struct{}]
	// unextended holds the versions whose thumbnails a play cannot
	// extend, for failureTTL.
	unextended *cache.Cache[accounts.ID, error]
	// skipped holds the reason each version played got no images, logged
	// once a refusalTTL; the zero identifier stands for every version
	// while the settings turn the images off.
	skipped *cache.Cache[accounts.ID, string]

	// budget is that above, which tests lower.
	budget int
}

// New starts the service, which Close stops.
func New(options Options) *Service {
	if options.Pace == 0 {
		options.Pace = requestInterval
	}
	if options.Poll == 0 {
		options.Poll = pollInterval
	}
	if options.LiveEvery == 0 {
		options.LiveEvery = liveInterval
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{
		Options:    options,
		cancel:     cancel,
		done:       make(chan struct{}),
		wake:       make(chan struct{}, 1),
		gate:       newGate(options.Pace, perHour, time.Hour, options.Poll),
		failed:     cache.New[accounts.ID, error](2000, failureTTL),
		refused:    cache.New[accounts.ID, error](2000, refusalTTL),
		touched:    cache.New[accounts.ID, struct{}](5000, touchEvery),
		unextended: cache.New[accounts.ID, error](2000, failureTTL),
		skipped:    cache.New[accounts.ID, string](2000, refusalTTL),
		budget:     budget,
	}
	var wg sync.WaitGroup
	// Renewing a link the images read is background work, paced.
	wg.Go(func() { s.run(library.Background(ctx)) })
	if options.Cached != nil {
		wg.Go(func() { s.live(ctx) })
	}
	go func() {
		wg.Wait()
		close(s.done)
	}()
	return s
}

// Close stops making images, and waits for the version under way.
func (s *Service) Close() {
	s.cancel()
	<-s.done
}

// Queue asks for the images of a version, in the background: the
// thumbnails and chapter images the settings turn on and it lacks. title
// names it in the log. They are made once the playback of device has
// stopped, and while nobody plays from the version's host, or for a local
// folder's or a torrent's file, that file. A version recently failed,
// already waiting, or of a paused host, is not queued; neither is one
// when the queue is full. A torrent's file gets the thumbnails its plays
// brought into the source cache alone: during the play, then once after
// it, never read from its swarm.
func (s *Service) Queue(version library.Version, title string, device accounts.ID) {
	if !s.on() {
		s.Skip(version, title, SkippedTurnedOff)
		return
	}
	if s.resting(version) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.waiting(version.ID) {
		return
	}
	if len(s.jobs) >= queueLength {
		s.Logger.Debug("The images of a version were not queued: too many are waiting", "title", title, "addon", version.Addon)
		return
	}
	s.jobs = append(s.jobs, newJob(version, title, device))
	s.Wake()
}

// on reports whether the settings turn thumbnails or chapter images on.
func (s *Service) on() bool {
	settings := s.Settings()
	return settings.Trickplay || settings.ChapterImages
}

// resting reports whether a version is left alone for now: its images
// recently failed, its file cannot give them, or its host is paused.
func (s *Service) resting(version library.Version) bool {
	if _, failed := s.failed.Get(version.ID); failed {
		return true
	}
	if _, refused := s.refused.Get(version.ID); refused {
		return true
	}
	return !hostless(version) && s.gate.paused(hostOf(version.URL))
}

// MakeLocal makes the images the files of local folders lack, one version
// at a time, waiting for each, as the task Make thumbnails does. versions
// calls each with every version of the local folders and the title it is
// of, until each fails. Versions of other sources are passed over, and so
// are those whose file cannot give images, failed recently, or by their
// last result. It does nothing while the settings turn the images off,
// and stops once they do.
func (s *Service) MakeLocal(ctx context.Context, versions func(ctx context.Context, each func(library.Version, string) error) error) error {
	if !s.on() {
		return nil
	}
	err := versions(ctx, func(version library.Version, title string) error {
		if !s.on() {
			return ErrTurnedOff
		}
		if !local(version) || s.resting(version) {
			return nil
		}
		if cannot, err := s.cannot(ctx, version.ID); err != nil || cannot {
			return err
		}
		select {
		case <-s.queueNow(version, title):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	if errors.Is(err, ErrTurnedOff) {
		return nil
	}
	return err
}

// queueNow queues a version with no playback to wait for, the queue full
// or not, and returns what is closed once it left the queue. A version
// waiting already is left as it is.
func (s *Service) queueNow(version library.Version, title string) <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.jobs {
		if j.version.ID == version.ID {
			return j.done
		}
	}
	j := newJob(version, title, accounts.ID{})
	s.jobs = append(s.jobs, j)
	s.Wake()
	return j.done
}

// ErrNotLocal reports images asked to be made again of a version that is
// not a local folder's file.
var ErrNotLocal = errors.New("only the images of a local folder's file are made again")

// MakeAgain makes the images of a local folder's file again, replacing
// those it has: it drops its thumbnails and chapter images and its last
// result, forgets that it failed or could not give them, and queues it
// with no playback to wait for, the queue full or not. title names it in
// the log.
func (s *Service) MakeAgain(ctx context.Context, version library.Version, title string) error {
	if !local(version) {
		return ErrNotLocal
	}
	if !s.on() {
		return ErrTurnedOff
	}
	if err := s.forget(ctx, version.ID); err != nil {
		return err
	}
	s.failed.Delete(version.ID)
	s.refused.Delete(version.ID)
	s.unextended.Delete(version.ID)
	s.mu.Lock()
	for i, j := range s.jobs {
		if j.version.ID == version.ID {
			s.jobs[i].after = accounts.ID{}
		}
	}
	s.mu.Unlock()
	s.queueNow(version, title)
	return nil
}

// Title names a title in the log: an episode by its series, season and
// number, another title by its name and year.
func Title(item library.Item) string {
	switch {
	case item.Kind == library.KindEpisode:
		return fmt.Sprintf("%s S%02dE%02d", item.SeriesName, item.ParentIndexNumber, item.IndexNumber)
	case item.ProductionYear > 0:
		return fmt.Sprintf("%s (%d)", item.Name, item.ProductionYear)
	}
	return item.Name
}

// Skip logs that a version played gets no images, and why: one of the
// Skipped reasons. Each version's reason is logged once a day; while the
// settings turn the images off, once a day for every version.
func (s *Service) Skip(version library.Version, title, reason string) {
	key := version.ID
	if reason == SkippedTurnedOff {
		key = accounts.ID{}
	}
	if logged, ok := s.skipped.Get(key); ok && logged == reason {
		return
	}
	s.skipped.Put(key, reason)
	if key == (accounts.ID{}) {
		s.Logger.Info("The versions played get no images", "reason", reason)
		return
	}
	s.Logger.Info("A version played gets no images", "title", title, "addon", version.Addon, "reason", reason)
}

// WatchPlaybacks has images wait for the playbacks playing lists: the
// Jellyfin handler's sessions.
func (s *Service) WatchPlaybacks(playing func() []Playing) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.playing = playing
}

// playbacks lists the playbacks under way.
func (s *Service) playbacks() []Playing {
	s.mu.Lock()
	playing := s.playing
	s.mu.Unlock()
	if playing == nil {
		return nil
	}
	return playing()
}

// Wake checks the waiting versions again, as when a playback stopped.
func (s *Service) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) run(ctx context.Context) {
	timer := time.NewTimer(s.Poll)
	defer timer.Stop()
	for {
		if j, ok := s.next(); ok {
			s.generate(ctx, j)
			s.finished()
			j.finish()
			continue
		}
		timer.Reset(s.Poll)
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-timer.C:
		}
	}
}

// next takes the first waiting version that may start, and marks it as
// worked on: no playback holds it back (see job.heldBy), its host is
// neither paused nor out of requests for the hour unless it is a local
// folder's or a torrent's file, and no play extends its thumbnails now.
// Only the versions waiting before the playbacks are read are weighed: one
// queued as its playback starts would otherwise be let through by
// playbacks read before that playback began. Queue wakes the next check,
// which weighs it.
func (s *Service) next() (job, bool) {
	s.mu.Lock()
	waiting := len(s.jobs)
	s.mu.Unlock()
	if waiting == 0 {
		return job{}, false
	}
	playing := s.playbacks()
	s.mu.Lock()
	defer s.mu.Unlock()
	// Queue appends: versions queued meanwhile follow those counted.
	for i, j := range s.jobs[:min(waiting, len(s.jobs))] {
		blocked := j.version.ID == s.extending || slices.ContainsFunc(playing, j.heldBy)
		if !blocked && (hostless(j.version) || s.gate.ready(j.host)) {
			s.jobs = slices.Delete(s.jobs, i, i+1)
			s.working, s.generating = true, j.version.ID
			return j, true
		}
	}
	return job{}, false
}

// finished marks the version next took as done.
func (s *Service) finished() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.working, s.generating = false, accounts.ID{}
}

// busyHost reports whether a playback reads host: one of a version not
// known reads none.
func busyHost(p Playing, host string) bool {
	return p.URL != "" && hostOf(p.URL) == host
}

// busy reports whether a playback reads host.
func (s *Service) busy(host string) bool {
	return slices.ContainsFunc(s.playbacks(), func(p Playing) bool { return busyHost(p, host) })
}

// playsFile reports whether a playback reads the file at url.
func (s *Service) playsFile(url string) bool {
	return slices.ContainsFunc(s.playbacks(), func(p Playing) bool { return p.URL == url })
}

// wanted is what a version lacks of what the settings turn on. partial
// describes the thumbnails a play made of the set to complete, at
// interval; none when its ThumbnailCount is 0.
type wanted struct {
	trickplay bool
	width     int
	interval  time.Duration
	partial   Info
	chapters  []media.Chapter
}

// generate makes what a version lacks of its images, and keeps the
// result. A local folder's file not analyzed yet is analyzed first. A
// torrent's file gets the thumbnails following those its plays made from
// what the source cache still holds, and nothing else.
func (s *Service) generate(ctx context.Context, j job) {
	version := j.version
	analysis, ok := s.analysis(ctx, j)
	if !ok {
		return
	}
	// What the source cache still holds of a play takes no request.
	if s.Cached != nil {
		added, err := s.extend(ctx, version, analysis, false)
		if version.Torrent != nil {
			s.extended(ctx, j, added, err)
			return
		}
		if err != nil && ctx.Err() == nil {
			s.Logger.Debug("The thumbnails of a version could not be made from what its play read", "title", j.title, "addon", version.Addon, "error", err)
		}
	}
	if version.Torrent != nil {
		return
	}
	want, err := s.wanted(ctx, version.ID, analysis)
	if err != nil {
		s.Logger.Warn("Reading the images of a version failed", "error", err)
		return
	}
	s.touch(ctx, version.ID)
	if !want.trickplay && len(want.chapters) == 0 {
		if _, video := videoStream(analysis); !video || analysis.Duration <= 0 {
			s.recordFailure(ctx, version, errNoVideo)
			return
		}
		s.recordMade(ctx, version)
		return
	}
	// The result is kept past the generation's time.
	generation, cancel := context.WithTimeout(ctx, generationTime)
	defer cancel()
	started := time.Now()
	read, err := s.make(generation, version, analysis, want)
	switch {
	case err == nil:
		s.Logger.Info("Made the images of a version", "title", j.title, "addon", version.Addon, "thumbnails", want.trickplay,
			"chapters", len(want.chapters), "keyframes", read.keyframes, "requests", read.requests,
			"duration", time.Since(started).Round(time.Second))
		s.recordMade(ctx, version)
		return
	case ctx.Err() != nil && errors.Is(err, context.Canceled):
		return
	case errors.Is(err, source.ErrSlowDown):
		s.Logger.Warn("A source asked to slow down: images of its host are paused", "title", j.title, "addon", version.Addon,
			"requests", read.requests, "pause", slowDownPause, "answer", source.Answer(err))
		s.gate.pause(j.host, slowDownPause)
		s.refused.Put(version.ID, err)
		s.dropHost(j.host)
	case lasting(err):
		s.Logger.Info("The images of a version cannot be made", append([]any{"title", j.title, "addon", version.Addon, "requests", read.requests}, failure(err)...)...)
		s.refused.Put(version.ID, err)
	default:
		s.Logger.Info("The images of a version could not be made", append([]any{"title", j.title, "addon", version.Addon, "requests", read.requests}, failure(err)...)...)
		s.failed.Put(version.ID, err)
	}
	s.recordFailure(ctx, version, err)
}

// analysis returns the analysis of a version, analyzing a local folder's
// file that was not; false for a version not analyzed, or one whose
// analysis failed, which is then kept as its result: a file ffprobe
// cannot read, or reads without a video track, cannot give images.
func (s *Service) analysis(ctx context.Context, j job) (media.Analysis, bool) {
	if analysis, ok := s.Analyzed(ctx, j.version.ID); ok {
		return analysis, true
	}
	if !local(j.version) || s.Analyze == nil {
		return media.Analysis{}, false
	}
	analysis, err := s.Analyze(ctx, j.version)
	if err == nil {
		return analysis, true
	}
	if ctx.Err() != nil {
		return media.Analysis{}, false
	}
	if state, _ := outcome(err); state == StateCannot {
		s.Logger.Info("The images of a version cannot be made", "title", j.title, "addon", j.version.Addon, "error", err)
		s.refused.Put(j.version.ID, err)
	} else {
		s.Logger.Info("The images of a version could not be made", "title", j.title, "addon", j.version.Addon, "error", err)
		s.failed.Put(j.version.ID, err)
	}
	s.recordFailure(ctx, j.version, err)
	return media.Analysis{}, false
}

// extended keeps the result of extending a torrent's thumbnails from what
// the source cache holds, after its play: made while it has thumbnails,
// and as generate keeps a failure.
func (s *Service) extended(ctx context.Context, j job, added int, err error) {
	version := j.version
	switch {
	case err == nil || errors.Is(err, source.ErrNotCached) || errors.Is(err, errConverting):
		if added > 0 {
			s.Logger.Info("Made thumbnails of a torrent's file from what its plays read", "title", j.title, "addon", version.Addon, "thumbnails", added)
		}
		settings := s.Settings()
		if !settings.Trickplay {
			return
		}
		if _, exists, err := s.trickplaySet(ctx, version.ID, settings.TrickplayWidth); err == nil && exists {
			s.recordMade(ctx, version)
		}
		return
	case ctx.Err() != nil || errors.Is(err, errGone):
		return
	case lasting(err):
		s.Logger.Info("The images of a version cannot be made", "title", j.title, "addon", version.Addon, "error", err)
		s.refused.Put(version.ID, err)
	default:
		s.Logger.Info("The images of a version could not be made", "title", j.title, "addon", version.Addon, "error", err)
		s.failed.Put(version.ID, err)
	}
	s.recordFailure(ctx, version, err)
}

// failure describes why making images failed, for the log: what the source
// answered when it answered oddly, since its errors may carry what its
// addon says, links included; the error otherwise.
func failure(err error) []any {
	if answer := source.Answer(err); answer != "" {
		return []any{"answer", answer}
	}
	return []any{"error", err}
}

// dropHost drops the waiting versions of a paused host: their next play
// queues them again.
func (s *Service) dropHost(host string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = slices.DeleteFunc(s.jobs, func(j job) bool {
		if hostless(j.version) || j.host != host {
			return false
		}
		j.finish()
		return true
	})
}

// lasting reports whether making images failed for a reason trying again
// would meet again.
func lasting(err error) bool {
	return errors.Is(err, container.ErrNoIndex) || errors.Is(err, container.ErrUnreadable) ||
		errors.Is(err, errBudget) || errors.Is(err, errNoVideo)
}

// errNoVideo reports a version without a video track, or without a
// duration to spread thumbnails over.
var errNoVideo = errors.New("no video to make images of")

// wanted reads what a version lacks of the images the settings turn on:
// the thumbnails it has not, or not all of those a play made.
func (s *Service) wanted(ctx context.Context, version accounts.ID, analysis media.Analysis) (wanted, error) {
	settings := s.Settings()
	var want wanted
	if settings.Trickplay && analysis.Duration > 0 {
		info, exists, err := s.trickplaySet(ctx, version, settings.TrickplayWidth)
		if err != nil {
			return wanted{}, err
		}
		want.width = settings.TrickplayWidth
		want.interval = time.Duration(settings.TrickplayInterval) * time.Second
		want.trickplay = !exists || !complete(info, analysis.Duration)
		if want.trickplay && exists && info.interval() == want.interval {
			want.partial = info
		}
	}
	if settings.ChapterImages && len(analysis.Chapters) > 0 {
		var exists bool
		if err := s.DB.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM chapter_images WHERE version_id = $1)", version).Scan(&exists); err != nil {
			return wanted{}, err
		}
		if !exists {
			want.chapters = analysis.Chapters
		}
	}
	return want, nil
}

// reads is what making a version's images read: the requests made of its
// source, and the keyframes decoded.
type reads struct {
	requests, keyframes int
}

// make reads keyframes within the version's budget, spread over its
// runtime, decodes them and keeps the images, each thumbnail and chapter
// showing the keyframe read nearest its time. Nothing is kept when a
// request fails. A local folder's file is read without budget nor pace,
// one keyframe for each thumbnail at the interval asked, waiting while
// that file plays.
func (s *Service) make(ctx context.Context, version library.Version, analysis media.Analysis, want wanted) (reads, error) {
	stream, ok := videoStream(analysis)
	if !ok {
		return reads{}, errNoVideo
	}
	src := s.Open(version)
	defer src.Release()
	p := &paced{src: src, host: hostOf(version.URL), gate: s.gate, busy: s.busy, left: s.budget}
	if local(version) {
		p.unpaced, p.left = true, math.MaxInt
		p.busy = func(string) bool { return s.playsFile(version.URL) }
	}
	size := analysis.Size
	if size <= 0 {
		// The first request tells the size.
		if _, err := p.Fetch(ctx, 0, 1); err != nil {
			return reads{requests: p.requests}, err
		}
		var known bool
		if size, known = src.KnownSize(); !known {
			return reads{requests: p.requests}, fmt.Errorf("%w: the source's size is unknown", container.ErrNoIndex)
		}
	}
	video, err := container.OpenVideo(ctx, p, size)
	if err != nil {
		return reads{requests: p.requests}, err
	}
	// Every request left could read a keyframe: one for each thumbnail,
	// their interval stretched so that each has its own, and one at each
	// chapter's start while requests are left for them; otherwise chapter
	// images show the thumbnails' keyframes. The thumbnails a play made
	// are kept, and those after them read; when the requests left stretch
	// the interval, the kept ones nearest each thumbnail's time show.
	var chapters []time.Duration
	for _, chapter := range want.chapters {
		chapters = append(chapters, chapter.Start)
	}
	targets := chapters
	interval := want.interval
	kept := time.Duration(want.partial.ThumbnailCount) * want.interval
	if want.trickplay {
		reads := p.left
		if kept > 0 && len(chapters) > 0 {
			// The chapters within the thumbnails kept need reads of their
			// own.
			reads = max(p.left/2, p.left-len(chapters))
		}
		interval = stepFor(analysis.Duration-kept, want.interval, reads)
		all := thumbnailTimes(analysis.Duration, interval)
		targets = all[sort.Search(len(all), func(i int) bool { return all[i] >= kept }):]
		if len(targets)+len(chapters) <= p.left {
			targets = append(targets, chapters...)
		}
	}
	order := chooseKeyframes(video.Keyframes, targets)
	hw, hdr := s.decoder(stream)
	var filters []string
	if want.trickplay {
		filters = append(filters, thumbnailFilter(want.width, hdr))
	}
	if len(want.chapters) > 0 {
		filters = append(filters, chapterFilter(hdr))
	}
	// Each keyframe is decoded as soon as it is read, and its images kept
	// with its time: the images shown are chosen by time, whatever the
	// order the keyframes were read in.
	var frames []decodedFrame
	err = video.ReadFrames(ctx, p, order, func(index int, frame []byte) error {
		images, err := s.decode(ctx, hw, video, frame, filters, false)
		switch {
		case err == nil:
			frames = append(frames, decodedFrame{at: video.Keyframes[index], images: images})
			return nil
		case ctx.Err() != nil:
			return ctx.Err()
		case !errors.Is(err, errNoImage) || len(frames) == 0:
			// FFmpeg cannot run, or the first keyframe does not decode: the
			// others are not read for nothing.
			return fmt.Errorf("%w: %w", container.ErrUnreadable, err)
		}
		s.Logger.Debug("A keyframe could not be decoded", "addon", version.Addon, "at", video.Keyframes[index], "error", err)
		return nil
	})
	read := reads{requests: p.requests, keyframes: len(frames)}
	if err != nil && (!errors.Is(err, errBudget) || len(frames) == 0) {
		return read, err
	}
	times := make([]time.Duration, len(frames))
	for i, f := range frames {
		times[i] = f.at
	}
	if want.trickplay {
		asked := thumbnailTimes(analysis.Duration, interval)
		count := len(asked)
		tiles := newTiler(count, interval)
		tiles.info.Width = want.width
		made := &madeThumbnails{s: s, version: version.ID, info: want.partial}
		for i, k := range shown(times, asked) {
			img := frames[k].images[0]
			if asked[i] < kept {
				nearest := min(want.partial.ThumbnailCount-1, int((asked[i]+want.interval/2)/want.interval))
				if img, err = made.at(ctx, nearest); err != nil {
					return read, err
				}
			}
			if err := tiles.add(img); err != nil {
				return read, err
			}
		}
		info, tileData, err := tiles.finish()
		if err != nil {
			return read, err
		}
		if err := s.SaveTrickplay(ctx, version.ID, version.Item, info, tileData); err != nil {
			return read, err
		}
	}
	if len(chapters) > 0 {
		encoded := map[int][]byte{}
		data := make([][]byte, len(chapters))
		for c, k := range shown(times, chapters) {
			if encoded[k] == nil {
				if encoded[k], err = encodeJPEG(frames[k].images[len(filters)-1]); err != nil {
					return read, err
				}
			}
			data[c] = encoded[k]
		}
		if err := s.SaveChapterImages(ctx, version.ID, version.Item, data); err != nil {
			return read, err
		}
	}
	return read, nil
}

// decodedFrame is a keyframe decoded: its time, and its image through each
// filter.
type decodedFrame struct {
	at     time.Duration
	images []*image.YCbCr
}

// thumbnailFilter scales keyframes to thumbnails width pixels wide,
// keeping the shape they are shown in, in full-range 4:2:0 as JPEG takes
// it; HDR is first tone mapped to SDR.
func thumbnailFilter(width int, hdr bool) string {
	return scaleFilter("w="+fmt.Sprint(width)+":h=trunc(ow/dar/2)*2", hdr)
}

// chapterFilter scales keyframes to chapter images at most chapterWidth
// pixels wide.
func chapterFilter(hdr bool) string {
	return scaleFilter(fmt.Sprintf(`w=trunc(min(%d\,iw)/2)*2:h=trunc(ow/dar/2)*2`, chapterWidth), hdr)
}

func scaleFilter(size string, hdr bool) string {
	if hdr {
		// As conversions tone map without a GPU (see hls), to full range.
		return strings.Join([]string{"scale=" + size, "zscale=t=linear:npl=100", "format=gbrpf32le", "zscale=p=bt709",
			"tonemap=tonemap=hable:desat=0", "zscale=t=bt709:m=bt709:r=full", "format=yuv420p", "setsar=1"}, ",")
	}
	return "scale=" + size + ":out_range=full,format=yuv420p,setsar=1"
}

func videoStream(analysis media.Analysis) (media.Stream, bool) {
	for _, stream := range analysis.Streams {
		if stream.Type == "video" && !stream.AttachedPicture {
			return stream, true
		}
	}
	return media.Stream{}, false
}
