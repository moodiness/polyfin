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
	"github.com/moodiness/polyfin/internal/torrent"
)

// Reading ahead of a play what it reads first.
//
// The first segment of an HLS play is ready once FFmpeg has read its
// bytes, up to the next segment's keyframe: from a debrid host, seconds
// for 4K. Those bytes are read into the source cache for the version a
// play chose, as its PlaybackInfo answers, so that FFmpeg finds them
// there; and, within a budget, for the titles a user is likely to play
// next (see WarmAhead). A resume reads the file's head, which FFmpeg
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
	// the same start.
	warmedFor = 10 * time.Minute
	// indexAheadTime bounds the keyframe index read during an analysis.
	indexAheadTime = time.Minute
)

// ErrWarmDeclined reports a warm ahead whose bytes its budget did not
// allow (see WarmAhead).
var ErrWarmDeclined = errors.New("the budget declined the first bytes")

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
// run at once, the newest. It returns at once. The version is the one a
// play chose: its source is read as a playback's. A version read from its
// input, on the server's disk, is not warmed.
func (s *Service) Warm(version library.Version, start time.Duration) {
	if s.ReadsInput(version) {
		return
	}
	if warmed, ok := s.warmed.Get(version.ID); ok && warmed == start {
		return
	}
	s.warmMu.Lock()
	if run := s.warming[version.ID]; run != nil {
		if run.start == start {
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
	go s.runWarm(ctx, cancel, version, start, run)
}

// WarmAhead reads what Warm reads, ahead of a play that may come, once
// allow, told how many of those bytes the source cache lacks, allows
// them; else it reads nothing more and returns ErrWarmDeclined. It reads
// in the background, giving way to playbacks, and returns once done. A
// version warmed lately from start costs nothing.
func (s *Service) WarmAhead(ctx context.Context, version library.Version, start time.Duration, allow func(missing int64) bool) error {
	if s.ReadsInput(version) {
		return nil
	}
	if warmed, ok := s.warmed.Get(version.ID); ok && warmed == start {
		return nil
	}
	if err := s.warmFrom(Background(ctx), version, start, allow); err != nil {
		return err
	}
	s.warmed.Put(version.ID, start)
	return nil
}

// runWarm runs a warm of version from start, run, until ctx ends, then
// forgets it. A panic of it, of what reads the version's source, is
// logged with its stack and fails that warm only.
func (s *Service) runWarm(ctx context.Context, cancel func(), version library.Version, start time.Duration, run *warmRun) {
	defer cancel()
	started := time.Now()
	err := func() (err error) {
		defer s.panicked(&err, "Reading a version's first bytes ahead ended on a panic", "addon", version.AddonLog)
		return s.warmFrom(ctx, version, start, nil)
	}()
	s.warmMu.Lock()
	if s.warming[version.ID] == run {
		delete(s.warming, version.ID)
	}
	s.warmMu.Unlock()
	if err != nil {
		s.logger.Debug("A version's first bytes could not be read ahead", "addon", version.AddonLog, "error", err)
		return
	}
	s.warmed.Put(version.ID, start)
	s.logger.Debug("Read a version's first bytes ahead", "addon", version.AddonLog, "from", start, "duration", time.Since(started))
}

// warmFrom reads what a play of version from start reads first, as a
// playback's reads unless ctx is marked Background, once allow, unless
// nil, allows the bytes the cache lacks of it. A version cut on a grid,
// or one FFmpeg searches (see hls.Plan.Searched), has no index that tells
// where its keyframes are: it is not warmed.
func (s *Service) warmFrom(ctx context.Context, version library.Version, start time.Duration, allow func(missing int64) bool) error {
	plan, err := s.Plan(ctx, version)
	if err != nil || plan.Grid() || plan.IsSearched() {
		return err
	}
	analysis, err := s.Analyze(ctx, version)
	if err != nil {
		return err
	}
	src := s.open(version)
	defer src.Release()
	if !background(ctx) {
		defer src.Urge()()
	}
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
	if allow != nil && !allow(src.Missing(spans...)) {
		return ErrWarmDeclined
	}
	return src.Warm(ctx, spans...)
}

// keyframeOffsets returns where a version's keyframes start: kept since its
// index was read, or read again from its head and index, in a slot for
// index reads (see indexSlot).
func (s *Service) keyframeOffsets(ctx context.Context, version library.Version, src *source.Source, size int64) ([]container.Keyframe, error) {
	if offsets, ok := s.offsets.Get(version.ID); ok {
		return offsets, nil
	}
	release, err := s.indexSlot(ctx, version)
	if err != nil {
		return nil, err
	}
	defer release()
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
// and kept the same way. It is background work: a play reading the index
// meanwhile has it read as a playback, and a torrent's file is read only
// once a playback joined its swarm.
func (s *Service) indexAhead(version library.Version) {
	if _, ok := s.indexes.Get(version.ID); ok {
		return
	}
	if _, failed := s.unindexed.Get(version.ID); failed {
		return
	}
	if s.idleTorrent(Background(context.Background()), version) {
		return
	}
	s.doShared("index ahead "+version.ID.String(), func() (any, error) {
		ctx, cancel := context.WithTimeout(Background(context.Background()), indexAheadTime)
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
// is closed: the next PlaybackInfo plays another version. A host asking to
// slow down tells nothing of the file: the version is kept. A torrent's
// route failing is told as such, without the addon, which was not asked.
func (s *Service) watchSource(version library.Version, src *source.Source, done <-chan struct{}) {
	failed := src.Failed()
	select {
	case <-failed:
		err := failureOf(src, failed)
		if errors.Is(err, source.ErrUnavailable) && !errors.Is(err, source.ErrSlowDown) {
			s.fail(version, err)
		}
		if torrent.RouteFailure(err) {
			s.logger.Info("A version's source failed while it played", "answer", source.Answer(err), "error", err)
			return
		}
		s.logger.Info("A version's source failed while it played", "addon", version.AddonLog, "answer", source.Answer(err))
	case <-done:
	}
}
