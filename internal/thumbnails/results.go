package thumbnails

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// The last result of making each version's images is kept (see
// thumbnail_results), for the title's versions page and Health to tell,
// across restarts, the versions whose file cannot give them.

// States of a version's images, as Results tells them: none made nor
// tried yet, waiting in the queue, being made, made, cannot be made from
// its file, or failed, worth trying again.
const (
	StateNone    = "none"
	StateWaiting = "waiting"
	StateWorking = "working"
	StateMade    = "made"
	StateCannot  = "cannot"
	StateFailed  = "failed"
)

// Reasons of a result: why a file cannot give images, or "error" for a
// failure; empty for images made.
const (
	ReasonNoIndex    = "no_index"
	ReasonUnreadable = "unreadable"
	ReasonNoVideo    = "no_video"
	ReasonError      = "error"
)

// Result is where the images of a version stand: State, Reason, the
// thumbnails kept when they were made, and when the result came, nil for
// none.
type Result struct {
	State  string
	Reason string
	Count  int
	At     *time.Time
}

// kind is the kind of file a version is, as its result keeps it.
func kind(version library.Version) string {
	switch {
	case local(version):
		return "local"
	case version.Torrent != nil:
		return "torrent"
	}
	return "link"
}

// outcome is the result of making images that failed with err: cannot,
// for a reason trying again would meet again, else failed. A file ffprobe
// cannot read is unreadable, and one it reads without a video track has
// no video.
func outcome(err error) (state, reason string) {
	switch {
	case errors.Is(err, container.ErrNoIndex):
		return StateCannot, ReasonNoIndex
	case errors.Is(err, container.ErrUnreadable), errors.Is(err, media.ErrUnreadable):
		return StateCannot, ReasonUnreadable
	case errors.Is(err, errNoVideo), errors.Is(err, media.ErrNotMedia):
		return StateCannot, ReasonNoVideo
	}
	return StateFailed, ReasonError
}

// record keeps the result of making a version's images, replacing the one
// it had. A version made again, with the same thumbnails, keeps the time
// of its result.
func (s *Service) record(ctx context.Context, version library.Version, state, reason string, count int) {
	_, err := s.DB.Exec(ctx, `INSERT INTO thumbnail_results (version_id, item_id, kind, state, reason, thumbnails)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (version_id) DO UPDATE SET item_id = excluded.item_id, kind = excluded.kind, state = excluded.state,
			reason = excluded.reason, thumbnails = excluded.thumbnails, at = now()
		WHERE (thumbnail_results.state, thumbnail_results.reason, thumbnail_results.thumbnails) IS DISTINCT FROM
			(excluded.state, excluded.reason, excluded.thumbnails)`,
		version.ID, version.Item, kind(version), state, reason, count)
	if err != nil && ctx.Err() == nil {
		s.Logger.Warn("The result of making the images of a version could not be saved", "error", err)
	}
}

// recordMade keeps that a version's images were made, with the thumbnails
// it has at the settings' width.
func (s *Service) recordMade(ctx context.Context, version library.Version) {
	count := 0
	if settings := s.Settings(); settings.Trickplay {
		info, _, err := s.trickplaySet(ctx, version.ID, settings.TrickplayWidth)
		if err != nil {
			if ctx.Err() == nil {
				s.Logger.Warn("Reading the images of a version failed", "error", err)
			}
			return
		}
		count = info.ThumbnailCount
	}
	s.record(ctx, version, StateMade, "", count)
}

// recordFailure keeps that making a version's images failed with err.
func (s *Service) recordFailure(ctx context.Context, version library.Version, err error) {
	state, reason := outcome(err)
	s.record(ctx, version, state, reason, 0)
}

// cannot reports whether a version's last result tells that its file
// cannot give images.
func (s *Service) cannot(ctx context.Context, version accounts.ID) (bool, error) {
	var state string
	err := s.DB.QueryRow(ctx, "SELECT state FROM thumbnail_results WHERE version_id = $1", version).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return state == StateCannot, err
}

// Results tells where the images of versions stand: waiting or being made
// as the queue holds them, else their last result, StateNone without one.
func (s *Service) Results(ctx context.Context, versions []accounts.ID) (map[accounts.ID]Result, error) {
	results := make(map[accounts.ID]Result, len(versions))
	if len(versions) == 0 {
		return results, nil
	}
	rows, err := s.DB.Query(ctx, "SELECT version_id, state, reason, thumbnails, at FROM thumbnail_results WHERE version_id = ANY($1)", versions)
	if err != nil {
		return nil, err
	}
	var id accounts.ID
	var result Result
	var at time.Time
	_, err = pgx.ForEachRow(rows, []any{&id, &result.State, &result.Reason, &result.Count, &at}, func() error {
		result.At = new(at)
		results[id] = result
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range versions {
		switch {
		case s.working && s.generating == id:
			results[id] = Result{State: StateWorking}
		case s.waiting(id):
			results[id] = Result{State: StateWaiting}
		default:
			if _, ok := results[id]; !ok {
				results[id] = Result{State: StateNone}
			}
		}
	}
	return results, nil
}

// waiting reports whether a version waits in the queue; the caller holds
// s.mu.
func (s *Service) waiting(version accounts.ID) bool {
	for _, j := range s.jobs {
		if j.version.ID == version {
			return true
		}
	}
	return false
}

// LocalCounts counts the files of local folders by their last result:
// made, cannot be made, failed.
func (s *Service) LocalCounts(ctx context.Context) (made, cannot, failed int, err error) {
	err = s.DB.QueryRow(ctx, `SELECT count(*) FILTER (WHERE state = 'made'), count(*) FILTER (WHERE state = 'cannot'),
		count(*) FILTER (WHERE state = 'failed') FROM thumbnail_results WHERE kind = 'local'`).Scan(&made, &cannot, &failed)
	return made, cannot, failed, err
}

// forget drops a version's thumbnails, chapter images and last result.
func (s *Service) forget(ctx context.Context, version accounts.ID) error {
	return pgx.BeginFunc(ctx, s.DB, func(tx pgx.Tx) error {
		// Tiles and chapter images go with their version.
		if _, err := tx.Exec(ctx, "DELETE FROM thumbnail_versions WHERE version_id = $1", version); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "DELETE FROM thumbnail_results WHERE version_id = $1", version)
		return err
	})
}
