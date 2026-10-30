package thumbnails

import (
	"context"
	"errors"
	"fmt"
	"image"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/source"
)

// Thumbnails are made during a play from what the playback reads: the
// keyframes whose bytes the source cache holds, read with no request. They
// go on from the start of the version, as far as those keyframes follow one
// another, so that the set kept covers its first minutes; the play's next
// opening shows them. What is left is read after the play (see make).

// Cached is what the source cache holds of a version's file, read with no
// request, failing with source.ErrNotCached past it: source.Cached is one.
type Cached interface {
	container.Reader
	container.Fetcher
	KnownSize() (int64, bool)
}

// liveInterval is how often the thumbnails of the versions playing are
// extended.
const liveInterval = 30 * time.Second

// errConverting stops the thumbnails of a play while a conversion runs on
// the processor.
var errConverting = errors.New("a conversion runs on the processor")

// live extends, every LiveEvery, the thumbnails of the versions played
// now, one at a time.
func (s *Service) live(ctx context.Context) {
	ticker := time.NewTicker(s.LiveEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		for _, version := range s.playingVersions() {
			if ctx.Err() != nil {
				return
			}
			s.extendPlaying(ctx, version)
		}
	}
}

// playingVersions are the versions waiting for their images whose
// playback goes on.
func (s *Service) playingVersions() []library.Version {
	playing := s.playbacks()
	s.mu.Lock()
	defer s.mu.Unlock()
	var versions []library.Version
	for _, j := range s.jobs {
		for _, p := range playing {
			if p.Device == j.after {
				versions = append(versions, j.version)
				break
			}
		}
	}
	return versions
}

// extendPlaying extends the thumbnails of a version playing, unless the
// settings turn them off, a conversion runs on the processor, its images
// are made after a play now, or it recently could not be extended.
func (s *Service) extendPlaying(ctx context.Context, version library.Version) {
	if !s.Settings().Trickplay || s.converting() {
		return
	}
	if _, failed := s.unextended.Get(version.ID); failed {
		return
	}
	s.mu.Lock()
	if s.generating == version.ID {
		s.mu.Unlock()
		return
	}
	s.extending = version.ID
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.extending = accounts.ID{}
		s.mu.Unlock()
	}()
	analysis, ok := s.Analyzed(ctx, version.ID)
	if !ok {
		return
	}
	added, err := s.extend(ctx, version, analysis, true)
	switch {
	case err == nil || errors.Is(err, source.ErrNotCached) || ctx.Err() != nil:
		if added > 0 {
			s.Logger.Debug("Made thumbnails of a version from what its play read", "addon", version.Addon, "thumbnails", added)
		}
	case errors.Is(err, errGone):
	default:
		s.Logger.Info("The thumbnails of a version cannot be made during its play", "addon", version.Addon, "error", err)
		s.unextended.Put(version.ID, err)
	}
}

// converting reports whether a conversion runs on the processor.
func (s *Service) converting() bool {
	return s.Converting != nil && s.Converting()
}

