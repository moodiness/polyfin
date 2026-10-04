// Package thumbnails makes the scrubbing thumbnails (Jellyfin's trickplay)
// and chapter images of the versions played, from their keyframes alone:
// read through the container's index, a few dozen spans of a file rather
// than the whole of it, gently, once nobody plays from its host, decoded
// by FFmpeg and kept in the database.
package thumbnails

import (
	"context"
	"errors"
	"fmt"
	"image"
	"log/slog"
	"slices"
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
// version it plays, empty when it is not known.
type Playing struct {
	Device accounts.ID
	URL    string
}

// Options are what the service needs.
type Options struct {
	DB *pgxpool.Pool
	// FFmpeg is the FFmpeg executable; Hardware the GPU it decodes on, nil
	// for none; ToneMapping tells whether it has the filters bringing HDR
	// to SDR.
	FFmpeg      string
	Hardware    *hls.Hardware
	ToneMapping bool
	// Settings returns the server settings, read as they apply.
	Settings func() accounts.Settings
	// Open opens a version's source, to be read with one attempt per
	// request; the caller releases it.
	Open func(library.Version) Source
	// Analyzed returns a version's analysis, if it was analyzed.
	Analyzed func(context.Context, accounts.ID) (media.Analysis, bool)
	// Pace spaces the requests to a host and how often waits are checked
	// again; zero for requestInterval and pollInterval. Tests shorten it.
	Pace, Poll time.Duration
	Logger     *slog.Logger
}

// job is a version waiting for its images, after the playback of device
// stopped.
type job struct {
	version library.Version
	host    string
	after   accounts.ID
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
	// playing lists the playbacks under way: images wait for them.
	playing func() []Playing

	failed  *cache.Cache[accounts.ID, error]
	refused *cache.Cache[accounts.ID, error]
	touched *cache.Cache[accounts.ID, struct{}]

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
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{
		Options: options,
		cancel:  cancel,
		done:    make(chan struct{}),
		wake:    make(chan struct{}, 1),
		gate:    newGate(options.Pace, perHour, time.Hour, options.Poll),
		failed:  cache.New[accounts.ID, error](2000, failureTTL),
		refused: cache.New[accounts.ID, error](2000, refusalTTL),
		touched: cache.New[accounts.ID, struct{}](5000, touchEvery),
		budget:  budget,
	}
	go s.run(ctx)
	return s
}

// Close stops making images, and waits for the version under way.
func (s *Service) Close() {
	s.cancel()
	<-s.done
}

// Queue asks for the images of a version, in the background: the
// thumbnails and chapter images the settings turn on and it lacks. They
// are made once the playback of device has stopped, and while nobody
// plays from the version's host. A version recently failed, already
// waiting, or of a paused host, is not queued; neither is one when the
// queue is full.
func (s *Service) Queue(version library.Version, device accounts.ID) {
	settings := s.Settings()
	if !settings.Trickplay && !settings.ChapterImages {
		return
	}
	if _, failed := s.failed.Get(version.ID); failed {
		return
	}
	if _, refused := s.refused.Get(version.ID); refused {
		return
	}
	host := hostOf(version.URL)
	if s.gate.paused(host) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if slices.ContainsFunc(s.jobs, func(j job) bool { return j.version.ID == version.ID }) {
		return
	}
	if len(s.jobs) == queueLength {
		s.Logger.Debug("The images of a version were not queued: too many are waiting", "addon", version.Addon)
		return
	}
	s.jobs = append(s.jobs, job{version: version, host: host, after: device})
	s.Wake()
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
	defer close(s.done)
	timer := time.NewTimer(s.Poll)
	defer timer.Stop()
	for {
		if j, ok := s.next(); ok {
			s.generate(ctx, j)
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

// next takes the first waiting version that may start: the playback that
// asked for it stopped, nobody plays from its host, and its host is
// neither paused nor out of requests for the hour.
func (s *Service) next() (job, bool) {
	playing := s.playbacks()
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, j := range s.jobs {
		blocked := slices.ContainsFunc(playing, func(p Playing) bool {
			return p.Device == j.after || busyHost(p, j.host)
		})
		if !blocked && s.gate.ready(j.host) {
			s.jobs = slices.Delete(s.jobs, i, i+1)
			return j, true
		}
	}
	return job{}, false
}

// busyHost reports whether a playback reads host: one of an unknown
// version may.
func busyHost(p Playing, host string) bool {
	return p.URL == "" || hostOf(p.URL) == host
}

// busy reports whether a playback reads host.
func (s *Service) busy(host string) bool {
	return slices.ContainsFunc(s.playbacks(), func(p Playing) bool { return busyHost(p, host) })
}

// wanted is what a version lacks of what the settings turn on.
type wanted struct {
	trickplay bool
	width     int
	interval  time.Duration
	chapters  []media.Chapter
}

// generate makes what a version lacks of its images.
func (s *Service) generate(ctx context.Context, j job) {
	version := j.version
	analysis, ok := s.Analyzed(ctx, version.ID)
	if !ok {
		return
	}
	want, err := s.wanted(ctx, version.ID, analysis)
	if err != nil {
		s.Logger.Warn("Reading the images of a version failed", "error", err)
		return
	}
	s.touch(ctx, version.ID)
	if !want.trickplay && len(want.chapters) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, generationTime)
	defer cancel()
	started := time.Now()
	read, err := s.make(ctx, version, analysis, want)
	switch {
	case err == nil:
		s.Logger.Info("Made the images of a version", "addon", version.Addon, "thumbnails", want.trickplay,
			"chapters", len(want.chapters), "keyframes", read.keyframes, "requests", read.requests,
			"duration", time.Since(started).Round(time.Second))
	case ctx.Err() != nil && errors.Is(err, context.Canceled):
	case errors.Is(err, source.ErrSlowDown):
		s.Logger.Warn("A source asked to slow down: images of its host are paused", "addon", version.Addon,
			"requests", read.requests, "pause", slowDownPause, "error", err)
		s.gate.pause(j.host, slowDownPause)
		s.refused.Put(version.ID, err)
		s.dropHost(j.host)
	case lasting(err):
		s.Logger.Info("The images of a version cannot be made", "addon", version.Addon, "requests", read.requests, "error", err)
		s.refused.Put(version.ID, err)
	default:
		s.Logger.Info("The images of a version could not be made", "addon", version.Addon, "requests", read.requests, "error", err)
		s.failed.Put(version.ID, err)
	}
}

// dropHost drops the waiting versions of a paused host: their next play
// queues them again.
func (s *Service) dropHost(host string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = slices.DeleteFunc(s.jobs, func(j job) bool { return j.host == host })
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

// wanted reads what a version lacks of the images the settings turn on.
func (s *Service) wanted(ctx context.Context, version accounts.ID, analysis media.Analysis) (wanted, error) {
	settings := s.Settings()
	var want wanted
	if settings.Trickplay && analysis.Duration > 0 {
		var exists bool
		if err := s.DB.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM trickplay_sets WHERE version_id = $1 AND width = $2)",
			version, settings.TrickplayWidth).Scan(&exists); err != nil {
			return wanted{}, err
		}
		want.trickplay, want.width = !exists, settings.TrickplayWidth
		want.interval = time.Duration(settings.TrickplayInterval) * time.Second
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
// request fails.
func (s *Service) make(ctx context.Context, version library.Version, analysis media.Analysis, want wanted) (reads, error) {
	stream, ok := videoStream(analysis)
	if !ok {
		return reads{}, errNoVideo
	}
	src := s.Open(version)
	defer src.Release()
	p := &paced{src: src, host: hostOf(version.URL), gate: s.gate, busy: s.busy, left: s.budget}
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
	// Every request left could read a keyframe: they are spread over the
	// runtime, or at the chapters' starts when only those are made.
	var targets []time.Duration
	if want.trickplay {
		targets = spread(analysis.Duration, p.left)
	} else {
		for _, chapter := range want.chapters {
			targets = append(targets, chapter.Start)
		}
	}
	order := chooseKeyframes(video.Keyframes, targets)
	hdr := s.ToneMapping && (stream.ColorTransfer == "smpte2084" || stream.ColorTransfer == "arib-std-b67")

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
		images, err := decodeKeyframe(ctx, s.FFmpeg, s.Hardware, video, frame, filters)
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
		count := thumbnailCount(analysis.Duration, want.interval)
		asked := make([]time.Duration, count)
		for i := range asked {
			asked[i] = time.Duration(i) * want.interval
		}
		tiles := newTiler(count, want.interval)
		tiles.info.Width = want.width
		for _, k := range shown(times, asked) {
			if err := tiles.add(frames[k].images[0]); err != nil {
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
	if len(want.chapters) > 0 {
		starts := make([]time.Duration, len(want.chapters))
		for i, chapter := range want.chapters {
			starts[i] = chapter.Start
		}
		encoded := map[int][]byte{}
		data := make([][]byte, len(starts))
		for c, k := range shown(times, starts) {
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
