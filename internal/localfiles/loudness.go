package localfiles

import (
	"context"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/loudness"
)

// measured is a track of a music folder to measure: its identifier and
// path, and the size and modification time of its file when it was read.
type measured struct {
	id       accounts.ID
	rel      string
	size     int64
	modified time.Time
}

// measureMusic measures, in the background after a scan, the loudness of
// the tracks of a music folder without a track gain, while the settings
// measure local songs (see loudness.Measures): those of known length,
// loudness.MaxLength at most, never measured, or whose last attempt
// failed loudness.RetryAfter ago, fewer than loudness.MaxAttempts times.
// FFmpeg reads them through the loopback interface, as playback does
// (see UseFiles), a meter's places at a time. A folder already being
// measured is left to the measure under way.
func (s *Service) measureMusic(id accounts.ID) {
	if s.meter == nil || s.files == "" || !loudness.Measures(s.settings(), false) {
		return
	}
	s.mu.Lock()
	if s.measuring[id] {
		s.mu.Unlock()
		return
	}
	s.measuring[id] = true
	s.measures.Add(1)
	s.mu.Unlock()
	s.wg.Go(func() {
		defer s.measures.Done()
		defer func() {
			s.mu.Lock()
			delete(s.measuring, id)
			s.mu.Unlock()
		}()
		if err := s.measureTracks(s.ctx, id); err != nil && s.ctx.Err() == nil {
			s.logger.Warn("The loudness of a music folder's tracks could not be measured", "folder", id, "error", err)
		}
	})
}

// measureTracks measures the tracks of a music folder due to be (see
// measureMusic), and keeps each one's loudness, or that it failed. It
// stops once the settings no longer measure local songs.
func (s *Service) measureTracks(ctx context.Context, id accounts.ID) error {
	rows, err := s.db.Query(ctx, `SELECT id, path, size, modified FROM local_tracks
		WHERE addon_id = $1 AND reason = '' AND track_gain IS NULL AND loudness IS NULL AND duration > 0 AND duration <= $2
			AND measure_failures < $3 AND (measure_after IS NULL OR measure_after <= $4)
		ORDER BY path`, id, loudness.MaxLength.Seconds(), loudness.MaxAttempts, s.now())
	if err != nil {
		return err
	}
	var due []measured
	var m measured
	if _, err := pgx.ForEachRow(rows, []any{&m.id, &m.rel, &m.size, &m.modified}, func() error {
		due = append(due, m)
		return nil
	}); err != nil {
		return err
	}
	group, groupCtx := errgroup.WithContext(ctx)
	// The meter bounds the tracks measured at once, over every folder and
	// the addons' songs; this bounds those waiting for it.
	group.SetLimit(2)
	for _, track := range due {
		if !loudness.Measures(s.settings(), false) {
			break
		}
		group.Go(func() error {
			l, err := s.meter.Measure(groupCtx, s.files+track.id.String()+strings.ToLower(path.Ext(track.rel)))
			if groupCtx.Err() != nil {
				return groupCtx.Err()
			}
			// Kept for the file as it was read: one changed meanwhile is
			// measured again.
			if err != nil {
				s.logger.Info("The loudness of a track could not be measured", "path", track.rel, "error", err)
				_, err = s.db.Exec(groupCtx, `UPDATE local_tracks SET measure_failures = measure_failures + 1, measure_after = $5
					WHERE addon_id = $1 AND id = $2 AND size = $3 AND modified = $4`,
					id, track.id, track.size, track.modified, s.now().Add(loudness.RetryAfter))
				return err
			}
			_, err = s.db.Exec(groupCtx, `UPDATE local_tracks SET loudness = $5, measure_failures = 0, measure_after = NULL
				WHERE addon_id = $1 AND id = $2 AND size = $3 AND modified = $4`, id, track.id, track.size, track.modified, l)
			return err
		})
	}
	return group.Wait()
}
