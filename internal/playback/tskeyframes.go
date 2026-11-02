package playback

import (
	"context"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// An MPEG-TS file has no keyframe index: FFmpeg finds its keyframes only
// by reading it whole. That of a local folder or a network share, on the
// server's disk or network, is read whole once: in the background, as a
// play is prepared when its title's details open, or at its first play
// over HLS, which waits for the read only scanWait: a play that comes
// before the read is done is converted, cut every few seconds, while the
// read goes on, and the next play copies. Its keyframes are kept as an
// index read from a Matroska file is (see scannedKeyframes): it is then
// cut on them, so that its video may be copied. A play keeps the cut it
// started with to its end (see playPlan). Jellyfin 12.4 copies such video
// too, though it reads keyframes only of .mkv files (EncodingOptions.cs
// L63): its playlist then lists segments of a fixed length
// (DynamicHlsPlaylistGenerator.cs L37-L47), which FFmpeg's HLS muxer ends
// on the next keyframe past their length (DynamicHlsController.cs L1642,
// L1853-L1866), so that a segment does not hold the times its playlist
// gives. An addon's MPEG-TS stream is not read whole, nor a recording,
// which may still grow: they are cut every few seconds, their video
// converted (see hls.NewGridPlan).

const (
	// scanTime bounds the read of an MPEG-TS file's keyframes, which reads
	// the file whole: a few GB a minute over a network share, many more on
	// a local disk. A file that takes longer plays converted, as one whose
	// keyframes cannot be read.
	scanTime = 3 * time.Minute
	// scanWait is how long a play waits for the read of its file's
	// keyframes, which then goes on in the background: about what ffprobe
	// takes to read a recording of an hour or two, a few GB, from a local
	// SSD, as long as a slow analysis takes. A file on a hard disk or a
	// share, read at 100 to 200 MB a second, is read so far only up to a
	// GB or so: its first play is converted, unless its title's details
	// opened earlier, which reads it ahead.
	scanWait = 5 * time.Second
	// scanFailureTTL is how long a file whose keyframes could not be read
	// is not read again, and how long a play's cut is kept unused (see
	// playPlan): longer than a play, so that a play converting it keeps
	// its plan to the end, and its next ones do not wait again.
	scanFailureTTL = 6 * time.Hour
)

// scansKeyframes reports whether the keyframes of a version analyzed so
// are read from the whole file (see scannedKeyframes): an MPEG-TS file of
// a local folder or a network share, which FFmpeg reads as one file.
func (s *Service) scansKeyframes(version library.Version, analysis media.Analysis) bool {
	return transportStream(analysis) && strings.HasPrefix(version.URL, s.localBase())
}

// playPlanKey names a play of a version: its play session.
type playPlanKey struct {
	session string
	version accounts.ID
}

// playPlan is the plan of a play of a version whose keyframes are read
// from the whole file (see scansKeyframes), as its play session first got
// it: cut every few seconds when the read was not done in time, on the
// keyframes once it is. A play keeps it to its end, though the read ends
// meanwhile: its playlist and segments keep their times. Other versions'
// plans do not change: they are Plan's.
func (s *Service) playPlan(ctx context.Context, remux Remux) (hls.Plan, error) {
	analysis, analyzed := s.Analyzed(ctx, remux.Version.ID)
	if remux.Session == "" || !analyzed || !s.scansKeyframes(remux.Version, analysis) {
		return s.Plan(ctx, remux.Version)
	}
	key := playPlanKey{remux.Session, remux.Version.ID}
	if plan, ok := s.playPlans.Get(key); ok {
		s.playPlans.Put(key, plan)
		return plan, nil
	}
	plan, err := s.Plan(ctx, remux.Version)
	if err != nil {
		return hls.Plan{}, err
	}
	// Two first requests at once: the plan kept first is the play's.
	s.playPlansMu.Lock()
	defer s.playPlansMu.Unlock()
	if kept, ok := s.playPlans.Get(key); ok {
		plan = kept
	}
	s.playPlans.Put(key, plan)
	return plan, nil
}

// scannedKeyframes returns the keyframe times of a version whose keyframes
// are read from the whole file (see scansKeyframes): remembered, saved, or
// read by ffprobe from the file served on the loopback interface, and then
// saved as a file's index is. The read is shared, and goes on when the
// request that started it is canceled, within scanTime. A file whose
// keyframes cannot be read is not read again for scanFailureTTL.
func (s *Service) scannedKeyframes(ctx context.Context, version library.Version) ([]time.Duration, error) {
	if times, ok := s.indexes.Get(version.ID); ok {
		return times, nil
	}
	if err, failed := s.unscanned.Get(version.ID); failed {
		return nil, err
	}
	result, err := s.shared(ctx, "keyframes "+version.ID.String(), func() (any, error) {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), scanTime)
		defer cancel()
		if times, ok := s.savedKeyframes(ctx, version.ID); ok {
			return times, nil
		}
		started := time.Now()
		times, err := s.prober.Keyframes(ctx, version.URL)
		if err != nil {
			s.unscanned.Put(version.ID, err)
			s.logger.Info("The keyframes of an MPEG-TS file could not be read: it plays converted", "addon", version.Addon, "error", err,
				"duration", time.Since(started).Round(time.Millisecond))
			return nil, err
		}
		s.saveKeyframes(ctx, version.ID, times)
		s.logger.Info("Read the keyframes of an MPEG-TS file", "addon", version.Addon, "keyframes", len(times),
			"duration", time.Since(started).Round(time.Millisecond))
		return times, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]time.Duration), nil
}
