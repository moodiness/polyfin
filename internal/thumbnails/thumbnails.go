// Package thumbnails makes the scrubbing thumbnails (Jellyfin's trickplay)
// and chapter images of the versions played, from their keyframes alone:
// read through the container's index, a few hundred spans of a file rather
// than the whole of it, paced, decoded by FFmpeg and kept in the database.
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
)

const (
	// queueLength bounds the versions waiting for their images: one is
	// made at a time, and more would wait for long; a version dropped is
	// queued again by its next play.
	queueLength = 8
	// requestInterval spaces the requests to a host: at most two a second.
	requestInterval = 500 * time.Millisecond
	// maxRequests bounds the requests the images of a version take, about
	// twenty minutes of requests at the pace above, and maxFrames the
	// keyframes read for its thumbnails: a long version shows the same
	// keyframe on a few thumbnails in a row rather than take more.
	maxRequests = 2500
	maxFrames   = 1000
	// generationTime bounds the making of a version's images.
	generationTime = time.Hour
	// failureTTL is how long a version whose images could not be made is
	// left alone, and refusalTTL one whose file cannot give them, for its
	// index, its codec or the requests it would take.
	failureTTL = 30 * time.Minute
	refusalTTL = 24 * time.Hour
	// chapterWidth is the width of chapter images, at most: apps show them
	// as cards of a few hundred pixels.
	chapterWidth = 640
	// touchEvery is how often the last use of a version's images is saved.
	touchEvery = time.Hour
)

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
	// Open opens a version's source; the caller releases it.
	Open func(library.Version) Source
	// Analyzed returns a version's analysis, if it was analyzed.
	Analyzed func(context.Context, accounts.ID) (media.Analysis, bool)
	Logger   *slog.Logger
}

// Service makes and keeps the images of versions.
type Service struct {
	Options
	queue  chan library.Version
	cancel context.CancelFunc
	done   chan struct{}
	pacer  *pacer

	mu     sync.Mutex
	queued map[accounts.ID]bool

	failed  *cache.Cache[accounts.ID, error]
	refused *cache.Cache[accounts.ID, error]
	touched *cache.Cache[accounts.ID, struct{}]

	// maxRequests and maxFrames are those above, which tests lower.
	maxRequests, maxFrames int
}

