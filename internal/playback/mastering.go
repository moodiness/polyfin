package playback

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// LearnMasteringDisplay learns whether the PQ video of a version carries
// its mastering display (see media.Stream.MasteringDisplay), when only
// that kept conversion, the video's planned conversion, from Intel's tone
// mapping: an analysis made before Polyfin recorded it. ffprobe reads the
// first frame of such a track once, and the answer is kept with the
// analysis; it reports true with the analysis then known, after which the
// conversion is planned again. A frame that cannot be read leaves the
// version on the processor, and is not read again for a while. Other GPUs,
// and video Intel would not tone map anyway, read nothing.
func (s *Service) LearnMasteringDisplay(ctx context.Context, version library.Version, analysis media.Analysis, conversion *VideoConversion) (media.Analysis, bool) {
	if conversion == nil || !conversion.ToneMapOnCPU || conversion.Hardware.Method != "vaapi" || !s.settings().GPUToneMapping ||
		version.HoldsConnection || !slices.ContainsFunc(analysis.Streams, media.NeedsMasteringProbe) {
		return analysis, false
	}
	if _, failed := s.unmastered.Get(version.ID); failed {
		return analysis, false
	}
	result, _, _ := s.flight.Do("mastering "+version.ID.String(), func() (any, error) {
		// The answer is kept even once the request that asked for it is
		// canceled, as an analysis is.
		ctx := context.WithoutCancel(ctx)
		src := s.open(version)
		defer src.Release()
		target, release := s.loopback.register(src)
		defer release()
		learned := analysis
		learned.Streams = slices.Clone(analysis.Streams)
		for i, stream := range learned.Streams {
			if !media.NeedsMasteringProbe(stream) {
				continue
			}
			found, err := s.ffprobe().MasteringDisplay(ctx, target, stream.Index)
			if err != nil {
				s.logger.Info("The first frame of an HDR video could not be read: the processor tone maps it", "addon", version.Addon, "error", err)
				s.unmastered.Put(version.ID, err)
				return analysis, nil
			}
			learned.Streams[i].MasteringDisplay = &found
		}
		if data, err := json.Marshal(learned); err == nil {
			if _, err := s.db.Exec(ctx, "UPDATE media_analyses SET analysis = $2 WHERE version_id = $1", version.ID, data); err != nil {
				s.logger.Warn("Saving a media analysis failed", "error", err)
			}
		}
		s.analyses.Put(version.ID, learned)
		return learned, nil
	})
	learned := result.(media.Analysis)
	return learned, !slices.ContainsFunc(learned.Streams, media.NeedsMasteringProbe)
}