// extend makes, from what the source cache holds of a version's file, the
// thumbnails following those it has at the settings' width and interval,
// as far as the keyframes they show are held one after another, and keeps
// them. A set at another interval is made again from the start; a set
// complete is left as it is. It returns how many thumbnails it added.
// guarded stops it while a conversion runs on the processor; FFmpeg runs
// at the lowest priority.
func (s *Service) extend(ctx context.Context, version library.Version, analysis media.Analysis, guarded bool) (int, error) {
	settings := s.Settings()
	stream, ok := videoStream(analysis)
	if !settings.Trickplay || !ok || analysis.Duration <= 0 {
		return 0, nil
	}
	interval := time.Duration(settings.TrickplayInterval) * time.Second
	info, exists, err := s.trickplaySet(ctx, version.ID, settings.TrickplayWidth)
	switch {
	case err != nil:
		return 0, err
	case exists && complete(info, analysis.Duration):
		return 0, nil
	case !exists || info.interval() != interval:
		info = Info{Width: settings.TrickplayWidth, TileWidth: tileColumns, TileHeight: tileRows, Interval: int(interval / time.Millisecond)}
	}
	from := info.ThumbnailCount
	src, ok := s.Cached(version)
	if !ok {
		return 0, nil
	}
	size := analysis.Size
	if size <= 0 {
		if size, ok = src.KnownSize(); !ok {
			return 0, nil
		}
	}
	video, err := container.OpenVideo(ctx, src, size)
	if err != nil {
		return 0, err
	}
	// The keyframe each following thumbnail shows, the nearest its time,
	// read once each, in order.
	times := thumbnailTimes(analysis.Duration, interval)[from:]
	shows := make([]int, len(times))
	var order []int
	for i, at := range times {
		shows[i] = nearest(video.Keyframes, at)
		if len(order) == 0 || order[len(order)-1] != shows[i] {
			order = append(order, shows[i])
		}
	}
	hw, hdr := s.decoder(stream)
	filters := []string{thumbnailFilter(info.Width, hdr)}
	images := map[int]*image.YCbCr{}
	var last *image.YCbCr
	err = video.ReadFrames(ctx, src, order, func(index int, frame []byte) error {
		if guarded && s.converting() {
			return errConverting
		}
		decoded, err := s.decode(ctx, hw, video, frame, filters, true)
		switch {
		case err == nil:
			last = decoded[0]
		case ctx.Err() != nil:
			return ctx.Err()
		case !errors.Is(err, errNoImage) || last == nil:
			return fmt.Errorf("%w: %w", container.ErrUnreadable, err)
		}
		// A keyframe that does not decode shows the one before.
		images[index] = last
		return nil
	})
	added := 0
	for added < len(shows) && images[shows[added]] != nil {
		added++
	}
	if added == 0 {
		return 0, err
	}
	if err != nil && !errors.Is(err, source.ErrNotCached) && !errors.Is(err, errConverting) {
		return 0, err
	}
	perTile := info.TileWidth * info.TileHeight
	first := from / perTile
	var tile []byte
	if from%perTile > 0 {
		if tile, err = s.keptTile(ctx, version.ID, info.Width, first); err != nil {
			return 0, err
		}
	}
	tiles, err := resumeTiler(info, tile, from%perTile)
	if err != nil {
		return 0, err
	}
	tiles.info.ThumbnailCount = from + added
	for _, k := range shows[:added] {
		if err := tiles.add(images[k]); err != nil {
			return 0, err
		}
	}
	info, data, err := tiles.finish()
	if err != nil {
		return 0, err
	}
	if err := s.saveTiles(ctx, version.ID, version.Item, info, first, data); err != nil {
		return 0, err
	}
	return added, nil
}

// complete reports whether a set of thumbnails covers a version lasting
// duration.
func complete(info Info, duration time.Duration) bool {
	return info.interval() > 0 && info.ThumbnailCount >= thumbnailCount(duration, info.interval())
}

// interval is the time between two thumbnails.
func (i Info) interval() time.Duration {
	return time.Duration(i.Interval) * time.Millisecond
}

// decoder is the GPU decoding a video stream, nil for none, and whether
// its HDR is tone mapped.
func (s *Service) decoder(stream media.Stream) (*hls.Hardware, bool) {
	hdr := s.ToneMapping && (stream.ColorTransfer == "smpte2084" || stream.ColorTransfer == "arib-std-b67")
	if s.Hardware == nil {
		return nil, hdr
	}
	return s.Hardware(stream.Codec, stream.BitDepth), hdr
}

// decode decodes a keyframe (see decodeKeyframe), one at a time.
func (s *Service) decode(ctx context.Context, hw *hls.Hardware, video *container.Video, frame []byte, filters []string, idle bool) ([]*image.YCbCr, error) {
	s.decoding.Lock()
	defer s.decoding.Unlock()
	return decodeKeyframe(ctx, s.FFmpeg, hw, video, frame, filters, idle)
}

// madeThumbnails reads the thumbnails of a set a play made, decoding one
// tile at a time.
type madeThumbnails struct {
	s       *Service
	version accounts.ID
	info    Info
	tile    int
	canvas  *image.YCbCr
}

// at returns the thumbnail n.
func (m *madeThumbnails) at(ctx context.Context, n int) (*image.YCbCr, error) {
	perTile := m.info.TileWidth * m.info.TileHeight
	if m.canvas == nil || m.tile != n/perTile {
		data, err := m.s.keptTile(ctx, m.version, m.info.Width, n/perTile)
		if err != nil {
			return nil, err
		}
		if m.canvas, err = decodeTile(data, m.info); err != nil {
			return nil, err
		}
		m.tile = n / perTile
	}
	return cell(m.canvas, m.info, n%perTile), nil
}
