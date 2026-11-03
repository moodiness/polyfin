package playback

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// numberTime bounds reading a Matroska file's head for its TrackNumbers:
// a few small reads.
const numberTime = 30 * time.Second

// ContainerTracks returns the container's own numbers of the tracks of an
// analyzed version, by FFmpeg index: an MP4 or QuickTime file's track IDs,
// which its analysis names, and a Matroska file's TrackNumbers, read from
// its head once and remembered. FFmpeg does not tell those. A track whose
// number is not known is left out; none when the version was not analyzed,
// or its head could not be read. The read goes on once ctx ends, for the
// next caller.
func (s *Service) ContainerTracks(ctx context.Context, version library.Version) map[int]uint64 {
	analysis, ok := s.Analyzed(ctx, version.ID)
	if !ok {
		return nil
	}
	if !matroska(analysis) {
		return mp4Tracks(analysis)
	}
	if numbers, ok := s.numbered.Get(version.ID); ok {
		return numbers
	}
	if _, failed := s.unnumbered.Get(version.ID); failed || s.idleTorrent(ctx, version) {
		return nil
	}
	result, err := s.shared(ctx, "numbered "+version.ID.String(), func() (any, error) {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), numberTime)
		defer cancel()
		src := s.open(version)
		defer src.Release()
		var m *container.Matroska
		err := s.readSized(ctx, version, analysis, src, func(size int64) error {
			var err error
			m, err = container.OpenMatroska(ctx, src, size)
			return err
		})
		if err != nil {
			// A file whose head does not read is not read again for a while;
			// nor is one whose host failed, unless a torrent's file was not
			// sent yet.
			if errors.Is(err, container.ErrNoIndex) || errors.Is(err, container.ErrUnreadable) || !passing(version, err) {
				s.unnumbered.Put(version.ID, err)
			}
			return nil, err
		}
		numbers := map[int]uint64{}
		for stream, track := range streamTracks(analysis, m.Tracks()) {
			numbers[stream] = track.Number
		}
		s.numbered.Put(version.ID, numbers)
		return numbers, nil
	})
	if err != nil {
		return nil
	}
	return result.(map[int]uint64)
}

// mp4Tracks are the track IDs of an MP4 or QuickTime file, as its analysis
// names them, by FFmpeg index.
func mp4Tracks(analysis media.Analysis) map[int]uint64 {
	if !slices.Contains(strings.Split(analysis.Format, ","), "mp4") {
		return nil
	}
	numbers := map[int]uint64{}
	for _, stream := range analysis.Streams {
		if id, err := strconv.ParseUint(stream.ID, 0, 64); err == nil && id > 0 {
			numbers[stream.Index] = id
		}
	}
	return numbers
}
