package playback

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/source"
)

// Reading ahead of a play what it reads first.
//
// The first segment of an HLS play is ready once FFmpeg has read its
// bytes, up to the next segment's keyframe: from a debrid host, seconds
// for 4K. Those bytes are read into the source cache as soon as the
// version's keyframe index is known, when a title's details open (with
// playback prepared ahead) or a PlaybackInfo plans the version, so that
// FFmpeg finds them there. A resume reads the file's head, which FFmpeg
// probes, and the segment it resumes at.

const (
	// warmBudget bounds the bytes a warm reads past the head, warmHead is
	// the head FFmpeg probes, and warmMargin what is read past the next
	// segment's keyframe, the start of its Cluster.
	warmBudget = 64 << 20
	warmHead   = 5 << 20
	warmMargin = 1 << 20
	// warmTime bounds a warm, maxWarms the warms under way at once: a
	// newer one stops the oldest.
	warmTime = 2 * time.Minute
	maxWarms = 2
	// warmedFor is how long a version warmed is not warmed again from
	// the same start, nor by Plan from another.
	warmedFor = 10 * time.Minute
	// indexAheadTime bounds the keyframe index read during an analysis.
	indexAheadTime = time.Minute
)

// warmRun is a warm under way.
type warmRun struct {
	start   time.Duration
	started time.Time
	cancel  context.CancelFunc
}

// Warm reads into the source cache, in the background, the bytes a play of
// version from start reads first: the head FFmpeg probes, then the
// segment playing at start up to the next one's keyframe, within a bound.
// A warm of the version from another start under way is replaced, one
// from the same start or done lately is not repeated, and at most a few
// run at once, the newest. It returns at once.
func (s *Service) Warm(version library.Version, start time.Duration) {
	s.warm(version, start, true)
}

// warm is Warm; a warm that does not replace one under way, nor repeats
// one done lately from any start, nor runs while the version is remuxed,
// is what Plan starts.
func (s *Service) warm(version library.Version, start time.Duration, replace bool) {
	if warmed, ok := s.warmed.Get(version.ID); ok && (warmed == start || !replace) {
		return
	}
	if !replace && s.sources.Streamed(version.ID) {
		return
	}
	s.warmMu.Lock()
	if run := s.warming[version.ID]; run != nil {
		if run.start == start || !replace {
			s.warmMu.Unlock()
			return
		}
		run.cancel()
		delete(s.warming, version.ID)
	}
	if len(s.warming) >= maxWarms {
		var oldest accounts.ID
		for id, run := range s.warming {
			if s.warming[oldest] == nil || run.started.Before(s.warming[oldest].started) {
				oldest = id
			}
		}
		s.warming[oldest].cancel()
		delete(s.warming, oldest)
	}
	ctx, cancel := context.WithTimeout(context.Background(), warmTime)
	run := &warmRun{start: start, started: time.Now(), cancel: cancel}
	s.warming[version.ID] = run
	s.warmMu.Unlock()
	go func() {
		defer cancel()
		started := time.Now()
		err := s.warmFrom(ctx, version, start)
		s.warmMu.Lock()
		if s.warming[version.ID] == run {
			delete(s.warming, version.ID)
		}
		s.warmMu.Unlock()
		if err != nil {
			s.logger.Debug("A version's first bytes could not be read ahead", "addon", version.Addon, "error", err)
			return
		}
		s.warmed.Put(version.ID, start)
		s.logger.Debug("Read a version's first bytes ahead", "addon", version.Addon, "from", start, "duration", time.Since(started))
	}()
}

// warmFrom reads what a play of version from start reads first.
func (s *Service) warmFrom(ctx context.Context, version library.Version, start time.Duration) error {
	plan, err := s.plan(ctx, version)
	if err != nil || plan.Grid() {
		return err
	}
	analysis, err := s.Analyze(ctx, version)
	if err != nil {
		return err
	}
	src := s.open(version)
	defer src.Release()
	var spans []source.Span
	err = s.readSized(ctx, version, analysis, src, func(size int64) error {
		offsets, err := s.keyframeOffsets(ctx, version, src, size)
		if err == nil {
			spans = warmSpans(plan, offsets, start, size)
		}
		return err
	})
	if err != nil {
		return err
	}
	return src.Warm(ctx, spans...)
}

