package playback

import (
	"context"
	"time"
)

// The subtitle tracks read whole through a version's index are kept in
// the database for later playbacks (see readTrack). Those no playback used
// for TrackKeepDays, of a version not analysed in that time either, are
// deleted: a later playback reads them again through the version's index,
// which stays.
const (
	// TrackKeepDays is how many days a track is kept after its last use.
	TrackKeepDays = 60
	// TrackSweepInterval is how often the tracks past their days are
	// deleted, by default.
	TrackSweepInterval = 24 * time.Hour
)

// SweepSubtitleTracks deletes the tracks read whole that no playback used
// for TrackKeepDays, a track's read_at telling when it was read or last
// used (see keptTrack), but those of a version analysed in that time, and
// returns how many it deleted.
func (s *Service) SweepSubtitleTracks(ctx context.Context) (int64, error) {
	tag, err := s.db.Exec(ctx, `DELETE FROM media_subtitle_tracks t
		WHERE t.read_at < now() - make_interval(days => $1)
		AND NOT EXISTS (SELECT 1 FROM media_analyses a WHERE a.version_id = t.version_id AND a.analyzed_at >= now() - make_interval(days => $1))`,
		TrackKeepDays)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