// New starts the service, which Close stops.
func New(options Options) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{
		Options:     options,
		queue:       make(chan library.Version, queueLength),
		cancel:      cancel,
		done:        make(chan struct{}),
		pacer:       newPacer(requestInterval),
		queued:      map[accounts.ID]bool{},
		failed:      cache.New[accounts.ID, error](2000, failureTTL),
		refused:     cache.New[accounts.ID, error](2000, refusalTTL),
		touched:     cache.New[accounts.ID, struct{}](5000, touchEvery),
		maxRequests: maxRequests,
		maxFrames:   maxFrames,
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
// thumbnails and chapter images the settings turn on and it lacks. A
// version recently failed, or already waiting, is not queued; neither is
// one when the queue is full.
func (s *Service) Queue(version library.Version) {
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.queued[version.ID] {
		return
	}
	select {
	case s.queue <- version:
		s.queued[version.ID] = true
	default:
		s.Logger.Debug("The images of a version were not queued: too many are waiting", "addon", version.Addon)
	}
}

func (s *Service) run(ctx context.Context) {
	defer close(s.done)
	for {
		select {
		case <-ctx.Done():
			return
		case version := <-s.queue:
			s.mu.Lock()
			delete(s.queued, version.ID)
			s.mu.Unlock()
			s.generate(ctx, version)
		}
	}
}

// wanted is what a version lacks of what the settings turn on.
type wanted struct {
	trickplay bool
	width     int
	interval  time.Duration
	chapters  []media.Chapter
}

// generate makes what a version lacks of its images.
func (s *Service) generate(ctx context.Context, version library.Version) {
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
	requests, err := s.make(ctx, version, analysis, want)
	switch {
	case err == nil:
		s.Logger.Info("Made the images of a version", "addon", version.Addon, "thumbnails", want.trickplay,
			"chapters", len(want.chapters), "requests", requests, "duration", time.Since(started).Round(time.Second))
	case ctx.Err() != nil && errors.Is(err, context.Canceled):
	case lasting(err):
		s.Logger.Info("The images of a version cannot be made", "addon", version.Addon, "requests", requests, "error", err)
		s.refused.Put(version.ID, err)
	default:
		s.Logger.Info("The images of a version could not be made", "addon", version.Addon, "requests", requests, "error", err)
		s.failed.Put(version.ID, err)
	}
}

// lasting reports whether making images failed for a reason trying again
// would meet again.
func lasting(err error) bool {
	return errors.Is(err, container.ErrNoIndex) || errors.Is(err, container.ErrUnreadable) ||
		errors.Is(err, errTooManyRequests) || errors.Is(err, errNoVideo)
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

// make reads the keyframes the images show, decodes them and keeps the
// images, returning the requests made of the source.
func (s *Service) make(ctx context.Context, version library.Version, analysis media.Analysis, want wanted) (int, error) {
	stream, ok := videoStream(analysis)
	if !ok {
		return 0, errNoVideo
	}
	src := s.Open(version)
	defer src.Release()
	p := &paced{src: src, host: hostOf(version.URL), pacer: s.pacer, left: s.maxRequests}
	size := analysis.Size
	if size <= 0 {
		var err error
		if size, err = p.Size(ctx); err != nil {
			return p.requests, err
		}
	}
	video, err := container.OpenVideo(ctx, p, size)
	if err != nil {
		return p.requests, err
	}
	var thumbs, chapters shots
	if want.trickplay {
		thumbs = planThumbnails(video.Keyframes, analysis.Duration, want.interval, s.maxFrames)
	}
	if len(want.chapters) > 0 {
		chapters = planChapters(video.Keyframes, want.chapters)
	}
	hdr := s.ToneMapping && (stream.ColorTransfer == "smpte2084" || stream.ColorTransfer == "arib-std-b67")

	var tiles *tiler
	var thumbnails, chapterImages *imageDecoder
	if want.trickplay {
		tiles = newTiler(len(thumbs.images), want.interval)
		tiles.info.Width = want.width
		thumbnails = &imageDecoder{shots: thumbs, place: tiles.add}
		if err := s.startImages(ctx, video, thumbnailFilter(want.width, hdr), thumbnails); err != nil {
			return p.requests, err
		}
		defer thumbnails.kill()
	}
	images := make([][]byte, len(want.chapters))
	if len(want.chapters) > 0 {
		chapterImages = &imageDecoder{shots: chapters, encoded: images}
		if err := s.startImages(ctx, video, chapterFilter(hdr), chapterImages); err != nil {
			return p.requests, err
		}
		defer chapterImages.kill()
	}

	err = video.ReadFrames(ctx, p, union(thumbs.keyframes, chapters.keyframes), func(index int, frame []byte) error {
		for _, d := range []*imageDecoder{thumbnails, chapterImages} {
			if d != nil && d.wants(index) {
				if err := d.write(frame); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return p.requests, err
	}
	if thumbnails != nil {
		if err := thumbnails.finish(); err != nil {
			return p.requests, err
		}
		info, tileData, err := tiles.finish()
		if err != nil {
			return p.requests, err
		}
		if err := s.SaveTrickplay(ctx, version.ID, version.Item, info, tileData); err != nil {
			return p.requests, err
		}
	}
	if chapterImages != nil {
		if err := chapterImages.finish(); err != nil {
			return p.requests, err
		}
		if err := s.SaveChapterImages(ctx, version.ID, version.Item, images); err != nil {
			return p.requests, err
		}
	}
	return p.requests, nil
}

// imageDecoder decodes the keyframes of shots, in order, and hands each
// image shown to place, as many times as images show it; or, for chapter
// images, encodes each into encoded.
type imageDecoder struct {
	*decoder
	shots   shots
	place   func(*image.YCbCr) error
	encoded [][]byte
	// next is the next image handed out; last, the last keyframe decoded.
	next int
	last *image.YCbCr
}

// startImages starts FFmpeg decoding the keyframes of d's shots.
func (s *Service) startImages(ctx context.Context, video *container.Video, filter string, d *imageDecoder) error {
	var err error
	d.decoder, err = startDecoder(ctx, s.FFmpeg, s.Hardware, video, filter, d.decoded)
	return err
}

// wants reports whether keyframe index is one the images show.
func (d *imageDecoder) wants(index int) bool {
	_, found := slices.BinarySearch(d.shots.keyframes, index)
	return found
}

// decoded hands out the images showing keyframe k of the shots.
func (d *imageDecoder) decoded(k int, img *image.YCbCr) error {
	if k >= len(d.shots.keyframes) {
		return nil
	}
	if d.encoded != nil {
		var data []byte
		for c, shown := range d.shots.images {
			if shown != k {
				continue
			}
			if data == nil {
				var err error
				if data, err = encodeJPEG(img); err != nil {
					return err
				}
			}
			d.encoded[c] = data
		}
		return nil
	}
	for d.next < len(d.shots.images) && d.shots.images[d.next] == k {
		if err := d.place(img); err != nil {
			return err
		}
		d.next++
	}
	d.last = img
	return nil
}

// finish waits for FFmpeg. The images of keyframes it did not decode at
// the end repeat the last it decoded; none decoded fails.
func (d *imageDecoder) finish() error {
	images, err := d.decoder.finish()
	if err != nil {
		return err
	}
	if images == 0 {
		return fmt.Errorf("%w: FFmpeg decoded no keyframe", container.ErrUnreadable)
	}
	if d.encoded != nil {
		var last []byte
		for c := range d.encoded {
			if d.encoded[c] == nil {
				d.encoded[c] = last
			}
			last = d.encoded[c]
		}
		for c := len(d.encoded) - 1; c >= 0; c-- {
			if d.encoded[c] == nil {
				d.encoded[c] = last
			}
			last = d.encoded[c]
		}
		return nil
	}
	for d.next < len(d.shots.images) {
		if err := d.place(d.last); err != nil {
			return err
		}
		d.next++
	}
	return nil
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