// keyframeOffsets returns where a version's keyframes start: kept since its
// index was read, or read again from its head and index.
func (s *Service) keyframeOffsets(ctx context.Context, version library.Version, src *source.Source, size int64) ([]container.Keyframe, error) {
	if offsets, ok := s.offsets.Get(version.ID); ok {
		return offsets, nil
	}
	offsets, err := container.KeyframeOffsets(ctx, src, size)
	if err != nil {
		return nil, err
	}
	s.offsets.Put(version.ID, offsets)
	return offsets, nil
}

// warmSpans are the bytes of a file of size bytes that a play from start
// reads first: the segment playing at start, from its keyframe to past
// the next segment's, within warmBudget, and the head FFmpeg probes when
// that segment is not the first.
func warmSpans(plan hls.Plan, offsets []container.Keyframe, start time.Duration, size int64) []source.Span {
	n := plan.Segment(max(start, 0))
	from := offsetAt(offsets, plan.Start(n))
	to := size
	if n+1 < plan.Len() {
		to = offsetAt(offsets, plan.Start(n+1)) + warmMargin
	}
	if n == 0 {
		from = 0
	}
	to = min(to, from+warmBudget, size)
	spans := []source.Span{{Off: from, End: to}}
	if from > warmHead {
		spans = append([]source.Span{{Off: 0, End: min(warmHead, size)}}, spans...)
	}
	return spans
}

// offsetAt is where the last keyframe at or before t starts, 0 when none.
func offsetAt(offsets []container.Keyframe, t time.Duration) int64 {
	i, found := slices.BinarySearchFunc(offsets, t, func(k container.Keyframe, t time.Duration) int { return cmp.Compare(k.Time, t) })
	if !found {
		i--
	}
	if i < 0 {
		return 0
	}
	return offsets[i].Offset
}

// ebmlMagic starts every Matroska and WebM file.
var ebmlMagic = []byte{0x1A, 0x45, 0xDF, 0xA3}

// indexAhead reads a version's keyframe index while it is analyzed, when it
// is a Matroska file, by its name or its first bytes: its index needs only
// its size, and an HLS play reads it next. The read is shared with Plan's,
// and kept the same way.
func (s *Service) indexAhead(version library.Version) {
	if _, ok := s.indexes.Get(version.ID); ok {
		return
	}
	if _, failed := s.unindexed.Get(version.ID); failed {
		return
	}
	s.flight.DoChan("index ahead "+version.ID.String(), func() (any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), indexAheadTime)
		defer cancel()
		src := s.open(version)
		defer src.Release()
		size, err := src.Size(ctx)
		if err != nil {
			return nil, err
		}
		if !matroskaName(version.Filename) {
			head := make([]byte, len(ebmlMagic))
			if _, err := src.ReadAt(ctx, head, 0); err != nil || !bytes.Equal(head, ebmlMagic) {
				return nil, err
			}
		}
		_, err = s.keyframes(ctx, version, media.Analysis{Size: size})
		return nil, err
	})
}

// matroskaName reports whether a file name is a Matroska or WebM file's.
func matroskaName(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".mkv", ".mk3d", ".webm":
		return true
	}
	return false
}

// watchSource keeps a version as failed once its source fails, until done
// is closed: the next PlaybackInfo plays another version.
func (s *Service) watchSource(version library.Version, src *source.Source, done <-chan struct{}) {
	failed := src.Failed()
	select {
	case <-failed:
		err := failureOf(src, failed)
		if errors.Is(err, source.ErrUnavailable) {
			s.failures.Put(version.ID, err)
		}
		s.logger.Info("A version's source failed while it played", "addon", version.Addon, "answer", source.Answer(err))
	case <-done:
	}
}
